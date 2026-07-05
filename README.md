# sdev

A small CLI for running many **isolated, parallel docker-compose workspaces** — grouped by **project**. Each task gets its own git worktrees, its own env profile, and its own stack on a unique set of host ports, so you can develop several features (and several projects) at once without anything colliding.

## Concepts

- **Project** — a registry at `core/projects.d/<name>.yml` describing a product: which repos it uses, its env-profile conf prefix, its docker stack. Projects are independent and can run in parallel. With no project files, an implicit `default` project uses the fallback registry in `core/.task-config.yml`.
- **Task** — an isolated workspace at `projects/<project>/<slug>/`: a git worktree (branch `task/<slug>`) of each of the project's repos, plus a generated `docker-compose.yml` and `.env` with a unique port offset.
- **Profile** — an environment (`local` / `dev` / `staging`) whose runtime config comes from `confs/<project>/<conf_prefix>.<profile>.env`, symlinked into each task as `app.env`. `staging` is guarded behind an explicit confirmation.

## Requirements

- **bash ≥ 4** (macOS ships 3.2 — `brew install bash`)
- **[yq](https://github.com/mikefarah/yq) v4** (`brew install yq`)
- **docker** + compose (Docker Desktop or OrbStack)

> **Platforms:** macOS and Linux (incl. WSL). The installer detects your shell rc
> (`.zshrc`/`.bashrc`/`.profile`). `sdev open` launches a browser on macOS and
> prints the URL elsewhere. Repo base branches default to `main` — set
> `master`/`develop` per repo in `sdev init`/`sdev edit`.

## Install

**One-line install (macOS / Linux):**

```bash
curl -fsSL https://raw.githubusercontent.com/santhosh2011/sdev/main/install.sh | bash
```

This downloads the latest published release, verifies its SHA-256, unpacks it, and hands off to the bundled installer (which places the tool, wires your shell, and preserves any existing data). Knobs (all optional): `SDEV_VERSION=v1.2.3` to pin a release, `SDEV_REPO=owner/repo` to install from a fork, `SDEV_HOME=/path` to set the project home non-interactively.

**Requirements** (the installer checks these): bash ≥ 4 (`brew install bash` on macOS — it ships 3.2), [mikefarah `yq`](https://github.com/mikefarah/yq) v4 (**not** the Python `yq`), and docker + compose (Docker Desktop or OrbStack).

### From the zip

You'll receive a `sdev-<version>.zip`. Then:

```bash
unzip sdev-<version>.zip
cd sdev
./install        # checks deps, installs the tool, asks where your project home lives, links `sdev`
```

To verify the download before installing:

```bash
shasum -a 256 -c sdev-<version>.zip.sha256
```

When run interactively, `./install` **prompts for your project home** (where projects, configs, and repos live; default `~/.sdev`), validates the path, then persists `export SDEV_HOME` and `PATH` into your shell rc inside an idempotent `# >>> sdev >>>` block. Set `SDEV_HOME` in the environment beforehand to skip the prompt (CI / scripted installs) — in that case the installer leaves your rc untouched and just prints the `PATH` line if needed.

`./install` is idempotent and never touches your data under `$SDEV_HOME`.

### Where things live

- **Tool code:** `~/.local/share/sdev` (replaceable; overwritten on upgrade).
- **Your data — `$SDEV_HOME`, default `~/.sdev`:** project definitions (`core/projects.d/`), env profiles/secrets (`confs/`), repo clones (`core/<project>/`), live workspaces (`projects/`), and the allocation ledger + warm pool (`state/`). Survives upgrades.

## Configure your first project

```bash
sdev init
```

`sdev init` asks for a project name, your repos (git URL or local checkout), branches, and compose roles. It writes `~/.sdev/core/projects.d/<name>.yml`, clones/links the repos under `~/.sdev/core/<name>/`, seeds `~/.sdev/confs/<name>/<prefix>.local.env`, and prints the exact commands to bring a stack up.

To change a project later — add or remove a repo, or edit its conf prefix, shell service, or stack — run `sdev edit <project>` (an interactive menu). Removing a repo that still has live task worktrees is refused unless you confirm; cloned sources are kept unless you pass `--delete-source` (symlinked local repos are only unlinked, never deleted).

### Pointing at an existing local clone

When you give `sdev init`/`sdev edit` a **path** (not a URL), sdev *symlinks*
that repo as the worktree source — it is not copied. Notes:

- `sdev new` fetches `origin/<base>` before branching, so the local repo needs a
  real `origin`. Use `sdev new <slug> --no-fetch` to skip the fetch (offline, or
  no remote).
- Don't move or delete the original — task worktrees branch off it and will break.
- `sdev edit` → remove only *unlinks* the symlink; your repo is untouched.

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

## Daily use

| Command | What it does |
|---|---|
| `sdev projects` | list all defined projects |
| `sdev use <project>` | pin the active project for this terminal |
| `sdev -p <project> <cmd>` | run one command against a project (overrides the pin) |
| `sdev edit [<project>]` | add/remove repos, edit conf prefix / shell service / stack services |
| `sdev new <slug>` | create a task (worktrees + stack) under the active project |
| `sdev up <slug>` | start the task's stack (`docker-compose up -d`) |
| `sdev ps / logs / shell <slug>` | status / tail logs / exec a shell in the shell service |
| `sdev open <slug>` | open the task's nginx URL in a browser |
| `sdev code / cd <slug>` | open the task dir in your editor / print its path |
| `sdev down / nuke <slug>` | stop (keep volumes) / stop + reclaim volumes |
| `sdev ls` | list all tasks across projects (the work-list dashboard) |
| `sdev end <slug> [--pool]` | tear down + archive (or return the worktree to the warm pool) |
| `sdev new <slug> --ephemeral` | create a short-lived, auto-reclaimable task (never pooled) |
| `sdev destroy <slug> [--force]` | force-remove a task: worktree + offset + entry, no archive |
| `sdev prune [--apply] [--pool]` | reclaim ephemeral/abandoned slots; `--pool` drains the warm pool |
| `sdev lease <slug> [holder]` | durably reserve a task (survives with no live process) |
| `sdev release <slug>` | drop a task's lease + process-lock |
| `sdev hold <slug>` | attach a self-healing process-lock (this shell) |
| `sdev doctor` | check deps + state-ledger integrity |

```bash
sdev use acme
sdev new login-fix          # -> projects/acme/login-fix, ports auto-assigned
sdev up login-fix
sdev open login-fix
```

`sdev new` fetches each repo from `origin` and starts the task branch off the **latest `origin/<base>`** (the per-repo `default_base`, e.g. `develop`) — so a new task always builds on the current integration branch, even if your local clone is behind. Pass `--no-fetch` to skip the fetch (offline / speed) and use whatever the source repo already has. If the fetch fails, it warns and falls back to the local base.

## Running in parallel

Pin different projects in different terminals (`sdev use acme` here, `sdev use beta` there). Port offsets are allocated from a single global pool across every project, so multiple stacks can be `up` simultaneously with no host-port collisions.

Allocation is **lock-protected**: every `sdev new` reserves its port offset from a central ledger under a portable lock, so two `sdev new` running at the same time (e.g. several agents) can never be handed the same offset. (Before this, both scanned for a free offset before either wrote its `.env` and collided on the second `sdev up`.)

## Central state, the warm pool & leases

sdev keeps one lock-protected ledger at **`$SDEV_HOME/state/state.yml`** — the single source of truth for port-offset allocation, the warm worktree pool, and per-task lease/lock state. All reads-and-writes of shared state go through a portable `mkdir(2)` lock (no `flock(1)`, which macOS lacks). On first use the ledger is **seeded from your existing tasks' `.env` `PORT_OFFSET`s**, so it never hands out an offset already in use.

**Warm pool.** `sdev end --pool` returns a task's git worktrees to a pool under `$SDEV_HOME/state/pool/` instead of deleting them: each tree is reset clean but its **gitignored deps / build caches** (`node_modules`, `build/`, `.venv`, `target/`, …) are kept. The next `sdev new` for the same repo **reuses** a pooled worktree — re-branding it to `task/<slug>` at the fresh base — so you skip a full checkout and re-install. An empty pool (or `--no-pool`) falls back to a fresh worktree.

```bash
sdev end login-fix --pool     # keep the worktree warm instead of deleting it
sdev new next-feature         # reuses it: deps/caches intact, rebranded to task/next-feature
```

**Leases & process-locks.** A task's port offset is only reclaimed when its workspace is gone *and* nothing holds it. Two things hold it:

- **Lease** — a *durable* reservation with no live process, for a background agent keeping a task across sessions or reboots. `sdev lease <slug> [holder]` sets it; a leased task is never auto-reclaimed until `sdev release <slug>`. Leases (even with no workspace) show under `sdev ls`.
- **Process-lock** — `sdev hold <slug>` pins a task to a live process (pid + start-time). It **self-heals**: once that process is gone (or its pid is reused, caught via the start-time), the lock reads as stale and the offset becomes reclaimable again.

`sdev ls` annotates each task with its state — `[ephemeral]`, `[leased:holder]`, `[lock:pid]`, or `[lock:stale]` (markers combine, e.g. `[ephemeral lock:1234]`) — and lists the warm pool. Run `sdev doctor` to check dependencies and ledger integrity (offset drift, duplicates, stale locks, orphaned pool entries).

## Ephemeral tasks & pruning

**Ephemeral tasks.** `sdev new <slug> --ephemeral` creates a *durable-lease-free, short-lived* slot that is **eligible for automatic reclamation**. It is the opposite of a leased task: it can never be leased (the two flags are mutually exclusive), it is **torn down fully on `sdev end`** — the worktree is removed and the offset freed, and it is **never returned to the warm pool** (no cached deps kept), and `sdev prune` will sweep it. Use it for throwaway checks and short agent runs you don't want to remember to clean up.

**`sdev prune`** is the safe, automatic-eligible sweep. By default it is a **dry-run** that previews what it would reclaim; pass `--apply` (or `-y`) to perform it:

- **ephemeral tasks** — reclaimed fully (worktree + offset + ledger entry).
- **abandoned ledger entries** — an offset reserved for a task whose workspace is gone and that is neither leased nor live-locked; the entry is dropped and the offset freed (the self-heal, no data to lose).
- **stale warm-pool entries** — pool records whose worktree vanished on disk.

`sdev prune --pool` *additionally* **drains the warm pool**: it removes every cached worktree to free disk (draining touches only pooled worktrees, never a live task). `--pool-only` drains the pool and leaves task reservations alone. `--project <name>` scopes the sweep. Prune **never** reclaims a task holding a live lease or a live process-lock — including an ephemeral one you have `sdev hold`-ed.

**`sdev destroy <slug>`** is the targeted nuke for a single task: it force-removes the worktree, deletes the `task/<slug>` branches, stops the stack, and frees the offset + ledger entry — **no archive, no pre-flight**. It refuses a task with a live lease or live process-lock unless you pass `--force`.

```bash
sdev new probe --ephemeral   # throwaway slot; auto-reclaimable, never pooled
sdev prune                   # preview: what would be reclaimed?
sdev prune --apply           # reclaim ephemeral + abandoned slots + stale pool entries
sdev prune --pool --apply    # …and drain the warm pool to reclaim disk
sdev destroy wedged-task -f  # force-remove one specific task, lease/lock and all
```

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

### Hooks

sdev also ships three Claude Code hooks that make an agent safer and better
oriented inside a task workspace:

- **session-context** (SessionStart) — injects the task identity (project, slug,
  branch, URL, ports) when Claude opens inside a task dir.
- **staging-guard** (PreToolUse) — refuses an agent-issued `sdev up` against a
  `staging` profile unless you pass `--yes`.
- **edit-reminder** (PostToolUse) — notes that edits need `sdev up <slug>` to take
  effect.

The hooks are wired **per task by default**: `sdev new` writes them into the
task's `.claude/settings.local.json`, so they fire only inside that workspace. A
project opts out with `hooks: false` in its `core/projects.d/<project>.yml`.
Already-created tasks are unaffected — recreate a task to adopt them.

When you opt into the Claude integration at install time, the **staging-guard** is
additionally merged into your global `~/.claude/settings.json` (idempotent,
preserving your other hooks) so `sdev up` against staging is guarded even when run
outside a task dir.

## Upgrading

Update in place to the latest release:

```bash
sdev update        # or the standalone alias: sdev-update
```

This fetches the latest published release, verifies its SHA-256, and reinstalls the tool code. Your `~/.sdev` config, secrets, clones, and workspaces are preserved — only the tool code is replaced. Pin a specific release with `SDEV_VERSION=v1.2.3 sdev update`. (Equivalently, unzip a newer `sdev-<version>.zip` and re-run `./install`.)

### Coming from an older in-repo layout?

If you previously ran sdev from a git clone (config under the clone itself), move it into `~/.sdev`:

```bash
sdev migrate --from /path/to/old/sdev-clone
```

> Note: end any open tasks before migrating — migrated live worktrees point at the old repo and may need to be recreated with `sdev new`.

## Troubleshooting

- **`docker: command not found` / daemon errors on `sdev up`:** start Docker
  Desktop or OrbStack; sdev shells out to `docker compose`/`docker-compose`.
- **"port is already allocated":** another task or app holds the port. Each task
  gets a unique offset (reserved under the state lock), so the culprit is usually
  an unrelated process — stop it, or stop a conflicting task (`sdev down <slug>`).
  Ports are listed by `sdev ls`; run `sdev doctor` to check the ledger.
- **"state lock busy":** a previous `sdev` was killed mid-write and left
  `$SDEV_HOME/state/lock`. sdev self-heals a lock whose owner process is dead;
  if it persists, remove it manually (`rm -rf "$SDEV_HOME/state/lock"`) once no
  `sdev` is running. `sdev doctor` reports a stale lock.
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

## License

[MIT](./LICENSE).
