#!/usr/bin/env bash
#
# 发版脚本：问版本号 -> 同步 VERSION 与前端 package.json -> 提交并打 tag -> 问是否推送。
#
# 由 `task release` 调用，参数通过环境变量传入：
#   VERSION   指定版本号（空 = 交互询问，回车取当前版本 +1 patch）
#   REMOTE    远程名，默认 origin
#   PUSH      true | false | ask（默认 ask）
#   DRY_RUN   true 时只打印将要发生什么，不改文件、不提交、不推送
#
# 每一步失败都立刻停下并说明停在哪一步——发版最怕的是「改了一半」：
# 版本号已写、tag 已打、却没推上去，下一个人看到的就是一个对不上的仓库。

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION="${VERSION:-}"
REMOTE="${REMOTE:-origin}"
PUSH="${PUSH:-ask}"
DRY_RUN="${DRY_RUN:-false}"

VERSION_FILE="$ROOT/VERSION"
PKG_FILE="$ROOT/apps/frontend/package.json"

die() {
  printf '✗ %s\n' "$1" >&2
  exit 1
}

info() { printf '· %s\n' "$1"; }
step() { printf '\n== %s ==\n' "$1"; }

# ---- 前置校验：宁可在这里失败，也不要留下半个版本 ----
step "前置校验"

git rev-parse --git-dir >/dev/null 2>&1 || die "当前目录不是 git 仓库"

if [ -n "$(git status --porcelain)" ]; then
  die "工作区有未提交的改动，先提交或 stash 再发版"
fi

if ! git symbolic-ref -q HEAD >/dev/null; then
  die "当前处于 detached HEAD，先切回一个分支"
fi

CURRENT="$(tr -d '[:space:]' <"$VERSION_FILE" 2>/dev/null || echo "0.0.0")"
info "当前版本：$CURRENT"

if git remote get-url "$REMOTE" >/dev/null 2>&1; then
  info "远程 $REMOTE：$(git remote get-url "$REMOTE")"
else
  info "未配置远程 $REMOTE，稍后会跳过推送"
  [ "$PUSH" = "true" ] && die "指定了 PUSH=true，但没有名为 $REMOTE 的远程"
  PUSH="false"
fi

# ---- 版本号 ----
step "确定版本号"

bump_patch() {
  local v="${1#v}" major minor patch
  IFS='.' read -r major minor patch <<<"$v"
  printf '%s.%s.%s' "${major:-0}" "${minor:-0}" "$(( ${patch%%-*} + 1 ))"
}

if [ -z "$VERSION" ]; then
  suggested="$(bump_patch "$CURRENT")"
  printf '版本号 [回车 = %s]：' "$suggested"
  read -r VERSION
  VERSION="${VERSION:-$suggested}"
fi

VERSION="${VERSION#v}"
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] \
  || die "版本号格式不对：$VERSION（期望 1.2.3 或 1.2.3-rc.1）"

TAG="v$VERSION"
info "目标版本：$VERSION（tag $TAG）"

if git rev-parse -q --verify "refs/tags/$TAG" >/dev/null; then
  die "tag $TAG 已存在"
fi

if [ "$DRY_RUN" = "true" ]; then
  printf '\n[dry-run] 将写入 %s 与 %s，提交并打 tag %s，%s\n' \
    "$VERSION_FILE" "$PKG_FILE" "$TAG" \
    "$([ "$PUSH" = "false" ] && echo "不推送" || echo "视确认结果推送")"
  exit 0
fi

# ---- 同步版本号 ----
step "同步版本号"

printf '%s\n' "$VERSION" >"$VERSION_FILE"

# 只替换 package.json 里第一个 "version" 字段（它就在文件顶部）。
if [ -f "$PKG_FILE" ]; then
  sed -i.bak -E "s/(\"version\"[[:space:]]*:[[:space:]]*\")[^\"]*(\")/\1$VERSION\2/" "$PKG_FILE"
  rm -f "$PKG_FILE.bak"
  info "已同步 apps/frontend/package.json"
fi

# ---- 提交 + tag ----
step "提交并打 tag"

if git diff --quiet -- "$VERSION_FILE" "$PKG_FILE"; then
  info "版本号已是 $VERSION，跳过提交，只打 tag"
else
  git add "$VERSION_FILE" "$PKG_FILE"
  git commit -m "chore(release): $TAG"
  info "已提交 chore(release): $TAG"
fi

git tag -a "$TAG" -m "release $TAG"
info "已打 tag $TAG"

# ---- 推送 ----
step "推送"

if [ "$PUSH" = "ask" ]; then
  printf '推送到 %s？[Y/n] ' "$REMOTE"
  read -r answer
  case "${answer:-y}" in
    [nN]*) PUSH="false" ;;
    *) PUSH="true" ;;
  esac
fi

if [ "$PUSH" = "true" ]; then
  git push --atomic "$REMOTE" HEAD "$TAG"
  info "已推送分支与 tag"
else
  info "未推送。稍后可手动执行："
  printf '  git push --atomic %s HEAD %s\n' "$REMOTE" "$TAG"
fi

printf '\n完成：%s\n' "$TAG"
