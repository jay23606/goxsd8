package parser

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// anyTypeName is the expanded name of xs:anyType, the ur-type (§3.4.7).
var anyTypeName = xsd.QName{Space: xsd.XMLSchemaNS, Local: "anyType"}

// anySimpleTypeName is the expanded name of xs:anySimpleType: the default
// {type definition} of a type=-less <attribute> (§3.2.2.1) and the fallback
// {simple type definition} of §3.4.2.2 case 5. [builtin.Seed] always seeds it,
// so symbols.builtins holds it before any document is produced.
var anySimpleTypeName = xsd.QName{Space: xsd.XMLSchemaNS, Local: "anySimpleType"}

// qNameName, booleanName and anyURIName are the expanded names of the three
// builtin simple types §3.2.7's four attribute declarations take their
// {type definition} from. [builtin.Seed] always seeds all three. anyURIName is
// also xml:base's {type definition} (produce_xmlnamespace.go).
var (
	qNameName   = xsd.QName{Space: xsd.XMLSchemaNS, Local: "QName"}
	booleanName = xsd.QName{Space: xsd.XMLSchemaNS, Local: "boolean"}
	anyURIName  = xsd.QName{Space: xsd.XMLSchemaNS, Local: "anyURI"}
)

// seedAnyType builds the ur-type Complex Type Definition xs:anyType (§3.4.7): a
// mixed complex type whose {content type} is a 1..1 sequence wrapping a single
// 0..unbounded lax ##any element wildcard, with a lax ##any attribute wildcard
// and no attribute uses, and whose {base type definition} is itself (the sole
// permitted self-derivation, any-type-itself). checkComplexBaseAcyclic (#173)
// recognises that self-derivation by name, so the seeded value passes finalize.
//
// Its {final} and {prohibited substitutions} stay EMPTY, and that is the mapped
// value rather than an unmapped one: §3.4.2.1's EBV is read off a source
// <complexType> and its ancestor <schema>, and the ur-type has neither — §3.4.7
// specifies the component outright. Nothing a document says, finalDefault=
// included, can make xs:anyType final.
func seedAnyType() (xsd.ComplexType, error) {
	anyNS, err := xsd.NewNamespaceConstraint(xsderr.Loc{}, xsd.NamespaceConstraintAny, nil, nil, nil)
	if err != nil {
		return xsd.ComplexType{}, err
	}
	wildcard, err := xsd.NewWildcard(xsderr.Loc{}, anyNS, xsd.ProcessLax)
	if err != nil {
		return xsd.ComplexType{}, err
	}
	inner, err := xsd.NewUnboundedOccurs(xsderr.Loc{}, 0)
	if err != nil {
		return xsd.ComplexType{}, err
	}
	wildcardParticle, err := xsd.NewParticle(xsderr.Loc{}, inner, xsd.ResolvedTerm{Term: wildcard})
	if err != nil {
		return xsd.ComplexType{}, err
	}
	seq, err := xsd.NewModelGroup(xsderr.Loc{}, xsd.CompositorSequence, []xsd.Particle{wildcardParticle})
	if err != nil {
		return xsd.ComplexType{}, err
	}
	oneOne, err := xsd.NewOccurs(xsderr.Loc{}, 1, 1)
	if err != nil {
		return xsd.ComplexType{}, err
	}
	topParticle, err := xsd.NewParticle(xsderr.Loc{}, oneOne, xsd.ResolvedTerm{Term: seq})
	if err != nil {
		return xsd.ComplexType{}, err
	}
	content := xsd.ElementContent{Mixed: true, Particle: topParticle}
	return xsd.NewComplexType(xsderr.Loc{}, anyTypeName, anyTypeName, nil,
		xsd.DerivationRestriction, false, nil, nil, &wildcard, content, nil, nil)
}

// seedInstanceAttributes builds the four Attribute Declarations §3.2.7 states are
// "present in every schema by definition", in its own §3.2.7.1–.4 order: xsi:type,
// xsi:nil, xsi:schemaLocation and xsi:noNamespaceSchemaLocation. Each carries the
// property table its section gives verbatim — {target namespace} the instance
// namespace, {scope} global with {parent} ·absent·, {value constraint} ·absent· —
// and differs only in {type definition}.
//
// §1.3.2 is why they are seeded UNCONDITIONALLY rather than when a reference asks
// for them: the four sit on the same footing as the builtin datatypes, "by
// definition part of every schema". They are seeded as COMPONENTS and never by
// composing an XMLSchema-instance schema document, which would put four
// declarations through sch-props-correct (§3.17.6.1) clause 2 against any user
// schema that declares them too.
//
// Every {type definition} is filled with a by-NAME reference rather than a
// resolved builtin: finalize's src-resolve ladder already walks the schema's
// {attribute declarations} and resolves exactly this slot, so threading pointers
// in from symbols.builtins would mint a second resolution mechanism beside it
// (STYLE T4).
func seedInstanceAttributes() ([]xsd.AttributeDeclaration, error) {
	// §3.2.7.3's {type definition} is an ANONYMOUS list type over anyURI, not a
	// reference to anyURI itself — the one shape among the four that has to be
	// built rather than named.
	//
	// Its property table gives {facets} ·absent·, and this deliberately diverges:
	// cos-st-restricts clause 2.2.1.2 admits exactly one {facets} set for a list
	// constructed directly on xs:anySimpleType — whiteSpace = collapse with
	// {fixed} = true — and finalize reaches this type through the ordinary
	// attribute-declaration walk, so an absent set is refused by xsd's
	// checkConstructedListFacets. §3.16.2.1 map.std.common case 3 manufactures
	// that same one-member set for every <list> a document writes, which is why
	// constructListType mints it too.
	//
	// Its {name} is the zero QName, absence's encoding here, so the {target
	// namespace} §3.2.7.3 pairs with it is unrepresentable: a QName carrying only
	// a Space would read as NAMED to every predicate testing for absence while
	// naming nothing. The property is unobservable either way — the type is
	// inline, in no by-name index.
	whiteSpace := xsd.NewFacet(xsd.FacetWhiteSpace, []string{"collapse"}, true)
	schemaLocationType, err := xsd.NewSimpleType(xsderr.Loc{}, xsd.QName{},
		xsd.ListDerivation{Item: xsd.SimpleTypeRef{Name: anyURIName}},
		xsd.OwnedSimpleType{Definition: xsd.AnySimpleType()}, []xsd.Facet{whiteSpace}, nil)
	if err != nil {
		return nil, err
	}

	seeds := []struct {
		local    string
		typeSlot xsd.TypeDefinitionOrRef
	}{
		{"type", xsd.TypeDefinitionRef{Name: qNameName}},
		{"nil", xsd.TypeDefinitionRef{Name: booleanName}},
		{"schemaLocation", xsd.InlineTypeDefinition{Definition: schemaLocationType}},
		{"noNamespaceSchemaLocation", xsd.TypeDefinitionRef{Name: anyURIName}},
	}
	decls := make([]xsd.AttributeDeclaration, 0, len(seeds))
	for _, s := range seeds {
		name := xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: s.local}
		a, err := xsd.NewAttributeDeclaration(xsderr.Loc{}, name, s.typeSlot,
			xsd.NewAttributeGlobalScope(), nil, false)
		if err != nil {
			return nil, err
		}
		decls = append(decls, a)
	}
	return decls, nil
}

// complexTypeIdentity is what a <complexType> under production is identified by.
// It is a SEALED SUM (STYLE T1/T2), unexported and sealed by the
// complexTypeIdentity marker method, with one arm per construction path §3.4.2
// and §4.2.4 admit:
//
//   - namedComplexType — a top-level <complexType>, identified by its expanded
//     {name} (§3.4.1 makes {context} ·absent· for it);
//   - elementOwnedComplexType — an inline ANONYMOUS <complexType>, which §3.4.1
//     ctd-context leaves nameless and gives a {context} instead: the minted
//     identity of the owning <element>'s declaration (§3.4.2.1 dcl.ctd.common,
//     "the Element Declaration corresponding to the nearest <element>
//     information item among the ancestor element information items");
//   - redefiningComplexType — a <complexType> child of a <redefine>, which is
//     NAMED and ALSO carries a minted identity, because src-expredef clause 1.1
//     makes it the {context} of the anonymous original it is paired with;
//   - redefineOriginalComplexType — that original: anonymous, contexted in the
//     redefining component, carrying the same minted identity, and itself owning
//     a further original when a chained <redefine> put a redefining declaration
//     in the redefined document (#585);
//   - typeAlternativeOwnedComplexType — the inline ANONYMOUS <complexType> of an
//     <alternative> (§3.12.2 declare-ta's second arm), which §3.4.2.1
//     dcl.ctd.common contexts in the enclosing ELEMENT declaration, walking past
//     the <alternative>, and which therefore carries TWO tokens: that shared
//     context and its own per-edge container mint.
//
// The last three are why this is a sum and not a two-optional-field struct. The
// struct it replaced derived "is this anonymous?" from owner-presence, which
// classifies a redefining type — named AND owner-carrying — as anonymous and
// gives every local element it declares an AnonymousComplexTypeScopeParent
// instead of a ComplexTypeScopeParent: a wrong {scope}.{parent}, representable
// only because a derived predicate over an optional-field pair cannot see the
// difference (STYLE T1). With one arm per path the shape is unrepresentable, and
// the "carries neither a name nor an owner" panics the struct needed are gone
// with the zero value they guarded.
//
// It never escapes the parser — it is the producer's own threading value, not a
// component property — and it decides three things at once, which is why it is
// one value rather than several parameters: which xsd constructor builds the
// type, which xsd.ElementScopeParent variant its nested local element
// declarations report, and which xsd.AttributeScopeParent variant its local
// attribute declarations report.
type complexTypeIdentity interface{ complexTypeIdentity() }

// namedComplexType identifies a top-level <complexType> by its expanded {name}.
// A name whose local part is empty is a grammar fault, rejected before anything
// is built — by topLevelName on run's dispatch path and by produceComplexType
// on the on-demand base-build path (see produceComplexType).
type namedComplexType struct{ name xsd.QName }

// elementOwnedComplexType identifies an inline anonymous <complexType> by the
// minted identity of the element declaration that owns it — the same
// xsd.ComponentID that declaration is built with and that the type's own
// {context} carries (§3.4.2.1 dcl.ctd.common). One mint per inline construct.
type elementOwnedComplexType struct{ owner xsd.ComponentID }

// redefiningComplexType identifies a <complexType> child of a <redefine>:
// src-expredef clause 1.2's "component which corresponds to the information item
// itself … except that its {base type definition} is the component defined in
// clause 1.1". It is NAMED — its name is what references in both the
// <redefine>ing and the <redefine>d document resolve to — and it also carries a
// minted identity, because clause 1.1 makes the original's {context} "the
// redefining component" and a ComplexTypeContext names a component by
// xsd.ComponentID, never by QName. One mint per redefinition, threaded into both
// halves of the pairing and stored on neither (see xsd.NewComplexTypeOwningBase).
type redefiningComplexType struct {
	name  xsd.QName
	owner xsd.ComponentID
}

// redefineOriginalComplexType identifies src-expredef clause 1.1's original: the
// component "which corresponds to the top-level definition item with the same
// name in the <redefine>d schema document … except that its {name} is ·absent·
// and its {context} is the redefining component".
//
// It is produced under the REDEFINED document's own producer, so its content
// takes that document's target namespace and schema-level defaults; only the
// {name}/{context} override travels with this identity.
//
// It carries TWO identities because an original may stand at both ends of an
// ownership edge. A CHAINED <redefine> — D1 redefines D2, D2 redefines D3, one
// (kind, name) throughout — makes D2's own redefining declaration the top-level
// definition item clause 1.1 reads, so the original built from it is anonymous
// AND owns an original of its own (D3's), under clause 1.2 applied at D2's level.
// Build it through newRedefineOriginal, never as a literal, so ownedOriginal is
// never the zero token.
type redefineOriginalComplexType struct {
	// owner is the identity of the component this original's {context} names —
	// clause 1.1's "the redefining component", the same value that component's
	// own identity carries — and the token its local declarations report as
	// {scope}.{parent} (xsd.AnonymousComplexTypeScopeParent's 1:1 pairing).
	owner xsd.ComponentID
	// ownedOriginal is the identity minted for the edge running the other way:
	// the {context} of the clause-1.1 original THIS one owns, when the
	// declaration it is built from is itself a redefining declaration. One mint
	// per edge — reusing owner for both would leave the two containers
	// indistinguishable, and their {context}s equal.
	ownedOriginal xsd.ComponentID
}

// newRedefineOriginal mints the identity of one clause-1.1 original: owner is the
// identity of the component that owns it, and the original's own owner-side token
// is minted here, one per ownership edge (see redefineOriginalComplexType).
func newRedefineOriginal(owner xsd.ComponentID) redefineOriginalComplexType {
	return redefineOriginalComplexType{owner: owner, ownedOriginal: xsd.NewComponentID()}
}

// typeAlternativeOwnedComplexType identifies the inline anonymous <complexType>
// of an <alternative> child of an <element> (§3.12.2 declare-ta: "the type
// definition corresponding to the complexType or simpleType among the children
// of the <alternative> element").
//
// It carries TWO tokens because the two directions elementOwnedComplexType
// collapses into one come apart here. §3.4.2.1 dcl.ctd.common gives the type "the
// Element Declaration corresponding to the nearest <element> information item
// among the ancestor element information items" — the <alternative> is not an
// <element>, so the walk goes straight past it and the {context} is the OWNING
// declaration's identity, shared with that element's own inline type and with
// every sibling <alternative>'s. The {scope}.{parent} its nested local
// declarations report cannot be that shared token: one owner reaching several
// anonymous containers would leave them indistinguishable
// (xsd.AnonymousComplexTypeScopeParent), so each <alternative> edge gets a mint
// of its own. Build it through newTypeAlternativeOwned, never as a literal, so
// container is never the zero token.
type typeAlternativeOwnedComplexType struct {
	// owner is the identity of the element declaration this type's {context}
	// names, minted by the producer before either component exists and threaded
	// into xsd.NewElementDeclarationOwningTypes as well.
	owner xsd.ComponentID
	// container is the identity this type is known by as a CONTAINER: the token
	// its nested local element and attribute declarations report as
	// {scope}.{parent}. One mint per <alternative> ownership edge.
	container xsd.ComponentID
}

// newTypeAlternativeOwned mints the identity of one <alternative>'s inline
// anonymous complex type: owner is the enclosing element declaration's identity,
// and the container token is minted here, one per ownership edge (see
// typeAlternativeOwnedComplexType).
func newTypeAlternativeOwned(owner xsd.ComponentID) typeAlternativeOwnedComplexType {
	return typeAlternativeOwnedComplexType{owner: owner, container: xsd.NewComponentID()}
}

// redefineOriginalContext returns the identity a src-expredef clause 1.1 ORIGINAL
// built under this type takes as its {context}, and false for the two arms that
// can own no original. It is the one place the two directions of the "owner"
// field are told apart: for a redefining type it is the mint clause 1.1's
// original is paired with, and for an original that is ITSELF built from a
// redefining declaration it is the second mint that original carries.
//
// The switch is exhaustive over the sealed sum; see topLevelComplexTypeName for
// why the default arm is unreachable.
func redefineOriginalContext(id complexTypeIdentity) (xsd.ComponentID, bool) {
	switch i := id.(type) {
	case redefiningComplexType:
		return i.owner, true
	case redefineOriginalComplexType:
		return i.ownedOriginal, true
	case namedComplexType, elementOwnedComplexType, typeAlternativeOwnedComplexType:
		return xsd.ComponentID{}, false
	default:
		panic("parser: redefineOriginalContext: non-exhaustive complexTypeIdentity switch")
	}
}

func (namedComplexType) complexTypeIdentity()                {}
func (elementOwnedComplexType) complexTypeIdentity()         {}
func (redefiningComplexType) complexTypeIdentity()           {}
func (redefineOriginalComplexType) complexTypeIdentity()     {}
func (typeAlternativeOwnedComplexType) complexTypeIdentity() {}

// topLevelComplexTypeName returns the expanded {name} an identity gives the type
// it builds, and false for the three arms that give none. The two NAMED arms are
// the two that can carry an unusable (empty-local-part) name, so this is what
// produceComplexType's grammar-fault guard reads.
//
// The switch is exhaustive over the sealed sum; the default arm asserts the
// invariant and is unreachable, since complexTypeIdentity is unexported and this
// file declares every arm.
func topLevelComplexTypeName(id complexTypeIdentity) (xsd.QName, bool) {
	switch i := id.(type) {
	case namedComplexType:
		return i.name, true
	case redefiningComplexType:
		return i.name, true
	case elementOwnedComplexType, redefineOriginalComplexType, typeAlternativeOwnedComplexType:
		return xsd.QName{}, false
	default:
		panic("parser: topLevelComplexTypeName: non-exhaustive complexTypeIdentity switch")
	}
}

// scopeParent returns the {scope}.{parent} every local element declaration
// nested in this type's content model reports (§3.3.2.3 dcl.elt.local: "the
// Complex Type Definition corresponding to that item"), in the variant the
// container's own identity admits.
//
// The three anonymous arms all yield AnonymousComplexTypeScopeParent carrying
// the token of the OWNERSHIP EDGE that reaches the container, which is that
// field's invariant (see xsd.AnonymousComplexTypeScopeParent). For a redefine
// original that token names the REDEFINING complex type — not because the
// original's locals are scoped to the redefinition (they are scoped to the
// original, which is what the token identifies), but because the original has no
// name and this token is what it is identified by. For a Type-Alternative-owned
// type it is the per-alternative CONTAINER token and never the shared owner: the
// owner reaches one anonymous type per <alternative> plus possibly the element's
// own, and one token across all of them would leave their nested locals reporting
// the same {scope}.{parent}.
//
// The switch is exhaustive over the sealed sum; see topLevelComplexTypeName for
// why the default arm is unreachable.
func scopeParentOf(id complexTypeIdentity) xsd.ElementScopeParent {
	switch i := id.(type) {
	case namedComplexType:
		return xsd.ComplexTypeScopeParent{Name: i.name}
	case redefiningComplexType:
		return xsd.ComplexTypeScopeParent{Name: i.name}
	case elementOwnedComplexType:
		return xsd.AnonymousComplexTypeScopeParent{Owner: i.owner}
	case redefineOriginalComplexType:
		return xsd.AnonymousComplexTypeScopeParent{Owner: i.owner}
	case typeAlternativeOwnedComplexType:
		return xsd.AnonymousComplexTypeScopeParent{Owner: i.container}
	default:
		panic("parser: scopeParentOf: non-exhaustive complexTypeIdentity switch")
	}
}

// attributeScopeParentOf returns the {scope}.{parent} every local attribute
// declaration among this type's own attribute content reports (§3.2.2.2
// dcl.att.local: "the Complex Type Definition corresponding to that item"), in
// the variant the container's own identity admits. It is the attribute-side twin
// of scopeParentOf: sc_a's alternation is CTD | AGD where sc_e's is CTD | MGD, so
// the two sums are distinct types and this function cannot be folded into that
// one.
//
// It is NOT what an attribute reached through an <attributeGroup ref> reports:
// such an attribute's own ancestor axis has no <complexType> in it, so it is
// declared once, in the group's own component, under the group's scope
// (buildAttributeGroup), and reaches the type at finalize through that
// component.
//
// The switch is exhaustive over the sealed sum; see topLevelComplexTypeName for
// why the default arm is unreachable.
func attributeScopeParentOf(id complexTypeIdentity) xsd.AttributeScopeParent {
	switch i := id.(type) {
	case namedComplexType:
		return xsd.AttributeComplexTypeScopeParent{Name: i.name}
	case redefiningComplexType:
		return xsd.AttributeComplexTypeScopeParent{Name: i.name}
	case elementOwnedComplexType:
		return xsd.AttributeAnonymousComplexTypeScopeParent{Owner: i.owner}
	case redefineOriginalComplexType:
		return xsd.AttributeAnonymousComplexTypeScopeParent{Owner: i.owner}
	case typeAlternativeOwnedComplexType:
		return xsd.AttributeAnonymousComplexTypeScopeParent{Owner: i.container}
	default:
		panic("parser: attributeScopeParentOf: non-exhaustive complexTypeIdentity switch")
	}
}

// newComplexType builds the Complex Type Definition this identity names, through
// the xsd entry point its arm admits: xsd.NewComplexType for a plain top-level
// type, xsd.NewComplexTypeOwningBase for a redefining one (which owns its base),
// and xsd.NewAnonymousComplexType for the three anonymous arms, whose §3.4.1
// tableau makes {context} Required and which differ only in which
// ComplexTypeContext arm that is. Every other argument is common to all five,
// which is why the entry points differ only in this one dispatch.
//
// The two ELEMENT-contexted arms build the identical component and are not
// folded into one case: they take their {context} from DIFFERENT fields — the
// element-owned arm's sole token, and the Type-Alternative-owned arm's owner
// rather than its container — and joining them would hide which field each reads
// behind a shared name (§3.4.2.1 dcl.ctd.common contexts both in the enclosing
// element declaration).
//
// base is the {base type definition} SLOT as resolveBaseType built it, so the
// redefining arm receives the InlineTypeDefinition holding src-expredef clause
// 1.1's original and every other arm a name (or nil). A redefining type whose
// base= is NOT its own name has no original to own; that is exactly what
// src-redefine clause 5 forbids and checkRedefinedComplexType charges before
// production begins, so the mismatch is unreachable and is reported as a plain
// producer fault rather than a fabricated rule violation.
//
// The ORIGINAL arm takes the slot BOTH ways, and is the one arm that does. It
// owns an original of its own exactly when the declaration it is built from is
// itself a redefining declaration — a chained <redefine>, where clause 1.1's
// "as defined in Schema Component Details (§3)" carries clause 1.2 with it — and
// carries a plain by-name base otherwise. Which it is, is the slot's own shape
// and is read from it (STYLE D3): resolveBaseType already made that decision,
// from the source element, when it chose the arm.
//
// The switch is exhaustive over the sealed sum; see topLevelComplexTypeName for
// why the default arm is unreachable.
func (p *producer) newComplexType(id complexTypeIdentity, loc xsderr.Loc, base xsd.TypeDefinitionOrRef, final []xsd.DerivationMethod, derivationMethod xsd.DerivationMethod, abstract bool, attributeContent []xsd.AttributeUseOrGroupRef, prohibitedAttributeNames []xsd.QName, attributeWildcard *xsd.Wildcard, contentType xsd.ContentType, prohibitedSubstitutions []xsd.DerivationMethod, assertions []xsd.Assertion) (xsd.ComplexType, error) {
	switch i := id.(type) {
	case namedComplexType:
		return xsd.NewComplexType(loc, i.name, baseTypeName(base), final,
			derivationMethod, abstract, attributeContent, prohibitedAttributeNames, attributeWildcard, contentType, prohibitedSubstitutions, assertions)
	case redefiningComplexType:
		original, owns := ownedComplexBase(base)
		if !owns {
			return xsd.ComplexType{}, fmt.Errorf("parser: the redefining <complexType> %s at %s did not resolve its own name as its {base type definition}, so src-expredef clause 1.2 has no clause-1.1 original to pair it with; src-redefine clause 5 requires that self-derivation and checkRedefinedComplexType charges it before production", i.name, loc)
		}
		return xsd.NewComplexTypeOwningBase(loc, i.owner, i.name, original, final,
			derivationMethod, abstract, attributeContent, prohibitedAttributeNames, attributeWildcard, contentType, prohibitedSubstitutions, assertions)
	case elementOwnedComplexType:
		return xsd.NewAnonymousComplexType(loc, xsd.ElementDeclarationContext{Component: i.owner}, baseTypeName(base), final,
			derivationMethod, abstract, attributeContent, prohibitedAttributeNames, attributeWildcard, contentType, prohibitedSubstitutions, assertions)
	case typeAlternativeOwnedComplexType:
		return xsd.NewAnonymousComplexType(loc, xsd.ElementDeclarationContext{Component: i.owner}, baseTypeName(base), final,
			derivationMethod, abstract, attributeContent, prohibitedAttributeNames, attributeWildcard, contentType, prohibitedSubstitutions, assertions)
	case redefineOriginalComplexType:
		context := xsd.ComplexTypeDefinitionContext{Component: i.owner}
		if original, owns := ownedComplexBase(base); owns {
			return xsd.NewAnonymousComplexTypeOwningBase(loc, i.ownedOriginal, context, original, final,
				derivationMethod, abstract, attributeContent, prohibitedAttributeNames, attributeWildcard, contentType, prohibitedSubstitutions, assertions)
		}
		return xsd.NewAnonymousComplexType(loc, context, baseTypeName(base), final,
			derivationMethod, abstract, attributeContent, prohibitedAttributeNames, attributeWildcard, contentType, prohibitedSubstitutions, assertions)
	default:
		panic("parser: newComplexType: non-exhaustive complexTypeIdentity switch")
	}
}

// baseTypeName reads back the QName the three by-name xsd constructors take from
// a {base type definition} slot: the TypeDefinitionRef arm's name, and the zero
// QName (·absent·) for a nil slot. An InlineTypeDefinition never reaches those
// constructors — only the redefining arm builds one, and it takes the component
// — so it is reported as absent here rather than silently losing a name it has
// none of.
func baseTypeName(base xsd.TypeDefinitionOrRef) xsd.QName {
	if ref, ok := base.(xsd.TypeDefinitionRef); ok {
		return ref.Name
	}
	return xsd.QName{}
}

// ownedComplexBase unwraps the {base type definition} slot resolveBaseType builds
// for src-expredef clause 1.1: the InlineTypeDefinition owning the anonymous
// original, which for a <complexType> redefinition is always a Complex Type
// Definition (it is produced from the redefined document's own top-level
// <complexType>). ok is false for every other slot shape.
func ownedComplexBase(base xsd.TypeDefinitionOrRef) (xsd.ComplexType, bool) {
	inline, ok := base.(xsd.InlineTypeDefinition)
	if !ok {
		return xsd.ComplexType{}, false
	}
	ct, ok := inline.Definition.(xsd.ComplexType)
	return ct, ok
}

// produceComplexType maps a <complexType> element (§3.4.2) into a Complex Type
// Definition, in all five source forms: implicit complex content (§3.4.2.3.2,
// restriction from xs:anyType), explicit <complexContent> with <restriction> or
// with <extension> (§3.4.2.3.3 clauses 4.1 and 4.2), and <simpleContent> with
// <restriction> or with <extension> (§3.4.2.2 cases 1-2 and 3-5). Every form
// that names a base= needs the {base type definition} — the COMPONENT for the
// three <simpleContent>/<complexContent> forms whose content-type tableau reads
// it, and only its {assertions} for the <complexContent> <restriction> form's
// §3.4.2.1 clause 1 fold — and buildComplexType/resolveBaseType supply it by
// building it on demand (§3.4.2's preamble: the mapping rules "depend upon the
// {base type definition} having been identified before they apply"). The implicit-content
// form names no base=; its base is xs:anyType, always already seeded.
//
// Which of the five forms this is decides which xs:complexTypeModel disjunct the
// <complexType>'s OWN children are ordered against (checkS4SChildOrder), so that
// check is charged here rather than inside the arms. The two child-position
// guards above it run first, on this entry path and so ahead of all five
// checkS4SChildOrder calls, because it would otherwise pre-empt each with a
// thinner verdict: repeatedContentAlternative, because a second <simpleContent>
// fills a position twice and only that guard names the offending sibling and the
// <complexType> bounding it; and misplacedOpenContent, because an <openContent>
// in any position it rejects also fills no position of the model one of those
// calls walks (#1047), and only that guard enumerates the positions
// xs:openContent IS legal in (#975).
//
// id is BOTH what the built type is constructed from — a {name} for a top-level
// one, a {context} for an inline anonymous one — and the {scope}.{parent} that
// every local element declaration nested in this type's content model (§3.3.2.3
// dcl.elt.local) and every local attribute declaration among its own attribute
// content (§3.2.2.2 dcl.att.local) reports. It is threaded down as an explicit
// xsd.ElementScopeParent / xsd.AttributeScopeParent parameter rather than stashed
// on the producer, so nesting can never mis-attribute a declaration. The
// attribute half stops at the <attributeGroup ref>, which is a scope boundary:
// the referenced group's attributes are the group's own, declared under its
// scope — see attributeScopeParentOf. The two sums are distinct types (§3.2.1
// sc_a's alternation is CTD | AGD where §3.3.1 sc_e's is CTD | MGD), so
// complexTypeIdentity emits one of each.
//
// A missing name on the NAMED arm is rejected FIRST, before any content is
// built, with the same plain grammar fault topLevelName raises and for the same
// reason (produce.go: name is use="required" typed xs:NCName on
// xs:topLevelComplexType in the schema for schema documents, and §3.4.3 src-ct
// states no clause of its own for it, incorporating the condition only by
// reference). Since #305 that fault is unreachable from run's document-order
// dispatch, which now takes every top-level name from topLevelName, and since
// #343 it is unreachable from a schema DOCUMENT at all: the other entry path
// into buildComplexType — resolveBaseType's on-demand build, whose name comes
// from a base= lexical resolved against the prescan index — can no longer carry
// an empty local part, because bindQName now rejects one as a lexical
// cvc-datatype-valid fault at the base attribute itself. What the guard covers
// is therefore a direct in-package call alone, and it is kept as the backstop
// for one: deleting it would make that path's verdict depend on whether the
// content happens to hold a local element, whose xsd.NewLocalScope would charge
// e-props-correct, an unrelated rule, and only sometimes — the #206 defect.
// The ANONYMOUS arm cannot reach the rejection at all: it carries no name to be
// missing, and its own equivalent — an unminted owner — is unconstructible,
// since produceElement/produceLocalElement mint the identity before calling.
//
// An ANONYMOUS complex type built here enters no xsd.SchemaBuilder.AddType
// (§3.17.2 scopes {type definitions} to the <complexType> children OF <schema>,
// and registering it would fork the component in two — see
// xsd/typedefinition.go's InlineTypeDefinition), and is reached instead through
// the SLOT that owns it. Finalize Phase A charges src-resolve that way, the two
// FINALIZE-side folds materialise §3.4.2.4 clause 3 and §3.4.2.5 clause 2 that
// way (xsd/ownedtypefold.go, #414), and since #438 the three read-only passes
// charge their verdicts that way too — checkContentModelsUnambiguous
// (xsd/particleattribution.go, cos-nonambig), checkElementDeclarationsConsistent
// (xsd/elementconsistent.go, cos-element-consistent) and checkComplexDerivations
// (xsd/complexderivation.go) with the checkAttributeUseNamesUnique
// (ct-props-correct clause 4) and checkExtensionAttributeUses
// (xsd/complexextension.go, cos-ct-extends clause 1.2) it drives, all three over
// xsd/complextypewalk.go's three roots.
//
// GAP(xsd): of the three identities here that yield an unnamed type —
// elementOwnedComplexType, newTypeAlternativeOwned and newRedefineOriginal — the
// LAST gets no §3.4.6 verdict at all, and the FIRST escapes one in a single
// shape. Both are #584's to close. A redefine original is seated by a {base type
// definition} rather than by a declaration's {type definition}, which is the one
// owning slot complextypewalk.go enters without charging. The escaping
// elementOwnedComplexType is the one a local <element> owns inside a <redefine>d
// <group>'s ORIGINAL definition (redefine.go's redefinedGroupRestricted, seated
// by AddRedefiningModelGroup into xsd/redefinition.go's
// modelGroupRedefinitions): its own seating is the ordinary {type
// definition} the walk charges everywhere else, but the original holding it is
// in no property and no index, and is therefore in none of complextypewalk.go's
// three roots. The {attribute uses} and {attribute wildcard} of BOTH ARE folded
// (xsd/attributeusefold.go's baseAttributeUses, #505; xsd/ownedtypefold.go's
// fourth root, #414), so what is withheld is a verdict and never a property
// value. §3.4.2.1 clause 1's {assertions} fold is not among the gaps and needs
// no issue of its own: assertionsWithBase runs HERE, on every produced type,
// anonymous ones included (#346). The attribute side has no slot to reach at
// all: no XSD production puts a <complexType> under an <attribute>, and
// xsd.NewAttributeDeclaration rejects the shape outright (§3.2.1,
// a-props-correct clause 1). The direction is open (under-rejection), never
// fail-closed, which is why conformance/schema.go judges an anonymous complex
// type on the same terms as a named one and narrows its lane for anonymity
// nowhere (#1126, complexTypeDecidable's doc comment).
func (p *producer) produceComplexType(id complexTypeIdentity, el *Element) (xsd.ComplexType, error) {
	if name, named := topLevelComplexTypeName(id); named && name.Local == "" {
		return xsd.ComplexType{}, fmt.Errorf("parser: top-level <complexType> at %s has no usable name: its name attribute is absent or empty, and the schema for schema documents requires one — xs:topLevelComplexType declares name use=\"required\" with type xs:NCName", el.Loc())
	}
	if dup := repeatedContentAlternative(el); dup != nil {
		return xsd.ComplexType{}, fmt.Errorf("parser: <%s> at %s is a second content alternative on the <complexType> at %s, which the schema for schema documents prohibits: xs:complexTypeModel (§3.4.2) is a plain xs:choice, so a <complexType> carries exactly one of <simpleContent>, <complexContent>, or the implicit-content form", dup.Name().Local(), dup.Loc(), el.Loc())
	}
	if oc := misplacedOpenContent(el); oc != nil {
		return xsd.ComplexType{}, fmt.Errorf("parser: <openContent> at %s is in a position the schema for schema documents does not allow: xs:openContent is referenced as a child of <complexType> only in the implicit-content form (no <simpleContent>/<complexContent>), under <complexContent> only of the <restriction>/<extension> alternant, and nowhere at all under <simpleContent>", oc.Loc())
	}
	sc := childElement(el, xsd.XMLSchemaNS, "simpleContent")
	cc := childElement(el, xsd.XMLSchemaNS, "complexContent")
	// xs:complexTypeModel's own child order, on whichever of its three disjuncts
	// this <complexType> writes.
	model := s4sComplexTypeImplicit
	if sc != nil || cc != nil {
		model = s4sComplexTypeWrapped
	}
	if err := checkS4SChildOrder(el, model); err != nil {
		return xsd.ComplexType{}, err
	}
	if sc != nil {
		return p.produceSimpleContent(id, el, sc)
	}
	if cc != nil {
		return p.produceComplexContent(id, el, cc)
	}
	return p.produceImplicitContent(id, el)
}

// produceImplicitContent maps a <complexType> with neither <simpleContent> nor
// <complexContent> (§3.4.2.3.2): the {base type definition} is xs:anyType and
// {derivation method} is restriction. The explicit content, attribute uses, and
// attribute wildcard come directly from the <complexType>'s own children, and
// this type is the {scope}.{parent} of every local element and every local
// attribute among them.
//
// Those children's ORDER is checked by produceComplexType, against
// s4sComplexTypeImplicit, before this is reached: the positions are the
// <complexType>'s own here, so the check belongs where the disjunct is chosen
// rather than to one arm of the dispatch.
func (p *producer) produceImplicitContent(id complexTypeIdentity, el *Element) (xsd.ComplexType, error) {
	mixed, _, err := boolAttr(el, "mixed")
	if err != nil {
		return xsd.ComplexType{}, err
	}
	abstract, _, err := boolAttr(el, "abstract")
	if err != nil {
		return xsd.ComplexType{}, err
	}
	// {assertions} (§3.4.2.1 clause 2) come from the <assert> children of the
	// <complexType> itself in this implicit-content form. Clause 1's fold of the
	// base's own {assertions} is not applied through assertionsWithBase here, and
	// does not need to be: the base is unconditionally xs:anyType, whose
	// {assertions} is the empty sequence (§3.4.7, seedAnyType), so the fold is
	// PROVABLY the identity — and it is the seeded xs:anyType that any lookup
	// would find, since resolveBaseType answers that name from symbols.anyType
	// and reaches no document. There is no base hop, so the content model is
	// entered at once.
	assertions := p.assertionsOf(el)
	p.enterContentModel(id, el, assertions)
	content, err := p.buildComplexContentType(el, mixed, scopeParentOf(id))
	if err != nil {
		return xsd.ComplexType{}, err
	}
	uses, prohibited, wildcard, err := p.produceAttributeUses(el, el, attributeScopeParentOf(id))
	if err != nil {
		return xsd.ComplexType{}, err
	}
	return p.newComplexType(id, el.Loc(), xsd.TypeDefinitionRef{Name: anyTypeName}, p.complexTypeFinal(el),
		xsd.DerivationRestriction, abstract, uses, prohibited, wildcard, content, p.complexTypeProhibitedSubstitutions(el), assertions)
}

// produceSimpleContent maps a <complexType><simpleContent> (§3.4.2.2) into a
// Complex Type Definition whose {content type} has {variety} simple, {particle}
// and {open content} ·absent·, and {simple type definition} computed by the
// five-case tableau keyed on the resolved {base type definition} and on which
// derivation alternant is chosen (simpleContentSimpleType). Both alternants map
// through this one body: §3.4.2.2 gives them the same {base type definition},
// the same {content type} shape and the same {derivation method} row, and they
// differ only in that tableau and in their content models, which the two
// children-reading calls below already tell apart.
//
// A <simpleContent> with NEITHER alternant is a plain grammar fault, and is
// reported as one: §3.4.2.2 states outright that "either <restriction> or
// <extension> must appear in the content of <simpleContent>", and that
// requirement lives in the schema for schema documents, which src-ct
// incorporates by reference without stating a clause of its own — the same
// footing as the <complexContent> half (produceComplexContent). A
// <simpleContent> carrying BOTH alternants is the other half of that same
// xs:choice and is charged the same way, by repeatedDerivationAlternant ahead of
// derivationAlternant — without it the <restriction>-first read would produce
// from one alternant and drop the other in silence.
//
// The CHILD ORDER of the <simpleContent> and of the alternant it holds is
// charged by checkS4SChildOrder, against s4sSimpleContentWrapper and then
// s4sSimpleRestriction/s4sSimpleExtension. Clause 2 below is behind both of
// them; clause 1 is NOT, since it is charged over the <complexType> rather than
// over anything under the <simpleContent> — the walk in front of it is
// produceComplexType's, which runs before this body is entered at all. The run
// order and why it is this repo's to choose are recorded on
// checkS4SChildOrder's doc.
//
// It enforces the two src-ct (§3.4.3) clauses this representation carries, both
// Schema Representation Constraints on the source XML and both charged before
// anything is built:
//
//   - clause 1 (simple-content-rules): with the <simpleContent> alternative
//     chosen, the <complexType> must not have mixed="true". It is charged at the
//     <complexType>'s own position, which is where the offending attribute is.
//   - clause 2: under a <restriction>, no facet-specifying element other than
//     xs:enumeration, xs:pattern or xs:assertion may appear more than once among
//     its [children] (repeatedFacetChild). It is charged HERE, ahead of the
//     synthesis, and not left to st-props-correct clause 4 — which would reject
//     the same document at xsd.NewSimpleType for two {facets} of one kind, but
//     under a Schema Component Constraint about the component rather than the
//     representation rule the source actually violates (STYLE E2), and only for
//     the kinds the two exception sets happen to agree on.
func (p *producer) produceSimpleContent(id complexTypeIdentity, ctElem, sc *Element) (xsd.ComplexType, error) {
	mixed, hasMixed, err := boolAttr(ctElem, "mixed")
	if err != nil {
		return xsd.ComplexType{}, err
	}
	if hasMixed && mixed {
		return xsd.ComplexType{}, xsderr.New(ruleSrcCT, ctElem.Loc(),
			"<complexType> has a <simpleContent> child and a mixed whose ·actual value· is true, but src-ct clause 1 forbids mixed = true when the <simpleContent> alternative is chosen")
	}
	if dup := repeatedDerivationAlternant(sc); dup != nil {
		return xsd.ComplexType{}, fmt.Errorf("parser: <%s> at %s is a second derivation alternant on the <simpleContent> at %s, which the schema for schema documents prohibits: xs:simpleContent (§3.4.2.2) holds a plain xs:choice, so a <simpleContent> carries exactly one of <restriction>, <extension>", dup.Name().Local(), dup.Loc(), sc.Loc())
	}
	if err := checkS4SChildOrder(sc, s4sSimpleContentWrapper); err != nil {
		return xsd.ComplexType{}, err
	}
	derivation, method := derivationAlternant(sc)
	if derivation == nil {
		return xsd.ComplexType{}, fmt.Errorf("parser: <simpleContent> at %s has neither a <restriction> nor an <extension> child, one of which §3.4.2.2 requires", sc.Loc())
	}
	alternantModel := s4sSimpleRestriction
	if method == xsd.DerivationExtension {
		alternantModel = s4sSimpleExtension
	}
	if err := checkS4SChildOrder(derivation, alternantModel); err != nil {
		return xsd.ComplexType{}, err
	}
	if dup := repeatedFacetChild(derivation, method); dup != nil {
		return xsd.ComplexType{}, xsderr.New(ruleSrcCT, dup.Loc(),
			"<%s> is a second %s facet among the [children] of a <restriction> under <simpleContent>, but src-ct clause 2 admits a repeated facet-specifying element only for xs:enumeration, xs:pattern and xs:assertion", dup.Name().Local(), dup.Name().Local())
	}
	baseLex, hasBase := derivation.Attr("base")
	if !hasBase {
		// Appendix A declares base use="required" on both xs:extensionType and
		// xs:simpleRestrictionType (which takes it from xs:restrictionType), and
		// §3.4.3 states no Schema Representation Constraint for its absence, so a
		// missing base is a plain grammar fault carrying no rule — the same reading
		// produceGroupRefParticle gives a <group> with no ref. It is charged here
		// rather than handed to resolveQName as an empty lexical, which would report
		// an absent attribute as a base="" the author never wrote.
		return xsd.ComplexType{}, fmt.Errorf("parser: the <%s> of a <simpleContent> at %s must carry a base attribute, but none is present", derivation.Name().Local(), derivation.Loc())
	}
	baseName, err := p.resolveQName(derivation, baseLex, "base")
	if err != nil {
		return xsd.ComplexType{}, err
	}
	base, baseRef, err := p.resolveBaseType(id, derivation, baseName)
	if err != nil {
		return xsd.ComplexType{}, err
	}
	baseDef, err := baseComponent(base, derivation)
	if err != nil {
		return xsd.ComplexType{}, err
	}
	simple, err := p.simpleContentSimpleType(derivation, method, baseDef)
	if err != nil {
		return xsd.ComplexType{}, err
	}
	abstract, _, err := boolAttr(ctElem, "abstract")
	if err != nil {
		return xsd.ComplexType{}, err
	}
	uses, prohibited, wildcard, err := p.produceAttributeUses(ctElem, derivation, attributeScopeParentOf(id))
	if err != nil {
		return xsd.ComplexType{}, err
	}
	// {assertions} (§3.4.2.1): clause 1's members of the resolved base's own
	// {assertions} — nothing at all when it is a simple type, the common case on
	// the extension alternant — ahead of clause 2's <assert> children of the
	// derivation alternant. Those <assert> children are the CTD's own assertions
	// and NOT facets of a synthesized simple type: the <assertion> inside
	// <restriction>'s facet choice is the facet (restrictionFacets folds it), and
	// the trailing <assert>* of the same content model is this property. Two
	// element names, two properties; conflating them would build a wrong component
	// in silence.
	return p.newComplexType(id, ctElem.Loc(), baseRef, p.complexTypeFinal(ctElem),
		method, abstract, uses, prohibited, wildcard, xsd.SimpleContent{SimpleType: simple},
		p.complexTypeProhibitedSubstitutions(ctElem), assertionsWithBase(base, p.assertionsOf(derivation)))
}

// repeatedFacetChild returns the SECOND facet-specifying child of one kind under
// a <simpleContent> <restriction> in document order, or nil when src-ct clause 2
// (§3.4.3) is satisfied: "no facet-specifying element other than xs:enumeration,
// xs:pattern, or xs:assertion may appear more than once among the [children] of
// <restriction>". method carries the clause's own antecedent — the rule is
// scoped to the <restriction> alternant — so the <extension> alternant, whose
// content model admits no facet child at all, answers nil without a scan.
//
// The three excepted names need no test of their own here, because facetKindOf
// — the ONE name→kind bridge this package consults (builtin.FacetKindByName,
// STYLE T4) — already answers "not a facet" for xs:enumeration and for
// xs:assertion, whose element name is not the facet name "assertions" the bridge
// is keyed by. Only xs:pattern has to be excepted explicitly. That reuse is what
// keeps a future constraining facet from being admitted twice here by a
// hand-typed list nobody updated (the drift builtin.FacetKindByName exists to
// end), and the two borrowed exceptions are pinned by a test that repeats all
// three excepted names under one <restriction> and requires it accepted.
//
// The kinds are collected in a slice scanned linearly, never a map: at most
// sixteen kinds per <restriction> is no measured hot path (STYLE D3), and the
// scan reports the FIRST repeat by document position (STYLE D2).
func repeatedFacetChild(restriction *Element, method xsd.DerivationMethod) *Element {
	if method != xsd.DerivationRestriction {
		return nil
	}
	var seen []xsd.FacetKind
	for _, child := range restriction.Children() {
		el, ok := child.(*Element)
		if !ok || el.Name().Space() != xsd.XMLSchemaNS {
			continue
		}
		kind, ok := facetKindOf(el.Name().Local())
		if !ok || kind == xsd.FacetPattern {
			continue
		}
		if slices.Contains(seen, kind) {
			return el
		}
		seen = append(seen, kind)
	}
	return nil
}

// simpleContentSimpleType is the §3.4.2.2 {content type}.{simple type
// definition} tableau, all five cases, keyed on the resolved {base type
// definition} and on which derivation alternant was chosen:
//
//   - case 1 — the base is a complex type whose own {content type} has {variety}
//     simple and the alternant is <restriction>: SYNTHESIZE an anonymous simple
//     type restricting B, which is 1.1 the <simpleType> among the <restriction>'s
//     [children] if there is one, and 1.2 otherwise the {simple type definition}
//     of the base's own {content type};
//   - case 2 — the base is a complex type whose own {content type} has {variety}
//     mixed and an ·emptiable· {particle}, and the alternant is <restriction>:
//     the same synthesis against SB, the inline <simpleType> if there is one and
//     ·xs:anySimpleType· otherwise;
//   - case 3 — the base is a complex type with simple content and the alternant
//     is <extension>: reuse THAT content type's {simple type definition};
//   - case 4 — the base is a simple type definition and the alternant is
//     <extension>: reuse it;
//   - case 5 (c-ctsc-bad): otherwise ·xs:anySimpleType·.
//
// Cases 3, 4 and 5 return an EXISTING *xsd.SimpleType pointer; nothing is
// rebuilt, so simple-type component identity is preserved
// (xsd/typedefinition.go). Cases 1 and 2 mint a NEW component, the one the
// tableau spells out property by property: {name} ·absent·, {final} the empty
// set, {base type definition} B/SB, {facets} the facet children mapped by
// §3.16.6.4 (restrictionFacets, shared verbatim with <simpleType><restriction>),
// and {variety}/{primitive type definition}/{item type definition}/{member type
// definitions} all "B's" — which xsd.SimpleType derives off the base chain
// rather than storing (STYLE D3), so naming B is the whole of it. Its
// {target namespace} and {context} are unmodeled here, exactly as they are for
// every other anonymous simple type this producer builds (resolveBase's inline
// base, declaredType's inline <simpleType>): the component is unnameable, so no
// reader can reach it by name, and §3.16.1 std-context is a property xsd models
// for no simple type at all. Its retained position is the <restriction>'s own —
// the source item the whole component is mapped from, this type having no
// declaring element to take one from.
//
// THE B SLOT IS THE OWNED ARM even when B has a name — a base with simple
// content usually hands over a builtin — because the tableau names a COMPONENT
// of the already-resolved base, not a QName in the source: the base= this
// <restriction> carries names the complex type, and B is read off it. That is
// the fourth licensed use of xsd.OwnedSimpleType, recorded in its doc beside the
// three that predate it; emitting a SimpleTypeRef instead would name a type this
// <restriction> never wrote, and would be unrepresentable for a B that is
// anonymous.
//
// CASE 2's ·emptiable· HALF IS NOT TESTED, and the omission changes no verdict.
// Emptiability is Particle Emptiable (§3.9.6.3, cos-group-emptiable), which
// xsd's ParticleEmptiable answers off an assembled Schema — <group ref> edges
// resolved and the effective total range computed — and no Schema exists at
// mapping time. What the untested half would decide is case 2 against case 5 for
// a mixed base whose particle is NOT ·emptiable·, and a complex type restricting
// such a base to simple content is rejected either way: derivation-ok-restriction
// (§3.4.6.3) clause 2 has no branch for it — 2.1 wants xs:anyType, whose own
// particle IS ·emptiable·; 2.2.2.1 wants a base with simple content, which a
// mixed base is not; 2.2.2.2 wants exactly the ·emptiable· particle this case
// lacks; 2.3 and 2.4 want a T that is not simple content. So the two readings
// differ only in which rejection the schema earns, never in whether it is
// rejected, and this is a deferred CHARGE rather than a withheld one (STYLE P3).
//
// Case 5 deliberately MAPS rather than rejects, on both alternants: the tableau
// names a result for every base, and the invalidity of the ones it catches is
// cos-ct-extends' (§3.4.6.2), ct-props-correct clause 2's (§3.4.6.1, a simple
// base under a restriction) or derivation-ok-restriction's (§3.4.6.3) to charge
// at finalize, never the mapping's. On the <restriction> alternant that means
// the facet children AND any inline <simpleType> are dropped, which is what the
// tableau says to do with them — case 5's result is ·xs:anySimpleType· itself,
// not a restriction of it — and no verdict rides on them, since every case-5
// restriction shape is rejected by one of those three rules.
//
// The seeded ·xs:anySimpleType· is read from the symbol table; a nil one (an
// unseeded backend) is rejected downstream by xsd.NewComplexType as an absent
// Required {simple type definition}.
func (p *producer) simpleContentSimpleType(derivation *Element, method xsd.DerivationMethod, base xsd.TypeDefinition) (*xsd.SimpleType, error) {
	anySimpleType := p.symbols.builtins[anySimpleTypeName]
	if method == xsd.DerivationExtension {
		return extendedSimpleType(base, anySimpleType), nil // cases 3, 4 and 5
	}
	b, restricts := restrictedSimpleBase(base, anySimpleType)
	if !restricts {
		return anySimpleType, nil // case 5
	}
	if inline := childElement(derivation, xsd.XMLSchemaNS, "simpleType"); inline != nil {
		st, err := p.constructSimpleType(xsd.QName{}, inline) // clause 1.1, and case 2's SB
		if err != nil {
			return nil, err
		}
		b = st
	}
	facets, err := p.restrictionFacets(derivation)
	if err != nil {
		return nil, err
	}
	return xsd.NewSimpleType(derivation.Loc(), xsd.QName{}, xsd.RestrictionDerivation{},
		xsd.OwnedSimpleType{Definition: b}, facets, nil)
}

// extendedSimpleType is the §3.4.2.2 tableau restricted to the <extension>
// alternant: case 3 (a base with simple content hands over its own {simple type
// definition}), case 4 (a simple type base is itself the result) and case 5
// (anything else falls through to ·xs:anySimpleType·).
func extendedSimpleType(base xsd.TypeDefinition, anySimpleType *xsd.SimpleType) *xsd.SimpleType {
	switch b := base.(type) {
	case *xsd.SimpleType:
		return b // case 4
	case xsd.ComplexType:
		switch bc := b.ContentType().(type) {
		case xsd.SimpleContent:
			return bc.SimpleType // case 3
		case xsd.EmptyContent, xsd.ElementContent:
			return anySimpleType // case 5
		default:
			panic("parser: extendedSimpleType: non-exhaustive ContentType switch")
		}
	default:
		panic("parser: extendedSimpleType: non-exhaustive TypeDefinition switch")
	}
}

// restrictedSimpleBase is the §3.4.2.2 tableau's case discrimination for the
// <restriction> alternant, and returns the DEFAULT base of the simple type cases
// 1 and 2 synthesize — clause 1.2's "the {simple type definition} of the {content
// type} of the {base type definition}" for case 1, and case 2's
// ·xs:anySimpleType· — which an inline <simpleType> child overrides at the caller
// (clause 1.1, and case 2's parenthesis). restricts is false for case 5, where
// nothing is synthesized at all.
//
// The discriminator is the BASE's own {content type}.{variety}, never whether an
// inline <simpleType> is present: that child chooses B WITHIN case 1 and SB
// within case 2, and a reading that keyed the case on it would put every
// inline-typed restriction of an element-only base into case 2, which the
// tableau leaves to case 5.
func restrictedSimpleBase(base xsd.TypeDefinition, anySimpleType *xsd.SimpleType) (*xsd.SimpleType, bool) {
	b, complex := base.(xsd.ComplexType)
	if !complex {
		return nil, false // case 5: a simple type base restricted rather than extended
	}
	switch bc := b.ContentType().(type) {
	case xsd.SimpleContent:
		return bc.SimpleType, true // case 1.2
	case xsd.ElementContent:
		if bc.Mixed {
			return anySimpleType, true // case 2
		}
		return nil, false // case 5: element-only content
	case xsd.EmptyContent:
		return nil, false // case 5
	default:
		panic("parser: restrictedSimpleBase: non-exhaustive ContentType switch")
	}
}

// produceComplexContent maps a <complexType><complexContent> (§3.4.2.3), in both
// derivation alternants: <restriction>, whose {content type} is purely structural
// (§3.4.2.3.3 clause 4.1), and <extension>, whose {content type} merges the
// resolved base's particle with this derivation's ·effective content· (clause
// 4.2). It enforces src-ct clause 5 (§3.4.3): when mixed is present on both
// <complexType> and <complexContent>, the two actual values must agree.
//
// A <complexContent> with neither alternant is a plain grammar fault, not a rule
// verdict: §3.4.2.3 states outright that "either <restriction> or <extension>
// must appear in the content of <complexContent>", and that requirement lives in
// the schema for schema documents, which src-ct incorporates by reference without
// stating a clause of its own (the same footing as a nameless top-level
// <complexType>). A <complexContent> carrying BOTH alternants is the other half
// of that same xs:choice and is charged the same way, by
// repeatedDerivationAlternant ahead of derivationAlternant — without it the
// <restriction>-first read would produce from one alternant and drop the other in
// silence.
//
// The CHILD ORDER of the <complexContent> and of the alternant it holds is
// charged by checkS4SChildOrder, against s4sComplexContentWrapper and then
// s4sComplexRestriction/s4sComplexExtension, both ahead of src-ct clause 5
// below. The run order and why it is this repo's to choose are recorded on
// checkS4SChildOrder's doc.
func (p *producer) produceComplexContent(id complexTypeIdentity, ctElem, cc *Element) (xsd.ComplexType, error) {
	if dup := repeatedDerivationAlternant(cc); dup != nil {
		return xsd.ComplexType{}, fmt.Errorf("parser: <%s> at %s is a second derivation alternant on the <complexContent> at %s, which the schema for schema documents prohibits: xs:complexContent (§3.4.2.3) holds a plain xs:choice, so a <complexContent> carries exactly one of <restriction>, <extension>", dup.Name().Local(), dup.Loc(), cc.Loc())
	}
	if err := checkS4SChildOrder(cc, s4sComplexContentWrapper); err != nil {
		return xsd.ComplexType{}, err
	}
	derivation, method := derivationAlternant(cc)
	if derivation == nil {
		return xsd.ComplexType{}, fmt.Errorf("parser: <complexContent> at %s has neither a <restriction> nor an <extension> child, one of which §3.4.2.3 requires", cc.Loc())
	}
	alternantModel := s4sComplexRestriction
	if method == xsd.DerivationExtension {
		alternantModel = s4sComplexExtension
	}
	if err := checkS4SChildOrder(derivation, alternantModel); err != nil {
		return xsd.ComplexType{}, err
	}
	ctMixed, ctHasMixed, err := boolAttr(ctElem, "mixed")
	if err != nil {
		return xsd.ComplexType{}, err
	}
	ccMixed, ccHasMixed, err := boolAttr(cc, "mixed")
	if err != nil {
		return xsd.ComplexType{}, err
	}
	if ctHasMixed && ccHasMixed && ctMixed != ccMixed {
		return xsd.ComplexType{}, xsderr.New(ruleSrcCT, cc.Loc(),
			"mixed is present on both <complexType> and <complexContent> with differing values, but src-ct clause 5 requires them to be the same")
	}
	// {effective mixed} (§3.4.2.3.3 clause 1): <complexContent>'s mixed if present,
	// else <complexType>'s, else false.
	mixed := ctMixed
	if ccHasMixed {
		mixed = ccMixed
	}
	abstract, _, err := boolAttr(ctElem, "abstract")
	if err != nil {
		return xsd.ComplexType{}, err
	}
	baseLex, hasBase := derivation.Attr("base")
	if !hasBase {
		// base is use="required" on both xs:extensionType and xs:complexRestrictionType
		// (Appendix A) and §3.4.3 states no clause for its absence, so this is a plain
		// grammar fault of the same kind as the missing alternant above — and charging
		// it here keeps an absent attribute from being reported as a base="" the
		// author never wrote.
		return xsd.ComplexType{}, fmt.Errorf("parser: the <%s> of a <complexContent> at %s must carry a base attribute, but none is present", derivation.Name().Local(), derivation.Loc())
	}
	baseName, err := p.resolveQName(derivation, baseLex, "base")
	if err != nil {
		return xsd.ComplexType{}, err
	}
	// The base is resolved here, for BOTH alternants, because §3.4.2.1 clause 1's
	// {assertions} fold reads it on both — where §3.4.2.3.3 clause 4's
	// content-type merge needs the COMPONENT on the extension alternant only.
	// Resolving it on the restriction alternant too moves nothing but the moment:
	// every named base is built by run at its own document-order position anyway,
	// and a base on its own base chain or resolving to nothing is charged the same
	// ct-props-correct clause 3 / src-resolve clause 1.1 verdict resolveBaseType
	// always charges. A base whose content model holds this derivation yields
	// only its {assertions}, which is all the restriction alternant reads.
	base, baseRef, err := p.resolveBaseType(id, derivation, baseName)
	if err != nil {
		return xsd.ComplexType{}, err
	}
	// {assertions} (§3.4.2.1): clause 1's members of the resolved base's own
	// {assertions}, then clause 2's <assert> children of the derivation alternant
	// — not of the enclosing <complexType> — in this explicit complex-content form.
	// They are final once the base is resolved, so the content model is entered
	// with them.
	assertions := assertionsWithBase(base, p.assertionsOf(derivation))
	p.enterContentModel(id, ctElem, assertions)
	content, err := p.complexContentType(derivation, method, base, mixed, scopeParentOf(id))
	if err != nil {
		return xsd.ComplexType{}, err
	}
	uses, prohibited, wildcard, err := p.produceAttributeUses(ctElem, derivation, attributeScopeParentOf(id))
	if err != nil {
		return xsd.ComplexType{}, err
	}
	return p.newComplexType(id, ctElem.Loc(), baseRef, p.complexTypeFinal(ctElem),
		method, abstract, uses, prohibited, wildcard, content, p.complexTypeProhibitedSubstitutions(ctElem), assertions)
}

// derivationAlternant returns the <restriction> or <extension> child of a
// <simpleContent> or a <complexContent> together with the {derivation method} it
// maps to, or (nil, 0) when neither is present. One function serves both
// wrappers (STYLE T4) because §3.4.2.2 and §3.4.2.3 state the same {derivation
// method} row in the same words — restriction for the <restriction> alternative,
// extension otherwise — and neither producer needs to tell the wrappers apart to
// read it. What the two wrappers do NOT share is the alternant's content model,
// and that is read by the callers, not here.
//
// Its two first-match reads are unambiguous because both callers charge
// repeatedDerivationAlternant ahead of them: a wrapper reaching here carries at
// most one alternant between the two, so which one is looked for first decides
// nothing.
func derivationAlternant(wrapper *Element) (*Element, xsd.DerivationMethod) {
	if r := childElement(wrapper, xsd.XMLSchemaNS, "restriction"); r != nil {
		return r, xsd.DerivationRestriction
	}
	if e := childElement(wrapper, xsd.XMLSchemaNS, "extension"); e != nil {
		return e, xsd.DerivationExtension
	}
	return nil, 0
}

// complexContentType computes the ·explicit content type· of a complex content
// (§3.4.2.3.3 clause 4) from the derivation alternant's children: clause 4.1 for
// a restriction, clause 4.2 for an extension. derivation is the <restriction> or
// <extension> element, base the {base type definition} its base= names
// (already resolved by the caller, which needs it for the §3.4.2.1 clause 1
// {assertions} fold on both alternants; only the extension branch reads it
// here, and needs the finished COMPONENT), effectiveMixed is clause
// 1's result, and scopeParent is the enclosing Complex Type Definition every
// local element declaration in this content model is scoped to (§3.3.2.3
// dcl.elt.local).
//
// Clause 6's ·wildcard element· wrap is applied on top of that result by
// openContentType, in both branches: the derivation alternant is exactly the
// element whose <openContent> child clause 5.1 reads.
func (p *producer) complexContentType(derivation *Element, method xsd.DerivationMethod, base resolvedBase, effectiveMixed bool, scopeParent xsd.ElementScopeParent) (xsd.ContentType, error) {
	if method == xsd.DerivationRestriction {
		return p.buildComplexContentType(derivation, effectiveMixed, scopeParent)
	}
	baseDef, err := baseComponent(base, derivation)
	if err != nil {
		return nil, err
	}
	effective, explicitEmpty, err := p.effectiveContent(derivation, effectiveMixed, scopeParent)
	if err != nil {
		return nil, err
	}
	explicit, err := xsd.ExtensionContentType(derivation.Loc(), baseDef, effective, explicitEmpty, effectiveMixed, p.resolveModelGroup)
	if err != nil {
		return nil, err
	}
	return p.openContentType(derivation, explicit)
}

// buildComplexContentType computes the {content type} of a restriction-derived
// (or implicit) complex content from parent's model-group child (§3.4.2.3.3
// clauses 2-4, restriction case). parent is the <complexType> (implicit) or the
// <restriction> (explicit complex content); effectiveMixed is clause 1's result;
// scopeParent is the enclosing Complex Type Definition every local element
// declaration in this content model is scoped to (§3.3.2.3 dcl.elt.local).
//
// Clauses 5-6 then fold in the ·wildcard element· (openContentType): parent is
// also the element whose <openContent> child clause 5.1 reads.
func (p *producer) buildComplexContentType(parent *Element, effectiveMixed bool, scopeParent xsd.ElementScopeParent) (xsd.ContentType, error) {
	effective, _, err := p.effectiveContent(parent, effectiveMixed, scopeParent)
	if err != nil {
		return nil, err
	}
	return p.openContentType(parent, explicitContentType(effective, effectiveMixed))
}

// explicitContentType is §3.4.2.3.3 clause 4.1 — the restriction branch's
// ·explicit content type· — as a total function of the already-computed
// ·effective content·: clause 4.1.1 (empty effective content ⇒ {variety} empty,
// which admits NO character content at all, unlike element-only) and clause 4.1.2
// (otherwise mixed iff ·effective mixed·). Clause 4.2.1 says the extension cases
// with a simple or empty/simple-content base yield "a Content Type as per clause
// 4.1.1 and clause 4.1.2 above", i.e. exactly this.
//
// It is stated a SECOND time, in package xsd, and that split was ruled at #392's
// warden pre-flight rather than overlooked. Clause 4.2 moved down to xsd because
// cos-ct-extends clause 1.5 needs it at finalize (xsd.ExtensionContentType), and
// it carries clause 4.1 with it for its own 4.2.1 arm; exporting a SECOND name
// for a two-line total function, whose one out-of-package caller would be this
// restriction branch, was more surface than the sharing is worth (STYLE T5). The
// two encodings are pinned against each other by this package's content-type
// tests, which run both arms over the same source shapes: a change to clause
// 4.1.1 or 4.1.2 is a change to both sites.
func explicitContentType(effective *xsd.Particle, effectiveMixed bool) xsd.ContentType {
	if effective == nil {
		return xsd.EmptyContent{} // clause 4.1.1
	}
	return xsd.ElementContent{Mixed: effectiveMixed, Particle: *effective} // clause 4.1.2
}

// effectiveContent computes the ·effective content· of a complex content
// (§3.4.2.3.3 clauses 2-3) from parent's model-group child, and reports whether
// the ·explicit content· (clause 2) was ***empty***. parent is the <complexType>
// (implicit content), <restriction> or <extension>.
//
// A nil particle means the ·effective content· itself is ***empty*** (clause
// 3.1.2). The two facts are distinct and both are needed: clause 4.2.2 keys on
// the EFFECTIVE content being empty, while clause 4.2.3.1 keys on the EXPLICIT
// content being empty — they differ exactly when clause 3.1.1 substitutes an
// empty 1..1 sequence for a mixed type with no model-group child, so that text is
// admitted.
//
// This is the single encoding of clauses 2-3: both the restriction branch
// (buildComplexContentType) and the extension branch (complexContentType) read
// their ·effective content· from here.
func (p *producer) effectiveContent(parent *Element, effectiveMixed bool, scopeParent xsd.ElementScopeParent) (effective *xsd.Particle, explicitEmpty bool, err error) {
	explicit, err := p.explicitContent(modelGroupChild(parent), scopeParent)
	if err != nil {
		return nil, false, err
	}
	if explicit != nil {
		return explicit, false, nil // clause 3.2
	}
	if !effectiveMixed {
		return nil, true, nil // clause 3.1.2: ·effective content· is ***empty***
	}
	// clause 3.1.1: a 1..1 particle over an empty sequence stands in, so a mixed
	// type with no model-group child still admits character content.
	part, err := emptySequenceParticle(parent.Loc())
	if err != nil {
		return nil, false, err
	}
	return &part, true, nil
}

// repeatedContentAlternative returns the SECOND <simpleContent>/<complexContent>
// child of a <complexType> in document order, or nil when it carries at most one
// between the two. xs:complexTypeModel (§3.4.2, xmlschema11-1.md:4757) is a plain
// xs:choice of <simpleContent>, <complexContent> and the implicit-content
// sequence, with the maxOccurs="1" the schema for schema documents defaults to,
// so a second one of either is a grammar fault — carrying no rule ID, on the same
// §5.1 footing as a misplaced <openContent> below, since src-ct (§3.4.3) states
// no clause for it and incorporates the schema for schema documents by reference.
//
// Charging it is what keeps produceComplexType's dispatch honest: that dispatch
// reads one wrapper through childElement, a FIRST-match lookup that silently
// drops every later sibling, so without this check a <complexType> carrying two
// of them assembles clean from whichever came first.
//
// The walk is not childElements: the fault spans TWO expanded names, and only a
// single pass over the children orders a <simpleContent> against a
// <complexContent> to name the one that is second (STYLE D2).
func repeatedContentAlternative(ctElem *Element) *Element {
	var first *Element
	for _, child := range ctElem.Children() {
		c, ok := child.(*Element)
		if !ok {
			continue
		}
		if !isXSD(c, "simpleContent") && !isXSD(c, "complexContent") {
			continue
		}
		if first != nil {
			return c
		}
		first = c
	}
	return nil
}

// repeatedDerivationAlternant returns the SECOND <restriction>/<extension> child
// of a <simpleContent> or a <complexContent> in document order, or nil when the
// wrapper carries at most one between the two. xs:simpleContent
// (xmlschema11-1.md:5003) and xs:complexContent (:4895) each hold a plain
// xs:choice of the two, with the maxOccurs="1" the schema for schema documents
// defaults to, so a second one of either is a grammar fault on the same §5.1
// footing as repeatedContentAlternative above: src-ct (§3.4.3) states no clause
// for it and incorporates the schema for schema documents by reference.
//
// Charging it is what keeps both producers' alternant reads honest: they share
// derivationAlternant, whose first-match read would map a malformed wrapper by
// its <restriction> and drop the <extension> in silence. That bias is not
// defensible, so the fault is charged ahead of the read rather than folded into
// it.
//
// The walk is not childElements for the same reason repeatedContentAlternative's
// is not: the fault spans TWO expanded names, and only a single pass over the
// children orders a <restriction> against an <extension> to name the one that is
// second (STYLE D2).
func repeatedDerivationAlternant(wrapper *Element) *Element {
	var first *Element
	for _, child := range wrapper.Children() {
		c, ok := child.(*Element)
		if !ok {
			continue
		}
		if !isXSD(c, "restriction") && !isXSD(c, "extension") {
			continue
		}
		if first != nil {
			return c
		}
		first = c
	}
	return nil
}

// misplacedOpenContent returns the <openContent> element a <complexType> carries
// in a position the schema for schema documents (§3.4.2) does not allow, or nil.
// The type's own <openContent> is legal only in the IMPLICIT content form —
// xs:complexType's content model puts it in the third alternative, beside the
// model group, never beside <simpleContent>/<complexContent> — under
// <complexContent> only as a child of the <restriction>/<extension> alternant,
// since xs:complexContent's content model is (annotation?, (restriction |
// extension)), and NOWHERE under <simpleContent>: that element's content model is
// the same (annotation?, (restriction | extension)) shape (spec L1687), but
// neither of ITS alternants admits an <openContent> (L1692/L1697 — the
// simple-content <restriction> takes facets and attribute declarations, and its
// one wildcard slot is ##other, which the XSD namespace is not).
//
// All three misplacements were previously invisible: the {content type} branches
// read <openContent> from the <complexType> or the complex-content derivation
// alternant only, so a stray one elsewhere was silently dropped and a schema a
// complete processor rejects would have assembled clean. Like <complexContent>
// with neither alternant (produceComplexContent), this is a plain grammar fault,
// not an xsderr rule verdict: src-ct (§3.4.3) states no clause for it and
// incorporates the schema for schema documents' own conditions by reference.
//
// Its two first-match wrapper reads are unambiguous because
// repeatedContentAlternative is charged ahead of it: a <complexType> reaching
// here carries at most one <simpleContent>/<complexContent> between them.
func misplacedOpenContent(ctElem *Element) *Element {
	cc := childElement(ctElem, xsd.XMLSchemaNS, "complexContent")
	sc := childElement(ctElem, xsd.XMLSchemaNS, "simpleContent")
	if cc == nil && sc == nil {
		return nil // the implicit form: an <openContent> child here is in its legal position
	}
	if own := childElement(ctElem, xsd.XMLSchemaNS, "openContent"); own != nil {
		return own
	}
	if cc != nil {
		return childElement(cc, xsd.XMLSchemaNS, "openContent")
	}
	return simpleContentOpenContent(sc)
}

// simpleContentOpenContent returns the <openContent> a <simpleContent> carries
// either directly or under its <restriction>/<extension> alternant, or nil — every
// position inside a <simpleContent> subtree is illegal (see misplacedOpenContent).
// The alternants are searched restriction-first, so a malformed source carrying
// both is reported at the same position every run (STYLE D1) — this check runs on
// produceComplexType's entry path, ahead of the produceSimpleContent dispatch
// that charges repeatedDerivationAlternant, so a <simpleContent> carrying both
// alternants does reach it.
func simpleContentOpenContent(sc *Element) *Element {
	if oc := childElement(sc, xsd.XMLSchemaNS, "openContent"); oc != nil {
		return oc
	}
	for _, alternant := range [...]string{"restriction", "extension"} {
		alt := childElement(sc, xsd.XMLSchemaNS, alternant)
		if alt == nil {
			continue
		}
		if oc := childElement(alt, xsd.XMLSchemaNS, "openContent"); oc != nil {
			return oc
		}
	}
	return nil
}

// openContentType applies §3.4.2.3.3 (dcl.ctd.ctcc.common) clauses 5 and 6 to an
// already-computed ·explicit content type·, yielding the complex type's final
// {content type}. owner is the element whose <openContent> child clause 5.1
// looks for: the <complexType> itself (implicit content) or the <restriction>/
// <extension> alternant. It is the single encoding of the Open Content fold, and
// every {content type} branch ends in it.
//
// src-ct (§3.4.3) clauses 3-4 are charged FIRST, before clause 5 selects
// anything: they are Schema Representation Constraints on the source
// <openContent> element, so they hold whether or not the ·wildcard element·
// turns out to matter — in particular for a mode="none" element, which clause
// 6.1 otherwise discards unexamined.
func (p *producer) openContentType(owner *Element, explicit xsd.ContentType) (xsd.ContentType, error) {
	if err := checkOpenContentAny(owner); err != nil {
		return nil, err
	}
	we, err := p.wildcardElement(owner, explicit)
	if err != nil {
		return nil, err
	}
	if we == nil {
		return explicit, nil // clause 6.1: the ·wildcard element· is ·absent·
	}
	oc, err := p.openContentOf(we, explicit)
	if err != nil {
		return nil, err
	}
	if oc == nil {
		return explicit, nil // clause 6.1: the ·wildcard element· has mode="none"
	}
	return wrapOpenContent(we.Loc(), explicit, *oc)
}

// checkOpenContentAny enforces src-ct (§3.4.3) clauses 3 and 4 on owner's own
// <openContent> child: an <any> is required when mode is not "none" (clause 3,
// since clause 6.2 has no {wildcard} to build without it) and forbidden when it
// is (clause 4, since clause 6.1 would ignore it). The schema-level
// <defaultOpenContent> is NOT governed by these clauses — src-ct is a constraint
// on <complexType> — so its own mandatory <any> is checked as a grammar fault by
// checkDefaultOpenContent instead, eagerly, before anything is produced.
//
// It then orders the <openContent>'s children against s4sOpenContent
// (checkS4SChildOrder, #1951), behind the two clauses rather than ahead of them:
// the exception that walk's doc records.
func checkOpenContentAny(owner *Element) error {
	oc := childElement(owner, xsd.XMLSchemaNS, "openContent")
	if oc == nil {
		return nil
	}
	none := openContentModeIsNone(oc)
	hasAny := childElement(oc, xsd.XMLSchemaNS, "any") != nil
	if none && hasAny {
		return xsderr.New(ruleSrcCT, oc.Loc(),
			`<openContent mode="none"> has an <any> child, but src-ct clause 4 forbids one`)
	}
	if !none && !hasAny {
		return xsderr.New(ruleSrcCT, oc.Loc(),
			`<openContent> whose mode is not "none" has no <any> child, but src-ct clause 3 requires one`)
	}
	return checkS4SChildOrder(oc, s4sOpenContent)
}

// wildcardElement selects §3.4.2.3.3 clause 5's ·wildcard element·, returning nil
// for clause 5.3 (·absent·): owner's own <openContent> child when present (clause
// 5.1 — presence alone decides, so an <openContent mode="none"> is how a type
// opts OUT of the document's default), otherwise this document's
// <defaultOpenContent> when the ·explicit content type· is not empty (5.2.1) or
// is empty and that element carries appliesToEmpty="true" (5.2.2).
//
// appliesToEmpty is read here and nowhere else: §3.4.1's Open Content record has
// no such property, so it must not travel past this selection (STYLE D3). It is
// an xs:boolean like any other schema attribute, so a lexical outside
// booleanRep is charged rather than read as false — which is why the selection
// returns an error at all.
func (p *producer) wildcardElement(owner *Element, explicit xsd.ContentType) (*Element, error) {
	if own := childElement(owner, xsd.XMLSchemaNS, "openContent"); own != nil {
		return own, nil // clause 5.1
	}
	def := p.defaultOpenContentElem()
	if def == nil {
		return nil, nil // clause 5.3: this document declares no default
	}
	if explicit.Variety() != xsd.ContentEmpty {
		return def, nil // clause 5.2.1
	}
	appliesToEmpty, _, err := boolAttr(def, "appliesToEmpty")
	if err != nil {
		return nil, err
	}
	if appliesToEmpty {
		return def, nil // clause 5.2.2
	}
	return nil, nil // clause 5.3
}

// defaultOpenContentElem returns the <defaultOpenContent> child of THIS
// document's <schema> (§3.4.2.3.3 clause 5.2's "the <schema> ancestor"), or nil
// when it declares none. The <schema> is read off p.schemaElem, never by walking
// Element.parent from the complex type: under ·override pre-processing· a
// substituted declaration is a child of the OVERRIDING document's <override>,
// so an ancestor walk would climb into that document and read ITS default,
// whereas §4.2.5 makes the substituted declaration a top-level declaration of
// the OVERRIDDEN document and produced by its producer (override.go, PRINCIPLES
// 16) — the same reading targetNamespace and the *FormDefault attributes already
// take. The two coincide for every non-override document, which is exactly why
// the wrong one is invisible without an override fixture.
//
// The <schema> content model admits at most one <defaultOpenContent>; a
// malformed document with several is mapped by its first, so the verdict is the
// same every run (STYLE D1). This is a PURE LOOKUP with no rejection path of its
// own: checkDefaultOpenContent has already judged the element's grammar for
// every document of the assembly by the time any complex type consults it, which
// is what keeps that verdict independent of whether any type reaches clause 5.2.
func (p *producer) defaultOpenContentElem() *Element {
	return childElement(p.schemaElem, xsd.XMLSchemaNS, "defaultOpenContent")
}

// checkDefaultOpenContent enforces the schema for schema documents grammar on
// this document's <defaultOpenContent>: its content model (annotation?, any)
// makes the <any> child mandatory, its mode attribute is restricted to
// (interleave | suffix) — "none" is an <openContent> mode alone — and the
// children present are ordered against s4sDefaultOpenContent (checkS4SChildOrder,
// #1951), in that order, so the first two keep their verdict on a document that
// also breaks the third.
//
// All three are plain grammar faults rather than xsderr rule verdicts, because no
// Schema Component Constraint governs <defaultOpenContent> at all: it is pure
// schema-document-level source grammar, consumed by §3.4.2.3.3
// (dcl.ctd.ctcc.common) clause 5.2's mapping, while ct-props-correct (§3.4.6.1)
// clause 1 is scoped to a Complex Type Definition's §3.4.1 property tableau —
// and no such component is in view here — and src-ct (§3.4.3) clauses 3-4 are
// constraints on <complexType>. The mode enumeration is therefore NOT routed
// through openContentModeOf, whose out-of-enumeration charge is ct-props-correct
// and whose job is to yield a {mode} value this check has no use for.
//
// Running it once per document, ahead of production, is what makes the verdict
// content-INDEPENDENT: a malformed <defaultOpenContent> is rejected because it
// is malformed, not because some complex type of the same document happened to
// have an ·explicit content type· that reaches clause 5.2 and consults it.
//
// It is called beside the pre-scan, for every document of the assembly, rather
// than from run: a base= in the document produced FIRST builds its base type
// through the DECLARING document's producer on demand (symbols.typeSource), so a
// per-run check would let a neighbour document's still-unjudged default be
// selected by clause 5.2 first — and a childless one would then reach
// openContentOf's panic instead of any verdict at all.
func (p *producer) checkDefaultOpenContent() error {
	def := p.defaultOpenContentElem()
	if def == nil {
		return nil
	}
	if childElement(def, xsd.XMLSchemaNS, "any") == nil {
		return fmt.Errorf("parser: <defaultOpenContent> at %s has no <any> child, but the schema for schema documents makes it mandatory: xs:defaultOpenContent's content model is (annotation?, any), whose <any> carries no minOccurs of its own — unlike xs:openContent's, which is minOccurs=\"0\"", def.Loc())
	}
	mode, present := def.Attr("mode")
	if present && !slices.Contains([]string{"interleave", "suffix"}, collapseTrim(mode)) {
		return fmt.Errorf(`parser: <defaultOpenContent> at %s has mode=%q, but the schema for schema documents admits only interleave or suffix there: xs:defaultOpenContent restricts mode to that two-value enumeration, and "none" belongs to xs:openContent's alone`, def.Loc(), mode)
	}
	return checkS4SChildOrder(def, s4sDefaultOpenContent)
}

// checkFormDefaults charges cvc-datatype-valid on this document's <schema>
// element for an attributeFormDefault or elementFormDefault outside
// xs:formChoice's qualified/unqualified enumeration.
//
// Running it once per document, ahead of production, is what makes the verdict
// content-INDEPENDENT — the argument checkDefaultOpenContent's doc states, and
// the reason this charge is NOT made where the value is read. localTargetNS
// consults a *FormDefault only for a local declaration that carries neither
// targetNamespace nor form=, so a document writing elementFormDefault="Qualified"
// and declaring no such element would never be charged from there at all, while
// the same document with one local element would be: the malformed attribute is a
// fault of the document, not of whichever declaration happened to consult it.
// form= and use= keep the opposite treatment for the opposite reason — every one
// a document writes is read by the declaration carrying it.
//
// It is called beside checkDefaultOpenContent, for every document of the
// assembly, so a document produced on demand through symbols.typeSource cannot
// mint a local name off a *FormDefault this check has not judged.
func (p *producer) checkFormDefaults() error {
	// A slice, not a map: the two attributes are judged in one order every run
	// (STYLE D1/D2), so a <schema> writing both malformed values names the same
	// one every time.
	for _, attr := range []string{"attributeFormDefault", "elementFormDefault"} {
		a, ok := p.schemaElem.attr(attr)
		if !ok {
			continue
		}
		if _, err := formChoiceQualified(a.Value(), a.Loc(), attr); err != nil {
			return err
		}
	}
	return nil
}

// openContentOf computes §3.4.2.3.3 clause 6's {open content} from the
// ·wildcard element· we, returning nil for clause 6.1 — a mode="none" element
// contributes NO Open Content record, which is why xsd.OpenContentMode has no
// third member (xsd/closedsets.go) and why the "none" token dies here rather
// than travelling as a mode.
//
// {mode} is the mode attribute's ·actual value·, defaulting to interleave, and
// {wildcard} is openContentWildcard's clause-6.2 combination.
func (p *producer) openContentOf(we *Element, explicit xsd.ContentType) (*xsd.OpenContent, error) {
	if openContentModeIsNone(we) {
		return nil, nil // clause 6.1
	}
	mode, err := openContentModeOf(we)
	if err != nil {
		return nil, err
	}
	anyElem := childElement(we, xsd.XMLSchemaNS, "any")
	if anyElem == nil {
		// Unreachable: a mode≠"none" <openContent> without an <any> was rejected by
		// checkOpenContentAny (src-ct clause 3) and a <defaultOpenContent> without
		// one by checkDefaultOpenContent before ANY document of the assembly was
		// produced, and those are the only two elements clause 5 can select.
		// Panicking names the broken invariant rather than fabricating a verdict for
		// a source shape that cannot reach here.
		panic("parser: openContentOf: ·wildcard element· with mode other than none has no <any> child")
	}
	w, err := p.produceWildcard(anyElem)
	if err != nil {
		return nil, err
	}
	wildcard, err := openContentWildcard(we.Loc(), w, explicit)
	if err != nil {
		return nil, err
	}
	oc, err := xsd.NewOpenContent(we.Loc(), mode, wildcard)
	if err != nil {
		return nil, err
	}
	return &oc, nil
}

// openContentWildcard computes clause 6.2's {wildcard}: the wildcard W
// corresponding to the ·wildcard element·'s <any> child when the ·explicit
// content type· carries no {open content}, and otherwise a wildcard whose
// {process contents} is W's and whose {namespace constraint} is the §3.10.6.3
// wildcard union (xsd.UnionNamespaceConstraint) of W's with
// that of ·explicit content type·.{open content}.{wildcard}.
//
// The second arm is live for an <extension> whose base already has an Open
// Content: §3.4.2.3.3 clause 4.2.2/4.2.3 hand the base's {open content} through
// into the ·explicit content type· (xsd.ExtensionContentType), and this derivation's
// own <openContent> then widens it rather than replacing it.
func openContentWildcard(loc xsderr.Loc, w xsd.Wildcard, explicit xsd.ContentType) (xsd.Wildcard, error) {
	ec, ok := explicit.(xsd.ElementContent)
	if !ok || ec.OpenContent == nil {
		return w, nil
	}
	unioned, err := xsd.UnionNamespaceConstraint(loc, w.NamespaceConstraint(), ec.OpenContent.Wildcard().NamespaceConstraint())
	if err != nil {
		return xsd.Wildcard{}, err
	}
	return xsd.NewWildcard(loc, unioned, w.ProcessContents())
}

// wrapOpenContent folds oc into the ·explicit content type· per §3.4.2.3.3
// clause 6.2's {variety}/{particle} half: an element-only or mixed explicit
// content keeps its {variety} and {particle} and merely gains the {open
// content}, while an ***empty*** one becomes ELEMENT-ONLY over the clause's
// synthesized empty-sequence particle, so the Open Content has a content model
// to interleave with or suffix. That element-only is unconditional — clause 6.2
// names it outright, with no reading of ·effective mixed· — because the empty
// ·explicit content type· that reaches here already answered the mixed question
// by being empty (clause 4.1.1).
func wrapOpenContent(loc xsderr.Loc, explicit xsd.ContentType, oc xsd.OpenContent) (xsd.ContentType, error) {
	switch e := explicit.(type) {
	case xsd.EmptyContent:
		particle, err := emptySequenceParticle(loc)
		if err != nil {
			return nil, err
		}
		return xsd.ElementContent{Mixed: false, Particle: particle, OpenContent: &oc}, nil
	case xsd.ElementContent:
		e.OpenContent = &oc
		return e, nil
	default:
		// xsd.SimpleContent, the only other member, is unreachable twice over:
		// {open content} is a property of ElementContent alone
		// (xsd/complextype.go), and no §3.4.2.3.3 branch feeding this function can
		// yield simple content — the <simpleContent> forms never reach it.
		// Panicking on the broken sealed sum matches xsd.ExtensionContentType;
		// charging a rule the type system already makes impossible would be a
		// fabricated verdict.
		panic("parser: wrapOpenContent: non-exhaustive ContentType switch")
	}
}

// openContentModeIsNone reports whether an <openContent>/<defaultOpenContent>
// element carries mode="none" — the ·actual value· of an xs:NMTOKEN-derived
// enumeration, so surrounding whitespace is collapsed away before the compare.
func openContentModeIsNone(we *Element) bool {
	return collapseTrim(attrOr(we, "mode")) == "none"
}

// openContentModeOf maps a ·wildcard element·'s mode attribute to the {mode} of
// clause 6.2's Open Content: interleave when absent (the schema for schema
// documents' default) or written out, suffix when written out. "none" never
// reaches here — openContentOf returns clause 6.1 before calling — so an
// out-of-enumeration token is what is left, charged to ct-props-correct clause 1
// (§3.4.6.1), the §3.4.1 tableau the {mode} property belongs to, exactly as
// xsd.NewOpenContent charges a mode it cannot represent.
//
// That charge is reachable for a type's OWN <openContent> alone, which is where
// a Complex Type Definition is in view to charge it against. The other
// ·wildcard element· clause 5 can select, the schema-level <defaultOpenContent>,
// still passes through here — but only ever on the interleave/suffix arms, its
// whole enumeration having been settled by checkDefaultOpenContent as a grammar
// fault, since ct-props-correct reaches no component there.
func openContentModeOf(we *Element) (xsd.OpenContentMode, error) {
	mode, ok := we.Attr("mode")
	if !ok {
		return xsd.OpenContentInterleave, nil
	}
	switch collapseTrim(mode) {
	case "interleave":
		return xsd.OpenContentInterleave, nil
	case "suffix":
		return xsd.OpenContentSuffix, nil
	}
	return 0, xsderr.New(ruleCTPropsCorr, we.Loc(),
		"open content mode %q is not one of none/interleave/suffix (ct-props-correct clause 1)", mode)
}

// emptySequenceParticle builds the 1..1 particle over an empty sequence model
// group that §3.4.2.3.3 names twice in identical words: clause 3.1.1's ·effective
// content· for a mixed type with no model-group child, and clause 6.2's
// {particle} when an ***empty*** ·explicit content type· has to carry an Open
// Content. One encoding for both (STYLE T4).
func emptySequenceParticle(loc xsderr.Loc) (xsd.Particle, error) {
	seq, err := xsd.NewModelGroup(loc, xsd.CompositorSequence, nil)
	if err != nil {
		return xsd.Particle{}, err
	}
	oneOne, err := xsd.NewOccurs(loc, 1, 1)
	if err != nil {
		return xsd.Particle{}, err
	}
	return xsd.NewParticle(loc, oneOne, xsd.ResolvedTerm{Term: seq})
}

// explicitContent maps the model-group child to the {explicit content} particle
// (§3.4.2.3.3 clause 2), returning nil for the ***empty*** cases: no group child
// (2.1.1), an empty <all>/<sequence> (2.1.2), a childless <choice minOccurs="0">
// (2.1.3), or a group child with maxOccurs="0" (2.1.4).
//
// Clause 2 decides what the model-group child maps TO, and nothing more: §5.1's
// first bullet binds the child and its whole subtree to the Schema for Schema
// Documents whatever they map to. So 2.1.4's arm runs the WHOLE clause-2.2
// mapping and discards the particle it returns (#883) rather than returning
// ahead of it — and since §3.7.2/§3.8.2 map the child itself to a Particle
// unless minOccurs is 0 as well, a maxOccurs="0" child with the default
// minOccurs of 1 earns the p-props-correct verdict here that the identical child
// already earned one level down.
//
// Clause 2.1 being a disjunction, that mapping is what orders the arms: 2.1.4 is
// tested FIRST, because an EMPTY <all>/<sequence> satisfies 2.1.2 as well and
// answering there would return before any Particle was built to charge (#901).
// Every arm makes the ·explicit content· ***empty*** either way.
//
// 2.1.2 and 2.1.3 have no subtree to walk, so an EMPTY <all> is the one shape
// that still needs its occurrence grammar charged ahead of them: allOccursGrammar
// runs before every elision test, since the {0,1} enumeration decides whether the
// <all> element is well-formed at all.
//
// scopeParent is passed through to every local element declaration built beneath
// this content model (§3.3.2.3 dcl.elt.local). It is generic rather than a
// ComplexTypeScopeParent because this function and the ones it calls are shared
// with the named-<group> tree, where the same elements are scoped to a Model
// Group Definition instead.
func (p *producer) explicitContent(group *Element, scopeParent xsd.ElementScopeParent) (*xsd.Particle, error) {
	if group == nil {
		return nil, nil // 2.1.1
	}
	local := group.Name().Local()
	if local == "all" {
		// Before any clause-2 elision: an <all> whose occurrence attributes are
		// outside the {0,1} enumeration is invalid however it maps.
		if err := allOccursGrammar(group); err != nil {
			return nil, err
		}
	}
	if maxOccursZero(group) {
		if _, err := p.modelGroupChildParticle(group, scopeParent); err != nil {
			return nil, err
		}
		return nil, nil // 2.1.4
	}
	hasChildren := hasParticleChild(group)
	if (local == "all" || local == "sequence") && !hasChildren {
		return nil, nil // 2.1.2
	}
	if local == "choice" && !hasChildren && minOccursZero(group) {
		return nil, nil // 2.1.3
	}
	return p.modelGroupChildParticle(group, scopeParent) // 2.2
}

// modelGroupChildParticle maps the model-group child of a
// <complexType>/<restriction>/<extension> by its form: a <group> is always a
// reference in that position (§3.7.2), an <all>/<choice>/<sequence> a model
// group (§3.8.2).
func (p *producer) modelGroupChildParticle(group *Element, scopeParent xsd.ElementScopeParent) (*xsd.Particle, error) {
	if group.Name().Local() == "group" {
		return p.produceGroupRefParticle(group)
	}
	return p.produceGroupParticle(group, scopeParent)
}

// produceGroupParticle maps an <all>/<choice>/<sequence> element to a Particle
// wrapping a Model Group (§3.8.2), with {particles} in document order. A
// minOccurs=maxOccurs=0 group maps to no component at all (§3.8.2) —
// produceGroupParticle returns (nil, nil) — so the caller omits it. The children
// are walked all the same: §5.1's first bullet binds every element information
// item in the schema document to the Schema for Schema Documents whatever it
// maps to, so the grammar faults groupParticles charges answer inside a subtree
// that contributes no component too (#883). That walk runs through
// discardingComponents and its particles are thrown away, since a subtree
// mapping to no component may register none either — see produceElementParticle
// for the one registration a content model reaches.
//
// occursOf therefore answers FIRST, since its elided verdict decides whether the
// walk may contribute components at all — but the verdict it charges on this
// element's own occurrence range (p-props-correct for min greater than max,
// cvc-datatype-valid for a lexical outside the declared type) is held back until
// after the walk, so a Schema for Schema Documents fault inside the subtree
// answers ahead of anything charged on the particle wrapping it.
//
// The grammar's own {0,1} occurrence restriction on <all> is charged here rather
// than only for the top model-group child explicitContent maps, on the same
// footing: a nested <all maxOccurs="2"> is an invalid <all> element wherever it
// sits, and leaving it to cos-all-limited at finalize both named a different
// rule and answered not at all once an ancestor was elided.
//
// WHERE the resulting model group may appear is not decided here at all:
// cos-all-limited (§3.8.6.2) is a Schema Component Constraint over
// {term}/{particles}, and §3.7.2 xr.mgd3 makes a <group ref> particle's {term}
// the referenced definition's {model group}, so a usage site's compositor is
// invisible until references resolve. xsd's finalize pass charges it
// (xsd/allgrouplimited.go, #469).
//
// scopeParent passes straight through: a model group is not a scope boundary
// (§3.3.2.3 names only <complexType> and the named <group> as ancestors that
// determine {scope}.{parent}).
func (p *producer) produceGroupParticle(group *Element, scopeParent xsd.ElementScopeParent) (*xsd.Particle, error) {
	local := group.Name().Local()
	compositor, ok := compositorOf(local)
	if !ok {
		// A <group> reference is mapped by produceGroupRefParticle before it
		// reaches here; any other name is an unexpected model-group child.
		return nil, fmt.Errorf("parser: model group child <%s> is not a compositor (all/choice/sequence)", local)
	}
	if local == "all" {
		if err := allOccursGrammar(group); err != nil {
			return nil, err
		}
	}
	occ, elided, occErr := occursOf(group)
	if elided {
		_, err := p.discardingComponents().groupParticles(group, scopeParent)
		return nil, err
	}
	particles, err := p.groupParticles(group, scopeParent)
	if err != nil {
		return nil, err
	}
	if occErr != nil {
		return nil, occErr
	}
	mg, err := xsd.NewModelGroup(group.Loc(), compositor, particles)
	if err != nil {
		return nil, err
	}
	part, err := xsd.NewParticle(group.Loc(), occ, xsd.ResolvedTerm{Term: mg})
	if err != nil {
		return nil, err
	}
	return &part, nil
}

// produceGroupRefParticle maps a <group ref> to a Particle whose {term} is a
// deferred ModelGroupRef (§3.7.2, xr.mgd3), mirroring produceElementParticle's
// <element ref> → ElementDeclarationRef mapping. A minOccurs=maxOccurs=0 <group
// ref> maps to no component at all (returns nil, §3.7.2) — but only after the
// two grammar faults below are charged, which §5.1's first bullet binds whatever
// the element maps to (#883). The reference is RETAINED, never rewritten to the
// {term} it denotes: resolution and the no-circular-groups check happen at
// finalize (#173: src-resolve clause 1.5, mg-props-correct clause 2), and
// neither VERDICT is ever duplicated here. Occurs-range correctness
// (p-props-correct §3.9.6.1 clause 2.1) is enforced by occursOf.
//
// One mapping rule nonetheless has to LOOK through a reference produced here:
// §3.4.2.3.3 clause 4.2.3 selects a sub-case by the {compositor} of the
// ·base particle·'s {term}, and that choice fixes a {content type} synthesized at
// produce time. xsd.ExtensionContentType reads it through the resolveModelGroup
// callback this producer passes it — a read, which leaves this particle untouched
// and charges nothing.
func (p *producer) produceGroupRefParticle(el *Element) (*xsd.Particle, error) {
	if err := rejectProhibitedRefAttrs(el); err != nil {
		return nil, err
	}
	ref, ok := el.Attr("ref")
	if !ok {
		// A <group> in a content model is always a reference (§3.8.2: the named
		// definition form appears only as a top-level <schema> child). The absence
		// has no dedicated SCC (§3.7.3 "None as such", Q6), so it is a plain
		// well-formedness fault, not an xsderr rule — the same footing as the
		// prohibited name rejectProhibitedRefAttrs charges just above.
		return nil, fmt.Errorf("parser: a <group> in a content model must be a reference (carry a ref attribute), but none is present")
	}
	occ, elided, err := occursOf(el)
	if err != nil {
		return nil, err
	}
	if elided {
		return nil, nil
	}
	qn, err := p.resolveQName(el, ref, "ref")
	if err != nil {
		return nil, err
	}
	if src, redefining := p.redefinedGroupOriginal(el, qn); redefining {
		// src-expredef clause 2: this is a redefining <group>'s licensed
		// self-reference, so "a component which corresponds to the top-level
		// definition item of that name and the appropriate kind in S2 is used" —
		// the ORIGINAL, which the visible name no longer reaches. The reference
		// cannot stay deferred, since resolving it by name at finalize would land on
		// the redefinition and read as a circular <group ref> graph
		// (mg-props-correct clause 2). It is resolved HERE instead, to exactly what
		// §3.7.2 xr.mgd3 says a <group ref> particle's {term} is — "the {model
		// group} of the model group definition ·resolved· to by the ·actual value·
		// of the ref attribute" — built under the REDEFINED document's producer so
		// its local declarations keep their own document's namespace and defaults.
		mg, err := src.owner.buildDefinitionModelGroup(src.elem, xsd.ModelGroupScopeParent{Name: qn})
		if err != nil {
			return nil, err
		}
		part, err := xsd.NewParticle(el.Loc(), occ, xsd.ResolvedTerm{Term: mg})
		if err != nil {
			return nil, err
		}
		return &part, nil
	}
	part, err := xsd.NewParticle(el.Loc(), occ, xsd.ModelGroupRef{Name: qn})
	if err != nil {
		return nil, err
	}
	return &part, nil
}

// produceModelGroupDefinition maps a top-level named <group> (§3.7.2, xr.mgd1)
// into a Model Group Definition. Every route that CONTRIBUTES a component
// reaches it through buildModelGroupDefinition, which memoizes it so one <group>
// element is mapped exactly once however many demand-driven lookups reach it;
// redefinedGroupRestricted calls it directly, for a value that reaches no
// builder. The named form has exactly one <all>/<choice>/<sequence> child, whose
// Model Group becomes {model group}; any other body, and any other child beside
// it, is rejected as the s4s-grammar fault it is, by buildDefinitionModelGroup,
// before a component is built. Occurrence on the child is irrelevant here — a
// model group definition carries no {min occurs}/{max occurs} (§3.7.2 note);
// those live solely on a <group ref> particle.
//
// name is also the {scope}.{parent} of every local element declaration in the
// body: §3.3.2.3 dcl.elt.local's "otherwise" branch — an <element> within a
// named <group> rather than under a <complexType> — scopes it to the Model Group
// Definition, not to whatever complex type later references the group.
//
// A missing name is rejected FIRST, before the body is built, for the reason
// produceComplexType gives: the schema for schema documents makes name
// use="required" with type xs:NCName on xs:namedGroup, and §3.7.3 states "None
// as such" for <group>'s Schema Representation Constraints, so the fault carries
// no rule ID — while deferring it would let a nameless <group> be judged by
// whether its body happens to hold a local element. Since #305 run reaches this
// only through topLevelName, which raises the identical fault a step earlier;
// what remains guarded here is the <redefine> path — produceRedefinition
// (redefine.go) mints the redefining declaration's name from the <redefine>
// child's own name attribute and never consults topLevelName, so a
// <group name=""> child of a <redefine> whose redefined document declares the
// same nameless <group> (recorded as the original, hence past src-expredef's
// closing requirement) arrives here with an empty local part — plus direct
// programmatic calls. resolveModelGroup's on-demand build does NOT reach it: its
// only caller is xsd.ExtensionContentType's group-lookup callback, reading a
// ModelGroupRef that only produceGroupRefParticle mints, and xsd.NewParticle
// rejects an empty ModelGroupRef local part as a component-invariant before any
// resolution happens — a backstop produceGroupRefParticle no longer reaches
// since #343, whose bindQName rejects the <group ref=""> lexical at the
// attribute (cvc-datatype-valid) before a ModelGroupRef is minted at all.
func (p *producer) produceModelGroupDefinition(name xsd.QName, el *Element) (xsd.ModelGroupDefinition, error) {
	if name.Local == "" {
		return xsd.ModelGroupDefinition{}, fmt.Errorf("parser: top-level <group> at %s has no usable name: its name attribute is absent or empty, and the schema for schema documents requires one — xs:namedGroup declares name use=\"required\" with type xs:NCName", el.Loc())
	}
	mg, err := p.buildDefinitionModelGroup(el, xsd.ModelGroupScopeParent{Name: name})
	if err != nil {
		return xsd.ModelGroupDefinition{}, err
	}
	return xsd.NewModelGroupDefinition(el.Loc(), name, mg)
}

// buildDefinitionModelGroup builds the {model group} of a top-level <group>
// definition from its single <all>/<choice>/<sequence> child (§3.7.2, xr.mgd1):
// the Model Group term itself, not a Particle (a definition carries no
// occurrence). Any other body is rejected by rejectNamedGroupBody, at the child
// that is not admitted rather than one phase later at the definition. A body
// carrying TWO compositors is the other end of the same cardinality and the same
// fault, charged here at the second (repeatedCompositorChild) once the first has
// been read, so the message can name the sibling it collides with. With one body
// found, checkS4SChildOrder walks the definition against s4sNamedGroup, charging
// any other XSD-namespace child — a <unique> or an <element> beside the body,
// before it or after it — on §5.1's first bullet and no rule ID (#1876). Only
// that admission fault is left for the walk to find: the two cardinality faults
// are answered above, and a late <annotation> by rejectLateAnnotation before any
// producer runs.
//
// The rejection belongs HERE and not at produceModelGroupDefinition, which is not
// the only caller: src-expredef clause 2 builds the ORIGINAL a redefining
// <group>'s self-reference resolves to through this function alone, and a zero
// ModelGroup on that path reaches xsd.NewParticle — which admits it, its {term}
// being a non-nil ResolvedTerm — and then the content automaton's Compositor
// switch, over a compositor no <all>/<choice>/<sequence> ever set.
//
// cos-all-limited is not charged here: an <all> definition body is legal outright
// (§3.8.6.2 clause 1.1), and the constraint's usage-site half is a component-shape
// verdict finalize owns (xsd/allgrouplimited.go). scopeParent is the enclosing
// definition, threaded to the local element declarations in the body (§3.3.2.3
// dcl.elt.local).
//
// Occurrence attributes on that child are rejected by rejectNamedGroupBodyOccurs
// BEFORE the body is descended into, so a prohibited minOccurs/maxOccurs is
// reported ahead of any fault among the particles it encloses.
func (p *producer) buildDefinitionModelGroup(el *Element, scopeParent xsd.ElementScopeParent) (xsd.ModelGroup, error) {
	group := compositorChild(el)
	if group == nil {
		return xsd.ModelGroup{}, rejectNamedGroupBody(el)
	}
	if dup := repeatedCompositorChild(el); dup != nil {
		return xsd.ModelGroup{}, fmt.Errorf("parser: <%s> at %s is a second <all>, <choice> or <sequence> in the body of the named <group> at %s, which already carries the <%s> at %s: xs:namedGroup's content model (xmlschema11-1.md:5187) is (annotation?, (all | choice | sequence)), whose inner choice is minOccurs=\"1\" maxOccurs=\"1\" and so admits exactly one", dup.Name().Local(), dup.Loc(), el.Loc(), group.Name().Local(), group.Loc())
	}
	if err := checkS4SChildOrder(el, s4sNamedGroup); err != nil {
		return xsd.ModelGroup{}, err
	}
	if err := rejectNamedGroupBodyOccurs(group, el); err != nil {
		return xsd.ModelGroup{}, err
	}
	compositor, _ := compositorOf(group.Name().Local()) // compositorChild guarantees ok
	particles, err := p.groupParticles(group, scopeParent)
	if err != nil {
		return xsd.ModelGroup{}, err
	}
	return xsd.NewModelGroup(group.Loc(), compositor, particles)
}

// rejectNamedGroupBody rejects a named <group> whose body is not the one
// <all>/<choice>/<sequence> the schema for schema documents requires of it. Call
// it only once compositorChild has answered nil, which is the whole set of bodies
// this describes: a nested <group> reference, a bare <element>, an
// annotation-only body, an empty one. A body with more than one compositor fails
// the same cardinality from the other side and is charged by
// repeatedCompositorChild's caller, on this citation.
//
// It reports at the first child xs:namedGroup does not admit, and only at the
// <group> itself when there is none, because an unadmitted child IS the mistake
// while an unfilled required position is all that is left to say when the
// document wrote nothing. That is rejectProhibitedRefAttrs's ordering read from
// the same side: report the mistake, not its consequence. A <group ref= name=>
// body is therefore charged here and not for its prohibited name — the reference
// form is not admitted in this position under any spelling.
//
// The fault carries NO numbered rule ID: §3.7.3 (xmlschema11-1.md:2286) reads
// "None as such." in full, there is no src-mgd, and mgd-props-correct (§3.7.6,
// :2302) is a Schema Component Constraint over an already-built tableau, one
// phase later and one element up from this document fault. What binds is §2.4
// clause 1, sd-valid (:615) — the s4s-grammar class, a plain error naming the
// offending item, its location and the Appendix A production (xsderr/doc.go,
// #966).
//
// Foreign-namespace children are skipped rather than charged here: xs:namedGroup
// admits none of them either, but this check answers only for the body's place.
// Beside a body, the s4sNamedGroup walk charges one as a child no position
// admits (#1982); with no body, the missing body is the fault charged.
func rejectNamedGroupBody(el *Element) error {
	for _, child := range el.Children() {
		c, ok := child.(*Element)
		if !ok || c.Name().Space() != xsd.XMLSchemaNS || c.Name().Local() == "annotation" {
			continue
		}
		return fmt.Errorf("parser: <%s> at %s is not admitted in the body of the named <group> at %s: xs:namedGroup's content model (xmlschema11-1.md:5187) is (annotation?, (all | choice | sequence)), which admits exactly one <all>, <choice> or <sequence> and no other element", c.Name().Local(), c.Loc(), el.Loc())
	}
	return fmt.Errorf("parser: the named <group> at %s has no <all>, <choice> or <sequence> child: xs:namedGroup's content model (xmlschema11-1.md:5187) is (annotation?, (all | choice | sequence)), whose inner choice is minOccurs=\"1\" maxOccurs=\"1\" and so requires exactly one", el.Loc())
}

// rejectNamedGroupBodyOccurs rejects the <all>/<choice>/<sequence> body of a
// named <group> carrying minOccurs or maxOccurs, which the schema for schema
// documents prohibits on it. xs:namedGroup writes its <all> arm as an INLINE
// xs:complexType restricting xs:all, and that restriction declares both
// attributes use="prohibited" (xmlschema11-1.md:5198-:5199); its <choice> and
// <sequence> arms are typed xs:simpleExplicitGroup (:5205-:5206), which declares
// the same two prohibitions (:5253-:5254). Neither attribute survives to that
// position: the base chain supplies them through xs:group's reference to the
// xs:occurs attribute group (:5167) and each restriction removes them again.
//
// The fault is PRESENCE, not value. use="prohibited" excludes the attribute
// however it is spelled, so minOccurs="1" — the default xs:occurs declares, a
// semantic no-op — is as much a fault as maxOccurs="2". That is what separates
// this guard from allOccursGrammar, whose {0,1} enumeration admits both of those
// and which guards the LOCAL <all> alone.
//
// The fault carries NO numbered rule ID, on rejectProhibitedAttrs's footing for
// the same prohibition one element up: §3.7.3 (xmlschema11-1.md:2286) reads
// "None as such." in full and there is no src-mgd, so what binds is §5.1 (:4296)
// directly — a schema document must be fully valid with respect to the Schema
// for Schema Documents. A plain error, never an xsderr.Rule (STYLE E2).
//
// The two attributes are checked in the grammar's own declaration order, so a
// body writing both is always reported at minOccurs (STYLE D2).
//
// A LOCAL compositor is never reached: this runs from buildDefinitionModelGroup
// alone, and there xs:explicitGroup keeps both attributes, narrowed for the <all>
// spelling by allOccursGrammar and by nothing else.
func rejectNamedGroupBodyOccurs(group, decl *Element) error {
	local := group.Name().Local()
	// compositorChild guarantees one of the three, the same guarantee
	// buildDefinitionModelGroup's compositorOf read relies on, so the <choice> and
	// <sequence> arms' shared type is what anything but <all> is charged against.
	grammar := "xs:simpleExplicitGroup (xmlschema11-1.md:5246)"
	if local == "all" {
		grammar = "xs:namedGroup's own <all> arm (xmlschema11-1.md:5193)"
	}
	for _, attr := range []string{"minOccurs", "maxOccurs"} {
		if _, ok := group.Attr(attr); !ok {
			continue
		}
		return fmt.Errorf("parser: <%s> at %s carries a %s attribute, which the schema for schema documents prohibits on the body of the named <group> at %s: %s restricts %s to use=\"prohibited\", and it is legal on a local compositor alone", local, group.Loc(), attr, decl.Loc(), grammar, attr)
	}
	return nil
}

// compositorChild returns el's first <all>/<choice>/<sequence> child (a model
// group definition's body, §3.7.2), or nil. Unlike modelGroupChild it excludes
// <group>: a top-level <group> definition's body is never a nested reference. A
// later such sibling is not returned and is not dropped either:
// repeatedCompositorChild charges it.
func compositorChild(el *Element) *Element {
	for _, child := range el.Children() {
		c, ok := child.(*Element)
		if !ok || c.Name().Space() != xsd.XMLSchemaNS {
			continue
		}
		switch c.Name().Local() {
		case "all", "choice", "sequence":
			return c
		}
	}
	return nil
}

// repeatedCompositorChild returns the SECOND <all>/<choice>/<sequence> child of a
// named <group> in document order, or nil when it carries at most one among the
// three. xs:namedGroup's content model (xmlschema11-1.md:5187) is (annotation?,
// (all | choice | sequence)) with the inner choice at minOccurs="1"
// maxOccurs="1", so a second one is the fault an absent one is, from the other
// side of one cardinality: sd-valid (§2.4 clause 1, :615), no rule ID, on
// rejectNamedGroupBody's citation above and reported at the second child.
//
// Charging it is what keeps buildDefinitionModelGroup's read honest, on
// repeatedContentAlternative's reasoning: compositorChild is a first-match lookup
// that drops every later sibling, so without this a <group> carrying two
// compositors maps from whichever came first and the second leaves no trace
// (#1048).
//
// The walk is not childElement: the fault spans THREE expanded names, and only a
// single pass over the children orders an <all> against a <choice> against a
// <sequence> to name the one that is second (STYLE D2). Foreign-namespace
// siblings are skipped rather than charged, on rejectNamedGroupBody's reasoning
// for the same skip — a scope choice, not a wildcard xs:namedGroup grants.
func repeatedCompositorChild(el *Element) *Element {
	var first *Element
	for _, child := range el.Children() {
		c, ok := child.(*Element)
		if !ok {
			continue
		}
		if !isXSD(c, "all") && !isXSD(c, "choice") && !isXSD(c, "sequence") {
			continue
		}
		if first != nil {
			return c
		}
		first = c
	}
	return nil
}

// groupParticles maps the particle children of a model group in document order,
// omitting each minOccurs=maxOccurs=0 child (which maps to no component, §3.9.2).
// scopeParent is the Complex Type Definition or Model Group Definition the whole
// content tree hangs under, threaded to each local element declaration.
//
// An <all> child of an <all> is refused here, and only that one. Appendix A's
// xs:allModel — the group xs:complexType "all" restricts its content to —
// admits just <element>, <any> and <group>, but the component that spelling maps
// to, an all group among another all group's {particles} at min=max=1, is
// exactly what cos-all-limited clause 1.3 permits: no Schema Component
// Constraint can name the fault, so the grammar production is the only text that
// forbids it. §3.8.3 states no Schema Representation Constraint for <all>
// either, so the fault carries no rule ID, the same footing as
// produceGroupRefParticle's missing-ref fault.
//
// A <sequence> or <choice> child is deliberately NOT refused here: it maps to
// the shape cos-all-limited clause 2 forbids, and xsd/allgrouplimited.go charges
// it at finalize with that rule ID and a positioned Loc.
func (p *producer) groupParticles(group *Element, scopeParent xsd.ElementScopeParent) ([]xsd.Particle, error) {
	var particles []xsd.Particle
	inAll := group.Name().Local() == "all"
	for _, child := range group.Children() {
		el, ok := child.(*Element)
		if !ok {
			continue
		}
		if el.Name().Space() != xsd.XMLSchemaNS {
			continue
		}
		var part *xsd.Particle
		var err error
		switch el.Name().Local() {
		case "annotation":
			continue
		case "element":
			part, err = p.produceElementParticle(el, scopeParent)
		case "any":
			part, err = p.produceAnyParticle(el)
		case "sequence", "choice", "all":
			if inAll && el.Name().Local() == "all" {
				return nil, fmt.Errorf("parser: <all> at %s is a child of an <all>, whose content Appendix A's xs:allModel restricts to <element>, <any> and <group>", el.Loc())
			}
			part, err = p.produceGroupParticle(el, scopeParent)
		case "group":
			part, err = p.produceGroupRefParticle(el)
		default:
			return nil, unexpectedModelGroupChild(group, el)
		}
		if err != nil {
			return nil, err
		}
		if part != nil {
			particles = append(particles, *part)
		}
	}
	return particles, nil
}

// unexpectedModelGroupChild is groupParticles' fault for a child the model group
// group's content model has no position for — an <attribute>, or an identity
// constraint (#1817) — positioned at the child and naming group as its owner
// (STYLE E3). An <all>'s content is xs:allModel (xmlschema11-1.md:5259); a
// <choice>'s or a <sequence>'s, local (xs:explicitGroup, :5229) or a named
// group's body (xs:simpleExplicitGroup, :5246), is xs:nestedParticle (:4657)
// either way. §5.1's first bullet (:4296) binds and mints no rule ID, so the
// error is plain (STYLE E2).
func unexpectedModelGroupChild(group, child *Element) error {
	owner := group.Name().Local()
	grammar := "xs:nestedParticle (xmlschema11-1.md:4657), which admits <element>, <group>, <choice>, <sequence> and <any>"
	if owner == "all" {
		grammar = "xs:allModel (xmlschema11-1.md:5259), which admits <element>, <any> and <group>"
	}
	return fmt.Errorf("parser: unexpected model group child <%s> at %s under the <%s> at %s: its content after the optional <annotation> is %s",
		child.Name().Local(), child.Loc(), owner, group.Loc(), grammar)
}

// produceElementParticle maps a local <element> to a Particle (§3.3.2.3). A
// minOccurs=maxOccurs=0 element maps to no component at all (returns nil). An
// <element ref="..."> yields a deferred ElementDeclarationRef term (resolved at
// finalize, #173); otherwise a sibling local Element Declaration is built inline,
// scoped to scopeParent (§3.3.2.3 dcl.elt.local). The ref form takes no
// scopeParent: it denotes a top-level declaration, whose own {scope} is global
// (§3.3.2.4 ref.elt.global maps only the Particle, never a declaration).
//
// The elided element's {term} is mapped all the same, for the grammar faults
// elementParticleTerm charges — an unbound prefix on ref=, and everything
// produceLocalElement rejects on the inline form — which §5.1's first bullet
// binds whatever the element maps to (#883). It is mapped through
// discardingComponents, and the result thrown away: "maps to no component at
// all" governs what the subtree CONTRIBUTES as firmly as §5.1 governs how it is
// spelled, and produceLocalElement reaches produceIdentityConstraint's name=
// arm, whose builder registration is NOT dropped by dropping the Particle.
//
// occursOf answers FIRST, since its elided verdict decides which producer maps
// the {term} — but the verdict it charges on this element's own occurrence range
// (p-props-correct for min greater than max, cvc-datatype-valid for a lexical
// outside the declared type) is held back until after that mapping, so a Schema
// for Schema Documents fault inside the <element> answers ahead of anything
// charged on the particle wrapping it (the ordering produceGroupParticle keeps
// for the same reason).
func (p *producer) produceElementParticle(el *Element, scopeParent xsd.ElementScopeParent) (*xsd.Particle, error) {
	occ, elided, occErr := occursOf(el)
	if elided {
		_, err := p.discardingComponents().elementParticleTerm(el, scopeParent)
		return nil, err
	}
	term, err := p.elementParticleTerm(el, scopeParent)
	if err != nil {
		return nil, err
	}
	if occErr != nil {
		return nil, occErr
	}
	part, err := xsd.NewParticle(el.Loc(), occ, term)
	if err != nil {
		return nil, err
	}
	return &part, nil
}

// elementParticleTerm maps a local <element> to the {term} of its Particle: the
// deferred ElementDeclarationRef of §3.3.2.4 ref.elt.global for the ref= form,
// the inline local Element Declaration of §3.3.2.3 dcl.elt.local otherwise.
func (p *producer) elementParticleTerm(el *Element, scopeParent xsd.ElementScopeParent) (xsd.TermOrRef, error) {
	if ref, hasRef := el.Attr("ref"); hasRef {
		// The ref= form reads no child of its own, so this is where its children are
		// ordered against xs:element's content model — the one model Appendix A gives
		// all three forms (s4sElement, #1076). It is NOT src-element clause 2.2
		// (§3.3.3, xmlschema11-1.md:1321), which admits <annotation> and nothing else
		// under a ref= form: that clause is a numbered constraint of its own, this
		// walk is the uncataloged §5.1 grammar class, and neither stands in for the
		// other. Both run, the walk FIRST — the run order checkS4SChildOrder's doc
		// records — so clause 2.2 answers only for the shapes the content model
		// admits.
		if err := checkS4SChildOrder(el, s4sElement); err != nil {
			return nil, err
		}
		if err := rejectRefElementUseProhibitedAttrs(el); err != nil {
			return nil, err
		}
		if err := rejectRefElementDeclarationAttrs(el); err != nil {
			return nil, err
		}
		if err := rejectRefElementChildren(el); err != nil {
			return nil, err
		}
		qn, err := p.resolveQName(el, ref, "ref")
		if err != nil {
			return nil, err
		}
		return xsd.ElementDeclarationRef{Name: qn}, nil
	}
	decl, err := p.produceLocalElement(el, scopeParent)
	if err != nil {
		return nil, err
	}
	return xsd.ResolvedTerm{Term: decl}, nil
}

// rejectRefElementUseProhibitedAttrs charges src-element clause 2.2 (§3.3.3,
// xmlschema11-1.md:1321) on the three attributes xs:localElement itself narrows
// to use="prohibited" — substitutionGroup, final and abstract
// (xmlschema11-1.md:5124-:5126) — written on a local <element ref="...">: "If
// ref is present, then no unqualified attributes are present other than
// minOccurs, maxOccurs, and id."
//
// They are walked in that grammar's declaration order, because the attribute the
// rejection names is output and output never follows the document's attribute
// order or a map's (STYLE D2).
//
// The three share ONE grammar fact, which is what separates them from
// rejectRefElementDeclarationAttrs's eight: every one of those is syntactically
// legal under ref= and clause 2.2's prose is the only thing rejecting it, while
// these three are prohibited on every local <element> whatever form it takes.
// Clause 2.2 is charged over all three all the same, since it reaches the XML
// attribute directly under ref= and the §5.1 grammar class produceLocalElement
// charges on the inline form (rejectLocalElementProhibitedAttrs) carries no rule
// ID at all.
//
// The rule ID is src-element, NOT the e-props-correct clause 3 produceLocalElement
// charges for substitutionGroup on the inline local form. The ref= form maps to
// an xsd.ElementDeclarationRef, a deferred reference with no {substitution group
// affiliations} property at all, so there is nothing for a component constraint
// over that property to be violated by; charging e-props-correct here would be a
// verdict the component tableau cannot carry (STYLE E2). The same holds a
// fortiori for final and abstract, which no clause of e-props-correct confines
// by {scope} on any form.
//
// "Unqualified attributes" scopes the clause to no-namespace attributes, which
// Element.Attr already enforces by construction: a foreign-namespace
// substitutionGroup is invisible to it, and xs:localElement admits foreign
// attributes outright (<anyAttribute namespace="##other"> at
// xmlschema11-1.md:5127).
func rejectRefElementUseProhibitedAttrs(el *Element) error {
	for _, name := range [...]string{"substitutionGroup", "final", "abstract"} {
		if _, ok := el.Attr(name); !ok {
			continue
		}
		return xsderr.New(ruleSrcElement, el.Loc(),
			"the <element ref=\"...\"> carries a %s attribute, but src-element clause 2.2 admits no unqualified attribute other than minOccurs, maxOccurs and id when ref is present",
			name)
	}
	return nil
}

// rejectRefElementDeclarationAttrs charges src-element clause 2.2 (§3.3.3,
// xmlschema11-1.md:1321) on the eight attributes only a DECLARING <element> can
// carry, written on a local <element ref="...">: "If ref is present, then no
// unqualified attributes are present other than minOccurs, maxOccurs, and id."
//
// They are walked in xs:element's own declaration order — name from the
// xs:defRef attribute group (xmlschema11-1.md:4642, reached from xs:element at
// :5065), then type, default, fixed, nillable, block, form and targetNamespace
// inline at :5066-:5082 — because the attribute the rejection names is output,
// and output never follows the document's attribute order or a map's (STYLE D2).
//
// Clause 2.2's prose is the ONLY footing that rejects these eight, which is what
// separates them from the three rejectRefElementUseProhibitedAttrs answers ahead
// of here: xs:localElement narrows exactly those three of the base type's
// attributes with use="prohibited" (substitutionGroup, final, abstract at
// xmlschema11-1.md:5124-:5126) and none of these, so every one of them is
// syntactically legal under ref= and no grammar walk will ever reach them.
// Clause 2's "if the item's parent is not <schema>" precondition is
// satisfied by the call site alone: elementParticleTerm maps local particles,
// and produceElement's top-level arm never reaches here.
//
// Two of the eight are ALSO governed by a second clause of the same rule, and
// both are charged under 2.2 regardless, so one attribute draws one rule ID from
// one helper: name violates clause 2.1 ("One of ref or name is present, but not
// both") simultaneously, and targetNamespace violates clause 4.1 (ed-with-ns,
// which requires name present) simultaneously. The spec adjudicates no
// precedence between clauses that fire together — §B leaves the tabulation of
// error conditions deliberately incomplete (xmlschema11-1.md:6925) — so the
// choice is this producer's, and clause 2.2 is the one it reaches by. Clauses
// 2.1 is charged nowhere in this producer; clause 4 is, on the inline local form
// alone (rejectLocalElementTargetNamespace), which a ref= form never reaches.
func rejectRefElementDeclarationAttrs(el *Element) error {
	for _, name := range [...]string{"name", "type", "default", "fixed", "nillable", "block", "form", "targetNamespace"} {
		if _, ok := el.Attr(name); !ok {
			continue
		}
		return xsderr.New(ruleSrcElement, el.Loc(),
			"the <element ref=\"...\"> carries a %s attribute, but src-element clause 2.2 admits no unqualified attribute other than minOccurs, maxOccurs and id when ref is present",
			name)
	}
	return nil
}

// rejectRefElementChildren charges src-element clause 2.2 (§3.3.3,
// xmlschema11-1.md:1321) on a child in the Schema namespace other than
// <annotation> under a local <element ref="...">: "If ref is present, then …
// no children in the Schema namespace (xs) other than <annotation>."
//
// It is narrower than checkS4SChildOrder(el, s4sElement) and cannot be folded
// into it. s4sElement is xs:element's content model, which Appendix A leaves
// byte-identical across all three forms the element takes, so an <element
// ref="x"><simpleType/></element> fills its "(simpleType | complexType)?"
// position and passes that walk outright. Clause 2.2 is a Schema Representation
// Constraint stated BEYOND the schema for schema documents, which is why it is
// the only footing this rejection stands on (STYLE E2). The walk keeps running
// ahead of it, on the run order checkS4SChildOrder's doc records.
//
// Only the CHILD half of the clause is charged here. Its other half — no
// unqualified attribute but minOccurs, maxOccurs and id — belongs to
// rejectRefElementUseProhibitedAttrs and rejectRefElementDeclarationAttrs, both
// of which run first, so a document violating both halves is answered at the
// attribute.
//
// A child outside the Schema namespace is not this clause's: the clause reaches
// that namespace alone. xs:element's grammar admits no foreign child, and the
// s4sElement walk ahead of this check charges one (#1982).
func rejectRefElementChildren(el *Element) error {
	for _, child := range el.Children() {
		c, ok := child.(*Element)
		if !ok || c.Name().Space() != xsd.XMLSchemaNS || c.Name().Local() == "annotation" {
			continue
		}
		return xsderr.New(ruleSrcElement, c.Loc(),
			"the <element ref=\"...\"> at %s carries a <%s> child, but src-element clause 2.2 admits no child in the Schema namespace other than <annotation> when ref is present",
			el.Loc(), c.Name().Local())
	}
	return nil
}

// rejectLocalElementProhibitedAttrs rejects a final or abstract attribute
// written on an inline local <element name="...">, which the schema for schema
// documents prohibits there: xs:localElement restricts both to use="prohibited"
// (xmlschema11-1.md:5125-:5126), unconditionally and whatever form the local
// element takes, while xs:topLevelElement admits both (:5086-:5107).
//
// The fault carries NO numbered rule ID, on the footing rejectProhibitedAttrs's
// doc establishes (parser/produce.go): §5.1 (xmlschema11-1.md:4289) makes it an
// error for a schema document not to be "fully valid with respect to a schema
// corresponding to the Schema for Schema Documents" (:4296), an error condition
// independent of the numbered Schema Representation Constraints beside it.
// Charging e-props-correct would be a fabricated verdict (STYLE E2): its clause 3
// (§3.3.6.1, xmlschema11-1.md:1520) confines {substitution group affiliations}
// by {scope}.{variety} and names no other property, and §3.3.2.1's Common
// Mapping Rules (:1144, dcl.elt.common) map {abstract} from abstract and
// {substitution group exclusions} from final/finalDefault UNCONDITIONALLY, so
// neither property reaches a state any component constraint forbids at local
// scope. src-element gives nothing either: clause 2.2's antecedent is "If ref is
// present", false on this form, and no other clause reaches these two names.
//
// substitutionGroup carries the same use="prohibited" on xs:localElement and is
// deliberately NOT in this list: produceLocalElement charges e-props-correct
// clause 3 over it because the Common Mapping Rule for {substitution group
// affiliations} really would build the non-empty value that clause forbids at
// local scope, and that reasoning reaches no further. The ref= form draws
// src-element clause 2.2 over all three instead
// (rejectRefElementUseProhibitedAttrs).
//
// The two are checked in the grammar's own declaration order, so a document
// writing both is always reported at final (STYLE D2).
func rejectLocalElementProhibitedAttrs(el *Element) error {
	for _, name := range [...]string{"final", "abstract"} {
		if _, ok := el.Attr(name); !ok {
			continue
		}
		return fmt.Errorf("parser: local <element> at %s carries a %s attribute, which the schema for schema documents prohibits on the local form: xs:localElement restricts %s to use=\"prohibited\", and it is legal on the top-level form alone", el.Loc(), name, name)
	}
	return nil
}

// rejectLocalElementTargetNamespace charges src-element clause 4 (ed-with-ns,
// §3.3.3, xmlschema11-1.md:1321) on a local inline <element name="..."> that
// writes an explicit targetNamespace: "If targetNamespace is present then all of
// the following are true: 4.1 name is present. 4.2 form is not present. 4.3 If
// the ancestor <schema> does not have a targetNamespace attribute or its ·actual
// value· is different from the ·actual value· of targetNamespace of <element>,
// then all of the following are true: 4.3.1 <element> has <complexType> as an
// ancestor. 4.3.2 (ed-with-ns-must-be-old) There is a <restriction> ancestor
// between the <element> and the nearest <complexType> ancestor, and the ·actual
// value· of the base attribute of <restriction> does not ·match· the name of
// xs:anyType."
//
// The clause reaches this INLINE LOCAL form alone, though it states no "parent
// is not <schema>" gate of its own. A top-level <element> may not write the
// attribute at all — xs:topLevelElement declares it use="prohibited"
// (xmlschema11-1.md:5086-:5107) where xs:localElement does not (:5110-:5130) —
// and rejectProhibitedAttrs charges that as the §5.1 grammar fault it is. A
// local <element ref="..."> is answered ahead of here by clause 2.2, whose
// rejectRefElementDeclarationAttrs doc records that 4.1 fires with it and loses.
//
// 4.3's antecedent carries the clause's whole force: an element writing its own
// document's targetNamespace REDUNDANTLY is legal wherever it stands, needing no
// <complexType> or <restriction> ancestor, and 4.3.1/4.3.2 are read only once
// the two namespaces differ. They are conjoined, so an <extension> between the
// element and the nearest <complexType>, or a <restriction base="xs:anyType">,
// fails the clause as squarely as no <complexType> ancestor at all.
//
// The ancestor <schema>'s targetNamespace is read as the EFFECTIVE one,
// p.target, and its presence as p.chameleon()'s: §F.1 task (a) writes the
// including namespace onto a chameleon document that has none of its own, and
// the constraint is stated over that transformed document. p.target is also what
// an <override>-substituted declaration is measured against, since such a
// declaration takes the OVERRIDDEN document's schema-level defaults (PRINCIPLES
// 16) — the footing localTargetNS reads the same value on for form="qualified".
// Presence is a distinct fact from an empty value here, exactly as
// [Element.Attr] keeps it: a schema with no targetNamespace at all satisfies
// 4.3's antecedent whatever the element wrote.
//
// 4.3.2's ·match· is baseMatchesAnyType's, asked of the <restriction> and not of
// this element: the lexical binds against the namespaces in scope where it was
// written (PRINCIPLES 19).
func (p *producer) rejectLocalElementTargetNamespace(el *Element) error {
	tns, ok := el.Attr("targetNamespace")
	if !ok {
		return nil
	}
	if _, hasName := el.Attr("name"); !hasName {
		return xsderr.New(ruleSrcElement, el.Loc(),
			"the local <element> carries a targetNamespace attribute but no name attribute, which src-element clause 4.1 requires whenever targetNamespace is present")
	}
	if _, hasForm := el.Attr("form"); hasForm {
		return xsderr.New(ruleSrcElement, el.Loc(),
			"the local <element> carries both a targetNamespace and a form attribute, but src-element clause 4.2 admits no form attribute when targetNamespace is present")
	}
	// 4.3's antecedent, which fails — leaving 4.3.1 and 4.3.2 unread — only when
	// the ancestor <schema> HAS a targetNamespace and the element wrote that same
	// one.
	_, own := p.schemaElem.Attr("targetNamespace")
	if (own || p.chameleon()) && tns == p.target {
		return nil
	}
	ct, restriction := nearestComplexTypeAndRestriction(el)
	if ct == nil {
		return xsderr.New(ruleSrcElement, el.Loc(),
			"the local <element> declares targetNamespace %q, which is not the ancestor <schema>'s, but has no <complexType> ancestor, which src-element clause 4.3.1 requires of an element declaring a namespace of its own", tns)
	}
	if restriction == nil {
		return xsderr.New(ruleSrcElement, el.Loc(),
			"the local <element> declares targetNamespace %q, which is not the ancestor <schema>'s, but no <restriction> stands between it and the <complexType> at %s, which src-element clause 4.3.2 (ed-with-ns-must-be-old) requires of an element declaring a namespace of its own",
			tns, ct.Loc())
	}
	if !p.baseMatchesAnyType(restriction) {
		return nil
	}
	return xsderr.New(ruleSrcElement, el.Loc(),
		"the local <element> declares targetNamespace %q, which is not the ancestor <schema>'s, but the <restriction> at %s standing between it and its <complexType> ancestor has base=%q, which ·matches· xs:anyType — src-element clause 4.3.2 (ed-with-ns-must-be-old) requires a base naming any other type",
		tns, restriction.Loc(), attrOr(restriction, "base"))
}

// baseMatchesAnyType reports whether el's base attribute ·matches· the name of
// xs:anyType — expanded-name equality (key-en-match, xmlschema11-1.md:2726)
// against the {name}/{target namespace} pair of the ur-type (:431) — with the
// lexical bound against el's OWN in-scope namespaces (PRINCIPLES 19).
//
// A base= that is absent, or whose prefix this producer cannot bind, ·matches·
// no name at all and so answers false. That is the reading src-element clause
// 4.3.2 and src-attribute clause 6.3.2 ask for, not a swallowed fault: the
// binding error belongs to el, whose own producer charges it at el's position,
// and produceComplexContent resolves base= BEFORE descending into the content
// model and the attribute uses that reach both callers here.
func (p *producer) baseMatchesAnyType(el *Element) bool {
	baseLex, hasBase := el.Attr("base")
	if !hasBase {
		return false
	}
	base, err := p.bindQName(el, baseLex, "base")
	return err == nil && base == anyTypeName
}

// nearestComplexTypeAndRestriction returns el's nearest <complexType> ancestor
// and the innermost <restriction> ancestor standing BETWEEN the two — the two
// ancestors src-element clause 4.3.1 and 4.3.2 (§3.3.3, xmlschema11-1.md:1321)
// and src-attribute clause 6.3.1 and 6.3.2 (§3.2.3, xmlschema11-1.md:868) are
// stated over, in wording those clauses share element for attribute. Both are nil
// when el has no <complexType> ancestor at all, since a <restriction> above one
// answers neither clause; restriction alone is nil when none intervenes.
//
// The walk is over the XML tree's ancestor axis, which the retained tree makes
// available (parser/tree.go's parent link) and which every term of the clauses is
// stated in: no component of any ancestor is consulted, and none exists yet
// while a local element or attribute is being mapped.
func nearestComplexTypeAndRestriction(el *Element) (complexType, restriction *Element) {
	for e := el.parent; e != nil; e = e.parent {
		if isXSD(e, "complexType") {
			return e, restriction
		}
		if restriction == nil && isXSD(e, "restriction") {
			restriction = e
		}
	}
	return nil, nil
}

// produceLocalElement maps a local inline <element name="..."> to a local
// Element Declaration (§3.3.2.3, dcl.elt.local, {scope} = local), including its
// {identity-constraint definitions}: §3.3.2.1's Common Mapping Rules apply
// uniformly to global and local declarations, so <key>/<keyref>/<unique> children
// are mapped here exactly as in produceElement. Registering them with the schema
// builder is the caller's job (produceElementParticle).
//
// Its {type definition} is §3.3.2.1 dcl.elt.common's tier chain, mapped by
// localDeclaredType. Tier 1's inline <complexType> child needs an identity
// minted before either component exists: the anonymous complex type it maps to
// is the {scope}.{parent} of its OWN nested local element declarations, and
// having no name it is identified by an xsd.ComponentID instead
// (xsd.AnonymousComplexTypeScopeParent, #340). The remaining tiers (the inline
// <simpleType> form, the type= form, the xs:anyType default) are mapped by
// declaredType, shared with the local-attribute chain.
//
// The declaration's identity is therefore minted UNCONDITIONALLY, before any
// tier is chosen, and every path returns through
// xsd.NewElementDeclarationOwningTypes: since #851 an <alternative> child may own
// an anonymous complex type of its own (§3.12.2 declare-ta), so a declaration
// with a plain type= owns types too and there is no tier-free path left. That one
// mint is the {context} (§3.4.2.1 dcl.ctd.common) of every type this declaration
// owns, and NewElementDeclarationOwningTypes checks each of them agrees; the
// per-edge CONTAINER tokens the nested scopes report are minted separately, one
// per <alternative> (see typeAlternativeOwnedComplexType).
//
// The element's OWN children are ordered against s4sElement here
// (checkS4SChildOrder, #1076), behind rejectBothInlineTypes for the reason that
// function's doc gives. The ref= form never reaches this function, and
// elementParticleTerm orders its children against the same model instead.
//
// src-element clause 3 (§3.3.3) is charged here for the both-present case, on the
// same footing produceElement charges it for a global <element>: without it,
// type= would silently win over an inline child. It covers the inline
// <complexType> arm as much as the <simpleType> one — the clause names both. It
// is charged BEHIND the walk, the run order checkS4SChildOrder's doc records for
// every src-* charge (#1246).
//
// final and abstract are rejected here, by rejectLocalElementProhibitedAttrs,
// behind the grammar walk — the order that same doc records — because the
// fault is of the walk's own uncataloged §5.1 class and carries no rule ID, so
// it answers ahead of every numbered src-element clause below it. That doc gives
// the reason no numbered rule fits, and the reason substitutionGroup is charged
// separately and earlier.
//
// src-element clause 4 (ed-with-ns) is charged here too, behind the grammar walk
// — the order that same doc records — and ahead of the name, by
// rejectLocalElementTargetNamespace: this is the only form of <element> the
// clause can reach (that function's doc gives the reason), and its 4.1 answers
// before declarationName would report the absent name that writing
// targetNamespace without one produces.
//
// An <element> carrying BOTH an inline <simpleType> and an inline <complexType>
// is rejected outright, with a plain grammar-fault error rather than a rule
// verdict: the schema for schema documents gives xs:localElement a single
// (simpleType | complexType)? slot, and no src-element clause states the
// condition (clause 3 is about a type child TOGETHER WITH type=, not about two
// type children). Letting one arm win silently would map a schema no processor
// accepts.
//
// e-props-correct clause 3 (§3.3.6.1) is charged here too, FIRST, and on the
// attribute rather than on the built component: a substitutionGroup= on a local
// <element> is prohibited by the schema for schema documents (use="prohibited"
// on xs:localElement, §3.3.2), and this producer runs no meta-schema validation
// pass ahead of mapping, so nothing else would see it. Reaching
// xsd.NewElementDeclaration's identical clause-3 check instead is not an option —
// the mapping would have to build a non-empty {substitution group affiliations}
// for a local declaration to trip it, which is exactly the state the clause
// forbids — and simply not reading the attribute here would silently ACCEPT the
// prohibited schema. It precedes the src-element test because it depends on
// nothing but the attribute's presence.
//
// The reach is the INLINE local form only, which is the only form that arrives
// here: elementParticleTerm's <element ref> branch returns before this function
// runs, and charges the same attribute there under src-element clause 2.2
// instead (rejectRefElementUseProhibitedAttrs, whose doc gives the reason the
// rule ID differs).
//
// {disallowed substitutions} comes from the same disallowedSubstitutions mapping
// the global path uses (STYLE T4): §3.3.2.1's row is a COMMON rule and the
// meta-schema leaves block= permitted on xs:localElement, unlike the three
// attributes it prohibits there (substitutionGroup, final, abstract), each of
// which this function rejects before reaching any mapping.
//
// scopeParent is the nearest <complexType> or named <group> ancestor's component,
// supplied by the caller (never recomputed from the element here — the ancestor
// axis reaches that ancestor's ELEMENT, as nearestComplexTypeAndRestriction
// walks it for clause 4, never the component it maps to, which does not exist
// until the caller finishes building it). It is a required parameter, and every
// path through this function builds the scope from it.
func (p *producer) produceLocalElement(el *Element, scopeParent xsd.ElementScopeParent) (xsd.ElementDeclaration, error) {
	if _, ok := el.Attr("substitutionGroup"); ok {
		return xsd.ElementDeclaration{}, xsderr.New(ruleEPropsCorrect, el.Loc(),
			"a local <element> carries a substitutionGroup attribute, but e-props-correct clause 3 confines a non-empty {substitution group affiliations} to a global {scope}.{variety} — the schema for schema documents accordingly declares the attribute use=\"prohibited\" on xs:localElement (§3.3.2)")
	}
	_, hasType := el.Attr("type")
	inlineSimple := childElement(el, xsd.XMLSchemaNS, "simpleType")
	inlineComplex := childElement(el, xsd.XMLSchemaNS, "complexType")
	if err := rejectBothInlineTypes(el, inlineSimple, inlineComplex); err != nil {
		return xsd.ElementDeclaration{}, err
	}
	if err := checkS4SChildOrder(el, s4sElement); err != nil {
		return xsd.ElementDeclaration{}, err
	}
	if hasType && (inlineSimple != nil || inlineComplex != nil) {
		return xsd.ElementDeclaration{}, xsderr.New(ruleSrcElement, el.Loc(),
			"element has both a type attribute and an inline <simpleType>/<complexType> child, but src-element clause 3 forbids both")
	}
	if err := rejectLocalElementProhibitedAttrs(el); err != nil {
		return xsd.ElementDeclaration{}, err
	}
	if err := p.rejectLocalElementTargetNamespace(el); err != nil {
		return xsd.ElementDeclaration{}, err
	}
	tns, err := p.localTargetNS(el, "elementFormDefault")
	if err != nil {
		return xsd.ElementDeclaration{}, err
	}
	qname, err := declarationName(el, tns)
	if err != nil {
		return xsd.ElementDeclaration{}, err
	}
	vc, err := valueConstraintOf(el, ruleSrcElement)
	if err != nil {
		return xsd.ElementDeclaration{}, err
	}
	nillable, _, err := boolAttr(el, "nillable")
	if err != nil {
		return xsd.ElementDeclaration{}, err
	}
	constraints, err := p.identityConstraintsOf(el)
	if err != nil {
		return xsd.ElementDeclaration{}, err
	}
	scope, err := xsd.NewLocalScope(el.Loc(), scopeParent)
	if err != nil {
		return xsd.ElementDeclaration{}, err
	}
	edID := xsd.NewComponentID()
	typeDef, err := p.localDeclaredType(el, edID, inlineComplex)
	if err != nil {
		return xsd.ElementDeclaration{}, err
	}
	typeTable, err := p.typeTableOf(el, edID, typeDef)
	if err != nil {
		return xsd.ElementDeclaration{}, err
	}
	return xsd.NewElementDeclarationOwningTypes(el.Loc(), edID, qname, typeDef, typeTable, scope, vc,
		nillable, constraints, nil, nil, false, p.disallowedSubstitutions(el))
}

// localDeclaredType maps §3.3.2.1 dcl.elt.common's tier chain for a LOCAL
// <element>, splitting off tier 1's inline <complexType> — which needs the
// declaration's minted identity, already in the caller's hand — and delegating
// every other tier to declaredType. inlineComplex is that child, or nil.
func (p *producer) localDeclaredType(el *Element, edID xsd.ComponentID, inlineComplex *Element) (xsd.TypeDefinitionOrRef, error) {
	if inlineComplex == nil {
		return p.declaredType(el, anyTypeName)
	}
	ct, err := p.produceComplexType(elementOwnedComplexType{owner: edID}, inlineComplex)
	if err != nil {
		return nil, err
	}
	return xsd.InlineTypeDefinition{Definition: ct}, nil
}

// rejectBothInlineTypes rejects an <element> carrying BOTH an inline
// <simpleType> and an inline <complexType> child. The schema for schema
// documents gives xs:element a single (simpleType | complexType)? slot, and
// src-element (§3.3.3) states no clause for two type children — clause 3 is
// about a type child together with type= — so this is a plain grammar fault like
// a nameless top-level <complexType>, not an xsderr rule verdict (STYLE E2:
// never fabricate a rule ID for a condition the spec states elsewhere).
//
// It is shared by the global and the local element path (STYLE T4): §3.3.2.1's
// tier 1 is a COMMON mapping rule and the meta-schema restriction is the same on
// xs:topLevelElement and xs:localElement, so one implementation serves both.
//
// s4sElement (produce_s4sorder.go) charges the same shape as a repeat of its
// single "(simpleType | complexType)?" position, and both paths run this guard
// FIRST so only one of the two ever answers: this one names BOTH children where
// that walk names the second alone. #444 owns whether the pairing stays.
func rejectBothInlineTypes(el *Element, inlineSimple, inlineComplex *Element) error {
	if inlineSimple == nil || inlineComplex == nil {
		return nil
	}
	return fmt.Errorf("parser: <element> at %s has both an inline <simpleType> and an inline <complexType> child, but the schema for schema documents allows at most one type child on an <element>", el.Loc())
}

// declaredType maps the {type definition} of an <element> or <attribute> whose
// both-present and inline-<complexType> cases the caller has already excluded.
// It is the ONE implementation of the parallel mapping chains — §3.3.2.1
// dcl.elt.common for an element, §3.2.2.1 dcl.att.global and §3.2.2.2
// dcl.att.local for an attribute — which agree on every tier this producer
// implements (STYLE T4):
//
//	tier 1  the anonymous type corresponding to the inline <simpleType> child;
//	tier 2  the type definition the type= attribute ·resolves· to;
//	last    dflt, the caller's fallback — xs:anyType for an element (§3.3.2.1
//	        clause 4), xs:anySimpleType for an attribute (§3.2.2.1/§3.2.2.2).
//
// It serves both GLOBAL paths as well as the local ones (#442, #733): the tier
// chain is COMMON to both element forms — §3.3.2.2 dcl.elt.global supplements
// {target namespace} and {scope} alone, never {type definition} — and the two
// attribute forms state one chain each in identical words, so neither a
// top-level <element> nor a top-level <attribute> has anything to map
// differently here, and produceElement and produceAttribute call this rather
// than restating it.
//
// The anonymous type is built once, here, and handed to the declaration as an
// xsd.InlineTypeDefinition: it goes into no symbol table, so the declaration is
// its sole owner. Its {context} (§3.16.1 std-context) is a SEPARATE property
// from the {type definition} this maps and stays unpopulated on every anonymous
// simple type the producer builds, global and local alike (#206).
//
// The element chain's clause 3 — "The declared {type definition} of the Element
// Declaration ·resolved· to by the FIRST QName in the ·actual value· of the
// substitutionGroup attribute, if present" — has no tier here, and its absence
// is not a gap: no caller can reach it through this function. substitutionGroup=
// is legal only on a top-level <element> (use="prohibited" on xs:localElement,
// §3.3.2), produceLocalElement charges e-props-correct clause 3 for a local one,
// and the attribute chain has no clause-3 analog at all. The global caller
// decides it itself, BETWEEN tier 2 and dflt, through substitutionGroupHeadType
// (#395, which needed it because e-props-correct clause 4 reads the resulting
// type) — which is also why this function's dflt is never the answer on that
// path. The two mappings deliberately read the list differently: {substitution
// group affiliations} resolves EVERY item, clause 3 the first one only.
func (p *producer) declaredType(el *Element, dflt xsd.QName) (xsd.TypeDefinitionOrRef, error) {
	if inline := childElement(el, xsd.XMLSchemaNS, "simpleType"); inline != nil {
		st, err := p.constructSimpleType(xsd.QName{}, inline) // tier 1
		if err != nil {
			return nil, err
		}
		return xsd.InlineTypeDefinition{Definition: st}, nil
	}
	typeLex, hasType := el.Attr("type")
	if !hasType {
		return xsd.TypeDefinitionRef{Name: dflt}, nil
	}
	qn, err := p.resolveQName(el, typeLex, "type") // tier 2
	if err != nil {
		return nil, err
	}
	return xsd.TypeDefinitionRef{Name: qn}, nil
}

// produceAnyParticle maps an <any> to a Particle whose {term} is a Wildcard
// (§3.10.2.1). A minOccurs=maxOccurs=0 <any> maps to no component (returns nil),
// but the Wildcard is mapped first and then dropped with the particle: §5.1's
// first bullet binds the <any> element information item whatever it maps to, so
// produceWildcard's namespace and processContents faults answer inside an elided
// subtree too (#883).
//
// The Wildcard is mapped with THIS producer, not the discarding copy
// produceElementParticle and produceGroupParticle use, because there is nothing
// to discard: produceWildcard reaches namespaceConstraint, disallowedNames and
// bindQName only, none of which touches the builder, so the value it returns is
// the whole of what it produces.
func (p *producer) produceAnyParticle(el *Element) (*xsd.Particle, error) {
	wildcard, err := p.produceWildcard(el)
	if err != nil {
		return nil, err
	}
	occ, elided, err := occursOf(el)
	if err != nil {
		return nil, err
	}
	if elided {
		return nil, nil
	}
	part, err := xsd.NewParticle(el.Loc(), occ, xsd.ResolvedTerm{Term: wildcard})
	if err != nil {
		return nil, err
	}
	return &part, nil
}

// produceAttributeUses maps the attribute-bearing children of parent (a
// <complexType>, <restriction>, or <extension>) into the type's attribute
// content, the expanded names §3.4.2.4 clause 3.2.2 blocks, and its ·local
// wildcard· — its own <anyAttribute>, if any.
//
// None of the three is a finished property. {attribute uses} is §3.4.2.4 clauses
// 1-3 and {attribute wildcard} §3.4.2.5 clauses 1-2, and every clause past the
// type's own children needs a component no producer holds: clause 2 and the
// ·complete wildcard· need the attribute group definitions the <attributeGroup
// ref> children name, which xsd/attributegroupfold.go folds in at finalize
// (#479), and clause 3 and clause 2.2 need the resolved base, which
// xsd/attributeusefold.go (#401) and xsd/attributewildcardfold.go (#414) fold
// in after it. So the content carries each <attributeGroup ref> as an
// xsd.AttributeGroupRef in document order, and the clause 3.2.2 names ride
// along for the same reason: the fold that consumes them runs at finalize, and
// by then the source is gone.
//
// ctElem is the <complexType> element itself, which is parent only in the
// implicit-content form: the two wrapped forms pass their <restriction>/
// <extension> as parent and the enclosing <complexType> here, because
// §3.4.2.4's default-attributes precondition reads defaultAttributesApply off
// the <complexType> while every other child this function walks is the
// derivation alternant's.
//
// scopeParent is the {scope}.{parent} (§3.2.1 sc_a) of parent's OWN <attribute>
// children — the enclosing complex type, supplied by the caller as an explicit
// parameter rather than stashed on the producer, so nesting can never
// mis-attribute a declaration. It does NOT reach the attributes of a referenced
// <attributeGroup>: those belong to the group's own component, built by
// buildAttributeGroup under the group's own scope (§3.2.2.2 dcl.att.local).
func (p *producer) produceAttributeUses(ctElem, parent *Element, scopeParent xsd.AttributeScopeParent) ([]xsd.AttributeUseOrGroupRef, []xsd.QName, *xsd.Wildcard, error) {
	content, wildcard, err := p.collectAttributeContent(parent, scopeParent)
	if err != nil {
		return nil, nil, nil, err
	}
	// §3.4.2.4's precondition, LAST: the synthesized <attributeGroup ref> appears
	// "after any other <attributeGroup> [children]", so it enters clause 2's union
	// behind them and its wildcard enters the §3.6.2.2 pre-order behind theirs —
	// which is what leaves the finalize-time intersection taking {process
	// contents} off the container's own <anyAttribute> when it has one.
	dflt, err := p.defaultAttributesRef(ctElem)
	if err != nil {
		return nil, nil, nil, err
	}
	if dflt != nil {
		content = append(content, *dflt)
	}
	prohibited, err := p.prohibitedAttributeNames(parent)
	if err != nil {
		return nil, nil, nil, err
	}
	return content, prohibited, wildcard, nil
}

// defaultAttributesRef applies §3.4.2.4's (dcl.ctd.attuses) precondition on
// ctElem: when the <schema> ancestor carries defaultAttributes and the
// <complexType> does not carry defaultAttributesApply="false", {attribute uses}
// is computed "as if there were an <attributeGroup> [child] with empty content
// and a ref [attribute] whose ·actual value· is the same as that of the
// defaultAttributes [attribute]". So it returns exactly that reference — an
// xsd.AttributeGroupRef — or nil when the precondition does not hold. §3.4.2.5
// (dcl.ctd.anyatt) states the IDENTICAL precondition for {attribute wildcard},
// and both are discharged by the one reference: finalize folds it in exactly
// as it folds a written <attributeGroup ref>, so the two properties cannot come
// apart.
//
// The default group in force is derived from p.schemaElem per call rather than
// stashed on the producer (STYLE D3, as chameleon() derives its own answer), and
// defaultAttributesApply is read here and nowhere else: neither attribute is a
// Schema-component property — §3.17.1's tableau has no {default attribute group}
// and §3.4.1's none for the opt-out — so neither may travel past the mapping
// that reads it, the same ruling wildcardElement gives appliesToEmpty.
//
// An ABSENT defaultAttributesApply folds: §3.4.2.4 tests only for the value
// false, and the attribute's declared default is true (XML Representation
// Summary, <complexType>).
//
// There is NO eager document-level check that defaultAttributes resolves. A
// document that declares one and defines no <complexType> synthesizes no
// reference, so an unresolvable QName there is charged nothing — the reference
// exists only where the precondition synthesizes it, and finalize charges it
// src-resolve clause 1.4 there, against the <complexType> that holds it.
//
// A <complexType> that ALSO writes an explicit <attributeGroup ref> naming the
// default group holds two references to one group, and folds it ONCE: the
// finalize-time closure takes the set union clause 2 describes, so no two uses
// with one expanded name reach ct-props-correct (§3.4.6.1) clause 4.
//
// That closure reaches ONE type, so it cannot see a BASE type's fold, and it
// does not have to. When a <complexContent> or <simpleContent> EXTENSION's base
// folds this same default group — the ordinary case, since the base need only
// not carry defaultAttributesApply="false" — the group's uses reach the derived
// type twice, once from clause 2 and once from §3.4.2.4 clause 3.1's
// inheritance of the base's already-folded {attribute uses}. Clause 3.1 unions
// SETS and holds the twice-reached member once, so ct-props-correct (§3.4.6.1)
// clause 4 has no two DISTINCT members to charge (xsd/attributeusefold.go,
// #1082).
//
// The QName is resolved at p.schemaElem, never at ctElem, because it is written
// there: that is the element whose in-scope prefixes bind it, whose <import>s
// license it (licensedNamespace reads containingSchema), and whose §F.1 task (b)
// chameleon rewrite reaches it (declares(p.schemaElem) holds by construction).
// Under <override> it also picks the right document: p.schemaElem is the
// OVERRIDDEN document's root even for a <complexType> substituted in from the
// overriding one, which is the reading §4.2.5 gives and defaultOpenContentElem
// already takes for <defaultOpenContent>.
func (p *producer) defaultAttributesRef(ctElem *Element) (*xsd.AttributeGroupRef, error) {
	lexical, ok := p.schemaElem.Attr("defaultAttributes")
	if !ok {
		return nil, nil
	}
	apply, present, err := boolAttr(ctElem, "defaultAttributesApply")
	if err != nil {
		return nil, err
	}
	if present && !apply {
		return nil, nil
	}
	qn, err := p.resolveQName(p.schemaElem, lexical, "defaultAttributes")
	if err != nil {
		return nil, err
	}
	return &xsd.AttributeGroupRef{Name: qn}, nil
}

// prohibitedAttributeNames is §3.4.2.4 clause 3.2.2's input: the expanded names
// of parent's own <attribute> children carrying use="prohibited", in document
// order.
//
// It reads parent's DIRECT children only, never descending <attributeGroup ref>,
// because clause 3.2.2 is written over "an <attribute> [child]" of the
// <restriction>/<extension>/<complexType> itself — a prohibited <attribute> inside
// a referenced group is one of the "other contexts" §3.4.2.4's Note calls
// pointless, and collectAttributeContent already ignores it.
//
// The name is computed rather than read off a component: §3.4.2.4's Note makes a
// prohibited <attribute> correspond to no component at all, so produceAttributeUse
// returns before building anything and there is nothing to take a name from. The
// two forms mirror that function's split — a ref= resolves as a QName (§3.2.2.3),
// a name= takes the local declaration's {target namespace} (§3.2.2.2).
//
// use= is read through attributeUseToken, the one encoding of that token (STYLE
// T4), because the ·actual value· is what clause 3.2.2 quantifies over: read
// raw, use=" prohibited " would map to no Attribute Use in produceAttributeUse
// and still be absent from this exclusion set, silently reinstating the base
// type's identically-named use at the finalize-time fold (#1328).
//
// No src-attribute clause is charged here: produceAttributeUse charges clauses
// 1, 2, 3, 5 and 6 upstream, before the same prohibited <attribute> declines
// to map, so re-charging them would put them in two encodings (#358). Its
// clause 3 charge makes the `!hasName` branch below unreachable —
// produceAttributeUses runs collectAttributeContent, which returns
// produceAttributeUse's rejection of a neither-ref-nor-name <attribute> before
// this function walks the same children — and the branch is kept as a fallback.
// The same walk charges an out-of-enumeration use= ahead of this function, which
// is why attributeUseToken's error is a fallback here too.
//
// The three failures that ARE surfaced here are an unresolvable ref= prefix —
// src-resolve, charged by resolveQName for every other reference in this
// producer — a name= that is not an xs:NCName, charged cvc-datatype-valid by
// declarationName for every other declaration name in it, and a form= outside
// xs:formChoice, charged cvc-datatype-valid by localTargetNS for every other
// local declaration. The third reaches this function ALONE, because
// produceAttributeUse returns at the prohibited form without building the
// sibling declaration that would otherwise read the attribute. A prohibited
// <attribute> earns no exemption from any of the three: all are faults of the
// schema document, and none needs a component to exist to be a fault.
//
// Clause 6 is why the localTargetNS read below can no longer mint a name in a
// namespace the clause forbids: the same collectAttributeContent walk charges
// it on these children first, so an inadmissible targetNamespace fails the
// whole schema before this function reaches one (#1243).
func (p *producer) prohibitedAttributeNames(parent *Element) ([]xsd.QName, error) {
	var names []xsd.QName
	for _, child := range parent.Children() {
		el, ok := child.(*Element)
		if !ok || el.Name().Space() != xsd.XMLSchemaNS || el.Name().Local() != "attribute" {
			continue
		}
		use, err := attributeUseToken(el)
		if err != nil {
			return nil, err // unreachable: produceAttributeUse charged the same token upstream
		}
		if use != "prohibited" {
			continue
		}
		if ref, hasRef := el.Attr("ref"); hasRef {
			qn, err := p.resolveQName(el, ref, "ref")
			if err != nil {
				return nil, err
			}
			names = append(names, qn)
			continue
		}
		if _, hasName := el.Attr("name"); !hasName {
			continue // unreachable: produceAttributeUse already rejected this shape upstream
		}
		tns, err := p.localTargetNS(el, "attributeFormDefault")
		if err != nil {
			return nil, err
		}
		qn, err := declarationName(el, tns)
		if err != nil {
			return nil, err
		}
		names = append(names, qn)
	}
	return names, nil
}

// buildAttributeGroup maps a top-level <attributeGroup> (§3.6.2) into an
// Attribute Group Definition over its OWN attribute content: its <attribute>
// children, its <attributeGroup ref> children as references, and its own
// <anyAttribute> as the ·local wildcard·. The §3.6.2.1 {attribute uses} closure
// and the §3.6.2.2 {attribute wildcard} intersection over the groups it
// references — cycles included, which §3.6.2.1 makes legal rather than an error
// — are taken at finalize (xsd/attributegroupfold.go, #479), where
// ag-props-correct (§3.6.6) clause 2 is charged over the folded uses too.
//
// Every <attribute> collected from elem's own body is scoped to THIS group by
// construction (§3.2.2.2 dcl.att.local: an <attribute> with no <complexType>
// ancestor takes the Attribute Group Definition corresponding to the
// <attributeGroup> it is within), so name is passed down as an
// xsd.AttributeGroupScopeParent. A complex type referencing the group reaches
// those same declarations through the group's component at finalize, never by
// re-mapping elem, so they carry that {scope}.{parent} whichever container
// reaches them.
//
// elem's children are walked against s4sNamedAttributeGroup first, on §5.1's
// first bullet and no rule ID (checkS4SChildOrder, #1876): a child
// xs:namedAttributeGroup does not admit — an identity constraint (#1817), an
// <element>, a <group>, an <assert> — or an <anyAttribute> repeated or followed
// by the attribute block is charged rather than dropped by
// collectAttributeContent, whose src-attribute charges stand behind that walk
// (#1246).
//
// The receiver MUST be the producer of the document that declares elem, for the
// reasons collectAttributeContent gives; a caller holding a typeSource builds
// through its owner.
func (p *producer) buildAttributeGroup(name xsd.QName, elem *Element) (xsd.AttributeGroupDefinition, error) {
	if err := checkS4SChildOrder(elem, s4sNamedAttributeGroup); err != nil {
		return xsd.AttributeGroupDefinition{}, err
	}
	content, wildcard, err := p.collectAttributeContent(elem, xsd.AttributeGroupScopeParent{Name: name})
	if err != nil {
		return xsd.AttributeGroupDefinition{}, err
	}
	return xsd.NewAttributeGroupDefinition(elem.Loc(), name, content, wildcard)
}

// collectAttributeContent maps container's OWN attribute content, in document
// order: an xsd.ResolvedAttributeUse per <attribute> child that maps to a use,
// and one member per <attributeGroup ref> child (attributeGroupMember). It
// returns container's own <anyAttribute> apart, as the ·local wildcard· §3.6.2.2
// names L. It follows no reference: the groups a container names are folded in
// at finalize, from their own components (xsd/attributegroupfold.go).
//
// The receiver MUST be the producer of the document that declares container: every
// child mapped here — a local <attribute>'s {target namespace} (§3.2.2.2) and its
// type=, an <attribute ref>, an <attributeGroup ref> — is resolved against the
// receiver's schemaElem and §F.1 coercion.
//
// scopeParent is the {scope}.{parent} of container's OWN <attribute> children
// (§3.2.2.2 dcl.att.local, the component container corresponds to).
func (p *producer) collectAttributeContent(container *Element, scopeParent xsd.AttributeScopeParent) ([]xsd.AttributeUseOrGroupRef, *xsd.Wildcard, error) {
	var wildcard *xsd.Wildcard
	if any := childElement(container, xsd.XMLSchemaNS, "anyAttribute"); any != nil {
		wc, err := p.produceWildcard(any)
		if err != nil {
			return nil, nil, err
		}
		wildcard = &wc
	}
	var content []xsd.AttributeUseOrGroupRef
	for _, child := range container.Children() {
		el, ok := child.(*Element)
		if !ok || el.Name().Space() != xsd.XMLSchemaNS {
			continue
		}
		switch el.Name().Local() {
		case "attribute":
			use, err := p.produceAttributeUse(el, scopeParent)
			if err != nil {
				return nil, nil, err
			}
			if use != nil {
				content = append(content, xsd.ResolvedAttributeUse{Use: *use})
			}
		case "attributeGroup":
			member, err := p.attributeGroupMember(el)
			if err != nil {
				return nil, nil, err
			}
			content = append(content, member)
		}
	}
	return content, wildcard, nil
}

// attributeGroupMember maps one <attributeGroup ref> child to its attribute
// content member. A nested <attributeGroup> with no ref, or one carrying the name
// xs:attributeGroupRef prohibits (rejectProhibitedRefAttrs), is a
// well-formedness fault with no dedicated SCC (§3.6.3 "None as such", grounding
// Q6), reported as a plain error.
//
// The ref= attribute is held by el, so p resolves its QName (§F.1 task (b)
// transforms xs:*/@ref in the document that carries it). What it NAMES is not
// looked up here: the member is an xsd.AttributeGroupRef, resolved against
// {attribute group definitions} at finalize (src-resolve clause 1.4), so a
// definition from any document of the assembly — or one the builder was handed
// with no document source at all — is reachable by name.
//
// The one exception is a redefining <attributeGroup>'s self-reference, which
// src-expredef clause 2 makes mean the ORIGINAL in the <redefine>d document
// (§4.2.4 clause 7.1). That original is in no index (clause 4.1.2), so a
// reference by name would resolve to the redefinition itself and drop every use
// the original contributes; it is built here instead, under the REDEFINED
// document's own producer so its local declarations keep their own document's
// namespace and defaults, and handed over by value as an
// xsd.ResolvedAttributeGroup — the attribute-side twin of the ResolvedTerm
// produceGroupRefParticle builds for a <group ref> self-reference. It carries
// the same expanded name as the redefinition, so its local declarations report
// the same {scope}.{parent} either way. The build terminates: the original lives
// in the redefined document, outside any <redefine>, so a self-reference in ITS
// body is an ordinary reference.
func (p *producer) attributeGroupMember(el *Element) (xsd.AttributeUseOrGroupRef, error) {
	if err := rejectProhibitedRefAttrs(el); err != nil {
		return nil, err
	}
	ref, ok := el.Attr("ref")
	if !ok {
		return nil, fmt.Errorf("parser: a nested <attributeGroup> must be a reference (carry a ref attribute), but none is present")
	}
	qn, err := p.resolveQName(el, ref, "ref")
	if err != nil {
		return nil, err
	}
	src, redefining := p.redefinedAttributeGroupOriginal(el, qn)
	if !redefining {
		return xsd.AttributeGroupRef{Name: qn}, nil
	}
	original, err := src.owner.buildAttributeGroup(qn, src.elem)
	if err != nil {
		return nil, err
	}
	return xsd.ResolvedAttributeGroup{Definition: original}, nil
}

// produceAttributeUse maps a local <attribute> to an Attribute Use (§3.2.2.2,
// dcl.att.local). use="prohibited" maps to no Attribute Use (returns nil). An
// <attribute ref="..."> yields a deferred AttributeDeclarationRef; otherwise a
// sibling local Attribute Declaration is built inline. It enforces the structural
// src-attribute clauses (§3.2.3): 1 (default and fixed mutually exclusive, via
// valueConstraintOf), 2 and 5 (the default/fixed × use= corner, via
// useValueConstraintOK), 3 (exactly one of ref/name; ref excludes
// simpleType/form/type) and 6.1 on the ref= form alone (#1242). It charges clause
// 4 (type= and an inline <simpleType> mutually exclusive) on the prohibited name=
// form alone; produceLocalAttribute holds that charge for every other use=
// (#1270).
//
// Clauses 1, 2, 3 and 5 are ALL charged BEFORE the use="prohibited" return, and
// clauses 4 and 6 AT that return for the name= form, because a Schema
// Representation Constraint holds of the <attribute> element information item
// itself (§5.1: "any element information items which violate any of the relevant
// Schema Representation Constraints"). Mapping to no component at all (§3.2.2)
// skips the component-building half of the mapping, never the validation:
// <attribute use="prohibited" default="d" fixed="f"/> violates clause 1 exactly as
// the optional form does, and <attribute ref="a:x" name="x" use="prohibited"/>
// violates clause 3.1 exactly as the non-prohibited form does. This includes the
// neither-ref-nor-name shape (<attribute use="prohibited"/>): clause 3.1 has no
// use= precondition, so that shape is REJECTED under prohibited exactly as it
// already was under every other use= value, not silently accepted as a no-op
// element (#358). Clause 6 (att-with-ns) reaches the prohibited form on the same
// reasoning and by its own call to rejectLocalAttributeTargetNamespace at that
// return, the one this function makes (#1243): the clause's antecedent is the
// presence of targetNamespace and carries no use= qualifier, so <attribute
// name="w" use="prohibited" targetNamespace="b"/> under an <extension> fails 6.3.2
// exactly as the optional form does. The ref= form takes the same antecedent to
// clause 6.1 in the hasRef block below, which likewise stands ahead of the
// prohibited return, so <attribute ref="a:x" use="prohibited"
// targetNamespace="b"/> is rejected exactly as the optional form is (#1242).
//
// Clause 4 reaches the prohibited form on that same reasoning and from that same
// return, by the call to rejectAttributeTypeAndSimpleType this function makes
// there (#1270): the clause's antecedent is the type attribute and a <simpleType>
// child both being present and carries no use= qualifier, so <attribute name="x"
// use="prohibited" type="xs:string"><simpleType/></attribute> is rejected exactly
// as the optional form is. Every other use= takes it from produceLocalAttribute,
// which this return precedes; both sites call the one helper, so the clause has a
// single encoding and two guarded call sites rather than a repeated predicate
// (STYLE D3).
//
// default=/fixed= on the <attribute> element map to the USE's own {value
// constraint} (§3.5.1 vc_au) for both forms: dcl.att.local (§3.2.2.2) leaves the
// sibling local declaration's own {value constraint} ·absent· unconditionally,
// and ref.att.local (§3.2.2.3) reads them off the ref-carrying element itself,
// never touching the referenced top-level declaration's.
//
// scopeParent reaches only the sibling local declaration of the name= form: the
// ref= form (§3.2.2.3 ref.att.local) builds no declaration at all, and the
// top-level one it names carries the global {scope} its own mapping gave it.
func (p *producer) produceAttributeUse(el *Element, scopeParent xsd.AttributeScopeParent) (*xsd.AttributeUse, error) {
	// Every local <attribute> reaches this function, ref= and use="prohibited"
	// included, so it is where all three of them are ordered against xs:attribute's
	// content model (s4sAttribute, #1076) — ahead of the prohibited form's early
	// return, for the reason produceElementParticle maps an elided element's {term}
	// all the same: mapping to no component bounds what the subtree CONTRIBUTES, not
	// how §5.1 binds the way it is spelled. Every src-attribute clause below is
	// charged behind this walk, which is the run order checkS4SChildOrder's doc
	// records.
	if err := checkS4SChildOrder(el, s4sAttribute); err != nil {
		return nil, err
	}
	use, err := attributeUseToken(el)
	if err != nil {
		return nil, err
	}
	vc, err := valueConstraintOf(el, ruleSrcAttribute)
	if err != nil {
		return nil, err
	}
	if err := useValueConstraintOK(el); err != nil {
		return nil, err
	}
	ref, hasRef := el.Attr("ref")
	_, hasName := el.Attr("name")
	if hasRef && hasName {
		return nil, xsderr.New(ruleSrcAttribute, el.Loc(),
			"attribute has both ref and name, but src-attribute clause 3 requires exactly one")
	}
	if !hasRef && !hasName {
		return nil, xsderr.New(ruleSrcAttribute, el.Loc(),
			"attribute has neither ref nor name, but src-attribute clause 3 requires exactly one")
	}
	if hasRef {
		if childElement(el, xsd.XMLSchemaNS, "simpleType") != nil {
			return nil, xsderr.New(ruleSrcAttribute, el.Loc(),
				"attribute has both ref and an inline <simpleType>, but src-attribute clause 3 forbids a simpleType with ref")
		}
		if _, hasForm := el.Attr("form"); hasForm {
			return nil, xsderr.New(ruleSrcAttribute, el.Loc(),
				"attribute has both ref and form, but src-attribute clause 3 forbids a form with ref")
		}
		if _, hasType := el.Attr("type"); hasType {
			return nil, xsderr.New(ruleSrcAttribute, el.Loc(),
				"attribute has both ref and type, but src-attribute clause 3 forbids a type with ref")
		}
		// Clause 6.1's ONE reachable failure anywhere, and so the only place it is
		// charged (#1242): clause 3.1 forces name ABSENT whenever ref is present, so a
		// ref= attribute writing targetNamespace fails 6.1 unconditionally. Clause 3.2's
		// forbidden-with-ref list names <simpleType>, form and type and stops short of
		// targetNamespace, unlike src-element clause 2.2's blanket exclusion that rejects
		// the element-side twin, so nothing above this reaches the shape.
		//
		// 6.2 and 6.3 are left unread: clause 6's conjuncts are independent ("all of the
		// following must be true"), so failing 6.1 alone is grounds for rejection.
		// rejectLocalAttributeTargetNamespace, which charges them, is not called here —
		// its whole premise is that name is present.
		if tns, hasTNS := el.Attr("targetNamespace"); hasTNS {
			return nil, xsderr.New(ruleSrcAttribute, el.Loc(),
				"the <attribute ref=\"...\"> declares targetNamespace %q, but src-attribute clause 6.1 requires a name attribute whenever targetNamespace is present, and clause 3.1 admits no name alongside ref", tns)
		}
	}
	if use == "prohibited" {
		// Clauses 4 and 6 (att-with-ns) are charged HERE for the name= form, and only
		// here, rather than by lifting either charge above this return: guarding the two
		// calls to this branch leaves produceLocalAttribute's clause-4-then-6 order
		// standing for every other use=, and repeats it here, so no charge is reordered
		// against another anywhere (#1243, #1270). The guard is
		// rejectLocalAttributeTargetNamespace's own premise, not a scope: name is present
		// on every <attribute> it reads. Clause 4 takes the same guard for a different
		// reason — clause 3.2 rejects a ref= attribute carrying type= or a <simpleType>
		// child above, so nothing carrying both survives to here on that arm. The ref=
		// arm takes clause 6.1 above instead, so a ref= attribute reaching this return
		// wrote no targetNamespace at all (#1242).
		if !hasRef {
			if err := rejectAttributeTypeAndSimpleType(el); err != nil {
				return nil, err
			}
			if err := p.rejectLocalAttributeTargetNamespace(el); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	required := use == "required"
	// A nil inheritable is an absent attribute, which NewAttributeUse maps to false
	// for the local form (§3.2.2.2 dcl.att.local) and to the referenced
	// declaration's {inheritable} for the ref form (§3.2.2.3 ref.att.local).
	value, present, err := boolAttr(el, "inheritable")
	if err != nil {
		return nil, err
	}
	var inheritable *bool
	if present {
		inheritable = &value
	}

	if hasRef {
		qn, err := p.resolveQName(el, ref, "ref")
		if err != nil {
			return nil, err
		}
		au, err := xsd.NewAttributeUse(el.Loc(), required, xsd.AttributeDeclarationRef{Name: qn}, vc, inheritable)
		if err != nil {
			return nil, err
		}
		return &au, nil
	}
	decl, err := p.produceLocalAttribute(el, scopeParent)
	if err != nil {
		return nil, err
	}
	au, err := xsd.NewAttributeUse(el.Loc(), required, xsd.LocalAttributeDeclaration{Declaration: decl}, vc, inheritable)
	if err != nil {
		return nil, err
	}
	return &au, nil
}

// attributeUseToken is the ·actual value· of an <attribute>'s use= (key-vv),
// with an absent attribute read as the schema for schema documents' declared
// default "optional" — the single encoding of that default AND of the
// enumeration, shared by produceAttributeUse, useValueConstraintOK and
// prohibitedAttributeNames (STYLE T4). The two mapping stages run in the order
// §4.1.4 fixes:
//
//   - pre-lexical. Appendix A declares use= as an xs:NMTOKEN restriction
//     (xmlschema11-1.md:4685) and NMTOKEN fixes whiteSpace to collapse
//     (§3.4.4.1), applied BEFORE lexical-space membership is tested, so
//     use=" required " is the value required and maps {required} = true.
//     collapseTrim reads that ·actual value·, and its doc carries the proof that
//     trimming §4.3.6's four characters decides membership in a whitespace-free
//     enumeration exactly as a full collapse would.
//   - enumeration. The restriction enumerates exactly prohibited, optional and
//     required, so anything else fails the type the attribute is declared with
//     and is charged cvc-datatype-valid (§4.1.4), which states no fallback
//     clause: use="foo" must not quietly read as optional and mint an Attribute
//     Use where §3.2.2's prohibited form maps to no component at all.
//
// An absent attribute is that declared default and is NOT an error, a branch
// distinct from present-but-invalid; boolAttr and facetFixed split the same two.
func attributeUseToken(el *Element) (string, error) {
	attr, hasUse := el.attr("use")
	if !hasUse {
		return "optional", nil
	}
	lexical := attr.Value()
	switch use := collapseTrim(lexical); use {
	case "prohibited", "optional", "required":
		return use, nil
	}
	return "", xsderr.New(ruleDatatypeValid, attr.Loc(),
		"<attribute> use value %q is not one of prohibited/optional/required, the enumeration the schema for schema documents restricts xs:NMTOKEN to there", lexical)
}

// useValueConstraintOK charges src-attribute (§3.2.3) clauses 2 and 5 against one
// <attribute> element: clause 2, "If default and use are both present, use must
// have the ·actual value· optional", and clause 5, "If fixed and use are both
// present, use must not have the ·actual value· prohibited". Neither clause is
// guarded by the element's parent, but only the local and ref= forms reach it:
// both clauses need a use= present, and the top-level form may not carry one at
// all (xs:topLevelAttribute, xmlschema11-1.md:4712, enforced in run by
// rejectProhibitedAttrs).
//
// The check lives here rather than in valueConstraintOf because that helper is
// shared with <element> (charged src-element clause 1), and <element> has no use=
// attribute to consult. valueConstraintOf stays the single encoding of clause 1.
//
// The token is the ·actual value· attributeUseToken maps, so both comparisons
// below read one of the three values Appendix A enumerates: a use= outside
// {optional, prohibited, required} fails its declared type and is charged there
// first, and a padded " optional " is the value optional here as it is
// everywhere else (#1328). Neither clause performs that enumeration check itself
// — the enumeration is imposed by the schema for schema documents, which
// src-attribute is explicitly "in addition to".
func useValueConstraintOK(el *Element) error {
	use, err := attributeUseToken(el)
	if err != nil {
		return err
	}
	if _, hasDefault := el.Attr("default"); hasDefault && use != "optional" {
		return xsderr.New(ruleSrcAttribute, el.Loc(),
			"attribute has default with use=%q, but src-attribute clause 2 requires use to be optional when default is present", use)
	}
	if _, hasFixed := el.Attr("fixed"); hasFixed && use == "prohibited" {
		return xsderr.New(ruleSrcAttribute, el.Loc(),
			"attribute has fixed with use=%q, but src-attribute clause 5 forbids prohibited when fixed is present", use)
	}
	return nil
}

// rejectLocalAttributeTargetNamespace charges src-attribute clause 6
// (att-with-ns, §3.2.3, xmlschema11-1.md:868) on a local <attribute name="...">
// that writes an explicit targetNamespace: "If the targetNamespace attribute is
// present then all of the following must be true: 6.1 The name attribute is
// present. 6.2 The form attribute is absent. 6.3 If the ancestor <schema> does
// not have a targetNamespace attribute or its ·actual value· is different from
// the ·actual value· of targetNamespace of <attribute>, then all of the following
// are true: 6.3.1 <attribute> has <complexType> as an ancestor. 6.3.2
// (att-with-ns-must-be-old) There is a <restriction> ancestor between the
// <attribute> and the nearest <complexType> ancestor, and the ·actual value· of
// the base attribute of <restriction> does not ·match· the name of xs:anyType."
//
// 6.1 is charged in produceAttributeUse's hasRef arm and never here: name is
// already present on every <attribute> that reaches this function.
// produceAttributeUse rejects the neither-ref-nor-name shape under clause 3.1
// before it, and BOTH call sites sit on its !hasRef arm — produceLocalAttribute
// for every use= that maps to a component, and the prohibited return's own
// guarded call for the one that maps to none — so name is present unconditionally
// and a 6.1 branch here could not be made to fire. The ref= form, where 6.1 is
// the failing conjunct because clause 3.1 forces name absent, is rejected there
// ahead of the reference resolution (#1242).
//
// A top-level <attribute> may not write the attribute at all —
// xs:topLevelAttribute declares it use="prohibited" (xmlschema11-1.md:4713) —
// and rejectProhibitedAttrs charges that as the §5.1 grammar fault it is.
//
// 6.3's antecedent carries the clause's whole force: an attribute writing its own
// document's targetNamespace REDUNDANTLY is legal wherever it stands, needing no
// <complexType> or <restriction> ancestor, and 6.3.1/6.3.2 are read only once the
// two namespaces differ. They are conjoined, so an <extension> between the
// attribute and the nearest <complexType>, or a <restriction base="xs:anyType">,
// fails the clause as squarely as no <complexType> ancestor at all — and §3.2.2
// admits <attributeGroup> as a local attribute's ancestor (xmlschema11-1.md:841),
// which reaches no <complexType> and so fails 6.3.1 outright.
//
// The ancestor <schema>'s targetNamespace is read as the EFFECTIVE one, p.target,
// and its presence as p.chameleon()'s, for the reason
// rejectLocalElementTargetNamespace's doc gives: §F.1 task (a) writes the
// including namespace onto a chameleon document that has none of its own, and the
// constraint is stated over that transformed document (xmlschema11-1.md:630).
//
// Every term is src-element clause 4's, element for attribute, so the ancestor
// walk and the ·match· are the same two helpers — nearestComplexTypeAndRestriction
// and baseMatchesAnyType — asked of the <attribute> instead (STYLE T4). The rule
// ID is not: this is ruleSrcAttribute's clause, not ruleSrcElement's.
func (p *producer) rejectLocalAttributeTargetNamespace(el *Element) error {
	tns, ok := el.Attr("targetNamespace")
	if !ok {
		return nil
	}
	if _, hasForm := el.Attr("form"); hasForm {
		return xsderr.New(ruleSrcAttribute, el.Loc(),
			"the local <attribute> carries both a targetNamespace and a form attribute, but src-attribute clause 6.2 admits no form attribute when targetNamespace is present")
	}
	// 6.3's antecedent, which fails — leaving 6.3.1 and 6.3.2 unread — only when
	// the ancestor <schema> HAS a targetNamespace and the attribute wrote that same
	// one.
	_, own := p.schemaElem.Attr("targetNamespace")
	if (own || p.chameleon()) && tns == p.target {
		return nil
	}
	ct, restriction := nearestComplexTypeAndRestriction(el)
	if ct == nil {
		return xsderr.New(ruleSrcAttribute, el.Loc(),
			"the local <attribute> declares targetNamespace %q, which is not the ancestor <schema>'s, but has no <complexType> ancestor, which src-attribute clause 6.3.1 requires of an attribute declaring a namespace of its own", tns)
	}
	if restriction == nil {
		return xsderr.New(ruleSrcAttribute, el.Loc(),
			"the local <attribute> declares targetNamespace %q, which is not the ancestor <schema>'s, but no <restriction> stands between it and the <complexType> at %s, which src-attribute clause 6.3.2 (att-with-ns-must-be-old) requires of an attribute declaring a namespace of its own",
			tns, ct.Loc())
	}
	if !p.baseMatchesAnyType(restriction) {
		return nil
	}
	return xsderr.New(ruleSrcAttribute, el.Loc(),
		"the local <attribute> declares targetNamespace %q, which is not the ancestor <schema>'s, but the <restriction> at %s standing between it and its <complexType> ancestor has base=%q, which ·matches· xs:anyType — src-attribute clause 6.3.2 (att-with-ns-must-be-old) requires a base naming any other type",
		tns, restriction.Loc(), attrOr(restriction, "base"))
}

// rejectAttributeTypeAndSimpleType charges src-attribute clause 4 (§3.2.3,
// xmlschema11-1.md:868) on a local <attribute name="...">: "The type attribute
// and a simpleType child element must not both be present."
//
// The clause's antecedent carries no use= qualifier, so both of the name= form's
// paths call it — produceLocalAttribute for every use= that maps to a
// declaration, and produceAttributeUse's use="prohibited" return for the one that
// maps to none (#1270) — and the predicate is written once (STYLE D3). The ref=
// arm reaches neither call site carrying both: clause 3.2 rejects a ref=
// attribute writing type=, and again one carrying a <simpleType> child, ahead of
// them.
func rejectAttributeTypeAndSimpleType(el *Element) error {
	_, hasType := el.Attr("type")
	if !hasType || childElement(el, xsd.XMLSchemaNS, "simpleType") == nil {
		return nil
	}
	return xsderr.New(ruleSrcAttribute, el.Loc(),
		"attribute has both a type attribute and an inline <simpleType> child, but src-attribute clause 4 forbids both")
}

// produceLocalAttribute maps the sibling local Attribute Declaration of a local
// <attribute> (§3.2.2.2, {scope} = local, {value constraint} always absent on the
// declaration — any default/fixed feeds the Attribute Use, #70). Its {type
// definition} is mapped by declaredType over §3.2.2.2's three tiers: the
// inline <simpleType> child (#229), the type= reference, or xs:anySimpleType.
// src-attribute clause 4 (§3.2.3) rejects the both-present case first, by
// rejectAttributeTypeAndSimpleType — for every use= but prohibited, which maps to
// no declaration and takes clause 4 from produceAttributeUse's own guarded call
// to that same helper, exactly as it takes clause 6 (#1270).
//
// Both clauses charged here are behind produceAttributeUse's s4s walk, which
// every local <attribute> passes through before reaching this function — the run
// order checkS4SChildOrder's doc records for every src-* charge. Clause 4 is the
// both-present fault src-element clause 3 charges on the element side, behind the
// walk there too (#1246).
//
// src-attribute clause 6 (att-with-ns) is charged here too, ahead of the name, by
// rejectLocalAttributeTargetNamespace — for every use= but prohibited, which maps
// to no declaration and takes its charge from produceAttributeUse's own guarded
// call instead (#1243). Until that charge ran the attribute was read by
// localTargetNS alone, which mints the name in whatever namespace it declares
// without asking whether the clause admits it.
//
// no-xmlns (§3.2.6.3) follows the name too, by rejectXmlnsName, and precedes
// no-xsi: it reads the {name} declarationName just minted and nothing else, so
// the local form gives it no namespace resolution to wait behind (#1465).
//
// no-xsi (§3.2.6.4) follows the name, by rejectXSITargetNamespace, because the
// namespace it tests is the one localTargetNS resolves from this attribute's own
// form/targetNamespace and the <schema>'s attributeFormDefault (§3.2.2.2) — an
// unqualified local attribute lands in no namespace and is admitted however the
// enclosing document is namespaced (#1446).
//
// scopeParent is the containing <complexType>'s or <attributeGroup>'s component,
// supplied by the caller and never recomputed from the element here: the ancestor
// axis reaches that ancestor's ELEMENT, as nearestComplexTypeAndRestriction walks
// it for clause 6, never the component it maps to. It is a required parameter:
// every local attribute declaration has a {scope}.{parent} (§3.2.1 sc_a), so
// there is no path through this function that does not build the scope from it.
func (p *producer) produceLocalAttribute(el *Element, scopeParent xsd.AttributeScopeParent) (xsd.AttributeDeclaration, error) {
	if err := rejectAttributeTypeAndSimpleType(el); err != nil {
		return xsd.AttributeDeclaration{}, err
	}
	if err := p.rejectLocalAttributeTargetNamespace(el); err != nil {
		return xsd.AttributeDeclaration{}, err
	}
	tns, err := p.localTargetNS(el, "attributeFormDefault")
	if err != nil {
		return xsd.AttributeDeclaration{}, err
	}
	qname, err := declarationName(el, tns)
	if err != nil {
		return xsd.AttributeDeclaration{}, err
	}
	if err := rejectXmlnsName(qname.Local, el); err != nil {
		return xsd.AttributeDeclaration{}, err
	}
	if err := rejectXSITargetNamespace(qname, el); err != nil {
		return xsd.AttributeDeclaration{}, err
	}
	typeDef, err := p.declaredType(el, anySimpleTypeName)
	if err != nil {
		return xsd.AttributeDeclaration{}, err
	}
	inheritable, _, err := boolAttr(el, "inheritable")
	if err != nil {
		return xsd.AttributeDeclaration{}, err
	}
	scope, err := xsd.NewAttributeLocalScope(el.Loc(), scopeParent)
	if err != nil {
		return xsd.AttributeDeclaration{}, err
	}
	return xsd.NewAttributeDeclaration(el.Loc(), qname, typeDef, scope, nil, inheritable)
}

// produceWildcard maps an <any>/<anyAttribute> to a Wildcard (§3.10.2.2). It
// enforces src-wildcard (§3.10.3): namespace and notNamespace must not both be
// present.
func (p *producer) produceWildcard(el *Element) (xsd.Wildcard, error) {
	nc, err := p.namespaceConstraint(el)
	if err != nil {
		return xsd.Wildcard{}, err
	}
	process := xsd.ProcessStrict
	if pc, ok := el.attr("processContents"); ok {
		process, err = processContentsOf(pc.Value(), pc.Loc())
		if err != nil {
			return xsd.Wildcard{}, err
		}
	}
	return xsd.NewWildcard(el.Loc(), nc, process)
}

// namespaceConstraint maps the namespace/notNamespace/notQName attributes of an
// <any>/<anyAttribute> to a Namespace Constraint (§3.10.2.2).
func (p *producer) namespaceConstraint(el *Element) (xsd.NamespaceConstraint, error) {
	ns, hasNS := el.Attr("namespace")
	notNS, hasNotNS := el.Attr("notNamespace")
	if hasNS && hasNotNS {
		return xsd.NamespaceConstraint{}, xsderr.New(ruleSrcWildcard, el.Loc(),
			"wildcard has both namespace and notNamespace, but src-wildcard forbids both")
	}
	disallowed, keywords, err := p.disallowedNames(el)
	if err != nil {
		return xsd.NamespaceConstraint{}, err
	}
	variety, namespaces := p.namespaceVarietyAndSet(ns, hasNS, notNS, hasNotNS)
	return xsd.NewNamespaceConstraint(el.Loc(), variety, namespaces, disallowed, keywords)
}

// namespaceVarietyAndSet computes {variety} and {namespaces} (§3.10.2.2):
//   - neither namespace nor notNamespace present → any, empty set;
//   - namespace="##any" → any, empty set;
//   - namespace="##other" → not, {·absent·} plus the target namespace if present;
//   - otherwise (a namespace/notNamespace token list) → enumeration for namespace
//     or not for notNamespace, with ##targetNamespace/##local substituted.
//
// It takes namespace's ·actual value· once, through collapseTrim, and uses that
// one value for both the keyword test and the token split. The mapping's
// ##any/##other clauses do not themselves say ·actual value·: the reading comes
// from the attribute's type, xs:namespaceList (Appendix A), whose
// specialNamespaceList member restricts xs:token (whiteSpace collapse), so
// namespace=" ##any " is ##any and namespace=" ##other " is ##other. The test is
// case-sensitive ("##Any" is an enumeration member), and an empty or
// whitespace-only value is an enumeration of the empty set, not a keyword.
func (p *producer) namespaceVarietyAndSet(ns string, hasNS bool, notNS string, hasNotNS bool) (xsd.NamespaceConstraintVariety, []xsd.Namespace) {
	if !hasNS && !hasNotNS {
		return xsd.NamespaceConstraintAny, nil
	}
	nsValue := collapseTrim(ns)
	if hasNS && nsValue == "##any" {
		return xsd.NamespaceConstraintAny, nil
	}
	if hasNS && nsValue == "##other" {
		set := []xsd.Namespace{xsd.NamespaceName("")}
		if p.target != "" {
			set = append(set, xsd.NamespaceName(p.target))
		}
		return xsd.NamespaceConstraintNot, set
	}
	list := nsValue
	variety := xsd.NamespaceConstraintEnumeration
	if hasNotNS {
		list = notNS
		variety = xsd.NamespaceConstraintNot
	}
	var set []xsd.Namespace
	for _, tok := range xmlSpaceFields(list) {
		switch tok {
		case "##targetNamespace":
			set = append(set, xsd.NamespaceName(p.target))
		case "##local":
			set = append(set, xsd.NamespaceName(""))
		default:
			set = append(set, xsd.NamespaceName(tok))
		}
	}
	return variety, set
}

// disallowedNames maps a notQName attribute to the two halves of {disallowed
// names} (§3.10.2.2): its literal QName items, and its ##defined/##definedSibling
// keyword tokens mapped to the component keywords defined/sibling. Note the
// asymmetry the spec fixes at §3.10.2.2: ##definedSibling maps to the keyword
// sibling ALONE, not to both keywords.
//
// An unrecognized ##-prefixed token is rejected. That is not a bespoke Structures
// constraint — src-wildcard (§3.10.3) says nothing about notQName's content — but
// an ordinary datatype-validity failure of the attribute against the type the
// schema for schema documents declares for it (§3.10.2): xs:qnameList for <any>,
// whose token member type enumerates exactly ##defined and ##definedSibling, and
// xs:qnameListA for <anyAttribute>, which enumerates only ##defined. So
// notQName="##definedSibling" on an <anyAttribute> is rejected HERE, by that
// enumeration, and is the machine-checkable form of w-props-correct clause 5 —
// the parser deliberately does not also repeat clause 5 as a component check (one
// enforcement point); xsd.NewComplexType/NewAttributeGroupDefinition still charge
// clause 5 on the programmatic-construction path that bypasses this producer.
//
// The literal-QName arm is equally hard-failing: a member whose prefix has no
// in-scope binding cannot be mapped to a QName value at all (Datatypes §3.3.18),
// and §3.10.2.2 offers no fallback that omits it, so dropping it would silently
// shrink {disallowed names} into a more permissive wildcard than the schema
// declares. bindQName's src-resolve error is propagated verbatim.
//
// It is bindQName rather than resolveQName precisely because §3.10.2's mapping
// takes these items as QName VALUES and never ·resolves· them to components: a
// notQName naming a namespace this document did not <import> blocks a name, it
// does not reach for a declaration, so src-resolve clause 4's licensing test
// (licensedNamespace) does not apply to it.
func (p *producer) disallowedNames(el *Element) ([]xsd.QName, []xsd.DisallowedNameKeyword, error) {
	notQName, ok := el.attr("notQName")
	if !ok {
		return nil, nil, nil
	}
	attributeWildcard := el.Name().Local() == "anyAttribute"
	var names []xsd.QName
	var keywords []xsd.DisallowedNameKeyword
	for _, tok := range xmlSpaceFields(notQName.Value()) {
		if strings.HasPrefix(tok, "##") {
			kw, err := disallowedNameKeywordOf(tok, attributeWildcard, notQName.Loc())
			if err != nil {
				return nil, nil, err
			}
			keywords = append(keywords, kw)
			continue
		}
		qn, err := p.bindQName(el, tok, "notQName")
		if err != nil {
			return nil, nil, err
		}
		names = append(names, qn)
	}
	return names, keywords, nil
}

// disallowedNameKeywordOf maps one ##-prefixed notQName token to its §3.10.1
// component keyword. attributeWildcard selects the declared type of the notQName
// attribute being validated (§3.10.2): xs:qnameListA for <anyAttribute>, which
// enumerates only ##defined, versus xs:qnameList for <any>, which also enumerates
// ##definedSibling. A token outside the applicable enumeration fails that type,
// charged cvc-datatype-valid — the generic "attribute value is not valid against
// its declared simple type" rule, not a wildcard-specific one.
func disallowedNameKeywordOf(tok string, attributeWildcard bool, loc xsderr.Loc) (xsd.DisallowedNameKeyword, error) {
	if tok == "##defined" {
		return xsd.DisallowedNameDefined, nil
	}
	if tok == "##definedSibling" && !attributeWildcard {
		return xsd.DisallowedNameSibling, nil
	}
	declaredType := "xs:qnameList"
	enumerated := "##defined or ##definedSibling"
	if attributeWildcard {
		declaredType, enumerated = "xs:qnameListA", "##defined"
	}
	return 0, xsderr.New(ruleDatatypeValid, loc,
		"notQName token %q is not valid against %s, whose keyword member type enumerates only %s", tok, declaredType, enumerated)
}

// formChoiceQualified is the ·actual value· of an xs:formChoice-typed attribute,
// as the truth of "= qualified" that §3.2.2.2 and §3.3.2.3 case 2 test. Appendix
// A declares xs:formChoice as an xs:NMTOKEN restriction enumerating exactly
// qualified and unqualified (xmlschema11-1.md:4478), and NMTOKEN fixes
// whiteSpace to collapse, applied BEFORE lexical-space membership is tested
// (§4.1.4) — so form=" qualified " is the value qualified, and anything outside
// the two-value enumeration fails the declared type and is charged
// cvc-datatype-valid rather than reading as unqualified. attr names the
// attribute read, one of form, elementFormDefault and attributeFormDefault, all
// three declared with this one type.
func formChoiceQualified(lexical string, loc xsderr.Loc, attr string) (bool, error) {
	switch collapseTrim(lexical) {
	case "qualified":
		return true, nil
	case "unqualified":
		return false, nil
	}
	return false, xsderr.New(ruleDatatypeValid, loc,
		"%s value %q is not one of qualified/unqualified, the enumeration the schema for schema documents restricts xs:formChoice to", attr, lexical)
}

// localTargetNS computes a local element/attribute declaration's {target
// namespace} (§3.3.2.3 / §3.2.2.2): an explicit targetNamespace attribute wins,
// else form= (qualified → the schema target, unqualified → absent), else the
// schema's *FormDefault (formDefaultAttr is "elementFormDefault" or
// "attributeFormDefault"), defaulting to absent (unqualified).
//
// form= is charged HERE, at the declaration that writes it, the way
// processContentsOf and openContentModeOf charge their own keyword attributes:
// every form= the schema for schema documents admits at all sits on a local
// declaration this function maps — rejectProhibitedAttrs rejects the top-level
// form outright — so no value escapes the check by never being read.
//
// The *FormDefault read is the other way round — it is a plain comparison
// against the collapsed lexical and charges NOTHING, because checkFormDefaults
// has already judged both attributes of every document of the assembly before
// any component is produced. That leaves exactly qualified and unqualified
// reachable here, so testing for the first is testing for the second's
// complement (#1328).
func (p *producer) localTargetNS(el *Element, formDefaultAttr string) (string, error) {
	if tns, ok := el.Attr("targetNamespace"); ok {
		return tns, nil
	}
	if form, ok := el.attr("form"); ok {
		qualified, err := formChoiceQualified(form.Value(), form.Loc(), "form")
		if err != nil {
			return "", err
		}
		if qualified {
			return p.target, nil
		}
		return "", nil
	}
	if fd, ok := p.schemaElem.Attr(formDefaultAttr); ok && collapseTrim(fd) == "qualified" {
		return p.target, nil
	}
	return "", nil
}

// occursOf maps the minOccurs/maxOccurs attributes to an Occurs (§3.9.2), each
// defaulting to 1. elided is true for the minOccurs=maxOccurs=0 case, which the
// XML mapping rules say "maps to no component at all" (§3.7.2/§3.8.2/§3.9.2) — the
// caller omits the particle entirely rather than building a vacuous Occurs{0,0}.
//
// Each value reaches xsd.NewOccurs saturated at math.MaxInt (occursNumeral.int),
// which can merge an inverted pair into an equal one and would name a saturated
// value the document never spelled. p-props-correct clause 2.1 (§3.9.6.1) is
// therefore decided here for every numeric pair, on the unsaturated numerals,
// and the message names those values (ruleParticleCorrect).
func occursOf(el *Element) (occ xsd.Occurs, elided bool, err error) {
	minN := occursNumeral("1")
	if minA, ok := el.attr("minOccurs"); ok {
		minN, err = nonNegativeNumeral(minA.Value(), minA.Loc(), "minOccurs")
		if err != nil {
			return xsd.Occurs{}, false, err
		}
	}
	min := minN.int()
	maxA, hasMax := el.attr("maxOccurs")
	if hasMax && collapseTrim(maxA.Value()) == "unbounded" {
		occ, err = xsd.NewUnboundedOccurs(el.Loc(), min)
		return occ, false, err
	}
	maxN := occursNumeral("1")
	if hasMax {
		maxN, err = nonNegativeNumeral(maxA.Value(), maxA.Loc(), "maxOccurs")
		if err != nil {
			return xsd.Occurs{}, false, err
		}
	}
	max := maxN.int()
	if min == 0 && max == 0 {
		return xsd.Occurs{}, true, nil
	}
	if minN.compare(maxN) > 0 {
		return xsd.Occurs{}, false, xsderr.New(ruleParticleCorrect, el.Loc(),
			"particle {min occurs} %s is greater than {max occurs} %s", minN, maxN)
	}
	occ, err = xsd.NewOccurs(el.Loc(), min, max)
	return occ, false, err
}

// occursNumeral is one xs:nonNegativeInteger value (§3.4.20) spelled as its
// canonical decimal numeral: ASCII digits with no sign and no leading zero, "0"
// alone for zero. The value space has no upper bound, so a value above
// math.MaxInt is held exactly; only the host int that int returns is bounded.
type occursNumeral string

// int returns the value as a host int, saturated at math.MaxInt. A value past the
// host's capacity is not an error — the literal is datatype-valid however large
// it is — and math.MaxInt is not xsd.Occurs' unbounded sentinel.
func (n occursNumeral) int() int {
	v, err := strconv.Atoi(string(n))
	if err != nil {
		// A canonical numeral is in Atoi's syntax, so the only error left is
		// strconv.ErrRange: the value is past math.MaxInt.
		return math.MaxInt
	}
	return v
}

// compare orders two values numerically: negative, zero or positive as n is less
// than, equal to or greater than m. A canonical numeral has no leading zero, so
// the longer is the larger and equal lengths order as strings.
func (n occursNumeral) compare(m occursNumeral) int {
	return cmp.Or(cmp.Compare(len(n), len(m)), strings.Compare(string(n), string(m)))
}

// nonNegativeNumeral reads an xs:nonNegativeInteger-valued occurrence attribute,
// charging cvc-datatype-valid (Datatypes §4.1.4) on a lexical outside that type:
// the schema for schema documents declares minOccurs type="xs:nonNegativeInteger"
// and maxOccurs type="xs:allNNI" (Appendix A's occurs attribute group), so such a
// lexical is an attribute failing its own declared type, which is what
// ruleDatatypeValid names. The test is lexical-space membership alone
// (nonNegativeIntegerLexical): xs:nonNegativeInteger is unbounded (§3.4.20), so a
// literal past math.MaxInt is datatype-valid and is never charged here (#1780).
//
// NOT p-props-correct (§3.9.6.1). That constraint quantifies over an existing
// particle's PROPERTIES, and occursOf returns this error before reaching
// xsd.NewOccurs/xsd.NewUnboundedOccurs, so no Occurs and no Particle is ever
// built for it to constrain. p-props-correct clause 2.1 — {min occurs} greater
// than a numeric {max occurs} — is charged by occursOf on the two values this
// helper has already read, before saturation (#932).
func nonNegativeNumeral(lexical string, loc xsderr.Loc, attr string) (occursNumeral, error) {
	s := collapseTrim(lexical)
	if _, ok := nonNegativeIntegerLexical(s); !ok {
		return "", xsderr.New(ruleDatatypeValid, loc,
			"%s value %q is not a nonNegativeInteger", attr, lexical)
	}
	digits := strings.TrimLeft(strings.TrimLeft(s, "+-"), "0")
	if digits == "" {
		return "0", nil
	}
	return occursNumeral(digits), nil
}

// nonNegativeInt reads an xs:nonNegativeInteger-valued occurrence attribute as
// nonNegativeNumeral does, charging the same rule, and returns the value
// saturated at math.MaxInt (occursNumeral.int). Its callers compare the value
// with 1 alone, which saturation cannot mislead.
func nonNegativeInt(lexical string, loc xsderr.Loc, attr string) (int, error) {
	n, err := nonNegativeNumeral(lexical, loc, attr)
	if err != nil {
		return 0, err
	}
	return n.int(), nil
}

// allOccursGrammar enforces the occurrence grammar the schema for schema
// documents gives the <all> ELEMENT (§3.8.2's XML representation summary,
// formalized in Appendix A's xs:complexType "all"): minOccurs restricts
// xs:nonNegativeInteger and maxOccurs restricts xs:allNNI, each by an enumeration
// admitting only 0 and 1 — so "unbounded", legal on <choice>/<sequence>, is
// excluded here along with every integer above 1. An absent attribute takes the
// declared default 1 and passes.
//
// §3.8.3 lists "None as such" for <all>'s Schema Representation Constraints, so
// the fault carries no src-*/cos-* rule: it is charged cvc-datatype-valid, the
// generic "attribute value is not valid against its declared type" rule. It is
// NOT cos-all-limited, which constrains where the resulting particle may appear
// rather than what the element's attributes may say, and not p-props-correct,
// which constrains a particle's properties and is charged by occursOf for
// min > max alone.
//
// Only the content-model <all> is checked: on the <all> body of a top-level named
// <group>, Appendix A's xs:namedGroup makes both attributes use="prohibited", a
// presence fault of a different declaration that rejectNamedGroupBodyOccurs
// charges instead — and charges for minOccurs="0" and maxOccurs="1" too, which
// this enumeration admits.
func allOccursGrammar(el *Element) error {
	if minA, ok := el.attr("minOccurs"); ok {
		if err := allOccursEnum(minA.Value(), minA.Loc(), "minOccurs"); err != nil {
			return err
		}
	}
	maxA, ok := el.attr("maxOccurs")
	if !ok {
		return nil
	}
	if collapseTrim(maxA.Value()) == "unbounded" {
		return xsderr.New(ruleDatatypeValid, maxA.Loc(),
			`<all> maxOccurs is "unbounded", but the schema for schema documents restricts it to the enumeration 0, 1`)
	}
	return allOccursEnum(maxA.Value(), maxA.Loc(), "maxOccurs")
}

// allOccursEnum checks one numeric <all> occurrence lexical against the {0,1}
// enumeration. The enumeration facet compares VALUES, so "01" is the value 1 and
// passes, matching how occursOf reads the same attribute; a lexical that is not a
// nonNegativeInteger at all fails its base type first, charged cvc-datatype-valid
// by nonNegativeInt — the same rule this function charges, since both are one
// attribute failing one declared type.
func allOccursEnum(lexical string, loc xsderr.Loc, attr string) error {
	n, err := nonNegativeInt(lexical, loc, attr)
	if err != nil {
		return err
	}
	if n > 1 {
		return xsderr.New(ruleDatatypeValid, loc,
			"<all> %s value %q is outside the enumeration 0, 1 that the schema for schema documents declares for it", attr, lexical)
	}
	return nil
}

// processContentsOf maps a processContents lexical to a ProcessContents token,
// charging cvc-datatype-valid (Datatypes §4.1.4) on an out-of-enumeration value:
// the schema for schema documents declares processContents as an xs:NMTOKEN
// restricted to the enumeration skip/lax/strict (Appendix A's wildcard attribute
// group, xmlschema11-1.md:5346-5353), so a lexical outside that value space is an
// attribute failing its own declared type, which is what ruleDatatypeValid names.
//
// NOT w-props-correct (§3.10.6.1). That constraint quantifies over an existing
// Wildcard's PROPERTIES, and produceWildcard returns this error before reaching
// xsd.NewWildcard, so no Wildcard is ever built for it to constrain. Clause 1 —
// {process contents} outside skip/lax/strict — is charged by that constructor, on
// the token this helper has already mapped (#950).
func processContentsOf(lexical string, loc xsderr.Loc) (xsd.ProcessContents, error) {
	switch collapseTrim(lexical) {
	case "skip":
		return xsd.ProcessSkip, nil
	case "strict":
		return xsd.ProcessStrict, nil
	case "lax":
		return xsd.ProcessLax, nil
	}
	return 0, xsderr.New(ruleDatatypeValid, loc,
		"wildcard processContents %q is not one of skip/strict/lax", lexical)
}

// compositorOf maps an <all>/<choice>/<sequence> local name to its Compositor.
// ok is false for <group> (a reference, mapped by produceGroupRefParticle) and
// any other name.
func compositorOf(local string) (xsd.Compositor, bool) {
	switch local {
	case "all":
		return xsd.CompositorAll, true
	case "choice":
		return xsd.CompositorChoice, true
	case "sequence":
		return xsd.CompositorSequence, true
	}
	return 0, false
}

// modelGroupChild returns el's first <all>/<choice>/<sequence>/<group> child (the
// model-group child of a <complexType>/<restriction>/<extension>), or nil.
func modelGroupChild(el *Element) *Element {
	for _, child := range el.Children() {
		c, ok := child.(*Element)
		if !ok || c.Name().Space() != xsd.XMLSchemaNS {
			continue
		}
		switch c.Name().Local() {
		case "all", "choice", "sequence", "group":
			return c
		}
	}
	return nil
}

// hasParticleChild reports whether group has any non-<annotation> element child
// (an element/any/group/choice/sequence), i.e. whether it is non-empty for the
// §3.4.2.3.3 clause 2 empty-content tests.
func hasParticleChild(group *Element) bool {
	for _, child := range group.Children() {
		c, ok := child.(*Element)
		if !ok || c.Name().Space() != xsd.XMLSchemaNS {
			continue
		}
		if c.Name().Local() != "annotation" {
			return true
		}
	}
	return false
}

// minOccursZero reports whether el's minOccurs is lexically 0.
//
// GAP(xsd): clause 2.1.3 tests the ·actual value·, so minOccurs="00" — value 0 —
// is not elided here, though occursOf's own both-zero test parses it and is.
// This is maxOccursZero's gap one clause over; #929 owns closing both.
func minOccursZero(el *Element) bool {
	v, ok := el.Attr("minOccurs")
	return ok && collapseTrim(v) == "0"
}

// maxOccursZero reports whether el's maxOccurs is lexically 0.
//
// GAP(xsd): clause 2.1.4 tests the ·actual value·, so maxOccurs="00" — value 0 —
// is not elided here, though occursOf's own both-zero test parses it and is.
// Reproduced through parser.Parse on #901; #929 owns closing it.
func maxOccursZero(el *Element) bool {
	v, ok := el.Attr("maxOccurs")
	return ok && collapseTrim(v) == "0"
}

// attrOr returns el's attribute local value, or the empty string when absent.
func attrOr(el *Element, local string) string {
	v, _ := el.Attr(local)
	return v
}

// boolAttr reads an xs:boolean-valued schema attribute as its ·actual value·
// (key-vv), reporting presence separately for the callers whose rule asks
// whether the attribute was written rather than what it says. The two mapping
// stages run in the order §4.1.4 fixes:
//
//   - pre-lexical. xs:boolean fixes whiteSpace to collapse (§3.3.2.3, §4.3.6)
//     and the facet is applied BEFORE lexical-space membership is tested, so
//     mixed=" 1 " is the value true. collapseTrim reads that ·actual value·, and
//     its doc carries the proof that trimming §4.3.6's four characters decides
//     booleanRep membership exactly as a full collapse would.
//   - lexical. booleanRep ::= 'true' | 'false' | '1' | '0' (§3.3.2.2), case
//     sensitive: "TRUE" and "yes" are not in the lexical space. Anything outside
//     those four is charged cvc-datatype-valid (§4.1.4), which states no fallback
//     clause — an out-of-space literal must not quietly become false.
//
// An absent attribute is the property's default and is NOT an error, a branch
// distinct from present-but-invalid; facetFixed splits the same two. On a fault
// both bools are their zero values and the error is the only meaningful return.
func boolAttr(el *Element, local string) (val bool, present bool, err error) {
	attr, ok := el.attr(local)
	if !ok {
		return false, false, nil
	}
	lexical := attr.Value()
	switch collapseTrim(lexical) {
	case "true", "1":
		return true, true, nil
	case "false", "0":
		return false, true, nil
	}
	return false, false, xsderr.New(ruleDatatypeValid, attr.Loc(),
		"<%s> %s value %q is not in the lexical space of xs:boolean (true, false, 1, 0) that the schema for schema documents declares for it",
		el.Name().Local(), local, lexical)
}
