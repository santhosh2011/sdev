load helpers
setup() {
  make_fixture
  mkdir -p "$WORKSPACE_ROOT/core/web" "$WORKSPACE_ROOT/confs/web" "$WORKSPACE_ROOT/remotes" "$WORKSPACE_ROOT/fakebin"
  git init --bare -q "$WORKSPACE_ROOT/remotes/apisrc.git"
  make_source_repo "$WORKSPACE_ROOT/core/web/apisrc" main
  git -C "$WORKSPACE_ROOT/core/web/apisrc" remote add origin "$WORKSPACE_ROOT/remotes/apisrc.git"
  git -C "$WORKSPACE_ROOT/core/web/apisrc" push -q origin main
  cat > "$WORKSPACE_ROOT/core/projects.d/web.yml" <<'YAML'
conf_prefix: web
stack_services: [nginx, api]
repos:
  api: { path: apisrc, default_base: main, compose_role: api }
YAML
  : > "$WORKSPACE_ROOT/confs/web/web.local.env"
  # fake gh: log args, print a PR url
  cat > "$WORKSPACE_ROOT/fakebin/gh" <<SH
#!/usr/bin/env bash
echo "GH \$*" >> "$WORKSPACE_ROOT/gh.log"
echo "https://github.com/x/apisrc/pull/1"
SH
  chmod +x "$WORKSPACE_ROOT/fakebin/gh"
  export PATH="$WORKSPACE_ROOT/fakebin:$PATH"
  export TERM_SESSION_ID="bats-$$"; export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR"
}
teardown() { rm -rf "$WORKSPACE_ROOT"; }

# Create a task and put one commit on its task/<slug> branch.
new_task_with_commit() {
  sdev -p web new "$1" --env local --no-fetch >/dev/null 2>&1
  local wt="$WORKSPACE_ROOT/projects/web/$1/apisrc"
  echo change > "$wt/newfile"
  git -C "$wt" add -A && git -C "$wt" -c user.email=t@t -c user.name=t commit -qm work
}

@test "sdev ship pushes the task branch and opens a PR with an assignee (--json)" {
  new_task_with_commit feat
  run bash -c "'$WORKSPACE_ROOT/bin/sdev' -p web ship feat --json 2>/dev/null"
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.task == "web/feat"' >/dev/null
  echo "$output" | jq -e '.prs[0].url | startswith("http")' >/dev/null
  echo "$output" | jq -e '.assignee | length > 0' >/dev/null
  # branch actually reached the remote
  git -C "$WORKSPACE_ROOT/remotes/apisrc.git" rev-parse --verify task/feat
  # gh pr create was invoked with an assignee
  grep -q -- '--assignee' "$WORKSPACE_ROOT/gh.log"
}

@test "sdev ship refuses a dirty worktree without --force" {
  new_task_with_commit dirty
  echo uncommitted > "$WORKSPACE_ROOT/projects/web/dirty/apisrc/scratch"
  run sdev -p web ship dirty --json
  [ "$status" -ne 0 ]
}
