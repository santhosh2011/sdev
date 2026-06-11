setup() {
  REPO="$(cd "$BATS_TEST_DIRNAME/.." && pwd)"
  INST="$(mktemp -d)/install"
  HOMEDIR="$(mktemp -d)/home"
  BINDIR="$(mktemp -d)/bin"
}
teardown() { rm -rf "$(dirname "$INST")" "$(dirname "$HOMEDIR")" "$(dirname "$BINDIR")"; }

run_install() {
  env SDEV_INSTALL="$INST" SDEV_HOME="$HOMEDIR" SDEV_BIN_DIR="$BINDIR" \
      bash "$REPO/install"
}

@test "install places tool, seeds home, links sdev" {
  run run_install
  [ "$status" -eq 0 ]
  [ -x "$INST/bin/sdev" ]
  [ -f "$INST/core/.task-config.yml" ]
  [ -f "$HOMEDIR/core/.task-config.yml" ]
  [ -d "$HOMEDIR/core/projects.d" ]
  [ -L "$BINDIR/sdev" ]
  [ "$(readlink "$BINDIR/sdev")" = "$INST/bin/sdev" ]
  [[ "$output" == *"export PATH"* ]]
}

@test "install is idempotent and preserves user data" {
  run run_install
  [ "$status" -eq 0 ]
  echo 'conf_prefix: keep' > "$HOMEDIR/core/projects.d/keep.yml"
  run run_install
  [ "$status" -eq 0 ]
  [ -f "$HOMEDIR/core/projects.d/keep.yml" ]
  run cat "$HOMEDIR/core/projects.d/keep.yml"
  [[ "$output" == *"keep"* ]]
}
