# SIGSYS(SYS_lstat) on Android/amd64 still crashes in v1.77.1 — #41 was closed without a fix

## Summary

`modernc.org/libc` v1.77.1 still crashes with `SIGSYS: bad system call` on **Android x86_64** (native emulator / Waydroid container) when opening a SQLite database via `modernc.org/sqlite`. This is the exact issue reported in [#41](https://gitlab.com/cznic/libc/-/issues/41) ("Use of SYS_lstat on amd64 hits Android seccomp filters"), which was **closed on 2025-04-10 but never fixed**: there is no commit touching `lstat`/seccomp/Android in the repository history, and the latest release (v1.77.1, 2026-09) still reproduces it.

## Environment

- Go 1.26 (linux/amd64 cross-built as `GOOS=linux GOARCH=amd64` static ELF, run inside an Android x86_64 environment)
- `modernc.org/sqlite v1.60.1` + `modernc.org/libc v1.77.1` (latest)
- Android 13 (API 33), x86_64 — app process under the standard Android seccomp policy

## Reproduce

```go
// minimal: open a SQLite database
import (
  "database/sql"
  _ "modernc.org/sqlite"
)
db, err := sql.Open("sqlite", "/path/to/test.db")
// → process dies with SIGSYS
```

## Crash (as captured in the app's log)

```
2026/10/04 03:31:41 homecast-go listening on http://0.0.0.0:28976 ...
SIGSYS: bad system call
PC=0x40d00e m=0 sigcode=1

goroutine 1 gp=0x... [syscall]:
syscall.Syscall(0x6, ...)            // 0x6 = SYS_lstat (amd64)
internal/syscall/unix...
modernc.org/libc.X__syscall2(...)
modernc.org/libc._fstatat_kstat(...)
modernc.org/libc.X__fstatat(...)
modernc.org/libc.Xfstatat(...)
modernc.org/libc.Xlstat(...)
modernc.org/sqlite/lib._unixFullPathname(...)
modernc.org/sqlite/lib._sqlite3OsFullPathname(...)
modernc.org/sqlite/lib._sqlite3PagerOpen(...)
...
```

## Root cause

The musl translation (`ccgo` output, `ccgo_linux_amd64.go`) keeps **legacy syscall branches** in `_fstatat_kstat` for old-kernel compatibility:

```go
if (fd == -100 || path[0] == '/') && flag == AT_SYMLINK_NOFOLLOW {
    ret = X__syscall2(SYS_lstat, path, bp)   // ← legacy syscall 6
} else if (fd == -100 || path[0] == '/') && flag == 0 {
    ret = X__syscall2(SYS_stat, path, bp)    // ← legacy syscall 4
} else {
    ret = X__syscall4(SYS_newfstatat, ...)
}
```

Android's seccomp whitelist for app processes **does not include the legacy `SYS_lstat`/`SYS_stat` numbers** (it mandates the `newfstatat` family), so the process is killed with `SIGSYS`/`SYS_SECCOMP`.

Note: **arm64 is unaffected** — `ccgo_linux_arm64.go` has no legacy `SYS_lstat`/`SYS_stat` branches (only `SYS_fstat` + `SYS_newfstatat`, both allowed on Android). This is strictly an x86_64-Android issue.

## Suggested fix (generator-level, not patch of generated code)

Since `ccgo_linux_amd64.go` is generated, patching the file would be overwritten on regeneration. Better: make **ccgo drop the legacy stat branches when targeting Android** — for `GOOS=android` the `_fstatat_kstat` branches can all use `SYS_newfstatat`, which is semantically equivalent on every kernel from 2.6.16 onwards:

```go
fmt.Printf(`
if (fd == -100 || path[0] == '/') && flag == AT_SYMLINK_NOFOLLOW {
    ret = X__syscall4(SYS_newfstatat, AT_FDCWD, path, bp, AT_SYMLINK_NOFOLLOW)
} else if (fd == -100 || path[0] == '/') && flag == 0 {
    ret = X__syscall4(SYS_newfstatat, AT_FDCWD, path, bp, 0)
} else {
    ...
}
`)
```

(or alternatively handle the `AT_EMPTY_PATH` case with `newfstatat(fd, "", st, AT_EMPTY_PATH)` instead of `SYS_fstat`).

## Why this matters

`modernc.org/sqlite` is the only CGO-free SQLite driver for Go; without this fix, **any Android x86_64 target (emulators, x86-only Android devices, Waydroid)** cannot use it at all. The arm64 path being clean means most real devices work, but the x86_64 emulator/testing story stays broken until the generator is fixed or a Go-level override is provided for Android.

References: [#41](https://gitlab.com/cznic/libc/-/issues/41), golang/go#27797 (Go's own workaround history for the same class of issue).