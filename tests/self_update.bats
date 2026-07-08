# End-to-end coverage for the distribution layer: the network installer
# (install.sh) and the self-update engine (self-update), exercised fully offline
# via SDEV_DIST_ZIP so no network / published release is needed.
setup() {
  REPO="$(cd "$BATS_TEST_DIRNAME/.." && pwd)"
  SCRATCH="$(mktemp -d)"
  OUT="$SCRATCH/out";   mkdir -p "$OUT"
  INST="$SCRATCH/inst"
  HOMEDIR="$SCRATCH/home"
  BINDIR="$SCRATCH/bin"
  FAKEHOME="$SCRATCH/fakehome"; mkdir -p "$FAKEHOME"
}
teardown() { rm -rf "$SCRATCH"; }

# Build a distributable zip pinned to an arbitrary bare <version>, plus its
# .sha256, into $OUT. Reuses ./dist for the real packaging, then re-stamps
# VERSION so a test can simulate an upgrade without touching the repo's VERSION.
# Prints the absolute zip path.
build_zip() {
  local ver="$1" stage base un out
  stage="$(mktemp -d)"
  bash "$REPO/dist" "$stage/base" >/dev/null
  base="$(echo "$stage/base"/*.zip)"
  un="$stage/un"; mkdir -p "$un"
  unzip -q "$base" -d "$un"
  printf '%s\n' "$ver" > "$un/sdev/VERSION"
  out="$OUT/sdev-$ver.zip"
  rm -f "$out"
  ( cd "$un" && zip -rq "$out" sdev )
  ( cd "$OUT" && shasum -a 256 "sdev-$ver.zip" > "sdev-$ver.zip.sha256" )
  rm -rf "$stage"
  printf '%s' "$out"
}

# Run install.sh / self-update non-interactively against the scratch dirs.
run_installer() {  # run_installer <script-abs-path> [extra env assignments as VAR=val ...]
  local script="$1"; shift
  env -u WORKSPACE_ROOT HOME="$FAKEHOME" SHELL=/bin/zsh \
      SDEV_INSTALL="$INST" SDEV_HOME="$HOMEDIR" SDEV_BIN_DIR="$BINDIR" \
      "$@" bash "$script" </dev/null
}

@test "install.sh installs from a local zip offline, links sdev + sdev-update" {
  zip="$(build_zip 1.2.3)"
  run run_installer "$REPO/install.sh" SDEV_DIST_ZIP="$zip"
  [ "$status" -eq 0 ]
  [ -x "$INST/bin/sdev" ]
  [ -x "$INST/self-update" ]
  [ -f "$INST/install.sh" ]
  [ "$(cat "$INST/VERSION")" = "1.2.3" ]
  [ -L "$BINDIR/sdev" ]
  [ -L "$BINDIR/sdev-update" ]
  [ "$(readlink "$BINDIR/sdev-update")" = "$INST/self-update" ]
  [[ "$output" == *"checksum verified"* ]]
  # env-provided home: installer must not touch the user's rc
  [ ! -f "$FAKEHOME/.zshrc" ]
}

@test "install.sh refuses a corrupted zip (checksum mismatch)" {
  zip="$(build_zip 1.2.3)"
  printf 'tampered' >> "$zip"   # digest in .sha256 no longer matches
  run run_installer "$REPO/install.sh" SDEV_DIST_ZIP="$zip"
  [ "$status" -ne 0 ]
  [ ! -e "$INST/bin/sdev" ]
  [[ "$output" == *"checksum"* ]]
}

@test "install.sh rejects an unsupported platform" {
  zip="$(build_zip 1.2.3)"
  # find_bash4 in install.sh needs 'uname'; we only override its os gate output.
  run env -u WORKSPACE_ROOT HOME="$FAKEHOME" SDEV_DIST_ZIP="$zip" \
      SDEV_INSTALL="$INST" SDEV_HOME="$HOMEDIR" SDEV_BIN_DIR="$BINDIR" \
      bash -c 'uname() { echo Windows_NT; }; export -f uname; bash "$1"' _ "$REPO/install.sh"
  [ "$status" -ne 0 ]
  [[ "$output" == *"macOS and Linux only"* ]]
}

@test "self-update upgrades an installed sdev old -> new (offline)" {
  # 1) baseline install at 1.0.0
  z1="$(build_zip 1.0.0)"
  run run_installer "$REPO/install.sh" SDEV_DIST_ZIP="$z1"
  [ "$status" -eq 0 ]
  [ "$(cat "$INST/VERSION")" = "1.0.0" ]
  # user data written after install must survive the update
  echo 'conf_prefix: keep' > "$HOMEDIR/core/projects.d/keep.yml"

  # 2) self-update to 2.0.0 using the bundled self-update engine
  z2="$(build_zip 2.0.0)"
  run run_installer "$INST/self-update" SDEV_DIST_ZIP="$z2"
  [ "$status" -eq 0 ]
  [ "$(cat "$INST/VERSION")" = "2.0.0" ]
  [[ "$output" == *"1.0.0 → 2.0.0"* ]]
  # data preserved
  [ -f "$HOMEDIR/core/projects.d/keep.yml" ]
}

@test "self-update is a no-op when already current" {
  z1="$(build_zip 1.0.0)"
  run run_installer "$REPO/install.sh" SDEV_DIST_ZIP="$z1"
  [ "$status" -eq 0 ]
  run run_installer "$INST/self-update" SDEV_DIST_ZIP="$z1"
  [ "$status" -eq 0 ]
  [[ "$output" == *"up to date (1.0.0)"* ]]
}

@test "sdev update dispatches to the bundled self-update engine" {
  z1="$(build_zip 1.0.0)"
  run run_installer "$REPO/install.sh" SDEV_DIST_ZIP="$z1"
  [ "$status" -eq 0 ]
  # `sdev update --help` must reach self-update's usage.
  run env -u WORKSPACE_ROOT HOME="$FAKEHOME" SDEV_HOME="$HOMEDIR" "$INST/bin/sdev" update --help
  [ "$status" -eq 0 ]
  [[ "$output" == *"sdev update"* ]]
  [[ "$output" == *"SDEV_DIST_ZIP"* ]]
}

@test "sdev update fails gracefully when self-update is missing" {
  z1="$(build_zip 1.0.0)"
  run run_installer "$REPO/install.sh" SDEV_DIST_ZIP="$z1"
  [ "$status" -eq 0 ]
  rm -f "$INST/self-update"
  run env -u WORKSPACE_ROOT HOME="$FAKEHOME" SDEV_HOME="$HOMEDIR" "$INST/bin/sdev" update
  [ "$status" -ne 0 ]
  [[ "$output" == *"self-update not found"* ]]
}
