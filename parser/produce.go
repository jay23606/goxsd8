package parser

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/kud360/goxsd8/builtin"
	"github.com/kud360/goxsd8/regex"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// The Schema Representation Constraints (§3.x.3) and other rules this package
// charges. Each string is a live entry in xsderr's generated catalog.
const (
	ruleSrcInclude            xsderr.Rule = "src-include"
	ruleSrcImport             xsderr.Rule = "src-import"
	ruleSrcImportNoSelfImport xsderr.Rule = "src-import-noselfimport"
	ruleSrcOverride           xsderr.Rule = "src-override"
	ruleSrcRedefine           xsderr.Rule = "src-redefine"
	ruleSrcExpRedefine        xsderr.Rule = "src-expredef"
	ruleSrcElement            xsderr.Rule = "src-element"
	ruleSrcAttribute          xsderr.Rule = "src-attribute"
	ruleSrcSimpleType         xsderr.Rule = "src-simple-type"
	ruleSrcResolve            xsderr.Rule = "src-resolve"
	ruleSTPropsCorr           xsderr.Rule = "st-props-correct"
	ruleCTPropsCorr           xsderr.Rule = "ct-props-correct"
	ruleSrcCT                 xsderr.Rule = "src-ct"
	ruleSrcWildcard           xsderr.Rule = "src-wildcard"
	ruleSrcIdentityConstraint xsderr.Rule = "src-identity-constraint"
	// ruleNoXmlns is the "xmlns Not Allowed" Schema Component Constraint
	// (§3.2.6.3), charged by rejectXmlnsName over both attribute declaration
	// productions. Like its §3.2.6.4 sibling below it is a Schema COMPONENT
	// Constraint charged nowhere but here; see rejectXmlnsName's GAP(xsd)
	// counterpart at xsd.NewAttributeDeclaration for the component-footing
	// residue.
	ruleNoXmlns xsderr.Rule = "no-xmlns"
	// ruleNoXSI is the "xsi: Not Allowed" Schema Component Constraint
	// (§3.2.6.4), charged by rejectXSITargetNamespace over both attribute
	// declaration productions. It is a Schema COMPONENT Constraint charged
	// nowhere but here, unlike every other one this producer shares with xsd:
	// see rejectXSITargetNamespace for why the component footing cannot carry
	// it.
	ruleNoXSI xsderr.Rule = "no-xsi"
	// ruleEPropsCorrect is the Element Declaration Properties Correct Schema
	// Component Constraint (§3.3.6.1). The producer charges only clause 3 ("If
	// E.{substitution group affiliations} is non-empty, then E.{scope}.{variety}
	// = global") and only in its SYNTACTIC form — a local <element> carrying a
	// substitutionGroup attribute, which the schema for schema documents declares
	// use="prohibited" on xs:localElement (§3.3.2). xsd.NewElementDeclaration
	// charges the same clause on the component, for the programmatic path that
	// bypasses this producer; the two are the same rule seen from either side of
	// the mapping.
	ruleEPropsCorrect xsderr.Rule = "e-props-correct"
	// ruleDatatypeValid is the generic "this attribute's value is not valid
	// against the simple type the schema for schema documents declares for it"
	// rule (Datatypes §4.1.4, cvc-datatype-valid). It is charged where a schema
	// document attribute fails its own declared type and no Structures Schema
	// Representation Constraint covers the case — notably an unrecognized ##
	// token in notQName, whose value space is fixed by xs:qnameList/xs:qnameListA
	// (§3.10.2) rather than by any src-wildcard clause, a QName-valued
	// attribute whose local part is empty (bindQName), which no src-* clause
	// reaches because they all presuppose a well-formed QName, a minOccurs/maxOccurs
	// lexical outside the xs:nonNegativeInteger/xs:allNNI types Appendix A's occurs
	// attribute group declares (nonNegativeNumeral), which p-props-correct cannot reach
	// because no particle exists yet (#932), a length or digits facet's value lexical
	// outside the xs:nonNegativeInteger/xs:positiveInteger its facet element declares
	// (facetCountValue), which facet compilation reaches only when an instance does
	// (#1774), and a processContents lexical outside the skip/lax/strict enumeration
	// Appendix A's wildcard attribute group declares (processContentsOf), which
	// w-props-correct cannot reach because no wildcard exists yet (#950), an id
	// value outside xs:ID (rejectInvalidID), and a block=/final= family value
	// outside its s4s derivation-set type (rejectInvalidDerivationSet).
	ruleDatatypeValid xsderr.Rule = "cvc-datatype-valid"
	// ruleValidationRootValid is Validation Root Valid (ID/IDREF) (§3.3.4.5,
	// cvc-id). The producer charges only clause 2 — no two elements of the
	// validation root carry the same xs:ID value — over a schema document's own
	// id attributes, read as an instance of the schema for schema documents
	// (rejectInvalidID).
	ruleValidationRootValid xsderr.Rule = "cvc-id"
	// ruleSchPropsCorrect is the Schema Properties Correct Schema Component
	// Constraint (§3.17.6.1). The producer charges only clause 2 ("None of the
	// {type definitions}, … properties contains two or more schema components with
	// the same expanded name") and only for a REDEFINED document's own top-level
	// definitions, which §4.2.4 clause 4.1.2 excepts from contributing components
	// and so hides from the by-name symbol tables (redefineSet.recordOriginal).
	// xsd.indexByName charges the same clause on the outermost assembled schema,
	// for every pair that does reach it as two named components; the two are the
	// same rule seen at either collection point.
	ruleSchPropsCorrect xsderr.Rule = "sch-props-correct"
	// ruleParticleCorrect is Particle Correct (§3.9.6.1). The producer charges
	// only clause 2.1 ({min occurs} greater than a numeric {max occurs}), in
	// occursOf, for every parsed minOccurs/maxOccurs pair: it orders the
	// unsaturated numerals and names them, where xsd.NewOccurs, handed both
	// saturated at math.MaxInt, could neither order a pair past it nor name the
	// document's values. NewOccurs charges the same clause for a synthesized
	// occurrence range, read from no attribute; the two are the same rule seen at
	// either collection point.
	ruleParticleCorrect xsderr.Rule = "p-props-correct"
)

// Produce maps the TOP-LEVEL <simpleType>, <element>, <attribute>,
// <complexType>, <attributeGroup>, <group>, and <notation> declarations of a
// single already-parsed schema document into xsd components, in document order,
// and returns the finalized [xsd.Schema]. The identity constraints of an
// <element> (global or local) are produced with it; each name= form is registered
// as a schema-level {identity-constraint definitions} member (§3.17.1), while a
// ref= form contributes the definition it names and registers nothing (§3.11.2).
//
// The document handed in is ·conditional-inclusion pre-processed· first
// (§4.2.2, parser/conditional.go), since [ReadDocument] performs no
// pre-processing: what is mapped below is S2 = ci(S1), and an ill-formed vc:
// attribute value is returned as an src-cip verdict.
//
// Produce is the SINGLE-DOCUMENT entry point: it never dereferences an
// <include>/<import>/<redefine>/<override>, so schema(D) here is immed(D) alone
// (§4.2.1). Multi-document assembly through <include> — and with it chameleon
// coercion (§4.2.3 clause 3.2, §F.1) — is [Parse]'s job; a caller holding only a
// document and a backend, with no resolver, keeps this entry point. Its
// <import> children are still READ, though never followed: src-resolve clause 4
// licenses this document's QName references from them (see parser/doc.go's
// Composition section), so a type= into a foreign namespace needs an <import>
// element naming it here as much as it does under [Parse]. One of them also
// SUPPLIES components: an <import> of the §1.3.2 XML namespace is answered with
// that namespace's four built-in attribute declarations and its xml:specialAttrs
// attribute group definition (parser/produce_xmlnamespace.go), a construction
// rather than a fetch (§4.2.6.1) and so as available here as under [Parse].
//
// backend is passed explicitly rather than defaulted to a builtin/strict policy
// here: that default belongs to [Parse], keeping this leaf free of a
// builtin/strict edge. Produce seeds the builtin datatypes from backend
// ([builtin.Seed]) so a type="xs:…" reference resolves at finalize; the SAME
// *[xsd.SimpleType] pointer identity is both AddType'd into the builder and used
// as a simple-type base, as [xsd.SimpleType] requires. backend also supplies the
// finalized schema's value space ([value.NewValueSpace] through
// [xsd.SchemaBuilder.FinalizeWith]), so the finalize-time constraints that reach
// into a value space decide there instead of failing open on the {lexical form}s:
// the two that COMPARE two {value}s — au-props-correct (§3.5.6) clause 3,
// loc-testSubP (§3.4.6.4) clauses 4.2 and 5.2.2 — and the two that VALIDATE one
// against its type — a-props-correct (§3.2.6.1) clause 2 and au-props-correct
// clause 2, both charging Simple Default Valid (§3.2.6.2).
//
// DEVIATION from parser/doc.go's "the parser collects them in document order
// rather than stopping at the first": Produce returns only the FIRST error. That
// promise is for the eventual full Parse; this first slice does not yet
// implement multi-error collection.
//
// A document whose root is not <schema> is a caller precondition fault, not a
// schema-validity verdict — §3.17.2 even allows <schema> not to be the document
// element — so it is reported as a plain Go error, deliberately NOT routed
// through xsderr (mirroring [builtin.MissingPrimitivesError]'s rationale): no
// src-*/cos-* rule governs "the document handed to a producer must be a schema
// document".
func Produce(doc *Document, backend value.Backend) (*xsd.Schema, error) {
	// §4.2.2: the document a caller hands in is S1, the pre-processing INPUT, since
	// [ReadDocument] performs no pre-processing of its own. Everything below maps
	// S2 = ci(S1), which is what "schema document" means in every rule this
	// producer charges.
	doc, err := conditionalInclude(doc)
	if err != nil {
		return nil, err
	}
	if !doc.IsSchema() {
		return nil, fmt.Errorf("parser: Produce requires a <schema> document root, got %s", doc.Root().Name().Local())
	}
	builder := xsd.NewSchemaBuilder()
	sym, err := newSymbols(builder, backend)
	if err != nil {
		return nil, err
	}
	// A lone document is its own assembly, so its effective target namespace is
	// its own targetNamespace and it is never a chameleon (§4.2.3 clause 2.3
	// needs an including document to borrow a namespace from).
	target := attrOr(doc.Root(), "targetNamespace")
	// Being the whole assembly, it is also the only document that could have
	// composed components under the XML namespace, which is the other half of
	// what supplyXMLNamespace reads.
	if err := addXMLNamespace(builder, []composedDocument{{schema: doc.Root(), target: target}}); err != nil {
		return nil, err
	}
	p := newProducer(doc, target, nil, nil, nil, builder, sym)
	if err := p.prescan(); err != nil {
		return nil, err
	}
	if err := p.checkDefaultOpenContent(); err != nil {
		return nil, err
	}
	if err := p.checkFormDefaults(); err != nil {
		return nil, err
	}
	if err := p.run(); err != nil {
		return nil, err
	}
	return builder.FinalizeWith(value.NewValueSpace(backend), builtin.NewRestrictionChecker(backend))
}

// symbols is the ASSEMBLY-WIDE symbol table: one set of indexes shared by every
// document's producer across an entire <include> closure, so a base= or
// <attributeGroup ref> in one document reaches a definition contributed by
// another (§4.2.3 clause 3.1.2, c-incl-incl). All four are pure lookup indexes,
// never ranged to produce user-visible order (STYLE D2).
type symbols struct {
	// simpleTypes maps each top-level named <simpleType>'s expanded name to its
	// source (raw element plus the producer of the document that declares it),
	// filled by the pre-scan so forward base= references between simple types
	// resolve (Structures §3.1.3).
	//
	// The owning producer is carried for the same reason complexTypes carries one:
	// §3.16.2.1 identifies {base type definition} as what "the actual value of the
	// base attribute" resolves to, and that attribute belongs to the DECLARING
	// document — src-resolve (§3.17.6.2) clause 4.1.1 scopes its absent-namespace
	// default to "the schema document containing the QName", and §F.1 task (b)
	// coerces a chameleon-included document's own unqualified references to that
	// document's effective target namespace. Built under a referring producer
	// instead, unqualifiedRefNS/declares would answer for the wrong document and
	// falsely reject a valid base=.
	simpleTypes map[xsd.QName]typeSource

	// complexTypes maps each top-level named <complexType>'s expanded name to
	// its source (raw element plus the producer of the document that declares
	// it), filled by the pre-scan so a base= on a <complexContent>/<simpleContent>
	// derivation reaches its {base type definition} regardless of document order
	// (§3.4.2's preamble: "the mapping rules … depend upon the {base type
	// definition} having been identified before they apply").
	//
	// The owning producer is carried, not just the element, because producers are
	// per-DOCUMENT (parse.go's compile) while this index is assembly-wide: a
	// complex type built on demand from a REFERRING document must still be built
	// under its OWN document's producer, or every local element declaration
	// inside it takes the wrong {target namespace} (localTargetNS reads
	// schemaElem's elementFormDefault, and unqualifiedRefNS/declares answer for
	// the declaring document).
	complexTypes map[xsd.QName]typeSource

	// modelGroups maps each top-level named <group>'s expanded name to its source
	// (raw element plus the producer of the document that declares it), filled by
	// the pre-scan so a <group ref> reaches its definition regardless of document
	// order (§3.1.3: "forward reference to named definitions and declarations is
	// allowed"). Only one mapping rule consults it — §3.4.2.3.3 clause 4.2.3's
	// sub-case test, which must know the {compositor} of the Model Group a
	// <group ref> ·base particle· resolves to (xsd.ExtensionContentType, through
	// resolveModelGroup); every OTHER <group ref> stays an unresolved
	// ModelGroupRef until finalize (produceGroupRefParticle).
	//
	// The owning producer is carried for BOTH reasons the two type indexes carry
	// one, because a <group> body holds local declarations AND unqualified
	// references: §3.3.2.3 takes a local <element>'s {target namespace} from "the
	// ancestor <schema> element information item", which is the DECLARING
	// document's, and src-resolve (§3.17.6.2) clause 4.1.1 scopes the
	// absent-namespace default of its type=/ref= to "the schema document containing
	// the QName", which §F.1 task (b) coerces when that document is a chameleon.
	// Built under a referring producer instead, localTargetNS would mint the local
	// names in the wrong namespace and unqualifiedRefNS/declares would answer for
	// the wrong document.
	//
	// There is no attribute-group twin. An <attributeGroup ref> is resolved at
	// finalize against xsd's {attribute group definitions} (#479), and no mapping
	// rule reads a referenced group at mapping time.
	modelGroups map[xsd.QName]typeSource

	// elements maps each top-level <element>'s expanded name to its source (raw
	// element plus the producer of the document that declares it), filled by the
	// pre-scan so §3.3.2.1's {type definition} clause 3 — "the declared {type
	// definition} of the Element Declaration ·resolved· to by the first QName in
	// the ·actual value· of the substitutionGroup attribute" — reaches a HEAD
	// declared later in the document or in another document of the assembly
	// (§3.1.3: forward reference to named declarations is allowed).
	//
	// It holds SOURCES, not built components, and clause 3 reads only ATTRIBUTES
	// and CHILD SHAPES off them (substitutionGroupHeadType: an inline type child,
	// then type=, then substitutionGroup= to walk on) rather than building the
	// head declaration on demand: an Element Declaration is not memoized the way
	// a type is, so building one here would produce a second component for the
	// same name and register its identity constraints twice. Where the head's
	// type is an inline anonymous <complexType>, the walk therefore yields a
	// REFERENCE to the head (xsd.SubstitutionGroupHeadTypeRef) and finalize reads
	// the head's own already-built {type definition} through it — the one place
	// the built component is needed, reached after every declaration exists
	// rather than during the pre-scan (#342).
	//
	// The owning producer is carried for the reason every other index carries one:
	// the head's type= is a QName in the HEAD's document, so it resolves under that
	// document's in-scope prefix bindings and §F.1 chameleon coercion (src-resolve
	// clause 4.1.1). Resolved under a referring producer instead, an unqualified
	// or differently-prefixed head type would name the wrong type.
	elements map[xsd.QName]typeSource

	// identityConstraints maps each NAMED <unique>/<key>/<keyref>'s expanded name
	// to its source, filled by the pre-scan so a <key ref="…"> reaches its
	// definition regardless of document order (§3.1.3: "forward reference to named
	// definitions and declarations is allowed, both within and between schema
	// documents"). Unlike the three indexes above it is filled from the WHOLE
	// document tree, not just the top level: §3.17.2 sources a schema's
	// {identity-constraint definitions} from the constraints "anywhere within the
	// [[children]]", so a reference may name one declared on an arbitrarily deep
	// local <element>.
	identityConstraints map[xsd.QName]identityConstraintSource

	// builtins is the FIXED set of Built-in Simple Type Definitions (§3.16.7),
	// exactly what [builtin.Seed] yields, keyed by ·expanded name·. newSymbols
	// fills it once and nothing writes it again, so it answers the same for a
	// given backend whatever any schema document declares — which is what
	// ctaStaticTypes needs, and what built cannot supply because a schema
	// TARGETING the XSD namespace writes its own top-level types into that memo.
	builtins map[xsd.QName]*xsd.SimpleType

	// built is the build-once MEMO for simple-type construction: an ABSENT key is
	// unbuilt, a PRESENT one is done, and there is no third state. It starts as a
	// copy of builtins, so a builtin starts out done, which is also what gives a
	// base= naming one the canonical component by pointer identity. The two maps
	// diverge from there and neither is derivable from the other: this one grows
	// with every named simple type the assembly builds.
	//
	// It is deliberately NOT a cycle guard, unlike its complex-type sibling
	// builtComplex: a simple type's base= is deferred to a name at mapping time
	// (xsd.SimpleTypeRef), so constructing one recurses into no other named
	// simple type and there is nothing to bound. What still reads this map is
	// resolveBaseType, which needs a live *xsd.SimpleType for a COMPLEX type's
	// base= — see buildSimpleType.
	built map[xsd.QName]*xsd.SimpleType

	// builtComplex is the memo + cycle guard for COMPLEX-type construction. An
	// absent key is unstarted, and a present one holds the complexBuild arm the
	// declaration's build has reached, so "started but unrecorded" stays
	// unrepresentable (STYLE T1/D3). The complexOnBaseChain arm is what terminates
	// demand-driven base construction on a circular chain, charged
	// ct-props-correct clause 3 (§3.4.6.1) — the SAME rule xsd/resolve.go's
	// checkComplexBaseAcyclic charges for the programmatic SchemaBuilder path; see
	// buildComplexType.
	//
	// It is keyed by DECLARATION and not by expanded name, for the reason
	// builtGroups states in full: a name addresses one component only in an
	// assembly sch-props-correct (§3.17.6.1) clause 2 accepts, and a <redefine>
	// whose redefined document ALSO contributes the name is exactly the assembly
	// where it addresses two.
	builtComplex map[*Element]complexBuild

	// anyType is the ur-type component (§3.4.7) the builder was seeded with, held
	// so a derivation naming it resolves to that very component rather than to a
	// rebuilt twin (resolveBaseType). It is no part of builtComplex because that
	// memo is keyed by the declaration built and xs:anyType is declared by no
	// document.
	anyType xsd.ComplexType

	// builtGroups is the memo + on-stack guard for MODEL GROUP DEFINITION
	// construction, with a tri-state (absent unstarted, present-nil on the build
	// stack, present-non-nil done).
	//
	// The memo half is a correctness requirement, not an optimization: mapping a
	// definition whose body holds a local <element> with a NAMED
	// <unique>/<key>/<keyref> REGISTERS that identity-constraint definition with
	// the builder (produceIdentityConstraint), so mapping the same <group> twice —
	// once on demand for a clause 4.2.3 sub-case test, once at its own
	// document-order position — would register it twice and fabricate a
	// sch-props-correct (§3.17.6.1) clause 2 duplicate-name collision against the
	// very definition it duplicates.
	//
	// That requirement is about one DECLARATION, which is why this map and
	// builtComplex are keyed by *Element and not by expanded name. An expanded
	// name addresses one declaration only in an assembly clause 2 accepts, and
	// §4.2.4 clause 4.1.2 leaves that routinely false: a document reached BOTH
	// plainly and through a <redefine> excepts nothing (redefinitions.excepts is
	// the intersection of the readings), so its original and the redefinition both
	// claim the name. Keyed by name, the memo hands produceRedefinition the
	// ORIGINAL for a redefining <group>/<complexType>, which then registers a
	// second copy of the original: clause 2 charges the collision against one
	// location twice, and src-redefine (§4.2.4) clause 6.2.2 compares the pair
	// (original, original) instead of (redefinition, original) (#1361). Keyed by
	// declaration the two are distinct, and where no two declarations share a name
	// the two keyings agree entry for entry.
	//
	// The on-stack half is a TERMINATION guard only, never a verdict: its sole
	// reader is resolveModelGroup, which reports an in-progress definition as
	// "does not resolve here" and lets the caller fall through. Rejecting a
	// circular <group ref> graph stays mg-props-correct clause 2's job at finalize
	// (xsd/resolve.go's checkModelGroupsAcyclic), which is where the whole graph is
	// visible; a second rejection path here would be a second encoding of one fact.
	//
	// That state IS reachable, as of #340: a definition body that re-enters
	// complex-type construction now exists — a local <element> with an inline
	// <complexType> — while every other child (a typed local <element>, a
	// wildcard, a nested compositor, a <group ref>) either retains its reference
	// or recurses only within the body. The sentinel is this guard's live
	// termination path for that shape (a <group ref> whose definition body
	// contains a local element whose inline <complexType> re-enters the same
	// <group ref>): it reports "does not resolve here" and lets the caller fall
	// through instead of recursing until the stack dies. It must be WRITTEN
	// regardless, since the memo cannot be filled before the definition exists,
	// so reading it here costs one branch always and stops a real infinite
	// recursion now.
	builtGroups map[*Element]*xsd.ModelGroupDefinition

	// redefineOriginals is the ON-STACK set of the <redefine>d documents'
	// declarations currently being built as src-expredef clause 1.1 ·original·s.
	// It is a set and not a memo: an original's {name} is ·absent·, so nothing can
	// name it and there is nothing to build once and hand back (see
	// redefinedComplexBase on why the anonymous hop stays out of the build memos).
	//
	// It is the guard for the one cycle those build sentinels cannot see:
	// §4.2.4 clause 4.1.1 makes each level's redefining child the "top-level
	// definition item" the next level pairs with, so a cycle of <redefine>s closed
	// on one (kind, name) builds a {base type definition} chain of anonymous hops
	// that re-enters itself without ever passing through a named build (#1349).
	// Its two readers charge the same acyclicity rule the finalize-side walks do —
	// ct-props-correct clause 3 at redefinedComplexBase, st-props-correct clause 2
	// at resolveBase — which is PRINCIPLES 9's "detect once at construction",
	// applied per construction path.
	redefineOriginals map[*Element]struct{}

	// builtIC is the build-once memo for identity-constraint construction. It has
	// NO on-stack sentinel, unlike builtComplex: mapping a definition reads
	// only its own <selector>/<field> and retains its refer= as an unresolved
	// QName, so construction never recurses into another definition and there is
	// no circularity to guard (PRINCIPLES 9).
	//
	// The memo is what makes §3.11.2's ref= mapping literal — "the corresponding
	// schema component IS the identity-constraint definition resolved to by the
	// actual value of the ref attribute" — rather than approximate: every
	// reference to a name yields the very component its definition contributed,
	// never a rebuilt twin.
	builtIC map[xsd.QName]xsd.IdentityConstraint

	// backend is the assembly's [value.Backend], retained past the one-time
	// [builtin.Seed] call so the assembly's two finalize-time capabilities —
	// [value.NewValueSpace] and [builtin.NewRestrictionChecker], the latter
	// charging the facet-value half of cos-st-restricts over every simple type
	// the finalized schema reaches — are built from the SAME backend the builtins
	// were seeded from. It lives here rather than on the per-document producer
	// for the same reason the indexes do: it is assembly-wide and identical for
	// every document.
	backend value.Backend
}

// typeSource is one entry of a symbols source index, or a <redefine>d
// original: a top-level declaration element together with the producer of the
// document that DECLARES it. On-demand construction from a reference runs
// through owner, never through the producer that happens to be asking, so the
// definition's local element and attribute declarations take their own
// document's target namespace and form defaults (§3.3.2.3 dcl.elt.local,
// §3.2.2.2 dcl.att.local) and its own unqualified QName references take their
// own document's §F.1 coercion — all properties of the declaring document, which
// assembly-wide visibility (§4.2.3 c-incl-incl) does not transfer to the asker.
type typeSource struct {
	elem  *Element
	owner *producer
}

// identityConstraintSource is one entry of symbols.identityConstraints: a NAMED
// <unique>/<key>/<keyref> element, the {identity-constraint category} its local
// name fixes (§3.11.2), and the producer of the document that DECLARES it. The
// category is carried rather than re-derived at each use because the pre-scan
// already computed it to decide whether to index the element at all (STYLE D3
// cuts the other way here: re-deriving would mint a second, fallible answer to a
// question already settled).
//
// A reference is built through owner, never through the producer that happens to
// be asking, for the reasons typeSource's doc gives: the <selector>/<field>
// XPath Expression records take their {namespace bindings} and {default
// namespace} from the DECLARING document (§3.13.1, and the xpathDefaultNamespace
// chain rooted at that document's <schema>), which assembly-wide visibility does
// not transfer to the asker.
type identityConstraintSource struct {
	elem     *Element
	category xsd.IdentityConstraintCategory
	owner    *producer
}

// newSymbols returns the empty assembly-wide symbol table, having seeded the
// builtin datatypes, xs:anyType and §3.2.7's four xsi: attribute declarations
// into builder — EXACTLY ONCE for the whole assembly, which is why seeding lives
// here and not in the per-document producer: seeding per document would add
// xs:string (and every other builtin) once per <include>d document and trip
// sch-props-correct (§3.17.6.1) clause 2 on any schema assembled from two or more
// documents.
//
// xs:anyType is the ur-type Complex Type Definition (§3.4.7). [builtin.Seed]
// yields only simple types (its doc defers anyType to M4 as "a parser-level
// structural concern"), so without it a bare type=-less <element>/<attribute> —
// which defaults to xs:anyType (§3.3.2.1 case 4) — would fail src-resolve at
// finalize. It is added to {type definitions} exactly like a produced complex
// type, so a type= reference to it resolves.
//
// seedInstanceAttributes is the {attribute declarations} counterpart, and the
// same duplication argument governs it: the four are added once, into the same
// set a document's own top-level <attribute> declarations reach, so an
// <attribute ref="xsi:type"/> resolves at finalize instead of being charged
// src-resolve clause 1.2.
//
// The XML namespace's components are NOT seeded here, and the asymmetry is
// src-resolve clause 4.2's: they are supplied CONDITIONALLY, by addXMLNamespace
// (produce_xmlnamespace.go), from each caller of this function once the whole
// document set is known. Nothing seeded here is conditional on anything.
func newSymbols(builder *xsd.SchemaBuilder, backend value.Backend) (*symbols, error) {
	seeded, err := builtin.Seed(backend)
	if err != nil {
		return nil, err
	}
	builtins := make(map[xsd.QName]*xsd.SimpleType, len(seeded))
	for _, b := range seeded {
		builder.AddType(b)
		builtins[b.Name()] = b
	}
	anyType, err := seedAnyType()
	if err != nil {
		return nil, err
	}
	builder.AddType(anyType)
	instanceAttrs, err := seedInstanceAttributes()
	if err != nil {
		return nil, err
	}
	for _, a := range instanceAttrs {
		builder.AddAttribute(a)
	}
	return &symbols{
		simpleTypes:         make(map[xsd.QName]typeSource),
		complexTypes:        make(map[xsd.QName]typeSource),
		modelGroups:         make(map[xsd.QName]typeSource),
		elements:            make(map[xsd.QName]typeSource),
		identityConstraints: make(map[xsd.QName]identityConstraintSource),
		builtins:            builtins,
		built:               maps.Clone(builtins),
		builtGroups:         make(map[*Element]*xsd.ModelGroupDefinition),
		builtIC:             make(map[xsd.QName]xsd.IdentityConstraint),
		redefineOriginals:   make(map[*Element]struct{}),
		builtComplex:        make(map[*Element]complexBuild),
		anyType:             anyType,
		backend:             backend,
	}, nil
}

// producer is the build context for ONE schema document within an assembly.
// Document order comes solely from walking schemaElem.Children(); the shared
// symbol table holds no order (STYLE D2).
type producer struct {
	schemaElem *Element
	// target is the EFFECTIVE target namespace this document's components are
	// minted in, which is per-DOCUMENT and not necessarily the document's own:
	// under chameleon inclusion (§4.2.3 clause 2.3) a document with no
	// targetNamespace of its own contributes its components to the INCLUDING
	// namespace (§F.1 task a). An <import>ed document is the opposite case — it is
	// always minted in its own namespace, since §4.2.6.2 applies no coercion. The
	// document's own targetNamespace attribute stays readable on schemaElem, so
	// chameleon is derived, never stored twice (STYLE D3).
	target string
	// ov is the ·override pre-processing· in force over this document (§4.2.5,
	// §F.2), nil when it was reached plainly or is the root. It substitutes for
	// this document's OWN top-level source declarations; the substituted
	// declarations are nonetheless produced by THIS producer, so they take this
	// document's target namespace and schema-level defaults, which is what §4.2.5's
	// document-level-defaults note requires (PRINCIPLES 16). The one thing they do
	// NOT take is this document's §F.1 chameleon coercion of unqualified QName
	// references, which §4.2.5 clause 3.2.1 orders BEFORE the substitution — see
	// unqualifiedRefNS.
	ov *overrideSet
	// rd holds the ·redefinition·s in force over this document (§4.2.4,
	// parser/redefine.go) — one per reading of it the assembly merged into this
	// discovery, empty on the [Produce] path. Like ov they are a property of the
	// PATHS the document was reached by, not of the document (STYLE D3). A
	// top-level declaration EVERY reading redefines contributes no component
	// (§4.2.4 clause 4.1.2), and each reading is where the declaration it redefines
	// is recorded as the original a self-reference in the redefining document
	// resolves to (src-expredef).
	rd redefinitions
	// redefines are the sets read from THIS document's own <redefine> children, in
	// document order — the other side of the same values: rd is what some other
	// document's <redefine> did to this one, redefines is what this document's
	// <redefine>s did to others. It is empty on the [Produce] path, which follows
	// no ·inter-schema-document reference·, and that emptiness is what makes a
	// <redefine> skipped there rather than half-applied.
	redefines []*redefineSet
	builder   *xsd.SchemaBuilder
	symbols   *symbols
}

// newProducer returns the build context for one document of an assembly. target
// is THIS document's effective target namespace (see [producer].target), ov the
// override in force over it (nil for none), rd the redefinitions in force over it
// (empty for none), redefines the sets read from its own <redefine> children
// (empty for none) and sym the assembly's shared symbol table; all are set at
// construction and never mutated into validity afterward (STYLE T1).
func newProducer(doc *Document, target string, ov *overrideSet, rd redefinitions, redefines []*redefineSet, builder *xsd.SchemaBuilder, sym *symbols) *producer {
	return &producer{schemaElem: doc.Root(), target: target, ov: ov, rd: rd, redefines: redefines, builder: builder, symbols: sym}
}

// chameleon reports whether this document is produced under chameleon coercion
// (§4.2.3 clause 2.3 / §F.1): it declares no targetNamespace of its own, yet the
// document that <include>d it has one. An <import>ed no-namespace document is
// therefore NOT a chameleon — the assembler discovers it under its own (absent)
// namespace, leaving target empty. It is a derived predicate computed on each
// call, never a stored field (STYLE D3), mirroring [Document.IsSchema].
func (p *producer) chameleon() bool {
	if p.target == "" {
		return false
	}
	_, own := p.schemaElem.Attr("targetNamespace")
	return !own
}

// rejectS4SFaults walks el's subtree once, before any producer runs, applying
// every schema for schema documents guard that is keyed on the element ITSELF
// rather than on a producer that would have reached it. A producer whose own
// grammar does not mention an element never looks at it, so a fault written
// where no producer descends would otherwise be silently discarded rather than
// rejected (#928).
//
// It descends through XSD-namespace elements only, and never into <appinfo> or
// <documentation>: those hold <xs:any processContents="lax"> content
// (xmlschema11-1.md:5727, :5740), where an element that happens to be named
// {XMLSchemaNS}annotation or {XMLSchemaNS}notation is content no guard here
// governs. It DOES descend into <annotation> — that is how
// rejectRepeatedAnnotations reaches an <annotation>'s direct children — but
// stops at the <appinfo>/<documentation> below it.
//
// Placement is charged before content: a <notation> standing where the grammar
// admits none is reported for where it stands, not for the second <annotation>
// it also carries. The four attribute guards run last, for the same reason:
// where an element stands, and how many of a child it carries and where, are
// all answered ahead of what its start tag spells — and an attribute name the
// grammar declares nowhere, or prohibits on a nested <complexType>, ahead of
// what an id or a block=/final= family attribute's value spells.
//
// <appinfo> and <documentation> are subject to rejectUndeclaredAttrs alone, and
// the split is the lax-content rule above: their CONTENT is governed by no guard
// here, their own start tag is still their own production's — xs:appinfo declares
// source (:5720-:5731) and xs:documentation source and xml:lang (:5733-:5745),
// neither of them through xs:annotated, and neither declares id.
//
// ids is the document's ID table so far, threaded through the walk for
// rejectInvalidID: the root call passes an empty one, and every element the walk
// reaches is visited in document order.
func rejectS4SFaults(el *Element, ids map[string]*Element) error {
	if el.Name().Space() != xsd.XMLSchemaNS {
		return nil
	}
	if isXSD(el, "appinfo") || isXSD(el, "documentation") {
		return rejectUndeclaredAttrs(el)
	}
	if err := rejectMisplacedNotation(el); err != nil {
		return err
	}
	if err := rejectRepeatedAnnotations(el); err != nil {
		return err
	}
	if err := rejectLateAnnotation(el); err != nil {
		return err
	}
	if err := rejectUndeclaredAttrs(el); err != nil {
		return err
	}
	if err := rejectLocalComplexTypeAttrs(el); err != nil {
		return err
	}
	if err := rejectInvalidID(el, ids); err != nil {
		return err
	}
	if err := rejectInvalidDerivationSet(el); err != nil {
		return err
	}
	for _, child := range el.Children() {
		c, ok := child.(*Element)
		if !ok {
			continue
		}
		if err := rejectS4SFaults(c, ids); err != nil {
			return err
		}
	}
	return nil
}

// rejectInvalidID rejects el's id attribute when its value is not a valid
// xs:ID, and otherwise records it in ids, the document's ID table keyed by the
// normalized value and holding the first element to carry each. Every XSD element
// the schema for schema documents declares id on types it xs:ID (xs:annotated,
// xmlschema11-1.md:4438; <schema> :4580; <annotation> :5759), and §5.1's first
// bullet (sd-valid, :615) makes the document's validity against that schema
// normative, so the attribute is judged as an instance of it:
//
//   - A value outside xs:ID's ·lexical space· — NCName's, the same pattern
//     (xmlschema11-2.md:1916), so ncNameRE is the check — is charged
//     cvc-datatype-valid (Datatypes §4.1.4), reached through cvc-attribute.
//     The empty value is such a value: it is present, so unlike
//     declarationName's empty name there is no absent attribute to conflate it
//     with.
//   - A value an earlier element of the SAME document already carries is
//     charged cvc-id clause 2 (§3.3.4.5, :1372): the validation root is the
//     document's <schema>, so its ID/IDREF table is this document's alone, and
//     the same value in two documents of one assembly is valid. Uniqueness is
//     not part of datatype validity (xmlschema11-2.md:1911), hence the second
//     rule.
//
// Both compare the ·actual value·: xs:ID carries whiteSpace = collapse
// (xmlschema11-2.md:1917), so id=" a " is valid and collides with id="a".
// collapseTrim is that normalization for the reason declarationName gives — a
// value collapse's interior folding would change is not an NCName either way,
// and is rejected before it reaches ids.
//
// Both are located at the offending element, which is the id attribute's
// position (an xmltree.Attribute carries its owner's). The walk that reaches el
// stops at <appinfo> and <documentation>, so an element in their lax content is
// never judged: §5.1 lets a processor treat invalid <annotation> descendants as
// valid (:616).
//
// GAP(parser): an overridden document's ids are judged over Dold as fetched,
// not over Dold′, which src-override clause 3's Note (xmlschema11-1.md:4171)
// makes the document that must conform; #875 owns it. rejectS4SFaults walks
// the raw root, and a substitute is judged in the overriding document's own
// walk and table. The direction differs by reader, and both directions occur:
// the ncNameRE check charges cvc-datatype-valid on an element §F.2 clause 1
// substitutes away (false reject: Dold's <element name="x" id="1"/>
// overridden by an x carrying no id); the ids lookup charges cvc-id on a
// member Dold′ lacks (false reject: Dold's x and y both carrying id="dup",
// x overridden by one carrying none) and misses a member Dold′ has (false
// accept: Dold's y carrying id="foo", x overridden by one carrying the same
// id="foo").
func rejectInvalidID(el *Element, ids map[string]*Element) error {
	lexical, ok := el.Attr("id")
	if !ok {
		return nil
	}
	id := collapseTrim(lexical)
	if !ncNameRE.MatchString(id) {
		return xsderr.New(ruleDatatypeValid, el.Loc(),
			"<%s> id %q is not in the ·lexical space· of xs:ID, the type the schema for schema documents declares for it (Structures §5.1, §A): an xs:ID is an NCName, which carries no colon and begins with a letter or '_' (Datatypes §3.4.8)",
			el.Name().Local(), id)
	}
	if first, dup := ids[id]; dup {
		return xsderr.New(ruleValidationRootValid, el.Loc(),
			"<%s> id %q duplicates the id of the <%s> at %s: an xs:ID value identifies one element of its schema document (cvc-id clause 2, Structures §3.3.4.5)",
			el.Name().Local(), id, first.Name().Local(), first.Loc())
	}
	ids[id] = el
	return nil
}

// s4sDerivationSet is one of the four simple types the schema for schema
// documents declares a block=/final= family attribute with. Each is the union
// of an xs:token restriction whose one enumeration is "#all" and a list whose
// item type restricts xs:derivationControl to members, so a value is in its
// ·lexical space· exactly when, after whiteSpace = collapse, it is "#all" alone
// or a whitespace-separated list of members. That makes "#all" beside another
// token invalid (it is neither arm), the empty value and a repeated member valid
// (an empty and a repeating list), and every token case-sensitive.
//
// members are transcribed from Appendix A, in the production's own enumeration
// order, on the footing s4sAttrRosters' doc states (produce_s4sattrs.go): name
// and line make each diffable against the spec text one lookup away. They
// coincide member for member with the ·relevant set· of every LOCAL attribute
// typed by them (elementBlockKeywords and its siblings below) and are still a
// separate fact: those sets are §3.3.2.1/§3.4.2.1/§3.16.2.1 mapping facts and
// these are Appendix A grammar facts, and the Default attributes are where the
// two part — blockDefault is xs:blockSet on every property it feeds.
type s4sDerivationSet struct {
	name    string
	members []xsd.DerivationMethod
}

var (
	// s4sDerivationSetType is xs:derivationSet (xmlschema11-1.md:4498), a list
	// of xs:reducedDerivationControl (:4488).
	s4sDerivationSetType = s4sDerivationSet{"xs:derivationSet", []xsd.DerivationMethod{xsd.DerivationExtension, xsd.DerivationRestriction}}
	// s4sBlockSet is xs:blockSet (xmlschema11-1.md:5017), whose list item type
	// is an inline restriction of xs:derivationControl.
	s4sBlockSet = s4sDerivationSet{"xs:blockSet", []xsd.DerivationMethod{xsd.DerivationExtension, xsd.DerivationRestriction, xsd.DerivationSubstitution}}
	// s4sFullDerivationSet is xs:fullDerivationSet (xmlschema11-1.md:4528), a
	// list of xs:typeDerivationControl (:4516).
	s4sFullDerivationSet = s4sDerivationSet{"xs:fullDerivationSet", []xsd.DerivationMethod{xsd.DerivationExtension, xsd.DerivationRestriction, xsd.DerivationList, xsd.DerivationUnion}}
	// s4sSimpleDerivationSet is xs:simpleDerivationSet (xmlschema11-2.md:3833),
	// whose list item type is an inline restriction of xs:derivationControl.
	s4sSimpleDerivationSet = s4sDerivationSet{"xs:simpleDerivationSet", []xsd.DerivationMethod{xsd.DerivationList, xsd.DerivationUnion, xsd.DerivationRestriction, xsd.DerivationExtension}}
)

// admits reports whether lexical is in s's ·lexical space·. Tokens are split by
// xmlSpaceFields, so a U+00A0 inside a value leaves one token that is no member.
func (s s4sDerivationSet) admits(lexical string) bool {
	items := xmlSpaceFields(lexical)
	if len(items) == 1 && items[0] == "#all" {
		return true
	}
	for _, item := range items {
		if !slices.ContainsFunc(s.members, func(m xsd.DerivationMethod) bool { return m.String() == item }) {
			return false
		}
	}
	return true
}

// spelled is s's members as the diagnostic lists them.
func (s s4sDerivationSet) spelled() string {
	names := make([]string, len(s.members))
	for i, m := range s.members {
		names[i] = m.String()
	}
	return strings.Join(names, ", ")
}

// derivationSetAttr is one block=/final= family attribute name paired with the
// s4s type its owner's production declares it with.
type derivationSetAttr struct {
	name string
	typ  s4sDerivationSet
}

// derivationSetAttrs returns the Appendix A production el's position types it
// with and the block=/final= family attributes that production declares, in
// declaration order, or nil attributes where it declares none:
//
//   - A top-level <element> (a child of <schema> or <override>) is
//     xs:topLevelElement, which keeps xs:element's final and block
//     (xmlschema11-1.md:5079-5080); every other <element>, the ref= form
//     included, is xs:localElement (:5110), which prohibits final (:5125) and
//     keeps block.
//   - A top-level <complexType> (a child of <schema>, <redefine> or
//     <override>) is xs:topLevelComplexType, which keeps xs:complexType's final
//     and block (:4797-4798); a nested one is xs:localComplexType, which
//     prohibits both (:4825-4826).
//   - A top-level <simpleType> is xs:topLevelSimpleType, which keeps
//     xs:simpleType's final (xmlschema11-2.md:3865); a nested one is
//     xs:localSimpleType, which prohibits it (:3908).
//   - <schema> declares finalDefault and blockDefault (xmlschema11-1.md:4569-4571).
//
// A prohibited attribute is left out rather than typed by the base production:
// use="prohibited" removes its declaration, so nothing assesses its value and the
// only fault is the attribute itself, which the form's own guard charges
// (rejectLocalElementProhibitedAttrs, rejectRefElementUseProhibitedAttrs,
// rejectLocalComplexTypeAttrs, rejectLocalSimpleTypeAttrs).
func derivationSetAttrs(el *Element) (string, []derivationSetAttr) {
	parent := el.parent
	schemaChild := parent == nil || isXSD(parent, "schema") || isXSD(parent, "override")
	topLevel := schemaChild || isXSD(parent, "redefine")
	switch el.Name().Local() {
	case "element":
		if !schemaChild {
			return "xs:localElement", []derivationSetAttr{{"block", s4sBlockSet}}
		}
		return "xs:topLevelElement", []derivationSetAttr{{"final", s4sDerivationSetType}, {"block", s4sBlockSet}}
	case "complexType":
		if !topLevel {
			return "xs:localComplexType", nil
		}
		return "xs:topLevelComplexType", []derivationSetAttr{{"final", s4sDerivationSetType}, {"block", s4sDerivationSetType}}
	case "simpleType":
		if !topLevel {
			return "xs:localSimpleType", nil
		}
		return "xs:topLevelSimpleType", []derivationSetAttr{{"final", s4sSimpleDerivationSet}}
	case "schema":
		return "xs:schema", []derivationSetAttr{{"finalDefault", s4sFullDerivationSet}, {"blockDefault", s4sBlockSet}}
	}
	return "", nil
}

// rejectInvalidDerivationSet rejects a block, final, blockDefault or
// finalDefault attribute on el whose value is outside the s4s derivation-set
// type el's production declares it with (derivationSetAttrs): block="foo",
// final="substitution" on a top-level <element>, final="#All", block="#all
// extension". §5.1's first bullet (xmlschema11-1.md:615) makes the document's
// validity against the schema for schema documents normative and no src-* clause
// covers the value, so it is charged cvc-datatype-valid (Datatypes §4.1.4),
// reached through cvc-attribute, exactly as rejectInvalidID charges an id outside
// xs:ID. It is located at el, the attribute's position (an xmltree.Attribute
// carries its owner's). An attribute el's production prohibits is not checked
// here at all, whatever its value: that is its form's prohibition guard's
// charge, and no datatype fault exists beside it.
//
// It is what makes effectiveDerivationSet's inputs well-typed: once it has
// passed, a local block=/final= its form declares names only members of its own
// ·relevant set·, one its form prohibits is rejected by that form's guard before
// any producer maps it, and only a Default attribute can carry a token the
// property it feeds drops.
func rejectInvalidDerivationSet(el *Element) error {
	grammar, attrs := derivationSetAttrs(el)
	for _, attr := range attrs {
		lexical, ok := el.Attr(attr.name)
		if !ok || attr.typ.admits(lexical) {
			continue
		}
		return xsderr.New(ruleDatatypeValid, el.Loc(),
			"<%s> %s %q is not in the ·lexical space· of %s, the type %s declares %s with (Structures §5.1, §A): its value is #all alone or a whitespace-separated list of %s",
			el.Name().Local(), attr.name, lexical, attr.typ.name, grammar, attr.name, attr.typ.spelled())
	}
	return nil
}

// rejectLocalComplexTypeAttrs rejects a NESTED <complexType> — one written
// anywhere but as a child of <schema>, <redefine> or <override> — that carries a
// name, abstract, final or block attribute, each of which xs:localComplexType
// restricts to use="prohibited" (xmlschema11-1.md:4823-4826). Each is legal on
// the top-level form alone: xs:topLevelComplexType (:4804) keeps the declarations
// xs:complexType makes for abstract, final and block (:4795-4798) and makes name
// use="required" (:4811). Position decides the form, on the reading
// rejectLocalSimpleTypeAttrs' doc gives for <simpleType>: <redefine>'s and
// <override>'s content models reach the same global xs:complexType element
// declaration the top level does.
//
// The fault carries NO numbered rule ID: src-ct's clauses (§3.4.3) say nothing
// about these attributes, so it stands on §5.1 (xmlschema11-1.md:4296) directly,
// the footing rejectProhibitedAttrs's doc derives; charging src-ct or
// ct-props-correct would be a fabricated verdict (STYLE E2).
//
// It is keyed on the <complexType> and reached from rejectS4SFaults' walk, one
// guard over every nested position — <element> at either level, <alternative> —
// rather than one per producer that builds an anonymous complex type (STYLE
// D3/T4). The attributes are checked in the grammar's own declaration order, so a
// document writing several is always reported at the same one (STYLE D2).
func rejectLocalComplexTypeAttrs(el *Element) error {
	if !isXSD(el, "complexType") {
		return nil
	}
	parent := el.parent
	if parent == nil || isXSD(parent, "schema") || isXSD(parent, "redefine") || isXSD(parent, "override") {
		return nil
	}
	for _, attr := range [...]string{"name", "abstract", "final", "block"} {
		if _, ok := el.Attr(attr); !ok {
			continue
		}
		return fmt.Errorf("parser: nested <complexType> at %s carries a %s attribute, which the schema for schema documents prohibits on the local form: xs:localComplexType restricts %s to use=\"prohibited\", and it is legal on the top-level form alone", el.Loc(), attr, attr)
	}
	return nil
}

// rejectMisplacedNotation rejects a <notation> written anywhere but as a child
// of <schema> or <override>. The schema for schema documents declares
// <notation> in ONE group arm, xs:schemaTop (xmlschema11-1.md:4462), and
// xs:schemaTop is referenced from exactly two content models: <schema>'s
// (:4562) and <override>'s (:5577). <redefine> is NOT a third — its own model
// reaches xs:redefinable (:5558), which is {simpleType, complexType, group,
// attributeGroup} (:4465-4477) and omits <notation> — so this legal-parent list
// is narrower than rejectLocalSimpleTypeAttrs's.
//
// The fault carries NO numbered rule ID: §3.14.3 and §3.14.4 both answer "None
// as such." (:3409, :3413), so it stands on §5.1's first bullet (:4296)
// directly, exactly as rejectNotationContent does, and charging a src-* verdict
// would be a fabricated rule ID (STYLE E2).
//
// It is keyed on the <notation> and reached from rejectS4SFaults' walk, which
// is one guard covering every illegal parent rather than one admission list per
// parent (STYLE D3/T4). The <appinfo>/<documentation> exclusion is that walk's,
// so an element named {XMLSchemaNS}notation inside lax wildcard content is
// never charged here.
func rejectMisplacedNotation(el *Element) error {
	if !isXSD(el, "notation") {
		return nil
	}
	parent := el.parent
	if parent == nil || isXSD(parent, "schema") || isXSD(parent, "override") {
		return nil
	}
	return fmt.Errorf("parser: <notation> at %s is not admitted inside the <%s> at %s: the schema for schema documents declares <notation> in the xs:schemaTop group alone, which only <schema> and <override> reference", el.Loc(), parent.Name().Local(), parent.Loc())
}

// rejectRepeatedAnnotations rejects, among el's children, two DISTINCT
// annotation faults resting on two DIFFERENT spec footings — do not conflate
// them.
//
// The first is xs:annotated's CARDINALITY: an element other than <annotation>
// carrying a second <annotation> child. The cardinality is the shape of
// xs:annotated itself — <xs:element ref="xs:annotation" minOccurs="0"/> inside a
// <xs:sequence> (xmlschema11-1.md:4436), maxOccurs defaulting to 1 — and that
// type "is extended by all types which allow annotation other than <schema>
// itself" (:4429), so one check over the child list covers every annotatable
// element rather than twenty call sites (STYLE D3/T4). TWO elements depart from
// it, both by declaring their own content model instead of extending
// xs:annotated, and both admit <annotation> unboundedly and interspersed:
// <schema> (:4558, :4563, plus the composition group it repeats unboundedly at
// :4555, whose own branch at :4448 is <annotation>) and <redefine>
// (:5556-5559). <override> is NOT among them: it bypasses xs:annotated too, but
// its own particle (:5576) is a plain <xs:element ref="xs:annotation"
// minOccurs="0"/>, so the default cardinality binds it like the rest.
//
// The second is <annotation>'s OWN CONTENT MODEL: an <annotation> whose direct
// children include an <annotation>. <annotation>'s content is
// (appinfo | documentation)* (:5747-5763, prose "Content: (appinfo |
// documentation)*" at :3480) — no <annotation> branch exists, so a nested
// <annotation> is inadmissible at ANY cardinality, one child no less than two.
// This is not the cardinality fault above and does NOT inherit its {schema,
// redefine} exemption: those exempt a PARENT from xs:annotated's maxOccurs="1",
// not an <annotation> from its own content model.
//
// Neither fault carries a numbered rule ID: §3.15.3, §3.15.4 and §3.15.5 each
// answer "None as such" for annotations (xmlschema11-1.md:3499, :3503, :3507),
// and no s4s-* identifier exists in the spec. Both stand on §5.1 (:4296)
// directly, the footing rejectProhibitedAttrs's doc derives in full, so charging
// a src-* or cos-* verdict would be a fabricated rule ID (STYLE E2).
//
// It checks ONE element's children; rejectS4SFaults is the walk that reaches
// every element of the document with it, and owns the <appinfo>/<documentation>
// exclusion that keeps lax wildcard content out of both faults.
func rejectRepeatedAnnotations(el *Element) error {
	if isXSD(el, "annotation") {
		if found := childElements(el, xsd.XMLSchemaNS, "annotation"); len(found) > 0 {
			return fmt.Errorf("parser: <annotation> child of <annotation> at %s, which the schema for schema documents prohibits: xs:annotation's content model is (appinfo|documentation)* and admits no nested <annotation> at any cardinality", found[0].Loc())
		}
	}
	if !isXSD(el, "annotation") && !isXSD(el, "schema") && !isXSD(el, "redefine") {
		if found := childElements(el, xsd.XMLSchemaNS, "annotation"); len(found) > 1 {
			return fmt.Errorf("parser: repeated <annotation> at %s, which the schema for schema documents prohibits: its parent <%s> at %s admits at most one, the maxOccurs=\"1\" xs:annotated defaults to, and only <schema> and <redefine> depart from that type", found[1].Loc(), el.Name().Local(), el.Loc())
		}
	}
	return nil
}

// prescan registers this document's top-level named <simpleType>s and
// <complexType>s (forward base= references, §3.1.3/§3.4.2), named <group>s
// (forward <group ref> resolution for §3.4.2.3.3 clause 4.2.3's sub-case test,
// resolveModelGroup) and top-level <element>s (forward substitutionGroup= heads
// for §3.3.2.1's {type definition} clause 3, substitutionGroupHeadType) in the
// assembly-wide symbol table, building nothing yet. EVERY document's prescan
// runs before ANY document's run, so a reference in one document reaches a
// definition in another (§4.2.3 c-incl-incl). Names are minted in the effective
// target namespace, so a chameleon document's definitions are registered under
// the including namespace (§F.1 task a). A named <attributeGroup> is registered
// nowhere: an <attributeGroup ref> is resolved at finalize against xsd's
// {attribute group definitions} (#479), which run's build of each definition
// enters.
//
// A name an <override> in force over this document substitutes for is registered
// with the OVERRIDING declaration (§F.2 clause 1), so a base= naming it reaches
// the replacement rather than the replaced definition — "overriding components
// are constructed as if the overridden components had never existed" (§4.2.5).
// An <attributeGroup ref> reaches the replacement the same way, because run
// builds only the replacement's component.
//
// A <redefine> child of this document contributes its OWN children as this
// document's definitions (§4.2.4 clause 4.1.1), so they are registered too — see
// prescanRedefine. In the other direction, a name a <redefine> in force over
// this document replaces is NOT registered here: §4.2.4 clause 4.1.2 excepts it
// from the components this document contributes, and it is recorded instead as
// the original the redefining declaration's self-reference resolves to
// (src-expredef clauses 1.1 and 2).
//
// It also registers this document's named <unique>/<key>/<keyref>s for forward
// <key ref="…"> resolution, but from the WHOLE subtree of each top-level
// declaration rather than from its top level: see prescanIdentityConstraints.
//
// It runs rejectS4SFaults over the whole document first, before any name is
// registered and before any body is walked.
func (p *producer) prescan() error {
	if err := rejectS4SFaults(p.schemaElem, map[string]*Element{}); err != nil {
		return err
	}
	for _, child := range p.schemaElem.Children() {
		el, ok := child.(*Element)
		if !ok {
			continue
		}
		if isXSD(el, "redefine") {
			if err := p.prescanRedefine(el); err != nil {
				return err
			}
			continue
		}
		decl := p.ov.replacement(el)
		if !compositionDirective(el) {
			p.prescanIdentityConstraints(decl)
		}
		name, ok := el.Attr("name")
		if !ok {
			continue
		}
		// Every reading whose <redefine> replaces this declaration takes it as the
		// hidden original src-expredef pairs that redefining declaration with,
		// recorded under THIS producer so its body is still mapped in its own
		// document's context.
		if err := p.rd.recordOriginals(componentKey{kind: el.Name().Local(), name: name}, typeSource{elem: decl, owner: p}); err != nil {
			return err
		}
		if p.rd.excepts(el) {
			// §4.2.4 clause 4.1.2: EVERY reading of this document explicitly redefines
			// this declaration, so no reading contributes a component under its own
			// name. One that does not redefine it still contributes it, and the
			// redefinition then collides with it under sch-props-correct clause 2.
			continue
		}
		switch {
		case isXSD(el, "simpleType"):
			p.symbols.simpleTypes[xsd.QName{Space: p.target, Local: name}] = typeSource{elem: decl, owner: p}
		case isXSD(el, "complexType"):
			p.symbols.complexTypes[xsd.QName{Space: p.target, Local: name}] = typeSource{elem: decl, owner: p}
		case isXSD(el, "group"):
			p.symbols.modelGroups[xsd.QName{Space: p.target, Local: name}] = typeSource{elem: decl, owner: p}
		case isXSD(el, "element"):
			p.symbols.elements[xsd.QName{Space: p.target, Local: name}] = typeSource{elem: decl, owner: p}
		}
	}
	return nil
}

// prescanIdentityConstraints registers every NAMED <unique>/<key>/<keyref> in
// el's subtree, so a <key ref="…"> resolves whatever the document order and
// whichever document of the assembly declares the target (§3.1.3). It descends
// the whole subtree because §3.17.2 sources a schema's {identity-constraint
// definitions} from "all the <key>, <keyref>, and <unique> element information
// items ANYWHERE within the [[children]]" — a constraint on a local <element>
// nested arbitrarily deep in a content model is as referenceable as a top-level
// declaration's.
//
// The ref= form is deliberately NOT indexed: it declares nothing (§3.11.2), so
// a reference chain is unrepresentable and there is no cycle to guard
// (PRINCIPLES 9). A name is minted in the effective target namespace, exactly as
// produceIdentityConstraint mints the definition's own {name}, so the index key
// and the component name agree under chameleon coercion (§F.1 task a).
//
// The walk is confined to what THIS producer actually maps, by three exclusions.
// prescan withholds the composition directives' subtrees (compositionDirective),
// whose contents belong to another document's producer. The two below are the
// two ways an element can stand in a schema document while corresponding to no
// component at all, and each is tested on el ITSELF, before el is indexed and
// before its children are reached — so the exclusion covers the whole subtree,
// including el, at both the roots the pre-scan is entered from (prescan,
// prescanRedefine) and every node beneath them.
//
// An element outside the Schema namespace is excluded because §A gives the
// schema for schema documents no element wildcard outside <appinfo> and
// <documentation>: such an element corresponds to no component wherever it
// stands, and so does everything written beneath it.
//
// An <annotation> is excluded because <appinfo> and <documentation> hold mixed,
// processContents="lax" content (§A), and §3 is explicit that "neither the
// correspondences described nor the XML Representation Constraints apply to
// elements in the Schema namespace which occur as descendants of <appinfo> or
// <documentation>": a <key name="…"> there is prose — an illustration, possibly
// truncated — and is mapped to no component by anyone.
//
// Indexing either would make the index a strict SUPERSET of
// {identity-constraint definitions}, the very property src-resolve clause 1.7
// looks a ref= up in, letting an unmapped element shadow a real same-named
// definition or satisfy a ref= that resolves to nothing.
func (p *producer) prescanIdentityConstraints(el *Element) {
	if el.Name().Space() != xsd.XMLSchemaNS {
		return
	}
	if isXSD(el, "annotation") {
		return
	}
	p.indexIdentityConstraint(el)
	for _, child := range el.Children() {
		c, ok := child.(*Element)
		if !ok {
			continue
		}
		p.prescanIdentityConstraints(c)
	}
}

// indexIdentityConstraint registers el in the assembly-wide index under its
// name=, when el is a NAMED <unique>/<key>/<keyref>; every other element is left
// unindexed, including the ref= form, which declares nothing (§3.11.2).
//
// The caller owns the question of whether el is mapped at all
// (prescanIdentityConstraints); this function owns only the question of whether
// a mapped element declares an identity constraint.
//
// The key is minted by declarationName, the same predicate that mints the {name}
// produceIdentityConstraint builds the definition under, so a ref= resolves
// against exactly the expanded names the definitions carry — an index keyed on
// the raw attribute would miss a whitespace-bearing name whose ·actual value·
// the definition normalizes away (§3.4.7.1, whiteSpace = collapse). A name
// declarationName REJECTS is left unindexed and its error dropped here: this
// pre-scan reports nothing, and produceIdentityConstraint charges that same
// value cvc-datatype-valid at the definition's own position (#675).
func (p *producer) indexIdentityConstraint(el *Element) {
	category, ok := identityConstraintCategoryOf(el.Name().Local())
	if !ok {
		return
	}
	if _, ok := el.Attr("name"); !ok {
		return
	}
	name, err := declarationName(el, p.target)
	if err != nil {
		return
	}
	p.symbols.identityConstraints[name] =
		identityConstraintSource{elem: el, category: category, owner: p}
}

// compositionDirective reports whether el is one of the four <schema> children
// whose subtree the identity-constraint pre-scan must NOT walk from here.
//
// Three of them — <include>, <import>, <override> — contribute no component of
// their own at all (§4.2.3, §4.2.6.2, §4.2.5), only the components of the
// document they name, and run skips all three: an <override>'s children are
// top-level declarations of the OVERRIDDEN document (§F.2 clause 1) and are
// produced, with their own target namespace and their own <schema> defaults, by
// that document's producer, which pre-scans them itself through
// overrideSet.replacement. The fourth, <redefine>, is the exception in both
// respects: its children ARE definitions of this document (§4.2.4 clause 4.1.1),
// so run produces them and prescanRedefine walks them — which is why prescan
// handles <redefine> before consulting this predicate at all, and why the
// predicate is not the same question as "does run skip it".
func compositionDirective(el *Element) bool {
	return isXSD(el, "include") || isXSD(el, "import") || isXSD(el, "override") || isXSD(el, "redefine")
}

// topLevelMapped reports whether this producer maps a <schema> child named local
// anywhere at all.
//
// It is the one statement of that vocabulary: run's dispatch below is exhaustive
// over it and consults it through rejectUnmappedTopLevel rather than carrying a
// default arm of its own, and census (census.go) reports exactly its complement.
// Neither can therefore disagree with the other about what is mapped (STYLE D3) —
// a name added to run's switch alone is rejected by that guard before the switch
// is reached, which its own test sees.
//
// The thirteen names are also exactly the children the schema for schema
// documents admits under <schema>, which is what lets rejectUnmappedTopLevel
// charge their complement as a grammar fault: xs:composition (:4442) and
// xs:schemaTop (:4451, through xs:redefinable :4465) hold twelve of them and
// <schema>'s own sequence holds <defaultOpenContent> (:4557). A name mapped here
// but inadmissible there — or admissible there and unmapped here — would break
// that identity, and rejectUnmappedTopLevel's doc states what it costs.
//
// Two of the names have no arm in run, for DIFFERENT reasons.
// <defaultOpenContent> is mapped by another pass of this producer:
// checkDefaultOpenContent reads it, as does every complex type that reaches
// §3.4.2.3.3 clause 5.2 (produce_complex.go). <annotation> maps to NO component
// at all — this producer builds nothing from it, and rejectS4SFaults only judges
// where the element stands. It is admitted here because §3.15.1 puts annotations
// outside ·validation· altogether — "Annotations do not participate in
// ·validation· as such. Provided an annotation itself satisfies all relevant
// ·Schema Component Constraints· it cannot affect the ·validation· of element
// information items" (:3465) — so no verdict this processor reaches can be short
// by the component it never builds.
func topLevelMapped(local string) bool {
	switch local {
	case "simpleType", "complexType", "element", "attribute", "group", "attributeGroup", "notation",
		"include", "import", "override", "redefine",
		"annotation", "defaultOpenContent":
		return true
	}
	return false
}

// topLevelDecls yields, in strict document order, every <schema> child this
// producer considers at all.
//
// It is the one walk of that top level: run below dispatches over it and census
// (census.go) takes topLevelMapped's complement over it, so a change to either
// skip test moves both readers together and neither can come to believe the
// other looked at a different set of children (STYLE T4).
//
// Each child is passed through the override in force over this document before
// it is yielded (§F.2 clause 1, #c-override-xslt-match, :6568): a top-level
// source declaration an <override> names is yielded as the OVERRIDING
// declaration in the overridden one's place, and every other child as written.
// The substitution never changes the element TYPE — it is half of the matching
// key — so a caller switching on the yielded name sees the same name either way.
//
// Three children are not yielded:
//
//   - a non-element child (comment, text, PI), which maps to nothing anywhere
//     and is not a construct;
//   - a child a ·redefinition· in force over this document excepts (§4.2.4
//     clause 4.1.2): the redefining declaration is produced by the REDEFINING
//     document's producer, in its own document-order position there, and this
//     one survives only as the hidden original behind it (src-expredef);
//   - a child outside the XSD namespace, per the gap below.
//
// GAP(parser): a top-level <schema> child outside the XSD namespace is neither
// rejected nor censused. <xs:schema>'s content model in the schema for schema
// documents (:4562) has no wildcard arm — xs:openAttrs admits foreign
// ATTRIBUTES, not foreign element children — so such a document is a §5.1
// first-bullet grammar fault (:4296); rejectS4SFaults returns at its own
// namespace test before reaching one, and no other pass of this producer judges
// it. Skipping it here keeps it out of the census too, so
// [AssembledDocument.Unmapped] does not name it either. Which of the two it
// becomes is owned by #1036. No error direction is claimed for it: the value
// withheld is a rejection, and the readers of that rejection are not enumerated
// here (STYLE P3a).
func (p *producer) topLevelDecls(yield func(decl *Element) bool) {
	for _, child := range p.schemaElem.Children() {
		el, ok := child.(*Element)
		if !ok {
			continue
		}
		if el.Name().Space() != xsd.XMLSchemaNS {
			continue
		}
		if p.rd.excepts(el) {
			continue
		}
		if !yield(p.ov.replacement(el)) {
			return
		}
	}
}

// rejectUnmappedTopLevel rejects a <schema> child in the XSD namespace whose
// local name no particle of <schema>'s content model admits (#1380).
//
// The content model is "(xs:composition*, (defaultOpenContent, annotation*)?,
// (xs:schemaTop, annotation*)*)" (xmlschema11-1.md:4554, the prose summary at
// §3.1.2 :3781), and it has no wildcard arm and no lax position — xs:openAttrs,
// which <schema>'s type extends, opens ATTRIBUTES alone (:4415). xs:composition
// (:4442) is the five §4.2.1 directives and <annotation>; xs:schemaTop (:4451),
// through xs:redefinable (:4465), is the seven declaration and definition forms.
// So the thirteen names the model admits are exactly topLevelMapped's thirteen,
// and this guard reads the mapping vocabulary instead of transcribing the model
// a second time (STYLE D3/T4): the two are the same list, and the diagnostic
// below quotes the model the identity rests on.
//
// It charges the NAME and nothing else. Order and cardinality are not judged
// here and no checkS4SChildOrder model exists for <schema>: <annotation> fills
// three positions of that model — the composition group's own branch (:4448) and
// the trailing repeat of each of the other two — and s4sModel's slots admit no
// name twice, so a transcription would reject annotations the grammar admits.
//
// The fault carries NO rule ID. It is the s4s-grammar class outright (§5.1's
// first bullet, :4296, restating §2.4 clause 1 sd-valid, :615, which Appendix
// B's three tables do not list), the footing rejectMisplacedNotation and
// rejectNotationContent already stand on; borrowing the cvc-* chain a processor
// would reach by validating the schema document as an ordinary instance against
// a materialized s4s schema would misstate the mechanism, which is this
// producer's direct reading of Appendix A (STYLE E2, xsderr/doc.go).
//
// A child outside the XSD namespace never reaches here: topLevelDecls does not
// yield one, and which of rejection or census it becomes is #1036's, not this
// guard's.
func rejectUnmappedTopLevel(schemaElem, decl *Element) error {
	if topLevelMapped(decl.Name().Local()) {
		return nil
	}
	return fmt.Errorf("parser: <%s> at %s fills no position of the content model the schema for schema documents gives the <schema> at %s: that model (xmlschema11-1.md:4554) is (xs:composition*, (defaultOpenContent, annotation*)?, (xs:schemaTop, annotation*)*), where xs:composition is include|import|redefine|override|annotation and xs:schemaTop is simpleType|complexType|group|attributeGroup|element|attribute|notation, so it admits no <%s> in any position",
		decl.Name().Local(), decl.Loc(), schemaElem.Loc(), decl.Name().Local())
}

// run produces each in-scope top-level declaration of the document into the
// shared builder, over topLevelDecls' walk and in its order. prescan must
// already have run — for every document of the assembly, not just this one.
//
// The six named kinds whose {name} the schema for schema documents makes
// use="required" — <simpleType>, <complexType>, <group>, <attributeGroup>,
// <element>, <attribute> — take it from topLevelName, which rejects an unusable
// one before any of them is built. Four of them — <element>, <attribute>,
// <group> and <attributeGroup> — pass rejectProhibitedAttrs first, so an
// attribute the schema for schema documents prohibits on the top-level form is
// named as the fault it is rather than reported as the missing name it causes.
// <simpleType> and <complexType> do not, because they have nothing to check:
// xs:topLevelSimpleType (xmlschema11-2.md:3876) and xs:topLevelComplexType
// (xmlschema11-1.md:4804) prohibit no attribute at all — each only makes name
// use="required", and that pair's prohibitions sit on the LOCAL grammar types
// instead (xs:localComplexType prohibits name, abstract, final and block,
// :4816).
//
// A child topLevelMapped declines is REJECTED before the switch is reached
// (rejectUnmappedTopLevel), which is why the switch has no default arm: the
// vocabulary lives in that predicate alone, and census (census.go) reports its
// complement beside the rejection.
func (p *producer) run() error {
	for decl := range p.topLevelDecls {
		if err := rejectUnmappedTopLevel(p.schemaElem, decl); err != nil {
			return err
		}
		switch decl.Name().Local() {
		case "simpleType":
			name, err := p.topLevelName(decl)
			if err != nil {
				return err
			}
			st, err := p.buildSimpleType(name, decl)
			if err != nil {
				return err
			}
			p.builder.AddType(st)
		case "element":
			if err := rejectProhibitedAttrs(decl, formTopLevel); err != nil {
				return err
			}
			name, err := p.topLevelName(decl)
			if err != nil {
				return err
			}
			ed, err := p.produceElement(name, decl)
			if err != nil {
				return err
			}
			p.builder.AddElement(ed)
		case "attribute":
			if err := rejectProhibitedAttrs(decl, formTopLevel); err != nil {
				return err
			}
			name, err := p.topLevelName(decl)
			if err != nil {
				return err
			}
			ad, err := p.produceAttribute(name, decl)
			if err != nil {
				return err
			}
			p.builder.AddAttribute(ad)
		case "complexType":
			name, err := p.topLevelName(decl)
			if err != nil {
				return err
			}
			// AddType happens HERE, at this type's own document-order position, and
			// never as a side effect of an on-demand base build from some other
			// type: buildComplexType populates the memo only, so a type built early
			// to serve a derivation still enters {type definitions} in document
			// order (STYLE D2).
			ct, err := p.buildComplexType(name, decl)
			if err != nil {
				return err
			}
			p.builder.AddType(ct)
		case "attributeGroup":
			if err := rejectProhibitedAttrs(decl, formTopLevel); err != nil {
				return err
			}
			name, err := p.topLevelName(decl)
			if err != nil {
				return err
			}
			ag, err := p.buildAttributeGroup(name, decl)
			if err != nil {
				return err
			}
			p.builder.AddAttributeGroup(ag)
		case "group":
			if err := rejectProhibitedAttrs(decl, formTopLevel); err != nil {
				return err
			}
			name, err := p.topLevelName(decl)
			if err != nil {
				return err
			}
			// AddModelGroup happens HERE, at this definition's own document-order
			// position, and never inside the on-demand build a clause 4.2.3 sub-case
			// test triggers (buildModelGroupDefinition populates the memo only), so
			// {model group definitions} stays in document order (STYLE D2).
			mgd, err := p.buildModelGroupDefinition(name, decl)
			if err != nil {
				return err
			}
			p.builder.AddModelGroup(mgd)
		case "include", "import", "override":
			// Consumed by the assembler (parse.go) during discovery, BEFORE any
			// document is produced: none contributes a component of its own
			// (§4.2.3, §4.2.5, §4.2.6.2), only the components of the document it
			// names, which reach the builder through that document's own producer.
			// [Produce], which follows no inter-document reference, skips them.
		case "redefine":
			// The one composition directive that DOES contribute components of its
			// own: its children are definitions of this document (§4.2.4 clause
			// 4.1.1, src-expredef). The document it names is still the assembler's
			// to discover, and reaches the builder through its own producer minus
			// the declarations this <redefine> excepts (clause 4.1.2).
			if err := p.produceRedefine(decl); err != nil {
				return err
			}
		case "notation":
			n, err := p.produceNotation(decl)
			if err != nil {
				return err
			}
			p.builder.AddNotation(n)
		case "annotation", "defaultOpenContent":
			// Not this slice's scope (§3.1.2). topLevelMapped says, for each
			// separately, what maps it and why the census does not report it.
		}
	}
	return nil
}

// topLevelName expands the name attribute of a top-level <simpleType>,
// <complexType>, <group>, <attributeGroup>, <element> or <attribute> into this
// document's target namespace (§3.17.2: a top-level declaration's {target
// namespace} is the <schema>'s), rejecting one that cannot serve as a {name} at
// all.
//
// The name's LEXICAL rejection — a value that is not an xs:NCName — is
// declarationName's, charged cvc-datatype-valid and shared with every other
// declaration-name path in this producer. What remains here is the ABSENT-or-
// EMPTY name.
//
// That rejection is a plain grammar fault, not an xsderr rule verdict, and it is
// the same fault for all six kinds. The schema for schema documents makes name
// use="required" with type xs:NCName on every one of the six productions
// topLevelGrammar names, so an absent attribute and an empty one are equally
// unusable — which is why the presence flag is deliberately discarded rather
// than branched on, and why the diagnostic cites the production for the whole
// guard rather than splitting it. No numbered Schema Representation Constraint
// states a clause of its own for it: §3.4.3 src-ct and §3.16.3 src-simple-type
// each incorporate the schema for schema documents by reference over their own
// clauses (none about name), and §3.6.3 src-attribute_group is literally "None
// as such". Charging src-ct, src-simple-type, st-props-correct, e-props-correct
// or a-props-correct here would be a fabricated verdict (STYLE E2); this is the
// footing <include> with no schemaLocation already stands on (parse.go).
//
// For the two DECLARATION kinds the parser-level rejection is deliberate
// defense in depth rather than the only enforcement: xsd.NewElementDeclaration
// and xsd.NewAttributeDeclaration independently reject an empty {name} citing
// e-props-correct clause 1 (§3.3.6.1) and a-props-correct clause 1 (§3.2.6.1),
// each through its component's property tableau, where {name} is a Required
// xs:NCName. That verdict stays theirs and is still what the LOCAL paths
// produce; this one supersedes it only for a top-level declaration, where the
// fault belongs to the schema document's grammar and must be reported without
// having built anything.
//
// For <simpleType> this helper is the ONLY enforcement there can be, and
// xsd.NewSimpleType must not grow a matching {name} guard: §3.16.1's tableau
// types Simple Type Definition's {name} "An xs:NCName value. Optional.",
// unqualified, because an anonymous simple type nested in <attribute>,
// <element>, <restriction>, <list> or <union> is a valid component with no
// {name} at all (§3.16.1 std-context: "Required if {name} is absent"). A
// constructor guard would reject every one of them. The required-name shape is
// the top-level SYNTACTIC form's alone — xs:topLevelSimpleType against
// xs:localSimpleType, whose name is use="prohibited" — so it can only be
// enforced here, where the raw XML still distinguishes them (#523).
//
// Rejecting here, in run's dispatch, is what makes the verdict
// CONTENT-INDEPENDENT: every one of the six kinds is judged before a single
// child of it is walked, so a nameless declaration cannot be judged by whether
// its content happens to hold a local element (whose own construction would
// otherwise charge an unrelated rule, and only sometimes) — the defect #206
// found for two of them and this closes for all six.
//
// <notation> is the one top-level named kind that does not come through here,
// and takes the NCName check from declarationName directly. Its empty name is
// covered where it is built: xsd.NewNotation rejects an empty {name} citing
// n-props-correct (§3.14.6), which a nameless <notation> document already
// produces end to end.
func (p *producer) topLevelName(decl *Element) (xsd.QName, error) {
	qname, err := declarationName(decl, p.target)
	if err != nil {
		return xsd.QName{}, err
	}
	if qname.Local == "" {
		grammar := topLevelGrammar(decl.Name().Local())
		if grammar == "" {
			// run dispatches on the local name before calling, so no other kind
			// reaches here; naming a production for one would be fabricated (STYLE
			// E2).
			return xsd.QName{}, fmt.Errorf("parser: <%s> is not one of the six top-level named kinds", decl.Name().Local())
		}
		return xsd.QName{}, fmt.Errorf("parser: top-level <%s> at %s has no usable name: its name attribute is absent or empty, and the schema for schema documents requires one — %s declares name use=\"required\" with type xs:NCName", decl.Name().Local(), decl.Loc(), grammar)
	}
	return qname, nil
}

// topLevelGrammar names the Appendix A production a top-level <simpleType>,
// <complexType>, <element>, <attribute>, <group> or <attributeGroup> is read
// against — the type its global element declaration carries — and returns "" for
// any other name.
//
// It is the ONE encoding of those six names for every caller that must pick one
// (STYLE T4): rejectProhibitedAttrs hangs its prohibition lists on four of them,
// topLevelName cites one in the fault an unusable name raises, and newRedefineSet
// (redefine.go) cites one for a <redefine> child. A REDEFINING declaration takes
// the same production as a top-level one, which is what lets one table serve both
// positions: §4.2.4's content model reaches the SAME global element declarations
// through xs:redefinable (xmlschema11-1.md:4465). The two guards that know their
// kind already — produceComplexType's and produceModelGroupDefinition's, both in
// produce_complex.go — spell that one production literally instead of asking.
//
// Each of the six declares name use="required" with type xs:NCName —
// xs:topLevelSimpleType (xmlschema11-2.md:3876, name at :3883),
// xs:topLevelComplexType (xmlschema11-1.md:4804, :4811), xs:topLevelElement
// (:5086, :5105), xs:topLevelAttribute (:4703, :4714), xs:namedGroup (:5187,
// :5209) and xs:namedAttributeGroup (:5502, :5510) — which is the fact every
// caller's diagnostic states.
func topLevelGrammar(local string) string {
	switch local {
	case "simpleType":
		return "xs:topLevelSimpleType"
	case "complexType":
		return "xs:topLevelComplexType"
	case "element":
		return "xs:topLevelElement"
	case "attribute":
		return "xs:topLevelAttribute"
	case "group":
		return "xs:namedGroup"
	case "attributeGroup":
		return "xs:namedAttributeGroup"
	}
	return ""
}

// rejectProhibitedAttrs rejects a top-level <element>, <attribute>, <group> or
// <attributeGroup>, or a redefining <group>/<attributeGroup> child of
// <redefine>, carrying any attribute the schema for schema documents prohibits
// on it: xs:topLevelElement restricts ref, form, targetNamespace, minOccurs and
// maxOccurs to use="prohibited" (xmlschema11-1.md:5100-:5104),
// xs:topLevelAttribute restricts ref, form, use and targetNamespace
// (:4710-:4713), xs:namedGroup restricts ref, minOccurs and maxOccurs
// (:5210-:5212), and xs:namedAttributeGroup restricts ref (:5511), each
// alongside the required name (:5105, :4714, :5209, :5510). Every one of them
// is legal on the corresponding LOCAL form, which is the whole distinction
// these grammar types draw.
//
// The four lists are NOT symmetric and none contains another, because each names
// only what its own kind's grammar admits SOMEWHERE. use is prohibited on the
// <attribute> side alone and minOccurs/maxOccurs on the <element> side alone,
// since xs:element has no use attribute at any level and xs:attribute no
// occurrence attributes at any level. The same asymmetry separates the two
// definition kinds: xs:group's base pulls in the xs:occurs attribute group
// (:5167), so xs:namedGroup has an occurrence pair to prohibit, while
// xs:attributeGroup never does, so xs:namedAttributeGroup has none — minOccurs
// on a top-level <attributeGroup> is not prohibited, it is simply absent from
// that grammar, a different fault this guard may not claim. Checking use= on an
// <element>, maxOccurs= on an <attribute> or minOccurs= on an <attributeGroup>
// would reject as prohibited what the grammar never admits in the first place.
//
// The fault carries NO numbered rule ID for any of the four kinds, but the route
// to that conclusion differs by kind and neither route may be swapped for the
// other.
//
// For <element> and <attribute> the route is an antecedent that excludes the top
// level: src-element clause 2 (§3.3.3) and src-attribute clause 3 (§3.2.3) both
// open "If the item's parent is not <schema>", and each states the ref/name
// exclusivity inside that antecedent (2.1, 3.1), which a top-level declaration
// does not satisfy. What binds is only those constraints' unnumbered preamble —
// "In addition to the conditions imposed … by the schema for schema documents" —
// which incorporates the grammar by reference without restating it.
//
// For <group> and <attributeGroup> there is no antecedent to read, because there
// is no schema representation constraint at all: §3.7.3 (:2286) and
// src-attribute_group (§3.6.3, :2196) each read, in full, "None as such." There
// is no src-mgd — that identifier appears nowhere in the spec, and neither does
// any s4s-* one — and mgd-props-correct (§3.7.6) and ag-props-correct (§3.6.6)
// are component constraints over the property tableau, which never sees the XML
// attribute. What binds is §5.1 (:4289) directly: a schema document must be
// "fully valid with respect to a schema corresponding to the Schema for Schema
// Documents", an error condition independent of, and additional to, the numbered
// Schema Representation Constraints listed beside it in the same section.
//
// Charging src-element, src-attribute, e-props-correct, a-props-correct,
// mgd-props-correct or ag-props-correct here would be a fabricated verdict
// (STYLE E2); this is the footing topLevelName's own grammar fault and <include>
// with no schemaLocation (parse.go) already stand on. src-attribute clause 6 and
// src-element clause 4 do govern targetNamespace numerically
// (xmlschema11-1.md:868, :1321) and are a DIFFERENT constraint: they bind the
// LOCAL declaration that writes an explicit targetNamespace, which the grammar
// admits, and say nothing about the top-level form that may not write one at
// all.
//
// EVERY caller runs it BEFORE the name is read, and the order is the whole
// point: a document that writes ref writes no name either — on any of the four
// kinds — so the name check would answer first and report the absent name, the
// consequence of writing ref, never the mistake itself. run's dispatch runs it
// before topLevelName; newRedefineSet (redefine.go) runs it before the name
// attribute it pairs a <redefine> child by, and prescanRedefine runs it over the
// declaration §F.2 clause 1 substitutes for such a child — both of them before
// produceRedefinition charges src-expredef's pairing miss over the same entry.
// Running before produceElement, produceAttribute, buildModelGroupDefinition and
// buildAttributeGroup also keeps the verdict content-independent, the discipline
// topLevelName's doc records, and puts this fault ahead of the src-attribute
// clauses produceAttribute charges over the same element item: a top-level use=
// is the prohibited attribute first, whatever value constraint it is paired
// with.
//
// The attributes are checked in the grammar's own declaration order, so a
// document writing more than one of them is always reported at the same one
// (STYLE D2).
//
// The REFERENCE forms of the two definition kinds are the mirror of this guard
// and belong to rejectProhibitedRefAttrs, which is charged where those positions
// are mapped rather than here.
func rejectProhibitedAttrs(decl *Element, form declForm) error {
	local := decl.Name().Local()
	var prohibited []string
	switch local {
	case "element":
		prohibited = []string{"ref", "form", "targetNamespace", "minOccurs", "maxOccurs"}
	case "attribute":
		prohibited = []string{"ref", "form", "use", "targetNamespace"}
	case "group":
		prohibited = []string{"ref", "minOccurs", "maxOccurs"}
	case "attributeGroup":
		prohibited = []string{"ref"}
	default:
		return nil
	}
	// The production each list hangs on comes from topLevelGrammar, so this
	// diagnostic and the unusable-name one cannot disagree about a name (STYLE T4).
	grammar := topLevelGrammar(local)
	for _, attr := range prohibited {
		if _, ok := decl.Attr(attr); !ok {
			continue
		}
		return fmt.Errorf("parser: %s <%s> at %s carries a %s attribute, which the schema for schema documents prohibits on the %s form: %s restricts %s to use=\"prohibited\", and it is legal on the local form alone", form, local, decl.Loc(), attr, form, grammar, attr)
	}
	return nil
}

// rejectProhibitedRefAttrs rejects a <group> in a content model or a nested
// <attributeGroup> in an attribute-group member list — the two REFERENCE forms —
// carrying a name attribute, which the schema for schema documents prohibits on
// them: xs:groupRef restricts name to use="prohibited" beside the required ref
// (xmlschema11-1.md:5223-:5224), and xs:attributeGroupRef does the same
// (:5522-:5523). name is the whole list because ref is the only other attribute
// either grammar type declares, which makes this the exact mirror of
// rejectProhibitedAttrs: each form prohibits what the other requires.
//
// The fault carries NO numbered rule ID, on the footing rejectProhibitedAttrs's
// doc establishes for the same two kinds: §3.7.3 (xmlschema11-1.md:2286) and
// src-attribute_group (§3.6.3, :2196) each read "None as such." in full, there is
// no src-mgd, and mgd-props-correct/ag-props-correct are component constraints
// over the property tableau that never see the XML attribute. What binds is §5.1
// (:4296) directly — a schema document must be fully valid with respect to the
// Schema for Schema Documents — so charging src-resolve, mgd-props-correct or
// ag-props-correct here would be fabricated (STYLE E2).
//
// EVERY caller runs it before it reads ref, which is rejectProhibitedAttrs's
// ordering point from the other side: a document that writes name in a reference
// position generally writes no ref, so the missing-ref fault would answer first
// and report the consequence of the mistake rather than the mistake.
//
// The <group> position is charged at produceGroupRefParticle alone, which every
// path to a reference-form <group> now enters: §3.4.2.3.3 clause 2.1.4's
// maxOccurs="0" arm runs the clause-2.2 mapping and discards its particle rather
// than returning ahead of it (#883), so explicitContent needs no charge of its
// own.
func rejectProhibitedRefAttrs(el *Element) error {
	local := el.Name().Local()
	var grammar string
	switch local {
	case "group":
		grammar = "xs:groupRef"
	case "attributeGroup":
		grammar = "xs:attributeGroupRef"
	default:
		// Both call sites dispatch on the local name, so no other element reaches
		// here; naming one is a caller fault rather than a document fault.
		return fmt.Errorf("parser: <%s> is not a reference-form <group> or <attributeGroup>", local)
	}
	if _, ok := el.Attr("name"); !ok {
		return nil
	}
	return fmt.Errorf("parser: <%s> at %s carries a name attribute, which the schema for schema documents prohibits on the reference form: %s restricts name to use=\"prohibited\", and it is legal on the definition form alone", local, el.Loc(), grammar)
}

// declForm names the position a named declaration is written in, and is read by
// rejectProhibitedAttrs's diagnostic alone. The two positions share one grammar
// type per kind — §4.2.4's content model reaches, through xs:redefinable
// (xmlschema11-1.md:4465), the SAME global <group> and <attributeGroup> element
// declarations (:5331, :5528) the top level does — so they share one prohibition
// list and differ only in what the message calls the offending element.
type declForm string

const (
	formTopLevel   declForm = "top-level"
	formRedefining declForm = "redefining"
)

// ncNameRE matches the NCName production ([Namespaces in XML] NT-NCName) — the
// ·lexical space· of xs:NCName, which Datatypes §3.4.7.1 fixes with the single
// pattern facet "\i\c* ∩ [\i-[:]][\c-[:]]*". It is compiled once, with the
// XSD flavor through [regex.Translate], from the pattern string the generated
// builtin row carries ([builtin.NCNamePattern]), and the NameStartChar/NameChar
// code-point sets behind \i and \c are the ones the regex package owns — neither
// is a second table here (PRINCIPLES 26/27). Those sets are hand-typed rather
// than generated (#989). FlavorXSD output is whole-string anchored, so a match
// means the WHOLE string is an NCName: any string containing ':' fails, as does
// one starting with a digit, '.' or '-', as does the empty string.
var ncNameRE = func() *regexp.Regexp {
	goRE, err := regex.Translate(builtin.NCNamePattern(), regex.FlavorXSD, "")
	if err != nil {
		panic("parser: translating the NCName pattern: " + err.Error())
	}
	return regexp.MustCompile(goRE)
}()

// declarationName bundles el's name attribute with the target namespace ns into
// the {name}/{target namespace} pair a named declaration carries, rejecting a
// name that is not in the ·lexical space· of xs:NCName. It is the SINGLE
// NCName check for every declaration name this producer maps — the six
// top-level kinds routed through topLevelName, the <notation> and <redefine>
// child that are not, and the local <element>/<attribute> forms.
//
// The name attribute's ·actual value· is the whiteSpace-normalized one, and the
// {name} it yields is that normalized string: xs:NCName carries whiteSpace =
// collapse (§3.4.7.1), so <element name="a "> declares "a". Trimming the four
// §4.3.6 whitespace characters is that normalization here, not a private copy of
// the collapse algorithm (STYLE T4) — collapse's other step, folding interior
// #x20 runs to one, cannot turn a non-NCName into an NCName, since no NCName
// contains a space at all.
//
// The charge is cvc-datatype-valid (Datatypes §4.1.4), the same footing #343
// put a QName-valued schema-document attribute on: Structures §5.1 makes a
// schema document's validity against the Schema for Schema Documents (§A)
// normative, §A types name as xs:NCName on every one of these productions, and
// a value outside a datatype's lexical space is not ·datatype-valid·. No
// Structures Schema Representation Constraint states a clause for it, so
// charging src-ct, e-props-correct or a-props-correct instead would be a
// fabricated verdict (STYLE E2).
//
// A name containing a colon takes this rejection and not the more specific
// no-xmlns (§3.2.6.3), even when it reads xmlns: or xmlns:a. no-xmlns governs
// the bare string "xmlns", which IS an NCName; its own Note derives the
// xmlns:* prohibition FROM the NCName constraint, so the lexical rule is the
// most specific one that applies to a colonized name (STYLE E2, one rule per
// error).
//
// The EMPTY name keeps whatever rejection it has today and is deliberately not
// recharged here, though the empty string is outside NCName's lexical space
// too: topLevelName conflates an absent name attribute with an empty one on
// purpose, and an ABSENT attribute is a missing-required-attribute fault
// against §A rather than a lexical one, so charging cvc-datatype-valid for the
// pair would mis-describe half of it. The local forms leave the empty name to
// xsd.NewElementDeclaration's e-props-correct clause 1 and
// xsd.NewAttributeDeclaration's a-props-correct clause 1.
func declarationName(el *Element, ns string) (xsd.QName, error) {
	lexical, _ := el.Attr("name")
	name := collapseTrim(lexical)
	if name != "" && !ncNameRE.MatchString(name) {
		return xsd.QName{}, xsderr.New(ruleDatatypeValid, el.Loc(),
			"<%s> name %q is not in the ·lexical space· of xs:NCName, the type the schema for schema documents declares for it (Structures §5.1, §A): an NCName carries no colon and begins with a letter or '_' (Datatypes §3.4.7.1)",
			el.Name().Local(), name)
	}
	return xsd.QName{Space: ns, Local: name}, nil
}

// rejectXmlnsName charges no-xmlns (§3.2.6.3, xmlschema11-1.md:986): "The
// {name} of an attribute declaration must not match xmlns." The constraint is
// one flat clause over one property, stated by §3.2.6 over "All attribute
// declarations", so both productions call it — produceAttribute for the
// top-level form (§3.2.2.1) and produceLocalAttribute for the local one
// (§3.2.2.2), which serves <attributeGroup> and <complexType> alike. Neither
// production's namespace machinery is involved: {name} is "the ·actual value· of
// the name [attribute]" word for word in both mapping summaries, so the two call
// sites pass the same thing and there is no local/top-level difference to
// encode, as there is for no-xsi's {target namespace} below.
//
// name is the {name} declarationName has already mapped, never the name
// attribute re-read here (STYLE T4), and that ordering is what keeps the
// COLONIZED forms on their existing charge: declarationName rejects a name
// outside NCName's ·lexical space· first, so xmlns: and xmlns:a never reach this
// function at all and stay charged cvc-datatype-valid, one fault to one rule
// (STYLE E2). §3.2.6.3's Note — "The {name} of an attribute is an ·NCName·,
// which implicitly prohibits attribute declarations of the form xmlns:*" —
// DERIVES that prohibition from the NCName constraint rather than stating a
// second clause, so nothing here need cover it.
//
// The comparison is against the WHOLE name, and what that protects is NOT those
// colonized forms — the lexical rejection holds them whether the comparison here
// is exact or a prefix, which is why the NCName ruling's test cannot see the
// difference. It is the ordinary NCName that merely begins with those five
// letters: xmlnsx and xmlns-1 are names this constraint says nothing about, and
// TestProduceXmlnsPrefixedAttributeNameAccepted is what a prefix comparison
// fails.
//
// It is charged ahead of no-xsi at both sites, being the constraint over the
// less derived property: {name} is decided the moment declarationName returns,
// while {target namespace} takes the whole form/targetNamespace/
// attributeFormDefault resolution of §3.2.2.2 behind it. Each site charges as
// soon as its own name exists, so the verdict does not depend on unrelated later
// mappings succeeding (#206) — a dangling ref elsewhere in the document no
// longer decides which rule a declaration named xmlns is charged.
//
// §3.3.6.1 e-props-correct carries no equivalent clause, so an ELEMENT
// declaration named xmlns is not an error: this constraint is attribute-only,
// which is why it is not in declarationName, the one helper every declaration
// form shares. A use="prohibited" local <attribute> is charged nothing either —
// it maps to no component at all (§3.2.2), and the constraint is stated over
// declarations.
func rejectXmlnsName(name string, el *Element) error {
	if name != "xmlns" {
		return nil
	}
	return xsderr.New(ruleNoXmlns, el.Loc(),
		"attribute declaration has %q as its {name}, which no-xmlns (§3.2.6.3) forbids whether the declaration is local or top-level: [XML Infoset] identifies an attribute so named as a namespace attribute rather than an ordinary one, so a schema neither need nor may declare it (§3.2.1)", name)
}

// rejectXSITargetNamespace charges no-xsi (§3.2.6.4, xmlschema11-1.md:990): "The
// {target namespace} of an attribute declaration, whether local or top-level,
// must not match http://www.w3.org/2001/XMLSchema-instance (unless it is one of
// the four built-in declarations given in the next section)." Both attribute
// declaration productions call it — produceAttribute for the top-level form and
// produceLocalAttribute for the local one — reading the pair each has already
// mapped rather than re-deriving it (STYLE T4).
//
// name is the declaration's OWN resolved {name}/{target namespace} pair, and the
// two productions resolve the namespace half by disjoint rules: §3.2.2.1 makes a
// top-level declaration's {target namespace} the ancestor <schema>'s
// targetNamespace, while §3.2.2.2 decides a local one from its own
// targetNamespace attribute, its form, and the <schema>'s attributeFormDefault
// (localTargetNS) — leaving an UNQUALIFIED local attribute in no namespace at
// all however the enclosing document is namespaced. So a document whose own
// targetNamespace is the xsi namespace may legally declare local unqualified
// attributes in it, and reading that attribute here as a stand-in for every
// declaration in the document would reject them.
//
// The "unless" needs no encoding: the four §3.2.7 declarations are SEEDED as
// components (seedInstanceAttributes) and reach no production, so every name
// that arrives here was written by a schema document and is forbidden whatever
// it is — xsi:type as squarely as xsi:foo, since the parenthetical exempts the
// four DECLARATIONS rather than their names.
//
// That is also why this Schema Component Constraint is charged at the mapping
// and not on the component, where xsd charges the others: at finalize the four
// seeded declarations are ordinary components, so a check there would have to
// exempt them by name — admitting exactly the four names a schema document may
// not declare. So a declaration built through xsd.NewAttributeDeclaration
// directly carries no such check — unstateable there only for the RESERVED
// four, since on that footing a seeded declaration and a schema's own are the
// same component, and simply unchecked for every other name in the namespace.
// xsd.NewAttributeDeclaration's own GAP(xsd) marker tracks that residue and
// enumerates what reads the component it admits.
//
// Each production charges it as soon as its pair exists, so the verdict does not
// depend on unrelated later mappings succeeding (#206). In the local form that
// point is BEHIND src-attribute clause 6, which gates the targetNamespace
// attribute the resolution reads (rejectLocalAttributeTargetNamespace, #1243).
// A use="prohibited" local <attribute> is charged nothing: it maps to no
// component at all (§3.2.2), and this constraint is stated over declarations.
func rejectXSITargetNamespace(name xsd.QName, el *Element) error {
	if name.Space != xsd.XMLSchemaInstanceNS {
		return nil
	}
	return xsderr.New(ruleNoXSI, el.Loc(),
		"attribute declaration %s has the XML Schema Instance namespace as its {target namespace}, which no-xsi (§3.2.6.4) forbids whether the declaration is local or top-level: the only attribute declarations in that namespace are the four Built-in Attribute Declarations of §3.2.7 — xsi:type, xsi:nil, xsi:schemaLocation, xsi:noNamespaceSchemaLocation — which every schema holds by definition and no schema document may declare", name)
}

// buildSimpleType returns the compiled simple type named name, building it once
// and memoizing the result. name is the zero QName only via constructSimpleType
// for anonymous inline types, which never enter this memoized path.
//
// IT CARRIES NO CYCLE GUARD, and no longer needs one. The tri-state on-stack
// sentinel this used to keep — and the st-props-correct clause 2 rejection built
// on it — existed because constructSimpleType RECURSED through a named base to
// obtain the live *xsd.SimpleType xsd.NewSimpleType demanded, so a circular
// base= would have re-entered this function for the same name and not
// terminated. The base is now DEFERRED: resolveBase emits an xsd.SimpleTypeRef
// for every by-name base and builds nothing, so no named simple type can
// re-enter its own construction and there is no recursion for a guard to bound.
// The parser therefore charges neither st-props-correct clause 2 nor src-resolve
// clause 1.1 for a simple type's base; finalize charges both, from xsd's
// checkSimpleBaseAcyclic and simpleTypeOfRef.
//
// The MEMO survives, and is not optional: resolveBaseType reads it (and falls
// through here on a miss) to obtain the live component a COMPLEX type's base=
// still needs, because xsd.NewComplexType demands a complete base component at
// construction. Its states are now two, not three — absent means unbuilt,
// present means built — so "started but unrecorded" is not representable
// (STYLE T1/D3).
func (p *producer) buildSimpleType(name xsd.QName, elem *Element) (*xsd.SimpleType, error) {
	if st, done := p.symbols.built[name]; done {
		return st, nil
	}
	st, err := p.constructSimpleType(name, elem)
	if err != nil {
		return nil, err
	}
	p.symbols.built[name] = st
	return st, nil
}

// buildComplexType returns the compiled complex type named name, building it
// (and, through resolveBaseType, its base chain) on demand with memoization and
// a cycle guard — the complex-type twin of buildSimpleType, for the same reason:
// §3.4.2's mapping rules "depend upon the {base type definition} having been
// identified before they apply", and xsd.NewComplexType demands a complete
// {content type} at construction, so the base COMPONENT must exist before the
// derived type can be built at all.
//
// It is the single entry point for a NAMED complex type: run's top-level
// dispatch, produceRedefinition's redefining <complexType>, and
// resolveBaseType's on-demand construction all go through it, so a named
// DECLARATION is mapped exactly once. That is what makes a reference to a
// redefined name resolve to the REDEFINITION from both documents, as
// src-expredef's note requires: the redefining declaration is what
// prescanRedefine registered under that name, so every by-name route ends at its
// memo entry. It populates the memo only — registering the component with the
// builder is run's or produceRedefinition's job, at the type's own
// document-order position.
//
// The memo is keyed by the DECLARATION, so name and elem are not two encodings
// of one fact: elem says WHICH <complexType> is mapped and name is the {name} the
// component is minted with, and the two come apart exactly where §4.2.4 makes
// two declarations claim one expanded name (see symbols.builtComplex). Keyed by
// name, produceRedefinition's own call handed back the ORIGINAL of a name the
// redefined document also contributes (#1361).
//
// An ANONYMOUS <complexType> deliberately does NOT come through here: it calls
// produceComplexType directly (produceElement and produceLocalElement for an
// inline child, redefinedComplexBase for src-expredef clause 1.1's original).
// Nothing can NAME it, so no by-name route could reach it and it would share a
// memo entry with nothing.
//
// Nor can it MEMBER a cycle this function's guard would catch — but the reason
// is narrower than it once was, and the difference is load-bearing. Nothing can
// NAME an anonymous type, so it can be no cycle's entry point and this guard
// would never see it. It can nonetheless sit ON a chain that closes: src-expredef
// clause 1.1's original is an anonymous type whose own base= names a top-level
// type again, so a cycle can run THROUGH it. PRINCIPLES 9's "construction order
// makes one impossible" therefore does NOT discharge the anonymous hop, and the
// blanket claim that it did was false the moment #505 landed. The rejection for
// such a chain is the complexOnBaseChain arm below, reached at the named type the
// chain comes back to, and its finalize-side twin xsd/resolve.go's
// checkComplexBaseAcyclic, which descends the anonymous hop for exactly this
// reason.
//
// A declaration whose OWN base hop is still being resolved (complexOnBaseChain)
// is a circular {base type definition} chain, charged ct-props-correct clause 3
// (§3.4.6.1). That is the SAME rule, with the same verdict, that
// xsd/resolve.go's checkComplexBaseAcyclic charges: two entry points on one rule
// for the two construction paths — this one for the producer, whose demand-driven
// recursion would otherwise not terminate, and that one for the programmatic
// SchemaBuilder, which has no producer and must stay self-defending. Neither
// substitutes for the other (PRINCIPLES 9's "detect once at construction" applies
// per construction path).
//
// The guard covers the base hop ONLY, because clause 3 is about {base type
// definition} and nothing else. Once the hop is resolved the declaration moves to
// complexContentPending (enterContentModel) while its content model is built, and
// a derivation reached from THERE — a local element's anonymous type whose base=
// names the enclosing type, particlesEb041's shape — is containment, not a
// cycle: its chain runs through this declaration to a base that is already
// resolved (#1842). That derivation must not need this declaration finished,
// since the memo holds a component only when produceComplexType returns, so
// resolveBaseType answers it from the pending arm without calling in here.
func (p *producer) buildComplexType(name xsd.QName, elem *Element) (xsd.ComplexType, error) {
	switch b := p.symbols.builtComplex[elem].(type) {
	case complexBuilt:
		return b.ct, nil
	case complexOnBaseChain:
		return xsd.ComplexType{}, xsderr.New(ruleCTPropsCorr, elem.Loc(),
			"circular complex type definition: %s derives ultimately from itself, but ct-props-correct clause 3 forbids a circular {base type definition} chain (only xs:anyType may be its own base)", name)
	case complexContentPending:
		return xsd.ComplexType{}, fmt.Errorf("parser: complex type %s at %s was asked for while its content model is still being built; only resolveBaseType reads that state, and it answers from the pending entry without calling buildComplexType", name, elem.Loc())
	}
	p.symbols.builtComplex[elem] = complexOnBaseChain{}

	ct, err := p.produceComplexType(p.namedComplexTypeIdentity(name, elem), elem)
	if err != nil {
		return xsd.ComplexType{}, err
	}
	p.symbols.builtComplex[elem] = complexBuilt{ct: ct}
	return ct, nil
}

// complexBuild is a declaration's builtComplex entry once buildComplexType has
// started it, in one of three arms: complexOnBaseChain while its {base type
// definition} is being resolved, complexContentPending once that is resolved and
// its content model is being built, and complexBuilt once produceComplexType has
// returned. It is a sealed sum over a closed set of build phases.
type complexBuild interface{ complexBuild() }

// complexOnBaseChain marks a declaration whose base hop is in progress; reaching
// it again is ct-props-correct clause 3's circular chain (buildComplexType).
type complexOnBaseChain struct{}

// complexContentPending marks a declaration whose base hop is resolved and whose
// content model is being built. assertions is its {assertions} (§3.4.2.1), already
// final at that point because clauses 1 and 2 read only the resolved base and the
// declaration's own <assert> children: it is all a <complexContent> <restriction>
// reached from that content model reads of its base (resolveBaseType).
type complexContentPending struct{ assertions []xsd.Assertion }

// complexBuilt holds a finished declaration's component.
type complexBuilt struct{ ct xsd.ComplexType }

func (complexOnBaseChain) complexBuild()    {}
func (complexContentPending) complexBuild() {}
func (complexBuilt) complexBuild()          {}

// enterContentModel moves a NAMED declaration from complexOnBaseChain to
// complexContentPending: el's {base type definition} is resolved and assertions
// is its final {assertions}. It is called by the two forms that carry a content
// model, produceImplicitContent and produceComplexContent, between resolving the
// base and building the content. An anonymous identity has no memo entry — it
// never comes through buildComplexType — so it is left alone; that includes a
// src-expredef clause 1.1 original, which is produced from a top-level
// declaration that may itself be on the base chain under its named identity.
func (p *producer) enterContentModel(id complexTypeIdentity, el *Element, assertions []xsd.Assertion) {
	if _, named := topLevelComplexTypeName(id); !named {
		return
	}
	p.symbols.builtComplex[el] = complexContentPending{assertions: assertions}
}

// namedComplexTypeIdentity chooses which NAMED arm of complexTypeIdentity elem
// is built under: the redefining one when elem is a <complexType> child of a
// <redefine> this document followed, and the plain one otherwise.
//
// The question is asked HERE, of the element, rather than at produceRedefinition
// alone, because that is not the only route to a redefining declaration:
// prescanRedefine registers it under its own expanded name, so a reference to
// the redefined name from either document arrives through resolveBaseType and
// buildComplexType instead. Deciding by element makes every route agree, which
// is what keeps the memo holding ONE component per DECLARATION (see
// buildComplexType).
//
// The redefining arm mints the identity src-expredef clause 1.1 needs for the
// original's {context}. One mint per redefinition, and only one is possible:
// buildComplexType's memo means the redefining declaration is produced exactly
// once per assembly.
func (p *producer) namedComplexTypeIdentity(name xsd.QName, elem *Element) complexTypeIdentity {
	if _, _, redefining := p.redefinitionOf(elem); redefining {
		return redefiningComplexType{name: name, owner: xsd.NewComponentID()}
	}
	return namedComplexType{name: name}
}

// resolveBaseType identifies the {base type definition} a base= attribute names
// (§3.4.2 preamble), in BOTH the forms a §3.4.2 mapping needs: the resolved
// COMPONENT (resolvedBase), which the content-type tableaux and §3.4.2.1 clause
// 1's {assertions} fold read, and the xsd.TypeDefinitionOrRef SLOT the built
// component stores. The two are returned together because one decision fixes
// both, and splitting them would let a caller pair a component with a slot that
// does not name it (STYLE D3). A named complex base whose content model holds
// this derivation is not finished yet, so it resolves to the pendingBase arm,
// which carries only the {assertions} its complexContentPending entry holds. at is the
// <restriction>/<extension> carrying the base=, charged for a failure; id is
// the identity of the type being built, which is what makes the redefine branch
// below reachable.
//
// A base of either variety is built through its OWN document's producer
// (typeSource's owner), never through p: see symbols.simpleTypes and
// symbols.complexTypes. An already-built simple base returns the very
// *xsd.SimpleType symbols.built holds — never a rebuilt twin, whose component
// identity would silently diverge (xsd/typedefinition.go); routing through the
// owner does not change that, since the memo is assembly-wide and every producer
// of the assembly shares it.
//
// A name that resolves to no type at all is charged src-resolve clause 1.1
// (§3.17.6.2) — the same rule, at the same clause, that resolveBase charges for
// a simple type's base and that finalize charges for an unresolvable
// {base type definition} reference (xsd/resolve.go's resolveTypeName). One rule,
// three entry points, identical verdict.
func (p *producer) resolveBaseType(id complexTypeIdentity, at *Element, name xsd.QName) (resolvedBase, xsd.TypeDefinitionOrRef, error) {
	if orig, owned, err := p.redefinedComplexBase(id, at, name); owned || err != nil {
		if err != nil {
			return nil, nil, err
		}
		return finishedBase{def: orig}, xsd.InlineTypeDefinition{Definition: orig}, nil
	}
	if name == anyTypeName {
		// The ur-type is declared by no document, so it is in no symbol table and
		// in no build memo: symbols.anyType holds the very component the builder
		// was seeded with (§3.4.7).
		return finishedBase{def: p.symbols.anyType}, xsd.TypeDefinitionRef{Name: name}, nil
	}
	if src, ok := p.symbols.complexTypes[name]; ok {
		// A base whose content model is being built is reached from inside that
		// content model, which is not a cycle (buildComplexType); only its
		// {assertions} can be read.
		if pending, ok := p.symbols.builtComplex[src.elem].(complexContentPending); ok {
			return pendingBase{name: name, assertions: pending.assertions}, xsd.TypeDefinitionRef{Name: name}, nil
		}
		// Unbuilt, built or on the base chain: buildComplexType handles the memo
		// hit and the ct-props-correct clause 3 cycle rejection alike.
		ct, err := src.owner.buildComplexType(name, src.elem)
		if err != nil {
			return nil, nil, err
		}
		return finishedBase{def: ct}, xsd.TypeDefinitionRef{Name: name}, nil
	}
	if st, ok := p.symbols.built[name]; ok && st != nil {
		return finishedBase{def: st}, xsd.TypeDefinitionRef{Name: name}, nil
	}
	if src, ok := p.symbols.simpleTypes[name]; ok {
		st, err := src.owner.buildSimpleType(name, src.elem)
		if err != nil {
			return nil, nil, err
		}
		return finishedBase{def: st}, xsd.TypeDefinitionRef{Name: name}, nil
	}
	return nil, nil, xsderr.New(ruleSrcResolve, at.Loc(),
		"base type %s does not resolve to any type definition in scope (src-resolve clause 1.1)", name)
}

// resolvedBase is the {base type definition} COMPONENT resolveBaseType
// identified, as far as a §3.4.2 mapping can read it when it asks, in one of two
// arms: finishedBase holds the finished component, and pendingBase stands for a
// named complex type whose own content model is still being built
// (complexContentPending), of which only its {assertions} are known.
// assertionsWithBase reads either arm; every other read goes through
// baseComponent, which refuses the pending one. It is a sealed sum over a closed
// set of resolution outcomes.
type resolvedBase interface{ resolvedBase() }

// finishedBase is a base whose component is finished.
type finishedBase struct{ def xsd.TypeDefinition }

// pendingBase is a named complex base reached from inside its own content model:
// name is its expanded name, and assertions its final {assertions}, held by its
// complexContentPending entry.
type pendingBase struct {
	name       xsd.QName
	assertions []xsd.Assertion
}

func (finishedBase) resolvedBase() {}
func (pendingBase) resolvedBase()  {}

// baseComponent returns the finished base for the mappings that read more than
// its {assertions}: §3.4.2.2's simple-type tableau and §3.4.2.3.3 clause 4.2's
// extension content type. at is the derivation alternant charged when the base
// is still being built.
//
// GAP(parser): #1883 — a <simpleContent> derivation, or a <complexContent> <extension>,
// whose base is a named complex type reached from inside that type's own content
// model is refused rather than mapped. Both read the base's {content type}, which
// is not finished until the content model holding this derivation is, and
// xsd.NewComplexType takes a complete {content type} by value, so the recursion
// has no fixed point to construct. No spec rule forbids the extension shape; the
// refusal is this producer's limit, and no suite fixture reaches it.
func baseComponent(base resolvedBase, at *Element) (xsd.TypeDefinition, error) {
	switch b := base.(type) {
	case finishedBase:
		return b.def, nil
	case pendingBase:
		return nil, fmt.Errorf("parser: the <%s> at %s derives from %s from inside %s's own content model, and this producer cannot map a derivation that reads a base {content type} still being built", at.Name().Local(), at.Loc(), b.name, b.name)
	}
	panic("parser: baseComponent: non-exhaustive resolvedBase switch")
}

// redefinedComplexBase builds src-expredef clause 1.1's ORIGINAL when at's base=
// is a redefining <complexType>'s self-reference: "one component which
// corresponds to the top-level definition item with the same name in the
// <redefine>d schema document, as defined in Schema Component Details (§3),
// except that its {name} is ·absent· and its {context} is the redefining
// component". owned is false for every other base=, which then resolves
// ordinarily — including a base= inside a redefining type that names some OTHER
// type, and a reference to the redefined name from anywhere else, both of which
// src-expredef's own note requires to reach the REDEFINITION.
//
// It is the complex-type twin of resolveBase's redefine branch (see there), and
// makes the same three moves for the same reasons:
//
//   - the source is the REDEFINED document's declaration (redefinedTypeBase),
//     never the redefining one, so the original's own base= resolves to whatever
//     that document said and is never re-pointed at the redefinition — the false
//     circularity the pairing exists to prevent;
//   - it is built under that document's OWN producer (src.owner), so it enters
//     no symbol table, is registered with no builder, and takes its own
//     document's target namespace and schema-level defaults;
//   - it goes to produceComplexType DIRECTLY rather than through
//     buildComplexType, because that memo is keyed by the declaration being
//     mapped and this declaration maps to TWO components: the ordinary named one
//     the redefined document contributes when no reading excepts it, and this
//     one, whose {name} clause 1.1 makes ·absent· and whose {context} is the
//     redefining component. Sharing the declaration's memo entry would hand one
//     of them out for the other (see buildComplexType).
//
// The identity it is built with carries the OWNING type's minted
// xsd.ComponentID, which is what makes the original's {context} point back at
// its owner; xsd.NewComplexTypeOwningBase checks the two agree.
//
// The owning type is the redefining one at the first level and, under a CHAINED
// <redefine>, another clause-1.1 original at every level below (#585) — so the
// identity that supplies the mint is asked for it rather than being one arm
// (redefineOriginalContext), and the recursion is what walks the chain. Each hop
// moves to the REDEFINED document, so the chain is finite while the <redefine>
// graph is acyclic over this name; a cycle over it is bounded and rejected by
// enterOriginal, exactly as resolveBase's twin is.
func (p *producer) redefinedComplexBase(id complexTypeIdentity, at *Element, name xsd.QName) (xsd.ComplexType, bool, error) {
	owner, owns := redefineOriginalContext(id)
	if !owns {
		return xsd.ComplexType{}, false, nil
	}
	src, self := p.redefinedTypeBase(at, name)
	if !self {
		return xsd.ComplexType{}, false, nil
	}
	if !p.enterOriginal(src.elem) {
		return xsd.ComplexType{}, true, xsderr.New(ruleCTPropsCorr, at.Loc(),
			"circular complex type definition: %s is redefined around a cycle of <redefine>s, so the original src-expredef clause 1.1 pairs this redefinition with derives ultimately from the redefinition again, but ct-props-correct clause 3 forbids a circular {base type definition} chain (only xs:anyType may be its own base)", name)
	}
	defer p.leaveOriginal(src.elem)

	orig, err := src.owner.produceComplexType(newRedefineOriginal(owner), src.elem)
	if err != nil {
		return xsd.ComplexType{}, true, err
	}
	return orig, true, nil
}

// buildModelGroupDefinition returns the Model Group Definition el declares under
// the name name, building it on demand with memoization — the model-group twin
// of buildComplexType, and like it the SINGLE entry point for mapping a
// top-level named <group>: run's document-order dispatch, produceRedefinition's
// redefining <group> and resolveModelGroup's demand-driven resolution all go
// through it, so one <group> ELEMENT is mapped exactly ONCE. That is a
// correctness requirement here, not a saving — see symbols.builtGroups for the
// duplicate identity-constraint registration a second mapping would fabricate,
// and for why the memo is keyed by that element rather than by name. It
// populates the memo only; registering the component with the builder is run's
// or produceRedefinition's job.
//
// Unlike buildComplexType it charges no cycle rejection of its own. It WRITES the
// on-stack sentinel, but the reader that makes a circular <group ref> graph
// terminate is resolveModelGroup — the only caller re-enterable for a definition
// still being built — and the REJECTION stays mg-props-correct clause 2's at
// finalize, where the whole graph is visible. A declaration therefore arrives
// here either unstarted or done, never on-stack: run is not re-entrant, and
// resolveModelGroup answers "does not resolve" for an on-stack one rather than
// calling in.
func (p *producer) buildModelGroupDefinition(name xsd.QName, el *Element) (xsd.ModelGroupDefinition, error) {
	if mgd := p.symbols.builtGroups[el]; mgd != nil {
		return *mgd, nil
	}
	p.symbols.builtGroups[el] = nil // mark on-stack

	mgd, err := p.produceModelGroupDefinition(name, el)
	if err != nil {
		return xsd.ModelGroupDefinition{}, err
	}
	p.symbols.builtGroups[el] = &mgd // replace the on-stack sentinel with the finished node
	return mgd, nil
}

// resolveModelGroup identifies the Model Group a <group ref> resolves its
// particle's {term} to (§3.7.2, xr.mgd3: "the {model group} of the model group
// definition ·resolved· to by the ·actual value· of the ref attribute"), building
// that definition on demand through its OWN document's producer for the reasons
// typeSource's doc gives.
//
// It is deliberately NOT the general <group ref> resolution — that stays
// finalize's (produceGroupRefParticle keeps the reference deferred). Its one
// caller is xsd.ExtensionContentType, to which it is passed as the group-lookup
// callback, and whose §3.4.2.3.3 clause 4.2.3 sub-case test must know the
// {compositor} behind a ·base particle· or ·effective content· BEFORE the
// {content type} is synthesized, which is at produce time and nowhere else.
//
// ok is false, with no error, for the two states in which no Model Group is
// (yet) knowable here, both of which leave the caller to fall through to a
// sub-case that assumes nothing:
//
//   - name matches no top-level <group> of the assembly. A dangling <group ref>
//     is charged src-resolve clause 1.5 at finalize, against the retained
//     ModelGroupRef; anticipating that verdict here would be a second encoding.
//   - the declaration name resolves to is already on the build stack, i.e. the
//     reference closes a <group ref> cycle. mg-props-correct clause 2 rejects
//     that at finalize; this function must only terminate, never reject (see
//     buildModelGroupDefinition). No schema reaches that branch today — see
//     symbols.builtGroups for why, and for why the branch is kept rather than
//     left to the stack.
//
// The symbol table is consulted BEFORE the memo, unlike its complex-type twin:
// the memo is keyed by declaration (symbols.builtGroups), and this function is
// handed a name, so the declaration that name addresses is what has to be found
// first. There is nothing the memo holds that the symbol table does not name —
// every element reaching buildModelGroupDefinition is a top-level <group> some
// prescan recorded.
func (p *producer) resolveModelGroup(name xsd.QName) (xsd.ModelGroup, bool, error) {
	src, ok := p.symbols.modelGroups[name]
	if !ok {
		return xsd.ModelGroup{}, false, nil
	}
	if mgd, started := p.symbols.builtGroups[src.elem]; started {
		if mgd == nil {
			return xsd.ModelGroup{}, false, nil // PRESENT-nil: on the build stack
		}
		return mgd.ModelGroup(), true, nil
	}
	mgd, err := src.owner.buildModelGroupDefinition(name, src.elem)
	if err != nil {
		return xsd.ModelGroup{}, false, err
	}
	return mgd.ModelGroup(), true, nil
}

// rejectLocalSimpleTypeAttrs rejects a NESTED <simpleType> — one written
// anywhere but as a child of <schema>, <redefine> or <override> — that carries a
// name or a final attribute, both of which xs:localSimpleType restricts to
// use="prohibited" (xmlschema11-2.md:3901, :3908), name's own documentation
// reading "Forbidden when nested". Each is legal on the top-level form alone,
// where xs:topLevelSimpleType makes name REQUIRED (:3883) and leaves final the
// optional attribute the abstract xs:simpleType declares (:3865).
//
// POSITION decides the form, never the name the caller passes: <redefine>'s and
// <override>'s content models reach, through xs:redefinable
// (xmlschema11-1.md:4465), the SAME global xs:simpleType element declaration
// (xmlschema11-2.md:3913) the top level does, typed xs:topLevelSimpleType, so a
// redefining or overriding <simpleType> keeps its name — and resolveBase passes
// the ZERO QName for the src-expredef ORIGINAL such a redefinition is paired
// with, a top-level element whose name §4.2.4 makes ·absent· rather than
// prohibited. Every other position admitting a <simpleType> child types it
// xs:localSimpleType: <list>, <union> and <restriction> (xmlschema11-2.md:3931,
// :3969, :3989), <element> and <attribute> at either level
// (xmlschema11-1.md:4681, :4708, :5057, :5092, :5116) and <alternative> (:5146),
// whose inline arm reaches here through alternativeTypes (#851).
//
// The fault carries NO numbered rule ID: src-simple-type's clauses (§3.16.3) say
// nothing about either attribute, and no s4s-* identifier exists in the spec at
// all. It stands on §5.1 (xmlschema11-1.md:4289, :4296) directly, the footing
// rejectProhibitedAttrs's doc derives in full for the four top-level kinds;
// charging src-simple-type or st-props-correct instead would be a fabricated
// verdict (STYLE E2).
//
// It is checked ONCE here rather than at each call site that constructs a
// <simpleType> with the zero QName (STYLE D3/T4), and BEFORE the body is
// walked, so the prohibited attribute is reported ahead of any content fault the
// same element also has. The two attributes are checked in the grammar's own
// declaration order, so a document writing both is always reported at name
// (STYLE D2).
func rejectLocalSimpleTypeAttrs(elem *Element) error {
	parent := elem.parent
	if parent == nil || isXSD(parent, "schema") || isXSD(parent, "redefine") || isXSD(parent, "override") {
		return nil
	}
	for _, attr := range []string{"name", "final"} {
		if _, ok := elem.Attr(attr); !ok {
			continue
		}
		return fmt.Errorf("parser: nested <simpleType> at %s carries a %s attribute, which the schema for schema documents prohibits on the local form: xs:localSimpleType restricts %s to use=\"prohibited\", and it is legal on the top-level form alone", elem.Loc(), attr, attr)
	}
	return nil
}

// constructSimpleType maps one <simpleType> element (named or anonymous) into a
// component. It is the ONE site a <simpleType>'s own children are read from, top
// level and inline alike, so it is where those children are ordered against
// xs:simpleType's content model (checkS4SChildOrder, s4sSimpleType, #1076) —
// before the body is chosen, since a document holding a child no position of that
// model admits is not fully valid against the schema for schema documents whether
// or not it also names an alternative.
//
// It dispatches on which of the three §3.16.2.1 alternatives the element's body
// chooses — <list> to constructListType, <union> to constructUnionType,
// <restriction> to the code below, which rejects the XSD-namespace children
// §4.1.2's content model has no position for (rejectOutOfModelFacetChildren),
// then the two children sharing one expanded name that src-simple-type clause 1
// forbids (rejectDuplicateRestrictionChildren), then a child out of the order
// s4sSimpleTypeRestriction gives the rest (#1951), resolves the base, maps the
// own facets and {final} (simpleTypeFinal), and constructs. It does NOT memoize
// — the memo/cycle bookkeeping lives in buildSimpleType; an anonymous inline
// type has no name to key on and is unreferenceable, so it is built here
// directly, once.
//
// It does NOT charge the facet-VALUE sub-clauses of cos-st-restricts (§3.16.6.2)
// — facet applicability against the primitive, and the bound/enumeration
// constraints in the base type's value space. Those need both the builtin
// applicability table and a [value.Backend], neither of which package xsd may
// depend on, so they are taken as a finalize-time capability instead: [Produce]
// installs [builtin.NewRestrictionChecker] at [xsd.SchemaBuilder.FinalizeWith]
// and xsd's checkSimpleTypeDerivations pass charges every simple type the
// assembled schema reaches. Charging it here instead would reach only the types
// THIS function builds, and would have to be repeated at every future
// construction site.
func (p *producer) constructSimpleType(name xsd.QName, elem *Element) (*xsd.SimpleType, error) {
	if err := rejectLocalSimpleTypeAttrs(elem); err != nil {
		return nil, err
	}
	if err := checkS4SChildOrder(elem, s4sSimpleType); err != nil {
		return nil, err
	}
	body, err := simpleTypeBody(elem)
	if err != nil {
		return nil, err
	}
	switch body.Name().Local() {
	case "list":
		return p.constructListType(name, elem, body)
	case "union":
		return p.constructUnionType(name, elem, body)
	}
	// The grammar fault is charged before the base is resolved, so the verdict on
	// an s4s-invalid <restriction> does not depend on whether its base= happens to
	// resolve — the content-independence discipline rejectProhibitedAttrs records.
	if err := rejectOutOfModelFacetChildren(body); err != nil {
		return nil, err
	}
	if err := rejectDuplicateRestrictionChildren(body); err != nil {
		return nil, err
	}
	if err := checkS4SChildOrder(body, s4sSimpleTypeRestriction); err != nil {
		return nil, err
	}
	base, err := p.resolveBase(body)
	if err != nil {
		return nil, err
	}
	facets, err := p.restrictionFacets(body)
	if err != nil {
		return nil, err
	}
	// The declared derivation is ·restriction·. It carries no property of its
	// own: §3.16.2.1 gives a restriction the {variety}, {primitive type
	// definition}, {item type definition} and {member type definitions} of its
	// {base type definition}, and xsd.SimpleType derives all four from the base
	// chain, so the producer no longer re-derives any of them here (STYLE D3).
	return xsd.NewSimpleType(elem.Loc(), name, xsd.RestrictionDerivation{}, base, facets, p.simpleTypeFinal(elem))
}

// constructListType maps a <simpleType> whose body is <list> (§3.16.2.1
// map.std.common case 2 plus map.std.list case 1) into ONE component — the very
// one this <simpleType> element declares, named or anonymous:
//
//   - {variety} = list and {base type definition} = xs:anySimpleType, both
//     directly, from map.std.common's <list> alternative. No anonymous
//     intermediate component is synthesized: the two-step shape is a fact about
//     xs:NMTOKENS/xs:IDREFS/xs:ENTITIES' own Datatypes definitions, encoded in
//     builtin.Seed's interposeListBase, and generalizing it here would attribute
//     this element's {name} and {context} to a phantom component.
//   - {item type definition} from listItem.
//   - {facets} = exactly one whiteSpace facet, {value} collapse and {fixed}
//     true, which map.std.common {facets} case 3 MANUFACTURES for every <list>
//     alternative. It is not optional bookkeeping: cos-st-restricts clause
//     2.2.1.2 admits that set and nothing else, so a list produced without it is
//     refused at finalize by xsd's checkConstructedListFacets.
//
// A <restriction> OF a named list type needs nothing here — it maps through the
// ordinary restriction path, and map.std.list case 2 gives it the base's item,
// which xsd.SimpleType.Item derives off the base chain (STYLE D3).
//
// The <list>'s children are ordered against s4sList first (checkS4SChildOrder,
// #1951), ahead of listItem's src-simple-type clause 3, so a second inline
// <simpleType> is charged rather than dropped behind the first.
func (p *producer) constructListType(name xsd.QName, elem, list *Element) (*xsd.SimpleType, error) {
	if err := checkS4SChildOrder(list, s4sList); err != nil {
		return nil, err
	}
	item, err := p.listItem(list)
	if err != nil {
		return nil, err
	}
	whiteSpace := xsd.NewFacet(xsd.FacetWhiteSpace, []string{"collapse"}, true)
	return xsd.NewSimpleType(elem.Loc(), name, xsd.ListDerivation{Item: item},
		xsd.OwnedSimpleType{Definition: xsd.AnySimpleType()}, []xsd.Facet{whiteSpace}, p.simpleTypeFinal(elem))
}

// listItem maps a <list>'s {item type definition} to the [xsd.SimpleTypeOrRef]
// arm that slot takes, enforcing src-simple-type clause 3 (§3.16.3): exactly one
// of an itemType= attribute or an inline <simpleType> child, never both, never
// neither.
//
// The arm split is resolveBase's, by OWNERSHIP: an inline <simpleType> child is
// built here and owned by the slot, and EVERY by-name itemType= is an
// xsd.SimpleTypeRef with no lookup and no build, so a forward-referenced item
// costs no resolution ladder here and its src-resolve clause 1.1 rejection is
// charged once, at finalize.
func (p *producer) listItem(list *Element) (xsd.SimpleTypeOrRef, error) {
	itemLex, hasItem := list.Attr("itemType")
	inline := childElement(list, xsd.XMLSchemaNS, "simpleType")

	if hasItem && inline != nil {
		return nil, xsderr.New(ruleSrcSimpleType, list.Loc(),
			"list has both an itemType attribute and an inline <simpleType> child, but src-simple-type clause 3 allows only one")
	}
	if !hasItem && inline == nil {
		return nil, xsderr.New(ruleSrcSimpleType, list.Loc(),
			"list has neither an itemType attribute nor an inline <simpleType> child, but src-simple-type clause 3 requires exactly one")
	}

	if inline != nil {
		// Anonymous item: built inline, once, with an absent {name} (zero QName).
		st, err := p.constructSimpleType(xsd.QName{}, inline)
		if err != nil {
			return nil, err
		}
		return xsd.OwnedSimpleType{Definition: st}, nil
	}

	qn, err := p.resolveQName(list, itemLex, "itemType")
	if err != nil {
		return nil, err
	}
	return xsd.SimpleTypeRef{Name: qn}, nil
}

// constructUnionType maps a <simpleType> whose body is <union> (§3.16.2.1
// map.std.common plus §3.16.2.4 map.std.union case 1) into ONE component — the
// very one this <simpleType> element declares, named or anonymous, on
// constructListType's argument against synthesizing an intermediate:
//
//   - {variety} = union and {base type definition} = xs:anySimpleType, both
//     directly, from map.std.common's <union> alternative ({base type
//     definition} case 2).
//   - {member type definitions} from unionMembers.
//   - {facets} = the EMPTY set, which map.std.common {facets} case 4 gives every
//     <union> alternative — the <list> alternative's manufactured whiteSpace
//     (case 3) has no union twin. It is not an omission to be filled in later:
//     cos-st-restricts clause 3.2.1.2 admits nothing else for a union
//     constructed directly from xs:anySimpleType, so a produced union carrying
//     any facet is refused at finalize by xsd's checkUnionGraph.
//
// A <restriction> OF a named union needs nothing here — it maps through the
// ordinary restriction path, and map.std.union case 2 gives it the base's
// membership, which xsd.SimpleType.Members derives off the base chain (STYLE
// D3).
func (p *producer) constructUnionType(name xsd.QName, elem, union *Element) (*xsd.SimpleType, error) {
	members, err := p.unionMembers(union)
	if err != nil {
		return nil, err
	}
	return xsd.NewSimpleType(elem.Loc(), name, xsd.UnionDerivation{Members: members},
		xsd.OwnedSimpleType{Definition: xsd.AnySimpleType()}, nil, p.simpleTypeFinal(elem))
}

// unionMembers maps a <union>'s {member type definitions} to the
// [xsd.SimpleTypeOrRef] sequence that property holds, in the ONE order
// §3.16.2.4 map.std.union case 1 fixes: "(a) ·resolved· to by the items in the
// ·actual value· of the memberTypes attribute of <union>, if any, and (b)
// corresponding to the <simpleType>s among the <union> children, if any, in
// order". Every memberTypes= item comes FIRST, in attribute-list order, then
// every inline child, in document order; the two sources never interleave.
// cos-st-restricts clause 3.2.2.3 pairs a restricting union's members with the
// base's POSITIONALLY, so this order is a component property and not a
// presentation choice (STYLE D2).
//
// It enforces src-simple-type clause 4 (§3.16.3): "either it has a non-empty
// memberTypes attribute or it has at least one simpleType child". Unlike clause
// 3 for <list>, the two sources are NOT mutually exclusive — both at once is
// legal and common — so only the neither case is a rejection. A memberTypes=
// present but whitespace-only contributes no item and so is not "non-empty"
// under this clause.
//
// The arm split is resolveBase's, by OWNERSHIP: an inline <simpleType> child is
// built here and owned by the slot, and EVERY by-name memberTypes= item is an
// xsd.SimpleTypeRef with no lookup and no build, so a forward-referenced member
// costs no resolution ladder here and its src-resolve clause 1.1 rejection is
// charged once, at finalize.
func (p *producer) unionMembers(union *Element) ([]xsd.SimpleTypeOrRef, error) {
	memberLex, _ := union.Attr("memberTypes")
	items := xmlSpaceFields(memberLex)
	inlines := childElements(union, xsd.XMLSchemaNS, "simpleType")
	if len(items) == 0 && len(inlines) == 0 {
		return nil, xsderr.New(ruleSrcSimpleType, union.Loc(),
			"union has neither a non-empty memberTypes attribute nor a <simpleType> child, but src-simple-type clause 4 requires at least one of the two")
	}

	members := make([]xsd.SimpleTypeOrRef, 0, len(items)+len(inlines))
	for _, item := range items {
		qn, err := p.resolveQName(union, item, "memberTypes")
		if err != nil {
			return nil, err
		}
		members = append(members, xsd.SimpleTypeRef{Name: qn})
	}
	for _, inline := range inlines {
		// Anonymous member: built inline, once, with an absent {name} (zero QName).
		st, err := p.constructSimpleType(xsd.QName{}, inline)
		if err != nil {
			return nil, err
		}
		members = append(members, xsd.OwnedSimpleType{Definition: st})
	}
	return members, nil
}

// simpleTypeBody returns the ONE §3.16.2.1 alternative a <simpleType> chooses —
// its <restriction>, <list> or <union> child. Neither none nor more than one is
// skipped silently: xs:simpleType's content model (xmlschema11-2.md:2743) admits
// exactly one, and a producer that picked a winner would drop the loser's whole
// mapping.
//
// The alternatives are examined in a fixed order so the rejection message is
// deterministic (STYLE D1) rather than following document order into two
// different messages for the same document.
//
// BOTH rejections carry NO rule ID. Writing two of the three alternatives or none
// fails that content model, which is §5.1's FIRST bullet (xmlschema11-1.md:4296)
// — "not fully valid with respect to … the Schema for Schema Documents" — the
// class carrying no numbered identifier. src-simple-type's four clauses (§3.16.3,
// :3658) each govern a condition WITHIN an alternative already chosen, and none
// states how many are present, so charging it for either branch would be a
// fabricated verdict (STYLE E2). The plain wrapped error is the footing
// checkS4SChildOrder (produce_s4sorder.go) already stands on for this same class.
//
// The two branches differ in REACH, not in class. Since #1076 no schema document
// reaches the MORE-THAN-ONE branch: constructSimpleType walks the same children
// against s4sSimpleType first, and a second alternative repeats a position that
// model admits at most once. It stays because census (census.go) asks this
// function directly and must stop walking a <simpleType> that chose two. The
// NEITHER branch IS reached, and is the one site enforcing "at least one
// alternative" against a real document: s4sSlot carries no required/minOccurs
// notion, so checkS4SChildOrder walks only the children that are present and an
// unfilled alternative position is invisible to it.
func simpleTypeBody(elem *Element) (*Element, error) {
	var chosen *Element
	for _, local := range []string{"restriction", "list", "union"} {
		alt := childElement(elem, xsd.XMLSchemaNS, local)
		if alt == nil {
			continue
		}
		if chosen != nil {
			return nil, fmt.Errorf("parser: <simpleType> at %s has both a <%s> and a <%s> child, but §3.16.2.1 admits exactly one of <restriction>, <list> and <union>",
				elem.Loc(), chosen.Name().Local(), local)
		}
		chosen = alt
	}
	if chosen == nil {
		return nil, fmt.Errorf("parser: <simpleType> at %s has no <restriction>, <list> or <union> child, but §3.16.2.1 requires exactly one of the three alternatives",
			elem.Loc())
	}
	return chosen, nil
}

// resolveBase maps a <restriction>'s {base type definition} to the
// [xsd.SimpleTypeOrRef] arm that slot takes. It enforces src-simple-type clause
// 2 (§3.16.3): exactly one of a base= attribute or an inline <simpleType> child,
// never both, never neither.
//
// WHICH ARM IS THE PRODUCER'S DECISION, and the split is by OWNERSHIP, the same
// split resolveBaseType makes for a complex type (#505):
//
//   - an inline <simpleType> child, and the §4.2.4 src-expredef ORIGINAL a
//     redefining <simpleType> is paired with, are built HERE and owned by this
//     slot: xsd.OwnedSimpleType. Neither has a name for anything to look up.
//   - EVERY by-name base= is xsd.SimpleTypeRef, with no lookup and no build.
//     Not "every base= that is not already built", not "every base= that is not
//     a builtin" — every one, which is what keeps the owned arm from becoming
//     an escape hatch out of deferred resolution (xsd/simpletyperef.go). It is
//     also what removes this function's whole former resolution ladder: the
//     src-resolve clause 1.1 rejection a name with no target used to get here
//     is charged once, at finalize.
func (p *producer) resolveBase(restriction *Element) (xsd.SimpleTypeOrRef, error) {
	baseLex, hasBase := restriction.Attr("base")
	inline := childElement(restriction, xsd.XMLSchemaNS, "simpleType")

	if hasBase && inline != nil {
		return nil, xsderr.New(ruleSrcSimpleType, restriction.Loc(),
			"restriction has both a base attribute and an inline <simpleType> child, but src-simple-type clause 2 allows only one")
	}
	if !hasBase && inline == nil {
		return nil, xsderr.New(ruleSrcSimpleType, restriction.Loc(),
			"restriction has neither a base attribute nor an inline <simpleType> child, but src-simple-type clause 2 requires exactly one")
	}

	if inline != nil {
		// Anonymous base: built inline, once, with an absent {name} (zero QName).
		st, err := p.constructSimpleType(xsd.QName{}, inline)
		if err != nil {
			return nil, err
		}
		return xsd.OwnedSimpleType{Definition: st}, nil
	}

	qn, err := p.resolveQName(restriction, baseLex, "base")
	if err != nil {
		return nil, err
	}
	if src, redefining := p.redefinedTypeBase(restriction, qn); redefining {
		// src-expredef clause 1.1/1.2: this <restriction> is a redefining
		// <simpleType>'s own, and its base names that type itself, so it resolves to
		// the ORIGINAL — "one component which corresponds to the top-level definition
		// item with the same name in the <redefine>d schema document … except that its
		// {name} is ·absent·". Built here, once, with the zero QName and under the
		// REDEFINED document's producer, so it enters no symbol table, is registered
		// with no builder, and takes its own document's namespace and defaults. That
		// is what keeps the redefinition an ordinary restriction rather than a
		// self-derivation (st-props-correct clause 2) — the name would otherwise
		// resolve back to the redefinition itself, which finalize's
		// checkSimpleBaseAcyclic would then reject.
		//
		// It recurses one level per document across a redefine closure. What bounds
		// it is enterOriginal, not the closure: each hop moves to the REDEFINED
		// document, so the walk is finite only while the <redefine> graph is acyclic
		// over this name, and a cycle over it is the circular {base type definition}
		// chain st-props-correct clause 2 forbids.
		if !p.enterOriginal(src.elem) {
			return nil, xsderr.New(ruleSTPropsCorr, restriction.Loc(),
				"circular simple type definition: %s is redefined around a cycle of <redefine>s, so the original src-expredef clause 1.1 pairs this redefinition with restricts the redefinition again, but st-props-correct clause 2 requires every simple type to reach a primitive datatype or xs:anySimpleType by following {base type definition} zero or more times", qn)
		}
		defer p.leaveOriginal(src.elem)

		orig, err := src.owner.constructSimpleType(xsd.QName{}, src.elem)
		if err != nil {
			return nil, err
		}
		return xsd.OwnedSimpleType{Definition: orig}, nil
	}
	return xsd.SimpleTypeRef{Name: qn}, nil
}

// rejectOutOfModelFacetChildren rejects an XSD-namespace child of a
// <simpleType>'s <restriction> that Datatypes §4.1.2's content model
// (xmlschema11-2.md:2748, the group xs:simpleRestrictionModel at :3929) has no
// position for. Its wildcard position is namespace="##other", which EXCLUDES the
// XSD namespace, so an XSD-namespace name the model does not list is admitted
// nowhere in it and the producer would otherwise map a component out of an
// s4s-invalid document (#972). Children outside the XSD namespace pass here: the
// s4sSimpleTypeRestriction walk behind this check holds them to the wildcard,
// which admits none in no namespace (#1982).
//
// The admitted set is what this producer can MAP: <annotation>, the inline base
// <simpleType>, and the xs:facet substitution group as facetKindOf answers for it
// plus the two names restrictionFacets folds above that lookup, <enumeration> and
// <assertion> (singular — §4.3.13 spells the FACET in the plural and no element
// bears that name). facetKindOf is the predicate rather than s4sFacetElement
// (produce_s4sorder.go), which admits the plural <assertions> because it only
// ORDERS names its models admit somewhere: harmless there, fatal for membership
// here.
//
// It charges §5.1's FIRST bullet (xmlschema11-1.md:4296), which carries NO
// numbered rule ID — the footing checkS4SChildOrder records at length and
// rejectProhibitedAttrs already stands on, charging src-* or cvc-* here being a
// fabricated verdict (STYLE E2). The first offending child in document order is
// the one reported (STYLE D2).
//
// ONLY constructSimpleType calls it, and the seam is the content model rather
// than the mapping: restrictionFacets' other caller, simpleContentSimpleType
// (produce_complex.go), reads an xs:simpleRestrictionType (xmlschema11-1.md:1692)
// whose tail "((attribute | attributeGroup)*, anyAttribute?), assert*" admits
// four names §4.1.2 has no position for, and produceAttributeUses and
// assertionsOf map all four off that same element. One shared answer would
// false-reject every one of them, so the check sits at the caller that knows
// which model it read.
//
// §5.1 binds the schema document "after the conditional-inclusion pre-processing
// described in Conditional inclusion (§4.2.2)", which is what keeps a child
// excluded by vc:minVersion (conditional.go) from ever reaching this walk.
func rejectOutOfModelFacetChildren(restriction *Element) error {
	for _, child := range restriction.Children() {
		el, ok := child.(*Element)
		if !ok || el.Name().Space() != xsd.XMLSchemaNS {
			continue
		}
		local := el.Name().Local()
		if local == "annotation" || local == "simpleType" || mappedFacetElement(local) {
			continue
		}
		return fmt.Errorf("parser: <%s> at %s is a child the schema for schema documents does not admit under the <restriction> at %s: a <simpleType>'s <restriction> extends xs:annotated with the group xs:simpleRestrictionModel (xmlschema11-2.md:3929, §4.1.2), giving the content model (annotation?, (simpleType?, (minExclusive | minInclusive | maxExclusive | maxInclusive | totalDigits | fractionDigits | length | minLength | maxLength | enumeration | whiteSpace | pattern | assertion | explicitTimezone | {any with namespace: ##other})*)), whose wildcard position excludes the XSD namespace",
			local, el.Loc(), restriction.Loc())
	}
	return nil
}

// rejectDuplicateRestrictionChildren charges src-simple-type clause 1 (§3.16.3,
// xmlschema11-1.md:3658-3659) over a <simpleType>'s <restriction>: "No two
// elements among the children of <restriction> have the same expanded name in
// the Schema (xs) namespace, unless that expanded name is one of xs:enumeration,
// xs:pattern, or xs:assertion." Those three are the clause's ONLY exceptions —
// the Note narrowing the common case to facets is informative — so <annotation>
// and the inline base <simpleType> are inside its reach no less than a repeated
// <length>.
//
// This is a NAMED Schema Representation Constraint and so an xsderr.Error, not
// the plain §5.1 grammar fault rejectOutOfModelFacetChildren charges over the
// same children (STYLE E2): the clause's own preamble scopes it "in addition to
// the conditions imposed … by the schema for schema documents". It runs BEHIND
// that grammar check, the run order checkS4SChildOrder's doc fixes for every
// src-* charge (#1246), and AHEAD of the s4sSimpleTypeRestriction walk, one of
// the two exceptions that doc names (#1951).
//
// Two shapes inside clause 1's reach are answered before this charge is, and
// neither is a reason to narrow it. A second <annotation> is answered by
// rejectRepeatedAnnotations, whose prescan walk reaches every element of the
// document rather than only the <restriction>s a producer descends into (#928);
// and two facet children of one kind reach st-props-correct clause 4 at
// construction (xsd.NewSimpleType) for the programmatic path that never sees a
// schema document at all. Whichever fires first, the document is rejected, and
// the clause is implemented here as the spec states it rather than as the
// remainder those two leave.
//
// Children outside the XSD namespace are skipped: the clause is scoped to "the
// same expanded name in the Schema (xs) namespace", and whether
// xs:simpleRestrictionModel's "{any with namespace: ##other}" position admits
// one is the s4sSimpleTypeRestriction walk's to answer (#1982).
// The first duplicate in document order is the one reported (STYLE D2).
func rejectDuplicateRestrictionChildren(restriction *Element) error {
	// A slice, scanned linearly: the names surviving the three exceptions are the
	// facet vocabulary plus <annotation> and <simpleType>, counted in tens (STYLE
	// D3), and s4sNames records the same reuse.
	var seen []string
	for _, child := range restriction.Children() {
		el, ok := child.(*Element)
		if !ok || el.Name().Space() != xsd.XMLSchemaNS {
			continue
		}
		local := el.Name().Local()
		if local == "enumeration" || local == "pattern" || local == "assertion" {
			continue
		}
		if slices.Contains(seen, local) {
			return xsderr.New(ruleSrcSimpleType, el.Loc(),
				"restriction has a second <%s> child, but src-simple-type clause 1 admits no two children with the same expanded name in the xs namespace unless that name is <enumeration>, <pattern> or <assertion>", local)
		}
		seen = append(seen, local)
	}
	return nil
}

// mappedFacetElement reports whether local is the ELEMENT name of a constraining
// facet restrictionFacets maps — facetKindOf's answer plus the two names that
// function folds above that lookup, <enumeration> and <assertion>.
//
// It is the MAPPING membership, not the ordering one: s4sFacetElement
// (produce_s4sorder.go) also admits the plural <assertions>, which names no
// element in the schema for schema documents and which restrictionFacets folds
// into nothing, and the two precisionDecimal scale facets. Harmless where only
// the ORDER of names a model admits somewhere is at stake, fatal for the two
// callers here: rejectOutOfModelFacetChildren would let an unmappable child
// through, and census (census.go) would call a silently dropped child mapped.
func mappedFacetElement(local string) bool {
	if local == "enumeration" || local == "assertion" {
		return true
	}
	_, ok := facetKindOf(local)
	return ok
}

// restrictionFacets maps the constraining-facet children of a <restriction> in
// document order. The plain-lexical facets map one-to-one, with three folding
// exceptions, each landing at the position of its kind's FIRST child element so
// the returned slice stays in document order (STYLE D2):
//
//   - every <assertion> child (Datatypes §4.3.13.2) folds into the SINGLE
//     assertions facet the §4.3.13 {value} rule describes — "a sequence of
//     Assertion components";
//   - every <pattern> child folds into the SINGLE pattern facet xr-pattern
//     (§4.3.4.2) describes, one {value} member per sibling, in document order;
//   - every <enumeration> child folds into the SINGLE enumeration facet
//     xr-enumeration (§4.3.5.2) describes — "a set of the actual values of all
//     the <enumeration> [children]'s value [attributes]" — one member per
//     sibling, in document order.
//
// Each facet's {value} is "the actual value of the value [attribute]"
// (f-l-value, f-mil-value, f-mal-value, f-w-value, f-td-value, f-fd-value,
// f-tz-value; f-ms-value and f-mns-value in xsd-precisionDecimal.md), read
// after the attribute type's whiteSpace normalization (Structures key-vv,
// key-nv): facetValueLexical collapses the kinds whose value is typed with
// whiteSpace = collapse and leaves <pattern> and the four bounds raw, and
// <enumeration> members keep their literal too.
//
// The non-facet children <annotation> and the inline base <simpleType> are
// skipped. src-simple-type clause 1 excepts xs:enumeration, xs:pattern and
// xs:assertion from its no-two-children-with-one-name rule, which is what makes
// all three folds reachable on a legal schema.
//
// A child whose name maps to no facet kind is skipped rather than rejected,
// because the two callers read DIFFERENT content models and only one of them can
// answer: rejectOutOfModelFacetChildren charges §4.1.2's, at the
// constructSimpleType site alone.
func (p *producer) restrictionFacets(restriction *Element) ([]xsd.Facet, error) {
	var facets []xsd.Facet
	var assertions []xsd.Assertion
	assertionsAt := 0
	var patterns []string
	patternAt := 0
	var members []xsd.EnumerationMember
	enumerationAt := 0
	for _, child := range restriction.Children() {
		el, ok := child.(*Element)
		if !ok {
			continue
		}
		if el.Name().Space() != xsd.XMLSchemaNS {
			continue
		}
		local := el.Name().Local()
		if local == "annotation" || local == "simpleType" {
			continue
		}
		if local == "enumeration" {
			// xr-enumeration (§4.3.5.2): the <enumeration> children of ONE
			// <restriction> are members of a single facet's {value} SET, not a facet
			// each — two same-kind ownFacets is what st-props-correct clause 4
			// rejects. Unlike xr-pattern this fold takes no union with the base
			// type's own enumeration facet: §4.3.5.2 has no such clause, so the
			// generic same-kind overlay (st-restrict-facets §3.16.6.4,
			// key-facets-overlay) governs and a restriction's enumeration REPLACES
			// the base's outright — which is what xsd's EffectiveFacets does with
			// every kind but pattern and assertions.
			//
			// The context is captured per <enumeration> element, not once per
			// <restriction>: §3.3.18 fixes a QName/NOTATION member's prefix scope to
			// the element the literal was written on, and siblings can carry
			// different bindings.
			val, _ := el.Attr("value")
			bindings, defaultNS := namespaceContextOf(el)
			members = append(members, xsd.NewEnumerationMember(val, bindings, defaultNS))
			folded := xsd.NewEnumerationFacet(members)
			if len(members) == 1 {
				enumerationAt = len(facets)
				facets = append(facets, folded)
				continue
			}
			facets[enumerationAt] = folded
			continue
		}
		if local == "assertion" {
			if len(assertions) == 0 {
				assertionsAt = len(facets)
			}
			assertions = append(assertions, xsd.NewAssertion(p.buildXPathExpression(el, "test")))
			continue
		}
		kind, ok := facetKindOf(local)
		if !ok {
			continue
		}
		val, _ := el.Attr("value")
		if kind == xsd.FacetPattern {
			// xr-pattern (§4.3.4.2): all the <pattern> children of ONE <restriction>
			// contribute BRANCHES of a single regular expression — patterns on the
			// same derivation step are ORed, only patterns on DIFFERENT steps are
			// ANDed (cvc-pattern-valid §4.3.4.4, one check per surviving facet). One
			// facet per sibling would carry cross-step meaning: two same-kind
			// ownFacets, which st-props-correct clause 4 rejects outright, so a
			// two-<pattern> restriction was unconstructible. The pattern facet has no
			// {fixed} property, and <pattern> has no fixed attribute (dc-pattern
			// §4.3.4.1, element-pattern §4.3.4.2).
			patterns = append(patterns, val)
			folded := xsd.NewFacet(kind, patterns, false)
			if len(patterns) == 1 {
				patternAt = len(facets)
				facets = append(facets, folded)
				continue
			}
			facets[patternAt] = folded
			continue
		}
		if err := facetCountValue(el, kind); err != nil {
			return nil, err
		}
		fixed, err := facetFixed(el)
		if err != nil {
			return nil, err
		}
		facets = append(facets, xsd.NewFacet(kind, []string{facetValueLexical(kind, val)}, fixed))
	}
	if len(assertions) == 0 {
		return facets, nil
	}
	return slices.Insert(facets, assertionsAt, xsd.NewAssertionsFacet(assertions)), nil
}

// produceElement maps a top-level <element> into a global Element Declaration
// (§3.3.2.2 dcl.elt.global), including its {identity-constraint definitions}
// (§3.3.2.1). Registering the produced identity constraints with the schema
// builder is the caller's job (run), keeping this one focused on building the
// declaration.
//
// qname is the expanded {name}/{target namespace}, taken from topLevelName by
// the single caller (run) rather than read from the element here, so an
// unusable name is a grammar fault charged before any of this mapping runs. The
// same caller rejects the attributes xs:topLevelElement prohibits — ref, form,
// targetNamespace, minOccurs, maxOccurs — one step earlier still
// (rejectProhibitedAttrs), so none of them reaches this mapping.
//
// The element's OWN children are ordered against s4sElement here
// (checkS4SChildOrder, #1076), the one content model Appendix A gives every form
// an <element> takes. src-element clause 3 is charged BEHIND that walk, the run
// order checkS4SChildOrder's doc records for every src-* charge (#1246).
// rejectBothInlineTypes runs AHEAD of it and is no exception to that order: the
// guard charges no rule, and it names BOTH children of a shape the walk reports
// as a repeat of the model's single type position (#444 owns the pairing).
//
// Its {type definition} is §3.3.2.1 dcl.elt.common's tier chain, which is a
// COMMON mapping rule — §3.3.2.2 supplements only {scope} and {target
// namespace}, never {type definition} — so no tier of it is mapped differently
// here than on the local path. Tier 1's inline <complexType> child is mapped in
// this function, because that arm needs an identity minted before either
// component exists: one xsd.ComponentID serves as both the anonymous type's
// {context} (§3.4.2.1 dcl.ctd.common) and the {scope}.{parent} of its own nested
// local elements (#340). That identity is minted for EVERY declaration this
// function builds, whichever tier wins, because an <alternative> child owns
// anonymous types under the same {context} (#851) — see typeTableOf. Tier 1's
// inline <simpleType> child and tier 2's type= go through declaredType, the one
// implementation both element forms and the local attribute form share (STYLE
// T4, #442). The anonymous simple type it builds carries no {context} of its
// own: §3.16.1 std-context is a separate property from {type definition} and is
// unmodeled here exactly as on the local path. The two element paths differ only
// in the {scope} the declaration gets.
//
// This is the ONLY path that can reach the chain's clause 3, the head's declared
// type: substitutionGroup= is legal on a top-level <element> alone (§3.3.2), so
// produceLocalElement rejects it outright and declaredType has no tier for it.
// Clause 3 is decided by substitutionGroupHeadType, from the FIRST resolved
// affiliation, which is why the affiliations are mapped before the type here,
// and it sits BETWEEN declaredType's tier 2 and its dflt — so the tiers stay
// exclusive and in §3.3.2.1's order.
func (p *producer) produceElement(qname xsd.QName, elem *Element) (xsd.ElementDeclaration, error) {
	_, hasType := elem.Attr("type")
	inlineSimple := childElement(elem, xsd.XMLSchemaNS, "simpleType")
	inlineComplex := childElement(elem, xsd.XMLSchemaNS, "complexType")

	if err := rejectBothInlineTypes(elem, inlineSimple, inlineComplex); err != nil {
		return xsd.ElementDeclaration{}, err
	}
	if err := checkS4SChildOrder(elem, s4sElement); err != nil {
		return xsd.ElementDeclaration{}, err
	}
	if hasType && (inlineSimple != nil || inlineComplex != nil) {
		return xsd.ElementDeclaration{}, xsderr.New(ruleSrcElement, elem.Loc(),
			"element has both a type attribute and an inline <simpleType>/<complexType> child, but src-element clause 3 forbids both")
	}

	vc, err := valueConstraintOf(elem, ruleSrcElement)
	if err != nil {
		return xsd.ElementDeclaration{}, err
	}

	// §3.3.2.1 dcl.elt.common: {nillable} and {abstract} are the ·actual values· of
	// the nillable and abstract attributes, otherwise false — boolAttr's absent
	// case. abstract is read on this path ALONE: the schema for schema documents
	// declares it use="prohibited" on xs:localElement (§A), so produceLocalElement
	// has no attribute to read.
	nillable, _, err := boolAttr(elem, "nillable")
	if err != nil {
		return xsd.ElementDeclaration{}, err
	}
	abstract, _, err := boolAttr(elem, "abstract")
	if err != nil {
		return xsd.ElementDeclaration{}, err
	}

	constraints, err := p.identityConstraintsOf(elem)
	if err != nil {
		return xsd.ElementDeclaration{}, err
	}
	affiliations, err := p.substitutionGroupAffiliations(elem)
	if err != nil {
		return xsd.ElementDeclaration{}, err
	}
	// The identity is minted UNCONDITIONALLY, before any clause is chosen: since
	// #851 an <alternative> child may own an anonymous complex type whose
	// {context} is this declaration (§3.4.2.1 dcl.ctd.common), so an <element>
	// with a plain type= owns types too and no path is ownership-free.
	edID := xsd.NewComponentID()
	var typeDef xsd.TypeDefinitionOrRef = xsd.TypeDefinitionRef{Name: anyTypeName} // §3.3.2.1 case 4
	switch {
	case inlineComplex != nil:
		ct, cerr := p.produceComplexType(elementOwnedComplexType{owner: edID}, inlineComplex) // case 1
		if cerr != nil {
			return xsd.ElementDeclaration{}, cerr
		}
		typeDef = xsd.InlineTypeDefinition{Definition: ct}
	case inlineSimple != nil, hasType:
		declared, derr := p.declaredType(elem, anyTypeName) // cases 1 and 2
		if derr != nil {
			return xsd.ElementDeclaration{}, derr
		}
		typeDef = declared
	case len(affiliations) > 0:
		inherited, herr := p.substitutionGroupHeadType(affiliations[0]) // case 3
		if herr != nil {
			return xsd.ElementDeclaration{}, herr
		}
		typeDef = inherited
	}
	typeTable, err := p.typeTableOf(elem, edID, typeDef)
	if err != nil {
		return xsd.ElementDeclaration{}, err
	}
	// §3.3.2.2 dcl.elt.global: {scope} is {variety} global, {parent} ·absent·.
	return xsd.NewElementDeclarationOwningTypes(elem.Loc(), edID, qname, typeDef, typeTable, xsd.NewGlobalScope(), vc,
		nillable, constraints, affiliations, p.substitutionGroupExclusions(elem), abstract, p.disallowedSubstitutions(elem))
}

// substitutionGroupAffiliations maps the substitutionGroup attribute of a
// top-level <element> into {substitution group affiliations} (§3.3.2.1
// dcl.elt.common: "A set of the element declarations ·resolved· to by the items
// in the ·actual value· of the substitutionGroup attribute, if present, otherwise
// the empty set").
//
// The attribute is typed `List of QName` (§3.3.2), so XSD 1.1 permits SEVERAL
// heads, and EVERY item contributes — unlike {type definition} clause 3, which
// reads the first item alone (substitutionGroupHeadType). Items are
// resolved and returned in LEXICAL order and the property is carried as a slice,
// not a set (STYLE D2): it reaches users through the cos-nonambig and
// cos-element-consistent diagnostics, and no map is ranged anywhere on the path.
//
// Only the prefix→namespace-name half of ·resolved· happens here, through the one
// in-scope-bindings resolver (STYLE T4), which also applies §F.1 task (b)'s
// chameleon coercion to an unqualified item. Existence is not checked here at
// all, and — unlike every other by-name reference this producer emits — it is not
// checked at finalize either: a head naming no declaration is an ·absent· member
// under §5.3 (Missing Sub-components), which makes the schema valid and only its
// USE invalid. See xsd/resolve.go's resolveElementDecl for that decision; the
// consequence here is that this function never fails on an unknown head, only on
// a lexically unresolvable prefix.
//
// It is called only from produceElement. The local path never builds
// affiliations: the INLINE local form is rejected for carrying the attribute at
// all (e-props-correct clause 3, produceLocalElement), and the <element ref>
// form returns from produceElementParticle before that check, ignoring the
// attribute instead of rejecting it (see the GAP marker on that branch).
//
// {substitution group exclusions} — the final=/finalDefault twin — is mapped
// alongside it by substitutionGroupExclusions, for the same operational reason:
// e-props-correct clause 4 (c-vs-sg) reads it on the HEAD of an affiliation
// edge, and xsd enforces that clause (xsd/substitutiongrouptypes.go, #395), so
// leaving it universally empty would silently un-block every head that spells
// final= on itself.
func (p *producer) substitutionGroupAffiliations(elem *Element) ([]xsd.QName, error) {
	lexical, ok := elem.Attr("substitutionGroup")
	if !ok {
		return nil, nil
	}
	var heads []xsd.QName
	for _, item := range xmlSpaceFields(lexical) {
		head, err := p.resolveQName(elem, item, "substitutionGroup")
		if err != nil {
			return nil, err
		}
		heads = append(heads, head)
	}
	return heads, nil
}

// substitutionGroupHeadType is §3.3.2.1 dcl.elt.common's {type definition} case
// 3: "The declared {type definition} of the Element Declaration ·resolved· to by
// the first QName in the ·actual value· of the substitutionGroup attribute, if
// present." head is that first QName — the caller passes affiliations[0], so the
// "first item alone" reading is applied to the RESOLVED list rather than
// re-derived from the lexical form (STYLE T4); every item feeds {substitution
// group affiliations}, but only the first feeds this property.
//
// It reads the head's SOURCE rather than its built component (symbols.elements),
// which is what makes a forward or cross-document head resolve: the head may be
// declared after this member or in another document, and every document's
// pre-scan runs before any document's run. Building the head declaration on
// demand is not an option — element declarations carry no build memo, so a
// second construction would register the head's identity constraints a second
// time and fabricate a sch-props-correct clause 2 collision.
//
// The result is a SLOT (xsd.TypeDefinitionOrRef), not a component, because that
// is what a produced declaration's {type definition} holds; finalize resolves it.
// Which arm depends on how the terminal head spells its own type:
//
//   - a NAMED type — the head's type= — is an xsd.TypeDefinitionRef, resolved at
//     finalize exactly as this element's own type= would be;
//   - an inline anonymous type — <complexType> or <simpleType> alike — is an
//     xsd.SubstitutionGroupHeadTypeRef naming that head. Case 3 makes the
//     member's {type definition} the head's own component, which no by-name
//     reference can reach and no second declaration can OWN (§3.4.2.1
//     dcl.ctd.common ties an anonymous complex type's {context} to one
//     declaration, §3.16.1 std-context an anonymous simple type's), so the slot
//     references the OWNER instead — see that type, and §3.4.6.5's no-identity
//     Note for why identity rather than a copy is what the spec asks for. The
//     arm is INDIFFERENT to which of the two the head spells, and every reader
//     of it is too: xsd's ResolvedType takes one hop to the head's own slot and reads
//     whatever component sits there.
//
// Three cases yield case 4's xs:anyType instead, each of them the honest answer
// rather than a fallback:
//
//   - the head names no top-level <element> in the assembly. The affiliation is
//     an ·absent· member under §5.3 (Missing Sub-components) — a valid schema,
//     W3C saxonData/Missing missing002 — so there is no declaration to take a
//     declared type from, and e-props-correct clause 4 skips the same edge
//     (xsd/substitutiongrouptypes.go);
//   - the head carries neither a type child, type=, nor substitutionGroup=, so
//     ITS own {type definition} is case 4's xs:anyType and the member inherits
//     exactly that;
//   - the walk returns to a head it has already visited. That is a circular
//     substitution group, which e-props-correct clause 5 rejects at finalize
//     (xsd/resolve.go's checkSubstitutionGroupsAcyclic) over the whole graph;
//     seen is a walk-scoped TERMINATION guard and never a verdict, the same split
//     symbols.builtGroups' on-stack half records. It is read only by key, so no
//     map iteration reaches any output (STYLE D2).
//
// CLAUSE ORDER. The walk tests the inline type CHILD before type=, matching the
// order §3.3.2.1's own table states ("the first of the following that applies",
// clause 1 being the child). It used to test type= first, which was unobservable
// — produceElement charges src-element clause 3 when a head carries both, so no
// head that reaches this walk with both survives its own production — but with
// clause 1 now yielding a real answer rather than a decline, the inverted order
// would be a genuine mis-mapping rather than a harmless one (#342, the #395
// post-land addendum). The both-present head is still rejected, by src-element
// clause 3 at the head's own turn; this walk reads the head's SOURCE out of the
// pre-scan index and never runs produceElement on it, so it deliberately charges
// no verdict of its own here.
func (p *producer) substitutionGroupHeadType(head xsd.QName) (xsd.TypeDefinitionOrRef, error) {
	seen := map[xsd.QName]bool{}
	for !seen[head] {
		seen[head] = true
		src, ok := p.symbols.elements[head]
		if !ok {
			return xsd.TypeDefinitionRef{Name: anyTypeName}, nil // an ·absent· head (§5.3)
		}
		if childElement(src.elem, xsd.XMLSchemaNS, "complexType") != nil ||
			childElement(src.elem, xsd.XMLSchemaNS, "simpleType") != nil { // clause 1
			return xsd.SubstitutionGroupHeadTypeRef{Head: head}, nil
		}
		if lex, has := src.elem.Attr("type"); has { // clause 2
			name, err := src.owner.resolveQName(src.elem, lex, "type")
			if err != nil {
				return nil, err
			}
			return xsd.TypeDefinitionRef{Name: name}, nil
		}
		lexHeads, has := src.elem.Attr("substitutionGroup")
		if !has {
			return xsd.TypeDefinitionRef{Name: anyTypeName}, nil // the head's own {type definition} is case 4
		}
		items := xmlSpaceFields(lexHeads)
		if len(items) == 0 {
			return xsd.TypeDefinitionRef{Name: anyTypeName}, nil
		}
		next, err := src.owner.resolveQName(src.elem, items[0], "substitutionGroup")
		if err != nil {
			return nil, err
		}
		head = next
	}
	// A circular affiliation chain; clause 5 charges it at finalize.
	return xsd.TypeDefinitionRef{Name: anyTypeName}, nil
}

// elementBlockKeywords is the ·relevant set· the {disallowed substitutions} row
// draws from — the §3.3.1 subset of {substitution, extension, restriction} — in
// the canonical order the row's own case 2 writes it in. It is the only one of
// the four sets here that contains substitution.
var elementBlockKeywords = []xsd.DerivationMethod{xsd.DerivationExtension, xsd.DerivationRestriction, xsd.DerivationSubstitution}

// elementFinalKeywords is the ·relevant set· the {substitution group exclusions}
// row draws from: {extension, restriction} (§3.3.1, §3.3.2.1), TWO keywords and
// not the block row's three. substitution is not one of them, and
// xsd.NewElementDeclaration rejects a set containing it as an e-props-correct
// clause 1 tableau violation.
//
// It has the same MEMBERS as complexTypeKeywords below and is deliberately a
// separate var, not an alias: §3.3.2.1's row states this relevant set in its own
// right ("with the relevant set being {extension, restriction}") while §3.4.2.1
// states the complex-type one, so these are two spec facts that happen to
// coincide, not one fact written twice — STYLE D3 forbids the latter and says
// nothing about the former. Sharing one var would silently couple an element
// property to a complex-type property.
var elementFinalKeywords = []xsd.DerivationMethod{xsd.DerivationExtension, xsd.DerivationRestriction}

// complexTypeKeywords is the ·relevant set· of BOTH complex-type properties in
// this family — {prohibited substitutions} and {final} (§3.4.2.1
// dcl.ctd.common). It is one set for the two because the table defines the
// second entirely by reference to the first: "[a]s for {prohibited
// substitutions} above, but using the final and finalDefault attributes in place
// of the block and blockDefault attributes". TWO members: substitution is not
// among them though blockDefault's grammar admits it, and neither list nor union
// is though finalDefault's does. Both §3.4.1 property definitions say "a subset
// of {extension, restriction}".
var complexTypeKeywords = []xsd.DerivationMethod{xsd.DerivationExtension, xsd.DerivationRestriction}

// simpleTypeFinalKeywords is the ·relevant set· of a simple type's {final}
// (§3.16.2.1 map.std.common): FOUR members, in the order that rule's case 2
// writes them. It is the widest of the four sets here and is exactly
// finalDefault's own vocabulary, so nothing is ever dropped on this path.
var simpleTypeFinalKeywords = []xsd.DerivationMethod{xsd.DerivationRestriction, xsd.DerivationExtension, xsd.DerivationList, xsd.DerivationUnion}

// effectiveDerivationSet is the three-case ·effective block value· analysis that
// every member of the block=/final= mapping family runs: {disallowed
// substitutions} and {substitution group exclusions} on an element (§3.3.2.1),
// {prohibited substitutions} and {final} on a complex type (§3.4.2.1), and
// {final} on a simple type (§3.16.2.1). The five mappings differ in exactly two
// things — the attribute-name pair and the ·relevant set· — so they are ONE
// function taking both rather than five copies of the case analysis (STYLE T4).
//
// The EBV is local on elem if PRESENT, otherwise fallback on the ancestor
// <schema> if present, otherwise the empty string. "Present" is a test on the
// attribute, not on its value: a local block=""/final="" is present, so it wins
// over the schema-level default and takes case 1 below — the distinction a "" ==
// absent reading would lose. Then the empty string maps to the empty set (case
// 1), "#all" to the whole relevant set (case 2), and anything else to the
// relevant-set members its whitespace-separated list names (case 3).
//
// The EBV's vocabulary is already checked when this runs: rejectInvalidDerivationSet
// charges cvc-datatype-valid, in rejectS4SFaults' walk ahead of every producer,
// on a block=/final=/blockDefault=/finalDefault= value outside the s4s type its
// production declares it with, and a form that prohibits the attribute rejects it
// before its producer maps it. Each local attribute's type has exactly its
// ·relevant set· as members, so a local EBV names only members of relevant. A
// Default EBV may name more, and those items are IGNORED here, as each table
// says of its own Default attribute — blockDefault "may include values other
// than restriction or extension", and "those values are ignored in the
// determination of {prohibited substitutions} for complex type definitions (they
// are used elsewhere)". That is what makes a shared finalDefault="list"
// contribute nothing to a complex type's {final} and blockDefault="substitution"
// nothing to its {prohibited substitutions}.
//
// Case 2 compares the ·actual value·: the s4s types' "#all" arm is an xs:token,
// whose whiteSpace = collapse makes block=" #all " the value "#all", and a valid
// EBV holding "#all" holds no other token, so collapseTrim is that collapse.
// Case 3's strings.Fields splits a valid EBV exactly as XML whitespace would:
// every token of one is an ASCII keyword.
//
// The result is in relevant's own fixed order, not the attribute's lexical
// order. These properties ARE sets drawn from a fixed set, so one set must have
// one encoding (STYLE D2): final="restriction extension" and final="extension
// restriction" must build the identical component. Each token is matched against
// xsd.DerivationMethod's own String(), which returns the verbatim spec token, so
// no relevant set writes the token spellings down a second time beside the
// methods that already carry them (STYLE D3). Case 2 returns a fresh slice,
// never relevant itself, so no caller can reach a package-level set.
func (p *producer) effectiveDerivationSet(elem *Element, local, fallback string, relevant []xsd.DerivationMethod) []xsd.DerivationMethod {
	ebv, ok := elem.Attr(local)
	if !ok {
		ebv, _ = p.schemaElem.Attr(fallback)
	}
	if ebv == "" {
		return nil // case 1
	}
	if collapseTrim(ebv) == "#all" {
		return slices.Clone(relevant) // case 2
	}
	items := strings.Fields(ebv)
	var set []xsd.DerivationMethod // case 3
	for _, m := range relevant {
		if slices.Contains(items, m.String()) {
			set = append(set, m)
		}
	}
	return set
}

// disallowedSubstitutions maps an <element>'s block attribute into {disallowed
// substitutions} (§3.3.2.1 dcl.elt.common), over the three-keyword element set.
//
// It is mapped in THIS slice, beside {substitution group affiliations}, because
// the two are one fact operationally: {disallowed substitutions} is read by
// cos-equiv-derived-ok-rec clause 2.1 (§3.3.6.3), the clause that decides whether
// a head admits substitution at all, so populating the affiliation edges while
// leaving the property universally empty would publish an OVER-BROAD
// ·substitution group· — every declared head would admit every member, which
// false-rejects valid schemas through cos-nonambig's ·overlap· relation. W3C
// MS-Element elemZ028a is exactly that shape: an affiliation chain a→b→c→d in
// which c and d each carry block="substitution", with b, c and d then named side
// by side in one <xs:all>. Half the pair is not a smaller change, it is a wrong
// one.
//
// The result is in the spec's own canonical order — extension, restriction,
// substitution, the order the table's case 2 writes the set in — rather than in
// the attribute's lexical order; effectiveDerivationSet's doc carries the STYLE
// D2 reasoning, which is the same for every member of the family.
//
// The ·relevant set· it draws from is elementBlockKeywords; the case analysis
// itself lives in effectiveDerivationSet, which the other four members of the
// family reuse unchanged.
func (p *producer) disallowedSubstitutions(elem *Element) []xsd.DerivationMethod {
	return p.effectiveDerivationSet(elem, "block", "blockDefault", elementBlockKeywords)
}

// substitutionGroupExclusions maps an <element>'s final attribute into
// {substitution group exclusions} (§3.3.2.1 dcl.elt.common, whose row for the
// property reads in full: "As for {disallowed substitutions} above, but using
// the final and finalDefault [attributes] in place of the block and blockDefault
// [attributes] and with the relevant set being {extension, restriction}"). So it
// is disallowedSubstitutions' case analysis over a different attribute pair and
// a different ·relevant set·, which is exactly what effectiveDerivationSet takes
// as parameters (STYLE T4) — including "#all", whose expansion here is the TWO
// keywords of elementFinalKeywords and not the three of the block row.
//
// It is mapped for the TOP-LEVEL form only. A local <element> carries no final
// attribute at all (§3.3.2 gives it to the top-level form alone), so its EBV
// could come only from the ancestor <schema>'s finalDefault — and the property
// would be unreadable there whatever it held: e-props-correct clause 4 reads
// {substitution group exclusions} on the HEAD of an affiliation edge, an
// affiliation ·resolves· by expanded name and so names a top-level declaration,
// and clause 3 keeps a local declaration out of the property in the first place.
// Mapping it on the local path would add a fact with no reader.
func (p *producer) substitutionGroupExclusions(elem *Element) []xsd.DerivationMethod {
	return p.effectiveDerivationSet(elem, "final", "finalDefault", elementFinalKeywords)
}

// complexTypeProhibitedSubstitutions maps a <complexType>'s block attribute into
// {prohibited substitutions} (§3.4.2.1 dcl.ctd.common), over the two-keyword
// complex-type set — NOT the three-keyword element set disallowedSubstitutions
// uses, since substitution is not a member of this property.
//
// It is read by cos-equiv-derived-ok-rec clause 2.2/2.3's blocking union
// (§3.3.6.3, substitutiongroup.go) and by ·validly substitutable· (§3.4.6.4,
// complexderivation.go), both of which saw a universally empty set until this
// mapping existed.
func (p *producer) complexTypeProhibitedSubstitutions(ctElem *Element) []xsd.DerivationMethod {
	return p.effectiveDerivationSet(ctElem, "block", "blockDefault", complexTypeKeywords)
}

// complexTypeFinal maps a <complexType>'s final attribute into {final} (§3.4.2.1
// dcl.ctd.common, "[a]s for {prohibited substitutions} above, but using the
// final and finalDefault attributes"), over the same two-keyword set — NOT the
// four-keyword simple-type set, so a list or union token inherited from a shared
// finalDefault is dropped here.
//
// It is read by cos-ct-extends clause 1.1 (§3.4.6.2) and derivation-ok-
// restriction clause 1 (§3.4.6.3).
func (p *producer) complexTypeFinal(ctElem *Element) []xsd.DerivationMethod {
	return p.effectiveDerivationSet(ctElem, "final", "finalDefault", complexTypeKeywords)
}

// simpleTypeFinal maps a <simpleType>'s final attribute into {final} (§3.16.2.1
// map.std.common's ·FS·), over the four-keyword simple-type set.
//
// It applies to an ANONYMOUS inline <simpleType> as well as a named one: the
// rule is stated for every Simple Type Definition, and although the schema for
// schema documents prohibits final= on the local form, an absent local attribute
// still falls through to the ancestor <schema>'s finalDefault.
//
// It is read by st-props-correct clause 3 (§3.16.6.1) and cos-st-restricts
// clauses 2.2.1.1 and 3.2.1.1 (§3.16.6.2).
func (p *producer) simpleTypeFinal(stElem *Element) []xsd.DerivationMethod {
	return p.effectiveDerivationSet(stElem, "final", "finalDefault", simpleTypeFinalKeywords)
}

// produceNotation maps a top-level <notation> into a Notation Declaration
// (§3.14.2): {name} bundled with the schema's target namespace, {system
// identifier} from system= and {public identifier} from public=, each absent
// when its attribute is. Both absent is rejected inside [xsd.NewNotation]
// (n-props-correct, §3.14.6) — §3.14.3 defines no Schema Representation
// Constraint of its own. A name that is not an xs:NCName is rejected by
// declarationName first, before anything is built, and content the element's s4s
// type does not admit by rejectNotationContent next. <notation> occurs as a
// <schema> or an <override> child, the two content models referencing
// xs:schemaTop (xmlschema11-1.md:4462, :4562, :5577), and rejectMisplacedNotation
// rejects it anywhere else — so there is no nested form to map, and an
// <override>'s <notation> is mapped here too, by the OVERRIDDEN document's
// producer (§F.2 clause 1).
func (p *producer) produceNotation(elem *Element) (xsd.Notation, error) {
	qname, err := declarationName(elem, p.target)
	if err != nil {
		return xsd.Notation{}, err
	}
	if err := rejectNotationContent(elem); err != nil {
		return xsd.Notation{}, err
	}
	var systemID, publicID *string
	if v, ok := elem.Attr("system"); ok {
		systemID = &v
	}
	if v, ok := elem.Attr("public"); ok {
		publicID = &v
	}
	return xsd.NewNotation(elem.Loc(), qname, systemID, publicID)
}

// rejectNotationContent rejects content under a <notation> that its schema for
// schema documents type does not admit. xs:notation extends xs:annotated and
// adds only name, public and system (xmlschema11-1.md:5701-:5709), so its whole
// content model is xs:annotated's own <xs:element ref="xs:annotation"
// minOccurs="0"/> inside a <xs:sequence> (:4426-:4438) — "(annotation?)", the
// prose XML Representation Summary's wording at :3376.
//
// That content is element-only, so character data other than whitespace is
// outside the model as well: xs:annotated descends from xs:openAttrs
// (:4412-:4422), which restricts xs:anyType with an <xs:anyAttribute> and so
// opens ATTRIBUTES alone, as the type's own documentation says at :4415. The
// whitespace is #x9/#xA/#xD/#x20 exactly, read through collapseTrim for the
// reason its doc gives.
//
// A SECOND <annotation> is not this function's fault to raise:
// rejectS4SFaults' walk already reaches every element of the document with
// rejectRepeatedAnnotations, <notation> among every other xs:annotated-derived
// element, and one s4s fault earns one diagnostic.
//
// The fault carries NO numbered rule ID: §3.14.3 and §3.14.4 both answer "None
// as such." (:3409, :3413), and n-props-correct (§3.14.6, :3429) is a tableau
// over {name}, {system identifier} and {public identifier} rather than a content
// model. It stands on §5.1's first bullet (:4296) directly, exactly as
// rejectRepeatedAnnotations does, so charging src-notation — an anchor xsderr's
// generated catalog extracted from a section whose body is "None as such.", not
// a constraint — would be a fabricated rule ID (STYLE E2).
func rejectNotationContent(elem *Element) error {
	for _, child := range elem.Children() {
		switch n := child.(type) {
		case *Element:
			if isXSD(n, "annotation") {
				continue
			}
			return fmt.Errorf("parser: <%s> at %s is not admitted inside the <notation> at %s: xs:notation extends xs:annotated, whose content model is (annotation?), so <annotation> is the only child element the schema for schema documents allows there", n.Name().Local(), n.Loc(), elem.Loc())
		case *Text:
			if collapseTrim(n.Data()) == "" {
				continue
			}
			return fmt.Errorf("parser: character data at %s is not admitted inside the <notation> at %s: xs:notation extends xs:annotated, whose content model is (annotation?) and holds elements only, so nothing but whitespace may appear between its tags", n.Loc(), elem.Loc())
		}
	}
	return nil
}

// produceAttribute maps a top-level <attribute> into a global Attribute
// Declaration (§3.2.2.1 dcl.att.global). Its {type definition} is mapped by
// declaredType over §3.2.2.1's three tiers — the inline <simpleType> child
// (#733), the type= reference, or xs:anySimpleType — which is the same chain
// §3.2.2.2 dcl.att.local states word for word and produceLocalAttribute maps
// through the same function. qname reaches it from topLevelName through run, for
// the reason produceElement's doc gives.
//
// The declaration's OWN children are ordered against s4sAttribute first
// (checkS4SChildOrder, #1076), the one content model Appendix A gives the
// top-level and the local form alike; produceAttributeUse orders every local
// <attribute> against it. Clause 4 below is charged BEHIND that walk, the run
// order checkS4SChildOrder's doc records for every src-* charge — the same one
// produceElement takes for src-element clause 3, the both-present fault of the
// element side (#1246).
//
// no-xmlns (§3.2.6.3) and no-xsi (§3.2.6.4) are charged behind that walk too and
// ahead of everything else, by rejectXmlnsName and rejectXSITargetNamespace:
// qname already carries both the {name} and the {target namespace} §3.2.2.1
// gives this form — the ancestor <schema>'s targetNamespace — so nothing else
// need be mapped to decide either (#1446, #1465).
//
// It charges the two src-attribute clauses (§3.2.3) this form can reach: 4
// (type= and an inline <simpleType> mutually exclusive) and 1 (default and fixed
// mutually exclusive, via valueConstraintOf). Clause 3 is guarded by "if the
// item's parent is not <schema>" and so does not reach this form; the ref it
// governs is prohibited outright on a top-level <attribute>, as are form, use
// and targetNamespace, all rejected in run by rejectProhibitedAttrs
// before this runs.
//
// Clauses 2 and 5 are the default/fixed × use= corner, and NEITHER can fire
// here: each needs a use= present (an absent one reads as the declared default
// "optional", which satisfies both), and a top-level use= is the prohibited
// attribute a step earlier — §3.2.2's Note says so, and the schema for schema
// documents is where that prohibition lives (#652). The clauses keep their
// single encoding in useValueConstraintOK, which produceAttributeUse charges
// over the local and ref= forms; this function no longer calls it.
func (p *producer) produceAttribute(qname xsd.QName, elem *Element) (xsd.AttributeDeclaration, error) {
	if err := checkS4SChildOrder(elem, s4sAttribute); err != nil {
		return xsd.AttributeDeclaration{}, err
	}
	if err := rejectXmlnsName(qname.Local, elem); err != nil {
		return xsd.AttributeDeclaration{}, err
	}
	if err := rejectXSITargetNamespace(qname, elem); err != nil {
		return xsd.AttributeDeclaration{}, err
	}
	_, hasType := elem.Attr("type")
	if hasType && childElement(elem, xsd.XMLSchemaNS, "simpleType") != nil {
		return xsd.AttributeDeclaration{}, xsderr.New(ruleSrcAttribute, elem.Loc(),
			"attribute has both a type attribute and an inline <simpleType> child, but src-attribute clause 4 forbids both")
	}

	vc, err := valueConstraintOf(elem, ruleSrcAttribute)
	if err != nil {
		return xsd.AttributeDeclaration{}, err
	}
	typeDef, err := p.declaredType(elem, anySimpleTypeName)
	if err != nil {
		return xsd.AttributeDeclaration{}, err
	}
	// §3.2.2.1 dcl.att.global: {inheritable} is the ·actual value· of
	// inheritable, if present, otherwise false — the zero value, so presence is
	// not a rule input.
	inheritable, _, err := boolAttr(elem, "inheritable")
	if err != nil {
		return xsd.AttributeDeclaration{}, err
	}
	return xsd.NewAttributeDeclaration(elem.Loc(), qname, typeDef, xsd.NewAttributeGlobalScope(), vc, inheritable)
}

// valueConstraintOf maps the default/fixed attributes of an <element>/<attribute>
// to a *ValueConstraint, rejecting the both-present case (src-element clause 1 /
// src-attribute clause 1). rule selects which of the two constraints is charged.
// It serves both a declaration's own {value constraint} (vc_e, vc_a) and an
// Attribute Use's (vc_au, §3.5.1) — the mapping from the two XML attributes to
// the {variety}/{lexical form} record is identical; which component the result is
// attached to is the caller's decision (§3.2.2.2 / §3.2.2.3).
//
// Every value constraint this producer builds is captured together with elem's
// namespace context, which is what a QName- or NOTATION-governed {lexical form}
// needs to denote a {value} at all (§3.3.18, adopted by §3.3.19): §3.2.6.2
// cos-valid-simple-default clause 2 maps the lexical in the scope where it was
// written, so the bindings must be taken here and travel with the record.
// Because all four producer call sites funnel through this one function, an
// element declaration's, an attribute declaration's and an attribute use's
// constraints are captured alike — a-props-correct (§3.2.6.1) clause 2 and
// au-props-correct (§3.5.6) clause 2 both.
func valueConstraintOf(elem *Element, rule xsderr.Rule) (*xsd.ValueConstraint, error) {
	defLex, hasDef := elem.Attr("default")
	fixLex, hasFix := elem.Attr("fixed")
	if hasDef && hasFix {
		return nil, xsderr.New(rule, elem.Loc(),
			"declaration has both default and fixed, but %s clause 1 forbids both", rule)
	}
	if !hasDef && !hasFix {
		return nil, nil
	}
	bindings, defaultNS := namespaceContextOf(elem)
	kind, lexical := xsd.ValueDefault, defLex
	if hasFix {
		kind, lexical = xsd.ValueFixed, fixLex
	}
	vc := xsd.NewValueConstraint(kind, lexical, bindings, defaultNS)
	return &vc, nil
}

// namespaceContextOf materializes elem's in-scope namespace context for a QName
// lexical written on it (§3.3.18): one Namespace Binding per PREFIXED in-scope
// namespace, plus the default namespace an unprefixed name binds to. It serves
// every property record carrying such a lexical — a Value Constraint's
// default=/fixed=, an enumeration facet member's value= — each capturing it at
// the element the literal was written on.
//
// The default namespace is the plain in-scope xmlns default, NOT the
// xpathDefaultNamespace chain (§3.13.2) — that one is XPath's alone, and reading
// it here would silently bind an unprefixed QName default= to the target
// namespace. xmlns="" undeclares the default namespace (Namespaces in XML 1.1)
// and a declared one is never empty, so an empty lookup result means exactly "no
// default namespace in scope" and yields nil, never a present "".
func namespaceContextOf(elem *Element) ([]xsd.NamespaceBinding, *string) {
	// ok is no signal here: the empty prefix always resolves (scope.lookup).
	uri, _ := elem.lookupPrefix("")
	if uri == "" {
		return inScopeBindings(elem), nil
	}
	return inScopeBindings(elem), &uri
}

// inScopeBindings is the ONE mapping from an element's in-scope PREFIXED
// namespaces to Namespace Bindings (STYLE T4), read by every property record
// that carries a namespace context: an XPath Expression's {namespace bindings}
// (§3.13.1 nb) and a Value Constraint's own (§3.3.18). The default namespace is
// never among them — each caller obtains it from lookupPrefix(""), under its own
// rule for what an absent one means.
func inScopeBindings(elem *Element) []xsd.NamespaceBinding {
	var bindings []xsd.NamespaceBinding
	for _, ns := range elem.inScopePrefixes() {
		bindings = append(bindings, xsd.NewNamespaceBinding(ns.Prefix(), ns.URI()))
	}
	return bindings
}

// resolveQName resolves a QName-valued lexical that must name a schema component
// (a type=/base=/ref= value) to its expanded name, under BOTH halves of
// §3.17.6.2 src-resolve clause 4 (cl.qnr.nsdeclared): bindQName maps the lexical
// against the namespace bindings in scope at elem, and licensedNamespace then
// requires the namespace name it landed on to be one the containing schema
// document may reach into at all.
//
// A QName-valued attribute whose value does NOT resolve to a component —
// notQName, whose items §3.10.2's {disallowed names} mapping takes as plain QName
// values — takes bindQName directly: src-resolve governs ·resolution·, so
// charging clause 4 there would reject a name the spec never asks to resolve.
//
// attr is the local name of the schema-document attribute lexical was read from
// (or whose whitespace-separated list lexical is an item of); it is diagnostic
// only, naming the construct the author wrote in the lexical rejection bindQName
// charges (STYLE E1).
func (p *producer) resolveQName(elem *Element, lexical, attr string) (xsd.QName, error) {
	qn, err := p.bindQName(elem, lexical, attr)
	if err != nil {
		return xsd.QName{}, err
	}
	if err := p.licensedNamespace(elem, lexical, qn.Space); err != nil {
		return xsd.QName{}, err
	}
	return qn, nil
}

// bindQName maps a QName-valued lexical to its expanded name against the
// namespace bindings in scope at elem (Datatypes §3.3.18). A prefixed name whose
// prefix is unbound is rejected. An unprefixed name binds to the in-scope default
// namespace, or — when none is declared (or the default is declared empty) — to
// unqualifiedRefNS: the no-namespace name (src-resolve clause 4.1.1) normally,
// the assembly's namespace under chameleon coercion, deliberately never the
// schema's own targetNamespace otherwise.
//
// Three lexical shapes are rejected before any of that — split out into
// qnameLexical, which conditional inclusion shares — charged cvc-datatype-valid
// (Datatypes §4.1.4) at attr: none is in the ·lexical space·
// of xs:QName, the type the schema for schema documents declares for every
// QName-valued attribute (Structures §5.1, Appendix A). §3.3.18 admits exactly
// the strings matching the Namespaces in XML QName production, whose
// PrefixedName ::= Prefix ':' LocalPart has an NCName on BOTH sides, and the
// NCName pattern \i\c* ∩ [\i-[:]][\c-[:]]* (§3.4.7.1) matches neither the empty
// string nor any string carrying a colon:
//
//   - an EMPTY LOCAL PART — the whole value empty (type=""), or a prefix with
//     nothing after the colon (type="xs:");
//   - an EMPTY PREFIX (type=":T"), which is not the unprefixed shape it is
//     otherwise bound as: a value holding a colon can only be a PrefixedName,
//     and an empty Prefix is no NCName (#631);
//   - MORE THAN ONE COLON (type="xs:a:b"), which no split into Prefix ':'
//     LocalPart can rescue, since a colon is outside NCName under either half.
//     strings.Cut splits at the FIRST colon, so every extra colon lands in
//     local and testing local alone decides the shape (#631).
//
// Each is a LEXICAL fault, logically prior to and independent of namespace
// binding (PRINCIPLES 19), so all three are decided first and never charged
// src-resolve, whose clauses presuppose a well-formed QName and govern
// ·resolution· alone (§3.17.6.2). Charging them here is also what keeps the
// zero xsd.QName out of a reference slot downstream, where an xsd constructor's
// representation-invariant backstop would report an author's mistake as
// xsderr.RuleComponentInvariant, a caller fault (#343).
//
// The lexical is whiteSpace-normalized through collapseTrim before it is split:
// xs:QName's whiteSpace facet is fixed to collapse (Datatypes §3.3.18.1,
// QName.whiteSpace), and src-resolve (§3.17.6.2) resolves the ·actual value·,
// so base="    xs:string " names xs:string. An interior space survives it —
// collapse folds a run to one #x20 and never deletes it — so "xs: string"
// keeps its " string" local part.
//
// GAP(parser): the check tests those three NCName properties only, not the
// whole ncNameRE predicate conditionalQNames applies to each half, so a
// non-NCName half ("xs: string", "xs:1a") passes the lexical test. A reference
// that must resolve is then charged src-resolve instead of cvc-datatype-valid,
// and a notQName item binds to a name no NCName can match (#1801).
func (p *producer) bindQName(elem *Element, lexical, attr string) (xsd.QName, error) {
	prefix, local, fault := qnameLexical(collapseTrim(lexical))
	if fault != "" {
		return xsd.QName{}, xsderr.New(ruleDatatypeValid, elem.Loc(),
			"<%s> %s value %q is not in the ·lexical space· of xs:QName, the type the schema for schema documents declares for it: %s (Datatypes §3.3.18, §3.4.7.1)",
			elem.Name().Local(), attr, lexical, fault)
	}

	if prefix == "" {
		// An absent default binding and an explicitly empty one (xmlns="") both
		// leave the reference's namespace name ·absent·, so both take the same path.
		uri, _ := elem.lookupPrefix("")
		if uri == "" {
			return xsd.QName{Space: p.unqualifiedRefNS(elem), Local: local}, nil
		}
		return xsd.QName{Space: uri, Local: local}, nil
	}

	uri, ok := elem.lookupPrefix(prefix)
	if !ok {
		return xsd.QName{}, xsderr.New(ruleSrcResolve, elem.Loc(),
			"the QName prefix %q of %q does not resolve to an in-scope namespace (src-resolve)", prefix, lexical)
	}
	return xsd.QName{Space: uri, Local: local}, nil
}

// qnameLexical splits a QName-valued lexical into its prefix and local part and
// reports, as fault, the ·lexical space· violation that disqualifies it as an
// xs:QName, or "" when the lexical is one. The unprefixed shape comes back with
// an empty prefix, which is unambiguous precisely because an empty prefix before
// a colon is itself a fault.
//
// It is the ONE encoding of §3.3.18's lexical space in this package: the three
// faults and their reasoning are [producer.bindQName]'s, and conditional
// inclusion needs the identical split for the QName items of a vc:typeAvailable
// list (parser/conditional.go) while charging a different rule at a different
// phase. Splitting the lexical test out is what keeps the two from drifting
// (STYLE T4).
func qnameLexical(lexical string) (prefix, local, fault string) {
	before, after, found := strings.Cut(lexical, ":")
	prefix, local = "", before
	if found {
		prefix, local = before, after
	}
	switch {
	case local == "":
		return prefix, local, "a QName's local part is an NCName and is never empty"
	case found && prefix == "":
		return prefix, local, "a QName's prefix is an NCName and is never empty"
	case strings.Contains(local, ":"):
		return prefix, local, "a QName's prefix and local part are both NCNames, and an NCName carries no colon, so a QName holds at most one"
	}
	return prefix, local, ""
}

// licensedNamespace charges src-resolve clause 4 (cl.qnr.nsdeclared, §3.17.6.2)
// on a reference written at elem whose namespace name resolved to ns: the schema
// document CONTAINING the reference must itself license that namespace, which is
// what §4.2.6.1 calls "licensing references to components across namespaces". The
// license is per-document and never assembly-wide — a namespace some sibling
// document of the assembly <import>ed licenses nothing here — so a reference that
// clauses 1-3 would happily resolve is still rejected when its own document never
// asked for the namespace (#279).
//
// §4.2.6.1 is explicit that this is NOT a §5.3 missing sub-component: such
// references "are not handled as if they referred to missing components", so the
// verdict is charged here, at the reference, rather than deferred to finalize.
//
// The two namespace facts it tests are the containing document's, read as the
// spec words them. Clause 4.1.1 asks whether that document's <schema> carries a
// targetNamespace ATTRIBUTE, which for an <override>'s children is the OVERRIDING
// document's. Clause 4.2.1 tests the EFFECTIVE target namespace p.target instead:
// §F.1 task (a) has already made the including namespace a chameleon document's
// own by the time clause 4 is evaluated, and that coercion is constant across an
// <include>/<override> cascade, so p.target is the containing document's coerced
// targetNamespace either way.
func (p *producer) licensedNamespace(elem *Element, lexical, ns string) error {
	schemaElem := containingSchema(elem)
	if ns == "" {
		// Clause 4.1: an ·absent· namespace name is licensed by a document that
		// declares no targetNamespace (4.1.1) or that <import>s the unqualified
		// namespace (4.1.2, §4.2.6.1's "if that attribute is absent, then the import
		// allows unqualified reference to components with no target namespace").
		own := attrOr(schemaElem, "targetNamespace")
		if own == "" || importsNamespace(schemaElem, "") {
			return nil
		}
		return xsderr.New(ruleSrcResolve, elem.Loc(),
			"the unqualified reference %q resolves into the ·absent· namespace, which this schema document does not license: src-resolve clause 4.1 needs it to declare no targetNamespace (it declares %q) or to carry an <import> with no namespace attribute",
			lexical, own)
	}
	// Clause 4.2.1 (the document's own target namespace) and clauses 4.2.3 / 4.2.4
	// (the XSD and XSI namespaces, licensed with no <import> at all).
	if ns == p.target || ns == xsd.XMLSchemaNS || ns == xsd.XMLSchemaInstanceNS {
		return nil
	}
	if importsNamespace(schemaElem, ns) { // clause 4.2.2
		return nil
	}
	return xsderr.New(ruleSrcResolve, elem.Loc(),
		"the reference %q resolves into namespace %q, which this schema document does not license: src-resolve clause 4.2 needs it to be the document's own target namespace %q, the namespace of one of the document's own <import> elements, or the XSD or XSI namespace",
		lexical, ns, p.target)
}

// importsNamespace reports whether the <schema> element information item schema
// carries an <import> licensing references into ns — clause 4.2.2 for a namespace
// name, clause 4.1.2 for the ·absent· one (ns == ""). Only that document's OWN
// <import> children answer, since 4.2.2 scopes them to "some <import> element
// information item contained in the <schema> element information item of THAT
// schema document".
//
// The ·absent· namespace is the empty string throughout this package, so an
// <import namespace=""> reads as a bare one and licenses an absent-namespace
// reference; keeping the attribute's PRESENCE apart here would be a second
// encoding of namespace-absence (STYLE D3), the same fold checkNoSelfImport
// documents.
func importsNamespace(schema *Element, ns string) bool {
	for _, child := range schema.Children() {
		el, ok := child.(*Element)
		if !ok || !isXSD(el, "import") {
			continue
		}
		if attrOr(el, "namespace") == ns {
			return true
		}
	}
	return false
}

// containingSchema returns the <schema> element information item of the schema
// document elem textually lives in — src-resolve clause 4's "the schema document
// containing the ·QName·". That is usually the producer's own document, but a
// declaration substituted into its top level by an <override> in a DIFFERENT
// document (§F.2 clause 1) is licensed by the OVERRIDING document's <import>s, so
// the answer is the topmost ancestor of elem rather than p.schemaElem. Every
// document reaching a producer has been checked to be a <schema>
// (Document.IsSchema), so the walk cannot land on anything else.
func containingSchema(elem *Element) *Element {
	root := elem
	for e := elem; e != nil; e = e.parent {
		root = e
	}
	return root
}

// unqualifiedRefNS is the namespace name the unqualified QName REFERENCE carried
// by elem resolves to. Normally that is the ·absent· namespace (src-resolve
// clause 4.1.1). Under chameleon coercion it is the assembly's target namespace:
// §F.1 task (b) "updates all unqualified QName references so that their
// namespace names become the actual value of the targetNamespace attribute" —
// every reference in the document, including one naming a sibling component of
// the same document — and §4.2.3's closing paragraph extends the conversion to
// still-unresolved reference QNames retained for finalize.
//
// elem is needed because a chameleon document's top level can hold a declaration
// that is NOT its own: §4.2.5 clause 3.2.1 orders the two transformations "first
// [§F.1] and then [§F.2]", so an <override>'s children are substituted into Dold
// AFTER §F.1 has run over it and are therefore untouched by task (b). They keep
// the ·absent· namespace for their unqualified references, even though the
// components they define are minted in the coerced target namespace.
func (p *producer) unqualifiedRefNS(elem *Element) string {
	if p.chameleon() && p.declares(elem) {
		return p.target
	}
	return ""
}

// declares reports whether elem is part of THIS document's tree, as opposed to a
// declaration substituted into its top level by an <override> in a different
// document (§F.2 clause 1). It asks containingSchema rather than storing
// provenance on the element, since provenance is a property of the producer's
// question, not of the raw node (STYLE D3).
func (p *producer) declares(elem *Element) bool {
	return containingSchema(elem) == p.schemaElem
}

// facetKindOf maps a PLAIN-LEXICAL constraining-facet element's local name to its
// [xsd.FacetKind], delegating the name↔kind bijection to
// [builtin.FacetKindByName] rather than retyping it: that one scan covers §4.3's
// whole constraining-facet set including the precisionDecimal extension facets
// maxScale/minScale (xsd-precisionDecimal.md §4.2/§4.3), which have exactly the
// totalDigits shape — {value} is the value attribute's xs:integer lexical form,
// {fixed} the fixed attribute's boolean — and so need no arm of their own here
// (#323).
//
// The two non-lexical kinds are excluded: enumeration's and assertions' {value}
// is not a lexical string (§4.3.5/§4.3.13), so xsd.NewFacet panics on both, and
// restrictionFacets handles them separately (assertion through
// xsd.NewAssertionsFacet, enumeration through xsd.NewEnumerationFacet) in folds
// that run strictly above this lookup. Excluding them HERE too is
// belt-and-suspenders, not redundant: the bridge table spells the assertions
// facet in the plural, so the singular <assertion> element those upstream checks
// intercept would not shield a schema's literal <assertions> child from reaching
// NewFacet.
func facetKindOf(local string) (xsd.FacetKind, bool) {
	kind, ok := builtin.FacetKindByName(builtin.FacetName(local))
	if !ok || kind == xsd.FacetEnumeration || kind == xsd.FacetAssertions {
		return 0, false
	}
	return kind, true
}

// isXSD reports whether el's expanded name is {XMLSchemaNS}local.
func isXSD(el *Element, local string) bool {
	return el.Name().Space() == xsd.XMLSchemaNS && el.Name().Local() == local
}

// childElement returns el's first child element with the expanded name
// {space}local, or nil.
func childElement(el *Element, space, local string) *Element {
	for _, child := range el.Children() {
		c, ok := child.(*Element)
		if !ok {
			continue
		}
		if name := c.Name(); name.Space() == space && name.Local() == local {
			return c
		}
	}
	return nil
}

// childElements returns every child element of el with the expanded name
// {space}local, in document order (STYLE D2). It is childElement's repeatable
// twin, for the content models that admit several — <union>'s inline
// <simpleType> children (§3.16.2.4 map.std.union case 1b) and <element>'s
// <alternative> children (§3.3.2.1 dcl.elt.common).
func childElements(el *Element, space, local string) []*Element {
	var found []*Element
	for _, child := range el.Children() {
		c, ok := child.(*Element)
		if !ok {
			continue
		}
		if name := c.Name(); name.Space() == space && name.Local() == local {
			found = append(found, c)
		}
	}
	return found
}

// facetValueLexical is the literal a facet element's value attribute contributes to its
// {value}: the attribute's ·normalized value· under the whiteSpace its declared type fixes
// (Structures key-vv, key-nv). The kinds whose value is typed with whiteSpace = collapse —
// length, minLength, maxLength and fractionDigits (xs:nonNegativeInteger), totalDigits
// (xs:positiveInteger), whiteSpace and explicitTimezone (xs:NMTOKEN enumerations), and
// maxScale and minScale (xs:integer, xsd-precisionDecimal.md f-ms-value, f-mns-value) — get
// §4.3.6's full collapse: every run of XML S (#x9, #xA, #xD, #x20) becomes one #x20 and the
// ends are dropped, so " 5 " and "&#x9;5&#xA;" are the literal "5", and no other Unicode
// space is touched. The only other kinds that reach here, the four bounds, keep the
// attribute verbatim: their value is an xs:anySimpleType (preserve), read later through the
// base type's own lexical mapping, which applies that type's whiteSpace itself. <pattern>
// (xs:string, preserve) and <enumeration> are read raw by restrictionFacets before this.
//
// The collapse is done here, where the attribute is read, and not in
// xsd.NewFacet or the value and xsd readers of {value}: those take a
// programmatic caller's literal as given.
func facetValueLexical(kind xsd.FacetKind, lexical string) string {
	switch kind {
	case xsd.FacetLength, xsd.FacetMinLength, xsd.FacetMaxLength, xsd.FacetFractionDigits,
		xsd.FacetTotalDigits, xsd.FacetWhiteSpace, xsd.FacetExplicitTimezone,
		xsd.FacetMaxScale, xsd.FacetMinScale:
		return strings.Join(xmlSpaceFields(lexical), " ")
	default:
		return lexical
	}
}

// collapseTrim strips the four characters §4.3.6's whiteSpace facet is
// whitespace for — #x9, #xA, #xD, #x20 — from both ends of a lexical form.
// whiteSpace is fixed to collapse for every datatype the schema for schema
// documents declares the attributes read here with, and it is applied BEFORE
// lexical-space membership is tested (§4.1.4), so a padded " true " is the
// ·actual value· true. It is this package's one spelling of that trim; route a
// new lexical comparison through it.
//
// It cannot be strings.TrimSpace, whose unicode.IsSpace class also cuts U+0085,
// U+00A0, U+2028 and the rest — characters §4.3.6 is NOT whitespace for and
// collapse PRESERVES, so trimming them would accept literals the spec rejects:
// maxOccurs="&#xA0;unbounded" read as unbounded, mode="&#xA0;none" as none.
//
// A four-character trim is not a private copy of the collapse algorithm (STYLE
// T4): it decides membership in a set of whitespace-free literals exactly as a
// full collapse would. Let T be the trimmed literal and R its collapse — if T
// holds interior XML whitespace then R holds a #x20 and no target literal
// contains one, so both reject; otherwise R == T. Every literal compared through
// this helper is whitespace-free — none/interleave/suffix, unbounded,
// skip/strict/lax, true/false/1/0, prohibited/optional/required,
// qualified/unqualified, #all, ##any/##other, the decimal digit strings,
// NCNames, xs:QNames, xs:decimal literals and the empty string — so the
// equivalence holds at every call site but one. [producer.bindQName], whose
// GAP(parser) marker leaves the full NCName test unapplied, rejects neither a T
// with interior whitespace nor its R lexically — but neither names a declared
// component, since every declared name is an NCName.
// [producer.namespaceVarietyAndSet] also splits its trimmed value into tokens,
// and xmlSpaceFields splits a value identically with or without its ends
// trimmed.
func collapseTrim(lexical string) string {
	return strings.Trim(lexical, xmlSpace)
}

// xmlSpace is the four characters §4.3.6's whiteSpace facet is whitespace for:
// #x9, #xA, #xD, #x20.
const xmlSpace = "\x09\x0A\x0D\x20"

// xmlSpaceFields splits a list-typed lexical into its items: the maximal runs
// of characters outside xmlSpace, the XML S production (xml.md [3]). A list
// value is delimited on #x20 (cvc-datatype-valid, Datatypes §4.1.4 clause 2.2)
// after whiteSpace = collapse has mapped #x9, #xA and #xD to it (§4.3.6), and on
// nothing else. strings.Fields is not this split: it also breaks on U+0085,
// U+00A0, U+1680, U+2028, U+3000 and the other Unicode spaces, so it can cut one
// valid item in two (U+1680 is an NCName character) or two invalid halves out
// of one invalid item.
func xmlSpaceFields(lexical string) []string {
	return strings.FieldsFunc(lexical, func(r rune) bool { return strings.ContainsRune(xmlSpace, r) })
}

// facetCountValue checks a length or digits facet element's value attribute
// against the type the schema for schema documents declares for it:
// xs:nonNegativeInteger on xs:numFacet, the type of <length>, <minLength>,
// <maxLength> and <fractionDigits> (xmlschema11-2.md:4059), and
// xs:positiveInteger on <totalDigits>, whose inline restriction of xs:numFacet
// narrows it (:4090). A literal outside that type is the schema document failing
// validation against the schema for schema documents (Structures §5.1), the
// attribute failing its declared type under cvc-attribute clause 3, charged
// cvc-datatype-valid (§4.1.4) at assembly time whether or not any instance
// reaches the facet. An xmltree.Attribute's position is its owning element's, so
// el.Loc() is the attribute's location.
//
// The positiveInteger fault stays cvc-datatype-valid rather than
// totalDigits-valid-restriction: "0" is outside the attribute's own declared
// type, which is a different question from comparing {value} with a base type's
// totalDigits (§4.3.11.4), charged at finalize.
//
// The test is lexical-space membership alone (nonNegativeIntegerLexical), with
// no machine-integer parse: both types are unbounded above, so a literal past
// math.MaxInt is inside them and must pass.
//
// GAP(parser): an absent value attribute is left alone — the declaration's
// use="required" is charged nowhere, and every facet reads the absence as ""
// (#1777).
func facetCountValue(el *Element, kind xsd.FacetKind) error {
	lexical, ok := el.Attr("value")
	if !ok {
		return nil
	}
	name := "<" + el.Name().Local() + ">"
	positive, inLexical := nonNegativeIntegerLexical(collapseTrim(lexical))
	switch kind {
	case xsd.FacetLength, xsd.FacetMinLength, xsd.FacetMaxLength, xsd.FacetFractionDigits:
		if inLexical {
			return nil
		}
		return xsderr.New(ruleDatatypeValid, el.Loc(),
			"%s value %q is not a nonNegativeInteger", name, lexical)
	case xsd.FacetTotalDigits:
		if positive {
			return nil
		}
		return xsderr.New(ruleDatatypeValid, el.Loc(),
			"%s value %q is not a positiveInteger", name, lexical)
	default:
		return nil
	}
}

// nonNegativeIntegerLexical reports whether the collapsed literal s is in
// xs:nonNegativeInteger's lexical space (§3.4.20.1: an optional sign, then one or
// more digits #x30-#x39, the sign "-" only on a form denoting zero) and, as
// positive, whether it is also in xs:positiveInteger's (§3.4.25.1: an optional
// "+", then digits at least one of which is not "0"). It scans rather than
// parses, so it has no upper bound.
func nonNegativeIntegerLexical(s string) (positive, ok bool) {
	digits, negative := strings.CutPrefix(s, "-")
	if !negative {
		digits = strings.TrimPrefix(s, "+")
	}
	nonzero := strings.TrimLeft(digits, "0") != ""
	if digits == "" || !allDigits(digits) || (negative && nonzero) {
		return false, false
	}
	return nonzero, true
}

// facetFixed maps a facet element's fixed attribute to that facet's {fixed}
// property: "The actual value of the fixed [attribute], if present, otherwise
// false" (xsd-precisionDecimal.md §4.2.2 xr-maxScale and §4.3.2 xr-minScale, the
// same wording Datatypes §4.3.x carries for each of the other eleven
// {fixed}-bearing facets, e.g. §4.3.6.1 f-w-fixed). "Actual value" is the
// xs:boolean VALUE the attribute's declared type maps its literal to, not the
// literal itself, so the two mapping stages run in the order §4.1.4 fixes:
//
//   - pre-lexical. xs:boolean fixes whiteSpace to collapse (§3.3.2.3, §4.3.6) and
//     the whiteSpace facet is applied BEFORE lexical-space membership is tested
//     (§4.1.4), so " true " is the value true. collapseTrim reads that ·actual
//     value·, and its doc carries the proof that trimming §4.3.6's four
//     characters decides booleanRep membership exactly as a full collapse would.
//     The trim set is load-bearing, and the wider class Go's strings package
//     trims is what that proof rules out.
//   - lexical. booleanRep ::= 'true' | 'false' | '1' | '0' (§3.3.2.2), case
//     sensitive: "TRUE" is not in the lexical space. Anything outside those four
//     is charged cvc-datatype-valid (§4.1.4) — a literal outside a datatype's
//     lexical space is invalid, and no clause lets it default to false.
//
// An absent attribute is the {fixed} = false default and is NOT an error; that is
// a branch distinct from present-but-invalid.
func facetFixed(el *Element) (bool, error) {
	lexical, ok := el.Attr("fixed")
	if !ok {
		return false, nil
	}
	switch collapseTrim(lexical) {
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	}
	return false, xsderr.New(ruleDatatypeValid, el.Loc(),
		"<%s> fixed value %q is not in the lexical space of xs:boolean (true, false, 1, 0) that the schema for schema documents declares for it",
		el.Name().Local(), lexical)
}
