package parser_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/xsderr"
)

// TestRestrictionAttributeWildcardKeyword pins derivation-ok-restriction clause
// 3 (c-ran) over two {attribute wildcard}s' {process contents}, end to end
// through Parse: past the cos-ns-subset gate, the base's keyword must
// ·subsume· the restriction's (loc-testSubP clauses 1-3), charged by
// checkAttributeRestrictionWildcard to the restricting type R.
//
// want is the message text from the rule onward, naming T, then B, then G,
// then S, then the clause, so a message that swaps either pair fails; an empty
// want is a row the clause accepts. The namespace="" rows bind nothing (an
// empty enumeration has no item ·attributed· to it), and neither does the row
// whose ·complete wildcard· is the empty intersection of an <attributeGroup
// ref>'s urn:p with R's own urn:q. The two xs:anyType rows assert the GAP(xsd)
// in checkRestrictionAttributes (complexderivation.go), a deliberate divergence
// from derivation-ok-restriction clause 3 for the conformance suite's sake, not
// a spec reading: loc-testSubP clause 2 refuses anyType's lax wildcard over a
// skip one, and the RULING at #2548 issuecomment-6134245663 accepts the pairing
// anyway.
func TestRestrictionAttributeWildcardKeyword(t *testing.T) {
	restriction := func(children string) string {
		return `<xs:complexType name="R"><xs:complexContent><xs:restriction base="tns:B">` +
			children + `</xs:restriction></xs:complexContent></xs:complexType>`
	}
	const subject = "[derivation-ok-restriction] complex type {urn:x}R restricts {urn:x}B, but {urn:x}B binds an attribute both {attribute wildcard}s admit and no declaration governs to a "
	for _, tc := range []struct {
		name  string
		base  string // B's attribute children
		group string // the <attributeGroup> on line 3, or ""
		under string // the type under test, on derivationDocLine
		want  string
	}{
		{"strict by lax", `<xs:anyAttribute processContents="strict"/>`, "",
			restriction(`<xs:anyAttribute processContents="lax"/>`),
			subject + "strict wildcard while the restriction binds it to a lax wildcard, and loc-testSubP clause 3 "},
		{"strict by skip", `<xs:anyAttribute processContents="strict"/>`, "",
			restriction(`<xs:anyAttribute processContents="skip"/>`),
			subject + "strict wildcard while the restriction binds it to a skip wildcard, and loc-testSubP clause 3 "},
		{"lax by skip", `<xs:anyAttribute processContents="lax"/>`, "",
			restriction(`<xs:anyAttribute processContents="skip"/>`),
			subject + "lax wildcard while the restriction binds it to a skip wildcard, and loc-testSubP clause 2 "},
		{"implicit strict by lax", `<xs:anyAttribute/>`, "",
			restriction(`<xs:anyAttribute processContents="lax"/>`),
			subject + "strict wildcard while the restriction binds it to a lax wildcard, and loc-testSubP clause 3 "},
		{"strict by a lax wildcard through attributeGroup ref", `<xs:anyAttribute/>`,
			`<xs:attributeGroup name="G"><xs:anyAttribute processContents="lax"/></xs:attributeGroup>`,
			restriction(`<xs:attributeGroup ref="tns:G"/>`),
			subject + "strict wildcard while the restriction binds it to a lax wildcard, and loc-testSubP clause 3 "},

		{"strict by strict", `<xs:anyAttribute processContents="strict"/>`, "",
			restriction(`<xs:anyAttribute processContents="strict"/>`), ""},
		{"lax by lax", `<xs:anyAttribute processContents="lax"/>`, "",
			restriction(`<xs:anyAttribute processContents="lax"/>`), ""},
		{"lax by strict", `<xs:anyAttribute processContents="lax"/>`, "",
			restriction(`<xs:anyAttribute processContents="strict"/>`), ""},
		{"skip by skip", `<xs:anyAttribute processContents="skip"/>`, "",
			restriction(`<xs:anyAttribute processContents="skip"/>`), ""},
		{"skip by lax", `<xs:anyAttribute processContents="skip"/>`, "",
			restriction(`<xs:anyAttribute processContents="lax"/>`), ""},
		{"skip by strict", `<xs:anyAttribute processContents="skip"/>`, "",
			restriction(`<xs:anyAttribute processContents="strict"/>`), ""},
		{"strict by an empty lax wildcard", `<xs:anyAttribute processContents="strict"/>`, "",
			restriction(`<xs:anyAttribute namespace="" processContents="lax"/>`), ""},
		{"strict by an empty skip wildcard", `<xs:anyAttribute processContents="strict"/>`, "",
			restriction(`<xs:anyAttribute namespace="" processContents="skip"/>`), ""},
		{"strict by an empty intersection of attributeGroup ref and own wildcards", `<xs:anyAttribute/>`,
			`<xs:attributeGroup name="G"><xs:anyAttribute namespace="urn:p" processContents="lax"/></xs:attributeGroup>`,
			restriction(`<xs:attributeGroup ref="tns:G"/><xs:anyAttribute namespace="urn:q" processContents="skip"/>`), ""},
		{"xs:anyType implicitly by skip", "", "",
			`<xs:complexType name="R"><xs:anyAttribute processContents="skip"/></xs:complexType>`, ""},
		{"xs:anyType explicitly by skip", "", "",
			`<xs:complexType name="R"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute processContents="skip"/></xs:restriction></xs:complexContent></xs:complexType>`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseMap(t, "main.xsd", map[string]string{
				"main.xsd": derivationDoc(`<xs:complexType name="B">`+tc.base+`</xs:complexType>`, tc.group, tc.under),
			})
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Parse rejected a restriction c-ran clause 3 accepts: %v", err)
				}
				return
			}
			var xe *xsderr.Error
			if !errors.As(err, &xe) {
				t.Fatalf("Parse error = %v, want an *xsderr.Error", err)
			}
			if xe.Rule != "derivation-ok-restriction" {
				t.Fatalf("rule = %q, want derivation-ok-restriction (%v)", xe.Rule, err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.want)
			}
			if xe.Loc.URI != "main.xsd" || xe.Loc.Line != derivationDocLine {
				t.Fatalf("loc = %s, want main.xsd:%d — the restricting <complexType name=\"R\">", xe.Loc, derivationDocLine)
			}
		})
	}
}
