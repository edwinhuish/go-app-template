#!/usr/bin/env bash
#
# 解析「这次发布是哪个版本」，供发布流水线的各 job 共用。
#
# 为什么单独成脚本：编译、发布、镜像三个 job 都要知道当前版本，
# 同一条规则抄三遍必然漂移 —— 一处忘了去掉 v 前缀，产物名就和 tag 对不上；
# 一处用分支名当版本，Release 会挂在一个叫 main 的 tag 上。
#
# 用法：
#   scripts/release-version.sh [--tag <tag>]
#
# 版本来源，按优先级：
#   1. --tag 参数        本地试跑用
#   2. INPUT_VERSION     手动触发（workflow_dispatch）时填的版本
#   3. GITHUB_REF        refs/tags/v1.2.3 -> v1.2.3
#   4. 仓库最新 tag       手动触发且没填版本时，按最新 tag 重发一次
#
# 输出（写 $GITHUB_OUTPUT；未设置时打到标准输出，便于本地试跑）：
#   version=1.2.3                     不带 v 前缀，注入 -X main.version
#   tag=v1.2.3                        标签名，Release 挂在这个 tag 上
#   commit=7b2b231                    短提交哈希，注入 -X main.commit
#   build_time=2026-11-10T10:18:49Z   UTC，注入 -X main.date
#
# 版本号只接受 X.Y.Z 或 X.Y.Z-预发布后缀：产物名、镜像 tag、Release tag 都直接取自它，
# 放过一个别的形状，后面三处都会跟着错。

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

die() {
  printf '✗ %s\n' "$1" >&2
  exit 1
}

info() { printf '· %s\n' "$1" >&2; }

# ---- 参数 ----
RAW=""
while [ $# -gt 0 ]; do
  case "$1" in
    --tag)
      [ $# -ge 2 ] || die "--tag 需要一个值"
      RAW="$2"
      shift 2
      ;;
    -*)
      die "未知参数：$1"
      ;;
    *)
      RAW="$1"
      shift
      ;;
  esac
done

# ---- 确定来源 ----
if [ -z "$RAW" ]; then
  RAW="${INPUT_VERSION:-}"
fi
if [ -z "$RAW" ] && [ "${GITHUB_REF_TYPE:-}" = "tag" ]; then
  RAW="${GITHUB_REF_NAME:-}"
fi
if [ -z "$RAW" ]; then
  case "${GITHUB_REF:-}" in
    refs/tags/*) RAW="${GITHUB_REF#refs/tags/}" ;;
  esac
fi
if [ -z "$RAW" ]; then
  # 手动触发又没填版本：按最新 tag 重发一次同样内容的 Release，比直接失败有用。
  git fetch --prune --tags >/dev/null 2>&1 || info "拉取 tag 失败，只能用本地已有的 tag"
  RAW="$(git tag --sort=-version:refname | head -1 || true)"
fi
[ -n "$RAW" ] || die "无法确定版本号：手动触发请在 version 输入里填 tag（如 v1.2.3），或先打一个 tag"

VERSION="${RAW#v}"
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]] \
  || die "版本号格式不对：$RAW（期望 v1.2.3 或 1.2.3-rc.1）"

TAG="v$VERSION"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo none)"
BUILD_TIME="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

info "version=$VERSION tag=$TAG commit=$COMMIT"

# ---- 输出 ----
emit() {
  if [ -n "${GITHUB_OUTPUT:-}" ]; then
    printf '%s=%s\n' "$1" "$2" >>"$GITHUB_OUTPUT"
  else
    printf '%s=%s\n' "$1" "$2"
  fi
}

emit version "$VERSION"
emit tag "$TAG"
emit commit "$COMMIT"
emit build_time "$BUILD_TIME"
