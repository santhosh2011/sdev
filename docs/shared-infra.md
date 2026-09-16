# Shared Postgres infrastructure

Shared Postgres is opt-in; lifecycle hooks bypass every unmarked workspace.
PDMT/SCDI template-and-registry adoption patches are available in the repository
under `artifacts/shared-infra/postgres-patches/README.md`. Both target Postgres
18, including the approved SCDI move from 16; operators apply them to their
deployed project definitions separately. Existing workspaces keep their copied
templates, credentials and database version. Redis, Nginx and
`prune --volumes` remain outside this change. Redis sharing first requires
application-specific queue/stream/key prefixes.

## Commands

```
sdev infra up                  # postgres-18 by default
sdev infra up 16               # independent server, if a project needs 16
sdev infra status --json       # flavor, image, offset, port, users, databases
sdev infra down postgres-18    # stop, keep data and offset
sdev infra down postgres-18 --destroy
```

`up` creates the stack once under `$SDEV_HOME/infra/<flavor>/` and waits for
Postgres readiness. `--no-boot` scaffolds without Docker. It keeps the image and
port reservation stable on later invocations; changing configuration does not
silently upgrade an existing server. The default image is
`pgvector/pgvector:pg<major>`, so vector-capable and plain SQL projects can share
one version family. PostgreSQL 13–99 flavor identifiers are accepted; the image
must actually exist and support the selected major version.

For an exact/unusual pin, give it its own flavor in `core/.task-config.yml`:

```yaml
defaults:
  infra_port_offset_base: 2000
  infra_postgres_flavors:
    postgres-18-pinned:
      image: pgvector/pgvector:pg18-trixie
```

Run `sdev infra up 18-pinned`. The leading version must match the configured
image. Each flavor has its own volume, port and DNS alias (`sdev-postgres-18`,
`sdev-postgres-18-pinned`, etc.). PostgreSQL 18+ mounts `/var/lib/postgresql`;
older majors mount `/var/lib/postgresql/data`. The host port is `5432 + offset`,
bound only to `127.0.0.1`. Containers use their flavor alias and port 5432.

One SDEV_HOME owns these machine-wide stacks. Point projects at that home;
a second home is refused if it tries to manage an existing flavor's containers.
Lifecycle and ledger changes run under the existing portable state lock.
Docker boot failures retain the reservation/config so `up` can be retried.
Infra lives outside `projects/`, so task scanners and pruning cannot reclaim it.
All offset allocators union task, core and infra reservations, plus task `.env`
scans. Go carries `shared_infra` through every ledger rewrite.

## Teardown and consumers

The network `sdev-shared` is created idempotently by `infra up`. Both provider
and consumer Compose files declare it external. Only the separate infra
Compose project declares/mounts `pgdata`; a workspace file must have **no
reference to that volume**, including as an external volume. Workspace
`down -v` therefore cannot delete shared database storage.

`status` and `down` derive consumers from running Docker containers on the
shared network, counting each Compose project once and an unlabelled container
by ID. Infra servers are excluded. Nothing stores a refcount. The guard is
conservative across flavors: any consumer protects all of them. A failed
Docker query fails closed. A stopped workspace is not a live consumer; its
database is still retained. The observation is a snapshot: direct Docker or
Compose operations outside sdev do not participate in its lifecycle lock.

`down --destroy` requires no consumers and the exact canonical flavor typed at
the prompt, or explicitly supplied as `SDEV_CONFIRM=postgres-18`. It deletes
**all databases** in that flavor, then its workspace and ledger reservation.
It leaves the external network in place. A failed Docker teardown preserves
the workspace and reservation for retry. There is no automatic infra teardown.

## Opt-in workspace integration

The reusable `bin/templates/shared-postgres.yml.tmpl` fragment supplies a
one-shot `dbinit` with no volumes and the external network. Its command is
embedded so a copied workspace is self-contained; the source command is
`bin/templates/dbinit.sh`. It serializes same-database creation with a Postgres
advisory lock, quotes identifiers through `format('%I', ...)`, and exits on SQL
failure. The application must depend on successful completion of `dbinit` and
join both its own network and `sdev-shared`.

Generation resolves the project's `infra.postgres: "18"` key and
snapshots `SDEV_POSTGRES_FLAVOR`, `SDEV_POSTGRES_IMAGE`, `DB_HOST`, `DB_PORT=5432`
and `DB_NAME` into the workspace `.env`. Database clients use the dev-only
`sdev`/`sdev` user/password. This is trusted local development infrastructure,
not a security boundary between projects. The Bash/Go creation paths call `bin/infra-task`, which uses
`infra_task_database <ledger-key>` under `with_state_lock` to derive the name;
SQL names are not accepted as lifecycle arguments. Opted-in project names
and slugs must be lowercase kebab-case. Database names are ASCII, capped at
63 bytes; long names use the first 54 characters plus `_` and eight SHA-256 hex
digits from the full key. Ambiguous short project/slug boundaries fail closed.

`infra_ensure <flavor>` is the locked, idempotent boot primitive for `sdev up`.
The task `end`/`destroy`/ephemeral-prune and `nuke` paths call
`infra_drop_task_database <ledger-key>` after stopping task containers and
**before** removing their workspace/reservation.
This helper reads only the flavor from the workspace snapshot, recomputes the
database name from the ledger key (ignoring `.env`'s `DB_NAME`), and issues
`DROP DATABASE ... WITH (FORCE)`. On failure, retain the task and retry cleanup.
Core stacks use the separate `@core/<project>` ledger identity and
`core__<project>` database prefix; `core down --destroy`, `--volumes` and
`refresh --reseed` perform scoped cleanup too. Existing local-database workspace
snapshots never enter these paths merely because the project's registry changed.
Shared Postgres is restricted to the local profile; booting a marked snapshot
under a different profile or with a changed DB_NAME fails closed.

Automatic orphan-database reconciliation is deferred. A disappeared workspace
has lost its flavor snapshot; its remaining database is visible in `infra
status` but is not guessed at or automatically deleted. An unbooted task whose
flavor has no reservation can be removed without starting a server.

Opt-in requires a paired custom template with `dbinit` and the external network;
setting the registry key alone with the generic local-database template is
rejected. No shipped registry enables this key. A direct consumer `./compose
up` requires an already-running infra flavor; absent external networking or an
unreachable server causes startup to fail. Direct workspace `down -v` preserves
the shared database; use `sdev nuke` or `end` for scoped database deletion.

## Verification

`bats tests/infra.bats` covers lifecycle guards, custom flavors, naming,
allocation and Bash–Go round-tripping (build `bin/sdev-go` for the latter).
`tests/infra_live.sh` is an opt-in OrbStack proof using unique Compose projects
and a disposable home under `build/`. It checks two databases with sentinel
rows, consumer protection, workspace `down -v`, sibling survival after scoped
DROP, concurrent dbinit, forced disconnection, stop/start persistence and
confirmed infra destruction. It never starts, stops or deletes live projects.
