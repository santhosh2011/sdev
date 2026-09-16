# Shared pip cache benchmark

The benchmark runs only disposable containers in OrbStack. It uses a copied
PDMT Compose template and a copied PDMT checkout, with the original
`backend/requirements.txt`. Four fresh Python 3.14 containers install the real
requirements: seed finishes first, then api/worker/beat install concurrently.
The templates' package-install command and cache mount are preserved; database,
Redis, UI, migrations, application servers and credentials are excluded.

Run `./run.sh <copied-pdmt-template> <copied-pdmt-checkout>`. Inputs are read-only;
all generated workspaces and logs are under ignored `work/`. The harness asserts
the OrbStack Docker context, creates uniquely named benchmark resources, and
cleans them up on exit. It never uses the live `sdev-pip-cache` volume.

Each scenario gets fresh container filesystems and a separate copy of the
sources. Before-first and before-second have independent project-owned caches.
After-first populates a new external cache; after-second reuses it after the
first workspace has run `down -v`. The harness verifies the external volume
survives that teardown and all four package installs exit successfully.
Timing covers Compose startup through completion of all four installs and the
exit-code checks; image pulls, source copying and teardown are excluded. This
is **pip dependency-ready startup**, not full application boot time. Image
layers were already present locally. A single sequential sample per scenario
is descriptive, not a controlled performance claim; host load and network vary.

The npm benchmark is blocked by the known pre-existing PDMT UI npm `edgesOut`
error tracked as `sdev-pdmt-ui-npm-fix-t4`. Two isolated attempts reproduced it.
No npm bug fix is included. The npm cache wiring is covered by the cache tests,
but no npm performance number is claimed.

Measured environment (2026-09-16): OrbStack, 10 vCPUs, 8,393,289,728 bytes VM
memory; Docker Compose 5.1.2; Python image
`sha256:8edbf9e42c7fb168b9c523718ed907117e6d2e60f5889c0c499bbda3a787da53`.
PDMT source commit: `1a7ad35c4c990a5fb7e660e0b42f2bb89abd64a0`.
Requirements SHA-256:
`965a7fb362b8e176df53facf2bb3dac67f1f26c3d69319054f30925d08cc59dd`.
The source requirements use version ranges, so future runs can resolve different
packages even with the same input file.

## Results

| Workspace | Cache state | Seconds |
|---|---|---:|
| Before: first | Independent empty cache | 98 |
| Before: second | Independent empty cache | 91 |
| After: first | Empty shared cache | 111 |
| After: second | Reused shared cache | 69 |

The second workspace completed **22 seconds sooner (24%)** with the shared
cache in this sample (91 → 69 seconds). The cold runs ranged from 91 to 111
seconds, so this is not a guaranteed improvement or a CPU-load measurement.
All sixteen Python install containers exited successfully, including the
concurrent consumers. The shared cache survived both workspace teardowns.
After the harness exited, no benchmark containers or named benchmark volumes
remained. No live application or live cache volume was changed.
