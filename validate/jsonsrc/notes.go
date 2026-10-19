package jsonsrc

import (
	"fmt"
	"strings"

	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// ruleCvcComplexType is the rule of the charge a characters note answers.
const ruleCvcComplexType xsderr.Rule = "cvc-complex-type"

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
// hint gives: the last violation of res at the note's rule and location, and
// none where no violation is there. Where the engine determines the type
// walker.scalarGoverning determines, a note is kept exactly where the engine
// makes the element's own text charge (walker.noteCharacters), and that
// charge is the last cvc-complex-type violation at the scalar's location: at
// a member's scalar no other charge stands, the element's own location being
// its key's; at an array item, whose location is its scalar's, the parent's
// clause 1.1 and 1.2 charges and its clause 5 charge at the item, and the
// item's clause 3 and 4 charges, all precede the item's [[children]]
// (validate's walk.child and walk.element), and no cvc-complex-type charge at
// the item follows its text charge, its content check charging once
// (validate's contentCheck.charged) and clause 1.2's ·initial value· charge
// needing simple content. Where the engine walks the element undecided
// instead, it charges nothing at that location (walker.scalarGoverning).
// Writing Msg through res.Violations() writes res's own violations, as
// validate.Result.Violations states; the rule ID and location stay the
// engine's.
func (w *walker) annotate(res *validate.Result) {
	answered := map[noteAt]bool{}
	vs := res.Violations()
	for i := len(vs) - 1; i >= 0; i-- {
		at := noteAt{loc: vs[i].Loc, rule: vs[i].Rule}
		n, ok := w.notes[at]
		if !ok || answered[at] {
			continue
		}
		answered[at] = true
		vs[i].Msg += n.hint()
	}
}
