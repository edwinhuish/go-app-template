#!/usr/bin/env bash
#
# 根目录统一入口的实现：把一条命令派发到 apps/ 下所有子项目的同名任务。
#
# 为什么用脚本遍历而不是根 Taskfile 的 includes：
# includes 要把每个子项目写死在根文件里，新增一端还得回来改一遍；它暴露的也是
# backend:build / frontend:build 这种命名空间任务，而不是「一条命令跑所有端」。
# 这里改成运行时扫 apps/*/Taskfile.y*ml：子项目定义了就执行，没定义就跳过，
# 加一个新端不需要动根目录任何文件。
#
# 用法：
#   scripts/task-entry.sh <命令> [参数...]
#
# 命令与参数：
#   install [--ci]     装依赖。--ci 走严格模式：子项目优先执行 install:ci
#                      （前端锁文件与 package.json 不一致时直接失败），没有就退回 install
#   lint [--no-fmt]    默认先 fmt 再 lint；--no-fmt 只检查不格式化
#   build / clean / dev / fmt / test / release
#                      不接受参数；release 先跑子项目的 release，再走本地发版流程
#
# 退出码：任一子项目失败即返回非 0（串行模式下后续子项目不再执行）。

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

# 串行：一个失败就停，并说明停在哪一端。候选任务名按顺序取第一个存在的。
run_serial() {
  local proj list name rc
  for proj in $(projects); do
    list="$(project_tasks "$proj")" || return $?
    if name="$(pick_task "$list" "$@")"; then
      info "$proj: $name"
      if task -d "$APPS/$proj" "$name"; then
        :
      else
        rc=$?
        printf '✗ apps/%s 的 %s 失败（后续子项目未执行）\n' "$proj" "$name" >&2
        return "$rc"
      fi
    else
      info "$proj: 未定义 $1，跳过"
    fi
  done
}

# 并行：给 dev 用。任一端退出即收掉其余进程——dev 的各端是配套的，
# 留一半在跑只会让人以为「还在开发中」。
run_parallel() {
  local -a pids=()
  local proj list name pid stopped rc=0

  # 打开 job control：每个后台子任务各自成为一个进程组（组长就是它自己的 pid），
  # 于是回收时可以用 `kill -TERM -<pid>` 把它拉起的进程一并收掉。
  # 注意这改变了 Ctrl-C 的投递范围：终端只送到本脚本这一组，子任务收不到，
  # 所以下面的 trap 必须负责转发（见 stop_children 的注释）。
  set -m

  for proj in $(projects); do
    list="$(project_tasks "$proj")" || return $?
    if name="$(pick_task "$list" "$@")"; then
      info "$proj: $name"
      task -d "$APPS/$proj" "$name" &
      pids+=("$!")
    else
      info "$proj: 未定义 $1，跳过"
    fi
  done

  [ "${#pids[@]}" -gt 0 ] || return 0

  # 只回收本脚本自己起的这几个进程；不按名字匹配，也不碰别人的进程。
  # stopped 记录「被我们主动收掉的」，它们的退出码不算失败。
  stopped=""
  stop_children() {
    local p running
    running="$(jobs -rp)"
    for p in "${pids[@]}"; do
      if printf '%s\n' "$running" | grep -qx "$p"; then
        stopped="$stopped $p"
        # 连进程组一起收：只 kill 外层 task 进程的话，它拉起的进程（air、vite、
        # 被启动的服务）会变成孤儿继续占着端口。
        kill -TERM -"$p" 2>/dev/null || kill -TERM "$p" 2>/dev/null || true
      fi
    done
  }
  # 开了 job control 之后，终端 Ctrl-C 只送到本脚本这一组，子任务收不到，
  # 所以这里必须显式转发一次，否则 Ctrl-C 只退脚本、开发进程留在后台。
  trap 'stop_children; exit 130' INT TERM

  # 轮询「还有几个在跑」用 jobs -r，不用 kill -0：已经退出但还没 wait 的进程是僵尸，
  # kill -0 对它照样成功，会一直以为它还在跑（dev 就永远收不了尾）。
  # 不用 wait -n：bash 3.2（macOS 自带）没有这个选项。
  while [ -n "$(jobs -rp)" ]; do
    # 有一端先退出了（编译失败、自己关掉…）：dev 的各端是配套的，留一半在跑只会
    # 让人以为「还在开发中」，直接收掉其余进程。
    if [ "$(jobs -rp | wc -l)" -lt "${#pids[@]}" ]; then
      stop_children
      break
    fi
    sleep 1
  done

  for pid in "${pids[@]}"; do
    case " $stopped " in
      *" $pid "*)
        wait "$pid" 2>/dev/null || true
        ;;
      *)
        if ! wait "$pid" 2>/dev/null; then
          rc=1
        fi
        ;;
    esac
  done
  return "$rc"
}

# ---- 命令 ----

COMMAND="${1:-}"
[ -n "$COMMAND" ] || die "用法：scripts/task-entry.sh <install|build|clean|dev|lint|fmt|release|test> [参数]"
shift

# 根 Taskfile 用 "$TASK_ARGS" 传参，没有参数时会传进来一个空串；先把它归一化成「没有参数」。
if [ $# -gt 0 ] && [ -z "$1" ]; then
  shift
fi

case "$COMMAND" in
  install)
    case "${1:-}" in
      '') run_serial install ;;
      --ci) run_serial install:ci install ;;
      *) die "install 只支持 --ci，收到：$1" ;;
    esac
    ;;
  lint)
    case "${1:-}" in
      '')
        run_serial fmt || exit $?
        run_serial lint
        ;;
      --no-fmt) run_serial lint ;;
      *) die "lint 只支持 --no-fmt，收到：$1" ;;
    esac
    ;;
  build | clean | fmt | test)
    [ $# -eq 0 ] || die "$COMMAND 不接受参数，收到：$1"
    run_serial "$COMMAND"
    ;;
  dev)
    [ $# -eq 0 ] || die "dev 不接受参数，收到：$1"
    run_parallel dev
    ;;
  release)
    [ $# -eq 0 ] || die "release 不接受参数（版本号用 VERSION=... 传）"
    # 子项目若自己定义了 release（例如要发布一个包），先让它跑完；再走框架的发版流程。
    run_serial release || exit $?
    exec "$ROOT/scripts/release.sh"
    ;;
  *)
    die "未知命令：$COMMAND（可用：install build clean dev lint fmt release test）"
    ;;
esac
