These patches change only download-cache mounts and declarations. They were
prepared from read-only copies of the deployed templates on 2026-09-16. They
have **not** been applied to the live SDEV_HOME. Apply from its root with
`git apply --check <patch>` followed by `git apply <patch>` after approval.
New tasks pick up template snapshots; existing tasks keep their old copies.

**pdmt.patch:** Adds `sdev-npm-cache` at `/root/.npm` and changes the existing
`pip-cache` to the external `sdev-pip-cache` volume. The four Python services
continue using the same mount anchor. Their installed packages, the UI's
anonymous `node_modules` volumes, Postgres and Redis remain workspace-local.
Install the updated sdev wrapper before creating new workspaces, or explicitly
run `docker volume create sdev-pip-cache` and `docker volume create sdev-npm-cache`
once before booting with an older wrapper. These commands are idempotent.

**scdi.patch:** Adds the external `sdev-npm-cache` volume to the UI at
`/root/.npm`. SCDI installs Python packages during image builds, so mounting a
runtime pip cache would not accelerate those builds; the patch does not add
one. Install the updated sdev wrapper before generating new workspaces, or
create `sdev-npm-cache` once before booting with an older wrapper. This cache
patch leaves SCDI's Postgres version unchanged; the approved version move and
shared-database adoption belong to the subsequent infrastructure changes.

YAML was edited with mikefarah yq v4; its serialization also normalizes spacing
and folds existing multiline YAML scalars without changing their values.
