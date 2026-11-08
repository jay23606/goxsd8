package codec

import (
	"bytes"
	"errors"
	"math/big"
	"reflect"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
)

// simpleContentTypes declares the simple-content types the rows decode:
// price, an extension of xs:decimal with the attribute cur; capped, a
// restriction of price with a maxInclusive facet; nums, an extension of the
// list ints with the attribute n; tagged, an extension of the union u, of
// xs:int and xs:date, with the attribute value, which claims its field name
// after the value field's; and rich, an extension of price adding the
// attribute k.
const simpleContentTypes = unionTypes + `<xs:complexType name="price"><xs:simpleContent><xs:extension base="xs:decimal"><xs:attribute name="cur" type="xs:string"/></xs:extension></xs:simpleContent></xs:complexType>
<xs:complexType name="capped"><xs:simpleContent><xs:restriction base="s:price"><xs:maxInclusive value="100"/></xs:restriction></xs:simpleContent></xs:complexType>
<xs:simpleType name="ints"><xs:list itemType="xs:int"/></xs:simpleType>
<xs:complexType name="nums"><xs:simpleContent><xs:extension base="s:ints"><xs:attribute name="n" type="xs:int"/></xs:extension></xs:simpleContent></xs:complexType>
<xs:complexType name="tagged"><xs:simpleContent><xs:extension base="s:u"><xs:attribute name="value" type="xs:string"/></xs:extension></xs:simpleContent></xs:complexType>
<xs:complexType name="rich"><xs:simpleContent><xs:extension base="s:price"><xs:attribute name="k" type="xs:int"/></xs:extension></xs:simpleContent></xs:complexType>`

// The simple-content schemas: scSeqSchema's s:r is a sequence of a local p of
// price, a local c of capped, a local l of nums, a local t of tagged, a local
// x of rich, two local elements d and f of price, d with a default and f with
// a fixed value, and an optional <element ref> to the top-level h of price
// with a default of its own; scPriceRoot's, scCappedRoot's, scNumsRoot's and
// scTaggedRoot's s:r is a document element of price, capped, nums and tagged;
// and scDefaultRoot's and scFixedRoot's of price with a default and with a
// fixed value.
const (
	scSeqSchema = simpleContentTypes + `<xs:element name="h" type="s:price" default="9"/>
<xs:element name="r"><xs:complexType><xs:sequence>
<xs:element name="p" type="s:price"/>
<xs:element name="c" type="s:capped"/>
<xs:element name="l" type="s:nums"/>
<xs:element name="t" type="s:tagged"/>
<xs:element name="x" type="s:rich"/>
<xs:element name="d" type="s:price" default="1.5"/>
<xs:element name="f" type="s:price" fixed="2"/>
<xs:element ref="s:h" minOccurs="0"/>
</xs:sequence></xs:complexType></xs:element>`
	scPriceRoot   = simpleContentTypes + `<xs:element name="r" type="s:price"/>`
	scCappedRoot  = simpleContentTypes + `<xs:element name="r" type="s:capped"/>`
	scNumsRoot    = simpleContentTypes + `<xs:element name="r" type="s:nums"/>`
	scTaggedRoot  = simpleContentTypes + `<xs:element name="r" type="s:tagged"/>`
	scDefaultRoot = simpleContentTypes + `<xs:element name="r" type="s:price" default="3.25"/>`
	scFixedRoot   = simpleContentTypes + `<xs:element name="r" type="s:price" fixed="4"/>`
)

// scPrice to scSeq are the types codegen emits for the simple-content
// schemas, which TestSimpleContentTypesAreCodegens pins: Price, which Capped
// and H share in shape, Nums, Tagged, its U the union rows' unionU, Rich, and
// scSeqSchema's R.
type (
	scPrice struct {
		Value big.Rat
		Cur   *string
	}
	scNums struct {
		Value []int32
		N     *int32
	}
	scTagged struct {
		Value  unionU
		Value2 *string
	}
	scRich struct {
		Value big.Rat
		K     *int32
		Cur   *string
	}
	scSeq struct {
		P scPrice
		C scPrice
		L scNums
		T scTagged
		X scRich
		D scPrice
		F scPrice
		H *scPrice
	}
)

// TestSimpleContentTypesAreCodegens pins the types above to those codegen
// emits for scSeqSchema, in shape (codegen/doc.go "# Complex types").
func TestSimpleContentTypesAreCodegens(t *testing.T) {
	price := "struct {\n\tValue big.Rat\n\tCur   *string\n}\n"
	gen := generated(t, schemaOf(t, scSeqSchema))
	for _, decl := range []string{
		"type R struct {\n\tP Price\n\tC Capped\n\tL Nums\n\tT Tagged\n\tX Rich\n\tD Price\n\tF Price\n\tH *H\n}\n",
		"type Price " + price, "type Capped " + price, "type H " + price,
		"type Nums struct {\n\tValue []int32\n\tN     *int32\n}\n",
		"type Tagged struct {\n\tValue  U\n\tValue2 *string\n}\n",
		"type Rich struct {\n\tValue big.Rat\n\tK     *int32\n\tCur   *string\n}\n",
		"type U interface {\n\tisU()\n}\n", "type UInt int32\n", "type UDate Date\n",
	} {
		if !bytes.Contains(gen, []byte(decl)) {
			t.Errorf("codegen emits\n%s\nwhich does not declare\n%s", gen, decl)
		}
	}
}

// TestDecodeSimpleContent pins the lift of simple content (§3.4.2.2
// dcl.ctd.ctsc): an element of a complex type with simple content decodes
// into codegen's struct, its value field the element's ·normalized value·
// under the {content type}'s {simple type definition} (cvc-complex-type
// clause 1.2, key-nv), read as a simple-typed element's is, and its
// attribute fields as for any complex type: an extension of xs:decimal into
// big.Rat, a restriction of it with a facet into the same Go type, an
// extension of a list into the slice, an extension of a union into the
// interface holding the active basic member's variant, an extension of an
// extension, its attribute uses in codegen's order, and an element with no
// character or element child whose declaration has a default or fixed
// {value constraint} holding its {lexical form} (cvc-elt clause 5.1.2): the
// document element, a local one of a named type whose other declarations
// supply another value or none, and a top-level one an <element ref> names.
// A whitespace-only element has a character child and holds its own value,
// and an absent <element ref> supplies nothing. CheckDecode returns nil for
// each row's T.
func TestDecodeSimpleContent(t *testing.T) {
	str := func(s string) *string { return &s }
	i32 := func(i int32) *int32 { return &i }
	rat := func(s string) big.Rat {
		r, ok := new(big.Rat).SetString(s)
		if !ok {
			t.Fatalf("bad rational %q", s)
		}
		return *r
	}
	variants := WithBranches[unionU](unionUInt(0), unionUDate{})
	rows := []struct {
		name, decls, attrs, body string
		into                     target
		want                     any
	}{
		{"an extension of xs:decimal with an attribute", scPriceRoot, ` cur="EUR"`, " 12.50 ", into[scPrice](), scPrice{Value: rat("12.5"), Cur: str("EUR")}},
		{"an extension of xs:decimal, its attribute absent", scPriceRoot, "", "7", into[scPrice](), scPrice{Value: rat("7")}},
		{"a restriction with a facet, the Go type unchanged", scCappedRoot, ` cur="x"`, "99.9", into[scPrice](), scPrice{Value: rat("99.9"), Cur: str("x")}},
		{"an extension of a list", scNumsRoot, ` n="2"`, " 1  2\n3 ", into[scNums](), scNums{Value: []int32{1, 2, 3}, N: i32(2)}},
		{"an extension of a list, empty", scNumsRoot, "", "", into[scNums](), scNums{Value: []int32{}}},
		{"an extension of a union, its xs:int member", scTaggedRoot, ` value="v"`, " 5 ", into[scTagged](variants), scTagged{Value: unionUInt(5), Value2: str("v")}},
		{"an extension of a union, its xs:date member", scTaggedRoot, "", "2000-01-02", into[scTagged](variants),
			scTagged{Value: unionUDate{Year: *big.NewInt(2000), Month: 1, Day: 2}}},
		{"a default on the document element, empty", scDefaultRoot, ` cur="c"`, "", into[scPrice](), scPrice{Value: rat("3.25"), Cur: str("c")}},
		{"a default on the document element, present", scDefaultRoot, "", "1", into[scPrice](), scPrice{Value: rat("1")}},
		{"a fixed value on the document element, empty", scFixedRoot, "", "", into[scPrice](), scPrice{Value: rat("4")}},
		{"a fixed value on the document element, equal and not identical", scFixedRoot, "", "4.0", into[scPrice](), scPrice{Value: rat("4")}},
		{"particles of a sequence", scSeqSchema, "", `<s:p cur="a">1</s:p><s:c>2</s:c><s:l>3 4</s:l><s:t>2001-02-03</s:t><s:x k="6" cur="b">5</s:x><s:d/><s:f></s:f><s:h/>`,
			into[scSeq](variants), scSeq{
				P: scPrice{Value: rat("1"), Cur: str("a")}, C: scPrice{Value: rat("2")}, L: scNums{Value: []int32{3, 4}},
				T: scTagged{Value: unionUDate{Year: *big.NewInt(2001), Month: 2, Day: 3}},
				X: scRich{Value: rat("5"), K: i32(6), Cur: str("b")},
				D: scPrice{Value: rat("1.5")}, F: scPrice{Value: rat("2")}, H: &scPrice{Value: rat("9")},
			}},
		{"particles of a sequence, values present, the ref absent", scSeqSchema, "", `<s:p>1</s:p><s:c>2</s:c><s:l/><s:t>8</s:t><s:x>5</s:x><s:d> 0.5 </s:d><s:f>2.00</s:f>`,
			into[scSeq](variants), scSeq{
				P: scPrice{Value: rat("1")}, C: scPrice{Value: rat("2")}, L: scNums{Value: []int32{}}, T: scTagged{Value: unionUInt(8)},
				X: scRich{Value: rat("5")}, D: scPrice{Value: rat("0.5")}, F: scPrice{Value: rat("2")},
			}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			schema := schemaOf(t, row.decls)
			data := instanceOf(row.attrs, row.body)
			mustAccept(t, schema, data)
			got, err := row.into.decode(schema, data)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, row.want) {
				t.Errorf("Decode = %#v, want %#v", got, row.want)
			}
			if err := row.into.check(schema); err != nil {
				t.Errorf("CheckDecode = %v, want nil for a row Decode decodes", err)
			}
		})
	}
}

// TestDecodeSimpleContentWhitespaceDefault pins that a whitespace-only
// element of simple content has a character child, so its declaration's
// default supplies nothing (cvc-elt clause 5.1): xs:string's value is the
// whitespace itself.
func TestDecodeSimpleContentWhitespaceDefault(t *testing.T) {
	type note struct{ Value string }
	schema := schemaOf(t, `<xs:element name="r" default="d"><xs:complexType><xs:simpleContent><xs:extension base="xs:string"/></xs:simpleContent></xs:complexType></xs:element>`)
	for _, row := range []struct{ body, want string }{{" ", " "}, {"", "d"}} {
		data := instanceOf("", row.body)
		mustAccept(t, schema, data)
		got, err := Decode[note](schema, strict.New(), data)
		if err != nil || got.Value != row.want {
			t.Errorf("Decode(%q) = %v, %v, want Value %q", row.body, got, err, row.want)
		}
	}
}

// TestDeclineErrorTextSimpleContent pins the message of a decline of simple
// content's {simple type definition}, named after the complex type it is the
// {content type} of: xs:anySimpleType's, charged at the document element, and
// CheckDecode's for the anonymous one a restriction with a facet gives, of a
// union two of whose basic members get one Go type.
func TestDeclineErrorTextSimpleContent(t *testing.T) {
	anon := schemaOf(t, `<xs:simpleType name="st"><xs:union memberTypes="xs:string xs:token"/></xs:simpleType>
<xs:complexType name="b"><xs:simpleContent><xs:extension base="s:st"/></xs:simpleContent></xs:complexType>`+
		root(`<xs:simpleContent><xs:restriction base="s:b"><xs:pattern value="a+"/></xs:restriction></xs:simpleContent>`))
	const wantAnon = "codec does not decode a union simple type two of whose basic members get one Go type: the {type definition} of element {urn:s}r declared at s.xsd:2:107, a type defined at s.xsd:2:128, through its {content type}'s anonymous {simple type definition}, a type defined at s.xsd:2:162, through the union's basic member {http://www.w3.org/2001/XMLSchema}token, a type defined at ?; a decoder refusal, not a schema or instance fault"
	if err := CheckDecode[struct{}](anon, rootName); err == nil || err.Error() != wantAnon {
		t.Errorf("CheckDecode = %v, want %q", err, wantAnon)
	}

	schema := schemaOf(t, root(`<xs:simpleContent><xs:extension base="xs:anySimpleType"><xs:attribute name="k" type="xs:string"/></xs:extension></xs:simpleContent>`))
	_, err := Decode[struct{}](schema, strict.New(), instanceOf("", "x"))
	const want = "?:1:1: codec does not decode a special simple type: the {type definition} of element {urn:s}r declared at s.xsd:1:181, a type defined at s.xsd:1:202, through its {content type}'s {simple type definition} {http://www.w3.org/2001/XMLSchema}anySimpleType, a type defined at ?; a decoder refusal, not a schema or instance fault"
	if err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}
	var de *DeclineError
	if !errors.As(err, &de) || de.Construct() != (Construct{constructSpecial}) {
		t.Errorf("Construct = %v, want %s", err, constructSpecial.name())
	}
}
