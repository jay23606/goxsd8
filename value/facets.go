package value

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"github.com/kud360/goxsd8/regex"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// This file drives the backend-generic facet pipeline: the pattern (lexical)
// and value-facet stages that run after the whiteSpace stage (whitespace.go)
// over an atomic type's effective facets. The fixed stage sequence (doc.go "The
// facet pipeline", ARCHITECTURE.md) is:
//
//	whiteSpace → pattern (lexical) → lexical mapping (Parse) → value facets → assertions
//
// The assertions stage is assertions.go's, run through the caller's
// [AssertionEvaluator].
//
// Pattern operates on the whiteSpace-normalized LEXICAL literal
// (cvc-pattern-valid, §4.3.4.4); enumeration/bounds/digits/length operate on
// the parsed VALUE (cvc-enumeration-valid §4.3.5.4, cvc-*Inclusive/*Exclusive
// §4.3.7–4.3.10, cvc-totalDigits/fractionDigits §4.3.11–4.3.12,
// cvc-length/minLength/maxLength §4.3.1–4.3.3). Each stage failure carries the
// SPECIFIC per-facet rule ID, never the cvc-facet-valid umbrella (§4.1.4).
//
// Compile-time assertions that the concrete checkers satisfy the pipeline-stage
// interfaces (backend.go): compile builds []LexicalFacet and []ValueFacet and
// ValidateLexical ranges over them polymorphically, so the interface
// satisfaction has real call sites.
var (
	_ LexicalFacet = patternFacet{}
	_ ValueFacet   = enumFacet{}
	_ ValueFacet   = boundFacet{}
	_ ValueFacet   = digitsFacet{}
	_ ValueFacet   = lengthFacet{}
	_ ValueFacet   = explicitTimezoneFacet{}
	_ ValueFacet   = scaleFacet{}
)

// The rules this file charges: the per-facet Validation Rules of Datatypes §4.3
// (one specific cvc-* ID per facet kind, never the cvc-facet-valid umbrella —
// see the pipeline comment above), plus the two Schema Representation
// Constraints that reject a malformed facet {value} at schema construction. Each
// string is a live entry in xsderr's generated catalog.
const (
	// ruleCvcDatatypeValid is Datatype Valid (Datatypes §4.1.4,
	// id="cvc-datatype-valid"): the umbrella instance-validity rule for "this
	// literal is not in the datatype's lexical/value space". This file charges it
	// only where no facet is implicated at all — a type no backend mapping
	// governs — and union.go charges it for a literal no member type accepts.
	ruleCvcDatatypeValid xsderr.Rule = "cvc-datatype-valid"
	// ruleSrcPatternValue is the Schema Representation Constraint on a pattern
	// facet's {value} (§4.3.4.3, id="src-pattern-value"): the value must be a
	// valid regular expression. Charged from two places, for two different
	// reaches: [CheckPatternSyntax], eagerly on every simple type the finalize
	// walk visits, which is the charge a schema-only caller gets; and
	// newPatternFacet below, lazily, when a type's facets are first compiled to
	// validate a literal, which additionally catches what the finalize charge
	// deliberately passes over (see regex.CheckSyntax).
	ruleSrcPatternValue xsderr.Rule = "src-pattern-value"
	// ruleCvcPatternValid is pattern Valid (§4.3.4.4, id="cvc-pattern-valid"): the
	// whiteSpace-normalized literal must match at least one member of the pattern
	// facet's {value} (the members are ORed, §4.3.4.4).
	ruleCvcPatternValid xsderr.Rule = "cvc-pattern-valid"
	// ruleSrcEnumerationValue is the Schema Representation Constraint on an
	// enumeration facet's {value} (§4.3.5.3, id="src-enumeration-value"): every
	// member must be in the value space of the type that declares the facet. A
	// member that fails to parse makes the SCHEMA malformed, so the bare
	// cvc-datatype-valid the lexical mapping returns is remapped to this rule.
	ruleSrcEnumerationValue xsderr.Rule = "src-enumeration-value"
	// ruleCvcEnumerationValid is enumeration Valid (§4.3.5.4,
	// id="cvc-enumeration-valid"): the candidate must be equal or identical to one
	// of the values in the enumeration facet's {value}.
	ruleCvcEnumerationValid xsderr.Rule = "cvc-enumeration-valid"
	// ruleCvcExplicitTimezoneValid is explicitTimezone Valid (§4.3.14.3,
	// id="cvc-explicitTimezone-valid"): a value's timezone presence must match the
	// facet's required/prohibited/optional {value}. The same ID also carries this
	// package's rejection of a malformed facet {value} — a token outside those
	// three (§4.3.14.1) — since the facet is unusable either way.
	ruleCvcExplicitTimezoneValid xsderr.Rule = "cvc-explicitTimezone-valid"
	// ruleCvcLengthValid is Length Valid (§4.3.1.3, id="cvc-length-valid").
	ruleCvcLengthValid xsderr.Rule = "cvc-length-valid"
	// ruleCvcMinLengthValid is minLength Valid (§4.3.2.3, id="cvc-minLength-valid").
	ruleCvcMinLengthValid xsderr.Rule = "cvc-minLength-valid"
	// ruleCvcMaxLengthValid is maxLength Valid (§4.3.3.3, id="cvc-maxLength-valid").
	ruleCvcMaxLengthValid xsderr.Rule = "cvc-maxLength-valid"
	// ruleCvcMaxInclusiveValid is maxInclusive Valid (§4.3.7.3,
	// id="cvc-maxInclusive-valid").
	ruleCvcMaxInclusiveValid xsderr.Rule = "cvc-maxInclusive-valid"
	// ruleCvcMaxExclusiveValid is maxExclusive Valid (§4.3.8.3,
	// id="cvc-maxExclusive-valid").
	ruleCvcMaxExclusiveValid xsderr.Rule = "cvc-maxExclusive-valid"
	// ruleCvcMinExclusiveValid is minExclusive Valid (§4.3.9.3,
	// id="cvc-minExclusive-valid").
	ruleCvcMinExclusiveValid xsderr.Rule = "cvc-minExclusive-valid"
	// ruleCvcMinInclusiveValid is minInclusive Valid (§4.3.10.3,
	// id="cvc-minInclusive-valid").
	ruleCvcMinInclusiveValid xsderr.Rule = "cvc-minInclusive-valid"
	// ruleCvcTotalDigitsValid is totalDigits Valid (§4.3.11.3,
	// id="cvc-totalDigits-valid").
	ruleCvcTotalDigitsValid xsderr.Rule = "cvc-totalDigits-valid"
	// ruleCvcFractionDigitsValid is fractionDigits Valid (§4.3.12.3,
	// id="cvc-fractionDigits-valid").
	ruleCvcFractionDigitsValid xsderr.Rule = "cvc-fractionDigits-valid"
	// ruleCvcMaxScaleValid is maxScale Valid (xsd-precisionDecimal.md §4.2.3,
	// id="cvc-maxScale-valid") — precisionDecimal-only (§3.3).
	ruleCvcMaxScaleValid xsderr.Rule = "cvc-maxScale-valid"
	// ruleCvcMinScaleValid is minScale Valid (xsd-precisionDecimal.md §4.3.3,
	// id="cvc-minScale-valid") — precisionDecimal-only (§3.3).
	ruleCvcMinScaleValid xsderr.Rule = "cvc-minScale-valid"
	// ruleCosApplicableFacets is Applicable Facets (§4.1.5,
	// id="cos-applicable-facets"): the Schema Component Constraint fixing which
	// ·constraining facets· "are allowed to be members of" a type's {facets}, keyed
	// on its {variety} and — for the atomic variety — its {primitive type
	// definition}. It is a Schema Component Constraint, not a Validation Rule, so it
	// is the ONE rule in this file that never decides an instance: it is charged
	// where the pipeline meets a facet paired with a value lacking the capability
	// that facet needs, which is precisely a facet sitting in {facets} outside the
	// set §4.1.5 permits (facetPrecondition, ValidateLexical).
	ruleCosApplicableFacets xsderr.Rule = "cos-applicable-facets"
)

// errTypeFault is the sentinel every [ValidateLexical] error that is NOT a
// verdict about the literal wraps: a fault of the *[xsd.SimpleType] it was
// handed, or of the [Backend] behind it. [IsDatatypeVerdict] is the one way to
// test it, and typeFault the one way to mark an error with it.
var errTypeFault = errors.New("value: a fault of the type or the backend, not a verdict about the lexical")

// errFacetPrecondition is the sentinel every facet-pipeline PRECONDITION fault
// wraps. The call sites that must tell such a fault apart from a validity verdict
// (dispatchUnion, checkEnumerationRestriction) then test ONE fact instead of
// matching a message or enumerating the two rule IDs the cohort spans —
// cos-applicable-facets and xsderr.RuleComponentInvariant. It stays unexported,
// reachable only through IsFacetPrecondition, so no consumer can reassign the
// sentinel out from under those decisions.
//
// It WRAPS errTypeFault rather than sitting beside it: a precondition fault is one
// kind of type fault, so the narrow predicate and the wide one read one chain and
// the classification has a single encoding (STYLE T4/D3).
var errFacetPrecondition = fmt.Errorf("%w: facet-pipeline precondition violated", errTypeFault)

// typeFault marks err as a fault of the type or the backend rather than a verdict
// about the literal, so [IsDatatypeVerdict] answers false for it. It is the ONE
// marking site besides facetPrecondition, which marks its own cohort through
// errFacetPrecondition, and it is idempotent: an error already marked — an item
// type's fault surfacing through listMapping.Parse, say — passes through
// unchanged rather than accumulating prefixes.
//
// It wraps rather than rebuilds, so xsderr.RuleOf and xsderr.LocOf still reach the
// *xsderr.Error underneath where there is one.
func typeFault(err error) error {
	if err == nil || errors.Is(err, errTypeFault) {
		return err
	}
	return fmt.Errorf("%w: %w", errTypeFault, err)
}

// facetPrecondition builds the *xsderr.Error for a facet-pipeline PRECONDITION
// fault: rule attributes it (ruleCosApplicableFacets for a facet paired with an
// incapable value, xsderr.RuleComponentInvariant for the whiteSpace representation
// invariant), and errFacetPrecondition, set as its Err, makes it discriminable
// through IsFacetPrecondition without any caller parsing a message. The sentinel
// rides the chain only: Msg is the formatted fact alone, opening with the
// offending item, never with the sentinel's text (STYLE E5), which xsderr.Wrap
// would copy in.
//
// It is the ONE construction site of the class (STYLE T4), which is also what keeps
// the cohort greppable now that the sites no longer share a marker STRING: `grep
// facetPrecondition(` enumerates every one of them.
//
// loc locates the component at fault wherever the site holds it: newBoundFacet and
// effectiveWhiteSpace both run at COMPILE time with st in hand, and both pass
// st.Loc(). The five sites inside a ValueFacet's CheckValue pass the zero Loc
// instead, and deliberately — a compiled checker holds the facet's own {value} and
// its kind, never a reference back to the *xsd.SimpleType it was built for, and no
// caller re-supplies one — so five of this class's seven members ship
// location-less. That is stated rather than left silent (STYLE E3), because the
// class is ABOUT a component: the fix is to carry the declaring type's Loc on each
// checker, not to invent one at the check site.
func facetPrecondition(rule xsderr.Rule, loc xsderr.Loc, format string, args ...any) *xsderr.Error {
	return &xsderr.Error{Rule: rule, Loc: loc, Msg: fmt.Sprintf(format, args...), Err: errFacetPrecondition}
}

// IsFacetPrecondition reports whether err is a facet-pipeline PRECONDITION fault
// — a fault in the *[xsd.SimpleType] handed to [ValidateLexical], not a verdict
// about the literal validated against it. [ValidateLexical] enumerates the exact
// states that produce one and why each is the caller's rather than the schema's.
//
// Test it before reading any [ValidateLexical] error as a validity verdict: a
// caller that charges such a fault as "this literal is invalid" turns its own
// construction bug into a false rejection of a valid schema or instance. The
// [xsderr.Rule] the error carries says which fault it is (cos-applicable-facets or
// [xsderr.RuleComponentInvariant], reachable through [xsderr.RuleOf]); this
// predicate says only that it is one of them, which is the question every caller
// deciding validity actually has.
func IsFacetPrecondition(err error) bool {
	return errors.Is(err, errFacetPrecondition)
}

// IsDatatypeVerdict reports whether err — an error [ValidateLexical] returned —
// is a VERDICT about the literal it was handed: the literal is not Datatype Valid
// (Datatypes §4.1.4) with respect to that type. It is false for a nil error,
// false for an assertions-facet decline ([IsAssertionDeclined]: the
// [AssertionEvaluator] did not decide a {test}, so the verdict is undecided),
// and false for every error that is instead a fault of the *[xsd.SimpleType] or
// of the [Backend]:
//
//   - no [Backend] mapping governs the type, its list's item type, or one of its
//     union's members. This is how xs:anySimpleType and xs:anyAtomicType arrive:
//     no backend maps the ·special· datatypes (§4.1), for which Datatype Valid is
//     unconditionally TRUE, and Structures §3.2.2.2's third tier types every
//     attribute declaration with no @type as xs:anySimpleType — so reading that
//     error as a verdict rejects every typeless attribute in existence.
//   - a construction-stage failure in the type's OWN facets: a pattern
//     [regex.Translate] cannot express (src-pattern-value), an enumeration or
//     bound facet whose DECLARING type the backend does not map (charged under
//     cvc-enumeration-valid or the bound's own cvc-*-valid rule, which cannot
//     be decided without it). Neither says anything about the literal.
//   - a facet-pipeline precondition fault ([IsFacetPrecondition], the narrower
//     question "which fault"), which would report the same error for EVERY
//     literal.
//   - an unresolvable {base type definition}, {item type definition} or member
//     anywhere the pipeline had to read: the TYPE could not be read at all.
//
// Test it before charging any [ValidateLexical] error as a validity violation. A
// caller that charges a type fault as "this literal is invalid" turns a backend
// gap or its own construction bug into a false rejection — and the rule ID does
// not tell the two apart, because an ungoverned type is reported under
// cvc-datatype-valid exactly as a genuine rejection is.
//
// The classification is by EXCLUSION, and that direction is FAIL-CLOSED: an
// error is a verdict unless it explicitly wraps the type-fault sentinel, so a
// stage this package grows later whose fault goes unmarked is charged as a
// verdict — the false reject this predicate exists to prevent. The exclusion is
// not a safety net, it is what makes the predicate total over the open set of
// backend Parse errors, which cannot be enumerated. What bounds the risk is that
// marking has exactly three sites, typeFault, facetPrecondition and
// assertionDeclined; a new non-verdict error must pass through one of them, and
// `grep typeFault(`, `grep facetPrecondition(` and `grep assertionDeclined(`
// enumerate the whole class.
func IsDatatypeVerdict(err error) bool {
	return err != nil && !errors.Is(err, errTypeFault) && !IsAssertionDeclined(err)
}

// ValidateLexical validates the lexical string rawLexical against st's effective
// facets through the full facet pipeline (whiteSpace → pattern → lexical mapping
// → value facets → assertions), returning the parsed value on success or the
// first error a stage produces (stop-on-first-failure; this does not collect all
// facet violations). A rejection carries the stage's *xsderr.Error, reachable
// through [xsderr.RuleOf] whether or not the error is wrapped; call
// [IsDatatypeVerdict] before reading any of them as a verdict. ctx is the
// VALIDATED INSTANCE's context, threaded to the governing mapping's Parse for the
// candidate value; a context-free cohort (decimal/boolean/string) passes nil
// here.
//
// a decides each {test} of an assertions facet the pipeline reaches
// (cvc-assertions-valid, Datatypes §4.3.13.3) — st's own, and those of every
// type the pipeline recurses into: a list's {item type definition} per item, a
// union's members — and MUST be non-nil. Its stage runs last, after every other
// value facet has accepted the value (cvc-datatype-valid clause 3): a {test} it
// fails is a cvc-assertions-valid rejection, and one it declines returns an
// error [IsAssertionDeclined] reports true for, which is not a verdict.
//
// Where r answers notationDeclarer, as *xsd.Schema does, a literal whose
// {primitive type definition} is NOTATION is also held to NOTATION's ·lexical
// space·, "the set of all names of notations declared in the current schema"
// (Datatypes §3.3.19): one whose prefix ctx does not bind, or whose QName names
// none of r's notations, is rejected under cvc-datatype-valid, after its value
// facets have accepted it and before its assertions stage, which it never
// reaches (declaredNotation). That holds wherever NOTATION decides the literal
// — st itself, a type derived from it, a list item or a union member, whose
// rejection lets the dispatch fall through to a later member (§4.1.4 cl.2.3).
// Against any other r, NOTATION is held to b's mapping and st's facets alone,
// and no literal is rejected as undeclared.
//
// PRECONDITION (caller-guarded, not PRE-checked here — but a violation is
// reported, see below): every facet on st is applicable to st per
// cos-applicable-facets (§4.1.5), and b maps st's governing type. The
// applicability half of that precondition is DISCHARGED for any st reachable in a
// schema finalized with an xsd.SimpleTypeRestrictionChecker installed, as the
// parser always does (cos-st-restricts clause 1.3.1 for the atomic case, clauses
// 2.2.2.4/3.2.2.4 inside package xsd for list and union). It remains the CALLER's
// to honor for an st assembled through the xsd constructors and finalized without
// that capability, or never finalized at all — so a violated precondition is
// reachable, and it is REPORTED rather than assumed away.
//
// st may be atomic, list or union variety, and EACH is decided end to end — the
// three cases of cvc-datatype-valid clause 2 (§4.1.4). Atomic (cl.2.1) and list
// (cl.2.2) share the pipeline below: both resolve their in-force whiteSpace
// facet, and a list resolves its value exactly as an atomic one does, because
// governingMapping wraps the item TYPE in a listMapping whose Parse recurses
// here per item — so an item is Datatype Valid by the whole rule, the item
// type's own facets included (clause dv_list, list.go). A union (cl.2.3,
// dv_union) takes the separate dispatch path in union.go instead: it carries no
// whiteSpace facet of its own (categorically not applicable, §4.1.5), and its
// literal is decided by its {member type definitions} in order — the first
// member that is itself Datatype Valid is the ·active member type·, its value
// IS the union's value (dv_union's V is a pass-through, never a union-shaped
// wrapper), and the union's own pattern and enumeration facets are applied
// around that dispatch.
//
// A VIOLATED PRECONDITION is returned as an *xsderr.Error that
// [IsFacetPrecondition] reports true for — never as a panic, and never as a
// validity verdict. There are exactly two such states:
//
//   - a value facet paired with a value lacking the CAPABILITY that facet needs: a
//     bound facet on a value that is not [Ordered], a length facet on one that is
//     not [Lengthed], a digit facet on one that is not [DigitCounted], a scale facet
//     on one that is not [Scaled], explicitTimezone on one that is not
//     [TimezoneAware]. Each is a facet sitting in {facets} outside the set §4.1.5
//     permits, so each is charged to cos-applicable-facets. Note the capability is
//     what is missing, not the property: a [TimezoneAware] value with no timezone
//     under a required explicitTimezone facet, and a [Scaled] value whose ·scale· is
//     absent, are ordinary verdicts (a rejection and a vacuous pass), not faults.
//   - an atomic or list st with no usable whiteSpace mode in force — no whiteSpace
//     facet at all, a multi-valued one, or a {value} outside the §4.3.6.1 domain —
//     where §3.16.7.4 and §4.3.6.1 guarantee one. That is charged to
//     [xsderr.RuleComponentInvariant], not to a cvc-* or cos-* ID: §4.3.6.3 states
//     outright that "there are no Validation Rules associated with whiteSpace", and
//     the state it names is a representation invariant of the component rather than
//     any numbered clause (effectiveWhiteSpace).
//
// Neither says anything about rawLexical, so a caller that charges one as a
// validity verdict converts its own construction bug into a FALSE REJECT of a valid
// schema or instance. Inside this package the two deciders that need to know WHICH
// fault discriminate it: dispatchUnion aborts the member scan instead of folding
// the fault into its rejection list, and checkEnumerationRestriction skips the
// member instead of re-charging it under §4.3.5.5.
//
// A precondition fault is one member of a WIDER class — every error this function
// returns that is a fault of st or of b rather than a verdict about rawLexical.
// [IsDatatypeVerdict] enumerates that class and is what a caller deciding validity
// tests; valueSpace.ValidDefault reads it and so does the instance validator.
// [IsFacetPrecondition] answers the narrower "which fault", which only the two
// sites above ask.
//
// The three §4.1.5 states in which NO facet is applicable at all are not faults and
// not errors: an absent {variety} (xs:anySimpleType), an atomic {variety} with an
// absent {primitive type definition} (xs:anyAtomicType), and a union, whose
// applicable facets are pattern, enumeration and assertions alone. No whiteSpace
// mode is in force for any of them and none is required, so the normalization stage
// is skipped and the literal is parsed as written.
//
// Facet {value} parsing is a separate concern with its own scope: an inherited
// enumeration/bound facet's lexical {value} is parsed in the DECLARING SCHEMA's
// context (see newEnumFacet/newBoundFacet), which for a context-free cohort is
// nil-equivalent. A QName/NOTATION enumeration member does NOT inherit ctx (the
// validated instance's context): newEnumFacet resolves each member's prefixes
// against the bindings in scope where its <enumeration> was written (§3.3.18),
// carried per member on the facet, never against this instance scope.
// r resolves st's {base type definition} chain, which a compiled schema may
// defer by name (xsd.SimpleTypeOrRef). It is a PARAMETER at this entry point and
// is stored NOWHERE in this package — not on [Backend], not on a compiled facet,
// not on the [xsd.ValueSpace] this package returns — so one backend and one
// value space serve every schema. An unresolvable base surfaces as the
// src-resolve error [xsd.SimpleType.Base] produces, which is neither a validity
// verdict about rawLexical nor a facet-pipeline precondition fault: it says the
// TYPE could not be read at all. A caller resolving against an assembled
// xsd.Schema passes it; one validating against a Schema-less graph passes a stub
// that resolves nothing, which is correct because such a graph carries no
// by-name base to resolve.
func ValidateLexical(b Backend, r xsd.TypeResolver, st *xsd.SimpleType, rawLexical string, ctx Context, a AssertionEvaluator) (Value, error) {
	v, _, err := validateLexical(b, r, st, rawLexical, ctx, a)
	return v, err
}

// ValidatingType is key-vtype clause 1 (Structures §3.16.4, consumed by
// key-TYPE-value downstream): the ·validating type· of rawLexical against st —
// st itself if st is not a union, otherwise the ·active basic member· of its
// transitive membership (dt-active-member/dt-active-basic-member, Datatypes
// §4.1.4 cl.2.3) that accepted it. It runs the same verdict ValidateLexical
// runs (validateUnion, for the union case), so a caller learns the member the
// verdict decided from ONE call instead of re-invoking ValidateLexical per
// candidate member itself to rediscover it, which validate/cvcid.go's
// validatingType used to do.
//
// For the union case, WHICH member decided is found by activeBasicMember
// (union.go), a scan of its own rather than a projection of dispatchUnion's:
// dispatchUnion folds any member fault that is not [IsFacetPrecondition] into
// "rejected" and keeps scanning — a gap value/valuespace.go's own GAP(value)
// comment charges to #462, not to be widened here. activeBasicMember instead
// declines the whole identification on ANY member error that is not a verdict
// ([IsDatatypeVerdict]), the wider test validate/cvcid.go's historical
// per-member check already applied. Riding dispatchUnion's own fold-tolerant
// granularity for the identification instead would make ValidatingType MORE
// PERMISSIVE than that historical behavior on a member with a
// non-facet-precondition fault — #462's fix to make, not this seam's to make
// silently, so ValidatingType's err can legitimately disagree with
// ValidateLexical's own verdict on such an input: a real, narrow, and
// deliberate divergence, not a bug.
//
// Both scans run each member's assertions facets through a, which MUST be
// non-nil, as ValidateLexical does: a member one of whose {test}s fails is not
// Datatype Valid, so a later member is the ·validating type·
// (dt-active-member), and a member one of whose {test}s a declines stops both
// scans, so no later member is chosen on a verdict nobody decided.
//
// A non-nil err here is not necessarily a rejection of rawLexical —
// [IsDatatypeVerdict] says which, on the same terms ValidateLexical's own
// callers already apply. On any non-nil err the *xsd.SimpleType result is nil.
//
// NOTATION is held to the notations r declares on ValidateLexical's terms, so
// an undeclared NOTATION literal is rejected here too, and a union member
// rejecting one is not the ·active basic member·.
func ValidatingType(b Backend, r xsd.TypeResolver, st *xsd.SimpleType, rawLexical string, ctx Context, a AssertionEvaluator) (*xsd.SimpleType, Value, error) {
	variety, err := st.Variety(r)
	if err != nil {
		return nil, nil, typeFault(err)
	}
	if _, ok := variety.(xsd.Union); !ok {
		v, _, err := validateLexical(b, r, st, rawLexical, ctx, a)
		if err != nil {
			return nil, nil, err
		}
		return st, v, nil
	}
	v, _, err := validateUnion(b, r, st, rawLexical, ctx, a)
	if err != nil {
		return nil, nil, err
	}
	t, err := activeBasicMember(b, r, st, rawLexical, ctx, a)
	if err != nil {
		return nil, nil, err
	}
	return t, v, nil
}

// validateLexical is ValidateLexical's internal form: the same verdict, plus the
// ·basic member· that actually decided the literal — st itself for the atomic and
// list varieties, the ·active basic member· for a union, which validateUnion
// reaches by recursing here per member (§4.1.4 cl.2.3, dt-active-basic-member).
//
// That third result exists for exactly one consumer, validateUnion, which needs
// the member twice: a union's own pattern facet must be matched against the
// literal as normalized by that member's whiteSpace ("in the case of unions the
// ·pre-lexical· facets to use are those associated with B in clause 2.3", the
// dv_vfacets note; PRINCIPLES 11), and the union's own assertions bind `$value`
// under that member (dt-xdmrep clause 4) — and only the callee knows which member
// that was. No caller outside this package needs it, so the exported wrapper
// drops it rather than widening the API (STYLE T5).
func validateLexical(b Backend, r xsd.TypeResolver, st *xsd.SimpleType, rawLexical string, ctx Context, a AssertionEvaluator) (Value, *xsd.SimpleType, error) {
	// {variety} dispatch, cvc-datatype-valid clause 2 (§4.1.4): a union takes
	// clause 2.3's member dispatch (union.go), which composes st's own facets
	// around the dispatched member's verdict rather than around st's own mapping.
	// Atomic (cl.2.1) and list (cl.2.2) share the path below.
	variety, err := st.Variety(r)
	if err != nil {
		return nil, nil, typeFault(err)
	}
	if _, ok := variety.(xsd.Union); ok {
		return validateUnion(b, r, st, rawLexical, ctx, a)
	}

	lexFacets, valFacets, assertFacets, err := compile(b, r, st, a)
	if err != nil {
		return nil, nil, typeFault(err)
	}

	// whiteSpace stage (§4.3.6): normalize using st's effective whiteSpace facet,
	// resolved off EffectiveFacets (the ordinary same-kind overlay, §3.16.6.4). A
	// zero mode with no error is §4.1.5's "no facets are applicable": the union
	// {variety} the dispatch above already took, and the two ·special· datatypes,
	// whose {variety} or {primitive type definition} is absent. Those normalize
	// nothing and the literal is parsed as written — the same `if ws != 0` guard
	// facetValue applies to a facet's own {value}.
	lexical, err := ApplyWhiteSpace(r, st, rawLexical)
	if err != nil {
		return nil, nil, err
	}

	// pattern (lexical) stage (cvc-pattern-valid, §4.3.4.4): checked on the
	// whiteSpace-normalized lexical, before the value even exists.
	for _, lf := range lexFacets {
		if err := lf.CheckLexical(lexical); err != nil {
			return nil, nil, err
		}
	}

	// lexical mapping: the candidate value is produced by st's OWN governing
	// mapping (its own, or its nearest mapped ancestor's — the widest-space rule
	// governs facet {value}s, not the application-facing candidate).
	m, ok, err := governingMapping(b, r, st, a)
	if err != nil {
		return nil, nil, typeFault(err)
	}
	if !ok {
		return nil, nil, typeFault(xsderr.New(ruleCvcDatatypeValid, xsderr.Loc{},
			"%s has no governing backend mapping, so the backend cannot decide cvc-datatype-valid for it", simpleTypeLabel(st.Name())))
	}
	v, err := m.Parse(lexical, ctx)
	if err != nil {
		return nil, nil, err
	}

	// value-facet stage: enumeration/bounds/digits/length on the parsed value.
	for _, vf := range valFacets {
		if err := vf.CheckValue(v); err != nil {
			return nil, nil, err
		}
	}
	if err := declaredNotation(r, st, variety, lexical, ctx); err != nil {
		return nil, nil, err
	}

	// assertions stage (cvc-assertions-valid, §4.3.13.3), last: every other
	// value facet has accepted v (cvc-datatype-valid clause 3).
	if err := checkAssertions(b, r, st, st, v, assertFacets, a); err != nil {
		return nil, nil, err
	}
	return v, st, nil
}

// compile builds the pattern (lexical) and value facet checkers for st from its
// EffectiveFacets: pattern regexes are translated and compiled here and facet
// {value}s are parsed here, so the stage runners downstream do neither. A bad
// pattern or an unmappable declaring type surfaces here as an *xsderr.Error, at
// the point the checkers are built rather than mid-stage.
//
// It runs once per validateLexical CALL, not once per type — which is once per
// atomic literal validated, once per union member attempt (validateUnion compiles
// the union's own facets, then every member the dispatch tries re-enters
// validateLexical and compiles its own), and once per LIST ITEM, since
// listMapping.Parse decides each whitespace-delimited token by recursing through
// validateLexical (list.go; dv_list §4.1.4 cl.2.2). So validating a 500-item
// xs:byte list translates and compiles the item type's pattern regexes 500 times.
// Nothing is amortized across calls and nothing memoizes st's checkers: a cache
// here would be derivable state (STYLE D3, PRINCIPLES 6). The call frequency is
// high and known — validate/ reaches ValidateLexical once per attribute
// (cvc-attribute cl.3), once per simpleContent element (cvc-complex-type
// cl.1.2) and once per simple-typed element (cvc-type cl.3.1.3) — so it has
// been profiled at that frequency rather than assumed: the cost was not the
// recompilation but regex.Translate materializing \p{C} one code point at a
// time, and it was fixed there (regex.runeSet.addTable, #913). Memoizing st's
// checkers becomes real performance work — measure first, then amortize behind
// one seam — when a profile puts the cost in this function.
//
// The whiteSpace facet is consumed by the normalize stage, not as a checker;
// explicitTimezone is a value facet handled here (cvc-explicitTimezone-valid,
// §4.3.14.3). An assertions facet is not compiled — its {test}s are XPath, which
// the [AssertionEvaluator] decides — but returned, in effective-facet order, for
// the assertions stage (checkAssertions) that runs after every value facet. a is
// threaded to the enumeration and bound facets, whose {value}s a list or union
// declaring type parses through the full pipeline (declaringFacetSpace).
func compile(b Backend, r xsd.TypeResolver, st *xsd.SimpleType, a AssertionEvaluator) ([]LexicalFacet, []ValueFacet, []xsd.Facet, error) {
	var lexFacets []LexicalFacet
	var valFacets []ValueFacet
	var assertFacets []xsd.Facet
	eff, err := st.EffectiveFacets(r)
	if err != nil {
		return nil, nil, nil, err
	}
	for i, ef := range eff {
		switch ef.Facet().Kind() {
		case xsd.FacetWhiteSpace:
			// Consumed by the whiteSpace normalize stage, not a checker.
		case xsd.FacetPattern:
			pf, err := newPatternFacet(ef.Facet(), newFacetOrigin(st, eff, i))
			if err != nil {
				return nil, nil, nil, err
			}
			lexFacets = append(lexFacets, pf)
		case xsd.FacetEnumeration:
			enf, err := newEnumFacet(b, r, st, ef.Facet(), newFacetOrigin(st, eff, i), a)
			if err != nil {
				return nil, nil, nil, err
			}
			valFacets = append(valFacets, enf)
		case xsd.FacetMaxInclusive, xsd.FacetMaxExclusive, xsd.FacetMinInclusive, xsd.FacetMinExclusive:
			bf, err := newBoundFacet(b, r, st, ef.Facet(), newFacetOrigin(st, eff, i), a)
			if err != nil {
				return nil, nil, nil, err
			}
			valFacets = append(valFacets, bf)
		case xsd.FacetTotalDigits, xsd.FacetFractionDigits:
			df, err := newDigitsFacet(ef.Facet())
			if err != nil {
				return nil, nil, nil, err
			}
			valFacets = append(valFacets, df)
		case xsd.FacetLength, xsd.FacetMinLength, xsd.FacetMaxLength:
			lf, err := newLengthFacet(r, st, ef.Facet())
			if err != nil {
				return nil, nil, nil, err
			}
			valFacets = append(valFacets, lf)
		case xsd.FacetExplicitTimezone:
			tf, err := newExplicitTimezoneFacet(ef.Facet())
			if err != nil {
				return nil, nil, nil, err
			}
			valFacets = append(valFacets, tf)
		case xsd.FacetMaxScale, xsd.FacetMinScale:
			sf, err := newScaleFacet(ef.Facet())
			if err != nil {
				return nil, nil, nil, err
			}
			valFacets = append(valFacets, sf)
		case xsd.FacetAssertions:
			// Collected for the assertions stage, not compiled: its {test}s are
			// the AssertionEvaluator's.
			assertFacets = append(assertFacets, ef.Facet())
		default:
			// A FacetKind with no case above is a package-internal completeness
			// bug, not instance data and not even caller data: the enum was
			// extended without wiring its checker here, and no *xsd.SimpleType a
			// caller can build reaches it (st-props-correct clause 5 rejects an
			// out-of-enum kind at construction, TestUnsupportedFacetKindRejected).
			// So it stays a panic where the capability faults became errors —
			// those name a fixable mistake in the caller's own component, this one
			// names a hole in this package — matching the kind-dispatch convention
			// of boundFacet.violates, boundRule, digitsRule, lengthRule and
			// scaleRule. Failing loud rather than silently dropping the facet is
			// the #133 silent-drop bug class. Trade-off: adding this default
			// disables golangci `exhaustive`'s compile-time FacetKind-coverage
			// check for this switch, so a future kind is caught here at
			// test/runtime instead of at lint time — still strictly better than a
			// silent no-op drop. FacetKind.String() never panics on an unknown
			// value, so %s below is always safe.
			panic(fmt.Sprintf("value: compile: unhandled FacetKind %s", ef.Facet().Kind()))
		}
	}
	return lexFacets, valFacets, assertFacets, nil
}

// facetOrigin is how the type compile checks holds one entry of its {facets}:
// declared by its own restriction, or inherited from the ancestor that declared
// it — the restriction case of the {facets} mapping (Datatypes §4.1.2 case 1),
// which the spec states as no chargeable relation, so this is provenance a
// pattern, enumeration or bound message renders, not a rule. The checked type
// is compile's st: the item or member type under a list or union, never the
// cvc-type governing type validate reports. The zero facetOrigin reads as own.
type facetOrigin struct {
	// declaring is EffectiveFacet.Declaring(): the zero QName for an anonymous
	// declarer.
	declaring xsd.QName
	// heir is the checked type when the facet is inherited, nil when it is the
	// checked type's own.
	heir *xsd.SimpleType
}

// newFacetOrigin decides whether eff[i], an entry of st.EffectiveFacets, is
// st's own: it is iff st.OwnFacets holds a facet of eff[i]'s kind and no later
// entry of eff has that kind; otherwise st inherits it. Kind and position decide
// it, never QName equality of Declaring() and st's name, which cannot tell an
// anonymous type's own facet from one an anonymous ancestor declared — both
// sides are the zero QName (#552). The rule rests on two facts: st declares at
// most one facet per kind (st-props-correct clause 4, xsd checkSTProps), and
// EffectiveFacets keeps st's own entry of a kind last — the only entry of a
// replace kind, the last of the pattern entries it keeps base before derived
// (xsd overlayFacet).
func newFacetOrigin(st *xsd.SimpleType, eff []xsd.EffectiveFacet, i int) facetOrigin {
	kind := eff[i].Facet().Kind()
	o := facetOrigin{declaring: eff[i].Declaring()}
	for _, later := range eff[i+1:] {
		if later.Facet().Kind() == kind {
			o.heir = st
			return o
		}
	}
	for _, own := range st.OwnFacets() {
		if own.Kind() == kind {
			return o
		}
	}
	o.heir = st
	return o
}

// relation renders the facet's tie to the type it was checked against, after
// the facet's noun: "of <declaring>" for an own facet, "that <heir> inherits
// from <declaring>" for an inherited one.
func (o facetOrigin) relation() string {
	if o.heir == nil {
		return "of " + simpleTypeLabel(o.declaring)
	}
	return fmt.Sprintf("that %s inherits from %s", simpleTypeLabel(o.heir.Name()), simpleTypeLabel(o.declaring))
}

// inheritedBy renders the heir after a message that already names the declaring
// type: "" for an own facet, " that <heir> inherits" for an inherited one.
func (o facetOrigin) inheritedBy() string {
	if o.heir == nil {
		return ""
	}
	return fmt.Sprintf(" that %s inherits", simpleTypeLabel(o.heir.Name()))
}

// governingMapping resolves the Mapping that governs node's value space. For a
// list {variety} it wraps the item TYPE in a listMapping, which decides each
// item by the full cvc-datatype-valid rule against that type — its own facets
// included, not merely its governing mapping (clause dv_list, §4.1.4 cl.2.2,
// list.go); for a union {variety} it wraps the {member type definitions} in a
// unionMapping (clause dv_union, §4.1.4 cl.2.3, union.go); otherwise it walks
// from node (inclusive) up the base chain and returns the first ancestor's
// Mapping the backend supplies — the widest-space resolution (backend.go,
// st-restrict-facets §3.16.6.4): a derived type without its own mapping is
// governed by its nearest mapped ancestor's.
//
// Both {variety} checks are applied only to node itself, never to every ancestor
// in the base-chain walk, because a constructed type's whole derivation chain
// shares its {variety} (Structures §3.16.1: a list's {item type definition} is
// never itself a list, and cos-st-restricts clause 3.2 makes a union restriction's
// base a union) — so if node is neither, none of its ancestors are either and the
// atomic loop below is unchanged. Either constructed branch is ungoverned when
// what it is built from is: listGoverned (list.go) carries why an ungoverned item
// type leaves the whole list ungoverned, unionGoverned (union.go) why one unmapped
// member spoils the whole dispatch — both yielding the same (Mapping{}, false)
// "ungoverned" outcome the atomic case returns.
//
// a is the [AssertionEvaluator] a list's or a union's synthesized Parse hands
// each item or member it decides through the full pipeline; the atomic case
// never reads it.
func governingMapping(b Backend, r xsd.TypeResolver, node *xsd.SimpleType, a AssertionEvaluator) (Mapping, bool, error) {
	variety, err := node.Variety(r)
	if err != nil {
		return Mapping{}, false, err
	}
	if _, ok := variety.(xsd.List); ok {
		item, err := node.Item(r)
		if err != nil {
			return Mapping{}, false, err
		}
		governed, err := listGoverned(b, r, item, a)
		if err != nil || !governed {
			return Mapping{}, false, err
		}
		return listMapping(b, r, item, a), true, nil
	}
	if _, ok := variety.(xsd.Union); ok {
		members, err := node.Members(r)
		if err != nil {
			return Mapping{}, false, err
		}
		governed, err := unionGoverned(b, r, members, a)
		if err != nil || !governed {
			return Mapping{}, false, err
		}
		return unionMapping(b, r, members, a), true, nil
	}
	s, ok, err := governingNode(b, r, node)
	if err != nil || !ok {
		return Mapping{}, false, err
	}
	m, ok := b.Mapping(s.Name())
	return m, ok, nil
}

// governingNode walks from node (inclusive) up the base chain and returns the
// first type the backend supplies a Mapping for — the widest-space rule
// (st-restrict-facets §3.16.6.4, backend.go) as a single encoding (STYLE T4),
// read by governingMapping for the Mapping it names and by governingType
// (valuespace.go) for the node's identity. It applies NO {variety} test: each
// caller states its own (see governingMapping for why one test on node settles
// the whole chain).
func governingNode(b Backend, r xsd.TypeResolver, node *xsd.SimpleType) (*xsd.SimpleType, bool, error) {
	for s := node; s != nil; {
		if _, ok := b.Mapping(s.Name()); ok {
			return s, true, nil
		}
		next, err := s.Base(r)
		if err != nil {
			return nil, false, err
		}
		s = next
	}
	return nil, false, nil
}

// declaringFacetSpace resolves the two things a Constraining Facet's raw {value}
// attribute string needs in order to become a Value: the Mapping that governs the
// space it is parsed in, and the whiteSpace mode that normalizes it first. Both
// are read off the type named declaring, found on leaf's base chain.
//
// The MAPPING is the widest-space rule (st-restrict-facets §3.16.6.4,
// backend.go) for an inherited facet: the governing mapping FROM the declaring
// type (its own, or its nearest mapped ancestor's) — never leaf's. A facet's
// lexical {value} is parsed in the value space of the type that DECLARES it, so
// a narrow derived representation can never corrupt an inherited bound/enum
// comparison (overflow, collapsed precision, different ordering).
//
// The whiteSpace MODE is the one in force on the declaring type's {base type
// definition}, not on the declaring type itself. A facet's {value} is "a value
// from the value space of the {base type definition}" (§4.3.7.1 f-mai-value and
// its siblings, §4.3.5.1 for enumeration), and reaching that value space from the
// facet's XML `value` attribute runs the base type's lexical mapping, whose first
// stage is that type's whiteSpace normalization (key-vv §3.1.3, key-nv §3.1.4,
// cvc-simple-type §3.16.4). A declaring type that overrides whiteSpace on its own
// account (say collapse over a replace base) narrows what its own INSTANCES may
// look like; it does not retroactively renormalize the facet {value}s it writes
// against the base. A zero mode — an unmapped/absent base, or one carrying no
// usable whiteSpace facet (xs:anySimpleType, xs:anyAtomicType, a union) — means
// no normalization applies (whiteSpaceInForce).
//
// Types are matched by QName, and an ANONYMOUS component (the zero QName) is a
// normal inhabitant of a base chain — every builtin list datatype restricts an
// anonymous intermediate list (§3.4.5/§3.4.10/§3.4.12, builtin.Seed), so such a
// node sits between xs:NMTOKENS/xs:IDREFS/xs:ENTITIES and xs:anySimpleType. It is
// never the DECLARING type of a facet reaching here: only enumeration and the
// four bound facets route through this resolution, and §3.16.2.1 map.std.common
// case 3 gives a constructed list exactly one facet, whiteSpace, which the
// normalization stage consumes without consulting Declaring(). A caller that
// nonetheless passed the zero QName would match the nearest anonymous node from
// leaf upward, which is the same nearest-declaration rule a named match gets.
//
// a is governingMapping's, read only for a list or union declaring type.
func declaringFacetSpace(b Backend, r xsd.TypeResolver, leaf *xsd.SimpleType, declaring xsd.QName, a AssertionEvaluator) (m Mapping, ws whiteSpace, ok bool, err error) {
	for s := leaf; s != nil; {
		base, err := s.Base(r)
		if err != nil {
			return Mapping{}, 0, false, err
		}
		if s.Name() != declaring {
			s = base
			continue
		}
		m, ok, err := governingMapping(b, r, s, a)
		if err != nil {
			return Mapping{}, 0, false, err
		}
		if !ok {
			return Mapping{}, 0, false, nil
		}
		ws, err := whiteSpaceInForce(r, base)
		if err != nil {
			return Mapping{}, 0, false, err
		}
		return m, ws, true, nil
	}
	return Mapping{}, 0, false, nil
}

// facetValue turns a Constraining Facet's RAW {value} attribute string into a
// member of the value space m governs, and is the ONLY place in this package a
// facet {value} is parsed: newBoundFacet and newEnumFacet reach it at
// instance-pipeline construction, restriction.go's boundLimit and
// checkEnumerationRestriction at schema-construction restriction checking.
//
// It normalizes BEFORE parsing because a facet's {value} property is already a
// parsed value ("a value from the value space of the {base type definition}",
// §4.3.7.1 f-mai-value and siblings), and the XML mapping from the `value`
// attribute to that property runs the ordinary lexical pipeline — whiteSpace
// first (key-nv §3.1.4, cvc-simple-type §3.16.4). So `<maxInclusive value=" 9 "/>`
// on a collapse-normalized base denotes exactly the {value} the untrailed
// spelling does. A zero ws means no mode is in force and raw is parsed unchanged.
//
// Keeping this single seam is what makes the *-valid-restriction SCCs
// (§4.3.7.4–§4.3.10.4, §4.3.5.5) pure already-parsed-value comparisons, as their
// clause text presumes: they never mention strings, so no normalization may
// happen at comparison time — it must all have happened here.
func facetValue(m Mapping, ws whiteSpace, raw string, ctx Context) (Value, error) {
	if ws != 0 {
		raw = normalizeWhiteSpace(raw, ws)
	}
	return m.Parse(raw, ctx)
}

// patternFacet is the pattern (lexical) stage (cvc-pattern-valid, §4.3.4.4).
// Each FacetPattern EffectiveFacet returned by EffectiveFacets represents ONE
// derivation step's OR-set (the branches declared at that step, ORed within its
// Values()); patterns at different steps are NOT folded into one facet — they
// stay as separate EffectiveFacets (§4.3.4.2 xr-pattern: cross-step patterns
// are ANDed, never merged into one flat OR-set). compile() builds one
// patternFacet per such EffectiveFacet, and ValidateLexical requires EVERY one
// to pass (AND-across-steps); within a single patternFacet a literal is
// pattern-valid if it matches ANY member (the same-step OR-set). The RE2
// regexes are compiled once at construction; facet and origin are kept so a
// rejection names the step that failed — its literals, the type that declares
// them, and whether the checked type inherits them.
type patternFacet struct {
	facet  xsd.Facet
	origin facetOrigin
	res    []*regexp.Regexp
}

// newPatternFacet translates each XSD-flavor pattern value of f to RE2 and
// compiles it (regex.FlavorXSD is implicitly whole-string anchored; ^ and $ are
// literal characters, not anchors). A bad pattern surfaces here, not
// mid-validation.
func newPatternFacet(f xsd.Facet, o facetOrigin) (patternFacet, error) {
	values := f.Values()
	res := make([]*regexp.Regexp, 0, len(values))
	for _, p := range values {
		goRE, err := regex.Translate(p, regex.FlavorXSD, "")
		if err != nil {
			return patternFacet{}, err // already an *xsderr.Error (src-pattern-value)
		}
		re, err := regexp.Compile(goRE)
		if err != nil {
			return patternFacet{}, xsderr.Wrap(ruleSrcPatternValue, xsderr.Loc{}, err)
		}
		res = append(res, re)
	}
	return patternFacet{facet: f, origin: o, res: res}, nil
}

// patternMemberEscaper renders one pattern {value} member for a
// cvc-pattern-valid message, between the double quotes CheckLexical adds:
// `&`, `"`, LF and CR become the XML character references `&amp;`, `&quot;`,
// `&#10;` and `&#13;`, so a quote inside a member never reads as the end of
// one, the `", "` joining members never appears inside one, and a line break
// never splits the message (#2350). Every other character stays as written —
// a backslash in particular is never doubled, so `[\-+]?[0-9]+` prints as the
// Datatypes spec spells it.
var patternMemberEscaper = strings.NewReplacer(`&`, "&amp;", `"`, "&quot;", "\n", "&#10;", "\r", "&#13;")

// CheckLexical accepts the normalized literal iff it matches at least one
// pattern in the OR-set (cvc-pattern-valid, §4.3.4.4). A rejection names the
// whole OR-set of the failed step — every member, never a single `|` branch,
// rendered by patternMemberEscaper — and its facetOrigin relation: the type on
// the base chain that declares it, and the checked type when that type inherits
// it, which is derived provenance, not a spec property (simpleTypeLabel renders
// the zero QName of an anonymous one).
func (p patternFacet) CheckLexical(normalized string) error {
	for _, re := range p.res {
		if re.MatchString(normalized) {
			return nil
		}
	}
	values := p.facet.Values()
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = `"` + patternMemberEscaper.Replace(v) + `"`
	}
	return xsderr.New(ruleCvcPatternValid, xsderr.Loc{},
		"%q matches no member of the pattern facet %s, whose {value} holds %s, but cvc-pattern-valid requires one to match",
		normalized, p.origin.relation(), strings.Join(quoted, ", "))
}

// enumFacet is the enumeration value-facet stage (cvc-enumeration-valid,
// §4.3.5.4): a candidate is valid iff it is "equal or identical to one of the
// values specified in {value}". The members are parsed once, in the value space
// of the type that DECLARES the enumeration (widest-space rule); origin names
// that type in a rejection.
type enumFacet struct {
	members []Value
	origin  facetOrigin
}

// newEnumFacet parses each enumeration {value} member via the declaring type's
// mapping (widest-space rule, st-restrict-facets §3.16.6.4). Each member carries
// its own namespace context — the bindings in scope where its <enumeration> was
// written (§3.3.18) — so a QName/NOTATION member's prefix resolves against the
// DECLARING schema's scope, threaded through a per-member memberContext rather
// than the hardcoded nil that mis-resolved it. A context-free member (every
// non-QName/NOTATION cohort) carries an empty context that resolves identically
// to nil, so those cohorts are unchanged.
//
// A member whose Parse fails — an unresolvable prefix, or otherwise not in the
// declaring type's value space — makes the SimpleType/facet itself malformed AT
// SCHEMA CONSTRUCTION, a Schema Representation Constraint (src-enumeration-value,
// §4.3.5.3), so the bare cvc-datatype-valid Parse returns is remapped to that
// construction-time rule — the sibling of src-pattern-value newPatternFacet
// already uses.
func newEnumFacet(b Backend, r xsd.TypeResolver, st *xsd.SimpleType, f xsd.Facet, o facetOrigin, a AssertionEvaluator) (enumFacet, error) {
	m, ws, ok, err := declaringFacetSpace(b, r, st, o.declaring, a)
	if err != nil {
		return enumFacet{}, err
	}
	if !ok {
		return enumFacet{}, xsderr.New(ruleCvcEnumerationValid, xsderr.Loc{},
			"%s, which declares the enumeration facet%s, has no governing backend mapping, so cvc-enumeration-valid cannot be decided against the facet's {value}",
			simpleTypeLabel(o.declaring), o.inheritedBy())
	}
	// compile() routes only FacetEnumeration facets here, so EnumerationMembers
	// always reports ok=true; the second result is discarded deliberately.
	enumMembers, _ := f.EnumerationMembers()
	members := make([]Value, 0, len(enumMembers))
	for _, em := range enumMembers {
		v, err := facetValue(m, ws, em.Lexical(), newMemberContext(em))
		if err != nil {
			return enumFacet{}, xsderr.Wrap(ruleSrcEnumerationValue, xsderr.Loc{}, err)
		}
		members = append(members, v)
	}
	return enumFacet{members: members, origin: o}, nil
}

// xmlNamespaceURI is the single reserved, implicitly-bound XML namespace prefix
// (Namespaces in XML §3): "xml" is bound by definition with no declaration.
// "xmlns" deliberately gets no implicit binding — it names
// namespace-declaration attributes, not a binding (WG ruling, bugzilla 4053).
const xmlNamespaceURI = "http://www.w3.org/XML/1998/namespace"

// nsContext is this package's ONE encoding of §3.3.18's prefix lookup (STYLE
// T4): the value.Context a QName/NOTATION Mapping.Parse consumes, built from a
// captured set of namespace bindings so a prefixed literal resolves against the
// bindings in scope WHERE IT WAS WRITTEN — an enumeration facet member's
// <enumeration> element (newMemberContext), a value constraint's
// <element>/<attribute> element ([ConstraintContext], valuespace.go). Its
// reserved-prefix rules match conformance.nsContext exactly (value cannot import
// the test-only conformance package, so this is a small second implementation of
// the same logic): "xml" is always bound, "xmlns" gets no case of its own —
// bound only where the captured bindings carry it, which no schema document
// can declare (Namespaces in XML §3) — and the empty prefix resolves to the
// {default namespace} if one is in scope (element-name semantics) else to no
// namespace.
//
// It is a value type, so every caller has a usable context and there is no
// nil-Context path to reason about. The map is an internal lookup, never ranged
// into output (STYLE D2).
type nsContext struct {
	bindings   map[string]string // non-empty prefix -> namespace name
	defaultNS  string
	hasDefault bool
}

// newNSContext builds the context from captured bindings and a {default
// namespace}, which hasDefault reports the presence of. The empty prefix is
// modeled by that {default namespace}, not by a "" binding, so the bindings map
// holds only non-empty prefixes.
func newNSContext(bindings []xsd.NamespaceBinding, defaultNS string, hasDefault bool) nsContext {
	c := nsContext{defaultNS: defaultNS, hasDefault: hasDefault}
	if len(bindings) > 0 {
		c.bindings = make(map[string]string, len(bindings))
		for _, b := range bindings {
			c.bindings[b.Prefix()] = b.Namespace()
		}
	}
	return c
}

// newMemberContext builds the per-member context from an enumeration member's
// own namespace bindings and {default namespace}.
func newMemberContext(m xsd.EnumerationMember) nsContext {
	ns, ok := m.DefaultNamespace()
	return newNSContext(m.NamespaceBindings(), ns, ok)
}

// LookupNamespace resolves prefix per §3.3.18. The reserved prefix "xml" is
// always bound (Namespaces in XML §3); "xmlns" has no case of its own, so it is
// bound only where the captured bindings carry it. The empty prefix (an
// unprefixed literal) binds to the {default namespace} if in scope, else to no
// namespace (ok=true, "") — element-name semantics, so an unprefixed literal is
// never rejected as unbound. A declared non-empty prefix resolves to its binding;
// any other non-empty prefix is genuinely unbound (ok=false), which the mapping's
// Parse turns into a rejection — remapped to src-enumeration-value (§4.3.5.3) for
// a facet member, and read as undecided for a value constraint (valuespace.go).
func (c nsContext) LookupNamespace(prefix string) (namespace string, ok bool) {
	if prefix == "xml" {
		return xmlNamespaceURI, true
	}
	if prefix == "" {
		if c.hasDefault {
			return c.defaultNS, true
		}
		return "", true
	}
	if ns, bound := c.bindings[prefix]; bound {
		return ns, true
	}
	return "", false
}

// CheckValue accepts v iff it is equal or identical to a member
// (cvc-enumeration-valid, §4.3.5.4). A rejection names the facet by its
// facetOrigin relation.
func (e enumFacet) CheckValue(v Value) error {
	for _, member := range e.members {
		if enumMatch(v, member) {
			return nil
		}
	}
	return xsderr.New(ruleCvcEnumerationValid, xsderr.Loc{},
		"value is not equal or identical to any member of the enumeration facet %s, but cvc-enumeration-valid requires it to be equal or identical to one",
		e.origin.relation())
}

// enumMatch reports the "equal or identical" relation cvc-enumeration-valid
// needs (§4.3.5.4). The relation itself is equalOrIdentical, the ONE encoding
// this package has of the §2.2.1/§2.2.2 union (STYLE T4); enumeration matching
// discards its decided result because "not decided" and "not matched" are the
// same verdict for a facet — a candidate carrying neither comparison capability
// matches nothing, which is what this call site has always done.
func enumMatch(candidate, member Value) bool {
	same, _ := equalOrIdentical(candidate, member)
	return same
}

// boundFacet is one of the four bound value-facet stages
// (cvc-maxInclusive/maxExclusive/minInclusive/minExclusive-valid, §4.3.7–4.3.10).
// The limit and candidate must both be Ordered (every bound-applicable primitive
// is ordered, cos-applicable-facets §4.1.5); one that is not is reported as a
// facetPrecondition fault, never as a rejection. An Incomparable Cmp is a legitimate
// spec outcome for a PARTIALLY ordered primitive (float/double): a value
// incomparable with a bounding facet's value is EXCLUDED from the restricted
// value space (§3.3.4.3/§3.3.5.3 Note — e.g. NaN against any numeric bound, or
// any value when the bound itself is NaN), so CheckValue REJECTS it rather than
// panicking.
type boundFacet struct {
	limit Ordered
	// lexical is the facet's {value} as the document wrote it, which a rejection
	// names; limit is its parsed form and cannot render it, since parsing drops
	// the spelling ("+007" and "7" are one limit).
	lexical string
	kind    xsd.FacetKind
	// origin names the facet's declaring type in a rejection.
	origin facetOrigin
}

// newBoundFacet parses the single bound {value} via the declaring type's
// mapping (widest-space rule), whiteSpace-normalized through its base's mode
// first (declaringFacetSpace/facetValue), and requires the result to be Ordered.
func newBoundFacet(b Backend, r xsd.TypeResolver, st *xsd.SimpleType, f xsd.Facet, o facetOrigin, a AssertionEvaluator) (boundFacet, error) {
	kind := f.Kind()
	rule := boundRule(kind)
	m, ws, ok, err := declaringFacetSpace(b, r, st, o.declaring, a)
	if err != nil {
		return boundFacet{}, err
	}
	if !ok {
		return boundFacet{}, xsderr.New(rule, xsderr.Loc{},
			"%s, which declares the %s facet%s, has no governing backend mapping, so %s cannot be decided against the facet's {value}",
			simpleTypeLabel(o.declaring), kind, o.inheritedBy(), rule)
	}
	values := f.Values()
	if len(values) != 1 {
		return boundFacet{}, xsderr.New(rule, xsderr.Loc{},
			"the %s facet of %s carries %d values rather than one, so %s cannot be decided against its {value}",
			kind, simpleTypeLabel(st.Name()), len(values), rule)
	}
	v, err := facetValue(m, ws, values[0], nil)
	if err != nil {
		return boundFacet{}, err
	}
	ord, ok := v.(Ordered)
	if !ok {
		return boundFacet{}, facetPrecondition(ruleCosApplicableFacets, st.Loc(),
			"the %s facet of %s, whose {value} %q parses to a %T that is not Ordered, is not applicable to that type, but cos-applicable-facets allows in {facets} only the facets applicable to the type",
			kind, simpleTypeLabel(st.Name()), values[0], v)
	}
	return boundFacet{limit: ord, lexical: values[0], kind: kind, origin: o}, nil
}

// CheckValue rejects a candidate that violates the bound (§4.3.7–4.3.10). A
// rejection names the facet by its facetOrigin relation; the cos-applicable-facets
// precondition names none.
func (bf boundFacet) CheckValue(v Value) error {
	cand, ok := v.(Ordered)
	if !ok {
		return facetPrecondition(ruleCosApplicableFacets, xsderr.Loc{},
			"the %s facet, whose {value} is %q, is not applicable to the type of a candidate that is not Ordered (%T), but cos-applicable-facets allows in {facets} only the facets applicable to the type",
			bf.kind, bf.lexical, v)
	}
	ord := cand.Cmp(bf.limit)
	if ord == Incomparable {
		// A value incomparable with the bound is excluded from the restricted
		// value space (§3.3.4.3/§3.3.5.3 Note): a real facet rejection, e.g. a
		// NaN candidate against a numeric bound, or any candidate when the bound
		// value is itself NaN (the restricted space is then empty).
		return xsderr.New(boundRule(bf.kind), xsderr.Loc{},
			"value is incomparable with the {value} %q of the %s facet %s, so it is excluded from the restricted value space, but %s requires a value %s it",
			bf.lexical, bf.kind, bf.origin.relation(), boundRule(bf.kind), boundRelation(bf.kind))
	}
	if bf.violates(ord) {
		return xsderr.New(boundRule(bf.kind), xsderr.Loc{},
			"value violates the %s facet %s, whose {value} is %q, but %s requires a value %s it",
			bf.kind, bf.origin.relation(), bf.lexical, boundRule(bf.kind), boundRelation(bf.kind))
	}
	return nil
}

// violates maps the candidate-vs-limit ordering to a bound violation per kind.
func (bf boundFacet) violates(ord Ordering) bool {
	switch bf.kind {
	case xsd.FacetMaxInclusive:
		return ord == Greater
	case xsd.FacetMaxExclusive:
		return ord == Greater || ord == Equal
	case xsd.FacetMinInclusive:
		return ord == Less
	case xsd.FacetMinExclusive:
		return ord == Less || ord == Equal
	default:
		panic(fmt.Sprintf("value: violates: %s is not a bound facet", bf.kind))
	}
}

// boundRelation is the order relation each bound facet's Validation Rule
// requires of a value against the facet's {value}, in that rule's own words:
// cvc-maxInclusive-valid "less than or equal to", cvc-maxExclusive-valid "less
// than", cvc-minInclusive-valid "greater than or equal to" and
// cvc-minExclusive-valid "greater than". It renders a rejection only; violates
// is the decision.
func boundRelation(k xsd.FacetKind) string {
	switch k {
	case xsd.FacetMaxInclusive:
		return "less than or equal to"
	case xsd.FacetMaxExclusive:
		return "less than"
	case xsd.FacetMinInclusive:
		return "greater than or equal to"
	case xsd.FacetMinExclusive:
		return "greater than"
	default:
		panic(fmt.Sprintf("value: boundRelation: %s is not a bound facet", k))
	}
}

// boundRule maps a bound facet kind to its per-facet rule ID (§4.3.7–4.3.10).
func boundRule(k xsd.FacetKind) xsderr.Rule {
	switch k {
	case xsd.FacetMaxInclusive:
		return ruleCvcMaxInclusiveValid
	case xsd.FacetMaxExclusive:
		return ruleCvcMaxExclusiveValid
	case xsd.FacetMinInclusive:
		return ruleCvcMinInclusiveValid
	case xsd.FacetMinExclusive:
		return ruleCvcMinExclusiveValid
	default:
		panic(fmt.Sprintf("value: boundRule: %s is not a bound facet", k))
	}
}

// digitsFacet is the totalDigits/fractionDigits value-facet stage
// (cvc-totalDigits-valid §4.3.11.3, cvc-fractionDigits-valid §4.3.12.3),
// decimal-only (cos-applicable-facets §4.1.5). Both are UPPER-BOUND (magnitude)
// constraints, not exact-count matches: violation is candidate digits > limit.
type digitsFacet struct {
	limit integerLiteral
	kind  xsd.FacetKind
}

// newDigitsFacet reads the facet's plain nonNegativeInteger {value} — a count,
// not a value in the declaring type's space, so no declaring-mapping lookup.
func newDigitsFacet(f xsd.Facet) (digitsFacet, error) {
	rule := digitsRule(f.Kind())
	n, err := facetCount(f, rule)
	if err != nil {
		return digitsFacet{}, err
	}
	return digitsFacet{limit: n, kind: f.Kind()}, nil
}

// CheckValue rejects a candidate whose digit count exceeds the limit
// (§4.3.11.3/§4.3.12.3).
func (df digitsFacet) CheckValue(v Value) error {
	dc, ok := v.(DigitCounted)
	if !ok {
		return facetPrecondition(ruleCosApplicableFacets, xsderr.Loc{},
			"the %s facet, whose {value} is %q, is not applicable to the type of a candidate that is not DigitCounted (%T), but cos-applicable-facets allows in {facets} only the facets applicable to the type",
			df.kind, df.limit, v)
	}
	got := dc.TotalDigits()
	if df.kind == xsd.FacetFractionDigits {
		got = dc.FractionDigits()
	}
	if df.limit.cmpInt(got) < 0 {
		return xsderr.New(digitsRule(df.kind), xsderr.Loc{},
			"value has %d %s, exceeds facet limit %s, which %s forbids", got, df.kind, df.limit, digitsRule(df.kind))
	}
	return nil
}

// digitsRule maps a digit facet kind to its per-facet rule ID.
func digitsRule(k xsd.FacetKind) xsderr.Rule {
	switch k {
	case xsd.FacetTotalDigits:
		return ruleCvcTotalDigitsValid
	case xsd.FacetFractionDigits:
		return ruleCvcFractionDigitsValid
	default:
		panic(fmt.Sprintf("value: digitsRule: %s is not a digit facet", k))
	}
}

// lengthFacet is the length/minLength/maxLength value-facet stage
// (cvc-length-valid §4.3.1.3, cvc-minLength-valid §4.3.2.3, cvc-maxLength-valid
// §4.3.3.3). For string the unit is Unicode codepoints (Lengthed.Len). Clause
// 1.3 of each rule is an unconditional exemption: when the {primitive type
// definition} is QName or NOTATION, "any {value} is facet-valid" regardless of
// the bound — captured in exempt at construction so CheckValue never measures
// such a value (see lengthExemptPrimitive).
type lengthFacet struct {
	limit integerLiteral
	kind  xsd.FacetKind
	// exempt makes CheckValue accept any value unconditionally when st's
	// {primitive type definition} is QName or NOTATION (clause 1.3 of
	// cvc-length-valid §4.3.1.3, cvc-minLength-valid §4.3.2.3, cvc-maxLength-valid
	// §4.3.3.3), independent of limit and of Lengthed.Len.
	exempt bool
}

// newLengthFacet reads the facet's plain nonNegativeInteger {value} (a count),
// so no declaring-mapping lookup. It records st's clause-1.3 exemption
// (QName/NOTATION {primitive type definition}) via lengthExemptPrimitive.
func newLengthFacet(r xsd.TypeResolver, st *xsd.SimpleType, f xsd.Facet) (lengthFacet, error) {
	rule := lengthRule(f.Kind())
	n, err := facetCount(f, rule)
	if err != nil {
		return lengthFacet{}, err
	}
	exempt, err := lengthExemptPrimitive(r, st)
	if err != nil {
		return lengthFacet{}, err
	}
	return lengthFacet{limit: n, kind: f.Kind(), exempt: exempt}, nil
}

// lengthExemptPrimitive reports whether st's resolved {primitive type
// definition} is QName or NOTATION, the two atomic primitives whose
// length/minLength/maxLength facets are an unconditional no-op — "any {value}
// is facet-valid" — per clause 1.3 of cvc-length-valid (§4.3.1.3),
// cvc-minLength-valid (§4.3.2.3), and cvc-maxLength-valid (§4.3.3.3). It keys
// off an atomic {variety}'s {primitive type definition} (§3.16.1), not
// the value's Go type nor a blanket "is atomic" test: clause 1.3 is a case
// split on the primitive, and the list case (clause 2) carries no such
// exemption. A non-atomic {variety} (nil / list / union) or an absent primitive
// (xs:anyAtomicType) is not exempt; this predicate never panics. An unresolvable
// {base type definition} on the way to either property is returned as an error
// rather than read as "not exempt", which would apply a length bound the spec
// exempts and false-reject every QName value under it.
func lengthExemptPrimitive(r xsd.TypeResolver, st *xsd.SimpleType) (bool, error) {
	variety, err := st.Variety(r)
	if err != nil {
		return false, err
	}
	if _, ok := variety.(xsd.Atomic); !ok {
		return false, nil
	}
	primitive, err := st.Primitive(r)
	if err != nil || primitive == nil {
		return false, err
	}
	name := primitive.Name()
	return name == qnameName || name == notationName, nil
}

// qnameName and notationName are the two {primitive type definition} QNames
// clause 1.3 exempts (see lengthExemptPrimitive). Written as plain struct
// literals because package value carries no reusable builtin-name constants;
// xsd.XMLSchemaNS is the XML Schema namespace.
var (
	qnameName    = xsd.QName{Space: xsd.XMLSchemaNS, Local: "QName"}
	notationName = xsd.QName{Space: xsd.XMLSchemaNS, Local: "NOTATION"}
)

// CheckValue rejects a candidate whose length violates the facet
// (§4.3.1.3–4.3.3.3), except when the {primitive type definition} is QName or
// NOTATION: clause 1.3 makes any value facet-valid, so an exempt facet
// short-circuits BEFORE the Lengthed path — the exemption is unconditional and
// must not depend on the value implementing Lengthed.
func (lf lengthFacet) CheckValue(v Value) error {
	if lf.exempt {
		return nil
	}
	l, ok := v.(Lengthed)
	if !ok {
		return facetPrecondition(ruleCosApplicableFacets, xsderr.Loc{},
			"the %s facet, whose {value} is %q, is not applicable to the type of a candidate that is not Lengthed (%T), but cos-applicable-facets allows in {facets} only the facets applicable to the type",
			lf.kind, lf.limit, v)
	}
	if lf.violates(l.Len()) {
		return xsderr.New(lengthRule(lf.kind), xsderr.Loc{},
			"value length %d violates the %s facet limit %s, which %s forbids", l.Len(), lf.kind, lf.limit, lengthRule(lf.kind))
	}
	return nil
}

// violates maps a length to a violation per kind.
func (lf lengthFacet) violates(n int) bool {
	c := lf.limit.cmpInt(n)
	switch lf.kind {
	case xsd.FacetLength:
		return c != 0
	case xsd.FacetMinLength:
		return c > 0
	case xsd.FacetMaxLength:
		return c < 0
	default:
		panic(fmt.Sprintf("value: violates: %s is not a length facet", lf.kind))
	}
}

// lengthRule maps a length facet kind to its per-facet rule ID.
func lengthRule(k xsd.FacetKind) xsderr.Rule {
	switch k {
	case xsd.FacetLength:
		return ruleCvcLengthValid
	case xsd.FacetMinLength:
		return ruleCvcMinLengthValid
	case xsd.FacetMaxLength:
		return ruleCvcMaxLengthValid
	default:
		panic(fmt.Sprintf("value: lengthRule: %s is not a length facet", k))
	}
}

// tzRequirement is the explicitTimezone {value} domain — exactly the three
// tokens required/prohibited/optional (§4.3.14.1), normalized from the facet's
// single NCName {value} at construction so CheckValue never re-parses a string.
type tzRequirement int

const (
	tzRequired tzRequirement = iota
	tzProhibited
	tzOptional
)

// token renders the requirement as its {value} token: the one spelling of each
// token, which newExplicitTimezoneFacet reads the facet's {value} against and a
// facet-stage message names.
func (t tzRequirement) token() string {
	switch t {
	case tzRequired:
		return "required"
	case tzProhibited:
		return "prohibited"
	case tzOptional:
		return "optional"
	default:
		panic(fmt.Sprintf("value: tzRequirement.token: %d is not a requirement", int(t)))
	}
}

// explicitTimezoneFacet is the explicitTimezone value-facet stage
// (cvc-explicitTimezone-valid, §4.3.14.3), applicable to the date/time family
// only (cos-applicable-facets §4.1.5). Its {value} is one of required/prohibited/
// optional (§4.3.14.1), resolved once at construction into a tzRequirement.
type explicitTimezoneFacet struct {
	requirement tzRequirement
}

// newExplicitTimezoneFacet reads the facet's single {value} token
// (required/prohibited/optional) — a plain NCName from the facet's XML
// representation (§4.3.14.2), not a value in the declaring type's space, so no
// declaring-mapping lookup (the digitsFacet/lengthFacet shape). Any other shape
// is a malformed facet, rejected here as an *xsderr.Error, not at check time.
func newExplicitTimezoneFacet(f xsd.Facet) (explicitTimezoneFacet, error) {
	values := f.Values()
	if len(values) != 1 {
		return explicitTimezoneFacet{}, xsderr.New(ruleCvcExplicitTimezoneValid, xsderr.Loc{},
			"the explicitTimezone facet carries %d values rather than one, so cvc-explicitTimezone-valid cannot be decided against its {value}", len(values))
	}
	for _, req := range []tzRequirement{tzRequired, tzProhibited, tzOptional} {
		if values[0] == req.token() {
			return explicitTimezoneFacet{requirement: req}, nil
		}
	}
	return explicitTimezoneFacet{}, xsderr.New(ruleCvcExplicitTimezoneValid, xsderr.Loc{},
		"the explicitTimezone facet's {value} %q is not required, prohibited or optional, the only three values cvc-explicitTimezone-valid clauses 1 to 3 decide",
		values[0])
}

// CheckValue enforces cvc-explicitTimezone-valid (§4.3.14.3): required demands a
// non-absent ·timezoneOffset·, prohibited demands an absent one, optional always
// passes (a real always-succeeding branch, not a dropped stage). The candidate
// must be TimezoneAware for the required/prohibited cases; a non-TimezoneAware
// value under an explicitTimezone facet is a schema-construction fault (the facet
// is not applicable to it, cos-applicable-facets §4.1.5), never instance data, so
// it returns a facetPrecondition error rather than a validity verdict — the
// boundFacet convention.
func (tf explicitTimezoneFacet) CheckValue(v Value) error {
	if tf.requirement == tzOptional {
		return nil
	}
	ta, ok := v.(TimezoneAware)
	if !ok {
		return facetPrecondition(ruleCosApplicableFacets, xsderr.Loc{},
			"the explicitTimezone facet, whose {value} is %q, is not applicable to the type of a candidate that is not TimezoneAware (%T), but cos-applicable-facets allows in {facets} only the facets applicable to the type",
			tf.requirement.token(), v)
	}
	if tf.requirement == tzRequired && !ta.HasTimezone() {
		return xsderr.New(ruleCvcExplicitTimezoneValid, xsderr.Loc{},
			"value has no explicit timezone, but cvc-explicitTimezone-valid clause 1 requires one where the explicitTimezone facet is required")
	}
	if tf.requirement == tzProhibited && ta.HasTimezone() {
		return xsderr.New(ruleCvcExplicitTimezoneValid, xsderr.Loc{},
			"value has an explicit timezone, but cvc-explicitTimezone-valid clause 2 forbids one where the explicitTimezone facet is prohibited")
	}
	return nil
}

// scaleFacet is the maxScale/minScale value-facet stage (cvc-maxScale-valid
// xsd-precisionDecimal.md §4.2.3, cvc-minScale-valid §4.3.3), applicable to
// precisionDecimal and its restrictions ONLY (§3.3) — no other primitive. Both
// facets' {value} is a plain xs:integer that may be NEGATIVE (unlike the
// nonNegativeInteger of totalDigits/fractionDigits/length), held in limit. One
// struct serves both kinds, discriminated by kind, mirroring digitsFacet.
//
// The two rules share a vacuous-pass clause: a value whose ·scale· is ABSENT —
// the specials NaN/±INF, which carry no scale (value.Scaled reports ok=false) —
// is facet-valid w.r.t. both facets regardless of {value} (clause 2 of each
// rule). Only a numeric value's ·scale· is compared against limit (clause 1):
// maxScale bounds it above (violation is scale > limit), minScale bounds it
// below (violation is scale < limit). Both are unbounded integers — ·scale·
// arrives from value.Scaled as a *big.Int — and are compared whole, so neither
// is narrowed to a host int.
type scaleFacet struct {
	limit integerLiteral
	kind  xsd.FacetKind
}

// newScaleFacet reads the facet's plain xs:integer {value} (a scale bound that
// may be negative, so facetInt not facetCount), keyed to the per-facet rule.
func newScaleFacet(f xsd.Facet) (scaleFacet, error) {
	n, err := facetInt(f, scaleRule(f.Kind()))
	if err != nil {
		return scaleFacet{}, err
	}
	return scaleFacet{limit: n, kind: f.Kind()}, nil
}

// CheckValue rejects a candidate whose ·scale· violates the facet
// (cvc-maxScale-valid §4.2.3, cvc-minScale-valid §4.3.3). A value with an absent
// ·scale· (a special: NaN/±INF) passes vacuously — clause 2 of both rules — so
// the Scale() ok=false path returns nil before any comparison. The candidate
// must be Scaled: maxScale/minScale apply to precisionDecimal only (§3.3), so a
// non-Scaled value under one of these facets is a schema-construction fault
// (cos-applicable-facets §4.1.5 not enforced upstream), never instance data, and
// returns a facetPrecondition error rather than a validity verdict — the boundFacet
// convention.
func (sf scaleFacet) CheckValue(v Value) error {
	sc, ok := v.(Scaled)
	if !ok {
		return facetPrecondition(ruleCosApplicableFacets, xsderr.Loc{},
			"the %s facet, whose {value} is %q, is not applicable to the type of a candidate that is not Scaled (%T), but cos-applicable-facets allows in {facets} only the facets applicable to the type",
			sf.kind, sf.limit, v)
	}
	scale, ok := sc.Scale()
	if !ok {
		return nil
	}
	if sf.violates(scale) {
		return xsderr.New(scaleRule(sf.kind), xsderr.Loc{},
			"value scale %d violates the %s facet limit %s, which %s forbids", scale, sf.kind, sf.limit, scaleRule(sf.kind))
	}
	return nil
}

// violates maps a ·scale· to a violation per kind (clause 1 of each rule). Both
// ·scale· (xsd-precisionDecimal §3.1) and {value} are unbounded integers, and
// cmpBig compares them exactly.
func (sf scaleFacet) violates(scale *big.Int) bool {
	c := sf.limit.cmpBig(scale)
	switch sf.kind {
	case xsd.FacetMaxScale:
		return c < 0
	case xsd.FacetMinScale:
		return c > 0
	default:
		panic(fmt.Sprintf("value: violates: %s is not a scale facet", sf.kind))
	}
}

// scaleRule maps a scale facet kind to its per-facet rule ID
// (xsd-precisionDecimal.md §4.2.3/§4.3.3).
func scaleRule(k xsd.FacetKind) xsderr.Rule {
	switch k {
	case xsd.FacetMaxScale:
		return ruleCvcMaxScaleValid
	case xsd.FacetMinScale:
		return ruleCvcMinScaleValid
	default:
		panic(fmt.Sprintf("value: scaleRule: %s is not a scale facet", k))
	}
}

// facetInt reads a single-valued facet's plain xs:integer {value} (a scale
// bound that MAY be negative — no nonNegativeInteger constraint, unlike
// facetCount), charging rule on a wrong value count or a non-integer literal.
// xs:integer is unbounded (Datatypes §3.4.13), so a literal past math.MaxInt or
// math.MinInt is a valid {value} and is returned whole, never charged.
func facetInt(f xsd.Facet, rule xsderr.Rule) (integerLiteral, error) {
	values := f.Values()
	if len(values) != 1 {
		return "", xsderr.New(rule, xsderr.Loc{},
			"the %s facet carries %d values rather than one, so %s cannot be decided against its {value}", f.Kind(), len(values), rule)
	}
	if _, ok := new(big.Int).SetString(values[0], 10); !ok {
		return "", xsderr.New(rule, xsderr.Loc{},
			"the %s facet's {value} %q is not an integer, so %s cannot be decided against it", f.Kind(), values[0], rule)
	}
	return integerLiteral(values[0]), nil
}

// facetCount reads a single-valued facet's plain xs:nonNegativeInteger {value}
// (a bare count for the digit and length facets), charging rule on a wrong value
// count or a literal that is not an integer or is negative.
// xs:nonNegativeInteger is unbounded (Datatypes §3.4.20), so a literal past
// math.MaxInt is a valid {value} and is returned whole, never charged.
func facetCount(f xsd.Facet, rule xsderr.Rule) (integerLiteral, error) {
	values := f.Values()
	if len(values) != 1 {
		return "", xsderr.New(rule, xsderr.Loc{},
			"the %s facet carries %d values rather than one, so %s cannot be decided against its {value}", f.Kind(), len(values), rule)
	}
	n, ok := new(big.Int).SetString(values[0], 10)
	if !ok || n.Sign() < 0 {
		return "", xsderr.New(rule, xsderr.Loc{},
			"the %s facet's {value} %q is not a nonNegativeInteger, so %s cannot be decided against it", f.Kind(), values[0], rule)
	}
	return integerLiteral(values[0]), nil
}

// integerLiteral is an integer facet {value} held as the literal the document
// wrote, which facetCount or facetInt has checked is an optional sign followed by
// decimal digits. The value spaces of the length, digits and scale facets are
// unbounded, so the literal is never narrowed to a host int: cmpBig compares it
// exactly with the integer an instance measures (cmpInt for an int measure),
// and a rejection names the literal itself.
type integerLiteral string

// cmpInt orders l against n by value: negative, zero or positive as l is less
// than, equal to or greater than n.
func (l integerLiteral) cmpInt(n int) int {
	return l.cmpBig(big.NewInt(int64(n)))
}

// cmpBig orders l against n by value: negative, zero or positive as l is less
// than, equal to or greater than n.
func (l integerLiteral) cmpBig(n *big.Int) int {
	v, ok := new(big.Int).SetString(string(l), 10)
	if !ok {
		// facetCount and facetInt admit only literals SetString reads.
		panic(fmt.Sprintf("value: cmpBig: %q is not an integer literal", string(l)))
	}
	return v.Cmp(n)
}
