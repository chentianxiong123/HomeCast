## SIGSYS(syscall 6, SYS_lstat) on Android x86_64 — modernc.org/sqlite crash, reproduced

### Summary

Opening a SQLite database via `modernc.org/sqlite` (driver backend `modernc.org/libc`) kills the process with `SIGSYS: bad system call` on **Android x86_64** (native emulator / Waydroid container). The offending syscall is the legacy `SYS_lstat` (6), which Android's seccomp policy for app processes does not allow.

### Environment

- Go 1.26, static ELF (`GOOS=linux GOARCH=amd64`), running inside an Android x86_64 (API 33) app process
- `modernc.org/sqlite v1.60.1` + `modernc.org/libc v1.77.1` (latest)
- Reproduced in: homecast (a home music streaming server, Go + htmx). The embedded server runs in the app process and dies on first SQLite open.

### Reproduce

```go
db, err := sql.Open("sqlite", "/path/x.db") // or the driver's internal full-pathname step
```

### Crash

```
homecast-go listening on http://0.0.0.0:28976 ...
SIGSYS: bad system call
PC=0x40d00e m=0 sigcode=1

goroutine 1 [syscall]:
syscall.Syscall(0x6, ...)                  // 0x6 = SYS_lstat (amd64)
modernc.org/libc.X__syscall2(...)
modernc.org/libc._fstatat_kstat(...)
modernc.org/libc.X__fstatat(...)
modernc.org/libc.Xfstatat(...)
modernc.org/libc.Xlstat(...)
modernc.org/sqlite/lib._unixFullPathname(...)
```

### Root cause

The ccgo-generated musl translation (`ccgo_linux_amd64.go`) keeps **legacy syscall branches** in `_fstatat_kstat` for old-kernel compatibility:

```go
if (fd == -100 || path[0] == '/') && flag == AT_SYMLINK_NOFOLLOW {
    ret = X__syscall2(SYS_lstat, path, bp)   // legacy, kill by Android seccomp
} else if (fd == -100 || path[0] == '/') && flag == 0 {
    ret = X__syscall2(SYS_stat, path, bp)    // legacy
} else {
    ret = X__syscall4(SYS_newfstatat, ...)   // Android allows this family
}
```

Android seccomp whitelist mandates the `newfstatat` family; the legacy numbers are a hard kill → `SIGSYS`/`SYS_SECCOMP`.

**arm64 is unaffected**: `ccgo_linux_arm64.go` has no `SYS_lstat`/`SYS_stat` branches (only `SYS_fstat` + `SYS_newfstatat`, both allowed). Real arm64 Android devices work; this is strictly an x86_64-Android issue (emulator / Waydroid testing story).

### Upstream

This is the same issue as https://gitlab.com/cznic/libc/-/issues/41 (closed 2025-04 without a fix; no commit touching lstat/seccomp/android exists, and v1.77.1 2026-09 still reproduces). Filing here as a tracked reproduction record for homecast.