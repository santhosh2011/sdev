# Design — sdev Claude Code skill + command (installer-bundled)

**Date:** 2026-06-22
**Status:** Approved (design)

## Summary

Ship a Claude Code **Skill** and a **slash-command** alongside sdev, so an agent
running in a user's terminal knows how to drive sdev (set up a project, run the
isolated/parallel docker-compose workspace lifecycle) and can take a feature from
zero to a running workspace in one command.

Both artifacts live in the repo, ride along in the distributed zip, and are
installed into the user's Claude config (`~/.claude`) by `./install` — on an
opt-in basis, only when `~/.claude` is detected.

## Goals

- A `sdev` Skill that covers the **whole tool**: first-time setup (`init`/`edit`,
  stack modeling) plus the daily lifecycle (`new`/`up`/`ps`/`logs`/`shell`/`open`/
  `code`/`cd`/`down`/`nuke`/`ls`/`end`).
- A `/sdev-start <slug> [description]` command that runs an end-to-end procedure:
  zero → running isolated workspace.
- The installer drops both into `~/.claude` **only when the user opts in**, and
  never touches unrelated Claude config.
- Upgrade-safe and idempotent, matching the existing `install` contract.

## Non-goals

- No uninstall command (matches the current `install`, which has none).
- No Claude Code *plugin* packaging (loose files into `~/.claude/skills` and
  `~/.claude/commands` are lighter and robust).
- Not a Workflow-tool JS orchestration script — this is a Skill + interactive
  slash-command.

## Repo layout (source of truth)

A new top-level `claude/` directory, tool-owned and versioned with sdev:

```
claude/
  skills/sdev/SKILL.md        # the Skill
  commands/sdev-start.md      # the /sdev-start command
```

These are tool code, not user data. They are overwritten on upgrade — the same
contract as `bin/` (replaceable; never under `$SDEV_HOME`).

## The Skill — `sdev`

`claude/skills/sdev/SKILL.md` with YAML frontmatter:

- `name: sdev`
- `description:` triggers when the user wants isolated or parallel development
  workspaces, docker-compose dev stacks, to "spin up a workspace", to run several
  features at once, or to set up / configure an sdev project.

Body (distilled from the README — guidance, not a verbatim copy):

- **Concepts** — project / task / profile (one line each).
- **Setup** — `sdev init`, `sdev edit`, stack modeling via `stack_services` and
  the `template:` key.
- **Daily lifecycle** — the command table
  (`new`/`up`/`ps`/`logs`/`shell`/`open`/`code`/`cd`/`down`/`nuke`/`ls`/`end`).
- **Parallel use** — pin a project per terminal (`sdev use`), `-p` override,
  single global port pool so stacks don't collide.
- **Gotchas** — bash ≥ 4, mikefarah `yq` v4 (not the Python `yq`), the `staging`
  confirmation guard, `--no-fetch` for offline/no-remote, and the symlinked-local-repo
  caveat (don't move/delete the source).
- A pointer to the `/sdev-start` command.

## The command — `/sdev-start <slug> [description]`

`claude/commands/sdev-start.md` (flat name → `/sdev-start`, not namespaced).
Frontmatter: `description` and `argument-hint: <slug> [description]`.

Procedure the agent follows:

1. **Resolve the active project.** Use the `sdev use` pin or a `-p` override. If
   no project is defined (`sdev projects` is empty), tell the user to run
   `sdev init` and stop.
2. **Create the task** — `sdev new <slug>` (mention `--no-fetch` if offline or the
   source repo has no remote).
3. **Bring the stack up** — `sdev up <slug>`.
4. **Record scope** — write `[description]` into the task's generated `CLAUDE.md`
   **Scope** line.
5. **Report** — `sdev ps <slug>`, print the nginx URL / debug ports, and offer
   `sdev open <slug>`.

## dist packaging

`dist` copies the repo's `claude/` directory into the staged `sdev/` package, so
the zip contains `sdev/claude/...`. One new copy block, alongside the existing
`bin/` copy.

## install integration (opt-in, detect)

- `install` adds `claude` to its tool-copy item list, so it lands in
  `$INSTALL_DIR/claude` (the install-dir source of truth, like `bin/`).
- A new step after the tool is placed:
  - **Interactive** and `~/.claude` exists → prompt
    `Install sdev Claude Code skill + command into ~/.claude? [Y/n]`.
    On yes (default), copy:
    - `$INSTALL_DIR/claude/skills/sdev` → `~/.claude/skills/sdev`
    - `$INSTALL_DIR/claude/commands/sdev-start.md` → `~/.claude/commands/sdev-start.md`
    Overwrites only those sdev-owned paths — idempotent and upgrade-safe; never
    touches other files under `~/.claude`.
  - **Non-interactive** → `SDEV_CLAUDE=1` enables the copy; otherwise skip.
  - No `~/.claude` and not explicitly enabled → skip silently (don't create
    `~/.claude` uninvited).
- Copy, not symlink: robust if the install dir is later removed, and matches how
  Claude config normally holds real files.

## Tests

- `tests/dist.bats` — the built zip contains `sdev/claude/skills/sdev/SKILL.md`
  and `sdev/claude/commands/sdev-start.md`.
- `tests/install.bats` —
  - piped `y` with a fake `$HOME/.claude` present → both files land in
    `~/.claude/skills/sdev/` and `~/.claude/commands/`;
  - piped `n` → nothing written to `~/.claude`;
  - no `~/.claude` + non-interactive → skipped, `~/.claude` not created;
  - `SDEV_CLAUDE=1` non-interactive → installed.

## Docs

README gains a short **"Claude Code integration"** section: what the skill and
command do, that they are opt-in at install time, where they land
(`~/.claude/skills/sdev`, `~/.claude/commands/sdev-start.md`), and that they are
refreshed on upgrade.

## Decisions (defaults chosen)

- Flat `/sdev-start`, not namespaced `/sdev:start`.
- Copy into `~/.claude`, not symlink.
- No uninstall (YAGNI; matches current `install`).
- Loose files, not a Claude Code plugin.
