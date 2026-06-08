load helpers
setup() {
  make_fixture
  echo 'conf_prefix: y' > "$WORKSPACE_ROOT/core/projects.d/default.yml"
  echo 'conf_prefix: p' > "$WORKSPACE_ROOT/core/projects.d/acme.yml"
  export TERM_SESSION_ID="bats-$$"
  export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR"
}
teardown() { rm -rf "$WORKSPACE_ROOT"; }

@test "sdev projects lists defined projects" {
  run sdev projects
  [ "$status" -eq 0 ]
  [[ "$output" == *"acme"* ]] && [[ "$output" == *"default"* ]]
}
@test "sdev use pins project for session" {
  run sdev use acme
  [ "$status" -eq 0 ]
  run sdev use
  [[ "$output" == *"acme"* ]]
}
@test "sdev use rejects unknown project" {
  run sdev use nope
  [ "$status" -ne 0 ]
}
@test "sdev cd resolves namespaced task dir" {
  mkdir -p "$WORKSPACE_ROOT/projects/acme/feat"
  run sdev -p acme cd feat
  [ "$status" -eq 0 ]
  [ "$output" = "$WORKSPACE_ROOT/projects/acme/feat" ]
}
@test "sdev cd falls back to legacy flat task" {
  mkdir -p "$WORKSPACE_ROOT/projects/oldtask"
  run sdev -p default cd oldtask
  [ "$status" -eq 0 ]
  [ "$output" = "$WORKSPACE_ROOT/projects/oldtask" ]
}
