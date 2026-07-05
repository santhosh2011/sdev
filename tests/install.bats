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

# Non-interactive install with the Claude opt-in decided by SDEV_CLAUDE.
# Note: the interactive [Y/n] prompt branch in maybe_install_claude ([[ -t 0 ]])
# is intentionally exercised only via SDEV_CLAUDE env paths below, since bats
# pipes stdin so -t 0 is false, and the prompt branch shares the same copy code.
run_install_claude() {
  env -u WORKSPACE_ROOT HOME="$FAKEHOME" SHELL=/bin/zsh \
      SDEV_INSTALL="$INST" SDEV_HOME="$HOMEDIR" SDEV_BIN_DIR="$BINDIR" \
      SDEV_CLAUDE="$1" \
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
  [[ "$output" == *"✓ git"* ]]
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

@test "claude: SDEV_CLAUDE=1 installs skill + command into ~/.claude" {
  run run_install_claude 1
  [ "$status" -eq 0 ]
  [ -f "$FAKEHOME/.claude/skills/sdev/SKILL.md" ]
  [ -f "$FAKEHOME/.claude/commands/sdev-start.md" ]
}

@test "claude: SDEV_CLAUDE=0 skips the Claude install" {
  run run_install_claude 0
  [ "$status" -eq 0 ]
  [ ! -e "$FAKEHOME/.claude/skills/sdev" ]
  [ ! -e "$FAKEHOME/.claude/commands/sdev-start.md" ]
}

@test "claude: non-interactive with no opt-in does not create ~/.claude" {
  run run_install
  [ "$status" -eq 0 ]
  [ ! -e "$FAKEHOME/.claude" ]
}

@test "claude: existing ~/.claude is not auto-populated without opt-in" {
  mkdir -p "$FAKEHOME/.claude"
  run run_install
  [ "$status" -eq 0 ]
  [ ! -e "$FAKEHOME/.claude/skills/sdev" ]
  [ ! -e "$FAKEHOME/.claude/commands/sdev-start.md" ]
}

@test "claude: SDEV_CLAUDE=1 merges the staging-guard into ~/.claude/settings.json" {
  run run_install_claude 1
  [ "$status" -eq 0 ]
  s="$FAKEHOME/.claude/settings.json"
  [ -f "$s" ]
  run bash -c "yq -p=json -r '[.hooks.PreToolUse[].hooks[].command] | join(\",\")' \"$s\" 2>/dev/null"
  [[ "$output" == *"claude/hooks/sdev-staging-guard"* ]]
}

@test "claude: global staging-guard merge is idempotent (no duplicate)" {
  run run_install_claude 1
  [ "$status" -eq 0 ]
  run run_install_claude 1
  [ "$status" -eq 0 ]
  s="$FAKEHOME/.claude/settings.json"
  run bash -c "yq -p=json -r '[.hooks.PreToolUse[].hooks[].command | select(. == \"$INST/claude/hooks/sdev-staging-guard\")] | length' \"$s\" 2>/dev/null"
  [ "$output" = "1" ]
}

@test "claude: global merge preserves an existing unrelated hook" {
  mkdir -p "$FAKEHOME/.claude"
  cat > "$FAKEHOME/.claude/settings.json" <<'JSON'
{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"/usr/local/bin/my-own-hook"}]}]}}
JSON
  run run_install_claude 1
  [ "$status" -eq 0 ]
  s="$FAKEHOME/.claude/settings.json"
  run bash -c "yq -p=json -r '[.hooks.PreToolUse[].hooks[].command] | join(\",\")' \"$s\" 2>/dev/null"
  [[ "$output" == *"/usr/local/bin/my-own-hook"* ]]
  [[ "$output" == *"sdev-staging-guard"* ]]
}

@test "claude: no opt-in does not create ~/.claude/settings.json" {
  run run_install
  [ "$status" -eq 0 ]
  [ ! -e "$FAKEHOME/.claude/settings.json" ]
}

# docker is a RUNTIME dependency (needed for 'sdev up'), not an install-time one.
# The installer must succeed — with a warning — when docker is absent, so the
# curl installer works on a fresh machine and CI can run on Docker-less runners
# (GitHub's macOS runners have no Docker). git/bash/yq stay hard requirements.
@test "install treats docker as runtime-only: succeeds (warns) when docker is absent" {
  # Mirror the whole PATH into a stub dir, then drop docker → a hermetic,
  # Docker-less environment regardless of what the test host has installed.
  # First occurrence wins (plain `ln -s`, no -f) so PATH precedence is preserved
  # — e.g. Homebrew's bash 5 shadows /bin/bash 3.2, exactly as a real shell resolves.
  local stub d f name
  stub="$(mktemp -d)/nodocker"; mkdir -p "$stub"
  IFS=: read -ra _dirs <<< "$PATH"
  for d in "${_dirs[@]}"; do
    [ -d "$d" ] || continue
    for f in "$d"/*; do
      [ -x "$f" ] || continue
      name="$(basename "$f")"
      [ -e "$stub/$name" ] || ln -s "$f" "$stub/$name" 2>/dev/null || true
    done
  done
  rm -f "$stub/docker"
  [ ! -e "$stub/docker" ]
  run env -i HOME="$FAKEHOME" SHELL=/bin/zsh PATH="$stub" \
      SDEV_INSTALL="$INST" SDEV_HOME="$HOMEDIR" SDEV_BIN_DIR="$BINDIR" \
      "$stub/bash" "$REPO/install"
  [ "$status" -eq 0 ]
  [ -x "$INST/bin/sdev" ]
  [[ "$output" == *"docker not found"* ]]
  rm -rf "$(dirname "$stub")"
}
