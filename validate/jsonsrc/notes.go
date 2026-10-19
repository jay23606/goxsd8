package jsonsrc

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// The rules of the charges a note answers, and that validate's
// contentCheck.element charges a parent's content under (contentCharge): a
// characters note answers cvc-complex-type, a scalarExpected cvc-type, and an
// unnamed cvc-complex-content.
const (
	ruleCvcType           xsderr.Rule = "cvc-type"
	ruleCvcComplexType    xsderr.Rule = "cvc-complex-type"
	ruleCvcComplexContent xsderr.Rule = "cvc-complex-content"
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
// appends to that charge's Msg, and own reports whether the charge is the
// element's own, which follows every charge its parent makes at the same rule
// and location, rather than the parent's, which precedes every charge of the
// element's there (walker.annotate). Its arms are characters, scalarExpected
// and unnamed.
type note interface {
	hint() string
	own() bool
}

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

// own is true: the element's text charge follows the parent's charges at an
// array item's location, which is its scalar's (walker.annotate).
func (characters) own() bool { return true }

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

func (scalarExpected) own() bool { return false }

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

func (unnamed) own() bool { return false }

// noteChild keeps a note at loc for the child member key that m classifies
// under in's object, its value's scope sc, standing at at, where every type
// that may govern the parent (object.governing) charges the parent's content
// there under one rule (placement.parentCharge): a scalarExpected where that
// is cvc-type, every such type being a simple type, and an unnamed where it is
// cvc-complex-content and walker.unnamedNote keeps one. Each parent's content is
// charged at one child at most (object.attribute), so the notes kept grow with
// the parents charged, not with the document (STYLE P4).
//
// GAP(jsonsrc): no note is kept where the mapping attributes the child to
// nothing (placement.attributed): its parent's content model, or one above it,
// is one xsd.Schema.ContentMatcher declines (object.unattributed, its GAP(xsd)
// marker, the maxPartitionStates ceiling RULED permanent there), or the schema
// view lacks the governor capability, which validate.Validator.Schema never
// returns. validate records its decline where the parent's own model is the
// one declined (contentCheck.match) and walks the XML twin's subtree undecided
// below it (its GAP(validate) marker in walk.child), and charges the parent's
// content at no child, so walker.annotate, the note's one consumer, leaves no
// charge the engine makes without a clause it would carry. Owned by #2670,
// which retires with #1601's ruling.
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
// marker, the maxPartitionStates ceiling RULED permanent there), or the schema
// view lacks the governor capability, which validate.Validator.Schema never
// returns. validate walks the XML twin's subtree undecided there (its
// GAP(validate) marker in walk.child) and charges it no text, so
// walker.annotate, the note's one consumer, leaves no charge the engine makes
// without a clause it would carry. Owned by #2670, which retires with #1601's
// ruling.
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
// no xsi:type, and its ·governing type definition· is what its ·governing
// element declaration· gives it (key-governing-type-elem clause 4), or, where
// it has none, the type walker.governs finds governing it. That declaration
// is the one walker.selector names; where it carries a {type table}, the type
// is the one the table selects for an element with no [attributes] and the
// [inherited attributes] at hands it (key-cta-ta-select clauses 1.1.2 and
// 1.1.3), as validate.Validator.ConditionallySelected, the engine's own scan,
// answers. It is false for an element ·skipped· by a skip wildcard, which
// nothing assesses (cvc-assess-elt clause 2), and at the sites
// walker.noteCharacters' GAP(jsonsrc) markers name.
//
// Below an element whose selection the engine withholds, the mapping can
// determine a type where every type that may govern each element between
// hands down alike (walker.selector), while the engine walks the subtree
// undecided and charges nothing there, the scalar's parent included; the note
// kept then answers no charge (walker.annotate).
func (w *walker) scalarGoverning(d xsd.ElementDeclaration, at placement) (xsd.TypeDefinition, bool) {
	declared, ok := w.schema.ResolvedType(d.TypeDefinition())
	if !ok {
		return nil, false
	}
	from, declares, read := w.selector(d, declared, at)
	if !read {
		return nil, false
	}
	if at.parent != nil && skips(at.a[0]) {
		return nil, false
	}
	if declares {
		if table, tabled := from.TypeTable(); tabled {
			inherited, _ := at.inherited()
			slot, decided := w.selected(table, nil, inherited)
			if !decided {
				return nil, false
			}
			return w.schema.ResolvedType(slot)
		}
	}
	ts := w.governing(d, declared, at)
	if len(ts) != 1 {
		return nil, false
	}
	return ts[0], true
}

// annotate appends to the Msg of the charge each note answers the clause its
// hint gives: of the violations of res at the note's rule and location, the
// last where the charge is the element's own (note.own) and the first where
// it is its parent's, and none where no violation is there.
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
// object.governing holds, a scalarExpected or unnamed note is kept exactly
// where the engine charges the parent's content at the child
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
	var answers []*xsderr.Error
	answered := map[noteAt]int{}
	for _, e := range res.Violations() {
		at := noteAt{loc: e.Loc, rule: e.Rule}
		n, ok := w.notes[at]
		if !ok {
			continue
		}
		i, seen := answered[at]
		if !seen {
			answered[at] = len(answers)
			answers = append(answers, e)
			continue
		}
		if n.own() {
			answers[i] = e
		}
	}
	for _, e := range answers {
		e.Msg += w.notes[noteAt{loc: e.Loc, rule: e.Rule}].hint()
	}
}
