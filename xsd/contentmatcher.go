package xsd

import (
	"cmp"
	"slices"
)

// This file is the M5 pull driver xsd/doc.go names: Matcher, the
// instance-guided advance of one complex type's {content type} particle, one
// child at a time. It decides Element Sequence Locally Valid (Particle)
// (§3.9.4.2, cvc-particle) — a one-clause wrapper over Element Sequence
// Accepted (Particle) (§3.9.4.3, cvc-accept) — and, through cvc-accept clause
// 3, Element Sequence Valid (§3.8.4.3, cvc-model-group) over §3.8.4.1's
// per-compositor recognition rules. The caller charges the enclosing
// cvc-complex-content (§3.4.4.3) or cvc-complex-type (§3.4.4.2) clause; nothing
// here builds an error, because "no particle admits this name" is a local
// non-match at every position but the last, and only the caller knows which
// position it is looking at.
//
// The walk COUNTS: state is one occurrence counter per particle of the
// flattened content model plus the ONE path of particles the last item was
// ·attributed to· (Matcher.path). No occurrence range is unfolded into copies, so
// maxOccurs="100000" costs one counter and not a hundred thousand positions —
// the opposite trade from particleattribution.go's automaton, which unfolds
// because ·compete· is a question about the MODEL and this one is a question
// about an INSTANCE. The two constructions are deliberately not shared:
// automaton.addAll accepts a documented SUPERSET of the interleave language
// (particleattribution.go), harmless only for ·compete·, which reads no
// language, and a false accept for every consumer that reads one, an instance
// walk included; and its unfolding bound changes the accepted language outright
// (e{3,6} reads as e{2,4}).
//
// # One particle per item, and more than one partition
//
// The walk never re-reads an item and never undoes one it has taken
// (PRINCIPLES 14). Two facts fix WHICH PARTICLE each item goes to, and neither
// is an assumption a comment makes:
//
//   - cos-nonambig (§3.8.6.4) has run. A Matcher is reachable only through a
//     *Schema, which only SchemaBuilder.Finalize produces, and Phase C rejects
//     a content model with two ·competing· particles. So at most one ELEMENT
//     particle live in one state admits a given ·expanded name·, and the
//     particle an item is ·attributed to· is not a choice this file makes.
//   - Where an element particle and a wildcard particle both admit one name —
//     the one competition 1.1 permits (Appendix G.1.3) — cvc-accept's closing
//     Note fixes the answer: "the validation process defined in this
//     specification matches the element information item against the Element
//     Declaration, both in identifying the Element Declaration as the item's
//     ·context-determined declaration·, and in choosing alternative paths
//     through a content model". Next therefore searches the live particles for
//     an element admission first and for a wildcard admission only if that
//     search fails, which is PRINCIPLES 14's "explicit content beats a
//     wildcard" stated as a search order.
//
// Neither settles WHICH ITERATION of a repeated ancestor an item falls in, the
// third non-determinism the same Note names: "nested particles each of which
// has {max occurs} greater than 1, where the input sequence can be partitioned
// in multiple ways ... there is no fixed rule for eliminating the
// non-determinism". cvc-accept clause 3.1 asks that question EXISTENTIALLY —
// "there is a partition of the sequence into n sub-sequences" — so a sequence
// is ·accepted· where SOME partition validates, and picking one partition and
// walking it answers a different question than the rule asks. A greedy pick —
// stay in the innermost open iteration for as long as it can — gets "a a b"
// wrong against (a{1,2}, b?){2,2}, which L(P) admits as the partition (a)(a b)
// and the greedy walk consumes as one iteration and then rejects.
//
// So the walk carries a SET of the partitions of the items so far that are
// still live, and a name is taken where any of them takes it. That is not a
// search: no partition re-reads an item, none is revisited, and the set is
// widened only where the greedy order is not already exact —
//
//   - a differing partition needs one particle P reachable both later in the
//     open iteration of some repeatable ancestor R and at the start of R's next
//     iteration (two DIFFERENT particles would ·compete·, which cos-nonambig has
//     already rejected);
//   - P starting R's body means every particle before it on its path is
//     skippable, so the shorter iteration the alternative closes before P holds
//     no mandatory particle of the body — and is a word of the body's language
//     only where the body has none at all, which is to say where it is
//     ·emptiable·, where the greedy pick is exact: its iteration count is never
//     ABOVE the alternative's, so no {max occurs} it satisfies is one the greedy
//     pick exceeds, and canExit lets the iterations still owed be empty, so
//     {min occurs} is met as well. The one way that iteration can hold a
//     mandatory particle is P ITSELF, taken again, which needs a repeating
//     particle inside a repeating one;
//   - so the set splits at a node R that repeats and holds such a P
//     (contentNode.ambiguous) and nowhere else. P has to stand at BOTH ends of
//     R's iteration boundary at once — reachable as the first item of a fresh
//     iteration, which needs every particle before it skippable, and reachable
//     with the open iteration already a whole word of the body, which needs
//     every particle after it skippable — so a body holding a mandatory
//     particle on either side of each of its repeating particles never splits,
//     however wide R's own occurrence range. A model with no such node carries
//     one partition for the whole sequence and costs exactly what the greedy
//     walk cost.
//
// # What the walk carries the set AS
//
// The live partitions differ in their occurrence counters and in nothing else
// (Matcher.path), and a counter stops at its node's {max occurs}, or at
// its {min occurs} where {max occurs} is unbounded (counterCap), because
// canRepeat, canExit, memberSatisfied and offerAllMembers are its only readers
// and none can tell a larger value from the clamp. So the set is a set of
// counter TUPLES, bounded by the SCHEMA and never by the instance.
//
// One entry per tuple is not enough. (a{1,500}){1,500} puts a quarter of a
// million partitions in flight at once — every (iterations so far, items in the
// open iteration) pair a prefix admits — and that is not an artefact of the
// bound: each pair really is reachable. The walk therefore carries a run of
// adjacent counts as one span and a tuple of spans as one region, which covers
// that width in about a thousand entries and costs one walk step each. region
// carries why one step decides a whole run at once, and partitionsBounded what
// a model's cover costs; ContentMatcher declines the models whose cover would
// outgrow maxPartitionStates, which is what makes one item's cost a constant of
// the schema rather than a function of the items already taken.
//
// # The {open content} split, and why it needs no search either
//
// Where the {content type}'s {open content} is PRESENT, cvc-complex-content
// (§3.4.4.3) clauses 2 and 3 decide the sequence S as two subsequences: an S1
// ·valid· with respect to {particle} (cvc-particle §3.9.4.2, clauses 2.2 and
// 3.2) and an S2 every member of which is ·valid· with respect to
// {open content}.{wildcard} (cvc-wildcard §3.10.4.1, clauses 2.4 and 3.4).
// Neither clause leaves the split open, so neither needs a search:
//
//   - clause 2.3 ({mode} suffix) admits an S2 only where S1 + E has no ·path·
//     in {particle} for S2's first element E (§3.8.4.1, key-path). Having a
//     path is prefix-closed, so exactly one S1 satisfies that: the longest
//     prefix of S that has one. A shorter prefix leaves 2.3 false for its own
//     first S2 item, and a longer one has no path.
//   - clause 3.3 ({mode} interleave) says the same of EVERY member E of S2,
//     against S3, the part of S1 preceding E. Each item's side is therefore
//     fixed by the items before it: {particle} takes the item wherever it can
//     extend, and the wildcard takes it only where {particle} provably cannot.
//
// That is PRINCIPLES 14's "explicit content beats an open-content wildcard at
// the current state" arriving as the clauses' own condition rather than as a
// tie-break this file invents. Next asks the two particle searches first and
// the open wildcard last, and an item the open wildcard takes leaves the walk
// state untouched, S2 being matched against that wildcard and nothing else.
// Clause 2.1's S = S1 + S2 is a concatenation, so suffix mode never offers
// {particle} another item once it has left it; clause 3.1's S1 × S2 is the
// interleave operator (§3.8.4.1.3), so interleave mode offers every item.

// Attribution is what one element information item of a matched child sequence
// is ·attributed to· (§3.4.4.4, key-att-to): the {term} of the particle
// [Matcher.Next] advanced over, or a present {open content} itself. It is a
// sealed sum (STYLE T2's closed-sum exception) with exactly three variants.
// [ElementDeclaration] (cvc-accept clause 2) and [Wildcard] (cvc-accept clause
// 1) are the two kinds of ·basic particle· an item can be attributed to — a
// Model Group is not one, and returning [Term] would make an attribution to one
// representable (STYLE T1). [*OpenContent] is the third, for an item
// cvc-complex-content clause 2.4 or 3.4 admitted, which §3.4.4.4 ·attributes
// to· the {open content} and not to any particle: the spec keeps the two apart
// throughout — §3.4.5.2 gives this case the [match information] keyword open
// where a ·wildcard particle· takes strict/lax/skip, and §3.4.6.4's
// key-dft-binding clauses 4-6 name them as alternative cases — so a consumer
// deciding a rule that quantifies over particles alone can decide it here.
//
// An [ElementDeclaration] result is the particle's own declaration D, which is
// the answer to "which particle consumed this item". For an item admitted
// through cvc-accept clause 2.3.2 the ·context-determined declaration· is the
// ·substituting declaration· S and not D; resolving S is the recursive
// assessment's job (§3.3.4.6), not this one's.
//
// A [Wildcard] result is the {term} of a ·wildcard particle· (§3.9.1, key-wp)
// and never the {wildcard} of an {open content}. That is the distinction
// e-validity clause 1.1.3 (§3.3.5.1) turns on: it quantifies over an item
// ·attributed to· a ***strict*** ·wildcard particle· and names no other
// component, so this arm decides it and the [*OpenContent] one does not.
//
// An [*OpenContent] result is the {open content} record of the {content type}
// being matched — never nil, and the same pointer [ElementContent] carries, so
// a consumer may compare it by identity as well as read {mode} and {wildcard}
// off it. A switch arm may therefore dereference it without a nil check:
// [Matcher.Next] reports no match at all where the {open content} is ·absent·.
type Attribution interface{ attribution() }

// attribution marks ElementDeclaration as an Attribution (cvc-accept clause
// 2.3); see the Attribution doc comment.
func (ElementDeclaration) attribution() {}

// attribution marks Wildcard as an Attribution (cvc-accept clause 1); see the
// Attribution doc comment.
func (Wildcard) attribution() {}

// attribution marks *OpenContent as an Attribution (§3.4.4.4: an item
// cvc-complex-content clause 2.4 or 3.4 admitted is ·attributed to· the {open
// content}, which is no particle); see the Attribution doc comment. The
// receiver is a POINTER so the arm has exactly one spelling — a value receiver
// would put the marker in both OpenContent's and *OpenContent's method sets,
// making two representations of one variant (STYLE T1) — and so that the arm
// carries the record's identity, which a copy would lose.
func (*OpenContent) attribution() {}

// contentNode is one particle of a flattened content model. children holds the
// node indices of a model group's {particles} in document order, and is nil for
// an ·element particle· or ·wildcard particle·, whose {term} is the leaf the
// walk matches names against.
//
// term is the RESOLVED {term}: the flattening follows an <element ref> through
// the element index and a <group ref> through the model group index, so no walk
// step re-resolves anything. Nodes are appended in preorder, so a node's index
// is less than every index in its subtree.
//
// ambiguous marks the one shape the greedy iteration boundary is not exact on
// (see the file comment): this node's {max occurs} is greater than 1, and its
// body holds a particle whose own {max occurs} is too and which the walk can
// reach both as the first item of a fresh iteration and with the open iteration
// already complete, so an item that particle takes can fall either side of the
// boundary. The live set splits at such a node and at no other, which is why
// markAmbiguous computes it once here rather than the walk re-deriving it per
// item.
type contentNode struct {
	occurs    Occurs
	term      Term
	children  []int
	ambiguous bool
}

// span is a RUN of occurrence counts one region carries for one node: every
// count from lo through hi, lo <= hi. A single count is the span whose ends are
// equal, so a region over spans of one count each is the cursor-per-partition
// encoding this walk carried before.
type span struct{ lo, hi int }

// region is a set of live partitions of the items taken so far (cvc-accept
// clause 3.1) that differ in NOTHING the walk can see but the occurrence
// counters inside them: a span of counts for every node of the flattened model.
// It denotes the cartesian product of its spans. A Matcher holds regions
// covering every partition those items can have reached, and
// [Matcher.Accepting] asks its question of the whole cover rather than of a
// chosen member.
//
// WHERE those partitions stand is not here: the path of nodes the last item was
// ·attributed to· is one for the whole live set and is held once, on the
// Matcher (Matcher.path).
//
// Counters are CLAMPED at counterCap, so a region records how its partitions
// stand and not how they got there.
//
// # The band invariant, and why one walk step serves a whole region
//
// Every span lies inside one BAND of its node — a run of counts over which the
// three questions the walk ever asks a counter give one answer each. Those
// questions are all thresholds: canRepeat asks count < {max occurs},
// canExit and memberSatisfied ask count >= {min occurs}, and offerAllMembers
// asks count > 0, so the answers change only at 1, at {min occurs} and at
// counterCap (bandEnd). A band-uniform region therefore drives advance
// identically for every partition it denotes: one walk step decides them all,
// and the step's result is again a region, because count moves a whole span by
// one and clearSubtree pins one to zero.
//
// normalize is what restores the invariant after count carries a span past a
// band edge, splitting the region in two rather than letting one walk step
// answer for partitions that disagree.
type region struct {
	counts []span
}

// clone copies the counters, so a search pass that fails leaves nothing behind
// and two regions split from one share no array. The path a failed pass moved
// is the walk's own scratch (Matcher.probe), which the next attempt overwrites.
func (c *region) clone() region {
	t := region{counts: make([]span, len(c.counts))}
	copy(t.counts, c.counts)
	return t
}

// Matcher advances one complex type's {content type} particle over an
// element-information-item sequence, deciding cvc-particle (§3.9.4.2) for the
// sequence one item at a time. Obtain one from [Schema.ContentMatcher]; the
// zero value is not usable.
//
// A Matcher is single-use and stateful: it holds every position in the content
// model the items so far can have reached (region) and, under a {mode} suffix
// {open content}, whether the sequence has left clause 2's S1 for its S2 — so
// the caller feeds it one element's [[children]] in document order and drops
// it. It is not safe for concurrent use, and nothing in it is shared with the
// schema beyond the immutable components the flattening read.
type Matcher struct {
	s     *Schema
	ct    ComplexType
	open  *OpenContent
	nodes []contentNode
	live  []region

	// path is the path of nodes the last item was ·attributed to·, outermost
	// first, and it is the LIVE SET's rather than any one region's. Every live
	// partition stands at the same ·basic particle·. Two that did not would
	// ·compete· for the name that put them there — key-compete (§3.8.4.2) calls
	// two particles competing when one sequence has two ·paths· identical but
	// for their last item, which is exactly two live partitions taking one name
	// to different particles — and cos-nonambig (§3.8.6.4) forbids a content
	// model to contain two ·element particles· or two ·wildcard particles· that
	// compete, which Finalize has already decided (Phase C's
	// checkContentModelsUnambiguous). The competition it permits, an element
	// particle against a wildcard particle, never reaches the live set: one
	// search pass admits one kind of particle (admitKind), and Matcher.Next puts
	// the name to elements first. The flattened model is a tree, so that one
	// particle fixes one chain of ancestors. What live partitions differ in is
	// the occurrence counters, which is what a region carries.
	//
	// The premise is load-bearing for the ENCODING and not only for the
	// verdicts: a live set that did hold two paths would be answered with one
	// of them, silently, there being nowhere to put the other.
	path []int

	// probe and reached are the search's own scratch, kept here so that a name
	// put to a wide live set allocates no path per region. probe is the path
	// the branch attempt under way is building: every attempt starts by copying
	// path over it, so a failed attempt unwinds nothing. reached is the path
	// the first branch that took the current name arrived at — every branch
	// that takes it arrives at the same one, by the premise above — which step
	// swaps into path once the whole live set has been offered the name.
	probe   []int
	reached []int

	inS2 bool
}

// ContentMatcher returns a [Matcher] over t's {content type} particle, or (nil,
// false) where the element sequence of an element ·governed by· t is not
// decidable here. The decision is made ONCE, at construction: a Matcher that
// exists decides every name put to it, and never declines mid-sequence.
//
// It is a method on *Schema rather than a free constructor because the walk
// rests on constraints Finalize has already decided — cos-nonambig (§3.8.6.4)
// for the determinism it never re-derives, mg-props-correct clause 2 for the
// <group ref> acyclicity that lets the flattening carry no visited set (STYLE
// D4) — and a *Schema is the one thing that cannot exist before they ran.
//
// t must be a complex type of s, because the walk follows an <element ref> or
// <group ref> in t's particle by NAME through s's own indexes rather than
// through pre-resolved pointers: a name absent from s declines (nil, false),
// while a name present in s but bound to a different definition silently builds
// a Matcher over a model the caller never wrote — the precondition preserves at
// walk time what src-resolve (§3.17.6.2 clauses 1.3 and 1.5) and
// sch-props-correct clause 2 (§3.17.6.1) settle at construction time.
//
// A present {open content} is DECIDED rather than declined: the Matcher holds
// it and [Matcher.Next] offers the open wildcard whatever {particle} cannot
// take, per cvc-complex-content clauses 2 and 3 (see the file comment).
//
// The two declines, none of them a violation:
//
//   - a {content type} whose {variety} is empty or simple, which holds no
//     particle at all. cvc-complex-type clauses 1.1 and 1.2 govern those
//     directly and need no matcher.
//   - GAP(xsd): an ·ambiguous· node whose widened subtrees need more than
//     maxPartitionStates regions to cover the partitions they put in flight at
//     once. RULED permanent by #1601 (STYLE P3b) rather than a ceiling waiting
//     on its next raise: that thread carries the ruling in full, and it rests
//     on the measured cost below rather than on a spec licence. No spec licence
//     covers this branch and this marker claims none — cvc-accept (§3.9.4.3)
//     clause 3.1 is a bare existential over ·partitions· with no search
//     procedure and no bound, §3.9.4.1.1's L(P) attaches none either, and
//     neither Appendix C nor Appendix E.1's implementation-defined checklist
//     lets a processor decline what it cannot afford. The SHAPE is decided —
//     (a{1,2}, b?){2,2} takes "a a b" — and so now is the WIDTH a
//     partition-per-cursor encoding could not carry: (a{1,500}){1,500} reaches
//     a quarter of a million live partitions and the spans of a region cover
//     them in about a thousand. What stays declined is what outgrows the
//     ceiling, and a model gets there by BREADTH as readily as by depth:
//     partitionsBounded products over EVERY widened node of the flattened tree,
//     so sibling repeating groups under a non-repeating one, and a row of
//     repeating leaves under one repeating group, reach it with no nesting at
//     all. Declining withholds the whole element-sequence verdict, whose
//     consumers are validate's Result.violations and its one reader
//     Result.Violations, both of which carry violations PRESENT — so the
//     decline costs a rejection and manufactures none; validate records each
//     child it leaves unmatched, or the element where it has no child, as an
//     Unevaluated instead. What makes the approximation permanent is the
//     distance between the ceiling and what still reaches it. A walk over every
//     buildable complex type in testdata/xsdtests finds THREE declining here,
//     all on this arm — particlesZ035_a, particlesZ036_b and particlesZ036_c —
//     and each carries at least one model-group counter whose clamped range
//     ALONE outruns the ceiling, past 10^8 in the two Z036s and past 10^11 in
//     Z035_a. A single factor with no room ends the count, so deciding them
//     means a per-item budget of that many regions: at the ≈1.5 KB per region
//     maxPartitionStates' doc measures, over a hundred gigabytes and over a
//     hundred terabytes for ONE item. Four instance cases sit on those three,
//     all banked fail, and three of the four are suite-declared VALID, which
//     that lane decides valid for a complex-typed root with content only where
//     a Matcher decides that content (#1841) — so what the residual costs the
//     suite is one missed rejection, particlesZ035_a.i, and at most three valid
//     observations. It is retired by an encoding that carries a live partition
//     set without enumerating its cover, never by moving the constant. Two
//     findings reopen the ruling, and a case merely declining here is neither,
//     since three already do. One is a schema whose partitionsBounded product
//     lands BETWEEN the ceiling and what a measured per-region cost can afford
//     — the plateau above 40804, the figure maxPartitionStates' doc derives, is
//     empty today, so a model in it would mean a raise buys verdicts again. The
//     other is a per-region cost measured low enough to bring one of the three
//     counters named above inside an affordable ceiling. Re-measure rather than
//     quoting these figures: the instrument is that corpus walk over buildable
//     complex types, not suiteindex, which cannot reach a population defined by
//     a resource product rather than by a name or a nesting.
func (s *Schema) ContentMatcher(t ComplexType) (*Matcher, bool) {
	ec, ok := t.ContentType().(ElementContent)
	if !ok {
		return nil, false
	}
	m := &Matcher{s: s, ct: t, open: ec.OpenContent}
	if _, ok := m.flatten(ec.Particle); !ok {
		return nil, false
	}
	m.markAmbiguous(0)
	if !m.partitionsBounded() {
		return nil, false
	}
	m.live = []region{{counts: make([]span, len(m.nodes))}}
	return m, true
}

// flatten appends the node for p and, for a model group {term}, its whole
// subtree, returning p's node index. It reports false for a reference that
// resolves to nothing — unreachable on a *Schema that m.ct belongs to, whose
// Phase A rejected a dangling <element ref>/<group ref> (src-resolve clauses
// 1.3 and 1.5), and a decline rather than a skipped particle so an unresolved
// name can never widen the accepted language.
func (m *Matcher) flatten(p Particle) (int, bool) {
	t, ok := m.resolveTerm(p.Term())
	if !ok {
		return 0, false
	}
	i := len(m.nodes)
	m.nodes = append(m.nodes, contentNode{occurs: p.Occurs(), term: t})
	g, isGroup := t.(ModelGroup)
	if !isGroup {
		return i, true
	}
	for _, c := range g.Particles() {
		j, ok := m.flatten(c)
		if !ok {
			return 0, false
		}
		m.nodes[i].children = append(m.nodes[i].children, j)
	}
	return i, true
}

// resolveTerm reads a particle's {term} slot, following an <element ref>
// through elementIndex and a <group ref> through modelGroupIndex to the
// referenced definition's {model group} (§3.7.2), exactly as
// particleattribution.go's addTerm and wildcardadmit.go's termContainsName read
// them, so no Schema.ModelGroup accessor is minted for an in-package reader
// (STYLE T5).
func (m *Matcher) resolveTerm(t TermOrRef) (Term, bool) {
	switch t := t.(type) {
	case ResolvedTerm:
		return t.Term, t.Term != nil
	case ElementDeclarationRef:
		d, ok := m.s.Element(t.Name)
		return d, ok
	case ModelGroupRef:
		mgd, ok := m.s.modelGroupIndex[t.Name]
		if !ok {
			return nil, false
		}
		return mgd.ModelGroup(), true
	default:
		panic("xsd: Matcher.resolveTerm: non-exhaustive TermOrRef switch")
	}
}

// markAmbiguous sets contentNode.ambiguous over the subtree at i.
func (m *Matcher) markAmbiguous(i int) {
	for _, c := range m.nodes[i].children {
		m.markAmbiguous(c)
	}
	m.nodes[i].ambiguous = repeatable(m.nodes[i].occurs) && m.splitsInBody(i)
}

// splitsInBody reports whether one iteration of the group at i holds the
// particle P the file comment's first bullet asks for: one that repeats and
// stands at BOTH ends of i's iteration boundary, so that an item it takes can
// be the next one of the open iteration or the first of a fresh one. Those are
// exactly repeat's two conditions — enterBody reaching P, and iterationComplete
// holding where P stands — so a node whose body holds no such particle never
// widens the live set, however large its occurrence range.
func (m *Matcher) splitsInBody(i int) bool {
	g, isGroup := m.nodes[i].term.(ModelGroup)
	if !isGroup {
		return false
	}
	return m.splitsAmong(i, g, true, true)
}

// splitsAmong searches the {particles} of the group g at i for that particle.
// first and last say whether the walk can still reach this group's own members
// as the first item of the enclosing iteration and with that iteration already
// complete; a sequence narrows both by what it puts before and after each
// member (§3.8.4.1.1), while a choice takes one member whole (§3.8.4.1.2) and
// an all group interleaves them (§3.8.4.1.3), so neither narrows either.
//
// No all group reaches here on a finalized schema: markAmbiguous short-circuits
// on repeatable before calling splitsInBody, so every group this searches stands
// at or under a repeating particle, and cos-all-limited (§3.8.6.2) clause 1 admits
// an all group only as a model group definition's {model group}, as the {term}
// of a {max occurs} = 1 particle that is a complex type's {particle}, or as a
// {min occurs} = {max occurs} = 1 member of another all group.
func (m *Matcher) splitsAmong(i int, g ModelGroup, first, last bool) bool {
	children := m.nodes[i].children
	for k, ch := range children {
		f, l := first, last
		if g.Compositor() == CompositorSequence {
			f = f && m.allEmptiable(children[:k])
			l = l && m.allEmptiable(children[k+1:])
		}
		if f && l && repeatable(m.nodes[ch].occurs) {
			return true
		}
		if cg, isGroup := m.nodes[ch].term.(ModelGroup); isGroup && m.splitsAmong(ch, cg, f, l) {
			return true
		}
	}
	return false
}

// allEmptiable reports whether every particle of ps ·accepts· the empty
// sequence, which is what makes the ones before a member skippable on the way
// in and the ones after it skippable on the way out.
func (m *Matcher) allEmptiable(ps []int) bool {
	for _, p := range ps {
		if !m.emptiable(p) {
			return false
		}
	}
	return true
}

// repeatable reports whether an occurrence range admits more than one
// occurrence.
func repeatable(o Occurs) bool {
	max, bounded := o.Max()
	return !bounded || max > 1
}

// maxPartitionStates is the BUDGET one item of the instance is allowed to
// spend: a name is put to every live region in turn, so one item's cost is a
// constant of the SCHEMA and never a function of the items already taken.
// ContentMatcher weighs partitionsBounded's estimate of that cover against the
// budget once, at construction, rather than a Matcher declining a name
// mid-sequence.
//
// It is 65536 and not 2048 because 2048 declined the widest model a ceiling of
// defensible cost reaches — a sequence over two SIBLING groups of {1,100},
// whose widened nodes product to 40804 — and what that raise costs is one item
// at BenchmarkMatcherNext's widest live set: 16129 regions, 17ms and 25MB,
// about a microsecond and 1.5KB per region held. At 2048 the same benchmark's
// widest set was 441 regions, 477µs and 687KB, so the cost is linear in the
// ceiling and a model pays it only by standing at the ceiling.
//
// 40804 is NOT the widest model the W3C suite holds, and 65536 does not decide
// the suite: particlesZ036_c products past 10^8, particlesZ036_b past 10^13,
// and particlesZ035_a carries a single {1,100000000000}. That tail declines at
// 65536 exactly as it did at 2048, and only a ceiling past 10^13 — one no cost
// measured here justifies — would decide it. No ceiling between 40804 and that
// tail buys another case, so every one of them decides the same four instance
// cases (#1601). 65536 is the least power of two clearing 40804, and a ceiling
// past the headroom it leaves buys slower items and no verdicts.
const maxPartitionStates = 65536

// partitionsBounded reports whether the regions covering the live partitions
// stay inside maxPartitionStates.
//
// # Why the counters are the whole of what varies
//
// The whole live set stands at one ·basic particle· and so shares one path,
// which the walk holds once and no region carries (Matcher.path derives it).
// Live partitions therefore differ ONLY in the occurrence counters an
// ·ambiguous· node's iteration boundary moves, its own and its subtree's
// (markWidened).
//
// # What a widened counter costs in regions
//
// Both factors below are an ESTIMATE of the cover, not a bound on the
// partitions: the walk's verdicts do not depend on either being tight, only on
// the budget one item is allowed to spend (maxPartitionStates).
//
//   - A model group's counter is an ITERATION count, and repeat clears the
//     subtree beneath it, so two live partitions that differ in it stand in
//     unrelated positions of the body and no span merges them. It costs its
//     whole clamped range, counterCap+1, as it did when every partition carried
//     its own cursor.
//   - A leaf's counter advances by ONE per item inside the open iteration, so
//     the partitions that differ only in where the last iteration boundary fell
//     inside that leaf's own run of items cover in one span. The boundary can
//     only fall where the items after it are still a word of the body, which
//     pins it to a run per leaf and not to a position per item — so the leaf
//     costs its band count (bandCount), which is at most four however wide its
//     occurrence range.
//
// Both factors are at most counterCap+1, the cursor-per-partition count this
// walk bounded before, so no model that was carried then is declined now.
//
// The product is of {max occurs} values and would overflow for a model that
// combines large ones, so it is never formed: each factor is checked against
// the room the running product leaves, and a factor with no room ends the count
// at once.
func (m *Matcher) partitionsBounded() bool {
	widened := make([]bool, len(m.nodes))
	m.markWidened(0, false, widened)
	states := 1
	for i, w := range widened {
		if !w {
			continue
		}
		values := m.regionFactor(i)
		if values > maxPartitionStates || states > maxPartitionStates/values {
			return false
		}
		states *= values
	}
	return true
}

// regionFactor is what the node at i contributes to partitionsBounded's
// estimate: its band count where it is a leaf, whose counter a span covers, and
// its clamped range where it is a model group, whose iteration count a span
// never spans across.
func (m *Matcher) regionFactor(i int) int {
	if _, isGroup := m.nodes[i].term.(ModelGroup); isGroup {
		return m.counterCap(i) + 1
	}
	return m.bandCount(i)
}

// markWidened marks every node of the subtree at i that lies inside an
// ·ambiguous· node's subtree, i's own ambiguity included. inside says whether an
// ancestor of i is one.
func (m *Matcher) markWidened(i int, inside bool, widened []bool) {
	inside = inside || m.nodes[i].ambiguous
	widened[i] = inside
	for _, c := range m.nodes[i].children {
		m.markWidened(c, inside, widened)
	}
}

// counterCap is the occurrence count beyond which canRepeat and canExit stop
// changing their answers: {max occurs} where it is a number, and {min occurs}
// where it is unbounded and only the {min occurs} half of cvc-accept clauses
// 1.1, 2.1 and 3.1 is left to decide.
func (m *Matcher) counterCap(i int) int {
	if max, bounded := m.nodes[i].occurs.Max(); bounded {
		return max
	}
	return m.nodes[i].occurs.Min()
}

// bandEnd reports the largest count w for which every count from v through w
// answers the walk's counter questions alike, so the bands of the node at i are
// the runs between the edges those answers change at (region).
func (m *Matcher) bandEnd(i, v int) int {
	top := m.counterCap(i)
	end := top
	for _, e := range [...]int{1, m.nodes[i].occurs.Min(), top} {
		if e > v && e-1 < end {
			end = e - 1
		}
	}
	return end
}

// bandCount is how many bands the node at i has, which is how many spans it
// takes to cover its whole clamped range. It is at most four: the edges are 0,
// 1, {min occurs} and counterCap, whatever the occurrence range between them.
func (m *Matcher) bandCount(i int) int {
	n := 0
	for v := 0; v <= m.counterCap(i); v = m.bandEnd(i, v) + 1 {
		n++
	}
	return n
}

// count records one more occurrence of the node at i in c, moving the whole
// span and clamping it at counterCap so that a region records where its
// partitions stand rather than how far they have gone (region). It is the one
// operation that can carry a span past a band edge, which normalize splits.
func (m *Matcher) count(c *region, i int) {
	top := m.counterCap(i)
	if c.counts[i].lo < top {
		c.counts[i].lo++
	}
	if c.counts[i].hi < top {
		c.counts[i].hi++
	}
}

// normalize appends the band-uniform regions covering t to rs, splitting t at
// the first band edge a span crosses and recurring on both halves. Only a node
// whose count has just moved can cross an edge, so one item taken costs at most
// one split per level of the path it was ·attributed to· (Matcher.path).
func (m *Matcher) normalize(rs []region, t region) []region {
	for i := range t.counts {
		end := m.bandEnd(i, t.counts[i].lo)
		if t.counts[i].hi <= end {
			continue
		}
		tail := t.clone()
		tail.counts[i].lo = end + 1
		t.counts[i].hi = end
		return m.normalize(m.normalize(rs, t), tail)
	}
	return append(rs, t)
}

// collapse folds rs into the fewest regions that cover the same partitions,
// which is what holds the live set inside the estimate partitionsBounded
// checked: without it two partitions that have converged would each go on
// splitting, and the set would grow with the instance. Sorting first puts the
// regions that differ in one node's span next to each other, so one pass over a
// stack merges every run of them (mergeInto), and the result is in one order
// whatever order advance produced them in (STYLE D2).
func (m *Matcher) collapse(rs []region) []region {
	slices.SortFunc(rs, compareRegions)
	out := rs[:0]
	for _, t := range rs {
		for len(out) > 0 && m.mergeInto(&out[len(out)-1], t) {
			t = out[len(out)-1]
			out = out[:len(out)-1]
		}
		out = append(out, t)
	}
	return out
}

// mergeInto widens a to cover b's partitions too and reports whether it could,
// which is where their spans differ at no more than one node, that node is a
// LEAF, and there the two runs meet or overlap without leaving the band both
// lie in. Two equal regions merge either way, which is how a duplicate is
// dropped. Standing at the same particle is not a condition it tests, the whole
// live set standing at one (Matcher.path).
//
// Only a LEAF's span widens, so the live set holds one region per assignment of
// the model groups' ITERATION counts and one span per band inside it, which is
// the shape partitionsBounded estimates. Widening an iteration count too leaves
// regions OVERLAPPING — one covering a body position another already covers —
// and an overlapping cover takes more entries, not fewer.
func (m *Matcher) mergeInto(a *region, b region) bool {
	j := -1
	for i := range a.counts {
		if a.counts[i] == b.counts[i] {
			continue
		}
		if j >= 0 {
			return false
		}
		j = i
	}
	if j < 0 {
		return true
	}
	if _, isGroup := m.nodes[j].term.(ModelGroup); isGroup {
		return false
	}
	lo, hi := min(a.counts[j].lo, b.counts[j].lo), max(a.counts[j].hi, b.counts[j].hi)
	if min(a.counts[j].hi, b.counts[j].hi)+1 < max(a.counts[j].lo, b.counts[j].lo) {
		return false
	}
	if m.bandEnd(j, lo) < hi {
		return false
	}
	a.counts[j] = span{lo, hi}
	return true
}

// compareRegions orders regions by span, outermost node first, so that collapse
// sees every pair differing in one node's span adjacently. There is no path to
// order them by first: the live set stands at one particle (Matcher.path).
func compareRegions(a, b region) int {
	for i, s := range a.counts {
		if c := cmp.Compare(s.lo, b.counts[i].lo); c != 0 {
			return c
		}
		if c := cmp.Compare(s.hi, b.counts[i].hi); c != 0 {
			return c
		}
	}
	return 0
}

// Next advances the content model over one element information item whose
// ·expanded name· is name, reporting what the item is ·attributed to·
// (§3.4.4.4). It reports (nil, false) when no particle live under any partition
// of the items already taken admits the name, which is the cvc-accept
// (§3.9.4.3) rejection its caller charges against that item's own location; the
// Matcher is unchanged by a rejected name, so a caller may stop at the first one
// or keep feeding.
//
// The first two searches are cvc-accept's element/wildcard precedence: every
// live particle of every live partition is offered the name as an ·element
// particle· first (clause 2.3.1's expanded-name match, then clause 2.3.2's
// ·substitution group· membership), and only a name no element particle
// admits is offered to the wildcard particles (clause 1, cvc-wildcard
// §3.10.4.1 in full).
//
// A third search follows them where the {content type}'s {open content} is
// present, and is reachable only once both have failed — which is exactly
// cvc-complex-content clause 2.3's and 3.3's "has no ·path· in {particle}"
// (openNext, and the file comment for why the resulting split is the only one
// those clauses admit).
func (m *Matcher) Next(name QName) (Attribution, bool) {
	if m.inS2 {
		return m.openNext(name)
	}
	if a, ok := m.step(nameTest{name, admitElements}); ok {
		return a, true
	}
	if a, ok := m.step(nameTest{name, admitWildcards}); ok {
		return a, true
	}
	return m.openNext(name)
}

// openNext offers name to {open content}.{wildcard}, the third tier
// cvc-complex-content clauses 2.4 and 3.4 add over cvc-accept's two, and
// reports the admitted item as ·attributed to· the {open content} (§3.4.4.4) —
// the record itself, on [Attribution]'s terms, and not its {wildcard}.
//
// The walk is not advanced: an S2 member is matched against the wildcard alone,
// so {particle} stands where S1 left it and [Matcher.Accepting] still asks
// clause 2.2's and 3.2's question of S1 and of nothing else. In {mode} suffix
// an admitted item also closes S1 for good (clause 2.1's S = S1 + S2 being a
// concatenation), which is the one thing an admitted name changes beyond the
// answer. A REJECTED name changes nothing at all, on [Matcher.Next]'s terms.
func (m *Matcher) openNext(name QName) (Attribution, bool) {
	if m.open == nil {
		return nil, false
	}
	if !m.s.allowsElementWildcardName(m.open.Wildcard(), m.ct, name) {
		return nil, false
	}
	if m.open.Mode() == OpenContentSuffix {
		m.inS2 = true
	}
	return m.open, true
}

// Accepting reports whether the sequence fed so far is ·accepted· by the
// content model — whether every open particle can be closed where the sequence
// stopped. A false result is the "the sequence ends short of a particle it must
// satisfy" half of cvc-accept, which the caller charges against the containing
// element rather than any child, there being no child at the offending
// position.
//
// Under a present {open content} the sequence asked about is S1 and not every
// item fed: cvc-complex-content clauses 2.2 and 3.2 put S1 alone to
// cvc-particle, and the items the open wildcard took are S2, which clauses 2.4
// and 3.4 have already decided one at a time.
//
// SOME partition has to close, not every one: cvc-accept clause 3.1 asks for
// the existence of one, so a sequence the greedy partition leaves owing a
// particle is still ·accepted· where another live partition owes nothing.
func (m *Matcher) Accepting() bool {
	for i := range m.live {
		if m.accepts(&m.live[i]) {
			return true
		}
	}
	return false
}

// Admissible reports what the next element information item could be
// ·attributed to· (§3.4.4.4) where the sequence stands: the {term} of every
// leaf particle [Matcher.Next]'s walk offers a name to, across every live
// partition, before it reports (nil, false), and then the {open content} where
// Next would offer it the name too. It is the expected set a caller names when
// it charges cvc-complex-content (§3.4.4.3) at a rejected item or at a
// sequence that ended short; neither that rule nor cvc-accept (§3.9.4.3)
// prescribes message content, and this prescribes none either.
//
// Next(n) succeeds exactly where some returned arm admits n: an
// [ElementDeclaration] by its ·expanded name· or a member of its ·substitution
// group· (cvc-accept clause 2.3), a [Wildcard] and the [*OpenContent]'s
// {wildcard} by cvc-wildcard (§3.10.4.1). A non-empty result does not mean some
// name is accepted — a wildcard's {disallowed names} can leave it admitting
// none — and an [ElementDeclaration] is the particle's own declaration D, never
// the ·substitution group· members it stands for.
//
// What the result holds follows from Next's own walk and from no rule stated
// here: a member of an all group is listed while it is short of its {max
// occurs}, through the gate every leaf Next reaches passes; the [*OpenContent]
// is listed under {mode} interleave always, under {mode} suffix alongside the
// particles until an item has started clause 2's S2, and alone once one has
// (clause 2.1's S = S1 + S2 being a concatenation).
//
// The particles are in ascending order of their place in the content model —
// the preorder of its flattened {particles}, which is document order — with the
// [*OpenContent] last. A particle reached from several live partitions is
// listed once, and no two listed element particles share an ·expanded name·:
// both would ·compete· for it, which cos-nonambig (§3.8.6.4) has already
// rejected. The result is empty where the content model admits no further item.
//
// It is read-only: [Matcher.Next] and [Matcher.Accepting] answer identically
// before and after it. Each call returns a fresh slice the caller may keep; its
// elements are the schema's immutable components on [Attribution]'s terms, the
// [*OpenContent] being the pointer [ElementContent] carries. One call walks
// every live region once, which is what one rejected name costs Next, so it is
// priced for a charge and not for every item.
func (m *Matcher) Admissible() []Attribution {
	var out []Attribution
	if !m.inS2 {
		f := &frontierTest{}
		for i := range m.live {
			m.advance(&m.live[i], f)
		}
		slices.Sort(f.hits)
		for _, i := range slices.Compact(f.hits) {
			a, isLeaf := m.nodes[i].term.(Attribution)
			if !isLeaf {
				panic("xsd: Matcher.Admissible: a model group is not a leaf of a flattened content model")
			}
			out = append(out, a)
		}
	}
	if m.open != nil {
		out = append(out, m.open)
	}
	return out
}

// accepts reports whether one partition closes where the sequence stopped:
// every node on the live path can be left with that partition's counters
// (canExit), or no item was taken at all and the model is ·emptiable·.
func (m *Matcher) accepts(c *region) bool {
	if len(m.path) == 0 {
		return m.emptiable(0)
	}
	for d := len(m.path) - 1; d >= 0; d-- {
		if !m.canExit(c, d) {
			return false
		}
	}
	return true
}

// admitKind is which kind of ·basic particle· one search pass will match a name
// against, so that cvc-accept's Note — an item both a Wildcard and an Element
// Declaration accept goes to the Element Declaration — is one search order and
// not a tie-break buried at the match site.
type admitKind int

const (
	admitElements admitKind = iota
	admitWildcards
)

// step runs one search pass over every live region, replacing the live set with
// the regions that took the name and leaving it untouched where none did. What
// advance produces is normalized back inside the band invariant and collapsed,
// so the set the next item is put to is the fewest regions covering the
// partitions still live (region).
//
// Every partition that takes the name ·attributes· it to the same ·basic
// particle· and reaches the same path (Matcher.path derives it), so the first
// attribution is the attribution and the partitions differ in nothing the
// caller can see. The pass therefore reads the live path throughout and
// installs the one advance reached at the end of it, where a name no region
// took leaves it as it stood ([Matcher.Next]).
func (m *Matcher) step(test leafTest) (Attribution, bool) {
	var taken Attribution
	m.reached = m.reached[:0]
	next := make([]region, 0, 2*len(m.live))
	for i := range m.live {
		a, cs := m.advance(&m.live[i], test)
		if len(cs) == 0 {
			continue
		}
		if taken == nil {
			taken = a
		}
		for _, c := range cs {
			next = m.normalize(next, c)
		}
	}
	if taken == nil {
		return nil, false
	}
	m.path, m.reached = m.reached, m.path
	m.live = m.collapse(next)
	return taken, true
}

// advance collects every region reachable from c by consuming the item test
// takes. It works outward exactly as one greedy walk does — the particle the
// last item was attributed to, then the rest of the iteration containing it,
// then a further iteration of that particle's group, then the same three
// questions one level out — and moving out is legal only while the level being
// left can be closed (canExit), which is where an unsatisfied {min occurs}
// stops the search rather than being noticed later.
//
// What the set adds to that walk is confined to an ·ambiguous· node, where BOTH
// answers are kept: the region that stays in the open iteration and the region
// that closes it and starts the next. Everywhere else the first answer is the
// only one (see the file comment), and the search ends as soon as no ·ambiguous·
// node is left to widen at.
//
// The regions it returns may straddle a band edge, count having just moved a
// whole span; normalize is what puts them back inside the band invariant, and
// no guard reads one in between.
//
// The walk it drives reads the live path and writes the scratch one, so every
// attempt below starts from the path as the last item left it (probeFromPath)
// and an attempt that fails is abandoned rather than unwound.
//
// A pass whose test takes nothing (frontierTest) fails every attempt, so it
// walks every leaf a rejected name is offered and leaves c, the live path and
// Matcher.reached as it found them: every region it would change is a clone,
// and keepProbe runs only for an attempt that took an item.
func (m *Matcher) advance(c *region, test leafTest) (Attribution, []region) {
	if len(m.path) == 0 {
		t := c.clone()
		m.probeFromPath()
		a, ok := m.enter(&t, 0, test)
		if !ok {
			return nil, nil
		}
		m.keepProbe()
		return a, []region{t}
	}
	var taken Attribution
	var out []region
	for d := len(m.path) - 1; d >= 0; d-- {
		t := c.clone()
		m.probeFromPath()
		if a, ok := m.continueIn(&t, d, test); ok {
			m.keepProbe()
			taken, out = a, append(out, t)
		}
		if len(out) == 0 || m.nodes[m.path[d]].ambiguous {
			r := c.clone()
			m.probeFromPath()
			if a, ok := m.repeat(&r, d, test); ok {
				m.keepProbe()
				taken, out = a, append(out, r)
			}
		}
		if len(out) > 0 && !m.widensAbove(d) {
			return taken, out
		}
		if !m.canExit(c, d) {
			return taken, out
		}
	}
	return taken, out
}

// probeFromPath starts one branch attempt at the live path. A failed attempt
// therefore costs no unwinding — the next one overwrites whatever it left — and
// a whole live set put to one name allocates no path per region.
func (m *Matcher) probeFromPath() {
	m.probe = append(m.probe[:0], m.path...)
}

// keepProbe records the path the attempt just taken arrived at as the one step
// will install, where no earlier attempt of the same pass has. Which is kept
// decides nothing: every branch that takes one name arrives at the same path
// (Matcher.path). An empty reached is the pass's "none yet", no branch that took
// an item leaving the path empty.
func (m *Matcher) keepProbe() {
	if len(m.reached) > 0 {
		return
	}
	m.reached = append(m.reached[:0], m.probe...)
}

// widensAbove reports whether the live path holds an ·ambiguous· node shallower
// than depth d. Where it does not, a region already found is the only one the
// rest of the path can reach, by the confinement contentNode.ambiguous and the
// file comment state.
func (m *Matcher) widensAbove(d int) bool {
	for _, i := range m.path[:d] {
		if m.nodes[i].ambiguous {
			return true
		}
	}
	return false
}

// continueIn consumes the item test takes inside the OPEN occurrence of the
// node at depth d of the live path, whose own deeper position has already
// failed to consume it and been closed: one more occurrence of a leaf, or a
// later member of a group's open iteration. A leaf is the path's last node, so
// one more occurrence of one leaves the path where it stands and the attempt's
// scratch copy of it untouched.
func (m *Matcher) continueIn(c *region, d int, test leafTest) (Attribution, bool) {
	i := m.path[d]
	g, isGroup := m.nodes[i].term.(ModelGroup)
	if !isGroup {
		a, ok := m.admits(c, i, test)
		if !ok {
			return nil, false
		}
		m.count(c, i)
		return a, true
	}
	return m.continueIteration(c, d, g, m.slotOf(i, m.path[d+1]), test)
}

// repeat consumes the item test takes as the first item of a FRESH iteration of
// the group at depth d of the live path, which needs that group's open
// iteration to be a whole word of its language already (iterationComplete) and
// its {max occurs} to admit another (cvc-accept clause 3.2). A leaf has no
// iteration to close — its occurrences are continueIn's counter — so it never
// repeats this way.
func (m *Matcher) repeat(c *region, d int, test leafTest) (Attribution, bool) {
	i := m.path[d]
	g, isGroup := m.nodes[i].term.(ModelGroup)
	if !isGroup {
		return nil, false
	}
	if !m.iterationComplete(c, i, g, m.slotOf(i, m.path[d+1])) {
		return nil, false
	}
	if !m.canRepeat(c, i) {
		return nil, false
	}
	m.clearSubtree(c, i)
	m.probe = m.probe[:d+1]
	a, ok := m.enterBody(c, i, g, test)
	if !ok {
		return nil, false
	}
	m.count(c, i)
	return a, true
}

// continueIteration consumes the item test takes later in the OPEN iteration of
// the group at depth d, whose member at slot has just been closed. Each
// compositor offers what §3.8.4.1 says follows: a sequence its remaining
// members while each skipped one is ·emptiable· (§3.8.4.1.1), a choice nothing
// at all, since an iteration of a choice is one member (§3.8.4.1.2), and an all
// group any member that can still take the item — one short of its {max
// occurs}, or a nested all group with an occurrence open to resume
// (offerAllMembers) — since S1 × … × Sn interleaves them (§3.8.4.1.3).
func (m *Matcher) continueIteration(c *region, d int, g ModelGroup, slot int, test leafTest) (Attribution, bool) {
	children := m.nodes[m.path[d]].children
	switch g.Compositor() {
	case CompositorChoice:
		return nil, false
	case CompositorAll:
		i := m.path[d]
		m.probe = m.probe[:d+1]
		return m.offerAllMembers(c, i, test)
	case CompositorSequence:
		m.probe = m.probe[:d+1]
		for _, ch := range children[slot+1:] {
			if a, ok := m.enter(c, ch, test); ok {
				return a, true
			}
			if !m.emptiable(ch) {
				return nil, false
			}
		}
		return nil, false
	default:
		panic("xsd: Matcher.continueIteration: non-exhaustive Compositor switch")
	}
}

// offerAllMembers offers test to each member of the all group at i in turn,
// which §3.8.4.1.3 licenses whatever member the last item went to: L(M) is
// S1 × … × Sn, the INTERLEAVE of the members' own sequences, so nothing
// requires one member's items to be contiguous.
//
// A member that is itself a model group — cos-all-limited (§3.8.6.2) clause 2
// admits an all group there and nothing else — is RESUMED where it already has
// an occurrence open and entered where it does not, and its occurrence counter
// is the whole of that discriminator: clause 1.3 pins the member to
// {min occurs} = {max occurs} = 1, so a non-zero count is the one open
// occurrence it will ever have.
func (m *Matcher) offerAllMembers(c *region, i int, test leafTest) (Attribution, bool) {
	for _, ch := range m.nodes[i].children {
		if _, isGroup := m.nodes[ch].term.(ModelGroup); isGroup && c.counts[ch].lo > 0 {
			if a, ok := m.resumeAll(c, ch, test); ok {
				return a, true
			}
			continue
		}
		if a, ok := m.enter(c, ch, test); ok {
			return a, true
		}
	}
	return nil, false
}

// resumeAll consumes the item test takes inside the occurrence of the nested
// all group at i that an earlier item already opened, putting i back on the
// attempt's path with its counters as they stand and offering test to its own
// members — to whatever depth clause 1.3's exactly-once nesting reaches.
// Neither the occurrence counter nor the subtree beneath it moves: this is the
// same occurrence suspended by an item that went to a sibling, not a new one.
// It leaves the attempt's path where it found it when test takes nothing, as
// enter does, so the sibling tried next descends from the right node.
func (m *Matcher) resumeAll(c *region, i int, test leafTest) (Attribution, bool) {
	m.probe = append(m.probe, i)
	a, ok := m.offerAllMembers(c, i, test)
	if !ok {
		m.probe = m.probe[:len(m.probe)-1]
		return nil, false
	}
	return a, true
}

// enter consumes the item test takes as the FIRST item of a fresh occurrence of
// the node at i, appending the nodes it descended through to the attempt's
// path. It leaves the walk state untouched when test takes nothing, so a caller
// may try members in turn.
func (m *Matcher) enter(c *region, i int, test leafTest) (Attribution, bool) {
	g, isGroup := m.nodes[i].term.(ModelGroup)
	if !isGroup {
		a, ok := m.admits(c, i, test)
		if !ok {
			return nil, false
		}
		m.count(c, i)
		m.probe = append(m.probe, i)
		return a, true
	}
	if !m.canRepeat(c, i) {
		return nil, false
	}
	m.probe = append(m.probe, i)
	a, ok := m.enterBody(c, i, g, test)
	if !ok {
		m.probe = m.probe[:len(m.probe)-1]
		return nil, false
	}
	m.count(c, i)
	return a, true
}

// enterBody consumes the item test takes as the first item of one iteration of
// the group at i: the first member of a sequence that admits it, while every
// member skipped before it is ·emptiable· (§3.8.4.1.1), or any member of a
// choice (§3.8.4.1.2) or an all group (§3.8.4.1.3). Members are tried in
// document order, which decides nothing a second member could have decided
// differently — cos-nonambig has already rejected a group where two of them
// admit one name.
func (m *Matcher) enterBody(c *region, i int, g ModelGroup, test leafTest) (Attribution, bool) {
	for _, ch := range m.nodes[i].children {
		if a, ok := m.enter(c, ch, test); ok {
			return a, true
		}
		if g.Compositor() == CompositorSequence && !m.emptiable(ch) {
			return nil, false
		}
	}
	return nil, false
}

// admits reports what the leaf particle at i attributes an item to under one
// search pass's test, or false when the particle is exhausted or the test does
// not take it. The {max occurs} gate is the particle's and is asked here, before
// the test, so a pass of either kind reaches exactly the leaves that still have
// an occurrence to give.
func (m *Matcher) admits(c *region, i int, test leafTest) (Attribution, bool) {
	if !m.canRepeat(c, i) {
		return nil, false
	}
	return test.take(m, i)
}

// leafTest is what one pass of the walk asks of each leaf particle it reaches
// with an occurrence left (admits). The walk — advance and everything it calls
// — is the same for every pass, so the two passes it serves differ in this
// question alone: nameTest takes one ·expanded name·, which is [Matcher.Next],
// and frontierTest takes nothing and records every leaf it was offered, which
// is [Matcher.Admissible]. A pass whose test never takes a leaf walks exactly
// the leaves a rejected name is offered.
type leafTest interface {
	take(m *Matcher, i int) (Attribution, bool)
}

// nameTest takes the leaf at i where it admits name under kind.
//
// The element case takes cvc-accept clause 2.3.1 before clause 2.3.2, which is
// what makes the reported declaration the particle's own D and not the
// ·substituting declaration· S. Clause 2.3.2's other conjuncts — D top-level, D
// not blocking substitution, S ·substitutable· for D — are the whole of what
// inSubstitutionGroupOf decides (substitutiongroup.go), so no part of the
// clause is restated here.
//
// The wildcard case is cvc-wildcard (§3.10.4.1) in full, including the
// defined/sibling {disallowed names} keywords, which need the containing
// complex type — the reason ContentMatcher takes one.
type nameTest struct {
	name QName
	kind admitKind
}

// take reports what the leaf at i attributes an item named t.name to; see the
// nameTest doc comment.
func (t nameTest) take(m *Matcher, i int) (Attribution, bool) {
	switch leaf := m.nodes[i].term.(type) {
	case ElementDeclaration:
		if t.kind != admitElements {
			return nil, false
		}
		if leaf.Name() == t.name {
			return leaf, true
		}
		if m.s.nameInSubstitutionGroupOf(t.name, leaf) {
			return leaf, true
		}
		return nil, false
	case Wildcard:
		if t.kind != admitWildcards {
			return nil, false
		}
		if !m.s.allowsElementWildcardName(leaf, m.ct, t.name) {
			return nil, false
		}
		return leaf, true
	case ModelGroup:
		panic("xsd: nameTest.take: a model group is not a leaf of a flattened content model")
	default:
		panic("xsd: nameTest.take: non-exhaustive Term switch")
	}
}

// frontierTest takes no leaf and appends the node index of every leaf it is
// offered to hits, in the order the walk offers them, duplicates included: one
// leaf is reached once per live region that reaches it.
type frontierTest struct {
	hits []int
}

// take records the leaf at i and declines it, so the walk goes on to every
// other leaf a rejected name would have been offered.
func (t *frontierTest) take(_ *Matcher, i int) (Attribution, bool) {
	t.hits = append(t.hits, i)
	return nil, false
}

// canRepeat reports whether the node at i may take one more occurrence
// (cvc-accept clauses 1.2, 2.2 and 3.2, the {max occurs} half).
func (m *Matcher) canRepeat(c *region, i int) bool {
	max, bounded := m.nodes[i].occurs.Max()
	return !bounded || c.counts[i].lo < max
}

// canExit reports whether the node at depth d of the live path can be left
// where c's partitions stand: its open iteration closed, and its own count at
// {min occurs} — or short of it with an ·emptiable· body, since the iterations
// still owed can then each be empty (cvc-accept clauses 1.1, 2.1 and 3.1).
//
// A member of an ALL group is left without its {min occurs} or its own open
// iteration being consulted at all. §3.8.4.1.3 makes an all group's language
// S1 × … × Sn, the INTERLEAVE of its members' own sequences, so a member is
// suspended and resumed rather than finished when the next item belongs to a
// sibling; what the member owes is owed to the group, and iterationComplete is
// where the group collects it from every member at once.
func (m *Matcher) canExit(c *region, d int) bool {
	i := m.path[d]
	if m.inAllGroup(d) {
		return true
	}
	if g, isGroup := m.nodes[i].term.(ModelGroup); isGroup {
		if !m.iterationComplete(c, i, g, m.slotOf(i, m.path[d+1])) {
			return false
		}
	}
	if c.counts[i].lo >= m.nodes[i].occurs.Min() {
		return true
	}
	return m.bodyEmptiable(i)
}

// inAllGroup reports whether the node at depth d of the live path is a member
// of an all group.
func (m *Matcher) inAllGroup(d int) bool {
	if d == 0 {
		return false
	}
	g, isGroup := m.nodes[m.path[d-1]].term.(ModelGroup)
	return isGroup && g.Compositor() == CompositorAll
}

// iterationComplete reports whether the open iteration of the group at i, whose
// member at slot has just been closed, has contributed a whole word of the
// group's language: for a sequence every later member is skippable, for a
// choice the one member taken is the whole of it, and for an all group every
// member has reached its own {min occurs}.
func (m *Matcher) iterationComplete(c *region, i int, g ModelGroup, slot int) bool {
	children := m.nodes[i].children
	switch g.Compositor() {
	case CompositorSequence:
		for _, ch := range children[slot+1:] {
			if !m.emptiable(ch) {
				return false
			}
		}
		return true
	case CompositorChoice:
		return true
	case CompositorAll:
		for _, ch := range children {
			if !m.memberSatisfied(c, ch) {
				return false
			}
		}
		return true
	default:
		panic("xsd: Matcher.iterationComplete: non-exhaustive Compositor switch")
	}
}

// memberSatisfied reports whether the member at i of an all group owes that
// group nothing where the sequence stands: its occurrence count is at
// {min occurs}, or short of it with an ·emptiable· body so the occurrences
// still owed can each be empty (cvc-accept clauses 1.1, 2.1 and 3.1).
//
// A member that is itself an all group — cos-all-limited (§3.8.6.2) clause 2
// admits no other kind there — owes what its OWN members owe as well, since
// §3.8.4.1.3 folds its language into the same interleave rather than closing it
// where the last of its items fell. That is the debt canExit declines to
// collect from a member of an all group, recursively over whatever depth clause
// 1.3 permits. A leaf has no members, so the recursion ends there.
func (m *Matcher) memberSatisfied(c *region, i int) bool {
	if c.counts[i].lo < m.nodes[i].occurs.Min() && !m.bodyEmptiable(i) {
		return false
	}
	for _, ch := range m.nodes[i].children {
		if !m.memberSatisfied(c, ch) {
			return false
		}
	}
	return true
}

// emptiable reports whether the particle at i ·accepts· the empty sequence
// (cos-group-emptiable §3.9.6.3 read over the particle rather than the group):
// either it need not occur at all, or one occurrence of it can be empty.
func (m *Matcher) emptiable(i int) bool {
	return m.nodes[i].occurs.Min() == 0 || m.bodyEmptiable(i)
}

// bodyEmptiable reports whether ONE occurrence of the particle at i can be
// empty. A leaf occurrence is one element information item and never is; a
// sequence or all group needs every member ·emptiable· and a choice needs one
// (§3.8.4.1.1-3), and an empty choice — which accepts nothing at all, not the
// empty sequence — needs none.
func (m *Matcher) bodyEmptiable(i int) bool {
	g, isGroup := m.nodes[i].term.(ModelGroup)
	if !isGroup {
		return false
	}
	children := m.nodes[i].children
	if g.Compositor() == CompositorChoice {
		for _, c := range children {
			if m.emptiable(c) {
				return true
			}
		}
		return false
	}
	for _, c := range children {
		if !m.emptiable(c) {
			return false
		}
	}
	return true
}

// clearSubtree resets the occurrence counters beneath i in c, which a new
// iteration of i starts over. i's own counter is the iteration count and is not
// touched.
func (m *Matcher) clearSubtree(c *region, i int) {
	for _, ch := range m.nodes[i].children {
		c.counts[ch] = span{}
		m.clearSubtree(c, ch)
	}
}

// slotOf reports the position of child among the {particles} of the group at i.
// It panics if child is not one of them: the path is built by descending
// through those {particles}, so a child that is not there is a walk bug and not
// an input the caller could have caused.
func (m *Matcher) slotOf(i, child int) int {
	for k, c := range m.nodes[i].children {
		if c == child {
			return k
		}
	}
	panic("xsd: Matcher.slotOf: path node is not a member of its parent group")
}
