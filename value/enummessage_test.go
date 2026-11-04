package value_test

import (
	"errors"
	"testing"

	"github.com/kud360/goxsd8/builtin"
	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// TestEnumerationRejectionNamesItsType pins what a cvc-enumeration-valid
// rejection (§4.3.5.4) says: the enumeration facet and the type that declares
// it, with no inheritance when the facet is the checked type's own (Color) and
// the checked type's inheritance when it is not (Shade, which declares only a
// maxLength) (#3335).
func TestEnumerationRejectionNamesItsType(t *testing.T) {
	b := strict.New()
	seeded, err := builtin.Seed(b)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	var str *xsd.SimpleType
	for _, st := range seeded {
		if st.Name().Local == "string" {
			str = st
		}
	}
	derive := func(local string, base *xsd.SimpleType, facet xsd.Facet) *xsd.SimpleType {
		t.Helper()
		st, err := xsd.NewSimpleType(xsderr.Loc{}, xsd.QName{Space: "urn:t", Local: local}, xsd.RestrictionDerivation{},
			xsd.OwnedSimpleType{Definition: base}, []xsd.Facet{facet}, nil)
		if err != nil {
			t.Fatalf("NewSimpleType(%s): %v", local, err)
		}
		if err := st.CheckDerivation(ownedChain{}); err != nil {
			t.Fatalf("CheckDerivation(%s): %v", local, err)
		}
		return st
	}
	color := derive("Color", str, xsd.NewEnumerationFacet([]xsd.EnumerationMember{
		xsd.NewEnumerationMember("red", nil, nil), xsd.NewEnumerationMember("green", nil, nil),
	}))
	shade := derive("Shade", color, xsd.NewFacet(xsd.FacetMaxLength, []string{"5"}, false))
	for _, tc := range []struct {
		name string
		st   *xsd.SimpleType
		want string
	}{
		{"own", color,
			`value is not equal or identical to any member of the enumeration facet of the simple type {urn:t}Color, but cvc-enumeration-valid requires it to be equal or identical to one`},
		{"inherited", shade,
			`value is not equal or identical to any member of the enumeration facet that the simple type {urn:t}Shade inherits from the simple type {urn:t}Color, but cvc-enumeration-valid requires it to be equal or identical to one`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := value.ValidateLexical(b, ownedChain{}, tc.st, "blue", nil, declineEvery{})
			var xe *xsderr.Error
			if !errors.As(err, &xe) || xe.Rule != "cvc-enumeration-valid" {
				t.Fatalf("ValidateLexical(%s, %q) = %v, want a cvc-enumeration-valid rejection", tc.st.Name(), "blue", err)
			}
			if xe.Msg != tc.want {
				t.Errorf("ValidateLexical(%s, %q) Msg =\n%s\nwant\n%s", tc.st.Name(), "blue", xe.Msg, tc.want)
			}
		})
	}
}
