// Package value defines the value-space contracts: how typed values are
// represented, compared, and produced from lexical forms — without fixing
// any particular Go representation.
//
// # Values are open
//
// [Value] is an alias for any: deliberately not a sealed interface
// (PRINCIPLES 2), so users bring their own backends and value types. What a
// value can do is discovered through small capability interfaces (STYLE T2),
// never a type switch over concrete types: [Eq] and [Ordered] for comparison
// (XSD value spaces are partially ordered — see [Ordering]), [Identical] for
// the identity relation enumeration matching needs, [Lengthed], [DigitCounted],
// [Scaled], [TimezoneAware] for the facets that measure a value,
// [Canonical] for value → canonical lexical, and [Listed] for the items of
// a list-variety value, which this package's own list mapping produces.
//
// # Backends
//
// A [Backend] supplies lexical↔value mappings for builtin types. It MUST cover
// the primitives (directly or via composition); it MAY also map derived
// builtins to give them their own, typically narrower, representation — derived
// types without their own mapping inherit the nearest mapped ancestor's (see
// package builtin); [GoverningMapping] names that mapping for an atomic type.
// QName and NOTATION need in-scope namespace bindings at parse time, so a
// mapping's Parse takes a [Context] (PRINCIPLES 19). [Override] composes
// backends per type — partial's mappings where defined, base otherwise — so a
// program can back only xs:decimal with a money type and keep the rest.
//
// # The widest-space rule (facet checks under derived mappings)
//
// A derived type's own [Mapping] governs the VALUE the application receives —
// never the space in which inherited facet checks run. A derived representation
// is allowed to be narrower than its base's value space; using it for
// base-chain semantics would corrupt them (overflow where the base space has
// none, collapsed precision, different ordering). So:
//
//   - enumeration and bound facets, wherever they sit on the derivation
//     chain, are compared in the value space of the type that DECLARES the
//     facet, parsed by that type's governing mapping (its own, or its nearest
//     mapped ancestor's — ultimately the primitive's, which is always the
//     widest);
//   - schema-build restriction checks (a derived facet must narrow the
//     base's) always run in the base's space;
//   - only after the wider-space checks pass is the derived mapping's Parse
//     used to produce the application-facing value; a lexical the checks
//     accept but the narrow representation cannot hold is a mapping error on
//     the derived type, reported as such — never a false validity verdict.
//
// Comparison and facet capabilities are NOT backend methods; they live on the
// values a Mapping produces. A backend's values must implement the capabilities
// its types' applicable facets require ([Ordered] for bounded types,
// [DigitCounted] for digit facets, [Scaled] for precisionDecimal, …) —
// value/backendtest verifies this mechanically.
//
// # The facet pipeline
//
// Validation of a literal is a fixed stage sequence (ARCHITECTURE.md):
// whiteSpace → pattern facets → lexical mapping → value facets → assertions.
// [LexicalFacet] is a stage that checks the normalized lexical form;
// [ValueFacet] is a stage that checks the parsed value. Every stage failure is
// an *xsderr.Error carrying the facet's rule ID and the pipeline stage that
// rejected. [ApplyWhiteSpace] is the whiteSpace stage alone, for a caller that
// needs the normalized lexical and no value.
//
// Not every error the pipeline returns is a VERDICT, though, and the difference is
// load-bearing. [IsDatatypeVerdict] is the test, and every caller deciding validity
// — inside this package and out — applies it before charging: an error it reports
// false for is a fault of the TYPE or of the BACKEND, and charging one as "this
// literal is invalid" turns a backend gap or a construction bug into a false
// reject. The rule ID does not tell them apart; an ungoverned type is reported
// under cvc-datatype-valid exactly as a genuine rejection is, which is how a
// typeless attribute (xs:anySimpleType, Structures §3.2.2.2) would otherwise be
// rejected by every processor that trusted the ID.
//
// The last stage is the one this package cannot run itself: an assertions
// facet's {test}s are XPath 2.0 (cvc-assertions-valid, §4.3.13.3), and package
// xpath imports this one. [ValidateLexical] and [ValidatingType] therefore take
// an [AssertionEvaluator], which answers each {test} with an [AssertionOutcome]:
// a failed one is an ordinary verdict under cvc-assertions-valid, and a declined
// one is the third non-verdict, which [IsAssertionDeclined] reports and
// [IsDatatypeVerdict] excludes. GAP(value): this package's own callers of the
// pipeline that are handed no evaluator — [CheckFacetRestriction] and the
// [xsd.ValueSpace] [NewValueSpace] returns — decline every {test}, each
// fail-open on the terms its own marker states. (#1042)
//
// One member of that class has a predicate of its own, because two sites need to
// know WHICH fault: a type may reach [ValidateLexical] carrying a facet that is not
// applicable to it at all (cos-applicable-facets §4.1.5) — a bound facet on an
// unordered value space, a length facet on an unmeasurable one — or with no usable
// whiteSpace mode where §3.16.7.4 guarantees one. Those are distinguished by
// [IsFacetPrecondition] and charged to cos-applicable-facets or
// [xsderr.RuleComponentInvariant] rather than to any cvc-* rule. Applicability
// itself is the xsd.SimpleTypeRestrictionChecker capability's to enforce —
// builtin.NewRestrictionChecker is the implementation, and a Schema finalized with
// it installed has the whole class discharged for every simple type it reaches.
//
// The list and union varieties recurse rather than adding stages. A list runs
// this whole pipeline against the ITEM TYPE per whitespace-delimited item — the
// item type's own facets included, which is what makes a list of a derived type
// (a list of xs:byte, whose mapping is xs:decimal's) reject an out-of-range item
// — and then its own facets over the resulting sequence. A union has no
// whiteSpace facet at all: it hands the RAW literal to its member types in
// order, and the first one that is itself datatype-valid supplies the value, the
// whiteSpace normalization its own pattern facet then matches against, and the
// type its own assertions facet binds `$value` under (dt-xdmrep clause 4): that
// ·active basic member·, never the union, is the st the evaluator is handed —
// so a union's value is always some member's value, never a wrapper of its own.
// [ValidatingType] names that member (key-vtype §3.16.4 cl.1, the ·validating
// type·): st itself, or the ·active basic member· a union dispatched to. Its
// own member-identification scan applies a WIDER fault test than the dispatch
// above does — it declines on any member error that is not [IsDatatypeVerdict],
// where the dispatch folds anything short of [IsFacetPrecondition] or
// [IsAssertionDeclined] into "rejected" and keeps scanning (dv_union's own #462
// gap) — so the two can disagree on which members fault; ValidatingType's doc
// comment is the authority on why. Both run each member's assertions facets, so
// a member one of whose {test}s fails is passed over for a later one
// (dt-active-member).
//
// A facet's OWN {value} goes through the same whiteSpace normalization before it
// is parsed, once at construction: a facet's {value} property is "a value from
// the value space of the {base type definition}" (§4.3.7.1 f-mai-value, §4.3.5.1),
// and reaching that value space runs the base's lexical mapping, whose first
// stage is its whiteSpace normalization (key-vv §3.1.3, key-nv §3.1.4). So
// `<maxInclusive value=" 9 "/>` on a collapse-normalized base denotes exactly
// what the untrailed spelling denotes, on both the validation and the
// restriction-checking path.
//
// # Construction-time facet checks
//
//	func CheckFacetRestriction(b Backend, r xsd.TypeResolver, t *xsd.SimpleType) error
//	func CheckPatternSyntax(t *xsd.SimpleType) error
//
// [CheckPatternSyntax] charges src-pattern-value (§4.3.4.3) on the <pattern>
// facets a type DECLARES: each value must be a regular expression per Datatypes
// Appendix G. The facet pipeline charges the same rule when it compiles those
// patterns, but only for a type some literal is validated against — so a schema
// nobody validates an instance against was never checked at all. It takes
// neither a [Backend] nor a resolver: a pattern's syntax is a property of the
// lexical value alone.
//
// [CheckFacetRestriction] is the once-per-type counterpart of that pipeline: it
// charges the value-space Schema Component Constraints relating a simple type's
// own bound and enumeration facets to its {base type definition}
// (maxInclusive/maxExclusive/minInclusive/minExclusive valid restriction
// §4.3.7.4–§4.3.10.4, enumeration valid restriction §4.3.5.5) — the half of
// cos-st-restricts that package xsd, a pure leaf with no value spaces, cannot
// reach — and, from the same four sections, the opposite-bound constraints
// relating a lower bound facet to an upper bound facet declared at the SAME
// derivation step (minInclusive <= maxInclusive, minExclusive <= maxExclusive,
// minExclusive < maxInclusive, minInclusive < maxExclusive), which are
// standalone §4.3 SCCs rather than a clause of cos-st-restricts. The two
// same-step bound SCCs that compare no {value}, maxInclusive-maxExclusive and
// minInclusive-minExclusive, are xsd.SimpleType.CheckDerivation's.
//
// Reach BOTH through builtin.NewRestrictionChecker, the
// xsd.SimpleTypeRestrictionChecker installed at xsd.SchemaBuilder.FinalizeWith:
// that implementation delegates pattern syntax first, then charges facet
// APPLICABILITY, then delegates the restriction constraints here. A schema
// finalized without it gets none of the three.
//
// # The resolver, threaded and never stored
//
// A Simple Type Definition's {base type definition}, {item type definition} and
// each of its {member type definitions} may be a DEFERRED reference by name
// (xsd.SimpleTypeOrRef), so every read of a derived property — {variety},
// {primitive type definition}, {item type definition}, {facets}, the governing
// mapping under the widest-space rule — is a walk that needs an
// [xsd.TypeResolver] to follow. [ValidateLexical] and [CheckFacetRestriction]
// therefore take one as a PARAMETER, and this package stores one NOWHERE: not on
// a [Backend], not on a compiled facet checker, not on the xsd.ValueSpace
// [NewValueSpace] returns. One backend and one value space consequently serve
// every schema, which is what the capability seams at
// xsd.SchemaBuilder.FinalizeWith assume. A caller validating against a
// Schema-less graph — one assembled entirely from live components, as
// builtin.Seed produces — passes a resolver that resolves nothing, which is total
// there because such a graph holds no by-name reference to look up.
// [ValidateLexical] and [ValidatingType] also read the resolver as the current
// schema's {notation declarations}, NOTATION's value space (§3.3.19), through a
// Notations method as *xsd.Schema has, and so does [CheckFacetRestriction]
// through them; against a resolver without one that value space cannot be
// judged, so a NOTATION value is held only to its mapping and facets and never
// rejected as undeclared.
//
// An UNRESOLVABLE reference surfaces as the src-resolve error xsd.SimpleType.Base
// and its siblings produce. It is not a validity verdict about the literal — it says
// the TYPE could not be read at all — so [ValidateLexical] returns it marked, and
// [IsDatatypeVerdict] reports false for it.
//
// # Value-constraint validity and comparison (the xsd.ValueSpace seam)
//
//	func NewValueSpace(b Backend) xsd.ValueSpace
//	func ConstraintMatches(b Backend, r xsd.TypeResolver, t *xsd.SimpleType, lexical string, ctx Context, vc xsd.ValueConstraint, a AssertionEvaluator) (same, decided bool)
//	func ConstraintContext(vc xsd.ValueConstraint) Context
//
// [ConstraintMatches] is the INSTANCE-time half, and is not part of the
// xsd.ValueSpace interface: an instance literal is not a Value Constraint, and the
// rule reading the answer (cvc-attribute §3.2.4.1 clause 4, cvc-au §3.5.4) belongs
// to the validator, not to schema assembly. It maps both an instance literal and a
// fixed constraint's {lexical form} through one type's pipeline — each under its
// own namespace context, the instance's and the schema document's, and each
// through the caller's [AssertionEvaluator] at the assertions stage — and compares
// the ·actual values· under the same equal-or-identical union, answering undecided
// on the same fail-open terms as everything below; a ·special· type
// (xs:anySimpleType, xs:anyAtomicType) skips the pipeline and is decided over its
// mapping union, as [ConstraintMatches] states.
//
// [ConstraintContext] is that schema document's context for one Value
// Constraint, the one every comparison here maps a {lexical form} under. It is
// exported for the validator, which maps a default's {lexical form} itself
// wherever the instance takes one on — a ·defaulted attribute· (cvc-complex-type
// §3.4.4.2 clause 4) or an element default (cvc-elt §3.3.4.3 clause 5.1.2) — and
// must never map it under the instance's bindings.
//
// [NewValueSpace] is what lets package xsd — a pure leaf that cannot import this
// one — decide the Structures constraints that reach into a value space. Two
// COMPARE two Value Constraints' {value}s: au-props-correct (§3.5.6) clause 3
// under the identity relation (§2.2.1), loc-testSubP (§3.4.6.4) clauses 4.2 and
// 5.2.2 under the equal-or-identical union (§2.2.2). It maps each side's {lexical
// form} through the governing mapping of the type that constrains it and compares
// the values with the [Identical]/[Eq] capabilities they carry. A pair of two
// ·special· types has no governing mapping and is decided over their mapping
// union instead, as [ConstraintMatches] decides one, under the comparison's own
// relation.
//
// The other two VALIDATE one Value Constraint against one type: a-props-correct
// (§3.2.6.1) clause 2 and au-props-correct clause 2, both charging Simple Default
// Valid (§3.2.6.2), which is Datatype Valid (§4.1.4) and so [ValidateLexical]'s
// job. Being one-sided it needs no shared mapping, so it decides the list and
// union varieties the comparisons refuse.
//
// It answers "undecided" — never a verdict — for everything it cannot decide: an
// ungoverned type (on the one-sided check, the ·special· xs:anySimpleType and
// xs:anyAtomicType included, for which Datatype Valid is unconditionally true;
// on the comparisons, a ·special· type paired with an ordinary one, and a member
// of the mapping union that cannot answer), an unmappable lexical, the
// context-dependent QName and NOTATION spaces on the one-sided check (the
// comparisons resolve them, under the bindings each value constraint captured,
// and answer undecided only for a prefix that resolves to nothing), a
// construction-stage failure in the type's own facets, a facet-pipeline
// precondition fault (the one-sided check reads the whole class through
// [IsDatatypeVerdict]), the list and union varieties on the comparisons, and any
// pair whose two types resolve to DIFFERENT governing mappings (the widest-space
// rule above is what makes a general/specific pair on one base chain comparable at
// all, and what makes anything else incommensurable). Undecided always accepts on
// the xsd side, so this seam can only narrow what a schema set admits.
//
// # Codegen seam
//
// A backend MAY, in a later milestone, implement an Emitter (API frozen in M9;
// not yet declared here) to contribute specialized decode/encode Go source at
// codegen time — parsing straight from the reader's byte window into the target
// field, facet checks inlined, no boxed Value. A backend without an Emitter
// falls back to the runtime Mapping path for its types. Both paths must produce
// identical values and identical error rule IDs; codec's differential tests
// enforce that.
package value
