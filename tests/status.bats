load helpers
setup() {
  make_fixture
  echo 'conf_prefix: y' > "$WORKSPACE_ROOT/core/projects.d/default.yml"
  echo 'conf_prefix: p' > "$WORKSPACE_ROOT/core/projects.d/acme.yml"
  mkdir -p "$WORKSPACE_ROOT/projects/default/a" \
           "$WORKSPACE_ROOT/projects/acme/b" "$WORKSPACE_ROOT/projects/acme/c"
  printf 'PORT_OFFSET=10\nNGINX_HOST_PORT=8090\nCOMPOSE_PROJECT_NAME=default_a\n' > "$WORKSPACE_ROOT/projects/default/a/.env"
  printf 'PORT_OFFSET=20\nNGINX_HOST_PORT=8100\nCOMPOSE_PROJECT_NAME=acme_b\n' > "$WORKSPACE_ROOT/projects/acme/b/.env"
  printf 'PORT_OFFSET=30\nNGINX_HOST_PORT=8110\nCOMPOSE_PROJECT_NAME=acme_c\n' > "$WORKSPACE_ROOT/projects/acme/c/.env"
  export TERM_SESSION_ID="bats-$$"
  export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR"
}
teardown() { rm -rf "$WORKSPACE_ROOT"; }

@test "sdev status --json reports per-project counts and totals" {
  run sdev status --json
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.totals.projects == 2' >/dev/null
  echo "$output" | jq -e '.totals.tasks == 3' >/dev/null
  echo "$output" | jq -e '.projects[] | select(.name == "acme") | .tasks == 2' >/dev/null
  echo "$output" | jq -e 'has("sdev_home")' >/dev/null
}

@test "sdev status --json has a definitive running total (0 in a clean fixture)" {
  run sdev status --json
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.totals.running == 0' >/dev/null
}

@test "sdev status human output lists projects" {
  run sdev status
  [ "$status" -eq 0 ]
  [[ "$output" == *"acme"* ]]
  [[ "$output" == *"default"* ]]
}

@test "sdev status --json reports the active project" {
  run sdev -p acme status --json
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.active_project == "acme"' >/dev/null
}
