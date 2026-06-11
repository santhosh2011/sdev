load helpers
setup() {
  make_fixture
  SDEV_TARGET="$(mktemp -d)/home"
  SRC="$(mktemp -d)/svc"
  make_source_repo "$SRC" main
  export TERM_SESSION_ID="bats-$$"; export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR"
}
teardown() { rm -rf "$WORKSPACE_ROOT" "$(dirname "$SDEV_TARGET")" "$(dirname "$SRC")"; }

@test "init writes a project registry, links a local repo, and seeds a conf" {
  run env SDEV_HOME="$SDEV_TARGET" "$WORKSPACE_ROOT/bin/init" <<EOF
acme
app
api
svc
$SRC
main
api

EOF
  [ "$status" -eq 0 ]
  [ -f "$SDEV_TARGET/core/projects.d/acme.yml" ]
  run yq -r '.repos.svc.path' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "svc" ]
  run yq -r '.conf_prefix' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "app" ]
  [ -e "$SDEV_TARGET/core/acme/svc" ]
  [ -L "$SDEV_TARGET/core/acme/svc" ]                 # symlinked, not copied
  run yq -r '.default_shell_service' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "api" ]
  [ -f "$SDEV_TARGET/confs/acme/app.local.env" ]
}

@test "init aborts when no repos are added" {
  run env SDEV_HOME="$SDEV_TARGET" "$WORKSPACE_ROOT/bin/init" <<EOF
acme
app
api

EOF
  [ "$status" -ne 0 ]
}

@test "init links an existing repo given by a ~-relative path" {
  FAKE_HOME="$(mktemp -d)/h"; mkdir -p "$FAKE_HOME"
  make_source_repo "$FAKE_HOME/work/api" main
  run env HOME="$FAKE_HOME" SDEV_HOME="$SDEV_TARGET" "$WORKSPACE_ROOT/bin/init" <<EOF
acme
app
api
api
~/work/api
main
api

EOF
  [ "$status" -eq 0 ]
  [ -L "$SDEV_TARGET/core/acme/api" ]
  [ -e "$SDEV_TARGET/core/acme/api/.git" ]   # link resolves to the real repo
  [[ "$output" == *"linked"* ]]
  rm -rf "$(dirname "$FAKE_HOME")"
}

@test "init refuses to overwrite an existing project" {
  # first run creates acme
  env SDEV_HOME="$SDEV_TARGET" "$WORKSPACE_ROOT/bin/init" <<EOF
acme
app
api
svc
$SRC
main
api

EOF
  # second run with same name must fail
  run env SDEV_HOME="$SDEV_TARGET" "$WORKSPACE_ROOT/bin/init" <<EOF
acme
app
api
svc
$SRC
main
api

EOF
  [ "$status" -ne 0 ]
}
