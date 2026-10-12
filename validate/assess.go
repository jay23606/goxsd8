package validate

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// ruleCvcAssessElt is Schema-Validity Assessment (Element) (Structures
// §3.3.4.6, cvc-assess-elt). Its three clauses only dispatch — strictly
// assessed, not assessed, laxly assessed — so a ·validation root· that
// determines no declaration and no type definition violates no numbered
// clause of it. The charge stands instead on §5.2 strict wildcard
// validation, whose Note has the invoking process expecting the
// ·validation root· "to be declared and valid" and otherwise reporting "an
// error to its environment"; cvc-assess-elt is the rule that policy is
// stated against, and the catalog carries the bare name.
//
// Assessment Outcome (Element) (§3.3.5.1, e-validity) clause 1.1.3 is charged
// under this Rule too ([walk.unresolvedStrictWildcardChild]), and the clause
// goes in the message on ruleCvcElt's terms. e-validity is a PSVI contribution
// with no cvc-* ID of its own, and no Rule is MINTED for it: xsderr/doc.go's
// s4s-grammar section rules that a Rule invented for a verdict the spec
// declines to catalog "would be read as a citation of a rule the spec does not
// state". It folds here rather than into cvc-complex-content because the clause
// is a property of ·assessment· — it reads a child's [validity] and the
// ·attribution· key-sva (§3.3.4.6) clause 3 dispatched on, not the element
// sequence's validity against a {content type} (#717).
//
// That message names "e-validity clause 1.1.3" and never "cvc-assess-elt
// clause 1.1.3": ·strictly assessed· (key-sva) has a clause 1.1.3 of its own,
// a different condition entirely, so spelling the carried Rule beside the
// borrowed clause number would cite the wrong sentence. STYLE E4's grep is
// served by the rule that STATES the clause. The charge's debug line carries
// this Rule and no clause for the same reason ([walk.logDecision]).
const ruleCvcAssessElt xsderr.Rule = "cvc-assess-elt"

// ruleCvcElt is Element Locally Valid (Element) (Structures §3.3.4.3,
// cvc-elt). The clause charged goes in the message, not the rule ID: unlike
// some rules whose dotted sub-ID the spec itself anchors (cos-ct-extends.1.2,
// src-simple-type.1), cvc-elt's catalog entry is the bare name only, so
// "cvc-elt.2" is not a valid [xsderr.Rule] (see [xsderr.IsValidRule]).
const ruleCvcElt xsderr.Rule = "cvc-elt"

// ruleCvcComplexType is Element Locally Valid (Complex Type) (Structures
// §3.4.4.2, cvc-complex-type). Clauses 2 and 3 — the attribute half — and
// clause 5 over element [[children]] are charged here; the clause number goes
// in the message on ruleCvcElt's terms.
const ruleCvcComplexType xsderr.Rule = "cvc-complex-type"

// ruleCvcType is Element Locally Valid (Type) (Structures §3.3.4.4, cvc-type).
// Clause 2, a complex ·governing type definition· whose {abstract} is true
// ([walk.abstractType]), and clause 3.1's three sub-clauses — the arm taken
// where the ·governing type definition· is a Simple Type Definition — are
// charged under it; the clause number goes in the message on ruleCvcElt's
// terms, the catalog carrying the bare name.
const ruleCvcType xsderr.Rule = "cvc-type"

// Assess walks root's subtree once — the element, then its [[attributes]],
// then its [[children]] in document order, recursively — and reports what
// the walk found. The walk topology is cvc-assess-elt's (§3.3.4.6, ·strictly
// assessed·).
//
// It decides the ·governing element declaration· of the validation root,
// which is the declaration root's ·expanded name· ·resolves· to among the
// schema's top-level element declarations (§3.3.4.6, ·governing element
// declaration· clause 4). Where there is no such declaration and no xsi:type
// ·resolving· to a type definition, no ·governing type definition· can exist
// either (key-governing-type-elem clause 8) and cvc-assess-elt clause 1 cannot
// apply: cvc-assess-elt is charged and the subtree is not walked. Clause 3
// would have the root ·laxly assessed· against xs:anyType, which this package
// does for a descendant and not for the root; charging instead is §5.2 strict
// wildcard validation. An undeclared root whose xsi:type DOES resolve is
// ·strictly assessed· against that type alone (key-sva clause 1.2), with no
// declaration to read cvc-elt against.
//
// A declaration whose {abstract} is true is charged cvc-elt clause 2 at the
// root as at every descendant, and the walk still runs
// ([walk.abstractDeclaration]).
//
// cvc-elt clause 1 (D ·non-absent·, E and D sharing an ·expanded name·) is
// satisfied by construction — D is the declaration found BY that expanded
// name. It is charged only as §5.3 directs, "as if clause 1 ... had failed",
// for an ·absent· {substitution group affiliations} member of D
// ([walk.absentAffiliation]) or an ·absent· ·selected type definition·
// ([walk.resolvedSelection]), at the root as at every descendant, and the
// element is then ·laxly assessed·.
//
// Where the root's ·governing type definition· is determinable (see
// governingType), the root itself is additionally assessed against it: cvc-type
// (§3.3.4.4) clause 2 is charged where that type is complex and its {abstract}
// is true, without halting the walk ([walk.abstractType]); clause 3 dispatches
// on that type.
//
// Clause 3.2, for a COMPLEX type, assesses it in both directions
// cvc-complex-type (§3.4.4.2) quantifies in. Its [[attributes]] go to clauses
// 2, 3 and 4, and through clause 2.1 to the cvc-attribute (§3.2.4.1) and cvc-au
// (§3.5.4) charges the value space decides (see [walk.attributes] and
// cvcattribute.go). Its [[children]] go to clause 1, and through clause 1.4 to
// cvc-complex-content (§3.4.4.3) over [xsd.Matcher] (see cvccomplexcontent.go),
// and each element [[child]] to clause 5, its ·governing type definition·
// against its ·locally declared type· ([walk.locallyDeclaredType]).
//
// Clause 3.1, for a SIMPLE one, is charged in three sub-clauses of its own:
// 3.1.1, admitting no [[attributes]] but xsi:type, xsi:nil, xsi:schemaLocation
// and xsi:noNamespaceSchemaLocation; 3.1.2, admitting no element information
// item [[children]] at all; and 3.1.3, the ·initial value· ·valid· with respect
// to that type per String Valid (§3.16.4), for an element that is not ·nilled·.
// The first two are not nil-gated and the third is.
//
// A DESCENDANT is assessed the same way, against the ·governing element
// declaration· the particle its parent's content model ·attributes· it to
// supplies (key-sva (§3.3.4.6) clause 3.1 over §3.4.4.4,
// [walk.childGoverning]): its own [[attributes]], its own [[children]] and its
// own ·initial value· reach the same charges the root's do. Two shapes below
// the root are not: a child ·attributed to· a skip Wildcard or to an {open
// content} with a skip {wildcard}, which is ·skipped· along with every element
// beneath it (clause 3.2, clause 2), and a child whose governance this package
// could not decide, which is walked against no type along with its whole
// subtree ([governance], [walk.child]). A child its parent ·attributed· to
// nothing on any other ground, and a child whose name ·resolves· to no
// declaration, are neither: the package doc states how each is governed, the
// second being ·strictly assessed· (key-sva) clause 3.3's ·lax assessment·
// against xs:anyType, whose {content type} and {attribute uses} constrain
// nothing any of these charges reads (#1823).
//
// Nothing else is decided: the remaining cvc-elt clauses, cvc-type's own
// clause 1 (T ·non-absent·) and cvc-complex-type clause 5 over [[attributes]]
// (key-ldt-att) are not evaluated, and clause 6 only for the assertions xpath
// compiles (cvcassertion.go), so a [Result] carrying no violation says that
// the root is declared, and that every element is governed by no abstract
// declaration and — where its type was determinable — is governed by no
// abstract complex type, carries no attribute clause 2 or clause 3.1.1 rejects
// and no attribute whose value this backend could read and found invalid,
// lacks no required attribute clause 3 asks for, has no content reject its
// ·governing type definition· could settle and no child clause 5 rejects, and
// fails no assertion it evaluated. It says nothing else about the document.
//
// It panics if root is nil, on the same grounds as [ElementChild].
func (v *Validator) Assess(root Element) *Result {
	if root == nil {
		panic("validate: Assess: nil root Element")
	}
	entities, _ := root.(UnparsedEntities) // nil: the source supports no [unparsedEntities]
	processed, told := root.(DeclarationsProcessed)
	w := walk{log: v.log, schema: v.schema, backend: v.backend, values: value.NewValueSpace(v.backend), entities: entities,
		declsUnread: told && !processed.AllDeclarationsProcessed(), now: time.Now().UTC()}
	var g governance
	d, found := v.Schema().Element(root.Name())
	if found {
		g = w.declaredGovernance(root, d, nil)
	}
	if !found {
		typed, assessable := w.instanceGovernance(root)
		if !assessable {
			w.res.violations = append(w.res.violations, xsderr.New(ruleCvcAssessElt, root.Loc(),
				"the validation root %s has no top-level element declaration, and no xsi:type ·resolving· to a type definition either, so it determines neither a ·governing element declaration· nor a ·governing type definition· and cannot be ·strictly assessed· (§5.2 strict wildcard validation)",
				root.Name()))
			return &w.res
		}
		g = typed
	}
	w.element(root, g, nil, assertionAncestry{}, nil)
	w.ids.charge(&w, root)
	return &w.res
}

// governance is what cvc-assess-elt (§3.3.4.6) settles about one element
// information item before any rule reads it: the ·governing element
// declaration· the descent determined for it, and the ·governing type
// definition· in force — the declaration's ·selected type definition·, or the
// ·instance-specified· one an xsi:type ·overrode· it with, or an instance one
// with no declaration behind it at all. It travels as one value because every
// consumer past the first needs both — the type decides cvc-complex-type's two
// halves, and the DECLARATION carries the {identity-constraint definitions}
// §3.11.4 quantifies over and the {nillable} its clause 4.2.3 reads.
//
// The zero value is an element assessed against nothing: no declaration, no
// type. It is cvc-assess-elt clause 3's ·lax assessment· (key-lva) against
// xs:anyType (laxlyAssessed) — also the fall back §5.3 gives a declared element
// whose {substitution group affiliations} has an ·absent· member or whose
// ·selected type definition· is ·absent· ([walk.declaredGovernance]) — and
// hasDecl TRUE with a nil typ is this package declining a type it could not
// determine — a distinction cvcid.go needs, since only the second could have
// hidden an ID. hasDecl false with a NON-nil typ is the third shape, key-sva
// clause 1.2's: an element with no ·governing element declaration· ·strictly
// assessed· against a type alone — its ·locally declared type· or an xsi:type
// ·overriding· it (key-governing-type-elem clauses 7 and 6,
// [walk.localGovernance]), or else an xsi:type that ·resolved· (clause 8,
// [walk.instanceGovernance]). Every rule that reads the DECLARATION — cvc-elt,
// §3.11.4's {identity-constraint definitions} — is vacuous for it, and every
// rule that reads the type applies in full.
//
// undecided is the fourth shape, and it is none of cvc-assess-elt's outcomes:
// the package could not decide this element's governance at all, not even
// whether it is assessed. It is a child of a parent whose type this package
// could not determine or whose clause 1.4 [xsd.Schema.ContentMatcher] does not
// decide ([walk.child]), or an element whose xsi:type ·override· could not be
// settled ([walk.localGovernance]); a child its parent ·attributed· to nothing
// for any other reason is decided, as the package doc states. It carries no
// declaration and no type, like the zero value, and it is kept apart from the
// zero value because the two hand their [[children]] different things: a
// ·laxly assessed· element hands them xs:anyType's lax wildcard ([walk.child]),
// an undecided one hands them nothing (laxlyAssessed).
//
// instance is whether typ is E's ·instance-specified type definition· (§3.3.4.1,
// key-itd) — the xsi:type-driven cases of key-governing-type-elem, clauses 3, 6
// and 8 here. It is recorded rather than re-derived because governingType folds the
// decision away and nothing left in this struct can recover it: comparing typ
// against decl.TypeDefinition() answers a DIFFERENT question, since a {type
// table} may ·conditionally select· a type that differs from the declared one
// with no xsi:type present at all, and key-itd counts only the attribute. The
// one reader is cvc-elt clause 5.1.1, whose antecedent is exactly this bit
// (cvccomplexcontent.go).
type governance struct {
	decl      xsd.ElementDeclaration
	hasDecl   bool
	typ       xsd.TypeDefinition
	instance  bool
	undecided bool
}

// laxlyAssessed reports that the element is cvc-assess-elt (§3.3.4.6) clause
// 3's ·laxly assessed· one (key-lva): no ·governing element declaration·, no
// ·governing type definition·, and not undecided — the zero value and nothing
// else. Such an element is locally validated against xs:anyType (key-lva
// clause 1); what that hands its [[children]] is [walk.child]'s to say.
func (g governance) laxlyAssessed() bool {
	return !g.hasDecl && g.typ == nil && !g.undecided
}

// typeUndetermined reports that this package could not determine the
// element's ·governing type definition·: the declined shape (hasDecl with a nil
// typ) or the undecided one. It is false for every outcome cvc-assess-elt
// itself gives, a ·laxly assessed· element included, whose absent type is the
// spec's and not this package's. Its readers are [walk.attributeType] and
// [contentCheck.element], the second for a ·nilled· element's children.
func (g governance) typeUndetermined() bool {
	return g.undecided || g.hasDecl && g.typ == nil
}

// complexType narrows the ·governing type definition· to the Complex Type
// Definition cvc-type clause 3.2 dispatches to cvc-complex-type for, or nil for
// anything else — including a SIMPLE governing type, which clause 3.1 decides
// instead (simpleType).
func (g governance) complexType() *xsd.ComplexType {
	ct, isComplex := g.typ.(xsd.ComplexType)
	if !isComplex {
		return nil
	}
	return &ct
}

// simpleType narrows the ·governing type definition· to the Simple Type
// Definition cvc-type clause 3.1 applies to, or nil for anything else. It is
// exactly complexType's complement over a determined type: clause 3 is a
// dispatch on T ITSELF, so a complex type carrying a simple {content type} is
// clause 3.2's and never clause 3.1's, however simple the value it holds.
//
// That is what makes it a different question from valueType, which spans both
// shapes because it asks what an ·initial value· MAPS THROUGH rather than which
// arm of clause 3 is live.
func (g governance) simpleType() *xsd.SimpleType {
	st, isSimple := g.typ.(*xsd.SimpleType)
	if !isSimple {
		return nil
	}
	return st
}

// valueType narrows the ·governing type definition· to the simple type an
// element's ·initial value· is read under — the two shapes §3.11.4 clause 3
// admits as a field node, "a simple type definition or a complex type
// definition with {variety} simple" — and nil for every other, which carries no
// [schema actual value] to be a ·key-sequence· member or an ·ID value·.
func (g governance) valueType() *xsd.SimpleType {
	return valueTypeOf(g.typ)
}

// valueTypeOf is the simple type an element of type td has its value in: td
// itself where it is simple, the {simple type definition} of its {content
// type} where it is complex with {variety} simple, and nil for every other
// type, nil included. It is [governance.valueType]'s answer for a type that
// governs nothing yet, which is what a ·locally declared type· is
// ([walk.childValue]).
func valueTypeOf(td xsd.TypeDefinition) *xsd.SimpleType {
	switch t := td.(type) {
	case *xsd.SimpleType:
		return t
	case xsd.ComplexType:
		if sc, isSimple := t.ContentType().(xsd.SimpleContent); isSimple {
			return sc.SimpleType
		}
	}
	return nil
}

// declaredGovernance pairs a ·governing element declaration· with the
// ·governing type definition· it supplies for e (governingType), whose
// [inherited attributes] are inherited. Two §5.3 (Missing Sub-components)
// routes end the pairing in ·lax assessment· — the zero [governance], with no
// declaration, so no xsi:type, {nillable}, {abstract} or identity constraint of
// d is read for e — each with cvc-elt clause 1 already charged: an ·absent·
// member of d's {substitution group affiliations}, checked first and whatever
// d's {type definition} is ([walk.absentAffiliation]), and then an ·absent·
// ·selected type definition· ([walk.selectedType]). A ·selected type
// definition· this package could not determine leaves d with no type instead:
// the decline [governance] distinguishes.
func (w *walk) declaredGovernance(e Element, d xsd.ElementDeclaration, inherited []inheritedAttribute) governance {
	if w.absentAffiliation(e, d) {
		return governance{}
	}
	selected, ok := w.selectedType(e, d, inherited)
	if !ok {
		return governance{decl: d, hasDecl: true}
	}
	if selected == nil {
		return governance{}
	}
	t, instance := w.governingType(e, d, selected)
	return governance{decl: d, hasDecl: true, typ: t, instance: instance}
}

// absentAffiliation reports whether a member of d's {substitution group
// affiliations} names no element declaration in the schema, and charges it.
// Finalize keeps such a name as an ·absent· member (§5.3 Missing
// Sub-components, xsd's resolveElementDecl), and §5.3 makes validating e
// against a component "any of whose properties has or contains such an
// ·absent· value" proceed "as if clause 1 of Element Locally Valid (Element)
// had failed", falling back to ·lax assessment·: cvc-elt clause 1 is CHARGED at
// e's own location, naming the first such head in document order, whatever
// the present members beside it.
func (w *walk) absentAffiliation(e Element, d xsd.ElementDeclaration) bool {
	for _, head := range d.SubstitutionGroupAffiliationNames() {
		if _, found := w.schema.Element(head); found {
			continue
		}
		w.res.violations = append(w.res.violations, xsderr.New(ruleCvcElt, e.Loc(),
			"the ·governing element declaration· of the element %s has an ·absent· member in its {substitution group affiliations}: the substitution group head %s names no element declaration in the schema, a missing sub-component that Structures §5.3 (Missing Sub-components) treats as if cvc-elt clause 1 had failed, so the element is ·laxly assessed·",
			e.Name(), head))
		w.logDecision("assessing element", e.Name(), e.Loc(), ruleCvcElt, "1", "charged")
		return true
	}
	return false
}

// governingType is the ·governing type definition· (§3.3.4.6) of an
// element information item whose ·governing element declaration· is d and whose
// ·selected type definition·, determined and not ·absent·, is selected; it is
// nil wherever this package cannot determine that type, and the element's
// attributes are then assessed against nothing. The second result is whether
// the type returned is E's ·instance-specified type definition·
// (governance.instance), which only clause 3 below supplies and which no later
// comparison can recover.
//
// It walks key-governing-type-elem's cases in their own order, this package
// stipulating no type of its own: clause 3, an ·instance-specified type
// definition· that ·overrides· the ·selected type definition·, then clause 4,
// the ·selected type definition· itself. The selected one (§3.3.4.1) is
// selectedType's, both of its cases — a {type table} that ·conditionally
// selects· (clause 1, cta.go) and a bare D.{type definition} (clause 2) — and
// it is also the fallback the Note under cvc-elt names for BOTH ways an
// xsi:type can fail to supply clause 3's case: an attribute that does not
// ·resolve· at all, and one that resolves to a type that does not ·override·.
// The two differ only in the charge, not in the type: the first leaves E with no
// ·instance-specified type definition· and cvc-elt clause 4 vacuously satisfied,
// the second violates clause 4 and is charged here (instanceOverride).
//
// The ·selected type definition· is settled BEFORE any xsi:type is read, in
// both cases, by [walk.declaredGovernance]: ·overriding· is a relation TO the
// selected type, so an undetermined selected type decides nothing about it, and
// neither does an ·absent· one (§5.3), whose element is ·laxly assessed·
// without this being called — as is one whose declaration has an ·absent·
// {substitution group affiliations} member, before its selected type is read.
//
// Each decline below withholds a type that could differ from the declaration's,
// and assessing the element against the WRONG type is a false reject in both
// directions — an attribute the real governing type declares looks unmatched, a
// child its real {content type} admits looks unattributable. The exits that
// leave a declared element with no type are five, of three kinds — the first
// two selectedType's, before this is called, the third this function's — and
// each records one [Unevaluated] at the element:
//
//   - A {test} in a {type table} that the §3.12.6 required-subset evaluator
//     cannot evaluate (conditionallySelected, cta.go), under key-cta-ta-select.
//   - A nil {type definition} slot of the ·selected type definition·
//     (resolvedSelection, cta.go), under cvc-elt clause 1: the declaration's
//     own (key-selected-type clause 2), the one of the alternative that
//     ·successfully selects·, or the {default type definition}'s — three
//     exits, each reachable from a finalized Schema of directly built
//     components.
//   - An error from [xsd.Schema.ValidlySubstitutable] deciding the ·override·
//     (instanceOverride), under cvc-elt clause 4. It is unreachable.
//
// The type this returns is the governing one for EVERY reader, and they narrow
// it separately from here on: governance.complexType and governance.simpleType
// for cvc-type clause 3's two arms, and governance.valueType for the ·initial
// value· §3.11.4 clause 3 and §3.17.5.2 read. No reader narrows further on the
// type's {name}: an anonymous governing type is assessed exactly as a named
// one, for both halves (#1116).
func (w *walk) governingType(e Element, d xsd.ElementDeclaration, selected xsd.TypeDefinition) (xsd.TypeDefinition, bool) {
	instance, specified := w.instanceTypeDefinition(e)
	if !specified {
		return selected, false
	}
	return w.instanceOverride(e, d, instance, selected)
}

// instanceOverride settles cvc-elt clause 4 for an element carrying an
// ·instance-specified type definition·, and reports the ·governing type
// definition· that leaves: the instance's where it ·overrides· the selected
// type, the selected one where it does not. The second result says which of the
// two it returned, key-governing-type-elem clause 3 against clause 4, and it is
// the ONE place that answer exists (governance.instance).
//
// ·overrides· (§3.3.4.2, key-overrides) is ·validly substitutable· subject to
// the blocking keywords of E's ·governing element declaration· — its
// {disallowed substitutions}, and nothing else. The target type's own
// {prohibited substitutions} joins the set INSIDE key-val-sub-type, for the
// complex/complex case alone, which is [xsd.Schema.ValidlySubstitutable]'s to
// apply and deliberately not re-applied here.
//
// An error from that predicate is never a verdict about the pair, so it is
// neither charged nor folded into an answer: it leaves the override undecided,
// so no clause 4 charge is made and NO type is returned, which is
// governingType's own decline and carries its consequences unchanged. It is
// recorded as [Unevaluated] under cvc-elt clause 4, the clause left undecided,
// as [walk.locallyDeclaredType] and [walk.localGovernance] record the same
// error under the clause each decides. Returning the selected type on an
// undecided override would assess an element that may carry a derived type's
// content against the base, which is the reject governingType's own doc rules
// out; charging clause 4 would reject a document for a fault in the schema's
// own base chain.
//
// GAP(validate): an [xsd.Schema.ValidlySubstitutable] error declines the
// override, an exit unreachable for a finalized Schema, for the reason xsd's
// validlyDerived states, both operands being components w.schema holds; no
// test drives it and none can. RULED permanent by #2174 (STYLE P3b), in
// [walk.localGovernance]'s form: a decline and not a panic, the reason being
// xsd's invariant and not the spec's. It is producer (b3) of the
// undetermined type [walk.idElement]'s marker declines and of the undecided
// children below it, which [walk.child]'s marker covers, and those two
// markers name its consumers.
func (w *walk) instanceOverride(e Element, d xsd.ElementDeclaration, instance, selected xsd.TypeDefinition) (xsd.TypeDefinition, bool) {
	overrides, err := w.schema.ValidlySubstitutable(instance, selected, d.DisallowedSubstitutions())
	if err != nil {
		w.decline("assessing element", e.Name(), e.Loc(), ruleCvcElt, "4",
			"the ·governing type definition· of the element %s was not determined: whether its xsi:type %s ·overrides· its ·selected type definition· %s (cvc-elt clause 4, §3.3.4.2 key-overrides) could not be settled: %v",
			e.Name(), typeName(instance), typeName(selected), err)
		return nil, false
	}
	if overrides {
		return instance, true
	}
	w.res.violations = append(w.res.violations, xsderr.New(ruleCvcElt, e.Loc(),
		"the xsi:type attribute of the element %s names the type definition %s, which is not ·validly substitutable· for the ·selected type definition· %s of its ·governing element declaration· subject to that declaration's {disallowed substitutions}, so it does not ·override· it as cvc-elt clause 4 requires (§3.3.4.2, key-overrides)",
		e.Name(), typeName(instance), typeName(selected)))
	return selected, false
}

// typeName names a type definition for a message: its ·expanded name·, or a
// description of the slot for an anonymous one, which has no name to cite.
func typeName(t xsd.TypeDefinition) string {
	name := t.Name()
	if name == (xsd.QName{}) {
		return "an anonymous type definition"
	}
	return name.String()
}

// childGoverning is key-sva (§3.3.4.6) clause 3 for one child element
// information item, read off the ·attribution· its parent's content model just
// gave it (§3.4.4.4): the ·governing element declaration· and ·governing type
// definition· to assess it against, determined exactly as the ·validation
// root·'s are (declaredGovernance), and whether it is assessed at all.
//
// assess is false for a ·skipped· child: one ·attributed to· a skip Wildcard —
// the {term} of a ·wildcard particle· — or to an {open content} whose
// {wildcard} is skip is not ·assessed· (cvc-assess-elt clause 2), and neither is
// any element below it — ·skipped· (§3.10.4.1, key-skipped) holds for an item
// "attributed to a skip wildcard or if one of its ancestor elements is", so the
// subtree binds no ID or IDREF and contributes nothing to an identity
// constraint's ·target node set·. cvc-wildcard makes that a hard stop and not a
// permissive pass: a skip wildcard leaves the item with no ·governing element
// declaration· at all and runs no ·QName resolution· to look for one, so skip
// and ·strictly assessed· (key-sva) clause 3.3 are different outcomes and not
// two spellings of one.
//
// The {open content} half is a reading and not a quotation (#1969, reversing
// #1576): key-skipped, clause 3.2 of ·strictly assessed· and cvc-wildcard name
// a skip Wildcard and never an Open Content, and key-att-to ·attributes· the
// item to the {open content} record. The ·default binding· of
// cos-content-act-restrict (§3.4.6.4, key-dft-binding) clause 6 binds an item
// ·attributed· to "an Open Content with a skip Wildcard" to the keyword skip,
// beside a skip ·wildcard particle·'s, where a declaration key-governing-ed
// clause 4 resolved would have bound it to that declaration by clause 1
// instead. cos-element-consistent clause 2.2 likewise counts only an {open
// content} "with a strict or lax Wildcard", which is evidence of the exclusion
// and not a rule of assessment. The child's NAME is still checked, by
// cvc-complex-content clause 2.4 or 3.4, before this is reached.
//
// The declaration the type is read off is key-governing-ed's, one case per
// [xsd.Attribution] variant (STYLE T2's closed-sum exception):
//
//   - clause 2, the ·context-determined declaration·: an ElementDeclaration
//     attribution whose ·expanded name· is the child's own is the {term} of the
//     element particle the child was ·attributed to·.
//   - clause 2 through the ·substituting declaration·: an ElementDeclaration
//     attribution with ANOTHER name is the particle's own D, which cvc-accept
//     clause 2.3.2 admitted the child under as a member of D's ·substitution
//     group·. The ·context-determined declaration· is then that member and
//     never D ([xsd.Attribution]), and a member is top-level by construction
//     (§3.3.6.4, cos-equiv-class), so it is the resolution below.
//   - clause 3, for a strict or a lax Wildcard: the declaration the child's
//     ·expanded name· ·resolves· to among the schema's top-level element
//     declarations, which is the resolution [Validator.Assess] makes for the
//     root. The two {process contents} share it exactly — §3.10.4.1 draws no
//     distinction in the resolution step itself — and what they differ in is
//     what an UNRESOLVED name costs the ENCLOSING element's [validity]: under
//     strict, e-validity clause 1.1.3, charged by
//     [walk.unresolvedStrictWildcardChild]; under lax, nothing, at the child or
//     anywhere else.
//   - clause 4, "otherwise", for an [*xsd.OpenContent] whose {wildcard} is
//     strict or lax: the same ·resolution·. An item cvc-complex-content clause
//     2.4 or 3.4 admitted is ·attributed to· the {open content} record and to
//     no particle (§3.4.4.4, key-att-to), so clause 3 never names it, and
//     clause 4 carries it — to the declaration its ·expanded name· ·resolves·
//     to, and to none where it resolves to none, which is ·strictly assessed·
//     (key-sva) clause 3.3's ·lax assessment·. A skip {wildcard} makes the
//     item ·skipped· instead, which is clause 4.1 and the assess=false above.
//     Clause 4.3 withholds the declaration wherever the item's ·locally
//     declared type· within parent ([xsd.Schema.LocallyDeclaredElementType])
//     is non-·absent·, resolved name or not: the item then has no ·governing
//     element declaration·, and that type, or an xsi:type ·overriding· it,
//     governs instead ([walk.localGovernance]). What this arm does NOT share
//     with the Wildcard one is clause 4.3, which clause 3 does not carry, and
//     e-validity clause 1.1.3, which quantifies over ·wildcard particles· alone
//     ([walk.unresolvedStrictWildcardChild]).
//
// parent is the enclosing element's complex ·governing type definition·,
// ·nilled· or not (clause 4.3 reads the type, not cvc-complex-type clause 1's
// nil gate), and nil where it has none; only the {open content} arm and the
// nil arm read it. inherited is e's [inherited attributes], which a {type
// table} on the declaration reads (cta.go) and nothing else here does.
//
// A nil attribution is one [contentCheck.element] decided, governed as the
// package doc promises through [walk.localOrResolvedGovernance]. An undecided
// child never reaches here, and neither does a ·laxly assessed· parent's
// ([walk.child]).
func (w *walk) childGoverning(e Element, a xsd.Attribution, parent *xsd.ComplexType, inherited []inheritedAttribute) (governance, bool) {
	switch t := a.(type) {
	case xsd.ElementDeclaration:
		if t.Name() != e.Name() {
			return w.resolvedGovernance(e, inherited), true
		}
		return w.declaredGovernance(e, t, inherited), true
	case xsd.Wildcard:
		if t.ProcessContents() == xsd.ProcessSkip {
			return governance{}, false
		}
		return w.resolvedGovernance(e, inherited), true
	case *xsd.OpenContent:
		if t.Wildcard().ProcessContents() == xsd.ProcessSkip {
			return governance{}, false
		}
		return w.localOrResolvedGovernance(e, parent, inherited), true
	default:
		return w.localOrResolvedGovernance(e, parent, inherited), true
	}
}

// localOrResolvedGovernance is key-governing-ed clause 4 for an e no particle
// names: no ·governing element declaration· where e's ·locally declared type·
// within parent ([xsd.Schema.LocallyDeclaredElementType], key-ldt-elem) is
// non-·absent· (clause 4.3), that type, or an xsi:type ·overriding· it,
// governing instead (key-governing-type-elem clauses 7 and 6,
// [walk.localGovernance]); otherwise the declaration e's ·expanded name·
// ·resolves· to, or none, which is ·lax assessment· ([walk.resolvedGovernance]).
// A nil parent has no ·locally declared type· to offer.
func (w *walk) localOrResolvedGovernance(e Element, parent *xsd.ComplexType, inherited []inheritedAttribute) governance {
	if parent != nil {
		if ldt, local := w.schema.LocallyDeclaredElementType(*parent, e.Name()); local {
			return w.localGovernance(e, ldt)
		}
	}
	return w.resolvedGovernance(e, inherited)
}

// unresolvedStrictWildcardChild settles Assessment Outcome (Element) (§3.3.5.1,
// e-validity) clause 1.1.3 for one child of the element content is checking: a
// [[children]] member ·attributed to· a ***strict*** ·wildcard particle· whose
// own [validity] is ***notKnown*** takes the ENCLOSING element off clause 1.1's
// arm, and clause 1.2 makes its [validity] invalid.
//
// Two causes leave such a child ·laxly assessed·, and the message names the one
// that holds. An unresolved name under a strict wildcard has neither a
// ·governing element declaration· nor a ·governing type definition·, which is
// cvc-assess-elt clause 3's ·lax assessment· (key-lva) against xs:anyType. A
// name that ·resolves· to a declaration with an ·absent· {substitution group
// affiliations} member or ·selected type definition· falls back to the same
// ·lax assessment· by §5.3 ([walk.declaredGovernance]), its cvc-elt clause 1
// charge already made at the child. The schema lookup below tells the two
// apart, since the zero [governance] both leave carries no trace of the
// declaration.
//
// The child itself is charged NOTHING here and is not halted: [walk.element]
// still runs over it, so its [[attributes]] and its [[children]] are assessed
// in their turn — unlike the ·skipped· child key-sva clause 3.2 stops at. Its
// [validity] is notKnown by e-validity clause 2, "otherwise", an item not
// ·strictly assessed· having no clause 1 to reach.
//
// The [xsd.Wildcard] assertion below is the whole of "·attributed to· a
// ·wildcard particle·" and is exact by itself: an item cvc-complex-content
// clause 2.4 or 3.4 admitted is ·attributed to· the {open content} (§3.4.4.4),
// which is no particle and which clause 1.1.3 does not name, and
// [xsd.Attribution] spells that with its own variant. Such a child charges
// nothing here however strict the {open content}'s {wildcard} is —
// key-governing-ed clause 4 leaves its unresolved name simply without a
// ·governing element declaration· — and a type carrying BOTH kinds of strict
// wildcard charges for its particle-attributed children and withholds for its
// open-attributed ones, the two being told apart by the ·attribution· and never
// by the type's shape.
//
// The guard after it (governance.laxlyAssessed) rules out [governance]'s other
// shapes alongside the genuine unresolved-name one: hasDecl true is this
// package declining a type it could not determine, not an unresolved-name story
// at all, and typ non-nil with hasDecl false is key-sva clause 1.2's xsi:type-
// driven ·strict assessment· (key-governing-type-elem clause 8) — a ·governing
// type definition· WAS determined there, from xsi:type rather than from
// ·resolution·, so neither ·strictly assessed· (key-sva) clause 3.3's lax path
// nor this clause is live. The undecided shape never arrives beside a Wildcard.
// Only the zero value — no declaration, no type at all — is charged, and it is
// both causes above: an unresolved name, and a resolved one §5.3 sent to ·lax
// assessment·.
//
// notKnown is read off the governance the descent just determined, and no
// subtree state is kept: clause 1.1.3 quantifies over E.[[children]] and
// E.[[attributes]], one generation and no further. A deeper descendant reaches
// an ancestor through clause 1.1.2 instead — the intervening parent is invalid
// by THIS clause, and invalidity climbs by 1.1.2 — which in this package is the
// violation already on [Result].
//
// The ATTRIBUTE half of clause 1.1.3 charges nothing and withholds nothing: a
// ·wildcard particle· (§3.9.1, key-wp) is a Particle whose {term} is a
// Wildcard, §3.4.4.4 ·attributes· an attribute information item to the
// {attribute wildcard} itself and never to a particle, and no particle holds an
// attribute wildcard. The clause's "(element or attribute respectively)" has no
// attribute case that can hold.
//
// The enclosing element is ·strictly assessed· wherever this is reached, which
// clause 1 requires before 1.1.3 is live, and the condition is therefore not
// re-tested: an [xsd.Attribution] exists only where that element's ·governing
// type definition· was determined and an [xsd.Matcher] was built over it, and a
// determined ·governing type definition· is cvc-assess-elt clause 1's own
// antecedent.
//
// The charge carries the CHILD's location, which is where the unresolved name
// is; the element whose [validity] it decides is named in the message.
func (w *walk) unresolvedStrictWildcardChild(content *contentCheck, child Element, a xsd.Attribution, g governance) {
	wild, wildcard := a.(xsd.Wildcard)
	if !wildcard || wild.ProcessContents() != xsd.ProcessStrict {
		return
	}
	if !g.laxlyAssessed() {
		return
	}
	cause := "·resolves· to no top-level element declaration"
	if _, declared := w.schema.Element(child.Name()); declared {
		cause = "·resolves· to a top-level element declaration that has or contains an ·absent· component — an ·absent· ·selected type definition· or {substitution group affiliations} member (§5.3 Missing Sub-components, charged under cvc-elt clause 1)"
	}
	w.res.violations = append(w.res.violations, xsderr.New(ruleCvcAssessElt, child.Loc(),
		"the element information item %s is ·attributed to· a ***strict*** ·wildcard particle· but %s, so it is ·laxly assessed· and its [validity] is ***notKnown***, which e-validity clause 1.1.3 (§3.3.5.1) makes the enclosing element %s invalid for",
		child.Name(), cause, content.e.Name()))
	content.log(w, child.Name(), child.Loc(), ruleCvcAssessElt, "", "charged")
}

// locallyDeclaredType settles cvc-complex-type (§3.4.4.2) clause 5 for one
// element [[child]] of the element content is checking: where neither the
// child's ·governing type definition· nor its ·locally declared type· within the
// parent's is ·absent·, the first is the same as the second or ·validly
// substitutable· for it ·without limitation· (key-val-sub-type-absolute, the
// empty set of blocking keywords).
//
// The ·locally declared type· is [xsd.Schema.LocallyDeclaredElementType]'s
// (key-ldt-elem), over the parent's ·governing type definition· as clause 5's
// "E" reads it. A parent with no complex ·governing type definition· — a simple
// one, one this package could not determine, a ·laxly assessed· one, whose
// xs:anyType answers ·absent· by case 1 — and a child with no ·governing type
// definition· are both clause 5's ·absent· antecedent, and nothing is charged.
// The clause quantifies over every child, whatever it was ·attributed to·. One
// a particle's element declaration governs passes where its ·governing type
// definition· is that declaration's {type definition}, a {type table}
// selection other than ·xs:error· (e-props-correct clause 7.1), or an xsi:type
// ·overriding· the declared type: the parent's content model contains that
// declaration, and cos-element-consistent gives every same-named declaration
// it contains one type. It can fail where the {type table} selected S, neither
// ·xs:error· nor the declared type, and the xsi:type ·overrides· S:
// key-overrides blocks under S's {prohibited substitutions} and not under the
// declared type's. A selection of ·xs:error· (clause 7.2) is charged here too,
// beside cvc-type's charges against it. One ·attributed to· an {open content}
// with a non-·absent· ·locally declared type· passes by construction too:
// key-governing-ed clause 4.3 has that type, or an xsi:type ·overriding· it
// ·without limitation·, govern it ([walk.localGovernance]). It is a child
// ·attributed to· a strict or lax Wildcard, governed by the top-level
// declaration its name ·resolves· to or by its xsi:type alone
// (key-governing-type-elem clause 8), that can fail.
//
// "The same as" is sameType.
//
// GAP(validate): an error from [xsd.Schema.ValidlySubstitutable] is never a
// verdict about the pair, so it is recorded as undecided and charges nothing.
// That exit is unreachable for a finalized Schema, for the reason xsd's
// validlyDerived states. RULED permanent by #2174 (STYLE P3b), in
// [walk.localGovernance]'s form: a decline and not a panic, the reason being
// xsd's invariant and not the spec's. Its one consumer is this function's own
// cvc-complex-type clause 5 charge, which it loses, fail-open: the function
// returns nothing, and the child's governance is settled before it is called.
//
// The charge carries the CHILD's location, where the mismatched item is; the
// parent whose validity clause 5 decides is named in the message.
func (w *walk) locallyDeclaredType(content *contentCheck, child Element, g governance) {
	parent := content.g.complexType()
	if parent == nil || g.typ == nil {
		return
	}
	ldt, ok := w.schema.LocallyDeclaredElementType(*parent, child.Name())
	if !ok {
		return
	}
	if sameType(g.typ, ldt) {
		content.log(w, child.Name(), child.Loc(), ruleCvcComplexType, "5", "satisfied")
		return
	}
	substitutable, err := w.schema.ValidlySubstitutable(g.typ, ldt, nil)
	if err != nil {
		w.decline("assessing element", child.Name(), child.Loc(), ruleCvcComplexType, "5",
			"cvc-complex-type clause 5 was not decided for the element %s: whether its ·governing type definition· %s is ·validly substitutable· for its ·locally declared type· %s within %s could not be settled: %v",
			child.Name(), typeName(g.typ), typeName(ldt), content.e.Name(), err)
		return
	}
	if substitutable {
		content.log(w, child.Name(), child.Loc(), ruleCvcComplexType, "5", "satisfied")
		return
	}
	w.res.violations = append(w.res.violations, xsderr.New(ruleCvcComplexType, child.Loc(),
		"the element %s is a child of %s, and its ·governing type definition· %s is neither the same as nor ·validly substitutable· ·without limitation· for its ·locally declared type· %s within the parent's ·governing type definition·, as cvc-complex-type clause 5 requires (§3.4.4.2, key-ldt-elem)",
		child.Name(), content.e.Name(), typeName(g.typ), typeName(ldt)))
	content.log(w, child.Name(), child.Loc(), ruleCvcComplexType, "5", "charged")
}

// sameType repeats xsd's unexported sameDeclaredType across the package
// boundary, under a weaker operand precondition: a is a ·governing type
// definition·, which may be a {type table} selection, where sameDeclaredType
// is sound only for two declarations' {type definition}s. It is not unified
// with it, because the export would carry a precondition only one caller
// meets.
//
// It is clause 5's "the same as" for a child's ·governing type
// definition· a and its ·locally declared type· b. Named types are one
// component when their ·expanded names· are equal. Two anonymous COMPLEX types
// are one component when their {context}s name one component, which is the
// only identity an anonymous complex type has: [xsd.Schema.ValidlySubstitutable]
// answers false for a type against itself when both are anonymous complex
// (cos-ct-derived-ok's no-identity Note), so without this a child governed by
// the very declaration the ·locally declared type· is read off would be
// charged. An anonymous simple type needs no arm here: cos-st-derived-ok answers
// a type against itself true.
//
// A {type table} alternative's inline type shares its declaration's {context},
// so it is reported the same as the declared type it may differ from. That is
// no false accept: e-props-correct clause 7.1 already made such an
// alternative ·validly substitutable· for the declared type under a blocking
// set that contains clause 5's empty one, and the clause 7.2 exception,
// xs:error, is named and never reaches the {context} arm.
func sameType(a, b xsd.TypeDefinition) bool {
	if a.Name() != (xsd.QName{}) || b.Name() != (xsd.QName{}) {
		return a.Name() == b.Name()
	}
	ca, ok := a.(xsd.ComplexType)
	if !ok {
		return false
	}
	cb, ok := b.(xsd.ComplexType)
	if !ok {
		return false
	}
	xa, ok := ca.Context()
	if !ok {
		return false
	}
	xb, ok := cb.Context()
	if !ok {
		return false
	}
	return xa.ID() == xb.ID()
}

// resolvedGovernance is the ·governing element declaration· of an element that
// is the one its ·expanded name· ·resolves· to among the schema's top-level
// element declarations (cvc-resolve-instance, §3.17.6.3), together with the
// type that declaration supplies. A name that resolves to nothing leaves the
// element with no declaration to read a type off, which is cvc-assess-elt
// clause 3's ·lax assessment· (key-lva) and no charge of its own: an
// unresolved name is the enclosing element's business where it is anyone's
// (§3.3.5.1 clause 1.1.3, [walk.unresolvedStrictWildcardChild]) and never the
// child's — unless the element's own xsi:type supplies a ·governing type
// definition· (instanceGovernance), which makes it ·strictly assessed· by
// key-sva clause 1.2 rather than ·laxly assessed· by cvc-assess-elt clause 3.
func (w *walk) resolvedGovernance(e Element, inherited []inheritedAttribute) governance {
	d, found := w.schema.Element(e.Name())
	if !found {
		g, _ := w.instanceGovernance(e)
		return g
	}
	return w.declaredGovernance(e, d, inherited)
}

// instanceGovernance is the governance of an element with no ·governing element
// declaration· at all: key-governing-type-elem clause 8, where the
// ·instance-specified type definition· is the ·governing type definition· with
// no ·selected type definition· to ·override·. The second result is false where
// the element determines no type either, which leaves it assessed against
// nothing.
//
// cvc-elt clause 4 cannot be violated here and is not charged: ·overriding· is a
// relation to a ·selected type definition·, and an element with no declaration
// has none, so the clause has nothing to compare against. No {disallowed
// substitutions} are read either, and none are missing: key-overrides clause 2
// asks for ·validly substitutable without limitation· where no ·governing
// element declaration· is known, and key-val-sub-type-absolute defines that as
// the EMPTY set of blocking keywords — nothing blocked, which is the most
// permissive set and not the least.
func (w *walk) instanceGovernance(e Element) (governance, bool) {
	t, specified := w.instanceTypeDefinition(e)
	if !specified {
		return governance{}, false
	}
	return governance{typ: t, instance: true}, true
}

// localGovernance is the governance of an element with no ·governing element
// declaration· because a non-·absent· ·locally declared type· ldt exists for it
// (key-governing-ed clause 4.3): key-governing-type-elem clause 6, an
// ·instance-specified type definition· that ·overrides· ldt, and otherwise
// clause 7, ldt itself. With no ·governing element declaration· known,
// ·overriding· is ·validly substitutable without limitation· (key-overrides
// clause 2), and no cvc-elt clause is live to charge an xsi:type that does not
// override: the element is ·strictly assessed· against ldt by key-sva
// (§3.3.4.6) clause 1.2 either way, which leaves cvc-complex-type clause 5
// satisfied for it by construction.
//
// An error from [xsd.Schema.ValidlySubstitutable] leaves clause 6 undecided:
// it is recorded as [Unevaluated] under cvc-assess-elt, whose debug line names
// no clause because the clause undecided is key-governing-type-elem's and not
// cvc-assess-elt's ([walk.logDecision]), and the element takes [governance]'s
// undecided shape, walked against nothing with its subtree as [walk.child]
// walks an undecided child, with cvc-id's clause 1 arm withheld on the same
// grounds.
//
// GAP(validate): the exit is unreachable for a finalized Schema, for the reason
// xsd's validlyDerived states, and it is kept as a decline rather than a panic
// because that reason is xsd's invariant and not the spec's.
// RULED permanent by #2174 (STYLE P3b), as are the two other declines of this
// error, [walk.instanceOverride] and [walk.locallyDeclaredType], in the same
// form. The undecided shape withholds cvc-attribute clauses 3 and 5 from the
// element's xsi attributes, and key-governing-ad keeps them governed by their
// built-in declarations (§3.2.7) here. The withholding is fail-open against
// [walk.instanceNilLexical]: its clause 3 charge of an xsi:nil lexical outside
// xs:boolean is lost. [walk.instanceTypeResolves] loses nothing, the xsi:type
// having ·resolved· to reach this exit, which satisfies its clauses 3 and 5
// both. Its other consumers are the ones the marker on [walk.child]'s undecided
// branch names, this element being that marker's producer (c), and that marker
// states each one's direction: [idTable.charge]'s clause 2 among them is
// fail-open, [walk.attributeType] declining every attribute of an undecided
// element rather than reading it by ·expanded name·.
func (w *walk) localGovernance(e Element, ldt xsd.TypeDefinition) governance {
	instance, specified := w.instanceTypeDefinition(e)
	if !specified {
		return governance{typ: ldt}
	}
	overrides, err := w.schema.ValidlySubstitutable(instance, ldt, nil)
	if err != nil {
		w.decline("assessing element", e.Name(), e.Loc(), ruleCvcAssessElt, "",
			"the ·governing type definition· of the element %s was not determined: whether its xsi:type %s ·overrides· its ·locally declared type· %s (key-governing-type-elem clause 6) could not be settled: %v",
			e.Name(), typeName(instance), typeName(ldt), err)
		w.ids.declined = true
		return governance{undecided: true}
	}
	if overrides {
		return governance{typ: instance, instance: true}
	}
	return governance{typ: ldt}
}

// walk is the state of one [Validator.Assess] call, held here and not on the
// Validator so nothing survives the call that made it. schema and backend are
// the Validator's, reachable from the walk methods that need them — the one
// resolves component slots, the other reads instance lexicals — and neither is
// written here.
//
// values is [value.NewValueSpace] over that same backend, built once per
// assessment for the charge that asks a value-constraint question of the SCHEMA
// rather than of the instance: cvc-elt clause 5.1.1
// ([contentCheck.defaultValid]). It is not derived state to be re-derived per
// call: the constructor is total on a non-nil backend and the result is
// immutable, so building it on that arm would allocate once per element
// reaching it to reach the same object. nodes counts the element information
// items the walk has entered, and the count doubles as each one's IDENTITY:
// §3.11.5's conflict resolution turns on "the same key-sequence but distinct
// nodes" and §3.17.5.2's [binding] is a SET of elements, and an [Element] is an
// interface whose == compares whatever an adapter's dynamic type compares. ids
// is the [ID/IDREF table] those ordinals bind into, assembled across the whole
// walk and read once, at the ·validation root· (cvcid.go). entities is the root
// narrowed to [UnparsedEntities] once, at the top of the call, and nil where the
// source does not support [unparsedEntities] — the nil is that fact's only
// encoding, and String Valid clause 3 reads it (cvcsimpletype.go). declsUnread
// is the root's [DeclarationsProcessed] answer, read once at the same point and
// inverted: true only where the source implements the capability and reports
// false, which is all clause 3's diagnostic needs. now is the current dateTime
// of the XPath dynamic context (xpath20.md §2.1.2), read off the clock once, at
// the top of the call, which cvc-xpath clause 6 (§3.13.4.2) makes constant
// during an assessment episode: every assertion and every assertions facet the
// walk evaluates is handed this one instant, so fn:current-date answers one
// date across the whole episode.
type walk struct {
	log         *slog.Logger
	schema      *xsd.Schema
	backend     value.Backend
	values      xsd.ValueSpace
	entities    UnparsedEntities
	declsUnread bool
	nodes       int
	ids         idTable
	res         Result
	now         time.Time
}

// elementContext is the [value.Context] an instance lexical is mapped under:
// the namespace bindings in scope at the element that OWNS it, which is where a
// QName- or NOTATION-valued lexical resolves its prefix (Datatypes §3.3.18,
// §3.3.19, PRINCIPLES 19). The owner is the attribute's element for an
// attribute's lexical (cvc-attribute clause 3) and the element itself for its
// ·initial value· (cvc-complex-type clause 1.2). A {value constraint}'s {lexical
// form} — a ·defaulted attribute·'s or an element default's — is never mapped
// under it: [value.ConstraintContext] is that literal's context.
//
// It exists so no site passes a nil Context. A nil one makes a backend reject
// every prefixed QName lexical for want of bindings, which is a false reject of
// every xsi-free QName value in existence.
type elementContext struct{ owner Element }

func (c elementContext) LookupNamespace(prefix string) (string, bool) {
	return c.owner.LookupPrefix(prefix)
}

// element assesses one element information item: the item itself, then its
// [[attributes]], then its [[children]], then its {assertions}.
//
// cvc-elt clause 3 is settled first, before anything reads the [[children]],
// because whether e is ·nilled· decides WHICH rules read them: a ·nilled·
// element's [[children]] are cvc-elt clause 3.2.3.1's business and not
// cvc-complex-type clause 1's, which applies "if E is not ·nilled·"
// (cvccomplexcontent.go).
//
// g.typ is e's ·governing type definition·, and cvc-type clause 3 dispatches
// e's [[attributes]] and its [[children]] on it in one direction each: clause
// 3.1 for a Simple Type Definition, whose three sub-clauses admit the four
// instance attributes ([walk.attributes]), no element [[child]] at all, and only
// an ·initial value· String Valid accepts (cvccomplexcontent.go); clause 3.2 for
// a complex one, which sends both halves to cvc-complex-type (§3.4.4.2) instead.
//
// The type is not propagated to the children as it stands: each of them gets
// its OWN, off the particle e's {content type} ·attributes· it to (§3.4.4.4,
// [walk.childGoverning]), which is key-sva (§3.3.4.6) clause 3.1's "the one
// identified in the course of checking the local validity of the parent". A
// ·governing type definition· this package could not determine therefore ends
// the typed descent at e — neither arm of clause 3 is live, a check with no
// type attributes nothing — and e's subtree is walked against nothing
// throughout. A ·laxly assessed· e has no type either and does NOT end it
// ([walk.child]).
//
// cvc-complex-type clause 6 comes after both halves: [walk.elementAssertions]
// evaluates each of e's {assertions} the XPath evaluator compiled, charging
// cvc-assertion for one that does not hold, and declines the rest
// (cvcassertion.go). The {test}s are compiled before the [[children]] arrive
// ([walk.compileAssertions]), because everything a compile reads is static;
// the evaluation waits for the [[children]] because cvc-assertion
// clause 1.1 validates e "normally, except that assertions are not checked"
// before any {test} is evaluated, and clause 2.3 binds `$value` to what that
// leaves: e's [schema actual value], known only once its character
// [[children]] are exhausted, and the empty sequence where e is by then known
// to be invalid — a violation recorded since e was entered, which
// violationsBefore marks.
//
// parent is the enclosing element's identity-constraint state, nil at the
// ·validation root·. It is what carries the {selector} and {fields} evaluations
// downward and the node tables back up (cvcidentityconstraint.go), so the
// bracketing here is load-bearing: identityCheck opens e's state before its
// [[attributes]] are read, because a field path may name one of them, and
// identityExit settles §3.11.4 and §3.11.5 only once e's [[children]] are
// exhausted, because a ·key-sequence· and a node table are complete only then.
//
// [icCheck.substitute] sits between the two, carrying cvc-elt clause 5's case
// split across from the content check that decided it: the ·initial value· the
// two rules below read is the one clause 5.1 substituted where its arm is live,
// and only the check that consumed the [[children]] knows that it is.
//
// inherited is e's own [inherited attributes] (§3.3.5.6), nil at the
// ·validation root·, and it travels down beside parent. What e hands its own
// [[children]] is computed here, after g, because key-p-inherited clause 3
// reads the ·attribution· of e's attributes to e's ·governing type definition·
// ([walk.handedDown]).
//
// up is what e's ancestors' clause 6 states read of e ([assertionAncestry]),
// and it travels down beside parent as e's own travels to e's [[children]]
// (assertionAncestry.below). e is reported to every counting ancestor's
// Tallies on entry, its own attributes to its own ([walk.tallyElement]), and
// e hands its parent's state its typed value, and the typed values of its
// attributes a {test} of the parent reads through a range variable, last,
// once everything that can find e invalid has run ([walk.keepChild]), and
// every counting ancestor reading e's value through a path of child steps that
// value at the same point ([walk.tallyValue]) — a value of mixed content the
// string-value e's frame collected from its subtree as it streamed past,
// opened on entry ([walk.stringValue]): cvc-assertion clause 1.1 validates the
// parent's [[children]] "in the usual way" before any {test} of the parent
// reads them. e's own string value, which its own {test}s read as `.`, is
// collected the same way into a frame of its own ([ownStringValue]), and what
// e contributes to every open frame beyond its text runs is settled once its
// [[children]] are exhausted, before its own {test}s are evaluated
// ([collectEnd]).
func (w *walk) element(e Element, g governance, parent *icCheck, up assertionAncestry, inherited []inheritedAttribute) {
	if w.log.Enabled(context.Background(), slog.LevelDebug) {
		w.log.Debug("assessing element", slog.Any("name", e.Name()), slog.Any("loc", e.Loc()))
	}
	violationsBefore := len(w.res.violations)
	unevaluatedBefore := len(w.res.unevaluated)
	w.abstractDeclaration(e, g)
	w.abstractType(e, g)
	isNilled := w.nilCheck(e, g)
	id := w.identityCheck(e, g, parent)
	w.idAttributes(id)
	w.attributes(e, g)
	content := w.contentCheck(e, g, isNilled)
	asserts := w.compileAssertions(g)
	w.tallyElement(e, g, up, asserts)
	up.collectElement(e, g, isNilled)
	frame := w.stringValue(e, g, content, up)
	inner := up.collecting
	if frame != nil {
		inner = frame
	}
	own := ownStringValue(asserts, content, inner)
	if own != nil {
		inner = own
	}
	w.children(e, content, id, up.below(e.Name(), asserts, inner), w.handedDown(e, g, inherited))
	if w.res.err != nil {
		// A walk that stopped on a source fault never settles §3.11.4 or
		// §3.17.5.2 for this element, on [contentCheck.end]'s grounds: the
		// [[children]] the source never finished delivering are ·target nodes·,
		// field values and ID declarations the rules would otherwise be charged
		// for the absence of. Nor cvc-complex-type clause 6, whose partial
		// ·PSVI· (cvc-assertion clause 1.2) those [[children]] are part of.
		return
	}
	collectEnd(w.schema, e, inner, g, content)
	w.elementAssertions(e, asserts, content, own, len(w.res.violations) > violationsBefore)
	id.substitute(content)
	w.idElement(id)
	w.identityExit(id)
	recorded := len(w.res.violations) > violationsBefore || len(w.res.unevaluated) > unevaluatedBefore
	w.keepChild(up.parent, e, g, content, frame, recorded)
	w.tallyValue(e, g, content, up, recorded)
}

// abstractDeclaration settles cvc-elt (§3.3.4.3) clause 2 for e: its
// ·governing element declaration·'s {abstract} is false. §3.3.1 lets an
// abstract declaration appear in a content model only so substitution can
// replace it, and forbids it ever to ·validate· element content itself.
//
// g.decl is key-governing-ed's declaration whichever route settled it, so the
// clause is charged at every depth: the validation root's top-level declaration
// ([Validator.Assess]), a particle's ·context-determined declaration· used
// directly (cvc-accept clause 2.3.1), and the declaration a strict or lax
// wildcard's child ·resolves· to (key-governing-ed clause 3), or a child no
// particle names (clause 4). A member that substitutes for an abstract head is
// governed by the member and not by the head (cvc-accept clause 2.3.2,
// [walk.childGoverning]), so only an abstract member is charged. An element
// with no ·governing element declaration· — one a skip wildcard ·skipped·, one
// ·laxly assessed·, one governed by a type alone, or one undecided — has no
// {abstract} to read.
//
// [walk.element] is the one path every element takes, once, so the clause is
// charged once per element. The charge does not halt the walk: ·strictly
// assessed· clauses 2 and 3 assess [[attributes]] and [[children]] whatever
// clause 1.1.2's evaluation returned, so an abstract declaration does not
// silence its element's subtree.
func (w *walk) abstractDeclaration(e Element, g governance) {
	if !g.hasDecl || !g.decl.Abstract() {
		return
	}
	w.res.violations = append(w.res.violations, xsderr.New(ruleCvcElt, e.Loc(),
		"the element %s is governed by an element declaration whose {abstract} is true, but cvc-elt clause 2 requires it to be false: an abstract declaration validates no element information item",
		e.Name()))
}

// abstractType settles cvc-type (§3.3.4.4) clause 2 for e: where its
// ·governing type definition· is a Complex Type Definition, that type's
// {abstract} is false. g.typ is the type key-governing-type-elem settled,
// whichever clause settled it — the declaration's ·selected type definition·
// (clause 4), an xsi:type ·overriding· it (clause 3), a ·locally declared type·
// or an xsi:type ·overriding· that (clauses 7 and 6), or an xsi:type alone
// (clause 8) — so an xsi:type that failed cvc-elt clause 4 is never charged
// here: governingType fell back to the selected type, and only that type's
// {abstract} is read. A Simple Type Definition has no {abstract}, and an
// element with no ·governing type definition· — ·laxly assessed· against
// xs:anyType, which is not abstract, or one this package could not type — has
// none to read.
//
// [walk.element] is the one path every element takes, once, so the clause is
// charged once per element: the cvc-type evaluation cvc-elt clauses 5.1.2 and
// 5.2.1 call for is the content check's reading of clause 3 over the ·initial
// value· (cvccomplexcontent.go), which never calls back here. The charge does
// not halt the walk: ·strictly assessed· (key-sva) gates on cvc-type clause 1
// alone, so e's [[attributes]] and [[children]] are still assessed against the
// abstract type.
func (w *walk) abstractType(e Element, g governance) {
	ct := g.complexType()
	if ct == nil || !ct.Abstract() {
		return
	}
	w.res.violations = append(w.res.violations, xsderr.New(ruleCvcType, e.Loc(),
		"the element %s is governed by the complex type definition %s, whose {abstract} is true, but cvc-type clause 2 requires a complex ·governing type definition·'s {abstract} to be false",
		e.Name(), typeName(g.typ)))
}

// attributes assesses E.[[attributes]] against the arm of cvc-type (§3.3.4.4)
// clause 3 that e's ·governing type definition· selects.
//
// Clause 3.1.1, for a SIMPLE governing type, is an emptiness test modulo four
// names and the whole of what that arm asks of the attributes
// (simpleTypeAttributes).
//
// Clause 3.2, for a complex one, sends them to cvc-complex-type (§3.4.4.2), in
// the three directions it quantifies in: clause 2 over the attribute
// information items PRESENT, in source order, then clause 3 over the
// {attribute uses} that must be present, then clause 4 over the ·defaulted
// attributes·. Violations reach [Result] in that order, which is the order they
// were found in.
//
// A ·laxly assessed· element (key-lva) is locally validated against
// xs:anyType, whose {attribute wildcard} is lax over every namespace (§3.4.7),
// so each of its attributes goes to clause 2.2 against that wildcard
// ([walk.attribute]); it has no {attribute uses} for clauses 3 and 4 to read.
// A ·governing type definition· this package could not determine selects
// NEITHER arm and decides nothing at all: every attribute is walked (the log
// records the visit) and none is charged or passed. That is the only nothing
// left here — a complex type is read for its two attribute properties whether
// its {name} is ·absent· or not, because §3.4.2.4 clause 3 and §3.4.2.5 clause
// 2 are "the same for all complex type definitions" and both folds reach a
// declaration-owned anonymous type through the slot that owns it
// (xsd/ownedtypefold.go, #414).
//
// cvc-attribute (§3.2.4.1) against an xsi:type attribute — clauses 3 and 5 —
// sits OUTSIDE that dispatch, ahead of it: the item is governed by its built-in
// declaration (§3.2.7.1) and not by e's type, so neither arm of cvc-type clause
// 3 and neither arm of cvc-complex-type clause 2 reaches it, and the charge is
// the same under a simple governing type, a complex one, and none at all
// ([walk.instanceTypeResolves], cvcattribute.go). cvc-attribute clause 3
// against an xsi:nil attribute sits beside it, on the same grounds, wherever e
// has no ·governing element declaration· ([walk.instanceNilLexical]). Neither
// charges an undecided e ([governance]), whose governance this package could
// not decide and whose attributes the true schema may ·skip·.
func (w *walk) attributes(e Element, g governance) {
	w.instanceTypeResolves(e, g)
	w.instanceNilLexical(e, g)
	if st := g.simpleType(); st != nil {
		w.simpleTypeAttributes(e, st)
		return
	}
	ct := g.complexType()
	attrs := e.Attributes()
	for _, a := range attrs {
		w.attribute(a, e, g)
	}
	if ct == nil {
		return
	}
	w.requiredAttributeUses(e, attrs, *ct)
	w.defaultedAttributes(e, attrs, *ct)
}

// simpleTypeAttributes settles cvc-type clause 3.1.1: E.[[attributes]] is
// empty except for xsi:type, xsi:nil, xsi:schemaLocation and
// xsi:noNamespaceSchemaLocation. Every other attribute information item is
// charged, at its own location and whatever its ·expanded name· — a Simple Type
// Definition has no {attribute uses} to match and no {attribute wildcard} to be
// ·valid· with respect to, so the clause admits no arm cvc-complex-type clause
// 2's two would correspond to and none of its declines transfer.
//
// It is not gated on ·nilled·: clause 3.1.3 alone carries clause 3.1's "if E is
// not ·nilled·" condition, so a ·nilled· element's attributes are read here
// exactly as any other element's are.
//
// isInstanceAttribute is the one encoding of those four names (STYLE T4). The
// list cvc-complex-type clause 2 excepts by name and the list this clause
// excepts by name are the same four §3.2.7 Built-in Attribute Declarations, and
// §3.2.6.4 no-xsi forbids a schema to redeclare any of them.
func (w *walk) simpleTypeAttributes(e Element, st *xsd.SimpleType) {
	for _, a := range e.Attributes() {
		if isInstanceAttribute(a.Name()) {
			w.logAttribute(a, ruleCvcType, "3.1.1", "exempt")
			continue
		}
		w.res.violations = append(w.res.violations, xsderr.New(ruleCvcType, a.Loc(),
			"the element %s carries the attribute %s, but its ·governing type definition· %s is a Simple Type Definition, and cvc-type clause 3.1.1 admits no attribute on such an element beyond xsi:type, xsi:nil, xsi:schemaLocation and xsi:noNamespaceSchemaLocation",
			e.Name(), a.Name(), typeName(st)))
		w.logAttribute(a, ruleCvcType, "3.1.1", "charged")
	}
}

// attribute assesses one attribute information item against clause 2, whose
// two arms are the whole rule: an attribute matching an attribute use is
// judged by cvc-attribute and cvc-au (clause 2.1, matchedAttribute), one
// matching none needs an {attribute wildcard} that admits it (clause 2.2).
// There is no third arm, so an attribute that matches neither violates clause
// 2 outright.
//
// An element with no complex ·governing type definition· reaches clause 2 only
// where it is ·laxly assessed·: against xs:anyType, which has no {attribute
// uses} and whose {attribute wildcard} admits every ·expanded name· with
// {process contents} lax (§3.4.7), so every attribute but the four clause 2
// excepts is ·attributed to· that wildcard ([walk.wildcardAttribute]). One
// whose type this package could not determine — undecided included — is
// walked and decided nothing about.
//
// e is the attribute's owner, carried for the namespace bindings a QName- or
// NOTATION-valued lexical resolves against (elementContext).
func (w *walk) attribute(a Attribute, e Element, g governance) {
	governing := g.complexType()
	if governing == nil && !g.laxlyAssessed() {
		w.logAttribute(a, "", "", "")
		return
	}
	if isInstanceAttribute(a.Name()) {
		// Clause 2 excepts xsi:type, xsi:nil, xsi:schemaLocation and
		// xsi:noNamespaceSchemaLocation (§3.2.7) from its quantifier by name,
		// so no arm of it applies to one: it is neither charged nor passed.
		w.logAttribute(a, ruleCvcComplexType, "2", "exempt")
		return
	}
	if governing == nil {
		w.logAttribute(a, ruleCvcComplexType, "2.2", "satisfied")
		w.wildcardAttribute(a, e, xsd.ProcessLax)
		return
	}
	u, matched := attributeUseNamed(governing.AttributeUses(), a.Name())
	if !matched {
		w.unmatchedAttribute(a, e, *governing)
		return
	}
	w.matchedAttribute(a, e, u)
}

// unmatchedAttribute settles clause 2 for an attribute matching no attribute
// use, where only clause 2.2 is left to satisfy.
//
// This is the one charge in the package that a property REPORTED TOO SMALL
// could fabricate, and what keeps it from fabricating one is that both
// properties are the spec's for every governing type this package can reach:
// §3.4.2.4 clause 3 and §3.4.2.5 clause 2 fold a declaration-owned ANONYMOUS
// type through the slot that owns it as well as a named one
// (xsd/ownedtypefold.go, #414). That fold needs no attribute-side root, because
// §3.2.1 types an attribute declaration's {type definition} as a Simple Type
// Definition and xsd.NewAttributeDeclaration enforces it, so no unfolded complex
// type exists on that side to reach here.
//
// Clause 2.2 is a conjunction and both conjuncts are decided here: 2.2.1, that
// the ·governing type definition· has an {attribute wildcard} at all, and
// 2.2.2, that the item is ·valid· with respect to it as Item Valid (Wildcard)
// (§3.10.4.1, cvc-wildcard) defines — the {namespace constraint} and the
// ##defined keyword together ([xsd.Schema.AllowsAttributeWildcardName]). Each
// failing conjunct charges clause 2 in its own message, there being no third
// arm of clause 2 to fall to.
//
// An admitted item is ·attributed to· the wildcard and goes on to
// [walk.wildcardAttribute]: under a ***strict*** or ***lax*** wildcard it is
// assessed against the top-level declaration its ·expanded name· ·resolves· to,
// cvc-attribute clauses 3 and 4 with the assertions facets clause 3 reaches, and under
// ***skip*** it is not assessed. A NON-admitted item is ·attributed to· nothing
// (§3.4.4.4), so no wildcard sends it to any declaration, and its own charge is
// the whole of what this element owes for it.
func (w *walk) unmatchedAttribute(a Attribute, e Element, governing xsd.ComplexType) {
	wild, has := governing.AttributeWildcard()
	if !has {
		w.res.violations = append(w.res.violations, xsderr.New(ruleCvcComplexType, a.Loc(),
			"the attribute %s matches no attribute use among the {attribute uses} of its element's ·governing type definition· (cvc-complex-type clause 2.1), which has no {attribute wildcard} for it to be ·valid· with respect to either (clause 2.2.1), and clause 2 has no third arm",
			a.Name()))
		w.logAttribute(a, ruleCvcComplexType, "2", "charged")
		return
	}
	if !w.schema.AllowsAttributeWildcardName(wild, a.Name()) {
		w.res.violations = append(w.res.violations, xsderr.New(ruleCvcComplexType, a.Loc(),
			"the attribute %s matches no attribute use among the {attribute uses} of its element's ·governing type definition· (cvc-complex-type clause 2.1), and the {attribute wildcard} of that type does not admit its ·expanded name· either, so it is not ·valid· with respect to that wildcard as cvc-complex-type clause 2.2.2 requires (Item Valid (Wildcard), §3.10.4.1)",
			a.Name()))
		w.logAttribute(a, ruleCvcComplexType, "2.2.2", "charged")
		return
	}
	w.logAttribute(a, ruleCvcComplexType, "2.2", "satisfied")
	w.wildcardAttribute(a, e, wild.ProcessContents())
}

// requiredAttributeUses charges clause 3 for each {required} attribute use
// of governing that no attribute information item of e carries the
// ·expanded name· of. It is an existence test over names and nothing more:
// whether a matching item is itself ·valid· is clause 2's question, and an
// OPTIONAL use with no matching item is silent here whatever its ·effective
// value constraint· says.
//
// attrs is e.[[attributes]] as [walk.attributes] read it, so the two clauses
// quantify over one reading of the source. The scan is NOT filtered by
// isInstanceAttribute: clause 2's exception is clause 2's alone, and its own
// Note keeps a use of xsi:type satisfying clause 3 while being ·attributed
// to· nothing.
func (w *walk) requiredAttributeUses(e Element, attrs []Attribute, governing xsd.ComplexType) {
	for _, u := range governing.AttributeUses() {
		if !u.Required() {
			continue
		}
		name := u.DeclarationName()
		if hasAttributeNamed(attrs, name) {
			continue
		}
		w.res.violations = append(w.res.violations, xsderr.New(ruleCvcComplexType, e.Loc(),
			"the element %s carries no attribute information item named %s, but its ·governing type definition· has an attribute use for that name whose {required} is true, and cvc-complex-type clause 3 requires one to be present",
			e.Name(), name))
		if w.log.Enabled(context.Background(), slog.LevelDebug) {
			w.log.Debug("assessing attribute use", slog.Any("name", name), slog.Any("loc", e.Loc()),
				slog.String("rule", string(ruleCvcComplexType)), slog.String("clause", "3"),
				slog.String("outcome", "charged"))
		}
	}
}

// logAttribute records one attribute information item's assessment on
// [walk.logDecision]'s terms. An attribute assessed against no ·governing type
// definition· has no rule to name and its line carries none — the walk visited
// it and decided nothing.
func (w *walk) logAttribute(a Attribute, rule xsderr.Rule, clause, outcome string) {
	w.logDecision("assessing attribute", a.Name(), a.Loc(), rule, clause, outcome)
}

// logDecision writes one event line (STYLE L1): the ·expanded name· and
// location of the item decided about always, and the rule, clause and outcome
// that settled it wherever anything did.
//
// An empty clause drops the key rather than emitting it empty: cvc-au is one
// undivided sentence with no numbered clauses, so there is no clause to name
// and a "clause=" with nothing after it would read as a missing value. A
// decision settled by a clause of some anchor other than rule passes it empty
// too, the clause going in the message: beside rule it would cite rule's
// sentence of that number, or one rule does not have (e-validity clause 1.1.3
// under [ruleCvcAssessElt]).
func (w *walk) logDecision(event string, name xsd.QName, loc xsderr.Loc, rule xsderr.Rule, clause, outcome string) {
	if !w.log.Enabled(context.Background(), slog.LevelDebug) {
		return
	}
	attrs := []slog.Attr{slog.Any("name", name), slog.Any("loc", loc)}
	if rule != "" {
		attrs = append(attrs, slog.String("rule", string(rule)))
		if clause != "" {
			attrs = append(attrs, slog.String("clause", clause))
		}
		attrs = append(attrs, slog.String("outcome", outcome))
	}
	w.log.LogAttrs(context.Background(), slog.LevelDebug, event, attrs...)
}

// decline records one check the walk REACHED and did not perform as an
// [Unevaluated] under rule — the bare ID the check would have been charged
// under, the clause going in the message — and logs it as "declined" at clause
// under event, naming the item at loc. Recording and logging are one call so a
// site cannot do one without the other: a decline that only logged would leave
// an empty [Result] reading as a walk that checked everything it reached.
//
// Every decline site of the package calls it, directly or through
// [contentCheck.decline], [walk.declineAttribute], [walk.declineID] or
// [icTarget.decline], except the ones [Unevaluated]'s own doc names as not
// recorded.
func (w *walk) decline(event string, name xsd.QName, loc xsderr.Loc, rule xsderr.Rule, clause, format string, args ...any) {
	w.res.unevaluated = append(w.res.unevaluated, newUnevaluated(rule, loc, format, args...))
	w.logDecision(event, name, loc, rule, clause, "declined")
}

// declineAttribute is [walk.decline] for a check on the attribute information
// item a, recorded at a's own location.
func (w *walk) declineAttribute(a Attribute, rule xsderr.Rule, clause, format string, args ...any) {
	w.decline("assessing attribute", a.Name(), a.Loc(), rule, clause, format, args...)
}

// logSkipped records the one child the walk does not assess at all: a ·skipped·
// one, ·attributed to· a skip Wildcard or to an {open content} with a skip
// {wildcard} ([walk.childGoverning]). Both log cvc-assess-elt clause 2, "If E
// is ·skipped·, then E must not be ·assessed·", the clause that stops the
// descent at a skip wildcard: key-sva (§3.3.4.6) clause 3.2 is the definition
// clause that names the skip, and key-skipped (§3.10.4.1) extends the
// ·skipped· outcome to every descendant. It is written here because
// [walk.element] is never reached for it, so its "assessing element" line —
// which every other element gets, whatever was or was not decided about it
// (STYLE L1) — has nowhere else to come from, and it carries the outcome that
// says the subtree below is unassessed rather than merely undecided.
func (w *walk) logSkipped(e Element) {
	if !w.log.Enabled(context.Background(), slog.LevelDebug) {
		return
	}
	w.log.LogAttrs(context.Background(), slog.LevelDebug, "assessing element",
		slog.Any("name", e.Name()), slog.Any("loc", e.Loc()),
		slog.String("rule", string(ruleCvcAssessElt)), slog.String("clause", "2"),
		slog.String("outcome", "skipped"))
}

// attributeUseNamed reports the attribute use among uses whose {attribute
// declaration} has the ·expanded name· n, as clause 2.1 matches. The name is
// read off the use itself ([xsd.AttributeUse.DeclarationName]) and never off
// a resolved declaration: the match is by name, and a resolution that failed
// for some unrelated reason must not turn a declared attribute into an
// unmatched one.
func attributeUseNamed(uses []xsd.AttributeUse, n xsd.QName) (xsd.AttributeUse, bool) {
	for _, u := range uses {
		if u.DeclarationName() == n {
			return u, true
		}
	}
	return xsd.AttributeUse{}, false
}

// hasAttributeNamed reports whether attrs holds an attribute information
// item whose ·expanded name· is n, as clause 3 asks.
func hasAttributeNamed(attrs []Attribute, n xsd.QName) bool {
	for _, a := range attrs {
		if a.Name() == n {
			return true
		}
	}
	return false
}

// isInstanceAttribute reports whether n is one of the four attributes
// cvc-complex-type clause 2 excepts by name — xsi:type, xsi:nil,
// xsi:schemaLocation, xsi:noNamespaceSchemaLocation, the Built-in Attribute
// Declarations of §3.2.7, which §3.2.6.4 no-xsi forbids a schema to
// redeclare.
func isInstanceAttribute(n xsd.QName) bool {
	if n.Space != xsd.XMLSchemaInstanceNS {
		return false
	}
	switch n.Local {
	case "type", "nil", "schemaLocation", "noNamespaceSchemaLocation":
		return true
	}
	return false
}

// text assesses one run of character information items. It reports the run's
// length rather than its content: instance data does not belong in a log.
// Under the silent default the guard leaves it with no body, on the same terms
// as attribute. The ·initial value· the run contributes to is assembled by the
// element's own [contentCheck], which is what the clauses reading it belong to.
func (w *walk) text(t Text) {
	if !w.log.Enabled(context.Background(), slog.LevelDebug) {
		return
	}
	w.log.Debug("assessing text", slog.Int("chars", len(t.Data())), slog.Any("loc", t.Loc()))
}

// children pulls e's [[children]] in document order and assesses each against
// content, the check e's own ·governing type definition· licenses. It stops at
// the first fault — one the cursor reports, or one raised deeper in the
// subtree — and keeps it: a later child assessed after a fault would be
// assessed out of a context the source never finished delivering.
//
// A walk that stopped early never reaches [contentCheck.end], so a truncated
// [[children]] cursor cannot be charged for ending short of a particle it
// would have satisfied.
//
// inherited is the [inherited attributes] each element [[child]] gets, the
// same for all of them ([walk.handedDown]), and below is their
// [assertionAncestry], whose parent is e's clause 6 state, which each of them
// hands its typed value and its read attributes to ([walk.keepChild]).
func (w *walk) children(e Element, content *contentCheck, id *icCheck, below assertionAncestry, inherited []inheritedAttribute) {
	kids := e.Children()
	for {
		c, ok := kids.Next()
		if !ok {
			break
		}
		w.child(c, content, id, below, inherited)
		if w.res.err != nil {
			return
		}
	}
	if err := kids.Err(); err != nil {
		w.res.err = fmt.Errorf("reading the children of %s at %s: %w", e.Name(), e.Loc(), err)
		return
	}
	content.end(w)
}

// child assesses one child, whichever arm it holds: against content first —
// where the item sits in its parent's {content type} is a fact about the
// PARENT — and then, for an element, as a subtree of its own, against the
// ·governing type definition· that same content check just ·attributed· it
// (key-sva (§3.3.4.6) clause 3.1, [walk.childGoverning]). A Child holding
// neither arm is an adapter bug, not a fault in the source, so it panics
// rather than reaching [Result.Err] — that field means the walk stopped on a
// source fault, and a CLI would otherwise report a bug in an adapter to a user
// as a broken document.
//
// A ·skipped· child stops here and not one level down, which is what makes it
// the whole SUBTREE that is not ·assessed· (key-sva clause 3.2, cvc-assess-elt
// clause 2): [walk.element] is the only path that assesses a child's own
// [[children]], so declining to call it leaves every element below the skipped
// one unassessed, whatever its own attribution would have been. It is also
// what keeps inherited, the child's [inherited attributes], from reaching a
// child e-inherited_attributes gives none: one attributed to a skip Wildcard.
// The one read of a ·skipped· subtree is [assertionAncestry.skipped]'s, and
// only where an ancestor's {test} counts nodes of it: the names of its elements
// and their attributes, for the Tally, and nothing assessed. A fault in the
// source met there stops the walk as one met here would.
//
// A child [contentCheck.element] reports undecided never reaches
// [walk.childGoverning]: it takes [governance]'s undecided shape here, and
// clause 5 ([walk.locallyDeclaredType]), which an undecided child's absent
// ·governing type definition· makes vacuous, is not consulted for it.
func (w *walk) child(c Child, content *contentCheck, id *icCheck, up assertionAncestry, inherited []inheritedAttribute) {
	if e, ok := c.Element(); ok {
		a, undecided := content.element(w, e)
		if a == nil && content.g.laxlyAssessed() {
			// A ·laxly assessed· parent (key-lva) is locally validated against
			// xs:anyType, whose {particle} is a lax wildcard over any namespace
			// (§3.4.7), so this child is ·attributed to· that wildcard
			// (cvc-complex-type clause 1.4) and key-governing-ed clause 3 gives
			// it the declaration its ·expanded name· ·resolves· to: ·strictly
			// assessed· where one resolves, ·laxly assessed· again where none
			// does, and never invalid for the want of one — the wildcard is lax,
			// so e-validity clause 1.1.3 is not live (#1823).
			w.element(e, w.resolvedGovernance(e, inherited), id, up, inherited)
			return
		}
		if undecided {
			// GAP(validate): an undecided child ([governance]) is one this
			// package could not decide the governance of, and not one §3.3.4.6
			// leaves ungoverned: its true attribution may leave it ·skipped·,
			// governed by a ·locally declared type·, or governed by a
			// ·context-determined declaration· (key-governing-ed clauses 2 and
			// 4, key-governing-type-elem clauses 5 to 7), and it is walked
			// against nothing. RULED permanent by #2174 (STYLE P3b): the shape has no
			// retirement route of its own and retires with its last producer.
			// Those are (a1) [contentCheck.match]'s nil matcher beside a
			// determined parent type, [xsd.Schema.ContentMatcher] declining
			// at the maxPartitionStates ceiling #1601 ruled permanent or at
			// flatten's dangling-reference arm, unreachable on a finalized
			// Schema; a parent whose ·governing type definition· was not
			// determined, by (b1) [walk.conditionallySelected]'s key-cta-ta-select
			// decline, which ta-props-correct clause 2 licenses, (b2)
			// [walk.resolvedSelection]'s nil {type definition} slot, which #2166
			// ruled permanent, or (b3) [walk.instanceOverride]'s
			// [xsd.Schema.ValidlySubstitutable] error, unreachable for the reason
			// xsd's validlyDerived states; and, transitively, a parent itself
			// undecided, which adds (c) [walk.localGovernance]'s ValidlySubstitutable
			// error, unreachable for the same reason.
			//
			// Its consumers, and the direction each charges in: [idTable.charge]
			// withholds its clause 1 arm (w.ids.declined below), fail-open, since a
			// declaration beneath the child may be one cvc-id never read. It keeps
			// charging clause 2 over what remains, fail-open too: [walk.attributeType]
			// declines every attribute of the child rather than reading it by ·expanded
			// name· (key-governing-ad clause 2 before clause 3), so [walk.idAttributes]
			// adds no member for it and [icCheck.fieldAttributes] declines a field over
			// it; walk.attributeType's own marker carries that decline.
			// [walk.instanceTypeResolves] and [walk.instanceNilLexical] charge it
			// nothing, so cvc-attribute clauses 3 and 5 against its xsi attributes are
			// lost, fail-open. [governance.laxlyAssessed] is false for it, so its own
			// [[children]] come back to this branch and not to the lax arm above: the
			// shape is NOT ·lax assessment· (cvc-assess-elt clause 3, key-lva), which
			// would resolve them by name against xs:anyType's lax wildcard, and it
			// accepts more than lax assessment would. Its nil type leaves
			// [walk.abstractType], [walk.attribute], [contentCheck.element] and
			// [walk.idElement] nothing to decide, [icCheck.fill] declines any field slot
			// the child would fill, and [walk.locallyDeclaredType] is not consulted
			// (above). [walk.tallyElement] declines the {assertions} of every
			// ancestor one of whose {test}s counts attribute nodes, or filters
			// children by their attributes, at the child's depth, whose Tallies
			// cannot be told of its ·defaulted attributes·, fail-open; any other
			// ancestor counts its element node.
			//
			// It records no [Unevaluated] of its own. A clause 1.4 the matcher
			// declined is content.element's cvc-complex-content record, and a
			// parent whose type was undetermined carries its own idElement or
			// [walk.localGovernance] record or sits below one, so a record here
			// would restate one decline per child.
			w.ids.declined = true
			w.element(e, governance{undecided: true}, id, up, inherited)
			return
		}
		g, assess := w.childGoverning(e, a, content.g.complexType(), inherited)
		if !assess {
			w.logSkipped(e)
			if err := up.skipped(e); err != nil {
				w.res.err = err
			}
			return
		}
		w.unresolvedStrictWildcardChild(content, e, a, g)
		w.locallyDeclaredType(content, e, g)
		w.element(e, g, id, up, inherited)
		return
	}
	t, ok := c.Text()
	if !ok {
		panic("validate: walk.child: Child holds neither arm; build one with ElementChild or TextChild")
	}
	content.text(w, t)
	id.text(t)
	up.collectText(content, t)
	w.text(t)
}
