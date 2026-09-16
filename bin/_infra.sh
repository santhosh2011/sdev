# shellcheck shell=bash
# Shared Postgres primitives. Source _lib.sh first. Lifecycle stays bash-only.
# These helpers form the workspace-adoption contract; no project is opted in.
infra_flavor() {
    local flavor="${1#postgres-}" major
    [[ "$flavor" =~ ^[1-9][0-9]*(-[a-z0-9]+)*$ && ${#flavor} -le 45 ]] || die "invalid Postgres flavor: $1"
    major="${flavor%%-*}"
    (( major >= 13 && major <= 99 )) || die "Postgres flavor must be major 13 or newer (two digits)"
    echo "postgres-$flavor"
}

infra_image() {   # $1=canonical flavor; custom pins need an explicit image
    local pin image
    pin="${1#postgres-}"
    image="$(F="$1" yq -r '.defaults.infra_postgres_flavors[strenv(F)].image // ""' "$GLOBAL_CONFIG")" || return
    if [[ -z "$image" ]]; then
        [[ "$pin" != *-* ]] || die "configure defaults.infra_postgres_flavors.$1.image for this pin"
        image="pgvector/pgvector:pg$pin"
    fi
    [[ "$image" != *[[:space:]]* && "$image" != -* ]] || die "invalid Postgres image"
    echo "$image"
}

infra_compose() {   # $1=flavor; remaining args passed to the infra wrapper
    local flavor="$1"; shift
    ( cd "$SDEV_HOME/infra/$flavor" && ./compose "$@" )
}

infra_entry() {   # $1=flavor $2=field
    [[ -f "$STATE_FILE" ]] || return 0
    F="$1" FIELD="$2" yq -r '.shared_infra[strenv(F)][strenv(FIELD)] // ""' "$STATE_FILE"
}

# Use observable engine state; a failed Docker query must never mean no users.
# Conservative across flavors: any non-infra consumer on sdev-shared protects
# all flavors. Unlabelled/non-Compose containers count by their container id.
infra_consumers() {
    local rows project flavor id
    rows="$(docker ps --filter network=sdev-shared --format '{{.Label "com.docker.compose.project"}}|{{.Label "io.sdev.infra.flavor"}}|{{.ID}}')" || return
    while IFS='|' read -r project flavor id; do
        [[ -n "$id" ]] || continue
        [[ -n "$flavor" && "$project" == "sdev-infra-$flavor" ]] && continue
        printf '%s\n' "${project:-$id}"
    done <<< "$rows" | sort -u
}

infra_guard_owner() {
    local rows id owner
    rows="$(docker ps -a --filter "label=com.docker.compose.project=sdev-infra-$1" --format '{{.ID}}|{{.Label "io.sdev.infra.home"}}')" || return
    [[ -n "$rows" ]] || return 0
    while IFS='|' read -r id owner; do
        [[ -n "$id" ]] || continue
        [[ "$owner" == "$SDEV_HOME" ]] || die "sdev-infra-$1 belongs to another SDEV_HOME; use its owner to manage it"
    done <<< "$rows"
}

# Called only under with_state_lock. Explicit error propagation is required:
# with_state_lock captures status with ||, disabling Bash errexit in functions.
infra_up_locked() {
    local flavor="$1" image offset dir major target
    [[ "${INFRA_NO_BOOT:-0}" == 1 ]] || infra_guard_owner "$flavor" || return
    image="$(infra_entry "$flavor" image)" || return
    [[ -n "$image" ]] || image="$(infra_image "$flavor")" || return
    dir="$SDEV_HOME/infra/$flavor"
    if [[ -d "$dir" && ! -f "$dir/.sdev-infra" ]]; then
        die "unmarked infra directory $dir; refusing to overwrite"
    fi
    offset="$(_allocate_infra_offset_locked "$flavor" "$image")" || return
    if [[ ! -d "$dir" ]]; then
        mkdir -p "$dir" || return
        # Mark before scaffolding: a failed copy is retryable, never booted.
        touch "$dir/.sdev-infra" || return
    fi
    major="${flavor#postgres-}"; major="${major%%-*}"
    target=/var/lib/postgresql/data
    (( major < 18 )) || target=/var/lib/postgresql
    # Only write immutable stack config once. Up does not silently upgrade pins.
    if [[ ! -f "$dir/.env" ]]; then
        { printf 'COMPOSE_PROJECT_NAME=sdev-infra-%s\n' "$flavor"
          printf 'INFRA_FLAVOR=%s\nINFRA_IMAGE=%s\n' "$flavor" "$image"
          printf 'INFRA_ALIAS=sdev-%s\nPG_HOST_PORT=%s\n' "$flavor" "$((5432 + offset))"
          printf 'PG_DATA_TARGET=%s\nSDEV_INFRA_HOME=%s\n' "$target" "$SDEV_HOME"
        } > "$dir/.env" || return
    fi
    [[ -f "$dir/docker-compose.yml" ]] || cp "$SDEV_INSTALL/bin/templates/infra-postgres.yml.tmpl" "$dir/docker-compose.yml" || return
    cp "$SDEV_INSTALL/bin/templates/compose.tmpl" "$dir/compose" || return
    chmod +x "$dir/compose" || return
    [[ "${INFRA_NO_BOOT:-0}" == 1 ]] && return 0
    docker network inspect sdev-shared >/dev/null 2>&1 || {
        docker network create sdev-shared >/dev/null || docker network inspect sdev-shared >/dev/null
    } || return
    infra_compose "$flavor" up -d --wait --wait-timeout 90
}
infra_ensure() {
    local flavor
    flavor="$(infra_flavor "$1")" || return
    with_state_lock infra_up_locked "$flavor"
}

infra_down_locked() {
    local flavor="$1" destroy="$2" consumers dir answer
    dir="$SDEV_HOME/infra/$flavor"
    [[ -f "$dir/.sdev-infra" ]] || die "no managed infra stack for $flavor"
    infra_guard_owner "$flavor" || return
    consumers="$(infra_consumers)" || return
    [[ -z "$consumers" ]] || die "shared network still has consumers: $(echo "$consumers" | tr '\n' ' ')"
    if [[ "$destroy" == 1 ]]; then
        printf 'Destroy ALL databases in %s. Type the flavor name to confirm: ' "$flavor" >&2
        answer="${SDEV_CONFIRM:-}"
        [[ -n "$answer" ]] || read -r answer || answer=""
        [[ "$answer" == "$flavor" ]] || die "aborted (flavor not confirmed)"
        infra_compose "$flavor" down -v --remove-orphans || return
        rm -rf "${dir:?}" || return
        _free_infra_stack_locked "$flavor" || return
    else
        infra_compose "$flavor" down || return
    fi
}

# Deterministic ASCII identifier from a ledger key, never from a DB_NAME input.
# Reject invalid keys instead of sanitizing SQL/path metacharacters away.
infra_database_name() {
    local key="$1" name hash
    if [[ "$key" =~ ^@core/[a-z0-9]+(-[a-z0-9]+)*$ ]]; then
        name="core__${key#@core/}"; name="${name//-/_}"
    else
        [[ "$key" =~ ^[a-z0-9]+(-[a-z0-9]+)*/[a-z0-9]+(-[a-z0-9]+)*$ ]] || die "invalid task ledger key: $key"
        name="${key//\//_}"; name="${name//-/_}"
    fi
    if (( ${#name} > 63 )); then
        if command -v sha256sum >/dev/null 2>&1; then
            hash="$(printf '%s' "$key" | sha256sum)" || return
        else
            hash="$(printf '%s' "$key" | shasum -a 256)" || return
        fi
        name="${name:0:54}_${hash:0:8}"
    fi
    echo "$name"
}

# Must hold the lock when making lifecycle decisions with this ownership check.
# Short names can collide across the project/slug boundary (a-b/c vs a/b-c).
# Fail closed instead of creating or dropping another reservation's database.
infra_task_database() {
    local key="$1" name other candidate
    name="$(infra_database_name "$key")" || return
    [[ -f "$STATE_FILE" ]] || die "task ledger is missing"
    if [[ "$key" == @core/* ]]; then
        K="${key#@core/}" yq -e '.core_stacks | has(strenv(K))' "$STATE_FILE" >/dev/null || die "unknown core ledger key: $key"
        echo "$name"; return
    fi
    K="$key" yq -e '.tasks | has(strenv(K))' "$STATE_FILE" >/dev/null || die "unknown task ledger key: $key"
    while IFS= read -r other; do
        [[ "$other" == "$key" || "$other" != */* ]] && continue
        candidate="$(infra_database_name "$other")" || return
        [[ "$candidate" != "$name" ]] || die "database name collision between $key and $other"
    done < <(yq -r '.tasks | keys | .[]' "$STATE_FILE")
    echo "$name"
}

# Workspace teardown primitive for adoption. Flavor comes from the workspace
# snapshot, not current project config; caller supplies only its ledger key.
# Preserve the task/ledger on failure so cleanup can be retried.
infra_drop_task_database_locked() {
    local key="$1" name flavor dir cpn running
    name="$(infra_task_database "$key")" || return
    dir="$(infra_workspace_dir "$key")" || return
    flavor="$(sed -n 's/^SDEV_POSTGRES_FLAVOR=//p' "$dir/.env")" || return
    [[ -n "$flavor" ]] || return 0
    flavor="$(infra_flavor "$flavor")" || return
    [[ -n "$(infra_entry "$flavor" offset)" ]] || return 0
    infra_guard_owner "$flavor" || return
    cpn="$(sed -n 's/^COMPOSE_PROJECT_NAME=//p' "$dir/.env")" || return
    [[ -n "$cpn" ]] || { cpn="${key//\//-}"; [[ "$key" != @core/* ]] || cpn="core-${key#@core/}"; }
    running="$(docker ps --filter "label=com.docker.compose.project=$cpn" -q)" || return
    [[ -z "$running" ]] || die "workspace containers still running; retry teardown before dropping its database"
    # %I is quoted by Postgres; the variable comes only from the validated key.
    infra_compose "$flavor" exec -T postgres psql -U sdev -d postgres -v ON_ERROR_STOP=1 -v "db=$name" <<'SQL'
SELECT format('DROP DATABASE IF EXISTS %I WITH (FORCE)', :'db') \gexec
SQL
}
infra_drop_task_database() { with_state_lock infra_drop_task_database_locked "$1"; }

infra_workspace_dir() {
    # Validate before deriving any filesystem path. Legacy flat tasks cannot
    # adopt shared Postgres, but their unmarked snapshots bypass this helper.
    infra_database_name "$1" >/dev/null || return
    if [[ "$1" == @core/* ]]; then echo "$SDEV_HOME/stacks/${1#@core/}"
    else echo "$SDEV_HOME/projects/$1"; fi
}

infra_workspace_env_locked() {
    local key="$1" project dir flavor image name profile
    dir="$(infra_workspace_dir "$key")" || return
    project="${key%%/*}"; [[ "$project" != @core ]] || project="${key#@core/}"
    flavor="$(yq -r '.infra.postgres // ""' "$(effective_project_file "$project")")" || return
    [[ -n "$flavor" ]] || return 0
    flavor="$(infra_flavor "$flavor")" || return
    profile="$(sed -n 's/^APP_ENV=//p' "$dir/.env")" || return
    [[ "$profile" == local ]] || die "shared Postgres requires the local profile"
    yq -e '.services.dbinit != null and .networks."sdev-shared".external == true' "$dir/docker-compose.yml" >/dev/null || die "infra.postgres requires a paired shared-Postgres template"
    name="$(infra_task_database "$key")" || return
    image="$(infra_entry "$flavor" image)" || return
    [[ -n "$image" ]] || image="$(infra_image "$flavor")" || return
    printf 'SDEV_POSTGRES_FLAVOR=%s\nSDEV_POSTGRES_IMAGE=%s\nDB_HOST=sdev-%s\nDB_PORT=5432\nDB_USER=sdev\nDB_PASSWORD=sdev\nDB_NAME=%s\n' "$flavor" "$image" "$flavor" "$name"
}

infra_ensure_workspace_locked() {
    local key="$1" dir flavor name profile
    dir="$(infra_workspace_dir "$key")" || return
    flavor="$(sed -n 's/^SDEV_POSTGRES_FLAVOR=//p' "$dir/.env")" || return
    [[ -n "$flavor" ]] || return 0
    flavor="$(infra_flavor "$flavor")" || return
    profile="$(sed -n 's/^APP_ENV=//p' "$dir/.env")" || return
    [[ "$profile" == local ]] || die "shared Postgres requires the local profile"
    name="$(infra_task_database "$key")" || return
    [[ "$(sed -n 's/^DB_NAME=//p' "$dir/.env")" == "$name" ]] || die "workspace DB_NAME differs from its ledger identity"
    infra_up_locked "$flavor"
}
