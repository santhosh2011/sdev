# sdev — Zippable Single-User Installer + `sdev init`

- **Date:** 2026-06-11
- **Status:** Approved (design); pending implementation plan
- **Author:** sathota

## Context

`sdev` is a bash CLI for running isolated, parallel docker-compose workspaces grouped by project. It works well for a single operator, but today the tool code, user config, secrets, repo clones, and live workspaces all live inside one directory: `WORKSPACE_ROOT` resolves to the cloned repo (parent of `bin/`).

The maintainer wants to distribute `sdev` to teammates by **handing out a zip**. Each teammate installs it and runs it **solo on their own machine** for **their own** projects. There is no multi-user mode, no shared/team config layer, and no package-manager or git-tap distribution.

The phrase "configure their development workflow" therefore means: a fresh user can easily set up `sdev` for their own repos and start a stack quickly.

### Problem with the current layout

Because `WORKSPACE_ROOT` conflates tool code with user data:

- A new zip (v2, v3, …) can't be dropped in without risking the user's project YAMLs, confs, clones, and workspaces, since they share one directory.
- Tool assets (templates, default config) and user data have no boundary.

## Goals

1. **Distribute by zip.** A `dist` script produces a clean `sdev-<VERSION>.zip` containing tool code only — no personal projects, secrets, or clones.
2. **Easy install on macOS + Linux.** An `install` script in the zip checks dependencies, places the tool, wires `PATH`, and creates the user-data home. Re-installing a newer zip never clobbers user data.
3. **Fast first run.** `sdev init` is an interactive wizard that writes the user's first real project and prints the exact commands to bring a stack up.

## Non-Goals (YAGNI)

- No demo/sample stack shipped.
- No shared or team-wide config layer; no config search-path precedence.
- No Homebrew tap, curl-pipe installer, or git-based distribution.
- No automatic dependency installation (print hints only).
- No non-interactive `sdev init` beyond what falls out naturally.

## Key Decisions

| Decision | Choice |
|---|---|
| Tool vs. user-data boundary | Split into `SDEV_INSTALL` (tool) and `SDEV_HOME` (user data, default `~/.sdev`) |
| Tool-asset resolution | Approach A — templates/defaults resolve from the install dir via `SCRIPT_DIR`; only user data resolves from `SDEV_HOME` |
| Distribution | Zip produced by a `dist` script |
| First-run UX | `sdev init` interactive wizard |
| Platforms | macOS + Linux |
| Install location (default) | tool → `~/.local/share/sdev`; `sdev` symlinked into `~/.local/bin` (fallback: print PATH line) |
| Upgrade model | Re-run `install` from a newer zip; `~/.sdev` is preserved |

## Architecture

### Path model

Replace the single `WORKSPACE_ROOT` with two clearly separated roots:

- **`SDEV_INSTALL`** — where the tool code lives. Resolved from `SCRIPT_DIR` (the existing symlink-following logic in `bin/sdev`). Env-overridable. Holds:
  - `bin/` (CLI + helpers)
  - `bin/templates/` (default compose/nginx/CLAUDE/settings templates)
  - packaged default config (`share/defaults/task-config.yml`) and `example.yml`
  - `README.md`, `LICENSE`, `VERSION`
- **`SDEV_HOME`** — user data root. Default `~/.sdev`, env-overridable. **Sub-paths are kept identical to today's in-repo layout** so only the *root* moves — this minimizes churn and keeps the existing test suite valid:
  ```
  ~/.sdev/
    core/.task-config.yml         # active defaults (seeded from install on first run, user-editable)
    core/.task-config.local.yml   # personal overrides (optional)
    core/projects.d/*.yml         # the user's project definitions
    core/<project>/<repo>/        # worktree source clones
    confs/<project>/*.env         # env profiles / secrets (gitignored / never packaged)
    projects/<project>/<slug>/    # live task workspaces (generated)
  ```

### Resolution model in `bin/_lib.sh`

The single `WORKSPACE_ROOT` concept splits into two roots, resolved authoritatively inside `_lib.sh` (the `bin/*` entry scripts stop setting `WORKSPACE_ROOT` themselves):

- `SDEV_INSTALL` = `$(dirname "${BASH_SOURCE[0]}")/..` — the tool dir. Holds `bin/`, `bin/templates/`, and the packaged default `core/.task-config.yml`.
- `SDEV_HOME` precedence:
  1. explicit `$SDEV_HOME` env var, else
  2. `$WORKSPACE_ROOT` if set in the env (**legacy alias** — used by the test fixtures and pre-migration installs; makes `SDEV_HOME == SDEV_INSTALL == WORKSPACE_ROOT`), else
  3. `$HOME/.sdev`.

| Path accessor | Root used |
|---|---|
| `GLOBAL_CONFIG = .../core/.task-config.yml` | `$SDEV_HOME` (seeded from `$SDEV_INSTALL/core/.task-config.yml`) |
| `LOCAL_CONFIG = .../core/.task-config.local.yml` | `$SDEV_HOME` |
| `PROJECTS_DIR = .../core/projects.d` | `$SDEV_HOME` |
| `config_template` default → `.../bin/templates/...` | `$SDEV_INSTALL` |
| `config_template` project override → `.../<v>` | `$SDEV_HOME` (custom templates are user data) |
| `profile_conf_file` → `.../confs/...` | `$SDEV_HOME` |
| `repo_source_dir` → `.../core/...` | `$SDEV_HOME` |
| `compute_next_offset` scans `.../projects` | `$SDEV_HOME` |
| `bin/sdev` / `end-task` / `list-tasks` task dirs `.../projects/...` | `$SDEV_HOME` |

Tool helper scripts (`new-task`, `end-task`, `list-tasks`) are invoked via `SCRIPT_DIR` by `bin/sdev`, so only `sdev` itself needs to be on `PATH`. Because the legacy alias makes the test fixtures (which export `WORKSPACE_ROOT` and copy `bin/` + `templates/` into it) resolve `SDEV_HOME == SDEV_INSTALL == WORKSPACE_ROOT`, **the existing bats suite passes unchanged**; new tests are added for the `SDEV_HOME`/`SDEV_INSTALL` split.

### Components

1. **`bin/_lib.sh`** — introduce `SDEV_INSTALL` / `SDEV_HOME`; update every path accessor per the table above. Add a one-time seed: if `$SDEV_HOME/.task-config.yml` is absent, copy the packaged default.

2. **`dist` script** (repo root) — builds `sdev-<VERSION>.zip`:
   - **Includes:** `bin/` (incl. `templates/`), `install`, the packaged default `core/.task-config.yml`, `core/projects.d/example.yml`, `confs/example/*.example`, `README.md`, `LICENSE`, `VERSION`.
   - **Excludes:** the maintainer's own `core/projects.d/*.yml` (everything except `example.yml`), `core/.task-config.local.yml`, `**/*.env` (keep `*.env.example`), `core/<project>/` clones, `projects/`, `.planning/`, `.git`, `tests/` (exclude to keep the zip lean).
   - Version comes from a `VERSION` file at repo root.
   - The committed `core/.task-config.yml` *is* the shipped default (the maintainer keeps personal tweaks in `.task-config.local.yml`, which is excluded), so it ships pristine.

3. **`install` script** (shipped in the zip; macOS + Linux):
   - Detect OS.
   - Dependency check: `bash ≥ 4`, `yq` v4, `docker` + compose. On any missing, print OS-specific install hints (`brew …` on macOS; `apt`/`dnf`/manual on Linux). Do **not** auto-install.
   - Copy the unpacked tool into `${SDEV_INSTALL:-$HOME/.local/share/sdev}` (replace existing tool dir).
   - Symlink `bin/sdev` into `~/.local/bin` if that dir is on `PATH`; otherwise print the `export PATH=…` line to add.
   - Create the `~/.sdev` skeleton (`core/projects.d/`, `confs/`, `projects/_archive/`) and seed `core/.task-config.yml` from `$SDEV_INSTALL/core/.task-config.yml` if absent.
   - **Idempotent:** safe to re-run; replaces the tool dir, never touches existing `~/.sdev` data.
   - Final message points the user at `sdev init`.

4. **`sdev init` wizard** (new subcommand in `bin/sdev`, likely delegating to a `bin/init` helper):
   - Prompt: project name (validated like a slug/identifier).
   - Loop prompt repos: repo name, git URL **or** local path, default base branch, compose role. Git URLs are cloned into `~/.sdev/core/<project>/<path>`; local paths are registered as the worktree source.
   - Prompt: `conf_prefix`, shell service, stack services (offer the defaults from `core/.task-config.yml`).
   - Write `~/.sdev/core/projects.d/<name>.yml`; create `~/.sdev/confs/<name>/<prefix>.local.env` from the example.
   - Run `sdev use <name>`.
   - Print the exact next commands: `sdev new <task> && sdev up <task> && sdev open <task>`.

5. **`sdev migrate`** (new subcommand) — moves an existing in-repo layout into `~/.sdev`. Because sub-paths are identical relative to the root, migration is a structure-preserving move:
   - `sdev migrate --from DIR` (the old clone/repo dir); copies/moves `core/projects.d/*` (excluding `example.yml`), `core/.task-config.yml`, `core/.task-config.local.yml`, `confs/`, `core/<project>/` clones, and `projects/` into `$SDEV_HOME`, preserving relative structure.
   - Refuses to overwrite an already-populated `$SDEV_HOME` unless `--force`.
   - Prints a summary of what moved.
   - **Backward-compat fallback:** the `WORKSPACE_ROOT` legacy alias (resolution rule 2 above) means a maintainer running from their existing clone keeps working with zero migration — `SDEV_HOME` resolves to the clone. Migration is only needed to adopt the `~/.sdev` location.

6. **Cross-platform** — `open` resolves to `open` (macOS) / `xdg-open` (Linux) / print-URL fallback (the print fallback already exists in `bin/sdev`). `bash ≥ 4` and `yq` enforced by the installer.

### Data flow

```
dist (maintainer machine)
  repo  ──filter──▶  sdev-<VERSION>.zip   (tool only)

install (teammate machine)
  zip  ──▶  $SDEV_INSTALL (~/.local/share/sdev)   + symlink ~/.local/bin/sdev
       └─▶  $SDEV_HOME (~/.sdev) skeleton + seeded core/.task-config.yml

sdev init
  prompts  ──▶  ~/.sdev/core/projects.d/<name>.yml
           ──▶  ~/.sdev/core/<name>/<repo> (clone)   ──▶  ~/.sdev/confs/<name>/<prefix>.local.env

sdev new / up / open  ──▶  ~/.sdev/projects/<name>/<slug>/  (worktrees + compose stack)
```

## Testing

Extend the existing `bats` suite (`tests/`):

- Update `tests/helpers.bash` to set `SDEV_HOME` (and `SDEV_INSTALL`) to temp dirs so path resolution is exercised under the new model; keep the existing suite green.
- `dist`: produced zip excludes personal `projects.d`, `**/*.env`, clones, and `projects/`.
- `install`: creates the `~/.sdev` skeleton; seeds default config; is idempotent; preserves existing `~/.sdev` on re-run.
- `sdev init`: writes a valid project YAML and matching conf from scripted input.
- Path resolution: `SDEV_HOME` / `SDEV_INSTALL` honored; legacy in-repo fallback works and warns.

## Documentation

Update `README.md`:

- Install-from-zip section (download, `./install`, `sdev init`).
- `sdev init` quickstart.
- Upgrade story: re-run `install` from a newer zip; `~/.sdev` config is preserved.
- Dependency/platform notes for macOS + Linux.

## Risks / Notes

- The `WORKSPACE_ROOT` → `SDEV_INSTALL`/`SDEV_HOME` split touches most path accessors in `_lib.sh`; the legacy fallback limits blast radius for one release.
- Git-clone-during-`init` can be slow or fail (auth, network); the wizard must handle failures without leaving a half-written project YAML.
- `VERSION` management is manual (a file at repo root); acceptable for zip distribution.
