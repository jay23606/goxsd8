package codec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// unionTypes declares the union types the union rows decode: u, of xs:int
// and xs:date, whose lexical spaces are disjoint; nest, a union of u and
// xs:boolean, whose basic members are u's and xs:boolean; num, of xs:int and
// xs:decimal, both of which accept "5"; withList, of xs:boolean and an
// anonymous list of xs:int; and the top-level element pin, of an anonymous
// union of xs:boolean and xs:int.
const unionTypes = `<xs:simpleType name="u"><xs:union memberTypes="xs:int xs:date"/></xs:simpleType>
<xs:simpleType name="nest"><xs:union memberTypes="s:u xs:boolean"/></xs:simpleType>
<xs:simpleType name="num"><xs:union memberTypes="xs:int xs:decimal"/></xs:simpleType>
<xs:simpleType name="withList"><xs:union memberTypes="xs:boolean"><xs:simpleType><xs:list itemType="xs:int"/></xs:simpleType></xs:union></xs:simpleType>
<xs:element name="pin"><xs:simpleType><xs:union memberTypes="xs:boolean xs:int"/></xs:simpleType></xs:element>`

// The union schemas: unionSchema's s:r holds a union at every site a field
// takes one: a local element occurring once, optionally and repeated, a list
// of u, the nested union, num, withList, an optional <element ref> to pin,
// an optional attribute and one with a default; unionRootSchema's s:r is a
// document element of the named union u, unionListRootSchema's of a list of
// u; and unionChoiceSchema's choice holds an alternative of u beside one of
// xs:string.
const (
	unionSchema = unionTypes + `<xs:element name="r"><xs:complexType><xs:sequence>
<xs:element name="a" type="s:u"/>
<xs:element name="opt" type="s:u" minOccurs="0"/>
<xs:element name="many" type="s:u" maxOccurs="unbounded"/>
<xs:element name="items"><xs:simpleType><xs:list itemType="s:u"/></xs:simpleType></xs:element>
<xs:element name="nested" type="s:nest"/>
<xs:element name="n" type="s:num"/>
<xs:element name="wl" type="s:withList"/>
<xs:element ref="s:pin" minOccurs="0"/>
</xs:sequence>
<xs:attribute name="at" type="s:u"/>
<xs:attribute name="dflt" type="s:u" default="2000-01-01"/>
</xs:complexType></xs:element>`
	unionRootSchema     = unionTypes + `<xs:element name="r" type="s:u"/>`
	unionListRootSchema = unionTypes + `<xs:element name="r"><xs:simpleType><xs:list itemType="s:u"/></xs:simpleType></xs:element>`
	unionChoiceSchema   = unionTypes + `<xs:element name="r"><xs:complexType><xs:choice><xs:element name="a" type="s:u"/><xs:element name="b" type="xs:string"/></xs:choice></xs:complexType></xs:element>`
	unionListMemberRoot = unionTypes + `<xs:element name="r"><xs:simpleType><xs:union memberTypes="xs:date"><xs:simpleType><xs:list itemType="s:u"/></xs:simpleType></xs:union></xs:simpleType></xs:element>`
)

// unions to unionChoiceB are the types codegen emits for the union schemas,
// which TestUnionTypesAreCodegens pins: unionSchema's R, Pin, PinUnion, U,
// Nest, Num and WithList with their variants and Date; unionRootSchema's R,
// a named type over U, as Size2 is over Size in codegen's unions golden;
// unionListRootSchema's R; unionChoiceSchema's R and its branches; and
// unionListMemberRoot's R, over RUnion, a union of xs:date and a list of u,
// whose variant RUnionList is []U.
type (
	unions struct {
		A      unionU
		Opt    unionU
		Many   []unionU
		Items  []unionU
		Nested unionNest
		N      unionNum
		Wl     unionWithList
		Pin    unionPin
		At     unionU
		Dflt   unionU
	}
	unionPin             unionPinUnion
	unionPinUnion        interface{ isPinUnion() }
	unionPinUnionBoolean bool
	unionPinUnionInt     int32
	unionU               interface{ isU() }
	unionUInt            int32
	unionUDate           unionDate
	unionDate            struct {
		Year           big.Int
		Month          uint8
		Day            uint8
		TimezoneOffset *int16
	}
	unionNest            interface{ isNest() }
	unionNestInt         int32
	unionNestDate        unionDate
	unionNestBoolean     bool
	unionNum             interface{ isNum() }
	unionNumInt          int32
	unionNumDecimal      big.Rat
	unionWithList        interface{ isWithList() }
	unionWithListBoolean bool
	unionWithListList    []int32
	unionRoot            unionU
	unionListRoot        []unionU
	unionChoice          interface{ isUnionChoice() }
	unionChoiceA         struct{ A unionU }
	unionChoiceB         struct{ B string }
	unionLM              unionLMUnion
	unionLMUnion         interface{ isRUnion() }
	unionLMUnionDate     unionDate
	unionLMUnionList     []unionU
)

func (unionLMUnionDate) isRUnion() {}
func (unionLMUnionList) isRUnion() {}

func (unionPinUnionBoolean) isPinUnion() {}
func (unionPinUnionInt) isPinUnion()     {}
func (unionUInt) isU()                   {}
func (unionUDate) isU()                  {}
func (unionNestInt) isNest()             {}
func (unionNestDate) isNest()            {}
func (unionNestBoolean) isNest()         {}
func (unionNumInt) isNum()               {}
func (unionNumDecimal) isNum()           {}
func (unionWithListBoolean) isWithList() {}
func (unionWithListList) isWithList()    {}
func (unionChoiceA) isUnionChoice()      {}
func (unionChoiceB) isUnionChoice()      {}

// unionUOther and unionUStray implement U and are none of codegen's variants:
// TestDecodeUnionMismatches' strays.
type (
	unionUOther int32
	unionUStray string
)

func (unionUOther) isU() {}
func (unionUStray) isU() {}

// unionVariants supplies every union interface's variants, in an order other
// than codegen's for U and Nest, which WithBranches admits.
var unionVariants = []Option{
	WithBranches[unionU](unionUDate{}, unionUInt(0)),
	WithBranches[unionNest](unionNestBoolean(false), unionNestInt(0), unionNestDate{}),
	WithBranches[unionNum](unionNumInt(0), unionNumDecimal{}),
	WithBranches[unionWithList](unionWithListBoolean(false), unionWithListList(nil)),
	WithBranches[unionPinUnion](unionPinUnionBoolean(false), unionPinUnionInt(0)),
	WithBranches[unionChoice](unionChoiceA{}, unionChoiceB{}),
	WithBranches[unionLMUnion](unionLMUnionList(nil), unionLMUnionDate{}),
}

// TestUnionTypesAreCodegens pins the types above to those codegen emits for
// each union schema, in shape (codegen/doc.go "# Simple types").
func TestUnionTypesAreCodegens(t *testing.T) {
	rows := []struct {
		decls string
		want  []string
	}{
		{unionSchema, []string{
			"type R struct {\n\tA      U\n\tOpt    U\n\tMany   []U\n\tItems  []U\n\tNested Nest\n\tN      Num\n\tWl     WithList\n\tPin    Pin\n\tAt     U\n\tDflt   U\n}\n",
			"type Pin PinUnion\n", "type PinUnion interface {\n\tisPinUnion()\n}\n", "type PinUnionBoolean bool\n", "type PinUnionInt int32\n",
			"type U interface {\n\tisU()\n}\n", "type UInt int32\n", "type UDate Date\n",
			"type Date struct {\n\tYear           big.Int\n\tMonth          uint8\n\tDay            uint8\n\tTimezoneOffset *int16\n}\n",
			"type NestInt int32\n", "type NestDate Date\n", "type NestBoolean bool\n",
			"type NumInt int32\n", "type NumDecimal big.Rat\n",
			"type WithListBoolean bool\n", "type WithListList []int32\n",
		}},
		{unionRootSchema, []string{"type R U\n"}},
		{unionListRootSchema, []string{"type R []U\n"}},
		{unionChoiceSchema, []string{"type RA struct {\n\tA U\n}\n\nfunc (RA) isR() {}\n", "type RB struct {\n\tB string\n}\n\nfunc (RB) isR() {}\n"}},
		{unionListMemberRoot, []string{"type R RUnion\n", "type RUnionDate Date\n", "type RUnionList []U\n"}},
	}
	for _, row := range rows {
		gen := generated(t, schemaOf(t, row.decls))
		for _, decl := range row.want {
			if !bytes.Contains(gen, []byte(decl)) {
				t.Errorf("codegen emits\n%s\nwhich does not declare\n%s", gen, decl)
			}
		}
	}
}

// TestDecodeUnions pins the union lift (Datatypes §2.4.1.3): a value decodes
// into the variant of its ·active basic member·, the first member in order
// that accepts it, descended through a member that is itself a union
// (dt-active-member, dt-active-basic-member, cvc-datatype-valid clause 2.3),
// read as that member's value is, its whiteSpace included: at every site a
// union reaches, each of u's members, a list's items of both members in one
// list, the nested union into its inner basic member's variant, "5" into
// num's xs:int variant though xs:decimal accepts it too, withList's list
// member into the variant over []int32, a list member of a union into the
// variant over []U, a date its xs:date member takes first, a choice's union
// alternative into its branch, an <element ref> to pin into Pin over
// PinUnion, an absent optional element and attribute as the nil interface,
// and the default's {lexical form} into its active member's variant, a
// present attribute's own value in its place. CheckDecode returns nil for
// each row's T.
func TestDecodeUnions(t *testing.T) {
	date := func(y int64, m, d uint8) unionDate { return unionDate{Year: *big.NewInt(y), Month: m, Day: d} }
	rows := []struct {
		name, decls, attrs, body string
		into                     target
		want                     any
	}{
		{"every site, the date member at each", unionSchema, ` at="2000-01-02"`,
			"<s:a>2000-01-03</s:a><s:opt>2000-01-04</s:opt><s:many>2000-01-05</s:many><s:items>2000-01-06</s:items>" +
				"<s:nested>2000-01-07</s:nested><s:n>5.5</s:n><s:wl>true</s:wl><s:pin>true</s:pin>", into[unions](unionVariants...),
			unions{A: unionUDate(date(2000, 1, 3)), Opt: unionUDate(date(2000, 1, 4)), Many: []unionU{unionUDate(date(2000, 1, 5))},
				Items: []unionU{unionUDate(date(2000, 1, 6))}, Nested: unionNestDate(date(2000, 1, 7)), N: unionNumDecimal(*big.NewRat(11, 2)),
				Wl: unionWithListBoolean(true), Pin: unionPinUnionBoolean(true), At: unionUDate(date(2000, 1, 2)), Dflt: unionUDate(date(2000, 1, 1))}},
		{"every site, the xs:int member at each, optional ones absent", unionSchema, ` dflt=" 7 "`,
			"<s:a> 5 </s:a><s:many>1</s:many><s:many>-2</s:many><s:items> 3  2000-01-01 4 </s:items><s:nested>6</s:nested><s:n>5</s:n><s:wl> 1  2 </s:wl>", into[unions](unionVariants...),
			unions{A: unionUInt(5), Many: []unionU{unionUInt(1), unionUInt(-2)}, Items: []unionU{unionUInt(3), unionUDate(date(2000, 1, 1)), unionUInt(4)},
				Nested: unionNestInt(6), N: unionNumInt(5), Wl: unionWithListList{1, 2}, Dflt: unionUInt(7)}},
		{"the nested union's own member", unionSchema, "", "<s:a>1</s:a><s:many>1</s:many><s:items/><s:nested>false</s:nested><s:n>1</s:n><s:wl>0</s:wl><s:pin>9</s:pin>", into[unions](unionVariants...),
			unions{A: unionUInt(1), Many: []unionU{unionUInt(1)}, Items: []unionU{}, Nested: unionNestBoolean(false), N: unionNumInt(1), Wl: unionWithListBoolean(false),
				Pin: unionPinUnionInt(9), Dflt: unionUDate(date(2000, 1, 1))}},
		{"a document element of a named union, the xs:int member, its whiteSpace collapsed", unionRootSchema, "", " 5 ", into[unionRoot](unionVariants...), unionRoot(unionUInt(5))},
		{"a document element of a named union, the date member", unionRootSchema, "", "2000-01-01", into[unionRoot](unionVariants...), unionRoot(unionUDate(date(2000, 1, 1)))},
		{"a document element of a list of a union, items of both members", unionListRootSchema, "", "5 2000-01-01", into[unionListRoot](unionVariants...),
			unionListRoot{unionUInt(5), unionUDate(date(2000, 1, 1))}},
		{"a choice's union alternative", unionChoiceSchema, "", "<s:a>8</s:a>", into[unionChoice](unionVariants...), unionChoice(unionChoiceA{A: unionUInt(8)})},
		{"a choice's xs:string alternative beside it", unionChoiceSchema, "", "<s:b>8</s:b>", into[unionChoice](unionVariants...), unionChoice(unionChoiceB{B: "8"})},
		{"a list member of a union of a list of a union, its items of both inner members", unionListMemberRoot, "", " 5  2000-01-01 ", into[unionLM](unionVariants...),
			unionLM(unionLMUnionList{unionUInt(5), unionUDate(date(2000, 1, 1))})},
		{"a list member of a union beside xs:date, which takes a date first", unionListMemberRoot, "", "2000-01-01", into[unionLM](unionVariants...),
			unionLM(unionLMUnionDate(date(2000, 1, 1)))},
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
			if g, w := spellUnion(reflect.ValueOf(&got).Elem()), spellUnion(reflect.ValueOf(&row.want).Elem()); g != w {
				t.Errorf("Decode = %s\nwant     %s", g, w)
			}
			if err := row.into.check(schema); err != nil {
				t.Errorf("CheckDecode = %v, want nil for a row Decode decodes", err)
			}
		})
	}
}

// spellUnion spells v for a comparison, naming each variant: an interface as
// nil or its dynamic type's name and value, a struct as {Field:value ...}, a
// struct over big.Int or big.Rat by its string, a pointer as nil or its
// target, and a slice as [items].
func spellUnion(v reflect.Value) string {
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return "nil"
		}
		return v.Elem().Type().Name() + "(" + spellUnion(v.Elem()) + ")"
	case reflect.Struct:
		if v.Type().ConvertibleTo(reflect.TypeFor[big.Int]()) {
			n := v.Convert(reflect.TypeFor[big.Int]()).Interface().(big.Int)
			return n.String()
		}
		if v.Type().ConvertibleTo(reflect.TypeFor[big.Rat]()) {
			r := v.Convert(reflect.TypeFor[big.Rat]()).Interface().(big.Rat)
			return r.RatString()
		}
		parts := make([]string, v.NumField())
		for i := range parts {
			parts[i] = v.Type().Field(i).Name + ":" + spellUnion(v.Field(i))
		}
		return "{" + strings.Join(parts, " ") + "}"
	case reflect.Pointer:
		if v.IsNil() {
			return "nil"
		}
		return spellUnion(v.Elem())
	case reflect.Slice:
		parts := make([]string, v.Len())
		for i := range parts {
			parts[i] = spellUnion(v.Index(i))
		}
		return fmt.Sprintf("%s[%s]", v.Type().Name(), strings.Join(parts, " "))
	default:
		return fmt.Sprint(v.Interface())
	}
}

// TestDecodeUnionMismatches pins item 6 for a union's interface: its variants
// reach Decode through WithBranches alone, and the set is ErrTypeMismatch
// when no WithBranches supplies it, its count differs, it holds a nil value,
// a type twice, or a variant over the Go type of no basic member an earlier
// variant did not match; and T itself is no interface of one unexported
// method. unionUOther and unionUStray implement U beside its two variants,
// over xs:int's Go type and over string. CheckDecode returns Decode's error.
func TestDecodeUnionMismatches(t *testing.T) {
	rows := []struct {
		name string
		into target
	}{
		{"no WithBranches", into[unionRoot]()},
		{"a missing variant", into[unionRoot](WithBranches[unionU](unionUInt(0)))},
		{"an extra variant", into[unionRoot](WithBranches[unionU](unionUInt(0), unionUDate{}, unionUOther(0)))},
		{"a nil variant", into[unionRoot](WithBranches[unionU](unionUInt(0), nil))},
		{"one variant twice", into[unionRoot](WithBranches[unionU](unionUInt(0), unionUInt(1)))},
		{"two variants over xs:int's Go type", into[unionRoot](WithBranches[unionU](unionUInt(0), unionUOther(0)))},
		{"a variant over the Go type of no member", into[unionRoot](WithBranches[unionU](unionUInt(0), unionUStray("")))},
		{"T no interface", into[int32](unionVariants...)},
		{"T an interface of an exported method", into[interface{ IsU() }](unionVariants...)},
	}
	schema := schemaOf(t, unionRootSchema)
	data := instanceOf("", "5")
	mustAccept(t, schema, data)
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			_, err := row.into.decode(schema, data)
			if !errors.Is(err, ErrTypeMismatch) {
				t.Fatalf("Decode error = %v, want ErrTypeMismatch", err)
			}
			assertProbeMismatch(t, row.into.check(schema), err)
		})
	}
}

// TestDecodeUnionDeclines pins item 5 for a union, CheckDecode's decline for
// each row: the union declines by its first declining basic member's
// construct in basic-member order, xs:int before and after the minted
// primitive (codegen/doc.go "# Simple types"), the member examined before any
// two members' Go types are compared; then by two basic members of one Go
// type, constructSharedVariant; then by an assertions facet on the union,
// on a member at any depth, an intervening union's included, or on a list
// member's item type, constructUnionAssertions. Each decline names the union
// as its declaration's {type definition} and, for a member's, that member.
func TestDecodeUnionDeclines(t *testing.T) {
	union := func(members string) string {
		return sequenceOf(`<xs:element name="a"><xs:simpleType><xs:union memberTypes="` + members + `"/></xs:simpleType></xs:element>`)
	}
	unionOf := func(memberTypes, inline string) string {
		return sequenceOf(`<xs:element name="a"><xs:simpleType><xs:union memberTypes="` + memberTypes + `">` + inline + `</xs:union></xs:simpleType></xs:element>`)
	}
	const assertion = `<xs:simpleType><xs:restriction base="xs:string"><xs:assertion test="true()"/></xs:restriction></xs:simpleType>`
	rows := []struct {
		name, decls string
		minted      bool
		construct   construct
		detail      string
	}{
		{"a minted primitive after xs:int", union("xs:int s:money"), true, constructOtherPrimitive, "through the union's basic member {urn:s}money"},
		{"a minted primitive before xs:int", union("s:money xs:int"), true, constructOtherPrimitive, "through the union's basic member {urn:s}money"},
		{"a minted primitive after two members of one Go type", union("xs:string xs:token s:money"), true, constructOtherPrimitive, "through the union's basic member {urn:s}money"},
		{"a list member of a minted primitive", unionOf("xs:int", `<xs:simpleType><xs:list itemType="s:money"/></xs:simpleType>`), true, constructOtherPrimitive,
			"through the union's anonymous basic member, a type defined at s.xsd:1:298, through the list type's {item type definition} {urn:s}money"},
		{"xs:string and xs:token", union("xs:string xs:token"), false, constructSharedVariant, "through the union's basic member {http://www.w3.org/2001/XMLSchema}token"},
		{"xs:string and an anonymous enumeration of it", unionOf("xs:string", `<xs:simpleType><xs:restriction base="xs:string"><xs:enumeration value="x"/></xs:restriction></xs:simpleType>`), false, constructSharedVariant, "through the union's anonymous basic member"},
		{"xs:int and a restriction of it", unionOf("xs:int", `<xs:simpleType><xs:restriction base="xs:int"><xs:minInclusive value="0"/></xs:restriction></xs:simpleType>`), false, constructSharedVariant, "through the union's anonymous basic member"},
		{"xs:QName and xs:NOTATION", union("xs:QName xs:NOTATION"), false, constructSharedVariant, "through the union's basic member {http://www.w3.org/2001/XMLSchema}NOTATION"},
		{"xs:hexBinary and xs:base64Binary", union("xs:hexBinary xs:base64Binary"), false, constructSharedVariant, "through the union's basic member {http://www.w3.org/2001/XMLSchema}base64Binary"},
		{"xs:integer and xs:nonNegativeInteger", union("xs:integer xs:nonNegativeInteger"), false, constructSharedVariant, "through the union's basic member {http://www.w3.org/2001/XMLSchema}nonNegativeInteger"},
		{"two lists of xs:int", unionOf("", `<xs:simpleType><xs:list itemType="xs:int"/></xs:simpleType><xs:simpleType><xs:list itemType="xs:int"/></xs:simpleType>`), false, constructSharedVariant, "through the union's anonymous basic member, a type defined at s.xsd:1:351"},
		{"two lists of a union", unionOf("", `<xs:simpleType><xs:list><xs:simpleType><xs:union memberTypes="xs:int"/></xs:simpleType></xs:list></xs:simpleType><xs:simpleType><xs:list><xs:simpleType><xs:union memberTypes="xs:date"/></xs:simpleType></xs:list></xs:simpleType>`),
			false, constructSharedVariant, "through the union's anonymous basic member"},
		{"members of one Go type before an assertions facet", unionOf("xs:string xs:token", assertion), false, constructSharedVariant, "through the union's basic member {http://www.w3.org/2001/XMLSchema}token"},
		{"an assertions facet on a member", unionOf("xs:int", assertion), false, constructUnionAssertions, ""},
		{"an assertions facet on the union itself", `<xs:simpleType name="v"><xs:union memberTypes="xs:int xs:date"/></xs:simpleType>` +
			sequenceOf(`<xs:element name="a"><xs:simpleType><xs:restriction base="s:v"><xs:assertion test="true()"/></xs:restriction></xs:simpleType></xs:element>`), false, constructUnionAssertions, ""},
		{"an assertions facet on an intervening union", `<xs:simpleType name="v"><xs:restriction><xs:simpleType><xs:union memberTypes="xs:int"/></xs:simpleType><xs:assertion test="true()"/></xs:restriction></xs:simpleType>` +
			union("s:v xs:date"), false, constructUnionAssertions, ""},
		{"an assertions facet on a list member's item type", unionOf("xs:date", `<xs:simpleType><xs:list>`+assertion+`</xs:list></xs:simpleType>`), false, constructUnionAssertions, ""},
		{"an assertions facet on a list member's union item's member", unionOf("xs:date", `<xs:simpleType><xs:list><xs:simpleType><xs:union memberTypes="xs:int">`+assertion+`</xs:union></xs:simpleType></xs:list></xs:simpleType>`),
			false, constructUnionAssertions, "through the union's anonymous basic member"},
		{"an assertions facet on a list's union item", sequenceOf(`<xs:element name="a"><xs:simpleType><xs:list><xs:simpleType><xs:union memberTypes="xs:int">` + assertion + `</xs:union></xs:simpleType></xs:list></xs:simpleType></xs:element>`),
			false, constructUnionAssertions, "through the list type's anonymous {item type definition}"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			build := schemaOf
			if row.minted {
				build = mintedSchema
			}
			err := CheckDecode[struct{}](build(t, row.decls), rootName)
			var de *DeclineError
			if !errors.As(err, &de) || de.Construct() != (Construct{row.construct}) {
				t.Fatalf("CheckDecode = %v, want the %s decline", err, row.construct.name())
			}
			if want := "codec does not decode " + row.construct.name() + ": element {urn:s}a declared at "; !strings.HasPrefix(err.Error(), want) || !strings.Contains(err.Error(), row.detail) {
				t.Errorf("CheckDecode = %q, want one opening with %q and holding %q", err, want, row.detail)
			}
		})
	}
}

// TestDecodeUnionMembersOfDistinctGoTypes pins the other half of
// constructSharedVariant over its condition: a union of any two of the
// fourteen datatypes whose Go type is a datatype struct of its own, xs:int
// beside a list of xs:int, a list of xs:int beside a list of xs:short, and
// xs:int beside itself, which basic-member order keeps once, decline
// nothing, so CheckDecode of struct{} passes item 5 to item 6's
// ErrTypeMismatch.
func TestDecodeUnionMembersOfDistinctGoTypes(t *testing.T) {
	datatypes := []string{"dateTime", "dateTimeStamp", "time", "date", "gYearMonth", "gYear", "gMonthDay", "gDay", "gMonth",
		"duration", "dayTimeDuration", "yearMonthDuration", "QName", "precisionDecimal"}
	var rows []string
	for i, a := range datatypes {
		for _, b := range datatypes[i+1:] {
			rows = append(rows, `memberTypes="xs:`+a+` xs:`+b+`">`)
		}
	}
	rows = append(rows,
		`memberTypes="xs:int"><xs:simpleType><xs:list itemType="xs:int"/></xs:simpleType>`,
		`><xs:simpleType><xs:list itemType="xs:int"/></xs:simpleType><xs:simpleType><xs:list itemType="xs:short"/></xs:simpleType>`,
		`memberTypes="xs:int xs:int">`)
	for _, row := range rows {
		schema := schemaOf(t, sequenceOf(`<xs:element name="a"><xs:simpleType><xs:union `+row+`</xs:union></xs:simpleType></xs:element>`))
		if err := CheckDecode[struct{}](schema, rootName); !errors.Is(err, ErrTypeMismatch) {
			t.Errorf("union %s: CheckDecode = %v, want ErrTypeMismatch, no decline", row, err)
		}
	}
}

// TestDecodeUnionChoiceSharedBranch pins sharesBranch's union bullet over two
// alternatives of one Go field name, x and X: beside another union, a
// complex type codegen gives a bare sealed interface, or a ref to a head of a
// substitution group, each holding an interface, they decline as two branches
// of one shape; beside codegen's struct, or an xs:int, they decline nothing,
// so CheckDecode of struct{} reaches item 6's ErrTypeMismatch.
func TestDecodeUnionChoiceSharedBranch(t *testing.T) {
	const decls = `<xs:complexType name="ci"><xs:choice><xs:element name="p" type="xs:string"/><xs:element name="q" type="xs:int"/></xs:choice></xs:complexType>
<xs:element name="X" type="xs:string"/><xs:element name="m" type="xs:string" substitutionGroup="s:X"/>`
	rows := []struct {
		name, other string
		declines    bool
	}{
		{"another union", `<xs:element name="X" form="unqualified" type="s:num"/>`, true},
		{"a bare sealed interface", `<xs:element name="X" form="unqualified" type="s:ci"/>`, true},
		{"a substitution group's interface", `<xs:element ref="s:X"/>`, true},
		{"codegen's struct", vStruct("X", ` form="unqualified"`), false},
		{"an xs:int", `<xs:element name="X" form="unqualified" type="xs:int"/>`, false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			schema := schemaOf(t, unionTypes+decls+root(`<xs:choice><xs:element name="x" type="s:u"/>`+row.other+`</xs:choice>`))
			err := CheckDecode[struct{}](schema, rootName)
			var de *DeclineError
			if declined := errors.As(err, &de) && de.Construct() == (Construct{constructSharedBranch}); declined != row.declines {
				t.Errorf("CheckDecode = %v, want the %s decline %t", err, constructSharedBranch.name(), row.declines)
			}
			if !row.declines && !errors.Is(err, ErrTypeMismatch) {
				t.Errorf("CheckDecode = %v, want ErrTypeMismatch", err)
			}
		})
	}
}

// TestDecodeUnionIdentificationError pins item 8's identification stage: a
// backend whose Parse rejects under codec's own bindings what validate
// accepted under its, so value.ValidatingType identifies no member in codec,
// returns a stage error naming the identification, the union, and the value,
// never a later member's variant.
func TestDecodeUnionIdentificationError(t *testing.T) {
	schema := schemaOf(t, unionRootSchema)
	data := instanceOf("", "5")
	got, err := Decode[unionRoot](schema, codecContext, data, unionVariants...)
	if got != nil {
		t.Fatalf("Decode = %v, want no value", *got)
	}
	var se *stageError
	if !errors.As(err, &se) || se.stage != stageIdentification {
		t.Fatalf("Decode error = %v, want a stage error of %s", err, stageIdentification.name())
	}
	offset := len(instanceOf("", "")) - len("</s:r>")
	if want := fmt.Sprintf(`codec: ?:1:%d: the identification of the ·active basic member· failed on "5", a value of {urn:s}u in the character content of element {urn:s}r at byte offset %d: `, offset+1, offset); !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error = %q\nwant one opening %q", err, want)
	}
	var de *DeclineError
	var xe *xsderr.Error
	if errors.As(err, &de) || errors.As(err, &xe) || errors.Is(err, ErrTypeMismatch) {
		t.Errorf("error %v is a decline, an *xsderr.Error or ErrTypeMismatch", err)
	}
}

// TestWithLoggerUnion pins the traces of a union value: identification logs
// nothing, and the active basic member's stages log as that member's, its
// type named as a basic member of the union; a list of a union logs
// whiteSpace once, under the list, and each item's stages under its own
// member.
func TestWithLoggerUnion(t *testing.T) {
	const intMember, dateMember = "{http://www.w3.org/2001/XMLSchema}int, a basic member of ", "{http://www.w3.org/2001/XMLSchema}date, a basic member of "
	const list = "the anonymous simple type of element {urn:s}r declared at s.xsd:5:111"
	const item = "{urn:s}u, the item type of " + list
	rows := []struct {
		name, decls, body string
		into              func(*xsd.Schema, []byte, ...Option) error
		want              []traceRecord
	}{
		{"a union", unionRootSchema, " 5 ", decodeInto[unionRoot], []traceRecord{
			{"whiteSpace", map[string]string{"rule": "rf-whiteSpace", "type": intMember + "{urn:s}u", "input": " 5 "}},
			{"lexical mapping", map[string]string{"rule": "cvc-datatype-valid", "clause": "2.1", "type": intMember + "{urn:s}u", "input": "5"}},
			{"conversion", map[string]string{"rule": "dt-canonical-mapping", "type": intMember + "{urn:s}u", "input": "5"}},
		}},
		{"a list of a union", unionListRootSchema, "5 2000-01-01", decodeInto[unionListRoot], []traceRecord{
			{"whiteSpace", map[string]string{"rule": "rf-whiteSpace", "type": list, "input": "5 2000-01-01"}},
			{"lexical mapping", map[string]string{"rule": "cvc-datatype-valid", "clause": "2.1", "type": intMember + item, "input": "5"}},
			{"conversion", map[string]string{"rule": "dt-canonical-mapping", "type": intMember + item, "input": "5"}},
			{"lexical mapping", map[string]string{"rule": "cvc-datatype-valid", "clause": "2.1", "type": dateMember + item, "input": "2000-01-01"}},
			{"conversion", map[string]string{"rule": "dt-canonical-mapping", "type": dateMember + item, "input": "2000-01-01"}},
		}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			schema := schemaOf(t, row.decls)
			data := instanceOf("", row.body)
			var buf bytes.Buffer
			opts := append(append([]Option(nil), unionVariants...), WithLogger(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))))
			if err := row.into(schema, data, opts...); err != nil {
				t.Fatal(err)
			}
			var got []traceRecord
			for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
				var tr traceRecord
				if err := json.Unmarshal(line, &tr); err != nil {
					t.Fatalf("record %s: %v", line, err)
				}
				if tr.Codec != nil {
					delete(tr.Codec, "loc")
					got = append(got, tr)
				}
			}
			if !reflect.DeepEqual(got, row.want) {
				t.Errorf("codec records = %+v\nwant %+v", got, row.want)
			}
		})
	}
}

// decodeInto decodes data against schema on builtin/strict into T under opts
// and returns Decode's error.
func decodeInto[T any](schema *xsd.Schema, data []byte, opts ...Option) error {
	_, err := Decode[T](schema, strict.New(), data, opts...)
	return err
}
