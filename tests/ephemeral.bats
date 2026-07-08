load helpers
setup() {
  make_fixture
  mkdir -p "$WORKSPACE_ROOT/core/widget" "$WORKSPACE_ROOT/confs/widget"
  make_source_repo "$WORKSPACE_ROOT/core/widget/svc" main
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

@test "new --ephemeral records ephemeral:true and ls annotates it" {
  STATE="$WORKSPACE_ROOT/state/state.yml"
  sdev -p widget new e1 --env local --no-fetch --ephemeral
  [ "$(K=widget/e1 yq -r '.tasks[strenv(K)].ephemeral' "$STATE")" = "true" ]
  run sdev -p widget ls
  [[ "$output" == *"widget/e1"* ]]
  [[ "$output" == *"[ephemeral]"* ]]
}

@test "--lease and --ephemeral are mutually exclusive" {
  run sdev -p widget new bad --env local --no-fetch --lease --ephemeral
  [ "$status" -ne 0 ]
  [[ "$output" == *"mutually exclusive"* ]]
}

@test "prune reclaims an ephemeral task but leaves a leased task untouched" {
  STATE="$WORKSPACE_ROOT/state/state.yml"
  sdev -p widget new eph --env local --no-fetch --ephemeral
  sdev -p widget new keep --env local --no-fetch --lease owner

  # Dry-run changes nothing.
  run sdev -p widget prune
  [ "$status" -eq 0 ]
  [[ "$output" == *"widget/eph"* ]]
  [[ "$output" == *"dry-run"* ]]
  [ -d "$WORKSPACE_ROOT/projects/widget/eph" ]     # untouched by dry-run

  run sdev -p widget prune --apply
  [ "$status" -eq 0 ]
  # Ephemeral reclaimed: workspace gone, ledger entry gone.
  [ ! -d "$WORKSPACE_ROOT/projects/widget/eph" ]
  [ "$(K=widget/eph yq -r '.tasks | has(strenv(K))' "$STATE")" = "false" ]
  # Leased task left fully intact.
  [ -d "$WORKSPACE_ROOT/projects/widget/keep" ]
  [ "$(K=widget/keep yq -r '.tasks | has(strenv(K))' "$STATE")" = "true" ]
  [ "$(K=widget/keep yq -r '.tasks[strenv(K)].lease' "$STATE")" = "true" ]
}

@test "prune releases an ephemeral PORT_OFFSET back to the ledger (no leak, no double-allocation)" {
  STATE="$WORKSPACE_ROOT/state/state.yml"
  sdev -p widget new eph --env local --no-fetch --ephemeral
  ephoff="$(K=widget/eph yq -r '.tasks[strenv(K)].offset' "$STATE")"
  sdev -p widget new keep --env local --no-fetch --lease owner
  keepoff="$(K=widget/keep yq -r '.tasks[strenv(K)].offset' "$STATE")"

  sdev -p widget prune --apply
  # The ephemeral offset is no longer reserved anywhere.
  run bash -c "yq -r '.tasks[].offset' '$STATE' | grep -qx '$ephoff'"
  [ "$status" -ne 0 ]

  # A fresh task reuses the freed offset — proof it was released, not leaked.
  sdev -p widget new reuse --env local --no-fetch
  reuseoff="$(grep '^PORT_OFFSET=' "$WORKSPACE_ROOT/projects/widget/reuse/.env" | cut -d= -f2)"
  [ "$reuseoff" = "$ephoff" ]

  # And no two live tasks share an offset (no double-allocation).
  dupes="$(yq -r '.tasks[].offset' "$STATE" | sort | uniq -d)"
  [ -z "$dupes" ]
  [ "$reuseoff" != "$keepoff" ]
}

@test "prune never reclaims an ephemeral task that holds a live process-lock" {
  STATE="$WORKSPACE_ROOT/state/state.yml"
  sdev -p widget new eph --env local --no-fetch --ephemeral
  # Hold it with THIS test shell (alive) so its process-lock is live.
  sdev -p widget hold eph --pid "$$"
  run sdev -p widget prune --apply
  [ "$status" -eq 0 ]
  [[ "$output" == *"protected"* ]]
  [ -d "$WORKSPACE_ROOT/projects/widget/eph" ]                    # not reclaimed
  [ "$(K=widget/eph yq -r '.tasks | has(strenv(K))' "$STATE")" = "true" ]
}

@test "prune drops an abandoned ledger entry (workspace gone, no lease/lock) and frees its offset" {
  STATE="$WORKSPACE_ROOT/state/state.yml"
  source "$WORKSPACE_ROOT/bin/_lib.sh"
  state_init
  # A leaked entry: offset reserved, but no workspace, not leased, no live lock.
  K="widget/ghost" yq -i '.seeded = true | .tasks[strenv(K)] = {"offset":10,"created_at":"t","lease":false,"lease_holder":"","pid":0,"proc_token":"","ephemeral":false}' "$STATE"
  run "$WORKSPACE_ROOT/bin/sdev" -p widget prune --apply
  [ "$status" -eq 0 ]
  [ "$(K=widget/ghost yq -r '.tasks | has(strenv(K))' "$STATE")" = "false" ]
}

@test "end on an ephemeral task never returns the worktree to the warm pool" {
  STATE="$WORKSPACE_ROOT/state/state.yml"
  sdev -p widget new eph --env local --no-fetch --ephemeral
  run sdev -p widget end eph --pool --force
  [ "$status" -eq 0 ]
  [[ "$output" == *"ephemeral"* ]]
  [ ! -d "$WORKSPACE_ROOT/projects/widget/eph" ]
  [ "$(yq -r '.pool | length' "$STATE")" = "0" ]          # nothing pooled
  [ "$(K=widget/eph yq -r '.tasks | has(strenv(K))' "$STATE")" = "false" ]
}

@test "prune --pool drains pooled worktrees without touching a live task" {
  STATE="$WORKSPACE_ROOT/state/state.yml"
  # A pooled worktree from a normal task.
  sdev -p widget new pooled --env local --no-fetch
  sdev -p widget end pooled --pool --force
  [ "$(yq -r '.pool | length' "$STATE")" = "1" ]
  poolpath="$(yq -r '.pool[0].path' "$STATE")"
  [ -d "$poolpath" ]

  # A separate live task that must be left alone.
  sdev -p widget new live --env local --no-fetch

  run sdev -p widget prune --pool --apply
  [ "$status" -eq 0 ]
  # Pool drained: entry gone AND the on-disk worktree removed.
  [ "$(yq -r '.pool | length' "$STATE")" = "0" ]
  [ ! -d "$poolpath" ]
  # Live task untouched.
  [ -d "$WORKSPACE_ROOT/projects/widget/live" ]
  [ "$(K=widget/live yq -r '.tasks | has(strenv(K))' "$STATE")" = "true" ]
}

@test "prune --pool drops a stale pool entry whose worktree vanished" {
  STATE="$WORKSPACE_ROOT/state/state.yml"
  sdev -p widget new pooled --env local --no-fetch
  sdev -p widget end pooled --pool --force
  poolpath="$(yq -r '.pool[0].path' "$STATE")"
  rm -rf "$poolpath"                                   # worktree vanishes out from under the ledger
  run sdev -p widget prune --apply                     # even without --pool, stale entries are dropped
  [ "$status" -eq 0 ]
  [[ "$output" == *"stale pool"* ]]
  [ "$(yq -r '.pool | length' "$STATE")" = "0" ]
}

@test "destroy force-removes a task (worktree + offset + entry); refuses a live lease without --force" {
  STATE="$WORKSPACE_ROOT/state/state.yml"
  sdev -p widget new held --env local --no-fetch --lease owner

  # Refused while leased.
  run sdev -p widget destroy held
  [ "$status" -ne 0 ]
  [[ "$output" == *"live lease"* ]]
  [ -d "$WORKSPACE_ROOT/projects/widget/held" ]

  # --force overrides.
  run sdev -p widget destroy held --force
  [ "$status" -eq 0 ]
  [ ! -d "$WORKSPACE_ROOT/projects/widget/held" ]
  [ "$(K=widget/held yq -r '.tasks | has(strenv(K))' "$STATE")" = "false" ]
  # Branch is gone too.
  run git -C "$WORKSPACE_ROOT/core/widget/svc" rev-parse --verify task/held
  [ "$status" -ne 0 ]
}

@test "doctor stays green after ephemeral create + prune" {
  sdev -p widget new eph --env local --no-fetch --ephemeral
  sdev -p widget new keep --env local --no-fetch
  sdev -p widget prune --apply
  run "$WORKSPACE_ROOT/bin/sdev" doctor
  [ "$status" -eq 0 ]
  [[ "$output" == *"doctor: OK"* ]]
}
