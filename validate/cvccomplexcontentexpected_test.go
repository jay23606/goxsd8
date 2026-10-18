package validate

import (
	"testing"

	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// These tests pin what a cvc-complex-content charge says the content model
// expected at the charged position ([xsd.Matcher.Admissible], rendered by
// expected): the whole message, so that the subject, the expected set and the
// inline citation each stay where STYLE E5 puts them.

// cGroup is a {min occurs} = {max occurs} = 1 particle over a model group of
// the given compositor.
func cGroup(t *testing.T, compositor xsd.Compositor, particles ...xsd.Particle) xsd.Particle {
	t.Helper()
	g, err := xsd.NewModelGroup(xsderr.Loc{}, compositor, particles)
	if err != nil {
		t.Fatalf("NewModelGroup: %v", err)
	}
	o, err := xsd.NewOccurs(xsderr.Loc{}, 1, 1)
	if err != nil {
		t.Fatalf("NewOccurs: %v", err)
	}
	p, err := xsd.NewParticle(xsderr.Loc{}, o, xsd.ResolvedTerm{Term: g})
	if err != nil {
		t.Fatalf("NewParticle: %v", err)
	}
	return p
}

// cWantMsg fails unless the one violation's message is exactly want.
func cWantMsg(t *testing.T, got []*xsderr.Error, want string) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("Violations() = %v, want exactly one", got)
	}
	if got[0].Msg != want {
		t.Errorf("Msg =\n\t%s\nwant\n\t%s", got[0].Msg, want)
	}
}

// cUnadmitted is the clause 1 message for an item no particle admits, naming
// what the content model expected there.
func cUnadmitted(item, expects string) string {
	return "the element information item " + item + " is ·attributed to· no particle of the {content type} of root at its position in the [[children]], where the content model expects " + expects + ", so the sequence is not ·valid· with respect to that {content type} as cvc-complex-content clause 1 requires"
}

// cEndsShort is the message for [[children]] that end with a {min occurs}
// owed, naming what the {particle} expected next.
func cEndsShort(expects, clause string) string {
	return "the [[children]] of root end before every particle of the {content type} of its ·governing type definition· has taken the occurrences its {min occurs} requires, where that {content type}'s {particle} expects " + expects + ", so the sequence is not ·valid· with respect to that {content type} as cvc-complex-content clause " + clause + " requires"
}

// A sequence that skips a required member is charged at the member after it,
// and the charge names the one it skipped: the operator's fix, with no schema
// in hand.
func TestContentChargeNamesTheSkippedSequenceMember(t *testing.T) {
	schema := cSchema(t, cSequence(t, false,
		cParticle(t, "a", 1, 1), cParticle(t, "b", 1, 1), cParticle(t, "c", 1, 1)))

	got := cAssess(t, schema, cRoot("a", "c"))

	wantContentCharge(t, got, "cvc-complex-content", "1", loc(3, 1))
	cWantMsg(t, got, cUnadmitted("c", "element declaration b"))
}

// A choice expects any one of its members, listed in the content model's
// order.
func TestContentChargeNamesEveryMemberOfAChoice(t *testing.T) {
	schema := cSchema(t, xsd.ElementContent{Particle: cGroup(t, xsd.CompositorSequence,
		cGroup(t, xsd.CompositorChoice,
			cParticle(t, "a", 1, 1), cParticle(t, "b", 1, 1), cParticle(t, "c", 1, 1)),
		cParticle(t, "d", 1, 1))})

	got := cAssess(t, schema, cRoot("d"))

	wantContentCharge(t, got, "cvc-complex-content", "1", loc(2, 1))
	cWantMsg(t, got, cUnadmitted("d", "one of element declaration a, element declaration b, element declaration c"))
}

// An all group expects the members it has not yet had, in the content model's
// order and not the order the instance took them in.
func TestContentChargeNamesTheUnspentMembersOfAnAllGroup(t *testing.T) {
	schema := cSchema(t, xsd.ElementContent{Particle: cGroup(t, xsd.CompositorAll,
		cParticle(t, "a", 1, 1), cParticle(t, "b", 1, 1), cParticle(t, "c", 1, 1))})

	got := cAssess(t, schema, cRoot("b", "b"))

	wantContentCharge(t, got, "cvc-complex-content", "1", loc(3, 1))
	cWantMsg(t, got, cUnadmitted("b", "one of element declaration a, element declaration c"))
}

// Past maxExpectedListed arms the list is cut and the rest counted exactly.
func TestContentChargeCountsTheArmsPastTheBound(t *testing.T) {
	var members []xsd.Particle
	for _, local := range []string{"a", "b", "c", "d", "e", "f"} {
		members = append(members, cParticle(t, local, 1, 1))
	}
	if len(members) != maxExpectedListed+1 {
		t.Fatalf("the fixture has %d members, want maxExpectedListed+1 = %d", len(members), maxExpectedListed+1)
	}
	schema := cSchema(t, xsd.ElementContent{Particle: cGroup(t, xsd.CompositorSequence,
		cGroup(t, xsd.CompositorChoice, members...),
		cParticle(t, "z", 1, 1))})

	got := cAssess(t, schema, cRoot("z"))

	cWantMsg(t, got, cUnadmitted("z", "one of element declaration a, element declaration b, element declaration c, element declaration d, element declaration e and 1 more"))
}

// Where the content model admits nothing more, the charge says so rather than
// listing nothing.
func TestContentChargeStatesAnEmptyExpectation(t *testing.T) {
	schema := cSchema(t, cSequence(t, false, cParticle(t, "a", 1, 1)))

	got := cAssess(t, schema, cRoot("a", "b"))

	cWantMsg(t, got, cUnadmitted("b", "no further element information item"))
}

// [[children]] that end short name what the {particle} still expects.
func TestContentChargeAtTheEndNamesWhatIsOwed(t *testing.T) {
	schema := cSchema(t, cSequence(t, false, cParticle(t, "a", 1, 1), cParticle(t, "b", 2, 2)))

	got := cAssess(t, schema, cRoot("a", "b"))

	wantContentCharge(t, got, "cvc-complex-content", "1", loc(1, 1))
	cWantMsg(t, got, cEndsShort("element declaration b", "1"))
}

// At the end of the [[children]] the {open content} is not listed, though
// Admissible offers it under interleave: an item it takes is S2's and pays
// nothing S1 owes (clause 3.2).
func TestContentChargeAtTheEndLeavesTheOpenContentOut(t *testing.T) {
	schema := cSchema(t, cOpenContent(t, xsd.OpenContentInterleave, xsd.ProcessSkip,
		cNamespaceConstraint(t, xsd.NamespaceConstraintAny),
		cParticle(t, "a", 1, 1), cParticle(t, "b", 1, 1)))

	got := cAssess(t, schema, cRoot("a", "x"))

	wantContentCharge(t, got, "cvc-complex-content", "3", loc(1, 1))
	cWantMsg(t, got, cEndsShort("element declaration b", "3"))
}

// An item neither the {particle} nor the {open content} admits names both
// halves' expectations, the {open content} last and its wildcard by its
// namespace constraint.
func TestOpenContentChargeNamesBothHalves(t *testing.T) {
	schema := cSchema(t, cOpenContent(t, xsd.OpenContentInterleave, xsd.ProcessSkip,
		cNamespaceConstraint(t, xsd.NamespaceConstraintEnumeration, "urn:open"),
		cParticle(t, "a", 1, 1), cParticle(t, "b", 1, 1)))

	got := cAssess(t, schema, cRoot("a", "zzz"))

	cWantMsg(t, got, `the element information item zzz is ·attributed to· no particle of the {content type} of root at its position in the [[children]], and its ·expanded name· is not ·valid· with respect to that {content type}'s {open content} wildcard either, where the content model expects one of element declaration b, open content wildcard enumeration "urn:open", so the sequence is not ·valid· with respect to that {content type} as cvc-complex-content clause 3 requires`)
}

// An expected element is named by its ·expanded name· in the notation the
// charged item's own name is rendered in.
func TestContentChargeNamesAnExpectedElementByExpandedName(t *testing.T) {
	scope, err := xsd.NewLocalScope(xsderr.Loc{}, xsd.ComplexTypeScopeParent{Name: xsd.QName{Local: "RootType"}})
	if err != nil {
		t.Fatalf("NewLocalScope: %v", err)
	}
	var members []xsd.Particle
	for _, local := range []string{"x", "y"} {
		d, err := xsd.NewElementDeclaration(xsderr.Loc{}, xsd.QName{Space: "urn:b", Local: local}, nil, nil, scope,
			nil, false, nil, nil, nil, false, nil)
		if err != nil {
			t.Fatalf("building {urn:b}%s: %v", local, err)
		}
		o, err := xsd.NewOccurs(xsderr.Loc{}, 1, 1)
		if err != nil {
			t.Fatalf("NewOccurs: %v", err)
		}
		p, err := xsd.NewParticle(xsderr.Loc{}, o, xsd.ResolvedTerm{Term: d})
		if err != nil {
			t.Fatalf("NewParticle: %v", err)
		}
		members = append(members, p)
	}
	schema := cSchema(t, cSequence(t, false, members...))
	root := &testElement{name: xsd.QName{Local: "root"}, loc: loc(1, 1)}
	root.kids = append(root.kids, ElementChild(&testElement{name: xsd.QName{Space: "urn:b", Local: "y"}, loc: loc(2, 1)}))

	got := cAssess(t, schema, root)

	cWantMsg(t, got, cUnadmitted("{urn:b}y", "element declaration {urn:b}x"))
}

// An expected wildcard is named by its namespace constraint, after the word
// "wildcard" so it cannot be read as an element's name, an ·absent·
// {namespaces} member included.
func TestContentChargeNamesAnExpectedWildcardByNamespaceConstraint(t *testing.T) {
	w, err := xsd.NewWildcard(xsderr.Loc{}, cNamespaceConstraint(t, xsd.NamespaceConstraintNot, "", "urn:no"), xsd.ProcessSkip)
	if err != nil {
		t.Fatalf("NewWildcard: %v", err)
	}
	o, err := xsd.NewOccurs(xsderr.Loc{}, 1, 1)
	if err != nil {
		t.Fatalf("NewOccurs: %v", err)
	}
	p, err := xsd.NewParticle(xsderr.Loc{}, o, xsd.ResolvedTerm{Term: w})
	if err != nil {
		t.Fatalf("NewParticle: %v", err)
	}
	schema := cSchema(t, cSequence(t, false, p))

	got := cAssess(t, schema, cRoot("q"))

	cWantMsg(t, got, cUnadmitted("q", `wildcard not ·absent· "urn:no"`))
}

// Under {mode} suffix, once an item has begun S2 the {particle} can take
// nothing more (clause 2.1's S = S1 + S2), so [[children]] that end with a
// {min occurs} still owed are charged with the {particle} expecting nothing,
// not the content model, whose {open content} would still take another item.
func TestContentChargeAtTheEndOnceTheSuffixHasBegun(t *testing.T) {
	schema := cSchema(t, cOpenContent(t, xsd.OpenContentSuffix, xsd.ProcessSkip,
		cNamespaceConstraint(t, xsd.NamespaceConstraintAny),
		cParticle(t, "a", 1, 1), cParticle(t, "b", 1, 1)))

	got := cAssess(t, schema, cRoot("a", "x"))

	wantContentCharge(t, got, "cvc-complex-content", "2", loc(1, 1))
	cWantMsg(t, got, cEndsShort("no further element information item", "2"))
}

// A choice with no members accepts nothing, so [[children]] that end where
// one is owed leave no leaf on the frontier, with no {open content} and no
// suffix in sight, and the charge says the {particle} expects nothing.
func TestContentChargeAtTheEndOfAnEmptyChoice(t *testing.T) {
	schema := cSchema(t, xsd.ElementContent{Particle: cGroup(t, xsd.CompositorSequence,
		cParticle(t, "a", 1, 1), cGroup(t, xsd.CompositorChoice))})

	got := cAssess(t, schema, cRoot("a"))

	wantContentCharge(t, got, "cvc-complex-content", "1", loc(1, 1))
	cWantMsg(t, got, cEndsShort("no further element information item", "1"))
}

// The bound counts particle arms only: past it the {open content} arm is still
// named, last, after the count.
func TestOpenContentChargeNamesTheOpenWildcardPastTheBound(t *testing.T) {
	var members []xsd.Particle
	for _, local := range []string{"a", "b", "c", "d", "e", "f"} {
		members = append(members, cParticle(t, local, 1, 1))
	}
	if len(members) != maxExpectedListed+1 {
		t.Fatalf("the fixture has %d members, want maxExpectedListed+1 = %d", len(members), maxExpectedListed+1)
	}
	schema := cSchema(t, cOpenContent(t, xsd.OpenContentInterleave, xsd.ProcessSkip,
		cNamespaceConstraint(t, xsd.NamespaceConstraintEnumeration, "urn:open"),
		cGroup(t, xsd.CompositorChoice, members...), cParticle(t, "z", 1, 1)))

	got := cAssess(t, schema, cRoot("z"))

	wantContentCharge(t, got, "cvc-complex-content", "3", loc(2, 1))
	cWantMsg(t, got, `the element information item z is ·attributed to· no particle of the {content type} of root at its position in the [[children]], and its ·expanded name· is not ·valid· with respect to that {content type}'s {open content} wildcard either, where the content model expects one of element declaration a, element declaration b, element declaration c, element declaration d, element declaration e and 1 more, or open content wildcard enumeration "urn:open", so the sequence is not ·valid· with respect to that {content type} as cvc-complex-content clause 3 requires`)
}

// cDisallowingQ is the {namespace constraint} any whose {disallowed names}
// holds the unqualified name q.
func cDisallowingQ(t *testing.T) xsd.NamespaceConstraint {
	t.Helper()
	nc, err := xsd.NewNamespaceConstraint(xsderr.Loc{}, xsd.NamespaceConstraintAny, nil, []xsd.QName{{Local: "q"}}, nil)
	if err != nil {
		t.Fatalf("NewNamespaceConstraint: %v", err)
	}
	return nc
}

// A ·wildcard particle· that allows the item's namespace and still declined it
// did so under its {disallowed names}, and the charge says it disallows the
// item rather than listing a wildcard the item seems to match.
func TestContentChargeNamesAWildcardThatDisallowsTheItem(t *testing.T) {
	w, err := xsd.NewWildcard(xsderr.Loc{}, cDisallowingQ(t), xsd.ProcessSkip)
	if err != nil {
		t.Fatalf("NewWildcard: %v", err)
	}
	o, err := xsd.NewOccurs(xsderr.Loc{}, 1, 1)
	if err != nil {
		t.Fatalf("NewOccurs: %v", err)
	}
	p, err := xsd.NewParticle(xsderr.Loc{}, o, xsd.ResolvedTerm{Term: w})
	if err != nil {
		t.Fatalf("NewParticle: %v", err)
	}
	schema := cSchema(t, cSequence(t, false, p))

	got := cAssess(t, schema, cRoot("q"))

	wantContentCharge(t, got, "cvc-complex-content", "1", loc(2, 1))
	cWantMsg(t, got, cUnadmitted("q", "wildcard any, which disallows q"))
}

// The {open content} arm is named as disallowing the item on the same terms.
func TestOpenContentChargeNamesAnOpenWildcardThatDisallowsTheItem(t *testing.T) {
	schema := cSchema(t, cOpenContent(t, xsd.OpenContentInterleave, xsd.ProcessSkip, cDisallowingQ(t),
		cParticle(t, "a", 1, 1), cParticle(t, "b", 1, 1)))

	got := cAssess(t, schema, cRoot("a", "q"))

	wantContentCharge(t, got, "cvc-complex-content", "3", loc(3, 1))
	cWantMsg(t, got, `the element information item q is ·attributed to· no particle of the {content type} of root at its position in the [[children]], and its ·expanded name· is not ·valid· with respect to that {content type}'s {open content} wildcard either, where the content model expects one of element declaration b, open content wildcard any, which disallows q, so the sequence is not ·valid· with respect to that {content type} as cvc-complex-content clause 3 requires`)
}
