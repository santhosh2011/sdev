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

@test "edit add: symlinks a local repo and writes the YAML block" {
  WEB="$(mktemp -d)/web"; make_source_repo "$WEB" main
  run edit acme <<EOF
a
web
$WEB
main
ui
q
EOF
  [ "$status" -eq 0 ]
  run yq -r '.repos.web.path' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "web" ]
  run yq -r '.repos.web.compose_role' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "ui" ]
  [ -L "$SDEV_TARGET/core/acme/web" ]
  rm -rf "$(dirname "$WEB")"
}

@test "edit add: refuses a duplicate repo name" {
  run edit acme <<EOF
a
api
$SRC
main
api
q
EOF
  [ "$status" -eq 0 ]
  run yq -r '.repos | keys | length' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "1" ]
}

@test "edit remove: deletes the YAML block and unlinks a symlinked source" {
  run edit acme <<EOF
r
api
q
EOF
  [ "$status" -eq 0 ]
  run yq -r '.repos | has("api")' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "false" ]
  [ ! -e "$SDEV_TARGET/core/acme/api" ]
  [ -d "$SRC/.git" ]
}

@test "edit remove: keeps a cloned source by default" {
  edit acme <<EOF
a
clone
file://$SRC
main
api
q
EOF
  [ -d "$SDEV_TARGET/core/acme/clone/.git" ]
  run edit acme <<EOF
r
clone
q
EOF
  [ "$status" -eq 0 ]
  run yq -r '.repos | has("clone")' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "false" ]
  [ -d "$SDEV_TARGET/core/acme/clone/.git" ]
}
