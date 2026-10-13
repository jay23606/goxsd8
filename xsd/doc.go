// Package xsd is the XSD 1.1 component model: the immutable, in-memory
// representation of a compiled schema set.
//
// It is a pure leaf — it imports only xsderr and the stdlib. Parsing
// lives in package parser; values in package value; this package is the
// shape those operate on.
//
// # Components
//
// The model covers the component kinds of Structures §2.2: simple and
// complex type definitions, element and attribute declarations, attribute
// and model groups, particles, wildcards, identity constraints, type
// alternatives (CTA), assertions, and notations. Structures §3.15
// {annotation} is not modelled: no cvc-/src-/cos- rule consumes it, so no
// component carries it and no accessor reports it (#407).
//
// Design rules (see docs/STYLE.md, docs/PRINCIPLES.md):
//
//   - Immutable after construction. Construction happens in phases
//     (parse → resolve → finalize) so no traversal ever needs a cycle
//     check (D4); spec-forbidden circularities are rejected at finalize
//     with their named src-/cos- rule. That covers the SIMPLE-type
//     {base type definition} chain too, now that the slot holds a
//     deferred reference (SimpleTypeOrRef): st-props-correct clause 2 is
//     charged by a finalize check like its complex-type twin, and the
//     unguarded chain walks behind Variety/Primitive/Item/Members/
//     EffectiveFacets rest on it having run — see Schema.resolve. The
//     {item type definition} and {member type definitions} slots hold
//     that same sum, so an itemType= or memberTypes= naming a
//     forward-declared type defers exactly as a base= does.
//
//   - Safe for concurrent use by multiple goroutines once
//     SchemaBuilder.Finalize or FinalizeWith returns: no *Schema method
//     writes any field, so a compiled schema shared across request
//     handlers needs no lock. The one exception is Schema.ContentMatcher's
//     return value — a *Matcher is single-use and stateful, and its own
//     doc says so.
//
//   - Every child collection is a slice in document order. Maps exist
//     only as internal lookup indexes and never determine order (D2).
//
//   - One fact, one encoding (D3): nothing derivable is stored. There is
//     no Primitive bool — a type that defines its own fundamental facets
//     is a primitive, answered by IsPrimitive(); effective facets are
//     computed on demand by merging the base chain, never cached.
//
//   - Closed sets (variety, derivation method, process-contents, use)
//     are typed constants with unexported tags, never strings (T1).
//
// Names are expanded QNames:
//
//	type QName struct{ Space, Local string }
//
// The zero value means absent/anonymous; String() renders Clark notation
// ("{ns}local").
//
// A name identifies a component that HAS one. An anonymous component has
// none, so where one component must point at another that may be
// anonymous — or at a LOCAL declaration, which is not name-unique — the
// reference carries a ComponentID instead: an opaque identity minted by
// the producer with NewComponentID before either endpoint is built, and
// threaded into both. It is compared with ==, never rendered (its
// underlying value is an address, so any textual or sorted form would be
// nondeterministic, D1/D2), and never derived from position — Loc is
// provenance, not identity. ComplexType's {context} (§3.4.1) is the first
// slot to use it, and the reciprocal second is the anonymous-container arm
// of each scope-parent sum — ElementScopeParent's
// AnonymousComplexTypeScopeParent (§3.3.1 sc_e) and AttributeScopeParent's
// AttributeAnonymousComplexTypeScopeParent (§3.2.1 sc_a). All three hold
// the SAME token: one mint per OWNERSHIP EDGE serves the back-pointers
// and the forward reference alike. A chained §4.2.4 <redefine>, whose
// anonymous originals nest, stacks two such edges and mints one token
// for each, so the containers stay distinguishable.
//
// The eight component kinds a schema's §3.17.1 properties hold — element
// and attribute declarations, complex and simple type definitions, model
// group and attribute group definitions, notations, and identity
// constraints — additionally report a source position through Loc. That
// position is PROVENANCE, not a component property at all — no kind's
// §3.x.1 property list has it. It is where the declaring element sits in
// the schema document, retained from the constructor so a finalize-time
// rejection can cite file:line:column instead of "?" — sch-props-correct
// clause 2 (schema.go) was the first such consumer, resolve.go's Phase
// A/B rejections (src-resolve, c-props-correct, ct-props-correct,
// mg-props-correct, e-props-correct) the second, defaultbinding.go's
// derivation-ok-restriction clause 3 rejections the third, and
// complexderivation.go's derivation-ok-restriction, ct-props-correct and
// cos-ct-extends rejections the fourth, and allgrouplimited.go's
// cos-all-limited rejections the fifth: each is charged to the REFERRING
// or offending component's own Loc, per the referrer-Loc convention
// documented on resolveReferences — for every derivation-ok-restriction
// clause that is the RESTRICTING complex type, the spec's T, which the
// constraint is stated against and every one of its messages already
// names, and for ct-props-correct the complex type whose properties it
// checks. cos-ct-extends clause 1.6 (c-vs-ctd-e) is positioned by the
// same two charge sites: derivation-ok-restriction clause 4's attribute
// and element halves are parameterized by locallyDeclaredTypeCheck, whose
// second literal is checkExtensionLocallyDeclaredTypes' — so a c-vs-ctd-e
// rejection carries the EXTENDING type's Loc, charged from
// complexderivation.go and NOT from any of complexextension.go's own
// twenty charges. The zero xsderr.Loc means the position is unknown, and
// is the correct value for a component with no schema document behind it —
// parser.Produce's synthesized xs:anyType and §3.2.7 xsi: attribute
// declarations, the four xml: attribute declarations and the
// xml:specialAttrs attribute group definition the parser supplies for an
// <import> of the §1.3.2 XML namespace it cannot fetch, and package
// builtin's seeded built-in datatypes, are the legitimate zero-Loc
// producers. Other constructors take a loc to charge their own rejections
// but do not retain it: nothing consumes those positions yet, so no
// accessor is exported for them (T5).
//
// # Query API
//
// Direct lookups over a compiled schema set — element, attribute, and
// type definitions by QName — exposed as minimal capability views (T3):
// a consumer that needs only element lookup receives an interface with
// exactly that method, not the whole schema. Those by-QName views are
// read-only windows onto the compiled set; they never copy it.
//
// The views are ElementResolver, AttributeResolver, and TypeResolver;
// *Schema satisfies all three, and SchemaBuilder.Finalize (or its sibling
// SchemaBuilder.FinalizeWith) is the only way to obtain a *Schema. A view
// itself may have other implementations: parser answers a TypeResolver
// over the built-in datatypes alone, before any *Schema exists.
//
// Alongside them *Schema enumerates each of the seven §3.17.1 properties
// it models in document order — Types, Elements, Attributes,
// AttributeGroups, ModelGroups, Notations, IdentityConstraints. Unlike the
// by-QName views these DO copy: each returns a fresh slice (the
// components in it are shared and immutable), so a caller cannot mutate
// through the result and desync a source-of-truth slice from the index
// derived from it. They are methods on *Schema and nothing more; no
// capability view bundles them until a second consumer needs one (T5).
//
// Two SLOT resolvers sit beside the by-QName lookups, for a consumer
// holding a component rather than a name: Schema.ResolvedType turns a
// TypeDefinitionOrRef — an element or attribute declaration's {type
// definition}, a complex type's {base type definition} — into the
// component it denotes, and Schema.ResolvedAttributeDeclaration does the
// same for an attribute use's {attribute declaration}. Both report (T,
// false) for a slot that is absent or names nothing, which is "not
// decidable here" and never a violation. AttributeUse.DeclarationName
// answers the expanded-name half of the second one without resolving
// anything, so a consumer comparing names alone cannot fail.
//
// This section is shipped surface — see the package Examples for the
// construct → Finalize → query sequence.
//
// # Mapping rules
//
// One §3.4.2 mapping rule is exported, and only because it has two
// callers on opposite sides of the parse/finalize line:
// ExtensionContentType is Structures §3.4.2.3.3 clause 4.2, the merge of
// a base's {content type} with an extension's ·effective content·. The
// producer applies it to one <extension> element at parse time;
// cos-ct-extends clause 1.5 applies it repeatedly at finalize, over a
// derivation chain re-ordered into an order no schema document
// expresses. It takes its <group ref> lookup as a parameter because
// those two callers resolve one differently, not because it is an
// extension point. Every other mapping rule this package completes —
// the §3.6.2.1/§3.6.2.2 attribute group fold, the §3.4.2.4 clause 3 and
// §3.4.2.5 clause 2 folds, and this one's structural inverse — stays
// in-package (T5).
//
// # Value spaces
//
// A few finalize-time constraints reach into a value space. Two COMPARE
// {value}s — au-props-correct (§3.5.6) clause 3, loc-testSubP (§3.4.6.4)
// clauses 4.2 and 5.2.2 — and two ask whether one {lexical form} denotes a
// value of its type at all: a-props-correct (§3.2.6.1) clause 2 and
// au-props-correct clause 2, both charging Simple Default Valid (§3.2.6.2).
// A {value} is an ·actual value·, not a lexical string, and mapping a
// lexical to a value needs package value, which is layered above this pure
// leaf — so the whole capability is taken as an INPUT: the ValueSpace
// interface, passed to SchemaBuilder.FinalizeWith. Every ValueSpace method
// may answer "undecided", and undecided always accepts, so plain Finalize
// (which installs undecidedValueSpace) is the fully fail-open configuration.
// value.NewValueSpace is the implementation the parser installs.
//
// One exported method takes a ValueSpace PER CALL instead of reading the
// installed one: Schema.ElementDefaultValid, whose caller charges
// cos-valid-default during instance assessment rather than at finalize and
// holds the space its own backend defines. A schema finalized without one
// answers every question undecided, so reading the installed space there
// would make that charge permanently undecidable.
//
// FinalizeWith installs a SECOND capability at the same seam, and it is
// deliberately a second interface rather than a fourth ValueSpace method:
// SimpleTypeRestrictionChecker charges the facet-VALUE half of
// cos-st-restricts (§3.16.6.2) — the atomic applicability clause, whose
// per-primitive applicable sets come from a generated table, and the bound
// and enumeration {value} constraints, which need a value space. That one is
// REJECT-CAPABLE: it answers with an error carrying a per-facet rule ID,
// which finalize returns as the schema's rejection, so bundling it with the
// fail-open comparisons would make an incoherent capability mix
// representable (T1). checkSimpleTypeDerivations puts every simple type
// the assembled schema reaches to it, anonymous inline ones included;
// builtin.NewRestrictionChecker is the implementation the parser installs.
//
// Both capabilities' methods take a TypeResolver and none may store one: a
// simple type's {base type definition} may be deferred by name, so every
// question either capability answers means walking a chain only a resolver
// can follow, and an implementation built once must serve whichever schema
// finalize hands it. The same pass also puts every simple type to
// SimpleType.CheckDerivation first — the GRAPH half of cos-st-restricts,
// which this package owns and which the constructors charged while the
// base was a live pointer.
//
// # Walk API
//
// Traversal of a type's effective content model. The reusable core is an
// algebra — type-derivation validity, substitution-group acceptance,
// wildcard admission (one canonical implementation per kind of wildcard,
// on *Schema because clauses 2-3 resolve the defined/sibling keywords
// against the declaration graph), attribute-use lookup. Of that algebra
// cvc-wildcard admission is reached two ways, by kind: ELEMENTS through
// Schema.ContentMatcher, whose Matcher asks it as the wildcard arm of
// cvc-accept and reports the answer as an Attribution, and ATTRIBUTES
// through Schema.AllowsAttributeWildcardName, there being no attribute-side
// matcher to carry one. Nothing exported answers clause 1 alone about a
// Wildcard: NamespaceConstraint.AllowsName and NamespaceConstraint.AllowsNamespace
// sit beneath both as the {namespace constraint} property's own accessors, naming
// the narrower rule (cvc-wildcard-name, §3.10.4.2) they decide, and a caller
// admitting a name reaches for one of the two above instead.
// Schema.ValidlySubstitutable is the derivation half's one exported entry point, for
// the instance-side reader of key-val-sub-type that cvc-elt clause 4 and
// key-governing-type-elem clause 6 need (key-overrides), and that cvc-complex-type
// clause 5 asks against the ·locally declared type·
// Schema.LocallyDeclaredElementType answers (key-ldt-elem).
// Schema.ContainedElementDeclarations is the set that definition's case 2 reads, for
// validate/jsonsrc, which classifies a JSON member by local name against it.
// Schema.ElementDefaultValid is cos-valid-default's, for the one cvc-elt clause
// 5.1.1 charges at assessment time over a type an xsi:type supplied rather than over
// any declaration's. The rest is in-package machinery finalize drives.
//
// One of the two drivers designed on that algebra ships: Matcher,
// validation's pull driver. Schema.ContentMatcher returns one over a
// complex type's {content type} particle, and Matcher.Next advances it one
// child at a time, reporting what that child is ·attributed to· as an
// Attribution — the sealed sum of ElementDeclaration and Wildcard, the two
// kinds of particle §3.4.4.4 attributes an item to, and *OpenContent, the
// {open content} record itself, which §3.4.4.4 attributes an item to in
// place of any particle (Attribution's doc carries what turns on the
// difference). Matcher.Accepting asks whether the sequence so far can end there.
// A present {open content} is DECIDED, in both {mode}s: cvc-complex-content
// clauses 2 and 3 split the sequence into the part {particle} takes and the
// part the open wildcard does, and the split each clause admits is unique
// (contentmatcher.go). The walk never backtracks and never re-reads an
// item, and it COUNTS occurrences rather than unfolding them. WHICH
// particle takes each item is fixed by cos-nonambig; WHICH ITERATION of a
// repeated ancestor the item falls in is the non-determinism cvc-accept's
// closing Note leaves open, and clause 3.1 asks it existentially, so the
// walk carries the live partitions as runs of occurrence counts rather
// than one entry each, and widens the set only where a repeating particle
// can take an item on either side of a repeating ancestor's iteration
// boundary (contentmatcher.go for that condition and for the bound that
// keeps the set a constant of the schema). It decides once, at construction,
// whether it decides at all: ContentMatcher reports false for a {content type}
// holding no particle and for the shapes contentmatcher.go declines, and a
// Matcher that exists never declines a name mid-sequence. Substitution groups
// are not expanded at construction — Next resolves membership per name, as
// cvc-accept clause 2.3.2 states it.
//
// A consumer that wants the whole model rather than one sequence still
// traverses it by hand, switching Particle.Term over the TermOrRef sealed
// sum and then, for the ResolvedTerm branch, over the Term sealed sum; the
// package Examples walk one end to end.
//
// # Planned contract (M9 — not yet implemented)
//
// The second driver does not exist: the package declares no Walker.
//
//	type Walker   // M9, codegen's push driver
//	    Exhaustive and schema-only: visits every particle reachable
//	    through sequences, choices, all-groups, and named group
//	    references, in document order.
//
// Substitution groups will not be expanded at walk time — instance-time
// concern. Recursive named-group references terminate by construction in
// the finalized model; the Walker will need no visited set beyond the
// path-scoped guard the spec's one legal nesting requires.
package xsd
