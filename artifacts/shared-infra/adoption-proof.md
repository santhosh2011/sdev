# PDMT/SCDI adoption verification

On 2026-09-17, `bash tests/adoption_live.sh` passed on OrbStack using
`pgvector/pgvector:pg18`. Both exact patches were applied to disposable copies
of the deployed templates and registries. The proof used unique workspace
names and a unique test flavor pointing to the PG18 image; it did not manage
any live project's containers, templates or data.

- Full Compose models resolved for both projects, including SCDI's optional
  worker profile. Every database client's environment used its generated
  workspace database, shared DNS alias and `sdev` credentials, overriding
  deliberately stale values in synthetic `app.env` files.
- Both patched dbinit services completed and created separate databases on
  one Postgres 18 server. Each database received a distinct sentinel row.
- PDMT workspace `down -v --remove-orphans` retained the provider's volume,
  external network and database row.
- Scoped PDMT cleanup dropped its database while SCDI's row survived.
  Scoped SCDI cleanup then succeeded. Cleanup removed all proof containers
  and its provider volume; the network was removed only if proof-created.

The proof started dbinit and Postgres only. It did not build application
images, run Alembic or seed fixtures, install npm dependencies, or boot APIs,
workers or UIs. It establishes the Compose/database wiring and teardown
behavior, not application compatibility with PostgreSQL 18. The approved SCDI
version change takes effect only for new workspaces, with no PG16 data reused.

`tests/adoption.bats` additionally gates exact patch applicability, database
volume removal, readiness and seed ordering, retained cache/Redis/Nginx/UI
configuration, Bash/native-router snapshot generation, and old-workspace
compatibility. All fixtures and generated workspaces are isolated from the
deployed SDEV_HOME.
