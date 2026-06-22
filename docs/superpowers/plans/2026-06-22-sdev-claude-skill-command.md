# sdev Claude Code skill + command — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a Claude Code `sdev` skill and a `/sdev-start` slash-command in the sdev repo, package them in the distributed zip, and have `./install` drop them into `~/.claude` on an opt-in basis.

**Architecture:** Two static content files live under a new repo-root `claude/` directory (tool-owned, like `bin/`). `dist` copies `claude/` into the zip. `install` copies `claude/` into the install dir, then — gated on `SDEV_CLAUDE` or an interactive prompt when `~/.claude` exists — copies the two sdev-owned paths into `~/.claude/skills/sdev` and `~/.claude/commands/sdev-start.md`.

**Tech Stack:** POSIX-ish bash (≥ 4), `zip`/`unzip`, bats tests, Markdown content files with YAML frontmatter.

## Global Constraints

- **bash ≥ 4** — scripts use `#!/usr/bin/env bash` and `set -euo pipefail`. Match existing style.
- **Idempotent + upgrade-safe** — re-running `install` overwrites only the two sdev-owned Claude paths; never touches other `~/.claude` content and never creates `~/.claude` uninvited.
- **Tests are bats** under `tests/`, run with `bats tests/`. Match the existing `setup()`/`teardown()`/helper style.
- **Exact install paths:** skill → `~/.claude/skills/sdev/SKILL.md`; command → `~/.claude/commands/sdev-start.md`. Repo source of truth → `claude/skills/sdev/SKILL.md`, `claude/commands/sdev-start.md`.
- **Decision precedence in `install`:** `SDEV_CLAUDE` (`1`/`true`/`yes`/`y` = install, anything else = skip) wins; else, on a real TTY (`-t 0`) with an existing `~/.claude`, prompt `[Y/n]` (default yes); else skip.
- **Commit after each task.** Conventional-commit messages, scoped (`feat(claude)`, `feat(dist)`, `feat(install)`, `docs`).

---

### Task 1: Author the Claude skill + command content files

**Files:**
- Create: `claude/skills/sdev/SKILL.md`
- Create: `claude/commands/sdev-start.md`
- Test: `tests/claude_assets.bats`

**Interfaces:**
- Consumes: nothing.
- Produces: the two content files at the exact repo paths above, consumed by `dist` (Task 2) and `install` (Task 3). `SKILL.md` frontmatter has `name: sdev` and a non-empty `description:`. `sdev-start.md` frontmatter has a non-empty `description:` and `argument-hint: <slug> [description]`.

- [ ] **Step 1: Write the failing test**

Create `tests/claude_assets.bats`:

```bash
setup() {
  REPO="$(cd "$BATS_TEST_DIRNAME/.." && pwd)"
}

@test "skill file exists with name and description frontmatter" {
  skill="$REPO/claude/skills/sdev/SKILL.md"
  [ -f "$skill" ]
  grep -qE '^name:[[:space:]]*sdev[[:space:]]*$' "$skill"
  grep -qE '^description:[[:space:]]*\S' "$skill"
}

@test "start command exists with description and argument-hint" {
  cmd="$REPO/claude/commands/sdev-start.md"
  [ -f "$cmd" ]
  grep -qE '^description:[[:space:]]*\S' "$cmd"
  grep -qE '^argument-hint:[[:space:]]*<slug>' "$cmd"
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bats tests/claude_assets.bats`
Expected: FAIL — files do not exist yet.

- [ ] **Step 3: Create the skill file**

Create `claude/skills/sdev/SKILL.md`:

```markdown
---
name: sdev
description: Use when the user wants isolated or parallel development workspaces, multiple feature branches running at once, per-task docker-compose stacks, to "spin up a workspace/environment", or to set up and configure an sdev project. Covers the full sdev lifecycle — project setup (init/edit) and running tasks (new/up/down/open/logs/shell/ls/end).
---

# sdev — isolated, parallel docker-compose workspaces

sdev runs many isolated, parallel docker-compose workspaces grouped by project.
Each task gets its own git worktrees, env profile, and stack on a unique set of
host ports — so several features (and several projects) run at once without
colliding.

## Concepts

- **Project** — a registry at `core/projects.d/<name>.yml`: which repos it uses,
  its env-profile conf prefix, its docker stack. Projects run in parallel.
- **Task** — an isolated workspace at `projects/<project>/<slug>/`: a git worktree
  (branch `task/<slug>`) of each repo, plus a generated `docker-compose.yml` and
  `.env` with a unique port offset.
- **Profile** — an environment (`local`/`dev`/`staging`) whose config comes from
  `confs/<project>/<conf_prefix>.<profile>.env`. `staging` is guarded.

## First-time setup

Run `sdev init` to define a project: name, repos (git URL or local checkout path),
base branches, conf prefix, and `stack_services`. It writes the project YAML,
clones/links repos, seeds a local env profile, and prints how to bring a stack up.

To change a project later — add/remove a repo, edit conf prefix / shell service /
stack services — run `sdev edit <project>` (interactive menu). Removing a repo with
live task worktrees is refused unless confirmed; cloned sources are kept unless you
pass `--delete-source` (symlinked local repos are only unlinked).

**Modeling the stack:** a new project inherits a generic
`db + redis + api + ui + nginx` template with placeholder images. Two knobs:
`stack_services` (host-exposed services that get a port offset — trim to what you
have, e.g. a single API → `[api]`) and `template:` in the project YAML (point at
your own `docker-compose.tmpl`). Each repo's `compose_role` maps it to a compose
service.

## Daily lifecycle

Pin a project first (`sdev use <project>`) or pass `-p <project>` per command.

| Command | What it does |
|---|---|
| `sdev projects` | list defined projects |
| `sdev use <project>` | pin the active project for this terminal |
| `sdev -p <project> <cmd>` | run one command against a project |
| `sdev new <slug>` | create a task (worktrees + stack) |
| `sdev up <slug>` | start the task's stack (detached) |
| `sdev ps / logs / shell <slug>` | status / tail logs / shell into the service |
| `sdev open <slug>` | open the task's nginx URL |
| `sdev code / cd <slug>` | open task dir in editor / print its path |
| `sdev down / nuke <slug>` | stop (keep volumes) / stop + reclaim volumes |
| `sdev ls` | list all tasks across projects |
| `sdev end <slug>` | tear down + archive a finished task |

`sdev new` fetches `origin/<base>` and branches off the latest `origin/<base>`.
Pass `--no-fetch` to skip the fetch (offline, or a local repo with no remote).

To go from zero to a running workspace in one step, use the `/sdev-start` command.

## Running in parallel

Pin different projects in different terminals (`sdev use acme` here,
`sdev use beta` there). Port offsets come from one global pool across every
project, so multiple stacks run `up` at once with no host-port collisions.

## Gotchas

- **bash ≥ 4** required (macOS ships 3.2 → `brew install bash`).
- **yq v4** must be mikefarah's `yq`, not the Python `yq` (check `yq --version`).
- **staging** tasks require typing `staging` to confirm `sdev up` (pass `--yes`
  non-interactively) — they touch real staging data.
- **symlinked local repos:** don't move or delete the original — task worktrees
  branch off it and will break.
- **"port already allocated":** stop a conflicting task (`sdev down <slug>`);
  ports are listed by `sdev ls`.
```

- [ ] **Step 4: Create the command file**

Create `claude/commands/sdev-start.md`:

```markdown
---
description: Take a feature from zero to a running, isolated sdev workspace — create the task, bring its stack up, record scope, and report the URL.
argument-hint: <slug> [description]
---

Start a new isolated sdev workspace end-to-end.

**Arguments:** `$ARGUMENTS` — the first token is the task `<slug>`; the rest is an
optional one-line description of the work.

Follow these steps, keeping the user informed and stopping to surface any command
that fails (missing dependency, port conflict, staging confirmation):

1. **Confirm a project is active.** Run `sdev projects`. If it lists none, tell the
   user to run `sdev init` first and stop. Otherwise use the pinned `sdev use`
   project (or ask which to use, or pass `-p <project>` on each command below).

2. **Create the task.** Run `sdev new <slug>`. If the machine is offline or the
   source repo has no remote, use `sdev new <slug> --no-fetch`.

3. **Bring the stack up.** Run `sdev up <slug>`.

4. **Record scope.** If a description was given, open the task's generated
   `CLAUDE.md` (in the directory printed by `sdev cd <slug>`) and replace the
   `(one-line — fill in)` placeholder on the **Scope** line with the description.

5. **Report.** Run `sdev ps <slug>`, then show the task's primary nginx URL and
   debug ports (from the task `CLAUDE.md`). Offer to run `sdev open <slug>` to open
   it in a browser.
```

- [ ] **Step 5: Run test to verify it passes**

Run: `bats tests/claude_assets.bats`
Expected: PASS (2 tests).

- [ ] **Step 6: Commit**

```bash
git add claude/skills/sdev/SKILL.md claude/commands/sdev-start.md tests/claude_assets.bats
git commit -m "feat(claude): add sdev skill + /sdev-start command content

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Package the Claude assets in the dist zip

**Files:**
- Modify: `dist`
- Test: `tests/dist.bats` (add one test)

**Interfaces:**
- Consumes: `claude/` from Task 1.
- Produces: zip entries `sdev/claude/skills/sdev/SKILL.md` and `sdev/claude/commands/sdev-start.md`.

- [ ] **Step 1: Write the failing test**

Add to `tests/dist.bats`:

```bash
@test "dist ships the Claude skill + command" {
  run bash "$REPO/dist" "$OUT"
  [ "$status" -eq 0 ]
  zipfile="$output"
  listing="$(unzip -Z1 "$zipfile")"
  echo "$listing" | grep -qx 'sdev/claude/skills/sdev/SKILL.md'
  echo "$listing" | grep -qx 'sdev/claude/commands/sdev-start.md'
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bats tests/dist.bats`
Expected: the new test FAILS — `claude/` is not copied into the stage yet.

- [ ] **Step 3: Add the copy block to `dist`**

In `dist`, after the existing `bin`/`install`/reference-asset copy block (after the loop copying `README.md LICENSE VERSION`, i.e. after the line `done` near line 18), add:

```bash
# claude/: optional Claude Code skill + slash-command (tool-owned).
if [[ -d "$REPO_DIR/claude" ]]; then
    cp -R "$REPO_DIR/claude" "$PKG/claude"
fi
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `bats tests/dist.bats`
Expected: PASS (all tests, including the existing exclusion checks — `claude/` adds no `.env` files and no `/projects/`).

- [ ] **Step 5: Commit**

```bash
git add dist tests/dist.bats
git commit -m "feat(dist): ship the Claude skill + command in the zip

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Install the Claude assets from `install` (opt-in)

**Files:**
- Modify: `install`
- Test: `tests/install.bats` (add a helper + four tests)

**Interfaces:**
- Consumes: `claude/` (copied into `$INSTALL_DIR/claude` by the tool-copy loop).
- Produces: `~/.claude/skills/sdev/SKILL.md` and `~/.claude/commands/sdev-start.md` when opted in. Function name: `maybe_install_claude` (no args). Env knob: `SDEV_CLAUDE`.

- [ ] **Step 1: Write the failing tests**

Add a helper to `tests/install.bats` (after `run_install_prompt`, before the first `@test`):

```bash
# Non-interactive install with the Claude opt-in decided by SDEV_CLAUDE.
run_install_claude() {
  env -u WORKSPACE_ROOT HOME="$FAKEHOME" SHELL=/bin/zsh \
      SDEV_INSTALL="$INST" SDEV_HOME="$HOMEDIR" SDEV_BIN_DIR="$BINDIR" \
      SDEV_CLAUDE="$1" \
      bash "$REPO/install"
}
```

Add these tests at the end of `tests/install.bats`:

```bash
@test "claude: SDEV_CLAUDE=1 installs skill + command into ~/.claude" {
  run run_install_claude 1
  [ "$status" -eq 0 ]
  [ -f "$FAKEHOME/.claude/skills/sdev/SKILL.md" ]
  [ -f "$FAKEHOME/.claude/commands/sdev-start.md" ]
}

@test "claude: SDEV_CLAUDE=0 skips the Claude install" {
  run run_install_claude 0
  [ "$status" -eq 0 ]
  [ ! -e "$FAKEHOME/.claude/skills/sdev" ]
  [ ! -e "$FAKEHOME/.claude/commands/sdev-start.md" ]
}

@test "claude: non-interactive with no opt-in does not create ~/.claude" {
  run run_install
  [ "$status" -eq 0 ]
  [ ! -e "$FAKEHOME/.claude" ]
}

@test "claude: existing ~/.claude is not auto-populated without opt-in" {
  mkdir -p "$FAKEHOME/.claude"
  run run_install
  [ "$status" -eq 0 ]
  [ ! -e "$FAKEHOME/.claude/skills/sdev" ]
  [ ! -e "$FAKEHOME/.claude/commands/sdev-start.md" ]
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `bats tests/install.bats`
Expected: the two `SDEV_CLAUDE=1` / `=0` assertions FAIL (no install logic yet); the two "no opt-in" tests may pass incidentally — that's fine, they guard against regressions.

- [ ] **Step 3: Add `claude` to the tool-copy loop**

In `install`, in section `# --- 2. place the tool`, change the copy loop item list to include `claude`:

```bash
    for item in bin core README.md LICENSE VERSION confs install claude; do
```

- [ ] **Step 4: Add the `maybe_install_claude` function**

In `install`, after the `persist_rc()` function definition (after its closing `}`, before `# --- 1. dependency check`), add:

```bash
# Optionally install the sdev Claude Code skill + command into ~/.claude.
# SDEV_CLAUDE (1/true/yes/y = install, anything else = skip) wins; otherwise, on a
# real TTY with an existing ~/.claude, prompt. Never auto-installs uninvited, and
# only ever writes the two sdev-owned paths.
maybe_install_claude() {
    local src="$INSTALL_DIR/claude"
    [[ -d "$src" ]] || return 0
    local do_it=0
    if [[ -n "${SDEV_CLAUDE:-}" ]]; then
        case "$SDEV_CLAUDE" in 1|true|yes|y|Y) do_it=1 ;; *) do_it=0 ;; esac
    elif [[ -t 0 && -d "$HOME/.claude" ]]; then
        printf 'Install sdev Claude Code skill + command into ~/.claude? [Y/n]: ' >&2
        IFS= read -r reply || reply=""
        case "$reply" in [Nn]*) do_it=0 ;; *) do_it=1 ;; esac
    fi
    [[ $do_it -eq 1 ]] || return 0
    mkdir -p "$HOME/.claude/skills" "$HOME/.claude/commands"
    rm -rf "${HOME:?}/.claude/skills/sdev"
    cp -R "$src/skills/sdev" "$HOME/.claude/skills/sdev"
    cp "$src/commands/sdev-start.md" "$HOME/.claude/commands/sdev-start.md"
    say "✓ installed sdev Claude skill + command into ~/.claude (skills/sdev, commands/sdev-start.md)"
}
```

- [ ] **Step 5: Call the function near the end of `install`**

In `install`, after section `# --- 5. wire PATH + persist exports` (after its `fi`, before the final `cat <<EOF` block), add:

```bash
# --- 6. optional: Claude Code skill + command ------------------------------
maybe_install_claude
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `bats tests/install.bats`
Expected: PASS — all existing tests plus the four new Claude tests.

- [ ] **Step 7: Run the full suite**

Run: `bats tests/`
Expected: PASS across all files (no regressions).

- [ ] **Step 8: Commit**

```bash
git add install tests/install.bats
git commit -m "feat(install): opt-in install of Claude skill + command into ~/.claude

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 4: Document the integration in the README

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: behavior from Tasks 1–3.
- Produces: a "Claude Code integration" section.

- [ ] **Step 1: Add the section**

In `README.md`, insert a new top-level section immediately before `## Upgrading`:

```markdown
## Claude Code integration

sdev ships an optional [Claude Code](https://claude.com/claude-code) **skill** and
**slash-command**. When you run `./install` and a `~/.claude` directory is present,
the installer asks whether to add them:

- **`sdev` skill** (`~/.claude/skills/sdev/`) — teaches Claude the full sdev
  workflow (project setup and the task lifecycle), so it can drive the tool for you.
- **`/sdev-start <slug> [description]`** (`~/.claude/commands/sdev-start.md`) — takes
  a feature from zero to a running, isolated workspace: creates the task, brings the
  stack up, records the scope, and reports the URL.

Decline the prompt (or run with no `~/.claude`) and nothing is written. For scripted
installs, set `SDEV_CLAUDE=1` to install them or leave it unset to skip. They are
refreshed on each upgrade and live alongside your other Claude config — the
installer only writes the two `sdev`-owned paths.
```

- [ ] **Step 2: Verify the section renders and references are correct**

Run: `grep -n "Claude Code integration" README.md`
Expected: one match, positioned before the `## Upgrading` heading (`grep -n "## Upgrading" README.md` line number is greater).

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document the opt-in Claude Code skill + command

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- Skill (`sdev`, full lifecycle + setup) → Task 1 ✓
- `/sdev-start <slug> [description]` end-to-end command → Task 1 ✓
- Repo layout `claude/skills/sdev/SKILL.md`, `claude/commands/sdev-start.md` → Task 1 ✓
- dist packaging → Task 2 ✓
- install opt-in (detect / prompt / `SDEV_CLAUDE`), idempotent, upgrade-safe, copy-not-symlink → Task 3 ✓
- Tests (dist zip contents; install SDEV_CLAUDE=1/0, no-opt-in no-create, existing-claude-not-populated) → Tasks 2–3 ✓
- README "Claude Code integration" → Task 4 ✓
- Decisions (flat `/sdev-start`, copy not symlink, no uninstall, loose files) → honored throughout ✓

**Placeholder scan:** No TBD/TODO/"handle edge cases" — all file content and test code is complete and literal.

**Type/name consistency:** Function `maybe_install_claude` and env `SDEV_CLAUDE` used identically in Task 3 steps 4–5 and the test helper. Install paths (`~/.claude/skills/sdev/SKILL.md`, `~/.claude/commands/sdev-start.md`) match between skill content, dist test, install logic, install tests, and README. Repo source paths consistent across all tasks.

**Note on the interactive prompt:** bats pipes stdin, so `[[ -t 0 ]]` is false in tests — the `[Y/n]` prompt branch is not unit-tested. The `SDEV_CLAUDE` paths (which share the same copy code) are fully tested; the prompt branch is a thin y/n → same-copy mapping verified by inspection.
