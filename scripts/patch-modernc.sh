#!/usr/bin/env bash
# 构建期补丁：modernc libc 的 _fstatat_kstat 用老 syscall（SYS_fstat/lstat/stat），
# 被 Android seccomp 白名单 SIGSYS（Android 强制 newfstatat 家族）。
# 用 Go -overlay 机制在构建时替换为语义等价的 newfstatat（现代内核/安卓均支持），
# 不 fork 依赖、不改模块缓存。
# 用法：patch-modernc.sh   → 输出 overlay.json 路径（stdout），patch 文件在 $HC_PATCH_DIR
set -euo pipefail

MOD="modernc.org/libc@v1.77.1"
GOMODCACHE="$(go env GOMODCACHE)"
SRC="$GOMODCACHE/$MOD/ccgo_linux_amd64.go"
[ -f "$SRC" ] || { echo "找不到 $SRC" >&2; exit 1; }

OUT_DIR="${HC_PATCH_DIR:-/tmp/hc-patch}"
mkdir -p "$OUT_DIR"
OUT="$OUT_DIR/ccgo_linux_amd64.go"
OVERLAY="$OUT_DIR/overlay.json"

# 4 处老 syscall → newfstatat（语义等价）：
#   1) SYS_fstat(fd)                  → newfstatat(fd, path, st, AT_EMPTY_PATH)      （空 path 分支）
#   2) SYS_stat(/proc/self/fd 兜底)    → newfstatat(AT_FDCWD, path, st, 0)
#   3) SYS_lstat(path)                 → newfstatat(AT_FDCWD, path, st, AT_SYMLINK_NOFOLLOW)
#   4) SYS_stat(path)                  → newfstatat(AT_FDCWD, path, st, 0)
sed \
  -e 's|ret = int32(X__syscall2(tls, int64(SYS_fstat), int64(fd), int64(bp)))|ret = int32(X__syscall4(tls, int64(SYS_newfstatat), int64(fd), int64(path), int64(bp), int64(AT_EMPTY_PATH)))|' \
  -e 's|ret = int32(X__syscall2(tls, int64(SYS_stat), int64(bp+144), int64(bp)))|ret = int32(X__syscall4(tls, int64(SYS_newfstatat), int64(-100), int64(bp+144), int64(bp), int64(0)))|' \
  -e 's|ret = int32(X__syscall2(tls, int64(SYS_lstat), int64(path), int64(bp)))|ret = int32(X__syscall4(tls, int64(SYS_newfstatat), int64(-100), int64(path), int64(bp), int64(AT_SYMLINK_NOFOLLOW)))|' \
  -e 's|ret = int32(X__syscall2(tls, int64(SYS_stat), int64(path), int64(bp)))|ret = int32(X__syscall4(tls, int64(SYS_newfstatat), int64(-100), int64(path), int64(bp), int64(0)))|' \
  "$SRC" > "$OUT"

# 确认 4 处全部替换且无残留
grep -q "SYS_fstat), int64(fd)" "$OUT" && { echo "PATCH-FAIL: SYS_fstat 残留" >&2; exit 1; }
grep -q "SYS_lstat)" "$OUT" && { echo "PATCH-FAIL: SYS_lstat 残留" >&2; exit 1; }
grep -q "int64(SYS_stat)," "$OUT" && { echo "PATCH-FAIL: SYS_stat 残留" >&2; exit 1; }
[ "$(grep -c "SYS_newfstatat" "$OUT")" -ge 7 ] || { echo "PATCH-FAIL: newfstatat 数量异常" >&2; exit 1; }

cat > "$OVERLAY" <<EOF
{
  "Replace": {
    "$GOMODCACHE/$MOD/ccgo_linux_amd64.go": "$OUT"
  }
}
EOF
echo "$OVERLAY"