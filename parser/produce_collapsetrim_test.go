package parser_test

import (
	"strings"
	"testing"

	"github.com/kud360/goxsd8/xsderr"
)

// TestProduceCollapseTrimRejectsNonXMLWhitespacePadding pins the character class
// every keyword and numeric attribute comparison in the producer trims: exactly
// #x9/#xA/#xD/#x20, the four characters §4.3.6's whiteSpace facet names, and no
// others.
//
// The facet is fixed to collapse for the datatypes these attributes are declared
// with and is applied before lexical-space membership is tested (§4.1.4), so the
// padding a conforming processor removes is exactly those four. Go's
// strings.TrimSpace cuts unicode.IsSpace instead — U+0085, U+00A0, U+2028 and
// more — which collapse PRESERVES, so every row below was silently ACCEPTED
// before parser.collapseTrim replaced it: maxOccurs="&#xA0;unbounded" read as
// unbounded, mode="&#xA0;none" as none, processContents="&#xA0;strict" as
// strict.
//
// The direction is what makes this table the only thing that can catch the
// defect: it is a false-ACCEPT, so no conformance fixture, vet pass or existing
// rejection test sees it. Each row therefore uses a padding character §4.3.6
// preserves — U+00A0, U+2028 or U+3000 — and asserts a REJECTION; reverting
// collapseTrim to strings.TrimSpace makes every one of them fail.
func TestProduceCollapseTrimRejectsNonXMLWhitespacePadding(t *testing.T) {
	// A slice, not a map: subtest order is output (STYLE D2). name is the producer
	// function the row pins. wantRule is empty for a plain grammar fault, which
	// carries no rule ID at all.
	cases := []struct {
		name     string
		body     string
		wantRule xsderr.Rule
		wantMsg  string
	}{
		{
			// checkDefaultOpenContent: the eager once-per-document
			// <defaultOpenContent> grammar check, whose interleave|suffix
			// enumeration carries no Schema Component Constraint of its own.
			name:    "checkDefaultOpenContent mode padded with U+00A0",
			body:    `<xs:defaultOpenContent mode="&#xA0;interleave"><xs:any/></xs:defaultOpenContent>`,
			wantMsg: "has mode=",
		},
		{
			// openContentModeIsNone, read first by checkOpenContentAny: a mode that
			// is not "none" needs an <any> child (src-ct clause 3). Padded, the
			// token is not "none", so the missing <any> is a fault — where
			// TrimSpace made it clause 6.1's silently discarded ·wildcard element·.
			name:     "openContentModeIsNone mode padded with U+3000",
			body:     `<xs:complexType name="T"><xs:openContent mode="&#x3000;none"/><xs:sequence/></xs:complexType>`,
			wantRule: "src-ct",
			wantMsg:  "clause 3",
		},
		{
			// openContentModeOf: the {mode} mapping of §3.4.2.3.3 clause 6.2,
			// charged ct-props-correct clause 1 out of enumeration.
			name:     "openContentModeOf mode padded with U+2028",
			body:     `<xs:complexType name="T"><xs:openContent mode="&#x2028;interleave"><xs:any/></xs:openContent><xs:sequence/></xs:complexType>`,
			wantRule: "ct-props-correct",
			wantMsg:  "open content mode",
		},
		{
			// occursOf: the maxOccurs="unbounded" keyword arm. Padded, it is not
			// the keyword, so it falls to nonNegativeNumeral and fails xs:allNNI's
			// numeric member instead of being read as ·unbounded·.
			name:     "occursOf maxOccurs unbounded padded with U+00A0",
			body:     `<xs:complexType name="T"><xs:sequence maxOccurs="&#xA0;unbounded"><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>`,
			wantRule: "cvc-datatype-valid",
			wantMsg:  "maxOccurs value",
		},
		{
			// nonNegativeNumeral: the nonNegativeIntegerLexical argument. The
			// digit scan rejects the padding itself, so the whole numeric family
			// depends on this one trim.
			name:     "nonNegativeNumeral minOccurs padded with U+00A0",
			body:     `<xs:complexType name="T"><xs:sequence minOccurs="&#xA0;1"><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>`,
			wantRule: "cvc-datatype-valid",
			wantMsg:  "minOccurs value",
		},
		{
			// allOccursGrammar: <all>'s own "unbounded" arm. Both readings reject,
			// so the rule ID alone cannot see the difference — the message is what
			// separates them. Padded, the lexical never reaches the {0,1}
			// enumeration verdict; it fails xs:nonNegativeInteger first.
			name:     "allOccursGrammar <all> maxOccurs unbounded padded with U+00A0",
			body:     `<xs:complexType name="T"><xs:all maxOccurs="&#xA0;unbounded"><xs:element name="a" type="xs:string"/></xs:all></xs:complexType>`,
			wantRule: "cvc-datatype-valid",
			wantMsg:  "is not a nonNegativeInteger",
		},
		{
			// processContentsOf: the skip/lax/strict enumeration of Appendix A's
			// wildcard attribute group.
			name:     "processContentsOf padded with U+00A0",
			body:     `<xs:complexType name="T"><xs:sequence><xs:any processContents="&#xA0;strict"/></xs:sequence></xs:complexType>`,
			wantRule: "cvc-datatype-valid",
			wantMsg:  "processContents",
		},
		{
			// minOccursZero: §3.4.2.3.3 clause 2.1.3's empty-<choice> elision.
			// Padded, the attribute is not "0", so the particle is built and
			// occursOf rejects the lexical — where TrimSpace elided the whole
			// model group and produced an empty content type without a murmur.
			name:     "minOccursZero <choice> minOccurs padded with U+00A0",
			body:     `<xs:complexType name="T"><xs:choice minOccurs="&#xA0;0"/></xs:complexType>`,
			wantRule: "cvc-datatype-valid",
			wantMsg:  "minOccurs value",
		},
		{
			// maxOccursZero: clause 2.1.4's maxOccurs="0" elision, the same shape
			// one clause over.
			name:     "maxOccursZero <sequence> maxOccurs padded with U+00A0",
			body:     `<xs:complexType name="T"><xs:sequence maxOccurs="&#xA0;0"><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>`,
			wantRule: "cvc-datatype-valid",
			wantMsg:  "maxOccurs value",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := produce(t, wrap("urn:a", tc.body))
			if err == nil {
				t.Fatalf("Produce accepted %s — the padding character is not §4.3.6 whitespace, so the token is out of its declared type", tc.body)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("error = %v, want it to name %q", err, tc.wantMsg)
			}
			got, ok := xsderr.RuleOf(err)
			if tc.wantRule == "" {
				if ok {
					t.Fatalf("error = %v, charged %s; <defaultOpenContent> has no Schema Component Constraint of its own, so this is a plain grammar fault", err, got)
				}
				if !strings.HasPrefix(err.Error(), "parser: <defaultOpenContent> at ") {
					t.Fatalf("error = %q, want it to open with the offending element and its position", err)
				}
				return
			}
			if !ok {
				t.Fatalf("error %v carries no xsderr rule, want %s", err, tc.wantRule)
			}
			if got != tc.wantRule {
				t.Fatalf("error charged %s, want %s (%v)", got, tc.wantRule, err)
			}
			loc, ok := xsderr.LocOf(err)
			if !ok || loc.URI != produceURI || loc.Col == 0 {
				t.Fatalf("position = %+v, want the charge positioned in %s (STYLE E3)", loc, produceURI)
			}
		})
	}
}

// TestParseRedefineSelfReferenceOccursPaddedUnbounded is the tenth collapse-trim
// site, the one outside produce_complex.go: checkSelfReferenceOccurs reads a
// redefining <group>'s self-reference for src-redefine clause 6.1.2, "the
// ·actual value· of both that group's minOccurs and maxOccurs [attribute] is 1
// (or ·absent·)".
//
// Both readings reject, so this pins the RULE rather than the fact of an error.
// maxOccurs="&#xA0;unbounded" is not the ·unbounded· keyword — U+00A0 survives
// collapse — so it is not a member of xs:allNNI at all, and the lexical fault
// against its declared type is charged before clause 6.1.2 has an actual value to
// compare. strings.TrimSpace read the keyword and charged src-redefine instead.
func TestParseRedefineSelfReferenceOccursPaddedUnbounded(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:group name="g"><xs:sequence>`+
			`<xs:group ref="tns:g" maxOccurs="&#xA0;unbounded"/>`+
			`</xs:sequence></xs:group>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a", `<xs:group name="g"><xs:sequence>`+
			`<xs:element name="a" type="xs:string"/></xs:sequence></xs:group>`),
	})
	mustRule(t, err, "cvc-datatype-valid", "maxOccurs value", "is not a nonNegativeInteger")
}
