package validate_test

import (
	"fmt"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// attr is this example's [validate.Attribute].
type attr struct {
	name  xsd.QName
	value string
}

func (a attr) Name() xsd.QName { return a.name }
func (a attr) Value() string   { return a.value }
func (a attr) Loc() xsderr.Loc { return xsderr.Loc{} }

// ExampleValidator_ConditionallySelected selects the type of a shape element
// by its kind attribute, through the {type table} its declaration carries, and
// then asks a table whose {test} reads a child element, which the engine cannot
// evaluate, so that selection is undecided.
func ExampleValidator_ConditionallySelected() {
	const shapesXSD = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="Shape">
    <xs:attribute name="kind" type="xs:string"/>
  </xs:complexType>
  <xs:complexType name="Circle">
    <xs:complexContent>
      <xs:extension base="Shape">
        <xs:attribute name="radius" type="xs:decimal"/>
      </xs:extension>
    </xs:complexContent>
  </xs:complexType>
  <xs:element name="shape" type="Shape">
    <xs:alternative test="@kind = 'circle'" type="Circle"/>
    <xs:alternative type="Shape"/>
  </xs:element>
  <xs:element name="probe" type="Shape">
    <xs:alternative test="child::x" type="Circle"/>
    <xs:alternative type="Shape"/>
  </xs:element>
</xs:schema>`

	backend := strict.New()
	schema, err := parser.Parse("shapes.xsd",
		parser.WithBackend(backend),
		parser.WithResolver(loader.Map(map[string]string{"shapes.xsd": shapesXSD})))
	if err != nil {
		fmt.Println(err)
		return
	}
	v, err := validate.New(schema, backend)
	if err != nil {
		fmt.Println(err)
		return
	}

	kind := xsd.QName{Local: "kind"}
	for _, c := range []struct {
		element string
		attrs   []validate.Attribute
	}{
		{"shape", []validate.Attribute{attr{kind, "circle"}}},
		{"shape", []validate.Attribute{attr{kind, "square"}}},
		{"probe", nil},
	} {
		decl, found := v.Schema().Element(xsd.QName{Local: c.element})
		if !found {
			fmt.Println("no declaration for", c.element)
			continue
		}
		table, tabled := decl.TypeTable()
		if !tabled {
			fmt.Println(c.element, "has no type table: its {type definition} is the selection")
			continue
		}
		// A root element has no [inherited attributes], so inherited is nil.
		slot, decided := v.ConditionallySelected(table, c.attrs, nil)
		if !decided {
			fmt.Println(c.element, "undecided: withheld, no fallback to the declared type")
			continue
		}
		typ, resolved := v.Schema().ResolvedType(slot)
		if !resolved {
			fmt.Println(c.element, "selects a slot that resolves to no type")
			continue
		}
		fmt.Println(c.element, "selects", typ.Name().Local)
	}

	// Output:
	// shape selects Circle
	// shape selects Shape
	// probe undecided: withheld, no fallback to the declared type
}
