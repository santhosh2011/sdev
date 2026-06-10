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

# Create the SDEV_HOME skeleton and seed the default config if absent. Idempotent.
ensure_home() {
    mkdir -p "$SDEV_HOME/core/projects.d" "$SDEV_HOME/confs" "$SDEV_HOME/projects/_archive"
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

die() { echo "error: $*" >&2; exit 1; }
log() { echo "[$(basename "$0")] $*"; }
