package cli

import "fmt"

// usageText is the `sdev help` output, kept in sync with usage() in bin/sdev.
const usageText = `Usage: sdev [-p <project>] <command> [args…]
       sdev <slug>                # shorthand for: sdev new <slug>

Project:
  use [<project>]                 pin active project for this terminal (no arg: show)
  projects                        list defined projects
  -p <project>                    override active project for one command

Commands:
  new <slug> [flags…]             create a new task workspace
  start <slug> [flags…]           front door: create-or-resume + boot + open (accepts new flags; --no-open, --json)
  review <slug> [--no-open]       render the task diff as an annotatable lavish surface (--json)
  ship <slug> [flags…]            push task branch(es) + open PR (assignee set); --assignee, --force, --json
  up <slug> [extra…]              start the stack (docker-compose up -d)
  down <slug>                     stop the stack (docker-compose down)
  nuke <slug>                     stop + reclaim volumes (down -v --remove-orphans)
  end <slug> [--pool] [flags…]    tear down + archive (or return worktree to pool)
  destroy <slug> [--force]        force-remove a task (worktree + offset + entry; no archive)
  prune [--apply] [--pool]        reclaim ephemeral/abandoned slots; --pool drains the warm pool
  lease <slug> [holder]           durably reserve a task (survives with no process)
  release <slug>                  drop a task's lease + process-lock
  hold <slug> [--pid N]           attach a self-healing process-lock (default: this shell)
  new <slug> --ephemeral          create a short-lived, auto-reclaimable task
  doctor                          check deps + state-ledger integrity
  ls | list [--json]              list alive / archived / orphan tasks
  status [--json]                 fleet overview across all projects
  ps <slug> [--json]              compose ps for a task
  logs <slug> [service…]          tail logs (-f); pass --no-follow to disable
  shell <slug> [service]          exec sh in service (default: project's shell service)
  open <slug>                     open http://localhost:<nginx-port>/ in browser
  code <slug>                     open the task dir in Zed
  cd <slug>                       print absolute task dir (use: cd "$(sdev cd <slug>)")
  init                            interactive wizard to configure your first project
  edit [<project>] [--delete-source]   add/remove repos, edit conf/shell/stack
  migrate --from <dir>            move an old in-repo sdev layout into $SDEV_HOME
  update                          fetch the latest release and reinstall in place
  help | -h | --help              show this help`

func usage() {
	fmt.Println(usageText)
}
