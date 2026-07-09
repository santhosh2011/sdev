# Project agent memory

This file is the project's committed home for project-intrinsic agent knowledge: build, test, release, architecture, and sharp-edge notes that should travel with the code.

## What sdev is

A bash CLI that runs many isolated, parallel docker-compose workspaces grouped by **project**. Layout invariant:

- Repo sources: `core/<project>/<repo>/` (a clone or a symlink to a local checkout).
- Task worktrees: `projects/<project>/<slug>/<repo_path>/` on branch `task/<slug>`.
- Archives: `projects/_archive/<project>/<slug>/`. Legacy flat tasks live at `projects/<slug>/`.
- Tool code vs data: `$SDEV_INSTALL` (parent of `bin/`) is the code; `$SDEV_HOME` (default `~/.sdev`) is user data. In the bats fixture `WORKSPACE_ROOT == SDEV_HOME == SDEV_INSTALL`.

## Assumptions (do not break)

- **bash ≥ 4** and **mikefarah `yq` v4** (NOT the Python yq). All YAML/JSON edits go through `yq`.
- **No `flock(1)`** — macOS lacks it. Portable locking only (see below).
- Portable across macOS + Linux: prefer POSIX-ish constructs; `date -u +%Y-%m-%dT%H:%M:%SZ` for timestamps, `ps -o lstart= -p <pid>` for a process start-time signature, `stat -c %Y || stat -f %m` for mtime.

## Central state ledger — `$SDEV_HOME/state/state.yml`

Single source of truth for three things, all defined in `bin/_lib.sh`:

- **`tasks`**: `"<project>/<slug>" -> {offset, created_at, lease, lease_holder, pid, proc_token, ephemeral}`. Port offsets are RESERVED here **under the lock** by `allocate_offset` — this closes the old `compute_next_offset` scan race (two concurrent `sdev new` both scanned a free offset before either wrote its `.env`). `used = ledger offsets ∪ a fresh .env scan` (belt-and-suspenders). First use seeds the ledger from existing task `.env` `PORT_OFFSET`s (`state_seed_from_env`, idempotent via `.seeded`). All entry-creation sites write every field including `ephemeral`; readers use `// false` so pre-`ephemeral` ledgers stay valid.
- **`pool`** + **`pool_seq`**: the warm worktree pool (see below).
- Reservation model: an offset is reclaimable only when its workspace is gone **and** it is not leased **and** it has no live process-lock (`state_reconcile`, run at every allocation — this is the self-heal).

### Locking

`with_state_lock CMD ...` wraps every read-modify-write. It is an **atomic symlink** at `$SDEV_HOME/state/lock` whose **target is the holder pid** (`ln -s "$$"`; Go's `internal/lock` does `os.Symlink(pid)`). Sharp edges baked into the impl (keep them):

- The symlink **carries the holder pid atomically** — creating it is a single syscall that fails if the path exists, so there is **no pid-less window**. This replaced an older `mkdir` lock-dir + separate pid file whose truncate-then-write window let a spinning waiter read an empty pid and wrongly break a live lock (a real duplicate-offset flake). Do not regress to a create-then-write-pid scheme.
- The EXIT trap is armed **before** entering the critical section so a `die()`/crash still releases (Go uses `defer`).
- **Stale-break must double-read** before removing a lock: a genuine handoff (holder releases, a new holder acquires) shows a *different* target on the second read, so only the **same dead pid read twice** is truly abandoned. It then atomically **claims** (rename to a unique grave) and **verifies** the claimed target is unchanged before discarding — else it restores it (via `ln -s`/`os.Symlink`, which fail rather than clobber a newer holder). A single read-then-break has a **check-vs-reacquire TOCTOU**: a waiter reads a since-exited holder's pid, judges it dead, and removes a lock a new holder just acquired — two processes in the critical section → duplicate offsets. Latent in pure bash (target is a shared, always-live `$$`) and pure Go, it surfaces when they **mix** (Go's per-process pids die and get re-checked). `tests/state_interop.bats` gates it with an `O_EXCL` double-hold self-check (`SDEV_LOCK_TRACE`).
- **bash ↔ Go interop:** `bin/_lib.sh` and `internal/lock` implement the *same* symlink protocol so both contend on one lock; a change to one (target format, break rule) must change the other. The `state.yml` schema, `_state_key_from_env`, and offset allocation likewise mirror across the two.
- Safe inside `$(...)` — the subshell's EXIT trap releases when the substitution ends. `$$` is the parent's pid inside a subshell; fine, it's only used for stale detection (and is always alive, which is *why* pure-bash never trips the break TOCTOU).
- The acquire loop uses **ramped backoff** (0.01s → 0.05s → 0.2s) under a **wall-clock deadline** (`_STATE_LOCK_BUSY_SECS`, 120s), NOT a fixed-interval spin count. Backoff is load-bearing: under heavy contention many waiters busy-polling on a tight interval starve the holder's `yq` and collapse throughput — the old fixed 0.05s poll + 600-try (~30s) ceiling made waiters time out and emit an **empty** offset (not a duplicate) on slow/2-core CI runners (`tests/state_lock.bats` "many concurrent offset allocations"). Keep the deadline wall-clock so it means the same regardless of per-poll speed.

### Warm worktree pool

`sdev end --pool` returns each repo worktree instead of deleting it: `git reset --hard` + `git clean -fd` (**NOT** `-fdx` — that keeps gitignored deps/build caches like `node_modules`, `build/`, `.venv`), then `git checkout --detach`, then `git worktree move` into `state/pool/<project>/<repo>.<seq>`. `sdev new` calls `pool_take <source>` and, on a hit, `git worktree move` back + `git checkout --no-track -B task/<slug> <start_point>` to re-brand (tracked files reset, ignored caches preserved). Any hiccup falls back to a fresh `git worktree add`. `--no-pool` forces fresh. The offset is freed on `end` either way (`free_task`).

### Leases, process-locks & ephemeral

- Lease: durable reservation, no live process (`sdev lease`/`release`, or `sdev new --lease`). Never auto-reclaimed. Shown in `sdev ls`, including leases whose workspace is gone.
- Process-lock: `sdev hold` records pid + `proc_token` (start-time). `_proc_alive` treats a reused pid (mismatched start-time) as dead → self-heals.
- Ephemeral (`sdev new --ephemeral`, `ephemeral: true`): a durable-lease-free, short-lived slot eligible for automatic reclamation. Mutually exclusive with `--lease` (enforced in `new-task`). On `end` it is torn down fully and `POOL` is forced to 0 (`task_is_ephemeral` check) — it never returns to the warm pool.
- `end` is an explicit teardown and frees everything (lease/lock included); leases only protect against *automatic* reclaim, not `end`.

### Reclamation — `sdev prune` & `sdev destroy`

- `force_teardown_task KEY` (in `_lib.sh`) is the shared force-teardown: docker down, `git worktree remove --force` per repo, `branch -D task/<slug>`, `rm -rf` the dir, then locked `free_task`. No archive, no pre-flight. It derives `project`/`slug` from the key (legacy flat keys → project `default`). The **only** shared-state write is the locked `free_task`.
- `bin/prune` (dispatched by `sdev prune`) classifies ledger tasks: live lease / live lock → **protected**; ephemeral (and not live-locked) → **full teardown**; workspace-gone + not-leased + dead/no-lock → **drop entry** (self-heal). Also drops stale pool entries, and with `--pool` **drains** the pool via `drain_pool_entry` (`git worktree remove` through each entry's recorded `source`, else `rm -rf`, then locked `pool_drop`). **Dry-run by default**; `--apply`/`-y` performs it. `--pool-only` skips task reclaim; `--project` scopes.
- `sdev destroy <slug>` (inline in `bin/sdev`) resolves a key from an on-disk workspace or a bare ledger entry (so a workspace-less lease is destroyable), refuses a live lease/lock without `--force`, then calls `force_teardown_task`.
- Prune/destroy never reclaim a live-leased or live-locked task (an ephemeral task you `sdev hold` is protected too). Tests: `tests/ephemeral.bats`.

## Testing

- `bats tests/` — all tests use `tests/helpers.bash::make_fixture` (isolated `WORKSPACE_ROOT`). `make_source_repo` builds a source git repo. New bin scripts must be added to the `cp` list in `make_fixture` (that's how `doctor` got shipped into the fixture).
- Concurrency is tested both end-to-end (two real `sdev new`) and by stressing `allocate_offset` in parallel subshells.
- `bin/dist` copies all of `bin/` into the zip; a new command needs no dist change, but `tests/dist.bats` asserts specific paths — check it if you add shipped assets.
- `sdev doctor` checks deps + ledger integrity (offset drift, duplicates, stale lock, orphaned pool entries); exits non-zero on FAIL, zero on WARN.
- `tests/self_update.bats` exercises the whole distribution layer **offline** via `SDEV_DIST_ZIP`: it builds versioned zips with `./dist` + a re-stamped `VERSION`, then drives `install.sh` and `self-update` against isolated scratch dirs (mirrors `install.bats`). No network / published release needed. `install.sh` / `self-update` are top-level scripts referenced by `$REPO` path in tests (like `install`/`dist`), not copied into the `make_fixture` bin dir.

## Distribution — CI, release, install & self-update

- **CI** (`.github/workflows/ci.yml`): a `shellcheck -x -S warning` job (keep its script list in sync when you add/rename shell scripts) and a **bats matrix over `ubuntu-latest` + `macos-latest`** (`fail-fast: false`). Linux installs `bats` + mikefarah `yq` v4; macOS uses `brew install bats-core yq bash` (macOS ships bash 3.2, tool needs ≥ 4). CI verifies yq is mikefarah v4 before running `bats tests`.
- **Release** (`.github/workflows/release-please.yml` + `release-please-config.json` + `.release-please-manifest.json`): release-please `release-type: simple` opens/maintains a release PR from conventional commits; on merge it tags + creates the GitHub release, then the `publish` job runs `./dist build` and uploads `sdev-<ver>.zip` + `.sha256` as release assets. The version lives in `.release-please-manifest.json` and is mirrored into `VERSION`.
- **`VERSION` annotation**: the repo file is `1.0.0 # x-release-please-version` — release-please updates the version at that marker. Anything reading it must strip the comment: `sed -E 's/[[:space:]]*#.*$//' | tr -d '[:space:]'`. `dist` ships a **clean bare** `VERSION` (no comment) to end users and names the zip `sdev-<bare>.zip`; `self-update`'s `read_version` strips it too. Only `dist`/`self-update` read `VERSION` today.
- **`install.sh`** (network installer / self-update engine): fetches the latest (or `SDEV_VERSION`) release zip from `SDEV_REPO` (default `santhosh2011/sdev`), **verifies SHA-256 (hard-fails on mismatch)**, unpacks, and hands off to the bundled `./install`. Must stay **bash-3.2 compatible** — `curl … | bash` runs under macOS system bash; it only *finds* a bash ≥ 4 (`find_bash4`) to run the real installer. `SDEV_DIST_ZIP` skips the fetch (offline / tests). Also gates platform (macOS/Linux only) and yq (mikefarah v4).
- **`self-update`** (`sdev update`, `sdev-update`, or direct): resolves the install dir (`SDEV_INSTALL`, else the dir it ships in, else `~/.local/share/sdev`), reuses `install.sh` as the download/place engine (fetching it from the repo as a last resort), runs it non-interactively (`</dev/null`), and reports `old → new`. Preserves `$SDEV_HOME`.
- **`dist`** ships `bin/`, `install`, `install.sh`, `self-update` (all +x), `README`/`LICENSE`, a clean `VERSION`, `core/` defaults + example, `confs/*.example` only, and `claude/`. **`install`** copies `install.sh`+`self-update` into the tool dir and links `$SDEV_BIN_DIR/sdev-update -> self-update` alongside `sdev`. `bin/sdev update` execs `$SCRIPT_DIR/../self-update`.
- **Install-time deps**: `install` (and legacy `bootstrap --check`) hard-require bash ≥ 4, mikefarah `yq` v4, and **git** (worktrees); **docker is runtime-only** (needed for `sdev up`) so it's a *warning*, not a fatal check in either. This is deliberate — GitHub's `macos-latest` runners have no Docker, so a fatal docker gate breaks the bats matrix (`install.bats`, `self_update.bats` run the real installer; `bootstrap.bats` runs `--check`). Covered by the "install treats docker as runtime-only" test.

- Add durable project-specific notes here as they are discovered through real work.
