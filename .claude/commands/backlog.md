---
description: Reconcile GitHub issues with reality and keep the ready queue value-ordered, consulting the user personas for API/CLI-facing work. Doc-only commit; no code changes.
---

**Claim the pass before anyone surveys** (#2010). `git fetch --prune
origin` and read every `origin/meta/backlog-*` tip:

- **Younger than WORKFLOW's 2h claim TTL:** a pass in flight, so stop.
- **Older, and its head is the bare claim commit:** a dead claim; claim
  as below.
- **Older, and its head is a `meta: backlog` commit:** this decides,
  with the tip's committer date (`%cI`) as `<date>`:

  ```sh
  git log origin/main --since=<date> --grep='^meta: backlog' --format=%h
  ```

  A printed commit means the pass landed or a later one superseded it:
  report the ref for deletion and claim as below. Nothing printed means a
  finished pass never landed: push an empty claim commit to that branch,
  merge `origin/main` forward, fill what it left, and land it as this
  pass instead of starting another (#2610).

To claim, cut `meta/backlog-<YYYYMMDD-HHMM>` from `origin/main`, commit an
empty claim on it and push it; the pass lands from that branch.

Delegate to the **cartographer** for its full /backlog pass: survey
reality, reconcile the issue list against it, reconcile the branch
namespace, and **rewrite docs/PLAN.md's Status section by replacement** —
lane scores read from `conformance/testdata/expectations/*.txt`, milestone
and queue counts read from GitHub, one date stamp for the section, and the
next planning action named. Replace it; never leave the previous version
beside the new one.

For API- or CLI-facing milestones, **you** — this session, not the
cartographer — delegate to **libuser** and **cliuser** when the published
surface changed since their last consultation, giving each only the README
and `go doc` output, then hand their reports to the cartographer to fold
into issue bodies. A hand-back with no report in it — empty, or a
placeholder — is a persona round not yet delivered: resume or re-run it
(#2513). Their findings are banded like any product issue; a pass
that consults while earlier findings stand unconsumed says why. A cartographer subagent cannot
spawn them, and it has already read the source, so a persona it
role-played itself would launder an insider's opinion as an outsider's
(#416).

Then delegate a log entry to **chronicler**, commit as
`meta: backlog <date>`, and land it by opening a PR and squash-merging it
in this same session — never leave the commit on an unmerged branch. No
code changes in this trigger.
