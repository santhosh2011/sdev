setup() {
  REPO="$(cd "$BATS_TEST_DIRNAME/.." && pwd)"
}

@test "skill file exists with name and description frontmatter" {
  skill="$REPO/claude/skills/sdev/SKILL.md"
  [ -f "$skill" ]
  grep -qE '^name:[[:space:]]*sdev[[:space:]]*$' "$skill"
  grep -qE '^description:[[:space:]]*\S' "$skill"
}

@test "start command exists with description and argument-hint" {
  cmd="$REPO/claude/commands/sdev-start.md"
  [ -f "$cmd" ]
  grep -qE '^description:[[:space:]]*\S' "$cmd"
  grep -qE '^argument-hint:[[:space:]]*<slug>' "$cmd"
}
