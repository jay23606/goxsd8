package validate

import (
	"strings"
	"testing"

	"github.com/kud360/goxsd8/xsd"
)

// The fixtures below drive an assertion {test} that sums, with fn:sum, the
// prices of the item grandchildren of <order> (xpath-functions.md §15.4.5),
// whose typed values the walk reports to the test's xpath.Tally as it passes
// them (walk.tallyAttributes). The shape is assert_019's po_sample: each
// <item> of <items> is declared Item, whose price is a Price (xs:decimal,
// minInclusive 0), and its {type table} selects ShortItem or LongItem, both
// extensions of Item; NoPrice restricts ShortItem, prohibiting its price.
// <odd> admits one local item and then any no-namespace element strictly,
// which the global item, of Strange whose price is an xs:string, governs;
// <skips> admits one local item and then any no-namespace element ·skipped·;
// <defs>' items are DefaultItem, a restriction of Item whose price defaults
// to 5; <und>'s item takes its type from an alternative whose {test} xpath
// declines, so its ·governing type definition· is undetermined.
const smSchemaText = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:simpleType name="Price">
    <xs:restriction base="xs:decimal"><xs:minInclusive value="0"/></xs:restriction>
  </xs:simpleType>
  <xs:complexType name="Item">
    <xs:attribute name="price" type="Price"/>
  </xs:complexType>
  <xs:complexType name="ShortItem">
    <xs:complexContent>
      <xs:extension base="Item"><xs:attribute name="id" type="xs:string"/></xs:extension>
    </xs:complexContent>
  </xs:complexType>
  <xs:complexType name="LongItem">
    <xs:complexContent>
      <xs:extension base="Item">
        <xs:sequence><xs:element name="description" type="xs:string"/></xs:sequence>
      </xs:extension>
    </xs:complexContent>
  </xs:complexType>
  <xs:complexType name="DefaultItem">
    <xs:complexContent>
      <xs:restriction base="Item"><xs:attribute name="price" type="Price" default="5"/></xs:restriction>
    </xs:complexContent>
  </xs:complexType>
  <xs:complexType name="NoPrice">
    <xs:complexContent>
      <xs:restriction base="ShortItem"><xs:attribute name="price" use="prohibited"/></xs:restriction>
    </xs:complexContent>
  </xs:complexType>
  <xs:complexType name="Strange">
    <xs:attribute name="price" type="xs:string"/>
  </xs:complexType>
  <xs:complexType name="Items">
    <xs:sequence>
      <xs:element name="item" type="Item" minOccurs="0" maxOccurs="unbounded">
        <xs:alternative test="@id" type="ShortItem"/>
        <xs:alternative type="LongItem"/>
      </xs:element>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="Odd">
    <xs:sequence>
      <xs:element name="item" type="Item" minOccurs="0"/>
      <xs:any namespace="##local" processContents="strict" minOccurs="0"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="Skips">
    <xs:sequence>
      <xs:element name="item" type="Item" minOccurs="0"/>
      <xs:any namespace="##local" processContents="skip" minOccurs="0"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="Defs">
    <xs:sequence>
      <xs:element name="item" type="DefaultItem" minOccurs="0" maxOccurs="unbounded"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="Und">
    <xs:sequence>
      <xs:element name="item" type="Item" minOccurs="0">
        <xs:alternative test="string-length(@k) gt 0" type="ShortItem"/>
      </xs:element>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="Order">
    <xs:sequence>
      <xs:element name="items" type="Items" minOccurs="0"/>
      <xs:element name="odd" type="Odd" minOccurs="0"/>
      <xs:element name="skips" type="Skips" minOccurs="0"/>
      <xs:element name="defs" type="Defs" minOccurs="0"/>
      <xs:element name="und" type="Und" minOccurs="0"/>
      <xs:element name="total" type="xs:decimal"/>
    </xs:sequence>
    <xs:assert test="TEST"/>
  </xs:complexType>
  <xs:element name="order" type="Order"/>
  <xs:element name="item" type="Strange"/>
</xs:schema>`

// smSchema is smSchemaText with test as Order's one assertion.
func smSchema(t *testing.T, test string) *xsd.Schema {
	t.Helper()
	escaped := strings.NewReplacer("&", "&amp;", "<", "&lt;", `"`, "&quot;").Replace(test)
	return parsedSchema(t, map[string]string{"main.xsd": strings.Replace(smSchemaText, "TEST", escaped, 1)})
}

// smItem is an <item> at line carrying attrs over kids.
func smItem(line int, attrs []Attribute, kids ...Child) Child {
	return ccNode(local("item"), line, attrs, kids...)
}

// smPrice is an item's price of lexical, at line.
func smPrice(lexical string, line int) []Attribute {
	return []Attribute{ccAttr(local("price"), lexical, line)}
}

// smDescription is a LongItem's <description> at line.
func smDescription(line int) Child {
	return ElementChild(&testElement{name: local("description"), loc: loc(line, 5)})
}

// smOrder is the validation root <order> at 1:1: a container named
// container over items, then <total> holding total.
func smOrder(container string, total string, items ...Child) *testElement {
	totalEl := &testElement{name: local("total"), loc: loc(20, 3), kids: []Child{TextChild(&testText{data: total, loc: loc(20, 10)})}}
	return &testElement{name: local("order"), loc: loc(1, 1), kids: []Child{
		ccNode(local(container), 2, nil, items...),
		ElementChild(totalEl),
	}}
}

// smDeclined fails unless res recorded a cvc-assertion decline at <order>
// naming want, and charged no cvc-assertion — whatever else res charged.
func smDeclined(t *testing.T, res *Result, want string) {
	t.Helper()
	found := false
	for _, u := range res.Unevaluated() {
		found = found || u.Rule() == ruleCvcAssertion && u.Loc() == loc(1, 1) && strings.Contains(u.Msg(), want)
	}
	if !found {
		t.Errorf("Unevaluated() = %v, want a cvc-assertion decline at %s naming %q", messages(res.Unevaluated()), loc(1, 1), want)
	}
	for _, v := range res.Violations() {
		if v.Rule == ruleCvcAssertion {
			t.Errorf("Violations() holds %v: a declined assertion is never charged", v)
		}
	}
}

// fn:sum adds the typed value of each price of each item of <items>, mapped
// under Price, the type the {test} was compiled to read off Item, the
// ·locally declared type· (key-ldt-elem): the {type table} selects ShortItem
// for an item with an id and LongItem for one without, and each extends Item,
// keeping its price use. 88.10 + 20 is 108.10, which a <total> of 108.10
// satisfies and one of 108.11 is charged for, as is a `* 0.1` against the
// wrong tax. An item with no price contributes nothing, and an item of
// <defs>, whose price defaults to 5, contributes its ·defaulted attribute·.
// With walk.tallyAttributes not called, every satisfied row but the one with
// no item is charged, each Tally left owing a value; with its ·defaulted
// attribute· loop skipped, the satisfied ·defaulted· row alone is; and with
// walk.staticTypeBelow stopping after the first step, every row of this test
// and the next is declined at compile.
func TestAssertionSumsGrandchildAttributes(t *testing.T) {
	short := func(line int, price string) Child {
		return smItem(line, append(smPrice(price, line), ccAttr(local("id"), "105", line)))
	}
	long := func(line int, attrs []Attribute) Child {
		return smItem(line, attrs, smDescription(line+1))
	}
	for _, tc := range []struct {
		why, test string
		root      *testElement
		charged   bool
	}{
		{"a CTA-selected short and long item", "total = sum(items/item/@price)",
			smOrder("items", "108.10", long(3, smPrice("88.10", 3)), short(5, "20")), false},
		{"the wrong total", "total = sum(items/item/@price)",
			smOrder("items", "108.11", long(3, smPrice("88.10", 3)), short(5, "20")), true},
		{"a tenth of the sum", "total = sum(items/item/@price) * 0.1",
			smOrder("items", "10.810", long(3, smPrice("88.10", 3)), short(5, "20")), false},
		{"the wrong tax", "total = sum(items/item/@price) * 0.1",
			smOrder("items", "10.82", long(3, smPrice("88.10", 3)), short(5, "20")), true},
		{"an item with no price", "total = sum(items/item/@price)",
			smOrder("items", "20", long(3, nil), short(5, "20")), false},
		{"no item at all", "total = sum(items/item/@price)",
			smOrder("items", "0"), false},
		{"a ·defaulted· price", "total = sum(defs/item/@price)",
			smOrder("defs", "25", smItem(3, smPrice("20", 3)), smItem(4, nil)), false},
		{"a ·defaulted· price, charged", "total = sum(defs/item/@price)",
			smOrder("defs", "20", smItem(3, smPrice("20", 3)), smItem(4, nil)), true},
	} {
		t.Run(tc.why, func(t *testing.T) {
			res := aAssess(t, smSchema(t, tc.test), tc.root)
			if !tc.charged {
				wantSatisfied(t, res, tc.test)
				return
			}
			wantAssertionCharge(t, res, "the element order is not ·valid· with respect to assertion 1 of 1 in the {assertions} of the ·governing type definition· Order, whose {test} is ")
		})
	}
}

// An attribute the {test} reads that has no typed value under the type it was
// compiled to read declines the assertion, which is never charged: a price
// with no ·actual value· under Price ("abc", which cvc-attribute clause 3
// charges); a price on an item an xsi:type governs by NoPrice, whose
// {attribute uses} match no price (which cvc-complex-type clause 3 charges); a
// price of an item the strict wildcard of <odd> admits, governed by the
// global item's Strange, whose price is an xs:string, not derived from Price;
// a price on an item the skip wildcard of <skips> ·skipped·, whose typed value
// is xs:untypedAtomic; and an item of <und>, whose ·governing type
// definition· was not determined, so its ·defaulted attributes·, a price
// among them, are unknown. With walk.tallyAttributes not called, the abc,
// Strange and ·skipped· rows are charged; the Strange row is evaluated with
// walk.attributeBelow's derivation test removed, the ·skipped· row with
// assertionAncestry.tallySkipped taking no lack, and the <und> row with
// ctaAttributePath.selectsAttributesAt answering false; the abc row's decline
// names Price and not its own type with walk.attributeBelow's String Valid
// skipped, and the NoPrice row panics with walk.tallyAttributes reading a
// price no use matches.
func TestAssertionSumDeclinesAnUnreadPrice(t *testing.T) {
	for _, tc := range []struct {
		why, test, want string
		root            *testElement
	}{
		{"a price with no ·actual value·", "total = sum(items/item/@price)",
			`"abc" has no ·actual value· under its type Price`,
			smOrder("items", "0", smItem(3, append(smPrice("abc", 3), ccAttr(local("id"), "1", 3))))},
		{"a price no {attribute use} matches", "total = sum(items/item/@price)",
			"no {attribute use} of its element's ·governing type definition· matches it",
			smOrder("items", "5", smItem(3, append(smPrice("5", 3), ccAttr(local("id"), "1", 3),
				ccAttr(xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: "type"}, "NoPrice", 3))))},
		{"a price of a type not derived from Price", "total = sum(odd/item/@price)",
			"string is neither Price, the type the {test} was compiled to read",
			smOrder("odd", "5", smItem(3, smPrice("5", 3)), smItem(4, smPrice("0", 4)))},
		{"a price on a ·skipped· item", "total = sum(skips/item/@price)",
			"its element is ·skipped·",
			smOrder("skips", "5", smItem(3, smPrice("5", 3)), smItem(4, smPrice("0", 4)))},
		{"an item whose type was not determined", "total = sum(und/item/@price)",
			"its ·governing type definition· was not determined",
			smOrder("und", "5", smItem(3, smPrice("5", 3)))},
	} {
		t.Run(tc.why, func(t *testing.T) {
			smDeclined(t, aAssess(t, smSchema(t, tc.test), tc.root), tc.want)
		})
	}
}
