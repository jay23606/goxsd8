package value_test

import (
	"fmt"
	"time"

	"github.com/kud360/goxsd8/builtin"
	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xpath"
	"github.com/kud360/goxsd8/xsd"
)

// oneType is a trivial value.Backend that maps exactly one QName. A real
// backend parses lexical forms into typed values implementing the value
// capabilities; this stub only needs to demonstrate composition.
type oneType struct {
	typ     xsd.QName
	mapping value.Mapping
}

func (b oneType) Mapping(typ xsd.QName) (value.Mapping, bool) {
	if typ == b.typ {
		return b.mapping, true
	}
	return value.Mapping{}, false
}

// ExampleOverride backs only xs:decimal with a custom "money" mapping while
// inheriting every other type — here xs:string — from the base backend.
func ExampleOverride() {
	// A QName is an expanded name {namespace}local; the XSD builtins live in
	// the XML Schema namespace.
	decimal := xsd.QName{Space: xsd.XMLSchemaNS, Local: "decimal"}
	str := xsd.QName{Space: xsd.XMLSchemaNS, Local: "string"}

	base := oneType{
		typ: str,
		mapping: value.Mapping{
			Parse: func(lexical string, _ value.Context) (value.Value, error) {
				return lexical, nil
			},
		},
	}
	money := oneType{
		typ: decimal,
		mapping: value.Mapping{
			Parse: func(lexical string, _ value.Context) (value.Value, error) {
				return "money(" + lexical + ")", nil
			},
		},
	}

	// Override yields money's mapping for xs:decimal and base's for the rest.
	backend := value.Override(base, money)

	if m, ok := backend.Mapping(decimal); ok {
		v, _ := m.Parse("3.14", nil)
		fmt.Printf("decimal -> %v\n", v)
	}
	if m, ok := backend.Mapping(str); ok {
		v, _ := m.Parse("hello", nil)
		fmt.Printf("string  -> %v\n", v)
	}

	// Output:
	// decimal -> money(3.14)
	// string  -> hello
}

// builtinsOnly is an xsd.TypeResolver that resolves no name: the seeded
// builtins link their {base type definition} chains by pointer, so a type
// drawn from builtin.Seed needs no by-name lookup.
type builtinsOnly struct{}

func (builtinsOnly) Type(xsd.QName) (xsd.TypeDefinition, bool) { return nil, false }

// ExampleValidateLexical reaches the value of a type its backend does not map
// directly. builtin/strict maps xs:decimal but not xs:integer, so
// Backend.Mapping answers false for xs:integer; ValidateLexical runs the whole
// facet pipeline and parses the literal under xs:decimal's mapping, the nearest
// mapped ancestor. The evaluator is the one package validate passes.
func ExampleValidateLexical() {
	backend := strict.New()
	seeded, err := builtin.Seed(backend)
	if err != nil {
		fmt.Println(err)
		return
	}
	var integer *xsd.SimpleType
	for _, st := range seeded {
		if st.Name() == (xsd.QName{Space: xsd.XMLSchemaNS, Local: "integer"}) {
			integer = st
		}
	}

	_, mapped := backend.Mapping(integer.Name())
	fmt.Println("mapped directly:", mapped)

	v, err := value.ValidateLexical(backend, builtinsOnly{}, integer, "42", nil, xpath.FacetAssertions(time.Time{}))
	if err != nil {
		fmt.Println(err)
		return
	}
	if c, ok := v.(value.Canonical); ok {
		fmt.Println("value:", c.Canonical())
	}

	// Output:
	// mapped directly: false
	// value: 42
}
