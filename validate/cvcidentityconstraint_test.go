package validate

import (
	"strings"
	"testing"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// These fixtures drive cvc-identity-constraint (§3.11.4) and the §3.11.5 node
// tables it rests on, over the schema shape icfixture_test.go builds.

// icItem is <item> with the given no-namespace attributes, one Loc per line.
func icItem(ns string, line int, attrs ...xsd.QName) func(...string) *testElement {
	return func(values ...string) *testElement {
		items := make([]Attribute, 0, len(attrs))
		for i, a := range attrs {
			items = append(items, icAttr(a, values[i], line))
		}
		return icElem(xsd.QName{Space: ns, Local: "item"}, line, items)
	}
}

// icIDed is <item id="..."> at line.
func icIDed(line int, id string) *testElement {
	return icItem("", line, xsd.QName{Local: "id"})(id)
}

// icRoot is <root> over the given children.
func icRoot(kids ...Element) *testElement {
	return icElem(xsd.QName{Local: "root"}, 1, nil, icKids(kids...)...)
}

// A key forbids two ·target nodes· to share a ·key-sequence· (clause 4.2.2),
// and the charge lands on the LATER of the two — the one a reader has to
// change.
func TestKeyChargesADuplicateKeySequence(t *testing.T) {
	key := icDef(t, "K", xsd.IdentityConstraintKey, ".//item", nil, "", "@id")
	schema := icSchema(t, "", false, []xsd.IdentityConstraint{key}, nil)

	icWantCharges(t, icAssess(t, schema, icRoot(icIDed(2, "a"), icIDed(3, "a"))),
		icCharge(ruleCvcIdentityConstraint, 3))
	icWantCharges(t, icAssess(t, schema, icRoot(icIDed(2, "a"), icIDed(3, "b"))))
}

// Clause 4.2.1 makes a MISSING field an error for a key, and clause 4.1 leaves
// the same document alone for a unique — whose own Note says so: "a selected
// unique node may have fields that do not have corresponding [schema actual
// value]s".
func TestKeyChargesAnAbsentFieldAndUniqueDoesNot(t *testing.T) {
	bare := icElem(xsd.QName{Local: "item"}, 3, nil)

	key := icDef(t, "K", xsd.IdentityConstraintKey, ".//item", nil, "", "@id")
	icWantCharges(t,
		icAssess(t, icSchema(t, "", false, []xsd.IdentityConstraint{key}, nil), icRoot(icIDed(2, "a"), bare)),
		icCharge(ruleCvcIdentityConstraint, 3))

	unique := icDef(t, "U", xsd.IdentityConstraintUnique, ".//item", nil, "", "@id")
	icWantCharges(t,
		icAssess(t, icSchema(t, "", false, []xsd.IdentityConstraint{unique}, nil), icRoot(icIDed(2, "a"), bare)))
}

// Clause 3 admits at most one node of a simple ·governing type definition· per
// field, so a field selecting two valued ones is charged at the second — the
// attribute information item that is one too many, not the ·target node·.
func TestKeyChargesAFieldSelectingTwoValuedNodes(t *testing.T) {
	key := icDef(t, "K", xsd.IdentityConstraintKey, ".//item", nil, "", "@*")
	schema := icSchema(t, "", false, []xsd.IdentityConstraint{key}, nil)

	two := icItem("", 2, xsd.QName{Local: "id"}, xsd.QName{Local: "k"})("a", "1")
	icWantCharges(t, icAssess(t, schema, icRoot(two)), icChargeAttr(ruleCvcIdentityConstraint, 2))
	icWantCharges(t, icAssess(t, schema, icRoot(icIDed(2, "a"))))
}

// A union is a sequence of distinct NODES, so an attribute two branches both
// select is ONE field node — not the clause 3 violation two would be.
func TestFieldUnionSelectsOneAttributeOnce(t *testing.T) {
	key := icDef(t, "K", xsd.IdentityConstraintKey, ".//item", nil, "", "@id|@*")
	schema := icSchema(t, "", false, []xsd.IdentityConstraint{key}, nil)

	icWantCharges(t, icAssess(t, schema, icRoot(icIDed(2, "a"))))
}

// Node tables propagate UPWARD and never sideways (PRINCIPLES 15): a keyref on
// <box> resolves against the key sequences sourced inside that box, and a key
// sourced in a SIBLING box is not among them — §3.11.4 clause 4.3's own Note,
// "only element information items within the sub-tree rooted at the element
// information item being ·validated· can be referenced successfully".
func TestKeyrefResolvesInsideItsSubtreeAndNotOutsideIt(t *testing.T) {
	key := icDef(t, "K", xsd.IdentityConstraintKey, "item", nil, "", "@id")
	keyref := icDef(t, "R", xsd.IdentityConstraintKeyref, "ref", nil, "K", "@r")
	schema := icSchema(t, "", false, nil, []xsd.IdentityConstraint{key, keyref})

	ref := func(line int, r string) *testElement {
		return icElem(xsd.QName{Local: "ref"}, line, []Attribute{icAttr(xsd.QName{Local: "r"}, r, line)})
	}
	box := func(line int, kids ...Element) *testElement {
		return icElem(xsd.QName{Local: "box"}, line, nil, icKids(kids...)...)
	}

	// One box holding both the key and the reference: the keyref resolves.
	inside := icElem(xsd.QName{Local: "root"}, 1, nil,
		ElementChild(box(2, icIDed(3, "a"), ref(4, "a"))))
	icWantCharges(t, icAssess(t, schema, inside))

	// The key one box over is invisible, however plainly it is in the
	// document: it never entered THIS box's node table.
	outside := icElem(xsd.QName{Local: "root"}, 1, nil,
		ElementChild(box(2, icIDed(3, "a"), ref(4, "a"))),
		ElementChild(box(5, icIDed(6, "b"), ref(7, "a"))))
	icWantCharges(t, icAssess(t, schema, outside), icCharge(ruleCvcIdentityConstraint, 7))
}

// Key-sequence members are compared in the VALUE space (Datatypes §2.2), so two
// xs:integer fields written "1" and "01" are ONE key sequence. A lexical
// comparison would find no duplicate here at all.
func TestKeySequencesCompareAsValuesAndNotLexicals(t *testing.T) {
	key := icDef(t, "K", xsd.IdentityConstraintKey, ".//item", nil, "", "@k")
	schema := icSchema(t, "", false, []xsd.IdentityConstraint{key}, nil)

	numbered := func(line int, k string) *testElement {
		return icItem("", line, xsd.QName{Local: "k"})(k)
	}
	icWantCharges(t, icAssess(t, schema, icRoot(numbered(2, "1"), numbered(3, "01"))),
		icCharge(ruleCvcIdentityConstraint, 3))
	icWantCharges(t, icAssess(t, schema, icRoot(numbered(2, "1"), numbered(3, "2"))))
}

// xpathDefaultNamespace supplies the default namespace of an unprefixed
// NameTest reached by an ELEMENT step and of no attribute step (PRINCIPLES 15).
// Both halves are pinned by one document: the selector "item" finds the
// namespaced elements only if the default reached it, and the field "@id" finds
// the NO-namespace attribute only if the default did not.
func TestSelectorAndFieldTreatTheDefaultNamespaceAsymmetrically(t *testing.T) {
	noNS, inNS := xsd.QName{Local: "id"}, xsd.QName{Space: icNS, Local: "id"}
	item := func(line int, bare, qualified string) *testElement {
		return icItem(icNS, line, noNS, inNS)(bare, qualified)
	}
	// The two items agree on the no-namespace id and differ on the namespaced
	// one, so which attribute the field selected decides whether they collide.
	doc := icRoot(item(2, "same", "1"), item(3, "same", "2"))
	def := icNS

	element := icDef(t, "K", xsd.IdentityConstraintKey, "item", &def, "", "@id")
	icWantCharges(t, icAssess(t, icSchema(t, icNS, false, []xsd.IdentityConstraint{element}, nil), doc),
		icCharge(ruleCvcIdentityConstraint, 3))

	qualified := icDef(t, "K", xsd.IdentityConstraintKey, "item", &def, "", "@p:id")
	icWantCharges(t, icAssess(t, icSchema(t, icNS, false, []xsd.IdentityConstraint{qualified}, nil), doc))
}

// A field node whose ·governing type definition· this package could not
// determine has no [schema actual value] to contribute (§3.3.5.4), so the
// constraint DECLINES rather than comparing lexicals — the same two documents
// that collide with a determinable type charge nothing with an xsi:type on the
// field node.
func TestFieldWithNoGoverningTypeDeclines(t *testing.T) {
	field := func(local string) *xsd.Schema {
		key := icDef(t, "K", xsd.IdentityConstraintKey, ".//item", nil, "", local)
		return icSchema(t, "", false, []xsd.IdentityConstraint{key}, nil)
	}
	named := func(line int, local, text string) *testElement {
		node := icElem(xsd.QName{Local: local}, line+1, nil,
			TextChild(&testText{data: text, loc: loc(line+1, 8)}))
		return icElem(xsd.QName{Local: "item"}, line, nil, ElementChild(node))
	}

	icWantCharges(t, icAssess(t, field("name"), icRoot(named(2, "name", "a"), named(4, "name", "a"))),
		icCharge(ruleCvcIdentityConstraint, 4))
	icWantCharges(t, icAssess(t, field("tabled"), icRoot(named(2, "tabled", "a"), named(4, "tabled", "a"))))
}

// Clause 4.2.3 forbids a key's ·key-sequence· to take an ELEMENT member from a
// declaration whose {nillable} is true, whatever the element's content is. The
// charge carries the field node's own Loc.
func TestKeyChargesANillableElementMember(t *testing.T) {
	key := icDef(t, "K", xsd.IdentityConstraintKey, ".//item", nil, "", "name")
	named := icElem(xsd.QName{Local: "item"}, 2, nil,
		ElementChild(icElem(xsd.QName{Local: "name"}, 3, nil,
			TextChild(&testText{data: "a", loc: loc(3, 8)}))))
	doc := icRoot(named)

	icWantCharges(t, icAssess(t, icSchema(t, "", false, []xsd.IdentityConstraint{key}, nil), doc))
	icWantCharges(t, icAssess(t, icSchema(t, "", true, []xsd.IdentityConstraint{key}, nil), doc),
		icCharge(ruleCvcIdentityConstraint, 3))
}

// Only a ·nilled· field node contributes no ·key-sequence· member
// (elementKeyMember), and ·nilled· is key-nilled's conjunction — D.{nillable} =
// true AND an ·actual value· of true ([nilled]) — not the PRESENCE of an xsi:nil
// attribute. Both conjuncts are pinned by the duplicate a unique charges only
// where both <name> nodes supplied their member: reading presence alone would
// leave every sequence below short and charge nothing at all.
func TestOnlyANilledFieldNodeContributesNoKeySequenceMember(t *testing.T) {
	unique := icDef(t, "U", xsd.IdentityConstraintUnique, ".//item", nil, "", "name")
	doc := func(nil_ string) *testElement {
		named := func(line int) *testElement {
			name := icElem(xsd.QName{Local: "name"}, line+1,
				[]Attribute{icAttr(xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: "nil"}, nil_, line+1)},
				TextChild(&testText{data: "a", loc: loc(line+1, 8)}))
			return icElem(xsd.QName{Local: "item"}, line, nil, ElementChild(name))
		}
		return icRoot(named(2), named(4))
	}
	ics := []xsd.IdentityConstraint{unique}

	icWantCharges(t, icAssess(t, icSchema(t, "", true, ics, nil), doc("false")),
		icCharge(ruleCvcIdentityConstraint, 4))

	// Both nodes ·nilled·: §3.3.5.4 gives each an absent [schema actual value],
	// so neither ·key-sequence· has a member and clause 4.1 compares nothing.
	// Their character [[children]] are cvc-elt clause 3.2.3.1's to charge, at
	// the offending [[child]]'s own position.
	icWantCharges(t, icAssess(t, icSchema(t, "", true, ics, nil), doc("true")),
		icChargeAt(ruleCvcElt, loc(3, 8)), icChargeAt(ruleCvcElt, loc(5, 8)))

	// xsi:nil = true on a declaration whose {nillable} is false makes no
	// ·nilled· node either: cvc-elt clause 3.1 charges the attribute, and the
	// members are supplied and compared as before.
	icWantCharges(t, icAssess(t, icSchema(t, "", false, ics, nil), doc("true")),
		icCharge(ruleCvcElt, 3), icCharge(ruleCvcElt, 5),
		icCharge(ruleCvcIdentityConstraint, 4))
}

// An identity constraint whose {selector} or {fields} fall outside the
// §3.11.6.2/§3.11.6.3 subset charges nothing at all (icFrame.declined's GAP),
// and neither does a keyref referring to a key that declined.
func TestUnreadablePathDeclinesTheWholeConstraint(t *testing.T) {
	key := icDef(t, "K", xsd.IdentityConstraintKey, "item[1]", nil, "", "@id")
	keyref := icDef(t, "R", xsd.IdentityConstraintKeyref, "ref", nil, "K", "@r")
	schema := icSchema(t, "", false, []xsd.IdentityConstraint{key, keyref}, nil)

	ref := icElem(xsd.QName{Local: "ref"}, 4, []Attribute{icAttr(xsd.QName{Local: "r"}, "zzz", 4)})
	icWantCharges(t, icAssess(t, schema, icRoot(icIDed(2, "a"), icIDed(3, "a"), ref)))
}

// A keyref whose {referenced key} has no node table at all is charged:
// "there is a node table associated with the {referenced key}" is the first
// conjunct of cvc-identity-constraint clause 4.3, and a key declared only on
// <box> has none at a <root> holding no <box>. Neither the key's element nor
// any binding of it occurred, which is what separates this from
// TestKeyrefChargesAgainstAnEmptyNodeTable: making icCheck.keyrefs skip a
// keyref whose binding is not found fails this test and passes that one.
func TestKeyrefChargesWhenTheReferencedKeyNeverOccurred(t *testing.T) {
	key := icDef(t, "K", xsd.IdentityConstraintKey, "item", nil, "", "@id")
	keyref := icDef(t, "R", xsd.IdentityConstraintKeyref, "ref", nil, "K", "@r")
	schema := icSchema(t, "", false, []xsd.IdentityConstraint{keyref}, []xsd.IdentityConstraint{key})

	ref := icElem(xsd.QName{Local: "ref"}, 2, []Attribute{icAttr(xsd.QName{Local: "r"}, "a", 2)})
	icWantCharges(t, icAssess(t, schema, icRoot(ref)), icCharge(ruleCvcIdentityConstraint, 2))
}

// A keyref whose {referenced key} has a node table with NO entries is charged:
// the key is declared on the <box> that occurs, so its table is present, and
// the selector selects no <item>, so the second conjunct of
// cvc-identity-constraint clause 4.3 — some entry's ·key-sequence· equal to
// the keyref member's — fails on an empty table. Making icCheck.keyrefs skip a
// found binding with no entries fails this test and passes
// TestKeyrefChargesWhenTheReferencedKeyNeverOccurred.
func TestKeyrefChargesAgainstAnEmptyNodeTable(t *testing.T) {
	key := icDef(t, "K", xsd.IdentityConstraintKey, "item", nil, "", "@id")
	keyref := icDef(t, "R", xsd.IdentityConstraintKeyref, "ref", nil, "K", "@r")
	schema := icSchema(t, "", false, nil, []xsd.IdentityConstraint{key, keyref})

	ref := icElem(xsd.QName{Local: "ref"}, 3, []Attribute{icAttr(xsd.QName{Local: "r"}, "a", 3)})
	doc := icElem(xsd.QName{Local: "root"}, 1, nil,
		ElementChild(icElem(xsd.QName{Local: "box"}, 2, nil, ElementChild(ref))))
	icWantCharges(t, icAssess(t, schema, doc), icCharge(ruleCvcIdentityConstraint, 3))
}

// A `@NameTest` field over an ANONYMOUS governing type reads that type's own
// {attribute uses} for the same reason cvc-id does (icCheck.fieldAttributes
// over walk.attributeType): the top-level declaration its ·expanded name·
// resolves to is not the ·governing type definition·, and a key-sequence member
// compared under the wrong simple type is compared in the wrong ·value space·.
// The two documents below are one duplicate under the top level's xs:integer
// and two distinct ·key-sequences· under the xs:string <kid> declares, so the
// silence here is the field's decided verdict and the charge is what reading
// the wrong type would cost.
func TestFieldOverAnAnonymousTypeReadsItsOwnTypeAndNotTheTopLevelOne(t *testing.T) {
	key := icDef(t, "K", xsd.IdentityConstraintKey, "kid", nil, "", "@aid")
	schema := icAnonymousSchema(t, "string", "integer", []xsd.IdentityConstraint{key})

	icWantCharges(t, icAssess(t, schema, icRoot(icKid(2, "aid")("1"), icKid(3, "aid")("01"))))
}

// A `@NameTest` field reads the same ·governing type definition· cvc-id does
// (icCheck.fieldAttributes over walk.attributeType), so an attribute a
// ***skip*** {attribute wildcard} admits lengthens no ·key-sequence· either
// (#1043): neither ·target node· below has a key-sequence to be charged clause
// 4.2.2 for.
//
// Each is charged clause 4.2.1 instead, and that is the whole difference #717
// made here. A ·target node· whose ·key-sequence· is short is out of the
// ·qualified node set·, which for a key is clause 4.2.1's charge (§3.11.4
// clause 3's Note names ·skipped· nodes as the case). The charge was withheld
// while ·skipped· was inferred from the wildcard's PRESENCE, since an attribute
// the wildcard did not admit arrived indistinguishably; cvc-wildcard now
// decides the ·attribution· ([walk.skippedAttribute]), so the absence read here
// is the spec's and the charge is made.
func TestSkipWildcardAttributeLengthensNoKeySequence(t *testing.T) {
	key := icDef(t, "K", xsd.IdentityConstraintKey, "item", nil, "", "@wid")
	twice := icRoot(idItem(2, "wid", "a"), idItem(3, "wid", "a"))

	icWantCharges(t, icAssess(t, icWildcardSchema(t, xsd.ProcessSkip, []xsd.IdentityConstraint{key}), twice),
		icCharge(ruleCvcIdentityConstraint, 2), icCharge(ruleCvcIdentityConstraint, 3))

	// Under lax the same field DOES read the top-level declaration and the two
	// key-sequences collide, which is the charge the skip case withholds. The
	// declaration is xs:ID, so the id it also binds is charged alongside it.
	icWantCharges(t, icAssess(t, icWildcardSchema(t, xsd.ProcessLax, []xsd.IdentityConstraint{key}), twice),
		icCharge(ruleCvcIdentityConstraint, 3), icChargeAttr(ruleCvcID, 3))
}

// cvc-identity-constraint clause 3 admits "zero or more ·skipped· nodes"
// beside a field's one simple-valued node, so a field selecting a ·skipped·
// attribute AND a declared, valued one on the same ·target node· is charged
// nothing under clause 3, and the valued one fills the slot, in either
// document order: icCheck.fieldAttributes passes the ·skipped· one by. Each
// <item> below carries @v beside a @s the ***skip*** wildcard admits, and the
// plain <item> after it shares its @v, so the one charge is clause 4.2.2's
// duplicate — which only a slot filled from @v can produce. Offering the
// ·skipped· attribute as an absent, decided member instead fails both
// documents with a clause 3 charge at the attribute.
func TestSkippedFieldAttributeBesideAValuedOneFillsTheSlot(t *testing.T) {
	key := icDef(t, "K", xsd.IdentityConstraintKey, "item", nil, "", "@v|@s")
	uses := []xsd.AttributeUse{icUse(t, xsd.QName{Local: "v"}, "string")}
	schema := icWildcardSchemaWith(t, anyWildcard(t, xsd.ProcessSkip), uses, []xsd.IdentityConstraint{key})

	v := func(line int) Attribute { return icAttr(xsd.QName{Local: "v"}, "a", line) }
	s := func(line int) Attribute { return icAttr(xsd.QName{Local: "s"}, "x", line) }
	item := func(line int, attrs ...Attribute) *testElement {
		return icElem(xsd.QName{Local: "item"}, line, attrs)
	}

	skippedFirst := icRoot(item(2, s(2), v(2)), item(3, v(3)))
	icWantCharges(t, icAssess(t, schema, skippedFirst), icCharge(ruleCvcIdentityConstraint, 3))

	valuedFirst := icRoot(item(2, v(2), s(2)), item(3, v(3)))
	icWantCharges(t, icAssess(t, schema, valuedFirst), icCharge(ruleCvcIdentityConstraint, 3))
}

// A field node that is an EMPTY element whose declaration carries a {value
// constraint} contributes the ·actual value· of that constraint's {lexical form}
// to the ·key-sequence·, which is exactly what §3.11.4's Note calls for: "default
// or fixed value constraints may play a part in ·key-sequences·" (cvc-elt clause
// 5.1, #853).
//
// Two such nodes therefore share ONE ·key-sequence· and a key charges clause
// 4.2.2 against the later — a charge the decline this replaced could not make,
// having contributed no member and left the sequence short. The three documents
// are the discriminating set: two substituted values collide, a substituted value
// and an owned one do not, and a MISSING <dtag> leaves the sequence short, which
// is clause 4.2.1's own charge and not this one.
func TestDefaultedEmptyElementFillsItsKeySequence(t *testing.T) {
	key := icDef(t, "K", xsd.IdentityConstraintKey, ".//item", nil, "", "dtag")
	schema := icSchema(t, "", false, []xsd.IdentityConstraint{key}, nil)

	icWantCharges(t, icAssess(t, schema, icRoot(idDefaulted(2), idDefaulted(4))),
		icCharge(ruleCvcIdentityConstraint, 4), icCharge(ruleCvcID, 5))

	own := icElem(xsd.QName{Local: "item"}, 4, nil, ElementChild(
		icElem(xsd.QName{Local: "dtag"}, 5, nil, TextChild(&testText{data: "d2", loc: loc(5, 8)}))))
	icWantCharges(t, icAssess(t, schema, icRoot(idDefaulted(2), own)))

	bare := icElem(xsd.QName{Local: "item"}, 4, nil)
	icWantCharges(t, icAssess(t, schema, icRoot(idDefaulted(2), bare)),
		icCharge(ruleCvcIdentityConstraint, 4))
}

// A `@NameTest` field selecting an absent ·defaulted attribute· reads the
// ·effective value constraint·'s value into the ·key-sequence· (§3.11.4 clause
// 3's first Note, sic-attrDefault), so each constraint below decides on it:
// a key over one defaulted and one distinct explicit value is satisfied, not
// charged clause 4.2.1 for a short sequence; a key and a unique each charge
// the later of two targets sharing a value, one of them defaulted, under
// clause 4.2.2 and 4.1 (the unique's pair "01" and default "1" are one
// xs:integer value); and a keyref whose defaulted value matches no key is
// charged clause 4.3 at its target.
func TestFieldSelectsADefaultedAttribute(t *testing.T) {
	key := icDef(t, "K", xsd.IdentityConstraintKey, "ditem", nil, "", "@dv")
	unique := icDef(t, "U", xsd.IdentityConstraintUnique, "ditem", nil, "", "@dv")
	keyref := icDef(t, "R", xsd.IdentityConstraintKeyref, "dref", nil, "K", "@dr")
	dref := icElem(xsd.QName{Local: "dref"}, 3, nil)
	// clause pins which clause the one charge in got was made under.
	clause := func(t *testing.T, got []*xsderr.Error, want string) {
		t.Helper()
		if len(got) != 1 || !strings.Contains(got[0].Error(), "clause "+want+" ") {
			t.Errorf("Violations() = %v, want one charged under clause %s", got, want)
		}
	}

	t.Run("key over distinct values", func(t *testing.T) {
		icWantCharges(t, icAssess(t, icDefaultedAttrSchema(t, []xsd.IdentityConstraint{key}),
			icRoot(icDItem(2), icDItem(3, "5"))))
	})
	t.Run("key sharing a defaulted value", func(t *testing.T) {
		got := icAssess(t, icDefaultedAttrSchema(t, []xsd.IdentityConstraint{key}), icRoot(icDItem(2), icDItem(3)))
		icWantCharges(t, got, icCharge(ruleCvcIdentityConstraint, 3))
		clause(t, got, "4.2.2")
	})
	t.Run("unique sharing a defaulted value", func(t *testing.T) {
		got := icAssess(t, icDefaultedAttrSchema(t, []xsd.IdentityConstraint{unique}), icRoot(icDItem(2, "01"), icDItem(3)))
		icWantCharges(t, got, icCharge(ruleCvcIdentityConstraint, 3))
		clause(t, got, "4.1")
	})
	t.Run("keyref over a defaulted value", func(t *testing.T) {
		schema := icDefaultedAttrSchema(t, []xsd.IdentityConstraint{key, keyref})
		got := icAssess(t, schema, icRoot(icDItem(2, "1"), dref))
		icWantCharges(t, got, icCharge(ruleCvcIdentityConstraint, 3))
		clause(t, got, "4.3")
		icWantCharges(t, icAssess(t, schema, icRoot(icDItem(2, "2"), dref)))
	})
	// A ·defaulted attribute· whose declaration's {type definition} is absent
	// has no [schema actual value] this processor can read, so the key declines
	// at its owner rather than charging clause 4.2.1 for a short sequence (the
	// GAP on icCheck.fieldDefaultedAttributes).
	t.Run("defaulted attribute with an absent type declines", func(t *testing.T) {
		typelessKey := icDef(t, "C", xsd.IdentityConstraintKey, "ditem", nil, "", "@dc")
		got, undecided := assessRecorded(t, icDefaultedAttrSchema(t, []xsd.IdentityConstraint{typelessKey}), icRoot(icDItem(2)))
		wantSilence(t, got, "a declined field charges nothing")
		wantDeclines(t, icDeclines(undecided),
			Unevaluated{rule: ruleCvcIdentityConstraint, loc: loc(2, 1), msg: "clauses 3 and 4 are undecided"})
	})
}

// A default's [schema actual value] in a ·key-sequence· is its {value
// constraint}'s {lexical form} mapped under the bindings that constraint
// captured (value.ConstraintContext), never the instance's: two <ditem>s whose
// xs:QName defaults "p:a" bind p only in the schema share one ·key-sequence·,
// so a key charges clause 4.2.2 against the later — for the ·defaulted
// attribute· @dq (icCheck.fieldDefaultedAttributes) and for the empty element
// <dqe> (icCheck.assessed) alike. Mapped under elementContext, p is unbound,
// each member is ·absent·, and the key charges clause 4.2.1 at both targets
// instead.
func TestKeySequenceReadsADefaultUnderItsOwnBindings(t *testing.T) {
	withDQE := func(line int) *testElement {
		return icElem(xsd.QName{Local: "ditem"}, line, nil, ElementChild(icElem(xsd.QName{Local: "dqe"}, line, nil)))
	}
	for _, tc := range []struct {
		field string
		root  *testElement
	}{
		{"@dq", icRoot(icDItem(2), icDItem(3))},
		{"dqe", icRoot(withDQE(2), withDQE(3))},
	} {
		t.Run(tc.field, func(t *testing.T) {
			key := icDef(t, "K", xsd.IdentityConstraintKey, "ditem", nil, "", tc.field)
			got := icAssess(t, icDefaultedAttrSchema(t, []xsd.IdentityConstraint{key}), tc.root)
			icWantCharges(t, got, icCharge(ruleCvcIdentityConstraint, 3))
			if !strings.Contains(got[0].Error(), "clause 4.2.2 ") {
				t.Errorf("Violations() = %v, want the one charge under clause 4.2.2", got)
			}
		})
	}
}

// The rule ID is the BARE catalog name, with the clause in the message text.
func TestIdentityConstraintRuleIsTheBareCatalogName(t *testing.T) {
	if !xsderr.IsValidRule(ruleCvcIdentityConstraint) {
		t.Errorf("%q is not a catalog rule", ruleCvcIdentityConstraint)
	}
	if xsderr.IsValidRule(xsderr.Rule("cvc-identity-constraint.4.2.1")) {
		t.Error("cvc-identity-constraint.4.2.1 is a catalog rule; the clause belongs in the message")
	}
}

// icDeclines is the cvc-identity-constraint records among undecided, in order.
func icDeclines(undecided []Unevaluated) []Unevaluated {
	var out []Unevaluated
	for _, u := range undecided {
		if u.Rule() == ruleCvcIdentityConstraint {
			out = append(out, u)
		}
	}
	return out
}

// A key whose field selects only ·nilled· nodes leaves every ·key-sequence·
// short, which §3.11.4 clause 3's Note names outright, so each ·target node·
// is out of the ·qualified node set· and clause 4.2.1 charges it. Nothing is
// withheld, so nothing is recorded.
func TestANilledFieldNodeLeavesAKeySequenceShort(t *testing.T) {
	key := icDef(t, "K", xsd.IdentityConstraintKey, ".//item", nil, "", "name")
	schema := icSchema(t, "", true, []xsd.IdentityConstraint{key}, nil)
	doc := icRoot(idNilled(2, "true"), idNilled(4, "true"))

	got, undecided := assessRecorded(t, schema, doc)
	icWantCharges(t, got, icCharge(ruleCvcIdentityConstraint, 2), icCharge(ruleCvcIdentityConstraint, 4))
	wantDeclines(t, undecided)
}

// Every identity-constraint decline is RECORDED at its origin: a path outside
// the subset at the element declaring the constraint (clause 4), a keyref whose
// {referenced key}'s node table was not assembled (clause 4.3), and a field node
// with no determinable ·governing type definition· (clause 3).
func TestIdentityConstraintDeclinesAreRecorded(t *testing.T) {
	key := icDef(t, "K", xsd.IdentityConstraintKey, "item[1]", nil, "", "@id")
	keyref := icDef(t, "R", xsd.IdentityConstraintKeyref, "ref", nil, "K", "@r")
	ref := icElem(xsd.QName{Local: "ref"}, 4, []Attribute{icAttr(xsd.QName{Local: "r"}, "zzz", 4)})
	_, undecided := assessRecorded(t, icSchema(t, "", false, []xsd.IdentityConstraint{key, keyref}, nil),
		icRoot(icIDed(2, "a"), icIDed(3, "a"), ref))
	wantDeclines(t, icDeclines(undecided),
		Unevaluated{rule: ruleCvcIdentityConstraint, loc: loc(1, 1), msg: "clauses 3 and 4 are undecided"},
		Unevaluated{rule: ruleCvcIdentityConstraint, loc: loc(1, 1), msg: "clause 4.3 is undecided"})

	tabled := icDef(t, "T", xsd.IdentityConstraintKey, ".//item", nil, "", "tabled")
	named := func(line int) *testElement {
		node := icElem(xsd.QName{Local: "tabled"}, line+1, nil, TextChild(&testText{data: "a", loc: loc(line+1, 8)}))
		return icElem(xsd.QName{Local: "item"}, line, nil, ElementChild(node))
	}
	_, undecided = assessRecorded(t, icSchema(t, "", false, []xsd.IdentityConstraint{tabled}, nil), icRoot(named(2), named(4)))
	wantDeclines(t, icDeclines(undecided),
		Unevaluated{rule: ruleCvcIdentityConstraint, loc: loc(3, 1), msg: "clauses 3 and 4 are undecided"},
		Unevaluated{rule: ruleCvcIdentityConstraint, loc: loc(5, 1), msg: "clauses 3 and 4 are undecided"})
}

// A `@wid` field node on an element whose ·governing type definition· was not
// determined is not typed by the top-level declaration its ·expanded name·
// resolves to (key-governing-ad clause 3), clause 2's ·context-determined
// declaration· from a use of the undetermined type coming first: here T's
// xs:string use, under which "a" and " a" are distinct, while the top-level
// xs:ID would collapse them into one value. The slot declines instead, so
// neither a unique's clause 4.1 nor a key's 4.2.2 is charged, nor is cvc-id
// clause 2 for the same two values, and each decline is recorded at the
// attribute (#2192).
func TestAnUndeterminedTypesAttributeFieldDeclines(t *testing.T) {
	doc := icRoot(icTabledE(2, "a"), icTabledE(3, " a"))
	for _, cat := range []xsd.IdentityConstraintCategory{xsd.IdentityConstraintUnique, xsd.IdentityConstraintKey} {
		ic := icDef(t, "U", cat, "e", nil, "", "@wid")
		got, undecided := assessRecorded(t, icTabledAttrSchema(t, []xsd.IdentityConstraint{ic}), doc)
		wantSilence(t, got, "@wid is xs:string under T, so \"a\" and \" a\" differ")
		wantDeclines(t, icDeclines(undecided),
			Unevaluated{rule: ruleCvcIdentityConstraint, loc: loc(2, 2), msg: "its ·governing type definition· could not be determined"},
			Unevaluated{rule: ruleCvcIdentityConstraint, loc: loc(3, 2), msg: "its ·governing type definition· could not be determined"})
	}
}

// A ·key-sequence· comparison sameKeyMember cannot make — a member value with
// neither value.Eq nor value.Identical, which opaqueStrings gives every
// xs:string value — withholds the clause that reads it, and is recorded:
// clause 4.2.2 against the later ·target node· of a key, and clause 4.3 against
// a keyref member. Each document also runs under the test backend, whose
// decided answer is what the record stands in for.
func TestUndecidedKeySequenceComparisonsAreRecorded(t *testing.T) {
	opaque := opaqueStrings()
	key := icDef(t, "K", xsd.IdentityConstraintKey, ".//item", nil, "", "@id")
	schema := icSchema(t, "", false, []xsd.IdentityConstraint{key}, nil)
	doc := icRoot(icIDed(2, "a"), icIDed(3, "a"))
	icWantCharges(t, icAssess(t, schema, doc), icCharge(ruleCvcIdentityConstraint, 3))
	got, undecided := assessRecordedWith(t, opaque, schema, doc)
	wantSilence(t, got, "an undecided comparison charges nothing")
	wantDeclines(t, icDeclines(undecided), Unevaluated{rule: ruleCvcIdentityConstraint, loc: loc(3, 1), msg: "clause 4.2.2 is undecided"})

	unique := icDef(t, "U", xsd.IdentityConstraintUnique, "item", nil, "", "@id")
	keyref := icDef(t, "R", xsd.IdentityConstraintKeyref, "ref", nil, "U", "@r")
	schema = icSchema(t, "", false, []xsd.IdentityConstraint{unique, keyref}, nil)
	doc = icRoot(icIDed(2, "a"), icElem(xsd.QName{Local: "ref"}, 3, []Attribute{icAttr(xsd.QName{Local: "r"}, "a", 3)}))
	got, undecided = assessRecorded(t, schema, doc)
	wantSilence(t, got, "a keyref member equal to a unique one resolves")
	wantDeclines(t, icDeclines(undecided))
	got, undecided = assessRecordedWith(t, opaque, schema, doc)
	wantSilence(t, got, "an undecided lookup charges nothing")
	wantDeclines(t, icDeclines(undecided), Unevaluated{rule: ruleCvcIdentityConstraint, loc: loc(3, 1), msg: "clause 4.3 is undecided"})
}

// §3.11.5's conflict resolution drops two child entries sharing a
// ·key-sequence·, so a keyref member matching that sequence is charged under
// clause 4.3: under the test backend the xs:token entry of the first <box> and
// the xs:string entry of the second are one xs:string value. Under
// opaqueStrings the comparison between the two cannot be made — re-read in the
// xs:string value space, the first has neither value.Eq nor value.Identical —
// so both entries are kept CONTESTED. The keyref member matches the xs:token
// entry decidedly, since opaqueStrings maps xs:token by the test backend's
// xs:string mapping, and is recorded as undecided rather than passing on an
// entry the proviso may have removed. Without the contested mark, that
// document walks clean.
func TestKeyrefMatchingOnlyAContestedEntryIsRecorded(t *testing.T) {
	unique := icDef(t, "U", xsd.IdentityConstraintUnique, "item", nil, "", "@id|@tok")
	keyref := icDef(t, "R", xsd.IdentityConstraintKeyref, "item", nil, "U", "@tok")
	schema := icSchema(t, "", false, []xsd.IdentityConstraint{keyref}, []xsd.IdentityConstraint{unique})
	box := func(line int, item *testElement) *testElement {
		return icElem(xsd.QName{Local: "box"}, line, nil, ElementChild(item))
	}
	doc := icRoot(idItem(2, "tok", "a"), box(3, idItem(4, "tok", "a")), box(5, icIDed(6, "a")))

	got, undecided := assessRecorded(t, schema, doc)
	icWantCharges(t, got, icCharge(ruleCvcIdentityConstraint, 2))
	wantDeclines(t, icDeclines(undecided))

	got, undecided = assessRecordedWith(t, opaqueStrings(), schema, doc)
	wantSilence(t, got, "a match on a contested entry charges nothing")
	wantDeclines(t, icDeclines(undecided), Unevaluated{rule: ruleCvcIdentityConstraint, loc: loc(2, 1), msg: "clause 4.3 is undecided"})

	// The boxes swapped: the xs:token entry the keyref member matches is now
	// the LATER of the undecided pair, so it is resolveEntryConflicts's j side
	// that marks it contested. With that side's mark made a no-op, the match
	// passes decidedly and this document walks clean.
	swapped := icRoot(idItem(2, "tok", "a"), box(3, icIDed(4, "a")), box(5, idItem(6, "tok", "a")))

	got, undecided = assessRecorded(t, schema, swapped)
	icWantCharges(t, got, icCharge(ruleCvcIdentityConstraint, 2))
	wantDeclines(t, icDeclines(undecided))

	got, undecided = assessRecordedWith(t, opaqueStrings(), schema, swapped)
	wantSilence(t, got, "a match on the later contested entry charges nothing")
	wantDeclines(t, icDeclines(undecided), Unevaluated{rule: ruleCvcIdentityConstraint, loc: loc(2, 1), msg: "clause 4.3 is undecided"})
}

// opaqueBackend is base with every value of typ wrapped in an opaqueValue.
type opaqueBackend struct {
	base value.Backend
	typ  xsd.QName
}

// opaqueOver is the test backend with typ mapped by via's mapping, every value
// of typ an opaqueValue: typ keeps a space of its own, as a backend may give a
// derived type, while via and the ·primitive· keep their capabilities.
func opaqueOver(typ, via string) value.Backend {
	return opaqueBackend{
		base: aliasedBackend{base: testBackend(), from: icBuiltin(typ), to: icBuiltin(via)},
		typ:  icBuiltin(typ),
	}
}

// opaqueStrings is the test backend with every xs:string value an
// opaqueValue, and xs:token mapped by the test backend's own xs:string
// mapping, so an xs:token member keeps the capabilities its value re-read in
// the xs:string ·primitive· value space loses.
func opaqueStrings() value.Backend {
	return opaqueBackend{
		base: aliasedBackend{base: testBackend(), from: icBuiltin("token"), to: icBuiltin("string")},
		typ:  icBuiltin("string"),
	}
}

func (b opaqueBackend) Mapping(typ xsd.QName) (value.Mapping, bool) {
	m, ok := b.base.Mapping(typ)
	if !ok || typ != b.typ {
		return m, ok
	}
	return value.Mapping{Parse: func(lexical string, ctx value.Context) (value.Value, error) {
		v, err := m.Parse(lexical, ctx)
		if err != nil {
			return nil, err
		}
		return opaqueValue{v: v}, nil
	}}, true
}

// opaqueValue is a value with neither value.Eq nor value.Identical, the
// backend coverage sameKeyMember declines on.
type opaqueValue struct {
	v value.Value
}

// Two members of different ·primitive· datatypes are neither identical nor
// equal (Datatypes §2.2.1, §2.2.2), so an xs:string "1" and an xs:integer "1"
// are two ·key-sequences· and the unique is satisfied — decided, so nothing is
// recorded. Under the aliased backend, which maps xs:string with xs:decimal's
// mapping, the two VALUES compare equal: only the primitives tell them apart.
func TestKeyMembersOfDifferentPrimitivesAreDistinct(t *testing.T) {
	unique := icDef(t, "U", xsd.IdentityConstraintUnique, ".//item", nil, "", "@id|@k")
	schema := icSchema(t, "", false, []xsd.IdentityConstraint{unique}, nil)
	doc := icRoot(idItem(2, "id", "1"), idItem(3, "k", "1"))

	aliased := aliasedBackend{base: testBackend(), from: icBuiltin("string"), to: icBuiltin("decimal")}
	for _, backend := range []value.Backend{testBackend(), aliased} {
		got, undecided := assessRecordedWith(t, backend, schema, doc)
		wantSilence(t, got, "members of different primitives are distinct")
		wantDeclines(t, icDeclines(undecided))
	}
}

// aliasedBackend is base with from mapped by to's mapping.
type aliasedBackend struct {
	base     value.Backend
	from, to xsd.QName
}

func (b aliasedBackend) Mapping(typ xsd.QName) (value.Mapping, bool) {
	if typ == b.from {
		return b.base.Mapping(b.to)
	}
	return b.base.Mapping(typ)
}

// Two members validated against different types derived from ONE ·primitive·
// are compared in that primitive's value space (Datatypes §2.2.1), so each
// charged pair below is one ·key-sequence· and the unique charges clause 4.1 at
// the later: xs:string and xs:ID; xs:integer "01" and xs:unsignedByte "1", one
// xs:decimal value; and xs:string "a b" and xs:token " a  b ", which xs:token's
// own whiteSpace collapses before the xs:string value is read. The uncharged
// pair is that normalization's other side: xs:string " a b" keeps its space.
func TestKeyMembersOfOnePrimitiveCompareInItsValueSpace(t *testing.T) {
	for _, tc := range []struct {
		fields       string
		first, later [2]string
		charged      bool
	}{
		{"@id|@xid", [2]string{"id", "a"}, [2]string{"xid", "a"}, true},
		{"@k|@ub", [2]string{"k", "01"}, [2]string{"ub", "1"}, true},
		{"@id|@tok", [2]string{"id", "a b"}, [2]string{"tok", " a  b "}, true},
		{"@id|@tok", [2]string{"id", " a b"}, [2]string{"tok", "a b"}, false},
	} {
		unique := icDef(t, "U", xsd.IdentityConstraintUnique, ".//item", nil, "", tc.fields)
		got, undecided := assessRecorded(t, icSchema(t, "", false, []xsd.IdentityConstraint{unique}, nil),
			icRoot(idItem(2, tc.first[0], tc.first[1]), idItem(3, tc.later[0], tc.later[1])))
		wantDeclines(t, icDeclines(undecided))
		if !tc.charged {
			wantSilence(t, got, "distinct xs:string values are two key-sequences")
			continue
		}
		icWantCharges(t, got, icCharge(ruleCvcIdentityConstraint, 3))
		if !strings.Contains(got[0].Error(), "clause 4.1 ") {
			t.Errorf("Violations() = %v, want the charge made under clause 4.1", got)
		}
	}
}

// A list of one item is not distinguished from the atomic value it holds
// (Structures §3.11.4, the paragraph after cvc-identity-constraint), so an
// xs:IDREFS keyref member "a" resolves against an xs:ID unique member "a". A
// list of two items has the length of no atomic value (Datatypes §2.2.1,
// §2.2.2), so "a b" resolves against nothing and clause 4.3 charges it.
func TestSingletonListMatchesAnAtomicKeyMember(t *testing.T) {
	unique := icDef(t, "U", xsd.IdentityConstraintUnique, "item", nil, "", "@xid")
	keyref := icDef(t, "R", xsd.IdentityConstraintKeyref, "item", nil, "U", "@refs")
	schema := icSchema(t, "", false, []xsd.IdentityConstraint{unique, keyref}, nil)
	doc := func(refs string) *testElement {
		return icRoot(idItem(2, "xid", "a"), idItem(3, "xid", "b"), idItem(4, "refs", refs))
	}

	got, undecided := assessRecorded(t, schema, doc("a"))
	wantSilence(t, got, "a singleton list resolves against its atomic item")
	wantDeclines(t, icDeclines(undecided))

	got, undecided = assessRecorded(t, schema, doc("a b"))
	icWantCharges(t, got, icCharge(ruleCvcIdentityConstraint, 4))
	if !strings.Contains(got[0].Error(), "clause 4.3 ") {
		t.Errorf("Violations() = %v, want the charge made under clause 4.3", got)
	}
	wantDeclines(t, icDeclines(undecided))
}

// A union-typed member is compared in the ·primitive· value space of its
// ·validating type· (key-vtype clause 1), the member that accepted its lexical
// (#2111's oracle ruling): a UPlain "01" validated as xs:integer is one
// xs:decimal value with an xs:integer "1", and a UPlain "1" is distinct from an
// xs:string "1" (Datatypes §2.2.1, §2.2.2) while a UPlain "a", validated as
// xs:string, is not distinct from one. UNest reaches its xs:integer and xs:ID
// members through a union member. Every row is decided, so nothing is recorded.
func TestUnionKeyMemberComparesByItsValidatingType(t *testing.T) {
	for _, tc := range []struct {
		fields       string
		first, later [2]string
		charged      bool
	}{
		{"@k|@upl", [2]string{"k", "1"}, [2]string{"upl", "01"}, true},
		{"@k|@upl", [2]string{"k", "1"}, [2]string{"upl", "2"}, false},
		{"@id|@upl", [2]string{"id", "1"}, [2]string{"upl", "1"}, false},
		{"@id|@upl", [2]string{"id", "a"}, [2]string{"upl", "a"}, true},
		{"@k|@unest", [2]string{"k", "1"}, [2]string{"unest", "01"}, true},
		{"@id|@unest", [2]string{"id", "a"}, [2]string{"unest", "a"}, true},
		{"@id|@unest", [2]string{"id", "1"}, [2]string{"unest", "1"}, false},
	} {
		t.Run(tc.first[0]+"="+tc.first[1]+","+tc.later[0]+"="+tc.later[1], func(t *testing.T) {
			unique := icDef(t, "U", xsd.IdentityConstraintUnique, ".//item", nil, "", tc.fields)
			got, undecided := assessRecorded(t, icSchema(t, "", false, []xsd.IdentityConstraint{unique}, nil),
				icRoot(idItem(2, tc.first[0], tc.first[1]), idItem(3, tc.later[0], tc.later[1])))
			wantDeclines(t, icDeclines(undecided))
			if !tc.charged {
				wantSilence(t, got, "members of different primitives, or unequal ones, are two key-sequences")
				return
			}
			icWantClause41(t, got, 3, 2)
		})
	}
}

// icWantClause41 fails unless got is exactly one clause 4.1 charge, made
// against the <item> ·target node· at later for sharing the ·key-sequence· of
// the one at earlier.
func icWantClause41(t *testing.T, got []*xsderr.Error, later, earlier int) {
	t.Helper()
	icWantCharges(t, got, icCharge(ruleCvcIdentityConstraint, later))
	prefix := loc(later, 1).String() + ": [cvc-identity-constraint] element item, a ·target node· of the unique U declared on root, has a ·key-sequence· equal or identical to the one at " + loc(earlier, 1).String() + ","
	if !strings.HasPrefix(got[0].Error(), prefix) || !strings.Contains(got[0].Error(), "clause 4.1 ") {
		t.Errorf("Violations() = %v, want a clause 4.1 charge opening %q", got, prefix)
	}
}

// A list member whose {item type definition} is a union is compared item by
// item, each item in the ·primitive· value space of its own ·validating type·
// (key-vtype clause 2). LUPlain "x 01" holds an xs:string and an xs:integer
// item, so it matches an LURef "x 01" — whose second item URef validates as
// xs:string — in its first item and not its second, and the lists are
// distinct; LUPlain "x y" holds two xs:string items and matches LURef "x y",
// which the xs:IDREF item "x" and xs:IDREF item "y" read as xs:string values,
// and the unique charges clause 4.1. Every row is decided, so nothing is
// recorded.
func TestListOfUnionKeyMemberComparesItemByItem(t *testing.T) {
	unique := icDef(t, "U", xsd.IdentityConstraintUnique, ".//item", nil, "", "@lup|@lur")
	schema := icSchema(t, "", false, []xsd.IdentityConstraint{unique}, nil)
	doc := func(lup, lur string) *testElement {
		return icRoot(idItem(2, "xid", "x"), idItem(3, "xid", "y"), idItem(4, "lup", lup), idItem(5, "lur", lur))
	}

	t.Run("mixed", func(t *testing.T) {
		got, undecided := assessRecorded(t, schema, doc("x 01", "x 01"))
		wantDeclines(t, icDeclines(undecided))
		wantSilence(t, got, "an xs:integer item is distinct from an xs:string one")
	})
	t.Run("strings", func(t *testing.T) {
		got, undecided := assessRecorded(t, schema, doc("x y", "x y"))
		wantDeclines(t, icDeclines(undecided))
		icWantClause41(t, got, 5, 4)
	})
}

// Two members validated against ONE union node are compared by their
// ·validating types· too, never by asking one value about the other: the two
// URef "a" values below are xs:IDREF values in a space of opaqueOver's own,
// with neither value.Eq nor value.Identical, yet both are the xs:string value
// "a", so the unique charges clause 4.1 at the later. An LURef list of the
// same item is the same pair one item deep.
func TestOneUnionNodeComparesInItsMembersPrimitive(t *testing.T) {
	opaque := opaqueOver("IDREF", "string")
	for _, field := range []string{"uref", "lur"} {
		t.Run(field, func(t *testing.T) {
			unique := icDef(t, "U", xsd.IdentityConstraintUnique, ".//item", nil, "", "@"+field)
			got, undecided := assessRecordedWith(t, opaque, icSchema(t, "", false, []xsd.IdentityConstraint{unique}, nil),
				icRoot(idItem(2, "xid", "a"), idItem(3, field, "a"), idItem(4, field, "a")))
			wantDeclines(t, icDeclines(undecided))
			icWantClause41(t, got, 4, 3)
		})
	}
}

// icTypedFieldSchema declares <root> over item*, each item over an optional
// <cx> whose declaration is untyped (xs:anyType, mixed) and an optional <ec>
// of an anonymous complex type with empty content, both {nillable} true, then
// an optional xs:string <s>, with one identity constraint of the given
// category over the selector "item" and the one field given.
func icTypedFieldSchema(t *testing.T, category, field string) *xsd.Schema {
	t.Helper()
	return parsedSchema(t, map[string]string{"main.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="root">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="item" maxOccurs="unbounded">
          <xs:complexType>
            <xs:sequence>
              <xs:element name="cx" nillable="true" minOccurs="0"/>
              <xs:element name="ec" nillable="true" minOccurs="0"><xs:complexType/></xs:element>
              <xs:element name="s" type="xs:string" minOccurs="0"/>
            </xs:sequence>
          </xs:complexType>
        </xs:element>
      </xs:sequence>
    </xs:complexType>
    <xs:` + category + ` name="C"><xs:selector xpath="item"/><xs:field xpath="` + field + `"/></xs:` + category + `>
  </xs:element>
</xs:schema>`})
}

// icTypedField is <root><item><local .../></item></root>, the field node at
// line 3 carrying attrs, with xs bound to the XSD namespace for an xsi:type.
func icTypedField(local string, attrs ...Attribute) *testElement {
	node := icElem(xsd.QName{Local: local}, 3, attrs)
	node.bindings = map[string]string{"xs": xsd.XMLSchemaNS}
	return icRoot(icElem(xsd.QName{Local: "item"}, 2, nil, ElementChild(node)))
}

// A field node whose ·governing type definition· is determined and is neither
// a simple type definition nor a complex type definition with {variety} simple
// is one of cvc-identity-constraint clause 3's "other nodes", ·nilled· or not,
// for a unique and a key alike: it is charged once, at the field node, and
// nothing is recorded as undecided — clause 3's list is closed and reads only
// the governing type. Without fill's split each charged row declines instead.
//
// The xsi:type row is idF018's instance 1: an ·override· to xs:string makes a
// ·nilled· <cx> simple-valued, so it shortens the unique's ·key-sequence· and
// is charged nothing — the check reads the governing type, not the
// declaration's.
func TestFieldNodeOfNonSimpleTypeIsChargedUnderClause3(t *testing.T) {
	xsiNil := icAttr(xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: "nil"}, "true", 3)
	xsiString := icAttr(xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: "type"}, "xs:string", 3)
	charged := []struct {
		name, category, field string
		doc                   *testElement
	}{
		{"unique over anyType", "unique", "cx", icTypedField("cx")},
		{"unique over nilled anyType", "unique", "cx", icTypedField("cx", xsiNil)},
		{"key over anyType", "key", "cx", icTypedField("cx")},
		{"key over nilled anyType", "key", "cx", icTypedField("cx", xsiNil)},
		{"unique over empty content", "unique", "ec", icTypedField("ec")},
		{"key over nilled empty content", "key", "ec", icTypedField("ec", xsiNil)},
	}
	for _, c := range charged {
		t.Run(c.name, func(t *testing.T) {
			got, undecided := assessRecorded(t, icTypedFieldSchema(t, c.category, c.field), c.doc)
			icWantCharges(t, got, icCharge(ruleCvcIdentityConstraint, 3))
			want := `instance.xml:3:1: [cvc-identity-constraint] element item, a ·target node· of the identity constraint C declared on root, has its field "` + c.field + `" select a node whose ·governing type definition· is neither`
			if len(got) == 1 && !strings.HasPrefix(got[0].Error(), want) {
				t.Errorf("Violations()[0] = %q, want it to open %q", got[0].Error(), want)
			}
			wantDeclines(t, undecided)
		})
	}
	// The first clause 3 charge a slot takes is the one reported, at its own
	// node: a valued <s> after the <cx> neither moves it nor adds a second.
	t.Run("unique over anyType then a valued node", func(t *testing.T) {
		s := icElem(xsd.QName{Local: "s"}, 4, nil, TextChild(&testText{data: "a", loc: loc(4, 4)}))
		doc := icRoot(icElem(xsd.QName{Local: "item"}, 2, nil,
			ElementChild(icElem(xsd.QName{Local: "cx"}, 3, nil)), ElementChild(s)))
		got, undecided := assessRecorded(t, icTypedFieldSchema(t, "unique", "*"), doc)
		icWantCharges(t, got, icCharge(ruleCvcIdentityConstraint, 3))
		wantDeclines(t, undecided)
	})
	t.Run("unique over nilled xsi:type xs:string", func(t *testing.T) {
		got, undecided := assessRecorded(t, icTypedFieldSchema(t, "unique", "cx"), icTypedField("cx", xsiNil, xsiString))
		wantSilence(t, got, "a nilled node of a simple governing type only shortens the key-sequence")
		wantDeclines(t, undecided)
	})
}

// icSpecialSchema declares <root> over item*, each <item> carrying an untyped
// @ast (xs:anySimpleType, §3.2.2.2), an @aat of xs:anyAtomicType, an xs:string
// @s and an untyped @r, over an optional <v> of xs:anySimpleType, with the
// identity constraints ics declared on <root>.
func icSpecialSchema(t *testing.T, ics string) *xsd.Schema {
	t.Helper()
	return parsedSchema(t, map[string]string{"main.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="root">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="item" maxOccurs="unbounded">
          <xs:complexType>
            <xs:sequence><xs:element name="v" type="xs:anySimpleType" minOccurs="0"/></xs:sequence>
            <xs:attribute name="ast"/>
            <xs:attribute name="aat" type="xs:anyAtomicType"/>
            <xs:attribute name="s" type="xs:string"/>
            <xs:attribute name="r"/>
          </xs:complexType>
        </xs:element>
      </xs:sequence>
    </xs:complexType>
    ` + ics + `
  </xs:element>
</xs:schema>`})
}

// icSpecialUnique is a unique named U over item with the one field given.
func icSpecialUnique(field string) string {
	return `<xs:unique name="U"><xs:selector xpath="item"/><xs:field xpath="` + field + `"/></xs:unique>`
}

// Two ·key-sequence· members of the ·special· types whose lexicals are
// byte-identical are SAME (#2124's oracle ruling: SAME under the xs:string
// member of the lexical mapping's union, Datatypes §3.2.1.2, §3.2.2.2), so the
// unique charges clause 4.1 at the later ·target node· and records nothing —
// either type against itself and against the other, the first row the shape of
// MS-Element2006-07-15 elemZ015.i. Before #2124 each row declined at the field
// node, charging nothing and recording a clause 3 decline per member.
func TestByteIdenticalSpecialKeyMembersAreSame(t *testing.T) {
	for _, tc := range []struct {
		fields       string
		first, later [2]string
	}{
		{"@ast", [2]string{"ast", "true"}, [2]string{"ast", "true"}},
		{"@aat", [2]string{"aat", "true"}, [2]string{"aat", "true"}},
		{"@ast|@aat", [2]string{"ast", "1.0"}, [2]string{"aat", "1.0"}},
	} {
		t.Run(tc.fields+":"+tc.first[0]+","+tc.later[0], func(t *testing.T) {
			got, undecided := assessRecorded(t, icSpecialSchema(t, icSpecialUnique(tc.fields)),
				icRoot(idItem(2, tc.first[0], tc.first[1]), idItem(3, tc.later[0], tc.later[1])))
			wantDeclines(t, icDeclines(undecided))
			icWantClause41(t, got, 3, 2)
		})
	}
	// An ELEMENT field node reaches the same comparison through
	// elementKeyMember: a key charges clause 4.2.2 at the later.
	t.Run("key over an element", func(t *testing.T) {
		valued := func(line int, text string) *testElement {
			v := icElem(xsd.QName{Local: "v"}, line+1, nil, TextChild(&testText{data: text, loc: loc(line+1, 4)}))
			return icElem(xsd.QName{Local: "item"}, line, nil, ElementChild(v))
		}
		key := `<xs:key name="K"><xs:selector xpath="item"/><xs:field xpath="v"/></xs:key>`
		got, undecided := assessRecorded(t, icSpecialSchema(t, key), icRoot(valued(2, "x"), valued(4, "x")))
		wantDeclines(t, icDeclines(undecided))
		icWantCharges(t, got, icCharge(ruleCvcIdentityConstraint, 4))
		if !strings.Contains(got[0].Error(), "clause 4.2.2 ") {
			t.Errorf("Violations() = %v, want the charge made under clause 4.2.2", got)
		}
	})
}

// Every other pair with a ·special· member stays undecided (#2124's ruling) and
// declines, charging nothing: "1" and "1.0" are equal under xs:decimal and
// unequal under xs:string, and a ·special· member against an xs:string one has
// no one value space to be compared in. The decline is recorded against the
// later ·target node·, and is never a NOT-same: a keyref member "1.0" against a
// unique member "1" records clause 4.3 as undecided rather than charging it.
func TestOtherSpecialKeyMemberPairsDecline(t *testing.T) {
	for _, tc := range []struct {
		fields       string
		first, later [2]string
	}{
		{"@ast", [2]string{"ast", "1"}, [2]string{"ast", "1.0"}},
		{"@ast|@aat", [2]string{"ast", "1"}, [2]string{"aat", "01"}},
		{"@ast|@s", [2]string{"ast", "a"}, [2]string{"s", "a"}},
		{"@s|@aat", [2]string{"s", "a"}, [2]string{"aat", "a"}},
	} {
		t.Run(tc.fields+":"+tc.first[1]+","+tc.later[1], func(t *testing.T) {
			got, undecided := assessRecorded(t, icSpecialSchema(t, icSpecialUnique(tc.fields)),
				icRoot(idItem(2, tc.first[0], tc.first[1]), idItem(3, tc.later[0], tc.later[1])))
			wantSilence(t, got, "an undecided special pair charges nothing")
			wantDeclines(t, icDeclines(undecided),
				Unevaluated{rule: ruleCvcIdentityConstraint, loc: loc(3, 1), msg: "clause 4.1 is undecided"})
		})
	}
	t.Run("keyref", func(t *testing.T) {
		ics := icSpecialUnique("@ast") +
			`<xs:keyref name="R" refer="U"><xs:selector xpath="item"/><xs:field xpath="@r"/></xs:keyref>`
		schema := icSpecialSchema(t, ics)

		got, undecided := assessRecorded(t, schema, icRoot(idItem(2, "ast", "1"), idItem(3, "r", "1")))
		wantSilence(t, got, "a byte-identical special keyref member resolves")
		wantDeclines(t, icDeclines(undecided))

		got, undecided = assessRecorded(t, schema, icRoot(idItem(2, "ast", "1"), idItem(3, "r", "1.0")))
		wantSilence(t, got, "an undecided special lookup charges nothing")
		wantDeclines(t, icDeclines(undecided),
			Unevaluated{rule: ruleCvcIdentityConstraint, loc: loc(3, 1), msg: "clause 4.3 is undecided"})
	})
}

// icCountSchema declares <root> over item*, each <item> carrying xs:integer
// attributes @a and @b over any number of nillable xs:integer <v>, with one
// identity constraint of the given category, named C, over the selector "item"
// and the one field given.
func icCountSchema(t *testing.T, category, field string) *xsd.Schema {
	t.Helper()
	return parsedSchema(t, map[string]string{"main.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="root">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="item" maxOccurs="unbounded">
          <xs:complexType>
            <xs:sequence>
              <xs:element name="v" type="xs:integer" nillable="true" minOccurs="0" maxOccurs="unbounded"/>
            </xs:sequence>
            <xs:attribute name="a" type="xs:integer"/>
            <xs:attribute name="b" type="xs:integer"/>
          </xs:complexType>
        </xs:element>
      </xs:sequence>
    </xs:complexType>
    <xs:` + category + ` name="C"><xs:selector xpath="item"/><xs:field xpath="` + field + `"/></xs:` + category + `>
  </xs:element>
</xs:schema>`})
}

// icCountValues is <root><item> at line 2 over one <v> per entry of vs, the
// k-th at line 3+k: the entry "nil" is a ·nilled· <v>, any other its text.
func icCountValues(vs ...string) *testElement {
	kids := make([]Child, 0, len(vs))
	for k, s := range vs {
		line := 3 + k
		if s == "nil" {
			xsiNil := icAttr(xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: "nil"}, "true", line)
			kids = append(kids, ElementChild(icElem(xsd.QName{Local: "v"}, line, []Attribute{xsiNil})))
			continue
		}
		kids = append(kids, ElementChild(icElem(xsd.QName{Local: "v"}, line, nil,
			TextChild(&testText{data: s, loc: loc(line, 4)}))))
	}
	return icRoot(icElem(xsd.QName{Local: "item"}, 2, nil, kids...))
}

// icCountAttributes is <root><item/></root> carrying each attribute given, the
// k-th at line 2+k, so a charge names which one it was made against.
func icCountAttributes(names []string, values ...string) *testElement {
	attrs := make([]Attribute, 0, len(names))
	for k, n := range names {
		attrs = append(attrs, icAttr(xsd.QName{Local: n}, values[k], 2+k))
	}
	return icRoot(icElem(xsd.QName{Local: "item"}, 2, attrs))
}

// icWantClause3Count fails unless got is the charges before, then exactly one
// clause 3 charge for more than one simple-valued node, made against the field
// node at at and opening with its subject — the field, the constraint C, its
// host <root> and the ·target node· <item>.
func icWantClause3Count(t *testing.T, got []*xsderr.Error, field string, at xsderr.Loc, before ...struct {
	rule xsderr.Rule
	loc  xsderr.Loc
},
) {
	t.Helper()
	icWantCharges(t, got, append(before, icChargeAt(ruleCvcIdentityConstraint, at))...)
	prefix := at.String() + `: [cvc-identity-constraint] element item, a ·target node· of the identity constraint C declared on root, has its field "` + field + `" select more than one node whose ·governing type definition· is a simple type definition`
	if last := got[len(got)-1].Error(); !strings.HasPrefix(last, prefix) {
		t.Errorf("Violations()[%d] = %q, want it to open %q", len(got)-1, last, prefix)
	}
}

// cvc-identity-constraint clause 3 counts a field's nodes by ·governing type
// definition· (#2121's oracle ruling): it admits "at most one node whose
// ·governing· type definition is either a simple type definition or a complex
// type definition with {variety} simple", and a ·nilled· node (key-nilled) or
// one whose lexical lies outside its lexical space keeps its governing type —
// §3.3.5.4 makes only its [schema actual value] ·absent·. So a second such node
// is charged at its own position whatever either value is, through element and
// attribute field nodes alike, and nothing is recorded as undecided. The
// invalid lexical "x" is also charged at its own node, by the rule that owns
// it; that charge is listed before the clause 3 one.
func TestClause3CountsSimpleValuedNodesWhateverTheirValues(t *testing.T) {
	for _, tc := range []struct {
		name string
		vs   []string
	}{
		{"two nilled", []string{"nil", "nil"}},
		{"nilled then valued", []string{"nil", "1"}},
		{"valued then nilled", []string{"1", "nil"}},
		{"absent then valued", []string{"x", "1"}},
		{"valued then absent", []string{"1", "x"}},
		{"two absent", []string{"x", "x"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, undecided := assessRecorded(t, icCountSchema(t, "unique", "v"), icCountValues(tc.vs...))
			wantDeclines(t, icDeclines(undecided))
			var before []struct {
				rule xsderr.Rule
				loc  xsderr.Loc
			}
			for k, s := range tc.vs {
				if s == "x" {
					before = append(before, icCharge(ruleCvcType, 3+k))
				}
			}
			icWantClause3Count(t, got, "v", loc(4, 1), before...)
		})
	}
	// An attribute field node reaches the same count through keyMember: an
	// ·absent·-valued @a first still charges the valued @b after it, at @b.
	for _, tc := range []struct {
		name    string
		a, b    string
		invalid int // the line of the attribute holding "x"
	}{
		{"absent then valued attribute", "x", "1", 2},
		{"valued then absent attribute", "1", "x", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, undecided := assessRecorded(t, icCountSchema(t, "unique", "@a|@b"),
				icCountAttributes([]string{"a", "b"}, tc.a, tc.b))
			wantDeclines(t, icDeclines(undecided))
			icWantClause3Count(t, got, "@a|@b", loc(3, 2), icChargeAttr(ruleCvcAttribute, tc.invalid))
		})
	}
}

// The one simple-valued node clause 3 admits is admitted whatever its value: a
// lone ·nilled· or ·absent·-valued field node is charged nothing under clause
// 3 and contributes no ·key-sequence· member, so a unique charges nothing and a
// key charges its ·target node· under clause 4.2.1 for the short
// ·key-sequence·.
func TestClause3AdmitsOneSimpleValuedNodeWhateverItsValue(t *testing.T) {
	want421 := func(t *testing.T, got []*xsderr.Error) {
		t.Helper()
		if len(got) == 2 && !strings.Contains(got[1].Error(), "clause 4.2.1 ") {
			t.Errorf("Violations() = %v, want the key charged under clause 4.2.1", got)
		}
	}
	t.Run("unique over one nilled", func(t *testing.T) {
		got, undecided := assessRecorded(t, icCountSchema(t, "unique", "v"), icCountValues("nil"))
		wantDeclines(t, icDeclines(undecided))
		wantSilence(t, got, "one nilled node is within clause 3's bound")
	})
	t.Run("unique over one absent attribute", func(t *testing.T) {
		got, undecided := assessRecorded(t, icCountSchema(t, "unique", "@a|@b"), icCountAttributes([]string{"a"}, "x"))
		wantDeclines(t, icDeclines(undecided))
		icWantCharges(t, got, icChargeAttr(ruleCvcAttribute, 2))
	})
	t.Run("key over one absent attribute", func(t *testing.T) {
		got, undecided := assessRecorded(t, icCountSchema(t, "key", "@a|@b"), icCountAttributes([]string{"a"}, "x"))
		wantDeclines(t, icDeclines(undecided))
		icWantCharges(t, got, icChargeAttr(ruleCvcAttribute, 2), icCharge(ruleCvcIdentityConstraint, 2))
		want421(t, got)
	})
	t.Run("key over one absent element", func(t *testing.T) {
		got, undecided := assessRecorded(t, icCountSchema(t, "key", "v"), icCountValues("x"))
		wantDeclines(t, icDeclines(undecided))
		icWantCharges(t, got, icCharge(ruleCvcType, 3), icCharge(ruleCvcIdentityConstraint, 2))
		want421(t, got)
	})
}

// Two ·special·-typed (xs:anySimpleType, xs:anyAtomicType) field nodes for one
// ·target node· are two nodes of a simple ·governing type definition·, so
// clause 3 charges the second at its own position, as icClause3Valued itself.
// The two lexicals are byte-identical and differ by position only, so the
// charge is the count's and no comparison's. Before #2124, keyMember declined
// each such node instead, recording a clause 3 decline and charging nothing.
func TestTwoSpecialFieldNodesAreChargedUnderClause3(t *testing.T) {
	for _, tc := range []struct{ field, second string }{
		{"@ast|@aat", "aat"},
		{"@ast|@r", "r"},
	} {
		t.Run(tc.field, func(t *testing.T) {
			unique := `<xs:unique name="C"><xs:selector xpath="item"/><xs:field xpath="` + tc.field + `"/></xs:unique>`
			got, undecided := assessRecorded(t, icSpecialSchema(t, unique),
				icCountAttributes([]string{"ast", tc.second}, "x", "x"))
			wantDeclines(t, icDeclines(undecided))
			icWantClause3Count(t, got, tc.field, loc(3, 2))
		})
	}
}
