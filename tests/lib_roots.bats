load helpers
setup() { make_fixture; }
teardown() { rm -rf "$WORKSPACE_ROOT"; unset SDEV_HOME SDEV_INSTALL; }

@test "WORKSPACE_ROOT is a legacy alias for SDEV_HOME (and equals SDEV_INSTALL in the fixture)" {
  # SDEV_INSTALL comes from BASH_SOURCE, not WORKSPACE_ROOT; they coincide here only
  # because the fixture copies _lib.sh into $WORKSPACE_ROOT/bin.
  run env -u SDEV_HOME bash -c \
    'source "'"$WORKSPACE_ROOT"'/bin/_lib.sh"; echo "$SDEV_HOME|$SDEV_INSTALL"'
  [ "$status" -eq 0 ]
  [ "$output" = "$WORKSPACE_ROOT|$WORKSPACE_ROOT" ]
}

@test "explicit SDEV_HOME wins over WORKSPACE_ROOT" {
  run env SDEV_HOME=/tmp/sdev-custom-home WORKSPACE_ROOT=/tmp/sdev-other bash -c \
    'source "'"$WORKSPACE_ROOT"'/bin/_lib.sh"; echo "$SDEV_HOME"'
  [ "$status" -eq 0 ]
  [ "$output" = "/tmp/sdev-custom-home" ]
}

@test "SDEV_HOME falls back to ~/.sdev when nothing is set" {
  run env -u WORKSPACE_ROOT -u SDEV_HOME HOME=/tmp/sdev-fakehome bash -c \
    'source "'"$WORKSPACE_ROOT"'/bin/_lib.sh"; echo "$SDEV_HOME"'
  [ "$output" = "/tmp/sdev-fakehome/.sdev" ]
}

@test "config_template default resolves under SDEV_INSTALL" {
  source "$WORKSPACE_ROOT/bin/_lib.sh"
  run config_template default
  [ "$status" -eq 0 ]
  [ "$output" = "$SDEV_INSTALL/bin/templates/docker-compose.yml.tmpl" ]
}

@test "ensure_home creates skeleton and seeds default config" {
  run env SDEV_HOME="$WORKSPACE_ROOT/seedtest" bash -c \
    'source "'"$WORKSPACE_ROOT"'/bin/_lib.sh"; ensure_home; \
     test -d "$SDEV_HOME/core/projects.d" && \
     test -d "$SDEV_HOME/projects/_archive" && \
     test -f "$SDEV_HOME/core/.task-config.yml"'
  [ "$status" -eq 0 ]
}
