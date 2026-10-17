# goxsd8 — Agent Instructions

You are working on **goxsd8**, a conformance-grade XSD 1.1 processor in Go
(module `github.com/kud360/goxsd8`). This repo is developed primarily by AI
agents. Follow this file exactly; it wins over your own preferences.

## The one rule that outranks everything

**Never regress the ratchet.** Conformance expectations live in
`conformance/testdata/expectations/`. Scores only move up. If your change
makes a previously passing case fail, either fix it or revert your change,
unless the arbiter banks it as a superseded pass (`.claude/agents/arbiter.md`,
"Ratchet integrity").
Never edit an expectations file downward to make CI green.

## Ground truth

The specs are LOCAL, in `docs/specs/md/`. Never guess spec behavior from
memory — grep the spec, and cite rule IDs (`cvc-complex-type.2.1`,
`cos-st-restricts`) in code and commit messages.

| File | Subject |
|---|---|
| `xmlschema11-1.md` | Structures |
| `xmlschema11-2.md` | Datatypes (Appendix E hfn definitions are the source of truth for builtin types) |
| `xpath20.md` | XPath 2.0 |
| `xpath-functions.md` | F&O — the function library and regex flavor |
| `xsd-precisionDecimal.md` | precisionDecimal |

**Each document below owns its subject, and none restates another.** When
two texts disagree, the owner wins and the other is the bug.

| Document | Owns |
|---|---|
| `docs/ARCHITECTURE.md` | package graph, boundaries, dependency direction |
| `docs/STYLE.md` | style rules — cite by letter ID (`STYLE T4`) |
| `docs/PRINCIPLES.md` | invariants and their rationale (`PRINCIPLES 9`) |
| `docs/WORKFLOW.md` | the rules every session obeys, whatever it is doing |
| `.claude/commands/<cmd>.md` | what that command does, step by step |
| `.claude/agents/<agent>.md` | that agent's standard, domain, and judgment |
| `docs/PLAN.md` | roadmap and milestones |
| `docs/ROUTINES.md` | schedule and environment requirements |
| `docs/LOG/<year>-<month>.md` | what happened, and what it cost |

## The gate

```sh
go build ./... && go test -skip '^TestConformance$' ./... && go vet ./...  # part 1
go tool lint                                          # part 2 (STYLE lint subset)
go tool commentwrap ./...                             # part 3 (-fix reflows)
go test ./conformance -run TestConformance -count=1 -timeout 30m -v  # part 4 (-v surfaces improved-but-unbanked cases)
```

Part 1 skips `TestConformance` because part 4 runs it; neither part is the
gate without the other. Part 4's `-timeout` is part of the command: the suite
alone uses most of Go's 10-minute default (#1698).

**The gate is run, not argued.** Run every part on every tree you hand off or
judge, whatever the diff contains — a prediction that a part cannot fail is
not a reason to skip it (#881, #1344). Run each command to its exit code
before your turn ends — in the foreground, or as a tracked background
command you wait on — because a run still in flight when the turn ends dies
with it (docs/ROUTINES.md). Run it unpiped and read its own exit code: a
pipeline's exit status is its last command's, so `go test ./... | tail`
reports whether `tail` succeeded and stays green however the test failed.
When the output is too long to read, redirect it to a file and read the
file afterwards.

**This block is the only definition of the gate.** A step named anywhere
else — a session brief, a LOG entry, an issue body — is not a gate step,
however confidently it is asserted; note it in one line and move on. Its
absence is not a gate failure. Adding a real step means editing this block
first.

Other commands:

```sh
GOXSD_RATCHET=1 go test ./conformance -run TestConformance -count=1  # arbiter only
go generate ./...                                # regenerate spec md + tables
go tool fetchspecs                               # (re)download pristine spec HTML
git submodule update --init testdata/xsdtests    # the W3C suite
git config core.hooksPath .githooks              # activate the repo's hooks — every session, a
                                                 # fresh clone carries no local git config
```

Surveys, in place of the greps they replace (PRINCIPLES 27):

```sh
go tool lanestatus                    # committed lane scores, as PLAN.md's table
go tool surface -base origin/main     # what this branch added/removed from the exported surface (T5)
go tool wipsurvey < issues.json       # LIVE/CLAIMED/EXPIRED/RETIRED/UNKNOWN branches, then the Working band's rows
go tool gapaudit  < gapissues.json    # GAP( markers vs trackers
go tool suiteindex <query>            # suite fixtures carrying a construct; <query> shapes below
go tool suiteindex -paths <query> | go tool casejoin join <lane>   # that census's candidate cases in a lane
go tool casejoin ids <fixture-path>   # every case ID naming that document, withheld ones included
go tool lanepartition -log run.log <lane> < gapissues.json   # banked fails clustered, largest first, vs open issues
```

`wipsurvey` and `gapaudit` read their issue list from **stdin** in `gh issue
list --json`-shaped JSON, so any GitHub channel that can write that JSON to
a file feeds them. docs/ROUTINES.md ranks the channels, states what to do
when one errors, and spells the paginate-and-reshape recipe under "Survey
input".

Empty stdin is a supported mode for a census, not a failure. `wipsurvey`
then reports leases only, can never report RETIRED, and reads every band
row's state UNKNOWN; `gapaudit` reports the
marker census only and reconciles nothing against trackers. Each says so in
its own output; neither exits non-zero. A run whose output an issue's
`## Acceptance` quotes is a measurement, and only a fed run discharges it
(#1157).

`suiteindex` censuses `testdata/xsdtests` by construct — namespace URI plus
local name, in whatever encoding and prefix each fixture spells it with — so
predict ratchet movement from its output rather than from a grep (#1239). An
absent submodule is a supported mode here too: it says so and exits 0. A
`<query>` names an element and, optionally, its attributes,
`element@targetNamespace`, in these shapes:

- **`|` — ANY of the names.** Between element names it censuses a FEATURE as
  the union of every element that establishes it:
  `'openContent|defaultOpenContent'` bounds `{open content}` (#1554). Between
  attribute names, an element carrying any one of them counts (#1391).
- **`*` — any local name.** `'*@mixed|abstract'` is every occurrence of
  either attribute, on any element, with its value (#1391).
- **`,` — ALL of the attribute names.** `attribute@targetNamespace,form`
  counts only an element carrying both (#1391).
- **`@*` — the (element, attribute) name pairs**, each with its fixtures, in
  place of the per-occurrence lines every other query prints: `'*@*'`
  (#1391).
- **`{uri}` and `{*}` — the namespace.** A braceless element name means the
  XSD namespace and answers 0 for an INSTANCE-side construct, whose element
  sits in whatever namespace its test targets; `{*}` is every namespace at
  once, the no-namespace one included:
  `'{*}*@{http://www.w3.org/2001/XMLSchema-instance}type'` is every `xsi:type`
  in the corpus (#1495).
- **`//` — nesting.** `A//B` admits a match of `B` only inside an element
  matching `A`, at any depth: `'*@maxOccurs//*@maxOccurs'` is every
  `maxOccurs` inside another one (#1585).
- **`=` — the resolved QName value.** The test applies to every listed
  attribute, per whitespace-separated token, resolved against that
  occurrence's own bindings; operands are braced Clark names or `UNBOUND`,
  joined by `|`:
  `'*@type|base|itemType|memberTypes={http://www.w3.org/2001/XMLSchema}ENTITY'`
  is every direct reference to `xs:ENTITY`, and its caveat says what it does
  not resolve (#1671).

Quote neither a `|` feature census nor a `//` census as a count of resolved
components: each figure is lexical, and wrong in both directions against that
population. A `|` census over-counts, because `<openContent mode="none">`
establishes nothing and a `<defaultOpenContent>` can miss its `appliesToEmpty`
gate (`dcl.ctd.ctcc.common` 6.1, 5.2); it under-counts, because an extension
inherits its base's `{open content}` with neither element of its own (4.2.2,
4.2.3.3) (#1554). A `//` census resolves no `<element ref>` or `<group ref>`;
read the caveat it prints before quoting the figure (#1585).

Join a census through `conformance/testdata/expectations/<lane>.txt`, the
lane whose score the prediction is about, with the `casejoin join` pipeline
above: it counts the candidate cases — those the lane carries a line for and
still banks `fail` — and prints the three directions that figure is wrong in.
Read it as a bound from above and never as a prediction of flips (#1642). A
case the suite withholds as inapplicable carries no line and cannot flip
(#1412); take that excluded remainder with `GOXSD_WITHHELD=1`, and
`conformance/doc.go` owns what it is. On the `instance` lane the join also
subtracts the cases the suite declares valid: each carries a banked `fail`
line and none can flip (#1561).

`casejoin ids` maps a fixture path to case IDs one-to-many: one document is
named by as many test groups as declare it (#1642).

## Style headlines

A digest to keep in working memory — `docs/STYLE.md` is authoritative and
its letter IDs (`STYLE T2`) are the citable ones. These numbers are not
IDs, and `PRINCIPLES N` is a different numbering space again.

1. Happy path stays left; return early; **no `else` blocks**.
2. Every error is checked, wrapped with context, and mapped to a spec rule
   via `xsderr`, carrying file:line:column. No dropped errors in loops.
3. Deterministic output always. Never range over a map to produce output;
   collections that reach users are slices in document order.
4. One fact, one encoding — no state derivable from other state, no
   redundant flags. No caches without a measured hot path.
5. No cycle checks — phased construction makes them unnecessary.
6. **No concurrency** in library code.
7. Make illegal states unrepresentable: unexported fields plus
   constructors. Capabilities are interfaces, not type switches (sealed
   sums for schema-closed sets are the one exception).
8. Export nothing without a consumer; every exported identifier is
   documented and justified.
9. Fail-open for unsupported XPath constructs (never false-reject), every
   site marked `// GAP(xpath): ...`. Dynamic errors are real failures.
10. `log/slog` only, injected, silent by default. Spec data tables are
    generated, never hand-typed.

## Writing

Everything you write here — docs, code comments, issue bodies, verdicts,
commit messages — is read by an agent mid-task who needs to act. Write for
that reader, and assume they are competent and trust the document.

- **Lead with the instruction**, in the imperative, and state it once. No
  summary before it, no example afterwards making the same point again.
- **Write for how the reader arrives.** Some grep: keep the searchable
  words in the normative sentence, not in the story around it, or the
  search returns provenance and misses the rule. Others arrive by route —
  a command already in hand, or a heading another document sent them to —
  and read forward from where they land. A hazard sentence goes on that
  path and names the command the reader is holding, not in the section
  where the subject is catalogued. Correct in the wrong place is
  undocumented (#1279).
- **Justify only what surprises.** Never explain why a rule is a good
  rule: if it is right the sentence is dead weight, and if it is wrong the
  sentence will not save it.
- **Evidence lives in `docs/LOG` and on issue threads.** A rule carries at
  most a bare `(#N)` for whoever wants to dig — never a list of them,
  never a date, a count, or what it cost.
- **Do not pre-argue against a misreading.** Prose that keeps being
  misread wants a different structure, or a tool, not another paragraph
  defending it.
- **Length is a signal.** A rule that needs paragraphs usually governs
  something too complicated, or is already stated somewhere else.

## Working here

Work is planned as GitHub issues (label `ready`). One issue is one focused
change and one landing. **The issue thread is the cross-session channel** —
groundings, verdicts, implementation accounts and RESUME notes are comments
there, because sessions run in ephemeral containers and **anything not
pushed does not exist**.

`docs/WORKFLOW.md` is normative for the branch scheme, checkpointing,
scope, landing and parking. The command files are the procedures
themselves; run one command per session.

Commit format:

```
<area>: <what changed> (#<issue>)

Spec: <rule ids touched>
Ratchet: <lane movement, or "unchanged">
```

Append a dated entry to `docs/LOG/<year>-<month>.md` before landing, so
the log rides in the session commit.

## The cast

Specialized subagents live in `.claude/agents/`: **mason** (implements),
**arbiter** (judges; the only agent that runs the ratchet), **oracle**
(spec exegesis, read-only), **warden** (API and type-safety review,
read-only), **cartographer** (planning and GitHub issues), **steward**
(architecture stewardship; files refactors, never implements),
**chronicler** (logs and retros), **libuser** / **cliuser** (role-play
consumers; see only the published surface).

Each agent owns its domain and exercises judgment inside it. The
orchestrating session coordinates, does no specialist work itself, and
never skips the arbiter.

This file's "one rule", and the ratchet-integrity section of
`.claude/agents/arbiter.md`, change only via a human-filed issue — never
in a retro.
