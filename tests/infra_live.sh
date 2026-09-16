#!/usr/bin/env bash
# Opt-in OrbStack integration proof; not run by bats tests/ or CI.
# Starts only uniquely named test stacks with a disposable SDEV_HOME in build/.
set -euo pipefail
cd "$(dirname "$0")/.."
[[ "$(docker context show)" == orbstack ]] || { echo 'OrbStack required' >&2; exit 1; }
proof="proof$(date +%s)$$"
export SDEV_HOME="$PWD/build/infra-$proof"
mkdir -p "$SDEV_HOME/core" "$SDEV_HOME/projects/demo"
cp core/.task-config.yml "$SDEV_HOME/core/.task-config.yml"
# shellcheck source=../bin/_lib.sh
source bin/_lib.sh
# shellcheck source=../bin/_infra.sh
source bin/_infra.sh
flavor="postgres-18-$proof"
F="$flavor" yq -i '.defaults.infra_port_offset_base = 24000 | .defaults.infra_postgres_flavors[strenv(F)].image = "pgvector/pgvector:pg18"' "$GLOBAL_CONFIG"
network_existed=0
docker network inspect sdev-shared >/dev/null 2>&1 && network_existed=1
cleanup() {
    for task in a b auto; do
        if [[ -f "$SDEV_HOME/projects/demo/$task/compose" ]]; then
            (cd "$SDEV_HOME/projects/demo/$task" && ./compose down -v --remove-orphans) >/dev/null 2>&1 || true
        fi
    done
    if [[ -f "$SDEV_HOME/projects/$proof/auto/compose" ]]; then
        (cd "$SDEV_HOME/projects/$proof/auto" && ./compose down -v --remove-orphans) >/dev/null 2>&1 || true
    fi
    # Only our uniquely named Compose project, never another infra stack.
    if [[ -f "$SDEV_HOME/infra/$flavor/compose" ]]; then
        infra_compose "$flavor" down -v --remove-orphans >/dev/null 2>&1 || true
    fi
    # Engine refuses removal if another consumer attached while proof ran.
    if [[ "$network_existed" == 0 ]]; then docker network rm sdev-shared >/dev/null 2>&1 || true; fi
}
trap cleanup EXIT
bin/sdev infra up "$flavor"
server="$(infra_compose "$flavor" ps -q postgres)"
bin/sdev infra up "$flavor"
[[ "$(infra_compose "$flavor" ps -q postgres)" == "$server" ]]
echo 'PASS repeat up preserves the owned running server'
for task in a b; do
    dir="$SDEV_HOME/projects/demo/$task"
    mkdir -p "$dir"
    allocate_offset "demo/$task" >/dev/null
    db="$(with_state_lock infra_task_database "demo/$task")"
    printf 'COMPOSE_PROJECT_NAME=sdev-%s-%s\nSDEV_POSTGRES_FLAVOR=%s\nSDEV_POSTGRES_IMAGE=pgvector/pgvector:pg18\nDB_NAME=%s\nDB_HOST=sdev-%s\n' "$proof" "$task" "$flavor" "$db" "$flavor" > "$dir/.env"
    cp bin/templates/shared-postgres.yml.tmpl "$dir/docker-compose.yml"
    cp bin/templates/compose.tmpl "$dir/compose"; chmod +x "$dir/compose"
    yq -i '.services.app = {"image": "pgvector/pgvector:pg18", "entrypoint": ["sleep", "infinity"], "networks": ["sdev-shared"], "depends_on": {"dbinit": {"condition": "service_completed_successfully"}}}' "$dir/docker-compose.yml"
    (cd "$dir" && ./compose up -d --wait)
    infra_compose "$flavor" exec -T postgres psql -U sdev -d "$db" -v ON_ERROR_STOP=1 -c "CREATE TABLE sentinel(value text); INSERT INTO sentinel VALUES ('keep-$task');"
done
bin/sdev infra status "$flavor" --json
if bin/sdev infra down "$flavor"; then echo 'ERROR: stopped with live consumers' >&2; exit 1; fi
(cd "$SDEV_HOME/projects/demo/a" && ./compose down -v --remove-orphans)
docker volume inspect "sdev-infra-${flavor}_pgdata" >/dev/null
docker network inspect sdev-shared >/dev/null
infra_drop_task_database demo/a
[[ "$(infra_compose "$flavor" exec -T postgres psql -U sdev -d postgres -tAc "SELECT count(*) FROM pg_database WHERE datname='demo_a'")" == 0 ]]
[[ "$(infra_compose "$flavor" exec -T postgres psql -U sdev -d demo_b -tAc 'SELECT value FROM sentinel')" == keep-b ]]
echo 'PASS workspace down -v preserves shared volume/network; scoped DROP preserves sibling data'
# Concurrent dbinit for one key must create exactly one database successfully.
(cd "$SDEV_HOME/projects/demo/a" && ./compose run --rm dbinit) > "$SDEV_HOME/init-one.log" 2>&1 & p1=$!
(cd "$SDEV_HOME/projects/demo/a" && ./compose run --rm dbinit) > "$SDEV_HOME/init-two.log" 2>&1 & p2=$!
wait "$p1"; wait "$p2"
# WITH (FORCE) really terminates an active connection to only the target DB.
infra_compose "$flavor" exec -T postgres psql -U sdev -d demo_a -c 'SELECT pg_sleep(60)' > "$SDEV_HOME/connection.log" 2>&1 & sleeper=$!
ready=0
for _ in $(seq 1 50); do
    if [[ "$(infra_compose "$flavor" exec -T postgres psql -U sdev -d postgres -tAc "SELECT count(*) FROM pg_stat_activity WHERE datname='demo_a' AND query LIKE 'SELECT pg_sleep%'")" -gt 0 ]]; then ready=1; break; fi
    sleep 0.1
done
[[ "$ready" == 1 ]]
infra_drop_task_database demo/a
wait "$sleeper" || true
echo 'PASS concurrent dbinit and DROP DATABASE WITH (FORCE)'
(cd "$SDEV_HOME/projects/demo/b" && ./compose down -v --remove-orphans)
# Conservative network-wide guard can see external users; do not stop them.
bin/sdev infra down "$flavor"
bin/sdev infra up "$flavor"
[[ "$(infra_compose "$flavor" exec -T postgres psql -U sdev -d demo_b -tAc 'SELECT value FROM sentinel')" == keep-b ]]
echo 'PASS infra stop/start preserves data'
# Exercise the real creation/up/end bridge, not only the lower-level helpers.
mkdir -p "$SDEV_HOME/core/$proof/api" "$SDEV_HOME/core/projects.d" "$SDEV_HOME/confs/$proof"
git -C "$SDEV_HOME/core/$proof/api" init -q -b main
git -C "$SDEV_HOME/core/$proof/api" config user.name proof
git -C "$SDEV_HOME/core/$proof/api" config user.email proof@example.invalid
echo proof > "$SDEV_HOME/core/$proof/api/README"
git -C "$SDEV_HOME/core/$proof/api" add README
git -C "$SDEV_HOME/core/$proof/api" commit -qm initial
cp "$SDEV_HOME/projects/demo/b/docker-compose.yml" "$SDEV_HOME/core/$proof/shared.tmpl"
F="$flavor" PROJECT="$proof" yq -n '{"conf_prefix": strenv(PROJECT), "infra": {"postgres": strenv(F)}, "template": ("core/" + strenv(PROJECT) + "/shared.tmpl"), "repos": {"api": {"path": "api", "default_base": "main", "compose_role": "api"}}}' > "$SDEV_HOME/core/projects.d/$proof.yml"
: > "$SDEV_HOME/confs/$proof/$proof.local.env"
bin/sdev -p "$proof" new auto --no-fetch
bin/sdev -p "$proof" up auto
# Plain up starts containers asynchronously; wait for the one-shot/app readiness.
(cd "$SDEV_HOME/projects/$proof/auto" && ./compose up -d --wait)
[[ "$(infra_compose "$flavor" exec -T postgres psql -U sdev -d postgres -tAc "SELECT count(*) FROM pg_database WHERE datname='${proof}_auto'")" == 1 ]]
bin/sdev -p "$proof" end auto --force
[[ "$(infra_compose "$flavor" exec -T postgres psql -U sdev -d postgres -tAc "SELECT count(*) FROM pg_database WHERE datname='${proof}_auto'")" == 0 ]]
[[ "$(infra_compose "$flavor" exec -T postgres psql -U sdev -d demo_b -tAc 'SELECT value FROM sentinel')" == keep-b ]]
echo 'PASS native task creation/up/end bridge and sibling survival'

SDEV_CONFIRM="$flavor" bin/sdev infra down "$flavor" --destroy
[[ -z "$(infra_entry "$flavor" offset)" ]]
! docker volume inspect "sdev-infra-${flavor}_pgdata" >/dev/null 2>&1
echo 'PASS confirmed destroy frees only the test flavor'
