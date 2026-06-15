load helpers
setup() {
  make_fixture
  SDEV_TARGET="$(mktemp -d)/home"; export SDEV_HOME="$SDEV_TARGET"
  mkdir -p "$SDEV_TARGET/core/projects.d" "$SDEV_TARGET/confs"
  SRC="$(mktemp -d)/svc"; make_source_repo "$SRC" main
  cat > "$SDEV_TARGET/core/projects.d/acme.yml" <<YAML
conf_prefix: app
default_shell_service: api
repos:
  api:
    path: api
    default_base: main
    compose_role: api
    link_node_modules: false
YAML
  mkdir -p "$SDEV_TARGET/core/acme"
  ln -s "$SRC" "$SDEV_TARGET/core/acme/api"
  export TERM_SESSION_ID="bats-$$"; export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR"
}
teardown() { rm -rf "$WORKSPACE_ROOT" "$(dirname "$SDEV_TARGET")" "$(dirname "$SRC")"; }

edit() { env SDEV_HOME="$SDEV_TARGET" SDEV_PROJECT=acme "$WORKSPACE_ROOT/bin/edit-project" "$@"; }

@test "edit shows the project summary then quits" {
  run edit acme <<<"q"
  [ "$status" -eq 0 ]
  [[ "$output" == *"conf_prefix"* ]]
  [[ "$output" == *"api"* ]]
}

@test "edit fails for an unknown project" {
  run edit nope <<<"q"
  [ "$status" -ne 0 ]
  [[ "$output" == *"not found"* ]]
}

@test "edit add: symlinks a local repo and writes the YAML block" {
  WEB="$(mktemp -d)/web"; make_source_repo "$WEB" main
  run edit acme <<EOF
a
web
$WEB
main
ui
q
EOF
  [ "$status" -eq 0 ]
  run yq -r '.repos.web.path' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "web" ]
  run yq -r '.repos.web.compose_role' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "ui" ]
  [ -L "$SDEV_TARGET/core/acme/web" ]
  rm -rf "$(dirname "$WEB")"
}

@test "edit add: refuses a duplicate repo name" {
  run edit acme <<EOF
a
api
$SRC
main
api
q
EOF
  [ "$status" -eq 0 ]
  run yq -r '.repos | keys | length' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "1" ]
}

@test "edit remove: deletes the YAML block and unlinks a symlinked source" {
  run edit acme <<EOF
r
api
q
EOF
  [ "$status" -eq 0 ]
  run yq -r '.repos | has("api")' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "false" ]
  [ ! -e "$SDEV_TARGET/core/acme/api" ]
  [ -d "$SRC/.git" ]
}

@test "edit remove: refuses when a task worktree exists, force removes it but keeps the branch" {
  # create a real task worktree of api under projects/acme/foo
  src="$SRC"
  task="$SDEV_TARGET/projects/acme/foo"; mkdir -p "$task"
  git -C "$src" worktree add --no-track "$task/api" -b task/foo main >/dev/null 2>&1

  # refusal path: anything but 'force' aborts, repo stays
  run edit acme <<EOF
r
api
no
q
EOF
  [ "$status" -eq 0 ]
  run yq -r '.repos | has("api")' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "true" ]
  [ -e "$task/api/.git" ]

  # force path: worktree removed, branch kept, repo dropped
  run edit acme <<EOF
r
api
force
q
EOF
  [ "$status" -eq 0 ]
  run yq -r '.repos | has("api")' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "false" ]
  [ ! -e "$task/api" ]
  run git -C "$src" branch --list task/foo
  [[ "$output" == *"task/foo"* ]]
}

@test "edit remove: keeps a cloned source by default" {
  edit acme <<EOF
a
clone
file://$SRC
main
api
q
EOF
  [ -d "$SDEV_TARGET/core/acme/clone/.git" ]
  run edit acme <<EOF
r
clone
q
EOF
  [ "$status" -eq 0 ]
  run yq -r '.repos | has("clone")' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "false" ]
  [ -d "$SDEV_TARGET/core/acme/clone/.git" ]
}

@test "edit remove --delete-source deletes a clean cloned source" {
  edit acme <<EOF
a
clone
file://$SRC
main
api
q
EOF
  [ -d "$SDEV_TARGET/core/acme/clone/.git" ]
  run edit acme --delete-source <<EOF
r
clone
q
EOF
  [ "$status" -eq 0 ]
  [ ! -e "$SDEV_TARGET/core/acme/clone" ]
  run yq -r '.repos | has("clone")' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "false" ]
}

@test "edit p/s/t: edits scalar fields and stack list" {
  run edit acme <<EOF
p
acme-api
s
worker
t
nginx, api, db
q
EOF
  [ "$status" -eq 0 ]
  run yq -r '.conf_prefix' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "acme-api" ]
  run yq -r '.default_shell_service' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "worker" ]
  run yq -r '.stack_services | join(",")' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "nginx,api,db" ]
}

@test "edit t: blank input clears stack_services (inherit global)" {
  yq -i '.stack_services = ["api","db"]' "$SDEV_TARGET/core/projects.d/acme.yml"
  run edit acme <<EOF
t

q
EOF
  [ "$status" -eq 0 ]
  run yq -r 'has("stack_services")' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "false" ]
}
