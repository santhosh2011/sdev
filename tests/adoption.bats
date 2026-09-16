#!/usr/bin/env bats
load helpers

setup() {
  make_fixture
  REPO="$(dirname "$REPO_BIN")"
  cp -R "$REPO/tests/fixtures/shared-adoption/core/." "$WORKSPACE_ROOT/core/"
  # Apply exactly what the operator will apply, including both registry keys.
  (cd "$WORKSPACE_ROOT" && git apply "$REPO/artifacts/shared-infra/postgres-patches/pdmt.patch" "$REPO/artifacts/shared-infra/postgres-patches/scdi.patch")
}
teardown() { rm -rf "$WORKSPACE_ROOT"; }

make_adoption_sources() {
  local project registry path prefix
  for project in pdmt scdi; do
    registry="$WORKSPACE_ROOT/core/projects.d/$project.yml"
    while IFS= read -r path; do
      make_source_repo "$WORKSPACE_ROOT/core/$project/$path" develop
    done < <(yq -r '.repos[].path' "$registry")
    prefix="$(yq -r '.conf_prefix' "$registry")"
    mkdir -p "$WORKSPACE_ROOT/confs/$project"
    # Deliberately stale shared config: Compose environment must override it.
    printf 'DB_HOST=old-db\nDB_USER=old-user\nDB_PASSWORD=old-pass\nDB_NAME=old-name\n' > "$WORKSPACE_ROOT/confs/$project/$prefix.local.env"
  done
}

@test "adoption patches use Postgres 18 and remove all workspace database storage" {
  for project in pdmt scdi; do
    file="$WORKSPACE_ROOT/core/$project/docker-compose.tmpl"
    registry="$WORKSPACE_ROOT/core/projects.d/$project.yml"
    [ "$(yq '.infra.postgres' "$registry")" = 18 ]
    [ "$(yq '.stack_services | contains(["db"])' "$registry")" = false ]
    [ "$(yq '.services | has("db") or has("postgres")' "$file")" = false ]
    [ "$(yq '.volumes | keys | .[] | select(test("postgres|pgdata|db-data"))' "$file")" = '' ]
    [ "$(yq '.services.dbinit' "$file")" = "$(yq '.services.dbinit' "$REPO_BIN/templates/shared-postgres.yml.tmpl")" ]
    [ "$(yq '.networks."sdev-shared"' "$file")" = "$(yq '.networks."sdev-shared"' "$REPO_BIN/templates/shared-postgres.yml.tmpl")" ]
  done
}

@test "every database client waits for dbinit and overrides old config on both networks" {
  for project in pdmt scdi; do
    file="$WORKSPACE_ROOT/core/$project/docker-compose.tmpl"
    clients='api worker beat'; [ "$project" != pdmt ] || clients="seed $clients"
    for service in $clients; do
      [ "$(S="$service" yq '.services[strenv(S)].depends_on.dbinit.condition' "$file")" = service_completed_successfully ]
      [ "$(S="$service" yq '.services[strenv(S)].networks | contains(["task-net", "sdev-shared"])' "$file")" = true ]
      for key in DB_HOST DB_PORT DB_USER DB_PASSWORD DB_NAME; do
        [ "$(S="$service" K="$key" yq 'explode(.) | .services[strenv(S)].environment[strenv(K)]' "$file")" = "\${$key}" ]
      done
      [ "$(S="$service" yq '.services[strenv(S)].command' "$file")" = "$(S="$service" yq '.services[strenv(S)].command' "$REPO/tests/fixtures/shared-adoption/core/$project/docker-compose.tmpl")" ]
    done
  done
}

@test "adoption preserves local Redis nginx UI caches and PDMT seed ordering" {
  for project in pdmt scdi; do
    before="$REPO/tests/fixtures/shared-adoption/core/$project/docker-compose.tmpl"
    after="$WORKSPACE_ROOT/core/$project/docker-compose.tmpl"
    [ "$(yq '.services.redis, .services.ui, .services.nginx' "$after")" = "$(yq '.services.redis, .services.ui, .services.nginx' "$before")" ]
    [ "$(yq '.volumes | del(.chips-postgres, .scdi-postgres)' "$before")" = "$(yq '.volumes' "$after")" ]
  done
  for service in api worker beat; do
    [ "$(S="$service" yq '.services[strenv(S)].depends_on.seed.condition' "$WORKSPACE_ROOT/core/pdmt/docker-compose.tmpl")" = service_completed_successfully ]
  done
}

@test "both adopted projects generate distinct shared database snapshots through bash and router" {
  make_adoption_sources
  for project in pdmt scdi; do
    SDEV_PROJECT="$project" "$WORKSPACE_ROOT/bin/new-task" bash --no-fetch
    sdev -p "$project" new routed --no-fetch
    for slug in bash routed; do
      env="$WORKSPACE_ROOT/projects/$project/$slug/.env"
      grep -qx "DB_NAME=${project}_$slug" "$env"
      grep -qx 'DB_HOST=sdev-postgres-18' "$env"
      grep -qx 'SDEV_POSTGRES_IMAGE=pgvector/pgvector:pg18' "$env"
      grep -qx 'DB_USER=sdev' "$env"
      ! grep -q '^DB_HOST_PORT=' "$env"
      cmp "$WORKSPACE_ROOT/core/$project/docker-compose.tmpl" "$WORKSPACE_ROOT/projects/$project/$slug/docker-compose.yml"
    done
  done
  [ ! -d "$WORKSPACE_ROOT/infra" ]
}

@test "PDMT and SCDI existing workspace snapshots retain local databases after adoption" {
  make_adoption_sources
  cp -R "$REPO/tests/fixtures/shared-adoption/core/." "$WORKSPACE_ROOT/core/"
  for project in pdmt scdi; do
    sdev -p "$project" new old --no-fetch
  done
  (cd "$WORKSPACE_ROOT" && git apply "$REPO/artifacts/shared-infra/postgres-patches/pdmt.patch" "$REPO/artifacts/shared-infra/postgres-patches/scdi.patch")
  for project in pdmt scdi; do
    cmp "$REPO/tests/fixtures/shared-adoption/core/$project/docker-compose.tmpl" "$WORKSPACE_ROOT/projects/$project/old/docker-compose.yml"
    ! grep -q '^SDEV_POSTGRES_FLAVOR=' "$WORKSPACE_ROOT/projects/$project/old/.env"
    sdev -p "$project" new shared --no-fetch
    grep -qx "DB_NAME=${project}_shared" "$WORKSPACE_ROOT/projects/$project/shared/.env"
  done
}
