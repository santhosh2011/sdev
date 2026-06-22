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
