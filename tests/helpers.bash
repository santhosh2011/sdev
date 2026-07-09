# Shared bats helpers: build an isolated WORKSPACE_ROOT fixture.
# Usage in a .bats file:
#   load helpers
#   setup() { make_fixture; }
#   teardown() { rm -rf "$WORKSPACE_ROOT"; }

# Repo bin dir. BATS_TEST_DIRNAME is the directory of the running .bats file
# (i.e. tests/), so bin/ is one level up. Falls back to BASH_SOURCE when sourced
# outside bats.
REPO_BIN="$(cd "${BATS_TEST_DIRNAME:-$(dirname "${BASH_SOURCE[0]}")}/../bin" && pwd)"

make_fixture() {
    # SDEV_HOME outranks WORKSPACE_ROOT in _lib.sh; unset it so the fixture
    # is authoritative even when the developer has SDEV_HOME exported.
    unset SDEV_HOME
    WORKSPACE_ROOT="$(mktemp -d)"
    export WORKSPACE_ROOT
    mkdir -p "$WORKSPACE_ROOT/core/projects.d" "$WORKSPACE_ROOT/confs" \
             "$WORKSPACE_ROOT/projects/_archive" "$WORKSPACE_ROOT/bin"

    # Copy the real bin scripts under test into the fixture so relative
    # SCRIPT_DIR resolution and `source _lib.sh` work against fixture paths.
    cp "$REPO_BIN"/_lib.sh "$REPO_BIN"/sdev "$REPO_BIN"/new-task \
       "$REPO_BIN"/list-tasks "$REPO_BIN"/end-task "$REPO_BIN"/migrate \
       "$REPO_BIN"/init "$REPO_BIN"/edit-project "$REPO_BIN"/doctor \
       "$REPO_BIN"/prune "$REPO_BIN"/status "$REPO_BIN"/start "$REPO_BIN"/review "$WORKSPACE_ROOT/bin/"
    cp -R "$REPO_BIN/templates" "$WORKSPACE_ROOT/bin/templates"
    # Ship the Claude hook scripts into the fixture so $SDEV_INSTALL/claude/hooks
    # resolves (SDEV_INSTALL = parent of bin = $WORKSPACE_ROOT).
    cp -R "$REPO_BIN/../claude" "$WORKSPACE_ROOT/claude"
    chmod +x "$WORKSPACE_ROOT/claude/hooks/"* 2>/dev/null || true
    chmod +x "$WORKSPACE_ROOT/bin/"*

    cat > "$WORKSPACE_ROOT/core/.task-config.yml" <<'YAML'
repos:
  api: { path: legacy_api, default_base: main, compose_role: api }
stack_services: [nginx, api, ui, db, redis]
defaults:
  default_env: local
  default_project: default
  port_step: 10
  base_ports: { nginx: 8080, api: 8291, ui: 5173, db: 3306, redis: 6379 }
YAML
}

# Create a bare-ish source git repo with one commit on a branch.
make_source_repo() {
    # $1 = absolute path, $2 = branch name (default main)
    local path="$1" branch="${2:-main}"
    mkdir -p "$path"
    git -C "$path" init -q -b "$branch"
    git -C "$path" config user.email t@t.dev
    git -C "$path" config user.name test
    echo seed > "$path/README"
    git -C "$path" add -A
    git -C "$path" commit -qm seed
}

# Run a fixture bin script with WORKSPACE_ROOT honored.
sdev() { "$WORKSPACE_ROOT/bin/sdev" "$@"; }
