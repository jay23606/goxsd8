package codec

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// The splice schemas, each declaring s:r, whose content sequence holds a
// nested sequence particle codegen splices (codegen/doc.go "# Particles"):
// spliceInlineSchema's, sequence(a, sequence(b)); spliceRefSchema's, a
// <group ref> to the sequence group g of b after a; spliceOptSchema's, a
// sequence with {min occurs} 0 of an optional b and a repeated c, after a,
// emptiable; spliceExtSchema's r is of t2, an extension of t, sequence(a),
// adding sequence(b), so t2's {particle} is sequence(t's particle, the added
// one) (§3.4.2.3.3 dcl.ctd.ctcc.common clause 4.2.3.3), and
// spliceExt2Schema's of t3, extending t2 by sequence(c); spliceWildInSchema's
// is sequence(sequence(a, any), b), spliceWildAfterSchema's
// sequence(sequence(a, b), any), and spliceWildRefSchema's
// sequence(g, b), g the group of sequence(a, any), each wildcard ##other;
// and spliceAnyNamesSchema's is sequence(a, sequence(any over urn:a), any
// over urn:b), whose two wildcard fields claim one Namer across the splice.
const (
	spliceInlineSchema = `<xs:element name="r"><xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/><xs:sequence><xs:element name="b" type="xs:int"/></xs:sequence></xs:sequence></xs:complexType></xs:element>`
	spliceRefSchema    = `<xs:group name="g"><xs:sequence><xs:element name="b" type="xs:int"/></xs:sequence></xs:group>
  <xs:element name="r"><xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/><xs:group ref="s:g"/></xs:sequence></xs:complexType></xs:element>`
	spliceOptSchema = `<xs:element name="r"><xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/>
  <xs:sequence minOccurs="0"><xs:element name="b" type="xs:int" minOccurs="0"/><xs:element name="c" type="xs:string" minOccurs="0" maxOccurs="unbounded"/></xs:sequence></xs:sequence></xs:complexType></xs:element>`
	spliceExtSchema = `<xs:complexType name="t"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>
  <xs:complexType name="t2"><xs:complexContent><xs:extension base="s:t"><xs:sequence><xs:element name="b" type="xs:int"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType>`
	spliceExt1Schema = spliceExtSchema + `<xs:element name="r" type="s:t2"/>`
	spliceExt2Schema = spliceExtSchema + `<xs:complexType name="t3"><xs:complexContent><xs:extension base="s:t2"><xs:sequence><xs:element name="c" type="xs:string"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType>
  <xs:element name="r" type="s:t3"/>`
	spliceWildInSchema    = `<xs:element name="r"><xs:complexType><xs:sequence><xs:sequence><xs:element name="a" type="xs:string"/><xs:any namespace="##other" processContents="lax"/></xs:sequence><xs:element name="b" type="xs:int"/></xs:sequence></xs:complexType></xs:element>`
	spliceWildAfterSchema = `<xs:element name="r"><xs:complexType><xs:sequence><xs:sequence><xs:element name="a" type="xs:string"/><xs:element name="b" type="xs:int"/></xs:sequence><xs:any namespace="##other" processContents="lax"/></xs:sequence></xs:complexType></xs:element>`
	spliceWildRefSchema   = `<xs:group name="g"><xs:sequence><xs:element name="a" type="xs:string"/><xs:any namespace="##other" processContents="lax"/></xs:sequence></xs:group>
  <xs:element name="r"><xs:complexType><xs:sequence><xs:group ref="s:g"/><xs:element name="b" type="xs:int"/></xs:sequence></xs:complexType></xs:element>`
	spliceAnyNamesSchema = `<xs:element name="r"><xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/><xs:sequence><xs:any namespace="urn:a" processContents="lax"/></xs:sequence><xs:any namespace="urn:b" processContents="lax"/></xs:sequence></xs:complexType></xs:element>`
)

// The types codegen emits for the splice schemas' s:r, in shape, which
// TestSpliceTypesAreCodegens pins.
type (
	spliceAB struct {
		A string
		B int32
	}
	spliceOpt struct {
		A string
		B *int32
		C []string
	}
	spliceABC struct {
		A string
		B int32
		C string
	}
	spliceWildIn struct {
		A   string
		Any anyElement
		B   int32
	}
	spliceWildAfter struct {
		A   string
		B   int32
		Any anyElement
	}
	spliceAnyNames struct {
		A    string
		Any  anyElement
		Any2 anyElement
	}
)

// TestSpliceTypesAreCodegens pins the types above to the struct codegen
// emits for each splice schema's s:r (codegen/doc.go "# Particles"): the
// nested sequence's fields in its place, in particle order, named by the one
// Namer of r's struct.
func TestSpliceTypesAreCodegens(t *testing.T) {
	rows := []struct {
		decls, want string
	}{
		{spliceInlineSchema, "type R struct {\n\tA string\n\tB int32\n}\n"},
		{spliceRefSchema, "type R struct {\n\tA string\n\tB int32\n}\n"},
		{spliceOptSchema, "type R struct {\n\tA string\n\tB *int32\n\tC []string\n}\n"},
		{spliceExt1Schema, "type R struct {\n\tA string\n\tB int32\n}\n"},
		{spliceExt2Schema, "type R struct {\n\tA string\n\tB int32\n\tC string\n}\n"},
		{spliceWildInSchema, "type R struct {\n\tA   string\n\tAny AnyElement\n\tB   int32\n}\n"},
		{spliceWildAfterSchema, "type R struct {\n\tA   string\n\tB   int32\n\tAny AnyElement\n}\n"},
		{spliceWildRefSchema, "type R struct {\n\tA   string\n\tAny AnyElement\n\tB   int32\n}\n"},
		{spliceAnyNamesSchema, "type R struct {\n\tA    string\n\tAny  AnyElement\n\tAny2 AnyElement\n}\n"},
	}
	for _, row := range rows {
		gen := generated(t, schemaOf(t, row.decls))
		if !bytes.Contains(gen, []byte(row.want)) {
			t.Errorf("codegen emits\n%s\nwhich does not declare\n%s", gen, row.want)
		}
	}
}

// TestDecodeSplices pins the splice lift: a nested sequence particle
// goshape.Splices answers true for, inline, through a <group ref>, with
// {min occurs} 0 over an emptiable group, absent and present, or an
// extension's {particle} sequence(base particle, added) at one level and
// two, decodes into the fields its own particles give, in particle order,
// and a wildcard particle inside a spliced sequence, inline or referenced,
// or after one decodes into its own field at its place, the field index
// wildcardSlot.places holds for its Matcher position. CheckDecode returns
// nil for each row's T.
func TestDecodeSplices(t *testing.T) {
	i := func(n int32) *int32 { return &n }
	w := func(space string) anyElement { return anyElement{Space: space, Local: "w"} }
	rows := []struct {
		name, decls, body string
		into              target
		want              any
	}{
		{"an inline nested sequence", spliceInlineSchema, "<s:a>x</s:a><s:b>1</s:b>", into[spliceAB](), spliceAB{A: "x", B: 1}},
		{"a <group ref> to a sequence group", spliceRefSchema, "<s:a>x</s:a><s:b>2</s:b>", into[spliceAB](), spliceAB{A: "x", B: 2}},
		{"an optional emptiable nested sequence, absent", spliceOptSchema, "<s:a>x</s:a>", into[spliceOpt](), spliceOpt{A: "x"}},
		{"an optional emptiable nested sequence, present", spliceOptSchema, "<s:a>x</s:a><s:b>3</s:b><s:c>p</s:c><s:c>q</s:c>", into[spliceOpt](),
			spliceOpt{A: "x", B: i(3), C: []string{"p", "q"}}},
		{"an extension adding a sequence", spliceExt1Schema, "<s:a>x</s:a><s:b>4</s:b>", into[spliceAB](), spliceAB{A: "x", B: 4}},
		{"an extension of an extension, each adding a sequence", spliceExt2Schema, "<s:a>x</s:a><s:b>5</s:b><s:c>y</s:c>", into[spliceABC](), spliceABC{A: "x", B: 5, C: "y"}},
		{"a wildcard inside a spliced sequence", spliceWildInSchema, `<s:a>x</s:a><o:w xmlns:o="urn:o"/><s:b>6</s:b>`, into[spliceWildIn](),
			spliceWildIn{A: "x", Any: w("urn:o"), B: 6}},
		{"a wildcard after a spliced sequence", spliceWildAfterSchema, `<s:a>x</s:a><s:b>7</s:b><o:w xmlns:o="urn:o"/>`, into[spliceWildAfter](),
			spliceWildAfter{A: "x", B: 7, Any: w("urn:o")}},
		{"a wildcard inside a spliced <group ref>", spliceWildRefSchema, `<s:a>x</s:a><o:w xmlns:o="urn:o"/><s:b>8</s:b>`, into[spliceWildIn](),
			spliceWildIn{A: "x", Any: w("urn:o"), B: 8}},
		{"two wildcards claiming one Namer across a splice", spliceAnyNamesSchema, `<s:a>x</s:a><a:w xmlns:a="urn:a"/><b:w xmlns:b="urn:b"/>`, into[spliceAnyNames](),
			spliceAnyNames{A: "x", Any: w("urn:a"), Any2: w("urn:b")}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			schema := schemaOf(t, row.decls)
			data := instanceOf("", row.body)
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

// TestDecodeSplicedWildcardOrdinal pins item 5's ordinal of a wildcard
// particle inside a spliced sequence, counted from 1 across the splice in
// particle order: type v's content is sequence(any, sequence(any)), the
// second wildcard rebuilt through xsd.NewParticle with the vacuous
// Occurs{0,0} (no schema document yields one, §3.10.2), so its plain error
// names it wildcard particle 2, Decode's and CheckDecode's alike.
func TestDecodeSplicedWildcardOrdinal(t *testing.T) {
	parsed := schemaOf(t, `<xs:complexType name="v"><xs:sequence><xs:any namespace="##other" processContents="lax"/></xs:sequence></xs:complexType><xs:element name="r" type="s:v"/>`)
	v := xsd.QName{Space: "urn:s", Local: "v"}
	declared, ok := parsed.Type(v)
	if !ok {
		t.Fatalf("the parsed schema declares no type %s", v)
	}
	content := declared.(xsd.ComplexType).ContentType().(xsd.ElementContent)
	wild := content.Particle.Term().(xsd.ResolvedTerm).Term.(xsd.ModelGroup).Particles()[0]
	none, err := xsd.NewOccurs(xsderr.Loc{}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	vacuous, err := xsd.NewParticle(xsderr.Loc{}, none, wild.Term())
	if err != nil {
		t.Fatal(err)
	}
	inner, err := xsd.NewModelGroup(xsderr.Loc{}, xsd.CompositorSequence, []xsd.Particle{vacuous})
	if err != nil {
		t.Fatal(err)
	}
	spliced, err := xsd.NewParticle(xsderr.Loc{}, content.Particle.Occurs(), xsd.ResolvedTerm{Term: inner})
	if err != nil {
		t.Fatal(err)
	}
	outer, err := xsd.NewModelGroup(xsderr.Loc{}, xsd.CompositorSequence, []xsd.Particle{wild, spliced})
	if err != nil {
		t.Fatal(err)
	}
	particle, err := xsd.NewParticle(xsderr.Loc{}, content.Particle.Occurs(), xsd.ResolvedTerm{Term: outer})
	if err != nil {
		t.Fatal(err)
	}
	ct, err := xsd.NewComplexType(declared.Loc(), v, xsd.QName{Space: xsd.XMLSchemaNS, Local: "anyType"}, nil, xsd.DerivationRestriction, false,
		nil, nil, nil, xsd.ElementContent{Particle: particle}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	schema := replacedSchema(t, parsed, ct)
	data := instanceOf("", `<o:w xmlns:o="urn:o"/>`)
	mustAccept(t, schema, data)
	_, decoded := Decode[struct{}](schema, strict.New(), data)
	const prefix, suffix = "codec: wildcard particle 2 of the {type definition} of element {urn:s}r", " has {max occurs} 0, for which codegen emits no type"
	for _, got := range []struct {
		name string
		err  error
	}{{"Decode", decoded}, {"CheckDecode", CheckDecode[struct{}](schema, rootName)}} {
		if got.err == nil || !strings.HasPrefix(got.err.Error(), prefix) || !strings.HasSuffix(got.err.Error(), suffix) {
			t.Errorf("%s = %v, want %q ... %q", got.name, got.err, prefix, suffix)
		}
	}
}
