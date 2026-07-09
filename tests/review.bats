load helpers
setup() {
  make_fixture
  mkdir -p "$WORKSPACE_ROOT/core/web" "$WORKSPACE_ROOT/confs/web" "$WORKSPACE_ROOT/fakebin"
  make_source_repo "$WORKSPACE_ROOT/core/web/apisrc" main
  cat > "$WORKSPACE_ROOT/core/projects.d/web.yml" <<'YAML'
conf_prefix: web
stack_services: [nginx, api]
repos:
  api: { path: apisrc, default_base: main, compose_role: api }
YAML
  : > "$WORKSPACE_ROOT/confs/web/web.local.env"
  # fake lavish-axi: log args, print a session url
  cat > "$WORKSPACE_ROOT/fakebin/lavish-axi" <<SH
#!/usr/bin/env bash
echo "LAVISH \$*" >> "$WORKSPACE_ROOT/lavish.log"
echo "url: http://127.0.0.1:4387/session/abc123"
SH
  chmod +x "$WORKSPACE_ROOT/fakebin/lavish-axi"
  export PATH="$WORKSPACE_ROOT/fakebin:$PATH"
  export TERM_SESSION_ID="bats-$$"; export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR"
}
teardown() { rm -rf "$WORKSPACE_ROOT"; }

new_task_with_commit() {
  sdev -p web new "$1" --env local --no-fetch >/dev/null 2>&1
  local wt="$WORKSPACE_ROOT/projects/web/$1/apisrc"
  printf 'alpha\nbeta\n' > "$wt/feature.txt"
  git -C "$wt" add -A && git -C "$wt" -c user.email=t@t -c user.name=t commit -qm work
}

@test "sdev review builds a diff artifact + JSON summary and opens lavish" {
  new_task_with_commit feat
  run bash -c "'$WORKSPACE_ROOT/bin/sdev' -p web review feat --json 2>/dev/null"
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.task == "web/feat"' >/dev/null
  echo "$output" | jq -e '.repos[0].repo == "api"' >/dev/null
  echo "$output" | jq -e '.repos[0].files >= 1' >/dev/null
  echo "$output" | jq -e '.lavish_url | startswith("http")' >/dev/null
  art="$(echo "$output" | jq -r '.artifact')"
  [ -f "$art" ]
  grep -q "feature.txt" "$art"
  grep -q -- "$art" "$WORKSPACE_ROOT/lavish.log"
}

@test "sdev review --no-open builds the artifact without invoking lavish" {
  new_task_with_commit quiet
  run bash -c "'$WORKSPACE_ROOT/bin/sdev' -p web review quiet --no-open --json 2>/dev/null"
  [ "$status" -eq 0 ]
  art="$(echo "$output" | jq -r '.artifact')"
  [ -f "$art" ]
  [ ! -f "$WORKSPACE_ROOT/lavish.log" ]
}
