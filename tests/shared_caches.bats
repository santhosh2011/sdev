load helpers

setup() {
  make_fixture
  export CACHE_LOG="$WORKSPACE_ROOT/docker.log"
  mkdir -p "$WORKSPACE_ROOT/mock" "$WORKSPACE_ROOT/task"
  cp "$REPO_BIN/templates/compose.tmpl" "$WORKSPACE_ROOT/task/compose"
  cp "$REPO_BIN/templates/docker-compose.yml.tmpl" "$WORKSPACE_ROOT/task/docker-compose.yml"
  touch "$WORKSPACE_ROOT/task/.env"
  chmod +x "$WORKSPACE_ROOT/task/compose"
  cat > "$WORKSPACE_ROOT/mock/docker" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$CACHE_LOG"
if [[ "$*" == 'volume inspect '* ]]; then exit 1; fi
if [[ "$*" == 'volume create '* && "${FAIL_CREATE:-0}" == 1 ]]; then exit 1; fi
SH
  cat > "$WORKSPACE_ROOT/mock/docker-compose" <<'SH'
#!/usr/bin/env bash
printf 'compose %s\n' "$*" >> "$CACHE_LOG"
SH
  chmod +x "$WORKSPACE_ROOT/mock/"*
  export PATH="$WORKSPACE_ROOT/mock:$PATH"
}
teardown() { rm -rf "$WORKSPACE_ROOT"; }

@test "compose creates only declared external download caches before boot" {
  run "$WORKSPACE_ROOT/task/compose" --profile workers up -d
  [ "$status" -eq 0 ]
  grep -qx 'volume create sdev-pip-cache' "$CACHE_LOG"
  grep -qx 'volume create sdev-npm-cache' "$CACHE_LOG"
  [ "$(tail -1 "$CACHE_LOG")" = 'compose --env-file .env --profile workers up -d' ]
  [ "$(yq '.volumes."ui-node-modules".external // false' "$WORKSPACE_ROOT/task/docker-compose.yml")" = false ]
}

@test "compose down and config do not provision caches" {
  "$WORKSPACE_ROOT/task/compose" down -v
  "$WORKSPACE_ROOT/task/compose" config
  ! grep -q '^volume ' "$CACHE_LOG"
}

@test "legacy templates do not acquire shared cache volumes" {
  yq -i 'del(.volumes."npm-cache") | .volumes."pip-cache" = {}' "$WORKSPACE_ROOT/task/docker-compose.yml"
  "$WORKSPACE_ROOT/task/compose" up -d
  ! grep -q '^volume ' "$CACHE_LOG"
}

@test "cache creation failure stops boot" {
  export FAIL_CREATE=1
  run "$WORKSPACE_ROOT/task/compose" up -d
  [ "$status" -ne 0 ]
  ! grep -q '^compose ' "$CACHE_LOG"
}

@test "cache volumes are shared across two generated workspaces" {
  mkdir -p "$WORKSPACE_ROOT/core/demo"
  make_source_repo "$WORKSPACE_ROOT/core/demo/api"
  cat > "$WORKSPACE_ROOT/core/projects.d/demo.yml" <<'YAML'
repos:
  api: {path: api, default_base: main}
YAML
  sdev -p demo new first --no-fetch
  sdev -p demo new second --no-fetch
  for task in first second; do
    file="$WORKSPACE_ROOT/projects/demo/$task/docker-compose.yml"
    [ "$(yq '.volumes."pip-cache".name' "$file")" = sdev-pip-cache ]
    [ "$(yq '.volumes."pip-cache".external' "$file")" = true ]
  done
}
