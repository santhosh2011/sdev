# sdev agent API

This is the machine-readable interface an agent (firstmate, or any orchestrator) uses to drive sdev.
It is the public seam of the "clean two-product seam" model: sdev owns the isolated workspaces and their lifecycle; the agent owns the conversation and the crew, and drives sdev through the commands below instead of parsing human output.

## Conventions

Every read/lifecycle command takes an opt-in `--json` flag and emits **one JSON object on stdout**.
Human-facing logs and progress go to **stderr**, so `2>/dev/null` leaves clean JSON on stdout.
Exit codes: `0` = ok, `1` = usage / not-found / needs-decisions, `2` = environment failure.
JSON is built with `jq` (never string concatenation), carries pre-computed aggregates, uses definitive empty states (`[]` + `0`), and includes a `next` array of suggested follow-up commands.
External tools (`lavish-axi`, `no-mistakes`, `gh`) are optional: when one is absent, the command does the rest and reports what it skipped rather than failing.

## Read surface (observe the fleet)

### `sdev status --json`

Fleet overview across all projects.

```json
{ "sdev_home": "...", "active_project": "scdi",
  "projects": [{"name": "scdi", "tasks": 5, "running": 0}],
  "totals": {"projects": 5, "tasks": 9, "running": 0}, "next": [...] }
```

### `sdev ls --json` (optionally `-p <project>`)

Structured task list.

```json
{ "project": "scdi",
  "alive": [{"task": "scdi/v2", "offset": 270, "nginx_port": 5470,
             "url": "http://localhost:5470/", "running": 0, "status": "stopped"}],
  "archived": [{"task": "...", "archived": "2026-06-01"}],
  "orphan_volumes": ["..."],
  "totals": {"alive": 5, "archived": 57, "orphans": 26, "running": 0} }
```

### `sdev ps <slug> --json`

Normalized `compose ps` for one task, plus the resolved URL.

```json
{ "task": "scdi/v2", "url": "http://localhost:5470/",
  "services": [{"name": "nginx", "state": "running", "ports": ["5470->80"]}] }
```

### `sdev doctor`

Environment + state-ledger diagnostics.
Prints `doctor: OK` and exits `0` when healthy; exits `2` on a hard failure.

## Lifecycle (the review -> ship loop)

The full loop an agent runs for a task:

```
sdev start <slug>  ->  (work happens in the isolated stack)  ->  sdev review <slug>  ->  sdev ship <slug>
```

### `sdev start <slug> --json [--no-open]`

The front door: create the task if it does not exist, boot its stack, and return the live URL.
Resumes an existing task instead of erroring.
Accepts every `new` flag (`--env`, `--repos`, `--<repo>-base`, `--no-fetch`, `--ephemeral`, `--lease`).

```json
{ "task": "web/feat", "project": "web", "env": "local",
  "url": "http://localhost:8080/", "nginx_port": 8080, "created": true, "next": [...] }
```

On boot failure it exits `2` with `{"error": {...}, "task": ..., "created": ...}` and leaves the task in place.

### `sdev review <slug> --json [--no-open] [--no-gate]`

Render the task's multi-repo diff as an annotatable lavish-axi surface and run the quality gate.
Exits `1` when the gate returns **needs-decisions** (a human call is required).

```json
{ "task": "web/feat", "project": "web",
  "repos": [{"repo": "api", "files": 5, "added": 202, "removed": 11}],
  "artifact": "/.../review-feat.html", "url": "...", "lavish_url": "http://127.0.0.1:4387/session/...",
  "gate": {"status": "clean", "detail": "..."}, "next": [...] }
```

The gate command defaults to `no-mistakes` and is overridable with `$SDEV_GATE_CMD`.
`gate.status` is one of `clean` / `needs-decisions` / `skipped`.

### `sdev ship <slug> --json [--assignee <who>] [--force]`

Push the task's branch(es) and open/update a PR with the assignee set.
Merge stays a human decision - `ship` never merges.

```json
{ "task": "web/feat", "project": "web", "assignee": "@me",
  "pushed": ["api"], "prs": [{"repo": "api", "url": "https://github.com/.../pull/1"}],
  "gh": true, "next": ["merge is yours: gh pr merge"] }
```

## How firstmate drives this

A crewmate finishing a task should:

1. `sdev review <slug> --json` and inspect `gate.status`.
   If `needs-decisions`, surface the lavish `lavish_url` and the gate detail to the captain and stop.
2. On `clean` (or after the captain approves), `sdev ship <slug> --json` and report the `prs[].url`.
3. Never merge - the captain merges.

Because every command emits structured JSON on stdout with logs on stderr, firstmate reads the result directly (`... --json 2>/dev/null | jq ...`) instead of scraping formatted text.
Adopting this is a prompt-level change on the firstmate side: point crewmate instructions at the `--json` commands above.
