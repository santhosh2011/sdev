Read-only snapshots of the deployed PDMT/SCDI templates and registries taken
on 2026-09-17, with the preceding cache patches applied to the template copies.
No app.env or application credentials are included. Absolute UI/nginx paths
are preserved as patch context; tests never read those deployed files.

`tests/adoption.bats` copies these into its disposable fixture and applies the
exact artifacts in `artifacts/shared-infra/postgres-patches/`. Keep these
baselines fixed when changing the patches so patch applicability stays tested.
These are not sdev installation defaults or live project definitions.
