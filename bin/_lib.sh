# shellcheck shell=bash
# Shared helpers for new-task / end-task / list-tasks.
# Source this, don't execute.

# --- root resolution ----------------------------------------------------------
# SDEV_INSTALL: where the tool code lives (this lib's parent dir).
SDEV_INSTALL="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# SDEV_HOME: user-data root. Precedence:
#   1. explicit $SDEV_HOME
#   2. $WORKSPACE_ROOT  (legacy alias: combined tool+data root; tests & pre-migration clones)
#   3. ~/.sdev
if [[ -n "${SDEV_HOME:-}" ]]; then
    :
elif [[ -n "${WORKSPACE_ROOT:-}" ]]; then
    SDEV_HOME="$WORKSPACE_ROOT"
else
    SDEV_HOME="$HOME/.sdev"
fi
export SDEV_INSTALL SDEV_HOME

GLOBAL_CONFIG="$SDEV_HOME/core/.task-config.yml"
# shellcheck disable=SC2034  # used by sourcing scripts and future tasks
LOCAL_CONFIG="$SDEV_HOME/core/.task-config.local.yml"
PROJECTS_DIR="$SDEV_HOME/core/projects.d"

# Central, lock-protected state. One ledger is the single source of truth for
# port-offset allocation, the warm worktree pool, and per-task lease/lock state.
# See the "state ledger" section below.
SDEV_STATE_DIR="$SDEV_HOME/state"
STATE_FILE="$SDEV_STATE_DIR/state.yml"
STATE_LOCK="$SDEV_STATE_DIR/lock"          # mkdir(2) lock-dir (portable; no flock)
POOL_DIR="$SDEV_STATE_DIR/pool"            # relocated warm worktrees live here

# Create the SDEV_HOME skeleton and seed the default config if absent. Idempotent.
ensure_home() {
    mkdir -p "$SDEV_HOME/core/projects.d" "$SDEV_HOME/confs" \
             "$SDEV_HOME/projects/_archive" "$SDEV_STATE_DIR" "$POOL_DIR"
    if [[ ! -f "$GLOBAL_CONFIG" && -f "$SDEV_INSTALL/core/.task-config.yml" ]]; then
        cp "$SDEV_INSTALL/core/.task-config.yml" "$GLOBAL_CONFIG"
    fi
}

# Path to a project's registry file (may not exist).
project_config_file() { echo "$PROJECTS_DIR/$1.yml"; }

# Registry file that actually holds repos/conf for a project: the project file
# if present, else the legacy global config (single-project backward compat).
effective_project_file() {
    local pf; pf="$(project_config_file "$1")"
    if [[ -f "$pf" ]]; then echo "$pf"; else echo "$GLOBAL_CONFIG"; fi
}

# Names of all defined projects (basenames of core/projects.d/*.yml).
config_projects() {
    local f
    shopt -s nullglob
    for f in "$PROJECTS_DIR"/*.yml; do basename "$f" .yml; done
}

# Read a value from the global defaults config.
global_get() { yq -r "$1" "$GLOBAL_CONFIG"; }



# All accessors take the project name as their first argument and read the
# project's effective registry file (project file, else legacy global).

config_repos() {   # $1=project
    yq -r '.repos | keys | .[]' "$(effective_project_file "$1")"
}

config_repo_path() {   # $1=project $2=repo
    yq -r ".repos.\"$2\".path" "$(effective_project_file "$1")"
}

config_repo_base() {   # $1=project $2=repo
    yq -r ".repos.\"$2\".default_base" "$(effective_project_file "$1")"
}

config_repo_attr() {   # $1=project $2=repo $3=attr  -> value or ""
    local v; v="$(yq -r ".repos.\"$2\".\"$3\" // \"\"" "$(effective_project_file "$1")")"
    [[ "$v" == "null" ]] && v=""
    echo "$v"
}

# Conf prefix: project value, else a neutral default.
config_conf_prefix() {   # $1=project
    local v; v="$(yq -r '.conf_prefix // ""' "$(effective_project_file "$1")")"
    if [[ -n "$v" && "$v" != "null" ]]; then echo "$v"; else echo "app"; fi
}

# Stack services: project list if non-empty, else global default list.
config_stack_services() {   # $1=project
    local pf n
    pf="$(effective_project_file "$1")"
    n="$(yq -r '(.stack_services // []) | length' "$pf")"
    if [[ "${n:-0}" -gt 0 ]]; then
        yq -r '.stack_services[]' "$pf"
    else
        yq -r '.stack_services[]' "$GLOBAL_CONFIG"
    fi
}

# Base port for a service: project base_ports override, else global default.
config_base_port() {   # $1=project $2=service
    local pf v
    pf="$(effective_project_file "$1")"
    v="$(yq -r ".base_ports.\"$2\" // \"\"" "$pf")"
    if [[ -n "$v" && "$v" != "null" ]]; then echo "$v"; return; fi
    yq -r ".defaults.base_ports.\"$2\"" "$GLOBAL_CONFIG"
}

# Compose template path: project override (relative to SDEV_HOME) or default.
config_template() {   # $1=project
    local v; v="$(yq -r '.template // ""' "$(effective_project_file "$1")")"
    if [[ -n "$v" && "$v" != "null" ]]; then
        echo "$SDEV_HOME/$v"
    else
        echo "$SDEV_INSTALL/bin/templates/docker-compose.yml.tmpl"
    fi
}

# Whether new-task should auto-prune default-template services for this project.
# Only the default template has known service names to prune; custom templates
# own their own service set.
config_uses_default_template() {   # $1=project
    local v; v="$(yq -r '.template // ""' "$(effective_project_file "$1")")"
    [[ -z "$v" || "$v" == "null" ]]
}

config_shell_service() {   # $1=project -> compose service name to exec into
    local v; v="$(yq -r '.default_shell_service // ""' "$(effective_project_file "$1")")"
    if [[ -n "$v" && "$v" != "null" ]]; then echo "$v"; else echo "api"; fi
}

# Whether new-task should wire sdev Claude hooks into the task's settings.
# Default enabled; a project opts out with `hooks: false`.
config_hooks_enabled() {   # $1=project
    local v; v="$(yq -r '.hooks | . == false' "$(effective_project_file "$1")")"
    [[ "$v" == "true" ]] && return 1
    return 0
}

# ---- project resolution --------------------------------------------------------

# Stable per-terminal key for the session-scoped active-project pointer.
session_key() {
    local k="${TERM_SESSION_ID:-}"
    [[ -z "$k" ]] && k="${SSH_TTY:-}"
    [[ -z "$k" ]] && k="${TTY:-}"
    if [[ -z "$k" ]] && [[ -t 0 ]]; then k="$(tty 2>/dev/null || true)"; fi
    [[ -z "$k" ]] && k="default"
    printf '%s' "$k" | tr '/ :.' '____'
}
session_project_dir()     { echo "${TMPDIR:-/tmp}/sdev/$(session_key)"; }
session_project_pointer() { echo "$(session_project_dir)/active-project"; }

# Resolve active project: flag > env > session pointer > default_project > 'default'.
resolve_project() {   # $1 = explicit flag value (may be empty)
    local p="${1:-}"
    [[ -n "$p" ]] && { echo "$p"; return; }
    [[ -n "${SDEV_PROJECT:-}" ]] && { echo "$SDEV_PROJECT"; return; }
    local ptr; ptr="$(session_project_pointer)"
    if [[ -f "$ptr" ]]; then
        local v; v="$(<"$ptr")"
        [[ -n "$v" ]] && { echo "$v"; return; }
    fi
    local d
    if [[ -f "$LOCAL_CONFIG" ]]; then
        d="$(yq -r '.defaults.default_project // ""' "$LOCAL_CONFIG" 2>/dev/null)"
        [[ -n "$d" && "$d" != "null" ]] && { echo "$d"; return; }
    fi
    d="$(yq -r '.defaults.default_project // ""' "$GLOBAL_CONFIG" 2>/dev/null)"
    [[ -n "$d" && "$d" != "null" ]] && { echo "$d"; return; }
    echo "default"
}

# Validate a resolved project exists (project file, or implicit 'default'). Echoes it.
require_project() {   # $1 = project
    local p="${1:-}"
    [[ -n "$p" ]] || die "no project resolved"
    if [[ -f "$(project_config_file "$p")" ]]; then echo "$p"; return; fi
    [[ "$p" == "default" ]] && { echo "$p"; return; }   # implicit single-project fallback
    die "unknown project '$p' (known: $(config_projects | tr '\n' ' ')default)"
}

# ---- env profiles --------------------------------------------------------------
VALID_PROFILES=(local dev staging)

is_valid_profile() {
    printf '%s\n' "${VALID_PROFILES[@]}" | grep -qx "$1"
}

# Absolute path to the conf file for a profile within a project.
profile_conf_file() {   # $1=profile $2=project
    local prefix pdir
    prefix="$(config_conf_prefix "$2")"
    pdir="$SDEV_HOME/confs/$2"
    if [[ -d "$pdir" ]]; then
        echo "$pdir/$prefix.$1.env"
    else
        echo "$SDEV_HOME/confs/$prefix.$1.env"   # legacy flat confs/
    fi
}

# Worktree source directory for a repo: core/<project>/<path>, else legacy core/<path>.
repo_source_dir() {   # $1=project $2=repo_path
    local ns="$SDEV_HOME/core/$1/$2"
    [[ -d "$ns" ]] && { echo "$ns"; return; }
    echo "$SDEV_HOME/core/$2"
}

# Personal override (gitignored) wins over committed default; fall back to "local".
config_default_env() {
    local val
    if [[ -f "$LOCAL_CONFIG" ]]; then
        val="$(yq -r '.defaults.default_env // ""' "$LOCAL_CONFIG" 2>/dev/null)"
        [[ -n "$val" && "$val" != "null" ]] && { echo "$val"; return; }
    fi
    val="$(yq -r '.defaults.default_env // ""' "$GLOBAL_CONFIG" 2>/dev/null)"
    [[ -n "$val" && "$val" != "null" ]] && { echo "$val"; return; }
    echo "local"
}

# Profile pinned on an existing task; accepts an absolute task dir.
task_pinned_profile() {   # $1 = absolute task dir
    local env_file="$1/.env"
    [[ -f "$env_file" ]] || return 0
    grep -E '^APP_ENV=' "$env_file" | head -1 | cut -d= -f2 || true
}

# Resolve the profile for a NEW task: explicit flag wins, else personal/committed default.
resolve_profile_for_new() {
    local p="${1:-}"
    [[ -z "$p" ]] && p="$(config_default_env)"
    is_valid_profile "$p" || die "invalid env profile '$p' (allowed: ${VALID_PROFILES[*]})"
    echo "$p"
}

validate_slug() {
    # kebab-case, 1-50 chars, no leading/trailing dash
    local slug="$1"
    [[ "$slug" =~ ^[a-z0-9]+(-[a-z0-9]+)*$ ]] && [[ ${#slug} -le 50 ]]
}

# Prompt to stderr, echo the answer (or default) to stdout. Used by init/edit.
ask() {   # $1=label $2=default(optional)
    local label="$1" def="${2:-}" ans
    if [[ -n "$def" ]]; then printf '%s [%s]: ' "$label" "$def" >&2
    else printf '%s: ' "$label" >&2; fi
    read -r ans || ans=""
    echo "${ans:-$def}"
}
valid_token() { [[ "$1" =~ ^[A-Za-z0-9._-]+$ ]]; }   # conf prefix, shell service, compose role, service
valid_ref()   { [[ "$1" =~ ^[A-Za-z0-9._/-]+$ ]]; }  # git base branch (allows '/')

compute_next_offset() {
    local step; step="$(global_get '.defaults.port_step')"
    local used=()
    while IFS= read -r env; do
        local o; o="$(grep -E '^PORT_OFFSET=' "$env" | cut -d= -f2)"
        [[ -n "$o" ]] && used+=("$o")
    done < <(find "$SDEV_HOME/projects" -type f -name .env 2>/dev/null)
    local candidate=$step
    while printf '%s\n' "${used[@]}" | grep -qx "$candidate"; do
        candidate=$((candidate + step))
    done
    echo "$candidate"
}

# ==============================================================================
# State ledger — central, lock-protected allocation state
# ==============================================================================
# $STATE_FILE (YAML) is the single source of truth for three things:
#   tasks    : "<project>/<slug>" -> {offset, created_at, lease, lease_holder,
#              pid, proc_token}. A task's port offset is RESERVED here under the
#              lock, which eliminates the compute_next_offset scan race (two
#              concurrent `sdev new` used to read the same free offset before
#              either wrote its .env).
#   pool     : warm worktrees returned by `sdev end --pool`, available for the
#              next `sdev new` to re-brand instead of creating from scratch.
#   pool_seq : monotonic counter naming pooled worktree dirs.
#
# Two kinds of reservation keep a task's offset from being reclaimed:
#   * lease        — a durable reservation with NO live process (a background
#                    agent holding a task across sessions/reboots). Never
#                    auto-reclaimed until explicitly released.
#   * process-lock — pid + proc_token (the process's start-time signature).
#                    Self-heals: once the pid is gone (or reused, detected via a
#                    mismatched start-time), the lock is treated as stale.
#
# Every read-modify-write goes through with_state_lock, a portable mkdir(2)
# lock-dir. flock(1) is deliberately avoided — macOS does not ship it.

_now() { date -u +%Y-%m-%dT%H:%M:%SZ; }

# --- portable lock ------------------------------------------------------------
_STATE_LOCK_HELD=""
_STATE_LOCK_STALE_SECS=10   # only a pid-less lock older than this is force-broken

# mtime epoch of a path (GNU `stat -c`, else BSD/macOS `stat -f`), or "" .
_mtime_epoch() { stat -c %Y "$1" 2>/dev/null || stat -f %m "$1" 2>/dev/null || true; }

# Break the lock ONLY when it is genuinely abandoned:
#   * its holder pid is recorded and that process is dead, OR
#   * it carries no holder pid AND is older than the stale grace period (a holder
#     SIGKILLed between mkdir and writing its pid).
# The pid file is written atomically (mv), so a present pid file is never a
# partial/empty read — that ambiguity used to let a waiter break a live lock.
_state_lock_break_if_stale() {
    local pidf="$STATE_LOCK/pid" pid mt now
    if [[ -f "$pidf" ]]; then
        pid="$(cat "$pidf" 2>/dev/null || true)"
        if [[ -n "$pid" ]]; then
            kill -0 "$pid" 2>/dev/null && return 0      # live holder — keep
            rm -rf "$STATE_LOCK" 2>/dev/null || true    # dead holder — break
            return 0
        fi
    fi
    # No (readable) pid yet: could be a lock mid-setup. Break only if it is old.
    mt="$(_mtime_epoch "$STATE_LOCK")"; now="$(date +%s 2>/dev/null || echo 0)"
    if [[ -n "$mt" && "$now" -ge 0 && $((now - mt)) -ge $_STATE_LOCK_STALE_SECS ]]; then
        rm -rf "$STATE_LOCK" 2>/dev/null || true
    fi
}

_state_lock_release() {
    [[ -n "$_STATE_LOCK_HELD" ]] || return 0
    rm -rf "$STATE_LOCK" 2>/dev/null || true
    _STATE_LOCK_HELD=""
}

# with_state_lock CMD [ARGS...] — run CMD while holding the exclusive state lock.
# Retries with backoff, self-heals a lock abandoned by a dead process, and
# releases on return OR via an EXIT trap (so a die()/crash in the critical
# section can't wedge the lock). Safe inside $(...) — the subshell's EXIT trap
# releases when the substitution ends.
with_state_lock() {
    mkdir -p "$SDEV_STATE_DIR"
    local tries=0
    while ! mkdir "$STATE_LOCK" 2>/dev/null; do
        _state_lock_break_if_stale
        tries=$((tries + 1))
        [[ $tries -gt 600 ]] && die "state lock busy: $STATE_LOCK (remove it if no sdev is running)"
        sleep 0.05
    done
    _STATE_LOCK_HELD="$STATE_LOCK"
    trap '_state_lock_release' EXIT              # arm release BEFORE anything can fail
    # Atomic pid write: readers see either no file or the complete pid, never a
    # half-written/empty one.
    printf '%s\n' "$$" > "$STATE_LOCK/pid.$$.tmp" 2>/dev/null \
        && mv -f "$STATE_LOCK/pid.$$.tmp" "$STATE_LOCK/pid" 2>/dev/null || true
    local rc=0
    "$@" || rc=$?
    _state_lock_release
    trap - EXIT
    return "$rc"
}

# --- process-lock liveness ----------------------------------------------------
# A pid's start-time signature. Two PIDs with the same number but different start
# times (PID reuse) produce different tokens, so a reused PID reads as dead.
_proc_token() { ps -o lstart= -p "$1" 2>/dev/null | tr -s ' ' | sed 's/^ *//; s/ *$//'; }

_proc_alive() {   # $1=pid  $2=expected token (optional)
    local pid="$1" want="${2:-}"
    [[ -n "$pid" && "$pid" != "0" && "$pid" != "null" ]] || return 1
    kill -0 "$pid" 2>/dev/null || return 1
    if [[ -n "$want" && "$want" != "null" ]]; then
        [[ "$(_proc_token "$pid")" == "$want" ]] || return 1
    fi
    return 0
}

# --- ledger primitives (callers must hold the lock) ---------------------------
_live_env_files() { find "$SDEV_HOME/projects" -type f -name .env 2>/dev/null | grep -v "/_archive/" || true; }

# projects/-relative key for a task .env path: "<project>/<slug>" or "<slug>".
_state_key_from_env() { local p="${1#"$SDEV_HOME"/projects/}"; echo "${p%/.env}"; }

state_init() {
    mkdir -p "$SDEV_STATE_DIR" "$POOL_DIR"
    [[ -f "$STATE_FILE" ]] && return 0
    cat > "$STATE_FILE" <<'YAML'
version: 1
seeded: false
pool_seq: 0
tasks: {}
pool: []
YAML
}

# One-time migration: seed the ledger from existing task .env PORT_OFFSETs so a
# fresh ledger never hands out an already-used offset. Idempotent via .seeded.
state_seed_from_env() {
    [[ "$(yq -r '.seeded // false' "$STATE_FILE" 2>/dev/null)" == "true" ]] && return 0
    local env off key
    while IFS= read -r env; do
        [[ -n "$env" ]] || continue
        off="$(grep -E '^PORT_OFFSET=' "$env" 2>/dev/null | head -1 | cut -d= -f2)"
        [[ -n "$off" ]] || continue
        key="$(_state_key_from_env "$env")"
        [[ -n "$key" ]] || continue
        K="$key" OFF="$off" TS="$(_now)" yq -i '
          .tasks[strenv(K)] = {"offset": (strenv(OFF)|tonumber), "created_at": strenv(TS),
            "lease": false, "lease_holder": "", "pid": 0, "proc_token": ""}' "$STATE_FILE"
    done < <(_live_env_files)
    yq -i '.seeded = true' "$STATE_FILE"
}

# free|leased|locked|stale for a ledger task key.
_task_reservation_state() {   # $1=key
    local key="$1" lease pid tok
    lease="$(K="$key" yq -r '.tasks[strenv(K)].lease // false' "$STATE_FILE" 2>/dev/null)"
    [[ "$lease" == "true" ]] && { echo leased; return; }
    pid="$(K="$key" yq -r '.tasks[strenv(K)].pid // 0' "$STATE_FILE" 2>/dev/null)"
    tok="$(K="$key" yq -r '.tasks[strenv(K)].proc_token // ""' "$STATE_FILE" 2>/dev/null)"
    if [[ -n "$pid" && "$pid" != "0" && "$pid" != "null" ]]; then
        if _proc_alive "$pid" "$tok"; then echo locked; else echo stale; fi
        return
    fi
    echo free
}

# Drop ledger tasks that no longer hold a reservation: workspace gone AND not
# leased AND no live process-lock. Frees their offsets — this is the self-heal.
state_reconcile() {
    local key st
    while IFS= read -r key; do
        [[ -n "$key" ]] || continue
        [[ -d "$SDEV_HOME/projects/$key" ]] && continue     # workspace present -> keep
        st="$(_task_reservation_state "$key")"
        case "$st" in
            leased|locked) : ;;                             # reservation valid -> keep
            *) K="$key" yq -i 'del(.tasks[strenv(K)])' "$STATE_FILE" ;;
        esac
    done < <(yq -r '.tasks | keys | .[]' "$STATE_FILE" 2>/dev/null)
}

# Create a bare ledger entry for an existing on-disk task if absent (offset read
# from its .env). Lets lease/hold operate on tasks predating the ledger.
_ensure_task_entry_locked() {   # $1=key
    local key="$1" off
    [[ "$(K="$key" yq -r '.tasks | has(strenv(K))' "$STATE_FILE")" == "true" ]] && return 0
    off="$(grep -E '^PORT_OFFSET=' "$SDEV_HOME/projects/$key/.env" 2>/dev/null | head -1 | cut -d= -f2)"
    [[ -n "$off" ]] || off=0
    K="$key" OFF="$off" TS="$(_now)" yq -i '
      .tasks[strenv(K)] = {"offset": (strenv(OFF)|tonumber), "created_at": strenv(TS),
        "lease": false, "lease_holder": "", "pid": 0, "proc_token": ""}' "$STATE_FILE"
}

# --- offset allocation --------------------------------------------------------
# Reserve the first free port offset for KEY, recording it in the ledger. Runs
# the full seed+reconcile cycle first. MUST run under the lock.
_allocate_offset_locked() {   # $1=key  $2=lease(0/1)  $3=holder
    local key="$1" want_lease="${2:-0}" holder="${3:-}"
    state_init; state_seed_from_env; state_reconcile
    local step; step="$(global_get '.defaults.port_step')"
    # used = ledger offsets ∪ a fresh .env scan (defends against any on-disk task
    # missing from the ledger), numeric-sorted-unique.
    local scan="" e o used candidate
    while IFS= read -r e; do
        [[ -n "$e" ]] || continue
        o="$(grep -E '^PORT_OFFSET=' "$e" 2>/dev/null | head -1 | cut -d= -f2)"
        [[ -n "$o" ]] && scan+="$o"$'\n'
    done < <(_live_env_files)
    used="$( { yq -r '.tasks[].offset' "$STATE_FILE" 2>/dev/null; printf '%s' "$scan"; } | sort -un )"
    candidate=$step
    while printf '%s\n' "$used" | grep -qx "$candidate"; do
        candidate=$((candidate + step))
    done
    local lease_bool="false"; [[ "$want_lease" == "1" ]] && lease_bool="true"
    K="$key" OFF="$candidate" TS="$(_now)" LB="$lease_bool" HLD="$holder" yq -i '
      .tasks[strenv(K)] = {"offset": (strenv(OFF)|tonumber), "created_at": strenv(TS),
        "lease": (strenv(LB)=="true"), "lease_holder": strenv(HLD),
        "pid": 0, "proc_token": ""}' "$STATE_FILE"
    echo "$candidate"
}

# allocate_offset KEY [lease0/1] [holder] -> echoes the reserved offset.
allocate_offset() { with_state_lock _allocate_offset_locked "$@"; }

_free_task_locked() { state_init; K="$1" yq -i 'del(.tasks[strenv(K)])' "$STATE_FILE"; }
# free_task KEY — drop a task's reservation (offset + lease + lock).
free_task() { with_state_lock _free_task_locked "$@"; }

# --- lease / process-lock -----------------------------------------------------
_set_lease_locked() {   # $1=key  $2=holder
    state_init; _ensure_task_entry_locked "$1"
    K="$1" HLD="${2:-}" yq -i '.tasks[strenv(K)].lease = true | .tasks[strenv(K)].lease_holder = strenv(HLD)' "$STATE_FILE"
}
set_lease() { with_state_lock _set_lease_locked "$@"; }

_set_lock_locked() {   # $1=key  $2=pid  $3=token
    state_init; _ensure_task_entry_locked "$1"
    K="$1" P="$2" TK="$3" yq -i '.tasks[strenv(K)].pid = (strenv(P)|tonumber) | .tasks[strenv(K)].proc_token = strenv(TK)' "$STATE_FILE"
}
set_lock() { with_state_lock _set_lock_locked "$@"; }

_clear_reservation_locked() {   # $1=key — drop lease + process-lock, keep offset
    state_init
    [[ "$(K="$1" yq -r '.tasks | has(strenv(K))' "$STATE_FILE")" == "true" ]] || return 0
    K="$1" yq -i '.tasks[strenv(K)].lease = false | .tasks[strenv(K)].lease_holder = "" | .tasks[strenv(K)].pid = 0 | .tasks[strenv(K)].proc_token = ""' "$STATE_FILE"
}
clear_reservation() { with_state_lock _clear_reservation_locked "$@"; }

# Short "leased:holder" / "lock:pid" / "lock:stale" annotation for `sdev ls`.
task_status_label() {   # $1=key
    [[ -f "$STATE_FILE" ]] || { echo ""; return; }
    local key="$1" lease holder pid tok
    [[ "$(K="$key" yq -r '.tasks | has(strenv(K))' "$STATE_FILE" 2>/dev/null)" == "true" ]] || { echo ""; return; }
    lease="$(K="$key" yq -r '.tasks[strenv(K)].lease // false' "$STATE_FILE")"
    if [[ "$lease" == "true" ]]; then
        holder="$(K="$key" yq -r '.tasks[strenv(K)].lease_holder // ""' "$STATE_FILE")"
        if [[ -n "$holder" && "$holder" != "null" ]]; then echo "leased:$holder"; else echo "leased"; fi
        return
    fi
    pid="$(K="$key" yq -r '.tasks[strenv(K)].pid // 0' "$STATE_FILE")"
    tok="$(K="$key" yq -r '.tasks[strenv(K)].proc_token // ""' "$STATE_FILE")"
    if [[ -n "$pid" && "$pid" != "0" && "$pid" != "null" ]]; then
        if _proc_alive "$pid" "$tok"; then echo "lock:$pid"; else echo "lock:stale"; fi
        return
    fi
    echo ""
}

# All leased task keys (shown by `sdev ls` even when their workspace is gone).
leased_task_keys() {
    [[ -f "$STATE_FILE" ]] || return 0
    yq -r '.tasks | to_entries | map(select(.value.lease == true)) | .[].key' "$STATE_FILE" 2>/dev/null
}

# --- warm worktree pool -------------------------------------------------------
# Reserve a uniquely-named pool destination for a repo. Echoes the dest path.
_pool_reserve_slot_locked() {   # $1=project  $2=repo_path
    state_init
    local seq; seq="$(yq -r '.pool_seq // 0' "$STATE_FILE")"; seq=$((seq + 1))
    SEQ="$seq" yq -i '.pool_seq = (strenv(SEQ)|tonumber)' "$STATE_FILE"
    echo "$POOL_DIR/$1/$2.$seq"
}
pool_reserve_slot() { with_state_lock _pool_reserve_slot_locked "$@"; }

_pool_record_locked() {   # $1=project $2=repo $3=repo_path $4=source $5=path
    state_init
    P="$1" R="$2" RP="$3" SRC="$4" PP="$5" TS="$(_now)" yq -i '
      .pool += [{"project": strenv(P), "repo": strenv(R), "repo_path": strenv(RP),
                 "source": strenv(SRC), "path": strenv(PP), "returned_at": strenv(TS)}]' "$STATE_FILE"
}
pool_record() { with_state_lock _pool_record_locked "$@"; }

# Pop the first pooled worktree for SOURCE. Echoes its path (empty if none).
_pool_take_locked() {   # $1=source
    state_init
    local path
    path="$(SRC="$1" yq -r '[.pool[] | select(.source == strenv(SRC))] | .[0].path // ""' "$STATE_FILE" 2>/dev/null)"
    [[ -z "$path" || "$path" == "null" ]] && { echo ""; return; }
    PP="$path" yq -i 'del(.pool[] | select(.path == strenv(PP)))' "$STATE_FILE"
    echo "$path"
}
pool_take() { with_state_lock _pool_take_locked "$@"; }

# Remove a pool entry by path without taking it (used when a stale entry is
# discovered — the on-disk worktree vanished).
_pool_drop_locked() { state_init; PP="$1" yq -i 'del(.pool[] | select(.path == strenv(PP)))' "$STATE_FILE"; }
pool_drop() { with_state_lock _pool_drop_locked "$@"; }

# All pool entry paths (for `sdev ls` / `sdev doctor`).
pool_paths() { [[ -f "$STATE_FILE" ]] || return 0; yq -r '.pool[].path' "$STATE_FILE" 2>/dev/null; }

# Clone (git URL) or symlink (existing local git repo) a repo source into
# core/<project>/<name>. Expands a leading ~. Echoes "cloned" or "linked" on
# success; prints the reason to stderr and returns 1 on failure.
add_repo_source() {   # $1=project $2=name $3=source_spec
    local project="$1" name="$2" spec="${3:-}"
    spec="${spec/#\~/$HOME}"
    local dest="$SDEV_HOME/core/$project/$name"
    if [[ -e "$dest" || -L "$dest" ]]; then
        echo "source already exists at $dest" >&2; return 1
    fi
    mkdir -p "$SDEV_HOME/core/$project"
    if [[ "$spec" == *://* || "$spec" == git@* ]]; then
        if ! git clone -q "$spec" "$dest" >&2; then
            echo "clone failed: $spec" >&2; return 1
        fi
        echo cloned
    elif [[ -d "$spec/.git" || -f "$spec/.git" ]]; then
        local abs; abs="$(cd "$spec" && pwd)"
        ln -s "$abs" "$dest"
        echo linked
    elif [[ -e "$spec" ]]; then
        echo "'$spec' exists but is not a git repo (no .git)" >&2; return 1
    else
        echo "'$spec' is not a git URL or an existing local repo" >&2; return 1
    fi
}

die() { echo "error: $*" >&2; exit 1; }
log() { echo "[$(basename "$0")] $*"; }
