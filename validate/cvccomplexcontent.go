package validate

import (
	"context"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xpath"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// ruleCvcComplexContent is Element Sequence Locally Valid (Complex Content)
// (Structures §3.4.4.3, cvc-complex-content). It is a "the appropriate case"
// construct over the {open content}: clause 1 — the whole rule where it is
// ·absent· — sends the sequence to cvc-particle (§3.9.4.2), clause 2 splits it
// for {mode} suffix and clause 3 for {mode} interleave, and [xsd.Matcher]
// decides all three. The clause charged goes in the message on ruleCvcElt's
// terms, since the catalog carries the bare name, and it is the one that was
// LIVE ([contentCheck.contentClause]) rather than clause 1 by default.
const ruleCvcComplexContent xsderr.Rule = "cvc-complex-content"

// This file decides one element's [[children]] against whichever arm of cvc-type
// (§3.3.4.4) clause 3 its ·governing type definition· selects — the CONTENT half
// of the dispatch whose attribute half is assess.go's.
//
// Clause 3.1, for a SIMPLE ·governing type definition·, contributes two
// sub-clauses here:
//
//   - 3.1.2, no element information item [[children]] at all. It is NOT gated on
//     ·nilled·, and needs no gate of its own: a ·nilled· element carrying an
//     element [[child]] is charged cvc-elt clause 3.2.3.1 by the same visit,
//     which says the same thing of the same item.
//   - 3.1.3, the ·initial value· ·valid· with respect to that type per String
//     Valid (§3.16.4), for an element that is not ·nilled· (simpleTypeValue).
//
// Clause 3.2 sends the same [[children]] to cvc-complex-type (§3.4.4.2) clause
// 1 instead. Its four sub-clauses are one dispatch on {content type}.{variety},
// and the rest of this file is that dispatch:
//
//   - 1.1, empty: no character AND no element information item [[children]],
//     white space included. Empty is STRICTER than element-only (PRINCIPLES
//     13), and clause 1.3's white-space allowance is the whole of the
//     difference — 1.1 states no exception of any kind.
//   - 1.2, simple: no element information item [[children]], AND the ·initial
//     value· — the [[character code]] of each character information item
//     [[child]], concatenated in order (Glossary) — ·valid· with respect to the
//     {simple type definition} per String Valid (§3.16.4). The first half is
//     settled per child, the second only once the [[children]] are exhausted.
//   - 1.3, element-only: no character information item [[children]] other than
//     XML 1.1 white space.
//   - 1.4, element-only or mixed: the sequence of element information items,
//     taken in order, is ·valid· per cvc-complex-content (§3.4.4.3), which
//     [xsd.Schema.ContentMatcher] decides one child at a time. Mixed restricts
//     character content not at all: clause 1.4 is the only clause it reaches.
//
// One element carries AT MOST ONE content charge. Every clause above says the
// same thing about the element once charged — it is not locally ·valid· with
// respect to its ·governing type definition·, so cvc-type clause 3.2 fails and
// its [validity] is invalid — and the first charge is the one at the offending
// position, which is what a reader needs. Continuing to match after a rejected
// child would report the rest of the sequence against a position the document
// never reached.
//
// Three cvc-elt (§3.3.4.3) clauses read the same [[children]] and are settled
// here rather than in cvcelt.go, because arriving children are what decide
// them:
//
//   - 3.2.3.1, for a ·nilled· element: no character or element information item
//     [[children]] at all. It REPLACES clause 1 rather than joining it — clause
//     1 applies "if E is not ·nilled·" — so the two are never both live, and it
//     shares the one-charge rule above for the same reason.
//   - 5.1, for an EMPTY element whose ·governing element declaration· carries a
//     {value constraint} and which is not ·nilled·: 5.1.1, that constraint is a
//     valid default for an ·instance-specified· ·governing type definition·
//     (defaultValid), and 5.1.2, cvc-type over the ·normalized value· the
//     constraint's {lexical form} substitutes ([contentCheck.assessed]). It is
//     the arm clause 5 takes INSTEAD of 5.2, so the cvc-type dispatch above runs
//     over the substituted value and never over the empty [[children]].
//   - 5.2.2, for an element on clause 5.2's arm whose ·governing element
//     declaration· carries a fixed {value constraint}: no element information
//     item [[children]] (5.2.2.1) and an ·initial value· agreeing with the
//     constraint (5.2.2.2). It joins clause 1 rather than replacing it, and is
//     settled once the [[children]] are exhausted (fixedValue).

// contentCheck is the state of one element's [[children]] assessment against
// its ·governing type definition·. It is built per element by
// [walk.contentCheck] and dropped when that element's [[children]] are
// exhausted.
//
// g carries that type, and the two narrowings of it read here are the two arms
// of cvc-type clause 3: governance.simpleType selects clause 3.1's, and
// governing (governance.complexType, plus the ·nilled· gate) selects clause
// 3.2's. Both being nil is a ·governing type definition· this package could not
// determine, which decides NEITHER arm: every child is walked and none is
// charged or passed by a type, and none is ·attributed to· anything either, so
// the whole subtree below such an element is walked against no type in its turn,
// undecided ([walk.child]). A ·laxly assessed· element, which has no type to
// determine, is the one exception ([walk.child]). The two cvc-elt clauses below
// still apply — they read the DECLARATION and not the type. A nil matcher
// beside a non-nil governing is none of those states: it is clause 1.4 alone
// declining — a shape [xsd.Schema.ContentMatcher] does not decide — while
// clauses 1.1 to 1.3 still hold, since they read the {variety} and not the
// particle. A present {open content} is no longer one of those shapes: the
// matcher decides both {mode}s, and every charge below names the clause that
// decided it.
//
// nilled is whether E is ·nilled· (§3.3.4.3, key-nilled), decided before any
// child arrives ([walk.nilCheck]). It turns cvc-complex-type clause 1 off
// wholesale, which governing is the one reading of, and cvc-elt clause 3.2.3.1
// on — the half an undetermined type cannot tell apart, an element with no
// ·governing type definition· charging neither clause. It also turns cvc-type
// clause 3.1.3 off, and clause 3.1.2 not at all.
//
// initial gathers the ·initial value· clauses 3.1.3, 1.2 and 5.2.2.2 test, and
// is written only where one of them reads it (gathers): no other clause reads a
// character information item [[child]] beyond the one it is charging, so
// gathering elsewhere would hold the whole of an element's text for nothing.
//
// sawElement and sawText record which kinds of [[children]] arrived, which is
// what cvc-elt clause 5's case split and clause 5.2.2.1 quantify over, and, for
// a ·nilled· element, whether it has any [[children]] at all, which
// [walk.assertionValue] reads. Under a nil matcher sawElement also says whether
// [contentCheck.match] already recorded clause 1.4's decline at a child, or
// [contentCheck.sequenceEnd] must record it at the element. They are not
// derivable from initial, which is gathered conditionally and holds nothing for
// the elements whose text no clause reads.
type contentCheck struct {
	e          Element
	g          governance
	nilled     bool
	matcher    *xsd.Matcher
	initial    strings.Builder
	sawElement bool
	sawText    bool
	charged    bool
}

// contentCheck builds the check for one element's [[children]].
func (w *walk) contentCheck(e Element, g governance, nilled bool) *contentCheck {
	c := &contentCheck{e: e, g: g, nilled: nilled}
	governing := c.governing()
	if governing == nil {
		return c
	}
	if m, ok := w.schema.ContentMatcher(*governing); ok {
		c.matcher = m
	}
	return c
}

// governing is the ·governing type definition· narrowed to the complex type
// cvc-complex-type clause 1 reads, and nil wherever that clause is not live: a
// ·governing type definition· this package could not determine or that is
// SIMPLE, which cvc-type clause 3.1 decides instead (governance.simpleType),
// and a ·nilled· element, clause 1 applying only "if E is not ·nilled·".
func (c *contentCheck) governing() *xsd.ComplexType {
	if c.nilled {
		return nil
	}
	return c.g.complexType()
}

// openContent is the {open content} of the ·governing type definition·'s
// {content type} (§3.4.1), and nil where it is ·absent· — including for a
// {variety} that carries no such property at all and for a check with no
// governing complex type.
func (c *contentCheck) openContent() *xsd.OpenContent {
	ct := c.governing()
	if ct == nil {
		return nil
	}
	ec, elementOnlyOrMixed := ct.ContentType().(xsd.ElementContent)
	if !elementOnlyOrMixed {
		return nil
	}
	return ec.OpenContent
}

// contentClause is the cvc-complex-content clause this element's [[children]]
// are decided under: clause 1 where {open content} is ·absent·, clause 2 for
// {mode} suffix and clause 3 for {mode} interleave (§3.4.4.3). Every charge and
// every log line from the sequence match reads it, so the "appropriate case"
// the rule selects is named once and not restated per message.
//
// The two {mode}s are [xsd.OpenContentMode]'s whole value space (§3.4.1, and
// there is no third for mode="none" — that is the ·absent· record above), so
// interleave is the remaining case rather than a default.
func (c *contentCheck) contentClause() string {
	oc := c.openContent()
	if oc == nil {
		return "1"
	}
	if oc.Mode() == xsd.OpenContentSuffix {
		return "2"
	}
	return "3"
}

// empty reports that E has neither element nor character information item
// [[children]] — the emptiness cvc-elt clause 5's case split and clause 5.2.2.1
// both quantify over.
//
// It is read off sawElement and sawText and not off initial, which is gathered
// conditionally. The answer is correct whether or not the element is ·nilled·:
// text and element each set their flag before any charge those same
// [[children]] provoke — cvc-elt clause 3.2.3.1's on the ·nilled· path among
// them — so a later run or item arriving after the element is charged leaves
// the flags untouched and the one that did the charging has already set one of
// them. The flags therefore say whether ANY [[child]] arrived, and which kind
// came first, but not every kind that arrived after a charge. A ·nilled· element
// is excluded from clause 5.1 and from clause 5.2.2 all the same
// (elementDefault takes it as its own parameter, fixedValue returns on
// c.nilled); [walk.assertionValue] asks it of a ·nilled· element to withhold
// its assertions where it has [[children]].
func (c *contentCheck) empty() bool {
	return !c.sawElement && !c.sawText
}

// defaulted is the {value constraint} cvc-elt clause 5.1 substitutes for this
// element's ·normalized value·, and whether clause 5.1's arm of the clause 5 case
// split is the live one (elementDefault).
//
// It is the element's ONE reading of that split (STYLE T4): assessed takes the
// string from it for the cvc-type charges here, and [icCheck.substitute] takes
// the same answer across to §3.11.4 and §3.17.5.2, which read the same value once
// these [[children]] are exhausted. Only this check knows E has neither element
// nor character information item [[children]], which is why the reading is here
// and not on either consumer.
func (c *contentCheck) defaulted() (xsd.ValueConstraint, bool) {
	return elementDefault(c.g, c.empty(), c.nilled)
}

// assessed is the ·initial value· cvc-elt clause 5 leaves this element assessed
// on, paired with the namespace context it is mapped under: D.{value
// constraint}.{lexical form} under [value.ConstraintContext] on clause 5.1's
// arm, the bindings in scope where the schema document wrote it
// (cos-valid-simple-default clause 2, Datatypes §3.3.18), and the ·initial
// value· E actually carries under E's own bindings (elementContext) on clause
// 5.2's. The two travel together so no reader can map a default under the
// instance's bindings. It is [icCheck.assessed]'s twin, one per state the
// [[children]] are read into.
func (c *contentCheck) assessed() (lexical string, ctx value.Context) {
	if vc, defaulted := c.defaulted(); defaulted {
		return vc.LexicalForm(), value.ConstraintContext(vc)
	}
	return c.initial.String(), elementContext{owner: c.e}
}

// defaultValid settles cvc-elt clause 5.1.1: where clause 5.1's arm is live and
// E's ·governing type definition· is an ·instance-specified type definition·,
// D.{value constraint} is a valid default for THAT type as Element Default Valid
// (Immediate) (§3.3.6.2, cos-valid-default) defines it.
//
// The antecedent is the ·instance-specified· one alone (governance.instance,
// §3.3.4.1 key-itd), which is narrower than "the governing type differs from the
// declared type": a {type table} ·conditionally selects· a ·selected type
// definition· with no xsi:type present at all, and clause 5.1.1 does not reach
// that case however far the selected type is from the declared one.
//
// Nothing is charged where the declaration's OWN type governs, and nothing is
// missed either: e-props-correct (§3.3.6.1) clause 2 has already charged
// cos-valid-default over that type at assembly, against the same {value
// constraint}, which is why clause 5.1.1 asks only about the type the INSTANCE
// introduced.
//
// The predicate is [xsd.Schema.ElementDefaultValid], the same one Phase E
// charges, and is deliberately not re-derived here (STYLE T4). Its verdict
// arrives under cos-valid-default, the predicate's own rule, naming which of
// that constraint's clauses turned the default away; THIS clause is named here
// and nowhere else, in the message [contentCheck.chargeCause] wraps that verdict
// as the cause of.
//
// The value space handed to it is the WALK's ([walk.values]) and not the one the
// schema was finalized with: a schema assembled through
// [xsd.SchemaBuilder.Finalize] carries undecidedValueSpace, which answers every
// question undecided, so reading its would leave this charge permanently
// undecidable for such a schema.
func (c *contentCheck) defaultValid(w *walk) {
	if c.charged || !c.g.instance || c.g.typ == nil {
		return
	}
	vc, defaulted := c.defaulted()
	if !defaulted {
		return
	}
	cause := w.schema.ElementDefaultValid(w.values, c.e.Loc(),
		"the element "+c.e.Name().String(), c.g.typ, vc)
	if cause == nil {
		c.log(w, c.e.Name(), c.e.Loc(), ruleCvcElt, "5.1.1", "satisfied")
		return
	}
	c.chargeCause(w, ruleCvcElt, "5.1.1", c.e.Loc(), cause,
		"the ·governing type definition· of the element %s is the ·instance-specified type definition· %s, and cvc-elt clause 5.1.1 requires the {value constraint} %q of its ·governing element declaration· to be a valid default for that type as Element Default Valid (Immediate) (§3.3.6.2) defines it",
		c.e.Name(), typeName(c.g.typ), vc.LexicalForm())
}

// gathers reports whether any charge reads this element's ·initial value·:
// cvc-type clause 3.1.3's String Valid, over a SIMPLE ·governing type
// definition·; cvc-complex-type clause 1.2's, over a complex one with a simple
// {content type}; or cvc-elt clause 5.2.2.2's agreement with a fixed {value
// constraint}. A ·nilled· element gathers nothing — every one of the three is
// gated on "E is not ·nilled·" — and neither does one whose text no clause would
// read.
func (c *contentCheck) gathers() bool {
	if c.nilled {
		return false
	}
	if _, fixed := elementFixed(c.g); fixed {
		return true
	}
	if c.g.simpleType() != nil {
		return true
	}
	ct := c.governing()
	return ct != nil && ct.ContentType().Variety() == xsd.ContentSimple
}

// text settles clauses 1.1 and 1.3 for one run of character information items —
// or cvc-elt clause 3.2.3.1, for a ·nilled· element, which admits no character
// [[child]] at all — and gathers the run into the ·initial value· clauses 3.1.3,
// 1.2 and 5.2.2.2 test at the end.
//
// A SIMPLE ·governing type definition· restricts no run on its own: clause 3.1
// has no character-content sub-clause besides 3.1.3, which tests the runs
// CONCATENATED exactly as clause 1.2 does.
//
// A run of NO characters is no character information item at all, so it
// reaches neither clause: both quantify over the items in [[children]], and an
// adapter that reports an empty run for <e></e> has reported a run, not an
// item. It contributes nothing to the ·initial value· either, that being a
// concatenation, and it is not the character [[child]] clause 5's case split
// turns on.
func (c *contentCheck) text(w *walk, t Text) {
	if c.charged || t.Data() == "" {
		return
	}
	c.sawText = true
	if c.nilled {
		c.charge(w, ruleCvcElt, "3.2.3.1", t.Loc(),
			"the element %s has xsi:nil = true, so it is ·nilled·, but it has a character information item [[child]], and cvc-elt clause 3.2.3.1 admits no character or element information item [[children]] on a ·nilled· element",
			c.e.Name())
		return
	}
	if c.gathers() {
		c.initial.WriteString(t.Data())
	}
	if c.governing() == nil {
		return
	}
	switch c.governing().ContentType().Variety() {
	case xsd.ContentEmpty:
		c.charge(w, ruleCvcComplexType, "1.1", t.Loc(),
			"the element %s has a character information item [[child]], but cvc-complex-type clause 1.1 admits no character or element information item [[children]] at all where the {content type}.{variety} of the ·governing type definition· is empty — white space included, which clause 1.3 allows for element-only and clause 1.1 allows for nothing",
			c.e.Name())
	case xsd.ContentElementOnly:
		if isXMLWhitespace(t.Data()) {
			return
		}
		c.charge(w, ruleCvcComplexType, "1.3", t.Loc(),
			"the element %s has a character information item [[child]] that is not white space, but cvc-complex-type clause 1.3 admits white space alone where the {content type}.{variety} of the ·governing type definition· is element-only",
			c.e.Name())
	case xsd.ContentSimple:
		// Clause 1.2 tests the runs CONCATENATED, so no run decides anything on
		// its own and none is charged here; gathers has already retained it.
	case xsd.ContentMixed:
		// A mixed {content type} restricts character content in no clause at
		// all.
	}
}

// element settles clauses 1.1, 1.2 and 1.4 for one element information item of
// the sequence — or cvc-type clause 3.1.2, under a simple ·governing type
// definition·, or cvc-elt clause 3.2.3.1, for a ·nilled· element — and reports
// what clause 1.4 ·attributed· the item to (§3.4.4.4) for the walk's own
// descent into it (key-sva (§3.3.4.6) clause 3.1, [walk.childGoverning]).
//
// The attribution is nil wherever nothing attributed the item, and undecided
// reports the two nils that are this package's own declines rather than the
// spec's outcomes: a check with no ·governing type definition· this package
// could determine — a ·laxly assessed· parent's included, whose attribution
// [walk.child] supplies before it reads undecided — and a clause 1.4
// [xsd.Schema.ContentMatcher] does not decide. Every other nil is decided: a
// simple ·governing type definition·, a ·nilled· element, a {variety} that
// admits no element information item [[child]] at all, a clause 1.4 that
// charged, and an element already charged — that last by this package's
// reading, not the spec's: the matcher is not advanced past a charge, so a
// later child is read as attributed to nothing. Either way the child is still WALKED —
// every charge here is against the parent, not the child — and how it is
// governed is the package promise the package doc states.
//
// A ·nilled· element whose type this package could not determine
// ([governance.typeUndetermined], the declined shape) is charged cvc-elt clause
// 3.2.3.1 all the same — the clause reads no type — but its child is reported
// undecided, on both arms that charge reaches it by, because what governs the
// child is the parent's type's to say: key-governing-ed clause 4.3 reads the
// parent's ·locally declared type·, and key-p-inherited clause 3 the
// ·attribution· of the parent's attributes to that type, which is the
// [inherited attributes] a child's {type table} selects on (key-cta-ta-select
// clause 1.1.3). A decided child there would be governed by its top-level
// declaration and selected on attributes read by ·expanded name·, wrong in
// both directions (#2211). The Result stays honest through the records already
// made: this charge at the child, and the parent's own [Unevaluated] records
// for the type it declined (key-cta-ta-select, or cvc-elt clause 1 or 4) and
// for cvc-id ([walk.idElement]).
func (c *contentCheck) element(w *walk, child Element) (a xsd.Attribution, undecided bool) {
	if c.charged {
		return nil, c.g.typeUndetermined()
	}
	c.sawElement = true
	if c.nilled {
		c.charge(w, ruleCvcElt, "3.2.3.1", child.Loc(),
			"the element %s has xsi:nil = true, so it is ·nilled·, but it has the element information item %s among its [[children]], and cvc-elt clause 3.2.3.1 admits no character or element information item [[children]] on a ·nilled· element",
			c.e.Name(), child.Name())
		return nil, c.g.typeUndetermined()
	}
	if st := c.g.simpleType(); st != nil {
		c.charge(w, ruleCvcType, "3.1.2", child.Loc(),
			"the element %s has the element information item %s among its [[children]], but its ·governing type definition· %s is a Simple Type Definition, and cvc-type clause 3.1.2 admits no element information item [[children]] on such an element",
			c.e.Name(), child.Name(), typeName(st))
		return nil, false
	}
	if c.governing() == nil {
		return nil, true
	}
	switch c.governing().ContentType().Variety() {
	case xsd.ContentEmpty:
		c.charge(w, ruleCvcComplexType, "1.1", child.Loc(),
			"the element %s has the element information item %s among its [[children]], but cvc-complex-type clause 1.1 admits no character or element information item [[children]] where the {content type}.{variety} of the ·governing type definition· is empty",
			c.e.Name(), child.Name())
	case xsd.ContentSimple:
		c.charge(w, ruleCvcComplexType, "1.2", child.Loc(),
			"the element %s has the element information item %s among its [[children]], but cvc-complex-type clause 1.2 admits no element information item [[children]] where the {content type}.{variety} of the ·governing type definition· is simple",
			c.e.Name(), child.Name())
	case xsd.ContentElementOnly, xsd.ContentMixed:
		return c.match(w, child)
	}
	return nil, false
}

// match advances the content model over one element information item (clause
// 1.4), reporting what the item is ·attributed to· and charging
// cvc-complex-content against the item's OWN location where no particle live at
// that position admits it.
//
// An item the {open content} took is ·attributed to· it (§3.4.4.4) and charges
// nothing, exactly as one a particle took: clauses 2.4 and 3.4 make it a member
// of S2, which the rule admits. The charge is reached only where the item
// satisfies NEITHER half — no ·path· in {particle} at its position and not
// ·valid· with respect to {open content}.{wildcard} — which is clause 2 or
// clause 3 failing as a whole.
//
// The charge names what the content model would have ·attributed· an item to
// at that position ([xsd.Matcher.Admissible], rendered by expected through
// declining), the {open content} included where it is present. Every listed
// arm was offered the item and declined it, so a wildcard arm whose {namespace
// constraint} allows the item's namespace declined it under its {disallowed
// names}, and is named as disallowing the item.
//
// Its two results are [contentCheck.element]'s, undecided only for the nil
// matcher.
func (c *contentCheck) match(w *walk, child Element) (xsd.Attribution, bool) {
	clause := c.contentClause()
	if c.matcher == nil {
		c.decline(w, child.Name(), child.Loc(), ruleCvcComplexContent, clause,
			"the element information item %s among the [[children]] of %s was not matched against the {content type} of its ·governing type definition·: xsd.Schema.ContentMatcher does not decide that {content type}'s shape, so whether the sequence is ·valid· with respect to it as cvc-complex-content clause %s requires is undecided",
			child.Name(), c.e.Name(), clause)
		return nil, true
	}
	if a, ok := c.matcher.Next(child.Name()); ok {
		if w.log.Enabled(context.Background(), slog.LevelDebug) {
			c.log(w, child.Name(), child.Loc(), ruleCvcComplexContent, clause, "attributed to "+attributedTo(a))
		}
		return a, false
	}
	if c.openContent() != nil {
		c.charge(w, ruleCvcComplexContent, clause, child.Loc(),
			"the element information item %s is ·attributed to· no particle of the {content type} of %s at its position in the [[children]], and its ·expanded name· is not ·valid· with respect to that {content type}'s {open content} wildcard either, where the content model expects %s, so the sequence is not ·valid· with respect to that {content type} as cvc-complex-content clause %s requires",
			child.Name(), c.e.Name(), expected(c.matcher.Admissible(), declining(child.Name())), clause)
		return nil, false
	}
	c.charge(w, ruleCvcComplexContent, "1", child.Loc(),
		"the element information item %s is ·attributed to· no particle of the {content type} of %s at its position in the [[children]], where the content model expects %s, so the sequence is not ·valid· with respect to that {content type} as cvc-complex-content clause 1 requires",
		child.Name(), c.e.Name(), expected(c.matcher.Admissible(), declining(child.Name())))
	return nil, false
}

// end settles the clauses that only exhausted [[children]] can settle.
//
// cvc-elt clause 5.1.1 comes before all of them (defaultValid), because clause 5
// is a case split on whether the [[children]] are empty and 5.1.1 is the first
// conjunct of the arm an empty element with a {value constraint} takes: the
// cvc-type charges below are that arm's SECOND conjunct, 5.1.2, over the
// substituted ·normalized value· ([contentCheck.assessed]).
//
// cvc-type clause 3.1.3 comes next, for a SIMPLE ·governing type definition·,
// over the runs text gathered (simpleTypeValue). It and the clause-1 dispatch
// below are never both live: clause 3's two arms are a dispatch on the
// ·governing type definition· itself, so at most one of governance.simpleType
// and governance.complexType is non-nil.
//
// The clause-1 dispatch is on the {content type} itself rather than on its
// {variety} (STYLE T2's closed-sum exception), because one of its arms reads a
// property inside it:
//
//   - simple: clause 1.2's ·initial value· half, over the same gathered runs.
//   - element-only and mixed: clause 1.4, for a sequence that ran out with a
//     particle its {min occurs} left open, or, under a nil matcher, its
//     decline for a sequence with no element information item.
//
// An empty {content type} settles nothing here. Clause 1.1 quantifies over the
// [[children]] PRESENT, every one of which was already decided as it arrived.
//
// cvc-elt clause 5.2.2 is settled after them, over the same exhausted
// [[children]], and for an element with no ·governing type definition· too — it
// reads the DECLARATION's {value constraint}, which a type this package could
// not determine does not withhold.
func (c *contentCheck) end(w *walk) {
	c.defaultValid(w)
	if c.charged {
		return
	}
	c.simpleTypeValue(w)
	if c.governing() != nil {
		switch ct := c.governing().ContentType().(type) {
		case xsd.SimpleContent:
			c.initialValue(w, ct.SimpleType)
		case xsd.ElementContent:
			c.sequenceEnd(w)
		case xsd.EmptyContent:
			// Settled child by child; see above.
		}
	}
	c.fixedValue(w)
}

// fixedValue settles cvc-elt clause 5.2.2: where D.{value constraint}.{variety}
// is fixed and E is not ·nilled·, E has no element information item [[children]]
// (5.2.2.1) and its ·initial value· agrees with the constraint (5.2.2.2).
//
// The rule is only reached through clause 5's case split, and this function
// applies that split rather than clause 5.2.2 alone: clause 5.2 holds when D has
// no {value constraint}, or E has element or character [[children]], or E is
// ·nilled·. With a fixed constraint present and E not nilled, the first and
// third disjuncts are false, so clause 5.2 — and with it 5.2.2 — is live for
// exactly the elements that HAVE [[children]]. An empty element with a fixed
// constraint takes clause 5.1's arm instead (defaultValid).
//
// Clause 5.2.2.2's two cases are read off the ·governing type definition·: a
// mixed complex type compares LEXICALS (5.2.2.2.1, "matches"), and a simple type
// or simple {content type} compares ·actual values· (5.2.2.2.2, "equal or
// identical"), which is value.ConstraintMatches over the same pipeline the
// attribute charges use. A governing type of any other shape — element-only,
// empty, or none determined — matches no case and clause 5.2.2.2 is vacuous.
//
// A content charge already made silences this one: clause 5.2.2 asks whether the
// ·initial value· AGREES with the fixed value, and an ·initial value· cvc-type
// has just rejected has no agreement to report that its own charge does not
// already say.
func (c *contentCheck) fixedValue(w *walk) {
	if c.charged {
		return
	}
	f, fixed := elementFixed(c.g)
	if !fixed || c.nilled {
		return
	}
	if c.empty() {
		return // clause 5.1's arm; see defaultValid
	}
	if c.sawElement {
		c.charge(w, ruleCvcElt, "5.2.2.1", c.e.Loc(),
			"the element %s has element information item [[children]], but its ·governing element declaration· carries the fixed {value constraint} %q, and cvc-elt clause 5.2.2.1 admits no element information item [[children]] on an element so constrained",
			c.e.Name(), f.LexicalForm())
		return
	}
	if ct := c.governing(); ct != nil && ct.ContentType().Variety() == xsd.ContentMixed {
		c.fixedLexical(w, f)
		return
	}
	c.fixedActualValue(w, f)
}

// fixedLexical settles clause 5.2.2.2.1: for a mixed {content type}, the
// ·initial value· of E matches D.{value constraint}.{lexical form}. The match is
// over LEXICALS and not values — a mixed type has no {simple type definition} to
// map either side through — so it is string equality and nothing else.
func (c *contentCheck) fixedLexical(w *walk, f xsd.ValueConstraint) {
	if c.initial.String() == f.LexicalForm() {
		c.log(w, c.e.Name(), c.e.Loc(), ruleCvcElt, "5.2.2.2.1", "satisfied")
		return
	}
	c.charge(w, ruleCvcElt, "5.2.2.2.1", c.e.Loc(),
		"the ·initial value· of the element %s does not match the {lexical form} %q of the fixed {value constraint} of its ·governing element declaration·, which cvc-elt clause 5.2.2.2.1 requires of an element whose ·governing type definition· has {content type}.{variety} = mixed",
		c.e.Name(), f.LexicalForm())
}

// fixedActualValue settles clause 5.2.2.2.2: for a simple ·governing type
// definition· — a Simple Type Definition, or a complex one with a simple
// {content type} — the ·actual value· of E is equal or identical to
// D.{value constraint}.{value}.
//
// A governing type that is neither leaves clause 5.2.2.2 with no applicable
// case and charges nothing. A ·special· governing type is decided over its
// mapping union ([value.ConstraintMatches]), and an assertions facet in an
// ordinary one's closure through [xpath.FacetAssertions]. An undecided
// comparison charges nothing, on [walk.fixedAgreement]'s terms and for the
// same reasons: an ungoverned type, a ·special· one whose literals some member
// of that union cannot compare, and a {lexical form} outside its own type's
// lexical space or failing one of its {test}s are a gap in this processor or a
// schema fault, not the instance's. It is recorded as an [Unevaluated] instead
// ([contentCheck.decline]), the clause having been reached and not performed.
//
// GAP(xpath): a {test} the evaluator declines on either side is undecided too,
// and declines here on [walk.fixedAgreement]'s terms. (#1042)
//
// GAP(value): a NOTATION {lexical form} naming no declared notation, tracked by
// #667, declines here on [walk.fixedAgreement]'s terms: ValidDefault's gate 1
// refuses every NOTATION-governed default, so no assembly judges it and
// [value.ConstraintMatches] answers it undecided.
func (c *contentCheck) fixedActualValue(w *walk, f xsd.ValueConstraint) {
	st := c.g.valueType()
	if st == nil {
		return
	}
	same, decided := value.ConstraintMatches(w.backend, w.schema, st, c.initial.String(), elementContext{owner: c.e}, f, xpath.FacetAssertions(w.now))
	if !decided {
		c.decline(w, c.e.Name(), c.e.Loc(), ruleCvcElt, "5.2.2.2.2",
			"the ·actual value· of the element %s was not compared with the {value} of the fixed {value constraint} %q of its ·governing element declaration·: value.ConstraintMatches could not decide the comparison, a fault of the type or of the value backend, two literals of a ·special· type that some member of its lexical mapping cannot compare, an assertions-facet {test} not evaluated on either side or failed by the {value}, or a NOTATION {value} naming no declared notation, which no assembly judges yet (#667), rather than a verdict about the value, so cvc-elt clause 5.2.2.2.2 is undecided",
			c.e.Name(), f.LexicalForm())
		return
	}
	if same {
		c.log(w, c.e.Name(), c.e.Loc(), ruleCvcElt, "5.2.2.2.2", "satisfied")
		return
	}
	c.charge(w, ruleCvcElt, "5.2.2.2.2", c.e.Loc(),
		"the ·actual value· of the element %s is neither equal nor identical to the {value} of the fixed {value constraint} %q of its ·governing element declaration·, which cvc-elt clause 5.2.2.2.2 requires",
		c.e.Name(), f.LexicalForm())
}

// stringValid runs String Valid (§3.16.4) over this element's ·initial value· —
// the string composed, in order, of the [[character code]] of each character
// information item in E.[[children]] (Glossary, ·initial value·) — against st,
// and reports decided and err on [walk.stringValid]'s terms, the rejection
// where there is one being Datatype Valid's or String Valid clause 3's.
//
// Two clauses ask it of the same string, and the CHARGE is each caller's own
// because each names a different property as the simple type: cvc-type clause
// 3.1.3, where the ·governing type definition· IS st (simpleTypeValue), and
// cvc-complex-type clause 1.2, where st is that type's {content
// type}.{simple type definition} (initialValue). What they withhold is
// identical, so it is stated once, here.
//
// st needs no resolution, unlike the {type definition} the attribute charges
// reach through — a ·governing type definition· is the component itself, and
// [xsd.SimpleContent] carries one that [xsd.NewComplexType] rejects a nil of
// (ct-props-correct clause 1) — so the declines of [walk.stringValid] are the
// whole of what these charges withhold.
//
// The string tested is [contentCheck.assessed]'s and not c.initial, because
// cvc-elt clause 5 dispatches BEFORE cvc-type reaches either rule and its two
// arms disagree about which item is assessed. On clause 5.1's, an EMPTY element
// whose declaration carries a {value constraint}, the item is the one with
// D.{value constraint}.{lexical form} used as its ·normalized value· (5.1.2), so
// the string is that lexical and never the empty ·initial value·. On clause
// 5.2's it is E itself (5.2.1), so an empty element with no {value constraint}
// is charged for the empty string exactly as one carrying text is charged for
// its own — the ·initial value· being a concatenation of no runs, which every
// type whose lexical space excludes "" then rejects.
//
// GAP(validate): a ValidateLexical error that is not a VERDICT is the same
// fail-open cvcattribute.go's declaredAttribute states in full, over the same
// [value.IsDatatypeVerdict] classification: an ungoverned simple type reports
// under cvc-datatype-valid exactly as a genuine rejection does, and charging it
// would reject every element whose character content this backend cannot read.
// RULED permanent by #774 (STYLE P3b), on declaredAttribute's terms; a ·special·
// type is decided and never reaches it ([xsd.SimpleType.IsSpecial]). An
// undecidable ·validating type· withholds String Valid clause 3's verdict on the
// terms [walk.entitiesDeclared] states. Either decline is recorded by the caller
// as an [Unevaluated] under the rule it withholds ([contentCheck.decline]).
//
// st's assertions facets are evaluated inside String Valid, so a failed {test}
// is part of the verdict both clauses charge; one the evaluator declines leaves
// decided false with that decline as the error, which each caller records under
// cvc-assertions-valid ([walk.declineAssertions], cvcassertion.go) in place of
// its own clause's decline.
func (c *contentCheck) stringValid(w *walk, st *xsd.SimpleType) (decided bool, err error) {
	lexical, ctx := c.assessed()
	return w.stringValid(st, lexical, ctx, c.e.Loc())
}

// simpleTypeValue settles cvc-type clause 3.1.3: where E is not ·nilled·, its
// ·initial value· is ·valid· with respect to the ·governing type definition·
// itself as String Valid (§3.16.4) defines it. The charge carries the element's
// own location: the value is assembled from every run and belongs to none of
// them.
//
// A ·nilled· element is skipped, clause 3.1.3 being the one sub-clause of
// clause 3.1 that carries the "if E is not ·nilled·" condition — its
// [[children]] are cvc-elt clause 3.2.3.1's business instead, and clauses 3.1.1
// and 3.1.2 still apply to it. The gate is stated here because it is this
// clause's, on fixedValue's terms: gathers withholds the ·initial value· of a
// ·nilled· element as well, but that is one mechanism serving three clauses and
// not a statement of any of their conditions.
func (c *contentCheck) simpleTypeValue(w *walk) {
	st := c.g.simpleType()
	if st == nil || c.nilled {
		return
	}
	decided, verdict := c.stringValid(w, st)
	if !decided {
		if w.declineAssertions(verdict, "assessing content", c.e.Name(), c.e.Loc(), "cvc-type clause 3.1.3") {
			return
		}
		c.decline(w, c.e.Name(), c.e.Loc(), ruleCvcType, "3.1.3",
			"the ·initial value· of the element %s was not decided against its ·governing type definition· %s: String Valid (§3.16.4) was withheld, the value backend reporting a fault of the type rather than a verdict about the lexical or the ·validating type· of an ·ENTITY value· being undecidable, so cvc-type clause 3.1.3 is undecided",
			c.e.Name(), typeName(st))
		return
	}
	if verdict == nil {
		c.log(w, c.e.Name(), c.e.Loc(), ruleCvcType, "3.1.3", "satisfied")
		return
	}
	c.chargeCause(w, ruleCvcType, "3.1.3", c.e.Loc(), verdict,
		"the ·initial value· of the element %s is not ·valid· with respect to its ·governing type definition· %s, which cvc-type clause 3.1.3 requires as per String Valid (§3.16.4)",
		c.e.Name(), typeName(st))
}

// initialValue settles the ·initial value· half of clause 1.2: the ·initial
// value· is ·valid· with respect to T.{content type}.{simple type definition}
// as String Valid (§3.16.4) defines it. The charge carries the CONTAINING
// element's location, on simpleTypeValue's terms.
func (c *contentCheck) initialValue(w *walk, st *xsd.SimpleType) {
	decided, verdict := c.stringValid(w, st)
	if !decided {
		if w.declineAssertions(verdict, "assessing content", c.e.Name(), c.e.Loc(), "cvc-complex-type clause 1.2") {
			return
		}
		c.decline(w, c.e.Name(), c.e.Loc(), ruleCvcComplexType, "1.2",
			"the ·initial value· of the element %s was not decided against the {simple type definition} %s of its ·governing type definition·'s {content type}: String Valid (§3.16.4) was withheld, the value backend reporting a fault of the type rather than a verdict about the lexical or the ·validating type· of an ·ENTITY value· being undecidable, so cvc-complex-type clause 1.2 is undecided",
			c.e.Name(), st.Name())
		return
	}
	if verdict == nil {
		c.log(w, c.e.Name(), c.e.Loc(), ruleCvcComplexType, "1.2", "satisfied")
		return
	}
	c.chargeCause(w, ruleCvcComplexType, "1.2", c.e.Loc(), verdict,
		"the ·initial value· of the element %s is not ·valid· with respect to the {simple type definition} %s of its ·governing type definition·'s {content type}, which cvc-complex-type clause 1.2 requires as per String Valid (§3.16.4)",
		c.e.Name(), st.Name())
}

// sequenceEnd settles clause 1.4 for a sequence that ran out: every item was
// ·attributed to· a particle, but the particles left open cannot all be closed.
// The charge carries the CONTAINING element's location, there being no child at
// the offending position to carry one.
//
// Under a present {open content} the question is asked of S1 alone
// ([xsd.Matcher.Accepting]) and is still asked: clauses 2.2 and 3.2 require S1
// to be ·valid· with respect to {particle}, so a sequence whose open-content
// items were all admitted still fails where the particle they were interleaved
// with is left short of its {min occurs}.
//
// The charge names what the {particle} would have taken next
// ([xsd.Matcher.Admissible], rendered by expected) with the {open content}
// arm filtered out: an item the open wildcard takes is S2's and never advances
// S1, so it pays none of the {min occurs} that clauses 2.2 and 3.2 find owed.
// The charge says the {particle} expects the list, not the content model, since
// the {open content} may still take items the list leaves out. The list can be
// empty — under {mode} suffix once an item has begun S2 (clause 2.1's S = S1 +
// S2 being a concatenation), and where what is owed is a choice with no
// members, which accepts nothing — and expected then says the {particle}
// expects no further element information item, which is true of both.
//
// Under a nil matcher clause 1.4 is undecided for the sequence as a whole, the
// EMPTY sequence included: whether a {content type} accepts no element
// information item at all is the matcher's to say, and a particle whose {min
// occurs} is positive rejects it. Where an element [[child]] arrived,
// [contentCheck.match] has already recorded that decline at the child; where
// none did, it is recorded here at the element's own location, so the element
// never reaches [Result] with clause 1.4 neither decided nor recorded.
func (c *contentCheck) sequenceEnd(w *walk) {
	clause := c.contentClause()
	if c.matcher == nil {
		if !c.sawElement {
			c.decline(w, c.e.Name(), c.e.Loc(), ruleCvcComplexContent, clause,
				"the element %s has no element information item among its [[children]], and that empty sequence was not matched against the {content type} of its ·governing type definition·: xsd.Schema.ContentMatcher does not decide that {content type}'s shape, so whether the sequence is ·valid· with respect to it as cvc-complex-content clause %s requires is undecided",
				c.e.Name(), clause)
		}
		return
	}
	if c.matcher.Accepting() {
		c.log(w, c.e.Name(), c.e.Loc(), ruleCvcComplexContent, clause, "accepted")
		return
	}
	owed := slices.DeleteFunc(c.matcher.Admissible(), func(a xsd.Attribution) bool {
		_, open := a.(*xsd.OpenContent)
		return open
	})
	c.charge(w, ruleCvcComplexContent, clause, c.e.Loc(),
		"the [[children]] of %s end before every particle of the {content type} of its ·governing type definition· has taken the occurrences its {min occurs} requires, where that {content type}'s {particle} expects %s, so the sequence is not ·valid· with respect to that {content type} as cvc-complex-content clause %s requires",
		c.e.Name(), expected(owed, attributedTo), clause)
}

// charge records one violation and closes the element to any further content
// charge (see the file comment).
func (c *contentCheck) charge(w *walk, rule xsderr.Rule, clause string, loc xsderr.Loc, format string, args ...any) {
	c.chargeCause(w, rule, clause, loc, nil, format, args...)
}

// chargeCause is [contentCheck.charge] for a clause that DELEGATES: cause is
// the verdict the delegated rule returned, and [causedBy] states what carrying
// it buys and how format must end.
func (c *contentCheck) chargeCause(w *walk, rule xsderr.Rule, clause string, loc xsderr.Loc, cause error, format string, args ...any) {
	w.res.violations = append(w.res.violations, causedBy(rule, loc, cause, format, args...))
	c.charged = true
	c.log(w, c.e.Name(), loc, rule, clause, "charged")
}

// decline records one content check this element REACHED and did not perform,
// on [walk.decline]'s terms. Unlike a charge it leaves c.charged alone: nothing
// was decided, so no later clause is silenced by it.
func (c *contentCheck) decline(w *walk, name xsd.QName, loc xsderr.Loc, rule xsderr.Rule, clause, format string, args ...any) {
	w.decline("assessing content", name, loc, rule, clause, format, args...)
}

// log records one content decision: which rule and clause settled it, and how
// (STYLE L1, [walk.logDecision]). A check that settles nothing logs nothing —
// the walk's own "assessing element"/"assessing text" lines already record the
// visit.
func (c *contentCheck) log(w *walk, name xsd.QName, loc xsderr.Loc, rule xsderr.Rule, clause, outcome string) {
	w.logDecision("assessing content", name, loc, rule, clause, outcome)
}

// maxExpectedListed is how many particle arms of [xsd.Matcher.Admissible] a
// content charge names before it counts the rest: past it, expected adds "and K
// more", K the exact number of particle arms left unnamed, so a wide choice or
// all group cannot make one violation as long as its model. The {open content}
// arm is outside the bound and always named.
const maxExpectedListed = 5

// expected renders the arms a content charge names as what is expected at the
// charged position, each by name — attributedTo, or declining at a rejected
// item. The particle arms are named in the order they come in, at most
// maxExpectedListed of them and then "and K more"; an [*xsd.OpenContent] arm,
// which Admissible puts last, is always named, after them. One arm is named by
// itself, several as "one of" a comma-separated list — the {open content} arm
// joined by ", or" after an "and K more" — and none as "no further element
// information item", never an empty list.
func expected(arms []xsd.Attribution, name func(xsd.Attribution) string) string {
	if len(arms) == 0 {
		return "no further element information item"
	}
	last := arms[len(arms)-1]
	particles := arms
	_, hasOpen := last.(*xsd.OpenContent)
	if hasOpen {
		particles = arms[:len(arms)-1]
	}
	listed := particles[:min(len(particles), maxExpectedListed)]
	names := make([]string, 0, len(listed)+1)
	for _, a := range listed {
		names = append(names, name(a))
	}
	if rest := len(particles) - len(listed); rest > 0 {
		list := "one of " + strings.Join(names, ", ") + " and " + strconv.Itoa(rest) + " more"
		if !hasOpen {
			return list
		}
		return list + ", or " + name(last)
	}
	if hasOpen {
		names = append(names, name(last))
	}
	if len(names) == 1 {
		return names[0]
	}
	return "one of " + strings.Join(names, ", ")
}

// attributedTo names what an item was or could be ·attributed to· (§3.4.4.4),
// for the log line [contentCheck.match] writes and for the arms a content charge
// names (expected), switching over the three variants [xsd.Attribution] seals
// (STYLE T2's closed-sum exception). An element declaration is named by its
// ·expanded name· in the notation the charged item's own name is rendered in,
// and a wildcard by its namespace constraint (namespaceConstraint), behind the
// word "wildcard" so neither reads as an element's name. The {open content} arm
// is spelled apart from the ·wildcard particle· one because that is the
// distinction the log exists to show: a reader asking why e-validity clause
// 1.1.3 charged one unresolvable child and not another reads the answer off
// these two lines. What the descent reads off the same Attribution — the child's
// ·governing element declaration·, including the ·substituting declaration· a
// substituting item carries rather than the particle's own — is
// [walk.childGoverning]'s, and the two are deliberately separate: a log line
// names what consumed the item, which is a fact about the PARENT's content
// model.
func attributedTo(a xsd.Attribution) string {
	switch t := a.(type) {
	case xsd.ElementDeclaration:
		return "element declaration " + t.Name().String()
	case xsd.Wildcard:
		return "wildcard " + namespaceConstraint(t.NamespaceConstraint())
	case *xsd.OpenContent:
		return "open content wildcard " + namespaceConstraint(t.Wildcard().NamespaceConstraint())
	default:
		return "particle"
	}
}

// declining names an arm [contentCheck.match] offered child to and that
// declined it, as attributedTo does, followed by ", which disallows <child>" for
// a ·wildcard particle· or {open content} arm whose {namespace constraint}
// allows child's namespace: such an arm declined child under its {disallowed
// names} — a QName member (cvc-wildcard-name clause 2) or the defined or
// sibling keyword (cvc-wildcard clauses 2 and 3) — which namespaceConstraint
// does not render. The switch is over the two wildcard-bearing variants
// [xsd.Attribution] seals (STYLE T2's closed-sum exception).
func declining(child xsd.QName) func(xsd.Attribution) string {
	ns := xsd.NamespaceName(child.Space)
	return func(a xsd.Attribution) string {
		var nc xsd.NamespaceConstraint
		switch t := a.(type) {
		case xsd.Wildcard:
			nc = t.NamespaceConstraint()
		case *xsd.OpenContent:
			nc = t.Wildcard().NamespaceConstraint()
		default:
			return attributedTo(a)
		}
		if !nc.AllowsNamespace(ns) {
			return attributedTo(a)
		}
		return attributedTo(a) + ", which disallows " + child.String()
	}
}

// namespaceConstraint renders a wildcard's {namespace constraint} (§3.10.1) as
// its {variety} followed by its {namespaces} in their own order, each quoted or
// ·absent·: "any", `enumeration "urn:a" ·absent·`, `not "urn:a"`. {disallowed
// names} is not rendered.
func namespaceConstraint(nc xsd.NamespaceConstraint) string {
	s := nc.Variety().String()
	for _, ns := range nc.Namespaces() {
		uri, present := ns.URI()
		if !present {
			s += " ·absent·"
			continue
		}
		s += " " + strconv.Quote(uri)
	}
	return s
}

// xmlWhitespace is white space as XML 1.1 defines it (S ::= (#x20 | #x9 | #xD
// | #xA)+), the set clause 1.3 allows an element-only {content type} to carry
// and the one whiteSpace = collapse removes (collapseXMLWhitespace,
// cvcelt.go). One encoding serves both (STYLE T4).
const xmlWhitespace = " \t\r\n"

// isXMLWhitespace reports whether every character of s is xmlWhitespace.
func isXMLWhitespace(s string) bool {
	return strings.TrimLeft(s, xmlWhitespace) == ""
}
