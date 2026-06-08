load helpers
setup() {
  make_fixture; source "$WORKSPACE_ROOT/bin/_lib.sh"
  cat > "$WORKSPACE_ROOT/core/projects.d/acme.yml" <<'YAML'
conf_prefix: acme-api
default_shell_service: api
stack_services: [nginx, api, web, db, redis]
repos:
  api: { path: my-api, default_base: main, compose_role: api, link_node_modules: false }
  web: { path: my-web, default_base: main, compose_role: ui,  link_node_modules: true  }
YAML
}
teardown() { rm -rf "$WORKSPACE_ROOT"; }

@test "config_repos reads project repos" {
  run config_repos acme
  [[ "$output" == *"api"* ]] && [[ "$output" == *"web"* ]]
}
@test "config_repo_path resolves path" {
  run config_repo_path acme web
  [ "$output" = "my-web" ]
}
@test "config_repo_attr reads link_node_modules" {
  run config_repo_attr acme web link_node_modules
  [ "$output" = "true" ]
}
@test "config_conf_prefix from project" {
  run config_conf_prefix acme
  [ "$output" = "acme-api" ]
}
@test "config_conf_prefix legacy default when absent" {
  run config_conf_prefix default
  [ "$output" = "app" ]
}
@test "config_stack_services project overrides global" {
  run config_stack_services acme
  [[ "$output" == *"api"* ]]
}
@test "config_stack_services falls back to global default" {
  run config_stack_services default
  [[ "$output" == *"nginx"* ]] && [[ "$output" == *"api"* ]]
}
@test "config_base_port project then global" {
  run config_base_port default api
  [ "$output" = "8291" ]
}
@test "config_shell_service default api" {
  run config_shell_service default
  [ "$output" = "api" ]
}
@test "config_shell_service project value" {
  run config_shell_service acme
  [ "$output" = "api" ]
}
@test "config_template defaults to bin/templates when project has no template key" {
  run config_template acme
  [ "$output" = "$WORKSPACE_ROOT/bin/templates/docker-compose.yml.tmpl" ]
}
@test "config_template uses project override when template key set" {
  cat > "$WORKSPACE_ROOT/core/projects.d/custom.yml" <<'YAML'
conf_prefix: c
template: core/custom/docker-compose.tmpl
repos: { api: { path: svc, default_base: main, compose_role: api } }
YAML
  run config_template custom
  [ "$output" = "$WORKSPACE_ROOT/core/custom/docker-compose.tmpl" ]
}
@test "config_uses_default_template true when no template key" {
  run config_uses_default_template acme
  [ "$status" -eq 0 ]
}
@test "config_uses_default_template false when template key present" {
  cat > "$WORKSPACE_ROOT/core/projects.d/custom2.yml" <<'YAML'
conf_prefix: c
template: core/custom2/docker-compose.tmpl
repos: { api: { path: svc, default_base: main, compose_role: api } }
YAML
  run config_uses_default_template custom2
  [ "$status" -ne 0 ]
}
