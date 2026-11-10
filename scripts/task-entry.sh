#!/usr/bin/env bash
#
# 根目录「所有端」入口的实现：把一条命令派发到 apps/ 下每个子项目的同名任务。
#
# 「单个端」不走这里：`task backend:build` 由根 Taskfile 的 includes 直接把任务名和参数
# 交给 apps/backend/Taskfile.yml，根目录没有任何代码参与，也就没有解析与改写。
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
# 根自己只有一个开关：lint 的 --no-fmt（只决定「要不要先跑 fmt 这一步」，摘掉它，
# 其余参数照旧原样转给子项目）。release 的 --version= / --remote= / --push / --dry-run
# 是框架自己的取值开关，见那个分支。
#
# 命名的两种形状（app 名进任务名，不进位置参数）：
#   task backend:build                  # 单个端：includes 直达，参数零解析
#   task build                          # 所有端：本脚本派发
#
# 命令：
#   install / build / clean / dev / fmt / lint / test
#     派发到各子项目的同名任务；dev 并行起所有端（Ctrl-C 一并退出）。
#   release
#     框架级的发版动作：先跑子项目的 release（若有），再走本地发版流程。
#
# 退出码：任一子项目失败即返回非 0（串行模式下后续子项目不再执行）。
# 全局模式下某个子项目没定义该任务就跳过，这是设计（新端可以只实现一部分命令）。

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

# 串行：一个失败就停，并说明停在哪一端。
#
# 用法：run_serial <任务名> [参数...]
# 任务名之后的所有参数原样转给子项目的同名任务，不检查、不改写：
# 它们可能是给旁边那个子项目的，能不能用由各端自己判断。
#
# 某个子项目没定义这个任务就跳过：新端可以先只实现一部分命令，
# 而"点到哪个端"由任务名（backend:build）决定，不由这里猜。
run_serial() {
  local task_name="${1:-}"
  [ -n "$task_name" ] || die "内部错误：run_serial 缺少任务名"
  [ $# -gt 0 ] && shift

  local proj list name rc
  for proj in $(projects); do
    list="$(project_tasks "$proj")" || return $?
    if name="$(pick_task "$list" "$task_name")"; then
      info "$proj: $name"
      # 有参数才加 `--`：Task 会把 `--` 之后的全部塞进子项目的 CLI_ARGS，
      # 因此含空格、含 =、以 - 开头的参数都不会被它自己当成开关。
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

# lint 的编排：默认先 fmt 再 lint。--no-fmt 是根自己的开关，只决定跳过第一步，
# 摘掉它之后其余参数照旧原样转给子项目（哪怕它其实不认识）。
run_lint() {
  local nofmt=false arg
  for arg in "$@"; do
    if [ "$arg" = "--no-fmt" ]; then
      nofmt=true
    fi
  done

  if [ "$nofmt" = true ]; then
    local -a kept=()
    for arg in "$@"; do
      if [ "$arg" != "--no-fmt" ]; then
        kept+=("$arg")
      fi
    done
    # bash 3.2 在 set -u 下展开空数组会报 unbound，所以分成两支。
    if [ "${#kept[@]}" -gt 0 ]; then
      run_serial lint "${kept[@]}"
    else
      run_serial lint
    fi
    return $?
  fi

  # fmt 是附加步骤：没有 fmt 的端（例如前端）自动跳过，不当作错误。
  run_serial fmt "$@" || return $?
  run_serial lint "$@"
}

# release 是框架级的动作（没有 backend:release 这回事），它自己的取值开关一律写成
# --key=value：值里带 . 和 -，写成位置参数就永远分不清是版本号还是别的什么。
#   task release -- --version=1.2.3 --remote=origin --push=false --dry-run
# 也与 CI 的环境变量通道并存（VERSION=1.2.3 task release）；两者都设时以命令行开关为准。
# 不认识的参数原样转给子项目的 release 任务，与全局命令一致：不校验、不拦截。
run_release() {
  local arg
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
      *)
        # 裸词几乎肯定是把版本号写成了位置参数：提示一句，但仍然原样转给子项目。
        case "$arg" in
          -*) ;;
          *)
            printf '· 提示：%s 会被原样转给子项目的 release 任务；框架取值请写 --version=… / --remote=… / --push=… / --dry-run\n' "$arg" >&2
            ;;
        esac
        kept+=("$arg")
        ;;
    esac
  done

  # release.sh 只认环境变量这一条通道（CI 也用它），所以在这里落成环境变量。
  export VERSION REMOTE PUSH DRY_RUN

  if [ "${#kept[@]}" -gt 0 ]; then
    run_serial release "${kept[@]}" || return $?
  else
    run_serial release || return $?
  fi
  exec "$ROOT/scripts/release.sh"
}

# 并行：给 dev 用。任一端退出即收掉其余进程——dev 的各端是配套的，
# 留一半在跑只会让人以为「还在开发中」。
#
# 用法：run_parallel <任务名> [参数...]，参数原样转给每个子项目的同名任务
# （`--db=pg` 这类只对后端有意义的开关，前端收到后自己忽略，不该由根来拦）。
run_parallel() {
  local task_name="${1:-}"
  [ -n "$task_name" ] || die "内部错误：run_parallel 缺少任务名"
  [ $# -gt 0 ] && shift

  local -a pids=()
  local proj list name pid stopped rc=0

  # 打开 job control：每个后台子任务各自成为一个进程组（组长就是它自己的 pid），
  # 于是回收时可以用 `kill -TERM -<pid>` 把它拉起的进程一并收掉。
  # 注意这改变了 Ctrl-C 的投递范围：终端只送到本脚本这一组，子任务收不到，
  # 所以下面的 trap 必须负责转发（见 stop_children 的注释）。
  set -m

  for proj in $(projects); do
    list="$(project_tasks "$proj")" || return $?
    if name="$(pick_task "$list" "$task_name")"; then
      info "$proj: $name"
      if [ $# -gt 0 ]; then
        task -d "$APPS/$proj" "$name" -- "$@" &
      else
        task -d "$APPS/$proj" "$name" &
      fi
      pids+=("$!")
    else
      info "$proj: 未定义 $task_name，跳过"
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

# 这里**不再**从位置参数里认子项目名：点名某个端靠任务名（`task backend:build`），
# 由根 Taskfile 的 includes 直接交给那一端。位置参数里的一切都是业务参数，原样往下传。

case "$COMMAND" in
  install | build | clean | fmt | test)
    # 参数不校验、不过滤：--ci、--db=pg、或者某个只有旁边那个子项目才认识的开关，
    # 都原样转下去。要不要用、认不认识，由各端自己决定。
    run_serial "$COMMAND" "$@"
    ;;
  lint)
    run_lint "$@"
    ;;
  dev)
    # 每个端都收到同一份参数（前端收到 --db=pg 自己忽略），并行起、任一端退出即全收。
    run_parallel dev "$@"
    ;;
  release)
    run_release "$@"
    ;;
  *)
    die "未知命令：$COMMAND（可用：install build clean dev lint fmt release test）"
    ;;
esac
