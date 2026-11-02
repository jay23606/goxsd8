package validate_test

import (
	"fmt"
	"strings"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/validate/xmlsrc"
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

// ExampleResult assesses one XML document end to end and folds both of its
// lists into one report: the qty child is not an xs:int, which the assessment
// charges, and the order's assertion is one this engine's XPath evaluator declines.
func ExampleResult() {
	const orderXSD = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="order">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="qty" type="xs:int"/>
      </xs:sequence>
      <xs:assert test="@* = 5"/>
    </xs:complexType>
  </xs:element>
</xs:schema>`
	const orderXML = `<?xml version="1.0"?>
  <order>
      <qty>many</qty>
  </order>`

	backend := strict.New()
	schema, err := parser.Parse("order.xsd",
		parser.WithBackend(backend),
		parser.WithResolver(loader.Map(map[string]string{"order.xsd": orderXSD})))
	if err != nil {
		fmt.Println(err)
		return
	}
	v, err := validate.New(schema, backend)
	if err != nil {
		fmt.Println(err)
		return
	}
	res, err := xmlsrc.Validate(v, strings.NewReader(orderXML), xmlsrc.WithURI("order.xml"))
	if err != nil {
		fmt.Println(err)
		return
	}

	// detail is one entry of the folded report; kind keeps a charge apart
	// from a check the assessment did not perform.
	type detail struct {
		kind      string
		rule      xsderr.Rule
		uri       string
		line, col int
		msg       string
	}
	var report []detail
	for _, e := range res.Violations() {
		report = append(report, detail{"violation", e.Rule, e.Loc.URI, e.Loc.Line, e.Loc.Col, e.Msg})
	}
	for _, u := range res.Unevaluated() {
		loc := u.Loc()
		report = append(report, detail{"undecided", u.Rule(), loc.URI, loc.Line, loc.Col, u.Msg()})
	}

	for _, d := range report {
		fmt.Printf("%s %s %s:%d:%d\n  %s\n", d.kind, d.rule, d.uri, d.line, d.col, d.msg)
	}

	// Output:
	// violation cvc-type order.xml:3:7
	//   element qty's value "many" is not a valid {http://www.w3.org/2001/XMLSchema}int, which cvc-type clause 3.1.3 requires: [cvc-pattern-valid] "many" matches no member of the pattern facet of the simple type {http://www.w3.org/2001/XMLSchema}integer, whose {value} holds "[\-+]?[0-9]+", but cvc-pattern-valid requires one to match
	// undecided cvc-assertion order.xml:2:3
	//   element order was not assessed against assertion 1 of 1 in the {assertions} of the ·governing type definition· an anonymous type definition, whose {test} is "@* = 5", which was not evaluated: this engine's XPath evaluator declined it because the attribute step @* is not supported in this position or over its declared type, so cvc-complex-type clause 6 is undecided for that assertion
}
