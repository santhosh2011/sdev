load helpers
setup() {
  make_fixture; source "$WORKSPACE_ROOT/bin/_lib.sh"
  echo 'conf_prefix: y' > "$WORKSPACE_ROOT/core/projects.d/default.yml"
  echo 'conf_prefix: p' > "$WORKSPACE_ROOT/core/projects.d/acme.yml"
  export TERM_SESSION_ID="bats-$$"
  export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR"
}
teardown() { rm -rf "$WORKSPACE_ROOT"; unset SDEV_PROJECT; }

@test "flag wins" {
  unset SDEV_PROJECT
  run resolve_project acme
  [ "$output" = "acme" ]
}
@test "env beats default" {
  export SDEV_PROJECT=acme
  run resolve_project ""
  [ "$output" = "acme" ]
}
@test "session pointer beats default" {
  unset SDEV_PROJECT
  mkdir -p "$(session_project_dir)"
  echo acme > "$(session_project_pointer)"
  run resolve_project ""
  [ "$output" = "acme" ]
}
@test "default_project from global config" {
  unset SDEV_PROJECT
  rm -rf "$(session_project_dir)"
  run resolve_project ""
  [ "$output" = "default" ]
}
@test "require_project errors on unknown" {
  run require_project nope
  [ "$status" -ne 0 ]
  [[ "$output" == *"unknown project"* ]]
}
@test "require_project accepts implicit default without project file" {
  rm -f "$WORKSPACE_ROOT/core/projects.d/default.yml"
  run require_project default
  [ "$status" -eq 0 ]
  [ "$output" = "default" ]
}
