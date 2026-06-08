load helpers

setup() {
  BOOT_HOME="$(mktemp -d)"
  REPO="$(cd "$BATS_TEST_DIRNAME/.." && pwd)"
}
teardown() { rm -rf "$BOOT_HOME"; }

@test "bootstrap --check passes when deps present" {
  run "$REPO/bootstrap" --check
  [ "$status" -eq 0 ]
  [[ "$output" == *"yq"* ]]
}

@test "bootstrap --scaffold creates core dirs in target and is idempotent" {
  run "$REPO/bootstrap" --scaffold --dir "$BOOT_HOME"
  [ "$status" -eq 0 ]
  [ -d "$BOOT_HOME/core/projects.d" ]
  [ -d "$BOOT_HOME/confs" ]
  [ -d "$BOOT_HOME/projects" ]
  # idempotent: second run still exits 0, doesn't clobber
  echo 'conf_prefix: x' > "$BOOT_HOME/core/projects.d/keep.yml"
  run "$REPO/bootstrap" --scaffold --dir "$BOOT_HOME"
  [ "$status" -eq 0 ]
  [ -f "$BOOT_HOME/core/projects.d/keep.yml" ]
}
