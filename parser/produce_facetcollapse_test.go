package parser_test

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xpath"
	"github.com/kud360/goxsd8/xsd"
)

// facetCollapseCase is one restriction whose type T is produced, whose own
// facet of kind carries {value} want, and under which lexical is then validated
// strictly.
type facetCollapseCase struct {
	name    string
	body    string
	kind    xsd.FacetKind
	want    string
	lexical string
}

// runFacetCollapseCases produces each case's schema, asserts T's own facet of
// the case's kind holds exactly the wanted {value}, and validates the case's
// lexical against T under strict ValidateLexical.
func runFacetCollapseCases(t *testing.T, cases []facetCollapseCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st, sch := simpleTypeOf(t, "T", tc.body)
			if _, err := value.ValidateLexical(strict.New(), sch, st, tc.lexical, nil, xpath.FacetAssertions(time.Time{})); err != nil {
				t.Errorf("ValidateLexical(%q) = %v, want accept", tc.lexical, err)
			}
			var got []string
			for _, f := range st.OwnFacets() {
				if f.Kind() == tc.kind {
					got = f.Values()
				}
			}
			if len(got) != 1 || got[0] != tc.want {
				t.Errorf("own %v {value} = %q, want [%q]", tc.kind, got, tc.want)
			}
		})
	}
}

// restrictT is a named simple type T restricting base with facets.
func restrictT(base, facets string) string {
	return `<xs:simpleType name="T"><xs:restriction base="` + base + `">` + facets + `</xs:restriction></xs:simpleType>`
}

// restrictBaseThenT is a named B restricting base with baseFacets, and T
// restricting B with facets, so T's facet is read against B's same-kind facet
// at finalize.
func restrictBaseThenT(base, baseFacets, facets string) string {
	return `<xs:simpleType name="B"><xs:restriction base="` + base + `">` + baseFacets + `</xs:restriction></xs:simpleType>` +
		`<xs:simpleType name="T"><xs:restriction base="B">` + facets + `</xs:restriction></xs:simpleType>`
}

// TestProduceFacetValueCollapsed pins that a facet {value} is "the actual value
// of the value [attribute]" (f-l-value, f-mil-value, f-mal-value, f-w-value,
// f-td-value, f-fd-value, f-tz-value; xsd-precisionDecimal.md f-ms-value,
// f-mns-value), read after the whiteSpace = collapse the attribute's type fixes
// (Structures key-vv, key-nv): the padding is gone from the produced {value},
// so neither finalize's countValue/scaleValue nor value's facet readers refuse
// it (#2481).
func TestProduceFacetValueCollapsed(t *testing.T) {
	runFacetCollapseCases(t, []facetCollapseCase{
		{"totalDigits", restrictT("xs:decimal", `<xs:totalDigits value=" 5 "/>`), xsd.FacetTotalDigits, "5", "12345"},
		{"maxScale", restrictT("xs:precisionDecimal", `<xs:maxScale value=" 5 "/>`), xsd.FacetMaxScale, "5", "1.5"},
		{"explicitTimezone", restrictT("xs:dateTime", `<xs:explicitTimezone value=" required "/>`), xsd.FacetExplicitTimezone, "required", "2000-01-01T00:00:00Z"},
		{"length", restrictT("xs:string", `<xs:length value=" 3 "/>`), xsd.FacetLength, "3", "abc"},
		{"minLength", restrictT("xs:string", `<xs:minLength value=" 1 "/>`), xsd.FacetMinLength, "1", "a"},
		{"maxLength tab and newline", restrictT("xs:string", `<xs:maxLength value="&#9;5&#10;"/>`), xsd.FacetMaxLength, "5", "abc"},
		{"fractionDigits", restrictT("xs:decimal", `<xs:fractionDigits value=" 2 "/>`), xsd.FacetFractionDigits, "2", "1.25"},
		{"minScale", restrictT("xs:precisionDecimal", `<xs:minScale value=" 1 "/>`), xsd.FacetMinScale, "1", "1.5"},
		{"whiteSpace", restrictT("xs:string", `<xs:whiteSpace value=" collapse "/>`), xsd.FacetWhiteSpace, "collapse", "a  b"},
		{"explicitTimezone tab and newline", restrictT("xs:dateTime", `<xs:explicitTimezone value="&#9;optional&#10;"/>`), xsd.FacetExplicitTimezone, "optional", "2000-01-01T00:00:00"},
		// The same-step pair is read by xsd's count-order check at finalize.
		{"minLength and maxLength same step", restrictT("xs:string", `<xs:minLength value=" 1 "/><xs:maxLength value=" 5 "/>`), xsd.FacetMaxLength, "5", "abc"},
		// The base carries the same kind, so finalize reads both through countValue
		// (length, maxLength, totalDigits) or scaleValue (maxScale, minScale).
		{"length over base length", restrictBaseThenT("xs:string", `<xs:length value="3"/>`, `<xs:length value=" 3 "/>`), xsd.FacetLength, "3", "abc"},
		{"maxLength over base maxLength", restrictBaseThenT("xs:string", `<xs:maxLength value="5"/>`, `<xs:maxLength value=" 3 "/>`), xsd.FacetMaxLength, "3", "abc"},
		{"totalDigits over base totalDigits", restrictBaseThenT("xs:decimal", `<xs:totalDigits value="5"/>`, `<xs:totalDigits value="&#9;3&#10;"/>`), xsd.FacetTotalDigits, "3", "123"},
		{"maxScale over base maxScale", restrictBaseThenT("xs:precisionDecimal", `<xs:maxScale value="5"/>`, `<xs:maxScale value=" 3 "/>`), xsd.FacetMaxScale, "3", "1.5"},
		{"minScale over base minScale", restrictBaseThenT("xs:precisionDecimal", `<xs:minScale value="1"/>`, `<xs:minScale value=" 2 "/>`), xsd.FacetMinScale, "2", "1.50"},
	})
}

// TestProduceFacetValueKeptRaw pins the facets whose value attribute is NOT
// collapsed by the parser: <pattern>'s is an xs:string (whiteSpace = preserve),
// and the four bounds' and <enumeration>'s are xs:anySimpleType, read later
// through the base type's own lexical mapping. Each {value} keeps its padding,
// and the xs:string rows show the padding is meaningful: " a " itself validates.
func TestProduceFacetValueKeptRaw(t *testing.T) {
	runFacetCollapseCases(t, []facetCollapseCase{
		{"pattern", restrictT("xs:string", `<xs:pattern value=" a "/>`), xsd.FacetPattern, " a ", " a "},
		{"enumeration over xs:string", restrictT("xs:string", `<xs:enumeration value=" a "/>`), xsd.FacetEnumeration, " a ", " a "},
		{"enumeration over xs:token", restrictT("xs:token", `<xs:enumeration value=" a "/>`), xsd.FacetEnumeration, " a ", "a"},
		{"maxInclusive", restrictT("xs:integer", `<xs:maxInclusive value=" 5 "/>`), xsd.FacetMaxInclusive, " 5 ", "5"},
		{"maxExclusive", restrictT("xs:integer", `<xs:maxExclusive value=" 5 "/>`), xsd.FacetMaxExclusive, " 5 ", "4"},
		{"minInclusive", restrictT("xs:integer", `<xs:minInclusive value=" 5 "/>`), xsd.FacetMinInclusive, " 5 ", "5"},
		{"minExclusive", restrictT("xs:integer", `<xs:minExclusive value=" 5 "/>`), xsd.FacetMinExclusive, " 5 ", "6"},
	})
}
