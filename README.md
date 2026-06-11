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

## Configure your first project

```bash
sdev init
```

`sdev init` asks for a project name, your repos (git URL or local checkout), branches, and compose roles. It writes `~/.sdev/core/projects.d/<name>.yml`, clones/links the repos under `~/.sdev/core/<name>/`, seeds `~/.sdev/confs/<name>/<prefix>.local.env`, and prints the exact commands to bring a stack up.

To edit a project later, open its YAML in `~/.sdev/core/projects.d/`.

## Daily use

| Command | What it does |
|---|---|
| `sdev projects` | list all defined projects |
| `sdev use <project>` | pin the active project for this terminal |
| `sdev -p <project> <cmd>` | run one command against a project (overrides the pin) |
| `sdev new <slug>` | create a task (worktrees + stack) under the active project |
| `sdev up <slug>` | start the task's stack (`docker-compose up -d`) |
| `sdev ps / logs / shell <slug>` | status / tail logs / exec a shell in the shell service |
| `sdev open <slug>` | open the task's nginx URL in a browser |
| `sdev code / cd <slug>` | open the task dir in your editor / print its path |
| `sdev down / nuke <slug>` | stop (keep volumes) / stop + reclaim volumes |
| `sdev ls` | list all tasks across projects (the work-list dashboard) |
| `sdev end <slug>` | tear down + archive a finished task |

```bash
sdev use acme
sdev new login-fix          # -> projects/acme/login-fix, ports auto-assigned
sdev up login-fix
sdev open login-fix
```

## Running in parallel

Pin different projects in different terminals (`sdev use acme` here, `sdev use beta` there). Port offsets are allocated from a single global pool across every project, so multiple stacks can be `up` simultaneously with no host-port collisions.

## Upgrading

Unzip the newer `sdev-<version>.zip` and re-run `./install`. Your `~/.sdev` config, secrets, clones, and workspaces are preserved — only the tool code is replaced.

### Coming from an older in-repo layout?

If you previously ran sdev from a git clone (config under the clone itself), move it into `~/.sdev`:

```bash
sdev migrate --from /path/to/old/sdev-clone
```

> Note: end any open tasks before migrating — migrated live worktrees point at the old repo and may need to be recreated with `sdev new`.

## License

[MIT](./LICENSE).
