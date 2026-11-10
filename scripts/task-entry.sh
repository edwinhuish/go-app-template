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
#   scripts/task-entry.sh <命令> [app 名] [开关]
#
# 参数规则（只有两条）：
#   1. 不带 -- 前缀的第一个参数 = 子项目名，命令只在这个子项目里执行；
#      不带则派发到 apps/ 下所有子项目（没定义该任务的跳过）。
#        scripts/task-entry.sh install backend --ci   # 只装 apps/backend，严格模式
#        scripts/task-entry.sh install --ci           # 所有子项目，严格模式
#   2. 以 -- 开头的参数是命令自己的开关，必须写在 app 名之后（没有 app 名时就是第一个）。
#
# 命令与开关：
#   install [--ci]     装依赖。--ci 会原样传给子项目的 install 任务（严格模式：前端按
#                      锁文件安装、锁文件与 package.json 不一致时直接失败）
#   lint [--no-fmt]    默认先 fmt 再 lint；--no-fmt 只检查不格式化
#   dev                不带 app 名时并行起所有端、不接受开关；点名某个 app 时，开关原样
#                      转给它的 dev（如 task dev -- backend --db=pg --once）
#   build / clean / fmt / test
#                      不接受开关；点名某个 app 时只跑它
#   release            框架级的发版动作，不接受 app 名；先跑子项目的 release（若有），
#                      再走本地发版流程
#
# 退出码：任一子项目失败即返回非 0（串行模式下后续子项目不再执行）。
# 点名了一个没有该任务的子项目时直接失败，而不是像全局模式那样跳过 —— 名字都点出来了，
# 静默跳过等于让人以为跑过了。

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

APPS="$ROOT/apps"

# 目标子项目：命令解析阶段若点名了某个 app 就写在这里，其余情况为空 = 全部子项目。
APP=""

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

# 命令要落到哪些子项目上：点名了就只跑那一个，否则全部。
targets() {
  if [ -n "$APP" ]; then
    printf '%s\n' "$APP"
  else
    projects
  fi
}

# 校验点名的 app：名字合法且 apps/<名字>/Taskfile.y*ml 真的存在。
# 单独成函数是为了在调用点决定退出，而不是在 $(...) 子 shell 里 exit —— 那里退出
# 只会结束子 shell，父脚本会带着空 APP 继续按「全部子项目」跑，等于静默忽略用户点名的对象。
check_app() {
  local name="$1" existing
  case "$name" in
    *[!A-Za-z0-9_-]*)
      printf '✗ app 名不合法：%s（只允许字母、数字、- 和 _）\n' "$name" >&2
      return 1
      ;;
  esac
  if [ -f "$APPS/$name/Taskfile.yml" ] || [ -f "$APPS/$name/Taskfile.yaml" ]; then
    return 0
  fi
  existing="$(projects | tr '\n' ' ')"
  printf '✗ 没有 apps/%s 这个子项目；现有：%s\n' "$name" "${existing:-（apps/ 下没有任何子项目）}" >&2
  printf '  app 名写在 `--` 之后的第一个位置：task <命令> -- %s\n' "$name" >&2
  printf '  命令自己的开关以 -- 开头，例如 --ci / --no-fmt\n' >&2
  return 1
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
#
# --optional：候选任务都没定义时直接跳过（给「默认先 fmt」这种附加步骤用）。
# 不点名的全局模式下本来就跳过；点名了某个 app 时，缺主任务是错误、缺附加步骤才跳过。
#
# `--` 之后是这个任务自己的开关，原样转给子项目任务（`task install -- backend --ci`）。
run_serial() {
  local optional=false
  local -a candidates=() child_args=()
  while [ $# -gt 0 ]; do
    case "$1" in
      --optional)
        optional=true
        shift
        ;;
      --)
        shift
        while [ $# -gt 0 ]; do
          child_args+=("$1")
          shift
        done
        ;;
      *)
        candidates+=("$1")
        shift
        ;;
    esac
  done

  local proj list name rc
  for proj in $(targets); do
    list="$(project_tasks "$proj")" || return $?
    if name="$(pick_task "$list" "${candidates[@]}")"; then
      info "$proj: $name"
      if [ "${#child_args[@]}" -gt 0 ]; then
        task -d "$APPS/$proj" "$name" -- "${child_args[@]}"
        rc=$?
      else
        task -d "$APPS/$proj" "$name"
        rc=$?
      fi
      if [ "$rc" -ne 0 ]; then
        printf '✗ apps/%s 的 %s 失败（后续子项目未执行）\n' "$proj" "$name" >&2
        return "$rc"
      fi
    elif [ "$optional" = true ] || [ -z "$APP" ]; then
      info "$proj: 未定义 ${candidates[0]}，跳过"
    else
      printf '✗ apps/%s 没有定义 %s\n' "$proj" "${candidates[*]}" >&2
      return 1
    fi
  done
}

# 并行：给 dev 用。任一端退出即收掉其余进程——dev 的各端是配套的，
# 留一半在跑只会让人以为「还在开发中」。
run_parallel() {
  # 点名了一个 app 就没有并行可言：直接前台跑，Ctrl-C / TTY 的语义和单端 dev 一致。
  if [ -n "$APP" ]; then
    run_serial "$@"
    return
  fi

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
[ -n "$COMMAND" ] || die "用法：scripts/task-entry.sh <install|build|clean|dev|lint|fmt|release|test> [app 名] [开关]"
shift

# 根 Taskfile 用 "$TASK_ARGS" 把整串 CLI_ARGS 当成一个参数传进来
# （`task install -- backend --ci` 到这里是 "backend --ci" 一段），本地调试时又是一段一段的。
# 统一按空白重新切开：这里只认 app 名与 --xxx 开关，两者都不含空格，拆分不会产生歧义。
#
# 拆完还要剥掉包裹的引号：Task 会对 CLI_ARGS 里的每个参数做一遍 shell-quote，
# 含 = 的参数会被单引号包住（`--db=pg` 到这一步是 `'--db=pg'`）。不剥的话，
# 要么匹配不上开关，要么把带引号的怪字符串原样转给子项目再来一轮加引号。
if [ $# -gt 0 ]; then
  read -r -a _args <<<"$*"
  set --
  for _arg in "${_args[@]}"; do
    _arg="${_arg#[\"\']}"
    _arg="${_arg%[\"\']}"
    set -- "$@" "$_arg"
  done
else
  set --
fi

# 第一个参数不以 -- 开头 = 子项目名，命令只落到它身上（`task install -- backend [--ci]`）。
if [ $# -gt 0 ]; then
  case "$1" in
    --*) ;;
    *)
      check_app "$1" || exit 1
      APP="$1"
      shift
      ;;
  esac
fi

case "$COMMAND" in
  install)
    [ $# -le 1 ] || die "install 最多一个开关（--ci），收到：$*"
    case "${1:-}" in
      '') run_serial install ;;
      # --ci 原样转给子项目的 install：严格模式怎么实现由各端自己决定
      # （前端是 --frozen-lockfile，后端是只下载不 tidy），根不替它们定义。
      --ci) run_serial install -- --ci ;;
      *) die "install 只支持 --ci，收到：$1" ;;
    esac
    ;;
  lint)
    [ $# -le 1 ] || die "lint 最多一个开关（--no-fmt），收到：$*"
    case "${1:-}" in
      '')
        # fmt 是附加步骤：点名的 app 没有 fmt（例如前端）时跳过，不当作错误。
        run_serial --optional fmt || exit $?
        run_serial lint
        ;;
      --no-fmt) run_serial lint ;;
      *) die "lint 只支持 --no-fmt，收到：$1" ;;
    esac
    ;;
  build | clean | fmt | test)
    [ $# -eq 0 ] || die "$COMMAND 不接受开关，收到：$1"
    run_serial "$COMMAND"
    ;;
  dev)
    if [ -n "$APP" ]; then
      # 点名了就只有一个端，"并行"退化成前台直跑；开关原样转给它的 dev
      # （task dev -- backend --db=pg --once）。
      run_parallel dev -- "$@"
    else
      # 全局模式没有统一的 dev 开关：--db=pg 这种只对后端有意义，转给前端就是错。
      [ $# -eq 0 ] || die "dev 的开关要指名给哪个子项目，例如：task dev -- backend $1"
      run_parallel dev
    fi
    ;;
  release)
    [ -z "$APP" ] || die "release 是框架级的发版动作（改版本号、打 tag、推送），不接受 app 名；某一端要跟着发布，请在那一端定义 release 任务"
    [ $# -eq 0 ] || die "release 不接受开关（版本号用 VERSION=... 传）"
    # 子项目若自己定义了 release（例如要发布一个包），先让它跑完；再走框架的发版流程。
    run_serial release || exit $?
    exec "$ROOT/scripts/release.sh"
    ;;
  *)
    die "未知命令：$COMMAND（可用：install build clean dev lint fmt release test）"
    ;;
esac
