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

@test "two concurrent 'sdev new' never receive the same PORT_OFFSET" {
  # Launch two `sdev new` at once. Before this feature both scanned for a free
  # offset before either wrote its .env and collided; the state lock fixes it.
  ( sdev -p widget new race-a --env local --no-fetch >/dev/null 2>&1 ) &
  ( sdev -p widget new race-b --env local --no-fetch >/dev/null 2>&1 ) &
  wait
  a="$(grep '^PORT_OFFSET=' "$WORKSPACE_ROOT/projects/widget/race-a/.env" | cut -d= -f2)"
  b="$(grep '^PORT_OFFSET=' "$WORKSPACE_ROOT/projects/widget/race-b/.env" | cut -d= -f2)"
  echo "race-a=$a race-b=$b"
  [ -n "$a" ] && [ -n "$b" ]
  [ "$a" != "$b" ]
}

@test "many concurrent offset allocations are all distinct (state lock)" {
  source "$WORKSPACE_ROOT/bin/_lib.sh"
  n=12
  for i in $(seq 1 "$n"); do
    mkdir -p "$WORKSPACE_ROOT/projects/widget/t$i"    # dir exists so reconcile keeps it
    ( allocate_offset "widget/t$i" 0 "" > "$TMPDIR/off.$i" 2>/dev/null ) &
  done
  wait
  offs=()
  for i in $(seq 1 "$n"); do offs+=("$(cat "$TMPDIR/off.$i")"); done
  echo "offsets: ${offs[*]}"
  uniq="$(printf '%s\n' "${offs[@]}" | sort -u | grep -c .)"
  [ "$uniq" -eq "$n" ]
}

@test "seeding: a fresh ledger adopts existing task offsets (no collision)" {
  source "$WORKSPACE_ROOT/bin/_lib.sh"
  # Pre-existing on-disk tasks with no ledger yet.
  mkdir -p "$WORKSPACE_ROOT/projects/widget/x" "$WORKSPACE_ROOT/projects/widget/y"
  echo 'PORT_OFFSET=10' > "$WORKSPACE_ROOT/projects/widget/x/.env"
  echo 'PORT_OFFSET=20' > "$WORKSPACE_ROOT/projects/widget/y/.env"
  mkdir -p "$WORKSPACE_ROOT/projects/widget/z"
  run allocate_offset "widget/z" 0 ""
  [ "$status" -eq 0 ]
  [ "$output" = "30" ]     # 10 and 20 are seeded as used
}

@test "leased tasks are not reclaimed (offset stays reserved with no workspace)" {
  source "$WORKSPACE_ROOT/bin/_lib.sh"
  state_init
  # Leased task at offset 10 with NO workspace dir on disk.
  K="widget/held" yq -i '.seeded = true | .tasks[strenv(K)] = {"offset":10,"created_at":"t","lease":true,"lease_holder":"bg","pid":0,"proc_token":""}' "$WORKSPACE_ROOT/state/state.yml"
  mkdir -p "$WORKSPACE_ROOT/projects/widget/new"
  run allocate_offset "widget/new" 0 ""
  [ "$status" -eq 0 ]
  [ "$output" = "20" ]     # 10 stays reserved for the lease, so 20 is next
  [ "$(K=widget/held yq -r '.tasks | has(strenv(K))' "$WORKSPACE_ROOT/state/state.yml")" = "true" ]
}

@test "dead process-locks self-heal (stale reservation is reclaimed)" {
  source "$WORKSPACE_ROOT/bin/_lib.sh"
  state_init
  # Task at offset 10, pid long dead, not leased, no workspace dir.
  K="widget/dead" yq -i '.seeded = true | .tasks[strenv(K)] = {"offset":10,"created_at":"t","lease":false,"lease_holder":"","pid":999999,"proc_token":"gone"}' "$WORKSPACE_ROOT/state/state.yml"
  mkdir -p "$WORKSPACE_ROOT/projects/widget/new"
  run allocate_offset "widget/new" 0 ""
  [ "$status" -eq 0 ]
  [ "$output" = "10" ]     # dead lock reclaimed, 10 reused
}

@test "a live process-lock is NOT reclaimed" {
  source "$WORKSPACE_ROOT/bin/_lib.sh"
  state_init
  # Lock held by THIS test's shell (alive), no workspace dir.
  tok="$(_proc_token "$$")"
  K="widget/live" P="$$" TK="$tok" yq -i '.seeded = true | .tasks[strenv(K)] = {"offset":10,"created_at":"t","lease":false,"lease_holder":"","pid":(strenv(P)|tonumber),"proc_token":strenv(TK)}' "$WORKSPACE_ROOT/state/state.yml"
  mkdir -p "$WORKSPACE_ROOT/projects/widget/new"
  run allocate_offset "widget/new" 0 ""
  [ "$status" -eq 0 ]
  [ "$output" = "20" ]     # live lock keeps 10 reserved
}

@test "sdev lease / release round-trips and surfaces in ls" {
  sdev -p widget new l1 --env local --no-fetch
  run sdev -p widget lease l1 nightowl
  [ "$status" -eq 0 ]
  run sdev -p widget ls
  [[ "$output" == *"widget/l1"* ]]
  [[ "$output" == *"leased:nightowl"* ]]
  run sdev -p widget release l1
  [ "$status" -eq 0 ]
  run sdev -p widget ls
  [[ "$output" != *"leased:nightowl"* ]]
}

@test "sdev hold records a process-lock; ls shows a dead one as stale" {
  sdev -p widget new h1 --env local --no-fetch
  # Hold with a PID that is already dead so ls reports it stale.
  run sdev -p widget hold h1 --pid 999999
  [ "$status" -eq 0 ]
  run sdev -p widget ls
  [[ "$output" == *"lock:stale"* ]]
}

@test "the state lock self-heals when its holder is dead" {
  source "$WORKSPACE_ROOT/bin/_lib.sh"
  mkdir -p "$SDEV_STATE_DIR"
  ln -s 999999 "$STATE_LOCK"         # dead holder, encoded in the lock symlink
  run with_state_lock true           # must break the stale lock and proceed
  [ "$status" -eq 0 ]
  [ ! -e "$STATE_LOCK" ]             # released afterwards
}

@test "state lock carries the holder pid the instant it is held (no pid-less window)" {
  source "$WORKSPACE_ROOT/bin/_lib.sh"
  mkdir -p "$SDEV_STATE_DIR"
  _held_is_symlink_to_me() { [[ -L "$STATE_LOCK" ]] && [[ "$(readlink "$STATE_LOCK")" == "$$"* ]]; }
  run with_state_lock _held_is_symlink_to_me
  [ "$status" -eq 0 ]
  [ ! -e "$STATE_LOCK" ]
}

@test "the state lock self-heals a legacy dir lock left by an older sdev" {
  source "$WORKSPACE_ROOT/bin/_lib.sh"
  mkdir -p "$SDEV_STATE_DIR"
  mkdir "$STATE_LOCK"                # old scheme: a dir
  echo 999999 > "$STATE_LOCK/pid"    # dead holder
  run with_state_lock true
  [ "$status" -eq 0 ]
  [ ! -e "$STATE_LOCK" ]
}
