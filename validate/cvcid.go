package validate

import (
	"strings"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xpath"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// ruleCvcID is Validation Root Valid (ID/IDREF) (Structures §3.3.4.5, cvc-id).
// The clause charged goes in the message on ruleCvcElt's terms: the catalog
// carries the bare name, so "cvc-id.2" is not a valid [xsderr.Rule] — and the
// spec's own Note asks only that the two cases be told apart, "to ensure that
// distinct error codes are associated with these two cases".
const ruleCvcID xsderr.Rule = "cvc-id"

// This file assembles the [ID/IDREF table] of §3.17.5.2 as the walk goes and
// charges cvc-id (§3.3.4.5) against it once, at the ·validation root· and
// nowhere else. That restriction is the rule's own: cvc-elt clause 7 invokes it
// "if E is the validation root", so an interior element with a duplicate id
// below the root is the ROOT's violation and never its own.
//
// §3.17.5.2 builds one ID/IDREF binding per distinct string that is an ·ID
// value· or an ·IDREF value· in the ·actual value· of a member of the ·eligible
// item set·, whose [binding] is the set of elements that purport to HAVE that
// id. cvc-id then reads two shapes off the result: an empty [binding] is a
// reference to an id nothing declares (clause 1), and a [binding] with more
// than one member is an id two elements both claim (clause 2, c-uba).
//
// The [binding] is a set of ELEMENTS and not of items, which decides two things
// this file would otherwise get wrong. An ID-typed ATTRIBUTE binds its owner;
// an ID-typed ELEMENT binds its PARENT — §3.17.5.2 asks for an element "which
// has an attribute in its [[attributes]] or an element in its [[children]]"
// carrying the value — so a root element whose own content is an id has nothing
// to bind to and yields an entry with an empty [binding], which is the case the
// spec's own Note calls out as not useful. And two ID-typed items on ONE
// element contribute one member between them, not two, so clause 2 does not
// fire for them.

// valueRole is what the ·validating type· of a value makes it (§3.16.4,
// key-TYPE-value): an ·ID value· its element declares, an ·IDREF value·
// referring to one, an ·ENTITY value· String Valid clause 3 reads, or none of
// these. The [ID/IDREF table] and clause 3 read one classification between
// them, so there is one resolver of ·validating type·s and not two (STYLE T4).
type valueRole int

const (
	roleNone valueRole = iota
	roleIDDeclare
	roleIDReference
	roleEntity
)

// isID reports whether r is one of the two roles §3.17.5.2 builds the [ID/IDREF
// table] from.
func (r valueRole) isID() bool { return r == roleIDDeclare || r == roleIDReference }

// isEntity reports whether r is an ·ENTITY value·'s.
func (r valueRole) isEntity() bool { return r == roleEntity }

// roleValue is one ·ID value·, ·IDREF value· or ·ENTITY value· of an item's
// ·actual value·, carrying the role its OWN ·validating type· gives it. The
// role travels per value and not per item because a list whose {item type
// definition} is a union decides it per item (§3.16.4 key-vtype clause 2), so
// one attribute's value can hold an ·ID value· and a string that is neither.
type roleValue struct {
	value string
	role  valueRole
}

// idTable is the ·validation root·'s [ID/IDREF table]: one entry per distinct
// id string, in the order the strings were first met, so several charges arrive
// in a fixed order (STYLE D2). The index is internal and read by string alone,
// never iterated into output.
//
// declined records that some item of the subtree could have been in the
// ·eligible item set· and could not be read — see idTable.charge for which of
// the two clauses that suppresses, and why it suppresses only one. Each site
// that sets it records its own [Unevaluated] ([walk.declineID]), except the
// three whose decline is recorded elsewhere or cannot arise
// (idDefaultedAttributes' unresolved use, walk.child's undecided child, and
// walk.localGovernance's undecided ·override·, recorded by its own
// walk.decline).
type idTable struct {
	entries  []*idEntry
	index    map[string]*idEntry
	declined bool
}

// idEntry is one ID/IDREF binding: the [id] string, the [binding] as the
// ordinals of the elements that purport to have it, and the two locations the
// charges cite — where the string was first seen at all, for clause 1's
// unresolved reference, and where the SECOND element claimed it, for clause 2's
// duplicate.
type idEntry struct {
	id      string
	binding []int
	first   xsderr.Loc
	dup     xsderr.Loc
}

// declare records that the element with ordinal node purports to have id. A
// node of -1 is an ID-typed element with no parent, which contributes the
// string to the table with nothing bound to it (see the file comment).
func (t *idTable) declare(id string, node int, loc xsderr.Loc) {
	e := t.entry(id, loc)
	for _, n := range e.binding {
		if n == node {
			return // one element, one member: [binding] is a SET of elements
		}
	}
	if node < 0 {
		return
	}
	e.binding = append(e.binding, node)
	if len(e.binding) == 2 {
		e.dup = loc
	}
}

// reference records that some item's ·actual value· names id, which puts the
// string in the table whether or not anything declares it.
func (t *idTable) reference(id string, loc xsderr.Loc) {
	t.entry(id, loc)
}

// entry returns the table's entry for id, adding it in first-seen order.
func (t *idTable) entry(id string, loc xsderr.Loc) *idEntry {
	if e, found := t.index[id]; found {
		return e
	}
	if t.index == nil {
		t.index = make(map[string]*idEntry)
	}
	e := &idEntry{id: id, first: loc}
	t.index[id] = e
	t.entries = append(t.entries, e)
	return e
}

// charge settles cvc-id against the assembled table, at the ·validation root·
// alone. Entries are read in first-seen order, so the charges are ordered by
// where each id first appeared in the document.
//
// Clause 2 is charged whether or not something declined, and clause 1 is not.
// The asymmetry is the direction each clause fails in under a missing item: an
// item this package could not read can only ADD entries and members, so it can
// turn a one-member [binding] into a two-member one — clause 2 stays true once
// observed — while an unobserved DECLARATION is exactly what would turn a
// clause 1 charge into no violation at all. Charging clause 1 off an incomplete
// table would reject a document for a gap in the processor.
func (t *idTable) charge(w *walk, root Element) {
	if w.res.err != nil {
		// A walk that stopped on a source fault assembled a table over part of
		// a document, and the part it never reached is where the missing
		// declaration would be (walk.element).
		return
	}
	for _, e := range t.entries {
		if len(e.binding) > 1 {
			w.res.violations = append(w.res.violations, xsderr.New(ruleCvcID, e.dup,
				"the id %q is claimed by more than one element information item of the sub-tree rooted at the ·validation root· %s, so its ID/IDREF binding has more than one member, which cvc-id clause 2 forbids",
				e.id, root.Name()))
			continue
		}
		if len(e.binding) == 0 && !t.declined {
			w.res.violations = append(w.res.violations, xsderr.New(ruleCvcID, e.first,
				"the id %q is referred to but no element information item of the sub-tree rooted at the ·validation root· %s declares it, so its ID/IDREF binding is the empty set, which cvc-id clause 1 forbids",
				e.id, root.Name()))
		}
	}
}

// idAttributes reads the ·eligible item set·'s attribute half off one element:
// every attribute information item whose ·governing type definition· this
// package can name, which is walk.attributeType's job.
//
// An attribute that matches no attribute use and resolves to no top-level
// declaration is not a decline where the element's ·governing type definition·
// was determinable: it is then ·laxly assessed· against nothing, so it has no
// ·governing type definition· and §3.17.5.2 clause 3 excludes it from the
// ·eligible item set· outright.
func (w *walk) idAttributes(c *icCheck) {
	ct := c.g.complexType()
	attrs := c.e.Attributes()
	for _, a := range attrs {
		st, typed := w.attributeType(c.g, a)
		if typed {
			w.idRecord(st, a.Value(), c.e, elementContext{owner: c.e}, c.node, a.Loc())
		}
	}
	if ct != nil {
		w.idDefaultedAttributes(c, attrs, *ct)
	}
}

// idDefaultedAttributes reads the ·eligible item set·'s ·defaulted attribute·
// half off one element: §3.17.5.2's own Note is explicit that "the use of
// [schema actual value] ... means that default or fixed value constraints may
// play a part", and Attribute Default Value (§3.4.5.1, sic-attrDefault) is what
// puts the item in the PSVI with the ·effective value constraint·'s value.
// Which uses are ·defaulted attributes· is [walk.defaultedConstraint]'s to say
// (key-dflt-att), and the item's ids bind to the OWNER element, as a present
// attribute's do. The item is synthesized and has no source position, so it
// cites the owner's, as [walk.defaultedAttribute]'s clause 4 charge does. A
// {lexical form} outside the type's lexical space records nothing
// ([walk.idRecord]'s String Valid gate): that is cvc-complex-type clause 4's to
// charge, and [walk.defaultedAttribute] charges it.
//
// GAP(validate): a use whose {attribute declaration} does not resolve, or whose
// {type definition} is not a resolvable simple type, declines. The first is
// unreachable on a *xsd.Schema that exists and records nothing; the second is
// the absent-or-COMPLEX {type definition} [walk.declaredAttribute]'s doc records,
// and is recorded as an [Unevaluated] ([walk.declineID]). The decline withholds
// cvc-id clause 1 alone ([idTable.charge]). RULED permanent by #774 (STYLE P3b),
// on cvcattribute.go's terms.
func (w *walk) idDefaultedAttributes(c *icCheck, attrs []Attribute, ct xsd.ComplexType) {
	for _, u := range ct.AttributeUses() {
		vc, defaulted := w.defaultedConstraint(u, attrs)
		if !defaulted {
			continue
		}
		d, resolved := w.schema.ResolvedAttributeDeclaration(u)
		if !resolved {
			// Unreachable on a *xsd.Schema that exists, so it records no
			// [Unevaluated] no schema can produce.
			w.ids.declined = true
			continue
		}
		st, simple := w.schema.ResolvedSimpleType(d.TypeDefinition())
		if !simple {
			w.declineID(c.e, c.e.Loc(),
				"the ·defaulted attribute· %s of the element %s was not read into the ID/IDREF table: its declaration's {type definition} is absent or not a simple type definition, so cvc-id clause 1 is undecided",
				u.DeclarationName(), c.e.Name())
			continue
		}
		w.idRecord(st, vc.LexicalForm(), c.e, value.ConstraintContext(vc), c.node, c.e.Loc())
	}
}

// idElement reads the ·eligible item set·'s element half off one element, once
// its [[children]] are exhausted and its ·initial value· is complete. The
// declaring node is the element's PARENT, per §3.17.5.2's [binding], and -1
// where it has none.
//
// The ·initial value· read is [icCheck.assessed]'s, so an EMPTY element whose
// declaration carries a {value constraint} declares the id that constraint's
// {lexical form} carries: §3.17.5.2's own Note is explicit that "default or fixed
// value constraints may play a part", and cvc-elt clause 5.1 is what puts the
// substituted item in the ·eligible item set· (#853).
//
// GAP(validate): a declaration whose type was not determinable declines,
// recorded as an [Unevaluated] ([walk.declineID]). RULED permanent by #2174
// (STYLE P3b): the decline has no retirement route of its own and retires with
// its last producer. Those are (b1) [walk.conditionallySelected]'s
// key-cta-ta-select decline of a {test} the §3.12.6 evaluator cannot evaluate,
// which ta-props-correct clause 2 licenses, (b2) [walk.resolvedSelection]'s
// ·absent· (nil) {type definition} slot, which #2166 ruled permanent, and (b3)
// [walk.instanceOverride]'s [xsd.Schema.ValidlySubstitutable] error,
// unreachable for the reason xsd's validlyDerived states. Its consumers, and
// the direction each charges in: [idTable.charge] withholds its clause 1 arm
// (the decline sets declined), fail-open, and keeps charging clause 2 over what
// remains, which is fail-open too: [walk.idAttributes] records none of the
// element's attributes, [walk.attributeType] declining every one of them rather
// than resolving it by ·expanded name· (key-governing-ad clause 2 before clause
// 3), so a duplicate an attribute would have made is lost and none is added;
// [icCheck.fieldAttributes] declines a field over them on the same decline.
// walk.attributeType's own marker carries that decline. The element's
// [[children]] are [walk.child]'s undecided branch, whose marker names that
// shape's consumers. An element with NO ·governing element declaration· is not
// that shape. A ·laxly assessed· one, a declared element §5.3 falls back to
// ·lax assessment· among them ([governedByDeclaration]), is assessed against
// xs:anyType, whose complex {content type} is not derived from ID and so
// contributes nothing under clause 3, and its [[children]] are read in their
// own turn ([walk.child]). An undecided one ([governance]) is [walk.child]'s
// decline, not this one's.
//
// A ·nilled· element contributes nothing and withholds nothing: §3.3.5.4 gives
// it an absent [schema actual value], and clause 2 of the ·eligible item set·
// excludes every item whose [schema actual value] is ·absent·.
func (w *walk) idElement(c *icCheck) {
	if c.g.hasDecl && c.g.typ == nil {
		w.declineID(c.e, c.e.Loc(),
			"the element %s was not read into the ID/IDREF table: its ·governing type definition· was not determinable, so cvc-id clause 1 is undecided",
			c.e.Name())
		return
	}
	st := c.g.valueType()
	if st == nil || nilled(c.e, c.g) {
		return
	}
	node := -1
	if c.parent != nil {
		node = c.parent.node
	}
	lexical, ctx := c.assessed()
	w.idRecord(st, lexical, c.e, ctx, node, c.e.Loc())
}

// idRecord adds whatever one item contributes to the table. node is the element
// the item's ·ID values· bind to (its owner for an attribute, its parent for an
// element); it is unread for a reference.
//
// The value is gated on String Valid succeeding, which is what makes the
// [schema actual value] non-absent as clause 2 of the ·eligible item set·
// requires: a lexical outside the type's lexical space has no ·actual value· to
// be an ·ID value·, and its own invalidity is cvc-attribute's or
// cvc-complex-type's to charge.
//
// The item is admitted by its ·governing type definition· (idCandidate) before
// the lexical is read at all, and classified by its ·validating type·
// afterwards (roleValues): clause 3 of the ·eligible item set· is what decides
// whether the value is worth mapping, so a type outside the ID/IDREF closure
// runs no pipeline here.
//
// GAP(validate): a value.ValidateLexical error that is not a VERDICT
// (value.IsDatatypeVerdict) declines, on cvcattribute.go's terms — an ungoverned
// type reports under cvc-datatype-valid exactly as a genuine rejection does, and
// reading one as "no id here" would hide a declaration clause 1 charges for the
// absence of. validatingType's member scan declines on the same class for the
// same reason. RULED permanent by #774 (STYLE P3b), on cvcattribute.go's terms.
// The class also holds an assertions-facet decline (value.IsAssertionDeclined),
// which is not permanent: it is [walk.declineAssertions]' residue (#1042).
// Each decline is recorded as an [Unevaluated] at the item ([walk.declineID]).
func (w *walk) idRecord(st *xsd.SimpleType, lexical string, owner Element, ctx value.Context, node int, loc xsderr.Loc) {
	candidate, decided := w.idCandidate(st)
	if !decided {
		w.declineID(owner, loc,
			"an item of the element %s was not read into the ID/IDREF table: whether its type %s is ·derived· or ·constructed· from ID, IDREF or IDREFS could not be decided, so cvc-id clause 1 is undecided",
			owner.Name(), typeName(st))
		return
	}
	if !candidate {
		return
	}
	if _, err := value.ValidateLexical(w.backend, w.schema, st, lexical, ctx, xpath.FacetAssertions(w.now)); err != nil {
		if !value.IsDatatypeVerdict(err) {
			w.declineID(owner, loc,
				"an item of the element %s was not read into the ID/IDREF table: the value backend reported a fault of its type %s rather than a verdict about the lexical, so cvc-id clause 1 is undecided",
				owner.Name(), typeName(st))
		}
		return
	}
	values, decided := w.roleValues(st, lexical, ctx)
	if !decided {
		w.declineID(owner, loc,
			"an item of the element %s was not read into the ID/IDREF table: the ·validating type· of its value under %s could not be decided, so cvc-id clause 1 is undecided",
			owner.Name(), typeName(st))
		return
	}
	for _, v := range values {
		switch v.role {
		case roleIDDeclare:
			w.ids.declare(v.value, node, loc)
		case roleIDReference:
			w.ids.reference(v.value, loc)
		case roleEntity, roleNone:
			// An ·ENTITY value· is String Valid clause 3's and no member of the
			// [ID/IDREF table]; roleValues yields no roleNone at all.
		}
	}
}

// declineID marks the [ID/IDREF table] as missing an item that could have been
// in the ·eligible item set· and records the withheld check on [walk.decline]'s
// terms, at loc — the item's own location — under cvc-id clause 1, the one
// clause the decline suppresses ([idTable.charge]).
func (w *walk) declineID(owner Element, loc xsderr.Loc, format string, args ...any) {
	w.ids.declined = true
	w.decline("assessing ID/IDREF table", owner.Name(), loc, ruleCvcID, "1", format, args...)
}

// valueTokens splits one item's ·actual value· into the strings a ·validating
// type· is read against: every item of a list value, or the single value of an
// atomic one. WHICH of them are ·ID values·, ·IDREF values· or ·ENTITY values·
// is roleValues's to say.
//
// The values are read off the COLLAPSED lexical rather than out of the parsed
// value, which is exact for the five types namedRole names and for every type
// derived from them: ID, IDREF, ENTITY and the item types of IDREFS and
// ENTITIES all derive from xs:NCName, whose ·value space· is its ·lexical
// space· (Datatypes §3.4.7) under the collapse whiteSpace its ancestor xs:token
// fixes. Every caller has already run String Valid clauses 1 and 2 over the
// lexical, so a value reaching here is one that mapping accepted.
//
// Items are delimited on XML white space alone (isXMLSpace): a list value is
// split on #x20 (cvc-datatype-valid, Datatypes §4.1.4 clause 2.2) after
// whiteSpace = collapse has mapped #x9, #xA and #xD to it (§4.3.6).
// strings.Fields would also break on U+1680, an NCName character, splitting one
// valid ID or IDREF in two.
func valueTokens(lexical string, list bool) []string {
	fields := strings.FieldsFunc(lexical, isXMLSpace)
	if list || len(fields) < 2 {
		return fields
	}
	return fields[:1]
}

// idCandidate settles clause 3 of the ·eligible item set· over one ·governing
// type definition·: the built-in ID, IDREF or IDREFS, "or a type definition
// ·derived· or ·constructed· directly or indirectly from any of these". The
// derivation walk is namedRole's, up {base type definition}s; the construction
// ones are into a list's {item type definition} and into a union's {member type
// definitions} — ·constructed· is the Datatypes term covering all three of
// restriction, list and union (dt-constructed), so a union holding an ID member
// is admitted here exactly as a list of ID is.
//
// It is a CANDIDACY test and never the classification: whether a value in that
// type's closure really is an ·ID value· is its ·validating type·'s to say
// (roleValues). A union of ID and string admits every item it governs and
// contributes a binding for only those the ID member validated.
func (w *walk) idCandidate(st *xsd.SimpleType) (candidate, decided bool) {
	return w.candidate(st, valueRole.isID)
}

// candidate is idCandidate's walk over any family of roles: whether st is, or
// is ·derived· or ·constructed· directly or indirectly from, a type namedRole
// gives a role in family. String Valid clause 3 asks it of ENTITY and ENTITIES
// (cvcsimpletype.go), on the same terms.
//
// It recurses into a list's {item type definition} and into each union's
// {member type definitions} with no visited set (STYLE D4). [New] takes only a
// finalized [xsd.Schema], and finalization has proved both walks finite: Phase
// B's checkUnionMembershipAcyclic charges cos-st-restricts clause 3.3 on a
// circular membership, and cos-st-restricts clause 2.1 rejects an item type
// that is a list or has a list in its transitive membership, so the list arm
// is entered at most once on any path.
func (w *walk) candidate(st *xsd.SimpleType, family func(valueRole) bool) (candidate, decided bool) {
	if st == nil {
		return false, true
	}
	role, _, decided := w.namedRole(st)
	if !decided {
		return false, false
	}
	if family(role) {
		return true, true
	}
	variety, err := st.Variety(w.schema)
	if err != nil {
		return false, false
	}
	switch variety.(type) {
	case xsd.List:
		item, err := st.Item(w.schema)
		if err != nil || item == nil {
			return false, false
		}
		return w.candidate(item, family)
	case xsd.Union:
		members, err := st.Members(w.schema)
		if err != nil {
			return false, false
		}
		for _, m := range members {
			candidate, decided := w.candidate(m, family)
			if !decided {
				return false, false
			}
			if candidate {
				return true, true
			}
		}
	}
	return false, true
}

// namedRole reads the ·derived· half of key-TYPE-value off st's {base type
// definition} chain: ID, IDREF and IDREFS by name for §3.17.5.2 clause 3,
// ENTITY and ENTITIES by name for String Valid clause 3, and every type
// reaching one of them by restriction.
//
// list distinguishes the two readings of the ·actual value·: IDREFS holds one
// ·IDREF value· per list item and ENTITIES one ·ENTITY value·, an atomic type
// exactly one. It answers for the chain alone and walks into no {item type
// definition} or {member type definitions} — those are candidate's and
// roleValues's, which need them for different questions.
//
// The chain walk carries no cycle guard (STYLE D4): Phase B's
// checkSimpleBaseAcyclic has rejected every circular {base type definition}
// chain, a type naming itself included, before [New] can be handed the schema,
// and the walk stops at xs:anySimpleType, the one type with no base.
func (w *walk) namedRole(st *xsd.SimpleType) (role valueRole, list, decided bool) {
	for t := st; t != nil; {
		switch t.Name() {
		case idName:
			return roleIDDeclare, false, true
		case idrefName:
			return roleIDReference, false, true
		case idrefsName:
			return roleIDReference, true, true
		case entityName:
			return roleEntity, false, true
		case entitiesName:
			return roleEntity, true, true
		}
		if t.IsAnySimpleType() {
			break
		}
		base, err := t.Base(w.schema)
		if err != nil {
			return roleNone, false, false
		}
		t = base
	}
	return roleNone, false, true
}

// roleValues is every ·ID value·, ·IDREF value· and ·ENTITY value· in one
// item's ·actual value·, each classified by its own ·validating type· (§3.16.4,
// key-vtype) and not by the ·governing type definition· a candidacy test
// admitted the item on. §3.17.5.2's ·ID value· is "one whose ·validating type·
// is or is ·derived· from ID" (key-TYPE-value), so a union of ID and string
// contributes nothing for a value the string member validated, and an ·ENTITY
// value· is read on the same terms. The [ID/IDREF table] takes the ID roles off
// the result (idRecord) and String Valid clause 3 the ENTITY one
// (cvcsimpletype.go).
//
// The two arms are key-vtype's two clauses. Clause 1 fixes the type of the whole
// value, which for a union is the ·active basic member· (validatingType); where
// that type has a namedRole the value is read straight off the lexical. Clause
// 2 is the list case, "the validating type of an (atomic) item value A
// occurring in V", decided PER ITEM — so a list of a union of ID and string
// contributes a binding for the tokens the ID member validated and nothing for
// the rest, within one value.
//
// The caller has already run String Valid over the whole lexical, which is what
// makes each member scan below find a member at all: the same dispatch accepted
// it (§4.1.4 cl.2.3). A scan that finds none is a decline, on validatingType's
// terms.
func (w *walk) roleValues(st *xsd.SimpleType, lexical string, ctx value.Context) ([]roleValue, bool) {
	t, decided := w.validatingType(st, lexical, ctx)
	if !decided {
		return nil, false
	}
	role, list, decided := w.namedRole(t)
	if !decided {
		return nil, false
	}
	if role != roleNone {
		tokens := valueTokens(lexical, list)
		values := make([]roleValue, 0, len(tokens))
		for _, v := range tokens {
			values = append(values, roleValue{value: v, role: role})
		}
		return values, true
	}
	variety, err := t.Variety(w.schema)
	if err != nil {
		return nil, false
	}
	if _, listed := variety.(xsd.List); !listed {
		return nil, true
	}
	item, err := t.Item(w.schema)
	if err != nil || item == nil {
		return nil, false
	}
	return w.itemRoleValues(item, lexical, ctx)
}

// itemRoleValues is key-vtype clause 2 over one list value: each item of it gets
// its own ·validating type·, "the basic member of I's transitive membership
// which actually validated the substring in N that corresponds to A" where the
// {item type definition} I is a union, and an item whose type has no role
// simply contributes nothing beside the items that do.
//
// The item type is where the descent ends: cos-st-restricts clause 2.1
// (§3.16.6.2) admits an {item type definition} whose {variety} is atomic, or
// union with "no types whose {variety} is list among the union's transitive
// membership", so no item of a list is itself a list and there is no second
// split to make.
func (w *walk) itemRoleValues(item *xsd.SimpleType, lexical string, ctx value.Context) ([]roleValue, bool) {
	var values []roleValue
	for _, f := range valueTokens(lexical, true) {
		t, decided := w.validatingType(item, f, ctx)
		if !decided {
			return nil, false
		}
		role, _, decided := w.namedRole(t)
		if !decided {
			return nil, false
		}
		if role == roleNone {
			continue
		}
		values = append(values, roleValue{value: f, role: role})
	}
	return values, true
}

// validatingType is key-vtype clause 1: the ·validating type· of a value is the
// type itself unless that type is a union, in which case it is "the basic member
// of T's transitive membership which actually ·validated· N" — the FIRST member
// of {member type definitions} the lexical is Datatype Valid against, in
// declared order (dt-active-member), descended through a member that is itself a
// union until a non-union one is reached (dt-active-basic-member). It is a thin
// wrapper over value.ValidatingType, which runs this identification (union.go's
// activeBasicMember) alongside its own verdict rather than making this package
// keep a second implementation of the same member scan (#819).
//
// Any error declines rather than classifies — value.ValidatingType's doc
// comment enumerates why an error there is not always a verdict about lexical,
// but this caller does not need to tell the two apart: a member whose error is
// not a VERDICT (value.IsDatatypeVerdict) is a fault of the TYPE or of the
// backend, and reading it as "this member rejected" would hand the value to a
// LATER member — idRecord's own GAP, on the same terms, an assertions-facet
// decline among them; a membership no member accepts contradicts the String
// Valid the caller already ran, so it is a disagreement between two readings
// of one dispatch and not a fact about the document. Both decline identically
// here.
func (w *walk) validatingType(st *xsd.SimpleType, lexical string, ctx value.Context) (*xsd.SimpleType, bool) {
	t, _, err := value.ValidatingType(w.backend, w.schema, st, lexical, ctx, xpath.FacetAssertions(w.now))
	if err != nil {
		return nil, false
	}
	return t, true
}

// The three built-in types §3.17.5.2 clause 3 names by hand, and the two whose
// values String Valid clause 3 reads (key-TYPE-value).
var (
	idName       = xsd.QName{Space: xsd.XMLSchemaNS, Local: "ID"}
	idrefName    = xsd.QName{Space: xsd.XMLSchemaNS, Local: "IDREF"}
	idrefsName   = xsd.QName{Space: xsd.XMLSchemaNS, Local: "IDREFS"}
	entityName   = xsd.QName{Space: xsd.XMLSchemaNS, Local: "ENTITY"}
	entitiesName = xsd.QName{Space: xsd.XMLSchemaNS, Local: "ENTITIES"}
)

// attributeType is the ·governing type definition· of one attribute
// information item, narrowed to the simple type its ·actual value· is read
// under. It is the one reading both cvc-id and a `@NameTest` identity-constraint
// field need, and neither re-derives it (STYLE T4).
//
// The two arms are cvc-complex-type clause 2's own: an attribute matching an
// attribute use of the element's ·governing type definition· is governed by
// that use's {attribute declaration}, and one matching none is admitted (if at
// all) by an {attribute wildcard}, whose strict and lax {process contents} both
// ·resolve· the attribute's ·expanded name· among the top-level attribute
// declarations (§3.10.4.1). A name that resolves to nothing under either arm
// leaves the attribute with no governing type, which is reported false.
//
// A ***skip*** {attribute wildcard} reports false without resolving anything:
// §3.10.4.1's Note performs QName resolution only for an item ·attributed to· a
// strict or lax wildcard, so a ·skipped· item has NO ·governing· declaration,
// key-sva (§3.3.4.6) clause 2.2 leaves it unassessed, and §3.17.5.2
// clause 3 excludes it from the ·eligible item set·. Two such attributes
// sharing a lexical bind no ·ID value· between them, so cvc-id clause 2 charges
// no duplicate for them (#1043).
//
// ·skipped· is a property of the ·attribution·, not of the wildcard: §3.4.4.4
// attributes an item to the wildcard only where the item MATCHES it, and an
// item matching neither a use nor the wildcard is ·attributed· to nothing,
// hence not ·skipped·, hence resolved by name after all under key-governing-ad
// (§3.2.4.2) clause 3. The guard below therefore tests skippedAttribute, which
// asks cvc-wildcard for the ·attribution· itself, and declines only the
// attribute that is genuinely ·skipped·. It is the attribute-side counterpart
// of [walk.childGoverning] (assess.go), which switches on the
// [xsd.Attribution] the content model produced; there is no attribute-side
// Attribution to switch on, a complex type carrying at most one {attribute
// wildcard} (§3.4.1), so the match is asked at the item instead.
//
// The wildcard arm is otherwise taken WITHOUT checking that a wildcard is
// present, and that is deliberate rather than an omission: key-governing-ad
// clause 3 resolves by name for every attribute that is not ·skipped·, and an
// attribute matching neither a use nor a wildcard is ·attributed· to nothing
// (§3.4.4.4) and so is not. It covers the element whose DETERMINED ·governing
// type definition· is not complex at all, where g.complexType() is nil and there
// is no {attribute wildcard} to be skip; and it covers the ·laxly assessed·
// element, since the {attribute wildcard} of xs:anyType is ***lax*** (§3.4.7)
// and lax resolves. An attribute matching neither a use nor a wildcard also
// violates cvc-complex-type clause 2, which [walk.unmatchedAttribute] charges
// in its own right.
//
// The {attribute uses} read here are the spec's for an ANONYMOUS governing type
// as much as for a named one (§3.4.2.4 clause 3 is "the same for all complex
// type definitions", and xsd/ownedtypefold.go folds a declaration-owned type
// through the slot that owns it), so the use arm is asked of every complex
// governing type and the top-level fallback below is reached only where no use
// matches. Reaching it on an unread use would govern the attribute by whichever
// type the schema happens to ALSO declare at the top level, that declaration
// being a DIFFERENT component from the use's {attribute declaration}.
//
// GAP(validate): an element whose ·governing type definition· this package
// could not determine ([governance.typeUndetermined]) — the declined shape
// [walk.idElement]'s marker names and the undecided one [walk.child]'s names —
// reports false for every attribute, before either arm is asked. Its true type
// is unknown, and key-governing-ad (§3.2.4.2) clause 2's ·context-determined
// declaration·, from a use of that type, comes before clause 3's resolution by
// ·expanded name·, while a skip {attribute wildcard} of it would leave the
// attribute with no ·governing attribute declaration· at all (key-skipped), so
// the top-level declaration is not known to be the attribute's. The decline is
// fail-open against both readers: [walk.idAttributes] adds no ·ID value· or
// ·IDREF value· for the attribute, so [idTable.charge]'s cvc-id clause 2 can
// only lose a duplicate, and its clause 1 arm is withheld already, every
// producer of either shape setting w.ids.declined; [icCheck.fieldAttributes]
// declines the field slot ([icTarget.decline]), which withholds
// cvc-identity-constraint clauses 3 and 4 — 4.1 and 4.2.2 among them — for that
// constraint. RULED permanent by #2174 (STYLE P3b): the decline consumes the
// shapes that ruling keeps and retires with their last producer.
func (w *walk) attributeType(g governance, a Attribute) (*xsd.SimpleType, bool) {
	if g.typeUndetermined() {
		return nil, false
	}
	if ct := g.complexType(); ct != nil {
		if u, matched := attributeUseNamed(ct.AttributeUses(), a.Name()); matched {
			d, resolved := w.schema.ResolvedAttributeDeclaration(u)
			if !resolved {
				return nil, false
			}
			return w.schema.ResolvedSimpleType(d.TypeDefinition())
		}
		if w.skippedAttribute(g, a) {
			return nil, false
		}
	}
	d, found := w.topLevelAttribute(a)
	if !found {
		return nil, false
	}
	return w.schema.ResolvedSimpleType(d.TypeDefinition())
}

// skippedAttribute reports whether a is ·skipped· (§3.10.4.1, key-skipped): it
// matches no {attribute use} of the element's ·governing type definition· and is
// ·attributed to· (§3.4.4.4) a ***skip*** {attribute wildcard} of that type.
// Both halves are required — an attribute the wildcard does not ADMIT is
// ·attributed to· nothing and so is not ·skipped· — and cvc-wildcard
// (§3.10.4.1) decides the second ([xsd.Schema.AllowsAttributeWildcardName]).
//
// It is the ONE encoding of ·skipped· on the attribute side (STYLE T4). Its two
// readers ask it for different reasons and must not drift: [walk.attributeType]
// withholds a ·governing type definition· for such an item, and
// [icCheck.fieldAttributes] leaves a field slot UNFILLED rather than declined
// for it, which is §3.11.4 clause 3's Note — a field evaluating to a ·skipped·
// node leaves the ·key-sequence· short.
//
// An element whose ·governing type definition· is not complex — or is not
// determinable at all — skips nothing: there is no {attribute wildcard} to be
// ·attributed to·, and §3.4.7 makes the one on xs:anyType ***lax***.
func (w *walk) skippedAttribute(g governance, a Attribute) bool {
	ct := g.complexType()
	if ct == nil {
		return false
	}
	if _, matched := attributeUseNamed(ct.AttributeUses(), a.Name()); matched {
		return false
	}
	wild, has := ct.AttributeWildcard()
	if !has || wild.ProcessContents() != xsd.ProcessSkip {
		return false
	}
	return w.schema.AllowsAttributeWildcardName(wild, a.Name())
}

// topLevelAttribute is the wildcard arm of [walk.attributeType] on its own: the
// top-level Attribute Declaration a's ·expanded name· ·resolves· to (§3.10.4.1,
// key-governing-ad clause 3), false where the name resolves to nothing.
//
// It is factored out because [walk.wildcardAttribute] (cvcattribute.go) asks the same
// question where cvc-attribute clauses 3 and 4 are CHARGED against that
// declaration, and the two must read one encoding of that resolution (STYLE
// T4): a second spelling of it there would let the declaration the charge is
// decided against drift from the one cvcid.go and cvcidentityconstraint.go
// decide the lexical against.
func (w *walk) topLevelAttribute(a Attribute) (xsd.AttributeDeclaration, bool) {
	return w.schema.Attribute(a.Name())
}
