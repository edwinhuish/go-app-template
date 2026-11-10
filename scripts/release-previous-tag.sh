#!/usr/bin/env bash
#
# 找出「上一个版本」的标签，给变更日志确定提交区间：previous_tag..current_tag。
#
# 用法：
#   scripts/release-previous-tag.sh [--minor] <current_tag>
#
#   --minor  跨版本比较：当前版本是 X.Y.0（升 minor 或升 major）时，找上一个版本线的
#            最新标签，而不是紧邻的上一个标签。1.3.0 的上一个版本是 1.2.x 的最新号，
#            不是 1.2.1 这种零散补丁 —— 否则日志里会漏掉半个版本的内容。
#
# 输出（写 $GITHUB_OUTPUT；未设置时打到标准输出）：
#   previous_tag=<v1.1.3>   首次发版时为空
#
# 只认稳定版本作为「上一个版本」：预发布（-beta / -rc / -alpha / -dev）是给测试用的，
# 拿它当对比基线，正式版的日志会缺掉整段。

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

die() {
  printf '✗ %s\n' "$1" >&2
  exit 1
}

info() { printf '· %s\n' "$1" >&2; }

MINOR_MODE=false
TARGET_TAG=""
while [ $# -gt 0 ]; do
  case "$1" in
    --minor | -minor)
      MINOR_MODE=true
      shift
      ;;
    -*)
      die "未知参数：$1"
      ;;
    *)
      if [ -z "$TARGET_TAG" ]; then
        TARGET_TAG="$1"
      fi
      shift
      ;;
  esac
done

[ -n "$TARGET_TAG" ] || die "用法：scripts/release-previous-tag.sh [--minor] <current_tag>"

# ---- 补齐历史 ----
# Actions 默认只取最近一次提交（浅克隆），tag 也不全；不补齐就算不出「上一个版本」。
if [ -f "$(git rev-parse --git-dir)/shallow" ]; then
  git fetch --prune --unshallow --tags >/dev/null || info "补齐历史失败，只能在现有 tag 范围内比较"
else
  git fetch --prune --tags >/dev/null || info "拉取 tag 失败，只能用本地已有的 tag"
fi

# 按版本号升序取出全部稳定标签。
# 一个都没有不算错误：第一个版本（哪怕它是预发布）本来就没有对比基线，
# 输出空 previous_tag，变更日志按「首次发版」处理即可。
all_tags="$(git tag --sort=version:refname | grep -vE -- '-(beta|rc|alpha|dev)' || true)"

# ---- 解析当前版本 ----
current_base="${TARGET_TAG#v}"
current_base="${current_base%%-*}"
IFS='.' read -r cur_major cur_minor cur_patch <<<"$current_base"
for part in "$cur_major" "$cur_minor" "$cur_patch"; do
  [[ "$part" =~ ^[0-9]+$ ]] || die "标签格式不对：$TARGET_TAG（期望 vX.Y.Z 或 vX.Y.Z-预发布）"
done
info "current: $TARGET_TAG (major=$cur_major minor=$cur_minor patch=$cur_patch)"

# 把版本压成可比较的整数。各段都按十进制解析，避免 08 之类被当成八进制。
version_key() {
  local v="${1#v}" a=0 b=0 c=0
  v="${v%%-*}"
  IFS='.' read -r a b c <<<"$v"
  echo $((10#${a:-0} * 1000000 + 10#${b:-0} * 1000 + 10#${c:-0}))
}

cur_key="$(version_key "$TARGET_TAG")"

# ---- 找上一个版本 ----
previous_tag=""

prev_key=0
while IFS= read -r tag; do
  [ -n "$tag" ] || continue
  [ "$tag" = "$TARGET_TAG" ] && continue
  key="$(version_key "$tag")"
  if [ "$key" -lt "$cur_key" ] && { [ -z "$previous_tag" ] || [ "$key" -gt "$prev_key" ]; }; then
    previous_tag="$tag"
    prev_key="$key"
  fi
done <<<"$all_tags"

if [[ "$TARGET_TAG" == *-* ]]; then
  # 预发布版本：优先用同一 X.Y 系列里最新的稳定版当基线（1.1.0-beta.1 对着 1.1.x 比），
  # 该系列还没有稳定版时，上面算出的「小于它的最新稳定版」就是答案。
  series_tag=""
  while IFS= read -r tag; do
    [ -n "$tag" ] || continue
    v="${tag#v}"
    v="${v%%-*}"
    IFS='.' read -r tag_major tag_minor _ <<<"$v"
    if [ "$tag_major" = "$cur_major" ] && [ "$tag_minor" = "$cur_minor" ]; then
      series_tag="$tag" # 已按版本升序，最后一次命中即最新
    fi
  done <<<"$all_tags"
  if [ -n "$series_tag" ]; then
    previous_tag="$series_tag"
  fi
elif [ "$MINOR_MODE" = true ] && [ "$cur_patch" -eq 0 ]; then
  # --minor 模式下 X.Y.0 取到的正是上一条版本线的最新号：「小于当前的最大稳定标签」
  # 与「上一个 minor / 上一个 major 的最新号」等价，所以不必分情况写两套。
  info "minor mode: X.Y.0 按上一条版本线比较"
fi

# ---- 输出 ----
if [ -z "$previous_tag" ]; then
  info "没找到上一个稳定版本：按首次发版处理"
fi

if [ -n "${GITHUB_OUTPUT:-}" ]; then
  printf 'previous_tag=%s\n' "$previous_tag" >>"$GITHUB_OUTPUT"
else
  printf 'previous_tag=%s\n' "$previous_tag"
fi
info "previous: ${previous_tag:-<none>}"
