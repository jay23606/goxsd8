package codec_test

import (
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/codec"
	"github.com/kud360/goxsd8/codec/testdata/readme/order"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/xsd"
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

// ExampleCheckDecode runs CheckDecode once at start-up, before any instance
// arrives, on an inline schema's two top-level element declarations: s:r,
// which decodes into r, and s:n, whose {nillable} true is a construct outside
// the decoded slice. It then checks s:r against a T that is not the type
// codegen emits for it.
func ExampleCheckDecode() {
	const doc = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:s" elementFormDefault="qualified">
  <xs:element name="r"><xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType></xs:element>
  <xs:element name="n" type="xs:string" nillable="true"/>
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
	fmt.Println(codec.CheckDecode[r](schema, xsd.QName{Space: "urn:s", Local: "r"}))

	// Item 5 of Decode's doc: a construct outside the decoded slice.
	err = codec.CheckDecode[string](schema, xsd.QName{Space: "urn:s", Local: "n"})
	var de *codec.DeclineError
	if !errors.As(err, &de) {
		log.Fatalf("CheckDecode: %v, want a *DeclineError", err)
	}
	fmt.Println(de.Construct())

	// Item 6 of Decode's doc: a T that is not the type codegen emits.
	err = codec.CheckDecode[int](schema, xsd.QName{Space: "urn:s", Local: "r"})
	fmt.Println(errors.Is(err, codec.ErrTypeMismatch))
	// Output:
	// <nil>
	// an element declaration with {nillable} true
	// true
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

// precisionDecimal, its Value interface precisionDecimalValue and the four
// arms are the datatype struct, sealed interface and arms codegen emits for
// xs:precisionDecimal, and decimals the struct it emits for
// ExampleWithPrecisionDecimalArms' s:r, written by hand so the example
// generates nothing.
type (
	precisionDecimal               struct{ Value precisionDecimalValue }
	precisionDecimalValue          interface{ isPrecisionDecimalValue() }
	precisionDecimalValueNumerical struct {
		Coefficient big.Int
		Scale       big.Int
		Negative    bool
	}
	precisionDecimalValuePositiveInfinity struct{}
	precisionDecimalValueNegativeInfinity struct{}
	precisionDecimalValueNotANumber       struct{}
	decimals                              struct{ V []precisionDecimal }
)

func (precisionDecimalValueNumerical) isPrecisionDecimalValue()        {}
func (precisionDecimalValuePositiveInfinity) isPrecisionDecimalValue() {}
func (precisionDecimalValueNegativeInfinity) isPrecisionDecimalValue() {}
func (precisionDecimalValueNotANumber) isPrecisionDecimalValue()       {}

// ExampleWithPrecisionDecimalArms parses an inline schema whose document
// element holds repeated xs:precisionDecimal elements, decodes 3E2, -0 and
// INF into the sealed interface precisionDecimalValue, given its arms in their
// roles by WithPrecisionDecimalArms, and type-switches on each value's arm.
func ExampleWithPrecisionDecimalArms() {
	const doc = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:s="urn:s" targetNamespace="urn:s" elementFormDefault="qualified">
  <xs:element name="r"><xs:complexType><xs:sequence>
    <xs:element name="v" type="xs:precisionDecimal" maxOccurs="unbounded"/>
  </xs:sequence></xs:complexType></xs:element>
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
	instance := []byte(`<s:r xmlns:s="urn:s"><s:v>3E2</s:v><s:v>-0</s:v><s:v>INF</s:v></s:r>`)
	arms := codec.WithPrecisionDecimalArms[precisionDecimalValue](precisionDecimalValueNumerical{},
		precisionDecimalValuePositiveInfinity{}, precisionDecimalValueNegativeInfinity{}, precisionDecimalValueNotANumber{})
	r, err := codec.Decode[decimals](schema, strict.New(), instance, arms)
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, v := range r.V {
		switch a := v.Value.(type) {
		case precisionDecimalValueNumerical:
			fmt.Println("numerical", a.Coefficient.String(), a.Scale.String(), a.Negative)
		case precisionDecimalValuePositiveInfinity:
			fmt.Println("positive infinity")
		case precisionDecimalValueNegativeInfinity:
			fmt.Println("negative infinity")
		case precisionDecimalValueNotANumber:
			fmt.Println("not a number")
		}
	}
	// Output:
	// numerical 3 -2 false
	// numerical 0 0 true
	// positive infinity
}
