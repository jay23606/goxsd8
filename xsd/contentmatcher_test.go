package xsd

import (
	"fmt"
	"slices"
	"testing"

	"github.com/kud360/goxsd8/xsderr"
)

// These tests drive Matcher through the exported entry points alone —
// Schema.ContentMatcher, Matcher.Next, Matcher.Accepting, Matcher.Admissible —
// but live in the package because the fixtures they share (uq, uOccurs, uGroup,
// uLocal, …) are particleattribution_test.go's. Every schema below is
// FINALIZED, which is what makes the models cos-nonambig-clean: a fixture that
// stopped being unambiguous would fail at cmSchema rather than silently
// exercise a model the walk is not licensed over.

// cmSchema finalizes a schema whose complex type {urn:upa}ct has p as its
// {content type} particle, and returns it with that type.
func cmSchema(t testing.TB, p Particle, extra func(*SchemaBuilder)) (*Schema, ComplexType) {
	t.Helper()
	ct := uCT(t, uq("ct"), p)
	b := NewSchemaBuilder()
	b.AddType(uNamedType(t, uq("T")))
	b.AddType(uNamedType(t, uq("U")))
	if extra != nil {
		extra(b)
	}
	b.AddType(ct)
	s, err := b.Finalize()
	if err != nil {
		t.Fatalf("finalizing the fixture schema: %v", err)
	}
	return s, ct
}

// cmMatcher is cmSchema plus the matcher over the type, failing when
// ContentMatcher declines — a decline is a distinct outcome every test that
// wants one asserts on its own.
func cmMatcher(t testing.TB, p Particle, extra func(*SchemaBuilder)) *Matcher {
	t.Helper()
	s, ct := cmSchema(t, p, extra)
	m, ok := s.ContentMatcher(ct)
	if !ok {
		t.Fatalf("ContentMatcher declined a model the test needs decided")
	}
	return m
}

// cmFeed puts each local name to m in turn and returns the index of the first
// one it rejected, or len(names) when it took them all.
func cmFeed(t *testing.T, m *Matcher, names ...string) int {
	t.Helper()
	for i, n := range names {
		if _, ok := m.Next(uq(n)); !ok {
			return i
		}
	}
	return len(names)
}

// cmAccept feeds names and asserts the whole sequence is accepted.
func cmAccept(t *testing.T, m *Matcher, names ...string) {
	t.Helper()
	if i := cmFeed(t, m, names...); i != len(names) {
		t.Fatalf("Next rejected %s at position %d of %v, want the whole sequence taken", names[i], i, names)
	}
	if !m.Accepting() {
		t.Errorf("Accepting() = false after %v, want true", names)
	}
}

// cmLeaf is a particle over a LOCAL element declaration named local, all of
// whose fixtures share the named type T so that two same-named declarations in
// one model do not trip cos-element-consistent.
func cmLeaf(t testing.TB, local string, o Occurs) Particle {
	t.Helper()
	return uParticle(t, o, ResolvedTerm{Term: uLocal(t, uq(local), uq("T"))})
}

// cmGroup wraps a model group in a particle with the given occurrence range.
func cmGroup(t testing.TB, o Occurs, compositor Compositor, particles ...Particle) Particle {
	t.Helper()
	return uParticle(t, o, ResolvedTerm{Term: uGroup(t, compositor, particles...)})
}

// cmPermutations returns every ordering of names, each a fresh slice, in a
// deterministic order (STYLE D2) so a failure names the same permutation on
// every run. It is what an all group's tests enumerate: §3.8.4.1.3 makes the
// group's language the interleave of its members' own, and for single-item
// members that is exactly the permutations.
func cmPermutations(names ...string) [][]string {
	if len(names) <= 1 {
		return [][]string{append([]string(nil), names...)}
	}
	var out [][]string
	for i := range names {
		rest := make([]string, 0, len(names)-1)
		rest = append(rest, names[:i]...)
		rest = append(rest, names[i+1:]...)
		for _, p := range cmPermutations(rest...) {
			out = append(out, append([]string{names[i]}, p...))
		}
	}
	return out
}

// A sequence takes its members in order, and each item is ·attributed to· the
// particle that consumed it (§3.8.4.1.1, cvc-accept clause 2.3.1).
func TestMatcherTakesASequenceInOrder(t *testing.T) {
	m := cmMatcher(t, cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
		cmLeaf(t, "a", uOccurs(t, 1, 1)),
		cmLeaf(t, "b", uOccurs(t, 1, 1))), nil)

	a, ok := m.Next(uq("a"))
	if !ok {
		t.Fatal("Next(a) declined the first member of the sequence")
	}
	d, isDecl := a.(ElementDeclaration)
	if !isDecl {
		t.Fatalf("Next(a) attributed the item to %T, want an ElementDeclaration", a)
	}
	if d.Name() != uq("a") {
		t.Errorf("attributed to %s, want %s", d.Name(), uq("a"))
	}
	if m.Accepting() {
		t.Error("Accepting() = true with the second member of the sequence still owed")
	}
	cmAccept(t, m, "b")
}

// A child no live particle admits is rejected, and rejecting it changes
// nothing: the same name put again after a name that DOES fit still fails, and
// the sequence is judged as if the rejected name had never been offered.
func TestMatcherRejectsAnUnadmittedChildWithoutMoving(t *testing.T) {
	m := cmMatcher(t, cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
		cmLeaf(t, "a", uOccurs(t, 1, 1)),
		cmLeaf(t, "b", uOccurs(t, 1, 1))), nil)

	if _, ok := m.Next(uq("a")); !ok {
		t.Fatal("Next(a) declined the first member")
	}
	if _, ok := m.Next(uq("zzz")); ok {
		t.Fatal("Next(zzz) took a name no particle admits")
	}
	cmAccept(t, m, "b")
}

// A sequence that stops short of a particle it must satisfy is not accepted,
// though every item in it was taken.
func TestMatcherRejectsASequenceThatEndsShort(t *testing.T) {
	m := cmMatcher(t, cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
		cmLeaf(t, "a", uOccurs(t, 1, 1)),
		cmLeaf(t, "b", uOccurs(t, 2, 3))), nil)

	if i := cmFeed(t, m, "a", "b"); i != 2 {
		t.Fatalf("Next rejected position %d, want both items taken", i)
	}
	if m.Accepting() {
		t.Error("Accepting() = true with only one of b's two required occurrences taken")
	}
	cmAccept(t, m, "b")
}

// An occurrence range is a counter, not an unfolding: a repeated particle takes
// items up to its {max occurs} and no further, and the particle after it takes
// the next one.
func TestMatcherCountsOccurrences(t *testing.T) {
	m := cmMatcher(t, cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
		cmLeaf(t, "a", uOccurs(t, 1, 2)),
		cmLeaf(t, "b", uOccurs(t, 1, 1))), nil)

	cmAccept(t, m, "a", "a", "b")

	over := cmMatcher(t, cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
		cmLeaf(t, "a", uOccurs(t, 1, 2)),
		cmLeaf(t, "b", uOccurs(t, 1, 1))), nil)
	if i := cmFeed(t, over, "a", "a", "a"); i != 2 {
		t.Errorf("the third a was rejected at position %d, want position 2 ({max occurs} = 2)", i)
	}
}

// A repeated choice starts a new iteration once the member it took is closed
// (§3.8.4.1.2), and stops at the group's own {max occurs}.
func TestMatcherRepeatsAChoice(t *testing.T) {
	model := func() Particle {
		return cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
			cmGroup(t, uOccurs(t, 1, 3), CompositorChoice,
				cmLeaf(t, "a", uOccurs(t, 1, 1)),
				cmLeaf(t, "b", uOccurs(t, 1, 1))))
	}
	cmAccept(t, cmMatcher(t, model(), nil), "a", "b", "a")

	over := cmMatcher(t, model(), nil)
	if i := cmFeed(t, over, "a", "b", "a", "b"); i != 3 {
		t.Errorf("the fourth item was rejected at position %d, want position 3 ({max occurs} = 3)", i)
	}
}

// An all group's members interleave (§3.8.4.1.3): each keeps its own counter,
// so a member may be taken again after another one has been.
func TestMatcherInterleavesAnAllGroup(t *testing.T) {
	model := func() Particle {
		return cmGroup(t, uOccurs(t, 1, 1), CompositorAll,
			cmLeaf(t, "a", uOccurs(t, 1, 2)),
			cmLeaf(t, "b", uOccurs(t, 1, 1)))
	}
	cmAccept(t, cmMatcher(t, model(), nil), "a", "b", "a")

	short := cmMatcher(t, model(), nil)
	if i := cmFeed(t, short, "a"); i != 1 {
		t.Fatalf("Next rejected a at position %d", i)
	}
	if short.Accepting() {
		t.Error("Accepting() = true with the all group's b member never taken")
	}
}

// An all group's member is SUSPENDED, not finished, when the next item belongs
// to a sibling: §3.8.4.1.3's interleave lets a member owing more occurrences be
// left and resumed, and only the group collects what every member owes. A walk
// that read a member's {min occurs} on the way out would reject this sequence
// at its second item, c having taken one of the two it needs.
func TestMatcherResumesAnAllGroupMemberThatOwesOccurrences(t *testing.T) {
	model := func() Particle {
		return cmGroup(t, uOccurs(t, 1, 1), CompositorAll,
			cmLeaf(t, "a", uOccurs(t, 0, 5)),
			cmLeaf(t, "b", uOccurs(t, 1, 5)),
			uParticle(t, uUnbounded(t, 2), ResolvedTerm{Term: uLocal(t, uq("c"), uq("T"))}),
			cmLeaf(t, "d", uOccurs(t, 1, 1)))
	}
	cmAccept(t, cmMatcher(t, model(), nil), "a", "b", "d", "c", "a", "c", "c", "a", "a", "b")

	short := cmMatcher(t, model(), nil)
	if i := cmFeed(t, short, "c", "b", "d"); i != 3 {
		t.Fatalf("Next rejected position %d, want all three taken", i)
	}
	if short.Accepting() {
		t.Error("Accepting() = true with only one of c's two required occurrences taken")
	}
}

// Where an ·element particle· and a ·wildcard particle· both admit one name —
// the competition 1.1 permits — the item goes to the Element Declaration, which
// is cvc-accept's closing Note and PRINCIPLES 14. A name only the wildcard
// admits still goes to the wildcard.
func TestMatcherPrefersAnElementParticleToAWildcard(t *testing.T) {
	model := func() Particle {
		return cmGroup(t, uOccurs(t, 1, 1), CompositorChoice,
			cmLeaf(t, "a", uOccurs(t, 1, 1)),
			uParticle(t, uOccurs(t, 1, 1), ResolvedTerm{
				Term: uWildcard(t, NamespaceConstraintAny, nil, ProcessLax)}))
	}

	got, ok := cmMatcher(t, model(), nil).Next(uq("a"))
	if !ok {
		t.Fatal("Next(a) declined a name both particles admit")
	}
	if _, isDecl := got.(ElementDeclaration); !isDecl {
		t.Errorf("Next(a) attributed the item to %T, want the ElementDeclaration", got)
	}

	got, ok = cmMatcher(t, model(), nil).Next(uq("other"))
	if !ok {
		t.Fatal("Next(other) declined a name the wildcard admits")
	}
	if _, isWild := got.(Wildcard); !isWild {
		t.Errorf("Next(other) attributed the item to %T, want the Wildcard", got)
	}
}

// cvc-accept clause 2.3.2 admits an item whose name is in the ·substitution
// group· of the particle's own declaration D, and the attribution is D — the
// particle's declaration, not the ·substituting declaration·.
func TestMatcherAdmitsASubstitutionGroupMember(t *testing.T) {
	extra := func(b *SchemaBuilder) {
		b.AddElement(uGlobal(t, uq("head"), uq("T")))
		b.AddElement(uGlobal(t, uq("member"), uq("T"), uq("head")))
	}
	m := cmMatcher(t, cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
		uParticle(t, uOccurs(t, 1, 1), ElementDeclarationRef{Name: uq("head")})), extra)

	got, ok := m.Next(uq("member"))
	if !ok {
		t.Fatal("Next(member) declined a member of the head's ·substitution group·")
	}
	d, isDecl := got.(ElementDeclaration)
	if !isDecl {
		t.Fatalf("attributed the item to %T, want an ElementDeclaration", got)
	}
	if d.Name() != uq("head") {
		t.Errorf("attributed to %s, want the particle's own declaration %s", d.Name(), uq("head"))
	}
	if !m.Accepting() {
		t.Error("Accepting() = false after the one member the model requires")
	}
}

// A repeated group whose body holds no repeating particle fills its open
// iteration before starting the next, and carries one cursor doing it: no other
// partition of the items accepts what that one rejects. cvc-accept counts an
// ·emptiable· body's unstarted iterations as satisfying {min occurs} — so
// (a?, b?){2,2} takes "a b" as one iteration and still accepts, while
// (a?, b){2,2}, whose body is not ·emptiable·, does not.
func TestMatcherFillsAnIterationBeforeStartingTheNext(t *testing.T) {
	emptiable := cmMatcher(t, cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
		cmGroup(t, uOccurs(t, 2, 2), CompositorSequence,
			cmLeaf(t, "a", uOccurs(t, 0, 1)),
			cmLeaf(t, "b", uOccurs(t, 0, 1)))), nil)
	cmAccept(t, emptiable, "a", "b")

	required := cmMatcher(t, cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
		cmGroup(t, uOccurs(t, 2, 2), CompositorSequence,
			cmLeaf(t, "a", uOccurs(t, 0, 1)),
			cmLeaf(t, "b", uOccurs(t, 1, 1)))), nil)
	if i := cmFeed(t, required, "a", "b"); i != 2 {
		t.Fatalf("Next rejected position %d of a b, want both taken", i)
	}
	if required.Accepting() {
		t.Error("Accepting() = true after one iteration of a group that requires two")
	}
	cmAccept(t, required, "b")
}

// An empty child sequence is accepted exactly when the content model's particle
// is ·emptiable·.
func TestMatcherAcceptsTheEmptySequenceOnlyWhenEmptiable(t *testing.T) {
	optional := cmMatcher(t, cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
		cmLeaf(t, "a", uOccurs(t, 0, 1))), nil)
	if !optional.Accepting() {
		t.Error("Accepting() = false for an empty sequence against an ·emptiable· model")
	}

	required := cmMatcher(t, cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
		cmLeaf(t, "a", uOccurs(t, 1, 1))), nil)
	if required.Accepting() {
		t.Error("Accepting() = true for an empty sequence against a model requiring one item")
	}
}

// The walk COUNTS occurrences instead of unfolding them, so an occurrence range
// no unfolding could materialize costs one counter. A model built by unfolding
// would allocate 200000 positions before reading a single child.
func TestMatcherCountsAHugeOccurrenceRangeRatherThanUnfoldingIt(t *testing.T) {
	const min, max = 100000, 200000
	m := cmMatcher(t, cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
		cmLeaf(t, "a", uOccurs(t, min, max)),
		cmLeaf(t, "b", uOccurs(t, 1, 1))), nil)

	for i := 0; i < min; i++ {
		if _, ok := m.Next(uq("a")); !ok {
			t.Fatalf("Next(a) rejected occurrence %d of %d", i+1, min)
		}
	}
	if m.Accepting() {
		t.Error("Accepting() = true with b still owed")
	}
	cmAccept(t, m, "b")
}

// The walk descends each optional level once and never revisits it. This model
// nests 40 ·emptiable· groups, so an implementation that backtracked over
// "taken or skipped" at every level would explore 2^40 combinations to place
// one item (PRINCIPLES 14); this one places it in 40 steps.
func TestMatcherDoesNotBacktrackOverNestedOptionalGroups(t *testing.T) {
	const depth = 40
	inner := cmGroup(t, uOccurs(t, 1, 1), CompositorSequence, cmLeaf(t, "z", uOccurs(t, 1, 1)))
	for i := depth; i > 0; i-- {
		inner = cmGroup(t, uOccurs(t, 0, 1), CompositorSequence,
			cmLeaf(t, "x"+itoa(i), uOccurs(t, 0, 1)),
			inner)
	}
	m := cmMatcher(t, cmGroup(t, uOccurs(t, 1, 1), CompositorSequence, inner), nil)

	cmAccept(t, m, "z")
}

// itoa spells a small non-negative int without pulling strconv into the test's
// import set for one call.
func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// A {content type} holding no particle is not this driver's question:
// cvc-complex-type clauses 1.1 and 1.2 govern the empty and simple varieties
// directly.
func TestContentMatcherDeclinesAParticlelessContentType(t *testing.T) {
	b := NewSchemaBuilder()
	empty := uNamedType(t, uq("empty"))
	b.AddType(empty)
	s, err := b.Finalize()
	if err != nil {
		t.Fatalf("finalizing: %v", err)
	}
	if _, ok := s.ContentMatcher(empty); ok {
		t.Error("ContentMatcher decided an empty {content type}")
	}
}

// cmOpenMatcher is cmMatcher over a {content type} whose {open content} is
// PRESENT, carrying mode and the wildcard w. The type is built here rather than
// through uCT because that fixture takes no {open content}, and the schema is
// FINALIZED on cmSchema's terms.
func cmOpenMatcher(t *testing.T, mode OpenContentMode, w Wildcard, p Particle) *Matcher {
	t.Helper()
	oc, err := NewOpenContent(xsderr.Loc{}, mode, w)
	if err != nil {
		t.Fatalf("NewOpenContent: %v", err)
	}
	ct, err := NewComplexType(xsderr.Loc{}, uq("ct"), QName{}, nil, DerivationRestriction, false,
		nil, nil, nil, ElementContent{Particle: p, OpenContent: &oc}, nil, nil)
	if err != nil {
		t.Fatalf("NewComplexType: %v", err)
	}
	b := NewSchemaBuilder()
	b.AddType(uNamedType(t, uq("T")))
	b.AddType(ct)
	s, err := b.Finalize()
	if err != nil {
		t.Fatalf("finalizing the fixture schema: %v", err)
	}
	m, ok := s.ContentMatcher(ct)
	if !ok {
		t.Fatalf("ContentMatcher declined a {content type} whose {open content} is present")
	}
	return m
}

// cmNext takes one name and asserts it was taken, returning what it was
// ·attributed to·.
func cmNext(t *testing.T, m *Matcher, local string) Attribution {
	t.Helper()
	a, ok := m.Next(uq(local))
	if !ok {
		t.Fatalf("Next(%s) rejected a name the model admits", local)
	}
	return a
}

// cmWantDeclaration asserts the item was ·attributed to· the element
// declaration named local — the {particle} half, S1.
func cmWantDeclaration(t *testing.T, a Attribution, local string) {
	t.Helper()
	d, isDecl := a.(ElementDeclaration)
	if !isDecl {
		t.Fatalf("attributed to %T, want the ElementDeclaration %s", a, local)
	}
	if d.Name() != uq(local) {
		t.Errorf("attributed to %s, want %s", d.Name(), uq(local))
	}
}

// cmWantWildcard asserts the item was ·attributed to· a ·wildcard particle·
// (§3.9.1, key-wp) whose {process contents} is pc. The variant alone settles
// which of the two ·attributions· (§3.4.4.4) this is, so pc pins the particle
// and nothing else.
func cmWantWildcard(t *testing.T, a Attribution, pc ProcessContents) {
	t.Helper()
	w, isWild := a.(Wildcard)
	if !isWild {
		t.Fatalf("attributed to %T, want a Wildcard", a)
	}
	if w.ProcessContents() != pc {
		t.Errorf("attributed to a %s wildcard, want the %s one", w.ProcessContents(), pc)
	}
}

// cmWantOpenContent asserts the item was ·attributed to· the {open content}
// record itself, whose {mode} is mode — the variant [Attribution] keeps apart
// from a ·wildcard particle· (§3.4.4.4), never nil on this arm.
func cmWantOpenContent(t *testing.T, a Attribution, mode OpenContentMode) {
	t.Helper()
	oc, isOpen := a.(*OpenContent)
	if !isOpen {
		t.Fatalf("attributed to %T, want the *OpenContent", a)
	}
	if oc == nil {
		t.Fatal("attributed to a nil *OpenContent, which Matcher.Next never reports")
	}
	if oc.Mode() != mode {
		t.Errorf("attributed to an {open content} in {mode} %s, want %s", oc.Mode(), mode)
	}
}

// cmOpenWildcard is the {open content} wildcard these fixtures share: it admits
// every name, so what a test's items are ·attributed to· turns on the
// {particle} alone.
func cmOpenWildcard(t *testing.T) Wildcard {
	t.Helper()
	return uWildcard(t, NamespaceConstraintAny, nil, ProcessSkip)
}

// Under {mode} suffix, S1 is the longest prefix the {particle} takes and S2 is
// everything after it, ·attributed to· the {open content} (cvc-complex-content
// clause 2, §3.4.4.4). S1 still has to satisfy the {particle}, which
// Accepting reports.
func TestMatcherTakesASuffixOfOpenContent(t *testing.T) {
	m := cmOpenMatcher(t, OpenContentSuffix, cmOpenWildcard(t),
		cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
			cmLeaf(t, "a", uOccurs(t, 1, 1)),
			cmLeaf(t, "b", uOccurs(t, 1, 1))))

	cmWantDeclaration(t, cmNext(t, m, "a"), "a")
	cmWantDeclaration(t, cmNext(t, m, "b"), "b")
	cmWantOpenContent(t, cmNext(t, m, "x"), OpenContentSuffix)
	cmWantOpenContent(t, cmNext(t, m, "y"), OpenContentSuffix)
	if !m.Accepting() {
		t.Error("Accepting() = false for an S1 the {particle} took whole")
	}
}

// Clause 2.1's S = S1 + S2 is a CONCATENATION, so suffix mode never offers the
// {particle} another item once an open-content item has started S2: the b here
// goes to the {open content} and the {particle} is left owing it, which
// Accepting charges. Interleave mode takes the same sequence (the test below),
// which is the whole difference between the two {mode}s.
func TestMatcherSuffixOpenContentNeverReturnsToTheParticle(t *testing.T) {
	m := cmOpenMatcher(t, OpenContentSuffix, cmOpenWildcard(t),
		cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
			cmLeaf(t, "a", uOccurs(t, 1, 1)),
			cmLeaf(t, "b", uOccurs(t, 1, 1))))

	cmWantDeclaration(t, cmNext(t, m, "a"), "a")
	cmWantOpenContent(t, cmNext(t, m, "x"), OpenContentSuffix)
	cmWantOpenContent(t, cmNext(t, m, "b"), OpenContentSuffix)
	if m.Accepting() {
		t.Error("Accepting() = true with the b particle left unsatisfied by S1")
	}
}

// Clause 3.1's S1 × S2 is the interleave operator, so an open-content item is
// not a suffix of anything: the {particle} takes the item after it exactly as
// if the item had not arrived (clause 3.3 evaluated against S3, the part of S1
// before it). This sequence is the minimum discriminator between the two
// {mode}s — clause 2 rejects it, clause 3 accepts it.
func TestMatcherInterleavesOpenContentWithTheParticle(t *testing.T) {
	m := cmOpenMatcher(t, OpenContentInterleave, cmOpenWildcard(t),
		cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
			cmLeaf(t, "a", uOccurs(t, 1, 1)),
			cmLeaf(t, "b", uOccurs(t, 1, 1))))

	cmWantDeclaration(t, cmNext(t, m, "a"), "a")
	cmWantOpenContent(t, cmNext(t, m, "x"), OpenContentInterleave)
	cmWantDeclaration(t, cmNext(t, m, "b"), "b")
	if !m.Accepting() {
		t.Error("Accepting() = false after the {particle} took the whole of S1")
	}
}

// The {open content} is the LAST tier, under both kinds of ·basic particle·:
// clause 3.3 licenses it only where the {particle} has no ·path· for the item,
// so a name an ·element particle· admits goes to the declaration and a name
// only a ·wildcard particle· admits goes to that particle's wildcard
// (PRINCIPLES 14).
func TestMatcherPrefersTheParticleToTheOpenContentWildcard(t *testing.T) {
	m := cmOpenMatcher(t, OpenContentInterleave, cmOpenWildcard(t),
		cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
			cmLeaf(t, "a", uOccurs(t, 1, 1)),
			uParticle(t, uOccurs(t, 1, 1), ResolvedTerm{
				Term: uWildcard(t, NamespaceConstraintNot, []Namespace{NamespaceName(uns)}, ProcessLax)})))

	cmWantDeclaration(t, cmNext(t, m, "a"), "a")
	got, ok := m.Next(QName{Space: "urn:other", Local: "wp"})
	if !ok {
		t.Fatal("Next rejected a name the ·wildcard particle· admits")
	}
	cmWantWildcard(t, got, ProcessLax)
	cmWantOpenContent(t, cmNext(t, m, "x"), OpenContentInterleave)
}

// The {open content} arm carries the record's IDENTITY and not a copy of it:
// the pointer Next reports is the one ElementContent.OpenContent holds, so a
// consumer holding the ·governing type definition· can ask whether THIS item
// went to THIS type's {open content} rather than comparing {wildcard} values —
// which a Wildcard, holding slices, cannot answer.
func TestMatcherAttributesAnOpenContentItemToTheRecordItself(t *testing.T) {
	oc, err := NewOpenContent(xsderr.Loc{}, OpenContentInterleave, cmOpenWildcard(t))
	if err != nil {
		t.Fatalf("NewOpenContent: %v", err)
	}
	content := ElementContent{Particle: cmLeaf(t, "a", uOccurs(t, 1, 1)), OpenContent: &oc}
	ct, err := NewComplexType(xsderr.Loc{}, uq("ct"), QName{}, nil, DerivationRestriction, false,
		nil, nil, nil, content, nil, nil)
	if err != nil {
		t.Fatalf("NewComplexType: %v", err)
	}
	b := NewSchemaBuilder()
	b.AddType(uNamedType(t, uq("T")))
	b.AddType(ct)
	s, err := b.Finalize()
	if err != nil {
		t.Fatalf("finalizing the fixture schema: %v", err)
	}
	m, ok := s.ContentMatcher(ct)
	if !ok {
		t.Fatal("ContentMatcher declined a {content type} whose {open content} is present")
	}

	a := cmNext(t, m, "x")

	held, elementOnly := ct.ContentType().(ElementContent)
	if !elementOnly {
		t.Fatalf("{content type} is %T, want the ElementContent built above", ct.ContentType())
	}
	took, isOpen := a.(*OpenContent)
	if !isOpen {
		t.Fatalf("attributed to %T, want the *OpenContent", a)
	}
	if took != held.OpenContent {
		t.Errorf("attributed to the {open content} at %p, want the one the type holds at %p", took, held.OpenContent)
	}
}

// An item the {particle} cannot take and {open content}.{wildcard} does not
// admit (cvc-wildcard §3.10.4.1) satisfies neither clause 2 nor clause 3 and is
// rejected — and rejecting it leaves the walk where it stood, so the item the
// {particle} was waiting for is still taken.
func TestMatcherRejectsANameNeitherTheParticleNorTheOpenWildcardAdmits(t *testing.T) {
	m := cmOpenMatcher(t, OpenContentInterleave,
		uWildcard(t, NamespaceConstraintNot, []Namespace{NamespaceName(uns)}, ProcessSkip),
		cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
			cmLeaf(t, "a", uOccurs(t, 1, 1)),
			cmLeaf(t, "b", uOccurs(t, 1, 1))))

	cmWantDeclaration(t, cmNext(t, m, "a"), "a")
	if _, ok := m.Next(uq("zzz")); ok {
		t.Fatal("Next took a name neither the {particle} nor the {open content} wildcard admits")
	}
	cmWantDeclaration(t, cmNext(t, m, "b"), "b")
	if !m.Accepting() {
		t.Error("Accepting() = false after a rejected name that should have changed nothing")
	}
}

// cmNested is (a{1,2}, b?){2,2}, cvc-accept's own named non-determinism and the
// file comment's worked counter-example: L(a{1,2}, b?) is {(a), (a b), (a a),
// (a a b)} and the outer particle's fixed n = 2 makes L(P) the concatenation of
// two of them.
func cmNested(t testing.TB) Particle {
	t.Helper()
	return cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
		cmGroup(t, uOccurs(t, 2, 2), CompositorSequence,
			cmLeaf(t, "a", uOccurs(t, 1, 2)),
			cmLeaf(t, "b", uOccurs(t, 0, 1))))
}

// cvc-accept clause 3.1 asks whether SOME partition of the sequence into
// iterations is valid, and the greedy one is not always it: (a{1,2}, b?){2,2}
// admits "a a b" as (a)(a b), while filling the open iteration first consumes
// the whole of it as one iteration and leaves the second owing a mandatory a.
// "a a a b" is (a a)(a b), where the greedy partition is the one that works, so
// the two run side by side.
func TestMatcherAcceptsAPartitionTheGreedyWalkMisses(t *testing.T) {
	cmAccept(t, cmMatcher(t, cmNested(t), nil), "a", "a", "b")
	cmAccept(t, cmMatcher(t, cmNested(t), nil), "a", "a", "a", "b")
}

// No partition is not every partition. "a a b b" needs an iteration ending in
// two b's and "a a a a a" a fifth a with both iterations full, and neither is a
// word of the body's language, so widening the walk over partitions accepts
// nothing L(P) does not.
func TestMatcherRejectsASequenceNoPartitionAdmits(t *testing.T) {
	over := cmMatcher(t, cmNested(t), nil)
	if i := cmFeed(t, over, "a", "a", "b", "b"); i != 3 {
		t.Errorf("the second b was rejected at position %d, want position 3", i)
	}

	full := cmMatcher(t, cmNested(t), nil)
	if i := cmFeed(t, full, "a", "a", "a", "a", "a"); i != 4 {
		t.Errorf("the fifth a was rejected at position %d, want position 4", i)
	}

	short := cmMatcher(t, cmNested(t), nil)
	if i := cmFeed(t, short, "a"); i != 1 {
		t.Fatalf("Next rejected a at position %d", i)
	}
	if short.Accepting() {
		t.Error("Accepting() = true with the second iteration's mandatory a still owed")
	}
}

// An iteration a partition closes has to be a whole word of the group's
// language: (a{1,2}, b){2,2} cannot draw a boundary after the second a, b being
// mandatory, so "a a b" is outside L(P) even though both the greedy partition
// and the one that starts a second iteration at that a can consume every item
// of it. Widening the walk over partitions must not manufacture that boundary.
func TestMatcherClosesAnIterationOnlyOnAWholeWordOfTheBody(t *testing.T) {
	model := func() Particle {
		return cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
			cmGroup(t, uOccurs(t, 2, 2), CompositorSequence,
				cmLeaf(t, "a", uOccurs(t, 1, 2)),
				cmLeaf(t, "b", uOccurs(t, 1, 1))))
	}
	cmAccept(t, cmMatcher(t, model(), nil), "a", "b", "a", "b")
	cmAccept(t, cmMatcher(t, model(), nil), "a", "a", "b", "a", "b")

	short := cmMatcher(t, model(), nil)
	if i := cmFeed(t, short, "a", "a", "b"); i != 3 {
		t.Fatalf("Next rejected position %d of a a b, want all three taken", i)
	}
	if short.Accepting() {
		t.Error("Accepting() = true for a a b, which no partition into two iterations of (a{1,2}, b) covers")
	}
}

// The FIRST viable partition is not the greedy one here, and no later item
// rescues it: (a{2,3}){2,2} takes "a a a a" only as (a a)(a a), while a walk
// that fills the open iteration draws (a a a)(a) and leaves the second
// iteration one occurrence short of a's {min occurs}. One a fewer and one a
// more are outside L(P) either way.
func TestMatcherSplitsIterationsAgainstTheGreedyBoundary(t *testing.T) {
	model := func() Particle {
		return cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
			cmGroup(t, uOccurs(t, 2, 2), CompositorSequence,
				cmLeaf(t, "a", uOccurs(t, 2, 3))))
	}
	cmAccept(t, cmMatcher(t, model(), nil), "a", "a", "a", "a")
	cmAccept(t, cmMatcher(t, model(), nil), "a", "a", "a", "a", "a")
	cmAccept(t, cmMatcher(t, model(), nil), "a", "a", "a", "a", "a", "a")

	short := cmMatcher(t, model(), nil)
	if i := cmFeed(t, short, "a", "a", "a"); i != 3 {
		t.Fatalf("Next rejected position %d of three a's, want all three taken", i)
	}
	if short.Accepting() {
		t.Error("Accepting() = true for three a's, which no partition into two iterations of a{2,3} covers")
	}

	over := cmMatcher(t, model(), nil)
	if i := cmFeed(t, over, "a", "a", "a", "a", "a", "a", "a"); i != 6 {
		t.Errorf("the seventh a was rejected at position %d, want position 6 (2 × 3)", i)
	}
}

// Widening the walk over partitions is not a search over the items already
// taken: the live set is CLAMPED and collapsed into spans, so it stays a
// constant of the schema however long the instance is.
//
// Both models below put a number of partitions in flight that grows with the
// instance, and neither puts a live entry in flight per partition. The first
// model's partitions of n a's into iterations of one to three number
// tribonacci(n) — about 1.8^n, which is 10^5000 here. The second draws its one
// iteration boundary anywhere in the items so far, so it has n of them after n
// items and is the shape one entry per partition could not carry at all: the
// clamped product that declined it is 3 × 20001.
//
// The ceiling asserted is a small constant and NOT maxPartitionStates, which
// 20000 live entries would also sit under.
func TestMatcherBoundsTheCursorSetOverANestedRepetition(t *testing.T) {
	const items, ceiling = 20000, 16
	for _, tc := range []struct {
		name  string
		model func() Particle
	}{
		{"an unbounded outer range", func() Particle {
			return cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
				cmGroup(t, uUnbounded(t, 1), CompositorSequence,
					cmLeaf(t, "a", uOccurs(t, 1, 3))))
		}},
		{"an inner range as wide as the instance", func() Particle {
			return cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
				cmGroup(t, uOccurs(t, 1, 2), CompositorSequence,
					cmLeaf(t, "a", uOccurs(t, 1, items))))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := cmMatcher(t, tc.model(), nil)
			for i := 0; i < items; i++ {
				if _, ok := m.Next(uq("a")); !ok {
					t.Fatalf("Next(a) rejected occurrence %d of %d", i+1, items)
				}
				if len(m.live) > ceiling {
					t.Fatalf("after %d items the walk carries %d live entries, want at most %d", i+1, len(m.live), ceiling)
				}
			}
			if !m.Accepting() {
				t.Error("Accepting() = false after a whole number of iterations")
			}
		})
	}
}

// A repeated body that puts a MANDATORY particle after each of its repeating
// particles pins its own iteration boundary: an item can extend the open
// iteration or start the next, never both, so no cursor ever splits and the
// occurrence ranges may be as wide as they like. The model below products
// 20001×4×4×4 over the subtree the old rule widened and was declined for it,
// while its live set is the one cursor a model with no ·ambiguous· node at all
// carries.
func TestContentMatcherCarriesAWideRepetitionWhoseBodyPinsTheBoundary(t *testing.T) {
	m := cmMatcher(t, cmWideBody(t), nil)

	cmAccept(t, m, "a", "b", "c")
	cmAccept(t, m, "a", "a", "b", "c", "c")

	short := cmMatcher(t, cmWideBody(t), nil)
	if i := cmFeed(t, short, "a", "b"); i != 2 {
		t.Fatalf("Next rejected %d items of the body's own prefix, want both taken", 2-i)
	}
	if short.Accepting() {
		t.Error("Accepting() = true for an iteration owing its mandatory c")
	}
	skipped := cmMatcher(t, cmWideBody(t), nil)
	if i := cmFeed(t, skipped, "a", "c"); i != 1 {
		t.Errorf("Next took c over the mandatory b at position %d, want a rejection at 1", i)
	}
}

// cmWideBody is (a{1,3}, b{1,3}, c{1,3}){1,20000}: every repeating member is
// followed by a mandatory one, so b and c can never start a fresh iteration
// while the open one could still take them, and a is never the item after a
// complete iteration and a candidate to extend one at the same time.
func cmWideBody(t testing.TB) Particle {
	t.Helper()
	return cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
		cmGroup(t, uOccurs(t, 1, 20000), CompositorSequence,
			cmLeaf(t, "a", uOccurs(t, 1, 3)),
			cmLeaf(t, "b", uOccurs(t, 1, 3)),
			cmLeaf(t, "c", uOccurs(t, 1, 3))))
}

// The width that model carries is carried at CONSTANT cost per item, which is
// what the decline was protecting: the walk holds one cursor over a sequence
// long enough that a set growing with the instance would be visible.
func TestMatcherBoundsTheCursorSetOverAWidePinnedRepetition(t *testing.T) {
	// The ceiling asserted is a small constant and NOT maxPartitionStates,
	// which a set growing by one entry per item would stay under for the whole
	// of this instance.
	const iterations, ceiling = 6667, 1 // 20001 items
	m := cmMatcher(t, cmWideBody(t), nil)

	for i := 0; i < iterations; i++ {
		for _, n := range []string{"a", "b", "c"} {
			if _, ok := m.Next(uq(n)); !ok {
				t.Fatalf("Next(%s) rejected iteration %d of %d", n, i+1, iterations)
			}
			if len(m.live) > ceiling {
				t.Fatalf("after iteration %d the walk carries %d cursors, want at most %d", i+1, len(m.live), ceiling)
			}
		}
	}
	if !m.Accepting() {
		t.Error("Accepting() = false after a whole number of iterations")
	}
}

// The width no ESTIMATE reaches is carried by the ENCODING. (a{1,500}){1,500}
// really does put a quarter of a million partitions in flight at once — every
// (iterations closed, items in the open iteration) pair a prefix of a's admits,
// and the product that used to decline it over-counts them by under a percent —
// so one live entry per partition cannot hold it and a span of adjacent counts
// per iteration can. ContentMatcher decides the model, and decides it in both
// directions: a run of a's L(P) admits is taken and accepted, and a name no
// particle of the model admits is still rejected.
func TestContentMatcherCarriesANestedRepetitionNoCursorPerPartitionCouldHold(t *testing.T) {
	const items = 1000
	s, ct := cmSchema(t, cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
		cmGroup(t, uOccurs(t, 1, 500), CompositorSequence,
			cmLeaf(t, "a", uOccurs(t, 1, 500)))), nil)

	m, ok := s.ContentMatcher(ct)
	if !ok {
		t.Fatal("ContentMatcher declined (a{1,500}){1,500}, whose partitions a span per iteration covers")
	}
	if m.Accepting() {
		t.Error("Accepting() = true for the empty sequence, which no iteration of a{1,500} is a word of")
	}
	for i := 0; i < items; i++ {
		if _, ok := m.Next(uq("a")); !ok {
			t.Fatalf("Next(a) rejected occurrence %d of %d", i+1, items)
		}
		if len(m.live) > maxPartitionStates {
			t.Fatalf("after %d items the walk carries %d live entries, want at most %d", i+1, len(m.live), maxPartitionStates)
		}
	}
	if !m.Accepting() {
		t.Errorf("Accepting() = false after %d a's, which L(P) partitions into two iterations and more", items)
	}
	if _, ok := m.Next(uq("b")); ok {
		t.Error("Next(b) took a name no particle of the model admits")
	}
}

// A live entry covers a RUN of occurrence counts, and every count in one run
// has to answer {min occurs} alike or one walk step decides for partitions that
// disagree. (a{3,4}){1,2} is the shortest model where a run straddles that
// answer: L(P) is a^3, a^4, a^6, a^7 and a^8, so after five a's the partitions
// standing at three in the open iteration can close it and the ones standing at
// two cannot, and a walk that read the run's lowest count for both would reject
// "a a a a a a", which is (a a a)(a a a).
func TestMatcherClosesAnIterationOnlyForTheCountsThatReachedMinOccurs(t *testing.T) {
	model := func() Particle {
		return cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
			cmGroup(t, uOccurs(t, 1, 2), CompositorSequence,
				cmLeaf(t, "a", uOccurs(t, 3, 4))))
	}
	for _, n := range []int{3, 4, 6, 7, 8} {
		cmAccept(t, cmMatcher(t, model(), nil), cmRepeat("a", n)...)
	}

	short := cmMatcher(t, model(), nil)
	if i := cmFeed(t, short, cmRepeat("a", 5)...); i != 5 {
		t.Fatalf("Next rejected a at position %d of five, want all five taken", i)
	}
	if short.Accepting() {
		t.Error("Accepting() = true for five a's, which no partition into iterations of a{3,4} covers")
	}

	over := cmMatcher(t, model(), nil)
	if i := cmFeed(t, over, cmRepeat("a", 9)...); i != 8 {
		t.Errorf("the ninth a was rejected at position %d, want position 8 (2 × 4)", i)
	}
}

// cmRepeat is one name n times, the sequence an occurrence-range test feeds.
func cmRepeat(name string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = name
	}
	return out
}

// The {max occurs} arithmetic survives the span: (a{1,20}){1,20} — a nested
// repetition the old ceiling declined for its product of 441 — takes four
// hundred a's and not the four hundred and first. A cover that dropped a
// partition would cut the run short, and one that invented a partition would
// run past the boundary.
func TestMatcherDecidesTheOccurrenceBoundaryOfANestedRepetition(t *testing.T) {
	const wide, items = 20, 400
	m := cmMatcher(t, cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
		cmGroup(t, uOccurs(t, 1, wide), CompositorSequence,
			cmLeaf(t, "a", uOccurs(t, 1, wide)))), nil)

	for i := 0; i < items; i++ {
		if _, ok := m.Next(uq("a")); !ok {
			t.Fatalf("Next(a) rejected occurrence %d of %d, which %d iterations of a{1,%d} cover", i+1, items, wide, wide)
		}
	}
	if !m.Accepting() {
		t.Errorf("Accepting() = false after %d a's, the whole of the model's language", items)
	}
	if _, ok := m.Next(uq("a")); ok {
		t.Errorf("Next(a) took occurrence %d, past %d × %d", items+1, wide, wide)
	}
}

// cmNestedAll is the shape cos-all-limited (§3.8.6.2) clause 2 admits inside an
// all group and nothing else admits: an all group among another all group's
// {particles}, through a {min occurs} = {max occurs} = 1 particle (clause 1.3).
//
// The nested group sits BETWEEN two leaf members rather than last, so every
// sequence below puts some name to a sibling AFTER the suspended group has been
// offered that name and refused it. That is the order a resume which fails
// without unwinding the path it appended to gets wrong, and it is invisible
// where the group is the last member tried.
func cmNestedAll(t testing.TB) Particle {
	t.Helper()
	return cmGroup(t, uOccurs(t, 1, 1), CompositorAll,
		cmLeaf(t, "a", uOccurs(t, 1, 1)),
		cmGroup(t, uOccurs(t, 1, 1), CompositorAll,
			cmLeaf(t, "c", uOccurs(t, 1, 1)),
			cmLeaf(t, "d", uOccurs(t, 1, 1))),
		cmLeaf(t, "b", uOccurs(t, 1, 1)))
}

// §3.8.4.1.3 folds a nested all group's language into the SAME interleave as
// its parent's: L(M) is S1 × … × Sn, where Si is any word of Pi's own language,
// and nothing there requires one member's items to be contiguous. So every
// permutation of the outer group's members and the inner group's is a word of
// the outer group.
func TestMatcherInterleavesANestedAllGroupWithItsParentsMembers(t *testing.T) {
	// a c b d alternates between the two groups and is the order an
	// implementation that validated the nested group as one contiguous block
	// would wrongly reject.
	cmAccept(t, cmMatcher(t, cmNestedAll(t), nil), "a", "c", "b", "d")

	for _, p := range cmPermutations("a", "b", "c", "d") {
		cmAccept(t, cmMatcher(t, cmNestedAll(t), nil), p...)
	}
}

// What a suspended member of an all group owes is owed to the group, and a
// member that is itself an all group owes what its OWN members owe: a sequence
// short of any of them is not ·accepted·, though every item in it was taken
// (cvc-accept clauses 2.1 and 3.1, applied at both nesting levels).
func TestMatcherRejectsAnAllGroupLeftIncompleteAtEitherNestingLevel(t *testing.T) {
	for _, tc := range []struct {
		name string
		feed []string
	}{
		{"the inner group is short a member", []string{"a", "b", "c"}},
		{"the outer group is short a member", []string{"c", "a", "d"}},
		{"the inner group was never entered", []string{"a", "b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := cmMatcher(t, cmNestedAll(t), nil)
			if i := cmFeed(t, m, tc.feed...); i != len(tc.feed) {
				t.Fatalf("Next rejected %s at position %d of %v, want every item taken", tc.feed[i], i, tc.feed)
			}
			if m.Accepting() {
				t.Errorf("Accepting() = true after %v, which leaves a member of an all group owed", tc.feed)
			}
		})
	}
}

// Clause 1.3 permits an all group inside an all group inside an all group, and
// the interleave folds at every level, so the walk recurses rather than
// handling one level of nesting.
func TestMatcherInterleavesAllGroupsNestedThreeDeep(t *testing.T) {
	model := func() Particle {
		return cmGroup(t, uOccurs(t, 1, 1), CompositorAll,
			cmLeaf(t, "a", uOccurs(t, 1, 1)),
			cmGroup(t, uOccurs(t, 1, 1), CompositorAll,
				cmLeaf(t, "b", uOccurs(t, 1, 1)),
				cmGroup(t, uOccurs(t, 1, 1), CompositorAll,
					cmLeaf(t, "c", uOccurs(t, 1, 1)),
					cmLeaf(t, "d", uOccurs(t, 1, 1)))))
	}
	for _, p := range cmPermutations("a", "b", "c", "d") {
		cmAccept(t, cmMatcher(t, model(), nil), p...)
	}

	m := cmMatcher(t, model(), nil)
	if i := cmFeed(t, m, "c", "a", "b"); i != 3 {
		t.Fatalf("Next rejected position %d, want all three taken", i)
	}
	if m.Accepting() {
		t.Error("Accepting() = true with the innermost group's d still owed")
	}
}

// {min occurs} and {max occurs} bound a member of a nested all group exactly as
// they bound one of the outer group — cvc-accept clauses 2.1 and 2.2, reached
// through clause 3 at every nesting depth. A singleton taken twice is rejected
// at both levels, and a zero-minimum member is optional at both.
func TestMatcherBoundsOccurrencesAtBothAllNestingLevels(t *testing.T) {
	model := func() Particle {
		return cmGroup(t, uOccurs(t, 1, 1), CompositorAll,
			cmLeaf(t, "a", uOccurs(t, 1, 1)),
			cmLeaf(t, "b", uOccurs(t, 0, 1)),
			cmGroup(t, uOccurs(t, 1, 1), CompositorAll,
				cmLeaf(t, "c", uOccurs(t, 1, 1)),
				cmLeaf(t, "d", uOccurs(t, 0, 1))))
	}
	cmAccept(t, cmMatcher(t, model(), nil), "a", "c")
	cmAccept(t, cmMatcher(t, model(), nil), "d", "a", "c")

	for _, tc := range []struct {
		name string
		feed []string
		at   int
	}{
		{"an outer member twice", []string{"a", "c", "a"}, 2},
		{"an inner member twice", []string{"c", "a", "c"}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := cmMatcher(t, model(), nil)
			if i := cmFeed(t, m, tc.feed...); i != tc.at {
				t.Fatalf("Next stopped at position %d of %v, want the repeat rejected at %d", i, tc.feed, tc.at)
			}
		})
	}
}

// A nested all group occurs exactly once (clause 1.3) but need not contribute
// an item: where every one of its own members is ·emptiable· the occurrence it
// owes can be the empty sequence, and the outer group collects nothing from it
// (cos-group-emptiable §3.9.6.3, cvc-accept clause 3.1).
func TestMatcherAcceptsAnEmptyNestedAllGroup(t *testing.T) {
	model := func() Particle {
		return cmGroup(t, uOccurs(t, 1, 1), CompositorAll,
			cmLeaf(t, "a", uOccurs(t, 1, 1)),
			cmGroup(t, uOccurs(t, 1, 1), CompositorAll,
				cmLeaf(t, "c", uOccurs(t, 0, 1)),
				cmLeaf(t, "d", uOccurs(t, 0, 1))))
	}
	cmAccept(t, cmMatcher(t, model(), nil), "a")
	cmAccept(t, cmMatcher(t, model(), nil), "d", "a")
}

// BenchmarkMatcherNext puts a number to the budget maxPartitionStates is the
// ceiling on: what ONE item of an instance costs in wall time and in
// allocation while the walk carries a live set as wide as that ceiling admits.
// Read its two widths together — the cost is per live region, so the slope
// between them is what a change to the constant multiplies.
//
// The model is cmSiblings and not a nested repetition because its live set
// reaches w regions after O(√w) items where a nest needs w of them: the widest
// live set the ceiling admits is out of a nest's reach in any runnable time.
func BenchmarkMatcherNext(b *testing.B) {
	widest := cmWidestIteration(b)
	for _, iter := range [...]int{widest / 4, widest} {
		b.Run(fmt.Sprintf("live=%d", iter*iter), func(b *testing.B) {
			m := cmSaturated(b, iter)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, ok := m.Next(uq("c")); !ok {
					b.Fatalf("Next(c) rejected item %d, which c's unbounded {max occurs} admits", i+1)
				}
			}
		})
	}
}

// The ceiling is reached by BREADTH as readily as by depth. The two {1,100}
// groups below are SIBLINGS under a sequence that never repeats, and
// partitionsBounded products over both all the same, for the 40804
// particlesZ034_a puts in flight — the widest model maxPartitionStates
// decides, not the widest the suite holds, which products past 10^8 and stays
// declined. ContentMatcher decides it — a ceiling under 40804 does not — and
// decides it in both directions.
func TestContentMatcherCarriesTwoSiblingRepetitionsWhoseCountsMultiply(t *testing.T) {
	m := cmMatcher(t, cmSiblings(t, 100), nil)

	cmAccept(t, m, "a", "a", "b", "c", "c")
	if _, ok := m.Next(uq("d")); ok {
		t.Error("Next(d) took a name no particle of the model admits")
	}
}

// cmSiblings is particlesZ034_a's shape at a given iteration cap: a
// non-repeating sequence over two REPEATING sibling groups, each holding one
// leaf of unbounded occurrence, with a mandatory leaf between them.
// partitionsBounded products over every widened node of the flattened tree, so
// two sibling groups multiply exactly as a nested pair does.
func cmSiblings(t testing.TB, iter int) Particle {
	t.Helper()
	return cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
		cmGroup(t, uOccurs(t, 1, iter), CompositorSequence,
			cmLeaf(t, "a", uUnbounded(t, 1))),
		cmLeaf(t, "b", uOccurs(t, 1, 1)),
		cmGroup(t, uOccurs(t, 1, iter), CompositorSequence,
			cmLeaf(t, "c", uUnbounded(t, 1))))
}

// cmWidestIteration is the largest iteration cap at which ContentMatcher still
// admits cmSiblings' model. It asks ContentMatcher rather than restating
// partitionsBounded's arithmetic, so it follows maxPartitionStates wherever
// that goes.
func cmWidestIteration(t testing.TB) int {
	t.Helper()
	lo, hi := 1, 2
	for cmAdmits(t, hi) {
		lo, hi = hi, 2*hi
	}
	for lo+1 < hi {
		mid := (lo + hi) / 2
		if cmAdmits(t, mid) {
			lo = mid
			continue
		}
		hi = mid
	}
	return lo
}

// cmAdmits reports whether ContentMatcher decides cmSiblings' model at the
// given iteration cap, which cmMatcher cannot answer because a decline is a
// test failure there.
func cmAdmits(t testing.TB, iter int) bool {
	t.Helper()
	s, ct := cmSchema(t, cmSiblings(t, iter), nil)
	_, ok := s.ContentMatcher(ct)
	return ok
}

// cmSaturated drives cmSiblings' model to its widest live set and returns it
// standing there. Each group widens the set by one region per item until its
// counter clamps at its {max occurs}, so the set plateaus at iter × iter and
// every further c holds it there — which is what lets a timed loop measure one
// width rather than a ramp.
func cmSaturated(t testing.TB, iter int) *Matcher {
	t.Helper()
	m := cmMatcher(t, cmSiblings(t, iter), nil)
	feed := append(cmRepeat("a", 2*iter), "b")
	feed = append(feed, cmRepeat("c", 2*iter)...)
	for i, n := range feed {
		if _, ok := m.Next(uq(n)); !ok {
			t.Fatalf("Next(%s) rejected item %d of the saturating feed", n, i+1)
		}
	}
	if len(m.live) != iter*iter {
		t.Fatalf("the saturating feed left %d live regions, want %d", len(m.live), iter*iter)
	}
	return m
}

// The whole live set stands at ONE ·basic particle·, which is what lets the
// walk hold the path once instead of one per region (Matcher.path derives it).
// Nothing in the walk can show that any more — a live set that did hold two
// paths would be answered with one of them and no sign of the other — so this
// sweep asserts it from outside, over the grid of models cmProbeModels
// generates and every feed cmProbeFeeds puts to each.
//
// What it compares is a per-region path the hoisted walk no longer stores: each
// live region is driven over the same name ALONE (cmSingleton), and every
// singleton that takes it has to arrive where the whole set did. A singleton
// that takes a name the set rejected, or a set that takes one no singleton
// does, is the same premise failing from the other side.
func TestMatcherHoldsOnePathForEveryLivePartition(t *testing.T) {
	models, feeds := cmProbeModels(t), cmProbeFeeds()
	for _, pm := range models {
		s, ct := cmProbeSchema(t, pm.name, pm.model())
		for _, feed := range feeds {
			m, ok := s.ContentMatcher(ct)
			if !ok {
				t.Fatalf("%s: ContentMatcher declined a model it decided a moment ago", pm.name)
			}
			for k, n := range feed {
				singles := make([]*Matcher, len(m.live))
				for i := range m.live {
					singles[i] = cmSingleton(m, i)
				}
				_, took := m.Next(uq(n))
				anyone := false
				for i, one := range singles {
					if _, ok := one.Next(uq(n)); !ok {
						continue
					}
					anyone = true
					if !took {
						t.Fatalf("%s: over %v at item %d, live region %d took %s and the live set did not",
							pm.name, feed, k, i, n)
					}
					if !slices.Equal(one.path, m.path) {
						t.Fatalf("%s: over %v at item %d, live region %d stands at %v and the live set at %v",
							pm.name, feed, k, i, one.path, m.path)
					}
				}
				if took && !anyone {
					t.Fatalf("%s: over %v at item %d, the live set took %s and no live region of it did",
						pm.name, feed, k, n)
				}
			}
		}
	}
	// The floor is a fraction of the grid, so that a generator edit collapsing
	// it fails here rather than sweeping nothing quietly.
	if len(models) < 300 {
		t.Fatalf("the sweep ran over %d models, want at least 300", len(models))
	}
	t.Logf("swept %d models over %d feeds each", len(models), len(feeds))
}

// cmSingleton is m with ONE of its live regions and nothing else: the same
// flattened model, the same path, that region's counters alone. Driving it over
// the name m is about to take shows where that partition on its own would
// stand. It shares m.nodes, which no walk step writes, and shares no scratch
// with m (Matcher.probe).
func cmSingleton(m *Matcher, i int) *Matcher {
	one := *m
	one.live = []region{m.live[i].clone()}
	one.path = slices.Clone(m.path)
	one.probe, one.reached = nil, nil
	return &one
}

// cmProbeSchema is cmSchema over one model of the grid, plus the assertion that
// the walk is licensed over it: every combination the grid generates is
// cos-nonambig-clean and inside maxPartitionStates today, so a model that stops
// being either fails here rather than dropping out of the sweep unnoticed.
func cmProbeSchema(t testing.TB, name string, p Particle) (*Schema, ComplexType) {
	t.Helper()
	s, ct := cmSchema(t, p, nil)
	if _, ok := s.ContentMatcher(ct); !ok {
		t.Fatalf("%s: ContentMatcher declined a model of the sweep's grid", name)
	}
	return s, ct
}

// cmProbeModel is one model of the sweep above: a name a failure can cite, and
// a builder, because a Particle is built against the testing.TB that holds the
// fixture's own failures.
type cmProbeModel struct {
	name  string
	model func() Particle
}

// cmRange is one occurrence range of the sweep's grid, carrying the spelling a
// model name uses for it.
type cmRange struct {
	name string
	o    func() Occurs
}

// cmLeafRanges are the occurrence ranges the sweep puts on a leaf: ranges that
// differ in which of a counter's band edges (0, 1, {min occurs}, counterCap)
// coincide, plus an unbounded one, whose counterCap is its {min occurs}.
func cmLeafRanges(t testing.TB) []cmRange {
	t.Helper()
	return []cmRange{
		{"0,1", func() Occurs { return uOccurs(t, 0, 1) }},
		{"1,1", func() Occurs { return uOccurs(t, 1, 1) }},
		{"1,2", func() Occurs { return uOccurs(t, 1, 2) }},
		{"2,3", func() Occurs { return uOccurs(t, 2, 3) }},
		{"1,unbounded", func() Occurs { return uUnbounded(t, 1) }},
	}
}

// cmGroupRanges are the ranges the sweep puts on a model group, which is where
// an ·ambiguous· node's iteration boundary splits the live set: every one of
// them but 1,1 repeats.
func cmGroupRanges(t testing.TB) []cmRange {
	t.Helper()
	return []cmRange{
		{"1,1", func() Occurs { return uOccurs(t, 1, 1) }},
		{"1,2", func() Occurs { return uOccurs(t, 1, 2) }},
		{"2,2", func() Occurs { return uOccurs(t, 2, 2) }},
		{"2,3", func() Occurs { return uOccurs(t, 2, 3) }},
		{"1,unbounded", func() Occurs { return uUnbounded(t, 1) }},
	}
}

// cmProbeModels is the grid of content models the sweep runs over: a repeating
// group over two leaves under every combination of compositor and occurrence
// range, a repeating group nested in one, an all group holding another, and the
// nested, sibling and pinned models the tests above name one at a time. Every
// combination is a model the walk is licensed over (cmProbeSchema), so the grid
// stays a plain product and carries no exclusions.
func cmProbeModels(t testing.TB) []cmProbeModel {
	t.Helper()
	var out []cmProbeModel
	leaves, groups := cmLeafRanges(t), cmGroupRanges(t)
	for _, comp := range []struct {
		name string
		c    Compositor
	}{{"sequence", CompositorSequence}, {"choice", CompositorChoice}} {
		for _, g := range groups {
			for _, x := range leaves {
				for _, y := range leaves {
					out = append(out, cmProbeModel{
						name: fmt.Sprintf("%s(a{%s},b{%s}){%s}", comp.name, x.name, y.name, g.name),
						model: func() Particle {
							return cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
								cmGroup(t, g.o(), comp.c,
									cmLeaf(t, "a", x.o()),
									cmLeaf(t, "b", y.o())))
						},
					})
				}
			}
		}
	}
	for _, g := range groups {
		for _, h := range groups {
			for _, x := range leaves {
				out = append(out, cmProbeModel{
					name: fmt.Sprintf("(a{%s},(b{1,2},c{0,1}){%s}){%s}", x.name, h.name, g.name),
					model: func() Particle {
						return cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
							cmGroup(t, g.o(), CompositorSequence,
								cmLeaf(t, "a", x.o()),
								cmGroup(t, h.o(), CompositorSequence,
									cmLeaf(t, "b", uOccurs(t, 1, 2)),
									cmLeaf(t, "c", uOccurs(t, 0, 1)))))
					},
				})
			}
		}
	}
	for _, x := range leaves {
		for _, y := range leaves {
			out = append(out, cmProbeModel{
				name: fmt.Sprintf("all(a{%s},b{%s},all(c,d?))", x.name, y.name),
				model: func() Particle {
					return cmGroup(t, uOccurs(t, 1, 1), CompositorAll,
						cmLeaf(t, "a", x.o()),
						cmLeaf(t, "b", y.o()),
						cmGroup(t, uOccurs(t, 1, 1), CompositorAll,
							cmLeaf(t, "c", uOccurs(t, 1, 1)),
							cmLeaf(t, "d", uOccurs(t, 0, 1))))
				},
			})
		}
	}
	out = append(out,
		cmProbeModel{"(a{1,2},b?){2,2}", func() Particle { return cmNested(t) }},
		cmProbeModel{"nested all", func() Particle { return cmNestedAll(t) }},
		cmProbeModel{"pinned wide body", func() Particle { return cmWideBody(t) }},
	)
	for _, iter := range []int{2, 3, 8} {
		out = append(out, cmProbeModel{
			name:  fmt.Sprintf("siblings{1,%d}", iter),
			model: func() Particle { return cmSiblings(t, iter) },
		})
	}
	return out
}

// cmProbeFeeds are the child sequences the sweep puts to every model: every
// word of length at most three over the four names the grid's models use, so
// that a model is driven into an open iteration, out of it and into the next,
// plus longer runs that hold the walk at a WIDE live set rather than at a
// narrow prefix of one.
func cmProbeFeeds() [][]string {
	names := []string{"a", "b", "c", "d"}
	feeds := [][]string{{}}
	for n := 0; n < 3; n++ {
		var next [][]string
		for _, f := range feeds {
			for _, name := range names {
				next = append(next, append(append([]string{}, f...), name))
			}
		}
		feeds = append(feeds, next...)
	}
	return append(feeds[1:],
		cmRepeat("a", 8),
		[]string{"a", "b", "a", "b", "a", "b", "a", "b"},
		[]string{"a", "a", "b", "a", "a", "b", "a", "a", "b"},
		[]string{"a", "b", "c", "a", "b", "c", "a", "b", "c"},
		[]string{"a", "a", "a", "a", "b", "a", "a", "a", "a"},
		[]string{"d", "c", "b", "a", "d", "c", "b", "a"},
	)
}

// cmAdmissible renders m.Admissible() as one token per arm: an element
// declaration by its local name, a ·wildcard particle· as "wildcard", the {open
// content} as "open". It fails on a declaration outside the fixtures' namespace,
// which a token would otherwise hide.
func cmAdmissible(t *testing.T, m *Matcher) []string {
	t.Helper()
	var out []string
	for _, a := range m.Admissible() {
		switch v := a.(type) {
		case ElementDeclaration:
			if v.Name().Space != uns {
				t.Fatalf("Admissible listed %s, outside the fixtures' namespace", v.Name())
			}
			out = append(out, v.Name().Local)
		case Wildcard:
			out = append(out, "wildcard")
		case *OpenContent:
			out = append(out, "open")
		default:
			t.Fatalf("Admissible listed %T, which Attribution does not seal", a)
		}
	}
	return out
}

// cmWantAdmissible asserts what Admissible lists after feed, which must be
// taken whole.
func cmWantAdmissible(t *testing.T, m *Matcher, feed []string, want ...string) {
	t.Helper()
	if i := cmFeed(t, m, feed...); i != len(feed) {
		t.Fatalf("Next rejected %s at position %d of %v, want the whole feed taken", feed[i], i, feed)
	}
	if got := cmAdmissible(t, m); !slices.Equal(got, want) {
		t.Errorf("Admissible() after %v = %v, want %v", feed, got, want)
	}
}

// cmAdmissibleCase is one row of the tables below: a feed and what Admissible
// lists after it.
type cmAdmissibleCase struct {
	feed []string
	want []string
}

// A sequence offers its next member and every ·emptiable· member after it up
// to the first one that is not (§3.8.4.1.1), in the model's order, and the
// sequence's end offers nothing: the empty result.
func TestMatcherAdmissibleFollowsASequence(t *testing.T) {
	model := func() Particle {
		return cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
			cmLeaf(t, "a", uOccurs(t, 1, 1)),
			cmLeaf(t, "b", uOccurs(t, 0, 1)),
			cmLeaf(t, "c", uOccurs(t, 1, 2)),
			cmLeaf(t, "d", uOccurs(t, 1, 1)))
	}
	for _, tc := range []cmAdmissibleCase{
		{nil, []string{"a"}},
		{[]string{"a"}, []string{"b", "c"}},
		{[]string{"a", "b"}, []string{"c"}},
		{[]string{"a", "c"}, []string{"c", "d"}},
		{[]string{"a", "c", "c"}, []string{"d"}},
		{[]string{"a", "c", "d"}, nil},
	} {
		cmWantAdmissible(t, cmMatcher(t, model(), nil), tc.feed, tc.want...)
	}

	// The walk offers b, later in the open iteration, before it offers a, the
	// first item of a fresh one, and the list is in the model's order anyway.
	repeating := cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
		cmGroup(t, uOccurs(t, 1, 2), CompositorSequence,
			cmLeaf(t, "a", uOccurs(t, 1, 1)),
			cmLeaf(t, "b", uOccurs(t, 0, 1))))
	cmWantAdmissible(t, cmMatcher(t, repeating, nil), []string{"a"}, "a", "b")
}

// A choice offers every member at once (§3.8.4.1.2), and once one is taken
// the iteration is over, so what follows the choice is offered and no other
// member is.
func TestMatcherAdmissibleOffersEveryMemberOfAChoice(t *testing.T) {
	model := func() Particle {
		return cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
			cmGroup(t, uOccurs(t, 1, 1), CompositorChoice,
				cmLeaf(t, "a", uOccurs(t, 1, 1)),
				cmLeaf(t, "b", uOccurs(t, 1, 1)),
				cmLeaf(t, "c", uOccurs(t, 1, 1))),
			cmLeaf(t, "d", uOccurs(t, 1, 1)))
	}
	for _, tc := range []cmAdmissibleCase{
		{nil, []string{"a", "b", "c"}},
		{[]string{"b"}, []string{"d"}},
	} {
		cmWantAdmissible(t, cmMatcher(t, model(), nil), tc.feed, tc.want...)
	}
}

// An all group offers every member short of its {max occurs}, a nested all
// group's members included, whatever member the last item went to
// (§3.8.4.1.3): a spent member drops out and the rest stay, in the model's
// order — a, c, d, b for all(a, all(c, d), b), not the order they were taken.
func TestMatcherAdmissibleOffersTheUnspentMembersOfAnAllGroup(t *testing.T) {
	for _, tc := range []cmAdmissibleCase{
		{nil, []string{"a", "c", "d", "b"}},
		{[]string{"b"}, []string{"a", "c", "d"}},
		{[]string{"a", "c"}, []string{"d", "b"}},
		{[]string{"d", "b", "c"}, []string{"a"}},
		{[]string{"d", "b", "c", "a"}, nil},
	} {
		cmWantAdmissible(t, cmMatcher(t, cmNestedAll(t), nil), tc.feed, tc.want...)
	}
}

// One particle is listed ONCE however many live partitions reach it. After "a
// a", (a{1,2}, b?){2,2} holds two partitions — (a a) still in the first
// iteration and (a)(a) in the second — and both offer b, while a is the second
// iteration's next a for one and its first for the other.
func TestMatcherAdmissibleListsAParticleOnceAcrossLivePartitions(t *testing.T) {
	m := cmMatcher(t, cmNested(t), nil)
	cmWantAdmissible(t, m, []string{"a", "a"}, "a", "b")
	if len(m.live) < 2 {
		t.Fatalf("the feed left %d live regions, want the two partitions the test is about", len(m.live))
	}
}

// Under {mode} interleave the {open content} is offered at every position,
// after the particles, and is the whole offer once the {particle} is done
// (cvc-complex-content clause 3).
func TestMatcherAdmissibleOffersInterleavedOpenContentLast(t *testing.T) {
	model := func() *Matcher {
		return cmOpenMatcher(t, OpenContentInterleave, cmOpenWildcard(t),
			cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
				cmLeaf(t, "a", uOccurs(t, 1, 1)),
				cmLeaf(t, "b", uOccurs(t, 1, 1))))
	}
	for _, tc := range []cmAdmissibleCase{
		{nil, []string{"a", "open"}},
		{[]string{"a", "x"}, []string{"b", "open"}},
		{[]string{"a", "x", "b"}, []string{"open"}},
	} {
		cmWantAdmissible(t, model(), tc.feed, tc.want...)
	}
}

// Under {mode} suffix the {open content} is offered beside the particles until
// an item starts S2, and alone after that, though the {particle} is still owed
// b: S = S1 + S2 is a concatenation (cvc-complex-content clause 2.1).
func TestMatcherAdmissibleOffersSuffixOpenContentAloneInsideS2(t *testing.T) {
	model := func() *Matcher {
		return cmOpenMatcher(t, OpenContentSuffix, cmOpenWildcard(t),
			cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
				cmLeaf(t, "a", uOccurs(t, 1, 1)),
				cmLeaf(t, "b", uOccurs(t, 1, 1))))
	}
	for _, tc := range []cmAdmissibleCase{
		{nil, []string{"a", "open"}},
		{[]string{"a"}, []string{"b", "open"}},
		{[]string{"a", "x"}, []string{"open"}},
	} {
		cmWantAdmissible(t, model(), tc.feed, tc.want...)
	}
}

// A ·wildcard particle· is listed in its place in the model like an element
// particle, and an element particle and a wildcard that admit one name — the
// competition 1.1 permits — are both listed.
func TestMatcherAdmissibleListsAWildcardParticleInItsPlace(t *testing.T) {
	m := cmMatcher(t, cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
		cmLeaf(t, "a", uOccurs(t, 0, 1)),
		uParticle(t, uOccurs(t, 0, 1), ResolvedTerm{Term: uWildcard(t, NamespaceConstraintAny, nil, ProcessLax)}),
		cmLeaf(t, "b", uOccurs(t, 1, 1))), nil)
	cmWantAdmissible(t, m, nil, "a", "wildcard", "b")
}

// cmClone is m with every live region copied, so Next can be asked of the copy
// without moving m. It shares m.nodes, which no walk step writes, and no
// scratch (Matcher.probe).
func cmClone(m *Matcher) *Matcher {
	c := *m
	c.live = make([]region, len(m.live))
	for i := range m.live {
		c.live[i] = m.live[i].clone()
	}
	c.path = slices.Clone(m.path)
	c.probe, c.reached = nil, nil
	return &c
}

// cmArmAdmits reports whether one Admissible arm admits name, on the terms
// Next decides it: an element declaration by name or ·substitution group·
// membership (cvc-accept clause 2.3), a wildcard or the {open content}'s by
// cvc-wildcard.
func cmArmAdmits(m *Matcher, a Attribution, name QName) bool {
	switch v := a.(type) {
	case ElementDeclaration:
		return v.Name() == name || m.s.nameInSubstitutionGroupOf(name, v)
	case Wildcard:
		return m.s.allowsElementWildcardName(v, m.ct, name)
	case *OpenContent:
		return m.s.allowsElementWildcardName(v.Wildcard(), m.ct, name)
	default:
		panic("cmArmAdmits: non-exhaustive Attribution switch")
	}
}

// Admissible is exactly the set Next decides by, at every position of every
// model in cmProbeModels' grid over every cmProbeFeeds feed, the end of the
// feed included: Next takes a name iff some listed arm admits it, for each of
// the grid's names and one no model uses. It is read-only as well: the call
// leaves the walk state as it found it, and a twin matcher fed the same items
// and never asked answers every Next and Accepting identically.
func TestMatcherAdmissibleIsTheSetNextDecidesBy(t *testing.T) {
	models, feeds := cmProbeModels(t), cmProbeFeeds()
	for _, pm := range models {
		s, ct := cmProbeSchema(t, pm.name, pm.model())
		for _, feed := range feeds {
			m, _ := s.ContentMatcher(ct)
			twin, _ := s.ContentMatcher(ct)
			for k, n := range feed {
				cmCheckAdmissible(t, m, fmt.Sprintf("%s: over %v at item %d", pm.name, feed, k))
				_, took := m.Next(uq(n))
				_, twinTook := twin.Next(uq(n))
				if took != twinTook || m.Accepting() != twin.Accepting() {
					t.Fatalf("%s: over %v at item %d, the matcher asked Admissible answers differently from its twin",
						pm.name, feed, k)
				}
			}
			cmCheckAdmissible(t, m, fmt.Sprintf("%s: at the end of %v", pm.name, feed))
		}
	}
}

// cmCheckAdmissible asserts, where m stands, that m.Admissible() leaves the
// walk state untouched and that Next takes each of a, b, c, d and e exactly
// where some arm it lists admits that name.
func cmCheckAdmissible(t *testing.T, m *Matcher, where string) {
	t.Helper()
	before := cmClone(m)
	arms := m.Admissible()
	sameRegion := func(x, y region) bool { return slices.Equal(x.counts, y.counts) }
	if !slices.EqualFunc(m.live, before.live, sameRegion) || !slices.Equal(m.path, before.path) || m.inS2 != before.inS2 {
		t.Fatalf("%s, Admissible changed the walk state", where)
	}
	for _, probe := range []string{"a", "b", "c", "d", "e"} {
		_, took := cmClone(m).Next(uq(probe))
		listed := slices.ContainsFunc(arms, func(a Attribution) bool { return cmArmAdmits(m, a, uq(probe)) })
		if took != listed {
			t.Fatalf("%s, Next(%s) = %t but the arms Admissible lists, %v, admit it: %t",
				where, probe, took, cmAdmissible(t, m), listed)
		}
	}
}
