load helpers
setup() {
  make_fixture
  mkdir -p "$WORKSPACE_ROOT/core/widget" "$WORKSPACE_ROOT/confs/widget"
  make_source_repo "$WORKSPACE_ROOT/core/widget/svc" main
  # A .gitignore so build caches (build/) survive `git clean -fd` on return.
  printf 'build/\nnode_modules/\n' > "$WORKSPACE_ROOT/core/widget/svc/.gitignore"
  git -C "$WORKSPACE_ROOT/core/widget/svc" add .gitignore
  git -C "$WORKSPACE_ROOT/core/widget/svc" commit -qm gitignore
  cat > "$WORKSPACE_ROOT/core/projects.d/widget.yml" <<'YAML'
conf_prefix: widget-api
stack_services: [api, db]
repos: { api: { path: svc, default_base: main, compose_role: api } }
YAML
  : > "$WORKSPACE_ROOT/confs/widget/widget-api.local.env"
  export TERM_SESSION_ID="bats-$$"; export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR"
}
teardown() { rm -rf "$WORKSPACE_ROOT"; }

STATE="__will_set__"

@test "end --pool returns a worktree; a later new reuses it (caches intact, rebranded)" {
  STATE="$WORKSPACE_ROOT/state/state.yml"

  sdev -p widget new one --env local --no-fetch
  # A gitignored build cache (must survive) and a tracked source edit (must be reset).
  mkdir -p "$WORKSPACE_ROOT/projects/widget/one/svc/build"
  echo CACHE > "$WORKSPACE_ROOT/projects/widget/one/svc/build/marker"
  echo dirty >> "$WORKSPACE_ROOT/projects/widget/one/svc/README"

  run sdev -p widget end one --pool --force
  [ "$status" -eq 0 ]
  [ ! -d "$WORKSPACE_ROOT/projects/widget/one" ]

  # The worktree is now in the pool with its cache preserved, offset freed.
  [ "$(yq -r '.pool | length' "$STATE")" = "1" ]
  poolpath="$(yq -r '.pool[0].path' "$STATE")"
  [ -f "$poolpath/build/marker" ]
  [ "$(yq -r '.tasks | has("widget/one")' "$STATE")" = "false" ]

  # Next new reuses the pooled worktree.
  run sdev -p widget new two --env local --no-fetch
  [ "$status" -eq 0 ]
  [[ "$output" == *"reused pooled worktree"* ]]

  # Cache survived the reuse...
  [ -f "$WORKSPACE_ROOT/projects/widget/two/svc/build/marker" ]
  # ...the worktree was rebranded...
  run git -C "$WORKSPACE_ROOT/projects/widget/two/svc" rev-parse --abbrev-ref HEAD
  [ "$output" = "task/two" ]
  # ...and the tracked source edit from the old task is gone (clean reset).
  run git -C "$WORKSPACE_ROOT/projects/widget/two/svc" status --porcelain
  [ -z "$output" ]
  # Pool is drained.
  [ "$(yq -r '.pool | length' "$STATE")" = "0" ]
}

@test "new falls back to a fresh worktree when the pool is empty" {
  run sdev -p widget new solo --env local --no-fetch
  [ "$status" -eq 0 ]
  [[ "$output" == *"creating worktree"* ]]
  [ -d "$WORKSPACE_ROOT/projects/widget/solo/svc" ]
}

@test "--no-pool forces a fresh worktree even when the pool has one" {
  sdev -p widget new one --env local --no-fetch
  sdev -p widget end one --pool --force
  [ "$(yq -r '.pool | length' "$WORKSPACE_ROOT/state/state.yml")" = "1" ]
  run sdev -p widget new two --env local --no-fetch --no-pool
  [ "$status" -eq 0 ]
  [[ "$output" == *"creating worktree"* ]]
  # Pool untouched (still one entry).
  [ "$(yq -r '.pool | length' "$WORKSPACE_ROOT/state/state.yml")" = "1" ]
}
