# sdev — `edit` command + "wider zip sharing" readiness

**Date:** 2026-06-15
**Status:** design (approved scope)
**Scope target:** make sdev robust enough to hand the zip to a wider audience — they can install, set up a project (existing clone or new repo), edit it, and run, without hitting walls. NOT full OSS infra (no CI / CONTRIBUTING / issue templates).

## Goal

Two things:
1. A first-class way to **add and remove repos** (and edit core fields) on an existing project — `sdev edit <project>`.
2. Close the gaps a stranger hits on the **unzip → install → init → new → up → open** journey.

## Out of scope

CI, contribution scaffolding, Homebrew/curl installer, docs site. (Deferred — release level chosen was "wider zip sharing".)

---

## Feature 1 — `sdev edit <project>` (interactive project editor)

Init-style interactive editor for an existing project. New script `bin/edit-project`, dispatched from `bin/sdev`.

### Dispatch (`bin/sdev`)
- Add `edit` to `is_reserved()` and `usage()`.
- `edit)` case: `exec env SDEV_PROJECT="$PROJECT" "$SCRIPT_DIR/edit-project" "$@"`.
- `sdev edit` (no arg) → active project via `resolve_project`. `sdev edit <name>` → that project. Project must exist (else: `die "project '<name>' not found — create it with: sdev init"`).

### UI
```
Project: acme  (core/projects.d/acme.yml)
  conf_prefix:    acme-api
  shell_service:  api
  stack_services: nginx, api, ui, db, redis
  repos:
    api   clone    base=main  role=api  -> core/acme/api
    web   symlink  base=main  role=ui   -> /Users/x/code/web

  [a] add repo     [r] remove repo
  [p] conf prefix  [s] shell service   [t] stack services
  [q] quit
>
```
- Each action persists **immediately** (matches `init`, which commits as it goes). `[q]` exits. No staged/discard mode — clones and worktree removals are filesystem ops that are unsafe to defer.
- After each action, redraw the summary.

### Actions
- **[a] add repo** — reuse `init`'s repo loop (see Feature 2). Prompts: name, git URL or local path, base branch, compose role. Clone (URL) or symlink (local path) into `core/<project>/<name>`; append the repo block to the YAML. Refuse if the repo name already exists in the project.
- **[r] remove repo** — pick by name (validate it exists in the YAML). Then:
  1. **Worktree check:** find live task worktrees at `projects/<project>/*/<name>`. If any exist, list them and refuse: `end those tasks first (sdev end <slug>), or type 'force' to remove the worktrees (branches kept)`. On `force`: for each, `git -C "$(repo_source_dir <project> <path>)" worktree remove --force <path>` — **never** `branch -D` (branches survive in the source repo).
  2. Remove the repo block from the YAML (`yq -i 'del(.repos.<name>)'`).
  3. **Source:** if `core/<project>/<name>` is a symlink → `rm` the symlink (never the external target). If it is a clone (dir) → keep by default; remove only if `--delete-source` flag was passed to `sdev edit`. Before deleting: if the clone has uncommitted or unpushed work, print what's at risk and require an interactive `yes` confirmation; if clean, delete after a simple confirm.
- **[p] conf prefix / [s] shell service** — prompt + validate (`valid_token`), `yq -i '.conf_prefix = "<v>"'` / `.default_shell_service`.
- **[t] stack services** — prompt for a comma/space list; validate tokens; `yq -i '.stack_services = [ ... ]'`. Empty input → `del(.stack_services)` (inherit global).

### Flags
`sdev edit [-p <project>] [<project>] [--delete-source]`. `--delete-source` only affects clone removal in `[r]`.

### YAML mutation strategy
Use `yq -i` (mikefarah v4) — already the established pattern (`new-task:148`). Accept that yq may lightly reformat; comments on untouched nodes are preserved. Document this.

---

## Feature 2 — shared repo-source helper (refactor)

`init` (lines 44-60) and `edit` both clone-or-symlink a repo into `core/<project>/<name>` and validate the source spec. Extract into `_lib.sh`:

```
add_repo_source <project> <name> <source_spec>
  # expands ~, classifies URL vs local-git vs invalid,
  # clones or symlinks into core/<project>/<name>,
  # echoes one of: cloned|linked, returns non-zero on failure.
```

`init` is refactored to call it; `edit`'s `[a]` calls it. One code path, tested once. No behavior change to `init`.

---

## Feature 3 — `git` dependency check (P0)

`install` and `bootstrap` verify bash≥4, yq v4, docker — but **not git**, which every project op needs. Add a git check to both, same shape as the existing ones:
```
if command -v git >/dev/null 2>&1; then say "✓ git"; else
  warn "✗ git not found (required for project setup and tasks)."; ok=0; fi
```

---

## Feature 4 — `stack_services` in `init` (#3)

`init` currently never asks about the stack, so every new project inherits the global `[nginx, api, ui, db, redis]` and the placeholder default template. Add to the `init` wizard (after shell service):
```
Stack services (host-exposed, comma-separated) [nginx,api,ui,db,redis]:
```
Write `stack_services: [...]` into the project YAML when the answer differs from the global default; omit to inherit. Validate tokens.

---

## Feature 5 — docs (#3 prose, #4, #5)

Edit `README.md`:
- **Modeling your stack** (#3): the default template is a generic db+redis+api+ui+nginx with *placeholder* images; explain `stack_services` trimming, giving a project its own `template:`, and the `compose_role` → service mapping. Set the expectation that "your repos need a docker setup matching the roles."
- **Troubleshooting** (#4): docker not running; "port already in use" (offsets); macOS bash 3.2 (`brew install bash`); wrong `yq` (must be mikefarah v4, not python-yq); "task not found in project"; the staging confirmation guard; `sdev edit`/`init` re-run safety.
- **Existing-local-repo caveats** (#5): symlinked sources need a real `origin` (because `sdev new` fetches `origin/<base>`; `--no-fetch` to skip); don't move/delete the original; worktrees branch off it.
- Document `sdev edit` in the daily-use table and the "Configure your first project" section.

---

## Feature 6 — P2 polish (#6)

- **Default base note:** README — `default_base` defaults to `main`; set `master`/`develop` per repo in `init`/`edit` as needed (already supported; just document).
- **Zip checksum:** `dist` writes `sdev-<version>.zip.sha256` next to the zip (`shasum -a 256`); README "install from zip" shows how to verify.
- **Platform notes:** README — Linux/WSL work (install detects `.bashrc`/`.profile`; `sdev open` prints the URL when no `open`); macOS needs `brew install bash`.

---

## Testing (bats)

New / extended:
- `tests/edit.bats`: add a repo (URL clone via a local bare fixture repo; and local-path symlink); remove a repo with no worktrees (YAML entry gone, symlink unlinked, clone kept); remove refused when a task worktree exists; remove `force` removes worktree but keeps branch; `--delete-source` removes clone; conf_prefix/shell_service/stack_services edits land in YAML.
- `tests/lib_*.bats`: cover the extracted `add_repo_source` (URL, local, invalid).
- `tests/install.bats` / a bootstrap test: assert git is now in the dep-check output.
- `tests/init.bats`: stack_services prompt writes the list / inherits on blank.
- `tests/dist.bats`: assert the `.sha256` file is produced.

Follow existing `tests/helpers.bash` fixture pattern (`make_fixture`, `make_source_repo`).

## Error handling

- `edit` on a missing project → actionable message pointing at `sdev init`.
- `add` duplicate repo name → refuse, no partial write.
- `remove` with live worktrees → refuse + list + `force` path; never silently delete branches or uncommitted work.
- `--delete-source` on a dirty/unpushed clone → print what's at risk, require interactive `yes`.
- All YAML writes via `yq -i` on a path that exists; verify with `yq` read-back after mutation, fail loudly if the file no longer parses.

## Backward compatibility

- Projects without `stack_services` keep inheriting the global default (unchanged).
- `init`'s external behavior is unchanged except the new stack prompt (which has a default = current behavior on Enter).
- No change to task layout, ports, or existing project YAMLs.
