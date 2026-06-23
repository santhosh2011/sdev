HOOKS="$(cd "$BATS_TEST_DIRNAME/../claude/hooks" && pwd)"

# Build a fake sdev task dir: $1=profile (default local). Echoes the task dir path.
make_task_dir() {
  local prof="${1:-local}"
  local root; root="$(mktemp -d)"
  local td="$root/projects/proj/my-slug"
  mkdir -p "$td"
  cat > "$td/.env" <<EOF
COMPOSE_PROJECT_NAME=proj-my-slug
PORT_OFFSET=10
APP_ENV=$prof
NGINX_HOST_PORT=8090
EOF
  : > "$td/CLAUDE.md"
  printf '%s' "$td"
}

@test "session-context: injects task identity when cwd is in a task dir" {
  td="$(make_task_dir local)"
  run bash -c "printf '%s' '{\"cwd\":\"$td\",\"hook_event_name\":\"SessionStart\"}' | '$HOOKS/sdev-session-context'"
  [ "$status" -eq 0 ]
  [[ "$output" == *"my-slug"* ]]
  [[ "$output" == *"proj"* ]]
  [[ "$output" == *"8090"* ]]
  [[ "$output" == *"additionalContext"* ]]
  rm -rf "$(dirname "$(dirname "$(dirname "$td")")")"
}

@test "session-context: silent no-op outside a task dir" {
  run bash -c "printf '%s' '{\"cwd\":\"/tmp\",\"hook_event_name\":\"SessionStart\"}' | '$HOOKS/sdev-session-context'"
  [ "$status" -eq 0 ]
  [ -z "$output" ]
}

@test "session-context: silent no-op on empty stdin" {
  run bash -c "printf '' | '$HOOKS/sdev-session-context'"
  [ "$status" -eq 0 ]
  [ -z "$output" ]
}
