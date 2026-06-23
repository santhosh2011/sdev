load helpers
setup() {
  make_fixture
  mkdir -p "$WORKSPACE_ROOT/core/widget" "$WORKSPACE_ROOT/confs/widget"
  make_source_repo "$WORKSPACE_ROOT/core/widget/svc" main
  cat > "$WORKSPACE_ROOT/core/projects.d/widget.yml" <<'YAML'
conf_prefix: widget-api
template: bin/templates/docker-compose.yml.tmpl
stack_services: [api, db, redis]
repos:
  api: { path: svc, default_base: main, compose_role: api, link_node_modules: false }
YAML
  : > "$WORKSPACE_ROOT/confs/widget/widget-api.local.env"
  export TERM_SESSION_ID="bats-$$"; export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR"
}
teardown() { rm -rf "$WORKSPACE_ROOT"; }

@test "new-task creates namespaced task dir + worktree + .env + app.env" {
  run sdev -p widget new feat-x --env local
  [ "$status" -eq 0 ]
  [ -d "$WORKSPACE_ROOT/projects/widget/feat-x" ]
  [ -d "$WORKSPACE_ROOT/projects/widget/feat-x/svc" ]
  run grep -c '^PORT_OFFSET=' "$WORKSPACE_ROOT/projects/widget/feat-x/.env"
  [ "$output" = "1" ]
  [ "$(readlink "$WORKSPACE_ROOT/projects/widget/feat-x/app.env")" = "$WORKSPACE_ROOT/confs/widget/widget-api.local.env" ]
  run git -C "$WORKSPACE_ROOT/core/widget/svc" rev-parse --verify task/feat-x
  [ "$status" -eq 0 ]
}

@test "new-task refuses duplicate slug within project" {
  sdev -p widget new dup --env local
  run sdev -p widget new dup --env local
  [ "$status" -ne 0 ]
}

@test "new-task wires the three sdev hooks into settings.local.json" {
  run sdev -p widget new hooked --env local
  [ "$status" -eq 0 ]
  s="$WORKSPACE_ROOT/projects/widget/hooked/.claude/settings.local.json"
  [ -f "$s" ]
  run cat "$s"
  [[ "$output" == *"sdev-session-context"* ]]
  [[ "$output" == *"sdev-staging-guard"* ]]
  [[ "$output" == *"sdev-edit-reminder"* ]]
  # absolute paths into the install dir
  [[ "$output" == *"$WORKSPACE_ROOT/claude/hooks/sdev-session-context"* ]]
  # valid JSON with the expected matchers
  run bash -c "yq -p=json -r '.hooks.SessionStart[0].matcher' '$s' 2>/dev/null"
  [ "$output" = "startup|resume" ]
  run bash -c "yq -p=json -r '.hooks.PreToolUse[0].matcher' '$s' 2>/dev/null"
  [ "$output" = "Bash" ]
}

@test "new-task honors hooks: false (plain settings, no hooks)" {
  cat >> "$WORKSPACE_ROOT/core/projects.d/widget.yml" <<'YAML'
hooks: false
YAML
  run sdev -p widget new plain --env local
  [ "$status" -eq 0 ]
  s="$WORKSPACE_ROOT/projects/widget/plain/.claude/settings.local.json"
  [ -f "$s" ]
  run cat "$s"
  [[ "$output" != *"sdev-session-context"* ]]
  [[ "$output" != *"hooks"* ]]
}
