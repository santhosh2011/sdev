load helpers
setup() {
  make_fixture
  echo 'conf_prefix: p' > "$WORKSPACE_ROOT/core/projects.d/acme.yml"
  export TERM_SESSION_ID="bats-$$"
  export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR"
}
teardown() { rm -rf "$WORKSPACE_ROOT"; }

# Build a task dir with a .env and a fake `compose` wrapper emitting canned ps output.
make_task() {   # $1=slug  $2=nginx_port  $3=compose-body
  local d="$WORKSPACE_ROOT/projects/acme/$1"
  mkdir -p "$d"
  printf 'NGINX_HOST_PORT=%s\n' "$2" > "$d/.env"
  printf '#!/usr/bin/env bash\n%s\n' "$3" > "$d/compose"
  chmod +x "$d/compose"
}

@test "sdev ps --json normalizes compose ps and adds the url" {
  make_task b 8100 'echo '"'"'{"Service":"nginx","State":"running","Publishers":[{"PublishedPort":8100,"TargetPort":80}]}'"'"'
echo '"'"'{"Service":"api","State":"running","Publishers":[]}'"'"''
  run sdev -p acme ps b --json
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.task == "acme/b"' >/dev/null
  echo "$output" | jq -e '.url == "http://localhost:8100/"' >/dev/null
  echo "$output" | jq -e '.services | length == 2' >/dev/null
  echo "$output" | jq -e '.services[] | select(.name == "nginx") | .ports[0] == "8100->80"' >/dev/null
}

@test "sdev ps --json yields definitive empty services when compose returns nothing" {
  make_task c 8110 'exit 0'
  run sdev -p acme ps c --json
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.services | type == "array" and length == 0' >/dev/null
  echo "$output" | jq -e '.url == "http://localhost:8110/"' >/dev/null
}

@test "sdev ps (human) still delegates to compose ps" {
  make_task d 8120 'echo "COMPOSE_PS_HUMAN $*"'
  run sdev -p acme ps d
  [ "$status" -eq 0 ]
  [[ "$output" == *"COMPOSE_PS_HUMAN"* ]]
  [[ "$output" == *"ps"* ]]
}
