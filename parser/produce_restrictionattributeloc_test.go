package parser_test

import (
	"errors"
	"testing"

	"github.com/kud360/goxsd8/xsderr"
)

// restrictionDoc builds a two-type document — base B carrying baseAttr, and
// restriction R re-declaring the same attribute as restrAttr — over
// derivationDoc's layout (produce_complexderivationloc_test.go), so
// <complexType name="B"> opens on line 2 and <complexType name="R"> on
// derivationDocLine. The two lines differ so an assertion on the reported line
// distinguishes the spec's T from its B.
func restrictionDoc(baseAttr, restrAttr string) string {
	return derivationDoc(
		`<xs:complexType name="B">`+baseAttr+`</xs:complexType>`, "",
		`<xs:complexType name="R"><xs:complexContent><xs:restriction base="tns:B">`+
			restrAttr+`</xs:restriction></xs:complexContent></xs:complexType>`)
}

// TestRestrictionAttributeRejectionsCiteTheRestrictingType pins the POSITION of
// the derivation-ok-restriction clause 3 (c-ran) rejections loc-testSubP raises
// over a pair of Attribute Uses, end to end through Parse: each is charged to
// the restricting complex type — the spec's T, whose own §3.4.6.3 obligation
// clause 3 is — and not to the zero Loc, which renders as "?" and tells a schema
// author nothing (#359).
//
// The four sub-clauses below are every one of loc-testSubP clause 5's rejection
// sites reachable from a parsed document. checkBindingSubsumes' two remaining
// sites in defaultbinding.go — its mismatched-kind fallthrough and its route to
// checkKeywordSubsumes' keyword charge (clauses 2 and 3) — cannot be driven
// from any document, because checkAttributeRestriction always passes it an
// attributeUseBinding as the specific binding; xsd's own
// TestBindingSubsumesChargesTheRestrictingType covers those two directly. The
// keyword charge's other route, through checkAttributeRestrictionWildcard, is
// TestRestrictionAttributeWildcardKeyword's.
//
// Asserting the exact line, rather than merely a non-zero Loc, is what makes the
// test fail if the sites are charged b.Loc() instead of t.Loc().
func TestRestrictionAttributeRejectionsCiteTheRestrictingType(t *testing.T) {
	for _, tc := range []struct {
		clause    string
		baseAttr  string
		restrAttr string
	}{
		{"5.1: the restriction's type is not validly derived from the base's",
			`<xs:attribute name="a" type="xs:string"/>`,
			`<xs:attribute name="a" type="xs:int"/>`},
		{"5.2: the restriction drops the base's fixed value",
			`<xs:attribute name="a" type="xs:string" fixed="v"/>`,
			`<xs:attribute name="a" type="xs:string"/>`},
		{"5.2.2: the restriction fixes a different value",
			`<xs:attribute name="a" type="xs:string" fixed="v"/>`,
			`<xs:attribute name="a" type="xs:string" fixed="w"/>`},
		{"5.3: {inheritable} differs",
			`<xs:attribute name="a" type="xs:string" inheritable="true"/>`,
			`<xs:attribute name="a" type="xs:string"/>`},
	} {
		t.Run(tc.clause, func(t *testing.T) {
			_, err := parseMap(t, "main.xsd", map[string]string{
				"main.xsd": restrictionDoc(tc.baseAttr, tc.restrAttr),
			})
			var xe *xsderr.Error
			if !errors.As(err, &xe) {
				t.Fatalf("Parse error = %v, want an *xsderr.Error", err)
			}
			if xe.Rule != "derivation-ok-restriction" {
				t.Fatalf("rule = %q, want derivation-ok-restriction (%v)", xe.Rule, err)
			}
			if xe.Loc == (xsderr.Loc{}) {
				t.Fatalf("loc is the zero Loc, rendering as %q: %v", xe.Loc, err)
			}
			if xe.Loc.URI != "main.xsd" || xe.Loc.Line != derivationDocLine {
				t.Fatalf("loc = %s, want main.xsd:%d — the restricting <complexType name=\"R\">", xe.Loc, derivationDocLine)
			}
		})
	}
}
