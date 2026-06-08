load helpers
setup() { make_fixture; source "$WORKSPACE_ROOT/bin/_lib.sh"; }
teardown() { rm -rf "$WORKSPACE_ROOT"; }

@test "config_projects lists project files" {
  cat > "$WORKSPACE_ROOT/core/projects.d/acme.yml" <<<'conf_prefix: x'
  cat > "$WORKSPACE_ROOT/core/projects.d/default.yml"  <<<'conf_prefix: y'
  run config_projects
  [ "$status" -eq 0 ]
  [[ "$output" == *"acme"* ]]
  [[ "$output" == *"default"* ]]
}

@test "effective_project_file falls back to global when no project file" {
  run effective_project_file default
  [ "$output" = "$WORKSPACE_ROOT/core/.task-config.yml" ]
}

@test "effective_project_file uses project file when present" {
  echo 'conf_prefix: z' > "$WORKSPACE_ROOT/core/projects.d/acme.yml"
  run effective_project_file acme
  [ "$output" = "$WORKSPACE_ROOT/core/projects.d/acme.yml" ]
}
