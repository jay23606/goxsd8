package validate

import (
	"slices"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// The fixtures below drive the RECURSIVE half of key-sva (§3.3.4.6) clause 3:
// a child of an already-assessed element is assessed against the ·governing
// element declaration· the parent's own content model ·attributed· it to
// (§3.4.4.4), one arm per [xsd.Attribution] variant. Every schema is built
// through the exported constructors and FINALIZED, so each content model is
// one cos-nonambig admitted and each substitution group edge is one
// e-props-correct clause 4 admitted.

// dType builds a NAMED complex type over content, carrying uses, derived from
// base by method. An empty base name leaves {base type definition} ·absent·,
// which is what every fixture but the substitution pair wants.
func dType(t *testing.T, name, base string, method xsd.DerivationMethod, uses []xsd.AttributeUse, content xsd.ContentType) xsd.ComplexType {
	t.Helper()
	var baseName xsd.QName
	if base != "" {
		baseName = xsd.QName{Local: base}
	}
	ct, err := xsd.NewComplexType(xsderr.Loc{}, xsd.QName{Local: name}, baseName, nil,
		method, false, attrContent(uses), nil, nil, content, nil, nil)
	if err != nil {
		t.Fatalf("building the %s complex type: %v", name, err)
	}
	return ct
}

// dTyped wraps a LOCAL element declaration named local, whose {type definition}
// names typ, in a 1..1 particle scoped to the complex type named parent.
func dTyped(t *testing.T, parent, local, typ string) xsd.Particle {
	t.Helper()
	scope, err := xsd.NewLocalScope(xsderr.Loc{}, xsd.ComplexTypeScopeParent{Name: xsd.QName{Local: parent}})
	if err != nil {
		t.Fatalf("NewLocalScope: %v", err)
	}
	d, err := xsd.NewElementDeclaration(xsderr.Loc{}, xsd.QName{Local: local},
		xsd.TypeDefinitionRef{Name: xsd.QName{Local: typ}}, nil, scope,
		nil, false, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("building the %s element declaration: %v", local, err)
	}
	return dParticleOver(t, xsd.ResolvedTerm{Term: d})
}

// dWildcard wraps a ·complete wildcard· with the given {process contents} in a
// 1..1 particle: {variety} any, so every name reaches cvc-wildcard's other
// clauses and none is turned away by the namespace constraint.
func dWildcard(t *testing.T, pc xsd.ProcessContents) xsd.Particle {
	t.Helper()
	c, err := xsd.NewNamespaceConstraint(xsderr.Loc{}, xsd.NamespaceConstraintAny, nil, nil, nil)
	if err != nil {
		t.Fatalf("building the namespace constraint: %v", err)
	}
	w, err := xsd.NewWildcard(xsderr.Loc{}, c, pc)
	if err != nil {
		t.Fatalf("building the %s wildcard: %v", pc, err)
	}
	return dParticleOver(t, xsd.ResolvedTerm{Term: w})
}

// dParticleOver wraps term in a 1..1 particle.
func dParticleOver(t *testing.T, term xsd.TermOrRef) xsd.Particle {
	t.Helper()
	o, err := xsd.NewOccurs(xsderr.Loc{}, 1, 1)
	if err != nil {
		t.Fatalf("NewOccurs: %v", err)
	}
	p, err := xsd.NewParticle(xsderr.Loc{}, o, term)
	if err != nil {
		t.Fatalf("NewParticle: %v", err)
	}
	return p
}

// dTopLevel is a top-level element declaration named local whose {type
// definition} names typ, with the given {substitution group affiliations}.
func dTopLevel(t *testing.T, local, typ string, affiliations ...xsd.QName) xsd.ElementDeclaration {
	t.Helper()
	d, err := xsd.NewElementDeclaration(xsderr.Loc{}, xsd.QName{Local: local},
		xsd.TypeDefinitionRef{Name: xsd.QName{Local: typ}}, nil, xsd.NewGlobalScope(),
		nil, false, nil, affiliations, nil, false, nil)
	if err != nil {
		t.Fatalf("building the top-level %s element declaration: %v", local, err)
	}
	return d
}

// dSchema declares "root" over a RootType whose {content type} is an
// element-only sequence of particles, seeding whatever else the fixture needs
// through extra.
func dSchema(t *testing.T, extra func(*xsd.SchemaBuilder), particles ...xsd.Particle) *xsd.Schema {
	t.Helper()
	return cSchemaFrom(t, dType(t, "RootType", "", xsd.DerivationRestriction, nil,
		cSequence(t, false, particles...)), extra)
}

// dElem is an element information item named local at line, holding kids.
func dElem(local string, line int, kids ...Child) *testElement {
	return &testElement{name: xsd.QName{Local: local}, kids: kids, loc: loc(line, 1)}
}

// A child ·attributed to· an element particle is assessed against that
// particle's {term}, its own [[children]] against that declaration's {content
// type} — recursively, so a defect TWO levels below the validation root is
// charged at its own position (key-sva (§3.3.4.6) clause 3.1 over
// key-governing-ed clause 2).
func TestDescendantContentIsAssessedAtDepth(t *testing.T) {
	schema := dSchema(t, func(b *xsd.SchemaBuilder) {
		b.AddType(dType(t, "KidType", "", xsd.DerivationRestriction, nil,
			cSequence(t, false, dTyped(t, "KidType", "grand", "GrandType"))))
		b.AddType(dType(t, "GrandType", "", xsd.DerivationRestriction, nil,
			cSequence(t, false, cParticle(t, "leaf", 1, 1))))
	}, dTyped(t, "RootType", "kid", "KidType"))

	// <root><kid><grand><leaf/></grand></kid></root>: every level's sequence is
	// exactly what its particle admits.
	wantSilence(t, cAssess(t, schema, dElem("root", 1,
		ElementChild(dElem("kid", 2, ElementChild(dElem("grand", 3,
			ElementChild(dElem("leaf", 4)))))))),
		"each descendant's [[children]] satisfy its ·governing type definition·")

	// The same tree with <grand> empty: GrandType's particle wants a <leaf>,
	// and the charge carries the position it ran out at — two levels down.
	got := cAssess(t, schema, dElem("root", 1,
		ElementChild(dElem("kid", 2, ElementChild(dElem("grand", 3))))))

	wantContentCharge(t, got, "cvc-complex-content", "1", loc(3, 1))
}

// The attribute half descends with it: a descendant's {required} attribute uses
// are cvc-complex-type clause 3's exactly as the root's are.
func TestDescendantAttributesAreAssessedAtDepth(t *testing.T) {
	schema := dSchema(t, func(b *xsd.SchemaBuilder) {
		b.AddType(dType(t, "KidType", "", xsd.DerivationRestriction,
			[]xsd.AttributeUse{aUse(t, "req", true, nil)}, xsd.EmptyContent{}))
	}, dTyped(t, "RootType", "kid", "KidType"))

	kid := dElem("kid", 2)
	kid.attrs = []Attribute{&testAttribute{name: local("req"), value: "v", loc: loc(2, 10)}}
	wantSilence(t, cAssess(t, schema, dElem("root", 1, ElementChild(kid))),
		"the descendant carries the attribute its {required} use names")

	got := cAssess(t, schema, dElem("root", 1, ElementChild(dElem("kid", 2))))

	wantContentCharge(t, got, "cvc-complex-type", "3", loc(2, 1))
	if !strings.Contains(got[0].Msg, "req") {
		t.Errorf("Msg = %q, want it to name the missing attribute", got[0].Msg)
	}
}

// A child admitted through cvc-accept clause 2.3.2 is assessed against the
// ·substituting declaration· and never against the particle's own D: the
// ·context-determined declaration· of a substituting item is the member's
// (§3.4.4.4 over key-governing-ed clause 2), and the head's {content type}
// would reject content the member's admits.
func TestDescendantSubstitutedForItsHeadIsAssessedAgainstTheMember(t *testing.T) {
	head := dType(t, "HeadType", "", xsd.DerivationRestriction, nil, xsd.EmptyContent{})
	schema := dSchema(t, func(b *xsd.SchemaBuilder) {
		b.AddType(head)
		b.AddType(dType(t, "MemberType", "HeadType", xsd.DerivationExtension, nil,
			cSequence(t, false, cParticle(t, "grand", 1, 1))))
		b.AddElement(dTopLevel(t, "kid", "HeadType"))
		b.AddElement(dTopLevel(t, "member", "MemberType", local("kid")))
	}, dParticleOver(t, xsd.ElementDeclarationRef{Name: local("kid")}))

	// HeadType's {content type} is empty and MemberType's admits one <grand>,
	// so this tree is silent only if the MEMBER's type governed.
	wantSilence(t, cAssess(t, schema, dElem("root", 1,
		ElementChild(dElem("member", 2, ElementChild(dElem("grand", 3)))))),
		"the ·substituting declaration· governs, not the particle's head")

	got := cAssess(t, schema, dElem("root", 1, ElementChild(dElem("member", 2))))

	wantContentCharge(t, got, "cvc-complex-content", "1", loc(2, 1))
}

// A child ·attributed to· a strict or a lax Wildcard has its ·expanded name·
// ·resolved· against the schema's top-level element declarations
// (key-governing-ed clause 3, cvc-resolve-instance §3.17.6.3) and is assessed
// against what that resolution found. §3.10.4.1 draws no distinction between
// the two in the resolution step, so the two arms decide alike.
func TestDescendantAttributedToAResolvingWildcardIsAssessed(t *testing.T) {
	for _, pc := range []xsd.ProcessContents{xsd.ProcessStrict, xsd.ProcessLax} {
		t.Run(pc.String(), func(t *testing.T) {
			schema := dSchema(t, func(b *xsd.SchemaBuilder) {
				b.AddType(dType(t, "KidType", "", xsd.DerivationRestriction, nil,
					cSequence(t, false, cParticle(t, "grand", 1, 1))))
				b.AddElement(dTopLevel(t, "kid", "KidType"))
			}, dWildcard(t, pc))

			wantSilence(t, cAssess(t, schema, dElem("root", 1,
				ElementChild(dElem("kid", 2, ElementChild(dElem("grand", 3)))))),
				"the resolved declaration's {content type} admits the sequence")

			got := cAssess(t, schema, dElem("root", 1, ElementChild(dElem("kid", 2))))
			wantContentCharge(t, got, "cvc-complex-content", "1", loc(2, 1))
		})
	}
}

// A name that ·resolves· to no top-level declaration leaves the child with no
// ·governing element declaration·, which is ·strictly assessed· (key-sva)
// clause 3.3 — ·laxly assessed· against xs:anyType — and no charge of its
// own, under a STRICT wildcard as much as under a lax one. What an unresolved
// name under a strict wildcard costs is the ENCLOSING element's [validity]
// (§3.3.5.1, e-validity clause 1.1.3), which is charged against <root> here
// and at the CHILD's location, that being where the unresolved name is.
//
// Under LAX the same document charges nothing anywhere: clause 1.1.3 names a
// ***strict*** ·wildcard particle· and no other, and the child is ·laxly
// assessed· either way. The lax row is what shows the strict charge comes from
// {process contents} and not from the shape of the document (#717).
func TestUnresolvedNameUnderAStrictWildcardChargesTheEnclosingElement(t *testing.T) {
	doc := func() Element {
		return dElem("root", 1, ElementChild(dElem("stranger", 2, ElementChild(dElem("anything", 3)))))
	}

	got := cAssess(t, dSchema(t, nil, dWildcard(t, xsd.ProcessStrict)), doc())
	if len(got) != 1 {
		t.Fatalf("Violations() = %v, want exactly one", got)
	}
	if got[0].Rule != "cvc-assess-elt" {
		t.Errorf("Rule = %q, want cvc-assess-elt — e-validity has no catalog ID of its own", got[0].Rule)
	}
	if got[0].Loc != loc(2, 1) {
		t.Errorf("Loc = %s, want the CHILD's position %s", got[0].Loc, loc(2, 1))
	}
	// The clause is named against the rule that STATES it and not against the
	// Rule the error carries: key-sva (§3.3.4.6) has a clause 1.1.3 of its own
	// and this is not it.
	for _, want := range []string{"·resolves· to no top-level element declaration", "e-validity clause 1.1.3", "notKnown", "the enclosing element root"} {
		if !strings.Contains(got[0].Msg, want) {
			t.Errorf("Msg = %q, want it to name %s", got[0].Msg, want)
		}
	}
	// Pinned as a PREFIX, not as a substring: the two names this message
	// interpolates are both present whichever order they are passed in, so only
	// the opening subject tells the ·attributed· child from the element the
	// clause charges (#1048).
	if !strings.HasPrefix(got[0].Msg, "element stranger is ·attributed to·") {
		t.Errorf("Msg = %q, want it to OPEN by naming the child as the ·attributed· item", got[0].Msg)
	}

	wantSilence(t, cAssess(t, dSchema(t, nil, dWildcard(t, xsd.ProcessLax)), doc()),
		"e-validity clause 1.1.3 names a strict wildcard particle alone")
}

// A name under a strict wildcard that DOES ·resolve·, to a declaration whose
// ·selected type definition· is ·absent· (§5.3), is ·laxly assessed· too, so
// e-validity clause 1.1.3 charges the enclosing element all the same — but the
// message names §5.3 as the cause, not an unresolved name. The cvc-elt clause 1
// charge comes first, at the same child, from the resolution itself.
func TestAbsentSelectionUnderAStrictWildcardNamesItsCause(t *testing.T) {
	schema := dSchema(t, func(b *xsd.SchemaBuilder) {
		kid, err := xsd.NewElementDeclaration(xsderr.Loc{}, local("kid"), absentHead, nil,
			xsd.NewGlobalScope(), nil, false, nil, nil, nil, false, nil)
		if err != nil {
			t.Fatalf("building the top-level kid element declaration: %v", err)
		}
		b.AddElement(kid)
	}, dWildcard(t, xsd.ProcessStrict))

	got := cAssess(t, schema, dElem("root", 1, ElementChild(dElem("kid", 2))))

	if len(got) != 2 {
		t.Fatalf("Violations() = %v, want the cvc-elt clause 1 charge then the e-validity clause 1.1.3 one", got)
	}
	if got[0].Rule != "cvc-elt" || got[0].Loc != loc(2, 1) {
		t.Errorf("violations[0] = %s at %s, want cvc-elt at the child %s", got[0].Rule, got[0].Loc, loc(2, 1))
	}
	if got[1].Rule != "cvc-assess-elt" || got[1].Loc != loc(2, 1) {
		t.Errorf("violations[1] = %s at %s, want cvc-assess-elt at the child %s", got[1].Rule, got[1].Loc, loc(2, 1))
	}
	const prefix = "element kid is ·attributed to· a ***strict*** ·wildcard particle· but ·resolves· to a top-level element declaration that has or contains an ·absent· component — an ·absent· ·selected type definition· or {substitution group affiliations} member, a missing sub-component charged under cvc-elt clause 1, "
	if !strings.HasPrefix(got[1].Msg, prefix) {
		t.Errorf("Msg = %q, want it to open %q", got[1].Msg, prefix)
	}
	if !strings.Contains(got[1].Msg, "the enclosing element root") {
		t.Errorf("Msg = %q, want it to name the enclosing element root", got[1].Msg)
	}
}

// The same clause is WITHHELD for an item the {open content} took: an item
// cvc-complex-content clause 2.4 or 3.4 admitted is ·attributed to· the {open
// content} (§3.4.4.4) and not to a ·wildcard particle·, which is what clause
// 1.1.3 quantifies over. Charging here would reject a document key-governing-ed
// clause 4 leaves merely without a ·governing element declaration·. The log is
// what tells this from a charge that never became live: the attribution line
// names the {open content} wildcard, so the item DID reach the clause.
func TestOpenContentAttributionWithholdsTheStrictWildcardParticleCharge(t *testing.T) {
	schema := cSchema(t, cOpenContent(t, xsd.OpenContentInterleave, xsd.ProcessStrict,
		cNamespaceConstraint(t, xsd.NamespaceConstraintAny), cParticle(t, "a", 1, 1)))

	got, visits := cAssessLogged(t, schema, cRoot("a", "stranger"))

	wantSilence(t, got, "the open-content item's ·attribution· is not a ·wildcard particle·")
	cWantLogged(t, visits,
		"assessing content validate.name=stranger validate.loc=instance.xml:3:1 validate.rule=cvc-complex-content validate.clause=3 validate.outcome=attributed to open content wildcard any")
}

// A type carrying BOTH a strict {open content} wildcard and a strict ·wildcard
// particle· charges clause 1.1.3 for the child the PARTICLE took and withholds
// it for the child the {open content} took, in one assessment: the two are told
// apart by the ·attribution· each child carries (§3.4.4.4) and never by the
// type's shape, which is the same for both of them here.
//
// PRINCIPLES 14 is what puts one child on each arm: the ·wildcard particle· is
// offered first and its {max occurs} is 1, so "p" goes to the particle and "o",
// for which {particle} then has no ·path·, goes to the {open content}. Neither
// name ·resolves· to a top-level element declaration, so both are ·laxly
// assessed· with [validity] notKnown and differ in nothing but the arm.
func TestBothWildcardKindsStrictChargeOnlyTheParticleAttributedChild(t *testing.T) {
	schema := cSchema(t, cOpenContent(t, xsd.OpenContentInterleave, xsd.ProcessStrict,
		cNamespaceConstraint(t, xsd.NamespaceConstraintAny), dWildcard(t, xsd.ProcessStrict)))

	got, visits := cAssessLogged(t, schema, cRoot("p", "o"))

	if len(got) != 1 {
		t.Fatalf("Violations() = %v, want exactly the particle-attributed child's clause 1.1.3 charge", got)
	}
	if got[0].Loc != loc(2, 1) {
		t.Errorf("Loc = %s, want the particle-attributed child's position %s", got[0].Loc, loc(2, 1))
	}
	if !strings.HasPrefix(got[0].Msg, "element p is ·attributed to·") {
		t.Errorf("Msg = %q, want the PARTICLE-attributed child p as its subject, not the open-attributed o", got[0].Msg)
	}
	cWantLogged(t, visits,
		"assessing content validate.name=p validate.loc=instance.xml:2:1 validate.rule=cvc-complex-content validate.clause=3 validate.outcome=attributed to wildcard any",
		"assessing content validate.name=o validate.loc=instance.xml:3:1 validate.rule=cvc-complex-content validate.clause=3 validate.outcome=attributed to open content wildcard any")
}

// Withholding clause 1.1.3 for the {open content} arm is not the same as
// assessing that child against nothing: key-governing-ed clause 4 still
// ·resolves· its ·expanded name· among the top-level element declarations, so a
// child the {open content} took IS assessed against the type its declaration
// supplies, and a defect in the child's own [[children]] is charged at the
// child's position. That holds under a strict and a lax {open content}
// wildcard alike; only a skip one makes the child ·skipped· (#1969).
func TestOpenContentAttributedChildIsAssessedAgainstItsResolvedDeclaration(t *testing.T) {
	for _, pc := range []xsd.ProcessContents{xsd.ProcessStrict, xsd.ProcessLax} {
		t.Run(pc.String(), func(t *testing.T) {
			schema := cSchemaFrom(t, dType(t, "RootType", "", xsd.DerivationRestriction, nil,
				cOpenContent(t, xsd.OpenContentInterleave, pc,
					cNamespaceConstraint(t, xsd.NamespaceConstraintAny), cParticle(t, "a", 1, 1))),
				func(b *xsd.SchemaBuilder) {
					b.AddType(dType(t, "OType", "", xsd.DerivationRestriction, nil,
						cSequence(t, false, cParticle(t, "needed", 1, 1))))
					b.AddElement(dTopLevel(t, "o", "OType"))
				})

			// <o> resolves to the top-level declaration, whose OType wants a
			// <needed> it does not carry.
			got := cAssess(t, schema, dElem("root", 1,
				ElementChild(dElem("a", 2)), ElementChild(dElem("o", 3))))

			wantContentCharge(t, got, "cvc-complex-content", "1", loc(3, 1))
		})
	}
}

// A child ·attributed to· a skip {open content} wildcard is ·skipped· with its
// whole subtree, as one ·attributed to· a skip ·wildcard particle· is: the walk
// never enters <x>, so neither it nor <junk> below it is visited (#1969). The
// visit log is what says so — a ·laxly assessed· child charges nothing too, and
// only a ·skipped· one leaves the subtree unvisited.
func TestOpenContentAttributedChildUnderASkipWildcardIsNotAssessed(t *testing.T) {
	schema := cSchemaFrom(t, dType(t, "RootType", "", xsd.DerivationRestriction, nil,
		cOpenContent(t, xsd.OpenContentInterleave, xsd.ProcessSkip,
			cNamespaceConstraint(t, xsd.NamespaceConstraintAny), cParticle(t, "a", 1, 1))), nil)

	log, visits := recordingLogger()
	v, err := New(schema, testBackend(), WithLogger(log))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	res := v.Assess(dElem("root", 1,
		ElementChild(dElem("a", 2)),
		ElementChild(dElem("x", 3, ElementChild(dElem("junk", 4))))))

	wantSilence(t, res.Violations(), "a ·skipped· item's schema-validity is not assessed")
	want := []string{
		"assessing element validate.name=root validate.loc=instance.xml:1:1",
		"assessing content validate.name=a validate.loc=instance.xml:2:1 " +
			"validate.rule=cvc-complex-content validate.clause=3 validate.outcome=attributed to element declaration a",
		"assessing element validate.name=a validate.loc=instance.xml:2:1 " +
			"validate.rule=cvc-elt validate.clause=1 validate.outcome=declined",
		"assessing element validate.name=a validate.loc=instance.xml:2:1",
		"assessing ID/IDREF table validate.name=a validate.loc=instance.xml:2:1 " +
			"validate.rule=cvc-id validate.clause=1 validate.outcome=declined",
		"assessing content validate.name=x validate.loc=instance.xml:3:1 " +
			"validate.rule=cvc-complex-content validate.clause=3 validate.outcome=attributed to open content wildcard any",
		"assessing element validate.name=x validate.loc=instance.xml:3:1 " +
			"validate.rule=cvc-assess-elt validate.clause=2 validate.outcome=skipped",
		"assessing content validate.name=root validate.loc=instance.xml:1:1 " +
			"validate.rule=cvc-complex-content validate.clause=3 validate.outcome=accepted",
	}
	if !slices.Equal(*visits, want) {
		t.Errorf("walk visited\n\t%s\nwant\n\t%s",
			strings.Join(*visits, "\n\t"), strings.Join(want, "\n\t"))
	}
}

// A ·skipped· child runs no ·QName resolution· (key-governing-ed clause 4.1):
// under a skip {open content} wildcard <o> is not assessed against the
// top-level declaration its name would ·resolve· to, so the <needed> that
// declaration's OType requires is charged nowhere. Under a strict {open
// content} the same document is charged at 3:1
// (TestOpenContentAttributedChildIsAssessedAgainstItsResolvedDeclaration).
func TestOpenContentAttributedChildUnderASkipWildcardIsNotAssessedAgainstItsResolvedDeclaration(t *testing.T) {
	schema := cSchemaFrom(t, dType(t, "RootType", "", xsd.DerivationRestriction, nil,
		cOpenContent(t, xsd.OpenContentInterleave, xsd.ProcessSkip,
			cNamespaceConstraint(t, xsd.NamespaceConstraintAny), cParticle(t, "a", 1, 1))),
		func(b *xsd.SchemaBuilder) {
			b.AddType(dType(t, "OType", "", xsd.DerivationRestriction, nil,
				cSequence(t, false, cParticle(t, "needed", 1, 1))))
			b.AddElement(dTopLevel(t, "o", "OType"))
		})

	// <o> would resolve to the top-level declaration, whose OType wants a
	// <needed> it does not carry.
	got := cAssess(t, schema, dElem("root", 1,
		ElementChild(dElem("a", 2)), ElementChild(dElem("o", 3))))

	wantSilence(t, got, "a ·skipped· item has no ·governing element declaration· to be charged against")
}

// The ·laxly assessed· child is still recursed, unlike a ·skipped· one: its own
// [[children]] and [[attributes]] are assessed in their turn (cvc-assess-elt
// clause 3, key-lva clause 2), against xs:anyType, whose {content type}
// and {attribute uses} reject none of them. The clause 1.1.3 charge above is
// the enclosing element's and is not repeated down the subtree: <anything>
// under <stranger> is ·attributed to· xs:anyType's LAX wildcard, resolves
// nothing, and is ·laxly assessed· in its turn, which e-validity clause 1.1.3
// does not reach.
//
// The charge's own line names cvc-assess-elt, the Rule it carries, and no
// clause: cvc-assess-elt has no clause 1.1.3, and the one key-sva has is a
// different condition, so the e-validity clause stays in the message.
func TestALaxlyAssessedChildIsWalkedAndChargesNothingBelowItself(t *testing.T) {
	log, visits := recordingLogger()
	v, err := New(dSchema(t, nil, dWildcard(t, xsd.ProcessStrict)), testBackend(), WithLogger(log))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	res := v.Assess(dElem("root", 1, ElementChild(dElem("stranger", 2, ElementChild(dElem("anything", 3))))))

	if n := len(res.Violations()); n != 1 {
		t.Fatalf("Violations() = %v, want exactly the one clause 1.1.3 charge", res.Violations())
	}
	for _, want := range []string{
		"assessing content validate.name=stranger validate.loc=instance.xml:2:1 " +
			"validate.rule=cvc-assess-elt validate.outcome=charged",
		"assessing element validate.name=stranger validate.loc=instance.xml:2:1",
		"assessing element validate.name=anything validate.loc=instance.xml:3:1",
	} {
		if !slices.Contains(*visits, want) {
			t.Errorf("walk visited\n\t%s\nwant it to include\n\t%s", strings.Join(*visits, "\n\t"), want)
		}
	}
}

// A child ·attributed to· a skip Wildcard is not ·assessed· at all (key-sva
// clause 3.2, cvc-assess-elt clause 2), and neither is any element below it:
// ·skipped· (§3.10.4.1 key-skipped) holds for every descendant of a skipped
// item, which the walk gets by not descending at all. The log is what
// distinguishes this from ·strictly assessed· (key-sva) clause 3.3 — both
// charge nothing, and only one of them leaves the subtree unvisited.
func TestDescendantAttributedToASkipWildcardIsNotAssessed(t *testing.T) {
	schema := dSchema(t, func(b *xsd.SchemaBuilder) {
		b.AddType(dType(t, "KidType", "", xsd.DerivationRestriction, nil,
			cSequence(t, false, cParticle(t, "grand", 1, 1))))
		b.AddElement(dTopLevel(t, "kid", "KidType"))
	}, dWildcard(t, xsd.ProcessSkip))

	log, visits := recordingLogger()
	v, err := New(schema, testBackend(), WithLogger(log))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// <kid> is missing the <grand> KidType requires, and carries a child of its
	// own that no declaration admits: a skipped subtree is assessed for none of it.
	res := v.Assess(dElem("root", 1, ElementChild(dElem("kid", 2, ElementChild(dElem("junk", 3))))))

	wantSilence(t, res.Violations(), "a ·skipped· item's schema-validity is not assessed")
	want := []string{
		"assessing element validate.name=root validate.loc=instance.xml:1:1",
		"assessing content validate.name=kid validate.loc=instance.xml:2:1 " +
			"validate.rule=cvc-complex-content validate.clause=1 validate.outcome=attributed to wildcard any",
		"assessing element validate.name=kid validate.loc=instance.xml:2:1 " +
			"validate.rule=cvc-assess-elt validate.clause=2 validate.outcome=skipped",
		"assessing content validate.name=root validate.loc=instance.xml:1:1 " +
			"validate.rule=cvc-complex-content validate.clause=1 validate.outcome=accepted",
	}
	if !slices.Equal(*visits, want) {
		t.Errorf("walk visited\n\t%s\nwant\n\t%s",
			strings.Join(*visits, "\n\t"), strings.Join(want, "\n\t"))
	}
}

// An element whose own ·governing type definition· was not determined attributes
// its [[children]] to nothing, so the descent stops being typed there rather
// than resuming further down: each element below is undecided, which is not
// the ·laxly assessed· zero value, and hands its own [[children]] nothing
// either. The second <kid> is what tells the two apart — ·laxly assessed·, its
// parent would hand it xs:anyType's lax wildcard and it would resolve to the
// top-level declaration whose KidType wants a <grand> (#1823).
func TestDescendantOfAnUngovernedElementIsAssessedAgainstNothing(t *testing.T) {
	schema := dSchema(t, func(b *xsd.SchemaBuilder) {
		b.AddType(dType(t, "KidType", "", xsd.DerivationRestriction, nil,
			cSequence(t, false, cParticle(t, "grand", 1, 1))))
		b.AddElement(dTopLevel(t, "kid", "KidType"))
	}, cParticle(t, "opaque", 1, 1))

	// <opaque> is a local declaration with an ·absent· {type definition}, so
	// nothing below it is governed — including a <kid> a top-level declaration
	// would otherwise have governed.
	wantSilence(t, cAssess(t, schema, dElem("root", 1,
		ElementChild(dElem("opaque", 2, ElementChild(dElem("kid", 3)))))),
		"an ungoverned element attributes its [[children]] to nothing")
	wantSilence(t, cAssess(t, schema, dElem("root", 1,
		ElementChild(dElem("opaque", 2, ElementChild(dElem("kid", 3, ElementChild(dElem("kid", 4)))))))),
		"an undecided element attributes its [[children]] to nothing")
}

// An undecided element's xsi:nil and xsi:type are charged nothing: this
// package could not decide the element's governance, so the item may be one
// the true schema ·skips· (key-governing-ad clause 3 and its Note), and
// cvc-attribute clauses 3 and 5 are not charged against it. The parent's own
// two declines, its withheld type (cvc-elt) and its ID/IDREF-table entry
// (cvc-id), are the whole of the Result's undecidedness. Each row fails with
// [walk.instanceNilLexical]'s or [walk.instanceTypeResolves]'s undecided
// guard removed, the cvc-attribute charge then reaching [Result] (#2159).
func TestUndecidedElementXSIAttributesAreChargedNothing(t *testing.T) {
	// The builtins are seeded so that xs:QName is there for clause 3 to charge
	// "1bad" against rather than decline.
	schema := dSchema(t, func(b *xsd.SchemaBuilder) {
		for _, st := range icSeeded(t) {
			b.AddType(st)
		}
	}, cParticle(t, "opaque", 1, 1))
	for _, tc := range []struct {
		why  string
		attr Attribute
	}{
		{"xsi:nil outside xs:boolean", xsiNilAttr("maybe")},
		{"xsi:type naming no type definition", xsiTypeAttr("nosuch")},
		{"xsi:type outside xs:QName", xsiTypeAttr("1bad")},
	} {
		t.Run(tc.why, func(t *testing.T) {
			kid := icElem(xsd.QName{Local: "kid"}, 3, []Attribute{tc.attr})
			got, unevaluated := assessRecorded(t, schema, dElem("root", 1,
				ElementChild(dElem("opaque", 2, ElementChild(kid)))))
			wantSilence(t, got, "an undecided element's xsi attributes are charged nothing")
			if len(unevaluated) != 2 || unevaluated[0].Loc() != loc(2, 1) || unevaluated[1].Loc() != loc(2, 1) {
				t.Errorf("Unevaluated() = %v, want the two records at <opaque>'s %s", messages(unevaluated), loc(2, 1))
			}
		})
	}
}
