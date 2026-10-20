package xsd

import "github.com/kud360/goxsd8/xsderr"

// ruleSchPropsCorrect is Schema Properties Correct (Structures §3.17.6.1,
// id="sch-props-correct"): a schema's properties must match the §3.17.1 Schema
// property tableau. Finalize enforces clause 2 — no two schema components of the
// same kind (the same {…definitions}/{…declarations} property) share an expanded
// name (target namespace + local name), locally decidable without any
// cross-reference resolution — and then runs the resolution pass (resolve.go,
// #173) that discharges cross-component QName resolution (src-resolve, §3.17.6.2)
// and the named-circularity rejections. Clause 1's remaining cross-reference-
// dependent requirements stay deferred; the instance-time cvc-resolve-instance
// (§3.17.6.3) lookups the Query views serve are a still-later consumer's concern.
const ruleSchPropsCorrect xsderr.Rule = "sch-props-correct"

// SchemaBuilder accumulates the schema components discovered during the
// parse/resolve phases (Structures §4.2.3 src-include: an implementation "must
// retain QName values for such references … until an appropriately-named
// component becomes available"). It performs no cross-reference resolution and
// no sch-props-correct enforcement beyond the per-kind duplicate detection
// Finalize runs — construction order is deliberately unconstrained per §4.1
// Layer 1's "lazy/just-in-time" note (§4.2.4 is <redefine>, a distinct topic).
// Call Finalize to obtain the immutable compiled Schema.
//
// INTENDED CALLER: a producer, not an application author. Every AddX takes an
// already-validated component value, and building one correctly means honoring
// every §3 tableau and cross-property invariant its constructor cannot check —
// which is parser.Produce's job (it maps a schema document onto these
// components and calls Finalize). An application that has a schema DOCUMENT
// should call parser.Parse and receive the finalized *Schema; this builder is
// for a producer synthesizing components some other way, and for the tests and
// Examples that need one component graph without a document behind it.
//
// Each slice holds its kind's components in the document order they were added
// (STYLE D2/D3); that order is the source of truth the Finalize indexes are
// derived from, never the reverse.
type SchemaBuilder struct {
	types               []TypeDefinition
	elements            []ElementDeclaration
	attributes          []AttributeDeclaration
	attributeGroups     []AttributeGroupDefinition
	modelGroups         []ModelGroupDefinition
	notations           []Notation
	identityConstraints []IdentityConstraint

	// attributeGroupRedefinitions holds the (redefinition, S2 original) pairings
	// AddRedefiningAttributeGroup records, in the order they were added, for
	// finalize to charge src-redefine clause 7.2.2 over (redefinition.go). It is
	// not a §3.17.1 property: the redefinition itself is in attributeGroups like
	// any other top-level definition, and the original is in no property at all.
	attributeGroupRedefinitions []attributeGroupRedefinition

	// modelGroupRedefinitions is the same for AddRedefiningModelGroup's pairings,
	// which finalize charges src-redefine clause 6.2.2 over (redefinition.go).
	modelGroupRedefinitions []modelGroupRedefinition
}

// NewSchemaBuilder returns an empty accumulating builder.
func NewSchemaBuilder() *SchemaBuilder { return &SchemaBuilder{} }

// AddType appends a top-level type definition in document order.
//
// It panics if t is nil — either a nil TypeDefinition interface value, or a
// non-nil interface wrapping a nil *SimpleType. A nil type definition is a
// caller/producer bug (the wrong constructor or an unchecked error), not a
// schema-validity condition, so a panic — not an xsderr rejection — is the
// right guard, mirroring NewFacet's wrong-constructor panic in simpletype.go.
func (b *SchemaBuilder) AddType(t TypeDefinition) {
	if t == nil {
		panic("xsd: SchemaBuilder.AddType: nil TypeDefinition")
	}
	if st, ok := t.(*SimpleType); ok && st == nil {
		panic("xsd: SchemaBuilder.AddType: nil *SimpleType")
	}
	b.types = append(b.types, t)
}

// AddElement appends a top-level element declaration in document order.
func (b *SchemaBuilder) AddElement(e ElementDeclaration) {
	b.elements = append(b.elements, e)
}

// AddAttribute appends a top-level attribute declaration in document order.
func (b *SchemaBuilder) AddAttribute(a AttributeDeclaration) {
	b.attributes = append(b.attributes, a)
}

// AddAttributeGroup appends a top-level attribute group definition in document
// order.
func (b *SchemaBuilder) AddAttributeGroup(g AttributeGroupDefinition) {
	b.attributeGroups = append(b.attributeGroups, g)
}

// AddModelGroup appends a top-level model group definition in document order.
func (b *SchemaBuilder) AddModelGroup(d ModelGroupDefinition) {
	b.modelGroups = append(b.modelGroups, d)
}

// AddNotation appends a top-level notation declaration in document order.
func (b *SchemaBuilder) AddNotation(n Notation) {
	b.notations = append(b.notations, n)
}

// AddIdentityConstraint appends an identity-constraint definition in document
// order. Alone among SchemaBuilder's adders it is NOT a top-level-only set:
// §3.17.2's XML Mapping Summary for Schema sources {identity-constraint
// definitions} from the <key>, <keyref>, and <unique> element information
// items "anywhere within the [[children]]" — every identity-constraint
// DEFINITION in the schema, at any nesting depth. The contrast is the spec's
// own: the same tableau scopes {element declarations} to "The (top-level)
// element declarations ... in the [[children]]". (§3.17.1 only DECLARES the
// property — "A set of Identity-Constraint Definition components" — and says
// nothing about scope.)
//
// Pass the name= form only. A <key ref="…">/<keyref ref="…">/<unique ref="…">
// defines NOTHING: per §3.11.2 "the corresponding schema component is the
// identity-constraint definition ·resolved· to by the ·actual value· of the ref
// [[attribute]]", so it contributes that EXISTING definition and must not be
// passed here — a second registration under the same name would fabricate a
// sch-props-correct (§3.17.6.1) clause 2 duplicate-expanded-name violation
// against the very definition it reuses.
//
// A producer that instead registers only its top-level definitions leaves the
// nested ones out of the property src-resolve (§3.17.6.2) clause 1.7 resolves
// identity-constraint QNames against: a nested <keyref>'s refer= target becomes
// unfindable and a valid schema is false-rejected at Finalize.
func (b *SchemaBuilder) AddIdentityConstraint(c IdentityConstraint) {
	b.identityConstraints = append(b.identityConstraints, c)
}

// Schema is the finalized, immutable compiled schema set (Structures §3.17.1,
// assembled per §4.2.1's "schema(D)"). It is constructible ONLY via
// SchemaBuilder.Finalize or SchemaBuilder.FinalizeWith: its fields are
// unexported and it has no other constructor, so a not-yet-finalized accumulator
// can never be handed off as a finalized Schema (STYLE T1) — "not finalized"
// (SchemaBuilder) and "finalized" (Schema) are distinct Go types, not two states
// of one type. The two entry points differ only in whether a [ValueSpace] is
// installed.
//
// *Schema is the Query API (xsd/doc.go): it satisfies TypeResolver,
// ElementResolver, and AttributeResolver through its Type, Element, and
// Attribute methods. Go's structural typing leaves that unprinted by go doc, so
// it is stated here — a consumer needing only one of the three takes the
// matching capability view rather than the whole *Schema (STYLE T3). Those
// three by-name lookups copy nothing; the seven document-order enumerators
// (Types, Elements, Attributes, AttributeGroups, ModelGroups, Notations,
// IdentityConstraints) each return a COPY of their slice, so no
// caller holds an aliasing handle with which to mutate a source-of-truth slice
// out of step with the index derived from it (STYLE T1).
//
// The document-order slices are the source of truth; the by-expanded-QName maps
// are indexes DERIVED from those slices at Finalize and exist only for O(1)
// lookup — they never determine iteration order (STYLE D2/D3; see xsd/doc.go's
// "Maps exist only as internal lookup indexes and never determine order").
//
// Cross-reference resolution (src-resolve §3.17.6.2) is a VALIDATION pass run at
// Finalize (resolve.go, #173): it verifies every retained QName reference
// resolves against these indexes and that no spec-forbidden circularity exists,
// but it stores no resolved-component pointer — a consumer follows a reference by
// a read-time index lookup (schema.Type/Element/Attribute), because a stored
// pointer would be state derivable from the QName plus the index (STYLE D3). The
// remaining sch-props-correct clause-1 requirements stay deferred; Finalize's own
// duplicate-name check is sch-props-correct §3.17.6.1 clause 2, locally decidable
// without any cross-reference resolution.
type Schema struct {
	types               []TypeDefinition
	elements            []ElementDeclaration
	attributes          []AttributeDeclaration
	attributeGroups     []AttributeGroupDefinition
	modelGroups         []ModelGroupDefinition
	notations           []Notation
	identityConstraints []IdentityConstraint

	// attributeGroupRedefinitions carries the builder's <attributeGroup> pairings
	// across finalize; checkAttributeGroupRedefinitions (redefinition.go) charges
	// clause 7.2.2 over them and resolveReferences (resolve.go) walks each
	// original's {attribute uses} for src-resolve.
	attributeGroupRedefinitions []attributeGroupRedefinition

	// modelGroupRedefinitions carries the builder's <group> pairings across
	// finalize; checkModelGroupRedefinitions (redefinition.go) charges clause
	// 6.2.2 over them and resolveReferences (resolve.go) walks each original's
	// {model group} for src-resolve.
	modelGroupRedefinitions []modelGroupRedefinition

	typeIndex           map[QName]TypeDefinition
	elementIndex        map[QName]ElementDeclaration
	attributeIndex      map[QName]AttributeDeclaration
	attributeGroupIndex map[QName]AttributeGroupDefinition
	modelGroupIndex     map[QName]ModelGroupDefinition
	notationIndex       map[QName]Notation
	idcIndex            map[QName]IdentityConstraint

	// valueSpace answers the value-space questions this package cannot answer
	// itself (see ValueSpace): the {value} comparisons au-props-correct clause 3
	// and loc-testSubP clauses 4.2/5.2.2 ask, and the Simple Default Valid
	// (§3.2.6.2) verdict a-props-correct clause 2 and au-props-correct clause 2
	// ask. It is never nil: Finalize installs undecidedValueSpace{}, so every
	// consumer calls it unconditionally.
	valueSpace ValueSpace

	// restrictionChecker charges the facet-VALUE half of cos-st-restricts (see
	// SimpleTypeRestrictionChecker), which this package cannot decide either: it
	// needs the generated per-primitive applicability table and a lexical→value
	// mapping. checkSimpleTypeDerivations (resolve.go) puts every simple type the
	// schema reaches to it. It is never nil: Finalize installs
	// undecidedRestrictionChecker{}, so that pass calls it unconditionally.
	restrictionChecker SimpleTypeRestrictionChecker
}

// Finalize builds the immutable Schema from the accumulated components. It
// copies each document-order slice onto fresh backing arrays — so the builder
// stays independently usable afterward, decoupled from the returned Schema —
// and builds each by-expanded-name index over the copy. The *SimpleType
// pointees a type-definition slice holds are shared, NOT deep-copied: pointer
// identity is load-bearing for SimpleType (see its doc), so the compiled Schema
// must reference the very same nodes.
//
// The two results are exclusive: on any rejection the returned *Schema is NIL,
// so a caller must test the error before dereferencing. A partially built
// Schema is never handed back — the resolution pass runs on an already-assembled
// value that is dropped when it fails.
//
// It rejects, charging Schema Properties Correct (§3.17.6.1, sch-props-correct)
// clause 2: two components of the same kind (the same §3.17.1 {…definitions}/
// {…declarations} property) sharing an expanded name — target namespace plus
// local name. The scan is deterministic (STYLE D2): each kind's slice is walked
// in document order and each name tested against a seen-set map, so the first
// duplicate by index is the one reported (the map is never ranged to produce
// the verdict). The rejection is charged to the LATER of the two — the
// duplicate at the higher document-order index, whose own Loc is the position a
// reader must edit — and names the first occurrence's position in the message.
// A component built with no real parser position (a synthesized or seeded one)
// reports the zero xsderr.Loc, which renders as "?". Components whose {name} is
// ABSENT (the zero QName) take no part in that check and enter no index:
// anonymous type definitions are exempt from the uniqueness requirement (§3.4.1,
// §3.16.1) and belong to no §3.17.1 by-name symbol table, so any number of them
// may be added and none is reachable through Type.
//
// After the indexes are built, Finalize runs the resolution pass (resolve.go):
// it walks the assembled components in document order and rejects any
// unresolvable QName reference (src-resolve, §3.17.6.2) or spec-forbidden named
// circularity. HARD-FAIL POLICY: §5.3 (Missing Sub-components) permits an
// unresolved *required* reference to degrade assessment to lax rather than
// reject the schema; goxsd8 deliberately hard-fails instead, as the right stance
// for a conformance processor validated against the W3C test suite. That is an
// implementation policy choice, not something §5.3 mandates.
//
// Every OTHER sch-props-correct clause (in particular clause 1's remaining
// cross-reference-dependent requirements) stays deferred to later passes.
//
// Finalize installs NEITHER of the two capabilities the resolution pass takes as
// inputs, so every question that would go to one goes unanswered. No value
// space: the {value}-identity comparisons (au-props-correct §3.5.6 clause 3,
// loc-testSubP §3.4.6.4 clauses 4.2 and 5.2.2) and the Simple Default Valid
// checks (§3.2.6.2, charged by a-props-correct §3.2.6.1 clause 2 and
// au-props-correct clause 2) are undecided and fail open. No
// [SimpleTypeRestrictionChecker]: the facet-VALUE half of cos-st-restricts
// (§3.16.6.2) is charged against no simple type at all. Both are the behavior
// this entry point has always had. A caller that can supply the lexical→value
// mapping and the per-primitive applicability table calls
// [SchemaBuilder.FinalizeWith] instead.
func (b *SchemaBuilder) Finalize() (*Schema, error) {
	return b.finalize(undecidedValueSpace{}, undecidedRestrictionChecker{})
}

// FinalizeWith is [SchemaBuilder.Finalize] with both finalize-time capabilities
// installed. vs answers the value-space questions package xsd cannot (see
// [ValueSpace]), so the resolution pass can decide au-props-correct (§3.5.6)
// clause 3, loc-testSubP (§3.4.6.4) clauses 4.2 and 5.2.2, and Simple Default
// Valid (§3.2.6.2) under a-props-correct (§3.2.6.1) clause 2 and au-props-correct
// clause 2, instead of waving them through. rc answers the other one (see
// [SimpleTypeRestrictionChecker]), so checkSimpleTypeDerivations can charge the
// facet-VALUE half of Derivation Valid (Restriction, Simple) (§3.16.6.2,
// cos-st-restricts) against every simple type the schema reaches. It is otherwise
// identical to Finalize — same components, same indexes — and can only NARROW
// what is accepted, never widen it: vs reports "undecided" wherever it cannot
// decide and undecided is accept, and rc returns a nil error wherever it cannot
// judge.
//
// One component fault the value space still does NOT diagnose, whatever is
// installed: an atomic or list *[SimpleType] carrying no whiteSpace facet in
// force at all, where Structures §3.16.7.4 and Datatypes §4.3.6.1 guarantee one
// (a union is exempt, not faulty: §4.1.5 leaves whiteSpace out of its applicable
// set). What a caller does NOT get is a wrong answer: a value space asked to
// validate a default against such a type reports the fault undecided rather than
// as a verdict, so this entry point neither rejects the schema for it nor fails
// (see checkSimpleDefault, valueconstraintvalid.go). The sibling fault — a facet
// that is not applicable to its type (cos-applicable-facets, Datatypes §4.1.5) —
// used to be undiagnosable here too and no longer is: its ATOMIC case is exactly
// what rc charges, over the whole reachable simple-type graph, and the list and
// union cases were already decided by checkVarietyApplicableFacets
// (derivation.go) at construction.
//
// It is a second entry point rather than a pair of setters on the builder because
// both are finalize-time INPUTS, not accumulated schema state: a mutable setter
// would make "builder with a capability installed" and "builder without" two
// states of one type, which is exactly what the SchemaBuilder/Schema split exists
// to avoid (STYLE T1). For the same reason the two arrive at ONE seam and not
// two: a third entry point, or an options struct whose nil members were legal,
// would make a partially installed schema representable.
//
// It panics if EITHER vs or rc is nil, on the same grounds as
// [SchemaBuilder.AddType]'s nil guard: a nil capability is a caller/producer bug,
// not a schema-validity condition.
func (b *SchemaBuilder) FinalizeWith(vs ValueSpace, rc SimpleTypeRestrictionChecker) (*Schema, error) {
	if vs == nil {
		panic("xsd: SchemaBuilder.FinalizeWith: nil ValueSpace")
	}
	if rc == nil {
		panic("xsd: SchemaBuilder.FinalizeWith: nil SimpleTypeRestrictionChecker")
	}
	return b.finalize(vs, rc)
}

// finalize is the one assembly path both entry points take; they differ only in
// the capabilities they install (STYLE T4).
func (b *SchemaBuilder) finalize(vs ValueSpace, rc SimpleTypeRestrictionChecker) (*Schema, error) {
	typeIndex, err := indexByName(b.types, "type definitions")
	if err != nil {
		return nil, err
	}
	elementIndex, err := indexByName(b.elements, "element declarations")
	if err != nil {
		return nil, err
	}
	attributeIndex, err := indexByName(b.attributes, "attribute declarations")
	if err != nil {
		return nil, err
	}
	attributeGroupIndex, err := indexByName(b.attributeGroups, "attribute group definitions")
	if err != nil {
		return nil, err
	}
	modelGroupIndex, err := indexByName(b.modelGroups, "model group definitions")
	if err != nil {
		return nil, err
	}
	notationIndex, err := indexByName(b.notations, "notation declarations")
	if err != nil {
		return nil, err
	}
	idcIndex, err := indexByName(b.identityConstraints, "identity-constraint definitions")
	if err != nil {
		return nil, err
	}
	s := &Schema{
		types:               cloneSlice(b.types),
		elements:            cloneSlice(b.elements),
		attributes:          cloneSlice(b.attributes),
		attributeGroups:     cloneSlice(b.attributeGroups),
		modelGroups:         cloneSlice(b.modelGroups),
		notations:           cloneSlice(b.notations),
		identityConstraints: cloneSlice(b.identityConstraints),

		attributeGroupRedefinitions: cloneSlice(b.attributeGroupRedefinitions),
		modelGroupRedefinitions:     cloneSlice(b.modelGroupRedefinitions),

		typeIndex:           typeIndex,
		elementIndex:        elementIndex,
		attributeIndex:      attributeIndex,
		attributeGroupIndex: attributeGroupIndex,
		modelGroupIndex:     modelGroupIndex,
		notationIndex:       notationIndex,
		idcIndex:            idcIndex,
		valueSpace:          vs,
		restrictionChecker:  rc,
	}
	if err := s.resolve(); err != nil {
		return nil, err
	}
	return s, nil
}

// namedComponent is what indexByName needs from a schema-level component kind:
// the expanded {name} to key its index by, and the source position to charge a
// duplicate rejection to. Every kind a schema's §3.17.1 properties hold
// satisfies it, including the TypeDefinition sum — which promotes both methods
// so its two variants stay on the one generic code path with no type switch
// (STYLE T2/T4). It stays unexported: it is a generic constraint, not a
// capability view a consumer takes (STYLE T5).
type namedComponent interface {
	Name() QName
	Loc() xsderr.Loc
}

// indexByName builds the by-expanded-name lookup index for one kind's
// document-order slice, rejecting sch-props-correct (§3.17.6.1) clause 2: two
// components of this kind sharing an expanded name. The slice is walked in
// document order (STYLE D2), so the first duplicate by index is the one
// reported; kind names the §3.17.1 property for the message. An empty slice
// yields a nil map (a nil map reads as a miss, which is the correct lookup
// behavior). The namedComponent constraint supplies both accessors it needs —
// the {name} to key by and the Loc to cite — keeping every kind on one code
// path (STYLE T4). The rejection is charged to the LATER (duplicate) component's
// own Loc and names the first occurrence's Loc in the message; both positions
// come from the walked slice, never from ranging the index map (STYLE D2).
//
// INVARIANT: the returned index holds only components whose {name} is PRESENT.
// A component with an absent {name} — the zero QName, the sound encoding of
// absence since NCName forbids an empty local name (see QName) — is skipped
// entirely: not keyed, not compared for duplicates. Anonymous type definitions
// are explicitly exempt from clause 2's uniqueness requirement (§3.4.1 and
// §3.16.1: "Except for anonymous … type definitions (those with no {name}) …"),
// and structurally they never belong in the §3.17.1 by-name symbol tables at
// all, so two of them are not a collision and neither is reachable by name.
func indexByName[T namedComponent](items []T, kind string) (map[QName]T, error) {
	if len(items) == 0 {
		return nil, nil
	}
	index := make(map[QName]T, len(items))
	for i, item := range items {
		n := item.Name()
		if n == (QName{}) {
			continue // absent {name}: anonymous, exempt from clause 2 (§3.4.1/§3.16.1)
		}
		if first, dup := index[n]; dup {
			return nil, xsderr.New(ruleSchPropsCorrect, item.Loc(),
				"schema {%s}[%d] repeats the expanded name %s (first declared at %s), but sch-props-correct clause 2 forbids two components of the same kind sharing an expanded name", kind, i, n, first.Loc())
		}
		index[n] = item
	}
	return index, nil
}

// cloneSlice returns a copy of s on a fresh backing array, holding an empty
// input as nil (this package's standing empty-is-nil convention). It is used by
// Finalize to decouple the compiled Schema's slices from the builder's.
func cloneSlice[T any](s []T) []T {
	if len(s) == 0 {
		return nil
	}
	return append([]T(nil), s...)
}

// ElementResolver is the minimal capability a consumer needs to look up a
// top-level element declaration by its expanded name (STYLE T3). Its consumers
// are validate.SchemaView, which embeds it, and element-reference resolution
// during finalize (src-resolve clause 1.3).
type ElementResolver interface {
	Element(name QName) (ElementDeclaration, bool)
}

// AttributeResolver is the minimal capability a consumer needs to look up a
// top-level attribute declaration by its expanded name (STYLE T3). Its
// consumers are validate.SchemaView, which embeds it, and attribute-use
// resolution during finalize (#173).
type AttributeResolver interface {
	Attribute(name QName) (AttributeDeclaration, bool)
}

// TypeResolver is the minimal capability a consumer needs to look up a
// top-level type definition (simple or complex) by its expanded name (STYLE
// T3). [*Schema] implements it over a finalized schema; parser answers one over
// the built-in datatypes alone, before any *Schema exists. Its consumers are
// this package and packages value, builtin, xpath, and validate.
type TypeResolver interface {
	Type(name QName) (TypeDefinition, bool)
}

// *Schema implements every Query capability view; these assertions keep that
// promise checked at compile time. It is not the only TypeResolver in the
// module: the parser answers one over the built-in datatypes ALONE, at
// construction time, for the ·in-scope schema definitions· §3.13.6.2 xpath-valid
// clause 2.2.5 fixes for a Type Alternative {test} — no finalized Schema exists
// at that point, and clause 2.2.5 would not put its types in scope anyway.
var (
	_ ElementResolver   = (*Schema)(nil)
	_ AttributeResolver = (*Schema)(nil)
	_ TypeResolver      = (*Schema)(nil)
)

// Element returns the top-level element declaration with the given expanded
// name and true, or the zero ElementDeclaration and false when none is
// declared. It is a read-only window onto the compiled set (§3.17.1 {element
// declarations}); it copies nothing.
func (s *Schema) Element(name QName) (ElementDeclaration, bool) {
	d, ok := s.elementIndex[name]
	return d, ok
}

// Attribute returns the top-level attribute declaration with the given expanded
// name and true, or the zero AttributeDeclaration and false when none is
// declared. It is a read-only window onto the compiled set (§3.17.1 {attribute
// declarations}); it copies nothing.
func (s *Schema) Attribute(name QName) (AttributeDeclaration, bool) {
	d, ok := s.attributeIndex[name]
	return d, ok
}

// Type returns the top-level type definition (simple or complex) with the given
// expanded name and true, or nil and false when none is declared. It is a
// read-only window onto the compiled set (§3.17.1 {type definitions}); it
// copies nothing.
func (s *Schema) Type(name QName) (TypeDefinition, bool) {
	d, ok := s.typeIndex[name]
	return d, ok
}

// ModelGroup returns the top-level model group definition with the given
// expanded name and true, or the zero ModelGroupDefinition and false when
// none is declared. It is a read-only window onto the compiled set (§3.17.1
// {model group definitions}); it copies nothing.
func (s *Schema) ModelGroup(name QName) (ModelGroupDefinition, bool) {
	d, ok := s.modelGroupIndex[name]
	return d, ok
}

// Types returns the {type definitions} property (§3.17.1): every TOP-LEVEL type
// definition, simple and complex alike, as the one TypeDefinition slice the one
// spec property is — it is never split by variety.
//
// The components are returned in the order they were added to the builder, and
// that order is a GUARANTEED stable part of this method's contract, even though
// §3.17.1 words the property as an unordered "A set of Type Definition
// components".
//
// The SLICE is copied: mutating the result does not affect s. Its ELEMENTS are
// not copied — the components are shared with s and immutable, and a
// *SimpleType's pointer identity is preserved (cloneSlice copies interface
// values, never pointees), which is load-bearing per [SimpleType] and Finalize.
//
// For a schema obtained from parser.Parse, the leading entries are components
// no schema document declares: parser.Produce (parser/produce.go) calls AddType
// for every seeded builtin simple type and for the synthesized xs:anyType
// BEFORE any document type, so the order is builtins first, then each
// document's top-level types in document order.
//
// It is the §3.17.1 property and nothing more — NOT transitive coverage of
// every type reachable from the schema. An inline (nested) type definition
// owned by a declaration is never added to this set, so a consumer needing
// those must walk the declarations itself. Anonymous top-level type definitions
// — {name} absent — ARE included even though Type cannot reach them: §3.4.1 and
// §3.16.1 exempt them from the by-name symbol table (see indexByName).
//
// An empty {type definitions} yields nil.
func (s *Schema) Types() []TypeDefinition {
	return cloneSlice(s.types)
}

// Elements returns the {element declarations} property (§3.17.1): the top-level
// element declarations, in the order they were added to the builder. That order
// is a guaranteed stable part of this method's contract, even though §3.17.1
// words the property as an unordered "A set of Element Declaration components".
//
// The slice is copied: mutating the result does not affect s. The declarations
// in it are shared with s and immutable. An empty {element declarations} yields
// nil.
func (s *Schema) Elements() []ElementDeclaration {
	return cloneSlice(s.elements)
}

// Attributes returns the {attribute declarations} property (§3.17.1): the
// top-level attribute declarations, in the order they were added to the
// builder. That order is a guaranteed stable part of this method's contract,
// even though §3.17.1 words the property as an unordered "A set of Attribute
// Declaration components".
//
// The slice is copied: mutating the result does not affect s. The declarations
// in it are shared with s and immutable. An empty {attribute declarations}
// yields nil.
//
// For a schema obtained from parser.Parse, the LEADING entries are declarations
// no schema document declares, and there are four or eight of them. The parser
// calls AddAttribute for each of §3.2.7's built-in xsi: declarations — xsi:type,
// xsi:nil, xsi:schemaLocation and xsi:noNamespaceSchemaLocation, "present in
// every schema by definition" — BEFORE any document declaration, always. It
// then adds the four the schema document for the XML namespace declares —
// xml:lang, xml:space, xml:base and xml:id — for a schema that <import>s
// http://www.w3.org/XML/1998/namespace and composes no document under it, which
// is the §1.3.2 namespace this processor supplies rather than fetches. So the
// order is those four, then those four when they are supplied, then each
// document's top-level attributes in document order.
//
// Tell the supplied ones from a document's by their Loc, which is the zero Loc
// for a component no schema document is behind (see this package's doc);
// counting on their PRESENCE is what the condition above forbids.
func (s *Schema) Attributes() []AttributeDeclaration {
	return cloneSlice(s.attributes)
}

// AttributeGroups returns the {attribute group definitions} property (§3.17.1):
// the top-level attribute group definitions, in the order they were added to
// the builder. That order is a guaranteed stable part of this method's
// contract, even though §3.17.1 words the property as an unordered "A set of
// Attribute Group Definition components".
//
// The slice is copied: mutating the result does not affect s. The definitions
// in it are shared with s and immutable. An empty {attribute group definitions}
// yields nil.
func (s *Schema) AttributeGroups() []AttributeGroupDefinition {
	return cloneSlice(s.attributeGroups)
}

// ModelGroups returns the {model group definitions} property (§3.17.1): the
// top-level model group definitions, in the order they were added to the
// builder. That order is a guaranteed stable part of this method's contract,
// even though §3.17.1 words the property as an unordered "A set of Model Group
// Definition components".
//
// The slice is copied: mutating the result does not affect s. The definitions
// in it are shared with s and immutable. An empty {model group definitions}
// yields nil.
func (s *Schema) ModelGroups() []ModelGroupDefinition {
	return cloneSlice(s.modelGroups)
}

// Notations returns the {notation declarations} property (§3.17.1): the
// top-level notation declarations, in the order they were added to the builder.
// That order is a guaranteed stable part of this method's contract, even though
// §3.17.1 words the property as an unordered "A set of Notation Declaration
// components".
//
// The slice is copied: mutating the result does not affect s. The declarations
// in it are shared with s and immutable. An empty {notation declarations}
// yields nil.
func (s *Schema) Notations() []Notation {
	return cloneSlice(s.notations)
}

// IdentityConstraints returns the SCHEMA-level {identity-constraint
// definitions} property (§3.17.1): every identity-constraint definition in the
// schema, at any nesting depth, in the order they were added to the builder.
// The depth is the spec's — §3.17.2 sources the property from the <key>,
// <keyref>, and <unique> element information items "anywhere within the
// [[children]]", not from top-level ones alone, so a definition declared under a
// local element declaration is a member exactly as a top-level one is (see
// [SchemaBuilder.AddIdentityConstraint], which also carves out the ref= reuse
// form).
//
// That order is a guaranteed stable part of this method's contract, even though
// §3.17.1 words the property as an unordered "A set of Identity-Constraint
// Definition components". It is a different scope from
// [ElementDeclaration.IdentityConstraints], which reports one element
// declaration's own §3.3.1 property.
//
// The slice is copied: mutating the result does not affect s. The definitions
// in it are shared with s and immutable. An empty {identity-constraint
// definitions} yields nil.
func (s *Schema) IdentityConstraints() []IdentityConstraint {
	return cloneSlice(s.identityConstraints)
}
