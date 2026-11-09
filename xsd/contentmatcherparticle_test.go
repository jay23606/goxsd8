package xsd

import (
	"reflect"
	"slices"
	"testing"
)

// These tests pin Matcher.NextParticle's position contract over
// contentmatcher_test.go's fixtures, every schema FINALIZED on cmSchema's
// terms.

// cmPosition takes one name through NextParticle, asserts it was taken, and
// returns the position.
func cmPosition(t *testing.T, m *Matcher, name QName) (Attribution, []int) {
	t.Helper()
	a, pos, ok := m.NextParticle(name)
	if !ok {
		t.Fatalf("NextParticle(%s) rejected a name the model admits", name)
	}
	return a, pos
}

// cmAnyParticle is a once-occurring ##any skip wildcard particle, mgF019's.
func cmAnyParticle(t *testing.T) Particle {
	t.Helper()
	return uParticle(t, uOccurs(t, 1, 1), ResolvedTerm{Term: uWildcard(t, NamespaceConstraintAny, nil, ProcessSkip)})
}

// Two sibling wildcard particles of equal {term}s, mgF019's shape, are told
// apart by position alone: Next answers each item with Wildcard values that
// compare equal, and NextParticle puts them at [0] and [1].
func TestNextParticleTellsApartEqualSiblingWildcards(t *testing.T) {
	m := cmMatcher(t, cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
		cmAnyParticle(t), cmAnyParticle(t), cmAnyParticle(t)), nil)

	var answers []Attribution
	for i, local := range []string{"x", "y", "z"} {
		a, pos := cmPosition(t, m, QName{Space: "urn:other", Local: local})
		cmWantWildcard(t, a, ProcessSkip)
		if !slices.Equal(pos, []int{i}) {
			t.Errorf("item %d attributed at %v, want [%d]", i, pos, i)
		}
		answers = append(answers, a)
	}
	if !reflect.DeepEqual(answers[0], answers[1]) {
		t.Error("the two wildcards' answers differ by value, so this test no longer shows the position is what separates them")
	}
}

// A particle inside a nested group is at one index per group level, outermost
// first: sequence(a, choice(b, c)) puts b at [1 0] and c at [1 1], and a,
// directly in the content type's group, at [0].
func TestNextParticleReportsANestedPosition(t *testing.T) {
	model := func() Particle {
		return cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
			cmLeaf(t, "a", uOccurs(t, 1, 1)),
			cmGroup(t, uOccurs(t, 1, 1), CompositorChoice,
				cmLeaf(t, "b", uOccurs(t, 1, 1)),
				cmLeaf(t, "c", uOccurs(t, 1, 1))))
	}
	for _, tc := range []struct {
		second string
		want   []int
	}{{"b", []int{1, 0}}, {"c", []int{1, 1}}} {
		m := cmMatcher(t, model(), nil)
		if _, pos := cmPosition(t, m, uq("a")); !slices.Equal(pos, []int{0}) {
			t.Errorf("a attributed at %v, want [0]", pos)
		}
		a, pos := cmPosition(t, m, uq(tc.second))
		cmWantDeclaration(t, a, tc.second)
		if !slices.Equal(pos, tc.want) {
			t.Errorf("%s attributed at %v, want %v", tc.second, pos, tc.want)
		}
	}
}

// An item the {open content} takes is attributed to no particle, so its
// position is empty and not nil; the particle's own items keep theirs.
func TestNextParticleReportsNoPositionForOpenContent(t *testing.T) {
	m := cmOpenMatcher(t, OpenContentInterleave, cmOpenWildcard(t),
		cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
			cmLeaf(t, "a", uOccurs(t, 1, 1)),
			cmLeaf(t, "b", uOccurs(t, 1, 1))))

	if _, pos := cmPosition(t, m, uq("a")); !slices.Equal(pos, []int{0}) {
		t.Errorf("a attributed at %v, want [0]", pos)
	}
	a, pos := cmPosition(t, m, uq("x"))
	cmWantOpenContent(t, a, OpenContentInterleave)
	if pos == nil || len(pos) != 0 {
		t.Errorf("the open content's item attributed at %#v, want an empty non-nil position", pos)
	}
	if _, pos := cmPosition(t, m, uq("b")); !slices.Equal(pos, []int{1}) {
		t.Errorf("b attributed at %v, want [1]", pos)
	}
}

// Where the {content type}'s {particle} is itself the element particle, no
// group holds it and the position is empty.
func TestNextParticleReportsNoPositionForALeafContentParticle(t *testing.T) {
	m := cmOpenMatcher(t, OpenContentInterleave, cmOpenWildcard(t), cmLeaf(t, "a", uOccurs(t, 1, 1)))

	a, pos := cmPosition(t, m, uq("a"))
	cmWantDeclaration(t, a, "a")
	if pos == nil || len(pos) != 0 {
		t.Errorf("a attributed at %#v, want an empty non-nil position", pos)
	}
}

// A rejected name reports a nil position and changes nothing: the next name
// is answered as if it had never been offered. The positions already returned
// are the caller's, unchanged by later calls.
func TestNextParticleRejectsWithANilPosition(t *testing.T) {
	m := cmMatcher(t, cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
		cmLeaf(t, "a", uOccurs(t, 1, 1)),
		cmLeaf(t, "b", uOccurs(t, 1, 1))), nil)

	_, first := cmPosition(t, m, uq("a"))
	a, pos, ok := m.NextParticle(uq("zzz"))
	if ok || a != nil || pos != nil {
		t.Fatalf("NextParticle(zzz) = %v, %#v, %t, want nil, nil, false", a, pos, ok)
	}
	a, pos = cmPosition(t, m, uq("b"))
	cmWantDeclaration(t, a, "b")
	if !slices.Equal(pos, []int{1}) {
		t.Errorf("b attributed at %v, want [1]", pos)
	}
	if !slices.Equal(first, []int{0}) {
		t.Errorf("a's position became %v after later calls, want [0]", first)
	}
}

// Next and NextParticle answer alike: two matchers over one model, fed the
// same names, one through each, agree on the answer and on whether the name
// was taken, at every item, over the inputs contentmatcher_test.go drives.
func TestNextAgreesWithNextParticle(t *testing.T) {
	type input struct {
		name  string
		build func() *Matcher
		items []QName
	}
	other := func(local string) QName { return QName{Space: "urn:other", Local: local} }
	seq := func() Particle {
		return cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
			cmLeaf(t, "a", uOccurs(t, 1, 1)),
			cmLeaf(t, "b", uOccurs(t, 1, 1)))
	}
	inputs := []input{
		{"sequence", func() *Matcher { return cmMatcher(t, seq(), nil) }, []QName{uq("a"), uq("zzz"), uq("b"), uq("b")}},
		{"nested partitions", func() *Matcher { return cmMatcher(t, cmNested(t), nil) },
			[]QName{uq("a"), uq("a"), uq("b"), uq("a"), uq("b"), uq("b")}},
		{"repeated choice", func() *Matcher {
			return cmMatcher(t, cmGroup(t, uOccurs(t, 1, 3), CompositorChoice,
				cmLeaf(t, "a", uOccurs(t, 1, 1)),
				cmLeaf(t, "b", uOccurs(t, 1, 1))), nil)
		}, []QName{uq("b"), uq("a"), uq("b"), uq("a")}},
		{"all group", func() *Matcher {
			return cmMatcher(t, cmGroup(t, uOccurs(t, 1, 1), CompositorAll,
				cmLeaf(t, "a", uOccurs(t, 1, 1)),
				cmLeaf(t, "b", uOccurs(t, 0, 1))), nil)
		}, []QName{uq("b"), uq("a"), uq("a")}},
		{"element before wildcard", func() *Matcher {
			return cmMatcher(t, cmGroup(t, uOccurs(t, 1, 1), CompositorChoice,
				cmLeaf(t, "a", uOccurs(t, 1, 1)),
				uParticle(t, uUnbounded(t, 1), ResolvedTerm{
					Term: uWildcard(t, NamespaceConstraintAny, nil, ProcessLax)})), nil)
		}, []QName{other("w"), uq("a"), other("v")}},
		{"equal wildcards", func() *Matcher {
			return cmMatcher(t, cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
				cmAnyParticle(t), cmAnyParticle(t)), nil)
		}, []QName{other("x"), other("y"), other("z")}},
		{"substitution group", func() *Matcher {
			return cmMatcher(t, cmGroup(t, uOccurs(t, 1, 1), CompositorSequence,
				uParticle(t, uOccurs(t, 1, 1), ElementDeclarationRef{Name: uq("head")})), func(b *SchemaBuilder) {
				b.AddElement(uGlobal(t, uq("head"), uq("T")))
				b.AddElement(uGlobal(t, uq("member"), uq("T"), uq("head")))
			})
		}, []QName{uq("member"), uq("head")}},
		{"suffix open content", func() *Matcher { return cmOpenMatcher(t, OpenContentSuffix, cmOpenWildcard(t), seq()) },
			[]QName{uq("a"), uq("x"), uq("b")}},
		{"interleave open content", func() *Matcher { return cmOpenMatcher(t, OpenContentInterleave, cmOpenWildcard(t), seq()) },
			[]QName{uq("a"), uq("x"), uq("b"), uq("y")}},
	}
	for _, in := range inputs {
		viaNext, viaParticle := in.build(), in.build()
		for i, name := range in.items {
			a, ok := viaNext.Next(name)
			b, _, okParticle := viaParticle.NextParticle(name)
			if ok != okParticle || !reflect.DeepEqual(a, b) {
				t.Errorf("%s, item %d (%s): Next = %v, %t; NextParticle = %v, %t", in.name, i, name, a, ok, b, okParticle)
			}
		}
		if viaNext.Accepting() != viaParticle.Accepting() {
			t.Errorf("%s: Accepting differs after the two matchers took the same items", in.name)
		}
	}
}
