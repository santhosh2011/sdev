#!/usr/bin/env bash
# Optional OrbStack proof of the actual PDMT/SCDI patches. No app builds/boots.
set -euo pipefail
cd "$(dirname "$0")/.."
[[ "$(docker context show)" == orbstack ]] || { echo 'OrbStack required' >&2; exit 1; }
repo="$PWD"
proof="adopt$(date +%s)$$"
export SDEV_HOME="$repo/build/$proof"
mkdir -p "$SDEV_HOME/core" "$SDEV_HOME/confs"
cp core/.task-config.yml "$SDEV_HOME/core/.task-config.yml"
cp -R tests/fixtures/shared-adoption/core/. "$SDEV_HOME/core/"
(cd "$SDEV_HOME" && git apply "$repo/artifacts/shared-infra/postgres-patches/pdmt.patch" "$repo/artifacts/shared-infra/postgres-patches/scdi.patch")
# shellcheck source=../bin/_lib.sh
source bin/_lib.sh
# shellcheck source=../bin/_infra.sh
source bin/_infra.sh
flavor="postgres-18-$proof"
F="$flavor" yq -i '.defaults.infra_port_offset_base = 26000 | .defaults.infra_postgres_flavors[strenv(F)].image = "pgvector/pgvector:pg18"' "$GLOBAL_CONFIG"
network_existed=0
docker network inspect sdev-shared >/dev/null 2>&1 && network_existed=1
cleanup() {
  for project in pdmt scdi; do
    dir="$SDEV_HOME/projects/$project/$proof"
    if [[ -f "$dir/docker-compose.yml" ]]; then
      (cd "$dir" && docker compose --profile workers down -v --remove-orphans) >/dev/null 2>&1 || true
    fi
  done
  if [[ -f "$SDEV_HOME/infra/$flavor/compose" ]]; then
    infra_compose "$flavor" down -v --remove-orphans >/dev/null 2>&1 || true
  fi
  if [[ "$network_existed" == 0 ]]; then docker network rm sdev-shared >/dev/null 2>&1 || true; fi
}
trap cleanup EXIT
for project in pdmt scdi; do
  registry="$SDEV_HOME/core/projects.d/$project.yml"
  # Same PG18 image, uniquely named test flavor: never take over a live server.
  F="$flavor" yq -i '.infra.postgres = strenv(F)' "$registry"
  while IFS= read -r path; do
    source_dir="$SDEV_HOME/core/$project/$path"
    mkdir -p "$source_dir"
    git -C "$source_dir" init -q -b develop
    git -C "$source_dir" config user.email proof@example.invalid
    git -C "$source_dir" config user.name proof
    echo proof > "$source_dir/README"
    git -C "$source_dir" add README
    git -C "$source_dir" commit -qm initial
  done < <(yq -r '.repos[].path' "$registry")
  prefix="$(yq -r '.conf_prefix' "$registry")"
  mkdir -p "$SDEV_HOME/confs/$project"
  printf 'DB_HOST=old-db\nDB_USER=old-user\nDB_PASSWORD=old-pass\nDB_NAME=old-db\n' > "$SDEV_HOME/confs/$project/$prefix.local.env"
  bin/sdev -p "$project" new "$proof" --no-fetch
  dir="$SDEV_HOME/projects/$project/$proof"
  if [[ "$project" == scdi ]]; then
    # The UI env_file is a deployed absolute path. Resolve the whole model with
    # an empty test file instead; never read the deployed credentials.
    : > "$dir/ui-test.env"
    yq -i '.services.ui.env_file = ["./ui-test.env"]' "$dir/docker-compose.yml"
  fi
  (cd "$dir" && docker compose --profile workers config) > "$dir/resolved.yml"
  clients='api worker beat'; [[ "$project" != pdmt ]] || clients="seed $clients"
  for client in $clients; do
    [[ "$(C="$client" yq '.services[strenv(C)].environment.DB_NAME' "$dir/resolved.yml")" == "${project}_$proof" ]]
    [[ "$(C="$client" yq '.services[strenv(C)].environment.DB_HOST' "$dir/resolved.yml")" == "sdev-$flavor" ]]
    [[ "$(C="$client" yq '.services[strenv(C)].environment.DB_USER' "$dir/resolved.yml")" == sdev ]]
    [[ "$(C="$client" yq '.services[strenv(C)].environment.DB_PASSWORD' "$dir/resolved.yml")" == sdev ]]
    [[ "$(C="$client" yq '.services[strenv(C)].environment.DB_PORT' "$dir/resolved.yml")" == 5432 ]]
  done
  bin/infra-task ensure "$project/$proof"
  (cd "$dir" && docker compose run --rm dbinit)
  infra_compose "$flavor" exec -T postgres psql -U sdev -d "${project}_$proof" -v ON_ERROR_STOP=1 -c "CREATE TABLE sentinel(value text); INSERT INTO sentinel VALUES ('$project');"
done
[[ "$(docker ps --filter "label=com.docker.compose.project=sdev-infra-$flavor" -q | wc -l | tr -d ' ')" == 1 ]]
echo 'PASS both patched projects resolve every DB client and initialize distinct databases on one PG18 server'
(cd "$SDEV_HOME/projects/pdmt/$proof" && docker compose down -v --remove-orphans)
docker volume inspect "sdev-infra-${flavor}_pgdata" >/dev/null
docker network inspect sdev-shared >/dev/null
[[ "$(infra_compose "$flavor" exec -T postgres psql -U sdev -d "pdmt_$proof" -tAc 'SELECT value FROM sentinel')" == pdmt ]]
bin/infra-task drop "pdmt/$proof"
[[ "$(infra_compose "$flavor" exec -T postgres psql -U sdev -d postgres -tAc "SELECT count(*) FROM pg_database WHERE datname='pdmt_$proof'")" == 0 ]]
[[ "$(infra_compose "$flavor" exec -T postgres psql -U sdev -d "scdi_$proof" -tAc 'SELECT value FROM sentinel')" == scdi ]]
echo 'PASS workspace down -v preserves storage; scoped PDMT drop preserves SCDI data'
(cd "$SDEV_HOME/projects/scdi/$proof" && docker compose --profile workers down -v --remove-orphans)
bin/infra-task drop "scdi/$proof"
echo 'PASS scoped SCDI cleanup'
