# Development Workflow

The rules every goxsd8 session obeys, whatever command it is running. The
commands themselves live in `.claude/commands/`; this file does not repeat
them.

goxsd8 is developed by Claude Code sessions — scheduled routines and
on-demand local runs execute the **same slash commands**, so automated and
interactive behavior are identical by construction. A session is short;
the repo and its GitHub issues are the brain.

## What survives a session

**The container is ephemeral.** A run may start from a fresh clone: local
git state — stashes, dirty trees, local-only branches, scratch dirs — does
not survive. Hence **anything not pushed does not exist** (PRINCIPLES 28).

Three things are durable, and everything a later session needs must be in
one of them:

- **The issue thread** — groundings, verdicts, RESUME notes, decisions.
- **`docs/LOG/<year>-<month>.md`** — what happened and what it cost.
- **Pushed branches and `main`** — the code.

A transcript is not durable. Neither is a local path: never point at one
in a comment.

For issue operations use whichever GitHub channel works — docs/ROUTINES.md
ranks them and states the fall-through rule. **A subagent's sandbox
carries its own channel, and it is regularly not the orchestrator's**: an
agent that cannot post returns the text it would have posted, and the
orchestrator posts it saying whose it is. A verdict, grounding,
implementation account or RESUME note that reaches no thread is lost,
whoever's fault the channel was.

## Claims that outlive the session

An issue body, a thread comment, an implementation account, a verdict, a
commit body and a log entry are all read later as fact. Before a claim
about the tree, the queue or a measurement enters one:

- **Open the source and quote the words** — the spec file for a rule ID,
  the cited `file:line` for a code claim, the issue itself for a claim
  about a body. This binds hardest when you are contradicting the source:
  *"the body is wrong about X"* is itself a claim about the body, and a
  verdict is where it becomes durable (#958). Memory of prior discussion
  is not reproduction, and neither is another artefact's account of the
  same source.
- **A citation resolves by KIND as well as number** — a `cvc-*` Validation
  Rule lives in a different subsection from a Schema Component Constraint,
  and CLAUDE.md's headline numbers are not STYLE IDs. Point at a site by
  its `GAP(...)` marker's text, its enclosing identifier, its heading or a
  quoted sentence, never by line number: a line number into a file the
  previous landing touched has a one-session shelf life. An edit that passes
  over a line citation converts it rather than re-anchoring it (#1356).
- **Take a figure from the instrument that produces it** — the exported
  surface from `go tool surface`, the branch namespace from `go tool
  wipsurvey`, a case count from a run. A `Ratchet:` trailer naming
  absolute lane endpoints pastes them; a writer with no instrument in
  reach states the delta alone and says that is what it is (#796). A
  measured behaviour names the entry point and the commit it was measured
  at — `parser.Parse` at `40eaeac` — because a fixture legal at one entry
  point can be rejected at another (#1629).
- **A set taken from the tree or the corpus states how it was taken** — the
  command, the paths or globs it ran over, whether tests were in, and the
  commit — and is read as a lower bound any later round may widen. A body
  that scopes work over such a set decides done by the command that
  re-derives it, never by the count (#912, #1485). A claim that the suite
  does or does not reach a construct carries its census, or is written as
  unchecked (#510, #679).
- **A lane's score is the census of
  `conformance/testdata/expectations/<lane>.txt` — its `pass` and `fail`
  line counts, which `go tool lanestatus` prints. A `docs/LOG` entry and a
  `Ratchet:` trailer quote that figure and no other.** It is the only lane
  figure the ratchet writes, so it is the only one a later session
  re-derives from the committed tree alone. A conformance run prints other
  per-lane counts — a decline census partitioning ONE RUN's failures,
  improved-but-unbanked and sanctioned removals offering movement it has
  not banked — and none of them scores the lane, so subtracting one
  instrument's delta from another's baseline yields a figure no instrument
  ever printed (#1120).
- **Anything unchecked is written as a hypothesis**, in the sentence that
  makes the claim, not in a caveat elsewhere. A claim carried over from
  another thread keeps its author's qualifiers verbatim: "for the shapes I
  could construct" is a bound, and quoting it somewhere more authoritative
  does not make it a finding (#550).
- **An instruction is checked against the tree it acts on, not the tree it
  was written against.** On absorbing `origin/main` — a merge forward, a
  resumed branch, a takeover — diff the absorbed range for `CLAUDE.md`,
  `docs/`, `.claude/`, and carry any change to the rules into every brief
  written after it. Before acting on an instruction that names a tree state
  — a pre-committed amendment, a brief, a body's site list — re-read the
  site in the tree you are about to edit; where they disagree the tree wins
  and the divergence is said on the thread (#609).

## The branch scheme (the WIP discovery index)

The remote branch namespace is a machine-readable index. Any session
reconstructs the in-flight state with one command:

```sh
git ls-remote --heads origin 'refs/heads/wip/*' 'refs/heads/parked/*'
```

**Read the remote, not the local remote-tracking refs.** GitHub deletes a
branch at merge, but `refs/remotes/origin/wip/*` keeps a stale entry until
pruned, so `git for-each-ref` invents in-flight work. Fetch with
`--prune`; `go tool wipsurvey` reads the remote regardless.

| Branch | Meaning | Lifecycle |
|---|---|---|
| `main` | always green; receives only squash-merged PRs | permanent |
| `wip/issue-<N>` | THE work branch for issue #N — at most one, name is stable | created when work starts; auto-deleted by GitHub on merge; retired in place if abandoned |
| `parked/untriaged-<YYYYMMDD-HHMMSS>` | unattributable work found in a dirty local tree | kept for human triage |

A maintenance command lands from a short-lived branch of its own; it opens
and squash-merges its PR in the same session, so it holds no lease and
claims no issue. `go tool wipsurvey` still prints it, ahead of main, in its
section for heads outside `wip/*` and `parked/*`, for as long as
it is landing: that section is cleanup for a human, never a lease row, so a
branch listed there is not in-flight work.

Invariants:

- **The name is the claim; the tip time is the lease.** `wip/issue-<N>`
  existing means #N has an in-flight attempt. Whether that attempt is
  alive is the tip's timestamp
  (`git log -1 --format=%cI origin/wip/issue-<N>`) against the **2h claim
  TTL**: newer is LIVE — off-limits, and so is its issue; older is
  EXPIRED — resumable.
- **Checkpoint pushes are the lease heartbeat.** A long step pushes
  intermediate commits rather than letting its lease lapse, and a step
  that produces nothing to commit pushes `git commit --allow-empty` instead.
  **Push one immediately before every delegation**: the orchestrator cannot
  push while a subagent round runs, so the round's whole life must fit
  inside the TTL that push starts (#1493).
- **A branch with no commits of its own — a claim pushed without its
  claim commit — is dated by its thread.** Its tip
  is the landing it branched from, so no tip-age reading may date it or
  retire it (#722); **the newest thread comment whose body opens with
  `RESUME:` or `TAKEOVER:`** dates it, against the same 2h TTL. No other
  comment does: a `GROUNDING:`, a verdict, a `MASON:` account and a
  planning note record work already done, not a session still holding the
  branch. With **no such comment ever posted** the claim is not takeable:
  it is pushed before its first heartbeat, so an undated claim is as likely
  seconds old as abandoned, and it stays CLAIMED until a human or a session
  settles it from the thread.
- **Taking over is how every resume begins**, an EXPIRED branch as much as
  a takeable CLAIMED one: post the `TAKEOVER:` comment naming the branch
  tip you found and its age, then push the heartbeat before any other work.
  The ref is then the arbiter of the race, and a rejected heartbeat means
  another session holds the branch: fetch and pick again (#867, #1469). The
  comment states what you read — tip, age, diff against main — never a
  cause you did not measure: a lapsed lease cannot tell a dead round from a
  slow one.
- **Races are settled by git's atomic ref updates, never by force.** A
  rejected push to `wip/*` means you lost the race: fetch, abandon the
  local attempt, pick something else. Only a push carrying a commit the
  ref lacks can be rejected — a push of the SHA the ref already holds
  succeeds — so every claim and every takeover pushes a commit of its own
  (`develop.md` step 2, #1743). Force-pushing `wip/*` or `parked/*`
  is forbidden — it is the one way sessions could stomp each other.
  Sessions only ever CREATE refs; the single deletion in the system is
  GitHub's auto-delete on merge.
- **A `wip/` branch is work only while its issue is open and not
  `needs-replan`.** Otherwise it is retired: landed branches vanish on
  merge, abandoned ones stay as re-planning evidence. A retired name is
  never contended, because an issue is never re-attempted under its own
  number — re-planning supersedes it with a new issue, and the fresh
  attempt starts as `wip/issue-<M>` from `origin/main`. A human operator's
  direction on the thread is the one thing that may send a session back to
  a parked branch; no session decides that for itself (#648).
- **Freshly-fetched `origin/main` is the only base.** Every diff, merge
  and branch point that names `main` is taken against `origin/main` after
  an explicit `git fetch origin main` in THIS session; a local `main` in
  an ephemeral container is stale, and diffing against it invents changes
  that are not there while hiding ones that are.
  `git diff origin/main...HEAD` is the shape that works. Equally: judge
  the COMMITTED tree — if `git status --porcelain` is non-empty, what was
  verified is not what will land. A session that opens on an old checkout
  moves to the fetched `origin/main` before it reads a rule or takes a
  measurement: `CLAUDE.md` read and `lanestatus` run on a stale tree
  answer for that tree.

## Checkpointing and hand-off

**Checkpoint at every step boundary, and before ending any session:**

```sh
git add -A && git commit -m "wip #<N>: <step completed>"
git push origin wip/issue-<N>
```

Add a `RESUME:` comment on the issue whenever the next action is not
obvious from the branch alone — last completed step, the exact next
action, and where the grounding is. On a branch with **no commits of its
own**, post one at every checkpoint whether or not the next action is
obvious: it is the only thing that dates such a branch's lease, and
without it the claim reads as undated for as long as it lives. The branch
carries the CONTENT; the comment carries the INTENT. Discovery never
depends on the comment.

Never destroy uncommitted work: `git clean`, `git restore .`,
`git checkout -- <file>` and stashing of any kind are forbidden. A dirty
local tree is pushed to `parked/untriaged-<ts>` and logged.

Ending early at a checkpoint — time budget, second reject, a wall — is a
first-class outcome, not a failure.

## One writer per checkout

A `wip/issue-<N>` checkout has exactly one writer: the orchestrating
session. Two writers in one tree is undefined behavior, not a discipline
problem — every gate command and every commit reports whatever is in the
tree at that instant, and the writers interleave at moments neither
chooses (#350).

- Only the orchestrating session's own git commands write into the
  `wip/issue-<N>` checkout.
- **Any subagent that writes files, runs a probe or runs the gate gets
  worktree isolation** — the harness's `isolation: "worktree"` option —
  whatever it writes: the invariant is about the tree, not the file type,
  so the chronicler's LOG entry, a grounding arbiter's probe and a
  warden's diff-review gate are isolated exactly as a mason round is
  (#1392, #2251). A mason may also break lines deliberately at any moment
  to check that a test notices. An isolated agent writes and moves HEAD in
  its own worktree only: never `git checkout` or `git switch` in a
  checkout a brief names. The worktree's branch is local-only and never
  pushed; the two halves of a `/retro` each get their own.
- The orchestrator never commits on a live subagent's behalf, for any
  reason — including a stop-hook "uncommitted changes" warning, which
  fires on an in-progress edit exactly as readily as a finished one
  (#296). A subagent's tree is commit-ready for hand-off only once it
  reports.
- **Commit-as-you-go: the subagent commits each self-contained step as it
  reaches it, not once at the end.** Its container can go away mid-round,
  and a commit on its worktree branch is the only part of an interrupted
  round a later session can read — the clause above forbids anyone else
  rescuing what it left uncommitted. A mason still breaks lines
  deliberately, and commits once it has put them back; the hand-off below
  takes the branch's commits as they stand, so there is nothing to tidy
  into one (#1443).
- **After that report the isolated branch's own commits go onto
  `wip/issue-<N>`, never a replay of them.** `git merge --ff-only` where
  it applies; where it does not — the ordinary shape after a takeover,
  whose mandatory heartbeat is a commit `wip/issue-<N>` has and the
  worktree, cut from `origin/main`, does not — `git merge --no-ff`. No
  cherry-pick, no rebase, no re-edit by hand: each rewrites the SHAs
  the subagent's account names, so a reader checking that account against
  the branch finds nothing (#1099). Then discard the worktree and
  checkpoint. To put any branch or worktree at a given commit, `git merge
  --ff-only <sha>` when the branch is that commit's ancestor, otherwise
  `git checkout -B <branch> <sha>`: `.claude/settings.json` denies `git
  reset --hard` (#2223).

**The orchestrator holds the pen only for words already written on the
thread.** Its edit transcribes a verdict's or a ruling's text, or is
mechanical — a reflow, a renumbering, a SHA — and its commit body links the
comment it transcribes. Composing new text is design content, in a `.md`
file as much as a `.go` one, and is a mason round (#1442).

**Every orchestrator edit is a new commit.** Amend and force-push of a
pushed `wip/*` or `parked/*` ref are forbidden without exception —
including one's own last commit, including to fix a commit message. An
amended pushed ref leaves the judge no baseline to check the account
against once the container is gone (#649). Disclose a breach on the thread
and record it in the log; it is a process note the orchestrator owns, not
a finding against the diff.

## Scope: what one landing carries

A landing carries its issue's change plus whatever that change makes
necessary. Finding adjacent work mid-implementation is normal and
expected. The question is never "was this in the issue body" but **"does
this need its own grounding, its own surface review, or its own ratchet
attribution?"**

- Needs a rule ID the thread's GROUNDING does not cover → separate issue.
- Adds exported surface the warden has not seen → warden pass now, or
  separate issue.
- Could move a conformance lane on its own → separate issue, so the
  movement stays attributable (PRINCIPLES 22).
- None of those → **absorb it**, and name it in the commit body so the
  arbiter reads it as intended scope rather than creep.

Deferring is not free: a follow-up costs a whole landing and opens a
hand-off where premises decay. A rename, an unexport, a stale comment, a
call site the change itself breaks — all are cheaper absorbed than filed.
Judge the diff by whether a reviewer can hold it at once, not by whether
it matches the body.

**Ordering is not splitting.** A review may find that one part of a change
must precede another. That is satisfied by commit order on one branch.
Split into a second issue only when the later part needs its own grounding
or will not fit the session; otherwise land both and close both from the
one PR.

## Landing

Landing is atomic. Accept → open a PR from the branch and squash-merge it
via the GitHub Merge API. The squash is the session's ONE commit (code
plus log entry), `Closes #<N>` in the body closes the issue, and GitHub
auto-deletes the head branch (keep the repo's "Automatically delete head
branches" setting ON). Nothing is ever committed directly to `main`.

**Write the squash title from the landed change**, never by inheriting a
branch commit's title: a finding against prose that a commit title also
states is a finding against the squash title, and the squash is the one
place it can still be corrected (#1329).

An iteration that lands no code — a park, a stood-down pick — lands its
LOG entry the way a maintenance command does: a short-lived branch of its
own, its own PR, squash-merged in the same session (#797).

**An issue closes by a landing** — a bound `Closes #<N>`, or precondition
4's hand close — **or by the cartographer as obsolete or duplicate**, with a
comment saying why. No other agent closes or reopens an issue; a close
without a landing or a comment is reversed by the orchestrator that finds
it, and said on the thread (#1351).

A landing that closes more than one issue writes ONE `Closes #<N>`
sentence per issue — `Closes #669. Closes #625. Closes #748.` — never the
comma form `Closes #669, #625, #748.`. GitHub binds a closing keyword to
the single reference immediately following it, and every later item of a
comma list is a plain mention that closes nothing.

To name an issue WITHOUT closing it, write **names and leaves open** —
never a closing keyword (`close`, `closes`, `closed`, `fix`, `fixes`,
`fixed`, `resolve`, `resolves`, `resolved`) in a negated clause or a
list heading. The binding holds whatever the surrounding prose means,
and a line break does not break it (#1034). GitHub closes from the squash
text and the PR description both, so `go tool landcheck` reads the two
planned texts before the PR opens (`-squash <file> -pr-body <file>`) and
exits 1 on the comma form or on a keyword whose bound `#<N>` a *names and
leaves open* or *not closing keywords* clause also names (#1178).

The LOG entry comes due here and nowhere earlier. `/develop` step 5
(Judge) runs before step 6 (Land), where the chronicler writes it, so the
tree the arbiter judged carries no entry for this issue and an empty
`docs/LOG/` diff at verdict time is the documented order rather than a
skipped step (#820).

Four preconditions, verified and stated by the orchestrating session —
the first three before the PR is opened, the fourth after the merge. None
is anyone else's to volunteer:

1. **The LOG entry naming THIS issue is among the branch's added lines** —
   `git diff origin/main...HEAD -- docs/LOG/` piped through
   `grep -E '^\+.*(\(#<N>\)|issues/<N>|#<N>)'` for the issue number being
   landed. Not "the chronicler was invoked", and not a non-empty
   `docs/LOG/` path diff, which a forward merge fills with other issues'
   entries (#813). The entry rides the session commit or the session does
   not land (PRINCIPLES 29). A pass that closes no issue — a `post-land`
   stamp, a `/backlog` run — has no number to grep for: its entry is due
   by the same rule and this check has nothing to say about it. `go tool
   landcheck -issue <N> -squash <file> -pr-body <file>` runs this
   precondition, its history check — the same diff deletes no line
   `origin/main`'s `docs/LOG/` carries and inserts only whole entries
   between base entries, a dated correction exempt (#2506) — precondition
   2's merge-base-current requirement and the closing-keyword check above
   (#963) — exit 0 clean, 1 if the grep, the history check or the keyword
   check fails, 2 if the base is stale or the tool cannot run.
   It reads the pushed head, not the local `HEAD`: it exits 1 while
   `git rev-list @{u}..HEAD` is non-empty and 2 when `HEAD` is behind its
   upstream or has none, and a hand-run of the grep runs that `rev-list`
   first (#1674).
2. **`origin/main` has not moved past the verdict's base** —
   `git log HEAD..origin/main` is empty. If it is not, merge forward and
   re-judge per **After the verdict**, then re-verify: main can drift
   again while the PR is open.
3. **Every implementation commit on the branch is dispositioned** — the
   thread's comments read back over the GitHub channel, not "mason
   reported". The set to iterate is what
   `git log --no-merges --format='%h %s' origin/main..HEAD -- ':(top,exclude)docs/LOG/'`
   yields; merge commits, the chronicler's LOG-only commit and
   `--allow-empty` lease heartbeats fall out of it and owe nothing, and the
   `(top)` magic is what holds that answer steady from a subdirectory.
   Every commit it does yield is covered by some `MASON:` comment on the
   thread, or the orchestrator states here which thread comment its pen
   transcribed (**One writer per checkout**). A commit that is neither is a
   mason round that was skipped, and the branch does not land (#1227). A
   `MASON:` comment covers a commit only if it carries the account itself —
   a body that is a local path carries nothing (**What survives a
   session**, #992) — and every SHA it names resolves in this checkout
   (`git cat-file -t <sha>`): an account naming a SHA that resolves nowhere
   describes work that does not exist, and the thread says so (#1355). A repair round's own comment dispositions every numbered item of
   the verdict it repairs. An account that was never written is a mason
   round, never the orchestrator's to compose (#565).
4. **Every issue the squash body names reads the state the body meant**
   — AFTER the merge, not before it (#1040). The iteration set is every
   `#<N>` reference the squash
   commit text names, subject and body, bound closing keyword and plain
   mention alike — `git log -1 --format=%B <squash-sha>` piped through
   `grep -oE '#[0-9]+' | sort -u`. Not the `Closes` sentences, which the
   comma form collapses to the single bound reference, and not `/develop`
   step 2's pick, which is one issue by construction while absorbed work
   is written in the commit body and nowhere else. Read every number that
   extraction yields back over the GitHub channel, and state `closed` or
   `open` for each. The failure is silent on both sides — the merge
   succeeds and the body reads as intent — and its signature is the first
   named issue closed with every later one of the same list open, which IS
   the comma form; a first named issue that is itself open is an unbound
   keyword of some other kind (#993). Close by hand whatever reads `open`,
   or state why the landing named it and did not close it; reopen by hand
   whatever reads `closed` that the body named and meant to leave open.

**A PR that closes no issue is verified by the agent that opens it**,
which verifies and states these preconditions in the orchestrating
session's place — the cartographer for its own `post-land` PR.
Precondition 1 already says what it has to say here. `go tool landcheck
-no-issue -squash <file> -pr-body <file>` checks the pushed head,
precondition 2 and precondition 1's history check as above, and exits 1
when the history check fails or on every bound closing keyword in either
text, since a PR of this shape closes nothing (#1178).
Precondition 2 binds with nothing keyed on a verdict: `git log
HEAD..origin/main` is empty, and a branch that is behind merges forward
and re-runs the gate.
Precondition 3 does not bind — the pass is held end to end by the agent
that opens it, and there is no owning issue thread for a `MASON:` comment
to be on; it composes its own account into the LOG entry and the PR body
instead. Precondition 4 runs its full procedure here, every `#<N>` the
squash body names read back alike: a body of this shape names issues in
order to record what it unblocked or disposed of, so `open` is an expected
reading, disposed of by stating why the landing named it rather than by
closing it by hand.

### After the verdict

A verdict measures a tree, and the tree that lands must be that tree.

**Merge `origin/main` forward before delegating the judgment.** Main
drifts under most branches, and a moved base costs nothing before a
verdict exists and a round after — so the only drift worth pricing is what
lands inside the verdict's own window. A merge forward is never a rebase
(force-push is forbidden, so a merge is the only mechanism); it names the
absorbed SHAs in the log entry and the PR body, re-runs the FULL gate on
the committed merged tree — a conflict-free auto-merge included, because a
clean merge is not evidence of a compatible one (#392) — and checks the
absorbed range for changed rules, the gate among them, per **Claims that
outlive the session**.

Anything that moves the tree after the verdict — a merge forward, a late
finding, a fix turned up while writing the log — is judged by one
question: can this change what the verdict measured?

- **The diff changed** → a full new round.
- **Only the base moved** → a gate-only round, and only when the verdict
  rests on a measurement that base can invalidate: a banked ratchet
  figure, a lane delta. A green gate just re-run is not such a
  measurement.
- **Provably nothing measured changed** → land it. Only text no compiler
  reads qualifies, proved mechanically: the diff since the accepted commit
  touches no line outside a comment or a `.md` file. It is its own commit,
  carrying that proof in its body; re-run CLAUDE.md's gate, whole, and
  move no ratchet figure (#1993).

What no class admits goes to the post-land ledger.

**A repair round is none of these classes.** The three are available because
an accepting verdict has already measured a tree; a repair produces the tree
no verdict has measured yet, so it is re-judged in full — gate and ratchet —
however little it touched, and the arbiter runs the ratchet on accept
regardless. A prose-only repair costs a whole round by design, not by
omission (#1105).

## Merge-conflict resolution

The one path where the checkpoint ritual's `git add -A` is not what
happens. Conflict resolution stages file by file, and a merge also stages
what it brought in cleanly, so any fix made *after* that staging is
invisible to `git commit`:

```sh
git merge origin/main
# resolve every conflict — AND make the follow-up fixes the merge implies
# in files it staged for you (a renamed symbol, a moved import)
git add -u                  # re-stage EVERY touched file, after the LAST edit
git diff --cached --stat    # mandatory: this is the tree that will land
git commit
```

`git diff --cached --stat` is a named step, not a sanity check: the whole
gate reads the working tree and nothing reads the index, so a fix made
after `git add` is silently reverted by the commit while every check
reports green (#179). `.githooks/pre-commit` refuses a commit where a path
has both staged and unstaged changes — activate it every session, since
local git config does not survive a fresh clone:

```sh
git config core.hooksPath .githooks
```

**The `docs/LOG` tail is the conflict every pre-land merge hits**, and the
one file where a plausible resolution silently deletes landed history.
Both sides are almost always pure appends, so the resolution is positional
and nothing else: base, then the other side's entries in their own order,
then yours, each byte-identical to its authored form — nothing reflowed,
reordered or tidied. Then prove it by reconstructing each parent from the
resolved file (`git show :2:<path>`, `git show :3:<path>`); each must
appear as an unbroken, unedited run. Heading arithmetic passes while an
entry is silently dropped.

## Parking

On a second arbiter rejection, a third subagent round lost to a container
restart on one issue, a `wip/` branch whose `go tool wipsurvey` reason
names its repeated empty-diff `TAKEOVER:` count (#1437), or a resume
whose merge will not resolve: checkpoint the branch one final time,
**relabel** the issue `needs-replan` and clear `ready`, and comment the
findings that killed the attempt. A surveying session that parks on the
empty-diff trigger has nothing to checkpoint and does not take the branch
first — taking it would be one more empty holding — so it relabels and
comments where the branch stands (#1492). Nothing is renamed or deleted —
the label alone retires the branch in place as re-planning evidence. **Two
rejections is the hard cap** (PRINCIPLES 30); never solicit a third
round. **Three lost rounds is the hard cap on the other trigger**:
nothing is known to be wrong with the change, so the finding to comment
is that the round does not fit a container's life, and the replacement
issue is a smaller one (#1443). The cap counts the lost-round statements
on the thread (`/develop`'s **Lost rounds**). After re-planning, the
cartographer files the replacement, names it on the parked thread, and
closes the `needs-replan` issue `not_planned` — never `completed`, which
marks work that landed (#493).

## GitHub conventions

**Labels**: **exactly one queue label** — `ready` (unblocked, sized for
one session), `blocked`, `needs-replan` or `epic` — plus an `area/` naming
the package or subject, spelt as the package is: `area/{xsd,xsderr,parser,`
`value,builtin,xpath,icpath,validate,codegen,codec,regex,loader,`
`conformance,cmd,docs,meta}`, and a
`kind/{feature,gap,bug,refactor,process,tooling,story,docs}`. This list is
the only one; other documents point here (#1019). Milestones mirror
docs/PLAN.md. `blocked` means waiting on a named dependency recorded in
`## Depends on` — an issue or a trigger, not only an open issue. The queue
labels are exclusive because the surveys read them that way: an issue
wearing two is pickable and retired at once, and nothing this repo runs
can see the contradiction (#968).

**The body states the decision; the thread holds the reasoning.** A body
carries goal, spec rule IDs, acceptance, surface and dependencies — enough
that an agent can start from it alone — and stays short enough to read at
once. Verdicts, groundings and analysis belong in comments; transcribing
them into the body is what produces issues longer than this document.

**Filing discipline** — a defective body outlives the session that filed
it (#315). What a body may assert is governed above, under Claims that
outlive the session; these are the rules particular to writing one,
applied before filing and again before grounding something already filed:

- **Correct a stale or wrong premise in the body**, not only in a comment;
  the next reader starts from the body. The comment stays as provenance.
- **Write back only a body read byte-faithfully and whole** — through
  repository-scoped REST (docs/ROUTINES.md). The MCP `issue_read` path
  strips angle-bracketed tokens, escapes punctuation, and truncates a long
  body without a marker, headings included (#764, #1014). The escaping is
  reversible; the stripping and the truncation are not. Where only that
  path is available, a write is blocked unless the copy is provably
  complete and every stripped token re-derives: otherwise comment on the
  thread and name the section the body still gets wrong. **A body known to
  be wrong is never landed silently.** `WebFetch` on the issue URL returns
  a small model's summary of the rendered page, never the stored body.
- **Search the open queue** for the primary file path and identifier, the
  way docs/ROUTINES.md says makes an empty answer mean something (#893). A
  hit is either a duplicate (close one, say which) or an adjacent issue
  (cross-reference both). Never pass a hit silently.
- **An `## Acceptance` ratchet promise states lane movement and names its
  condition** ("moves the `schema` lane **provided** #N has landed"), so a
  later re-plan leaves the staleness one grep away. A body never quotes an
  absolute banked figure; where one is needed it names
  `conformance/testdata/expectations/<lane>.txt` as the thing to read at
  work time (#646).
- **A marker arm never asks this issue's landing to cite this issue.**
  It says to remove the marker where the landing closes the gap, and
  names a separate open tracker for any part left open; `Tracked by
  #<this issue>` is a dead end the moment the issue closes (STYLE P3,
  deliberate gaps are tracked; #2308).
- **An `## Acceptance` bullet is ruled before anything is implemented
  against it** — `/develop` step 3, which owns the questions that ruling
  asks and the shape it is posted in. Writing the bullet correctly at
  filing time is not that ruling.
- **A body cannot add a step to the loop.** The command files are the only
  list of steps, exactly as CLAUDE.md's block is the only definition of the
  gate: a body invokes a review by filling the field the loop reads —
  `## Surface`, which step 3 rules against the tree — never by prose saying
  to expect one. Body prose is advisory whatever it asserts, so a step
  wanted and not invoked is a step nobody owes (#692).

`// GAP(...)` and fail-open sites get `kind/gap` tracking issues, so
nothing fails open silently forever.

For a table that regenerates spec-derived output, do not make
"byte-identical regeneration" an acceptance criterion unless a grep first
confirms the committed table already holds every member the widened
generator admits — regeneration correctly ADDS missed entries, which is
usually the point. The right criterion is "additive-only, and every added
entry is a genuine spec member."
