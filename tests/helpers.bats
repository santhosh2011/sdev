load helpers
setup() { make_fixture; }
teardown() { rm -rf "$WORKSPACE_ROOT"; }

@test "fixture builds with global config" {
  [ -f "$WORKSPACE_ROOT/core/.task-config.yml" ]
  run yq -r '.defaults.default_project' "$WORKSPACE_ROOT/core/.task-config.yml"
  [ "$output" = "default" ]
}
