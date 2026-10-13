---
description: One develop iteration — survey the WIP index, resume or start an issue branch, ground, implement, judge, land. Stop after one issue.
---

One iteration, then stop. You are the orchestrator: you delegate all
specialist work, you never skip the arbiter, and you never wait for a
human — abort hanging commands and log the failure.

docs/WORKFLOW.md is normative for the branch scheme, checkpointing, scope,
landing and parking; this file is the sequence. **Checkpoint (commit +
push) at every step boundary** — it is both the crash guard and the lease
heartbeat.

**Before every delegation**, re-read the issue's state and
`git ls-remote --heads origin wip/issue-<N>`: a closed issue or a vanished
ref ends the iteration, whatever the survey said when it ran (#1460). Then
push the heartbeat WORKFLOW's lease invariant requires before a round.

**A brief carries the issue and never narrows the delegate's own file.**
Paste the body's `## Goal`, `## Spec` and `## Acceptance` and every thread
comment that declares itself normative into the request verbatim: the
oracle has no tool that reads GitHub (#2142). A judge round's brief also
pastes the `GROUNDING:` comment and the latest `MASON:` account whole, and
a round-2 judge brief the round-1 verdict whole, so the arbiter states its
outcome with no GitHub read of the thread (#2449). A brief names the gate
only as CLAUDE.md's gate, whole (#1959); never bars or reshapes a duty the
delegate's agent file assigns it, the arbiter's ratchet run on accept
above all (#1812); never asks a subagent to push, since its commits leave
its worktree only through WORKFLOW's hand-off (#2133); and never names a
`Co-Authored-By` model — each commit names the model that wrote it
(#1996). Re-read a brief before sending it: a placeholder where pasted
text belongs sends nothing (#2513).

**Lost rounds.** A delegated round — mason's, the arbiter's, any agent's —
is lost when it ends without its report: the agent returned without one,
a placeholder counting as none, or the session that delegated it is gone.
Elapsed time is not a signature; a round still running is not lost
however long it takes (#1426), and an interim notice that the agent is
waiting on its own background work is neither its report nor a loss:
wait for the report (#2250). Re-delegate a lost round from the thread as
it stands, never re-grounding: a mason round from the `GROUNDING:`
comment, naming the dead
worktree in the new prompt if it is still on disk with commits so the
round resumes from them; a judge round from the tree it was judging.
**Every re-delegation after a loss posts a `RESUME:` or `TAKEOVER:`
comment naming the lost round and its evidence, whatever the branch
holds** (#1468) — those comments are what WORKFLOW's **Parking** cap
counts.

1. **Survey.** `git config core.hooksPath .githooks` (idempotent, and a
   fresh clone carries no local config), then `git fetch --prune origin`
   — without `--prune` the local view keeps refs for branches GitHub
   deleted at merge. A dirty local tree gets pushed to
   `parked/untriaged-<YYYYMMDD-HHMMSS>` and logged — never cleaned.

2. **Pick.** Run `go tool wipsurvey` — CLAUDE.md spells its input — rather
   than re-deriving the branch namespace by hand (#399). It reports each
   `wip/issue-<N>` as LIVE, CLAIMED, EXPIRED, RETIRED or UNKNOWN. LIVE
   branches and their issues are off-limits; RETIRED ones are not work;
   UNKNOWN — and an EXPIRED row whose reason says its ancestry is
   undecided — wants `git fetch origin` and a re-run (#810). A CLAIMED
   branch is dated by its thread's `RESUME:`/`TAKEOVER:` comments, not by
   git. If no channel yields an issue list, run it on empty stdin: the
   lease-only report classifies everything but RETIRED, which is not a
   reason to survey by hand.
   - **Resuming beats starting.** Take the oldest EXPIRED — or takeable
     CLAIMED — `wip/issue-<N>` whose issue is open and not `needs-replan`,
     and take it over per WORKFLOW's lease invariant before anything else.
     Then merge `origin/main` in if main moved (never rebase; if the
     conflicts are not tractable, park it and pick again), read the newest
     `RESUME:` comment, and continue from its "Next:" at the matching step
     below.
   - **Nothing to resume, and the band is stale or nothing is ready →
     plan, then stop.** The band is stale when the newest `meta: retro`
     commit on `origin/main` is newer than the newest `meta: backlog` one
     (`git log -1 --format=%s -E --grep='^meta: (retro|backlog)'
     origin/main` answers it), or when none of its rows is open: a retro
     re-ranks the queue only through the next stamp, so a session picking
     from the old band spends itself on the order the retro replaced.
     Delegate a `/backlog` pass to **cartographer** and land it as
     `.claude/commands/backlog.md` does.
   - Otherwise claim the highest-priority `ready` issue — `docs/PLAN.md`'s
     Working band orders them — that reads `open`, whose dependencies are
     closed, with no live branch, and whose `## Notes` names no same-file
     relation to an issue with a LIVE branch (#1111). A closed issue can
     still wear `ready`, and the band keeps landed rows until its next
     stamp, so read each row's state from the issue list that fed
     `wipsurvey`, never issue by issue (#2399). Claim with a commit only
     this session could write, then push it:

     ```sh
     git switch -c wip/issue-<N> origin/main
     git commit --allow-empty -m "claim #<N> $(date -u +%FT%T.%NZ)"
     git push -u origin HEAD
     ```

     The push is the claim, and a rejected push means you lost the race —
     fetch and pick again. Without the commit a second claimer at the same
     `origin/main` tip pushes the SHA the ref already holds, which git
     answers "Everything up-to-date", exit 0; a create-only
     `--force-with-lease=<ref>:` push answers the same (#1743).

3. **Ground.** Read the whole thread first: a comment that declares itself
   normative for the issue binds as the body does (#654). Search the open
   queue for this issue's primary
   file path and identifier and record what any hit was (duplicate or
   adjacent).

   Before delegating the oracle, run `git submodule update --init
   testdata/xsdtests` in the checkout it reads (#1989).

   Delegate the grounding arbiter with worktree isolation: its probes are
   writes (WORKFLOW **One writer per checkout**). Grounding is posted as
   ONE `GROUNDING:` comment — the only durable copy — with each block
   headed by the agent that wrote it:
   - the **oracle** answers the spec — the clauses and rule IDs in scope —
     and rules each `## Acceptance` bullet whose truth turns on spec text.
     An issue whose `## Spec` is n/a has no oracle block, and says so.
   - the **arbiter** rules every other `## Acceptance` bullet, rules
     `## Surface`, and makes the PREDICTION: each of those is a fact about
     the tree or the corpus, answered by running an instrument the oracle
     has no tools for (#1588). The bars it rules are the ones its own
     verdict will apply.

   Grounding rules bars, not designs: where the code goes is mason's.

   **A grounding statement holds only over what the grounding checked.** A
   licence, a precedent named for reuse, a marker's disposition or a
   proposed discriminator is checked against everything it speaks for, and
   says what that was; what was not checked is written as unmeasured, for
   mason to measure before the verdict. That population is:
   - the inputs the probe actually ran, not the change it licenses (#1948);
   - a reused helper's accepted value range, against the new site's
     declared type (#1781);
   - a `GAP(` marker's quoted wording, not the issue's cases, with an owner
     named for any remainder the landing leaves (#1798);
   - every input domain the exported entry point admits — flavours, flags,
     the position an attribute sits at — not only the ones that move the
     ratchet (#1868);
   - what the input the check reads represents: which document
     `xs:override` makes it judge, and which absent or defaulted state
     shares its encoding (#1868);
   - for a fault-versus-limitation discriminator, each behaviour a
     conforming processor must supply that this one lacks, and the bucket
     the discriminator puts a well-formed input exercising it in (#2075);
   - a target case's decline: a `GOXSD_DECLINES=1` token names the exit
     that declined, not the construct, so a case counts as held by the
     issue's construct alone only once its whole `{test}` or document is
     read for a second declining construct (#2582).

   **ACCEPTANCE** — rule every bullet as the filer wrote it, against the
   current tree. Two questions, and a bullet fails on either: **can this
   bar fail?** — one that holds however the change turns out is vacuous —
   and **is what it says TRUE?** A bullet describing the state this
   landing creates is judged on whether a conforming change can make it
   true, not on whether it is true yet; `unsatisfiable` means none can
   (#1601). Where the deliverable is prose the bullet's sentence IS the
   artifact, so a false premise ships (#1188). An "at least" list stays
   open, and narrowing its scope at grounding is not a ruling on it
   (#1050).

   ```
   ACCEPTANCE:
   - "<the bullet>" — satisfiable | vacuous | false | unsatisfiable | n/a, and why
   ```

   A bullet ruled vacuous, false or unsatisfiable is re-stated in the body
   before step 4 starts, and so is any premise a later step falsifies — by
   the orchestrator where a ruling states the replacement text (WORKFLOW's
   pen), by the **cartographer** otherwise (#1380). A re-statement carries
   every standing ruling on the bullets it keeps: a bar ruled unreachable
   does not survive a re-scope unchanged (#1102).

   **SURFACE** — name what this change adds to or alters in the exported
   contract, taken from the tree: whether the contract moves, never where
   the code should go (#812). The filer's `## Surface` is not that ruling;
   the issue whose scope is least understood is the one most likely to
   read `none` (#484).

   **PREDICTION** — turn the `Ratchet:` claim into a lane-movement
   prediction before step 4 delegates. A bullet that measures a population
   and disclaims a prediction is this block's input, not an exemption.
   Census the construct the change's mechanism reads — the element or
   attribute whose presence or value the new code tests — never the
   feature that contains it, and union every element that establishes a
   feature (#1708). Take the excluded remainder with `GOXSD_WITHHELD=1`,
   then join and count as CLAUDE.md's surveys block states;
   `conformance/doc.go` owns what that remainder is. State one of three
   outcomes: a lane and a figure; `unchanged`, which carries the same
   derivation; or `not derived — <what stopped it>`.

   Go below the join's candidate count only by named candidates: each
   discard by case ID, with a reason read from the case itself — its
   fixture, or its test group in the suite catalog. A reason not read from
   the case is no reason, and the candidate stays in the figure; where that
   leaves only a bound, state `not derived — <bound>` (#1657, #1708). Never
   narrow by de-duplicating or pattern-reading `suiteindex`'s value
   strings: a property decided per element or per fixture — whether a
   QName prefix is bound, whether a name resolves, any outcome a layer
   between the producer and `Result.Violations` decides — cannot be read
   off one. The **ratchet prediction** paragraph of
   `.claude/agents/arbiter.md`'s `## Judging` owns what an `instance`-lane
   claim needs.

   ```
   PREDICTION:
   <lane>: +N | unchanged | not derived — <what stopped it>
   <the invocations run, and what each returned>
   <candidates discarded, each by ID and the case text that rules it out — or none>
   ```

4. **Implement.** Where step 3's SURFACE found an exported contract added
   or altered — a new identifier, or a change to what an existing one
   accepts, returns or promises in its doc — have **warden** pre-flight
   the planned shape before any code exists. `go tool surface` answers the
   signature question alone and is not that ruling. Where the body offers
   arms, a `RULING:` comment picks the arm and its constraints and quotes
   any earlier ruling it relies on; it names no implementation site (#1480).
   Then delegate to **mason**, always with worktree isolation, and put
   WORKFLOW's commit-as-you-go clause in its prompt — and in every mason
   prompt this command sends, step 5's repair round and a lost round's
   re-delegation included. The ratchet run is the arbiter's, and no part
   of the gate is it. Only once mason reports, bring its branch onto
   `wip/issue-<N>` under WORKFLOW's **One writer per checkout** hand-off
   clause — fast-forward or merge, never a replay. If the change added or
   altered public API in that same sense, warden reviews the diff too
   (#1168), with worktree isolation, since it runs the gate. Post both
   verdicts on the issue.

   Mason may absorb adjacent work under docs/WORKFLOW.md's scope rule.
   Absorbed items belong in the commit body, not in a new issue.

5. **Judge.** `git fetch origin main` first and merge it forward if it
   moved — before the arbiter, per WORKFLOW's **After the verdict**, where
   it costs no round instead of one. After every merge forward that
   precedes an arbiter round — this one, and the re-judge step 6's
   Landing precondition 2 calls for — run `go vet ./...` on the committed
   merged tree before you delegate; it type-checks the test files
   `go build` skips, and must exit 0, else make the follow-up fixes the
   merge implies per WORKFLOW's **Merge-conflict resolution**, as a
   follow-up commit and never an amend (#2026). Then delegate to
   **arbiter**, with worktree isolation like any subagent that writes: on
   accept it runs and banks the ratchet as its agent file requires, and its
   bank commit comes onto `wip/issue-<N>` under the same hand-off clause as
   mason's (#1812). On reject: one repair round by mason, briefed from the
   whole posted verdict, each defect class's site list carried whole with
   the sites it rules sound (#2449) — never from a partial finding set, and
   never before the verdict is posted (#1426) — then re-judge in full. On a
   second reject: park per WORKFLOW, then go to step 6's log entry and
   stop. On accept, dispose of the verdict's remaining findings per
   WORKFLOW's **After the verdict** before step 6.

6. **Land.** Delegate the log entry to **chronicler** first — isolated,
   and brought onto the branch like any subagent's commits — so it rides
   the session commit. Verify each of WORKFLOW's **Landing** preconditions
   yourself, by the check it names: the first three before you open the PR
   and squash-merge it via the Merge API using CLAUDE.md's commit format,
   the fourth after that merge. A PR may close more than one issue when a
   landing carried more than one.

7. **Post-land.** Delegate to **cartographer**: unblock whatever this
   landing unblocked, and dispose of every follow-up this session raised
   while it is still fresh. Tell it in the same prompt that its
   `post-land` log entry lands through its own PR — commit locally, open a
   PR, squash-merge it — never a push straight to `main` (#1109). Before
   the session ends, read that PR's state back: an open one is yours to
   land, merged forward with WORKFLOW's positional `docs/LOG` resolution,
   because an entry left on a branch is read by no later session (#1897).

Ending early at a checkpoint with a good `RESUME:` comment is a successful
session. Budget: one landing.
