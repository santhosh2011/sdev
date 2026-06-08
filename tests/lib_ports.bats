load helpers
setup() { make_fixture; source "$WORKSPACE_ROOT/bin/_lib.sh"; }
teardown() { rm -rf "$WORKSPACE_ROOT"; }

@test "compute_next_offset scans nested + flat .env recursively" {
  mkdir -p "$WORKSPACE_ROOT/projects/default/a" "$WORKSPACE_ROOT/projects/acme/b" "$WORKSPACE_ROOT/projects/legacy"
  echo 'PORT_OFFSET=10' > "$WORKSPACE_ROOT/projects/default/a/.env"
  echo 'PORT_OFFSET=20' > "$WORKSPACE_ROOT/projects/acme/b/.env"
  echo 'PORT_OFFSET=30' > "$WORKSPACE_ROOT/projects/legacy/.env"
  run compute_next_offset
  [ "$output" = "40" ]
}
@test "profile_conf_file project-namespaced when confs/<project> exists" {
  echo 'conf_prefix: acme-api' > "$WORKSPACE_ROOT/core/projects.d/acme.yml"
  mkdir -p "$WORKSPACE_ROOT/confs/acme"
  run profile_conf_file local acme
  [ "$output" = "$WORKSPACE_ROOT/confs/acme/acme-api.local.env" ]
}
@test "profile_conf_file legacy path when no confs/<project> dir" {
  run profile_conf_file local default
  [ "$output" = "$WORKSPACE_ROOT/confs/app.local.env" ]
}
@test "repo_source_dir prefers core/<project>/<repo>" {
  mkdir -p "$WORKSPACE_ROOT/core/acme/my-x"
  run repo_source_dir acme my-x
  [ "$output" = "$WORKSPACE_ROOT/core/acme/my-x" ]
}
@test "repo_source_dir falls back to legacy core/<repo>" {
  mkdir -p "$WORKSPACE_ROOT/core/legacy_api"
  run repo_source_dir default legacy_api
  [ "$output" = "$WORKSPACE_ROOT/core/legacy_api" ]
}
