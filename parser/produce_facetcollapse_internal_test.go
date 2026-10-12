package parser

import (
	"testing"

	"github.com/kud360/goxsd8/xsd"
)

// TestFacetValueLexicalCollapse pins the parts of §4.3.6's collapse no produced
// schema can show, because no padded literal of a collapsing kind is valid with
// interior space: every interior run of XML S becomes one #x20, and a Unicode
// space outside XML S (U+00A0) is kept, at either end or inside.
func TestFacetValueLexicalCollapse(t *testing.T) {
	cases := []struct {
		kind    xsd.FacetKind
		lexical string
		want    string
	}{
		{xsd.FacetWhiteSpace, "\t col \n\r lapse ", "col lapse"},
		{xsd.FacetLength, " 5  ", " 5  "},
		{xsd.FacetMinScale, " 1  2 ", "1  2"},
		{xsd.FacetPattern, "\t a  b ", "\t a  b "},
	}
	for _, tc := range cases {
		if got := facetValueLexical(tc.kind, tc.lexical); got != tc.want {
			t.Errorf("facetValueLexical(%v, %q) = %q, want %q", tc.kind, tc.lexical, got, tc.want)
		}
	}
}
