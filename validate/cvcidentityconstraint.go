package validate

import (
	"strings"

	"github.com/kud360/goxsd8/icpath"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xpath"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// ruleCvcIdentityConstraint is Identity-constraint Satisfied (Structures
// §3.11.4, cvc-identity-constraint). The clause charged goes in the message on
// ruleCvcElt's terms: the catalog carries the bare name, so
// "cvc-identity-constraint.4.2.1" is not a valid [xsderr.Rule].
const ruleCvcIdentityConstraint xsderr.Rule = "cvc-identity-constraint"

// This file decides cvc-identity-constraint (§3.11.4) for every element the
// walk assesses, and builds the [identity-constraint table] of §3.11.5 for
// every element it visits. The path evaluation the two rest on is the icpath
// package's.
//
// The whole design follows from one sentence of §3.11.5: an element's node
// tables are "assembled strictly recursively from the node tables of
// descendants". Node tables therefore propagate UPWARD and only upward
// (PRINCIPLES 15) — which is what makes clause 4.3's Note true, that "only
// element information items within the sub-tree rooted at the element
// information item being ·validated· can be referenced successfully", and what
// makes a keyref on E blind to a key sourced in a SIBLING's subtree.
//
// The walk is the recursion §3.11.5 asks for, so the state rides it rather than
// being rebuilt over a tree the engine never holds ([Children] is a pull
// cursor):
//
//   - ENTERING an element, [walk.identityCheck] advances every live selector
//     and field path one level ([icpath.Live.Advance]), opens a frame for each
//     identity constraint its own ·governing element declaration· declares, and
//     registers the element as a ·target node· or as a field node wherever a
//     path completed.
//   - LEAVING it, [walk.identityExit] fills the field slots this element's own
//     ·initial value· supplies, settles clauses 3 and 4 for the frames rooted
//     here, merges its children's node tables with its own qualified entries
//     under §3.11.5's conflict resolution, settles clause 4.3 against the
//     result, and hands the merged table to its parent.
//
// Clause 2 — "each node in the ·target node set· is either the context node or
// an element node among its descendants" — is satisfied by construction and
// never charged: a selector cursor is seeded at the element the constraint is
// declared on and only ever advances DOWNWARD into that element's [[children]],
// so no path this file evaluates can reach a node outside the subtree.
//
// ·Skipped· nodes need no filtering either (clause 1's "after omitting all
// element nodes corresponding to element information items that are
// ·skipped·"): [walk.child] returns before [walk.element] for a child
// ·attributed to· a skip wildcard or to an {open content} with a skip
// {wildcard} ([walk.childGoverning]), so no icCheck is ever built for it or for
// anything beneath it, and its subtree contributes no target, no field value
// and no node-table entry.

// icCheck is the identity-constraint state of ONE element information item,
// built by [walk.identityCheck] as the walk enters it and settled by
// [walk.identityExit] as the walk leaves. It is the analogue of contentCheck
// for §3.11.4, and like it holds nothing that outlives its element — except
// table, which is the one thing §3.11.5 has travel upward.
//
// sels and flds are the cursors live AT this element, ready to advance into its
// [[children]]; frames are the constraints declared on this element, whose
// targets and node table close here. pending records the field slots this
// element's OWN ·initial value· fills, which is why gather exists: an element's
// character content is complete only once its [[children]] are exhausted, so a
// field node's value is recorded on the way out and not on the way in.
//
// defaulted and hasDefault are the {value constraint} cvc-elt clause 5.1
// substituted for that ·initial value·, written by substitute once the
// [[children]] are exhausted and read by assessed. They are not derivable from
// initial: a {lexical form} of "" substitutes the same empty string an element
// with no character [[children]] already has, and the two are different answers.
type icCheck struct {
	e      Element
	g      governance
	node   int
	parent *icCheck

	frames []*icFrame
	sels   []icSelCursor
	flds   []icFieldCursor

	pending    []icPending
	gather     bool
	initial    strings.Builder
	defaulted  xsd.ValueConstraint
	hasDefault bool

	table icTable
}

// icPending is one field slot waiting on the ·initial value· of the element
// whose icCheck holds it.
type icPending struct {
	target *icTarget
	index  int
}

// icSelCursor is one constraint's {selector} evaluation, live at one element.
type icSelCursor struct {
	frame *icFrame
	live  icpath.Live
}

// icFieldCursor is one ·target node·'s evaluation of one {fields} member, live
// at one element. index is that member's position in {fields}, which is also
// its position in the ·key-sequence· (§3.11.4 clause 3, "in the order of the
// {fields} property").
type icFieldCursor struct {
	target *icTarget
	index  int
	live   icpath.Live
}

// icFrame is one identity constraint being evaluated against the element it is
// declared on — E, in §3.11.4's terms. Its targets accumulate as the descent
// finds them and are settled together when E's [[children]] are exhausted,
// because a ·target node·'s ·key-sequence· is complete only once its own
// subtree has been walked.
//
// declined marks a constraint whose {selector} or {fields} the icpath package
// could not compile. It charges nothing, and — through icTable's own declined
// flag — no keyref referring to it charges either, so a path this processor
// cannot read costs a rejection in neither direction.
//
// GAP(xpath): an {expression} outside the ·selector subset· (§3.11.6.2) or the
// ·field subset· (§3.11.6.3) is DECLINED by [icpath.CompileSelector] and
// [icpath.CompileField], and the identity constraint carrying it charges
// nothing at all. Charging on a path this processor cannot read would reject a
// document for a gap in the processor. The withheld value's whole consumer set
// is Result.violations, reached through icCheck.open setting this flag, and its
// one reader Result.Violations; both carry violations PRESENT, so withholding
// one can only cost a rejection and never manufacture one. The decline itself
// is recorded as an [Unevaluated] ([icCheck.declineFrame]).
//
// WHAT STILL ARRIVES HERE, now that parser charges c-selector-xpath and
// c-fields-xpaths over the shapes [icpath.SelectorViolation] enumerates and
// icpath compiles the `child::` and `attribute::` spellings clause 2.2 admits
// (#1887 owns what follows): a legal XPath 2.0 expression neither subset admits
// that no charged shape covers and icpath's lexer cannot read whole — a KindTest
// with an argument (`element(a)`), a FunctionCall with a prefixed name or an
// argument other than a StringLiteral (`p:f('x')`, `f(a)`), a Wildcard `*:a`,
// and any other operator or literal outside a predicate; a predicate holding a
// token icpath's lexer does not read (`a[1]`, `a[b!='c']`); a non-expression
// icpath does not charge: Steps with no separator, the parent step `..`
// among them (#1829), a colon run it cannot read, or any other stream the
// lexer cannot read whole, an unread rune or an unfinished token (`a#b`,
// `f('x'`, `item()`); a `.//` with no element step left once the self steps are
// removed, which production [3]'s bare `.` Step makes assembly-LEGAL and only
// this matcher cannot represent; and — because a component assembled directly
// through [xsd.NewIdentityConstraint] reaches no assembler — any charged shape
// as well.
type icFrame struct {
	ic       xsd.IdentityConstraint
	sel      icpath.Expr
	fields   []icpath.Expr
	targets  []*icTarget
	declined bool
}

// icTarget is one member of a constraint's ·target node set· (§3.11.4 clause
// 1), with one slot per {fields} member. node is the ordinal the walk assigns
// each element it visits, which is this package's node IDENTITY: §3.11.5's
// conflict resolution turns on "the same key-sequence but distinct nodes", and
// an [Element] is an interface an adapter implements, so == on one compares
// whatever the adapter's dynamic type compares.
type icTarget struct {
	frame *icFrame
	e     Element
	node  int
	slots []icSlot
}

// icSlot is what one {fields} member selected for one ·target node·.
//
// Clause 3 admits a field's node sequence only if it holds "at most one node
// whose ·governing· type definition is either a simple type definition or a
// complex type definition with {variety} simple, and no other nodes" (beside
// ·skipped· ones, which never reach a slot). taken is that one node, counted by
// its ·governing type definition· alone, and member its value where it has one
// ([icSlot.filled]); a slot not filled and not charged contributes no member,
// which shortens the ·key-sequence· and drops the node out of the ·qualified
// node set·; charge is the first clause 3 violation, if any. loc is that
// charge's node where there is one, and the filled member's otherwise.
//
// declined is not a state of the rule but this processor's own: a field node
// whose ·governing type definition· could not be determined has no [schema
// actual value] to read (§3.3.5.4, which defines the property only "if and
// only if a governing type definition is known"), and a lexical comparison in
// its place is licensed nowhere.
type icSlot struct {
	taken    icTaken
	member   icKeyMember
	charge   icClause3
	declined bool
	loc      xsderr.Loc
}

// icTaken is the one node of a simple-valued ·governing type definition· a
// field's node sequence may hold under cvc-identity-constraint clause 3, if it
// has been met: none yet, one whose [schema actual value] is ·absent·, or one
// whose value is not.
type icTaken uint8

const (
	// icTakenNone is a slot that has met no simple-valued node.
	icTakenNone icTaken = iota
	// icTakenAbsent is a ·nilled· node (§3.3.4.3, key-nilled) or one whose
	// lexical lies outside its type's lexical space, both of which §3.3.5.4
	// gives an ·absent· [schema actual value]: it counts toward clause 3's "at
	// most one" and contributes no ·key-sequence· member.
	icTakenAbsent
	// icTakenValued is a node with a non-absent [schema actual value], which
	// member holds.
	icTakenValued
)

// filled reports whether the slot contributes a ·key-sequence· member.
func (s *icSlot) filled() bool {
	return s.taken == icTakenValued
}

// icClause3 is which of cvc-identity-constraint clause 3's bounds a field's
// node sequence broke first: none, a second simple-valued node, or a node of a
// type clause 3 admits as none of its nodes.
type icClause3 uint8

const (
	// icClause3None is a sequence within clause 3's bounds.
	icClause3None icClause3 = iota
	// icClause3Valued is a second node whose ·governing type definition· is a
	// simple type definition or a complex type definition with {variety}
	// simple, over clause 3's "at most one" — whatever either node's [schema
	// actual value], ·absent· included.
	icClause3Valued
	// icClause3Other is a node whose ·governing type definition· is determined
	// and is neither a simple type definition nor a complex type definition
	// with {content type}.{variety} simple, one of clause 3's "other nodes",
	// ·nilled· or not.
	icClause3Other
)

// icKeyMember is one member of a ·key-sequence·: an ·actual value· in the value
// space its own type governs, never a lexical (Datatypes §2.2, and see
// [walk.sameKeyMember]). The one exception is a member of a ·special· st, whose
// lexical names no one ·actual value·: v is nil, and lexical alone is compared
// ([sameSpecialMember]).
//
// lexical and ctx are the lexical v was read off and the namespace context it
// was mapped under. They are read only where the two members of a pair were
// validated against different simple types, or against one whose values lie in
// more than one value space, which [walk.primitiveItems] answers by re-reading
// lexical in the ·primitive· value space of its ·validating type·; v alone
// cannot answer it, since a backend may map a derived type into a space of its
// own. st stays the declared type, a union included: the ·validating type· is
// identified per comparison, so a failure to identify it declines that
// comparison and never the member.
//
// element and nillable travel with it for clause 4.2.3 alone, which asks
// whether an ELEMENT member "was assessed as ·valid· by reference to an element
// declaration whose {nillable} is true". The spec's own Note licenses reading
// the declaration's property directly where the PSVI contribution recording it
// is not provided, which this package does not provide.
type icKeyMember struct {
	st       *xsd.SimpleType
	v        value.Value
	lexical  string
	ctx      value.Context
	element  bool
	nillable bool
}

// icKeySequence is the ·key-sequence· of one ·target node· (§3.11.4 clause 3),
// in {fields} order.
type icKeySequence []icKeyMember

// identityCheck opens the identity-constraint state of one element: the cursors
// its parent's live paths reach it with, and the frames its own ·governing
// element declaration· declares.
//
// The node ordinal is assigned here, once per element the walk enters, so it
// increases in document order and no two elements share one.
func (w *walk) identityCheck(e Element, g governance, parent *icCheck) *icCheck {
	w.nodes++
	c := &icCheck{e: e, g: g, node: w.nodes, parent: parent}
	if parent != nil {
		c.inherit(w, parent)
	}
	c.open(w, g)
	if candidate, decided := w.idCandidate(g.valueType()); !decided || candidate {
		// The ·initial value· of an element clause 3 of the ·eligible item set·
		// admits is read on the way out (cvcid.go's idElement), so its character
		// content has to be gathered on the way through. The gate is the
		// CANDIDACY one and never the classification, which needs that value to
		// run at all, and it gathers for an undecided answer too so that it is
		// never narrower than idRecord's own.
		c.gather = true
	}
	return c
}

// inherit advances every cursor live at the parent one level down, into c's
// element, and records what completed there: a {selector} path completing makes
// c's element a ·target node· of that frame, a {fields} path completing makes it
// (or one of its [[attributes]]) that target's field node.
func (c *icCheck) inherit(w *walk, parent *icCheck) {
	for _, cur := range parent.sels {
		live, sel := cur.live.Advance(c.e.Name())
		c.sels = append(c.sels, icSelCursor{frame: cur.frame, live: live})
		if sel.SelectsElement() {
			c.addTarget(w, cur.frame)
		}
	}
	for _, cur := range parent.flds {
		live, sel := cur.live.Advance(c.e.Name())
		c.flds = append(c.flds, icFieldCursor{target: cur.target, index: cur.index, live: live})
		if sel.SelectsElement() {
			c.pendElement(cur.target, cur.index)
		}
		if sel.SelectsAttributes() {
			c.fieldAttributes(w, cur.target, cur.index, sel)
		}
	}
}

// open builds one frame per identity constraint of c's ·governing element
// declaration· — §3.11.4 quantifies over "an identity-constraint", and
// cvc-elt clause 6 applies it to each member of the declaration's
// {identity-constraint definitions}.
//
// An element with no ·governing element declaration· declares no constraint:
// its own subtree is still walked, and still propagates its descendants' node
// tables upward, because §3.11.5's clause 1 quantifies over the children's
// tables and not over anything the element itself declares.
func (c *icCheck) open(w *walk, g governance) {
	if !g.hasDecl {
		return
	}
	for _, ic := range g.decl.IdentityConstraints() {
		f := &icFrame{ic: ic}
		c.frames = append(c.frames, f)
		sel, ok := icpath.CompileSelector(ic.Selector())
		if !ok {
			c.declineFrame(w, f, "{selector}")
			continue
		}
		for _, x := range ic.Fields() {
			fx, ok := icpath.CompileField(x)
			if !ok {
				c.declineFrame(w, f, "{fields}")
				break
			}
			f.fields = append(f.fields, fx)
		}
		if f.declined {
			continue
		}
		f.sel = sel
		c.sels = append(c.sels, icSelCursor{frame: f, live: sel.Start()})
		if sel.Self().SelectsElement() {
			c.addTarget(w, f)
		}
	}
}

// declineFrame declines f, a constraint declared on c's element whose part —
// its {selector} or one of its {fields} — icpath could not compile, and records
// the withheld clauses on [walk.decline]'s terms at c's element.
func (c *icCheck) declineFrame(w *walk, f *icFrame, part string) {
	f.declined = true
	w.decline("assessing identity constraint", c.e.Name(), c.e.Loc(), ruleCvcIdentityConstraint, "4",
		"element %s declares the identity constraint %s, which was not evaluated: its %s lies outside the path subset this processor compiles, so cvc-identity-constraint clauses 3 and 4 are undecided for it",
		c.e.Name(), f.ic.Name(), part)
}

// addTarget records c's element as a member of f's ·target node set· and opens
// one field cursor per {fields} member with that element as the context node,
// settling on the spot the two shapes advance never sees: a field selecting the
// target itself (`.`) and one selecting an attribute of it (`@id`).
func (c *icCheck) addTarget(w *walk, f *icFrame) {
	t := &icTarget{e: c.e, node: c.node, slots: make([]icSlot, len(f.fields)), frame: f}
	f.targets = append(f.targets, t)
	for i := range f.fields {
		c.flds = append(c.flds, icFieldCursor{target: t, index: i, live: f.fields[i].Start()})
		sel := f.fields[i].Self()
		if sel.SelectsElement() {
			c.pendElement(t, i)
		}
		if sel.SelectsAttributes() {
			c.fieldAttributes(w, t, i, sel)
		}
	}
}

// pendElement records that c's own element is the field node for one slot. The
// value is read on the way out (icCheck.fill), so gather is set here to start
// collecting the character information item [[children]] the ·initial value· is
// the concatenation of.
func (c *icCheck) pendElement(t *icTarget, i int) {
	c.pending = append(c.pending, icPending{target: t, index: i})
	c.gather = true
}

// fieldAttributes settles one slot from c's element's [[attributes]], for the
// field paths ending in `@NameTest` that completed here. Every matching
// attribute is offered to the slot, not just the first: clause 3 bounds the
// sequence at one node of a simple ·governing type definition·, whatever its
// value, and a NameTest of `@*` or `@p:*` can select several, which the slot
// records as the clause 3 violation it is.
//
// The scan is over the ATTRIBUTES and not over the tests, so an attribute two
// branches of a union both select is offered once. An XPath union is a sequence
// of distinct NODES, so offering it twice would charge clause 3 for a field
// like `@id|@*` that selects exactly one.
//
// An attribute whose ·governing type definition· [walk.attributeType] could not
// name declines the slot rather than being skipped: the [schema actual value]
// clause 3 reads is that type's, and comparing the member under the wrong
// simple type decides clause 4.1 by the wrong ·value space·.
//
// A ·skipped· attribute is the OTHER absence and leaves the slot simply
// unfilled: §3.11.4 clause 3's Note names a field evaluating to "a sequence
// consisting only of ·skipped· or ·nilled· nodes" as leaving the ·key-sequence·
// short, and a short one keeps its ·target node· out of the ·qualified node
// set·, which for a key is [icFrame.keyOnly]'s clause 4.2.1 charge. The charge
// is made and not withheld because the absence is the SPEC's and not this
// processor's: [walk.skippedAttribute] decides the ·attribution· through
// cvc-wildcard rather than inferring it from the wildcard's presence, so an
// attribute that wildcard does not admit is typed by key-governing-ad clause 3
// instead and never arrives here as ·skipped·.
//
// The ·defaulted attributes· of c's element are offered too, after the
// attributes it carries ([icCheck.fieldDefaultedAttributes]).
func (c *icCheck) fieldAttributes(w *walk, t *icTarget, i int, sel icpath.Selection) {
	attrs := c.e.Attributes()
	for _, a := range attrs {
		if !sel.SelectsAttribute(a.Name()) {
			continue
		}
		st, typed := w.attributeType(c.g, a)
		if !typed {
			if w.skippedAttribute(c.g, a) {
				continue
			}
			t.decline(w, i, a.Name(), a.Loc(), "its ·governing type definition· could not be determined")
			continue
		}
		m, present, decided := w.keyMember(st, a.Value(), elementContext{owner: c.e}, false, false)
		t.offer(w, i, a.Name(), a.Loc(), m, present, decided)
	}
	if ct := c.g.complexType(); ct != nil {
		c.fieldDefaultedAttributes(w, t, i, sel, attrs, *ct)
	}
}

// fieldDefaultedAttributes offers slot i each ·defaulted attribute· of c's
// element that sel selects, ct being its governing Complex Type Definition and
// attrs its [[attributes]]. §3.11.4 clause 3's first Note is explicit that "the
// use of [schema actual value] ... means that default or fixed value
// constraints may play a part in ·key-sequence·s", and Attribute Default Value
// (§3.4.5.1, sic-attrDefault) puts the item in the PSVI with the ·effective
// value constraint·'s {value}. Which uses are defaulted is
// [walk.defaultedConstraint]'s to say (key-dflt-att), read here as
// [walk.idDefaultedAttributes] reads it for cvc-id. The member is typed by the
// use's declaration's {type definition} and compared by value, like any other.
// The item is synthesized and has no source position, so it cites the owner's.
// A wildcard attribute is never defaulted, so the ·skipped· arm above has no
// counterpart here.
//
// GAP(validate): a use whose {attribute declaration} does not resolve, or whose
// {type definition} is not a resolvable simple type, declines the slot. The
// first is unreachable on a *xsd.Schema that exists and records nothing; the
// second is the absent-or-COMPLEX {type definition} [walk.declaredAttribute]'s
// doc records, and is recorded as an [Unevaluated] ([icTarget.decline]). The
// decline withholds cvc-identity-constraint clauses 3 and 4 for that
// constraint ([icFrame.qualify]). RULED permanent by #774 (STYLE P3b), on
// cvcattribute.go's terms.
func (c *icCheck) fieldDefaultedAttributes(w *walk, t *icTarget, i int, sel icpath.Selection, attrs []Attribute, ct xsd.ComplexType) {
	for _, u := range ct.AttributeUses() {
		if !sel.SelectsAttribute(u.DeclarationName()) {
			continue
		}
		vc, defaulted := w.defaultedConstraint(u, attrs)
		if !defaulted {
			continue
		}
		d, resolved := w.schema.ResolvedAttributeDeclaration(u)
		if !resolved {
			// Unreachable on a *xsd.Schema that exists, so it records no
			// [Unevaluated] no schema can produce.
			t.slots[i].declined = true
			continue
		}
		st, simple := w.schema.ResolvedSimpleType(d.TypeDefinition())
		if !simple {
			t.decline(w, i, u.DeclarationName(), c.e.Loc(), "it is a ·defaulted attribute· whose declaration's {type definition} is absent")
			continue
		}
		m, present, decided := w.keyMember(st, vc.LexicalForm(), value.ConstraintContext(vc), false, false)
		t.offer(w, i, u.DeclarationName(), c.e.Loc(), m, present, decided)
	}
}

// text gathers one run of character information items into the ·initial value·
// a field node's [schema actual value] is read off, for an element some field
// path selected and for no other (see pendElement). An element no field selects
// holds nothing here.
func (c *icCheck) text(t Text) {
	if !c.gather {
		return
	}
	c.initial.WriteString(t.Data())
}

// substitute takes cvc-elt clause 5's case split from the content check that
// consumed the same [[children]] ([contentCheck.defaulted]), which is the one
// reading of it (STYLE T4): only that check records whether E has element or
// character information item [[children]] at all, and §3.11.4 and §3.17.5.2 read
// the value the split settles rather than deciding it a second time.
//
// It is called once per element, after the [[children]] are exhausted and before
// anything reads assessed ([walk.element]).
func (c *icCheck) substitute(content *contentCheck) {
	c.defaulted, c.hasDefault = content.defaulted()
}

// assessed is the ·initial value· cvc-elt clause 5 leaves this element assessed
// on, which is what §3.11.4 clause 3 and §3.17.5.2 both read a [schema actual
// value] off, paired with the namespace context it is mapped under, on
// [contentCheck.assessed]'s terms: D.{value constraint}.{lexical form} under
// [value.ConstraintContext] on clause 5.1's arm, and the gathered ·initial
// value· under E's own bindings (elementContext) on clause 5.2's.
//
// §3.11.4's own Note is why the substituted one reaches here and not just
// cvc-type: "the use of [schema actual value] in the definition of ·key sequence·
// above means that default or fixed value constraints may play a part in
// ·key-sequences·", and §3.17.5.2's Note says the same of the ·eligible item set·.
func (c *icCheck) assessed() (lexical string, ctx value.Context) {
	if c.hasDefault {
		return c.defaulted.LexicalForm(), value.ConstraintContext(c.defaulted)
	}
	return c.initial.String(), elementContext{owner: c.e}
}

// identityExit settles everything about one element that only its exhausted
// [[children]] can settle, in the order §3.11.5 and §3.11.4 depend on:
//
//  1. the field slots this element's own ·initial value· fills;
//  2. clauses 3 and 4.1/4.2 for the frames rooted here, which is also what
//     produces this element's clause 2 entries (§3.11.5's c-kc);
//  3. the merge of those with the children's entries, under §3.11.5's conflict
//     resolution;
//  4. clause 4.3 for the keyref frames rooted here, which reads the table step
//     3 just finished — §3.11.4 calls §3.11.5 "logically prior to this clause";
//  5. the handover of the merged table to the parent, which is the whole of the
//     upward propagation.
func (w *walk) identityExit(c *icCheck) {
	c.fill(w)
	c.evaluate(w)
	c.table.resolveConflicts(w)
	c.keyrefs(w)
	if c.parent != nil {
		c.parent.table.absorb(c.table)
	}
}

// fill records c's element as the field node it was selected as, from its own
// ·governing type definition· and its own ·initial value·, in one of two arms
// before any value is read.
//
// No ·governing type definition· at all (a {type table} carrying a {test} the
// §3.12.6 evaluator declines, an unresolvable slot, an xsi:type whose
// ·override· could not be decided, an element whose governance this walk could
// not decide (undecided), or one ·laxly assessed· with no ·instance-specified type
// definition·) declines rather than contribute a value, recorded as an
// [Unevaluated] ([icTarget.decline]).
//
// A determined one that is neither a simple type definition nor a complex type
// with {content type}.{variety} simple — governance.valueType's nil — makes
// the node one of cvc-identity-constraint clause 3's "other nodes", and the
// slot is charged for it ([icSlot.other]): clause 3's list is closed and reads
// only the governing type.
//
// The package doc's Contract section states what a ·nilled· element (§3.3.4.3,
// key-nilled) contributes as a field node, of either kind of type (§3.3.5.4;
// §3.11.4 clause 3's Note). [icFrame.keyOnly] charges a key for the short
// ·key-sequence· under clause 4.2.1, and [icSlot.record] counts the node toward
// clause 3.
//
// An EMPTY element whose declaration carries a {value constraint} is not among
// them: cvc-elt clause 5.1 replaces the item assessed with one whose ·normalized
// value· is that constraint's {lexical form}, and that lexical is what the field
// node contributes (assessed, #853).
func (c *icCheck) fill(w *walk) {
	if len(c.pending) == 0 {
		return
	}
	var m icKeyMember
	present, decided := false, false
	switch {
	case c.g.typ == nil:
		// decided stays false, which offer records as the decline.
	case c.g.valueType() == nil:
		for _, p := range c.pending {
			p.target.slots[p.index].other(c.e.Loc())
		}
		return
	default:
		m, present, decided = w.elementKeyMember(c)
	}
	for _, p := range c.pending {
		p.target.offer(w, p.index, c.e.Name(), c.e.Loc(), m, present, decided)
	}
}

// elementKeyMember is the ·key-sequence· member c's element contributes as a
// field node, on fill's terms: fill calls it only where the ·governing type
// definition· is simple-valued.
func (w *walk) elementKeyMember(c *icCheck) (icKeyMember, bool, bool) {
	st := c.g.valueType()
	if nilled(c.e, c.g) {
		return icKeyMember{}, false, true
	}
	nillable := c.g.hasDecl && c.g.decl.Nillable()
	lexical, ctx := c.assessed()
	return w.keyMember(st, lexical, ctx, true, nillable)
}

// keyMember maps one field node's lexical to the ·actual value· that is its
// [schema actual value], through the same String Valid (§3.16.4) pipeline the
// attribute charges run (value.ValidateLexical, under ctx: the namespace
// bindings in scope at the node that owns the lexical, elementContext, for a
// lexical the instance carries, and [value.ConstraintContext] for a {value
// constraint}'s {lexical form} — a ·defaulted attribute·'s or an element
// default's).
//
// The three answers are the three the rule distinguishes. present=true is a
// non-absent [schema actual value]. present=false with decided=true is an
// ABSENT one — a lexical outside the type's lexical space has no ·actual value·
// (§3.3.5.4 clause 1.3), so the node contributes nothing to the ·key-sequence·
// and its own invalidity is cvc-attribute's or cvc-complex-type's to charge,
// not this rule's; it still counts toward clause 3's "at most one"
// ([icSlot.record]). decided=false is this processor declining.
//
// GAP(validate): that decline covers a value.ValidateLexical error that is a
// fault of the TYPE or of the backend rather than a verdict about the lexical
// (value.IsDatatypeVerdict) — an ungoverned type above all, which reports under
// cvc-datatype-valid exactly as a genuine rejection does. Reading one as
// "absent" would silently shorten a ·key-sequence·, and a short one is what
// clause 4.2.1 charges a key for. RULED permanent by #774 (STYLE P3b), on
// cvcattribute.go's terms: an ungoverned type is backend coverage. An
// assertions-facet decline (value.IsAssertionDeclined) declines here too, as
// [walk.declineAssertions]' residue (#1042). The caller records the decline
// ([icTarget.offer]).
//
// A ·special· st ([xsd.SimpleType.IsSpecial]) is decided PRESENT with no v: its
// [schema actual value] is not ·absent·, yet its lexical names no one ·actual
// value·: the oracle ruled which one it contributes undecidable from the spec
// text. The member carries its lexical, which is already its
// normalized value — neither type has a whiteSpace facet (§4.3.6) — and
// [sameSpecialMember] decides a pair on it only where both lexicals are
// byte-identical, declining every other pair it is in, RULED permanent by #2124
// (STYLE P3b).
func (w *walk) keyMember(st *xsd.SimpleType, lexical string, ctx value.Context, element, nillable bool) (icKeyMember, bool, bool) {
	if st == nil {
		return icKeyMember{}, false, false
	}
	if st.IsSpecial() {
		return icKeyMember{st: st, lexical: lexical, ctx: ctx, element: element, nillable: nillable}, true, true
	}
	v, err := value.ValidateLexical(w.backend, w.schema, st, lexical, ctx, xpath.FacetAssertions(w.now))
	if err != nil {
		return icKeyMember{}, false, value.IsDatatypeVerdict(err)
	}
	return icKeyMember{st: st, v: v, lexical: lexical, ctx: ctx, element: element, nillable: nillable}, true, true
}

// offer takes one candidate field node's answer — the node named name at loc —
// into slot i, declining it where the node's value was undecided.
func (t *icTarget) offer(w *walk, i int, name xsd.QName, loc xsderr.Loc, m icKeyMember, present, decided bool) {
	if !decided {
		t.decline(w, i, name, loc, "its [schema actual value] could not be read, a fault of its type or of the value backend rather than a verdict about its lexical")
		return
	}
	t.slots[i].record(m, present, loc)
}

// decline poisons slot i outright, for the field node named name at loc, and
// records it on [walk.decline]'s terms: the value the node did not yield could
// have been the one that qualified the target, and guessing either way is what
// the decline exists to avoid. why states what could not be read.
func (t *icTarget) decline(w *walk, i int, name xsd.QName, loc xsderr.Loc, why string) {
	t.slots[i].declined = true
	w.decline("assessing identity constraint", name, loc, ruleCvcIdentityConstraint, "3",
		"node %s, which the field %q of the identity constraint %s selected for the ·target node· %s, was not read: %s, so cvc-identity-constraint clauses 3 and 4 are undecided for that constraint",
		name, t.frame.ic.Fields()[i].Expression(), t.frame.ic.Name(), t.e.Name(), why)
}

// record takes one decided field node of a simple-valued ·governing type
// definition· into the slot, per clause 3's bound of at most one such node,
// which counts nodes and not values: a second one is charged at its own loc,
// unless the slot already carries a charge, whether either [schema actual
// value] is ·absent· or not. An absent one (present=false) is taken without a
// member, so the slot stays unfilled and the ·key-sequence· short.
func (s *icSlot) record(m icKeyMember, present bool, loc xsderr.Loc) {
	if s.taken != icTakenNone {
		s.charge3(icClause3Valued, loc)
		return
	}
	if !present {
		s.taken = icTakenAbsent
		return
	}
	s.taken, s.member = icTakenValued, m
	if s.charge == icClause3None {
		s.loc = loc
	}
}

// other charges the slot for a field node at loc that clause 3 admits as none
// of its nodes (icClause3Other), unless the slot already carries a charge.
func (s *icSlot) other(loc xsderr.Loc) {
	s.charge3(icClause3Other, loc)
}

// charge3 records the slot's clause 3 charge as c at loc; the first charge
// wins, and its loc is the one reported.
func (s *icSlot) charge3(c icClause3, loc xsderr.Loc) {
	if s.charge != icClause3None {
		return
	}
	s.charge, s.loc = c, loc
}

// sequence is the target's ·key-sequence·: the values of its filled slots, in
// {fields} order. It is only meaningful for a member of the ·qualified node
// set·, where every slot is filled by construction.
func (t *icTarget) sequence() icKeySequence {
	seq := make(icKeySequence, 0, len(t.slots))
	for i := range t.slots {
		if !t.slots[i].filled() {
			continue
		}
		seq = append(seq, t.slots[i].member)
	}
	return seq
}

// evaluate settles clause 4 for the key and unique frames rooted at c's
// element, and records their ·qualified node set· as this element's own clause
// 2 entries in §3.11.5's c-kc.
//
// The keyref frames are NOT settled here but in keyrefs, after the table is
// merged and conflict-resolved: clause 4.3 reads "a node table associated with
// the {referenced key} in the [identity-constraint table] of E", and a key
// declared on E itself contributes to that table through this very pass.
//
// §3.11.5's ·eligible identity-constraint· gate is deliberately not applied to
// the entries: a binding is declared for every key and unique frame here,
// eligible or not. The property that gate decides — whether an
// [identity-constraint table] carries a binding at all — is unobservable to
// this package, and the only observable consequence of declaring one anyway is
// that clause 4.3 finds MORE entries to match, which costs a rejection and
// manufactures none. Its whole consumer set is icCheck.keyrefs, which charges
// on a key-sequence NOT found.
func (c *icCheck) evaluate(w *walk) {
	for _, f := range c.frames {
		if f.ic.Category() == xsd.IdentityConstraintKeyref {
			continue
		}
		b := c.table.declare(f.ic.Name())
		if f.declined {
			b.declined = true
			continue
		}
		q, ok := f.qualify(w, c.e)
		if !ok {
			b.declined = true
			continue
		}
		f.duplicates(w, c.e, q)
		if f.ic.Category() == xsd.IdentityConstraintKey {
			f.keyOnly(w, c.e, q)
		}
		for _, t := range q {
			b.entries = append(b.entries, icEntry{seq: t.sequence(), node: t.node})
		}
	}
}

// qualify is clause 4's ·qualified node set·: the members of the ·target node
// set· whose ·key-sequence· is as long as {fields}. It charges clause 3 on the
// way for a field that selected more than one node of a simple ·governing type
// definition· or a node clause 3 admits as none of its nodes (icSlot.charge).
//
// It reports ok=false where the frame must not settle any further clause: a
// slot this processor declined (icSlot.declined), and a clause 3 charge, after
// which the ·key-sequences· of the offending target are not the rule's. Both
// leave the frame's node-table binding declined, so a keyref referring to it
// declines too rather than charging for entries that were never assembled.
func (f *icFrame) qualify(w *walk, host Element) ([]*icTarget, bool) {
	var q []*icTarget
	ok := true
	for _, t := range f.targets {
		short := false
		for i := range t.slots {
			s := &t.slots[i]
			if s.declined {
				return nil, false
			}
			if s.charge != icClause3None {
				w.res.violations = append(w.res.violations, xsderr.New(ruleCvcIdentityConstraint, s.loc,
					s.charge.message(), t.e.Name(), f.ic.Name(), host.Name(), f.ic.Fields()[i].Expression()))
				ok = false
				continue
			}
			if !s.filled() {
				short = true
			}
		}
		if !short {
			q = append(q, t)
		}
	}
	if !ok {
		return nil, false
	}
	return q, true
}

// message is the format of the clause 3 charge c, over the ·target node·'s
// name, the constraint's, the host's and the field's expression.
func (c icClause3) message() string {
	if c == icClause3Other {
		return "element %s, a ·target node· of the identity constraint %s declared on %s, has its field %q select a node whose ·governing type definition· is neither a simple type definition nor a complex type definition with {variety} simple, which cvc-identity-constraint clause 3 admits as none of its nodes"
	}
	return "element %s, a ·target node· of the identity constraint %s declared on %s, has its field %q select more than one node whose ·governing type definition· is a simple type definition or a complex type definition with {variety} simple, ·nilled· nodes and nodes with an ·absent· [schema actual value] included, but cvc-identity-constraint clause 3 admits at most one"
}

// duplicates is clause 4.1 for a unique and clause 4.2.2 for a key, which are
// one test over two categories: no two members of the ·qualified node set· have
// ·key-sequences· whose members are pairwise equal or identical.
//
// The scan is over pairs in document order and charges the LATER member of each
// duplicated pair, which is the one a reader has to delete; an earlier member
// already charged is not charged again for a third occurrence, so n equal
// key-sequences charge n-1 times and not n(n-1)/2. A pair [walk.sameKeySequence]
// cannot decide charges nothing and is recorded as an [Unevaluated] against its
// later member, once per member however many pairs it is undecided in.
func (f *icFrame) duplicates(w *walk, host Element, q []*icTarget) {
	charged := make([]bool, len(q))
	undecided := make([]bool, len(q))
	seqs := make([]icKeySequence, len(q))
	for i, t := range q {
		seqs[i] = t.sequence()
	}
	for i := range q {
		for j := i + 1; j < len(q); j++ {
			if charged[j] {
				continue
			}
			same, decided := w.sameKeySequence(seqs[i], seqs[j])
			if !decided && !undecided[j] {
				undecided[j] = true
				w.decline("assessing identity constraint", q[j].e.Name(), q[j].e.Loc(), ruleCvcIdentityConstraint, strings.TrimPrefix(f.duplicateClause(), "clause "),
					"element %s, a ·target node· of the %s %s declared on %s, was not compared with the one at %s: a member pair of their ·key-sequences· could not be compared in one value space, so cvc-identity-constraint %s is undecided",
					q[j].e.Name(), f.ic.Category(), f.ic.Name(), host.Name(), q[i].e.Loc(), f.duplicateClause())
			}
			if !decided || !same {
				continue
			}
			charged[j] = true
			w.res.violations = append(w.res.violations, xsderr.New(ruleCvcIdentityConstraint, q[j].e.Loc(),
				"element %s, a ·target node· of the %s %s declared on %s, has a ·key-sequence· equal or identical to the one at %s, but cvc-identity-constraint %s forbids two members of the ·qualified node set· to share one",
				q[j].e.Name(), f.ic.Category(), f.ic.Name(), host.Name(), q[i].e.Loc(), f.duplicateClause()))
		}
	}
}

// duplicateClause names the clause the shared test is charged under for this
// frame's category: 4.1 for a unique, 4.2.2 for a key.
func (f *icFrame) duplicateClause() string {
	if f.ic.Category() == xsd.IdentityConstraintKey {
		return "clause 4.2.2"
	}
	return "clause 4.1"
}

// keyOnly charges the two clauses a key has and a unique does not.
//
// Clause 4.2.1 — the ·target node set· and the ·qualified node set· are equal —
// is what makes a MISSING field an error for a key where it is none for a
// unique, whose own Note says so outright: "a selected unique node may have
// fields that do not have corresponding [schema actual value]s". The charge
// carries the ·target node·'s own location, which is where the missing field
// was to have been.
//
// Clause 4.2.3 forbids an ELEMENT member of a ·key-sequence· that was assessed
// against a declaration whose {nillable} is true, whatever the element's
// content turned out to be. It is read off the declaration's own property, per
// the clause's Note.
func (f *icFrame) keyOnly(w *walk, host Element, q []*icTarget) {
	qualified := make(map[int]bool, len(q))
	for _, t := range q {
		qualified[t.node] = true
	}
	for _, t := range f.targets {
		if qualified[t.node] {
			continue
		}
		w.res.violations = append(w.res.violations, xsderr.New(ruleCvcIdentityConstraint, t.e.Loc(),
			"element %s, a ·target node· of the key %s declared on %s, has a ·key-sequence· shorter than its {fields}, so it is not a member of the ·qualified node set·, but cvc-identity-constraint clause 4.2.1 requires a key's two node sets to be equal",
			t.e.Name(), f.ic.Name(), host.Name()))
	}
	for _, t := range q {
		for i := range t.slots {
			m := t.slots[i].member
			if !m.element || !m.nillable {
				continue
			}
			w.res.violations = append(w.res.violations, xsderr.New(ruleCvcIdentityConstraint, t.slots[i].loc,
				"element %s, a ·target node· of the key %s declared on %s, has its field %q contribute an element member to its ·key-sequence· whose element declaration has {nillable} true, which cvc-identity-constraint clause 4.2.3 forbids for a key",
				t.e.Name(), f.ic.Name(), host.Name(), f.ic.Fields()[i].Expression()))
		}
	}
}

// keyrefs charges clause 4.3 for the keyref frames rooted at c's element:
// each member of the ·qualified node set· must find a ·key-sequence· equal or
// identical to its own in the node table associated with the {referenced key}
// in E's own [identity-constraint table].
//
// E's OWN table is the whole of the rule's reach, and the reason a keyref
// resolves only inside its own subtree: §3.11.5 assembles that table from its
// children's tables and from what E itself qualified, never from a sibling's or
// an ancestor's.
//
// Four shapes decline instead of charging: a frame whose paths did not compile
// (recorded where it was opened), a ·qualified node set· qualify could not
// settle (recorded at the slot that declined, or charged under clause 3), a
// binding some descendant declined for one of those same reasons, and a member
// lookup could not decide. The last two withhold this keyref's own clause 4.3
// and are recorded here, per keyref and per member. A referenced key with no
// binding AT ALL is not one of them — that is a key whose own element never
// occurred in the subtree, which is exactly the "there is a node table" half of
// clause 4.3 failing, and it is charged.
func (c *icCheck) keyrefs(w *walk) {
	for _, f := range c.frames {
		if f.ic.Category() != xsd.IdentityConstraintKeyref || f.declined {
			continue
		}
		q, ok := f.qualify(w, c.e)
		if !ok {
			continue
		}
		ref, _ := f.ic.ReferencedKeyName()
		b, found := c.table.binding(ref)
		if found && b.declined {
			w.decline("assessing identity constraint", c.e.Name(), c.e.Loc(), ruleCvcIdentityConstraint, "4.3",
				"element %s declares the keyref %s, which was not resolved: the node table of its {referenced key} %s was not assembled in full, so cvc-identity-constraint clause 4.3 is undecided for it",
				c.e.Name(), f.ic.Name(), ref)
			continue
		}
		for _, t := range q {
			if found {
				same, decided := b.lookup(w, t.sequence())
				if same {
					continue
				}
				if !decided {
					w.decline("assessing identity constraint", t.e.Name(), t.e.Loc(), ruleCvcIdentityConstraint, "4.3",
						"element %s, a ·target node· of the keyref %s declared on %s, was not resolved against the node table of its {referenced key} %s: a member pair could not be compared in one value space, or it matched only an entry the table's conflict resolution could not settle, so cvc-identity-constraint clause 4.3 is undecided for it",
						t.e.Name(), f.ic.Name(), c.e.Name(), ref)
					continue
				}
			}
			w.res.violations = append(w.res.violations, xsderr.New(ruleCvcIdentityConstraint, t.e.Loc(),
				"element %s, a ·target node· of the keyref %s declared on %s, has a ·key-sequence· matching no entry in the node table of its {referenced key} %s, which holds only the sub-tree rooted at %s, but cvc-identity-constraint clause 4.3 requires one to match",
				t.e.Name(), f.ic.Name(), c.e.Name(), ref, c.e.Name()))
		}
	}
}

// sameKeySequence is clause 4.1/4.2.2/4.3's "equal or identical, member for
// member". decided is false wherever any member pair could not be compared, so
// clause 4.3 — the one that charges on a NON-match — never charges off a
// comparison this processor could not make.
func (w *walk) sameKeySequence(a, b icKeySequence) (same, decided bool) {
	if len(a) != len(b) {
		return false, true
	}
	for i := range a {
		same, decided := w.sameKeyMember(a[i], b[i])
		if !decided {
			return false, false
		}
		if !same {
			return false, true
		}
	}
	return true, true
}

// sameKeyMember compares two ·key-sequence· members in the VALUE space, which
// is the whole of Datatypes §2.2's contribution to this rule: "1" and "01" are
// one xs:integer key, and a lexical comparison in its place would report them
// as two.
//
// Two members validated against the SAME [xsd.SimpleType] node, which a compiled
// schema shares one of per type (xsd/simpletype.go), are compared by asking one
// value about the other ([sameValue]), where that node implies one value space
// ([walk.oneValueSpace]). Every other pair is not: a backend's mapping for a
// derived type may represent values in a space of its own, so asking one of
// them would answer a decided NOT-same, which is the one outcome clause 4.3
// turns into a rejection. That pair is compared in the ·primitive· value space
// instead ([walk.sameAcrossTypes]), which reads a union-typed member, and each
// item of a list member whose {item type definition} is a union, by its
// ·validating type· (key-vtype clauses 1 and 2, [walk.basicType]).
//
// GAP(validate): a pair neither path can compare declines (decided=false)
// instead: a value with neither value.Eq nor value.Identical; and a member
// whose type chain or ·validating type· does not resolve, or whose re-reading
// in its ·primitive· value space fails ([walk.primitiveItems]). RULED permanent
// by #2115 (STYLE P3b), on #774's terms: the first is backend coverage, and
// every failure of the second is a fault of the type or of the backend on a
// lexical the member's own type accepted, never a verdict about the lexical,
// so a decided NOT-same there would be a clause 4.3 false reject. The readers
// of the answer, all through [walk.sameKeySequence], charge only on a decided
// one: [icFrame.duplicates] charges clause 4.1/4.2.2 on a decided same and
// records the decline instead; [resolveEntryConflicts] keeps both entries,
// marked contested; [icBinding.lookup] reports the member undecided, which
// [icCheck.keyrefs] records instead of charging clause 4.3. So a decline
// withholds a charge and manufactures none.
//
// A pair with a ·special· member is decided by [sameSpecialMember] alone and
// never reaches either path.
func (w *walk) sameKeyMember(a, b icKeyMember) (same, decided bool) {
	if a.st.IsSpecial() || b.st.IsSpecial() {
		return sameSpecialMember(a, b)
	}
	if a.st == b.st && w.oneValueSpace(a.st) {
		return sameValue(a.v, b.v)
	}
	return w.sameAcrossTypes(a, b)
}

// sameSpecialMember compares a pair of which at least one member's type is
// ·special· ([walk.keyMember]). Two ·special· members whose lexicals are
// byte-identical are SAME: xs:string is a member of the lexical mapping's union
// (Datatypes §3.2.1.2, §3.2.2.2) and maps one literal to one value identical to
// itself, so cvc-identity-constraint clause 4.1's "equal or identical" holds
// under one reading of the mapping, and under no reading is a literal unequal to
// itself save NaN under float, which "equal or identical" absorbs.
//
// GAP(validate): every other such pair declines (decided=false) — two ·special·
// members with differing lexicals, which are equal under xs:decimal and unequal
// under xs:string ("1" and "1.0"), and a ·special· member paired with an
// ordinary one. RULED permanent by #2124 (STYLE P3b): the oracle ruled the
// [schema actual value] of a ·special·-typed field node undecidable from the
// spec text, since Datatypes §3.2.1.2 and Structures §3.11.4 clause 3 name no
// one ·actual value· for it and equality across ·primitive· datatypes is always
// false (§2.2.1, §2.2.2). No NOT-same is ever answered here: the mapping-union
// fold that could reach one (value's specialMatches) answers a false one under
// a narrowed primitive [value.Override] (#2045), and needs each member's own
// namespace context besides.
func sameSpecialMember(a, b icKeyMember) (same, decided bool) {
	if a.st.IsSpecial() && b.st.IsSpecial() && a.lexical == b.lexical {
		return true, true
	}
	return false, false
}

// oneValueSpace reports that every value validated against st lies in one
// value space: neither st nor, for a list, its {item type definition} is a
// union. A union is not one, nor a list of one: each value, or each item, lies
// in the space of the member that validated it (key-vtype clauses 1 and 2), and
// two of those may be two members' spaces. false where the type chain does not
// resolve, which sends the pair to [walk.sameAcrossTypes] to decline.
func (w *walk) oneValueSpace(st *xsd.SimpleType) bool {
	variety, err := st.Variety(w.schema)
	if err != nil {
		return false
	}
	if _, isList := variety.(xsd.List); isList {
		item, err := st.Item(w.schema)
		if err != nil {
			return false
		}
		variety, err = item.Variety(w.schema)
		if err != nil {
			return false
		}
	}
	_, isUnion := variety.(xsd.Union)
	return !isUnion
}

// sameValue is the equal-or-identical union of Datatypes §2.2.2 — "all
// comparisons for 'sameness' prescribed by this specification test for either
// equality or identity, not for identity alone" — over the two capability
// interfaces package value publishes for exactly this question. decided is false
// where a has neither.
func sameValue(a, b value.Value) (same, decided bool) {
	id, hasIdentical := a.(value.Identical)
	if hasIdentical && id.Identical(b) {
		return true, true
	}
	eq, hasEq := a.(value.Eq)
	if hasEq && eq.Eq(b) {
		return true, true
	}
	return false, hasIdentical || hasEq
}

// sameAcrossTypes compares two members validated against different simple
// types, or against one whose values lie in more than one value space
// ([walk.oneValueSpace]), each read as the sequence of its atomic values
// in their ·primitive· value spaces ([walk.primitiveItems]):
//
//   - an atomic member is a sequence of one, per the paragraph of Structures
//     §3.11.4 after cvc-identity-constraint: "single atomic values are not
//     distinguished from lists with single items";
//   - sequences of different lengths are neither identical nor equal (Datatypes
//     §2.2.1, §2.2.2: two lists are the same only if they "have the same
//     length"), so a list of two items, or of none, matches no atomic member;
//   - items of different ·primitive· datatypes are "artificially distinct" and
//     "artificially unequal" (§2.2.1, §2.2.2), whatever they look like;
//   - items of one ·primitive· are compared by that primitive's own identity and
//     equality, whichever types derived from it they were validated against.
//
// Two sequences are the same where every item pair is identical or every item
// pair is equal — lists are equal "if and only if they have the same length and
// their items are pairwise equal" (§2.2.2), and identical likewise (§2.2.1) —
// which for two atomic members is [sameValue]'s union. decided is false where
// either member cannot be read, or an item has neither capability.
func (w *walk) sameAcrossTypes(a, b icKeyMember) (same, decided bool) {
	as, ok := w.primitiveItems(a)
	if !ok {
		return false, false
	}
	bs, ok := w.primitiveItems(b)
	if !ok {
		return false, false
	}
	if len(as) != len(bs) {
		return false, true
	}
	for i := range as {
		if as[i].primitive.Name() != bs[i].primitive.Name() {
			return false, true
		}
	}
	identical, equal := true, true
	for i := range as {
		id, hasIdentical := as[i].v.(value.Identical)
		eq, hasEq := as[i].v.(value.Eq)
		if !hasIdentical && !hasEq {
			return false, false
		}
		identical = identical && hasIdentical && id.Identical(bs[i].v)
		equal = equal && hasEq && eq.Eq(bs[i].v)
	}
	return identical || equal, true
}

// icPrimitiveItem is one atomic value of a ·key-sequence· member, read in the
// value space of its type's {primitive type definition}. Two primitives are
// compared by {name}, which is unique among the ·primitive· datatypes.
type icPrimitiveItem struct {
	primitive *xsd.SimpleType
	v         value.Value
}

// primitiveItems reads m as the sequence of its atomic values, each in the value
// space of its own ·validating type·'s ·primitive·: one item for an atomic
// member, one per list item for a list member. The ·validating type· of a
// union-typed member is the ·active basic member· that accepted its lexical
// (key-vtype clause 1), and each item of a list member is read the same way
// against the {item type definition} (clause 2), so one union yields items of
// as many primitives as its members have ([walk.basicType]).
//
// Each item is read off the lexical the validating type normalized, under that
// type's whiteSpace facet ([normalizedLexical]): an xs:token member written
// " a  b " is the value "a b", and re-reading the raw lexical under xs:string,
// whose whiteSpace is preserve, would read a different value. A list's
// whiteSpace is collapse (§4.3.6.1), so its items are its normalized lexical's
// tokens, which hold no white space for an item type's own normalization to
// change. The namespace context is m's own, the one [walk.keyMember] mapped
// its lexical under.
//
// ok is false where a resolution or the re-reading fails: on a lexical m's own
// type accepted, that is a fault of the type or of the backend and not a
// verdict about the lexical ([walk.sameKeyMember]'s GAP).
func (w *walk) primitiveItems(m icKeyMember) ([]icPrimitiveItem, bool) {
	st, ok := w.basicType(m.st, m.lexical, m.ctx)
	if !ok {
		return nil, false
	}
	variety, err := st.Variety(w.schema)
	if err != nil {
		return nil, false
	}
	switch variety.(type) {
	case xsd.Atomic:
		normalized, ok := normalizedLexical(w.schema, st, m.lexical)
		if !ok {
			return nil, false
		}
		item, ok := w.primitiveItem(st, normalized, m.ctx)
		return []icPrimitiveItem{item}, ok
	case xsd.List:
		itemType, err := st.Item(w.schema)
		if err != nil {
			return nil, false
		}
		tokens := strings.FieldsFunc(m.lexical, isXMLSpace)
		items := make([]icPrimitiveItem, 0, len(tokens))
		for _, token := range tokens {
			basic, ok := w.basicType(itemType, token, m.ctx)
			if !ok {
				return nil, false
			}
			item, ok := w.primitiveItem(basic, token, m.ctx)
			if !ok {
				return nil, false
			}
			items = append(items, item)
		}
		return items, true
	}
	return nil, false
}

// basicType is the ·validating type· of lexical against st (key-vtype clause
// 1, Structures §3.16.4): st itself unless st is a union, otherwise the ·active
// basic member· [walk.validatingType] identifies. A non-union st is returned
// without re-running its verdict, which [walk.keyMember] already ran. ok is
// false where st's chain does not resolve or the identification fails, which
// declines only the comparison that asked: keyMember's slot decision never
// runs through here, so a slot it decided stays decided.
func (w *walk) basicType(st *xsd.SimpleType, lexical string, ctx value.Context) (*xsd.SimpleType, bool) {
	variety, err := st.Variety(w.schema)
	if err != nil {
		return nil, false
	}
	if _, isUnion := variety.(xsd.Union); !isUnion {
		return st, true
	}
	return w.validatingType(st, lexical, ctx)
}

// primitiveItem reads one normalized atomic lexical, accepted by st, in the value
// space of st's {primitive type definition}. ok is false where that property is
// absent ([xsd.SimpleType.Primitive]'s nil), where the chain does not resolve,
// or where the primitive's mapping rejects the lexical, each a decline.
func (w *walk) primitiveItem(st *xsd.SimpleType, lexical string, ctx value.Context) (icPrimitiveItem, bool) {
	primitive, err := st.Primitive(w.schema)
	if err != nil || primitive == nil {
		return icPrimitiveItem{}, false
	}
	v, err := value.ValidateLexical(w.backend, w.schema, primitive, lexical, ctx, xpath.FacetAssertions(w.now))
	if err != nil {
		return icPrimitiveItem{}, false
	}
	return icPrimitiveItem{primitive: primitive, v: v}, true
}

// normalizedLexical is lexical under the whiteSpace facet in force on st
// (Datatypes §4.3.6), which an atomic type with a ·primitive· always carries
// (§3.16.7.4). ok is false where st carries no single recognized {value}.
//
// It reads the facet itself because package value's own resolution is
// unexported, and the pipeline's normalized lexical is not among
// value.ValidateLexical's results.
func normalizedLexical(r xsd.TypeResolver, st *xsd.SimpleType, lexical string) (string, bool) {
	facets, err := st.EffectiveFacets(r)
	if err != nil {
		return "", false
	}
	for _, ef := range facets {
		if ef.Facet().Kind() != xsd.FacetWhiteSpace {
			continue
		}
		values := ef.Facet().Values()
		if len(values) != 1 {
			return "", false
		}
		switch values[0] {
		case "preserve":
			return lexical, true
		case "replace":
			return strings.Map(replaceXMLSpace, lexical), true
		case "collapse":
			return collapseXMLWhitespace(lexical), true
		}
		return "", false
	}
	return "", false
}

// replaceXMLSpace is whiteSpace = replace on one rune (§4.3.6): each of
// xmlWhitespace's characters becomes a space.
func replaceXMLSpace(r rune) rune {
	if isXMLSpace(r) {
		return ' '
	}
	return r
}

// icTable is one element's [identity-constraint table] (§3.11.5): one binding
// per identity-constraint definition, in the order the definitions were first
// met, so several charges off one table arrive in a fixed order (STYLE D2). The
// definitions are keyed by their {name}, which is a schema-wide identity —
// xsd's own idcIndex is keyed by it, and it is what a keyref's {referenced key}
// names.
type icTable struct{ bindings []*icBinding }

// icBinding is one Identity-constraint Binding of §3.11.5: a {definition} and
// its ·node table·. declined marks a binding this processor could not assemble
// faithfully, so clause 4.3 declines against it rather than charging for
// entries that were never built.
type icBinding struct {
	def      xsd.QName
	entries  []icEntry
	declined bool
}

// icEntry is one (·key-sequence·, node) pair of a ·node table·. fromChild
// records which of §3.11.5's two clauses the entry owes its inclusion to, which
// is the only thing the conflict resolution reads: "potential conflicts are
// resolved by not including any conflicting entries which would have owed their
// inclusion to clause 1".
//
// contested marks an entry the proviso would have dropped had a comparison
// [resolveEntryConflicts] could not decide come out "same": it is kept, and a
// clause 4.3 match against it alone is undecided rather than satisfied
// ([icBinding.lookup]). It travels up with the entry, since the entry it was
// contested against may itself be dropped before the next level compares them.
type icEntry struct {
	seq       icKeySequence
	node      int
	fromChild bool
	contested bool
}

// declare returns the binding for def, adding it in first-seen order where the
// table has none. A key or unique frame declares its binding whether or not it
// found anything, so an ABSENT binding means the constraint's own element never
// occurred in the subtree — which is what clause 4.3 charges on.
func (t *icTable) declare(def xsd.QName) *icBinding {
	if b, found := t.binding(def); found {
		return b
	}
	b := &icBinding{def: def}
	t.bindings = append(t.bindings, b)
	return b
}

// binding reports the binding for def, or false where the table has none.
func (t *icTable) binding(def xsd.QName) (*icBinding, bool) {
	for _, b := range t.bindings {
		if b.def == def {
			return b, true
		}
	}
	return nil, false
}

// absorb merges one child's [identity-constraint table] into this one, which is
// §3.11.5's clause 1 and the whole of the upward propagation: every entry of
// every binding in a child's table is an entry of the parent's, marked as owing
// its inclusion to that clause. A declined binding stays declined all the way
// up, so a keyref anywhere above the decline declines with it.
func (t *icTable) absorb(child icTable) {
	for _, cb := range child.bindings {
		b := t.declare(cb.def)
		if cb.declined {
			b.declined = true
		}
		for _, e := range cb.entries {
			e.fromChild = true
			b.entries = append(b.entries, e)
		}
	}
}

// resolveConflicts applies §3.11.5's proviso to every binding: the table holds
// its entries "provided no two entries have the same key-sequence but distinct
// nodes".
func (t *icTable) resolveConflicts(w *walk) {
	for _, b := range t.bindings {
		b.entries = resolveEntryConflicts(w, b.entries)
	}
}

// resolveEntryConflicts drops the conflicting entries §3.11.5 names: those that
// "would have owed their inclusion to clause 1", i.e. the ones that arrived
// from a child. Where BOTH sides of a conflict arrived that way both go, which
// the spec spells out — "if all the conflicting entries arose under clause 1
// above, this means no entry at all will appear for the offending
// key-sequence".
//
// Two conflicting entries that both arose LOCALLY are a shape the proviso does
// not name, because clause 4.1/4.2.2 has already charged for it. The earlier one
// is kept, which is deterministic (the scan is in document order) and keeps the
// table larger.
//
// An undecided comparison is not a conflict either, and both departures keep
// MORE entries than the proviso would. The whole consumer set of a node table
// is icCheck.keyrefs, whose charge condition is a key-sequence NOT found among
// them, so an extra entry costs a rejection and can manufacture none. An entry
// an undecided comparison would have dropped is marked contested, so a keyref
// member matching it alone records clause 4.3 as undecided instead of passing.
func resolveEntryConflicts(w *walk, entries []icEntry) []icEntry {
	drop := make([]bool, len(entries))
	for i := range entries {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].node == entries[j].node {
				continue
			}
			same, decided := w.sameKeySequence(entries[i].seq, entries[j].seq)
			if decided && !same {
				continue
			}
			loseI := entries[i].fromChild
			loseJ := entries[j].fromChild || !entries[i].fromChild
			if !decided {
				entries[i].contested = entries[i].contested || loseI
				entries[j].contested = entries[j].contested || loseJ
				continue
			}
			drop[i] = drop[i] || loseI
			drop[j] = drop[j] || loseJ
		}
	}
	kept := make([]icEntry, 0, len(entries))
	for i, e := range entries {
		if drop[i] {
			continue
		}
		kept = append(kept, e)
	}
	return kept
}

// lookup is clause 4.3's search: is there an entry in this node table whose
// ·key-sequence· is equal or identical to seq, member for member? decided is
// false where no uncontested entry matched but some comparison could not be
// made or some contested entry matched, so the caller declines instead of
// charging or passing.
func (b *icBinding) lookup(w *walk, seq icKeySequence) (same, decided bool) {
	decided = true
	for _, e := range b.entries {
		match, ok := w.sameKeySequence(e.seq, seq)
		if match && !e.contested {
			return true, true
		}
		if match || !ok {
			decided = false
		}
	}
	return false, decided
}
