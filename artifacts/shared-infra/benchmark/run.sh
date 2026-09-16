#!/usr/bin/env bash
# Usage: run.sh COPY_OF_PDMT_TEMPLATE COPY_OF_PDMT_REPO
# Both inputs must be disposable copies. All services run in OrbStack.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
template="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
source_copy="$(cd "$2" && pwd)"
[[ "$(docker context show)" == orbstack ]] || { echo 'OrbStack context required' >&2; exit 1; }
bench_id="sdev-cache-bench-$(date +%s)-$$"
bench_dir="$here/work/$bench_id"
mkdir -p "$bench_dir"
compose=(docker-compose)
projects=()
cleanup() {
    for project in "${projects[@]}"; do
        "${compose[@]}" -p "$project" -f "$bench_dir/$project/docker-compose.yml" down -v --remove-orphans >/dev/null 2>&1 || true
    done
    docker volume rm "${bench_id}-pip" >/dev/null 2>&1 || true
}
trap cleanup EXIT
# Warm only the benchmark's own external caches. Never use the machine's caches.
docker volume create "${bench_id}-pip" >/dev/null
printf 'case,seconds\n' > "$here/results.csv"
for scenario in before-first before-second after-first after-second; do
    project="$bench_id-$scenario"
    projects+=("$project")
    dir="$bench_dir/$project"
    mkdir -p "$dir"
    cp -R "$source_copy" "$dir/edm-apps-obs-pdmt-chips-ui"
    cp "$template" "$dir/docker-compose.yml"
    # Measure pip dependency-ready startup, not migrations or app readiness.
    # Keep PDMT's real four installs: seed first, then api/worker/beat together.
    # npm timing is blocked by the known sdev-pdmt-ui-npm-fix-t4 edgesOut bug.
    yq -i 'del(.services.db, .services.redis, .services.ui, .volumes."chips-postgres") |
      .services[] |= (del(.env_file, .ports, .depends_on) | .restart = "no") |
      .services.seed.command = ["bash", "-lc", "pip install --quiet -r requirements.txt"] |
      .services.api.command = .services.seed.command |
      .services.worker.command = .services.seed.command |
      .services.beat.command = .services.seed.command |
      .services.api.depends_on.seed.condition = "service_completed_successfully" |
      .services.worker.depends_on.seed.condition = "service_completed_successfully" |
      .services.beat.depends_on.seed.condition = "service_completed_successfully"' "$dir/docker-compose.yml"
    if [[ "$scenario" == after-* ]]; then
        PIP="${bench_id}-pip" yq -i '
          .volumes."pip-cache" = {"external": true, "name": strenv(PIP)}' "$dir/docker-compose.yml"
    fi
    start="$(date +%s)"
    "${compose[@]}" -p "$project" -f "$dir/docker-compose.yml" up --abort-on-container-failure > "$dir/boot.log" 2>&1
    # Compose should return failure, but independently verify every exit code.
    ids="$("${compose[@]}" -p "$project" -f "$dir/docker-compose.yml" ps -aq)"
    [[ $(echo "$ids" | wc -l | tr -d ' ') == 4 ]]
    while read -r id; do
        [[ "$(docker inspect -f '{{.State.ExitCode}}' "$id")" == 0 ]]
    done <<< "$ids"
    printf '%s,%s\n' "$scenario" "$(( $(date +%s) - start ))" | tee -a "$here/results.csv"
    "${compose[@]}" -p "$project" -f "$dir/docker-compose.yml" down -v --remove-orphans >> "$dir/boot.log" 2>&1
    if [[ "$scenario" == after-* ]]; then
        docker volume inspect "${bench_id}-pip" >/dev/null
    fi
done
