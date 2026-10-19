package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// An assertions facet's fn:year-from-date over `$value` is evaluated
// (xpath-functions.md §10.5.14; cvc-assertions-valid clause 1.4): the year of
// the ·localized value·, never normalized to UTC, so `2008-12-31-14:00` and
// `2008-01-01+14:00` are both 2008, and a negative year is negative. The
// empty sequence is returned for an empty argument, which `empty(…)` reads as
// a decided true and `… eq 2008` as a decided false. An xs:string argument is
// err:XPTY0004 (xpath20.md §3.1.5), which FAILS the facet — under fn:not too,
// where a false comparison would hold — and is never declined. Every other
// arity declines (err:XPST0017).
func TestFacetYearFromDate(t *testing.T) {
	date := asBuiltin(t, "date")
	for _, tc := range []struct {
		test, lexical string
		want          value.AssertionOutcome
	}{
		{"year-from-date($value) eq 2008", "2008-07-28", value.AssertionHolds},
		{"year-from-date($value) eq 2008", "2018-07-28", value.AssertionFails},
		{"year-from-date($value) eq 2008", "2008-12-31-14:00", value.AssertionHolds},
		{"year-from-date($value) eq 2008", "2008-01-01+14:00", value.AssertionHolds},
		{"year-from-date($value) lt 0", "-0044-03-15", value.AssertionHolds},
		{"year-from-date($value) instance of xs:integer", "2008-07-28", value.AssertionHolds},
		{"empty(year-from-date(()))", "2008-07-28", value.AssertionHolds},
		{"year-from-date(()) eq 2008", "2008-07-28", value.AssertionFails},
		{"year-from-date('2008-01-01') eq 2008", "2008-07-28", value.AssertionFails},
		{"not(year-from-date('2008-01-01') eq 2008)", "2008-07-28", value.AssertionFails},
		{"year-from-date() eq 2008", "2008-07-28", value.AssertionDeclined},
		{"year-from-date($value, $value) eq 2008", "2008-07-28", value.AssertionDeclined},
	} {
		got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, date, ctaExprRecord(tc.test, "", "xs", xsd.XMLSchemaNS), fcValue(t, date, tc.lexical))
		if got != tc.want {
			t.Errorf("Evaluate(%q, %q) = %d, want %d", tc.test, tc.lexical, got, tc.want)
		}
	}
}

// On the assertion façade an xs:untypedAtomic argument is cast to xs:date
// (xpath20.md §3.1.5) and an xs:string one is err:XPTY0004: an
// xs:anySimpleType attribute decides both ways, one that does not cast raises
// err:FORG0001, which fn:not does not invert, and so does an xs:string one
// whose string is a date.
func TestAssertionYearFromDateArgument(t *testing.T) {
	for _, tc := range []struct {
		expr  string
		attrs []asTyped
		want  bool
	}{
		{"year-from-date(@w) eq 2008", []asTyped{{uq("w"), "anySimpleType", "2008-03-01"}}, true},
		{"year-from-date(@w) eq 2008", []asTyped{{uq("w"), "anySimpleType", "2009-03-01"}}, false},
		{"not(year-from-date(@w) eq 2008)", []asTyped{{uq("w"), "anySimpleType", "2009-03-01"}}, true},
		{"not(year-from-date(@w) eq 2008)", []asTyped{{uq("w"), "anySimpleType", "x"}}, false},
		{"not(year-from-date(@s) eq 2008)", []asTyped{{uq("s"), "string", "2008-03-01"}}, false},
		{"empty(year-from-date(@w))", nil, true},
	} {
		if got := afEval(t, tc.expr, tc.attrs); got != tc.want {
			t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.attrs, got, tc.want)
		}
	}
}
