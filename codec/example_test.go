package codec_test

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/codec"
	"github.com/kud360/goxsd8/codec/testdata/readme/order"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
)

// ExampleDecode parses README's order.xsd, decodes README's order.xml into
// the Order codegen emits for it, then decodes an instance the schema rejects
// and prints each of its violations by its Loc and rule.
func ExampleDecode() {
	readme := filepath.Join("..", "testdata", "readme")
	schema, err := parser.Parse("order.xsd", parser.WithResolver(loader.Dir(readme)))
	if err != nil {
		log.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(readme, "order.xml"))
	if err != nil {
		log.Fatal(err)
	}
	o, err := codec.Decode[order.Order](schema, strict.New(), data, codec.WithURI("order.xml"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(o.Item, *o.Kind)

	invalid := []byte(`<o:order xmlns:o="urn:o" kind="x" size="2">
  <o:note>n</o:note>
</o:order>`)
	_, err = codec.Decode[order.Order](schema, strict.New(), invalid, codec.WithURI("order.xml"))
	// Every violation, as Decode's item 3 reaches them.
	for _, v := range codec.ViolationsOf(err) {
		fmt.Println(v.Loc, v.Rule)
	}
	// Output:
	// [a b] x
	// order.xml:1:35 cvc-complex-type
	// order.xml:2:3 cvc-complex-content
}

// ExampleUndecidedError parses an inline schema whose document element's
// type carries an assertion the XPath subset declines, decodes an instance of
// it, and prints the rule of the check the validator left undecided.
func ExampleUndecidedError() {
	const doc = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:s="urn:s" targetNamespace="urn:s" elementFormDefault="qualified">
  <xs:element name="r"><xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence>
    <xs:assert test="(for $x in s:a return string-length($x)) = 1"/></xs:complexType></xs:element>
</xs:schema>`
	resolve := loader.ResolverFunc(func(_, location string) (io.ReadCloser, string, error) {
		if location != "s.xsd" {
			return nil, "", fmt.Errorf("no document %q", location)
		}
		return io.NopCloser(strings.NewReader(doc)), location, nil
	})
	schema, err := parser.Parse("s.xsd", parser.WithResolver(resolve))
	if err != nil {
		log.Fatal(err)
	}
	type r struct{ A string }
	_, err = codec.Decode[r](schema, strict.New(), []byte(`<s:r xmlns:s="urn:s"><s:a>x</s:a></s:r>`))
	var ue *codec.UndecidedError
	if !errors.As(err, &ue) {
		log.Fatalf("Decode: %v, want an *UndecidedError", err)
	}
	fmt.Println(ue.Unevaluated().Rule())
	// Output:
	// cvc-assertion
}

// pay, payCard and payCash are the sealed interface and branch structs
// codegen emits for ExampleWithBranches' s:pay, written by hand so the example
// generates nothing.
type (
	pay     interface{ isPay() }
	payCard struct{ Card string }
	payCash struct{ Cash []string }
)

func (payCard) isPay() {}
func (payCash) isPay() {}

// ExampleWithBranches parses an inline schema whose document element's type
// is a choice occurring once, decodes an instance of it into the sealed
// interface pay, given its branches by WithBranches, and type-switches on the
// branch the instance matched.
func ExampleWithBranches() {
	const doc = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:s="urn:s" targetNamespace="urn:s" elementFormDefault="qualified">
  <xs:element name="pay"><xs:complexType><xs:choice>
    <xs:element name="card" type="xs:string"/><xs:element name="cash" type="xs:token" maxOccurs="3"/>
  </xs:choice></xs:complexType></xs:element>
</xs:schema>`
	resolve := loader.ResolverFunc(func(_, location string) (io.ReadCloser, string, error) {
		if location != "s.xsd" {
			return nil, "", fmt.Errorf("no document %q", location)
		}
		return io.NopCloser(strings.NewReader(doc)), location, nil
	})
	schema, err := parser.Parse("s.xsd", parser.WithResolver(resolve))
	if err != nil {
		log.Fatal(err)
	}
	instance := []byte(`<s:pay xmlns:s="urn:s"><s:card>4111</s:card></s:pay>`)
	p, err := codec.Decode[pay](schema, strict.New(), instance, codec.WithBranches[pay](payCard{}, payCash{}))
	if err != nil {
		fmt.Println(err)
		return
	}
	switch b := (*p).(type) {
	case payCard:
		fmt.Println("card", b.Card)
	case payCash:
		fmt.Println("cash", b.Cash)
	}
	// Output:
	// card 4111
}
