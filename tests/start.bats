load helpers
setup() {
  make_fixture
  echo 'conf_prefix: p' > "$WORKSPACE_ROOT/core/projects.d/acme.yml"
  export TERM_SESSION_ID="bats-$$"
  export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR"
}
teardown() { rm -rf "$WORKSPACE_ROOT"; }

# An existing task dir with a fake `compose` that logs its args.
make_running_task() {   # $1=slug $2=port
  local d="$WORKSPACE_ROOT/projects/acme/$1"
  mkdir -p "$d"
  printf 'NGINX_HOST_PORT=%s\nAPP_ENV=local\n' "$2" > "$d/.env"
  cat > "$d/compose" <<SH
#!/usr/bin/env bash
echo "COMPOSE \$*" >> "$d/.compose.log"
SH
  chmod +x "$d/compose"
}

@test "sdev start resumes an existing task: boots it, returns url, created=false" {
  make_running_task b 8100
  run sdev -p acme start b --json --no-open
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.task == "acme/b"' >/dev/null
  echo "$output" | jq -e '.url == "http://localhost:8100/"' >/dev/null
  echo "$output" | jq -e '.created == false' >/dev/null
  # the stack was actually booted
  grep -q 'up -d' "$WORKSPACE_ROOT/projects/acme/b/.compose.log"
}

@test "sdev start --no-open does not launch a browser (prints reachable url in json)" {
  make_running_task c 8110
  run sdev -p acme start c --json --no-open
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.url == "http://localhost:8110/"' >/dev/null
}

# Fake Docker and Compose so cache provisioning and `up` need no real engine.
stub_compose() {
  mkdir -p "$WORKSPACE_ROOT/fakebin"
  cat > "$WORKSPACE_ROOT/fakebin/docker-compose" <<SH
#!/usr/bin/env bash
echo "FAKE \$*" >> "$WORKSPACE_ROOT/compose.log"
exit 0
SH
  cat > "$WORKSPACE_ROOT/fakebin/docker" <<SH
#!/usr/bin/env bash
echo "DOCKER \$*" >> "$WORKSPACE_ROOT/compose.log"
case "\$*" in
  'volume inspect sdev-pip-cache'|'volume inspect sdev-npm-cache') exit 1 ;;
  'volume create sdev-pip-cache'|'volume create sdev-npm-cache') exit 0 ;;
  *) exit 1 ;;
esac
SH
  chmod +x "$WORKSPACE_ROOT/fakebin/docker-compose" "$WORKSPACE_ROOT/fakebin/docker"
  export PATH="$WORKSPACE_ROOT/fakebin:$PATH"
}

@test "sdev start creates a missing task, boots it, returns url + created=true" {
  mkdir -p "$WORKSPACE_ROOT/core/web" "$WORKSPACE_ROOT/confs/web"
  make_source_repo "$WORKSPACE_ROOT/core/web/apisrc" main
  cat > "$WORKSPACE_ROOT/core/projects.d/web.yml" <<'YAML'
conf_prefix: web
stack_services: [nginx, api]
repos:
  api: { path: apisrc, default_base: main, compose_role: api }
YAML
  : > "$WORKSPACE_ROOT/confs/web/web.local.env"
  stub_compose
  # new-task is chatty on stderr; --json data goes to stdout, so read stdout only.
  run bash -c "'$WORKSPACE_ROOT/bin/sdev' -p web start feat --env local --no-fetch --json --no-open 2>/dev/null"
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.created == true' >/dev/null
  echo "$output" | jq -e '.task == "web/feat"' >/dev/null
  echo "$output" | jq -e '.url | startswith("http://localhost:")' >/dev/null
  [ -d "$WORKSPACE_ROOT/projects/web/feat" ]
  grep -qx 'DOCKER volume create sdev-pip-cache' "$WORKSPACE_ROOT/compose.log"
  grep -q 'up -d' "$WORKSPACE_ROOT/compose.log"
}

@test "sdev start exits 2 with a structured error when the stack fails to boot" {
  make_running_task d 8130
  printf '#!/usr/bin/env bash\nexit 1\n' > "$WORKSPACE_ROOT/projects/acme/d/compose"
  chmod +x "$WORKSPACE_ROOT/projects/acme/d/compose"
  run sdev -p acme start d --json --no-open
  [ "$status" -eq 2 ]
  echo "$output" | jq -e '.error.code == "up_failed"' >/dev/null
  [ -d "$WORKSPACE_ROOT/projects/acme/d" ]
}
