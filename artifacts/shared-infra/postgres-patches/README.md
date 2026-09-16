# PDMT and SCDI adoption

These patches opt both projects into the **same Postgres 18 flavor**. They
change each project's template and registry together. They target the deployed
files copied on 2026-09-17, **after** the cache patches in `../cache-patches/`.
No deployed file, existing workspace or application data was changed by this PR.
Firstmate applies these artifacts separately under the existing approval flow.

**`pdmt.patch`:** Replace the workspace `db` service and `chips-postgres` volume
with the shipped dbinit one-shot, add the external network to every Python
database client, and interpolate all database credentials from the generated
workspace `.env`. Preserve the `seed` migration/fixture step and its completion
dependencies. Set `infra.postgres: "18"` and remove the unused `db` host-port
slot. Redis, UI and the shared download caches retain their existing behavior.

**`scdi.patch`:** Remove `postgres:16` and `scdi-postgres`; use the same shared
Postgres 18 server as PDMT. The API, worker and beat override all five database
connection fields (including credentials from old `app.env`), wait for dbinit,
and join the shared network alongside `task-net`. Set the registry opt-in and
remove its obsolete `db` host-port slot. The API's existing Alembic startup
command, worker profiles, Redis, Nginx, UI and caches are preserved. This applies
the approved major-version change only to **new** workspaces; it never mounts
an existing PostgreSQL 16 data directory into PostgreSQL 18.

## Applying

Install sdev with the merged shared-infra foundation first. Use the patches
from this repository checkout; they are operational artifacts, not files
automatically installed into user data by the sdev distribution.

From the target SDEV_HOME, with `patch_root` set to this checkout's absolute
`artifacts/shared-infra` path:

```bash
# For each project, apply its cache patch first IF it has not already landed.
git apply --check "$patch_root/cache-patches/pdmt.patch"
git apply "$patch_root/cache-patches/pdmt.patch"
git apply --check "$patch_root/cache-patches/scdi.patch"
git apply "$patch_root/cache-patches/scdi.patch"

# Check the complete template+registry adoption before writing either file.
git apply --check "$patch_root/postgres-patches/pdmt.patch" \
                  "$patch_root/postgres-patches/scdi.patch"
git apply "$patch_root/postgres-patches/pdmt.patch" \
          "$patch_root/postgres-patches/scdi.patch"
```

If a cache patch is already applied, `git apply --reverse --check <cache-patch>`
can verify that baseline; skip its forward application. If neither direction
checks cleanly, refresh the patch against the current template instead of
forcing it or replacing unrelated application changes. Apply the template and
registry as a pair, before creating new tasks. Do not reconfigure existing
workspace snapshots or modify shared conf files.

New local workspaces created with `sdev -p pdmt new <slug>` or
`sdev -p scdi new <slug>` receive `postgres-18`, its DNS alias, and a distinct
database name. `sdev up` starts the shared server as needed. For direct
`./compose up`, first run `sdev infra up 18`. There is no workspace database
port anymore; `sdev infra status` reports the shared host port (container clients
use port 5432). Existing workspaces, including SCDI's old PostgreSQL 16 stacks,
keep their copied templates until normal teardown. This is gradual adoption,
not an immediate fleet-wide shutdown or data migration.

## Validation

`bats tests/adoption.bats` applies both exact patches to checked-in baselines,
checks every database client's credentials/network/readiness dependencies,
generates workspaces through Bash and the available native router, and proves
old snapshots retain their private database configuration. Baselines contain
no application secrets and are only fixtures, never installation defaults.

`bash tests/adoption_live.sh` is an optional OrbStack proof. It resolves both
full Compose models using synthetic config, runs each patched dbinit against
one uniquely named PG18 server, and checks separate databases and sentinel
rows. Workspace `down -v` preserves shared storage; scoped PDMT deletion leaves
SCDI data intact. It starts no application services, installs no application
dependencies and does not validate application migrations or UI behavior.
The existing PDMT npm failure is outside this change.

Shared Redis still requires application queue/key prefixes. Shared Nginx and
volume pruning remain deferred. These patches do not delete old DB volumes.
