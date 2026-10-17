package value

import (
	"errors"
	"strings"

	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// This file is the adapter that lets package xsd — a pure leaf that cannot
// import this one (PRINCIPLES 1) — answer the Structures constraints that reach
// into a value space. Two of them COMPARE the {value}s of two Value Constraints:
//
//   - au-props-correct (§3.5.6) clause 3: "U.{value constraint}.{value} is
//     IDENTICAL to U.{attribute declaration}.{value constraint}.{value}" —
//     Datatypes §2.2.1's identity relation.
//   - loc-testSubP (§3.4.6.4) clauses 4.2 and 5.2.2: "an EQUAL OR IDENTICAL
//     value" — the §2.2.1/§2.2.2 union, which §2.2.2 makes the default reading
//     of every sameness test the specification prescribes.
//
// Both are VALUE-space tests, not lexical ones: {value} is an ·actual value·
// (key-vv §3.2.1), so "1" and "01" are the same xs:integer {value} under two
// {lexical form}s, and xsd.ValueConstraint carries only the latter.
//
// The third, cos-valid-simple-default (§3.2.6.2) — charged by a-props-correct
// (§3.2.6.1) clause 2 and au-props-correct clause 2 — is one-sided: it asks
// whether ONE {lexical form} denotes a value of ONE type at all, which is
// Datatype Valid (§4.1.4) and so [ValidateLexical]'s whole job. It needs no
// shared mapping across two types, so it decides the list and union varieties
// the comparisons refuse; what it must NOT do is turn the pipeline's non-verdict
// errors into rejections (ValidDefault says which, and why).

// NewValueSpace returns the [xsd.ValueSpace] backed by b: it maps a Value
// Constraint's {lexical form} through the governing mapping of the type that
// constrains it, compares the resulting values with the capability relations the
// values themselves carry ([Identical], [Eq]), and validates one against its type
// through the full facet pipeline ([ValidateLexical]).
//
// It honors the fail-open contract [xsd.ValueSpace] states: every question it
// cannot answer reports decided=false rather than a verdict, so a type no
// backend mapping governs, an unmappable lexical, a cross-type comparison, or an
// error that belongs to the type rather than to the value constraint can never
// turn into a schema rejection. Two ·special· types (xs:anySimpleType,
// xs:anyAtomicType) are the one ungoverned pair the comparisons decide: over
// their mapping union, as [ConstraintMatches] does, under each comparison's own
// relation. A ·special· type against an ordinary one stays undecided, and
// ValidDefault still answers undecided for a ·special· type, for which
// Datatype Valid is unconditionally true.
//
// It panics if b is nil, matching parser.WithBackend's guard: a nil backend is a
// caller bug, not a schema-validity condition.
func NewValueSpace(b Backend) xsd.ValueSpace {
	if b == nil {
		panic("value: NewValueSpace: nil Backend")
	}
	return valueSpace{b: b}
}

// valueSpace is NewValueSpace's implementation. It holds only the backend: the
// governing mapping and the whiteSpace mode in force are recomputed per
// comparison from the types handed in, never cached (STYLE D3 — a value
// constraint is compared at most a handful of times per finalize, so there is no
// measured hot path to cache for).
type valueSpace struct{ b Backend }

// Identical is Datatypes §2.2.1's identity relation, which au-props-correct
// clause 3 compares two {value}s under.
func (vs valueSpace) Identical(r xsd.TypeResolver, ta *xsd.SimpleType, a xsd.ValueConstraint, tb *xsd.SimpleType, b xsd.ValueConstraint) (bool, bool) {
	return vs.compare(r, ta, a, tb, b, identical)
}

// EqualOrIdentical is the §2.2.1/§2.2.2 equal-or-identical union, which
// loc-testSubP clauses 4.2 and 5.2.2 compare two {value}s under.
func (vs valueSpace) EqualOrIdentical(r xsd.TypeResolver, ta *xsd.SimpleType, a xsd.ValueConstraint, tb *xsd.SimpleType, b xsd.ValueConstraint) (bool, bool) {
	return vs.compare(r, ta, a, tb, b, equalOrIdentical)
}

// relation is one of the two sameness relations the comparisons answer under:
// identical (§2.2.1, au-props-correct clause 3) or equalOrIdentical (§2.2.2,
// loc-testSubP clauses 4.2 and 5.2.2, cvc-attribute clause 4). Those two
// functions are its only values; it exists so one comparison path can carry
// either without a flag.
type relation func(a, b Value) (same, decided bool)

// compare is the ONE gate both comparisons pass through (STYLE T4), deciding a
// and b under rel.
//
// A pair whose types are BOTH ·special· is decided over the mapping union by
// specialMatches before any whiteSpace or shared mapping is sought (#2040's
// RULING, taken here by #1379): NOT-same holds exactly when no member admits
// both raw literals as values the same under rel, so au-props-correct clause 3
// scans the union under identity and loc-testSubP clauses 4.2 and 5.2.2 under
// equal-or-identical. A pair with ONE ·special· side stays undecided: values
// finds no governing mapping for it (#2087).
//
// Every other pair is mapped into one value space by values and compared there.
func (vs valueSpace) compare(r xsd.TypeResolver, ta *xsd.SimpleType, a xsd.ValueConstraint, tb *xsd.SimpleType, b xsd.ValueConstraint, rel relation) (same, decided bool) {
	if ta.IsSpecial() && tb.IsSpecial() {
		return specialMatches(vs.b, ta, tb, a.LexicalForm(), ConstraintContext(a), b.LexicalForm(), ConstraintContext(b), rel)
	}
	av, bv, ok := vs.values(r, ta, a, tb, b)
	if !ok {
		return false, false
	}
	return rel(av, bv)
}

// ValidDefault is Simple Default Valid (§3.2.6.2, cos-valid-simple-default),
// which a-props-correct clause 2 and au-props-correct clause 2 both charge: is
// vc.{lexical form} Datatype Valid (§4.1.4) with respect to t? [ValidateLexical]
// decides exactly that rule — all three of its clauses, over all three varieties
// — so the whole of this method is the two gates that separate the errors it
// returns which ARE that verdict from the ones that are not. Every gate answers
// undecided, never invalid, per the [xsd.ValueSpace] fail-open contract. A
// verdict rides out as the returned cause, unmodified and still carrying its own
// cvc-* rule; an error a gate catches is discarded, and that gate answers
// undecided and causeless.
//
// The first gate runs BEFORE the pipeline; the second reads the pipeline's own
// error through the ONE classification this package publishes:
//
//  1. GAP(value): needsContext. A QName- or NOTATION-governed value space
//     anywhere in t's variety closure needs the namespace bindings in scope at
//     the literal (§3.3.18/§3.3.19, PRINCIPLES 19). vc now CARRIES those
//     bindings, and the comparisons resolve them (sharedMapping), so what is
//     still unclaimed here is the routing: this method does not refuse the list
//     and union varieties, and threading a [Context] to the literal through
//     validateUnion and listMapping — which re-derive their own dispatch — is a
//     rejection surface of its own (a list OF QName, a union WITH a QName
//     member), with its own grounding and its own ratchet attribution. Until
//     that lands the whole closure stays undecided, because a nil [Context]
//     makes a backend reject EVERY QName lexical and every QName default would
//     be a false reject.
//  2. [IsDatatypeVerdict]. An ungoverned type, a construction-stage failure in
//     t's facets, a facet-pipeline precondition fault and an unresolvable base
//     are each a fault of T or of the backend, and each would be reported for
//     EVERY literal — so reading one as clause 2 would reject the schema under a
//     rule with nothing to say about it, for a default no lexical could have
//     satisfied. That predicate names the whole class and why each member
//     belongs to it; this method does not re-derive the list, which is what the
//     three separate pre-check gates it replaced amounted to. The same gate
//     catches an assertions-facet decline ([IsAssertionDeclined]):
//     GAP(value): this package holds no XPath engine, so the pipeline runs
//     with assertionsUndecided and every {lexical form} the other facets of t
//     accept is undecided wherever t's closure carries an assertions facet.
//     The reader of the withheld verdict is xsd's Schema.checkSimpleDefault
//     (a-props-correct and au-props-correct clause 2), which charges only a
//     decided cause and accepts an undecided one, so the direction is
//     fail-open. (#1042)
//
// One residue is recorded rather than papered over. GAP(value): union member
// facet compilation. dispatchUnion folds every member's rejection into one
// cvc-datatype-valid error of its own, so a construction-stage failure in a
// MEMBER's facets loses its marking on the way out and still reaches the caller
// as a decided reject. The LIST half of that residue is already closed:
// listMapping's Parse recurses through validateLexical, which marks an item
// type's fault, and the marking survives the item error's propagation out of
// Parse.
//
// The error model no longer stands in the way of closing the union half: the
// marking now exists, and dispatchUnion already aborts its member scan on one
// class of fault ([IsFacetPrecondition]) — widening that test to the whole class
// is the change. What is left is not difficulty but ATTRIBUTION. Widening it
// turns member faults from decided rejects into declines, which moves verdicts
// and so may move a lane, and that movement belongs to the issue that owns the
// residue (#462) rather than being absorbed here where it would be credited to
// an unrelated landing (PRINCIPLES 22).
//
// nil is passed as the [Context] because gate 1 has already excluded every
// context-dependent literal — unlike values, which parses each side under the
// context its own value constraint captured.
func (vs valueSpace) ValidDefault(r xsd.TypeResolver, t *xsd.SimpleType, vc xsd.ValueConstraint) (error, bool) {
	needs, err := needsContext(r, t)
	if err != nil || needs {
		//nolint:nilerr // the first result is the VERDICT's cause, not this call's error: a needsContext fault is a fault of the type, which gate 1 answers undecided and so causeless.
		return nil, false
	}
	_, err = ValidateLexical(vs.b, r, t, vc.LexicalForm(), nil, assertionsUndecided{})
	if err == nil {
		return nil, true
	}
	if !IsDatatypeVerdict(err) {
		return nil, false
	}
	return err, true
}

// ConstraintMatches reports whether lexical — an instance literal — and
// vc.{lexical form} denote the SAME ·actual value· of t: the "equal or identical"
// agreement cvc-attribute (§3.2.4.1) clause 4 and cvc-au (§3.5.4) both test
// against a fixed Value Constraint. decided is false wherever the question cannot
// be answered, on the same fail-open terms [ValidDefault] states, so a caller can
// never charge a violation off a fault of the type or of the backend.
//
// A ·special· t (xs:anySimpleType, xs:anyAtomicType) never reaches the pipeline:
// its lexical mapping is not a function but the union of every ·primitive·
// mapping and, for xs:anySimpleType alone, every list mapping (Datatypes
// §3.2.1.2, §3.2.2.2), so "1" and "1.0" may denote one decimal. Each member of
// that union answers the (same, decided) pair primitiveMatches documents, and
// ConstraintMatches answers:
//
//   - SAME where lexical and vc.{lexical form} are byte-identical, or where some
//     member maps both to values that are equal or identical;
//   - NOT-same where every member is decided and none says same;
//   - undecided otherwise — in particular where the backend does not map a
//     primitive, where a member's two values carry neither [Identical] nor [Eq],
//     and for QName and NOTATION where ctx is nil.
//
// Both literals are tested RAW, and no member's own whiteSpace runs first:
// Structures §3.1 gives xs:anySimpleType the preserve normalization, and
// neither type has a whiteSpace facet (§4.3.6). That breaks Parse's
// "normalized" precondition harmlessly — a Parse that over-admits a raw literal
// can only add a SAME or an undecided, never a NOT-same. The byte-identical
// shortcut is sound only for these types: for any other type one literal can
// denote two values, a QName under two sets of bindings (§3.3.18).
//
// GAP(value): narrowed primitive mappings (#2045). A Parse rejection with an
// *xsderr.Error is read as "not in the lexical space", which relies on each
// primitive's mapping covering its whole lexical space (see [Backend]). An
// [Override] that narrows a primitive rejects in-space literals the same way, so
// two literals only that primitive equates — "1" and "true" under a narrowed
// xs:boolean — answer NOT-same through specialMatches, and every reader of it
// charges on NOT-same: validate's walk.fixedAgreement and
// contentCheck.fixedActualValue through this function, and xsd's
// checkAttributeUseValueConstraint (au-props-correct clause 3),
// fixedValueConstraintSubsumes and attributeValueConstraintsAgree (loc-testSubP
// clauses 4.2 and 5.2.2) through [NewValueSpace]'s comparisons. Under such a
// backend this residue is fail-CLOSED at all five. xs:decimal, the [Override]
// example, is covered by float, double and precisionDecimal, which equate every
// pair of decimal literals that denote one value.
//
// For every other t, both sides are validated against t itself, so no
// shared-mapping search is needed (contrast values, which compares two
// DIFFERENT types' constraints): one type fixes one governing mapping and one
// whiteSpace mode, and running each side through the whole pipeline is what
// makes the comparison a VALUE-space one — String Valid (§3.16.4) clause 1
// normalizes, clause 2 maps, and only then is there an ·actual value· to
// compare. A lexical comparison in its place would report "1" and "01" as
// different xs:integers.
//
// The two sides are parsed under DIFFERENT contexts, and that asymmetry is the
// point: ctx is the instance's, resolving a QName lexical against the namespace
// bindings in scope where the attribute was written, while vc carries the bindings
// in scope where its own {lexical form} was written in the schema document
// (§3.3.18, [ConstraintContext]). One shared context would decide a QName agreement
// wrongly in both directions.
//
// a decides each {test} of an assertions facet the pipeline reaches on either
// side (cvc-assertions-valid via cvc-datatype-valid clause 3, Datatypes
// §4.1.4), on [ValidateLexical]'s terms, and MUST be non-nil — even for a
// ·special· t, which never reads it.
//
// For a t that is not ·special·, a side that fails to validate is undecided,
// never a mismatch: a {test} a FAILS on either side is such a failure like any
// other. For the instance side that is not a lost verdict: a literal that is
// not Datatype Valid against t — outside its lexical space, or failing one of
// its facets, assertions included — already fails cvc-attribute clause 3 or
// cvc-elt clause 5.2.1, which the caller charges in its own right, and
// reporting "not the same value" for what is really "not a value of t at all"
// would charge the agreement clause as well for one defect. For vc's side it is
// the schema's own cos-valid-simple-default obligation (§3.2.6.2), charged at
// finalize — except an assertions facet's: the ValidDefault of the
// [xsd.ValueSpace] [NewValueSpace] returns runs the pipeline with no evaluator
// of its own and declines every {test} (its gate 2's GAP(value)), so a
// vc.{lexical form} failing a {test} is charged by no assembly, and is
// undecided here rather than NOT-same all the same.
//
// GAP(xpath): a {test} a DECLINES ([IsAssertionDeclined]) is undecided too;
// which {test}s the evaluator validate passes declines is stated at
// xpath.FacetAssertions' own marker. This function's readers, validate's
// walk.fixedAgreement (cvc-attribute clause 4, cvc-au) and
// contentCheck.fixedActualValue (cvc-elt clause 5.2.2.2.2), charge only a
// decided NOT-same and decline an undecided answer, so the direction is
// fail-open. (#1042)
//
// GAP(value): a NOTATION vc.{lexical form} naming no declared notation, tracked
// by #667. ValidDefault's gate 1 (needsContext) refuses every NOTATION-governed
// default, so no finalize judges such a value, and it is undecided here, never
// NOT-same, until #667 routes those defaults through ValidDefault.
func ConstraintMatches(b Backend, r xsd.TypeResolver, t *xsd.SimpleType, lexical string, ctx Context, vc xsd.ValueConstraint, a AssertionEvaluator) (same, decided bool) {
	if t.IsSpecial() {
		return specialMatches(b, t, t, lexical, ctx, vc.LexicalForm(), ConstraintContext(vc), equalOrIdentical)
	}
	av, err := ValidateLexical(b, r, t, lexical, ctx, a)
	if err != nil {
		return false, false
	}
	cv, err := ValidateLexical(b, r, t, vc.LexicalForm(), ConstraintContext(vc), a)
	if err != nil {
		return false, false
	}
	return equalOrIdentical(av, cv)
}

// specialMatches decides a pair of raw literals x and y — x parsed under xctx,
// y under yctx — of the ·special· types tx and ty over their mapping union
// under rel: the byte-identical shortcut, then the scan of every ·primitive·
// member and, when tx and ty are BOTH xs:anySimpleType, every list member,
// folded in the order [ConstraintMatches] states — any member SAME is SAME,
// else any undecided member is undecided, else NOT-same. [ConstraintMatches]
// hands it t twice under equalOrIdentical; valueSpace.compare hands it both
// value constraints' types under either relation.
//
// The list members are scanned only where both types admit lists: a literal of
// xs:anyAtomicType denotes no list value (Datatypes §3.2.2.2), so a list member
// cannot equate it with anything.
//
// The byte-identical shortcut holds under both relations: xs:string is a member
// of the union and maps one literal to one value identical to itself.
func specialMatches(b Backend, tx, ty *xsd.SimpleType, x string, xctx Context, y string, yctx Context, rel relation) (same, decided bool) {
	if x == y {
		return true, true
	}
	same, decided = primitivesMatch(b, x, xctx, y, yctx, rel)
	if same || tx != xsd.AnySimpleType() || ty != xsd.AnySimpleType() {
		return same, decided
	}
	listSame, listDecided := listsMatch(b, x, xctx, y, yctx, rel)
	if listSame {
		return true, true
	}
	return false, decided && listDecided
}

// primitivesMatch scans every ·primitive· mapping (primitiveNames) over the raw
// literals x and y under rel, folded as specialMatches states.
func primitivesMatch(b Backend, x string, xctx Context, y string, yctx Context, rel relation) (same, decided bool) {
	decided = true
	for _, name := range primitiveNames {
		s, d := primitiveMatches(b, name, x, xctx, y, yctx, rel)
		if s {
			return true, true
		}
		decided = decided && d
	}
	return false, decided
}

// primitiveMatches is one primitive member's pair under rel. A literal the
// mapping's Parse rejects with an *xsderr.Error is outside that primitive's
// lexical space, so the member is decided not-same: for NOT-same, "not
// admitted" and "admitted, different" are one fact. Any other Parse error, a
// primitive the backend does not map, and a QName or NOTATION member without
// both contexts are undecided — a nil [Context] makes a mapping reject every
// such literal, which would read as a decided non-admission.
func primitiveMatches(b Backend, name xsd.QName, x string, xctx Context, y string, yctx Context, rel relation) (same, decided bool) {
	if (name == qnameName || name == notationName) && (xctx == nil || yctx == nil) {
		return false, false
	}
	m, ok := b.Mapping(name)
	if !ok {
		return false, false
	}
	xv, err := m.Parse(x, xctx)
	if err != nil {
		return false, isLexicalRejection(err)
	}
	yv, err := m.Parse(y, yctx)
	if err != nil {
		return false, isLexicalRejection(err)
	}
	return rel(xv, yv)
}

// isLexicalRejection reports whether a Parse error is the mapping's own
// rejection of the literal — an *xsderr.Error, per the [Mapping] contract —
// rather than a fault of the backend.
func isLexicalRejection(err error) bool {
	var e *xsderr.Error
	return errors.As(err, &e)
}

// listsMatch is the list members' pair under rel, for xs:anySimpleType alone.
// The most permissive list type has a union of every primitive as its item
// type, so each position is scanned by primitivesMatch on its own. Two lists
// differ when their lengths differ (Datatypes §2.2.1, §2.2.2) or either literal
// is no list literal, and otherwise when any position is decided not-same; a
// list of one item against an atomic value needs nothing here, primitivesMatch
// having already compared that single token.
func listsMatch(b Backend, x string, xctx Context, y string, yctx Context, rel relation) (same, decided bool) {
	xs, xok := listTokens(x)
	ys, yok := listTokens(y)
	if !xok || !yok || len(xs) != len(ys) {
		return false, true
	}
	decided = true
	for i := range xs {
		s, d := primitivesMatch(b, xs[i], xctx, ys[i], yctx, rel)
		if d && !s {
			return false, true
		}
		decided = decided && d
	}
	return decided, decided
}

// listTokens splits a raw literal into the item literals of a list (Datatypes
// §2.4.1.2, "space-separated ·literals·"): "" is the empty list, and a literal with a
// leading, trailing or doubled #x20 is no list literal, since no whiteSpace runs
// before a ·special· type's mapping (Structures §3.1).
func listTokens(lit string) ([]string, bool) {
	if lit == "" {
		return nil, true
	}
	tokens := strings.Split(lit, " ")
	for _, tok := range tokens {
		if tok == "" {
			return nil, false
		}
	}
	return tokens, true
}

// values maps both {lexical form}s to ·actual values· IN ONE VALUE SPACE, or
// reports ok=false when it cannot — compare's fail-open path for every pair that
// is not two ·special· types.
//
// The one space is the shared governing mapping of ta and tb (sharedMapping).
// Each side is whiteSpace-normalized under ITS OWN type's effective mode before
// being parsed, because normalization is the first stage of that type's lexical
// mapping (key-nv §3.1.4, cvc-simple-type §3.16.4) and a restriction may narrow
// the mode its base declared. A type with no mode in force (xs:anySimpleType, a
// union) yields the zero mode, which normalizeWhiteSpace refuses, so it is
// reported undecided instead.
//
// Each side is also parsed under ITS OWN [Context], built from the namespace
// bindings that side's value constraint captured where its {lexical form} was
// written (§3.3.18/§3.3.19, PRINCIPLES 19). The two literals may come from
// schema documents binding the same prefix differently, or different prefixes
// to one namespace, so a single shared context would decide the comparison
// wrongly in both directions.
//
// A Parse failure is undecided, never a verdict: a {lexical form} outside the
// type's lexical space is au-props-correct clause 2 / cos-valid-simple-default's
// business (§3.2.6.2), a separate obligation this adapter must not pre-empt by
// reporting "not the same value" for what is really "not a value at all".
//
// GAP(value): a QName or NOTATION prefix with no binding in the context its own
// constraint captured takes that same path and stays undecided. It needs no
// branch and no tri-state of its own — the mapping's Parse rejects the literal,
// and this function reports the rejection undecided. Nor is it a spec category:
// such a literal already fails Datatype Valid, so cos-valid-simple-default
// clause 1 rejects the schema before either comparison is asked anything. What
// it must not become is a NOT-same verdict, which every consumer of these two
// methods charges on — xsd's checkAttributeUseValueConstraint
// (valueconstraintvalid.go, au-props-correct clause 3),
// fixedValueConstraintSubsumes and attributeValueConstraintsAgree
// (defaultbinding.go, loc-testSubP clauses 4.2 and 5.2.2) each reject on
// decided-and-not-same and accept on undecided — so a wrong NOT-same here is a
// false schema rejection at all three.
func (vs valueSpace) values(r xsd.TypeResolver, ta *xsd.SimpleType, a xsd.ValueConstraint, tb *xsd.SimpleType, b xsd.ValueConstraint) (Value, Value, bool) {
	m, ok := vs.sharedMapping(r, ta, tb)
	if !ok {
		return nil, nil, false
	}
	aws, aerr := whiteSpaceInForce(r, ta)
	bws, berr := whiteSpaceInForce(r, tb)
	if aerr != nil || berr != nil {
		return nil, nil, false // an unresolvable base: undecided, never a verdict
	}
	if aws == 0 || bws == 0 {
		return nil, nil, false
	}
	av, err := m.Parse(normalizeWhiteSpace(a.LexicalForm(), aws), ConstraintContext(a))
	if err != nil {
		return nil, nil, false
	}
	bv, err := m.Parse(normalizeWhiteSpace(b.LexicalForm(), bws), ConstraintContext(b))
	if err != nil {
		return nil, nil, false
	}
	return av, bv, true
}

// ConstraintContext is the [Context] vc.{lexical form} is mapped under: the
// namespace bindings in scope at the schema-document element that wrote it
// (§3.3.18, fixed there by cos-valid-simple-default clause 2), never the
// instance's. vc comes from [xsd.ElementDeclaration.ValueConstraint],
// [xsd.AttributeDeclaration.ValueConstraint] or [xsd.AttributeUse.ValueConstraint],
// or, for a ·defaulted attribute·, from [xsd.Schema.EffectiveValueConstraint]. Pass
// it as [ValidateLexical]'s or [ValidatingType]'s ctx wherever the literal is a
// value constraint's {lexical form} supplied at assessment time — a ·defaulted
// attribute· (key-dflt-att) or an element default (cvc-elt clause 5.1.2). The
// prefix "xml" is always bound, to the XML namespace; the empty prefix resolves to
// vc's {default namespace} where it has one and to no namespace otherwise; every
// other prefix, "xmlns" included, resolves only through a binding vc carries, and
// no schema document can declare "xmlns" (Namespaces in XML §3). A vc that captured
// no bindings still yields a total, non-nil context.
func ConstraintContext(vc xsd.ValueConstraint) Context {
	ns, ok := vc.DefaultNamespace()
	return newNSContext(vc.NamespaceBindings(), ns, ok)
}

// sharedMapping resolves the ONE Mapping that governs both ta's and tb's values,
// or reports ok=false when no single mapping does. It is the correctness-critical
// half of this adapter, because the two constraints it serves compare values
// across DIFFERENT types: loc-testSubP clause 5.1 requires only that S's type be
// ·derived· from G's, so a general xs:decimal and a specific xs:integer routinely
// meet here.
//
// The widest-space rule (doc.go, backend.go) is what makes such a pair
// comparable at all: a derived type without its own mapping is governed by its
// nearest mapped ancestor's, so xs:integer and xs:byte both resolve to whatever
// governs them upward, and Datatypes §2.2.1's own note settles the semantics —
// "'+2', treated as a decimal, '+2', treated as an integer, and '+2', treated as
// a byte, all denote the same value. They are not only equal but identical."
//
// When the two sides resolve to DIFFERENT governing types the comparison is
// refused. That is not conservatism, it is §2.2.1/§2.2.2's own rule: "values
// from different ·primitive· datatypes' ·value spaces· are made artificially
// distinct", and a backend's derived mapping may in addition represent values in
// a narrower space of its own, where a value of the base space has no image.
// Parsing each side under its own mapping and comparing the results across the
// two would be exactly the corruption the widest-space rule forbids, and could
// report a spurious NOT-same — the one outcome the fail-open contract rules out.
//
// The list and union varieties are refused outright, on ta or tb: their governing
// mappings are synthesized per type (listMapping over the {item type definition},
// unionMapping over the {member type definitions}), so "the same mapping"
// is not a property two distinct nodes can be checked for by identity here, and
// no comparison is better than a guessed one.
//
// The two context-dependent primitives, QName and NOTATION, are NOT refused:
// each side's value constraint carries the namespace bindings in scope AT ITS
// LITERAL (§3.3.18/§3.3.19, PRINCIPLES 19), which values resolves each side's
// prefix under, so "a:x" and "b:x" compare SAME when both prefixes name one
// namespace and "p:x" compares NOT-same across documents that bind "p"
// differently — the two verdicts a lexical comparison gets wrong in opposite
// directions. The prefix that resolves to nothing is values' residual, marked
// there.
//
// An unresolvable {base type definition} on either side is refused: the
// widest-space walk cannot finish, so there is no shared mapping to name, and
// refusing is the fail-open answer this whole adapter owes.
func (vs valueSpace) sharedMapping(r xsd.TypeResolver, ta, tb *xsd.SimpleType) (Mapping, bool) {
	ga, ok, err := governingType(vs.b, r, ta)
	if err != nil || !ok {
		return Mapping{}, false
	}
	gb, ok, err := governingType(vs.b, r, tb)
	if err != nil || !ok {
		return Mapping{}, false
	}
	if ga != gb {
		return Mapping{}, false // incommensurable: two different value spaces
	}
	return vs.b.Mapping(ga.Name())
}

// governingType is the type node whose Mapping governs node's value space: node
// itself, or — under the widest-space rule, walked by the shared governingNode
// (facets.go) that governingMapping also reads — its nearest mapped ancestor.
// The NODE is returned rather than the Mapping because a Mapping is a struct of
// funcs and so cannot be compared; identity of the governing node is what tells
// two value spaces apart, and pointer identity is the right test because
// xsd.SimpleType's whole contract is that one node per type is shared across a
// compiled schema (simpletype.go).
//
// Only the atomic {variety} resolves: see sharedMapping for why list and union
// are refused.
func governingType(b Backend, r xsd.TypeResolver, node *xsd.SimpleType) (*xsd.SimpleType, bool, error) {
	variety, err := node.Variety(r)
	if err != nil {
		return nil, false, err
	}
	if _, ok := variety.(xsd.Atomic); !ok {
		return nil, false, nil
	}
	return governingNode(b, r, node)
}

// contextDependent reports whether t's {primitive type definition} is QName or
// NOTATION — the two primitives whose lexical mapping needs a [Context] (§3.3.18,
// §3.3.19). An absent {primitive type definition} (xs:anyAtomicType alone,
// §3.16.1) is not one of them.
func contextDependent(r xsd.TypeResolver, t *xsd.SimpleType) (bool, error) {
	variety, err := t.Variety(r)
	if err != nil {
		return false, err
	}
	if _, ok := variety.(xsd.Atomic); !ok {
		return false, nil
	}
	p, err := t.Primitive(r)
	if err != nil || p == nil {
		return false, err
	}
	return p.Name() == qnameName || p.Name() == notationName, nil
}

// needsContext reports whether t's value space is governed, anywhere in its
// {item type definition}/{member type definitions} closure, by QName or NOTATION
// — the recursive form of contextDependent (above).
//
// The closure form is needed because ValidDefault, unlike Identical and
// EqualOrIdentical, does NOT refuse the list and union varieties: it needs one
// type's mapping rather than a shared one across two, so ValidateLexical's own
// list/union dispatch (facets.go, union.go) decides them, and a list OF QName or
// a union WITH a QName member would otherwise reach a backend with the nil
// Context ValidDefault passes and be rejected for want of bindings it never
// threaded — the value constraint carries them, but the dispatch does not route
// them (gate 1). governingMapping recurses the same closure for the same
// reason (facets.go). For a union, ANY context-dependent member makes the whole
// union undecided: dispatch takes the first member that accepts, so a member
// that could only be decided WITH context could change the verdict.
//
// The nil guards on Item and each member are belt-and-braces over a state no
// component can be in, NOT a reachable one: the ListDerivation.Item and
// UnionDerivation.Members slots are xsd.SimpleTypeOrRefs that admit no encoding
// of absence, rejected by xsd.NewSimpleType at construction, and an unresolvable
// by-name one is an error out of Item/Members rather than a nil. They stay
// because everything below them is nil-hostile too — the recursive Variety()
// call they gate, and governingMapping on the very next line — so removing them
// would trade a documented impossibility for a crash rather than for a verdict.
//
// No visited set is needed (PRINCIPLES 9), on two arguments that are no longer
// one. The ITEM edge cannot close a loop that reaches here: any cycle of
// itemType= references contains a list whose item is a list, which
// xsd.SimpleType.CheckDerivation rejects under cos-st-restricts clause 2.1
// before a schema finalizes. The MEMBER edge rests on xsd's finalize-time
// checkUnionMembershipAcyclic, which charges cos-st-restricts clause 3.3
// (no-self-membership) against the assembled membership graph — see
// xsd/derivation.go's CheckDerivation clause 3.3 paragraph. A caller reaching
// here with components that never went through xsd.Finalize owes that
// acyclicity itself.
func needsContext(r xsd.TypeResolver, t *xsd.SimpleType) (bool, error) {
	variety, err := t.Variety(r)
	if err != nil {
		return false, err
	}
	switch variety.(type) {
	case xsd.Atomic:
		return contextDependent(r, t)
	case xsd.List:
		item, err := t.Item(r)
		if err != nil || item == nil {
			return false, err
		}
		return needsContext(r, item)
	case xsd.Union:
		members, err := t.Members(r)
		if err != nil {
			return false, err
		}
		for _, m := range members {
			if m == nil {
				continue
			}
			needs, err := needsContext(r, m)
			if err != nil || needs {
				return needs, err
			}
		}
	}
	return false, nil
}

// identical reports Datatypes §2.2.1's identity relation over two values, with
// decided=false when neither relation is available on a.
//
// A value that implements [Identical] answers directly: that capability exists
// precisely for the value types whose identity DIFFERS from their equality
// (float/double, where +0 = −0 but the two are not identical and NaN is
// identical to itself while equal to nothing; dateTime and duration, equal but
// not identical across remembered timezone offsets). For every other value
// §2.2.2 settles it — "the equality relation for most datatypes IS the identity
// relation" — so order equality answers.
//
// That fallback is sound for STRICT identity only because of the obligation
// [Identical]'s own doc states: a value type whose identity differs from its
// equality MUST implement [Identical], so an [Eq]-only value has declared the
// two relations to be one. It is NOT sound merely by analogy with enumeration
// matching, which tests the equal-or-identical UNION (equalOrIdentical) and so
// can absorb an Eq answer in the union direction regardless.
//
// This is deliberately NOT equalOrIdentical: for a value that DOES distinguish
// the two, au-props-correct clause 3 says "identical", so an equal-but-not-
// identical pair must answer (false, true) here and (true, true) there.
func identical(a, b Value) (same, decided bool) {
	if id, ok := a.(Identical); ok {
		return id.Identical(b), true
	}
	if eq, ok := a.(Eq); ok {
		return eq.Eq(b), true
	}
	return false, false
}

// equalOrIdentical reports the §2.2.1/§2.2.2 "equal or identical" union — "all
// comparisons for 'sameness' prescribed by this specification test for either
// equality or identity, not for identity alone" (§2.2.2) — with decided=false
// when a carries NEITHER relation, so a caller that must distinguish "not the
// same" from "not comparable" can.
//
// It is the ONE encoding of that union in this package (STYLE T4): enumeration
// matching (cvc-enumeration-valid §4.3.5.4, enumMatch), the Structures
// value-constraint clauses (loc-testSubP 4.2/5.2.2, via
// valueSpace.EqualOrIdentical) and the instance-time fixed-value agreement
// (cvc-attribute cl.4 / cvc-au, via [ConstraintMatches]) all read it, and none
// re-derives it.
func equalOrIdentical(a, b Value) (same, decided bool) {
	id, hasIdentical := a.(Identical)
	if hasIdentical && id.Identical(b) {
		return true, true
	}
	eq, hasEq := a.(Eq)
	if hasEq && eq.Eq(b) {
		return true, true
	}
	return false, hasIdentical || hasEq
}
