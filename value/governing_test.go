package value_test

import (
	"testing"

	"github.com/kud360/goxsd8/builtin"
	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// seededType returns the builtin named local from builtin.Seed(b).
func seededType(t *testing.T, b value.Backend, local string) *xsd.SimpleType {
	t.Helper()
	seeded, err := builtin.Seed(b)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	for _, st := range seeded {
		if st.Name() == (xsd.QName{Space: xsd.XMLSchemaNS, Local: local}) {
			return st
		}
	}
	t.Fatalf("Seed: no xs:%s", local)
	return nil
}

// TestGoverningMappingNearestMappedAncestor pins that an atomic type the backend
// does not map takes its nearest mapped ancestor's mapping: strict maps no
// integer-family type, so xs:int is governed by xs:decimal's.
func TestGoverningMappingNearestMappedAncestor(t *testing.T) {
	b := strict.New()
	m, ok, err := value.GoverningMapping(b, builtinsOnly{}, seededType(t, b, "int"))
	if err != nil || !ok {
		t.Fatalf("GoverningMapping(xs:int) = ok %v, err %v; want ok true, nil", ok, err)
	}
	v, err := m.Parse("01", nil)
	if err != nil {
		t.Fatalf("Parse(%q): %v", "01", err)
	}
	if m.Canonical == nil {
		t.Fatalf("Canonical is nil, want xs:decimal's")
	}
	c, err := m.Canonical(v)
	if err != nil || c != "1" {
		t.Errorf("Canonical(Parse(%q)) = %q, %v; want %q, nil", "01", c, err, "1")
	}
}

// TestGoverningMappingOwnMappingFirst pins that a backend mapping st itself
// governs it before any ancestor does.
func TestGoverningMappingOwnMappingFirst(t *testing.T) {
	intName := xsd.QName{Space: xsd.XMLSchemaNS, Local: "int"}
	b := value.Override(strict.New(), mapBackend{intName: constMapping("p-int")})
	m, ok, err := value.GoverningMapping(b, builtinsOnly{}, seededType(t, b, "int"))
	if err != nil || !ok {
		t.Fatalf("GoverningMapping(xs:int) = ok %v, err %v; want ok true, nil", ok, err)
	}
	v, err := m.Parse("1", nil)
	if err != nil || v != "p-int" {
		t.Errorf("Parse = %v, %v; want p's mapping (%q)", v, err, "p-int")
	}
}

// TestGoverningMappingRefusesList pins that a list type answers ok false with
// the zero Mapping, even under a backend that maps the list type's own name.
func TestGoverningMappingRefusesList(t *testing.T) {
	nmtokens := xsd.QName{Space: xsd.XMLSchemaNS, Local: "NMTOKENS"}
	b := value.Override(strict.New(), mapBackend{nmtokens: constMapping("p-nmtokens")})
	m, ok, err := value.GoverningMapping(b, builtinsOnly{}, seededType(t, b, "NMTOKENS"))
	if err != nil || ok || m.Parse != nil || m.Canonical != nil {
		t.Errorf("GoverningMapping(xs:NMTOKENS) = parse nil %v, ok %v, err %v; want zero Mapping, false, nil",
			m.Parse == nil, ok, err)
	}
}

// TestGoverningMappingUnreadableChainIsAFault pins that an unresolvable {base
// type definition} is an error, ok false, that is no datatype verdict.
func TestGoverningMappingUnreadableChainIsAFault(t *testing.T) {
	st, err := xsd.NewSimpleType(xsderr.Loc{}, xsd.QName{Local: "dangling"}, xsd.RestrictionDerivation{},
		xsd.SimpleTypeRef{Name: xsd.QName{Space: "urn:x", Local: "gone"}}, nil, nil)
	if err != nil {
		t.Fatalf("NewSimpleType: %v", err)
	}
	m, ok, err := value.GoverningMapping(strict.New(), builtinsOnly{}, st)
	if err == nil || ok || m.Parse != nil {
		t.Fatalf("GoverningMapping(dangling) = parse nil %v, ok %v, err %v; want zero Mapping, false, an error",
			m.Parse == nil, ok, err)
	}
	if value.IsDatatypeVerdict(err) {
		t.Errorf("IsDatatypeVerdict(%v) = true, want false", err)
	}
}
