load helpers
setup() {
  make_fixture
  echo 'conf_prefix: y' > "$WORKSPACE_ROOT/core/projects.d/default.yml"
  echo 'conf_prefix: p' > "$WORKSPACE_ROOT/core/projects.d/acme.yml"
  mkdir -p "$WORKSPACE_ROOT/projects/default/a" "$WORKSPACE_ROOT/projects/acme/b"
  echo -e 'PORT_OFFSET=10\nNGINX_HOST_PORT=8090' > "$WORKSPACE_ROOT/projects/default/a/.env"
  echo -e 'PORT_OFFSET=20\nNGINX_HOST_PORT=8100' > "$WORKSPACE_ROOT/projects/acme/b/.env"
}
teardown() { rm -rf "$WORKSPACE_ROOT"; }

@test "unscoped list shows both projects' tasks" {
  run "$WORKSPACE_ROOT/bin/list-tasks"
  [ "$status" -eq 0 ]
  [[ "$output" == *"default"* ]] && [[ "$output" == *"a"* ]]
  [[ "$output" == *"acme"* ]] && [[ "$output" == *"b"* ]]
}
@test "scoped list shows only that project" {
  run "$WORKSPACE_ROOT/bin/list-tasks" --project acme
  [ "$status" -eq 0 ]
  [[ "$output" == *"b"* ]]
  [[ "$output" != *"default/a"* ]]
}
