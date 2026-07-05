setup() {
  REPO="$(cd "$BATS_TEST_DIRNAME/.." && pwd)"
  OUT="$(mktemp -d)"
}
teardown() { rm -rf "$OUT"; }

@test "dist produces a zip with tool code and reference assets only" {
  run bash "$REPO/dist" "$OUT"
  [ "$status" -eq 0 ]
  zipfile="$output"
  [ -f "$zipfile" ]
  listing="$(unzip -Z1 "$zipfile")"
  echo "$listing" | grep -qx 'sdev/bin/sdev'
  echo "$listing" | grep -qx 'sdev/bin/_lib.sh'
  echo "$listing" | grep -qx 'sdev/install'
  echo "$listing" | grep -qx 'sdev/core/.task-config.yml'
  echo "$listing" | grep -qx 'sdev/core/projects.d/example.yml'
  echo "$listing" | grep -qx 'sdev/confs/example/app.local.env.example'
  # exclusions: no live workspaces, no real env files
  ! echo "$listing" | grep -q '/projects/'
  ! echo "$listing" | grep -qE '\.env$'
}

@test "dist works with no explicit output dir (default build/)" {
  run bash "$REPO/dist"
  [ "$status" -eq 0 ]
  [ -f "$output" ]
  [[ "$output" == "$REPO/build/"* ]]
  rm -rf "$REPO/build"
}

@test "dist emits a .sha256 checksum next to the zip" {
  run bash "$REPO/dist" "$OUT"
  [ "$status" -eq 0 ]
  zipfile="$output"
  [ -f "$zipfile.sha256" ]
  ( cd "$(dirname "$zipfile")" && shasum -a 256 -c "$(basename "$zipfile").sha256" )
}

@test "dist ships the Claude skill + command" {
  run bash "$REPO/dist" "$OUT"
  [ "$status" -eq 0 ]
  zipfile="$output"
  listing="$(unzip -Z1 "$zipfile")"
  echo "$listing" | grep -qx 'sdev/claude/skills/sdev/SKILL.md'
  echo "$listing" | grep -qx 'sdev/claude/commands/sdev-start.md'
}

@test "dist ships the hook scripts" {
  run bash "$REPO/dist" "$OUT"
  [ "$status" -eq 0 ]
  zipfile="$output"
  listing="$(unzip -Z1 "$zipfile")"
  echo "$listing" | grep -qx 'sdev/claude/hooks/sdev-session-context'
  echo "$listing" | grep -qx 'sdev/claude/hooks/sdev-staging-guard'
  echo "$listing" | grep -qx 'sdev/claude/hooks/sdev-edit-reminder'
}

@test "dist ships the network installer + self-update engine (executable)" {
  run bash "$REPO/dist" "$OUT"
  [ "$status" -eq 0 ]
  zipfile="$output"
  listing="$(unzip -Z1 "$zipfile")"
  echo "$listing" | grep -qx 'sdev/install.sh'
  echo "$listing" | grep -qx 'sdev/self-update'
  # Executable bit survives into the archive (checks the -A permission field).
  unzip -Z "$zipfile" 'sdev/install.sh'  | grep -qE '^-.{2}x'
  unzip -Z "$zipfile" 'sdev/self-update' | grep -qE '^-.{2}x'
}

@test "dist ships a clean bare VERSION (no release-please annotation)" {
  run bash "$REPO/dist" "$OUT"
  [ "$status" -eq 0 ]
  zipfile="$output"
  ver="$(unzip -p "$zipfile" sdev/VERSION)"
  # Repo VERSION carries a '# x-release-please-version' comment; the shipped one must not.
  [[ "$ver" != *"#"* ]]
  [[ "$ver" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]
  # And the zip is named for that bare version.
  [[ "$(basename "$zipfile")" == "sdev-$ver.zip" ]]
}
