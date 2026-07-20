load helpers

# A two-repo project so the core stack exercises the multi-repo worktree path.
setup() {
  make_fixture
  mkdir -p "$WORKSPACE_ROOT/core/demo" "$WORKSPACE_ROOT/confs/demo"
  make_source_repo "$WORKSPACE_ROOT/core/demo/api" main
  make_source_repo "$WORKSPACE_ROOT/core/demo/ui" main
  cat > "$WORKSPACE_ROOT/core/projects.d/demo.yml" <<'YAML'
conf_prefix: demo-app
stack_services: [nginx, api, ui, db]
repos:
  api: { path: api, default_base: main, compose_role: api }
  ui:  { path: ui,  default_base: main, compose_role: ui }
YAML
  : > "$WORKSPACE_ROOT/confs/demo/demo-app.local.env"
  export TERM_SESSION_ID="bats-$$"; export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR"
}
teardown() { rm -rf "$WORKSPACE_ROOT"; }

STATE="__will_set__"

@test "core up scaffolds a stacks/ workspace with a marker and a core-band ledger offset" {
  STATE="$WORKSPACE_ROOT/state/state.yml"
  sdev -p demo core up --no-boot --no-fetch
  [ -f "$WORKSPACE_ROOT/stacks/demo/.sdev-core" ]
  [ "$(K=demo yq -r '.core_stacks[strenv(K)].offset' "$STATE")" -ge 1000 ]
}

@test "core up creates one detached worktree per repo (multi-repo)" {
  sdev -p demo core up --no-boot --no-fetch
  [ -e "$WORKSPACE_ROOT/stacks/demo/api/.git" ]
  [ -e "$WORKSPACE_ROOT/stacks/demo/ui/.git" ]
  run git -C "$WORKSPACE_ROOT/stacks/demo/api" symbolic-ref -q HEAD
  [ "$status" -ne 0 ]
}

@test "core up writes a core-<project> compose project name and offset-derived ports" {
  sdev -p demo core up --no-boot --no-fetch
  grep -qx "COMPOSE_PROJECT_NAME=core-demo" "$WORKSPACE_ROOT/stacks/demo/.env"
  off="$(grep '^PORT_OFFSET=' "$WORKSPACE_ROOT/stacks/demo/.env" | cut -d= -f2)"
  grep -qx "NGINX_HOST_PORT=$((8080 + off))" "$WORKSPACE_ROOT/stacks/demo/.env"
}

@test "core up reuses the same stable offset on a second up (idempotent)" {
  STATE="$WORKSPACE_ROOT/state/state.yml"
  sdev -p demo core up --no-boot --no-fetch
  first="$(K=demo yq -r '.core_stacks[strenv(K)].offset' "$STATE")"
  sdev -p demo core up --no-boot --no-fetch
  second="$(K=demo yq -r '.core_stacks[strenv(K)].offset' "$STATE")"
  [ "$first" = "$second" ]
}

@test "core and task offsets never collide and are recorded in separate namespaces" {
  STATE="$WORKSPACE_ROOT/state/state.yml"
  sdev -p demo core up --no-boot --no-fetch
  coreoff="$(K=demo yq -r '.core_stacks[strenv(K)].offset' "$STATE")"
  sdev -p demo new t1 --env local --no-fetch
  taskoff="$(K=demo/t1 yq -r '.tasks[strenv(K)].offset' "$STATE")"
  [ "$coreoff" != "$taskoff" ]
  dupes="$( { yq -r '.tasks[].offset' "$STATE"; yq -r '.core_stacks[].offset' "$STATE"; } | sort | uniq -d )"
  [ -z "$dupes" ]
}

@test "core refresh refuses on uncommitted changes and never discards them" {
  sdev -p demo core up --no-boot --no-fetch
  echo scratch > "$WORKSPACE_ROOT/stacks/demo/api/UNCOMMITTED"
  run sdev -p demo core refresh --no-boot --no-fetch
  [ "$status" -ne 0 ]
  [[ "$output" == *"uncommitted"* ]]
  [ -f "$WORKSPACE_ROOT/stacks/demo/api/UNCOMMITTED" ]
}

@test "core refresh fast-forwards the detached HEAD to a new base commit" {
  sdev -p demo core up --no-boot --no-fetch
  before="$(git -C "$WORKSPACE_ROOT/stacks/demo/api" rev-parse HEAD)"
  echo more > "$WORKSPACE_ROOT/core/demo/api/f2"
  git -C "$WORKSPACE_ROOT/core/demo/api" add -A
  git -C "$WORKSPACE_ROOT/core/demo/api" commit -qm second
  sdev -p demo core refresh --no-boot --no-fetch
  after="$(git -C "$WORKSPACE_ROOT/stacks/demo/api" rev-parse HEAD)"
  [ "$before" != "$after" ]
}

@test "core down --destroy removes the workspace and frees the offset" {
  STATE="$WORKSPACE_ROOT/state/state.yml"
  sdev -p demo core up --no-boot --no-fetch
  sdev -p demo core down --destroy
  [ ! -d "$WORKSPACE_ROOT/stacks/demo" ]
  [ "$(K=demo yq -r '.core_stacks | has(strenv(K))' "$STATE")" = "false" ]
}

@test "a freed core offset is reusable by the next core stack (no leak)" {
  STATE="$WORKSPACE_ROOT/state/state.yml"
  sdev -p demo core up --no-boot --no-fetch
  first="$(K=demo yq -r '.core_stacks[strenv(K)].offset' "$STATE")"
  sdev -p demo core down --destroy
  sdev -p demo core up --no-boot --no-fetch
  [ "$(K=demo yq -r '.core_stacks[strenv(K)].offset' "$STATE")" = "$first" ]
}

@test "prune --apply never reclaims a core stack" {
  STATE="$WORKSPACE_ROOT/state/state.yml"
  sdev -p demo core up --no-boot --no-fetch
  sdev -p demo prune --apply
  [ -d "$WORKSPACE_ROOT/stacks/demo" ]
  [ "$(K=demo yq -r '.core_stacks | has(strenv(K))' "$STATE")" = "true" ]
}

@test "a task ledger write preserves the core_stacks namespace (bash<->go interop)" {
  STATE="$WORKSPACE_ROOT/state/state.yml"
  sdev -p demo core up --no-boot --no-fetch
  coreoff="$(K=demo yq -r '.core_stacks[strenv(K)].offset' "$STATE")"
  sdev -p demo new t1 --env local --no-fetch
  sdev -p demo end t1 --force
  [ "$(K=demo yq -r '.core_stacks | has(strenv(K))' "$STATE")" = "true" ]
  [ "$(K=demo yq -r '.core_stacks[strenv(K)].offset' "$STATE")" = "$coreoff" ]
}

@test "core status --json reports the stack name, offset and url" {
  sdev -p demo core up --no-boot --no-fetch
  run sdev -p demo core status --json
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.stack == "core-demo"' >/dev/null
  echo "$output" | jq -e '.offset >= 1000' >/dev/null
}

@test "core doctor stays green after a core up + task create" {
  sdev -p demo core up --no-boot --no-fetch
  sdev -p demo new t1 --env local --no-fetch
  run "$WORKSPACE_ROOT/bin/sdev" doctor
  [ "$status" -eq 0 ]
  [[ "$output" == *"doctor: OK"* ]]
}
