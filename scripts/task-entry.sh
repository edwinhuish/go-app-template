#!/usr/bin/env bash
#
# 根目录「所有端」入口的实现：把一条命令派发到 apps/ 下每个子项目的同名任务。
#
# 「单个端」不走这里：`task server:build` 由根 Taskfile 的 includes 直接把任务名和参数
# 交给 apps/server/Taskfile.yml，根目录没有任何代码参与，也就没有解析与改写。
# 本脚本只负责**全量**派发（task build / task dev …）和框架级的 release。
#
# 用法：
#   scripts/task-entry.sh <命令> [参数...]
#
# 参数规则只有一条：一个不改地转给每个子项目的同名任务（内部就是
# `task -d apps/<名字> <任务> -- <参数...>`）。根不校验、不过滤、不重排、不猜含义，
# 也不因为「没见过这个开关」报错：这些参数可能是给旁边那个子项目的，
# 认不认识、要不要用，由各端自己决定（不认识的忽略）。
#
# 根自己没有业务开关：7 个命令（install / build / clean / dev / fmt / lint / test）走的是
# 同一段逻辑，没有命令专属的解析、没有串联步骤、也不区分执行模式。唯一的例外是 release，
# 它有自己的取值（第一个裸词 = 版本号，另有 --remote= / --push= / --dry-run），见那个分支。
#
# 命名的两种形状（app 名进任务名，不进位置参数）：
#   task server:build                  # 单个端：includes 直达，参数零解析
#   task build                          # 所有端：本脚本派发
#
# 命令：
#   install / build / clean / dev / fmt / lint / test
#     逐个跑各子项目的同名任务，参数原样转给每一个。
#   release
#     框架级的发版动作：先跑子项目的 release（若有），再走本地发版流程。
#
# 退出码：任一子项目失败即返回非 0（后续子项目不再执行）。
# 某个子项目没定义该任务就跳过，这是设计（新端可以只实现一部分命令）。

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

APPS="$ROOT/apps"

die() {
  printf '✗ %s\n' "$1" >&2
  exit 1
}

info() { printf '· %s\n' "$1"; }

# ---- 子项目发现 ----

# 列出 apps/ 下带 Taskfile 的子项目目录名（glob 展开已按名字排序，执行顺序因此稳定）。
projects() {
  local f
  for f in "$APPS"/*/Taskfile.yml "$APPS"/*/Taskfile.yaml; do
    [ -f "$f" ] || continue
    basename "$(dirname "$f")"
  done
}

# 取子项目的任务列表。Taskfile 本身有问题时直接失败——静默当成「没定义这个任务」
# 会让人以为这个端被跳过了，实际是它坏了。
project_tasks() {
  local out
  if ! out="$(task -d "$APPS/$1" --list-all 2>&1)"; then
    printf '✗ 读取 apps/%s 的任务列表失败：\n%s\n' "$1" "$out" >&2
    return 1
  fi
  printf '%s\n' "$out"
}

# 从任务列表里挑第一个存在的候选任务名，都没有则返回 1。
pick_task() {
  local list="$1" candidate
  shift
  for candidate in "$@"; do
    if printf '%s\n' "$list" | grep -qE "^\* ${candidate}:([[:space:]]|$)"; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done
  return 1
}

# ---- 执行 ----

# 唯一的派发逻辑：遍历 apps/ 下的子项目，跑同名任务，把同一份参数原样转给每一个。
#
# 除 release 之外的 7 个命令在实现上没有任何区别：不区分执行模式、不认命令名的特殊含义、
# 不做串联步骤。想只跑一端就点名（`task server:build`），想要多端同时活着就各开一个终端
# （`task server:dev` / `task web:dev`）——这些都由任务名表达，不由脚本猜。
#
# 用法：run <任务名> [参数...]
# 任务名之后的所有参数原样转给子项目的同名任务，不检查、不改写：
# 它们可能是给旁边那个子项目的，能不能用由各端自己判断。
#
# 某个子项目没定义这个任务就跳过：新端可以先只实现一部分命令。
run() {
  local task_name="${1:-}"
  [ -n "$task_name" ] || die "内部错误：run 缺少任务名"
  [ $# -gt 0 ] && shift

  local proj list name rc
  for proj in $(projects); do
    list="$(project_tasks "$proj")" || return $?
    if name="$(pick_task "$list" "$task_name")"; then
      info "$proj: $name"
      # 有参数才加 `--`：Task 会把 `--` 之后的全部塞进子项目的 CLI_ARGS，
      # 因此含空格、含 =、以 - 开头的参数都不会被它自己当成开关。
      # 前台执行：一个端失败就停，并说明停在哪一端（后面的端不再跑）。
      if [ $# -gt 0 ]; then
        task -d "$APPS/$proj" "$name" -- "$@"
        rc=$?
      else
        task -d "$APPS/$proj" "$name"
        rc=$?
      fi
      if [ "$rc" -ne 0 ]; then
        printf '✗ apps/%s 的 %s 失败（后续子项目未执行）\n' "$proj" "$name" >&2
        return "$rc"
      fi
    else
      info "$proj: 未定义 $task_name，跳过"
    fi
  done
}

# release 是框架级的动作（没有 server:release 这回事），它自己吃掉两类参数：
#   1. 版本号：第一个裸词就是版本号（`task release -- 1.2.3`），也接受 --version=1.2.3；
#   2. 其余取值：--remote=origin / --push=false / --dry-run，值里可能带 . 和 -，
#      所以带值的写成 --key=value，不给位置参数留第二种解释。
#   task release -- 1.2.3 --dry-run
# 也与 CI 的环境变量通道并存（VERSION=1.2.3 task release）；两者都设时以命令行上的为准。
# 认不出来的参数（含第二个及以后的裸词）原样转给子项目的 release 任务，与全局命令一致：
# 不校验、不拦截。
run_release() {
  local arg bare_used=false
  local -a kept=()
  VERSION="${VERSION:-}"
  REMOTE="${REMOTE:-origin}"
  PUSH="${PUSH:-ask}"
  DRY_RUN="${DRY_RUN:-false}"

  for arg in "$@"; do
    case "$arg" in
      --version=*) VERSION="${arg#*=}" ;;
      --remote=*) REMOTE="${arg#*=}" ;;
      --push) PUSH=true ;;
      --push=*) PUSH="${arg#*=}" ;;
      --dry-run) DRY_RUN=true ;;
      --dry-run=*) DRY_RUN="${arg#*=}" ;;
      -*)
        # 认不出的开关原样转给子项目，不猜它是什么。
        kept+=("$arg")
        ;;
      *)
        # 裸词 = 版本号。只认第一个：release 只有一颗版本号，第二个裸词没有别的身份，
        # 原样转给子项目。格式对错交给 release.sh 校验，那里会明确说「期望 1.2.3」。
        if [ "$bare_used" = false ]; then
          VERSION="$arg"
          bare_used=true
        else
          kept+=("$arg")
        fi
        ;;
    esac
  done

  # release.sh 只认环境变量这一条通道（CI 也用它），所以在这里落成环境变量。
  export VERSION REMOTE PUSH DRY_RUN

  if [ "${#kept[@]}" -gt 0 ]; then
    run release "${kept[@]}" || return $?
  else
    run release || return $?
  fi
  exec "$ROOT/scripts/release.sh"
}

# ---- 命令 ----

COMMAND="${1:-}"
[ -n "$COMMAND" ] || die "用法：scripts/task-entry.sh <install|build|clean|dev|lint|fmt|release|test> [参数...]"
shift

# 根 Taskfile 不再把参数拼成字符串当 argv 传进来，而是把 Task 的 CLI_ARGS 原样放进
# TASK_ARGS 环境变量（见根 Taskfile 的 env）。Task 对每个参数按 shell 规则加引号后
# 用空格拼成一串，所以 `eval "set --"` 是唯一无损的反向操作：
#   - 含空格的参数（`-run 'Test Foo'`）不会被拆成两段
#   - 含 = 或以 - 开头的参数不会被剥引号或误认成开关
#   - 空串、含 * / ; / 引号的参数都不会被展开或改写
# 直接运行本脚本（不经 Task）时 TASK_ARGS 为空，命令行参数本来就是分开的，直接用。
if [ $# -eq 0 ] && [ -n "${TASK_ARGS:-}" ]; then
  eval "set -- $TASK_ARGS"
fi

# 直接运行时习惯上会写 `scripts/task-entry.sh build -- --ci`：同一个 `--` 在这里
# 只是分隔符，不构成参数，剥掉一次（经 Task 进来时它早已被 Task 吃掉）。
if [ "${1:-}" = "--" ]; then
  shift
fi

# 这里**不再**从位置参数里认子项目名：点名某个端靠任务名（`task server:build`），
# 由根 Taskfile 的 includes 直接交给那一端。位置参数里的一切都是业务参数，原样往下传。

case "$COMMAND" in
  install | build | clean | dev | fmt | lint | test)
    # 7 个命令走同一段逻辑，没有分支：遍历 apps/、跑同名任务、参数原样转给每一个。
    # 参数不校验、不过滤：--ci、--db=pg、或者某个只有旁边那个子项目才认识的开关，
    # 都原样转下去。要不要用、认不认识，由各端自己决定。
    run "$COMMAND" "$@"
    ;;
  release)
    run_release "$@"
    ;;
  *)
    die "未知命令：$COMMAND（可用：install build clean dev lint fmt release test）"
    ;;
esac
