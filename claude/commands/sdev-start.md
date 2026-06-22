---
description: Take a feature from zero to a running, isolated sdev workspace — create the task, bring its stack up, record scope, and report the URL.
argument-hint: <slug> [description]
---

Start a new isolated sdev workspace end-to-end.

**Arguments:** `$ARGUMENTS` — the first token is the task `<slug>`; the rest is an
optional one-line description of the work.

Follow these steps, keeping the user informed and stopping to surface any command
that fails (missing dependency, port conflict, staging confirmation):

1. **Confirm a project is active.** Run `sdev projects`. If it lists none, tell the
   user to run `sdev init` first and stop. Otherwise use the pinned `sdev use`
   project (or ask which to use, or pass `-p <project>` on each command below).

2. **Create the task.** Run `sdev new <slug>`. If the machine is offline or the
   source repo has no remote, use `sdev new <slug> --no-fetch`.

3. **Bring the stack up.** Run `sdev up <slug>`.

4. **Record scope.** If a description was given, open the task's generated
   `CLAUDE.md` (in the directory printed by `sdev cd <slug>`) and replace the
   `(one-line — fill in)` placeholder on the **Scope** line with the description.

5. **Report.** Run `sdev ps <slug>`, then show the task's primary nginx URL and
   debug ports (from the task `CLAUDE.md`). Offer to run `sdev open <slug>` to open
   it in a browser.
