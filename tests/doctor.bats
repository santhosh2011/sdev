load helpers
setup() {
  make_fixture
  echo 'conf_prefix: y' > "$WORKSPACE_ROOT/core/projects.d/default.yml"
  export TERM_SESSION_ID="bats-$$"
  export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR"
}
teardown() { rm -rf "$WORKSPACE_ROOT"; }

@test "sdev doctor is a diagnostic, not a task slug (footgun fixed)" {
  run sdev doctor
  [ ! -d "$WORKSPACE_ROOT/projects/default/doctor" ]
  [ ! -d "$WORKSPACE_ROOT/projects/doctor" ]
}

@test "sdev doctor reports environment checks and exits 0 when tooling present" {
  run sdev doctor
  [ "$status" -eq 0 ]
  [[ "$output" == *"bash"* ]]
  [[ "$output" == *"yq"* ]]
  [[ "$output" == *"jq"* ]]
  [[ "$output" == *"docker"* ]]
}

@test "sdev doctor --json emits valid JSON with a checks array" {
  run sdev doctor --json
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.checks | type == "array"' >/dev/null
  echo "$output" | jq -e '.checks[] | select(.name == "jq")' >/dev/null
  echo "$output" | jq -e 'has("ok")' >/dev/null
}

@test "sdev doctor is listed as reserved (help mentions it)" {
  run sdev help
  [[ "$output" == *"doctor"* ]]
}
