# MR 2 verification: shared Postgres foundation

The opt-in `tests/infra_live.sh` completed on OrbStack on 2026-09-16 using
`pgvector/pgvector:pg18`, a unique test flavor and a disposable SDEV_HOME under
the worktree's ignored `build/` directory. No live application was opted in.

Observed results:

- Two independent workspace databases (`demo_a`, `demo_b`) were initialized
  by the shipped one-shot fragment and populated with distinct sentinel rows.
- `infra status --json` reported two consumers and both databases.
- `infra down` refused while those consumers were running.
- Workspace A's `down -v --remove-orphans` preserved the infra-owned volume
  and external network. Its scoped DROP removed only `demo_a`; `demo_b` still
  returned `keep-b` from its sentinel table.
- Two concurrent dbinit invocations for `demo_a` both completed successfully.
- `DROP DATABASE ... WITH (FORCE)` terminated a deliberately open connection
  to `demo_a` and removed that database.
- Infra stop/start retained the other database's sentinel row.
- The native task creation/up/end bridge created a database, booted its
  consumer, then dropped only that database while the sibling sentinel survived.
- Confirmed destruction removed the unique test flavor's volume and freed
  its offset. Cleanup left no test containers or test volumes. The shared
  network was removed only because the proof created it and it was unused.

The Bats suite mocks Docker for lifecycle failure paths and Bash–Go ledger
interoperability. This engine proof checks the actual Compose/Postgres behavior
that mocks cannot establish. Neither proof claims application-level isolation:
these local databases use one trusted development administrator.
