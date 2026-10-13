package conformance

import (
	"fmt"
	"os"
	"slices"
	"testing"
)

// TestConformance runs the W3C suite, and the curated JSON corpus beside it
// (curatedJSONCases), through the lane executors and enforces the ratchet, per
// conformance/doc.go.
//
//   - Submodule absent: fail with a pointer to `git submodule update --init`
//     (issue #309), unless a read-only run explicitly opted out with
//     GOXSD_SUITE_OPTIONAL=1. A run that executed zero cases must never be
//     mistaken for a green one, and the opt-out never covers a ratchet run.
//   - Read-only (default): Compare each lane's observed run against its
//     committed expectations and fail only on a Regressed case; New/Improved/
//     Vanished do not fail the read-only run (doc.go "Running"). Improved is
//     logged so an agent that cannot run the ratchet can still see the upward
//     movement its branch earned but has not banked (issue #303).
//   - Every lane, on either path, reports its decline census (issue #327): the
//     count of recorded failures no executor decided at all, re-checked this run
//     rather than trusted. GOXSD_DECLINES=1 also lists their IDs.
//   - Sanctioned applicability removals — cases discovery withheld because the
//     suite's own metadata scopes them away from this processor (issue #576) —
//     are logged on the read-only path and never fail it. GOXSD_WITHHELD=1
//     additionally lists, on either path, the withheld IDs no lane banked at
//     all (doc.go "Sanctioned applicability removals").
//   - GOXSD_RATCHET=1: additionally Ratchet each lane and rewrite its file;
//     a Ratchet refusal (an unnamed regression, vanished, a named superseded
//     case that did not regress, or a removal count the run did not assert)
//     fails the test. Arbiter only. Writing is all-or-nothing across
//     lanes (ratchetAll, issue #581): every lane merges first, and one lane's
//     refusal leaves every lane's file untouched.
//   - GOXSD_RATCHET_REMOVALS=<lane>=<n>,…: arbiter-only, ratchet-path-only
//     assertion of the sanctioned removals each lane is expected to bank; set
//     without GOXSD_RATCHET=1 it fails the run rather than half-applying.
//   - GOXSD_RATCHET_SUPERSEDED=<lane>:<id>,…: the same gate, naming the
//     superseded passes each lane banks as `fail` (issue #1827).
//   - GOXSD_CASE=<id>: narrow execution to one case across all lanes.
func TestConformance(t *testing.T) {
	ratcheting := os.Getenv("GOXSD_RATCHET") == "1"
	removals := assertedRemovals(t, ratcheting)
	superseded := assertedSuperseded(t, ratcheting)
	index := suitePath()
	if err := checkSuitePresent(index); err != nil {
		endUnusableSuiteRun(t, err, ratcheting)
		return
	}

	found, err := parseSuite(index)
	if err != nil {
		t.Fatalf("parsing suite: %v", err)
	}
	curated, err := curatedJSONCases(jsonCorpusIndex)
	if err != nil {
		t.Fatalf("reading the curated JSON corpus: %v", err)
	}
	t.Logf("discovered %d cases across %d lanes, %d withheld as inapplicable, and %d curated JSON cases",
		len(found.cases), len(defaultLanes()), len(found.withheld), len(curated))

	cases := append(slices.Clip(found.cases), curated...)
	if only, ok := os.LookupEnv("GOXSD_CASE"); ok {
		cases = narrowToCase(t, cases, only)
	}

	lanes := defaultLanes()
	runs := make([]laneRun, 0, len(lanes))
	for _, l := range lanes {
		runs = append(runs, observeConformanceLane(t, l, cases))
	}
	reportWithheld(t, found.withheld, runs)

	if !ratcheting {
		for _, r := range runs {
			reportLaneReadOnly(t, r, found.withheld)
		}
		return
	}
	if err := ratchetAll(expectationsDir, runs, found.withheld, removals, superseded); err != nil {
		t.Error(err)
	}
}

// assertedRemovals reads the arbiter's per-lane removal assertion
// (ratchetRemovalsEnv) and ends the run on anything removalAssertions refuses —
// a value set off the ratchet path, or a malformed one. The gate lives in
// removalAssertions so it is testable; this is only its env binding, in the same
// shape endUnusableSuiteRun uses for the suite opt-out (issue #309).
func assertedRemovals(t *testing.T, ratcheting bool) map[string]RemovalAssertion {
	t.Helper()
	raw, set := os.LookupEnv(ratchetRemovalsEnv)
	byLane, err := removalAssertions(raw, set, ratcheting)
	if err != nil {
		t.Fatalf("%s: %v", ratchetRemovalsEnv, err)
	}
	return byLane
}

// assertedSuperseded reads the arbiter's per-lane superseded names
// (ratchetSupersededEnv) and ends the run on anything supersededAssertions
// refuses, in assertedRemovals' shape.
func assertedSuperseded(t *testing.T, ratcheting bool) map[string][]string {
	t.Helper()
	raw, set := os.LookupEnv(ratchetSupersededEnv)
	byLane, err := supersededAssertions(raw, set, ratcheting)
	if err != nil {
		t.Fatalf("%s: %v", ratchetSupersededEnv, err)
	}
	return byLane
}

// suiteRunEnd is how a run whose suite index is unusable ends: fatal says the
// run FAILS rather than skips, and msg is the single line it prints. Returning
// the decision instead of taking a testing.TB keeps the policy exercisable in
// all four env combinations from an ordinary table test (issue #375).
type suiteRunEnd struct {
	fatal bool
	msg   string
}

// unusableSuiteEnd decides how the run ends when checkSuitePresent refuses the
// index. optedOut is GOXSD_SUITE_OPTIONAL=1 and ratcheting is GOXSD_RATCHET=1,
// taken as arguments so this stays a pure decision its caller binds to the
// environment — the same split removalAssertions uses for the other opt-in.
//
// The policy: a missing suite is a hard failure by default, so a container
// without the submodule cannot exit `ok` (issue #309). The opt-out downgrades
// that to a skip for a READ-ONLY run alone, never for a ratchet run whose whole
// output would otherwise be an unearned "no movement".
func unusableSuiteEnd(err error, optedOut, ratcheting bool) suiteRunEnd {
	if !optedOut {
		return suiteRunEnd{
			fatal: true,
			msg:   fmt.Sprintf("%v (set %s=1 only in an environment that legitimately has no suite)", err, suiteOptionalEnv),
		}
	}
	if ratcheting {
		return suiteRunEnd{
			fatal: true,
			msg:   fmt.Sprintf("%v (%s=1 does not cover a ratchet run: an empty suite must not report no movement)", err, suiteOptionalEnv),
		}
	}
	return suiteRunEnd{msg: fmt.Sprintf("%v (skipped: %s=1)", err, suiteOptionalEnv)}
}

// endUnusableSuiteRun reports unusableSuiteEnd's decision, binding it to the
// environment. Every path ends the run, but the caller returns rather than
// trusting that: the fall-through would run the suite on an index just proved
// missing (issue #375).
func endUnusableSuiteRun(t *testing.T, err error, ratcheting bool) {
	t.Helper()
	end := unusableSuiteEnd(err, os.Getenv(suiteOptionalEnv) == "1", ratcheting)
	if end.fatal {
		t.Fatal(end.msg)
	}
	t.Skip(end.msg)
}

// suiteAbsentSkipMsg is what a fixture-driven test prints when the suite is
// absent. It names the submodule-init command as written from the MODULE ROOT
// (suiteModulePath), which is where a reader of a `go test ./...` skip line
// stands — the package-relative suiteRoot would print a command that fails from
// there (issue #374).
const suiteAbsentSkipMsg = "W3C suite not present; run `git submodule update --init " + suiteModulePath + "` from the module root"

// skipWithoutSuite skips the calling test when the suite index is absent. It is
// the one body of the unconditional skip the fixture-driven tests in
// datatypes_test.go, schema_test.go and instance_test.go each take (doc.go
// "Missing suite"); TestConformance never skips this way.
func skipWithoutSuite(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(suitePath()); err != nil {
		t.Skip(suiteAbsentSkipMsg)
	}
}

// observeConformanceLane executes one lane and reports what the run saw of it:
// its case count and its decline census (issue #327). Both are pure reporting
// and belong to either path, so this pass is the same for a read-only and a
// ratcheting run. It judges nothing about the score — the read-only verdict and
// the upward-only merge both happen once EVERY lane has been observed, which is
// what lets one lane's refusal be known before any lane's file is written
// (ratchetAll, issue #581).
func observeConformanceLane(t *testing.T, l lane, cases []caseSpec) laneRun {
	t.Helper()
	actual := runLane(l, cases)
	expected, err := LoadExpectations(laneFile(l.name))
	if err != nil {
		t.Fatalf("lane %s: loading expectations: %v", l.name, err)
	}
	t.Logf("lane %s: %d cases", l.name, len(actual))
	reportDeclines(t, l, cases, actual)
	return laneRun{name: l.name, expected: expected, actual: actual}
}

// reportLaneReadOnly applies the read-only policy: fail on a Regressed case, and
// on nothing else about the score. Improved cases and sanctioned applicability
// removals are reported through t.Logf so an agent who cannot run the ratchet
// still sees what its branch would bank; banking either needs GOXSD_RATCHET=1,
// and a removal needs the arbiter's asserted count on top of that.
//
// A Compare error is a different animal and does fail the read-only run: it means
// discovery both withheld and produced one case, which is a runner bug rather
// than a conformance result, and no run should proceed on a self-contradicting
// case list.
//
// withheld is the suite-wide set of case IDs discovery ruled inapplicable; it
// needs no per-lane routing because a withheld ID only classifies against the
// lane whose committed expectations carry it.
func reportLaneReadOnly(t *testing.T, r laneRun, withheld []string) {
	t.Helper()
	d, err := Compare(r.expected, r.actual, withheld)
	if err != nil {
		t.Errorf("lane %s: %v", r.name, err)
		return
	}
	if len(d.Regressed) > 0 {
		t.Errorf("lane %s: %d regressed case(s): %v", r.name, len(d.Regressed), d.Regressed)
	}
	if len(d.Improved) > 0 {
		t.Logf("lane %s: %d case(s) improved, not yet banked (GOXSD_RATCHET=1 required to write): %v",
			r.name, len(d.Improved), d.Improved)
	}
	if len(d.Removed) > 0 {
		t.Logf("lane %s: %d sanctioned applicability removal(s), not yet banked (GOXSD_RATCHET=1 with %s=%s=%d required to write): %v",
			r.name, len(d.Removed), ratchetRemovalsEnv, r.name, len(d.Removed), d.Removed)
	}
}

// censusNotScore is the disclosure every decline-census line carries. Both
// counts partition ONE RUN's recorded failures — the decided disagreements are
// in neither, and a failure already flipped to pass this run is in no count at
// all — so neither is the lane's score, and a reader who quotes one into a
// `Ratchet:` trailer publishes a figure the committed tree cannot reproduce
// (issue #1120). It is one string appended to every census line rather than
// prose repeated in each, so the disclosure is worded identically on every line
// and is edited in one place (STYLE D3).
const censusNotScore = "not the lane score, which `go tool lanestatus` reports"

// reportDeclines surfaces the lane's decline census (issue #327) through the
// same t.Logf channel the Improved cases already use, so a -v run shows how many
// of the lane's recorded failures no executor decided at all. That count is the
// harvest queue's size: it drops at the landing that widens an engine, instead
// of a wrongly-declined case waiting for an unrelated reader edit to stumble
// over it. The census reports only — it scores nothing and writes nothing, so it
// runs on the ratcheting path too.
func reportDeclines(t *testing.T, l lane, cases []caseSpec, actual map[string]Status) {
	t.Helper()
	census := takeDeclineCensus(l, cases, actual)
	if len(census.candidates) > 0 {
		t.Logf("lane %s: %d declined case(s) recorded fail — harvest candidates re-checked this run (%s=1 lists them), %s",
			l.name, len(census.candidates), declinesEnv, censusNotScore)
	}
	if len(census.indeterminate) > 0 {
		t.Logf("lane %s: %d further case(s) declined as indeterminate, never harvestable (issue #277), %s",
			l.name, len(census.indeterminate), censusNotScore)
	}
	if os.Getenv(declinesEnv) != "1" {
		return
	}
	t.Logf("lane %s: decline candidates: %v", l.name, refusedIDs(census.candidates))
	t.Logf("lane %s: indeterminate declines: %v", l.name, census.indeterminate)
	t.Logf("lane %s: decided disagreements: %v", l.name, chargedIDs(l, census.disagreed))
}

// chargedIDs renders a census's decided disagreements for the GOXSD_DECLINES=1
// listing: each case's ID, followed by `=` and what the lane's executor charged
// it when the lane has a charge probe (lane.charge, issue #1740). The probe
// re-runs only these cases, and only under the opt-in, so a default run pays
// nothing for it.
func chargedIDs(l lane, disagreed []caseSpec) []string {
	out := make([]string, 0, len(disagreed))
	for _, c := range disagreed {
		if l.charge == nil {
			out = append(out, c.id)
			continue
		}
		out = append(out, c.id+"="+l.charge(c))
	}
	return out
}

// refusedIDs renders a census's decline candidates for the GOXSD_DECLINES=1
// listing: each case's ID, followed by `=` and the refusal the probe's
// executor run named where it named one (#2008). The refusal rides on the
// census's own probe, so the listing re-runs nothing.
func refusedIDs(candidates []declinedCase) []string {
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if c.why == "" {
			out = append(out, c.id)
			continue
		}
		out = append(out, c.id+"="+string(c.why))
	}
	return out
}

// reportWithheld lists the withheld case IDs no lane banked (withheldEnv). It is
// the one slice of discovery's withheld set no other line of a run names
// (doc.go "Sanctioned applicability removals").
//
// Like the decline census this reports only — it scores no case and writes no
// expectation — so it runs on the ratcheting path too, and the count is logged
// beside the list because the opt-in asked for both.
func reportWithheld(t *testing.T, withheld []string, runs []laneRun) {
	t.Helper()
	if os.Getenv(withheldEnv) != "1" {
		return
	}
	unbanked := unbankedWithheld(withheld, runs)
	t.Logf("%d of %d withheld case(s) carry no committed expectation in any lane: %v",
		len(unbanked), len(withheld), unbanked)
}

// narrowToCase keeps only the case whose ID equals only, failing clearly if no
// discovered case matches (GOXSD_CASE debugging aid).
func narrowToCase(t *testing.T, cases []caseSpec, only string) []caseSpec {
	t.Helper()
	for _, c := range cases {
		if c.id == only {
			return []caseSpec{c}
		}
	}
	t.Fatalf("GOXSD_CASE=%q matched none of the %d discovered cases", only, len(cases))
	return nil
}
