package jsonsrc

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kud360/goxsd8/internal/xmlname"
	"github.com/kud360/goxsd8/internal/xmlqname"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// The rules of the charges a note answers, and that validate's
// contentCheck.element charges a parent's content under (contentCharge): a
// characters note answers cvc-complex-type, a scalarExpected cvc-type, an
// unnamed and an arrayItem cvc-complex-content, a notOverriding and a
// nonNillable cvc-elt, and a typeLexical cvc-attribute.
const (
	ruleCvcType           xsderr.Rule = "cvc-type"
	ruleCvcComplexType    xsderr.Rule = "cvc-complex-type"
	ruleCvcComplexContent xsderr.Rule = "cvc-complex-content"
	ruleCvcElt            xsderr.Rule = "cvc-elt"
	ruleCvcAttribute      xsderr.Rule = "cvc-attribute"
)

// xmlWhitespace is white space as XML defines it (S ::= (#x20 | #x9 | #xD |
// #xA)+), the text cvc-complex-type clause 1.3 leaves uncharged under an
// element-only {content type}. It restates validate's xmlWhitespace across the
// package boundary, which exports no such test.
const xmlWhitespace = " \t\r\n"

// noteAt is the charge a note answers: its rule and its location.
type noteAt struct {
	loc  xsderr.Loc
	rule xsderr.Rule
}

// note is what the mapping knows about one member, in the instance's own
// terms, kept at the charge it answers (noteAt): hint is the clause [Validate]
// appends to that charge's Msg, and answers which of the violations at its
// rule and location, in the result's order, that charge is (walker.annotate).
// Its arms are characters, scalarExpected, unnamed, arrayItem, notOverriding,
// nonNillable and typeLexical.
type note interface {
	hint() string
	answers() chargeOrder
}

// chargeOrder is which of the violations at a note's rule and location the
// charge it answers is: the first or the last, in the result's order.
type chargeOrder uint8

const (
	firstCharge chargeOrder = iota
	lastCharge
)

// characters is a scalar member whose element the engine charges
// cvc-complex-type for the scalar's text, kept at that charge's rule and
// location (walker.noteCharacters): key is the member's key as spelled, an
// array item's being its array's, lexical the scalar's lexical form, and
// variety the {content type}.{variety} of the element's ·governing type
// definition·, empty or element-only.
type characters struct {
	key, lexical string
	variety      xsd.ContentTypeVariety
}

// hint is the clause [Validate] appends to the charge c answers: a JSON object
// spells an element's element content or its empty content, and a scalar
// spells text.
func (c characters) hint() string {
	return fmt.Sprintf("; JSON member %q has the scalar value %q, but the type of its element has %s content: object expected, {%q:{...}}",
		c.key, c.lexical, c.variety, c.key)
}

// answers is the last: the element's text charge follows the parent's charges
// at an array item's location, which is its scalar's (walker.annotate).
func (characters) answers() chargeOrder { return lastCharge }

// scalarExpected is the first child member of the object of an element whose
// every type that may govern it is a simple type, kept at the cvc-type clause
// 3.1.2 charge the parent takes there (walker.noteChild): key is the member's
// key as spelled, an array item's being its array's, and owner the parent's.
type scalarExpected struct {
	key, owner string
}

// hint answers the parent's cvc-type clause 3.1.2 charge: a scalar spells a
// simple type's text, and an object spells element children.
func (s scalarExpected) hint() string {
	return fmt.Sprintf("; JSON member %q is a member of the object of %q, whose type is a simple type: scalar expected, {%q:<string, number or boolean>}",
		s.key, s.owner, s.owner)
}

// answers is the first: the parent's charge at a child precedes the child's
// own there (walker.annotate).
func (scalarExpected) answers() chargeOrder { return firstCharge }

// unnamed is a child member that no child element declaration and no
// attribute use of its parent's type names, kept at the cvc-complex-content
// charge the parent's content model makes there (walker.noteChild): key is
// the member's key as spelled, an array item's being its array's, owner the
// parent's, keys the unprefixed keys that map to the attribute uses and child
// element declarations the parent's type carries (walker.mapsKey), each quoted
// as a key spells it, and attribute whether key is "_" before the local name
// of an attribute use the mapping names by that local name alone.
type unnamed struct {
	key, owner string
	keys       []string
	attribute  bool
}

// hint answers the parent's cvc-complex-content charge: it lists the keys the
// type carries and, for an attribute key behind "_", names the key that names
// the attribute.
func (u unnamed) hint() string {
	hint := fmt.Sprintf("; JSON member %q names no child element declaration and no attribute use of the type of %q", u.key, u.owner)
	if len(u.keys) > 0 {
		hint += ", whose members are keyed " + strings.Join(u.keys, ", ")
	}
	if u.attribute {
		hint += fmt.Sprintf("; an attribute member is keyed by the attribute's own name, %q, not %q", u.key[1:], u.key)
	}
	return hint
}

// answers is the first, as scalarExpected's is.
func (unnamed) answers() chargeOrder { return firstCharge }

// arrayItem is an item of an array-valued member, kept at the
// cvc-complex-content charge the parent's content model makes there where no
// unnamed note is kept (walker.noteChild): key is the array's key as spelled,
// and item the item's place in the array, from 1.
type arrayItem struct {
	key  string
	item int
}

// hint answers the parent's cvc-complex-content charge: an array spells one
// element per item, so the element charged is one item of it.
func (a arrayItem) hint() string {
	return fmt.Sprintf("; JSON member %q is an array, each of whose items is one element of that name: the element charged is item %d of it",
		a.key, a.item)
}

// answers is the first, as scalarExpected's is: the item's own
// cvc-complex-content charges at its location, an object item's at its '{',
// follow the parent's.
func (arrayItem) answers() chargeOrder { return firstCharge }

// notOverriding is an object member whose "$type" names a type that does not
// ·override· the ·selected type definition· of the element's ·governing
// element declaration·, kept at the cvc-elt clause 4 charge the engine makes
// at the element (walker.noteOverride): key is the member's
// key as spelled, an array item's being its array's, and lexical the
// "$type"'s value.
type notOverriding struct {
	key, lexical string
}

// hint answers the element's cvc-elt clause 4 charge: "$type" spells xsi:type.
func (n notOverriding) hint() string {
	return fmt.Sprintf("; JSON member \"$type\" of %q spells its xsi:type, %q: the type it names governs the element only where it ·overrides· the type the element's declaration selects, so name one that does, or drop the \"$type\"",
		n.key, n.lexical)
}

// answers is the first: validate's walk charges clause 4 as it settles the
// element's governance (walk.instanceOverride), before its clause 2 charge
// (walk.abstractDeclaration) and its clause 5 charges, which follow its
// [[children]], and a clause 1 charge leaves no clause 4 one
// (validate.Validator.Governing); clause 3 reads an xsi:nil, which only a null
// member carries, and no charge of the parent's at the element's location is
// a cvc-elt one, the parent being no null.
func (notOverriding) answers() chargeOrder { return firstCharge }

// nonNillable is a null member whose element's ·governing element
// declaration· is not nillable, kept at the cvc-elt clause 3.1 charge the
// engine makes at the element (walker.noteNil): key is the member's key as
// spelled, an array item's being its array's.
type nonNillable struct {
	key string
}

// hint answers the element's cvc-elt clause 3.1 charge: null spells
// xsi:nil="true".
func (n nonNillable) hint() string {
	return fmt.Sprintf("; JSON member %q is null, which spells xsi:nil=\"true\", but the declaration of its element is not nillable: give it a value",
		n.key)
}

// answers is the last: validate's walk charges clause 2
// (walk.abstractDeclaration) before clause 3.1 (walk.nilCheck), and no
// clause after it. Clause 5 is charged after the [[children]], and none of
// its arms is charged at a null: an element that is not ·nilled· and has no
// [[children]] takes clause 5.1 wherever its declaration has a {value
// constraint}, and clause 5.1.1, its one cvc-elt charge, reads an
// ·instance-specified type definition· (contentCheck.defaultValid), which
// only an object's "$type" presents. A clause 1 charge leaves no clause 3.1
// one, clause 4 reads an xsi:type too, and no charge of the parent's at the
// element's location is a cvc-elt one, the parent being no null.
func (nonNillable) answers() chargeOrder { return lastCharge }

// typeLexical is a "$type" member whose value has no xs:QName ·actual value·,
// kept at the cvc-attribute clause 3 charge the engine makes at its key
// (walker.noteTypeLexical): key is the key of the member whose object it
// leads, as spelled, an array item's being its array's, and prefix the
// value's prefix where that is the one fault, no "$xmlns" in scope binding
// it, and empty otherwise.
type typeLexical struct {
	key, prefix string
}

// hint answers the cvc-attribute clause 3 charge: "$type" spells xsi:type,
// whose value is an xs:QName, and a prefix is bound by a "$xmlns" member.
func (t typeLexical) hint() string {
	if t.prefix == "" {
		return fmt.Sprintf("; JSON member \"$type\" of %q spells its xsi:type, an xs:QName: give it as \"local\" or \"p:local\", each part an NCName",
			t.key)
	}
	return fmt.Sprintf("; JSON member \"$type\" of %q spells its xsi:type, an xs:QName, and no \"$xmlns\" in scope binds its prefix %q: bind it in a \"$xmlns\" member leading this object or one enclosing it, {\"$xmlns\":{%q:\"<namespace name>\"}}",
		t.key, t.prefix, t.prefix)
}

// answers is the first, and the only: the engine charges an xsi:type under
// cvc-attribute clause 3 or clause 5, never both (walk.instanceTypeResolves),
// and every other attribute stands at a key of its own.
func (typeLexical) answers() chargeOrder { return firstCharge }

// noteChild keeps a note at loc for the child member key that m classifies
// under in's object, its value's scope sc, standing at at, where every type
// that may govern the parent (object.governing) charges the parent's content
// there under one rule (placement.parentCharge): a scalarExpected where that
// is cvc-type, every such type being a simple type, and where it is
// cvc-complex-content an unnamed where walker.unnamedNote keeps one, and
// otherwise, at an item of an array, an arrayItem. Each parent's content is
// charged at one child at most (object.attribute), so the notes kept grow with
// the parents charged, not with the document (STYLE P4).
//
// GAP(jsonsrc): no note is kept where the mapping attributes the child to
// nothing (placement.attributed): its parent's content model, or one above it,
// is one xsd.Schema.ContentMatcher declines (object.unattributed, its GAP(xsd)
// marker, the maxPartitionStates ceiling RULED permanent there). validate
// records its decline where the parent's own model is the one declined
// (contentCheck.match) and walks the XML twin's subtree undecided below it
// (its GAP(validate) marker in walk.child), and charges the parent's content
// at no child, so walker.annotate, the note's one consumer, leaves no charge
// the engine makes without a clause it would carry. Owned by #2670, which
// retires with #1601's ruling.
//
// GAP(jsonsrc): no note is kept where the types that may govern the parent
// differ on the charge at the child, or, for an unnamed, on the keys they
// carry (walker.unnamedNote): several types may govern it only where a {type
// table} at or above it has a selection the engine withholds, a {test} it
// reaches being outside its §3.12.6 compiler's subset, or one the mapping
// does not read (walker.selector), and the engine walks such a parent
// undecided and charges its content at no child, so walker.annotate, the
// note's one consumer, leaves no charge the engine makes without a clause it
// would carry. RULED permanent by #2702: selecting past a withheld
// key-cta-select alternative is unsound, as walker.classify's marker of the
// same ruling says.
func (w *walker) noteChild(m elementMember, key string, loc xsderr.Loc, sc *scope, in *members, at placement) {
	rule, ok := at.parentCharge()
	if !ok {
		return
	}
	if rule == ruleCvcType {
		w.notes[noteAt{loc: loc, rule: rule}] = scalarExpected{key: key, owner: in.owner.key}
		return
	}
	if rule != ruleCvcComplexContent {
		return
	}
	if u, ok := w.unnamedNote(key, loc, sc, in); ok {
		w.notes[noteAt{loc: loc, rule: rule}] = u
		return
	}
	if in.run != nil {
		w.notes[noteAt{loc: loc, rule: rule}] = arrayItem{key: key, item: in.run.items}
	}
}

// unnamedNote is the unnamed note for the child member key, at loc, its value's
// scope sc, of in's object, o, whose parent's content is charged
// cvc-complex-content there, and false where none is kept. One is kept where
// every type that may govern the parent (object.governing) has no element
// wildcard (object.carried), carries the same keys, the expanded names of its
// attribute uses and child element declarations, as each other one, and
// classifies the key as a child none of its declarations names
// (walker.memberOf, declaredElement.named, bareElement), which no type with
// an {attribute wildcard} does: walker.wildcardMember classifies a key no
// declaration or use carries as an attribute under one, or declines it. The
// keys that map to those names (walker.mapsKey) are then the list, in the
// first type's order, each local name being one key's, since memberOf
// declines an unprefixed key two of them carry. The note names the attribute
// use behind a key "_" before its local name where walker.attributeKeyed
// names that use under o, as the mapping classifies the members, and the same
// one under every such type.
func (w *walker) unnamedNote(key string, loc xsderr.Loc, sc *scope, in *members) (unnamed, bool) {
	o := in.object
	k, err := w.keyName(key, o, sc, false, loc)
	if err != nil {
		return unnamed{}, false
	}
	var names []xsd.QName
	governing := make([]*object, 0, len(o.governing))
	for i, gt := range o.governing {
		g := w.object(o.owner, gt.typ)
		carried, ok := g.carried()
		if !ok || (i > 0 && !sameNames(names, carried)) {
			return unnamed{}, false
		}
		m, err := w.memberOf(g, key, k, loc)
		if err != nil || !namesNone(m) {
			return unnamed{}, false
		}
		names = carried
		governing = append(governing, g)
	}
	u := unnamed{key: key, owner: in.owner.key}
	for _, n := range names {
		if w.mapsKey(governing[0], n.Local, loc) {
			u.keys = append(u.keys, fmt.Sprintf("%q", n.Local))
		}
	}
	if len(key) < 2 || key[0] != '_' {
		return u, true
	}
	name, ok := w.attributeKeyed(o, key[1:], loc)
	if !ok {
		return u, true
	}
	for _, g := range governing {
		if n, ok := w.attributeKeyed(g, key[1:], loc); !ok || n != name {
			return u, true
		}
	}
	u.attribute = true
	return u, true
}

// carried is the expanded names of o's attribute uses and then of its child
// element declarations, each once, in that order, and false where o has an
// element wildcard, which names children beyond them.
func (o *object) carried() ([]xsd.QName, bool) {
	if len(o.elementWildcards) > 0 {
		return nil, false
	}
	var names []xsd.QName
	for _, u := range o.uses {
		names = appendName(names, u.DeclarationName())
	}
	for _, d := range o.contained {
		names = appendName(names, d.Name())
	}
	return names, true
}

// mapsKey reports whether the unprefixed key local, at loc, maps under g to an
// attribute use or a child element declaration g carries, as walker.memberOf
// classifies it: false where memberOf declines it — local names a use in the
// xsi namespace, or a use of a type with simple content, or uses or
// declarations of two expanded names, or both a use and a declaration — or
// classifies it as a child no declaration of g names.
func (w *walker) mapsKey(g *object, local string, loc xsderr.Loc) bool {
	m, err := w.memberOf(g, local, keyName{name: xsd.QName{Local: local}}, loc)
	return err == nil && !namesNone(m)
}

// appendName appends n to names unless it is there already.
func appendName(names []xsd.QName, n xsd.QName) []xsd.QName {
	if slices.Contains(names, n) {
		return names
	}
	return append(names, n)
}

// sameNames reports whether a and b, each holding a name once, hold the same
// names, in whatever order.
func sameNames(a, b []xsd.QName) bool {
	if len(a) != len(b) {
		return false
	}
	for _, n := range a {
		if !slices.Contains(b, n) {
			return false
		}
	}
	return true
}

// namesNone reports whether m is a child no child element declaration of the
// type it is classified under names: one walker.namedChild names, under the
// top-level declaration of its name or none.
func namesNone(m member) bool {
	switch m := m.(type) {
	case declaredElement:
		return m.named
	case bareElement:
		return true
	case attributeMember:
		return false
	}
	return false
}

// noteOverride keeps a notOverriding note at e's location for e, whose
// "$type" typ names instance, where the engine charges cvc-elt clause 4
// there: validate.Validator.Governing answers e a ·governing element
// declaration·, as declared says, and a type t that governs e and is not
// instance. instance, a named type, is the type Governing answers exactly
// where it ·overrides· the ·selected type definition·, so t is that selected
// type, which instance does not ·override· under the declaration's
// {disallowed substitutions} (key-overrides, walk.instanceOverride). Where
// the declaration carries a {type table} whose selection the mapping does not
// read, or the engine withholds it, or an ·absent· component sends e to
// cvc-elt clause 1 instead (§5.3), Governing answers no type or no
// declaration, and the engine charges no clause 4. Its callers are
// declaredElement.object, with the answer read with no selection
// (walker.governedAt), and walker.objectElement, with the one walker.overrode
// read the selection for.
func (w *walker) noteOverride(e *element, typ *attribute, declared bool, instance, t xsd.TypeDefinition) {
	if !declared || t == nil || sameNamed(t, instance) {
		return
	}
	w.notes[noteAt{loc: e.loc, rule: ruleCvcElt}] = notOverriding{key: e.key, lexical: typ.value}
}

// noteNil keeps a nonNillable note at loc, the location of the null-valued
// element m classifies under the member key, standing at at, where the engine
// charges cvc-elt clause 3.1 there: the declaration walker.governingDeclaration
// finds governing it is not nillable, and every type it can select resolves
// (walker.declarationType), so that no cvc-elt clause 1 charge takes the
// element to ·lax assessment· instead (validate.Validator.Governing). Clause
// 3.1 reads the declaration's {nillable} alone, whatever type governs the
// element (walk.nilCheck), so a nillable declaration, whose ·nilled· element
// the engine charges under clause 3.2.3.2 where it is fixed, keeps none.
func (w *walker) noteNil(m elementMember, key string, loc xsderr.Loc, at placement) {
	d, ok := m.(declaredElement)
	if !ok {
		return
	}
	gd, ok := w.governingDeclaration(d.decl.Name(), at)
	if !ok || gd.Nillable() || len(w.declarationType(gd)) == 0 {
		return
	}
	w.notes[noteAt{loc: loc, rule: ruleCvcElt}] = nonNillable{key: key}
}

// governingDeclaration is the ·governing element declaration· of an element
// named name standing at at, as validate.Validator.Governing decides it with
// no xsi:type and its selection unread (walker.governedAt). It is false where
// the element stands nowhere the engine assesses it (walker.assessed), and
// where Governing answers it none — a ·locally declared type· governing it,
// its name ·resolving· to none, or an ·absent· component sending it to ·lax
// assessment· (§5.3) — the engine then assessing it with no declaration and
// charging it no cvc-elt clause but clause 1. Governing answers no error
// here, the element read with no xsi:type.
func (w *walker) governingDeclaration(name xsd.QName, at placement) (xsd.ElementDeclaration, bool) {
	if !w.assessed(at, name) {
		return xsd.ElementDeclaration{}, false
	}
	gd, declared, _, _, err := w.governedAt(at, name, nil, unread)
	return gd, declared && err == nil
}

// noteTypeLexical keeps a typeLexical note at the key of e's "$type" member
// typ, where none is nil, whose value has no xs:QName ·actual value· under e's
// bindings, so that the engine charges cvc-attribute clause 3 there
// (walk.instanceTypeLexical): it is no QName — empty after the whiteSpace
// collapse xs:QName fixes, of a colon structure no QName has, or with a part
// that is no NCName — or its prefix is one no "$xmlns" in scope binds
// (element.LookupPrefix). The split is [xmlqname.Resolve]'s, read through a
// lookup that binds every prefix to itself, so that the namespace it returns
// is the prefix, and the local part it returns where it splits no QName is
// empty, which is no NCName; the NCName test is [xmlname.IsNCName], which
// accepts the strings builtin/strict's xs:QName mapping accepts as NCNames.
//
// GAP(jsonsrc): under a value backend that does not map xs:QName, or maps it
// otherwise than builtin/strict, the engine declines clause 3 and charges a
// lexical of no QName under cvc-attribute clause 5 at the "$type"'s key, or
// nothing (walk.instanceTypeLexical's GAP(validate) marker), and the note
// goes on that clause 5 charge, which its clause does not answer, or on none.
// RULED permanent by #774: mapping xs:QName is backend coverage, not this
// package's, and the mapping reads no backend.
func (w *walker) noteTypeLexical(e *element, typ *attribute) {
	if typ == nil {
		return
	}
	at := noteAt{loc: typ.loc, rule: ruleCvcAttribute}
	prefix, local, _ := xmlqname.Resolve(typ.value, func(p string) (string, bool) { return p, true })
	if !xmlname.IsNCName(local) || (prefix != "" && !xmlname.IsNCName(prefix)) {
		w.notes[at] = typeLexical{key: e.key}
		return
	}
	if _, bound := e.LookupPrefix(prefix); bound {
		return
	}
	w.notes[at] = typeLexical{key: e.key, prefix: prefix}
}

// noteCharacters keeps a characters note at loc, the location of the scalar
// whose lexical form is lexical, for the member key that m classifies,
// standing at at, where the type walker.scalarGoverning finds governing the
// element charges its text: a complex type whose {content type} is empty,
// which cvc-complex-type clause 1.1 charges for any character information
// item, or element-only, which clause 1.3 charges for one that is not all
// white space (xmlWhitespace). An empty lexical is no character information
// item (walker.scalar), and is charged under neither. A bareElement has no
// declaration, and the engine assesses it under ·xs:anyType· (key-lva), whose
// content is mixed. The notes kept grow with the scalars those types charge,
// the document never being held (STYLE P4). walker.scalarGoverning
// determines no type at the two sites below.
//
// GAP(jsonsrc): no note is kept where the mapping attributes the element to
// nothing (placement.attributed): its parent's content model, or one above it,
// is one xsd.Schema.ContentMatcher declines (object.unattributed, its GAP(xsd)
// marker, the maxPartitionStates ceiling RULED permanent there). validate
// walks the XML twin's subtree undecided there (its GAP(validate) marker in
// walk.child) and charges it no text, so walker.annotate, the note's one
// consumer, leaves no charge the engine makes without a clause it would carry.
// Owned by #2670, which retires with #1601's ruling.
//
// GAP(jsonsrc): no note is kept where the declaration carries a {type table}
// whose selection the engine withholds, a {test} it reaches being outside its
// §3.12.6 compiler's subset, or where the mapping reads no selection
// (walker.selector): the types that may govern the parent differ, or hand
// down different [inherited attributes] (walker.handDown). That holds whether
// or not the types the table can select, or the types that may govern the
// element, agree on charging the scalar. The engine walks each such element
// undecided, its own selection withheld or one above it, and charges it no
// text, so walker.annotate, the note's one consumer, leaves no charge the
// engine makes without a clause it would carry; where the element's own
// selection is withheld, its parent can still charge at an array item's
// location, and a note kept there would answer that charge. RULED permanent
// by #2702: selecting past a withheld key-cta-select alternative is unsound,
// as walker.classify's marker of the same ruling says.
func (w *walker) noteCharacters(m elementMember, key, lexical string, loc xsderr.Loc, at placement) {
	d, ok := m.(declaredElement)
	if !ok || lexical == "" {
		return
	}
	t, ok := w.scalarGoverning(d.decl, at)
	if !ok {
		return
	}
	ct, ok := t.(xsd.ComplexType)
	if !ok {
		return
	}
	v := ct.ContentType().Variety()
	if v != xsd.ContentEmpty && v != xsd.ContentElementOnly {
		return
	}
	if v == xsd.ContentElementOnly && strings.TrimLeft(lexical, xmlWhitespace) == "" {
		return
	}
	w.notes[noteAt{loc: loc, rule: ruleCvcComplexType}] = characters{key: key, lexical: lexical, variety: v}
}

// scalarGoverning is the type that governs a scalar-valued element the
// declaration d names, standing at at, as the engine determines it, and false
// where the mapping does not determine it: a scalar carries no attribute, so
// no xsi:type, and its ·governing type definition· is what
// validate.Validator.Governing decides for it (walker.governedAt), the
// selection a {type table} on its ·governing element declaration· makes being
// the one the table makes for an element with no [attributes] and the
// [inherited attributes] at hands it (key-cta-ta-select clauses 1.1.2 and
// 1.1.3), as validate.Validator.ConditionallySelected, the engine's own scan,
// answers; and ·xs:anyType· where Governing answers neither a type nor a
// declaration, the element being ·laxly assessed· (key-lva). It is false
// where the mapping reads no selection for the element (walker.selector), for
// an element ·skipped· by a skip wildcard, which nothing assesses
// (cvc-assess-elt clause 2), for a child of a parent whose children the
// engine walks undecided (object.unassessed), where Governing answers a
// declaration and no type, below an element whose selection it withholds,
// though the mapping can determine a type there where every type that may
// govern each element between hands down alike (walker.selector), and at the
// sites walker.noteCharacters' GAP(jsonsrc) markers name. Governing answers
// no error here, the element read with no xsi:type.
func (w *walker) scalarGoverning(d xsd.ElementDeclaration, at placement) (xsd.TypeDefinition, bool) {
	if _, _, read := w.selector(d, at); !read {
		return nil, false
	}
	if at.parent != nil && at.parent.unassessed {
		return nil, false
	}
	inherited, _ := at.inherited()
	selected := func(table xsd.TypeTable) (xsd.TypeDefinitionOrRef, bool) { return w.selected(table, nil, inherited) }
	_, declared, t, assessed, err := w.governedAt(at, d.Name(), nil, selected)
	if err != nil || !assessed {
		return nil, false
	}
	if t != nil {
		return t, true
	}
	if declared {
		return nil, false
	}
	return w.schema.ResolvedType(anyTypeRef)
}

// annotate appends to the Msg of the charge each note answers the clause its
// hint gives: of the violations of res at the note's rule and location, in
// res's order, the first or the last as note.answers says, and none where no
// violation is there. Each arm's answers states the order its charge takes among the
// others there; the two paragraphs below state it for characters and for
// the parent's notes.
//
// Where the engine determines the type walker.scalarGoverning determines, a
// characters note is kept exactly where the engine makes the element's own
// text charge (walker.noteCharacters), and that charge is the last
// cvc-complex-type violation at the scalar's location: at a member's scalar
// no other charge stands, the element's own location being its key's; at an
// array item, whose location is its scalar's, the parent's clause 1.1 and 1.2
// charges and its clause 5 charge at the item, and the item's clause 3 and 4
// charges, all precede the item's [[children]] (validate's walk.child and
// walk.element), and no cvc-complex-type charge at the item follows its text
// charge, its content check charging once (validate's contentCheck.charged)
// and clause 1.2's ·initial value· charge needing simple content.
//
// Where the engine determines the type that governs the parent, the one
// object.governing holds, a scalarExpected, unnamed or arrayItem note is kept
// exactly where the engine charges the parent's content at the child
// (walker.noteChild), and that charge is the first violation at the child's
// location under its rule: validate's walk.child charges the parent's content
// at a child before it assesses the child, every charge of the child's own at
// that location following, and the parent's earlier charges stand at its
// attribute members' and its earlier children's locations.
//
// Where the engine walks the element undecided instead, it charges nothing at
// that location (walker.scalarGoverning, walker.noteChild). Writing Msg
// through res.Violations() writes res's own violations, as
// validate.Result.Violations states; the rule ID and location stay the
// engine's.
func (w *walker) annotate(res *validate.Result) {
	var noted []noteAt
	charges := map[noteAt][]*xsderr.Error{}
	for _, e := range res.Violations() {
		at := noteAt{loc: e.Loc, rule: e.Rule}
		if _, ok := w.notes[at]; !ok {
			continue
		}
		if _, seen := charges[at]; !seen {
			noted = append(noted, at)
		}
		charges[at] = append(charges[at], e)
	}
	for _, at := range noted {
		n, es := w.notes[at], charges[at]
		e := es[0]
		if n.answers() == lastCharge {
			e = es[len(es)-1]
		}
		e.Msg += n.hint()
	}
}
