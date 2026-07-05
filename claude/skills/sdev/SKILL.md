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
| `sdev ls` | list all tasks across projects (shows ephemeral/lease/lock state + warm pool) |
| `sdev end <slug> [--pool]` | tear down + archive (or return worktree to the warm pool) |
| `sdev new <slug> --ephemeral` | create a short-lived, auto-reclaimable task (never pooled) |
| `sdev destroy <slug> [--force]` | force-remove a task (worktree + offset + entry; no archive) |
| `sdev prune [--apply] [--pool]` | reclaim ephemeral/abandoned slots; `--pool` drains the warm pool |
| `sdev lease <slug> [holder]` | durably reserve a task (survives with no process) |
| `sdev release <slug>` | drop a task's lease + process-lock |
| `sdev doctor` | check deps + state-ledger integrity |

`sdev new` fetches `origin/<base>` and branches off the latest `origin/<base>`.
Pass `--no-fetch` to skip the fetch (offline, or a local repo with no remote).

To go from zero to a running workspace in one step, use the `/sdev-start` command.

## Running in parallel

Pin different projects in different terminals (`sdev use acme` here,
`sdev use beta` there). Port offsets are reserved from one lock-protected ledger
(`$SDEV_HOME/state/state.yml`) across every project, so multiple stacks run `up`
at once with no host-port collisions — even when several `sdev new` run
concurrently.

## Warm pool & leases

- `sdev end <slug> --pool` keeps a task's worktree warm (deps/build caches intact)
  instead of deleting it; the next `sdev new` for that repo reuses it, rebranded.
- `sdev lease <slug>` durably reserves a task so a background agent holding it
  across sessions is never auto-reclaimed; `sdev release` drops it. A dead
  process-lock self-heals. `sdev ls` shows `[leased:…]` / `[lock:…]` state.
- `sdev new <slug> --ephemeral` is the opposite of a lease: a short-lived,
  auto-reclaimable slot that is torn down fully on `end` (never pooled) and swept
  by `sdev prune`. `sdev prune` is a dry-run preview by default (`--apply` to
  perform) — it reclaims ephemeral + abandoned slots and, with `--pool`, drains
  the warm pool to free disk. It never touches a live-leased or live-locked task.
  `sdev destroy <slug>` force-removes one specific task (`--force` past a lease/lock).

## Gotchas

- **bash ≥ 4** required (macOS ships 3.2 → `brew install bash`).
- **yq v4** must be mikefarah's `yq`, not the Python `yq` (check `yq --version`).
- **staging** tasks require typing `staging` to confirm `sdev up` (pass `--yes`
  non-interactively) — they touch real staging data.
- **symlinked local repos:** don't move or delete the original — task worktrees
  branch off it and will break.
- **"port already allocated":** stop a conflicting task (`sdev down <slug>`);
  ports are listed by `sdev ls`.
