load helpers
setup() {
  make_fixture
  export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR" "$WORKSPACE_ROOT/mock"
  export INFRA_LOG="$WORKSPACE_ROOT/docker.log" SQL_LOG="$WORKSPACE_ROOT/sql.log"
  cat > "$WORKSPACE_ROOT/mock/docker" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$INFRA_LOG"
case "$*" in
  'ps --filter network='*)
    [[ "${MOCK_QUERY_FAIL:-0}" == 0 ]] || exit 1
    printf '%s\n' "${MOCK_CONSUMERS:-}" ;;
  'ps -a '*) printf '%s\n' "${MOCK_OWNER:-}" ;;
  'ps '*) printf '%s\n' "${MOCK_STATUS:-}" ;;
  'network inspect '*) exit 1 ;;
  'network create '*) echo sdev-shared ;;
  *) echo "unexpected Docker call" >&2; exit 1 ;;
esac
SH
  cat > "$WORKSPACE_ROOT/mock/docker-compose" <<'SH'
#!/usr/bin/env bash
printf 'compose %s\n' "$*" >> "$INFRA_LOG"
case "$*" in
  *' down '*) [[ "${MOCK_DOWN_FAIL:-0}" == 0 ]] || exit 1 ;;
  *' up '*) [[ "${MOCK_UP_FAIL:-0}" == 0 ]] || exit 1 ;;
  *' -tAc '*) printf '%s\n' '["demo_one"]' ;;
  *' exec -T postgres psql '*) cat > "$SQL_LOG"; [[ "${MOCK_SQL_FAIL:-0}" == 0 ]] || exit 1 ;;
esac
SH
  chmod +x "$WORKSPACE_ROOT/mock/"*
  export PATH="$WORKSPACE_ROOT/mock:$PATH"
  source "$WORKSPACE_ROOT/bin/_lib.sh"
  source "$WORKSPACE_ROOT/bin/_infra.sh"
}
teardown() { rm -rf "$WORKSPACE_ROOT"; }

@test "infra up is idempotent and reserves a standing offset outside projects" {
  sdev infra up 18
  off="$(infra_entry postgres-18 offset)"
  created="$(infra_entry postgres-18 created_at)"
  sdev infra up postgres-18
  [ "$off" -eq 2000 ]
  [ "$(infra_entry postgres-18 offset)" = "$off" ]
  [ "$(infra_entry postgres-18 created_at)" = "$created" ]
  [ -f "$SDEV_HOME/infra/postgres-18/.sdev-infra" ]
  grep -qx 'PG_HOST_PORT=7432' "$SDEV_HOME/infra/postgres-18/.env"
  [ "$(yq '.networks."sdev-shared".external' "$SDEV_HOME/infra/postgres-18/docker-compose.yml")" = true ]
  grep -q 'up -d --wait --wait-timeout 90' "$INFRA_LOG"
  sdev prune --apply
  [ "$(infra_entry postgres-18 offset)" = "$off" ]
}

@test "flavors have distinct offsets aliases images and correct PG data mounts" {
  yq -i '.defaults.infra_postgres_flavors.postgres-18-pinned.image = "pgvector/pgvector:pg18-trixie"' "$GLOBAL_CONFIG"
  sdev infra up 18 --no-boot
  sdev infra up 16 --no-boot
  sdev infra up 18-pinned --no-boot
  [ "$(infra_entry postgres-16 offset)" -eq 2010 ]
  [ "$(infra_entry postgres-18-pinned offset)" -eq 2020 ]
  grep -qx 'INFRA_ALIAS=sdev-postgres-18-pinned' "$SDEV_HOME/infra/postgres-18-pinned/.env"
  grep -qx 'PG_DATA_TARGET=/var/lib/postgresql/data' "$SDEV_HOME/infra/postgres-16/.env"
  grep -qx 'PG_DATA_TARGET=/var/lib/postgresql' "$SDEV_HOME/infra/postgres-18/.env"
  [ "$(infra_entry postgres-18-pinned image)" = pgvector/pgvector:pg18-trixie ]
  run sdev infra up 18-unconfigured --no-boot
  [ "$status" -ne 0 ]
}

@test "all three allocators union ledger and fresh workspace offsets" {
  yq -i '.defaults.infra_port_offset_base = 10 | .defaults.core_port_offset_base = 10' "$GLOBAL_CONFIG"
  sdev infra up 18 --no-boot
  mkdir -p "$SDEV_HOME/projects/demo/old"
  echo PORT_OFFSET=20 > "$SDEV_HOME/projects/demo/old/.env"
  [ "$(allocate_core_offset demo main)" -eq 30 ]
  [ "$(allocate_offset demo/new 1 tester)" -eq 40 ]
  sdev infra up 16 --no-boot
  [ "$(infra_entry postgres-16 offset)" -eq 50 ]
}

@test "bash-go task writes preserve shared_infra and skip its offsets" {
  [[ -x "$WORKSPACE_ROOT/bin/sdev-go" ]] || skip "sdev-go not built"
  yq -i '.defaults.infra_port_offset_base = 10' "$GLOBAL_CONFIG"
  sdev infra up 18 --no-boot
  before="$(yq -o=json -I=0 '.shared_infra' "$STATE_FILE")"
  mkdir -p "$SDEV_HOME/projects/default/test"
  [ "$("$WORKSPACE_ROOT/bin/sdev-go" alloc default/test)" -eq 20 ]
  "$WORKSPACE_ROOT/bin/sdev-go" destroy test --force
  [ "$(yq -o=json -I=0 '.shared_infra' "$STATE_FILE")" = "$before" ]
  [ "$(allocate_offset demo/bash 1 test)" -eq 20 ]
  # Native distributed entrypoint also delegates infra to bash.
  "$WORKSPACE_ROOT/bin/sdev-go" infra up 16 --no-boot
  [ "$(infra_entry postgres-16 offset)" -eq 30 ]
}

@test "infra down refuses live users and Docker query failure" {
  sdev infra up 18 --no-boot
  export MOCK_CONSUMERS='demo-one||abc'
  run sdev infra down 18
  [ "$status" -ne 0 ]
  [[ "$output" == *consumers* ]]
  export MOCK_CONSUMERS='' MOCK_QUERY_FAIL=1
  run sdev infra down 18 --destroy
  [ "$status" -ne 0 ]
  ! grep -q 'compose .* down' "$INFRA_LOG"
  [ -n "$(infra_entry postgres-18 offset)" ]
}

@test "infra status derives unique consumers and database list from Docker" {
  sdev infra up 18 --no-boot
  export MOCK_CONSUMERS=$'demo-one||aaa\ndemo-one||bbb\n||unlabelled\nsdev-infra-postgres-18|postgres-18|server'
  export MOCK_STATUS='Up 2 minutes (healthy)'
  run sdev infra status --json
  [ "$status" -eq 0 ]
  [ "$(yq '.[0].consumers' <<< "$output")" -eq 2 ]
  [ "$(yq '.[0].databases[0]' <<< "$output")" = demo_one ]
  [ "$(yq '.[0].port' <<< "$output")" -eq 7432 ]
  [ "$(yq '.shared_infra.postgres-18 | has("refcount")' "$STATE_FILE")" = false ]
}

@test "down preserves data and destroy needs exact confirmation" {
  sdev infra up 18 --no-boot
  sdev infra down 18
  [ -f "$SDEV_HOME/infra/postgres-18/.sdev-infra" ]
  [ -n "$(infra_entry postgres-18 offset)" ]
  SDEV_CONFIRM=18 run sdev infra down 18 --destroy
  [ "$status" -ne 0 ]
  export SDEV_CONFIRM=postgres-18
  sdev infra down 18 --destroy
  [ ! -d "$SDEV_HOME/infra/postgres-18" ]
  [ -z "$(infra_entry postgres-18 offset)" ]
  ! grep -q 'network rm' "$INFRA_LOG"
}

@test "failed destroy retains directory and reservation for retry" {
  sdev infra up 18 --no-boot
  export MOCK_DOWN_FAIL=1 SDEV_CONFIRM=postgres-18
  run sdev infra down 18 --destroy
  [ "$status" -ne 0 ]
  [ -f "$SDEV_HOME/infra/postgres-18/.sdev-infra" ]
  [ -n "$(infra_entry postgres-18 offset)" ]
  [ ! -e "$STATE_LOCK" ]
}

@test "infra refuses foreign homes invalid flavors and unmarked directories" {
  export MOCK_OWNER="id|/another/home"
  run sdev infra up 18
  [ "$status" -ne 0 ]
  [[ "$output" == *another*SDEV_HOME* ]]
  unset MOCK_OWNER
  run sdev infra up '../18'
  [ "$status" -ne 0 ]
  mkdir -p "$SDEV_HOME/infra/postgres-18"
  echo keep > "$SDEV_HOME/infra/postgres-18/unowned"
  run sdev infra up 18 --no-boot
  [ "$status" -ne 0 ]
  [ -f "$SDEV_HOME/infra/postgres-18/unowned" ]
}

@test "database naming caps at 63 bytes with a hash and rejects unsafe keys" {
  [ "$(infra_database_name pdmt/glossary-scope-b3)" = pdmt_glossary_scope_b3 ]
  long="$(printf '%060d' 0)"
  first="$(infra_database_name "$long/one")"
  second="$(infra_database_name "$long/two")"
  [ "${#first}" -eq 63 ]
  [[ "$first" =~ _[0-9a-f]{8}$ ]]
  [ "$first" != "$second" ]
  run infra_database_name 'pdmt/x;DROP DATABASE postgres'
  [ "$status" -ne 0 ]
}

@test "scoped DROP recomputes name from ledger key and ignores env DB_NAME" {
  sdev infra up 18 --no-boot
  mkdir -p "$SDEV_HOME/projects/demo/one"
  printf 'SDEV_POSTGRES_FLAVOR=postgres-18\nDB_NAME=postgres\n' > "$SDEV_HOME/projects/demo/one/.env"
  allocate_offset demo/one >/dev/null
  infra_drop_task_database demo/one
  grep -q 'db=demo_one' "$INFRA_LOG"
  grep -q 'DROP DATABASE IF EXISTS %I WITH (FORCE)' "$SQL_LOG"
  run infra_drop_task_database demo/missing
  [ "$status" -ne 0 ]
}

@test "ambiguous project-slug database names fail closed" {
  allocate_offset a-b/c 1 holder >/dev/null
  allocate_offset a/b-c 1 holder >/dev/null
  run with_state_lock infra_task_database a-b/c
  [ "$status" -ne 0 ]
  [[ "$output" == *collision* ]]
}

@test "workspace dbinit fragment has no volume reference and external network" {
  file="$WORKSPACE_ROOT/bin/templates/shared-postgres.yml.tmpl"
  [ "$(yq 'has("volumes")' "$file")" = false ]
  [ "$(yq '.services.dbinit | has("volumes")' "$file")" = false ]
  [ "$(yq '.networks."sdev-shared".external' "$file")" = true ]
  [ "$(yq '.services.dbinit.command | length' "$file")" -eq 1 ]
  grep -q 'pg_advisory_lock' "$file"
}

@test "failed infra boot retains one reservation and releases the lock for retry" {
  export MOCK_UP_FAIL=1
  run sdev infra up 18
  [ "$status" -ne 0 ]
  off="$(infra_entry postgres-18 offset)"
  [ "$off" -eq 2000 ]
  [ ! -e "$STATE_LOCK" ]
  export MOCK_UP_FAIL=0
  sdev infra up 18
  [ "$(infra_entry postgres-18 offset)" = "$off" ]
}

@test "unlabelled existing infra project is not silently adopted" {
  export MOCK_OWNER='existing-id|'
  run sdev infra up 18
  [ "$status" -ne 0 ]
  [ ! -d "$SDEV_HOME/infra/postgres-18" ]
}

@test "empty infra status is inert and invalid offset configuration fails closed" {
  run sdev infra status --json
  [ "$status" -eq 0 ]
  [ "$output" = '[]' ]
  [ ! -e "$INFRA_LOG" ]
  yq -i '.defaults.port_step = 0' "$GLOBAL_CONFIG"
  run sdev infra up 18 --no-boot
  [ "$status" -ne 0 ]
  [ -z "$(infra_entry postgres-18 offset)" ]
}

@test "dbinit fragment embeds the shipped script with Compose escaping" {
  script="$(yq -r '.services.dbinit.command[0] | split("$$") | join("$")' "$WORKSPACE_ROOT/bin/templates/shared-postgres.yml.tmpl")"
  [ "$script" = "$(cat "$WORKSPACE_ROOT/bin/templates/dbinit.sh")" ]
}

make_shared_project() {
  mkdir -p "$SDEV_HOME/core/demo" "$SDEV_HOME/confs/demo"
  make_source_repo "$SDEV_HOME/core/demo/api" main
  cp "$REPO_BIN/templates/shared-postgres.yml.tmpl" "$SDEV_HOME/core/demo/compose.tmpl"
  cat > "$SDEV_HOME/core/projects.d/demo.yml" <<'YAML'
conf_prefix: demo
infra: {postgres: "18"}
template: core/demo/compose.tmpl
repos:
  api: {path: api, default_base: main, compose_role: api}
YAML
  : > "$SDEV_HOME/confs/demo/demo.local.env"
}

@test "opted-in creation snapshots identity and up ensures infra with native or bash router" {
  make_shared_project
  sdev -p demo new one --no-fetch
  env="$SDEV_HOME/projects/demo/one/.env"
  grep -qx 'DB_NAME=demo_one' "$env"
  grep -qx 'DB_HOST=sdev-postgres-18' "$env"
  grep -qx 'SDEV_POSTGRES_FLAVOR=postgres-18' "$env"
  [ -z "$(infra_entry postgres-18 offset)" ]
  sdev -p demo up one
  [ -n "$(infra_entry postgres-18 offset)" ]
  sdev -p demo nuke one
  grep -q 'db=demo_one' "$INFRA_LOG"
  [ -d "$SDEV_HOME/projects/demo/one" ]
  [ "$(K=demo/one yq '.tasks | has(strenv(K))' "$STATE_FILE")" = true ]
}

@test "old snapshots stay local after registry opt-in" {
  make_shared_project
  yq -i 'del(.infra)' "$SDEV_HOME/core/projects.d/demo.yml"
  sdev -p demo new old --no-fetch
  yq -i '.infra.postgres = "18"' "$SDEV_HOME/core/projects.d/demo.yml"
  sdev -p demo up old
  [ -z "$(infra_entry postgres-18 offset)" ]
  ! grep -q '^SDEV_POSTGRES_FLAVOR=' "$SDEV_HOME/projects/demo/old/.env"
}

@test "failed scoped cleanup preserves task workspace and ledger for retry" {
  make_shared_project
  sdev -p demo new one --no-fetch
  sdev -p demo up one
  export MOCK_SQL_FAIL=1
  run sdev -p demo end one --force
  [ "$status" -ne 0 ]
  [ -d "$SDEV_HOME/projects/demo/one/api" ]
  [ "$(K=demo/one yq '.tasks | has(strenv(K))' "$STATE_FILE")" = true ]
  export MOCK_SQL_FAIL=0
  sdev -p demo end one --force
  [ ! -d "$SDEV_HOME/projects/demo/one" ]
  [ -n "$(infra_entry postgres-18 offset)" ]
}

@test "core and task database identities are disjoint and destroy is scoped" {
  make_shared_project
  sdev -p demo core up --no-boot --no-fetch
  grep -qx 'DB_NAME=core__demo' "$SDEV_HOME/stacks/demo/.env"
  sdev infra up 18 --no-boot
  sdev -p demo core down --destroy
  grep -q 'db=core__demo' "$INFRA_LOG"
  [ -n "$(infra_entry postgres-18 offset)" ]
}

@test "unbooted shared tasks can be destroyed without starting infra" {
  make_shared_project
  sdev -p demo new one --no-fetch
  sdev -p demo destroy one --force
  [ ! -d "$SDEV_HOME/projects/demo/one" ]
  [ -z "$(infra_entry postgres-18 offset)" ]
  [ ! -f "$SQL_LOG" ]
}
