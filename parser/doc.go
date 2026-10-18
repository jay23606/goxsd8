// Package parser compiles XSD schema documents into the xsd component
// model. It is deterministic, multi-phase, and cycle-check-free by
// construction.
//
// # Three phases (STYLE D4)
//
//	phase 1  parse    — each schema document → raw form via parser/xmltree;
//	                    every raw node keeps its Loc, so every later error
//	                    can cite file:line:column. ·Conditional-inclusion
//	                    pre-processing· (§4.2.2, parser/conditional.go) is
//	                    applied to each document as it is read, so what
//	                    every later phase sees is S2 = ci(S1).
//	phase 2  resolve  — schema composition (include, import, override
//	                    and redefine) through ONE
//	                    loader.Resolver, with chameleon namespace
//	                    coercion on include, override and redefine only,
//	                    override pre-processing and redefinition both
//	                    carried as data alongside the effective namespace
//	                    (parser/override.go, parser/redefine.go), and
//	                    QName reference resolution through a symbol table
//	                    seeded once per assembly with the builtins
//	                    (builtin.Seed).
//	phase 3  finalize — components completed in dependency order: a
//	                    component's item/member types are finished before
//	                    it is, and a SIMPLE type's base= is deferred to a
//	                    name this phase resolves. Spec-forbidden
//	                    circularities (circular simple-type and
//	                    complex-type base chains, circular groups,
//	                    circular substitution groups) are rejected HERE,
//	                    once, with their named src-/cos- rule and
//	                    location. UPA, particle-restriction, and EDC
//	                    checks run against the finalized shape, as do BOTH
//	                    halves of cos-st-restricts over every simple type
//	                    the assembled schema reaches: the graph half
//	                    through xsd.SimpleType.CheckDerivation and the
//	                    facet-value half through
//	                    builtin.NewRestrictionChecker, installed at
//	                    xsd.SchemaBuilder.FinalizeWith.
//
// The phase structure makes cycle REJECTION unnecessary at traversal
// time — no traversal carries a `seen` set in order to detect a
// forbidden circularity. The one exception is where the spec itself
// declares the cycle legal: attribute-group references may form cycles
// (§3.6.2.1), and xsd's finalize-time transitive-closure fold of
// {attribute uses} and {attribute wildcard} carries a visited set purely
// to bound the walk and avoid re-descending a group already folded in —
// it rejects nothing. This package holds each <attributeGroup ref> as a
// reference and follows none. Likewise the composition index keyed by
// resolved location AND the namespace the document was reached under AND
// the override applied to it is DOCUMENT IDENTITY, not a cycle guard:
// §4.2.3 makes two xs:includes of the same resolved location the same
// schema document and declares include cycles legal, §4.2.6.2 says as
// much for repeated xs:imports, §4.2.5 says as much for equivalent
// xs:overrides (and requires the processor to recognize that closure has
// been reached, which is exactly what the index does), the namespace is
// part of the key because one document reached as a chameleon include
// and as an import yields two different component sets, and the override
// is part of it because one document overridden two different ways
// likewise does. Each distinct reading is loaded once and nothing is
// rejected.
//
// # Determinism
//
// All child collections are built as slices in document order; symbol
// tables are internal indexes only (STYLE D2). Parsing the same schema
// set produces an identical model, identical error list, identical
// order.
//
// # Conditional inclusion
//
// Every schema document is ·conditional-inclusion pre-processed· as it is
// read — root, <include>d, <import>ed, <override>n and <redefine>d alike,
// and the document a caller hands [Produce] too. §4.2.2's transform removes
// each element whose vc:minVersion, vc:maxVersion, vc:typeAvailable,
// vc:typeUnavailable, vc:facetAvailable or vc:facetUnavailable attribute
// excludes this processor (V = 1.1), along with that element's attributes and
// its whole subtree: the retained version band is vc:minVersion ≤ V <
// vc:maxVersion. An ignored <schema> root becomes the stub §4.2.2 prescribes,
// keeping only targetNamespace and the two version attributes and no children
// at all, so [Parse] succeeds over such a root and assembles none of that
// document's components. An ill-formed value is charged src-cip, not guessed
// at, unless the element carrying it is already ignored — by an ancestor, or
// by an attribute of its own earlier in the order listed above (§4.2.2's
// order, not the start tag's) — since S2 does not carry it. A retained
// construct that references a declaration §4.2.2 removed is a reference to
// nothing, and is treated as any unresolved QName is: the §5.3 bullet under
// Composition gaps states which slots reject it under src-resolve and which
// one keeps it as an ·absent· member.
//
// It runs FIRST, before every other pre-processing kind and before any
// directive of the document is followed, which is what makes a declaration
// §4.2.2 removes invisible to sch-props-correct clause 2 and an <include>
// it removes a document never fetched. Everything the rest of this package
// reads is S2, never the raw parsed document: "It is S2, not S1, which is
// required to conform to this specification."
//
// # Composition
//
// The assembled schema is one [xsd.Schema] — the §3.17.1 Schema
// component, which is already multi-namespace-capable, since every
// component carries its own namespace in its expanded name. There is no
// separate "schema set" type.
//
// # Current coverage
//
//	func Parse(location string, opts ...Option) (*xsd.Schema, error)
//	func ParseReport(location string, opts ...Option) (*xsd.Schema,
//	    *AssemblyReport, error)
//	func ParseSet(roots []Root, opts ...Option) (*xsd.Schema,
//	    *AssemblyReport, error)
//	    Roots: RootAt(location), HintAt(namespace, location).
//	    Options: WithResolver(loader.Resolver) (default loader.Dir(".")),
//	    WithBackend(value.Backend) (default builtin/strict),
//	    WithLogger(*slog.Logger) (default silent).
//
// ParseSet is the entry point: it assembles several root locations into
// ONE schema, each a schema document named outright (RootAt) or a schema
// location hint followed as an <xs:import> of its namespace (HintAt), and
// reports per root which hints named no document
// ([AssemblyReport.UnfollowedRoots]). ParseReport is ParseSet over one
// RootAt, and Parse the wrapper that drops its report. The
// [AssemblyReport] answers what the assembled [xsd.Schema] cannot — which
// schema documents went into it, in discovery order, which
// ·inter-schema-document references· could not be followed to one, and
// what each discovery holds that no dispatch of this producer maps to a
// component ([UnmappedConstruct]; several regions of the vocabulary so
// far, not all of it, so its silence is not yet coverage) — so a consumer
// reasoning about the DOCUMENT SET (§4.2.1's schema(D)) does not have to
// re-walk §4.2's composition edges, or re-derive what this package reads,
// itself. It is populated even when an error is returned.
//
// ParseSet assembles the <xs:include>, <xs:override>, <xs:redefine> and
// <xs:import> closure of every root document (§4.2.3, §4.2.5, §4.2.4,
// §4.2.6.2), including chameleon coercion of a no-targetNamespace
// <xs:include>d document into the including namespace (§F.1) — both the
// components it declares and the unqualified QName references inside it.
// An <xs:import>ed document is never coerced: it contributes its
// components in its OWN target namespace (absent included), which is
// what makes a cross-namespace reference resolve. An <xs:override>d
// document IS coerced, on the same terms as an <xs:include>d one (§4.2.5
// clause 2.3): before the override is applied, the overridden document's
// identically-named source declarations are substituted by the
// override's children (§F.2), and the substitution cascades into every
// document that one <xs:include>s or <xs:override>s — §4.2.5's ·target
// set· — while stopping at <xs:import>. A substituted declaration is
// produced by the OVERRIDDEN document's producer, so the overridden
// document's target namespace and schema-level defaults apply to it
// (§4.2.5's document-level-defaults note, PRINCIPLES 16). An
// <xs:redefine>d document is coerced on the same terms too (§4.2.4
// clause 3.3), and is the one composition kind that BOTH includes and
// subtracts: the redefined document contributes every component it
// declares EXCEPT those the <redefine> names (clause 4.1.2), while the
// <redefine>'s own children contribute the replacements (clause 4.1.1)
// as definitions of the REDEFINING document. A redefining
// simpleType/complexType is paired with a hidden {name}-·absent· copy of
// the definition it replaces, which is what its own base= resolves to; a
// redefining group/attributeGroup self-reference resolves to the
// original the same way (src-expredef, parser/redefine.go), and both
// still do when an <xs:override> substitutes for the redefining child
// (§F.2 clause 1). All four redefinable kinds are produced; the hidden
// copy a redefining complexType owns is anonymous, so what finalize does
// and does not charge against it is recorded on xsd's
// checkComplexDerivations with the same limitation for every other
// anonymous type. A redefined document that redefines a document of its
// OWN for the same name composes: clause 4.1.1 makes its redefining child
// a top-level definition of it, so the outer redefinition pairs with that
// child and the chain of hidden copies runs one level deep per link (for
// a simpleType/complexType — see the composition gaps for the clause-2
// kinds). A redefined document declaring the redefined name TWICE at top
// level — clause 4.1.1 counts a nested <xs:redefine>'s child among those
// declarations — is REJECTED under sch-props-correct (§3.17.6.1) clause
// 2, charged where the pair is collected (parser/redefine.go's
// recordOriginal) because clause 4.1.2 keeps both out of the by-name
// tables finalize polices. §4.2.4 marks the whole mechanism
// ·deprecated·. A non-empty <xs:redefine> whose schemaLocation does not
// resolve is an ERROR (src-redefine clause
// 1) where an <xs:include>'s is explicitly not one (src-include clause
// 2.4). Documents are loaded once, keyed by resolved location, the
// namespace they were reached under and the override applied to them, so
// a repeated include, import or equivalent override, a diamond, or a
// (spec-legal) cycle contributes its components once. The redefinition
// is deliberately not part of that key, so a document reached both
// plainly and as an xs:redefine target — or redefined twice — is
// composed once and every definition a redefinition does not name is
// contributed once: §4.2.4 gives the redefining schema "components
// identical to all the schema components of S2, with the exception of
// those explicitly redefined", which is §4.2.3's own wording for a plain
// include over the same schema(D2). What each reading excepts is applied
// per reading instead, so the definitions two readings disagree about do
// still collide under sch-props-correct clause 2 (#1349). Loading once
// suppresses the second COMPOSITION only, which is all §4.2.6.2's note
// asks for: a repeated <xs:import> is still judged against src-import
// clause 3, so one whose namespace disagrees with the already-loaded
// document's targetNamespace is rejected however that document was first
// reached.
//
// What the assembly contributes is not what a reference may reach for. A
// QName in a schema document resolves only into a namespace THAT document
// licenses (§3.17.6.2 src-resolve clause 4, cl.qnr.nsdeclared): its own
// effective (post-§F.1) target namespace, a namespace one of its OWN
// <xs:import> children names, or the XSD or XSI namespace — the last two
// with no <xs:import> at all. An unqualified reference left in the
// ·absent· namespace is licensed only by a document that declares no
// targetNamespace or carries a bare <xs:import> (clause 4.1). The license
// is per schema DOCUMENT, never assembly-wide: a namespace some sibling
// document of the assembly imported licenses nothing here, which is
// exactly what §4.2.6.1 states — "if references to components in a given
// namespace N appear in a schema document S, then S must contain an
// <import> element importing N". Such a reference is rejected as
// src-resolve at the reference itself, and deliberately not treated as a
// §5.3 missing sub-component, which §4.2.6.1 also says in as many words.
//
// [Produce] remains the single-document entry point: it maps one
// already-read document and follows no inter-document reference at all —
// it does READ that document's <xs:import> children, since clause 4
// licenses its references from them, but dereferences none of them.
//
// # Composition gaps
//
//   - GAP(xsd): a CHAINED <xs:redefine> of a <group> or an
//     <attributeGroup> — one whose redefined document redefines a
//     document of its own for the same name — is REFUSED under
//     src-expredef's closing requirement, on a schema §4.2.4 clause
//     4.1.1 makes valid. The two kinds src-expredef clause 1 pairs,
//     simpleType and complexType, do compose. Fail-CLOSED, with one
//     consumer: produceRedefinition's closing-requirement check
//     (parser/redefine.go's chainedOriginal, where the marker sits and
//     records why the two clauses being charged is not enough). Owned
//     by #744.
//   - GAP(xsd): §5.3 (Missing Sub-components) is never reported as such.
//     A namespace an <xs:import> declares but no document of the
//     assembly supplies — a bare import, or one whose schemaLocation
//     does not resolve — leaves that namespace's components genuinely
//     missing, and the only way that surfaces is an actual QName
//     reference into it failing src-resolve at finalize (which this
//     package hard-fails, see [xsd.SchemaBuilder.Finalize]); an assembly
//     that makes no such reference is accepted, which under-rejects.
//     The XML namespace is the one namespace this gap does not reach:
//     an explicit <xs:import> of it is served, in place of the document
//     it cannot fetch, the components the 2009 xml.xsd declares — four
//     attribute declarations and the xml:specialAttrs attribute group
//     definition, built by parser/produce_xmlnamespace.go's
//     addXMLNamespace. The src-resolve clause 4 licensing above
//     does NOT close this gap and cannot: clause 4 judges whether the
//     document ASKED for the namespace, §5.3 what follows when a
//     namespace it did ask for supplies no such component — §4.2.6.1
//     holds the two apart in as many words ("references … not imported
//     by that schema document … are not handled as if they referred to
//     missing components"). That hard-fail is itself the deviation from
//     §5.3, which makes an unresolved reference an ·absent· value and
//     defers the consequence to ·assessment·, never rejecting the
//     schema. One slot is already aligned: a {substitution group
//     affiliations} member naming no declaration is retained as ·absent·
//     rather than rejected (see xsd/resolve.go's resolveElementDecl), and
//     the validation side charges cvc-elt clause 1 for it and falls back
//     to the ·lax assessment· §5.3 requires. The remaining slots —
//     {type definition}, <element ref>, <attribute ref>, <group ref>,
//     keyref, and a simple type's base=, itemType= or memberTypes= —
//     still reject, and each would also need that fallback on the
//     validation side, which among them only a {type definition} naming
//     an ·absent· substitution group head has (validate's
//     walk.resolvedSelection) — a shape only a directly built declaration
//     carries, since this package gives such a member xs:anyType. This
//     bullet is the one place the reason is written down; the marker in
//     xsd/resolve.go points here rather than restate it. The spec
//     question is settled: §5.3 DOES govern a failed ·resolution·, so the
//     hard-fail is a deliberate implementation policy choice and not
//     something §5.3 mandates (the 2026-09-14 oracle ruling, on issue
//     1426's thread with its citations). The cost of reversing it was
//     measured once and stands here as history rather than re-derived:
//     branch wip/issue-434 at e86b5c3 carries the construction half in
//     full — every by-name arm retaining ·absent· rather than charging
//     src-resolve — and regressed 35 schema cases against 9 improved
//     there and 14 in instance; issue 434 was declined not_planned on
//     that measurement. Reversing the policy therefore means either
//     accepting a schema-lane regression of that order or minting a
//     ratchet class able to record one, and CLAUDE.md's closing sentence
//     reserves both calls to a human-filed issue ("This file's 'one
//     rule', and the ratchet-integrity section of
//     .claude/agents/arbiter.md, change only via a human-filed issue —
//     never in a retro"). No issue owns this gap, and neither marker
//     names one.
//   - GAP(xsd): two DISTINCT <xs:redefine> elements whose children are
//     textually equivalent each contribute their own replacement
//     components, so redefining one document the same way down two paths
//     yields two components of one expanded name and a sch-props-correct
//     clause 2 rejection where §4.2.4's note ("multiple equivalent
//     <xs:redefine>ing of the same schema document will not constitute a
//     violation") wants none. The REDEFINED document is composed once
//     (see the composition paragraph above); what is never identified is
//     one redefining declaration with another, which §4.2.4 asks for in
//     the same breath as it warns of "the necessity of establishing
//     identity component by component". This over-rejects, so it can
//     lose a valid assembly, never accept an invalid one.
//     <xs:override> no longer has this gap: its identity is the
//     substituted elements' ·canonical content· with the source location
//     left out (parser/override.go's writeCanonicalElement), so two
//     distinct but equivalent <xs:override> elements do reach one
//     document identity. #603 owns closing this.
//   - Two children of ONE <xs:override> with the same element type and
//     name are reported under src-override, though §F.2's normative
//     stylesheet resolves the pair as first-match-wins and the published
//     REC therefore makes the case valid. This tracks the Working Group's
//     recorded intent instead (W3C Bugzilla 17617, 2012-06-29: "an
//     erratum is needed to make this situation an error"); the erratum
//     was never filed, and W3C Override/over021 is `accepted` on the
//     strength of it. Over-rejects in the same direction as the entry
//     above.
//
// # Planned instance-hint reader (not yet implemented)
//
//	func SchemaLocationHints(instance io.Reader) ([]Hint, error)
//	    The xsi:schemaLocation reader shared by the CLI, the validator,
//	    and the conformance harness — one implementation, resolved
//	    relative to the instance location, routed through the same
//	    Resolver as root schemas.
//
// Schema-validity violations are *xsderr.Error values carrying src-*/
// cos-*/derivation-ok-* rules — src-cip among them, for a vc: attribute
// whose value is not valid against the type §4.2.2 declares for it — plus
// cvc-datatype-valid where a schema document attribute is simply not
// valid against the type the schema for schema documents declares for it
// (a declaration name that is not an xs:NCName, a QName-valued attribute
// with an empty local part, an empty prefix, or more than one colon, an
// unrecognized ## token in notQName, §3.10.2, a minOccurs/maxOccurs
// outside xs:nonNegativeInteger/xs:allNNI, a length, minLength, maxLength
// or fractionDigits value outside xs:nonNegativeInteger or a totalDigits
// value outside xs:positiveInteger, a processContents outside the
// enumeration skip/lax/strict, an id outside xs:ID, a block, final,
// blockDefault or finalDefault value outside the derivation-set type its
// element declares it with), which no Schema Representation Constraint
// covers — and cvc-id clause 2 where two elements of ONE schema document
// carry the same id, the document's <schema> being its validation root; the
// same id in two documents of an assembly is valid. An overridden
// document's ids are judged as fetched, not as Dold′ (rejectInvalidID's
// GAP(parser) marker, #875). A document that is simply not VALID against
// the schema for schema documents — a prohibited attribute on a top-level
// form or on a nested <complexType> or <simpleType>, a final or abstract on
// an inline local <element>, an unprefixed attribute the element's grammar
// declares nowhere, a child written where that element's content model
// admits none, a child repeated past its maxOccurs, a child whose name the
// model admits nowhere — is §5.1's first bullet and carries NO rule ID at
// all: it is a plain wrapped error naming the offending item and the grammar
// it violates (parser's rejectProhibitedAttrs, rejectLocalComplexTypeAttrs,
// rejectLocalSimpleTypeAttrs, rejectLocalElementProhibitedAttrs,
// rejectUndeclaredAttrs, checkS4SChildOrder, rejectLateAnnotation,
// rejectUnmappedTopLevel, rejectOutOfModelFacetChildren and simpleTypeBody's
// own two alternative-count branches). An attribute in ANOTHER namespace is
// admitted wherever it stands, which is the whole of what xs:openAttrs'
// ##other wildcard admits. PLANNED (not yet implemented): collecting them in
// document order rather than stopping at the first — [Parse] and [Produce]
// both return only the first error today.
package parser
