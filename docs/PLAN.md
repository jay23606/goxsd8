# goxsd8 Roadmap

**This file is status, not history.** It says where the project stands and
what each milestone is for. It does not narrate how it got there — that is
`docs/LOG/` — and it is not the work queue: GitHub issues are, and the
milestone links below go to the live lists.

Milestones map one-to-one to GitHub milestones. The cartographer carves
each into session-sized `ready` issues; the develop loop closes them one
per landing. Prefer vertical slices that move a conformance lane over
horizontal completeness.

**The Status section is REPLACED, never appended to.** `/backlog` rewrites
it wholesale from the sources — `go tool lanestatus` for the lane scores,
GitHub for milestone and queue counts — and stamps it with the date it
read them. One stamp for the whole section, so a reader can tell staleness
from wrongness at a glance. Never add a dated paragraph beside the old
one — appending is what this replaces.

## Status — 2026-10-19 (post-land for #2781, on `main` at `7eea6746`)

**North star: M8, the `json` adapter, through #2783, which now leads the band.**
#2781 landed jsonsrc's first message clause ("object expected" on a scalar's
own cvc-complex-type text charge) and unblocked #2783, the key-list and
"scalar expected" clauses that finish #2722's re-plan. A `GOXSD_DECLINES=1`
run on `7610745d` leaves no unruled W3C cluster on any active lane, and #2781
changed no file the W3C lanes read. **Lean: product**, unchanged: the retro's
product share fell from 76% to 54%, and W3C yield was 4 cases in 54 sessions.

### Conformance lanes

**This table is `go tool lanestatus`, pasted verbatim, on `main` at
`7eea6746`.** It is the committed expectations census, which `docs/WORKFLOW.md`
names as the lane score (#1120).

| Lane | Pass | Fail | Total |
|---|---:|---:|---:|
| `ber` | — | — | 0 |
| `datatypes` | 1163 | 10 | 1173 |
| `instance` | 26290 | 71 | 26361 |
| `json` | 43 | 0 | 43 |
| `schema` | 15302 | 96 | 15398 |
| `xpath` | — | — | 0 |

**No lane moved at `7eea6746`**: #2781 changes message text only, so `json`
banks no curated row. **`datatypes ⊆ instance` is intended** (#1507). **An
em dash means a lane with no cases yet, not a lane scoring zero.**
`datatypes` is M3 and complete. `schema` is M4, `instance` is M5, and `json`
is M8, and all three are active. `json` moves only by curated cases a
landing adds. `xpath` waits on M7 and `ber` on M11.

### What holds each active lane's failures

**Measured on `7610745d`** (a read-only `GOXSD_DECLINES=1` run fed to `go
tool lanepartition`). #2781 touched only `validate/jsonsrc`, `cmd/goxsd8` and
one doc comment in `validate`, so the figures stand. Every figure is a part of
the banked fails and a bound from above.

| lane | banked fail | declined | indeterminate (#277) | decided against the suite |
|---|---:|---:|---:|---:|
| `instance` | 71 | 39 | 5 | 27 |
| `schema` | 96 | 1 | 11 | 84 |
| `datatypes` | 10 | 6 | — | 4 |

- **`VC` is empty.** No `instance` or `schema` fail is banked under it.
- **Every larger `instance` cluster is ruled or recorded.** `MS-Regex`
  18 is ruled by #1899, `MS-Attribute`'s 11 by #2151, and `MS-Particles`'
  ceiling by #1601. `MS-Additional`'s 5 are named in #2171. The 7
  `strict-attribute-unresolved` declines are #1912's ruled divergence, and
  #2237's Acceptance turns them into decided disagreements, fail to fail.
  `MS-Schema`'s 4 and 3 match #744 and #921 by test-set name only.
  `lanepartition` reads open issues only, so a ruled cluster prints "no open
  issue names it" (#2542).
- **`schema`:** `MS-Wildcards` 49 is a recorded divergence (#1809), and
  `Open`'s 5 are `blocked` on #1374. `Simple`'s 5 match #513, #620, #634 and
  #1236 by test-set name.
- **`json` has no fails to partition.** It moves by the curated rows a slice
  adds.

### Branch namespace, `origin` (report-only; containers cannot delete a ref)

**`go tool wipsurvey`, fed by `go tool issuefeed`** (`rows 2816 distinct 2816
min 1 max 2816 no gaps`). No `wip/issue-<N>` is LIVE. `wip/issue-2781` is
gone, as a landing leaves it.

- **RETIRED, deletable now:** `wip/issue-2013`, `wip/issue-2332`,
  `wip/issue-2696` and `wip/issue-2699` (each issue closed).
- **RETIRED, keep until #2783 lands:** `wip/issue-2722` (`8a3fa106`) is the
  only copy of the round-1 `TestNoteHints` rows #2783 ports.
- **EXPIRED:** `wip/issue-2081`, a parked grounding with no code, on an issue
  `blocked` on the owner.
- **Certified for deletion and still standing:** `meta/backlog-2026-10-01-b`,
  `meta/backlog-20261013-0808`, `chronicler-345`, `meta/post-land-1816`,
  `meta/post-land-1908`, `postland-2099` and `postland/issue-2284`.
- **For human triage:** `parked/untriaged-20260930-110432`,
  `parked/untriaged-20260930-215455`, and eight subagent branches the survey
  skips: `wip/issue-1861-{mason,chronicler}`, `wip/issue-1926-{mason,chronicler}`,
  `wip/issue-2008-{mason,chronicler}`, `wip/issue-1988-mason-repair` and
  `wip/issue-1988-chron2`. Each of their issues is closed and landed.

### Marker census

**`go tool gapaudit` on `7eea6746`, fed the same walk:** 181 markers across 11
areas (xpath 63, xsd 32, jsonsrc 31, validate 23, value 11, conformance 7,
parser 6, xml 3, landcheck 2, regex 2, cmd 1). #2781 added the two on
`walker.noteCharacters`, one owned by #2670 and one RULED permanent by #2702;
neither is in Group 1. **Group 1 has 11 rows.** Eight are #2719's. The other
three have an owner written into the body: `xsd/derivation.go:1293` (#555),
`value/valuespace.go:152` (#462) and `tools/landcheck/keywords.go:48`
(#2541). **Group 2 has 27 rows**, all kind/gap trackers that carry no marker
by design.

### Milestones and queue

**Read from GitHub after this pass: 337 open issues.**

| milestone | open | closed | state |
|---|---:|---:|---|
| M1 — Spec infrastructure | 0 | 3 | done |
| M2 — Foundation leaves | 0 | 5 | done |
| M3 — Datatypes vertical slice | 0 | 13 | complete |
| **M4 — Schema parsing** | **61** | **180** | active |
| **M5 — Instance validation (XML)** | **9** | **136** | active |
| **M6 — XPath required subset** | **6** | **46** | active |
| M7 — XPath 2.0 growth | 1 | 1 | #2619 open |
| **M8 — JSON instance adapter** | **8** | **39** | active; `json` 43 |
| M9–M12 | 0 | 0 | not started |

The open issues are **321 `ready`, 14 `blocked` and 2 `epic`**, which sums to
337. No issue is `needs-replan` or unlabelled. Open `kind/process` is 3
(#1885, #2803, #2810). #2810 carries no milestone.

- **This pass unblocked #2783** (#2781 closed). No other open body names
  #2781 as a dependency.
- **`blocked` is 14:** #16, #555, #1042, #1051, #1374, #1609, #1790, #1885,
  #2022, #2081, #2376, #2533, #2670 and #2749.
  - #2376 waits on #2370, and #1051 on #1183 and #1196. All are open.
  - #2670 waits on #1601's ruling reopening. #2533 waits on an
    evaluation-time CTA decline.
  - #1885 and #2081 wait on the owner.
- **Measured refactors over the files #2781 changed,** re-run at `7eea6746`,
  are flat: #2790 6, #2791 8, and #2792 5 and 2. Every other figure stands
  from `7610745d`, `438fadae`, `7a12227e`, `74c38cc5` or `12ff9797`: #2675 3,
  1 and 1, #2595 1 and 1, #2237 24, #2372 27, #2600 2, #1865 2, #2370 32,
  #363 2, #848 2 and 0, #1080 8, #1958 2, #2101 1 and 1, #2205 2, #2239 2,
  #2329 1, #2359 14, #2373 16, #2374 22, #2375 1 and 1, #2599 186, #2601 0,
  #2663 4, #2793 3.

### Persona consultations

**Both personas were consulted at `3a8126e`.** Their findings are filed or
folded. #2781 left the exported surface unchanged; its contract moved in doc
prose and `usage` only. No round is due until the surface next changes.

### Working band

This is ordered for a `/develop` session: take the highest row you can start.
**Run `wipsurvey` fed before starting**; its WORKING BAND section prints each
row's state. Each row names one issue (#1636).

**Tier A: M8, the `json` adapter.**

| # | issue | why here |
|---|---|---|
| 1 | #2783 | **#2722's key-list and "scalar expected" clauses**, on #2781's `noteAt` and `annotate`, with the round-2 REPAIR DIRECTION as its bar. Its round-1 rows live on `wip/issue-2722` only and are ported. |
| 2 | #2723 | **A truncated document with a trailing LF** is located on a line it does not have. Small, one offset index. |
| 3 | #2796 | **A grounding first.** It either produces a twin-divergent probe for `instanceUnder` or closes `not_planned`. |

**Tier B: jsonsrc refactors, flat at `7eea6746`.**

| # | issue | why here |
|---|---|---|
| 4 | #2790 | **`governor` assertions, 6.** Each M8 slice adds sites. One `SchemaView` widening deletes all six. |
| 5 | #2791 | **The governance mirror, 8.** It needs a warden pre-flight on mechanism (a) or (b). It would also retire #2796, and #2781's `scalarGoverning` now reads the mirror. |

**Tier C: tooling.**

| # | issue | why here |
|---|---|---|
| 6 | #2789 | **The fired-trigger blind spot that hid #1880 for eight days.** It lists `blocked` issues whose thread carries an owner comment, which each pass checks by hand. |
| 7 | #2748 | **`issuefeed -comments`.** It closes #1153 and #1217. |

**Named below the band, on purpose.**
- **M5, `conformance/subtreeroot.go`:** #2237 (24; the 7
  `strict-attribute-unresolved` declines become decided disagreements, fail
  to fail) and #755 (the hint reader's home).
- **M8:**
  - #2780 (the `-format` clause, CLI only; its `usage` sentence goes beside
    #2781's).
  - #2769 (a test asserts no `Unevaluated()`).
  - #2757 (jsonsrc's doc restructure; census re-taken at 475 lines, and
    `Validate`'s doc now names `walker.noteCharacters`).
  - #2670 (waits on #1601).
  - #2798 (doc only).
  - #2792 (needs a warden pre-flight, flat at 5 and 2).
- **Process:** #2803 (a park's LOG entry rides the `wip/` branch, two
  sightings, no round lost) and #2810 (`SendMessage` recovery of a round that
  ended with no report, two sightings, no round lost this time).
- **Persona documentation, behind lane work by the retro's bar:**
  - #2797, #1088, #1144, #1430, #688 and #2313.
  - `cmd/goxsd8`'s `usage` family: #1407, #1089, #1433, #1571 and #1570.
  - README: #1143 and #1003.
- **Marker bookkeeping:** #2719 (gapaudit's 8 Group 1 rows). A session in
  `value/valuespace.go`, `xsd/defaultbinding.go` or `xsd/contentrestricts.go`
  absorbs its row.
- **M7:** #2619 (`as-props-correct` clause 2; needs a warden ruling on its
  `xpath` export).
- **`xpath/cta.go` and `xpath/ctaparser.go`:** #2675 (flat at 5), #2416 and
  #2595. A session taking one reads the others.
- **M6:** #2663 (flat at 4) and #2662 share `validate/cvcassertion.go`. #2455,
  #2437 and #2394 are in `xpath/assertion.go`, one at a time with #2370, #2314
  and #889.
- **STYLE E5 sweeps,** measured flat: #2370, #2372, #2373, #2374 and #2375.
  #2376 waits on #2370.
- **M5 residue:** #2764 (a repeated attribute name, `parser/xmltree`), #2494,
  #1825, #1382, #1257, #1250, #920 and #2419.
- **M4, where `Ratchet: unchanged` is expected or the bound is small:** #2087,
  #2052, #1873, #1877, #1883, #1887, #1801 and #1777. #1998 and #1953 are in
  `xsd/contentrestricts.go`, so either one fires #1609's arm 3.
- **Docs and bookkeeping:** #2679, #2624 with #793, #1803, #1881, #785, #1343
  and #1502. **`value`:** #721, #2045 and #462. **`regex`:** #2148 and #2147.
- **Tooling, after row 7:** #2542, #2203, #2539, #2605, #1897, #2541, #1935,
  #2070, #2066, #678, #1464, #1838, #1794 and #1735.
- **Refactors:** #2599 (186) and #2601, #2793 (3), and the flat list above.
  #989, #1736, #1701, #845, #1757, #1825, #2416, #2419 and #2437 are
  unmeasured.

### Next planning action

1. **A `/backlog` run.** Every W3C cluster on the active lanes is ruled or
   recorded, so name `instance`'s next north star — the ruling that holds the
   most cases, or the largest yield no cluster shows — and confirm M8 as the
   lead track.
2. **Post-land for #2783.** Move `wip/issue-2722` to the deletable list, and
   re-take #2757's census.
3. **Owed to a human:**
   - #1885 (arbiter.md's Ratchet integrity), #2081, #1790, #2022 and #2749.
   - The ref deletions under Branch namespace.

## Milestones

### M0 — Scaffold — done

Repo layout, docs, local specs plus conversion tooling, W3C suite
submodule, per-package `doc.go` contracts, agent personas and commands,
lint gate.

### M1 — Spec infrastructure — done

The hfn → TypeSpec generator emitting `builtin/gen_typespec.go` for all 49
builtins including precisionDecimal, sourced from Appendix E and the
per-type property tables; the conformance ratchet (expectations
load/compare/merge, upward-only, `suite.xml` runner, lane files); and
`xsderr`'s `Rule`/catalog wiring.

### M2 — Foundation leaves — done

`xsderr` (Error/Rule/Loc plus narrowing helpers), `loader` (Resolver with
Dir/FS/HTTP/Map/Chain), `parser/xmltree` (streaming position-tracking
decoder), and the `xsd.QName` expanded-name type that `value.Backend` and
the builtin table key on. Full unit tests; fuzz targets for xmltree.

### M3 — Datatypes vertical slice — complete

All 20 primitives mapped; the facet pipeline, value spaces and canonical
mappings behind the **`datatypes` lane**. Remaining open work is
follow-up, not milestone scope.

### M4 — Schema parsing — active — [epic #79](https://github.com/kud360/goxsd8/issues/79)

Three-phase parser over the composition model — include, import, redefine,
override, chameleon coercion — with UPA, EDC and particle restriction
designed into the model shape from the start, plus the `xsd` component
model and the finalize/resolve phase (`src-resolve`, dependency-ordered
finalization, named-circularity rejection). **`schema` lane.**

The original carve (#167–#183) is landed. **Every §3.4.2 complex-type
representation form is now produced**: `<simpleContent>` with
`<restriction>` was the last one declined outright and #909 built it on
2026-08-22. Open work is the long tail of producer widening, finalize
validity and composition edge cases, and it has changed shape — what
remains is predominantly **s4s grammar faults the producer decides and
ACCEPTS** rather than forms it cannot build. The two exemplars this
paragraph used to name, #956 (child order and `maxOccurs` across the
derivation alternants) and #958 (a facet element name the gate admits and
the producer drops), both landed on 2026-08-22/23; **#884** (malformed named
`<group>` bodies collapsing into one `mgd-props-correct` message) joined them at
`b3f295a`, and **#972** — an XSD-namespace child §4.1.2's
`<simpleType><restriction>` has no position for, dropped by `restrictionFacets`
so the producer built a schema the harness called `valid` — landed at `e491ddb`
for **`schema` +26** — the family's largest single move until **#1047** took
`schema` +34 on 2026-08-28. **#471** landed the local `<element ref=>` carrying
`substitutionGroup=` on 2026-09-03 (`31263ed`, `Ratchet: unchanged` — the
grammar-level `use="prohibited"` shape rejects with no case in the corpus to
bank) and its grounding carved two siblings from its scope: **#1205**
(`final=`/`abstract=`, the other grammar-prohibited pair) and **#1206**, which
**landed 2026-09-04** (`1780670`, `schema` **+16**) — `src-element` clause 2.2's
prose-only eight attributes on a local `<element ref=>`. **#1205 LANDED
2026-09-06** (`24ac7d3`) for `schema` **+5**, and it retired this family's
"ratchet-neutral by construction" inheritance: #471's neutrality came from
reaching the `ref=` form ALONE, while #1205 also reaches the inline form, where
six local fixtures exist and five moved. Its `ref=` arm IS neutral as predicted
(**0** fixtures suite-wide pair `ref=` with either attribute), and the inherited
prediction never shipped as a claim because that issue's own Acceptance made
measurement a precondition. **#1206's own landing then
carved two more**, from a UTF-16LE corner of the suite no UTF-8 grep had
reached: **#1215** (`src-element` clause 4, `ed-with-ns`) and **#1216**
(`src-attribute` clause 6, `att-with-ns`). **Both LANDED 2026-09-04** —
`976bdc2` for `schema` **+4** and `aeaf59e` for **+5**, each banking one and two
cases past its own prediction — and **each carved a successor from its own
boundary**: #1216's two `GAP(xsd)` markers became **#1242** (clause 6.1's one
reachable failure, a local `<attribute ref=>`) and **#1243** (an
`<attribute use="prohibited">` escaping clause 6 entirely), and **both have now
LANDED** — #1243 on 2026-09-05 (`3febb22`) and #1242 on 2026-09-06 (`6e95f25`),
**each `Ratchet: unchanged` and each measured that way by `go tool suiteindex`
construct census rather than assumed** — ratchet-neutral like #471 before them
(NOT like #1205, which banked **+5** on its inline arm), but the first two of
the family to prove it with an instrument instead of a grep. **#1243 carved one successor, #1242 carved none**,
which ENDS the run below: #1270 (`src-attribute` clause 4 charged nowhere on the
`use="prohibited"` branch) was #1243's, and #1242's landing closed its arm with
no residue. **#1270 has itself now LANDED — 2026-09-06, `d0fd4c6`,
`Ratchet: unchanged` and measured that way by construct census — and it carved
NO successor of this family either.** Its two residues are #1301 (a duplicate
clause-4 predicate surviving at the top-level producer, still open) and #1300 (a
process defect about how a gate command is invoked — **LANDED 2026-09-07,
`34aee07`**, `Ratchet: unchanged`); neither is a producer that decides and
ACCEPTS, so neither extends this family. **#455 has now LANDED too — 2026-09-07,
`8aca2a3`, `Ratchet: unchanged`, structurally excluded before it was measured
twice — and it CARVED a successor: #1328** (`use`, `form` and the two
`*FormDefault` attributes compared raw at every site, measured accepting
`use="foo"` and reading `use=" required "` as `{required}`=false). It also
unblocked **#456**, which was filed long before and is a discharge rather than a
carve. **#931 has now LANDED too — 2026-09-08, `c5e5e375`, `schema` +6
(fail → pass), zero regressions** — the occurrence attributes on a named
`<group>`'s body compositor, and this chain's first non-zero since #1205's +5.
**It ends the zero run**: the four landings before it all read
`Ratchet: unchanged` (#1243, #1242, #1270, #455) and the fifth banked six, so
ratchet-neutrality is a property of the individual member and not of the family,
and no further member may inherit the prediction. **It is also the one member
whose count was NOT taken by construct census** — its prediction came from a
hand-written producer-level probe, was wrong on three of its four named
candidates and under-predicted the lane by two, which is the defect **#1332**
owns. It carved no producer-side successor: #1332 is `kind/process` and **#1333**
pins the `xs:redefine` entry into `buildDefinitionModelGroup` by test, and
neither decides and ACCEPTS. **#456 has now LANDED too — 2026-09-08,
`5b84206`, `schema` +20 (fail → pass), zero regressions** — every one of the
twenty a suite-declared-`invalid` fixture writing an xs:boolean schema
attribute outside `booleanRep`. **It is the SECOND consecutive non-zero and the
largest single bank in this chain's record** (against `unchanged`, #1205's +5
and #931's +6), which settles what #931 opened: the run of four ratchet-neutral
landings before it was the anomaly, not the rule, and no member may inherit
either prediction. **It also answers #931's census defect from the other
side.** `go tool suiteindex` could not take this census either — it censuses by
construct, and this population is defined by attribute VALUE — but the member
said so before being asked and predicted the twenty EXACTLY by censusing the
attribute values directly. **Naming the instrument's limit is what #931 did not
do**, which is the distinction #1332 now owns. It carved no successor: its one
unmet acceptance item was dispositioned to the existing #717, not filed.
**#1328 has now LANDED too — 2026-09-09, `51d9cf9`, `schema` +18 (13968 →
13986, fail → pass), zero regressions** — `use=`, `form=` and the two
`*FormDefault` attributes read as ·actual values· rather than raw literals, all
four being `xs:NMTOKEN` restrictions Appendix A enumerates and `cvc-datatype-valid`
stating no fallback clause. **It is the THIRD consecutive non-zero** (#931 +6,
#456 +20, now +18) and **the first member of this family to predict its flip set
EXACTLY**: banded on 24 enumerated fixtures plus ten measured shapes, it banked
the body's 18 `fail` rows to the letter and left the 6 `pass` rows the body
flagged as the risk unmoved — the census re-taken against the tree rather than
trusted from the body. **It carved NO producer-side successor.**
`effectiveDerivationSet`'s silent drop of unrecognized
`block`/`final`/`blockDefault`/`finalDefault` tokens is ruled out of scope AND
unfiled by its own Notes, on a different spec reading (`blockDefault` *"may
include values other than"* the relevant set); its arbiter's one non-blocking
observation was ruled not a STYLE T4 defect and recorded rather than filed; and
#653, whose items 3 and 4 this landing discharged, took a thread comment and a
body re-statement rather than a new issue. Live producer-decides-and-accepts
members are now **#929** alone — which #455's landing widened to own
`minOccursZero`'s clause-2.1.3 compare as well as `maxOccursZero`'s. **The family
carved a successor at
every landing from #471 through #1243, and #1242 is the first that did not** —
one reading is the tail reaching zero, the other is that this pair was narrower
than its predecessors; two more landings decide it. **#1270 was the first of
those two and came back empty; #455 is the SECOND and came back with #1328**, so
the test the last stamp set is answered — but **#1328 is itself the THIRD
CONSECUTIVE member to carve nothing**, after #931 and #456, so *"the family is
still carving"* is no longer the reading the record supports: the generator is
producing landings and lane movement, not successors, and the queue this family
feeds has narrowed to #929 alone. Note what carried the one carve there was —
#455's successor came out of an ARBITER's non-blocking scope note, not out of a
construct census, which is a different generator from the one #1270 and #1242
exhausted; #1328's arbiter produced a non-blocking observation too, and that one
was ruled not a defect, so the generator fired and came back empty. A
second, narrower family opened beside it — the rejections the
producer already makes **correctly but describes badly**, whose bar
`xsderr/doc.go` set with #966 — and it is **discharged**: #975 landed at
`1dcffbf`, so every s4s-grammar rejection now names its Appendix A production.

**A THIRD family is the one that has been paying, and it has SIX landed
members.** #1030's unmapped-construct census turned the "decides and ACCEPTS"
family from a shape into a list, and the list delivered `schema` **+34**
(#1047), **+23** (#1046, plus `instance` +15), **+21** (#1076), **+16** (#1048)
and **+2** (#1099). For #1046/#1047/#1048 the gate-side alternative was
**measured and ruled out** — widening `conformance/schema.go`'s shape gate costs
a banked ratchet win — and #1076 and #1099 arrived with no measured count at
all, banded on the criterion *"the shape is in the suite's invalid corpus"*
instead. **That criterion is five-for-five on DIRECTION and predicts nothing
about MAGNITUDE**; the +21 and the +2 came from the same family four days
apart. The Status section carries the consequence for how the band is ordered.

**The sixth member is #1275 (`34c39be`, 2026-09-07) and it is the first to bank
NOTHING — because it did not use the criterion, it measured.** A twelfth
`s4sModel` orders an `<alternative>`'s own children against `xs:altType`, the
same section 5.1 first-bullet fault #1047 and #1076 charge at their positions;
the corpus was censused before implementation and holds **no** `<alternative>`
carrying an out-of-model child across 232 occurrences in 81 fixtures, so
`Ratchet: unchanged` was predicted and then measured. **The criterion is a proxy
for a census that `go tool suiteindex` can now take directly**, and where the
two disagree the census is the fact — that is the methodological change this
member carries, not a sixth data point for the proxy. Read the criterion's
five-for-five as five-for-five *among issues banded on it*, which is a smaller
claim than it looks.

**The family replenishes itself as it is worked, which is what to expect and
not tail growth** — every issue it has added was filed by the post-land pass of
a landing that moved the lane. **Its shape is also changing**: #1047, #1076 and
#1099 all widened `checkS4SChildOrder` or its callers, and the issues arriving
now — #1097 (`32070b8`) and **#1136** (`d854e1d`, 2026-09-05), both landed, and
open #1098, #1133 and #1135 — are defects *in* that widening rather than further
sites for it. **A producer that decides more is a producer with more to get
wrong, and the census does not name that class.** #1136 is the one that stopped
the accumulation: it replaced four independent local run-order guesses with ONE
recorded decision, and its own follow-ups (#1246, landed 2026-09-07 as
`a363592`, and #1247, landed 2026-09-08 as `77b0419`) are about that record
rather than about a fifth guess. **That record now states a rule scoped by
WHERE rather than a family roster**, and #1247's own follow-ups (#1342, #1343)
are about its wording rather than its membership.

**#1275 shows the site-widening half is NOT exhausted, and it arrived from a
third route again.** It is a further SITE — a twelfth `s4sModel` at a position
nothing had ordered — not a defect in the widening, and it came from neither
#1030's census list nor a run-order defect: #1050's reconciliation of
`parser/census.go`'s Scope prose NAMED the silence in the tree and tracked it
with nothing, and #1275 was the filing that was owed. **Every widening site the
producer still owes is discoverable from that Scope section**, whose remaining
live silences it enumerates; the open **#1276** would make that section's truth a
DERIVED test rather than prose, which is what would turn the remainder into a
list the way #1030 did once already.

**Two more of this family landed 2026-09-09/10, and between them they closed
the two AXES rather than two more sites.** **#1369** rejects an unprefixed
attribute no Appendix A production of its element declares (`schema` **+17**),
on a hand-transcribed roster of all 49 productions; **#1380** rejects an
XSD-namespace `<schema>` child Appendix A admits at no top-level position
(**+9**), and its load-bearing finding was that **the parser charge alone banks
ZERO** — all twelve suite witnesses were declined by `conformance/schema.go`'s
top-level `default` arm, so the harness widening had to be absorbed. **What
remains after them is neither axis but the VALUES on them**: `maxOccurs="00"`
(#929), `##defined` (#606) and the eleven cases #731 carved into #1411–#1414 are
all attribute-VALUE faults. **#1391 landed the attribute axis they needed**, so
`go tool suiteindex '*@maxOccurs'` and its siblings now census them; #929 and
#606 both still predict `unchanged` for want of a census that is no longer
missing, and re-ranking them on one is owed.

**A schema-construction slice can sit OUTSIDE §5.1's grammar-and-`src-*` frame
entirely, and the expensive case is §5.3.** **That question is now RULED**:
§5.3 governs a failed `·resolution·`, so `Finalize`'s hard-fail is a deliberate
implementation policy choice rather than a spec requirement, and reversing it
costs 35 measured `schema` cases with no ratchet mechanism able to record the
loss — a call CLAUDE.md reserves to a human-filed issue. The ruling is on #1426's
thread; #1426 is closed `not_planned` and **#1498** carries the marker prose that
records it. **#1429** owns the harness guard the attempt broke. **Both missing
built-in component sets have LANDED** — **#1427** on 2026-09-12 (`c1f31b5`,
`schema` +7, `instance` +15) and **#1428** on 2026-09-12 (`cd36ad8`, `schema`
+2). **Neither waited on the ruling, and that is the finding** — the withheld
flips were a missing component set, not a §5.3 deferral, which is now measured
rather than argued.

**Read the milestone count as a floor.** The GitHub milestone holds the feature
slices; the comment-accuracy, doc and process issues that post-land passes file
against the same packages sit outside it — #1135 is M4 work carrying no
milestone today, and #1136 was too until it landed. **The sharpest instance for
LANE MOVEMENT is #1126**, which moved `schema` +475 and `instance` +113 in one
landing and carried no milestone, so the count did not move at all. (**#1411
passed it on the `schema` side on 2026-09-13 with +609**, banking the
`MS-Regex2006-07-15` corpus whole; #1126 remains the largest that moved two lanes
at once.) **The sharpest for SCOPE is #434**: a core §5.3
schema-construction issue, banded as the best lane slice in the queue, carried no
milestone for its whole life and closed outside M4's count.

### M5 — Instance validation (XML) — [epic #250](https://github.com/kud360/goxsd8/issues/250)

`validate` engine plus `validate/xmlsrc`: non-backtracking content matching,
identity constraints, `xsi:type`/`xsi:nil`, wildcards, open content, default
and fixed values. **`instance` lane.** *"Greedy"* is struck here and from
PRINCIPLES 14: #782 replaced the greedy walk with a live SET of partitions,
which `cvc-accept` clause 3.1's existential requires, and the invariant that
survives is no-backtracking rather than greed. **"Bounded" is struck too, and
by #1557 rather than by #782**: arm (b) made `partitionsBounded` an ESTIMATE of
the cover instead of an upper bound on live entries, and what survives
unconditionally is a per-item BUDGET — one item's cost is a constant of the
SCHEMA, never a function of the items already taken.

Carved 2026-08-12 into ten slices, #710–#719, and now **eighteen** — #766 was
split out of #714 by a warden pre-flight, #773, #774 and #775 out of #766 by
another, #782 and #783 out of #715 by its own, **#790** filed from #715's
warden verdict, which had named the seam and been read by nobody who filed it,
and **#1516** for the `{open content}` clauses. **All eighteen have landed.**
The last two, #773 (`c1851ae`, `instance` +9) and #774 (`5fb2cd5`, `Ratchet:
unchanged`), landed on 2026-09-24. #1516, #782 and #783 all landed 2026-09-16/17
and took `instance` 11151 → 11203. Of the thirteen that had
landed before this window, **#719** joined them on 2026-08-27 at `bd887cd`, wiring
`cvc-assertion` fail-open at every variety level and shipping the
`validate.Unevaluated` channel; it moved no lane and correctly so, because a
fail-open declines exactly where the engine already declined. It unblocked #56
and its successor **#1042** (assertion EVALUATION) is the first issue this
project has ever placed on **M6**. The other twelve: the infoset
seam and engine skeleton (**#710**), the `xmlsrc` adapter (**#711**) and root
dispatch (**#712**), none of which moved the lane and correctly so — the first
two decide no `cvc-` rule and nothing yet ran the cases; **the lane driver
(#713), which took `instance` off zero — 0 → 18 pass** (the #175 analogue, placed
fourth so every slice after it reports a real number); `cvc-complex-type`
clauses 2–3 (**#714**, 19 → 29); the `value.Backend` seam with the four
datatype-valid attribute charges (**#766**, 29 → 193); the greedy
non-backtracking content matcher (**#715**, 193 → 520); `cvc-complex-type`
clause 1.2's ·initial value· against String Valid (**#775**, 532 → 535), which
also closed #759 and landed `docs/STYLE.md` **E4**; **the descent (#790,
535 → 1017)**, which threads each descendant's ·context-determined declaration·
into the recursive walk (key-sva (§3.3.4.6) clause 3.1) — the largest single
move any lane has recorded; identity constraints and the ID/IDREF table (**#718**,
1017 → 1133); `xsi:type` and `xsi:nil` deciding rather than declining (**#716**,
+183); and a union-governed item classified by its ·validating type· (**#813**,
+9, unioned onto #716's). #913's cvc-type clause 3.1 landing added **9409**,
itself M5. **#1738** (`f6ab5b1`, 2026-09-27) added **10,508**, the largest single
lane move this project has recorded: the lane's first `valid` observation, for a
simple leaf root. Its second slice, **#1808** (`cb0f6e3`, 2026-09-28, +34),
admitted the complex empty leaf root. The third, **#1841** (`aad7b2c`, 2026-09-29,
+2204), admitted an assessed subtree root WITH content. Its successors lift that
gate's exclusions one decision at a time: the ID family (#1857), the content-less
root (#1855, which folded both leaf gates into `assessedSubtreeRoot`), identity
constraints (#1858) and xsi:type (#1859) have landed. Read the rest off the
queue by `conformance/subtreeroot.go`.

**Landings OUTSIDE this milestone keep moving this lane, by four distinct
mechanisms, and a running total of them is not maintained here** — take the
figure from the Status section's table (#646). The four:

- **A slice PRODUCES a component the engine could not previously see** — #733
  (a top-level `<xs:attribute>`'s inline `<xs:simpleType>`), #909
  (`<simpleContent>` `<restriction>`), the CTA pair #842/#851. **#1427 LANDED as
  the next of these on 2026-09-12** (`c1f31b5`) and is the mechanism's cleanest
  instance: §3.2.7's four built-in `xsi:` attribute declarations were seeded as
  COMPONENTS, a schema referencing one stopped hard-failing `src-resolve`, and
  **`instance` banked +15** alongside `schema` +7. **The figure this paragraph
  used to carry — 14 — was an inference transcribed from `wip/issue-434` and was
  WITHDRAWN at grounding** before the landing measured 15; read it as the worked
  example of why a transcribed prediction is re-derived rather than inherited
  (#1332).
- **A slice DECIDES a `{type table}` the engine previously withheld.**
- **The MEASUREMENT is fixed and the engine never changed** — #862, where
  `resolveExpected` was picking the wrong feature-scoped expected verdict, so
  the answers had been right all along. A lane can move because the measurement
  was wrong.
- **The SCHEMA the engine was handed is fixed** — #1001, where §4.2.2's
  ·conditional inclusion· removed declarations colliding under
  `sch-props-correct` clause 2, so five documents the engine had never been
  given a usable schema for became decidable.

**The largest of them is #853 (+141 at `2310710`) and it carries no milestone**,
which is a finding about the LABEL and not about M5's carve: it decides a `cvc-`
rule at assessment time on an XML instance, which is M5's own definition of its
scope, and it sat outside only because a `kind/gap` filed by a post-land pass
never acquired one. **Read the M5 milestone count as a floor and never as the
lane's remaining work** — the same caveat M4's section makes, for the same
mechanical reason, and the `instance` lane is where it costs most.

**#853 repeats #913's lesson rather than #790's.** It was banded UNMEASURED,
with an instruction to go count first, and outperformed every candidate that
arrived with a count: a slice that decides a *new* rule on a commonly-declined
shape moves the number far more than its rule count suggests. **The Status
section carries what three stamps of this have settled** — a document count is
a filter on candidates, never a sort key, and the invalid-corpus criterion
predicts direction and not magnitude.

**A flat M5 landing is routinely CORRECT and the section should not read as if
it were not.** #1043 (`5d3d222`) declined the ·governing type definition· of a
skip-wildcard attribute, withdrawing a `cvc-id` charge §3.10.4.1's Note says was
never owed; #1116 (`9919faf`) charged cvc-complex-type clause 2 against an
anonymous governing type behind a harness gate that admits no case of the shape.
Withdrawing a wrong charge on a document already banked `fail`, or deciding a
shape the executor withholds, cannot register as movement. **That is what the
ratchet's zero-flip-down means, and it is a result to explain rather than a
null one** — and #1116's explanation was banked and then paid out. **#1126
(`d433f7f`) deleted the harness gate #1116's `Ratchet:` trailer named**, and both
lanes moved on the same run: `schema` +475, `instance` +113. The charge was
right when it measured flat; the executor was what withheld it.

**The milestone's shape changed with #790, not just its number.** The first eight
slices decided the ·validation root· and nothing else, so each one bought tens of
cases; #790 bought 482 by making the same rules reach the other 99% of every
document. **The remaining slices inherited that multiplier and spent it**: #718
and #716, the band's top two at the time, bought a further 299 between them by
deciding their rules at every node rather than one.

**The carve is no longer a chain, and reading it as one loses work.** The
original ordering assumed a slice becomes `ready` as the one before it lands.
That held until #714, which had **two** successors directly behind it, and #766
has **three**; #715 had four, of which #716, #718 and #719 became `ready` when it
landed while #717 waits on #248 as well. **A slice can also travel backwards, and
then forwards again**: #718 went `ready` → `blocked` when its own grounding
falsified its premise and filed #790 as the enabler it needs, and returned to
`ready` when #790 landed. Both moves were made by reading the *code* against the
falsifying claim, not by reading the dependency's closed state — the round trip
is the worked example of what a `## Depends on` line is for. Relabel from those
lines,
never from the milestone or from the issue numbers' order — and sweep for issues
carrying **no queue label at all**, which is how #773 and #774 sat outside both
queues for a day. **The CLI's own `validate` subcommand, #720, LANDED
2026-09-04** (`f4d8f76`) and moved no lane by construction — `go list -deps
./conformance | grep -c cmd/goxsd8` is `0`. It is the reason this milestone's
count moves independently of its lane, and the 2026-09-05 window is the cleanest
demonstration yet: **three of its four subcommand issues LANDED** — #1223
(`4ab65d6`, exit 4 for an instance the assessment declined to decide), #1229
(`20010ed`) and #1230 (`38ade3e`) — **three more were filed onto it** (#1250,
#1251, #1257), and the milestone's open count did not move at all while
`instance` stayed flat at 11029. **Every M5 issue that entered or left in that
window was `cmd/goxsd8`.** **The roster of open subcommand issues is not written
here.** It went stale twice inside four days — #1260 and #1261 landed, then
#1251 — for the reason the lane figure below is not written here either (#646).
Read it off the queue: `area/cmd` on this milestone. The durable half is the
milestone rule behind that roster, and it is the reason the count reads low: **a
subcommand issue spanning `parse` (M4) as well as `validate` carries NO
milestone deliberately**, so this milestone's `cmd/goxsd8` work is undercounted
here rather than overcounted. Read the count as a floor and never as the lane's
remaining work.

**The lane score is a floor built for soundness, and no jump has ever changed
what the number means.** The lane observes "valid" only for a violation-free
`Result` with nothing unevaluated whose root passes one of the shape gates
`conformance/instance.go` names (#1738, #1808, #1841). Each gate admits only
shapes where the lane gate or the walk decides every applicable clause. Any
other violation-free `Result` DECLINES rather than passing, because `Assess` does
not evaluate `e-validity`'s other conjuncts for it. **Every other passing case is
an expected-INVALID one by construction**, not by measurement, and the failures
that remain are overwhelmingly declines rather than disagreements. The
milestone's remaining slices are what turn declines into decisions.

**Do not read the lane score as a pass rate.** It is the count of documents this
engine can honestly call not-valid. **Read the current figure from the Status
section's table and never from this paragraph** — an absolute figure written
here is stale before the next landing (#646). It grew most because #913
decided `cvc-type` clause 3.1 — the commonest simple-typed-leaf shape the lane
had declined outright — which is the counterpart to #790's lesson, not a
contradiction of it: a slice that decides a *new* rule moves the number far
MORE than its rule count suggests when the declined shape is common, and #913
moved it more than #790's descent did.

**Every suite-VALID `instance` case the lane decides wrong has an owner, and the
suite-INVALID ones it decides valid are a class the subtree gate made.** Read the
counts from the Status section's partition, never from this paragraph. The
suite-valid false rejects are #921's (two `gMonth` cases with an unmodeled
`queried` status); #771 decided the ones whose schema documents only the
instance's own `xsi:schemaLocation` names. The suite-invalid
admissions are cases #1841's gate admits on a `value.ValidateLexical` verdict
inside its trust boundary. Each scores Fail, and none is a false pass. The
18 `MS-Regex` cases among them, each holding an astral code point, are #1899's
permanent suite-expectation divergence.

The decline census that separated harvest candidates from indeterminates
predates every M5 landing from #766 onward and every outside-M5 mover — #909,
#853 and #1116 included — and is not re-derived here. **It is now, by a wide margin, the oldest measurement this milestone still
argues from**, and #786 is the nearest issue to it.

The design constraints are fixed by `validate/doc.go` and PRINCIPLES 8, 11,
13, 14 and 15, and the carve does not reopen them.

### M6 — XPath required subset

CTA restricted subset plus assertion essentials, fail-open with `GAP`
markers, IDC selector and field paths. Dynamic-error direction per
PRINCIPLES 20. Not filed as an epic — speculative epics two milestones out
earn nothing.

**Part of this milestone has already shipped, ahead of it and outside it.**
**#842** landed the §3.12.6 required-subset CTA evaluator on 2026-08-16 at
`3509de6` — pulled forward because M5's `instance` lane cannot decide a
conditionally-typed element without it — and `xpath` stopped being a
`doc.go`-only package. **#861** then corrected its clause-2 error handling and
**#851** gave §3.12.2's inline arm a real anonymous component. Three more
landed on 2026-08-18: **#858** (`d02aa59`, the three cast-shaped constructs),
**#886** (`3867fb5`, `ta-props-correct` clause 2 charged at schema construction
time) and **#887** (`f1250c0`, comparator legality decided from `xpath20.md`
Appendix B.2's generated rows). **All three moved no lane, and all three said
so** — the required subset is now largely built, and building it has not been
what converts cases.

What remains of the CTA subset is **#889** (value-level numeric widening, §B.1
rule 1.1's float→double promotion, which #858 withheld rather than faked) and
**#2631**, the cast-target static error a later decline discards. **#894**
(err:XPST0051/XPST0080, the static remainder #886 did not charge) landed
2026-10-14 at `4914723`, `schema` +1 (`s3_12ii06s`).
**#888**, a cast target that is `xs:QName`, was absorbed by the assertion slice
#2587 (`6f714c3`, 2026-10-13). **#859**, the wildcard `ta-AttrName`
arms, **landed 2026-08-18 at `ea0650a`** — this paragraph carried it as
remaining work for fifteen days, arguing about a stale lease on a branch whose
issue had already closed the same day. **#871**, the §3.12.4 clause 1.1.3
·inherited attributes· merge, landed on 2026-09-25 at `2e9ab84`; #1682 (the
`ref=` use-level `{inheritable}` fallback) now has it as a live reader. Neither
of the two still open (#889, #2631) carries a milestone, which is the same
pattern M4's tail records.

**Assertion evaluation is #1042, the GAP tracker, `blocked` on its slices.** It
owns `cvc-assertion` (§3.13.4.1) and `cvc-assertions-valid` (§4.3.13.3) and
retires the `GAP(validate)` markers **#719** landed on 2026-08-27 at `bd887cd`,
one milestone early, because the `instance` lane must decline every case whose
outcome turns on an assertion. #1042 closes when no assertion decline is left.

**Tier 2 is carved one slice at a time.** Landed: **#2232** (`e3490bd`,
`instance` +8, complex-type `cvc-assertion` over the grammar `CompileCTATest`
parses, attributes typed), **#2245** (`08eb2de`, +1, `$value` and the value
comparators), **#2244** (`01bfdab`, +8, xs:untypedAtomic attributes), **#2267**
(`21311ef`, +12, child-axis paths), **#2243** (`3990401`, unchanged, the
implicit timezone), **#2246** (`9f8d73d`, +11, simple-type
`cvc-assertions-valid` through an injected `value.AssertionEvaluator`),
**#2268** (`7fa21b3`, +19, `fn:count` and `.//`), **#2269** (`43661a5`, +17,
arithmetic), **#2295** (`9af63ae`, +15, the F&O string and sequence core),
**#2298** (`645657f`, unchanged, a fixed value over an asserting type),
**#2340** (`0ec64ec`, +8, `fn:exists`/`fn:empty` and child paths `a/b`),
**#2273** (`47ca5b3`, +4, identity and up-the-hierarchy casts), **#2342**
(`ba56979`, unchanged, a union's own facet), **#2322** (`9bf4e48`, unchanged,
·skipped· subtrees in a count) and **#2339** (`4e4e228`, unchanged, float/double
casts now decline), **#2321** (`747b425`, +3, existence EBV), **#2383**
(`354d3d7`, unchanged, a literal cast that decided wrong now declines),
**#2318** (`2328932`, +6, predicates and `|` inside `fn:count`), **#2320**
(`cb842d0`, +2, `count($value)` and `distinct-values`), **#2387** (`c473241`,
+2, a mixed child's string-value), **#2358** (`610f599`, +2, `fn:string` over a
typed date/time), **#2341** (`5b18339`, +2, `if` in a boolean position),
**#2448** (`b8486b1`, +3, `.` and `to`, replacing the parked #2332), **#2451**
(`1a1922c`, +9, `fn:current-date`), **#2450** (`a6e20db`, +7, `castable as`),
**#2452** (`c7b3054`, +2, `instance of` and `fn:data`), **#2472** (`c895157`,
+3, a string-literal sequence), **#2468** (`ec19d7c`, +2, `.` under
element-only content), **#2453** (`bb84f11`, +2, facet `position()`/`last()`),
**#2473** (`e0c02cf`, +2, `namespace-uri`/`in-scope-prefixes`), **#2442**
(`b79e639`, +1, `fn:concat`), **#2445** (`bc6dedb`, unchanged, `N/*` and
`N[a and b]`), **#2319** (`f819aed`, +6, a quantifier over E's children decided
by attribute or next sibling), **#2522** (`2c089be`, +3, a `preceding::` step in
a nested predicate), **#2520** (`265de89`, +2, a quantifier over an atomized
`$value`), **#2462** (`ad2874a`, unchanged, a Type Alternative's cast
declining an assertions facet rather than deciding it at a zero instant),
**#2446** (`6af2d74`, +1, `fn:sum` over an attribute path), **#2521**
(`f6e9811`, +1, a quantifier over a positional child window), **#2477**
(`3e715c7`, +1, `data(a/b)`), **#2476** (`e4d6086`, +1, `child::N` and `N[k]`
behind `fn:data`), **#2478** (`e77235b`, +1, an AttributeTest under `instance
of`), **#2509** (`dda7940`, unchanged, a typed EBV operand decided by
`ctaCarriedType`), **#2579** (`52342d6`, +2, `fn:doc` raises), **#2578**
(`e63cf8c`, +2, a `comment()` step selects nothing), **#2553** (`d1113f8`,
+1, a bound child's own attributes, with `fn:number` and
`fn:substring-after`) and **#2587** (`6f714c3`, +1, a StringLiteral cast to
`xs:QName`, absorbing #888).
#1042's `## Depends on` lists the open slices and the residue not yet filed;
this paragraph does not repeat them.

**#56 LANDED on 2026-08-28 at `3160813`**, ten days after #719 unblocked it and
`Ratchet: unchanged` as its body predicted. It records the CTA compile-time
withhold into `Result.Unevaluated` under `key-cta-ta-select` (§3.12.4), with no
second type and no `Evaluated bool` — the encoding #719 shipped
(`Rule()`/`Loc()`/`Msg()`, `Result.Unevaluated()` in document order), reused
rather than paralleled (STYLE D3), exactly as #842's warden pre-flight had ruled
on D3. **Surface: none new.** STYLE P3's fail-open discipline is only honest if a
fail-open answer is distinguishable from a real pass, and both of this
milestone's fail-open channels — the assertion sites #719 collects and the CTA
withhold #56 records — now reach the same slice under their own rule IDs.

**Its first consequence was a documentation defect, not a code one.**
`validate/doc.go`'s Contract states that an empty `Violations()` beside a
non-empty `Unevaluated()` is not a pass, while README's validation snippet — the
module's only working one, since `validate` ships no runnable `Example` (#1088)
— named `res.Err()` as the sole incompleteness signal. #56's landing is what
made that reachable on an ordinary conditionally-typed document rather than only
on one carrying an assertion. **#1122** landed the fix at `a458893`; the
snippet's remaining wording is #1292's.

### M7 — XPath 2.0 growth

Grammar completion toward full XPath 2.0 plus the F&O function library
(`docs/specs/md/xpath20.md`, `xpath-functions.md`). **`xpath` lane.**

### M8 — JSON instance adapter

`validate/jsonsrc` mapping JSON onto the abstract infoset. **`json` lane**
— curated cases; the W3C suite has no JSON lane.

### M9 — Codegen

Deterministic emission, namer, sealed choice sums, capability-view
interfaces, multiple schemas to multiple output dirs, golden-file tests.
The public `value.Emitter` API freezes here.

### M10 — Codec

Runtime path plus generated fast path; differential tests (identical
values, identical error rule IDs) and `testing.AllocsPerRun` budgets.

### M11 — BER instance adapter

`validate/bersrc`. **`ber` lane** — curated cases.

### M12 — Native backend completion

`builtin/native` mappings and emitter, backendtest green, performance
pass.

## v1.0 — the stability line

1.0 is declared by a human, not by a milestone rollover (expected after
M12). Until then **pre-1.0 mobility** applies: interfaces, package
boundaries and exported names move freely whenever the steward's audit
finds a better placement, and the ratchet and the gate are the only
compatibility promises. After 1.0, exported-surface changes require a
deprecation path and a compatibility argument, and the audit's posture
flips from "move it now" to "guard the surface". Narrower freezes may land
earlier where a milestone says so — `value.Emitter` at M9.

## Non-goals

- Schema mutation and editing APIs.
- XSD 1.0 compatibility quirks — this is an XSD 1.1 processor.
