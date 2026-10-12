package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// cdNow is the current dateTime the fixtures below inject: 2026-10-07, at an
// offset of +01:00, half an hour past midnight — the UTC instant is still
// 2026-10-06, so a date taken in UTC rather than in the dateTime's own offset
// is a different date.
var cdNow = time.Date(2026, time.October, 7, 0, 30, 0, 0, time.FixedZone("", 3600))

// cdEval compiles expr for an E with empty content, or fails the test, and
// evaluates it with now as the current dateTime.
func cdEval(t *testing.T, expr string, now time.Time) bool {
	t.Helper()
	test, ok := afCompile(t, expr, xsd.EmptyContent{})
	if !ok {
		t.Fatalf("CompileAssertionTest(%q): declined, want compiled", expr)
	}
	return test.Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, nil, ValueBinding{}, now)
}

// fn:current-date is xs:date(fn:current-dateTime()) (xpath-functions.md §16.4)
// of the instant Evaluate is handed: after 2000-01-01 and not before it, the
// date in the instant's own offset, +01:00, and rendered with it (§17.1.5) —
// never the UTC date 2026-10-06 nor the implicit timezone Z, which F&O §10.4
// gives only the untimezoned operand: 2026-10-07+01:00 starts an hour before
// 2026-10-07 at Z, so it is less. Two calls in one {test} return the same date
// (§16.4 "stable"; cvc-xpath clause 6). Every row declines with the
// "current-date" arm of ctaParser.libraryCall removed; the two `eq
// xs:date(...)` rows and the string row fail with ctaCurrentDateItem rendering
// in.now.UTC(). The last row survives that mutation, 2026-10-06Z being less
// than 2026-10-07 too: it pins the implicit timezone Z, not the offset.
func TestAssertionCurrentDate(t *testing.T) {
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"current-date() gt xs:date('2000-01-01')", true},
		{"current-date() lt xs:date('2000-01-01')", false},
		{"current-date() = current-date()", true},
		{"current-date() eq current-date()", true},
		{"current-date() eq xs:date('2026-10-07+01:00')", true},
		{"current-date() eq xs:date('2026-10-06Z')", false},
		{"string(current-date()) = '2026-10-07+01:00'", true},
		{"current-date() lt xs:date('2026-10-07')", true},
	} {
		if got := cdEval(t, tc.expr, cdNow); got != tc.want {
			t.Errorf("Evaluate(%q) at %v = %v, want %v", tc.expr, cdNow, got, tc.want)
		}
	}
}

// The instant is the argument and nothing else: the same {test} answers by the
// date of whichever instant it is handed, so a caller holding one instant per
// episode gets one date per episode.
func TestAssertionCurrentDateReadsTheInstantHandedIn(t *testing.T) {
	const expr = "current-date() gt xs:date('2000-01-01')"
	if !cdEval(t, expr, cdNow) {
		t.Errorf("Evaluate(%q) at %v = false, want true", expr, cdNow)
	}
	before := time.Date(1999, time.December, 31, 12, 0, 0, 0, time.UTC)
	if cdEval(t, expr, before) {
		t.Errorf("Evaluate(%q) at %v = true, want false", expr, before)
	}
}

// The zero [time.Time] is an ordinary instant, 0001-01-01T00:00:00Z, and never
// the current time: fn:current-date over it is 0001-01-01Z, both for an
// element's assertion and for an assertions facet.
func TestCurrentDateZeroInstant(t *testing.T) {
	const expr = "string(current-date()) = '0001-01-01Z'"
	if !cdEval(t, expr, time.Time{}) {
		t.Errorf("Evaluate(%q) at the zero instant = false, want true", expr)
	}
	date := asBuiltin(t, "date")
	const test = "$value eq current-date()"
	got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, date, ctaExprRecord(test, ""), fcValue(t, date, "0001-01-01Z"))
	if got != value.AssertionHolds {
		t.Errorf("FacetAssertions(zero instant).Evaluate(%q over 0001-01-01Z) = %d, want %d", test, got, value.AssertionHolds)
	}
}

// An instant whose offset no timezoneFrag spells — not a whole number of
// minutes, or beyond ±14:00 — is read in UTC: 2026-10-07T00:30 at +01:00:30 or
// +15:00 is 2026-10-06 in UTC, dated 2026-10-06Z. With ctaCurrentDateItem
// formatting in.now unsettled, the +01:00:30 row renders 2026-10-07+01:00, a
// different date, and the +15:00 row renders an invalid lexical and raises.
// ±14:00 is spelled, keeping its own date and offset: the last two rows fail
// with the bound tested as >= 14*3600.
func TestAssertionCurrentDateUnspellableOffset(t *testing.T) {
	for _, tc := range []struct {
		offset int
		expr   string
	}{
		{3600 + 30, "current-date() eq xs:date('2026-10-06Z')"},
		{15 * 3600, "current-date() eq xs:date('2026-10-06Z')"},
		{14 * 3600, "string(current-date()) = '2026-10-07+14:00'"},
		{-14 * 3600, "string(current-date()) = '2026-10-07-14:00'"},
	} {
		now := time.Date(2026, time.October, 7, 0, 30, 0, 0, time.FixedZone("", tc.offset))
		if !cdEval(t, tc.expr, now) {
			t.Errorf("Evaluate(%q) at %v = false, want true", tc.expr, now)
		}
	}
}

// An assertions facet reads the instant [FacetAssertions] was built with:
// `$value lt current-date()` holds for a past date and fails for a future one,
// and `$value gt current-date()` the reverse. Every row declines with the
// "current-date" arm of ctaParser.libraryCall removed. With
// facetAssertions.Evaluate dropping f.now, the two rows over 2000-01-01 flip,
// the zero instant 0001-01-01 preceding 2000-01-01; the rows over 2080-01-01
// answer as before.
func TestFacetCurrentDate(t *testing.T) {
	date := asBuiltin(t, "date")
	for _, tc := range []struct {
		test, lexical string
		want          value.AssertionOutcome
	}{
		{"$value lt current-date()", "2000-01-01", value.AssertionHolds},
		{"$value lt current-date()", "2080-01-01", value.AssertionFails},
		{"$value gt current-date()", "2080-01-01", value.AssertionHolds},
		{"$value gt current-date()", "2000-01-01", value.AssertionFails},
	} {
		got := FacetAssertions(cdNow).Evaluate(backend(), seededTypes, date, ctaExprRecord(tc.test, ""), fcValue(t, date, tc.lexical))
		if got != tc.want {
			t.Errorf("Evaluate(%q over %s) = %d, want %d", tc.test, tc.lexical, got, tc.want)
		}
	}
}

// A cast inside an assertion validates against its target with the
// assertion's own instant (ctaValidate), so an assertions facet the target
// carries reads the same current dateTime: 2000-01-01 is valid against a
// restriction of xs:date asserting `$value lt current-date()` at cdNow and
// invalid at the zero instant, 0001-01-01. With ctaValidate passing the zero
// [time.Time] in place of in.now, the first answer is raised.
func TestCastValidatesAtTheEvaluationInstant(t *testing.T) {
	facets := []xsd.Facet{xsd.NewAssertionsFacet([]xsd.Assertion{xsd.NewAssertion(asRecord("$value lt current-date()"))})}
	past, err := xsd.NewSimpleType(xsderr.Loc{}, xsd.QName{Space: ctaUserNS, Local: "PastDate"},
		xsd.RestrictionDerivation{}, xsd.SimpleTypeRef{Name: ctaBuiltin("date")}, facets, nil)
	if err != nil {
		t.Fatalf("building PastDate: %v", err)
	}
	types := asTypesWith(past)
	for _, tc := range []struct {
		now  time.Time
		want bool
	}{
		{cdNow, true},
		{time.Time{}, false},
	} {
		env := ctaEnv{backend: backend(), types: types, input: ctaTypedInput{attrs: noTypedAttributes, children: noChildElements, now: tc.now}}
		if _, got := ctaValidated(ctaValidate("2000-01-01", past, env)); got != tc.want {
			t.Errorf("ctaValidate(2000-01-01 against PastDate) at %v validated = %v, want %v", tc.now, got, tc.want)
		}
	}
}

// A Type Alternative's cast validates with ctaAssertionsDeclined: its input
// holds no current dateTime, so an assertions facet the target carries is
// declined, not decided at an instant nothing supplied, and the decline maps to
// ctaRaised (ctaValidate's GAP(xpath)). FutureDate asserts `$value gt
// current-date()`, which 2000-01-01 satisfies at the zero instant, 0001-01-01.
func TestTypeAlternativeCastDeclinesAssertions(t *testing.T) {
	test := asRecord("$value gt current-date()")
	facets := []xsd.Facet{xsd.NewAssertionsFacet([]xsd.Assertion{xsd.NewAssertion(test)})}
	future, err := xsd.NewSimpleType(xsderr.Loc{}, xsd.QName{Space: ctaUserNS, Local: "FutureDate"},
		xsd.RestrictionDerivation{}, xsd.SimpleTypeRef{Name: ctaBuiltin("date")}, facets, nil)
	if err != nil {
		t.Fatalf("building FutureDate: %v", err)
	}
	types := asTypesWith(future)
	env := ctaEnv{backend: backend(), types: types, input: ctaLexicalInput{}}
	if _, validated := ctaValidated(ctaValidate("2000-01-01", future, env)); validated {
		t.Errorf("ctaValidate(2000-01-01 against FutureDate) on a Type Alternative's input validated, want raised")
	}
	date := asBuiltin(t, "date")
	if got := (ctaAssertionsDeclined{}).Evaluate(backend(), types, date, test, fcValue(t, date, "2000-01-01")); got != value.AssertionDeclined {
		t.Errorf("ctaAssertionsDeclined.Evaluate = %d, want AssertionDeclined", got)
	}
}

// fn:current-date takes no argument: one is the wrong arity (err:XPST0017),
// declined. A Type Alternative's {test} declines the call outright, its
// grammar admitting fn:not and constructors alone (§3.12.6 clause 3,
// ta-props-correct 2; TestCompileCTATestDeclinesLibraryFunctions). With
// currentDateCall admitting any arity, the row compiles.
func TestCurrentDateArityDeclines(t *testing.T) {
	if _, ok := afCompile(t, "current-date(1) = current-date()", xsd.EmptyContent{}); ok {
		t.Error("CompileAssertionTest(current-date(1) = current-date()): compiled, want declined")
	}
}
