load helpers
setup() {
  make_fixture
  SDEV_TARGET="$(mktemp -d)/home"; export SDEV_HOME="$SDEV_TARGET"
  mkdir -p "$SDEV_HOME"
  SRC="$(mktemp -d)/svc"; make_source_repo "$SRC" main
}
teardown() { rm -rf "$WORKSPACE_ROOT" "$(dirname "$SDEV_TARGET")" "$(dirname "$SRC")"; }

# Source _lib.sh from the fixture so SDEV_HOME resolution uses our target.
_load_lib() { SDEV_HOME="$SDEV_TARGET" source "$WORKSPACE_ROOT/bin/_lib.sh"; }

@test "add_repo_source symlinks an existing local repo" {
  _load_lib
  run add_repo_source acme api "$SRC"
  [ "$status" -eq 0 ]
  [ "$output" = "linked" ]
  [ -L "$SDEV_TARGET/core/acme/api" ]
  [ -e "$SDEV_TARGET/core/acme/api/.git" ]
}

@test "add_repo_source clones a URL (file:// remote)" {
  _load_lib
  run add_repo_source acme api "file://$SRC"
  [ "$status" -eq 0 ]
  [ "$output" = "cloned" ]
  [ -d "$SDEV_TARGET/core/acme/api/.git" ]
}

@test "add_repo_source rejects a non-repo path" {
  _load_lib
  notrepo="$(mktemp -d)"
  run add_repo_source acme api "$notrepo"
  [ "$status" -ne 0 ]
  rm -rf "$notrepo"
}

@test "add_repo_source refuses an already-existing dest" {
  _load_lib
  add_repo_source acme api "$SRC"
  run add_repo_source acme api "$SRC"
  [ "$status" -ne 0 ]
}
