# sdev Zippable Installer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn sdev into a zip-distributable, single-user tool: clean tool/data separation, a cross-platform installer, and a `sdev init` first-run wizard.

**Architecture:** Split the conflated `WORKSPACE_ROOT` into `SDEV_INSTALL` (tool code) and `SDEV_HOME` (user data, default `~/.sdev`), resolved authoritatively in `bin/_lib.sh`. `WORKSPACE_ROOT` becomes a legacy alias so the existing test suite and pre-migration clones keep working. Add `dist` (build zip), `install` (place tool + seed `~/.sdev` + wire PATH), `sdev init` (project wizard), and `sdev migrate` (move an old in-repo layout into `~/.sdev`).

**Tech Stack:** bash ≥ 4, yq v4, docker compose, bats-core (tests), zip/unzip (packaging).

**Spec:** `docs/superpowers/specs/2026-06-11-sdev-installer-design.md`

**Conventions for every task:** run the full suite with `bats tests/` from the repo root; a task is done only when it is green. Commit messages end with the trailer:
```
Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
```

---

### Task 1: Root resolution + path accessors in `bin/_lib.sh`

Introduce `SDEV_INSTALL` / `SDEV_HOME`, repoint every data path to `SDEV_HOME` and every tool-asset path to `SDEV_INSTALL`, and add `ensure_home`. `WORKSPACE_ROOT` survives only as a legacy alias.

**Files:**
- Test: `tests/lib_roots.bats` (create)
- Modify: `bin/_lib.sh`

- [ ] **Step 1: Write the failing test**

Create `tests/lib_roots.bats`:

```bash
load helpers
setup() { make_fixture; }
teardown() { rm -rf "$WORKSPACE_ROOT"; unset SDEV_HOME SDEV_INSTALL; }

@test "WORKSPACE_ROOT acts as legacy alias for SDEV_HOME and SDEV_INSTALL" {
  run env -u SDEV_HOME bash -c \
    'source "'"$WORKSPACE_ROOT"'/bin/_lib.sh"; echo "$SDEV_HOME|$SDEV_INSTALL"'
  [ "$status" -eq 0 ]
  [ "$output" = "$WORKSPACE_ROOT|$WORKSPACE_ROOT" ]
}

@test "explicit SDEV_HOME wins over WORKSPACE_ROOT" {
  run env SDEV_HOME=/tmp/sdev-custom-home bash -c \
    'source "'"$WORKSPACE_ROOT"'/bin/_lib.sh"; echo "$SDEV_HOME"'
  [ "$output" = "/tmp/sdev-custom-home" ]
}

@test "SDEV_HOME falls back to ~/.sdev when nothing is set" {
  run env -u WORKSPACE_ROOT -u SDEV_HOME HOME=/tmp/sdev-fakehome bash -c \
    'source "'"$WORKSPACE_ROOT"'/bin/_lib.sh"; echo "$SDEV_HOME"'
  [ "$output" = "/tmp/sdev-fakehome/.sdev" ]
}

@test "config_template default resolves under SDEV_INSTALL" {
  source "$WORKSPACE_ROOT/bin/_lib.sh"
  run config_template default
  [ "$output" = "$SDEV_INSTALL/bin/templates/docker-compose.yml.tmpl" ]
}

@test "ensure_home creates skeleton and seeds default config" {
  run env SDEV_HOME="$WORKSPACE_ROOT/seedtest" bash -c \
    'source "'"$WORKSPACE_ROOT"'/bin/_lib.sh"; ensure_home; \
     test -d "$SDEV_HOME/core/projects.d" && \
     test -d "$SDEV_HOME/projects/_archive" && \
     test -f "$SDEV_HOME/core/.task-config.yml"'
  [ "$status" -eq 0 ]
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `bats tests/lib_roots.bats`
Expected: FAIL — `$SDEV_HOME`/`$SDEV_INSTALL` are unset (empty output) and `ensure_home` is an unknown command.

- [ ] **Step 3: Replace the header block in `bin/_lib.sh`**

Replace lines 1–9 (from the `# shellcheck shell=bash` header through the `PROJECTS_DIR=` line):

```bash
# shellcheck shell=bash
# Shared helpers for new-task / end-task / list-tasks.
# Source this, don't execute.

# --- root resolution ----------------------------------------------------------
# SDEV_INSTALL: where the tool code lives (this lib's parent dir).
SDEV_INSTALL="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# SDEV_HOME: user-data root. Precedence:
#   1. explicit $SDEV_HOME
#   2. $WORKSPACE_ROOT  (legacy alias: combined tool+data root; tests & pre-migration clones)
#   3. ~/.sdev
if [[ -n "${SDEV_HOME:-}" ]]; then
    :
elif [[ -n "${WORKSPACE_ROOT:-}" ]]; then
    SDEV_HOME="$WORKSPACE_ROOT"
else
    SDEV_HOME="$HOME/.sdev"
fi
export SDEV_INSTALL SDEV_HOME

GLOBAL_CONFIG="$SDEV_HOME/core/.task-config.yml"
# shellcheck disable=SC2034  # used by sourcing scripts and future tasks
LOCAL_CONFIG="$SDEV_HOME/core/.task-config.local.yml"
PROJECTS_DIR="$SDEV_HOME/core/projects.d"

# Create the SDEV_HOME skeleton and seed the default config if absent. Idempotent.
ensure_home() {
    mkdir -p "$SDEV_HOME/core/projects.d" "$SDEV_HOME/confs" "$SDEV_HOME/projects/_archive"
    if [[ ! -f "$GLOBAL_CONFIG" && -f "$SDEV_INSTALL/core/.task-config.yml" ]]; then
        cp "$SDEV_INSTALL/core/.task-config.yml" "$GLOBAL_CONFIG"
    fi
}
```

- [ ] **Step 4: Repoint `config_template` to the split roots**

In `config_template`, change the body so the project override resolves under `SDEV_HOME` and the default under `SDEV_INSTALL`:

```bash
config_template() {   # $1=project
    local v; v="$(yq -r '.template // ""' "$(effective_project_file "$1")")"
    if [[ -n "$v" && "$v" != "null" ]]; then
        echo "$SDEV_HOME/$v"
    else
        echo "$SDEV_INSTALL/bin/templates/docker-compose.yml.tmpl"
    fi
}
```

- [ ] **Step 5: Repoint the remaining data accessors from `WORKSPACE_ROOT` to `SDEV_HOME`**

In `profile_conf_file`:
```bash
    pdir="$SDEV_HOME/confs/$2"
    if [[ -d "$pdir" ]]; then
        echo "$pdir/$prefix.$1.env"
    else
        echo "$SDEV_HOME/confs/$prefix.$1.env"   # legacy flat confs/
    fi
```

In `repo_source_dir`:
```bash
repo_source_dir() {   # $1=project $2=repo_path
    local ns="$SDEV_HOME/core/$1/$2"
    [[ -d "$ns" ]] && { echo "$ns"; return; }
    echo "$SDEV_HOME/core/$2"
}
```

In `config_default_env`, change the first line of the body:
```bash
    local local_cfg="$SDEV_HOME/core/.task-config.local.yml"
```

In `compute_next_offset`, change the `find` root:
```bash
    done < <(find "$SDEV_HOME/projects" -type f -name .env 2>/dev/null)
```

- [ ] **Step 6: Run the new test to verify it passes**

Run: `bats tests/lib_roots.bats`
Expected: PASS (5 tests).

- [ ] **Step 7: Run the full suite to confirm no regressions**

Run: `bats tests/`
Expected: PASS — all existing files plus `lib_roots.bats`. (Existing tests export `WORKSPACE_ROOT`, so `SDEV_HOME == SDEV_INSTALL == WORKSPACE_ROOT` and behavior is unchanged.)

- [ ] **Step 8: Commit**

```bash
git add bin/_lib.sh tests/lib_roots.bats
git commit -m "refactor(lib): split WORKSPACE_ROOT into SDEV_INSTALL + SDEV_HOME"
```

---

### Task 2: Drop `WORKSPACE_ROOT` self-assignment from entry scripts

The four `bin/*` entry scripts each force `WORKSPACE_ROOT` to their own parent dir before sourcing `_lib.sh`, which would defeat the `~/.sdev` default. Remove those assignments and switch their direct path references to `SDEV_HOME`. They keep `SCRIPT_DIR` (used to source `_lib.sh` and copy templates).

**Files:**
- Modify: `bin/sdev`, `bin/new-task`, `bin/end-task`, `bin/list-tasks`

- [ ] **Step 1: `bin/sdev` — remove the WORKSPACE_ROOT block**

Delete these two lines (currently 13–14, just below the `SCRIPT_DIR=` resolution loop):
```bash
: "${WORKSPACE_ROOT:=$(cd "$SCRIPT_DIR/.." && pwd)}"
export WORKSPACE_ROOT
```

- [ ] **Step 2: `bin/sdev` — switch task-dir references to `SDEV_HOME`**

In `require_task_dir`:
```bash
require_task_dir() {
    local slug="$1"
    [[ -n "$slug" ]] || die "slug required"
    local ns="$SDEV_HOME/projects/$PROJECT/$slug"
    [[ -d "$ns" ]] && { echo "$ns"; return; }
    local flat="$SDEV_HOME/projects/$slug"
    [[ -d "$flat" ]] && { echo "$flat"; return; }
    die "task '$slug' not found in project '$PROJECT' (or legacy projects/$slug)"
}
```

In the `projects)` case, change the glob:
```bash
            for d in "$SDEV_HOME"/projects/"$p"/*/; do
```

- [ ] **Step 3: `bin/new-task` — remove the WORKSPACE_ROOT block and repoint task dirs**

Delete (currently lines 5–6):
```bash
: "${WORKSPACE_ROOT:=$(cd "$SCRIPT_DIR/.." && pwd)}"
export WORKSPACE_ROOT
```

Change the task-dir definition and archived-task guard:
```bash
TASK_DIR="$SDEV_HOME/projects/$PROJECT/$SLUG"
[[ -e "$TASK_DIR" ]] && die "projects/$PROJECT/$SLUG already exists"
[[ -e "$SDEV_HOME/projects/_archive/$PROJECT/$SLUG" ]] && die "archived task projects/_archive/$PROJECT/$SLUG exists — restore or pick another slug"
```

- [ ] **Step 4: `bin/end-task` — remove the WORKSPACE_ROOT block and repoint task dirs**

Delete (currently lines 5–6):
```bash
: "${WORKSPACE_ROOT:=$(cd "$SCRIPT_DIR/.." && pwd)}"
export WORKSPACE_ROOT
```

Change the task-dir resolution and the archive path:
```bash
TASK_DIR="$SDEV_HOME/projects/$PROJECT/$SLUG"
if [[ ! -d "$TASK_DIR" ]]; then
    if [[ -d "$SDEV_HOME/projects/$SLUG" ]]; then
        TASK_DIR="$SDEV_HOME/projects/$SLUG"
    else
        die "task '$SLUG' not found in project '$PROJECT'"
    fi
fi
```
and:
```bash
ARCHIVE="$SDEV_HOME/projects/_archive/$PROJECT/$SLUG"
```

- [ ] **Step 5: `bin/list-tasks` — remove the WORKSPACE_ROOT block and repoint scans**

Delete (currently lines 5–6):
```bash
: "${WORKSPACE_ROOT:=$(cd "$SCRIPT_DIR/.." && pwd)}"
export WORKSPACE_ROOT
```

Change the three scan roots:
```bash
for pdir in "$SDEV_HOME"/projects/*/; do
```
```bash
for adir in "$SDEV_HOME"/projects/_archive/*/; do
```
```bash
known=$(ls -1 "$SDEV_HOME/projects/" "$SDEV_HOME/projects/_archive/" 2>/dev/null | sort -u | grep -v '^_archive$' || true)
```

- [ ] **Step 6: Verify no stray data-path `WORKSPACE_ROOT` references remain**

Run: `grep -rn 'WORKSPACE_ROOT' bin/`
Expected: the ONLY match is the legacy-alias read in `bin/_lib.sh` (`elif [[ -n "${WORKSPACE_ROOT:-}" ]]`). No `$WORKSPACE_ROOT/...` path uses anywhere.

- [ ] **Step 7: Run the full suite**

Run: `bats tests/`
Expected: PASS — all tests green. (`tests/newtask.bats`, `tests/sdev_cli.bats`, etc. export `WORKSPACE_ROOT`; the alias keeps `$SDEV_HOME` equal to it.)

- [ ] **Step 8: Commit**

```bash
git add bin/sdev bin/new-task bin/end-task bin/list-tasks
git commit -m "refactor(bin): resolve task/data paths from SDEV_HOME, drop WORKSPACE_ROOT self-set"
```

---

### Task 3: `sdev migrate` — move an in-repo layout into `~/.sdev`

A structure-preserving copy from an old combined-root clone into `SDEV_HOME`.

**Files:**
- Create: `bin/migrate`
- Modify: `bin/sdev` (dispatch + usage + reserved keywords)
- Test: `tests/migrate.bats` (create)

- [ ] **Step 1: Write the failing test**

Create `tests/migrate.bats`:

```bash
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `bats tests/migrate.bats`
Expected: FAIL — `unknown command: migrate`.

- [ ] **Step 3: Create `bin/migrate`**

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=./_lib.sh
source "$SCRIPT_DIR/_lib.sh"

FROM=""; FORCE=0
while [[ $# -gt 0 ]]; do
    case "$1" in
        --from) FROM="${2:-}"; shift 2 ;;
        --force) FORCE=1; shift ;;
        -h|--help) echo "Usage: sdev migrate --from <old-sdev-dir> [--force]"; exit 0 ;;
        *) die "unexpected arg: $1" ;;
    esac
done

[[ -n "$FROM" ]] || die "--from <old-sdev-dir> required"
[[ -d "$FROM" ]] || die "--from dir not found: $FROM"
FROM="$(cd "$FROM" && pwd)"

ensure_home
[[ "$FROM" != "$SDEV_HOME" ]] || die "--from is the same as SDEV_HOME ($SDEV_HOME)"

# Refuse a populated home (any project file other than example.yml) unless --force.
if [[ $FORCE -eq 0 ]]; then
    shopt -s nullglob
    for f in "$PROJECTS_DIR"/*.yml; do
        [[ "$(basename "$f")" == example.yml ]] && continue
        die "$SDEV_HOME already has projects — pass --force to merge"
    done
fi

moved=0
copy_in() {   # $1 = path relative to FROM
    local rel="$1" src="$FROM/$1" dst="$SDEV_HOME/$1"
    [[ -e "$src" ]] || return 0
    mkdir -p "$(dirname "$dst")"
    cp -R "$src" "$dst"
    log "migrated $rel"
    moved=$((moved + 1))
}

shopt -s nullglob
mkdir -p "$PROJECTS_DIR"
for f in "$FROM"/core/projects.d/*.yml; do
    [[ "$(basename "$f")" == example.yml ]] && continue
    cp "$f" "$PROJECTS_DIR/"
    log "migrated core/projects.d/$(basename "$f")"
    moved=$((moved + 1))
done

copy_in core/.task-config.yml
copy_in core/.task-config.local.yml

for d in "$FROM"/core/*/; do
    name="$(basename "$d")"
    [[ "$name" == projects.d ]] && continue
    copy_in "core/$name"
done

for d in "$FROM"/confs/*/; do
    copy_in "confs/$(basename "$d")"
done

copy_in projects

log "migration complete — $moved item(s) into $SDEV_HOME"
```

- [ ] **Step 4: Make it executable and wire dispatch in `bin/sdev`**

Run: `chmod +x bin/migrate`

In `bin/sdev`, add `init|migrate` to `is_reserved`:
```bash
        new|up|down|nuke|end|ls|list|ps|logs|shell|open|code|cd|use|projects|init|migrate|help|-h|--help) return 0 ;;
```

Add to the `usage()` heredoc, under `Commands:` (after the `end` line):
```
  init                            interactive wizard to configure your first project
  migrate --from <dir>            move an old in-repo sdev layout into \$SDEV_HOME
```

Add cases to the dispatch `case "$CMD" in` block (after the `end)` case):
```bash
    init)
        exec "$SCRIPT_DIR/init"
        ;;
    migrate)
        exec "$SCRIPT_DIR/migrate" "$@"
        ;;
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `bats tests/migrate.bats`
Expected: PASS (2 tests).

- [ ] **Step 6: Run the full suite**

Run: `bats tests/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add bin/migrate bin/sdev tests/migrate.bats
git commit -m "feat(sdev): add 'migrate' to move in-repo config into SDEV_HOME"
```

---

### Task 4: `sdev init` — first-run project wizard

Interactive wizard that writes the user's first project registry, clones/links its repos, seeds a local conf, and prints next steps. Reads prompts via `read` so it is testable from a here-doc.

**Files:**
- Create: `bin/init`
- Test: `tests/init.bats` (create)
- (Dispatch already added in Task 3.)

- [ ] **Step 1: Write the failing test**

Create `tests/init.bats`:

```bash
load helpers
setup() {
  make_fixture
  SDEV_TARGET="$(mktemp -d)/home"
  SRC="$(mktemp -d)/svc"
  make_source_repo "$SRC" main
  export TERM_SESSION_ID="bats-$$"; export TMPDIR="$WORKSPACE_ROOT/tmp"; mkdir -p "$TMPDIR"
}
teardown() { rm -rf "$WORKSPACE_ROOT" "$(dirname "$SDEV_TARGET")" "$(dirname "$SRC")"; }

@test "init writes a project registry, links a local repo, and seeds a conf" {
  run env SDEV_HOME="$SDEV_TARGET" "$WORKSPACE_ROOT/bin/init" <<EOF
acme
app
api
svc
$SRC
main
api

EOF
  [ "$status" -eq 0 ]
  [ -f "$SDEV_TARGET/core/projects.d/acme.yml" ]
  run yq -r '.repos.svc.path' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "svc" ]
  run yq -r '.conf_prefix' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "app" ]
  [ -e "$SDEV_TARGET/core/acme/svc" ]
  [ -f "$SDEV_TARGET/confs/acme/app.local.env" ]
}

@test "init aborts when no repos are added" {
  run env SDEV_HOME="$SDEV_TARGET" "$WORKSPACE_ROOT/bin/init" <<EOF
acme
app
api

EOF
  [ "$status" -ne 0 ]
}
```

Note: the first test's here-doc lines are, in order — project name, conf prefix, shell service, repo name, repo source, base branch, compose role, then a blank line to finish the repo loop.

- [ ] **Step 2: Run the test to verify it fails**

Run: `bats tests/init.bats`
Expected: FAIL — `bin/init` does not exist.

- [ ] **Step 3: Create `bin/init`**

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=./_lib.sh
source "$SCRIPT_DIR/_lib.sh"

ensure_home

# Prompt to stderr, echo the answer (or default) to stdout.
ask() {   # $1=label $2=default(optional)
    local label="$1" def="${2:-}" ans
    if [[ -n "$def" ]]; then printf '%s [%s]: ' "$label" "$def" >&2
    else printf '%s: ' "$label" >&2; fi
    read -r ans || ans=""
    echo "${ans:-$def}"
}

project="$(ask 'Project name (kebab-case)')"
validate_slug "$project" || die "invalid project name '$project' — kebab-case, 1-50 chars"
proj_file="$PROJECTS_DIR/$project.yml"
[[ -e "$proj_file" ]] && die "project '$project' already exists at $proj_file"

conf_prefix="$(ask 'Conf prefix' app)"
shell_service="$(ask 'Shell service (compose service for `sdev shell`)' api)"

declare -a R_NAME R_PATH R_BASE R_ROLE
while :; do
    name="$(ask 'Repo name (blank to finish)')"
    [[ -z "$name" ]] && break
    if ! validate_slug "$name"; then echo "  invalid repo name; skipping" >&2; continue; fi
    source_spec="$(ask "  git URL or local path for '$name'")"
    base="$(ask '  default base branch' main)"
    role="$(ask '  compose role' "$name")"
    dest="$SDEV_HOME/core/$project/$name"
    mkdir -p "$SDEV_HOME/core/$project"
    if [[ "$source_spec" == *://* || "$source_spec" == git@* ]]; then
        log "cloning $source_spec -> $dest"
        if ! git clone "$source_spec" "$dest"; then
            echo "  clone failed; skipping repo '$name'" >&2; continue
        fi
    elif [[ -d "$source_spec/.git" || -f "$source_spec/.git" ]]; then
        ln -s "$(cd "$source_spec" && pwd)" "$dest"
    else
        echo "  '$source_spec' is not a git URL or a git checkout; skipping '$name'" >&2; continue
    fi
    R_NAME+=("$name"); R_PATH+=("$name"); R_BASE+=("$base"); R_ROLE+=("$role")
done

[[ ${#R_NAME[@]} -gt 0 ]] || die "no repos added — aborting"

{
    echo "conf_prefix: $conf_prefix"
    echo "default_shell_service: $shell_service"
    echo "repos:"
    for i in "${!R_NAME[@]}"; do
        echo "  ${R_NAME[$i]}:"
        echo "    path: ${R_PATH[$i]}"
        echo "    default_base: ${R_BASE[$i]}"
        echo "    compose_role: ${R_ROLE[$i]}"
        echo "    link_node_modules: false"
    done
} > "$proj_file"
log "wrote $proj_file"

mkdir -p "$SDEV_HOME/confs/$project"
conf_dest="$SDEV_HOME/confs/$project/$conf_prefix.local.env"
if [[ ! -f "$conf_dest" ]]; then
    if [[ -f "$SDEV_INSTALL/confs/example/app.local.env.example" ]]; then
        cp "$SDEV_INSTALL/confs/example/app.local.env.example" "$conf_dest"
    else
        echo "APP_ENV=local" > "$conf_dest"
    fi
fi
log "wrote $conf_dest"

# Pin the project for this terminal (best-effort; ignore session quirks).
"$SCRIPT_DIR/sdev" use "$project" >/dev/null 2>&1 || true

cat <<EOF

✓ project '$project' is ready.

next steps:
  sdev use $project
  sdev new my-task
  sdev up my-task
  sdev open my-task
EOF
```

- [ ] **Step 4: Make it executable**

Run: `chmod +x bin/init`

- [ ] **Step 5: Run the test to verify it passes**

Run: `bats tests/init.bats`
Expected: PASS (2 tests).

- [ ] **Step 6: Run the full suite**

Run: `bats tests/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add bin/init tests/init.bats
git commit -m "feat(sdev): add 'init' first-run project wizard"
```

---

### Task 5: `install` script (macOS + Linux)

Ships at the root of the zip. Checks deps, places the tool in `SDEV_INSTALL`, seeds `SDEV_HOME`, and wires `PATH` — idempotent and never clobbering user data.

**Files:**
- Create: `install` (repo root)
- Create: `VERSION` (repo root)
- Test: `tests/install.bats` (create)

- [ ] **Step 1: Create the `VERSION` file**

Create `VERSION` with a single line:
```
1.0.0
```

- [ ] **Step 2: Write the failing test**

Create `tests/install.bats`:

```bash
setup() {
  REPO="$(cd "$BATS_TEST_DIRNAME/.." && pwd)"
  INST="$(mktemp -d)/install"
  HOMEDIR="$(mktemp -d)/home"
  BINDIR="$(mktemp -d)/bin"
}
teardown() { rm -rf "$(dirname "$INST")" "$(dirname "$HOMEDIR")" "$(dirname "$BINDIR")"; }

run_install() {
  env SDEV_INSTALL="$INST" SDEV_HOME="$HOMEDIR" SDEV_BIN_DIR="$BINDIR" \
      bash "$REPO/install"
}

@test "install places tool, seeds home, links sdev" {
  run run_install
  [ "$status" -eq 0 ]
  [ -x "$INST/bin/sdev" ]
  [ -f "$INST/core/.task-config.yml" ]
  [ -f "$HOMEDIR/core/.task-config.yml" ]
  [ -d "$HOMEDIR/core/projects.d" ]
  [ -L "$BINDIR/sdev" ]
  [ "$(readlink "$BINDIR/sdev")" = "$INST/bin/sdev" ]
}

@test "install is idempotent and preserves user data" {
  run_install
  echo 'conf_prefix: keep' > "$HOMEDIR/core/projects.d/keep.yml"
  run run_install
  [ "$status" -eq 0 ]
  [ -f "$HOMEDIR/core/projects.d/keep.yml" ]
  run cat "$HOMEDIR/core/projects.d/keep.yml"
  [[ "$output" == *"keep"* ]]
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `bats tests/install.bats`
Expected: FAIL — `install` does not exist.

- [ ] **Step 4: Create `install`**

```bash
#!/usr/bin/env bash
set -euo pipefail

# The unpacked tool dir (this script ships at the zip root).
SRC_DIR="$(cd "$(dirname "$0")" && pwd)"

INSTALL_DIR="${SDEV_INSTALL:-$HOME/.local/share/sdev}"
HOME_DIR="${SDEV_HOME:-$HOME/.sdev}"
BIN_DIR="${SDEV_BIN_DIR:-$HOME/.local/bin}"

say()  { printf '%s\n' "$*"; }
warn() { printf '%s\n' "$*" >&2; }

# --- 1. dependency check ----------------------------------------------------
os="$(uname -s)"
ok=1
if [[ "${BASH_VERSINFO[0]:-0}" -lt 4 ]]; then
    warn "✗ bash >= 4 required (found ${BASH_VERSION:-?})."
    if [[ "$os" == Darwin ]]; then warn "   macOS: brew install bash"
    else warn "   Linux: install a newer bash via your package manager"; fi
    ok=0
else say "✓ bash ${BASH_VERSINFO[0]}"; fi
if command -v yq >/dev/null 2>&1; then say "✓ yq ($(yq --version 2>/dev/null))"; else
    warn "✗ yq v4 not found."
    if [[ "$os" == Darwin ]]; then warn "   macOS: brew install yq"
    else warn "   Linux: see https://github.com/mikefarah/yq#install"; fi
    ok=0; fi
if command -v docker >/dev/null 2>&1; then say "✓ docker"; else
    warn "✗ docker not found (needed for 'sdev up'). Install Docker Desktop or OrbStack."
    ok=0; fi
if [[ $ok -ne 1 ]]; then
    warn ""; warn "Install the missing dependencies above, then re-run ./install."
    exit 1
fi

# --- 2. place the tool ------------------------------------------------------
mkdir -p "$INSTALL_DIR"
for item in bin core README.md LICENSE VERSION confs install; do
    [[ -e "$SRC_DIR/$item" ]] || continue
    rm -rf "${INSTALL_DIR:?}/$item"
    cp -R "$SRC_DIR/$item" "$INSTALL_DIR/$item"
done
chmod +x "$INSTALL_DIR/bin/"* 2>/dev/null || true
chmod +x "$INSTALL_DIR/install" 2>/dev/null || true
say "✓ tool installed to $INSTALL_DIR"

# --- 3. create user-data home (idempotent; never clobbers) ------------------
mkdir -p "$HOME_DIR/core/projects.d" "$HOME_DIR/confs" "$HOME_DIR/projects/_archive"
if [[ ! -f "$HOME_DIR/core/.task-config.yml" && -f "$INSTALL_DIR/core/.task-config.yml" ]]; then
    cp "$INSTALL_DIR/core/.task-config.yml" "$HOME_DIR/core/.task-config.yml"
    say "✓ seeded $HOME_DIR/core/.task-config.yml"
fi
say "✓ user-data home ready at $HOME_DIR"

# --- 4. wire PATH -----------------------------------------------------------
mkdir -p "$BIN_DIR"
ln -sf "$INSTALL_DIR/bin/sdev" "$BIN_DIR/sdev"
say "✓ linked $BIN_DIR/sdev -> $INSTALL_DIR/bin/sdev"
case ":$PATH:" in
    *":$BIN_DIR:"*) : ;;
    *)
        say ""
        say "Add $BIN_DIR to your PATH (e.g. in ~/.zshrc or ~/.bashrc):"
        say "    export PATH=\"$BIN_DIR:\$PATH\""
        ;;
esac

cat <<EOF

Done. Next:
  sdev init      # configure your first project
EOF
```

- [ ] **Step 5: Make it executable**

Run: `chmod +x install`

- [ ] **Step 6: Run the test to verify it passes**

Run: `bats tests/install.bats`
Expected: PASS (2 tests). (Requires bash≥4, yq, docker present — the same assumption as `tests/bootstrap.bats`.)

- [ ] **Step 7: Run the full suite**

Run: `bats tests/`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add install VERSION tests/install.bats
git commit -m "feat: add cross-platform install script + VERSION"
```

---

### Task 6: `dist` script — build the distributable zip

Stages tool code + reference assets only (no personal projects, clones, or secrets) and zips it.

**Files:**
- Create: `dist` (repo root)
- Test: `tests/dist.bats` (create)

- [ ] **Step 1: Write the failing test**

Create `tests/dist.bats`:

```bash
setup() {
  REPO="$(cd "$BATS_TEST_DIRNAME/.." && pwd)"
  OUT="$(mktemp -d)"
}
teardown() { rm -rf "$OUT"; }

@test "dist produces a zip with tool code and reference assets only" {
  run bash "$REPO/dist" "$OUT"
  [ "$status" -eq 0 ]
  zipfile="$output"
  [ -f "$zipfile" ]
  listing="$(unzip -Z1 "$zipfile")"
  echo "$listing" | grep -qx 'sdev/bin/sdev'
  echo "$listing" | grep -qx 'sdev/install'
  echo "$listing" | grep -qx 'sdev/core/.task-config.yml'
  echo "$listing" | grep -qx 'sdev/core/projects.d/example.yml'
  # exclusions: no live workspaces, no real env files
  ! echo "$listing" | grep -q '/projects/'
  ! echo "$listing" | grep -E '\.env$' | grep -qv '\.env\.example$'
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `bats tests/dist.bats`
Expected: FAIL — `dist` does not exist.

- [ ] **Step 3: Create `dist`**

```bash
#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
VERSION="$(cat "$REPO_DIR/VERSION" 2>/dev/null || echo 0.0.0)"
OUT_DIR="${1:-$REPO_DIR/dist}"

STAGE="$(mktemp -d)"
PKG="$STAGE/sdev"
mkdir -p "$PKG"

# Tool code + top-level reference assets.
cp -R "$REPO_DIR/bin" "$PKG/bin"
cp "$REPO_DIR/install" "$PKG/install"
for f in README.md LICENSE VERSION; do
    [[ -f "$REPO_DIR/$f" ]] && cp "$REPO_DIR/$f" "$PKG/$f"
done

# core/: ship ONLY the default config + example registry — no personal projects, no clones.
mkdir -p "$PKG/core/projects.d"
cp "$REPO_DIR/core/.task-config.yml" "$PKG/core/.task-config.yml"
cp "$REPO_DIR/core/projects.d/example.yml" "$PKG/core/projects.d/example.yml"

# confs/: ship ONLY *.example files — never real env files.
if [[ -d "$REPO_DIR/confs" ]]; then
    while IFS= read -r f; do
        rel="${f#"$REPO_DIR"/}"
        mkdir -p "$PKG/$(dirname "$rel")"
        cp "$f" "$PKG/$rel"
    done < <(find "$REPO_DIR/confs" -type f -name '*.example')
fi

mkdir -p "$OUT_DIR"
ZIP="$OUT_DIR/sdev-$VERSION.zip"
rm -f "$ZIP"
( cd "$STAGE" && zip -rq "$ZIP" sdev )
rm -rf "$STAGE"

echo "$ZIP"
```

- [ ] **Step 4: Make it executable**

Run: `chmod +x dist`

- [ ] **Step 5: Run the test to verify it passes**

Run: `bats tests/dist.bats`
Expected: PASS. (Requires `zip`/`unzip`.)

- [ ] **Step 6: End-to-end smoke check (manual, optional but recommended)**

Run:
```bash
./dist /tmp/sdevdist
cd /tmp && rm -rf unz && mkdir unz && cd unz && unzip -q /tmp/sdevdist/sdev-*.zip
SDEV_INSTALL=/tmp/itest SDEV_HOME=/tmp/htest SDEV_BIN_DIR=/tmp/btest bash sdev/install
```
Expected: install succeeds; `/tmp/itest/bin/sdev` exists, `/tmp/htest/core/.task-config.yml` seeded, `/tmp/btest/sdev` symlink present.

- [ ] **Step 7: Commit**

```bash
git add dist tests/dist.bats
git commit -m "feat: add dist script to build the distributable zip"
```

---

### Task 7: Documentation — install-from-zip, `sdev init`, upgrade

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Replace the `## Install` section**

Replace the current `## Install` section (the `git clone … export PATH …` block) with:

```markdown
## Install (from the zip)

You'll receive a `sdev-<version>.zip`. Then:

```bash
unzip sdev-<version>.zip
cd sdev
./install        # checks deps, installs the tool, seeds ~/.sdev, links `sdev` onto your PATH
```

`./install` is idempotent and never touches your data in `~/.sdev`. If `~/.local/bin` isn't on your `PATH`, it prints the line to add.

**Requirements** (the installer checks these): bash ≥ 4 (`brew install bash` on macOS), [yq](https://github.com/mikefarah/yq) v4, and docker + compose (Docker Desktop or OrbStack).

### Where things live

- **Tool code:** `~/.local/share/sdev` (replaceable; overwritten on upgrade).
- **Your data — `$SDEV_HOME`, default `~/.sdev`:** project definitions (`core/projects.d/`), env profiles/secrets (`confs/`), repo clones (`core/<project>/`), and live workspaces (`projects/`). Survives upgrades.
```

- [ ] **Step 2: Replace the `## Define a project` section**

Replace it with the wizard-first flow:

```markdown
## Configure your first project

```bash
sdev init
```

`sdev init` asks for a project name, your repos (git URL or local checkout), branches, and compose roles. It writes `~/.sdev/core/projects.d/<name>.yml`, clones/links the repos under `~/.sdev/core/<name>/`, seeds `~/.sdev/confs/<name>/<prefix>.local.env`, and prints the exact commands to bring a stack up.

To edit a project later, open its YAML in `~/.sdev/core/projects.d/`.
```

- [ ] **Step 3: Add an `## Upgrading` section (before `## License`)**

```markdown
## Upgrading

Unzip the newer `sdev-<version>.zip` and re-run `./install`. Your `~/.sdev` config, secrets, clones, and workspaces are preserved — only the tool code is replaced.

### Coming from an older in-repo layout?

If you previously ran sdev from a git clone (config under the clone itself), move it into `~/.sdev`:

```bash
sdev migrate --from /path/to/old/sdev-clone
```
```

- [ ] **Step 4: Verify the suite is still green**

Run: `bats tests/`
Expected: PASS (all tests across all files).

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs: install-from-zip, sdev init quickstart, upgrade + migrate"
```

---

## Self-Review

**Spec coverage:**
- Path model (`SDEV_INSTALL`/`SDEV_HOME`, accessor table) → Tasks 1 & 2.
- Migration + backward-compat (legacy `WORKSPACE_ROOT` alias) → Task 1 (alias) + Task 3 (`migrate`).
- `dist` (tool-only zip, exclusions) → Task 6.
- `install` (deps, place tool, seed home, PATH, idempotent, mac+linux) → Task 5.
- `sdev init` wizard → Task 4.
- Cross-platform (`open` fallback already present in `bin/sdev`; installer handles both OSes) → Task 5; the existing `code`/`open` fallbacks in `bin/sdev` are unchanged and already cover Linux/no-editor cases.
- Tests (helpers, dist exclusions, install idempotency, init YAML, root resolution, legacy fallback) → Tasks 1,3,4,5,6.
- Docs → Task 7.

**Placeholder scan:** No TBD/TODO; every code step contains complete content. `<name>`/`<slug>`/`<version>` appear only inside command examples and generated paths, which is intentional.

**Type/name consistency:** `SDEV_INSTALL`, `SDEV_HOME`, `ensure_home`, `GLOBAL_CONFIG`, `PROJECTS_DIR` used identically across Tasks 1–6. The `init`/`migrate` dispatch keywords match the `is_reserved` list and the `case` arms added in Task 3. Env overrides `SDEV_INSTALL`/`SDEV_HOME`/`SDEV_BIN_DIR` are consistent between `install` and its test.

**Known assumptions (acceptable):** `tests/install.bats` and `tests/dist.bats` assume `bash≥4`, `yq`, `docker`, and `zip`/`unzip` are present — the same environment `tests/bootstrap.bats` already requires. `VERSION` is maintained by hand.
```
