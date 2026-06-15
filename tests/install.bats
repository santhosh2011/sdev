setup() {
  REPO="$(cd "$BATS_TEST_DIRNAME/.." && pwd)"
  INST="$(mktemp -d)/install"
  HOMEDIR="$(mktemp -d)/home"
  BINDIR="$(mktemp -d)/bin"
  FAKEHOME="$(mktemp -d)/fakehome"
  mkdir -p "$FAKEHOME"
}
teardown() {
  rm -rf "$(dirname "$INST")" "$(dirname "$HOMEDIR")" \
         "$(dirname "$BINDIR")" "$(dirname "$FAKEHOME")"
}

# Non-interactive: SDEV_HOME provided via env (CI / power users). No prompt, no rc edits.
run_install() {
  env -u WORKSPACE_ROOT HOME="$FAKEHOME" SHELL=/bin/zsh \
      SDEV_INSTALL="$INST" SDEV_HOME="$HOMEDIR" SDEV_BIN_DIR="$BINDIR" \
      bash "$REPO/install"
}

# Interactive path: SDEV_HOME unset, project home supplied on stdin. $1 = piped input.
run_install_prompt() {
  printf '%s\n' "$1" | env -u SDEV_HOME -u WORKSPACE_ROOT HOME="$FAKEHOME" SHELL=/bin/zsh \
      SDEV_INSTALL="$INST" SDEV_BIN_DIR="$BINDIR" \
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
  # env-provided home: installer must not touch the user's rc
  [ ! -f "$FAKEHOME/.zshrc" ]
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

@test "prompt: uses project home from stdin and persists to rc" {
  PICK="$(mktemp -d)/picked"
  run run_install_prompt "$PICK"
  [ "$status" -eq 0 ]
  [ -d "$PICK/core/projects.d" ]
  [ -f "$PICK/core/.task-config.yml" ]
  [ -f "$FAKEHOME/.zshrc" ]
  run grep -F "$PICK" "$FAKEHOME/.zshrc"
  [ "$status" -eq 0 ]
  run grep -F "export SDEV_HOME" "$FAKEHOME/.zshrc"
  [ "$status" -eq 0 ]
  rm -rf "$(dirname "$PICK")"
}

@test "prompt: empty input falls back to default ~/.sdev" {
  run run_install_prompt ""
  [ "$status" -eq 0 ]
  [ -d "$FAKEHOME/.sdev/core/projects.d" ]
}

@test "prompt: tilde input expands to HOME" {
  run run_install_prompt "~/devhome"
  [ "$status" -eq 0 ]
  [ -d "$FAKEHOME/devhome/core/projects.d" ]
}

@test "prompt: rejects a file path as project home" {
  FILEPATH="$(mktemp)"
  run run_install_prompt "$FILEPATH"
  [ "$status" -ne 0 ]
  rm -f "$FILEPATH"
}

@test "prompt: rc persistence is idempotent (single sdev block)" {
  PICK="$(mktemp -d)/picked2"
  run run_install_prompt "$PICK"
  [ "$status" -eq 0 ]
  run run_install_prompt "$PICK"
  [ "$status" -eq 0 ]
  run grep -c ">>> sdev >>>" "$FAKEHOME/.zshrc"
  [ "$output" -eq 1 ]
  rm -rf "$(dirname "$PICK")"
}
