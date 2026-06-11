load helpers

# These tests exercise new-task's "branch the task off the latest origin/<base>"
# behavior. setup builds an upstream repo + a clone (so the clone has an origin),
# and the project's default_base is 'develop'.
setup() {
  make_fixture
  UPSTREAM="$(mktemp -d)/upstream"
  make_source_repo "$UPSTREAM" develop
  mkdir -p "$WORKSPACE_ROOT/core/widget" "$WORKSPACE_ROOT/confs/widget"
  git clone -q "$UPSTREAM" "$WORKSPACE_ROOT/core/widget/svc"
  cat > "$WORKSPACE_ROOT/core/projects.d/widget.yml" <<'YAML'
conf_prefix: widget-api
template: bin/templates/docker-compose.yml.tmpl
stack_services: [api, db, redis]
repos:
  api: { path: svc, default_base: develop, compose_role: api, link_node_modules: false }
YAML
  : > "$WORKSPACE_ROOT/confs/widget/widget-api.local.env"
  export TERM_SESSION_ID="bats-$$"; export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR"
}
teardown() { rm -rf "$WORKSPACE_ROOT" "$(dirname "$UPSTREAM")"; }

# Advance upstream 'develop' past what the clone currently has.
advance_upstream() {
  echo more > "$UPSTREAM/f2"
  git -C "$UPSTREAM" add -A
  git -C "$UPSTREAM" commit -qm second
}

@test "new-task branches the task off the freshly fetched origin/<base>" {
  advance_upstream
  upstream_tip="$(git -C "$UPSTREAM" rev-parse develop)"
  # the clone's local develop is still the old tip (not yet fetched)
  [ "$(git -C "$WORKSPACE_ROOT/core/widget/svc" rev-parse develop)" != "$upstream_tip" ]

  run sdev -p widget new feat-x --env local
  [ "$status" -eq 0 ]
  run git -C "$WORKSPACE_ROOT/core/widget/svc" rev-parse task/feat-x
  [ "$output" = "$upstream_tip" ]
}

@test "new-task --no-fetch branches off the existing base without fetching" {
  old_tip="$(git -C "$WORKSPACE_ROOT/core/widget/svc" rev-parse develop)"
  advance_upstream
  upstream_tip="$(git -C "$UPSTREAM" rev-parse develop)"

  run sdev -p widget new feat-y --env local --no-fetch
  [ "$status" -eq 0 ]
  run git -C "$WORKSPACE_ROOT/core/widget/svc" rev-parse task/feat-y
  [ "$output" = "$old_tip" ]
  [ "$output" != "$upstream_tip" ]
}

@test "new-task warns and falls back to local base when fetch fails (no origin)" {
  make_source_repo "$WORKSPACE_ROOT/core/widget/noremote" develop
  cat > "$WORKSPACE_ROOT/core/projects.d/widget.yml" <<'YAML'
conf_prefix: widget-api
template: bin/templates/docker-compose.yml.tmpl
stack_services: [api, db, redis]
repos:
  api: { path: noremote, default_base: develop, compose_role: api, link_node_modules: false }
YAML
  run sdev -p widget new feat-z --env local
  [ "$status" -eq 0 ]
  [[ "$output" == *"fetch"* ]]
  run git -C "$WORKSPACE_ROOT/core/widget/noremote" rev-parse --verify task/feat-z
  [ "$status" -eq 0 ]
}
