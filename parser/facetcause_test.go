package parser_test

import (
	"testing"
)

// TestFacetValueOutsideLexicalSpaceRendersItsCause pins the whole rejection
// line of a bound or enumeration facet whose {value} is outside its base
// type's lexical space: the outer rule's "[rule] " is followed directly by the
// wrapped verdict's own "[rule] msg", with no "?: " between them for the
// verdict's unknown position (#3542).
func TestFacetValueOutsideLexicalSpaceRendersItsCause(t *testing.T) {
	for _, tc := range []struct {
		facet, want string
	}{
		{"maxInclusive", `main.xsd:1:56: [maxInclusive-valid-restriction] [cvc-datatype-valid] "abc" is not in the lexical space of decimal, which cvc-datatype-valid clause 2.1 requires it to be in`},
		{"minExclusive", `main.xsd:1:56: [minExclusive-valid-restriction] [cvc-datatype-valid] "abc" is not in the lexical space of decimal, which cvc-datatype-valid clause 2.1 requires it to be in`},
		{"enumeration", `main.xsd:1:56: [enumeration-valid-restriction] [cvc-pattern-valid] "abc" matches no member of the pattern facet of the simple type {http://www.w3.org/2001/XMLSchema}integer, whose {value} holds "[\-+]?[0-9]+", but cvc-pattern-valid requires one to match`},
	} {
		_, err := parseMap(t, "main.xsd", map[string]string{"main.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"><xs:simpleType name="t"><xs:restriction base="xs:integer"><xs:` + tc.facet + ` value="abc"/></xs:restriction></xs:simpleType></xs:schema>`})
		if err == nil {
			t.Errorf("%s: parse = nil, want the facet's {value} rejected", tc.facet)
			continue
		}
		if got := err.Error(); got != tc.want {
			t.Errorf("%s: err = %q, want %q", tc.facet, got, tc.want)
		}
	}
}
