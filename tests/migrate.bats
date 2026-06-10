load helpers
setup() {
  make_fixture                      # WORKSPACE_ROOT == SDEV_HOME == fixture (legacy alias)
  OLD="$(mktemp -d)"
  mkdir -p "$OLD/core/projects.d" "$OLD/confs/acme" "$OLD/core/acme/svc" "$OLD/projects/acme/t1"
  echo 'conf_prefix: app' > "$OLD/core/projects.d/acme.yml"
  echo 'conf_prefix: app' > "$OLD/core/projects.d/example.yml"   # must NOT migrate
  echo 'defaults: { port_step: 10 }' > "$OLD/core/.task-config.yml"
  echo 'APP_ENV=local' > "$OLD/confs/acme/app.local.env"
  echo 'PORT_OFFSET=10' > "$OLD/projects/acme/t1/.env"
  SDEV_TARGET="$(mktemp -d)/home"
}
teardown() { rm -rf "$WORKSPACE_ROOT" "$OLD" "$(dirname "$SDEV_TARGET")"; }

@test "migrate copies project, confs, clones, workspaces; skips example.yml" {
  run env SDEV_HOME="$SDEV_TARGET" "$WORKSPACE_ROOT/bin/sdev" migrate --from "$OLD"
  [ "$status" -eq 0 ]
  [ -f "$SDEV_TARGET/core/projects.d/acme.yml" ]
  [ ! -f "$SDEV_TARGET/core/projects.d/example.yml" ]
  [ -f "$SDEV_TARGET/core/.task-config.yml" ]
  [ -f "$SDEV_TARGET/confs/acme/app.local.env" ]
  [ -d "$SDEV_TARGET/core/acme/svc" ]
  [ -f "$SDEV_TARGET/projects/acme/t1/.env" ]
}

@test "migrate refuses a populated home without --force" {
  mkdir -p "$SDEV_TARGET/core/projects.d"
  echo 'conf_prefix: app' > "$SDEV_TARGET/core/projects.d/existing.yml"
  run env SDEV_HOME="$SDEV_TARGET" "$WORKSPACE_ROOT/bin/sdev" migrate --from "$OLD"
  [ "$status" -ne 0 ]
  [[ "$output" == *"already has projects"* ]]
}

@test "migrate --force merges into an existing populated home without nesting" {
  mkdir -p "$SDEV_TARGET/core/projects.d"
  echo 'conf_prefix: app' > "$SDEV_TARGET/core/projects.d/existing.yml"
  run env SDEV_HOME="$SDEV_TARGET" "$WORKSPACE_ROOT/bin/sdev" migrate --force --from "$OLD"
  [ "$status" -eq 0 ]
  [ -f "$SDEV_TARGET/core/projects.d/acme.yml" ]
  [ -f "$SDEV_TARGET/core/projects.d/existing.yml" ]   # pre-existing not clobbered
  [ -d "$SDEV_TARGET/core/acme/svc" ]                  # clone dir present, not nested
  [ ! -d "$SDEV_TARGET/core/acme/acme" ]               # no double-nesting
}
