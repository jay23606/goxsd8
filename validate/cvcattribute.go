package validate

import (
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xpath"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// This file holds the ATTRIBUTE charges that need a value space: an attribute
// information item's lexical read through its declaration's {type definition}
// (cvc-attribute clause 3), the two independent fixed-value agreements
// (cvc-attribute clause 4 and cvc-au), and a ·defaulted attribute·'s own
// {lexical form} (cvc-complex-type clause 4). All four are reached from
// [walk.attributes]; the walk topology itself is assess.go's, and the content
// half's own value charge — an element's ·initial value· under cvc-complex-type
// clause 1.2 — is cvccomplexcontent.go's.
//
// Every one of them delegates to String Valid (§3.16.4, cvc-simple-type) and
// so inherits its three clauses, which [walk.stringValid] runs
// (cvcsimpletype.go): whiteSpace normalization (cl.1) and Datatype Valid (cl.2,
// Datatypes §4.1.4) through value.ValidateLexical, in that order, then clause
// 3's "every ·ENTITY value· in V is a ·declared entity name·" against the
// document's [unparsedEntities] ([UnparsedEntities]). Clause 2 includes that a
// NOTATION value names a notation declared in the schema (Datatypes §3.3.19),
// which value.ValidateLexical decides against [xsd.Schema.Notations].
//
// GAP(validate): the [unparsedEntities] validate/xmlsrc presents is read from
// the DOCTYPE's internal subset, its internal parameter entities expanded under
// a depth and size bound. The external DTD subset is declined, and so is an
// external parameter entity or one past that bound, so an unparsed entity
// declared only there is not a member: XML 1.0 §5.1 and §5.2 oblige a
// non-validating processor to read the document entity alone, RULED permanent
// by #1668 (STYLE P3b). The direction is fail-CLOSED: the set's one reader,
// [walk.entitiesDeclared], charges on a name the set lacks, and words the
// charge through [DeclarationsProcessed] as possibly declared where the source
// did not read.

// ruleCvcAttribute is Attribute Locally Valid (Structures §3.2.4.1,
// cvc-attribute). The clause charged goes in the message on ruleCvcElt's
// terms: the catalog carries the bare name, so "cvc-attribute.3" is not a
// valid [xsderr.Rule].
const ruleCvcAttribute xsderr.Rule = "cvc-attribute"

// ruleCvcAu is Attribute Locally Valid (Use) (Structures §3.5.4, cvc-au). It
// is a rule of its own and not a clause of cvc-attribute: it tests the
// ATTRIBUTE USE's {value constraint}, where cvc-attribute clause 4 tests the
// DECLARATION's, and au-props-correct clause 3 reconciles the two at schema
// assembly rather than making either instance check redundant.
const ruleCvcAu xsderr.Rule = "cvc-au"

// attributeValidity is the [validity] of an attribute information item
// (Structures §3.2.5.1, sic-a-outcome) as this package settles it: valid where
// it is locally ·valid· by cvc-attribute (§3.2.4.1), invalid where cvc-attribute
// clause 3 or 4 was charged, and notKnown where this package declined either
// clause and so did not decide it. The closed set is named after the PSVI's
// three values, but its notKnown is this package's withheld verdict and not
// the PSVI's unassessed one. cvc-au (§3.5.4) never sets it: that rule makes
// the ELEMENT invalid through cvc-complex-type clause 2.1 and leaves the
// attribute's [validity] to cvc-attribute alone.
type attributeValidity uint8

const (
	// attributeValid is an attribute cvc-attribute holds of.
	attributeValid attributeValidity = iota
	// attributeInvalid is an attribute cvc-attribute clause 3 or 4 charged.
	attributeInvalid
	// attributeNotKnown is an attribute cvc-attribute clause 3 or 4 declined.
	attributeNotKnown
)

// attributeVerdict is one attribute information item E carries that matched an
// {attribute use} of E's ·governing type definition· (cvc-complex-type clause
// 2.1), with its [validity] ([walk.matchedAttribute]).
type attributeVerdict struct {
	a        Attribute
	validity attributeValidity
}

// matchedAttribute settles clause 2.1 for an attribute information item that
// matched an attribute use, which is where cvc-complex-type sends it: to
// cvc-attribute against the use's {attribute declaration}
// ([walk.declaredAttribute]) and to cvc-au against the use itself. Both are
// charged here, in that order, and both can fire for one attribute — they read
// two different {value constraint}s. It reports the attribute's [validity],
// which cvc-attribute alone decides ([attributeValidity]): a cvc-au charge
// leaves it valid.
//
// A use whose {attribute declaration} does not resolve is declined, notKnown. It
// is unreachable on a *xsd.Schema that exists (Phase A charges src-resolve for a
// dangling Ref), and a decline rather than a charge because a resolution that
// failed says nothing about the attribute. It logs alone: no schema produces it.
// cvc-au is asked only where cvc-attribute clause 3 was satisfied, the ·actual
// value· it compares existing only then.
func (w *walk) matchedAttribute(a Attribute, e Element, u xsd.AttributeUse) attributeValidity {
	d, resolved := w.schema.ResolvedAttributeDeclaration(u)
	if !resolved {
		// Unreachable on a *xsd.Schema that exists (see the doc above), so it
		// logs alone rather than recording an [Unevaluated] no schema can produce.
		w.logAttribute(a, ruleCvcAttribute, "3", "declined")
		return attributeNotKnown
	}
	st, validity := w.declaredAttribute(a, e, d)
	if st == nil {
		return validity
	}
	if f, fixed := useFixed(u); fixed {
		w.fixedAgreement(a, e, st, f)
	}
	return validity
}

// wildcardAttribute settles key-sva (§3.3.4.6) clause 2 for an
// attribute information item ·attributed to· an {attribute wildcard} whose
// {process contents} is pc (cvc-complex-type clause 2.2): the {attribute
// wildcard} of its element's ·governing type definition·
// ([walk.unmatchedAttribute]), or xs:anyType's on a ·laxly assessed· element
// ([walk.attribute], §3.4.7).
//
// Under ***strict*** or ***lax*** its ·governing attribute declaration· is the
// top-level one its ·expanded name· ·resolves· to (key-governing-ad clause 3,
// §3.10.4.1), and clause 2.1 assesses it against that declaration: cvc-attribute
// clauses 3 and 4 ([walk.declaredAttribute]). Under ***skip*** it is ·skipped·
// (key-skipped) and has no governing declaration, and a name that resolves no
// declaration has none either, under lax and strict alike: clause 2.2 leaves
// both unassessed, and nothing is charged or recorded. e-validity clause 1.1.3
// does not reach the second — it names an item ·attributed to· a strict
// ·wildcard particle·, and an {attribute wildcard} sits in no particle.
func (w *walk) wildcardAttribute(a Attribute, e Element, pc xsd.ProcessContents) {
	if pc == xsd.ProcessSkip {
		return
	}
	d, found := w.topLevelAttribute(a)
	if !found {
		return
	}
	w.declaredAttribute(a, e, d)
}

// declaredAttribute charges cvc-attribute (§3.2.4.1) clauses 3 and 4 for the
// attribute information item a against its ·governing attribute declaration·
// d, and reports the {type definition} clause 3 was satisfied against — nil
// where clause 3 was charged or declined, leaving no ·actual value· for a later
// fixed-value agreement to read — and the attribute's [validity]
// ([attributeValidity]): invalid where clause 3 or 4 was charged, notKnown
// where either was declined, valid otherwise. It is the one encoding of that
// assessment, reached from both arms of cvc-complex-type clause 2
// ([walk.matchedAttribute] for 2.1, [walk.wildcardAttribute] for 2.2).
//
// The assertions facets clause 3's String Valid reaches on the declaration's
// {type definition} are evaluated inside it, so a failed {test} is part of the
// verdict clause 3 charges; one the evaluator declines is recorded under
// cvc-assertions-valid at the attribute ([walk.declineAssertions],
// cvcassertion.go) in place of the clause 3 decline.
//
// The three declines below withhold a verdict rather than guess one, and each is
// recorded as an [Unevaluated] at the attribute ([walk.declineAttribute]):
//
//   - a {type definition} that is absent or COMPLEX. GAP(validate): §3.2.1
//     types the slot as a Required Simple Type Definition, and both shapes still
//     reach an assembled schema. [xsd.NewAttributeDeclaration] accepts a nil
//     {type definition} from a caller building components directly — never from
//     a schema document, where §3.2.2.2's third tier assigns xs:anySimpleType —
//     and it refuses only an INLINE complex type, so a by-name @type naming a
//     complex type passes it and Finalize alike. (An unresolvable one never gets
//     here: Finalize charges src-resolve for a by-name reference naming
//     nothing.) Both are schema faults Finalize leaves unchecked with the rest of
//     sch-props-correct clause 1, a deferral [xsd.Schema]'s own doc records, so
//     this package declines rather than charge cvc-attribute clause 2 for an
//     absence xsd admitted. RULED permanent by #774 (STYLE P3b): the charge
//     belongs to xsd's deferred schema check, not to the instance walk.
//   - a lexical whose ValidateLexical error is not a VERDICT
//     ([value.IsDatatypeVerdict]). GAP(validate): a type no backend maps is the
//     live case, and the resulting error carries cvc-datatype-valid exactly as
//     a genuine rejection does, so charging it would reject every attribute of
//     that type for a gap in the processor. RULED permanent by #774 (STYLE P3b):
//     the decline is inherent to [value.IsDatatypeVerdict]'s split, and a type
//     no backend maps is backend coverage to close, not this package's. The two
//     ·special· datatypes never reach it: [walk.stringValid] decides them
//     ([xsd.SimpleType.IsSpecial]), so the typeless <attribute> §3.2.2.2's third
//     tier types as xs:anySimpleType is satisfied, not declined.
//   - an ·ENTITY value· candidate whose ·validating type· this package cannot
//     decide, on [walk.entitiesDeclared]'s terms.
func (w *walk) declaredAttribute(a Attribute, e Element, d xsd.AttributeDeclaration) (*xsd.SimpleType, attributeValidity) {
	st, simple := w.schema.ResolvedSimpleType(d.TypeDefinition())
	if !simple {
		w.declineAttribute(a, ruleCvcAttribute, "3",
			"the ·initial value· of the attribute %s was not decided against its declaration's {type definition}, which is absent or not a simple type definition, so cvc-attribute clause 3 is undecided",
			a.Name())
		return nil, attributeNotKnown
	}
	decided, verdict := w.stringValid(st, a.Value(), elementContext{owner: e}, a.Loc())
	if !decided {
		if w.declineAssertions(verdict, "assessing attribute", a.Name(), a.Loc(), "cvc-attribute clause 3") {
			return nil, attributeNotKnown
		}
		w.declineAttribute(a, ruleCvcAttribute, "3",
			"the ·initial value· of the attribute %s was not decided against its declaration's {type definition} %s: String Valid (§3.16.4) was withheld, the value backend reporting a fault of the type rather than a verdict about the lexical or the ·validating type· of an ·ENTITY value· being undecidable, so cvc-attribute clause 3 is undecided",
			a.Name(), st.Name())
		return nil, attributeNotKnown
	}
	if verdict != nil {
		w.res.violations = append(w.res.violations, causedBy(ruleCvcAttribute, a.Loc(), verdict,
			"the ·initial value· of the attribute %s is not ·valid· with respect to its declaration's {type definition} %s, which cvc-attribute clause 3 requires as per String Valid (§3.16.4)",
			a.Name(), st.Name()))
		w.logAttribute(a, ruleCvcAttribute, "3", "charged")
		return nil, attributeInvalid
	}
	w.logAttribute(a, ruleCvcAttribute, "3", "satisfied")
	f, fixed := declarationFixed(d)
	if !fixed {
		return st, attributeValid
	}
	return st, w.fixedAgreement(a, e, st, f)
}

// instanceTypeResolves charges cvc-attribute (§3.2.4.1) against e's xsi:type
// attribute, where D is the built-in declaration for the type attribute
// (§3.2.7.1): clause 3, String Valid against D's xs:QName {type definition}
// ([walk.instanceTypeLexical]), and clause 5, that A's ·actual value·
// ·resolves· to a type definition.
//
// It is reached from [walk.attributes] (assess.go) and never through
// [walk.matchedAttribute], because no attribute use ever matches xsi:type —
// cvc-complex-type clause 2 excepts the four Built-in Attribute Declarations by
// name (isInstanceAttribute) and §3.2.6.4 no-xsi forbids a schema to
// redeclare them. That exception is cvc-complex-type clause 2's own quantifier
// and nothing wider: the Note under §3.2.4.2 keeps an xsi:type attribute
// governed by its built-in declaration whatever the element's ·governing type
// definition· is, so key-sva (§3.3.4.6) clause 2.1 assesses the item through
// cvc-assess-attr (§3.2.4.3), whose clause 2 evaluates it against cvc-attribute
// regardless, and this is the one site that walk reaches it at. It is called
// for BOTH arms of cvc-type clause 3 for the same reason: the governing type
// decides nothing about an attribute it never governs.
//
// The two clauses are one conjunction, but clause 5 quantifies over A's
// ·actual value·, which a lexical clause 3 rejects does not have: such a
// lexical is charged under clause 3 alone and clause 5 is declined, so no
// lexical is charged twice, and none under a clause that does not hold its
// defect. A lexical clause 3 accepts is a QName, and clause 5 reads the name
// [walk.resolveInstanceType] makes of it; one clause 3 declines reaches clause 5
// as it would with no clause 3 at all.
//
// The charge is the ATTRIBUTE's alone and the ELEMENT is untouched by it: a
// lexical that does not ·resolve· leaves E with no ·instance-specified type
// definition· at all, so cvc-elt clause 4 stays vacuous and the element is
// assessed against its ·selected type definition· (the Note under cvc-elt,
// [walk.instanceTypeDefinition]). Neither clause carries such fallback wording,
// and each charges whether or not that fallback succeeds.
//
// An undecided e ([governance]) is charged and declined NOTHING under either
// clause: this package could not decide e's governance, and the true schema may
// make it an item it ·skips·, whose attributes key-governing-ad clause 3 and
// its Note leave with no ·governing attribute declaration·. A Result whose only
// defect is a decline must not also carry a definitive charge on such an item.
// Nothing is recorded in the charge's place: an undecided element sits below a
// record ([walk.child]), or carries its own decline ([walk.localGovernance]).
// [walk.attribute] logs the item ungoverned. A child its parent decidedly
// ·attributed· to nothing is not undecided and is charged here like any other.
func (w *walk) instanceTypeResolves(e Element, g governance) {
	if g.undecided {
		return
	}
	a, present := instanceAttribute(e, "type")
	if !present {
		return
	}
	if !w.instanceTypeLexical(a, e) {
		w.declineAttribute(a, ruleCvcAttribute, "5",
			"the xsi:type attribute of the element %s has no ·actual value· for cvc-attribute clause 5 to read, its lexical %q having been charged under clause 3, so clause 5 is undecided",
			e.Name(), a.Value())
		return
	}
	switch name, _, outcome := w.resolveInstanceType(e, a); outcome {
	case instanceTypeResolved:
		w.logAttribute(a, ruleCvcAttribute, "5", "satisfied")
	case instanceTypeNoValue:
		w.declineAttribute(a, ruleCvcAttribute, "5",
			"the xsi:type attribute of the element %s has the lexical %q, which this package could not map to a QName to ·resolve·, so cvc-attribute clause 5 is undecided",
			e.Name(), a.Value())
	case instanceTypeUnresolved:
		w.res.violations = append(w.res.violations, xsderr.New(ruleCvcAttribute, a.Loc(),
			"the xsi:type attribute of the element %s has the lexical %q, and the schema declares no type definition named %q for it to ·resolve· to (§3.17.6.3, cvc-resolve-instance), which cvc-attribute clause 5 requires of an attribute governed by the built-in declaration for the type attribute (§3.2.7.1)",
			e.Name(), a.Value(), name))
		w.logAttribute(a, ruleCvcAttribute, "5", "charged")
	}
}

// instanceNilLexical charges cvc-attribute (§3.2.4.1) clause 3 against e's
// xsi:nil attribute where e has no ·governing element declaration·: its
// ·initial value· String Valid (§3.16.4) against xs:boolean, the {type
// definition} of the built-in declaration for the nil attribute (§3.2.7.2). The
// attribute is governed by that declaration whatever e's assessment
// (key-governing-ad, and the Note under it), so it is charged on a ·laxly
// assessed· element (key-lva) and on one ·strictly assessed· against a type
// alone — its ·locally declared type·, or an xsi:type ·overriding· it or
// standing in for an ·absent· one (key-governing-type-elem clauses 6-8).
//
// Under a ·governing element declaration· it charges nothing: [walk.nilCheck]'s
// cvc-elt clause 3.1 or 3.2 already makes the element invalid for the same
// lexical, and the one defect is charged once. The lexical is read through
// [instanceBooleanOf] and not the injected value.Backend, on [nilled]'s grounds.
//
// An undecided e ([governance]) is charged nothing either, on
// [walk.instanceTypeResolves]'s grounds: whether cvc-assess-elt assesses it
// is undecided, so the declaration-less shapes above do not include it.
func (w *walk) instanceNilLexical(e Element, g governance) {
	if g.hasDecl || g.undecided {
		return
	}
	a, present := instanceAttribute(e, "nil")
	if !present {
		return
	}
	if instanceBooleanOf(a.Value()) != instanceBooleanUnreadable {
		w.logAttribute(a, ruleCvcAttribute, "3", "satisfied")
		return
	}
	w.res.violations = append(w.res.violations, xsderr.New(ruleCvcAttribute, a.Loc(),
		"the xsi:nil attribute of the element %s has the ·initial value· %q, which is not ·valid· with respect to xs:boolean, the {type definition} of the built-in declaration for the nil attribute (§3.2.7.2), as cvc-attribute clause 3 requires per String Valid (§3.16.4): after the whiteSpace collapse it is none of the literals true, false, 1 and 0",
		e.Name(), a.Value()))
	w.logAttribute(a, ruleCvcAttribute, "3", "charged")
}

// qnameTypeName is xs:QName, the {type definition} of the built-in declaration
// for the type attribute (§3.2.7.1).
var qnameTypeName = xsd.QName{Space: xsd.XMLSchemaNS, Local: "QName"}

// instanceTypeLexical charges cvc-attribute (§3.2.4.1) clause 3 against the
// xsi:type attribute a of e — its ·initial value· String Valid (§3.16.4)
// against xs:QName — and reports whether a has an ·actual value· for clause 5
// to quantify over, which is false exactly where it charges. The lexical is
// mapped under e's in-scope bindings, so a prefix with no binding there fails
// the mapping Datatype Valid (Datatypes §4.1.4 clause 2.1) requires a value
// from, as an empty lexical, a colon structure no QName has and a part that is
// no NCName fail xs:QName's lexical space (§3.3.18).
//
// It is [walk.declaredAttribute]'s clause 3 over the built-in declaration for
// the type attribute, which no attribute use carries, and declines on the same
// terms, reporting true so clause 5 reads the lexical as it would with no clause
// 3 at all:
//
//   - a schema whose {type definitions} carry no simple xs:QName, which only
//     one assembled through [xsd.SchemaBuilder.Finalize] without the built-in
//     types seeded can be.
//   - a lexical whose ValidateLexical error is not a VERDICT
//     ([value.IsDatatypeVerdict]). GAP(validate): a backend that does not map
//     xs:QName leaves an xsi:type lexical outside its lexical space to clause
//     5, which declines the ones [resolveInstanceQName] turns away and charges
//     the rest as names no type carries. RULED permanent by #774 (STYLE P3b),
//     on [walk.declaredAttribute]'s terms: mapping xs:QName is backend coverage,
//     not this package's.
func (w *walk) instanceTypeLexical(a Attribute, e Element) bool {
	st, simple := w.schema.ResolvedSimpleType(xsd.TypeDefinitionRef{Name: qnameTypeName})
	if !simple {
		w.declineAttribute(a, ruleCvcAttribute, "3",
			"the xsi:type attribute of the element %s was not decided against xs:QName, the {type definition} of the built-in declaration for the type attribute (§3.2.7.1): the schema carries no simple type of that name, so cvc-attribute clause 3 is undecided",
			e.Name())
		return true
	}
	_, err := value.ValidateLexical(w.backend, w.schema, st, a.Value(), elementContext{owner: e}, xpath.FacetAssertions(w.now))
	if err == nil {
		w.logAttribute(a, ruleCvcAttribute, "3", "satisfied")
		return true
	}
	if !value.IsDatatypeVerdict(err) {
		w.declineAttribute(a, ruleCvcAttribute, "3",
			"the xsi:type attribute of the element %s was not decided against %s, the {type definition} of the built-in declaration for the type attribute (§3.2.7.1): the value backend reported a fault of the type rather than a verdict about the lexical, so cvc-attribute clause 3 is undecided",
			e.Name(), st.Name())
		return true
	}
	w.res.violations = append(w.res.violations, causedBy(ruleCvcAttribute, a.Loc(), err,
		"the xsi:type attribute of the element %s has the ·initial value· %q, which is not ·valid· with respect to %s, the {type definition} of the built-in declaration for the type attribute (§3.2.7.1), as cvc-attribute clause 3 requires per String Valid (§3.16.4)",
		e.Name(), a.Value(), st.Name()))
	w.logAttribute(a, ruleCvcAttribute, "3", "charged")
	return false
}

// fixedConstraint is one fixed {value constraint} together with the rule that
// reads it and the words that rule's message needs: the clause it charges (empty
// for cvc-au, which is one undivided sentence with no numbered clauses) and the
// component the constraint sits on. They travel WITH the constraint rather than
// being derived from the rule ID at the message site, so adding a third reader
// is one constructor and not a third arm of a mapping that can go stale.
type fixedConstraint struct {
	vc     xsd.ValueConstraint
	rule   xsderr.Rule
	clause string
	owner  string
}

// fixedAgreement charges the "equal or identical" agreement one fixed {value
// constraint} demands of a present attribute — cvc-attribute clause 4 for the
// declaration's, cvc-au for the use's — or nothing at all where there is no
// fixed constraint to agree with, which is the vacuous case both rules state.
// It reports its outcome as the [attributeValidity] it gives where f is the
// declaration's: valid where the values agree, invalid where it charges, and
// notKnown where it declines. Only [walk.declaredAttribute] reads it; the
// cvc-au call from [walk.matchedAttribute] discards it, since cvc-au never
// decides the attribute's [validity].
//
// The comparison is in the VALUE space and never over lexicals:
// [value.ConstraintMatches] maps both sides through st's pipeline, the
// attribute's under the instance's namespace context and the constraint's
// under the schema document's, so "1" and "01" agree as one xs:integer and
// "a:x" and "b:x" agree exactly when both prefixes name one namespace.
//
// Each side's assertions facets are evaluated through [xpath.FacetAssertions]
// (cvc-assertions-valid via cvc-datatype-valid clause 3), so an assertions
// facet in st's closure decides the comparison rather than declining it.
//
// A ·special· st, whose lexical mapping is not a function (Datatypes §3.2.1.2),
// is decided over that mapping's union of primitive and list mappings, as
// [value.ConstraintMatches] states.
//
// An undecided answer charges nothing and is recorded as an [Unevaluated].
// GAP(validate): that covers a type this backend does not govern, a ·special·
// st whose two literals some member of its mapping union cannot compare (an
// unmapped primitive, or values with no sameness relation), and a {lexical form}
// outside its own type's lexical space (a schema fault cos-valid-simple-default
// charges at assembly, not the instance's). Charging on undecided would reject a
// document for a gap in the processor. The ·special· residue is RULED permanent
// by #2040 (STYLE P3b): a member that cannot answer may be the one that equates
// the two literals, so no verdict exists to give; #2029 ruled the wider residue
// this one narrows (provenance). The rest is RULED permanent by #774 (STYLE
// P3b): cos-valid-simple-default (§3.2.6.2) is a Schema Component Constraint,
// and a schema assembled through [xsd.SchemaBuilder.Finalize] rather than
// FinalizeWith carries an undecided value space, so that check may never have
// run; the instance walk has no sound verdict to give in its place.
//
// GAP(xpath): a {test} the evaluator declines on either side is undecided too;
// it charges nothing and is recorded as above, on the terms
// [value.ConstraintMatches] states. (#1042)
//
// A {lexical form} that fails one of st's {test}s charges nothing and is
// recorded as above as well: the ValidDefault of the [xsd.ValueSpace]
// [value.NewValueSpace] returns runs the pipeline with no evaluator of its own
// and declines every {test} (its gate 2's GAP(value)), so such a {lexical form}
// is charged by no assembly, and is undecided here rather than NOT-same all the
// same.
//
// GAP(value): a NOTATION {lexical form} naming no declared notation, tracked by
// #667. [value.ConstraintMatches] answers it undecided, not NOT-same, because no
// assembly ever judges it: [xsd.ValueSpace]'s ValidDefault refuses every
// NOTATION-governed default at its gate 1 (needsContext), even under
// FinalizeWith, so cos-valid-simple-default never charges it. The comparison
// declines here until #667 routes those defaults through ValidDefault.
func (w *walk) fixedAgreement(a Attribute, e Element, st *xsd.SimpleType, f fixedConstraint) attributeValidity {
	same, decided := value.ConstraintMatches(w.backend, w.schema, st, a.Value(), elementContext{owner: e}, f.vc, xpath.FacetAssertions(w.now))
	if !decided {
		w.declineAttribute(a, f.rule, f.clause,
			"the ·actual value· of the attribute %s was not compared with the {value} of the fixed {value constraint} %q on its %s: value.ConstraintMatches could not decide the comparison, a fault of the type or of the value backend, two literals of a ·special· type that some member of its lexical mapping cannot compare, an assertions-facet {test} not evaluated on either side or failed by the {value}, or a NOTATION {value} naming no declared notation, which no assembly judges yet (#667), rather than a verdict about the value, so %s is undecided",
			a.Name(), f.vc.LexicalForm(), f.owner, citation(f.rule, f.clause))
		return attributeNotKnown
	}
	if same {
		w.logAttribute(a, f.rule, f.clause, "satisfied")
		return attributeValid
	}
	w.res.violations = append(w.res.violations, xsderr.New(f.rule, a.Loc(),
		"the ·actual value· of the attribute %s is neither equal nor identical to the {value} of the fixed {value constraint} %q on its %s, which %s requires",
		a.Name(), f.vc.LexicalForm(), f.owner, citation(f.rule, f.clause)))
	w.logAttribute(a, f.rule, f.clause, "charged")
	return attributeInvalid
}

// declarationFixed reports D.{value constraint} where its {variety} is fixed —
// the only shape cvc-attribute clause 4 tests.
func declarationFixed(d xsd.AttributeDeclaration) (fixedConstraint, bool) {
	vc, has := d.ValueConstraint()
	if !has || vc.Kind() != xsd.ValueFixed {
		return fixedConstraint{}, false
	}
	return fixedConstraint{vc: vc, rule: ruleCvcAttribute, clause: "4", owner: "attribute declaration"}, true
}

// useFixed reports U.{value constraint} where its {variety} is fixed — the only
// shape cvc-au tests. It reads the USE's own property and never the ·effective
// value constraint·, which would substitute the declaration's fixed value for a
// use that carries none and charge cvc-au for an agreement the rule does not
// demand.
func useFixed(u xsd.AttributeUse) (fixedConstraint, bool) {
	vc, has := u.ValueConstraint()
	if !has || vc.Kind() != xsd.ValueFixed {
		return fixedConstraint{}, false
	}
	return fixedConstraint{vc: vc, rule: ruleCvcAu, owner: "attribute use"}, true
}

// citation renders the rule and clause a message names inline (STYLE E4), and
// the bare rule for one that has no numbered clauses at all.
func citation(rule xsderr.Rule, clause string) string {
	if clause == "" {
		return string(rule)
	}
	return string(rule) + " clause " + clause
}

// defaultedAttributes charges cvc-complex-type (§3.4.4.2) clause 4: for each
// ·defaulted attribute· of e, the {lexical form} of its ·effective value
// constraint· must be ·valid· with respect to the DECLARATION's {type
// definition} per String Valid. It validates a value the schema supplies, not
// one the document carries — the attribute is absent, which is what makes it
// defaulted — so it is the one charge here that a schema alone determines.
//
// The five conjuncts of ·defaulted attribute· (key-dflt-att) are applied in
// their own order: membership in {attribute uses} is the range, then
// {required} = false (clause 2), then a non-·absent· ·effective value
// constraint· (clause 3), then not one of the four Built-in Attribute
// Declarations (clause 4, isInstanceAttribute — §3.2.6.4 no-xsi forbids
// a schema to redeclare those names, so the name settles it), then no
// attribute information item matching the declaration per clause 2.1
// (clause 5).
//
// The scan is over governing.AttributeUses() in document order, never a map, so
// several charges on one element arrive in a fixed order (STYLE D2). attrs is
// e.[[attributes]] as [walk.attributes] read it, so clause 5 quantifies over
// the same reading of the source clauses 2 and 3 did.
//
// A charge here is rare by construction rather than by accident: a schema
// finalized with a value space installed has already charged
// cos-valid-simple-default (§3.2.6.2) against the same {lexical form}, under
// a-props-correct clause 2 or au-props-correct clause 2, so only a schema
// assembled through [xsd.SchemaBuilder.Finalize] — which installs none —
// reaches this clause with an invalid default still in it, or one whose type's
// closure reaches QName or NOTATION, which [xsd.ValueSpace]'s ValidDefault
// leaves undecided at assembly (#667) and this clause decides.
func (w *walk) defaultedAttributes(e Element, attrs []Attribute, governing xsd.ComplexType) {
	for _, u := range governing.AttributeUses() {
		vc, defaulted := w.defaultedConstraint(u, attrs)
		if !defaulted {
			continue
		}
		w.defaultedAttribute(e, u, vc)
	}
}

// defaultedConstraint reports whether u, a member of the {attribute uses} of
// the complex type governing an element whose [[attributes]] are attrs, is a
// ·defaulted attribute· of that element (key-dflt-att clauses 2 to 5, in their
// own order), and if so the ·effective value constraint· whose {lexical form}
// is the [[normalized value]] Attribute Default Value (§3.4.5.1) supplies. It is
// the one encoding of that definition, read by defaultedAttributes,
// [walk.handedDown], [walk.idDefaultedAttributes],
// [icCheck.fieldDefaultedAttributes] and [walk.assertionValues].
func (w *walk) defaultedConstraint(u xsd.AttributeUse, attrs []Attribute) (xsd.ValueConstraint, bool) {
	if u.Required() { // clause 2
		return xsd.ValueConstraint{}, false
	}
	vc, constrained := w.schema.EffectiveValueConstraint(u)
	if !constrained { // clause 3
		return xsd.ValueConstraint{}, false
	}
	if isInstanceAttribute(u.DeclarationName()) { // clause 4
		return xsd.ValueConstraint{}, false
	}
	if hasAttributeNamed(attrs, u.DeclarationName()) { // clause 5
		return xsd.ValueConstraint{}, false
	}
	return vc, true
}

// defaultedAttribute charges clause 4 for one ·defaulted attribute·. The type
// is read off A.{attribute declaration}.{type definition} and never off the
// use, which the clause is explicit about.
//
// The question is String Valid (§3.16.4) over one {lexical form} against one
// type, asked through [walk.stringValid] — the same check cvc-attribute clause
// 3 runs over a carried attribute — under [value.ConstraintContext]: the
// {lexical form} is a literal of the schema document, so a QName- or
// NOTATION-valued one resolves its prefix against the bindings in scope where
// the schema wrote it (cos-valid-simple-default clause 2, Datatypes §3.3.18),
// never against e's. Against w.schema, value.ValidateLexical also judges whether
// a NOTATION default names a declared notation (Datatypes §3.3.19), and an
// ·ENTITY value· in the default is a ·declared entity name· or not by the
// DOCUMENT's [unparsedEntities] ([walk.entitiesDeclared]). A decided rejection
// hands back the verdict itself, which the charge carries as its wrapped cause
// (validate.go's causedBy).
//
// A withheld verdict charges nothing and is recorded as an [Unevaluated] at the
// element: an assertions-facet decline under cvc-assertions-valid
// ([walk.declineAssertions]), and any other — an ungoverned type, a
// construction-stage facet failure, a facet-pipeline precondition fault, an
// undecidable ·validating type· for clause 3 — under clause 4
// ([walk.declineDefaulted]), on [walk.declaredAttribute]'s terms; so is a
// {type definition} that is absent or complex.
func (w *walk) defaultedAttribute(e Element, u xsd.AttributeUse, vc xsd.ValueConstraint) {
	d, resolved := w.schema.ResolvedAttributeDeclaration(u)
	if !resolved {
		// Unreachable on a *xsd.Schema that exists, on [walk.matchedAttribute]'s
		// terms, so it records no [Unevaluated] no schema can produce.
		return
	}
	st, simple := w.schema.ResolvedSimpleType(d.TypeDefinition())
	if !simple {
		w.declineDefaulted(e, u,
			"the {lexical form} %q of the ·effective value constraint· of the ·defaulted attribute· %s of the element %s was not decided against that declaration's {type definition}, which is absent or not a simple type definition, so cvc-complex-type clause 4 is undecided",
			vc.LexicalForm(), u.DeclarationName(), e.Name())
		return
	}
	decided, verdict := w.stringValid(st, vc.LexicalForm(), value.ConstraintContext(vc), e.Loc())
	if !decided {
		if w.declineAssertions(verdict, "assessing attribute use", u.DeclarationName(), e.Loc(), "cvc-complex-type clause 4") {
			return
		}
		w.declineDefaulted(e, u,
			"the {lexical form} %q of the ·effective value constraint· of the ·defaulted attribute· %s of the element %s was not decided against that declaration's {type definition} %s: String Valid (§3.16.4) was withheld, the value backend reporting a fault of the type rather than a verdict about the lexical or the ·validating type· of an ·ENTITY value· being undecidable, so cvc-complex-type clause 4 is undecided",
			vc.LexicalForm(), u.DeclarationName(), e.Name(), st.Name())
		return
	}
	if verdict == nil {
		return
	}
	w.res.violations = append(w.res.violations, causedBy(ruleCvcComplexType, e.Loc(), verdict,
		"the element %s carries no attribute information item named %s, and the {lexical form} %q of the ·effective value constraint· of the ·defaulted attribute· it would supply is not ·valid· with respect to that declaration's {type definition} %s, which cvc-complex-type clause 4 requires as per String Valid (§3.16.4)",
		e.Name(), u.DeclarationName(), vc.LexicalForm(), st.Name()))
}

// declineDefaulted is [walk.decline] for cvc-complex-type clause 4 over the
// ·defaulted attribute· u of e, recorded at e's location: the attribute is
// absent, which is what makes it defaulted, so there is no attribute item to
// carry a Loc.
func (w *walk) declineDefaulted(e Element, u xsd.AttributeUse, format string, args ...any) {
	w.decline("assessing attribute use", u.DeclarationName(), e.Loc(), ruleCvcComplexType, "4", format, args...)
}
