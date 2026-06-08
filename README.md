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

## Install

```bash
git clone <your-fork-url> ~/sdev
cd ~/sdev
./bootstrap                          # checks deps, scaffolds dirs, prints the PATH line
export PATH="$HOME/sdev/bin:$PATH"   # add this to your ~/.zshrc or ~/.bashrc
```

`./bootstrap` is safe to re-run. Use `./bootstrap --check` to verify dependencies only, or `./bootstrap --path` to re-print the PATH line.

## Define a project

```bash
cp core/projects.d/example.yml core/projects.d/acme.yml   # then edit
mkdir -p core/acme && git -C core/acme clone <repo> my-api-repo   # worktree source(s)
mkdir -p confs/acme && cp confs/example/app.local.env.example confs/acme/app.local.env
```

`acme.yml` declares the repos (each becomes a worktree in every task), the `conf_prefix`, the shell service, and optionally a project-specific compose `template`. Repo worktree sources live under `core/<project>/<path>`.

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

## License

[MIT](./LICENSE).
