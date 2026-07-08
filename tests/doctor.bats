load helpers
setup() {
  make_fixture
  mkdir -p "$WORKSPACE_ROOT/core/widget" "$WORKSPACE_ROOT/confs/widget"
  make_source_repo "$WORKSPACE_ROOT/core/widget/svc" main
  cat > "$WORKSPACE_ROOT/core/projects.d/widget.yml" <<'YAML'
conf_prefix: widget-api
stack_services: [api, db]
repos: { api: { path: svc, default_base: main, compose_role: api } }
YAML
  : > "$WORKSPACE_ROOT/confs/widget/widget-api.local.env"
  export TERM_SESSION_ID="bats-$$"; export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR"
}
teardown() { rm -rf "$WORKSPACE_ROOT"; }

@test "doctor passes on a clean install (no ledger yet)" {
  run "$WORKSPACE_ROOT/bin/sdev" doctor
  [ "$status" -eq 0 ]
  [[ "$output" == *"doctor: OK"* ]]
}

@test "doctor passes with a healthy ledger" {
  sdev -p widget new one --env local --no-fetch
  run "$WORKSPACE_ROOT/bin/sdev" doctor
  [ "$status" -eq 0 ]
  [[ "$output" == *"ledger offsets match on-disk tasks"* ]]
  [[ "$output" == *"no duplicate offsets in ledger"* ]]
  [[ "$output" == *"doctor: OK"* ]]
}

@test "doctor FAILS on duplicate offsets in the ledger" {
  sdev -p widget new one --env local --no-fetch
  sdev -p widget new two --env local --no-fetch
  # Corrupt the ledger: force a duplicate offset.
  K="widget/two" yq -i '.tasks[strenv(K)].offset = 10' "$WORKSPACE_ROOT/state/state.yml"
  # Also make the .env agree so the drift check doesn't mask the dup check.
  sed -i.bak 's/^PORT_OFFSET=.*/PORT_OFFSET=10/' "$WORKSPACE_ROOT/projects/widget/two/.env" && rm -f "$WORKSPACE_ROOT/projects/widget/two/.env.bak"
  run "$WORKSPACE_ROOT/bin/sdev" doctor
  [ "$status" -ne 0 ]
  [[ "$output" == *"duplicate offsets"* ]]
}
