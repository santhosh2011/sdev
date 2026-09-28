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

# A real remote lets the guard distinguish fresh server state from stale refs.
setup_remote() {
  src="$WORKSPACE_ROOT/core/widget/svc"
  wt="$WORKSPACE_ROOT/projects/widget/gone/svc"
  remote="$WORKSPACE_ROOT/remote.git"
  git clone -q --bare "$src" "$remote"
  git -C "$src" remote add origin "$remote"
  git -C "$src" fetch -q origin
  git -C "$wt" checkout -qb worker/actual-work
  # No real Docker calls in these preflight/teardown tests.
  mkdir -p "$WORKSPACE_ROOT/mock-bin"
  printf '#!/usr/bin/env bash\nexit 0\n' > "$WORKSPACE_ROOT/mock-bin/docker"
  chmod +x "$WORKSPACE_ROOT/mock-bin/docker"
  export PATH="$WORKSPACE_ROOT/mock-bin:$PATH"
}

commit_work() {
  echo work > "$wt/work.txt"
  git -C "$wt" add work.txt
  git -C "$wt" commit -qm work
}

@test "end-task accepts landed HEAD after fetch even when setup branch is outside target" {
  setup_remote
  # The abandoned setup branch diverges; the actual checked-out work lands.
  echo abandoned > "$src/abandoned.txt"
  git -C "$src" add abandoned.txt
  git -C "$src" commit -qm abandoned
  git -C "$src" branch -f task/gone HEAD
  commit_work
  git -C "$wt" push -q "$remote" HEAD:refs/heads/main
  # Pushing by URL deliberately leaves origin/main stale.
  ! git -C "$wt" merge-base --is-ancestor HEAD origin/main
  run sdev -p widget end gone --merge-target origin/main
  [ "$status" -eq 0 ]
  [ ! -d "$wt" ]
}

@test "end-task refuses unlanded HEAD even when setup branch is landed" {
  setup_remote
  commit_work
  git -C "$wt" merge-base --is-ancestor task/gone origin/main
  run sdev -p widget end gone --merge-target origin/main
  [ "$status" -ne 0 ]
  [[ "$output" == *"HEAD (worker/actual-work) not in origin/main"* ]]
  [ -f "$wt/work.txt" ]
}

@test "end-task accepts a bare merge target" {
  setup_remote
  run sdev -p widget end gone --merge-target main
  [ "$status" -eq 0 ]
  [ ! -d "$wt" ]
}

@test "end-task defaults to the configured repo base" {
  setup_remote
  run sdev -p widget end gone
  [ "$status" -eq 0 ]
  [ ! -d "$wt" ]
}

@test "end-task refuses stale landed refs when fetch fails" {
  setup_remote
  git -C "$src" remote set-url origin "$WORKSPACE_ROOT/missing.git"
  run sdev -p widget end gone
  [ "$status" -ne 0 ]
  [[ "$output" == *"cannot fetch origin/main"* ]]
  [ -d "$wt" ]
}

@test "end-task refuses a deleted remote target despite its stale tracking ref" {
  setup_remote
  git --git-dir="$remote" update-ref -d refs/heads/main
  run sdev -p widget end gone
  [ "$status" -ne 0 ]
  [[ "$output" == *"cannot fetch origin/main"* ]]
  [ -d "$wt" ]
}

@test "end-task checks detached HEAD and refuses unlanded work" {
  setup_remote
  commit_work
  git -C "$wt" checkout -q --detach
  run sdev -p widget end gone
  [ "$status" -ne 0 ]
  [[ "$output" == *"HEAD (detached) not in origin/main"* ]]
  [ -f "$wt/work.txt" ]
}

@test "end-task work-branch protects work no longer checked out" {
  setup_remote
  commit_work
  git -C "$wt" checkout -q task/gone
  run sdev -p widget end gone --work-branch worker/actual-work
  [ "$status" -ne 0 ]
  [[ "$output" == *"refs/heads/worker/actual-work not in origin/main"* ]]
  [ -d "$wt" ]
  git -C "$wt" push -q "$remote" worker/actual-work:refs/heads/main
  run sdev -p widget end gone --work-branch worker/actual-work
  [ "$status" -eq 0 ]
}

@test "end-task work-branch cannot bypass an unlanded HEAD" {
  setup_remote
  commit_work
  run sdev -p widget end gone --work-branch task/gone
  [ "$status" -ne 0 ]
  [[ "$output" == *"HEAD (worker/actual-work) not in origin/main"* ]]
  [ -f "$wt/work.txt" ]
}

@test "end-task refuses an unknown work branch" {
  setup_remote
  run sdev -p widget end gone --work-branch missing
  [ "$status" -ne 0 ]
  [ -d "$wt" ]
}

@test "end-task still refuses dirty landed work" {
  setup_remote
  echo dirty > "$wt/dirty.txt"
  run sdev -p widget end gone
  [ "$status" -ne 0 ]
  [[ "$output" == *"working tree dirty"* ]]
  [ -f "$wt/dirty.txt" ]
}

@test "end-task supports explicit remotes and slash-containing bare targets" {
  setup_remote
  git --git-dir="$remote" branch release/next main
  git -C "$src" remote rename origin upstream
  run sdev -p widget end gone --merge-target upstream/release/next
  [ "$status" -eq 0 ]
}

@test "end-task treats slash-containing bare targets as origin branches" {
  setup_remote
  git --git-dir="$remote" branch release/next main
  run sdev -p widget end gone --merge-target release/next
  [ "$status" -eq 0 ]
}

@test "end-task checks nested configured repo paths" {
  setup_remote
  mkdir -p "$WORKSPACE_ROOT/projects/widget/gone/nested"
  git -C "$src" worktree move "$wt" "$WORKSPACE_ROOT/projects/widget/gone/nested/svc"
  wt="$WORKSPACE_ROOT/projects/widget/gone/nested/svc"
  yq -i '.repos.api.path = "nested/svc"' "$WORKSPACE_ROOT/core/projects.d/widget.yml"
  commit_work
  run sdev -p widget end gone
  [ "$status" -ne 0 ]
  [[ "$output" == *"HEAD (worker/actual-work) not in origin/main"* ]]
  [ -f "$wt/work.txt" ]
}

@test "end-task refuses missing default base rather than guessing" {
  setup_remote
  yq -i 'del(.repos.api.default_base)' "$WORKSPACE_ROOT/core/projects.d/widget.yml"
  run sdev -p widget end gone
  [ "$status" -ne 0 ]
  [[ "$output" == *"no default_base configured"* ]]
  [ -d "$wt" ]
}

@test "end-task refuses an empty repo configuration" {
  setup_remote
  yq -i '.repos = {}' "$WORKSPACE_ROOT/core/projects.d/widget.yml"
  run sdev -p widget end gone
  [ "$status" -ne 0 ]
  [ -d "$wt" ]
}

@test "end-task resolves different default targets for every repo" {
  setup_remote
  make_source_repo "$WORKSPACE_ROOT/core/widget/other" develop
  git clone -q --bare "$WORKSPACE_ROOT/core/widget/other" "$WORKSPACE_ROOT/other.git"
  git -C "$WORKSPACE_ROOT/core/widget/other" remote add origin "$WORKSPACE_ROOT/other.git"
  git -C "$WORKSPACE_ROOT/core/widget/other" worktree add -qb worker/other "$WORKSPACE_ROOT/projects/widget/gone/other"
  yq -i '.repos.other = {"path": "other", "default_base": "develop"}' "$WORKSPACE_ROOT/core/projects.d/widget.yml"
  run sdev -p widget end gone
  [ "$status" -eq 0 ]
  [ ! -d "$WORKSPACE_ROOT/projects/widget/gone" ]
}
