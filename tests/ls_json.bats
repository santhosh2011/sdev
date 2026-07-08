load helpers
setup() {
  make_fixture
  echo 'conf_prefix: y' > "$WORKSPACE_ROOT/core/projects.d/default.yml"
  echo 'conf_prefix: p' > "$WORKSPACE_ROOT/core/projects.d/acme.yml"
  mkdir -p "$WORKSPACE_ROOT/projects/default/a" "$WORKSPACE_ROOT/projects/acme/b"
  printf 'PORT_OFFSET=10\nNGINX_HOST_PORT=8090\nCOMPOSE_PROJECT_NAME=default_a\n' > "$WORKSPACE_ROOT/projects/default/a/.env"
  printf 'PORT_OFFSET=20\nNGINX_HOST_PORT=8100\nCOMPOSE_PROJECT_NAME=acme_b\n' > "$WORKSPACE_ROOT/projects/acme/b/.env"
  mkdir -p "$WORKSPACE_ROOT/projects/_archive/acme/old"
  printf -- '- archive_date: 2026-06-01\n' > "$WORKSPACE_ROOT/projects/_archive/acme/old/ARCHIVE_INFO.md"
  export TERM_SESSION_ID="bats-$$"
  export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR"
}
teardown() { rm -rf "$WORKSPACE_ROOT"; }

@test "sdev ls --json lists alive tasks with url + totals" {
  run sdev ls --json
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.alive | length == 2' >/dev/null
  echo "$output" | jq -e '.alive[] | select(.task == "acme/b") | .nginx_port == 8100' >/dev/null
  echo "$output" | jq -e '.alive[] | select(.task == "acme/b") | .url == "http://localhost:8100/"' >/dev/null
  echo "$output" | jq -e '.totals.alive == 2' >/dev/null
}

@test "sdev ls --json includes archived tasks with date" {
  run sdev ls --json
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.archived[] | select(.task == "acme/old") | .archived == "2026-06-01"' >/dev/null
}

@test "sdev ls --json scoped to a project shows only its tasks" {
  run sdev -p acme ls --json
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.alive | length == 1' >/dev/null
  echo "$output" | jq -e '.alive[0].task == "acme/b"' >/dev/null
  echo "$output" | jq -e '.project == "acme"' >/dev/null
}

@test "sdev ls --json has array-typed orphan_volumes and a running total" {
  run sdev ls --json
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.orphan_volumes | type == "array"' >/dev/null
  echo "$output" | jq -e '.totals | has("running")' >/dev/null
}

@test "sdev ls (human) still works unchanged" {
  run sdev ls
  [ "$status" -eq 0 ]
  [[ "$output" == *"Alive tasks"* ]]
  [[ "$output" == *"acme/b"* ]]
}
