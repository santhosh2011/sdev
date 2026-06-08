load helpers
setup() {
  make_fixture
  mkdir -p "$WORKSPACE_ROOT/core/widget" "$WORKSPACE_ROOT/confs/widget"
  make_source_repo "$WORKSPACE_ROOT/core/widget/svc" main
  cat > "$WORKSPACE_ROOT/core/projects.d/widget.yml" <<'YAML'
conf_prefix: widget-api
template: bin/templates/docker-compose.yml.tmpl
stack_services: [api]
repos: { api: { path: svc, default_base: main, compose_role: api } }
YAML
  : > "$WORKSPACE_ROOT/confs/widget/widget-api.local.env"
  export TERM_SESSION_ID="bats-$$"; export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR"
  sdev -p widget new gone --env local
}
teardown() { rm -rf "$WORKSPACE_ROOT"; }

@test "end-task archives a namespaced task and frees the worktree" {
  run env SDEV_PROJECT=widget "$WORKSPACE_ROOT/bin/end-task" gone --force
  [ "$status" -eq 0 ]
  [ ! -d "$WORKSPACE_ROOT/projects/widget/gone" ]
  [ -d "$WORKSPACE_ROOT/projects/_archive/widget/gone" ]
  run git -C "$WORKSPACE_ROOT/core/widget/svc" worktree list
  [[ "$output" != *"projects/widget/gone"* ]]
}
