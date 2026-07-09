load helpers
setup() {
  make_fixture
  export TERM_SESSION_ID="bats-$$"
  export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR"
}
teardown() { rm -rf "$WORKSPACE_ROOT"; }

# The migration's crux: bash allocate_offset and the Go `sdev-go alloc` must
# contend on the SAME state lock + ledger. A subtle divergence in the lock
# protocol reintroduces the duplicate-offset race, so this gates slice 2.
#
# The bug this guards against is a check-vs-reacquire TOCTOU in the stale-lock
# break: a waiter reads a since-exited holder's pid, judges it dead, and removes
# a lock that a NEW holder just acquired. It only surfaces when mixing bash
# (shared, always-live $$ target) with Go (per-process pids that die), so the
# test races both and asserts, across many rounds, zero duplicate offsets and
# zero DOUBLEHOLD reports from the lock's gated concurrency self-check.
@test "bash and Go allocators never double-hold the lock or collide on offsets" {
  [[ -x "$WORKSPACE_ROOT/bin/sdev-go" ]] || skip "sdev-go not built (bash-only run)"
  source "$WORKSPACE_ROOT/bin/_lib.sh"
  mkdir -p "$SDEV_STATE_DIR"
  export SDEV_LOCK_TRACE="$SDEV_STATE_DIR/trace"; : > "$SDEV_LOCK_TRACE"

  local rounds=25 n=12
  for r in $(seq 1 "$rounds"); do
    rm -rf "${SDEV_STATE_DIR:?}"/* "$WORKSPACE_ROOT/projects/default"
    mkdir -p "$SDEV_STATE_DIR"
    local i off
    for i in $(seq 1 "$n"); do
      mkdir -p "$WORKSPACE_ROOT/projects/default/t$i"
      if (( i % 2 == 0 )); then
        ( allocate_offset "default/t$i" 0 "" > "$TMPDIR/off.$i" 2>/dev/null ) &
      else
        ( "$WORKSPACE_ROOT/bin/sdev-go" alloc "default/t$i" > "$TMPDIR/off.$i" 2>/dev/null ) &
      fi
    done
    wait
    local offs=()
    for i in $(seq 1 "$n"); do offs+=("$(cat "$TMPDIR/off.$i" 2>/dev/null)"); done
    local uniq
    uniq="$(printf '%s\n' "${offs[@]}" | sort -u | grep -c .)"
    if [[ "$uniq" -ne "$n" ]]; then
      echo "round $r offset collision: ${offs[*]}"; return 1
    fi
  done

  # The lock's O_EXCL self-check appends a DOUBLEHOLD line if two holders were
  # ever inside the critical section at once.
  if grep -q DOUBLEHOLD "$SDEV_LOCK_TRACE"; then
    echo "double-hold detected:"; cat "$SDEV_LOCK_TRACE"; return 1
  fi
}

@test "a Go-written ledger is readable by bash yq (format interop)" {
  [[ -x "$WORKSPACE_ROOT/bin/sdev-go" ]] || skip "sdev-go not built"
  mkdir -p "$WORKSPACE_ROOT/projects/default/g1"
  run "$WORKSPACE_ROOT/bin/sdev-go" alloc "default/g1"
  [ "$status" -eq 0 ]
  [ "$output" = "10" ]
  [ "$(K=default/g1 yq -r '.tasks[strenv(K)].offset' "$WORKSPACE_ROOT/state/state.yml")" = "10" ]
}

@test "a bash-written ledger is readable by Go (format interop)" {
  [[ -x "$WORKSPACE_ROOT/bin/sdev-go" ]] || skip "sdev-go not built"
  source "$WORKSPACE_ROOT/bin/_lib.sh"
  mkdir -p "$WORKSPACE_ROOT/projects/default/b1" "$WORKSPACE_ROOT/projects/default/g2"
  allocate_offset "default/b1" 0 "" >/dev/null      # bash writes offset 10
  run "$WORKSPACE_ROOT/bin/sdev-go" alloc "default/g2"
  [ "$status" -eq 0 ]
  [ "$output" = "20" ]                              # Go read bash's 10, picked 20
}
