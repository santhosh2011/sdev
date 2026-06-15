load helpers
setup() {
  make_fixture
  SDEV_TARGET="$(mktemp -d)/home"; export SDEV_HOME="$SDEV_TARGET"
  mkdir -p "$SDEV_TARGET/core/projects.d" "$SDEV_TARGET/confs"
  SRC="$(mktemp -d)/svc"; make_source_repo "$SRC" main
  cat > "$SDEV_TARGET/core/projects.d/acme.yml" <<YAML
conf_prefix: app
default_shell_service: api
repos:
  api:
    path: api
    default_base: main
    compose_role: api
    link_node_modules: false
YAML
  mkdir -p "$SDEV_TARGET/core/acme"
  ln -s "$SRC" "$SDEV_TARGET/core/acme/api"
  export TERM_SESSION_ID="bats-$$"; export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR"
}
teardown() { rm -rf "$WORKSPACE_ROOT" "$(dirname "$SDEV_TARGET")" "$(dirname "$SRC")"; }

edit() { env SDEV_HOME="$SDEV_TARGET" SDEV_PROJECT=acme "$WORKSPACE_ROOT/bin/edit-project" "$@"; }

@test "edit shows the project summary then quits" {
  run edit acme <<<"q"
  [ "$status" -eq 0 ]
  [[ "$output" == *"conf_prefix"* ]]
  [[ "$output" == *"api"* ]]
}

@test "edit fails for an unknown project" {
  run edit nope <<<"q"
  [ "$status" -ne 0 ]
  [[ "$output" == *"not found"* ]]
}
