#!/usr/bin/env bash
#
# 从提交历史生成 Release 正文。
#
# 用法：
#   scripts/release-notes.sh <current_tag> [previous_tag]
#
# previous_tag 为空表示首次发版，取该 tag 可达的全部提交。
#
# 只收 conventional commit 的五类（feat / fix / docs / perf / refactor）：
# chore / test / ci 对使用者没有信息量，写进正文只会把真正的变更淹掉。
# 一条都归不上类时（仓库提交风格不统一）退化成完整列表 —— 宁可列出全部，
# 也不要发出一个空正文的 Release。
#
# 输出：写到 release-notes.md（OUTPUT_FILE 可覆盖），并回显到日志。

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

die() {
  printf '✗ %s\n' "$1" >&2
  exit 1
}

info() { printf '· %s\n' "$1" >&2; }

[ $# -ge 1 ] || die "用法：scripts/release-notes.sh <current_tag> [previous_tag]"

CURRENT_TAG="$1"
PREVIOUS_TAG="${2:-}"
OUTPUT_FILE="${OUTPUT_FILE:-release-notes.md}"

if [ -n "$PREVIOUS_TAG" ]; then
  RANGE="$PREVIOUS_TAG..$CURRENT_TAG"
  info "提交区间：$RANGE"
else
  RANGE="$CURRENT_TAG"
  info "首次发版：取 $CURRENT_TAG 可达的全部提交"
fi

# 提交标题里可能有空格、括号、竖线，用 %x1f（单元分隔符）分字段，避免解析串位。
entries="$(git log --no-merges --pretty=format:"%s%x1f%an%x1f%h" "$RANGE" 2>/dev/null || true)"

declare -A buckets=([feat]="" [fix]="" [docs]="" [perf]="" [refactor]="")
others=""

while IFS=$'\x1f' read -r subject author sha; do
  [ -n "${subject:-}" ] || continue

  type=""
  text=""
  if [[ "$subject" =~ ^(feat|fix|docs|perf|refactor)(\([^\)]*\))?!?:[[:space:]]*(.*)$ ]]; then
    # 组 1 = 类型，组 2 = 可选的作用域，组 3 = 去掉前缀后的说明。
    type="${BASH_REMATCH[1]}"
    text="${BASH_REMATCH[3]}"
  fi

  line="- ${text:-$subject} — @${author} (${sha})"
  if [ -n "$type" ]; then
    buckets[$type]+="$line"$'\n'
  else
    others+="$line"$'\n'
  fi
done <<<"$entries"

notes=""
section() {
  [ -n "$2" ] || return 0
  notes+=$'\n'"### $1"$'\n\n'"$2"
}

section "新功能 (feat)" "${buckets[feat]}"
section "缺陷修复 (fix)" "${buckets[fix]}"
section "文档 (docs)" "${buckets[docs]}"
section "性能优化 (perf)" "${buckets[perf]}"
section "代码重构 (refactor)" "${buckets[refactor]}"

if [ -z "$notes" ]; then
  notes=$'\n'"### 变更"$'\n\n'"${others:-本次发布没有可列出的提交。}"$'\n'
fi

if [ -n "${GITHUB_REPOSITORY:-}" ] && [ -n "$PREVIOUS_TAG" ]; then
  notes+=$'\n'"**完整对比**：https://github.com/${GITHUB_REPOSITORY}/compare/${PREVIOUS_TAG}...${CURRENT_TAG}"$'\n'
fi

# 去掉首尾多余空行：正文开头或结尾多一行空白，Release 页面上就是一段无意义的留白。
while [ -n "$notes" ] && [ "${notes:0:1}" = $'\n' ]; do
  notes="${notes:1}"
done
while [ -n "$notes" ] && [ "${notes: -1}" = $'\n' ]; do
  notes="${notes%$'\n'}"
done

printf '%s\n' "$notes" >"$OUTPUT_FILE"
info "已写入 $OUTPUT_FILE"
cat "$OUTPUT_FILE"
