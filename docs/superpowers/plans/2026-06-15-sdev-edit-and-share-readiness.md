# sdev `edit` + wider-zip-sharing readiness — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `sdev edit <project>` (interactive add/remove repo + conf/shell/stack edits), close the stranger's-journey gaps (git dep check, stack prompt in init, docs, zip checksum) so the zip can be handed to a wider audience.

**Architecture:** New `bin/edit-project` script dispatched from `bin/sdev`, sharing a new `add_repo_source` helper in `_lib.sh` with `bin/init`. All project-YAML mutations use `yq -i` (already used in `new-task`). Worktree removal mirrors the safe pattern already in `new-task`/`end-task` (`git worktree remove --force`, branches kept). Docs and dist polish are additive.

**Tech Stack:** bash ≥4, yq v4 (mikefarah), git, docker; bats 1.x for tests.

**Spec:** `docs/superpowers/specs/2026-06-15-sdev-edit-and-share-readiness-design.md`

---

## File Structure

- **Create** `bin/edit-project` — the interactive editor (show summary; add/remove repo; set conf_prefix/shell_service/stack_services).
- **Modify** `bin/_lib.sh` — add `add_repo_source <project> <name> <spec>` helper.
- **Modify** `bin/init` — call `add_repo_source`; add a `stack_services` prompt.
- **Modify** `bin/sdev` — register `edit` in `is_reserved`, `usage`, dispatch.
- **Modify** `install`, `bootstrap` — add `git` to dependency checks.
- **Modify** `dist` — emit `<zip>.sha256`.
- **Modify** `README.md` — Modeling-your-stack, Troubleshooting, local-repo caveats, platform notes, document `sdev edit`.
- **Tests:** `tests/lib_repo_source.bats` (new), `tests/edit.bats` (new), extend `tests/init.bats`, `tests/install.bats`, `tests/dist.bats`.

Run the whole suite with `bats tests/` from the repo root. Lint with `shellcheck bin/* install bootstrap dist`.

---

## Task 1: `add_repo_source` helper in `_lib.sh`

**Files:**
- Modify: `bin/_lib.sh` (append near the other repo helpers, before `die()`)
- Test: `tests/lib_repo_source.bats` (create)

- [ ] **Step 1: Write the failing test**

```bash
# tests/lib_repo_source.bats
load helpers
setup() {
  make_fixture
  SDEV_TARGET="$(mktemp -d)/home"; export SDEV_HOME="$SDEV_TARGET"
  mkdir -p "$SDEV_HOME"
  SRC="$(mktemp -d)/svc"; make_source_repo "$SRC" main
}
teardown() { rm -rf "$WORKSPACE_ROOT" "$(dirname "$SDEV_TARGET")" "$(dirname "$SRC")"; }

# Source _lib.sh from the fixture so SDEV_HOME resolution uses our target.
_load_lib() { SDEV_HOME="$SDEV_TARGET" source "$WORKSPACE_ROOT/bin/_lib.sh"; }

@test "add_repo_source symlinks an existing local repo" {
  _load_lib
  run add_repo_source acme api "$SRC"
  [ "$status" -eq 0 ]
  [ "$output" = "linked" ]
  [ -L "$SDEV_TARGET/core/acme/api" ]
  [ -e "$SDEV_TARGET/core/acme/api/.git" ]
}

@test "add_repo_source clones a URL (file:// remote)" {
  _load_lib
  run add_repo_source acme api "file://$SRC"
  [ "$status" -eq 0 ]
  [ "$output" = "cloned" ]
  [ -d "$SDEV_TARGET/core/acme/api/.git" ]
}

@test "add_repo_source rejects a non-repo path" {
  _load_lib
  notrepo="$(mktemp -d)"
  run add_repo_source acme api "$notrepo"
  [ "$status" -ne 0 ]
  rm -rf "$notrepo"
}

@test "add_repo_source refuses an already-existing dest" {
  _load_lib
  add_repo_source acme api "$SRC"
  run add_repo_source acme api "$SRC"
  [ "$status" -ne 0 ]
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bats tests/lib_repo_source.bats`
Expected: FAIL — `add_repo_source: command not found`.

- [ ] **Step 3: Add the helper to `bin/_lib.sh`**

Insert before `die() { ... }` near the end of the file:

```bash
# Clone (git URL) or symlink (existing local git repo) a repo source into
# core/<project>/<name>. Expands a leading ~. Echoes "cloned" or "linked" on
# success; prints the reason to stderr and returns 1 on failure.
add_repo_source() {   # $1=project $2=name $3=source_spec
    local project="$1" name="$2" spec="${3:-}"
    spec="${spec/#\~/$HOME}"
    local dest="$SDEV_HOME/core/$project/$name"
    if [[ -e "$dest" || -L "$dest" ]]; then
        echo "source already exists at $dest" >&2; return 1
    fi
    mkdir -p "$SDEV_HOME/core/$project"
    if [[ "$spec" == *://* || "$spec" == git@* ]]; then
        if ! git clone "$spec" "$dest" >&2; then
            echo "clone failed: $spec" >&2; return 1
        fi
        echo cloned
    elif [[ -d "$spec/.git" || -f "$spec/.git" ]]; then
        local abs; abs="$(cd "$spec" && pwd)"
        ln -s "$abs" "$dest"
        echo linked
    elif [[ -e "$spec" ]]; then
        echo "'$spec' exists but is not a git repo (no .git)" >&2; return 1
    else
        echo "'$spec' is not a git URL or an existing local repo" >&2; return 1
    fi
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bats tests/lib_repo_source.bats`
Expected: PASS (4 tests).

- [ ] **Step 5: shellcheck + commit**

```bash
shellcheck bin/_lib.sh
git add bin/_lib.sh tests/lib_repo_source.bats
git commit -m "feat(lib): add add_repo_source helper (clone or symlink a repo source)"
```

---

## Task 2: Refactor `init` to use `add_repo_source`

No behavior change — `init`'s existing tests must still pass.

**Files:**
- Modify: `bin/init:44-60`
- Test: `tests/init.bats` (existing, unchanged in this task)

- [ ] **Step 1: Confirm existing init tests pass (baseline)**

Run: `bats tests/init.bats`
Expected: PASS (4 tests).

- [ ] **Step 2: Replace the clone/symlink block**

In `bin/init`, replace lines 44-60 (from `mkdir -p "$SDEV_HOME/core/$project"` through the closing `fi` of the source classification) with:

```bash
    if out="$(add_repo_source "$project" "$name" "$source_spec")"; then
        log "$out repo '$name'"
    else
        echo "  skipping repo '$name'" >&2
        continue
    fi
```

(The surrounding `while` loop, the `R_NAME+=(...)` append on the next line, and everything else stay as-is. `add_repo_source` is available because `init` already sources `_lib.sh`.)

- [ ] **Step 3: Run init tests to verify no regression**

Run: `bats tests/init.bats`
Expected: PASS (4 tests) — symlink test, abort test, ~-path test, refuse-overwrite test all green.

- [ ] **Step 4: shellcheck + commit**

```bash
shellcheck bin/init
git add bin/init
git commit -m "refactor(init): use shared add_repo_source helper"
```

---

## Task 3: `git` dependency check (P0)

**Files:**
- Modify: `install` (after the docker check, before the `if [[ $ok -ne 1 ]]` block)
- Modify: `bootstrap` (`check_deps`)
- Test: `tests/install.bats` (extend the first test)

- [ ] **Step 1: Write the failing assertion**

In `tests/install.bats`, inside the test `@test "install places tool, seeds home, links sdev"`, add after the existing PATH assertion:

```bash
  [[ "$output" == *"git"* ]]
```

- [ ] **Step 2: Run to verify it fails**

Run: `bats tests/install.bats -f "places tool"`
Expected: FAIL — output has no `git` line yet.

- [ ] **Step 3: Add the git check to `install`**

In `install`, immediately after the docker check block (the `if command -v docker ... fi` ending at the line before `if [[ $ok -ne 1 ]]; then`), insert:

```bash
if command -v git >/dev/null 2>&1; then say "✓ git"; else
    warn "✗ git not found (required for project setup and tasks)."
    if [[ "$os" == Darwin ]]; then warn "   macOS: xcode-select --install (or brew install git)"
    else warn "   Linux: install git via your package manager"; fi
    ok=0
fi
```

- [ ] **Step 4: Add the git check to `bootstrap`**

In `bootstrap`'s `check_deps()`, after the docker check line and before `[[ $ok -eq 1 ]]`, insert:

```bash
    if command -v git >/dev/null 2>&1; then echo "✓ git"; else
        echo "✗ git not found (required for project setup and tasks)." >&2; ok=0; fi
```

- [ ] **Step 5: Run tests to verify pass**

Run: `bats tests/install.bats`
Expected: PASS (all install tests).

- [ ] **Step 6: shellcheck + commit**

```bash
shellcheck install bootstrap
git add install bootstrap tests/install.bats
git commit -m "feat(install): require git in dependency checks"
```

---

## Task 4: `stack_services` prompt in `init`

Adds a prompt after "Shell service". This shifts stdin order, so existing `init.bats` heredocs need one extra line.

**Files:**
- Modify: `bin/init` (add prompt after line 30; write `stack_services` in the YAML heredoc)
- Test: `tests/init.bats` (fix existing heredocs + add one test)

- [ ] **Step 1: Update existing init heredocs (they break otherwise)**

In `tests/init.bats`, each `init` invocation feeds lines in order: project, conf_prefix, shell_service, then the repo loop. Insert a **blank line** (accept default stack) immediately **after** the shell-service line (`api`) and **before** the first repo name in every heredoc. Concretely, the four heredocs become (blank line added after `api`):

Test "init writes a project registry…":
```
acme
app
api

svc
$SRC
main
api

```
Test "init aborts when no repos are added":
```
acme
app
api


```
Test "init links an existing repo given by a ~-relative path":
```
acme
app
api

api
~/work/api
main
api

```
Test "init refuses to overwrite an existing project" (both heredocs):
```
acme
app
api

svc
$SRC
main
api

```

- [ ] **Step 2: Write the new failing test**

Add to `tests/init.bats`:

```bash
@test "init writes stack_services when provided" {
  run env SDEV_HOME="$SDEV_TARGET" "$WORKSPACE_ROOT/bin/init" <<EOF
acme
app
api
api,db
svc
$SRC
main
api

EOF
  [ "$status" -eq 0 ]
  run yq -r '.stack_services | join(",")' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "api,db" ]
}

@test "init omits stack_services on blank (inherits global)" {
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
  run yq -r 'has("stack_services")' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "false" ]
}
```

- [ ] **Step 3: Run to verify failure**

Run: `bats tests/init.bats -f stack_services`
Expected: FAIL — init has no stack prompt; the `api,db` line is consumed as a repo name.

- [ ] **Step 4: Add the prompt + collect the answer in `init`**

In `bin/init`, after the shell-service block (after line 30, `valid_token "$shell_service" || die ...`), add:

```bash
stack_services="$(ask 'Stack services (host-exposed, comma/space separated; blank = inherit default)' '')"
declare -a STACK=()
if [[ -n "$stack_services" ]]; then
    for s in ${stack_services//,/ }; do
        valid_token "$s" || die "invalid stack service '$s'"
        STACK+=("$s")
    done
fi
```

- [ ] **Step 5: Write `stack_services` into the project YAML**

In `bin/init`, inside the YAML-writing block (the `{ ... } > "$proj_file"` group, currently lines 66-77), add — right after the `echo "default_shell_service: $shell_service"` line and before `echo "repos:"`:

```bash
    if [[ ${#STACK[@]} -gt 0 ]]; then
        printf 'stack_services: ['
        printf '%s' "${STACK[0]}"
        for ((i=1; i<${#STACK[@]}; i++)); do printf ', %s' "${STACK[$i]}"; done
        printf ']\n'
    fi
```

- [ ] **Step 6: Run init tests to verify pass**

Run: `bats tests/init.bats`
Expected: PASS (6 tests).

- [ ] **Step 7: shellcheck + commit**

```bash
shellcheck bin/init
git add bin/init tests/init.bats
git commit -m "feat(init): prompt for stack_services"
```

---

## Task 5: `bin/edit-project` skeleton + `sdev edit` dispatch

Creates the editor with the show-summary loop and quit; wires it into `sdev`.

**Files:**
- Create: `bin/edit-project`
- Modify: `bin/sdev` (`is_reserved`, `usage`, dispatch)
- Test: `tests/edit.bats` (create)

- [ ] **Step 1: Write the failing test**

```bash
# tests/edit.bats
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
  ln -s "$SRC" "$SDEV_TARGET/core/acme/api" 2>/dev/null || { mkdir -p "$SDEV_TARGET/core/acme"; ln -s "$SRC" "$SDEV_TARGET/core/acme/api"; }
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
```

- [ ] **Step 2: Run to verify failure**

Run: `bats tests/edit.bats`
Expected: FAIL — `bin/edit-project` does not exist.

- [ ] **Step 3: Create `bin/edit-project`**

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=./_lib.sh
source "$SCRIPT_DIR/_lib.sh"
ensure_home

DELETE_SOURCE=0
ARGS=()
while [[ $# -gt 0 ]]; do
    case "$1" in
        --delete-source) DELETE_SOURCE=1; shift ;;
        *) ARGS+=("$1"); shift ;;
    esac
done
set -- "${ARGS[@]:-}"
[[ -z "${1:-}" ]] && set --

project="${1:-$(resolve_project "${SDEV_PROJECT:-}")}"
[[ -n "$project" ]] || die "no project given and none active"
proj_file="$(project_config_file "$project")"
[[ -f "$proj_file" ]] || die "project '$project' not found — create it with: sdev init"

ask() {   # $1=label $2=default(optional)
    local label="$1" def="${2:-}" ans
    if [[ -n "$def" ]]; then printf '%s [%s]: ' "$label" "$def" >&2
    else printf '%s: ' "$label" >&2; fi
    read -r ans || ans=""
    echo "${ans:-$def}"
}
valid_token() { [[ "$1" =~ ^[A-Za-z0-9._-]+$ ]]; }
valid_ref()   { [[ "$1" =~ ^[A-Za-z0-9._/-]+$ ]]; }

show() {
    echo ""
    echo "Project: $project  ($proj_file)"
    echo "  conf_prefix:    $(yq -r '.conf_prefix // "app"' "$proj_file")"
    echo "  shell_service:  $(yq -r '.default_shell_service // "api"' "$proj_file")"
    local ss; ss="$(yq -r '(.stack_services // []) | join(", ")' "$proj_file")"
    if [[ -n "$ss" ]]; then echo "  stack_services: $ss"; else echo "  stack_services: (inherits global default)"; fi
    echo "  repos:"
    local r p b role dest kind
    while read -r r; do
        [[ -z "$r" ]] && continue
        p="$(yq -r ".repos.\"$r\".path" "$proj_file")"
        b="$(yq -r ".repos.\"$r\".default_base" "$proj_file")"
        role="$(yq -r ".repos.\"$r\".compose_role" "$proj_file")"
        dest="$SDEV_HOME/core/$project/$p"
        if [[ -L "$dest" ]]; then kind="symlink -> $(readlink "$dest")"
        elif [[ -d "$dest" ]]; then kind="clone"
        else kind="MISSING SOURCE"; fi
        printf '    %-12s base=%-8s role=%-8s %s\n' "$r" "$b" "$role" "$kind"
    done < <(yq -r '.repos | keys | .[]' "$proj_file" 2>/dev/null)
}

while :; do
    show
    printf '\n  [a] add repo   [r] remove repo   [p] conf prefix   [s] shell service   [t] stack services   [q] quit\n> ' >&2
    read -r choice || break
    case "$choice" in
        q|"") break ;;
        *) echo "  (not yet implemented: '$choice')" >&2 ;;
    esac
done
```

```bash
chmod +x bin/edit-project
```

- [ ] **Step 4: Wire `edit` into `bin/sdev`**

In `bin/sdev`, add `edit` to the `is_reserved()` case list (line 60), e.g. change `...|init|migrate|help|...` to include `edit`:
```bash
        new|up|down|nuke|end|ls|list|ps|logs|shell|open|code|cd|use|projects|init|edit|migrate|help|-h|--help) return 0 ;;
```
Add to `usage()` after the `init` line:
```
  edit [<project>] [--delete-source]   add/remove repos, edit conf/shell/stack
```
Add a dispatch case after the `init)` case:
```bash
    edit)
        exec env SDEV_PROJECT="$PROJECT" "$SCRIPT_DIR/edit-project" "$@"
        ;;
```

- [ ] **Step 5: Run to verify pass**

Run: `bats tests/edit.bats`
Expected: PASS (2 tests).

- [ ] **Step 6: shellcheck + commit**

```bash
shellcheck bin/edit-project bin/sdev
git add bin/edit-project bin/sdev tests/edit.bats
git commit -m "feat(sdev): add 'edit' command skeleton (summary + dispatch)"
```

---

## Task 6: `edit` — add a repo

**Files:**
- Modify: `bin/edit-project` (add `add_repo` fn + wire `a`)
- Test: `tests/edit.bats`

- [ ] **Step 1: Write the failing test**

Add to `tests/edit.bats`:

```bash
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
  [ "$status" -eq 0 ]   # editor loop survives; the add is refused
  run yq -r '.repos | keys | length' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "1" ]
}
```

- [ ] **Step 2: Run to verify failure**

Run: `bats tests/edit.bats -f "edit add"`
Expected: FAIL — `a` prints "not yet implemented".

- [ ] **Step 3: Add the `add_repo` function**

In `bin/edit-project`, add this function above the `while :;` loop:

```bash
add_repo() {
    local name source base role out
    name="$(ask 'Repo name')"
    if ! validate_slug "$name"; then echo "  invalid repo name '$name'" >&2; return 0; fi
    if yq -e ".repos | has(\"$name\")" "$proj_file" >/dev/null 2>&1; then
        echo "  repo '$name' already exists" >&2; return 0
    fi
    source="$(ask '  git URL to clone, or path to an existing local repo')"
    base="$(ask '  default base branch' main)"
    if ! valid_ref "$base"; then echo "  invalid base branch '$base'" >&2; return 0; fi
    role="$(ask '  compose role' "$name")"
    if ! valid_token "$role"; then echo "  invalid compose role '$role'" >&2; return 0; fi
    if out="$(add_repo_source "$project" "$name" "$source")"; then
        log "$out repo '$name'"
    else
        echo "  could not add source; aborting add" >&2; return 0
    fi
    yq -i ".repos.\"$name\" = {\"path\": \"$name\", \"default_base\": \"$base\", \"compose_role\": \"$role\", \"link_node_modules\": false}" "$proj_file"
    log "added repo '$name'"
}
```

- [ ] **Step 4: Wire `a` in the menu**

In the menu `case`, replace the `q|"") break ;;` / `*)` block so it reads:
```bash
    case "$choice" in
        a) add_repo ;;
        q|"") break ;;
        *) echo "  (not yet implemented: '$choice')" >&2 ;;
    esac
```

- [ ] **Step 5: Run to verify pass**

Run: `bats tests/edit.bats`
Expected: PASS (4 tests).

- [ ] **Step 6: shellcheck + commit**

```bash
shellcheck bin/edit-project
git add bin/edit-project tests/edit.bats
git commit -m "feat(edit): add repo (clone/symlink + yq write)"
```

---

## Task 7: `edit` — remove a repo (no live worktrees)

**Files:**
- Modify: `bin/edit-project` (add `remove_repo` fn + wire `r`)
- Test: `tests/edit.bats`

- [ ] **Step 1: Write the failing test**

```bash
@test "edit remove: deletes the YAML block and unlinks a symlinked source" {
  run edit acme <<EOF
r
api
q
EOF
  [ "$status" -eq 0 ]
  run yq -r '.repos | has("api")' "$SDEV_TARGET/core/projects.d/acme.yml"
  [ "$output" = "false" ]
  [ ! -e "$SDEV_TARGET/core/acme/api" ]    # symlink removed; real $SRC untouched
  [ -d "$SRC/.git" ]                        # original repo still there
}

@test "edit remove: keeps a cloned source by default" {
  # add a cloned repo, then remove it without --delete-source
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
  [ -d "$SDEV_TARGET/core/acme/clone/.git" ]   # clone kept
}
```

- [ ] **Step 2: Run to verify failure**

Run: `bats tests/edit.bats -f "edit remove"`
Expected: FAIL — `r` not implemented.

- [ ] **Step 3: Add the `remove_repo` function**

```bash
remove_repo() {
    local name; name="$(ask 'Repo to remove')"
    if ! yq -e ".repos | has(\"$name\")" "$proj_file" >/dev/null 2>&1; then
        echo "  no such repo '$name'" >&2; return 0
    fi
    local p; p="$(yq -r ".repos.\"$name\".path" "$proj_file")"

    # 1. live task worktrees?
    local -a wts=()
    shopt -s nullglob
    local d
    for d in "$SDEV_HOME/projects/$project"/*/; do
        [[ -e "$d$p/.git" ]] && wts+=("${d}$p")
    done
    shopt -u nullglob
    if [[ ${#wts[@]} -gt 0 ]]; then
        echo "  ⚠ live task worktrees use '$name':" >&2
        printf '      %s\n' "${wts[@]}" >&2
        echo "  end those tasks (sdev end <slug>), or type 'force' to remove the worktrees (branches kept):" >&2
        local ans; read -r ans || ans=""
        if [[ "$ans" != force ]]; then echo "  aborted removal of '$name'" >&2; return 0; fi
        local src; src="$(repo_source_dir "$project" "$p")"
        local w
        for w in "${wts[@]}"; do
            git -C "$src" worktree remove --force "$w" 2>/dev/null || true
            echo "    removed worktree $w (branch kept)" >&2
        done
    fi

    # 2. source: unlink symlink; keep clone unless --delete-source
    local dest="$SDEV_HOME/core/$project/$p"
    if [[ -L "$dest" ]]; then
        rm -f "$dest"; log "unlinked symlinked source $dest"
    elif [[ -d "$dest" ]]; then
        if [[ $DELETE_SOURCE -eq 1 ]]; then
            local dirty; dirty="$(git -C "$dest" status --porcelain 2>/dev/null | wc -l | tr -d ' ')"
            if [[ "$dirty" != 0 ]]; then
                echo "  ⚠ $dest has $dirty uncommitted change(s). Type 'yes' to delete anyway:" >&2
                local a; read -r a || a=""
                if [[ "$a" != yes ]]; then echo "  keeping source $dest" >&2; dest=""; fi
            fi
            if [[ -n "$dest" ]]; then rm -rf "$dest"; log "deleted clone $dest"; fi
        else
            log "kept clone at $dest (pass --delete-source to remove)"
        fi
    fi

    # 3. drop the YAML block
    yq -i "del(.repos.\"$name\")" "$proj_file"
    log "removed repo '$name'"
}
```

- [ ] **Step 4: Wire `r` in the menu**

Add `r) remove_repo ;;` to the menu `case` (above `q|"")`).

- [ ] **Step 5: Run to verify pass**

Run: `bats tests/edit.bats`
Expected: PASS (6 tests).

- [ ] **Step 6: shellcheck + commit**

```bash
shellcheck bin/edit-project
git add bin/edit-project tests/edit.bats
git commit -m "feat(edit): remove repo (worktree-safe, keeps source by default)"
```

---

## Task 8: `edit` — remove refusal & `force` with live worktrees

**Files:**
- Modify: `bin/edit-project` (already implemented in Task 7 — this task only adds tests)
- Test: `tests/edit.bats`

- [ ] **Step 1: Write the test**

```bash
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
  [[ "$output" == *"task/foo"* ]]   # branch preserved
}
```

- [ ] **Step 2: Run to verify pass**

Run: `bats tests/edit.bats -f "refuses when a task worktree"`
Expected: PASS (logic landed in Task 7).

- [ ] **Step 3: Commit**

```bash
git add tests/edit.bats
git commit -m "test(edit): cover worktree refusal and force-remove (branch kept)"
```

---

## Task 9: `edit` — set conf_prefix / shell_service / stack_services

**Files:**
- Modify: `bin/edit-project` (`set_scalar`, `set_stack` + wire `p`/`s`/`t`)
- Test: `tests/edit.bats`

- [ ] **Step 1: Write the failing test**

```bash
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
```

- [ ] **Step 2: Run to verify failure**

Run: `bats tests/edit.bats -f "edit p/s/t"`
Expected: FAIL — `p`/`s`/`t` not implemented.

- [ ] **Step 3: Add the functions**

```bash
set_scalar() {   # $1=yaml-key $2=label
    local key="$1" label="$2" v
    v="$(ask "$label")"
    if ! valid_token "$v"; then echo "  invalid value '$v'" >&2; return 0; fi
    yq -i ".$key = \"$v\"" "$proj_file"
    log "set $key = $v"
}

set_stack() {
    local line; line="$(ask 'Stack services (comma/space separated; blank = inherit global)')"
    if [[ -z "$line" ]]; then
        yq -i 'del(.stack_services)' "$proj_file"; log "stack_services -> inherit global"; return 0
    fi
    local -a svcs=(); local s
    for s in ${line//,/ }; do
        if ! valid_token "$s"; then echo "  invalid service '$s'" >&2; return 0; fi
        svcs+=("$s")
    done
    local json="["; local i
    for i in "${!svcs[@]}"; do
        json+="\"${svcs[$i]}\""
        [[ $i -lt $(( ${#svcs[@]} - 1 )) ]] && json+=","
    done
    json+="]"
    yq -i ".stack_services = $json" "$proj_file"
    log "stack_services = ${svcs[*]}"
}
```

- [ ] **Step 4: Wire `p`/`s`/`t`**

Add to the menu `case`:
```bash
        p) set_scalar conf_prefix 'Conf prefix' ;;
        s) set_scalar default_shell_service 'Shell service' ;;
        t) set_stack ;;
```

- [ ] **Step 5: Run to verify pass**

Run: `bats tests/edit.bats`
Expected: PASS (8 tests).

- [ ] **Step 6: shellcheck + commit**

```bash
shellcheck bin/edit-project
git add bin/edit-project tests/edit.bats
git commit -m "feat(edit): set conf_prefix / shell_service / stack_services"
```

---

## Task 10: `dist` — emit SHA256 checksum

**Files:**
- Modify: `dist` (after the zip is created)
- Test: `tests/dist.bats`

- [ ] **Step 1: Write the failing test**

Add to `tests/dist.bats`:

```bash
@test "dist emits a .sha256 checksum next to the zip" {
  run bash "$REPO/dist" "$OUT"
  [ "$status" -eq 0 ]
  zipfile="$output"
  [ -f "$zipfile.sha256" ]
  # checksum verifies
  ( cd "$(dirname "$zipfile")" && shasum -a 256 -c "$(basename "$zipfile").sha256" )
}
```

- [ ] **Step 2: Run to verify failure**

Run: `bats tests/dist.bats -f checksum`
Expected: FAIL — no `.sha256` produced.

- [ ] **Step 3: Add checksum generation to `dist`**

In `dist`, between `( cd "$STAGE" && zip -rq "$ZIP" sdev )` and `echo "$ZIP"`, insert:

```bash
# Integrity checksum for the published zip.
( cd "$(dirname "$ZIP")" && shasum -a 256 "$(basename "$ZIP")" > "$(basename "$ZIP").sha256" )
```

- [ ] **Step 4: Run to verify pass**

Run: `bats tests/dist.bats`
Expected: PASS (3 tests).

- [ ] **Step 5: shellcheck + commit**

```bash
shellcheck dist
git add dist tests/dist.bats
git commit -m "feat(dist): emit sha256 checksum for the zip"
```

---

## Task 11: README docs

No automated test — verify by reading. Add four sections + document `sdev edit`.

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Document `sdev edit` in the daily-use table**

In the daily-use command table (after the `sdev init` / project rows), add a row:
```
| `sdev edit [<project>]` | add/remove repos, edit conf prefix / shell service / stack services |
```
And in the "Configure your first project" section, after the `sdev init` paragraph, add:
> To change a project later — add or remove a repo, or edit its conf prefix, shell service, or stack — run `sdev edit <project>` (an interactive menu). Removing a repo that still has live task worktrees is refused unless you confirm; cloned sources are kept unless you pass `--delete-source` (symlinked local repos are only unlinked, never deleted).

- [ ] **Step 2: Add "Modeling your stack" section**

After the "Configure your first project" section, add:

```markdown
## Modeling your stack

A new project inherits the default compose template — a generic
`db + redis + api + ui + nginx` stack with **placeholder** images (`sdev up`
runs, but `api`/`ui` are stubs until you point them at your images). Two knobs:

- **`stack_services`** (set in `sdev init`/`sdev edit`, or the project YAML):
  the host-exposed services that get a port offset. Trim it to what you have —
  a single API → `[api]`.
- **`template:`** in `core/projects.d/<project>.yml`: point at your own
  `core/<project>/docker-compose.tmpl` to model a real stack. Each repo's
  `compose_role` maps it to a compose service (the role names the service the
  repo's worktree is mounted into).

Your repos need a Docker setup (image or `build:` context) matching those roles
for `sdev up` to run your actual app.
```

- [ ] **Step 3: Add "Existing local repos" caveats**

In the project-setup area, add:

```markdown
### Pointing at an existing local clone

When you give `sdev init`/`sdev edit` a **path** (not a URL), sdev *symlinks*
that repo as the worktree source — it is not copied. Notes:

- `sdev new` fetches `origin/<base>` before branching, so the local repo needs a
  real `origin`. Use `sdev new <slug> --no-fetch` to skip the fetch (offline, or
  no remote).
- Don't move or delete the original — task worktrees branch off it and will break.
- `sdev edit` → remove only *unlinks* the symlink; your repo is untouched.
```

- [ ] **Step 4: Add "Troubleshooting" section near the end**

```markdown
## Troubleshooting

- **`docker: command not found` / daemon errors on `sdev up`:** start Docker
  Desktop or OrbStack; sdev shells out to `docker compose`/`docker-compose`.
- **"port is already allocated":** another task or app holds the port. Each task
  gets a unique offset; stop a conflicting task (`sdev down <slug>`) or an
  unrelated process. Ports are listed by `sdev ls`.
- **macOS: "bash >= 4 required":** macOS ships bash 3.2. `brew install bash`
  (sdev's scripts use `#!/usr/bin/env bash`, so a newer bash on `PATH` is used).
- **`yq` errors / wrong output:** sdev needs **mikefarah `yq` v4**, not the
  Python `yq`. Check `yq --version`; install via `brew install yq`.
- **"task '<slug>' not found in project '<project>'":** you're in a different
  active project. Check `sdev use` and pass `-p <project>` or `sdev use <project>`.
- **Staging prompt blocks `sdev up`:** `staging` tasks require typing `staging`
  to confirm (they touch real staging data). Pass `--yes` for non-interactive use.
- **`sdev init`/`sdev edit` re-run:** `init` refuses to overwrite an existing
  project; use `sdev edit` to change one.
```

- [ ] **Step 5: Add platform notes**

In the Requirements/Install area, add a short note:
```markdown
> **Platforms:** macOS and Linux (incl. WSL). The installer detects your shell rc
> (`.zshrc`/`.bashrc`/`.profile`). `sdev open` launches a browser on macOS and
> prints the URL elsewhere. Repo base branches default to `main` — set
> `master`/`develop` per repo in `sdev init`/`sdev edit`.
```

- [ ] **Step 6: Commit**

```bash
git add README.md
git commit -m "docs: edit command, modeling your stack, local-repo caveats, troubleshooting, platforms"
```

---

## Final verification

- [ ] **Run the whole suite**

Run: `bats tests/`
Expected: all green (existing + new `lib_repo_source.bats`, `edit.bats`, extended `init/install/dist`).

- [ ] **Lint everything**

Run: `shellcheck bin/* install bootstrap dist`
Expected: no warnings (add targeted `# shellcheck disable=` only where a warning is a known false positive, matching existing style).

- [ ] **Smoke test the editor by hand**

```bash
SDEV_HOME=$(mktemp -d) bin/init        # create a throwaway project
SDEV_HOME=... bin/sdev edit <project>  # add a repo, remove it, edit stack; confirm YAML
```

- [ ] **Build the zip**

Run: `./dist` then verify `build/sdev-<version>.zip` + `.sha256` exist and `shasum -a 256 -c` passes.

---

## Self-Review notes (author)

- **Spec coverage:** Feature 1 → Tasks 5-9; Feature 2 → Task 1-2; Feature 3 → Task 3; Feature 4 → Task 4; Feature 5 (docs) → Task 11; Feature 6 (checksum/base/platforms) → Tasks 10-11. All covered.
- **Type/name consistency:** `add_repo_source` signature identical across Tasks 1/2/6; `proj_file`, `project`, `DELETE_SOURCE` consistent across edit tasks; menu keys `a/r/p/s/t/q` consistent.
- **Ordering risk:** Task 4 explicitly fixes the init heredoc stdin shift before adding the prompt.
