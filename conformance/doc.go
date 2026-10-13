// Package conformance runs the W3C XSD test suite against this processor
// and enforces the ratchet: conformance only moves up.
//
// This package is test-only. Nothing in the library imports it.
//
// # The suite
//
// The W3C suite lives at testdata/xsdtests (a pinned git submodule;
// populate with `git submodule update --init testdata/xsdtests`). Its
// index is suite.xml, which references test sets; each test group carries
// schema tests and instance tests with declared expected outcomes. The
// auxiliary extra-suite.xml sibling is discovered alongside it (issue #135):
// it carries the precisionDecimal test sets the W3C suite moved out of
// suite.xml when the type left XSD 1.1, and shares one test set
// (common/introspection.testSet) with the main index, discovered once.
//
// A test set, test group, schema test or instance test may declare a
// `version` attribute listing the versions and FEATURES it applies to,
// OR-connected: a processor supporting any listed token runs it. A level
// listing no token this processor supports is not applicable and yields no
// cases — it is absent from the run, not declined and not scored (issue
// #446); discovery WITHHOLDS every case it would have produced, which is
// what makes an already-committed expectation for one a sanctioned
// applicability removal (below) rather than a Vanished regression. This
// processor supports the tokens `1.1` and `Unicode_6.0.0`, so groups scoped
// to XSD 1.0 only, and groups scoped to the `full-xpath-in-CTA` feature
// whose engine is unlanded, are both out of scope. An absent `version`
// applies to everything.
//
// `expected/@version` is a different question answered from the same claimed
// tokens: they are AND-connected there, and choose WHICH declared outcome
// binds a case this processor already runs. A case whose every declaration
// is scoped to a configuration this processor does not claim has no
// prescribed outcome for it and is DECLINED — recorded fail, never scored a
// pass — rather than bound to whichever declaration the suite author wrote
// first (issues #446, #862).
//
// # The curated JSON corpus
//
// The json lane's cases are not the suite's: they live at
// conformance/testdata/json/json.testSet, a testSet in the suite's own catalog
// format beside the schema and JSON documents it names. Each instanceTest is
// one json case, ID `goxsd8-json/<group>/json/<test>`, assessed through
// validate/jsonsrc against its group's schemaTest documents; a schemaTest
// there is not a case. An invalid case's documentation opens "expects
// <rule>", the cvc-* rule the engine charges it under, and a unit test holds
// it to that. Add a case by adding an instanceTest and its document.
//
// TestConformance reads the corpus beside the suite (jsoncases.go). Neither
// parseSuite nor the Catalog reader sees it, and only the json lane's selector
// claims its cases, so it moves no other lane's total.
//
// # Lanes and expectation files
//
// Expectations are committed at conformance/testdata/expectations/, one
// lane per file:
//
//	datatypes.txt   simple-type / facet cases            (from M3)
//	schema.txt      schema-validity cases                (from M4)
//	instance.txt    instance-validity cases              (from M5)
//	xpath.txt       XPath engine cases                   (from M7)
//	json.txt        JSON-adapter cases (curated)         (from M8)
//	ber.txt         BER-adapter cases (curated)          (from M11)
//
// A case may appear in more than one lane file: lane populations overlap, and
// each lane scores the case under its own executor (see the lane type in
// conformance/runner.go). Every datatypes.txt case is also scored by the
// instance lane.
//
// File format: one case per line, `<case-id> <pass|fail>`, sorted by case
// ID; `#` starts a comment. `pass` means this processor agrees with the
// suite's declared outcome; `fail` records a known gap so a regression is
// loud and an improvement is harvestable. A missing lane file is an empty
// lane, not an error. Expectation files are machine-written only — never
// edited by hand, and NEVER edited downward. A line leaves a file only as a
// sanctioned applicability removal, and a `pass` becomes `fail` only as a
// superseded pass (both below).
//
// A suite case may declare validity="indeterminate": the Working Group could
// not agree the case has one right answer, and the suite's own catalog DTD
// makes that a category disjoint from valid|invalid. Such a case is DECLINED —
// no executor is run for it and it is always recorded `fail` — so it can never
// score a pass (issue #277). Scoring it as if it meant "invalid" would have
// credited this processor for rejecting a document nobody agrees should be
// rejected. The decline is a harness-scoring convention, not a spec
// requirement: XSD 1.1 makes [validity] three-valued and states that schema
// validity is not a binary predicate, so there is no basis for equating
// indeterminate with invalid.
//
// # Mechanics (the M1 implementation contract)
//
//	LoadExpectations(path) (map[string]Status, error)
//	    Missing file => empty lane.
//
//	Compare(expected, actual, withheld) (Delta, error)
//	    Delta partitions cases into Improved (expected fail, now pass),
//	    Regressed (expected pass, now fail — never acceptable), New (no
//	    expectation yet), Removed (a sanctioned applicability removal —
//	    see below) and Vanished (expected case the run no longer produced
//	    and did not withhold). The sixth class, Superseded, Compare never
//	    fills: only Ratchet's superseded names move a case there (below).
//	    withheld is the runner's authoritative list of case IDs discovery
//	    declined to produce; an ID it does not name can never be Removed. A withheld case the run also produced is a
//	    runner bug and is returned as an error.
//
//	Ratchet(expected, actual, withheld, removals, superseded) (map[string]Status, error)
//	    Merge, upward-only bar the cases superseded names: Improved flips
//	    to pass, New is recorded at its observed status, Removed cases have
//	    their lines DELETED, and Superseded cases are recorded fail. Any
//	    Regressed or Vanished case aborts the entire merge with an error —
//	    the ratchet refuses to move at all rather than record a downgrade —
//	    and so do a Removed count other than the one removals asserts
//	    (AssertRemovals; the zero RemovalAssertion asserts none) and a
//	    superseded name that did not regress in the lane. Ratchet decides
//	    ONE lane; the runner merges every lane before it writes any,
//	    so a refusal here withholds every lane's file (issue #581).
//
//	WriteExpectations(path, m) error
//	    Always sorted by case ID (STYLE D1/D2).
//
//	Catalog(suiteDir) ([]CatalogEntry, error)
//	    Every entry the suite catalog rooted at suiteDir names — one per
//	    declared schemaTest and instanceTest, sorted by case ID — with the
//	    documents it names as slash paths relative to the suite root, the
//	    declared outcome, and whether discovery withholds it. It READS the
//	    catalog: it runs no case, scores nothing, and its answer reaches no
//	    lane. The set of IDs it reports is LARGER than the set a run
//	    produces, withheld entries being catalog entries that carry no line
//	    in any lane, so a caller counting cases that could flip a score
//	    joins these IDs against a lane file rather than counting them
//	    (issue #1412). What it will not do is describe an APPLICABLE entry
//	    declaring no expected outcome: that entry is refused, in discovery's
//	    own words, rather than reported with an outcome the catalog never
//	    declared. Its consumers are `go tool casejoin`, which turns a
//	    `go tool suiteindex` census of fixture paths into those IDs
//	    (issue #1642), and `go tool lanepartition`, which clusters a lane's
//	    banked fails by declared outcome and test set (issue #1740).
//
// # Sanctioned applicability removals
//
// The W3C suite carries its own applicability metadata: a testSet, testGroup,
// schemaTest or instanceTest may be scoped to processor versions or features
// this XSD 1.1 processor does not claim, in which case the case was never this
// processor's to score. When discovery withholds such a case, its committed
// expectation is a SANCTIONED REMOVAL rather than a Vanished regression, and
// banking it deletes the line (issue #576, a repo-owner ruling; a merely
// tolerated removal would be re-offered and re-refused on every later run).
//
// Two independent locks keep that from becoming a way to lower a score. The
// classifier is the runner's withheld set — an ID no discovery level named is
// Vanished, full stop, and no narrative can move it. The second lock is the
// arbiter's per-lane asserted count: unless the number of removals in EVERY lane
// is exactly the number that lane's run predicted, the entire merge is refused
// and no lane's file is written (issue #581), so a removal that silently
// appears, disappears, or changes lane cannot pass. Everything else is
// unchanged — scores still only move up, and genuine Regressed and Vanished
// cases still abort the merge whatever the assertion says.
//
// A withheld case that carries no committed expectation in ANY lane is not a
// removal — no lane has a line to delete — and classifies as nothing, so
// nothing else a run prints names it. That remainder is what a ratchet
// PREDICTION must exclude: a case no lane banked cannot flip a lane's score
// (issue #1412). It is listed on demand, on both the read-only and ratchet
// paths:
//
//	GOXSD_WITHHELD=1
//	    Lists the withheld case IDs no lane's committed file carries, sorted.
//	    Deliberately not the whole withheld set: the suite-wide total is
//	    logged every run, and the withheld IDs that DO carry a line are
//	    already printed per lane as sanctioned removals on the read-only
//	    path (issue #1514) — the ratchet path banks them instead of
//	    printing them.
//
// # Superseded passes
//
// A banked `pass` that held only because two defects cancelled out, until a
// correct fix to one exposed the other, is a SUPERSEDED PASS: not a regression,
// and bankable as `fail` (issue #1827, a repo-owner ruling). No machinery can
// tell one from a genuine regression, so the class is asserted BY CASE ID,
// never by count — a count would let one genuine regression stand in for the
// asserted case. The arbiter names each case per lane on the ratchet run:
//
//	GOXSD_RATCHET_SUPERSEDED=<lane>:<case-id>,...
//	    e.g. `instance:VC/vc002/instance/vc002.n1.xml`. Ratchet moves each
//	    named case out of Regressed into Superseded and banks it `fail`. A
//	    regression it does not name still aborts; so does a named case that
//	    is not Regressed in that lane (still passing, Vanished, or never
//	    banked `pass`), and so does a malformed value — a missing `:`, an
//	    unknown lane, an empty ID, a lane:ID pair named twice. One ID in two
//	    lanes is two names. Gated as GOXSD_RATCHET_REMOVALS is: set without
//	    GOXSD_RATCHET=1, the run FAILS. Unset, no case is named.
//
// When a name is justified is the arbiter's to rule (.claude/agents/arbiter.md,
// "Ratchet integrity"); this package checks only that each name is a
// regression of the lane it names. A read-only run takes no names, so it still
// fails on a superseded case as Regressed.
//
// # Running
//
//	go test ./conformance -run TestConformance -count=1
//	    Read-only: runs the suite, Compares against committed
//	    expectations, fails on any Regressed case. Improved cases and
//	    sanctioned applicability removals are logged (visible with -v),
//	    never written: a non-arbiter agent can thus see the movement
//	    pending banking (issue #303) without being able to bank it.
//
//	GOXSD_RATCHET=1 go test ./conformance -run TestConformance -count=1
//	    Additionally Ratchets each lane and rewrites its file. Arbiter
//	    only (see docs/WORKFLOW.md); every flipped case must be
//	    explainable by the diff under judgment. Writing is all-or-nothing
//	    across lanes (issue #581): every lane is merged first, and one
//	    lane's refusal leaves EVERY lane's file untouched, so a refused
//	    run never leaves a half-banked tree behind its FAIL.
//
//	GOXSD_RATCHET_REMOVALS=<lane>=<n>,...
//	    The arbiter's per-lane assertion of how many sanctioned
//	    applicability removals this run will bank, e.g.
//	    `schema=34,instance=65`. Any other number refuses the entire merge
//	    — every lane's, not only the miscounted lane's — as does a name no
//	    lane carries. It covers a ratchet run ONLY: set without
//	    GOXSD_RATCHET=1, the run FAILS rather than quietly ignoring it —
//	    the same rule GOXSD_SUITE_OPTIONAL follows below. Unset, every lane
//	    asserts none, which refuses any removal.
//
// The runner supports single-case reproduction for debugging:
// GOXSD_CASE=<case-id> narrows the run to one case with debug logging.
//
// # The decline census
//
// An executor either DECIDES a case — it observes this processor's outcome and
// compares that with the suite's declared one — or DECLINES it, because no
// reader recognized the fixture's shape. Both record `fail`, so a decline is a
// snapshot of engine capability taken when the decline code was WRITTEN, and
// nothing re-checked it: d3_3_4v16 stayed wrongly declined for nine days after
// the capability that covers it landed, until an unrelated reader edit stumbled
// over it (issue #327).
//
// Every run therefore re-attempts each recorded failure with its declared
// outcome flipped, which separates decided failures (the flipped run passes)
// from declines (Fail under both polarities), and logs the per-lane decline
// count (visible with -v). That count is the harvest queue's size, so it moves
// visibly at the landing that widens an engine. The census reports only: it
// scores no case and writes no expectation.
//
// Neither census count is the lane's SCORE, and every logged census line says
// so where it prints (issue #1120). They partition the failures of ONE RUN,
// which is not the set the committed file records: a case this run decided and
// got wrong is in neither count, and a case whose expectation still reads
// `fail` while this run passes it is in no count at all. The lane's score is
// the census of its expectation file — what `go tool lanestatus` prints, and
// what docs/WORKFLOW.md's "Claims that outlive the session" binds a `docs/LOG`
// entry and a `Ratchet:` trailer to quote.
//
//	GOXSD_DECLINES=1
//	    Additionally logs, per lane, three sorted case-ID lists partitioning
//	    the run's recorded failures: `decline candidates:`, `indeterminate
//	    declines:` (the #277 cases the second count counts), and `decided
//	    disagreements:`, where a lane with a charge probe (lane.charge; the
//	    schema lane) writes each ID as `<id>=<charge>` — the rule
//	    xsderr.RuleOf reads off the assembly's error, `(unruled)` for an error
//	    carrying none, `(accepted)` for an assembly that succeeded. A lane
//	    whose executor names its refusals (the instance lane) writes each
//	    decline candidate as `<id>=<refusal>` — the token naming the exit
//	    that declined it, one of instance.go's refuse* constants, read off
//	    the census's own probe run (#2008). An unevaluated decline's token
//	    is `unevaluated:<rule>[,<rule>…]`, each distinct
//	    validate.Unevaluated.Rule of the records behind it once, in the
//	    order the result first lists it (#2106). The schema and datatypes
//	    lanes list candidates by ID alone.
//	    tools/internal/declinecensus reads these lines for `go tool
//	    lanepartition -log` (#1740) and `go tool casejoin -log` (#1785), so
//	    a label renamed in reportDeclines is renamed there. Opt-in because a
//	    lane still awaiting its milestone declines every case it claims and
//	    would bury the run's other reporting.
//
// # Missing suite
//
// TestConformance FAILS when testdata/xsdtests is not populated, naming
// `git submodule update --init testdata/xsdtests` (issue #309). A run that
// executed zero cases must never be reportable as a green run, and a ratchet
// run over an empty suite must never be reportable as "no movement".
//
//	GOXSD_SUITE_OPTIONAL=1
//	    Explicit opt-out for an environment that legitimately has no suite
//	    checkout: the missing-suite failure becomes a skip. It applies to
//	    read-only runs ONLY — under GOXSD_RATCHET=1 the run still fails.
//
// The supplementary fixture-driven tests in this package (datatypes_test.go,
// schema_test.go, instance_test.go, census_test.go) keep their plain skip on
// a missing suite, unconditionally (skipWithoutSuite). They are deliberately
// treated differently from TestConformance: each drives a named cohort of suite
// documents to prove one executor decides them for the right reason, so
// skipping them loses unit coverage but cannot dress an empty run up as
// suite-wide conformance. TestConformance is the only test whose silence was
// mistakable for a score.
package conformance
