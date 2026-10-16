package jsonsrc_test

import (
	"fmt"
	"strings"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/validate/jsonsrc"
)

// ExampleValidate assesses three JSON instances against README's order.xsd:
// one the engine charges nothing, one whose empty array leaves order without
// the item child its sequence requires, and one the mapping declines because
// an attribute member follows a child member. The violation is a verdict in
// the result; the decline is an error, with no result. Clean output is still
// not "valid": see [validate.Result].
func ExampleValidate() {
	const orderXSD = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
           targetNamespace="urn:o" elementFormDefault="qualified">
  <xs:element name="order">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="item" type="xs:string" maxOccurs="unbounded"/>
      </xs:sequence>
      <xs:attribute name="kind" type="xs:string"/>
    </xs:complexType>
  </xs:element>
</xs:schema>`

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

	for _, doc := range []string{
		`{"o:order":{"$xmlns":{"o":"urn:o"},"kind":"x","item":["a","b"]}}`,
		`{"o:order":{"$xmlns":{"o":"urn:o"},"kind":"x","item":[]}}`,
		`{"o:order":{"$xmlns":{"o":"urn:o"},"item":["a"],"kind":"x"}}`,
	} {
		res, err := jsonsrc.Validate(v, strings.NewReader(doc), jsonsrc.WithURI("order.json"))
		if err != nil {
			fmt.Println("error:", err)
			continue
		}
		if len(res.Violations()) == 0 && len(res.Unevaluated()) == 0 {
			fmt.Println("no violations, nothing undecided")
		}
		for _, e := range res.Violations() { // []*xsderr.Error, in document order
			fmt.Printf("violation: %s [%s]\n", e.Loc, e.Rule)
		}
		for _, u := range res.Unevaluated() { // a check reached and not performed
			fmt.Printf("undecided: %s [%s]\n", u.Loc(), u.Rule())
		}
	}

	// Output:
	// no violations, nothing undecided
	// violation: order.json:1:2 [cvc-complex-content]
	// error: jsonsrc: order.json:1:49: attribute member "kind" follows a child member of {urn:o}order; attribute members lead the object, as an element's attributes are read before its children
}
