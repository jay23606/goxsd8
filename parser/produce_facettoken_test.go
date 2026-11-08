package parser_test

import (
	"strings"
	"testing"

	"github.com/kud360/goxsd8/xsderr"
)

// TestProduceFacetTokenValueRejected pins that a <whiteSpace> value outside
// preserve/replace/collapse and an <explicitTimezone> value outside
// optional/required/prohibited — the enumerations the schema for schema
// documents declares for each (xmlschema11-2.md:4136, :4189) — are charged
// cvc-datatype-valid at the value attribute, from Produce alone (#3495). The
// message is pinned from its opening subject, so a swapped argument cannot pass.
func TestProduceFacetTokenValueRejected(t *testing.T) {
	cases := []struct {
		name    string
		base    string
		facet   string
		wantMsg string
	}{
		{"whiteSpace bogus", "string", `<xs:whiteSpace value="bogus"/>`, `<whiteSpace> value "bogus" is not one of preserve, replace, collapse`},
		{"explicitTimezone something", "dateTime", `<xs:explicitTimezone value="something"/>`, `<explicitTimezone> value "something" is not one of optional, required, prohibited`},
		{"whiteSpace empty", "string", `<xs:whiteSpace value=""/>`, `<whiteSpace> value "" is not one of preserve, replace, collapse`},
		{"whiteSpace case", "string", `<xs:whiteSpace value="Collapse"/>`, `<whiteSpace> value "Collapse" is not one of preserve, replace, collapse`},
		{"whiteSpace interior space", "string", `<xs:whiteSpace value="col lapse"/>`, `<whiteSpace> value "col lapse" is not one of preserve, replace, collapse`},
		{"explicitTimezone case", "dateTime", `<xs:explicitTimezone value="Required"/>`, `<explicitTimezone> value "Required" is not one of optional, required, prohibited`},
		// U+00A0 is not XML whitespace, so collapse keeps it and the token is not
		// in the enumeration.
		{"explicitTimezone non-XML-whitespace padding", "dateTime", "<xs:explicitTimezone value=\"\u00a0required\"/>", `<explicitTimezone> value "\u00a0required" is not one of optional, required, prohibited`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := produce(t, facetValueSchema(tc.base, tc.facet))
			assertRule(t, err, "cvc-datatype-valid")
			if !strings.Contains(err.Error(), "] "+tc.wantMsg) {
				t.Fatalf("diagnostic %q, want it to open with %q", err, tc.wantMsg)
			}
			loc, ok := xsderr.LocOf(err)
			if !ok {
				t.Fatalf("error %v carries no position (STYLE E3)", err)
			}
			col := strings.Index(tc.facet, "value=") + 1
			if loc.URI != produceURI || loc.Line != 3 || loc.Col != col {
				t.Fatalf("position = %s:%d:%d, want the value attribute at %s:3:%d",
					loc.URI, loc.Line, loc.Col, produceURI, col)
			}
		})
	}
}

// TestProduceFacetTokenValueAdmitted pins each of the six in-domain tokens and
// the XML-whitespace-padded forms, which xs:NMTOKEN's whiteSpace = collapse
// normalizes before the enumeration is tested (§4.1.4).
func TestProduceFacetTokenValueAdmitted(t *testing.T) {
	cases := []struct {
		name  string
		base  string
		facet string
	}{
		{"whiteSpace preserve", "string", `<xs:whiteSpace value="preserve"/>`},
		{"whiteSpace replace", "string", `<xs:whiteSpace value="replace"/>`},
		{"whiteSpace collapse", "string", `<xs:whiteSpace value="collapse"/>`},
		{"whiteSpace padded", "string", `<xs:whiteSpace value=" collapse "/>`},
		{"explicitTimezone optional", "dateTime", `<xs:explicitTimezone value="optional"/>`},
		{"explicitTimezone required", "dateTime", `<xs:explicitTimezone value="required"/>`},
		{"explicitTimezone prohibited", "dateTime", `<xs:explicitTimezone value="prohibited"/>`},
		{"explicitTimezone padded", "dateTime", `<xs:explicitTimezone value=" required "/>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := produce(t, facetValueSchema(tc.base, tc.facet)); err != nil {
				t.Fatalf("Produce: %v", err)
			}
		})
	}
}
