package codec

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/validate/xmlsrc"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// root declares s:r of an anonymous complex type whose content is body.
func root(body string) string {
	return `<xs:element name="r"><xs:complexType>` + body + `</xs:complexType></xs:element>`
}

// sequenceOf is a root whose content is a sequence of particles.
func sequenceOf(particles string) string {
	return root(`<xs:sequence>` + particles + `</xs:sequence>`)
}

// vStruct is a local element declaration named name, its attributes attrs,
// of an anonymous complex type whose content is a sequence of v, an
// xs:string, so codegen gives it a struct of one field V string.
func vStruct(name, attrs string) string {
	return `<xs:element name="` + name + `"` + attrs + `><xs:complexType><xs:sequence><xs:element name="v" type="xs:string"/></xs:sequence></xs:complexType></xs:element>`
}

// TestDecodeDeclines pins one row per construct Decode declines: each
// schema and instance is valid, so validation accepts, and Decode returns
// the construct's *DeclineError at the declining element's instance Loc,
// wrapping errors.ErrUnsupported, and no value. Each row decodes into a
// struct with no field, so a decline is never reported as the T check's
// ErrTypeMismatch: the declines precede it.
func TestDecodeDeclines(t *testing.T) {
	const a = `<s:a>x</s:a>`
	rows := []struct {
		name      string
		decls     string
		attrs     string
		content   string
		construct construct
		line, col int
	}{
		{"type table", `<xs:complexType name="rt"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>
			<xs:element name="r" type="s:rt"><xs:alternative test="@k" type="s:rt"/></xs:element>`, "", a, constructTypeTable, 1, 1},
		{"nillable", `<xs:element name="r" nillable="true"><xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType></xs:element>`,
			"", a, constructNillable, 1, 1},
		{"nillable local element with a fixed value", sequenceOf(`<xs:element name="a" type="xs:string" nillable="true" fixed="x"/>`), "", `<s:a>x</s:a>`, constructNillable, 1, 1},
		{"document element of mixed content with a default", `<xs:element name="r" default="d"><xs:complexType mixed="true"><xs:sequence><xs:element name="a" type="xs:string" minOccurs="0"/></xs:sequence></xs:complexType></xs:element>`,
			"", "", constructMixedContent, 1, 1},
		{"simple root of a union of xs:string and xs:token", `<xs:element name="r"><xs:simpleType><xs:union memberTypes="xs:string xs:token"/></xs:simpleType></xs:element>`, "", "1.0", constructSharedVariant, 1, 1},
		{"simple root of a list of a union of xs:string and xs:token", `<xs:element name="r"><xs:simpleType><xs:list><xs:simpleType><xs:union memberTypes="xs:string xs:token"/></xs:simpleType></xs:list></xs:simpleType></xs:element>`, "", "1.0 2.0", constructSharedVariant, 1, 1},
		{"document element of an abstract complex type of empty content, under xsi:type", `<xs:complexType name="ab" abstract="true"/>
			<xs:complexType name="cc"><xs:complexContent><xs:extension base="s:ab"><xs:attribute name="k" type="xs:string"/></xs:extension></xs:complexContent></xs:complexType>
			<xs:element name="r" type="s:ab"/>`, ` xsi:type="s:cc"`, "", constructAbstractTypeElement, 1, 1},
		{"document element of an abstract complex type of element-only content, under xsi:type", `<xs:complexType name="ab" abstract="true"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>
			<xs:complexType name="cc"><xs:complexContent><xs:extension base="s:ab"/></xs:complexContent></xs:complexType>
			<xs:element name="r" type="s:ab"/>`, ` xsi:type="s:cc"`, `<s:a>x</s:a>`, constructAbstractTypeElement, 1, 1},
		{"simple content of a special type", root(`<xs:simpleContent><xs:extension base="xs:anySimpleType"/></xs:simpleContent>`), "", "x", constructSpecial, 1, 1},
		{"simple content of a union of xs:string and xs:token", root(`<xs:simpleContent><xs:extension base="s:st"/></xs:simpleContent>`) +
			`<xs:simpleType name="st"><xs:union memberTypes="xs:string xs:token"/></xs:simpleType>`, "", "x", constructSharedVariant, 1, 1},
		{"mixed content", `<xs:element name="r"><xs:complexType mixed="true"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType></xs:element>`,
			"", a, constructMixedContent, 1, 1},
		{"open content", root(`<xs:openContent><xs:any namespace="##other" processContents="lax"/></xs:openContent><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence>`),
			"", a, constructOpenContent, 1, 1},
		{"a <group ref> content particle to an all group", `<xs:group name="g"><xs:all><xs:element name="a" type="xs:string"/></xs:all></xs:group>` + root(`<xs:group ref="s:g"/>`),
			"", a, constructAllContent, 1, 1},
		{"a <group ref> content particle to a sequence group, occurring twice", `<xs:group name="g"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:group>` + root(`<xs:group ref="s:g" maxOccurs="2"/>`),
			"", a, constructSequenceOccurs, 1, 1},
		{"an optional <group ref> content particle to a sequence group", `<xs:group name="g"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:group>` + root(`<xs:group ref="s:g" minOccurs="0"/>`),
			"", a, constructSequenceOccurs, 1, 1},
		{"a <group ref> content particle to a choice group, occurring twice", `<xs:group name="g"><xs:choice><xs:element name="a" type="xs:string"/></xs:choice></xs:group>` + root(`<xs:group ref="s:g" maxOccurs="2"/>`),
			"", a, constructChoiceOccurs, 1, 1},
		{"content choice occurring twice", root(`<xs:choice maxOccurs="2"><xs:element name="a" type="xs:string"/></xs:choice>`), "", a, constructChoiceOccurs, 1, 1},
		{"optional content choice", root(`<xs:choice minOccurs="0"><xs:element name="a" type="xs:string"/></xs:choice>`), "", a, constructChoiceOccurs, 1, 1},
		{"choice alternative of a sequence", root(`<xs:choice><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence><xs:element name="b" type="xs:string"/></xs:choice>`),
			"", a, constructNestedGroup, 1, 1},
		{"choice alternatives sharing a branch shape, refs to a and A", `<xs:element name="a" type="xs:string"/><xs:element name="A" type="xs:string"/>` + root(`<xs:choice><xs:element ref="s:a"/><xs:element ref="s:A"/></xs:choice>`),
			"", a, constructSharedBranch, 1, 1},
		{"choice alternative of a special type", root(`<xs:choice><xs:element name="a" type="xs:string"/><xs:element name="b" type="xs:anySimpleType"/></xs:choice>`),
			"", a, constructSpecial, 1, 1},
		{"choice alternatives sharing a branch shape, qualified and not", root(`<xs:choice><xs:element name="x" type="xs:string"/><xs:element name="x" form="unqualified" type="xs:string"/></xs:choice>`),
			"", `<s:x>v</s:x>`, constructSharedBranch, 1, 1},
		{"choice alternatives sharing a branch shape, a repeated xs:int and a list of xs:int", root(`<xs:choice><xs:element name="x" type="xs:int" maxOccurs="unbounded"/><xs:element name="X"><xs:simpleType><xs:list itemType="xs:int"/></xs:simpleType></xs:element></xs:choice>`),
			"", `<s:x>1</s:x>`, constructSharedBranch, 1, 1},
		{"choice alternatives sharing a branch shape, a repeated xs:hexBinary and a list of xs:hexBinary", root(`<xs:choice><xs:element name="x" type="xs:hexBinary" maxOccurs="unbounded"/><xs:element name="X"><xs:simpleType><xs:list itemType="xs:hexBinary"/></xs:simpleType></xs:element></xs:choice>`),
			"", `<s:x>0A</s:x>`, constructSharedBranch, 1, 1},
		{"choice alternatives sharing a branch shape, an xs:hexBinary and an xs:base64Binary", root(`<xs:choice><xs:element name="x" type="xs:hexBinary"/><xs:element name="X" type="xs:base64Binary"/></xs:choice>`),
			"", `<s:x>0A</s:x>`, constructSharedBranch, 1, 1},
		{"choice alternatives sharing a branch shape, a repeated xs:unsignedByte and an xs:hexBinary", root(`<xs:choice><xs:element name="x" type="xs:unsignedByte" maxOccurs="unbounded"/><xs:element name="X" type="xs:hexBinary"/></xs:choice>`),
			"", `<s:x>1</s:x>`, constructSharedBranch, 1, 1},
		{"choice alternatives sharing a branch shape, two lists of xs:int", root(`<xs:choice><xs:element name="x"><xs:simpleType><xs:list itemType="xs:int"/></xs:simpleType></xs:element><xs:element name="X"><xs:simpleType><xs:list itemType="xs:int"/></xs:simpleType></xs:element></xs:choice>`),
			"", `<s:x>1</s:x>`, constructSharedBranch, 1, 1},
		{"choice alternatives sharing a branch shape, a repeated xs:int and a ref to a list of xs:int", `<xs:element name="X"><xs:simpleType><xs:list itemType="xs:int"/></xs:simpleType></xs:element>` + root(`<xs:choice><xs:element name="x" type="xs:int" maxOccurs="unbounded"/><xs:element ref="s:X"/></xs:choice>`),
			"", `<s:X>1 2</s:X>`, constructSharedBranch, 1, 1},
		{"choice alternatives sharing a branch shape, a ref to a list of xs:int and a repeated xs:int", `<xs:element name="X"><xs:simpleType><xs:list itemType="xs:int"/></xs:simpleType></xs:element>` + root(`<xs:choice><xs:element ref="s:X"/><xs:element name="x" type="xs:int" maxOccurs="unbounded"/></xs:choice>`),
			"", `<s:X>1 2</s:X>`, constructSharedBranch, 1, 1},
		{"choice alternatives sharing a branch shape, a ref to an xs:date element and a local xs:date", `<xs:element name="d" type="xs:date"/>` + root(`<xs:choice><xs:element ref="s:d"/><xs:element name="d" form="unqualified" type="xs:date"/></xs:choice>`),
			"", `<s:d>2000-01-01</s:d>`, constructSharedBranch, 1, 1},
		{"choice alternatives sharing a branch shape, a and A", root(`<xs:choice><xs:element name="a" type="xs:string"/><xs:element name="A" type="xs:token"/></xs:choice>`),
			"", a, constructSharedBranch, 1, 1},
		{"all content", root(`<xs:all><xs:element name="a" type="xs:string"/></xs:all>`), "", a, constructAllContent, 1, 1},
		{"content sequence occurring twice", root(`<xs:sequence maxOccurs="2"><xs:element name="a" type="xs:string"/></xs:sequence>`), "", a, constructSequenceOccurs, 1, 1},
		{"nested model group", sequenceOf(`<xs:choice><xs:element name="a" type="xs:string"/></xs:choice>`), "", a, constructNestedGroup, 1, 1},
		{"nested <group ref>", `<xs:group name="g"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:group>` + sequenceOf(`<xs:group ref="s:g"/>`),
			"", a, constructNestedGroup, 1, 1},
		{"a second wildcard particle of a sequence", sequenceOf(`<xs:any namespace="##local" processContents="lax"/><xs:element name="a" type="xs:string"/><xs:any namespace="##other" processContents="lax"/>`),
			"", `<w/>` + a + `<o:w xmlns:o="urn:o"/>`, constructWildcardParticle, 1, 1},
		{"a second wildcard alternative of a choice, codegen's either", root(`<xs:choice><xs:any namespace="##local"/><xs:any namespace="##other" processContents="lax"/></xs:choice>`),
			"", `<o:w xmlns:o="urn:o"/>`, constructWildcardParticle, 1, 1},
		{"a wildcard particle in mixed content", `<xs:element name="r"><xs:complexType mixed="true"><xs:sequence><xs:any processContents="lax"/></xs:sequence></xs:complexType></xs:element>`,
			"", "t" + a, constructMixedContent, 1, 1},
		{"a wildcard particle beside an attribute wildcard", root(`<xs:sequence><xs:any processContents="lax"/></xs:sequence><xs:anyAttribute processContents="lax"/>`),
			"", a, constructAttributeWildcard, 1, 1},
		{"element ref to a head whose group holds a nillable member", `<xs:element name="a" type="xs:string"/><xs:element name="m" type="xs:string" nillable="true" substitutionGroup="s:a"/>` + sequenceOf(`<xs:element ref="s:a"/>`),
			"", a, constructNillable, 1, 1},
		{"element ref to a group of two declining members, the nillable one first", `<xs:complexType name="ct" mixed="true"><xs:sequence/></xs:complexType><xs:element name="h" type="s:ct" abstract="true"/>
			<xs:element name="m" type="s:ct" nillable="true" substitutionGroup="s:h"/><xs:element name="n" type="s:ct" substitutionGroup="s:h"/>` + sequenceOf(`<xs:element ref="s:h"/>`),
			"", `<s:n/>`, constructNillable, 1, 1},
		{"element ref to a group of two declining members, the one of mixed content first", `<xs:complexType name="ct" mixed="true"><xs:sequence/></xs:complexType><xs:element name="h" type="s:ct" abstract="true"/>
			<xs:element name="n" type="s:ct" substitutionGroup="s:h"/><xs:element name="m" type="s:ct" nillable="true" substitutionGroup="s:h"/>` + sequenceOf(`<xs:element ref="s:h"/>`),
			"", `<s:n/>`, constructMixedContent, 1, 1},
		{"element ref to a group whose members share a branch shape, a and A", `<xs:element name="h" type="xs:string" abstract="true"/>
			<xs:element name="a" type="xs:string" substitutionGroup="s:h"/><xs:element name="A" type="xs:string" substitutionGroup="s:h"/>` + sequenceOf(`<xs:element ref="s:h"/>`),
			"", a, constructSharedBranch, 1, 1},
		{"a local particle sharing the expanded name of an earlier ref's member", `<xs:element name="g" type="xs:string"/><xs:element name="m" type="xs:string" substitutionGroup="s:g"/>` +
			sequenceOf(`<xs:element ref="s:g"/><xs:element name="m" type="xs:string"/>`), "", `<s:g>x</s:g><s:m>y</s:m>`, constructSharedName, 1, 1},
		{"a ref's member sharing the expanded name of an earlier local particle", `<xs:element name="g" type="xs:string"/><xs:element name="m" type="xs:string" substitutionGroup="s:g"/>` +
			sequenceOf(`<xs:element name="m" type="xs:string"/><xs:element ref="s:g"/>`), "", `<s:m>x</s:m><s:g>y</s:g>`, constructSharedName, 1, 1},
		{"a ref to a member of an earlier ref's group", `<xs:element name="g" type="xs:string"/><xs:element name="m" type="xs:string" substitutionGroup="s:g"/>` +
			sequenceOf(`<xs:element ref="s:g"/><xs:element ref="s:m"/>`), "", `<s:g>x</s:g><s:m>y</s:m>`, constructSharedName, 1, 1},
		{"two refs whose groups share a member, s2_2_2v01's shape", `<xs:element name="e0" type="xs:string"/><xs:element name="e1" type="xs:string"/>
			<xs:element name="e2" type="xs:string" abstract="true" substitutionGroup="s:e0 s:e1"/><xs:element name="e3" type="xs:string" substitutionGroup="s:e2"/>` +
			sequenceOf(`<xs:element ref="s:e0"/><xs:element ref="s:e1"/>`), "", `<s:e3>x</s:e3><s:e3>y</s:e3>`, constructSharedName, 1, 1},
		{"element ref to a nillable head", `<xs:element name="h" type="xs:string" nillable="true"/>` + sequenceOf(`<xs:element ref="s:h"/>`), "", `<s:h>x</s:h>`, constructNillable, 1, 1},
		{"element ref to a head of a special type", `<xs:element name="h" type="xs:anySimpleType"/>` + sequenceOf(`<xs:element ref="s:h"/>`), "", `<s:h>1.0</s:h>`, constructSpecial, 1, 1},
		{"element ref sharing a local particle's expanded name", `<xs:element name="a" type="xs:string"/>` + sequenceOf(`<xs:element name="a" type="xs:string"/><xs:element ref="s:a"/>`),
			"", a + a, constructSharedName, 1, 1},
		{"local element of a mixed restriction of xs:anyType, which declines by its own content", `<xs:complexType name="dt" mixed="true"><xs:complexContent><xs:restriction base="xs:anyType"><xs:sequence><xs:element name="b" type="xs:string" minOccurs="0"/></xs:sequence></xs:restriction></xs:complexContent></xs:complexType>` +
			sequenceOf(`<xs:element name="a" type="s:dt"/>`), "", `<s:a>t</s:a>`, constructMixedContent, 1, 1},
		{"local element of emptiable mixed content with a default", sequenceOf(`<xs:element name="a" default="d"><xs:complexType mixed="true"><xs:sequence><xs:element name="b" type="xs:string" minOccurs="0"/></xs:sequence></xs:complexType></xs:element>`),
			"", `<s:a/>`, constructMixedContent, 1, 1},
		{"local element of simple content of a special type, ahead of its attribute uses", sequenceOf(`<xs:element name="a"><xs:complexType><xs:simpleContent><xs:extension base="xs:anyAtomicType"><xs:attribute name="k"><xs:simpleType><xs:union memberTypes="xs:string xs:token"/></xs:simpleType></xs:attribute><xs:anyAttribute processContents="lax"/></xs:extension></xs:simpleContent></xs:complexType></xs:element>`),
			"", a, constructSpecial, 1, 1},
		{"local element of simple content, its attribute use of a special type", sequenceOf(`<xs:element name="a"><xs:complexType><xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="k" type="xs:anySimpleType"/></xs:extension></xs:simpleContent></xs:complexType></xs:element>`),
			"", a, constructSpecial, 1, 1},
		{"local element of simple content with an attribute wildcard", sequenceOf(`<xs:element name="a"><xs:complexType><xs:simpleContent><xs:extension base="xs:string"><xs:anyAttribute processContents="lax"/></xs:extension></xs:simpleContent></xs:complexType></xs:element>`),
			"", a, constructAttributeWildcard, 1, 1},
		{"local element of an abstract complex type", `<xs:complexType name="ab" abstract="true"><xs:sequence><xs:element name="x" type="xs:string"/></xs:sequence></xs:complexType>
			<xs:complexType name="cc"><xs:complexContent><xs:extension base="s:ab"/></xs:complexContent></xs:complexType>` + sequenceOf(`<xs:element name="a" type="s:ab" minOccurs="0"/>`),
			"", "", constructAbstractTypeElement, 1, 1},
		{"nillable local element of complex type", sequenceOf(`<xs:element name="a" nillable="true"><xs:complexType><xs:sequence><xs:element name="b" type="xs:string"/></xs:sequence></xs:complexType></xs:element>`),
			"", `<s:a><s:b>x</s:b></s:a>`, constructNillable, 1, 1},
		{"a nested type's particle of a union of xs:string and xs:token", sequenceOf(`<xs:element name="a"><xs:complexType><xs:sequence><xs:element name="b"><xs:simpleType><xs:union memberTypes="xs:string xs:token"/></xs:simpleType></xs:element></xs:sequence></xs:complexType></xs:element>`),
			"", `<s:a><s:b>1.0</s:b></s:a>`, constructSharedVariant, 1, 1},
		{"a nested type's attribute wildcard", sequenceOf(`<xs:element name="a"><xs:complexType><xs:sequence><xs:element name="b" type="xs:string"/></xs:sequence><xs:anyAttribute processContents="lax"/></xs:complexType></xs:element>`),
			"", `<s:a><s:b>x</s:b></s:a>`, constructAttributeWildcard, 1, 1},
		{"choice alternatives sharing a branch shape, two structs of one shape", root(`<xs:choice>` + vStruct("x", "") + vStruct("X", "") + `</xs:choice>`),
			"", `<s:x><s:v>v</s:v></s:x>`, constructSharedBranch, 1, 1},
		{"choice alternatives sharing a name, two structs of differing fields", root(`<xs:choice>` + vStruct("x", "") +
			`<xs:element name="X"><xs:complexType><xs:sequence><xs:element name="y" type="xs:int"/></xs:sequence></xs:complexType></xs:element></xs:choice>`),
			"", `<s:x><s:v>v</s:v></s:x>`, constructSharedBranch, 1, 1},
		{"choice alternatives sharing a name, of Cj and of Ck holding an optional Cj", `<xs:complexType name="Cj"><xs:sequence><xs:element name="v" type="xs:string"/></xs:sequence></xs:complexType>
			<xs:complexType name="Ck"><xs:sequence><xs:element name="y" type="s:Cj" minOccurs="0"/></xs:sequence></xs:complexType>` +
			root(`<xs:choice><xs:element name="x" type="s:Cj"/><xs:element name="X" type="s:Ck"/></xs:choice>`),
			"", `<s:x><s:v>v</s:v></s:x>`, constructSharedBranch, 1, 1},
		{"choice alternatives sharing a name, a choice's interface and a substitution group's", `<xs:complexType name="Cx"><xs:choice><xs:element name="a" type="xs:string"/><xs:element name="b" type="xs:int"/></xs:choice></xs:complexType>
			<xs:element name="X" type="xs:string"/><xs:element name="m" type="xs:string" substitutionGroup="s:X"/>` +
			root(`<xs:choice><xs:element name="x" type="s:Cx"/><xs:element ref="s:X"/></xs:choice>`),
			"", `<s:x><s:a>a</s:a></s:x>`, constructSharedBranch, 1, 1},
		{"choice alternatives sharing a name, a struct and an xs:date", root(`<xs:choice>` + vStruct("x", "") + `<xs:element name="X" type="xs:date"/></xs:choice>`),
			"", `<s:x><s:v>v</s:v></s:x>`, constructSharedBranch, 1, 1},
		{"choice alternatives sharing a name, a struct and an xs:precisionDecimal", root(`<xs:choice>` + vStruct("x", "") + `<xs:element name="X" type="xs:precisionDecimal"/></xs:choice>`),
			"", `<s:x><s:v>v</s:v></s:x>`, constructSharedBranch, 1, 1},
		{"choice alternatives sharing a name, a struct and an xs:decimal", root(`<xs:choice>` + vStruct("x", "") + `<xs:element name="X" type="xs:decimal"/></xs:choice>`),
			"", `<s:x><s:v>v</s:v></s:x>`, constructSharedBranch, 1, 1},
		{"choice alternatives sharing a name, a struct and a wildcard", root(`<xs:choice>` + vStruct("any", "") + `<xs:any namespace="##other" processContents="lax"/></xs:choice>`),
			"", `<s:any><s:v>v</s:v></s:any>`, constructSharedBranch, 1, 1},
		{"choice alternatives sharing a name, a wildcard and an xs:date", root(`<xs:choice><xs:any namespace="##other" processContents="lax"/><xs:element name="any" type="xs:date"/></xs:choice>`),
			"", `<s:any>2000-01-01</s:any>`, constructSharedBranch, 1, 1},
		{"particles sharing an expanded name", sequenceOf(`<xs:element name="a" type="xs:string"/><xs:element name="b" type="xs:string"/><xs:element name="a" type="xs:string"/>`),
			"", a + `<s:b>y</s:b>` + a, constructSharedName, 1, 1},
		{"list of a union of xs:string and xs:token", sequenceOf(`<xs:element name="a"><xs:simpleType><xs:list><xs:simpleType><xs:union memberTypes="xs:string xs:token"/></xs:simpleType></xs:list></xs:simpleType></xs:element>`), "", a, constructSharedVariant, 1, 1},
		{"attribute of a list of a union of xs:string and xs:token", root(`<xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence><xs:attribute name="b"><xs:simpleType><xs:list><xs:simpleType><xs:union memberTypes="xs:string xs:token"/></xs:simpleType></xs:list></xs:simpleType></xs:attribute>`),
			` b="1.0"`, a, constructSharedVariant, 1, 1},
		{"union of xs:string and xs:token", sequenceOf(`<xs:element name="a"><xs:simpleType><xs:union memberTypes="xs:string xs:token"/></xs:simpleType></xs:element>`), "", a, constructSharedVariant, 1, 1},
		{"xs:anySimpleType", sequenceOf(`<xs:element name="a" type="xs:anySimpleType"/>`), "", a, constructSpecial, 1, 1},
		{"xs:anyAtomicType", sequenceOf(`<xs:element name="a" type="xs:anyAtomicType"/>`), "", a, constructSpecial, 1, 1},
		{"attribute use's type", root(`<xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence><xs:attribute name="b" type="xs:anySimpleType"/>`),
			` b="1.0"`, a, constructSpecial, 1, 1},
		{"attribute use in the xsi namespace", `<xs:import namespace="http://www.w3.org/2001/XMLSchema-instance"/>` +
			root(`<xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence><xs:attribute ref="xsi:nil"/>`), "", a, constructXsiAttribute, 1, 1},
		{"attribute wildcard", root(`<xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence><xs:anyAttribute processContents="lax"/>`),
			"", a, constructAttributeWildcard, 1, 1},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			schema := schemaOf(t, row.decls)
			data := instanceOf(row.attrs, row.content)
			mustAccept(t, schema, data)
			got, err := Decode[struct{}](schema, strict.New(), data)
			if got != nil {
				t.Fatalf("Decode = %v, want no value", got)
			}
			assertDecline(t, err, row.construct, row.line, row.col)
			assertProbeDecline(t, CheckDecode[struct{}](schema, rootName), err)
		})
	}
	// C's alternatives e and E each hold C's own interface, unbound when the
	// second is read, and no finite instance validates, so CheckDecode alone
	// probes it: it returns, declining.
	selfPair := schemaOf(t, `<xs:complexType name="C"><xs:choice><xs:element name="e" type="s:C"/><xs:element name="E" type="s:C"/></xs:choice></xs:complexType>
		<xs:element name="r" type="s:C"/>`)
	var de *DeclineError
	if err := CheckDecode[struct{}](selfPair, rootName); !errors.As(err, &de) || de.Construct() != (Construct{constructSharedBranch}) {
		t.Errorf("CheckDecode(choice(e:C, E:C)) = %v, want the %s decline", err, constructSharedBranch.name())
	}
}

// assertProbeDecline fails the test unless probe, CheckDecode's error for a
// row, is decoded, Decode's item-5 decline for it, charged to no instance
// Loc: a *DeclineError of the same Construct whose Loc reports none, and
// whose text is decoded's less its opening instance Loc and ": ".
func assertProbeDecline(t *testing.T, probe, decoded error) {
	t.Helper()
	var pd, dd *DeclineError
	if !errors.As(probe, &pd) || !errors.As(decoded, &dd) {
		t.Fatalf("CheckDecode = %v, Decode = %v, want a *DeclineError from each", probe, decoded)
	}
	if pd.Construct() != dd.Construct() {
		t.Errorf("CheckDecode's Construct() = %s, want Decode's %s", pd.Construct(), dd.Construct())
	}
	if loc, charged := pd.Loc(); charged || loc != (xsderr.Loc{}) {
		t.Errorf("CheckDecode's Loc() = %s, %t, want the zero Loc and false", loc, charged)
	}
	loc, _ := dd.Loc()
	if want := loc.String() + ": " + probe.Error(); decoded.Error() != want {
		t.Errorf("Decode's error = %q, want its Loc, then CheckDecode's %q", decoded, probe)
	}
}

// TestCheckDecodePlainErrors pins items 1 to 3 of CheckDecode's doc: a nil
// schema, a name no top-level declaration has, the zero QName included, and
// an abstract declaration, a nillable one too, each a plain error, which is
// no decline, no ErrTypeMismatch, no errors.ErrUnsupported and no rule. T is
// codegen's struct for each declaration, so only the condition the row names
// answers.
func TestCheckDecodePlainErrors(t *testing.T) {
	const body = `<xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>`
	inSlice := schemaOf(t, `<xs:element name="r">`+body+`</xs:element>`)
	rows := []struct {
		name   string
		schema *xsd.Schema
		qname  xsd.QName
		want   string
	}{
		{"nil schema", nil, rootName, "codec: CheckDecode: nil *xsd.Schema"},
		{"an undeclared name", inSlice, xsd.QName{Space: "urn:s", Local: "x"},
			`codec: no top-level element declaration has the expanded name "{urn:s}x"`},
		{"the zero QName", inSlice, xsd.QName{}, `codec: no top-level element declaration has the expanded name ""`},
		{"an abstract declaration", schemaOf(t, `<xs:element name="r" abstract="true">`+body+`</xs:element>`), rootName,
			"codec: element {urn:s}r declared at s.xsd:1:181 has {abstract} true, so no document element validates against it (cvc-elt clause 2)"},
		{"an abstract nillable declaration", schemaOf(t, `<xs:element name="r" abstract="true" nillable="true">`+body+`</xs:element>`), rootName,
			"codec: element {urn:s}r declared at s.xsd:1:181 has {abstract} true, so no document element validates against it (cvc-elt clause 2)"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			err := CheckDecode[struct{ A string }](row.schema, row.qname)
			if err == nil || err.Error() != row.want {
				t.Fatalf("CheckDecode = %v, want %q", err, row.want)
			}
			var de *DeclineError
			if _, isRule := xsderr.RuleOf(err); isRule || errors.As(err, &de) || errors.Is(err, ErrTypeMismatch) || errors.Is(err, errors.ErrUnsupported) {
				t.Errorf("CheckDecode = %v, want a plain error", err)
			}
		})
	}
	if err := CheckDecode[struct{ A string }](inSlice, rootName); err != nil {
		t.Errorf("CheckDecode(the in-slice declaration) = %v, want nil", err)
	}
}

// TestCheckDecodeMaxOccursZero pins item 5's plain error for a particle with
// {max occurs} 0, an element particle's and a wildcard particle's: CheckDecode
// returns the text Decode returns for a valid instance, and neither is a
// decline, ErrTypeMismatch, errors.ErrUnsupported or a rule. No schema
// document yields such a particle (§3.3.2 and §3.10.2: an <element> or <any>
// with minOccurs=maxOccurs=0 maps to no component), so the schema is built
// from a parsed one with type v's one inner particle rebuilt through
// xsd.NewParticle with the vacuous Occurs{0,0}.
func TestCheckDecodeMaxOccursZero(t *testing.T) {
	rows := []struct{ name, particle, want string }{
		{"element", `<xs:element name="a" type="xs:string"/>`,
			"codec: the particle of element {urn:s}a declared at s.xsd:1:219 has {max occurs} 0, for which codegen emits no type"},
		{"wildcard", `<xs:any processContents="lax"/>`,
			"codec: the wildcard particle of the {type definition} of element {urn:s}r declared at s.xsd:1:281, a type defined at s.xsd:1:181 has {max occurs} 0, for which codegen emits no type"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) { checkMaxOccursZero(t, row.particle, row.want) })
	}
}

// checkMaxOccursZero is TestCheckDecodeMaxOccursZero's row: type v's one
// inner particle, as written spells it, rebuilt vacuous, and want the plain
// error Decode and CheckDecode each return.
func checkMaxOccursZero(t *testing.T, written, want string) {
	t.Helper()
	parsed := schemaOf(t, `<xs:complexType name="v"><xs:sequence>`+written+`</xs:sequence></xs:complexType><xs:element name="r" type="s:v"/>`)
	v := xsd.QName{Space: "urn:s", Local: "v"}
	declared, ok := parsed.Type(v)
	if !ok {
		t.Fatalf("the parsed schema declares no type %s", v)
	}
	content := declared.(xsd.ComplexType).ContentType().(xsd.ElementContent)
	inner := content.Particle.Term().(xsd.ResolvedTerm).Term.(xsd.ModelGroup).Particles()[0]
	none, err := xsd.NewOccurs(xsderr.Loc{}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	vacuous, err := xsd.NewParticle(xsderr.Loc{}, none, inner.Term())
	if err != nil {
		t.Fatal(err)
	}
	group, err := xsd.NewModelGroup(xsderr.Loc{}, xsd.CompositorSequence, []xsd.Particle{vacuous})
	if err != nil {
		t.Fatal(err)
	}
	particle, err := xsd.NewParticle(xsderr.Loc{}, content.Particle.Occurs(), xsd.ResolvedTerm{Term: group})
	if err != nil {
		t.Fatal(err)
	}
	ct, err := xsd.NewComplexType(declared.Loc(), v, xsd.QName{Space: xsd.XMLSchemaNS, Local: "anyType"}, nil, xsd.DerivationRestriction, false,
		nil, nil, nil, xsd.ElementContent{Particle: particle}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	schema := replacedSchema(t, parsed, ct)
	data := instanceOf("", "")
	mustAccept(t, schema, data)
	_, decoded := Decode[struct{}](schema, strict.New(), data)
	probe := CheckDecode[struct{}](schema, rootName)
	for _, got := range []struct {
		name string
		err  error
	}{{"Decode", decoded}, {"CheckDecode", probe}} {
		if got.err == nil || got.err.Error() != want {
			t.Fatalf("%s = %v, want %q", got.name, got.err, want)
		}
		var de *DeclineError
		if _, isRule := xsderr.RuleOf(got.err); isRule || errors.As(got.err, &de) || errors.Is(got.err, ErrTypeMismatch) || errors.Is(got.err, errors.ErrUnsupported) {
			t.Errorf("%s = %v, want a plain error", got.name, got.err)
		}
	}
}

// replacedSchema returns the schema of parsed's types and its element and
// attribute declarations, with replacement in place of the type of its name.
func replacedSchema(t *testing.T, parsed *xsd.Schema, replacement xsd.TypeDefinition) *xsd.Schema {
	t.Helper()
	b := xsd.NewSchemaBuilder()
	for _, td := range parsed.Types() {
		if td.Name() == replacement.Name() {
			b.AddType(replacement)
			continue
		}
		b.AddType(td)
	}
	for _, d := range parsed.Elements() {
		b.AddElement(d)
	}
	for _, d := range parsed.Attributes() {
		b.AddAttribute(d)
	}
	schema, err := b.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	return schema
}

// money is the name of the ·implementation-defined· primitive mintedSchema
// mints.
var money = xsd.QName{Space: "urn:s", Local: "money"}

// mintedSchema returns schemaOf's schema of decls and a simple type s:money,
// which decls may reference, with money replaced by an
// ·implementation-defined· primitive of that name (Datatypes dt-primitive)
// minted through xsd.NewPrimitiveType: a primitive Decode decodes none of, so
// a type of it declines as constructOtherPrimitive. No mapping of
// builtin/strict governs it, so Decode stops at item 4 and CheckDecode alone
// reaches the decline.
func mintedSchema(t *testing.T, decls string) *xsd.Schema {
	t.Helper()
	parsed := schemaOf(t, decls+`<xs:simpleType name="money"><xs:restriction base="xs:string"/></xs:simpleType>`)
	primitive, err := xsd.NewPrimitiveType(xsderr.Loc{}, money, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return replacedSchema(t, parsed, primitive)
}

// TestCheckDecodeDeclineBeforeMismatch pins item 4 of CheckDecode's doc
// before item 5: a declaration outside the slice, of a minted primitive,
// probed with a T that mismatches what codegen emits returns the decline.
func TestCheckDecodeDeclineBeforeMismatch(t *testing.T) {
	schema := mintedSchema(t, `<xs:element name="r" type="s:money"/>`)
	var de *DeclineError
	if err := CheckDecode[struct{ A int }](schema, rootName); !errors.As(err, &de) || de.Construct() != (Construct{constructOtherPrimitive}) {
		t.Errorf("CheckDecode = %v, want the decline of %s", err, constructOtherPrimitive.name())
	}
}

// TestDecodeXsiTypeDeclines pins the xsi:type decline (#3202's ruling,
// refused permanently where the selected type is concrete): an element
// carrying xsi:type, the document element or a child, declines at its own
// instance Loc after the T check, even where the xsi:type names the declared
// type itself. The child row is the one whose decoding into the base's field
// would lose the derived type's value: xs:token collapses what xs:string
// preserves.
func TestDecodeXsiTypeDeclines(t *testing.T) {
	schema := schemaOf(t, `<xs:complexType name="rt"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>
		<xs:element name="r" type="s:rt"/>`)
	rows := []struct {
		name      string
		attrs     string
		content   string
		line, col int
	}{
		{"the document element, naming its declared type", ` xsi:type="s:rt"`, `<s:a>x</s:a>`, 1, 1},
		{"a child, naming a type derived from its concrete one", "", `<s:a xsi:type="xs:token"> x  y </s:a>`, 1, 120},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			data := instanceOf(row.attrs, row.content)
			mustAccept(t, schema, data)
			got, err := Decode[struct{ A string }](schema, strict.New(), data)
			if got != nil {
				t.Fatalf("Decode = %v, want no value", got)
			}
			assertDecline(t, err, constructXsiType, row.line, row.col)
		})
	}
}

// TestDecodeSimpleRootXsiType pins the xsi:type decline on a document element
// of simple type: under xsi:type="xs:token" an xs:string root's value
// collapses, so it declines at its own Loc after the T check, which string
// passes, and holds no value.
func TestDecodeSimpleRootXsiType(t *testing.T) {
	schema := schemaOf(t, `<xs:element name="r" type="xs:string"/>`)
	data := instanceOf(` xsi:type="xs:token"`, ` x  y `)
	mustAccept(t, schema, data)
	got, err := Decode[string](schema, strict.New(), data)
	if got != nil {
		t.Fatalf("Decode = %q, want no value", *got)
	}
	assertDecline(t, err, constructXsiType, 1, 1)
}

// TestDecodeUndeclaredRootXsiType pins that a document element no top-level
// declaration declares, which validate accepts through its xsi:type alone
// (§3.3.4.6 cvc-assess-elt), declines as xsi:type at its own Loc.
func TestDecodeUndeclaredRootXsiType(t *testing.T) {
	schema := schemaOf(t, `<xs:complexType name="rt"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>`)
	data := instanceOf(` xsi:type="s:rt"`, `<s:a>x</s:a>`)
	mustAccept(t, schema, data)
	got, err := Decode[struct{ A string }](schema, strict.New(), data)
	if got != nil {
		t.Fatalf("Decode = %v, want no value", got)
	}
	assertDecline(t, err, constructXsiType, 1, 1)
}

// TestDeclineConstruct pins DeclineError.Construct's contract: two declines
// of one construct, from two schemas and declining sites, have equal
// Constructs, and declines of two different constructs have different ones.
// One row per construct group: the declaration, the content, a particle, an
// attribute use and xsi:type.
func TestDeclineConstruct(t *testing.T) {
	const a = `<s:a>x</s:a>`
	xsiImport := `<xs:import namespace="http://www.w3.org/2001/XMLSchema-instance"/>`
	type decl struct{ decls, attrs, content string }
	rows := []struct {
		group string
		pair  [2]decl
	}{
		{"declaration", [2]decl{
			{`<xs:element name="r" nillable="true"><xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType></xs:element>`, "", a},
			{sequenceOf(`<xs:element name="a" type="xs:string" nillable="true"/>`), "", a},
		}},
		{"content", [2]decl{
			{root(`<xs:choice maxOccurs="2"><xs:element name="a" type="xs:string"/></xs:choice>`), "", a},
			{`<xs:complexType name="ct"><xs:choice minOccurs="0"><xs:element name="b" type="xs:string"/><xs:element name="a" type="xs:string"/></xs:choice></xs:complexType><xs:element name="r" type="s:ct"/>`, "", a},
		}},
		{"particle", [2]decl{
			{sequenceOf(`<xs:any processContents="lax"/><xs:any processContents="lax"/>`), "", a + `<s:b/>`},
			{root(`<xs:choice><xs:any namespace="##local" processContents="lax"/><xs:any namespace="##other" processContents="skip"/></xs:choice>`), "", `<o:c xmlns:o="urn:o"/>`},
		}},
		{"attribute use", [2]decl{
			{xsiImport + root(`<xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence><xs:attribute ref="xsi:nil"/>`), "", a},
			{xsiImport + root(`<xs:sequence><xs:element name="b" type="xs:string"/></xs:sequence><xs:attribute name="c" type="xs:string"/><xs:attribute ref="xsi:schemaLocation"/>`), "", `<s:b>y</s:b>`},
		}},
		{"xsi:type", [2]decl{
			{`<xs:complexType name="rt"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType><xs:element name="r" type="s:rt"/>`, ` xsi:type="s:rt"`, a},
			{sequenceOf(`<xs:element name="a" type="xs:string"/>`), "", `<s:a xsi:type="xs:token">x</s:a>`},
		}},
	}
	constructs := make([]Construct, len(rows))
	for i, row := range rows {
		var got [2]Construct
		for j, d := range row.pair {
			schema := schemaOf(t, d.decls)
			data := instanceOf(d.attrs, d.content)
			mustAccept(t, schema, data)
			_, err := Decode[struct{ A string }](schema, strict.New(), data)
			var de *DeclineError
			if !errors.As(err, &de) {
				t.Fatalf("%s %d: Decode error = %v, want a *DeclineError", row.group, j, err)
			}
			got[j] = de.Construct()
		}
		if got[0] != got[1] {
			t.Errorf("%s: two declines of one construct have Constructs %s and %s, want equal", row.group, got[0], got[1])
		}
		constructs[i] = got[0]
	}
	for i := range rows {
		for j := i + 1; j < len(rows); j++ {
			if constructs[i] == constructs[j] {
				t.Errorf("the %s and %s declines have equal Constructs %s, want different", rows[i].group, rows[j].group, constructs[i])
			}
		}
	}
}

// TestDeclineErrorText pins a decline's whole message for a construct of the
// document element's type: the declaration's schema Loc, then the type's,
// labelled as the type's, then the refusal tail.
func TestDeclineErrorText(t *testing.T) {
	schema := schemaOf(t, root(`<xs:choice maxOccurs="2"><xs:element name="a" type="xs:string"/></xs:choice>`))
	_, err := Decode[struct{}](schema, strict.New(), instanceOf("", `<s:a>x</s:a>`))
	const want = "?:1:1: codec does not decode a content choice occurring other than exactly once: the {type definition} of element {urn:s}r declared at s.xsd:1:181, a type defined at s.xsd:1:202; a decoder refusal, not a schema or instance fault"
	if err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}
}

// TestDeclineErrorTextSimpleRoot pins the message of a decline of the
// document element's type when that type is simple and built in,
// xs:anySimpleType, and that it precedes the T check: T is int, which is not
// the Go type of any simple type codegen emits.
func TestDeclineErrorTextSimpleRoot(t *testing.T) {
	schema := schemaOf(t, `<xs:element name="r" type="xs:anySimpleType"/>`)
	_, err := Decode[int](schema, strict.New(), instanceOf("", `1.0`))
	const want = "?:1:1: codec does not decode a special simple type: the {type definition} of element {urn:s}r declared at s.xsd:1:181, a type defined at ?; a decoder refusal, not a schema or instance fault"
	if err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}
}

// mustAccept fails the test unless validate accepts data against schema.
func mustAccept(t *testing.T, schema *xsd.Schema, data []byte) {
	t.Helper()
	v, err := validate.New(schema, strict.New())
	if err != nil {
		t.Fatal(err)
	}
	res, err := xmlsrc.Validate(v, bytes.NewReader(data))
	if err != nil || res.Err() != nil || res.Standing() != validate.NotProvenInvalid {
		t.Fatalf("the row's instance is not accepted: %v %v %v", err, res.Violations(), res.Unevaluated())
	}
}

// assertDecline fails the test unless err is the *DeclineError of
// construct c at instance line and col, wrapping errors.ErrUnsupported.
func assertDecline(t *testing.T, err error, c construct, line, col int) {
	t.Helper()
	var de *DeclineError
	if !errors.As(err, &de) {
		t.Fatalf("Decode error = %v, want a *DeclineError", err)
	}
	if got := de.Construct(); got != (Construct{c}) || got.String() != c.name() {
		t.Errorf("Construct() = %s, want %s", got, c.name())
	}
	loc, charged := de.Loc()
	if !charged || loc.Line != line || loc.Col != col {
		t.Errorf("Loc = %s, %t, want %d:%d, true", loc, charged, line, col)
	}
	if !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("error %v does not wrap errors.ErrUnsupported", err)
	}
	if want := loc.String() + ": codec does not decode " + c.name() + ": "; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error %q does not open with %q", err, want)
	}
}

// TestDeclineErrorTextOtherPrimitive pins CheckDecode's message for a decline
// of a local element whose type's primitive, the minted s:money, is
// ·implementation-defined·, so none of those atomicDecline admits.
func TestDeclineErrorTextOtherPrimitive(t *testing.T) {
	schema := mintedSchema(t, sequenceOf(`<xs:element name="a" type="s:money"/>`))
	err := CheckDecode[struct{}](schema, rootName)
	const want = "codec does not decode an atomic simple type whose primitive is ·implementation-defined·, other than xs:precisionDecimal: element {urn:s}a declared at s.xsd:1:231; a decoder refusal, not a schema or instance fault"
	if err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}
}
