package xsd

import (
	"strconv"

	"github.com/kud360/goxsd8/xsderr"
)

// ruleCosElementConsistent is Element Declarations Consistent (Structures
// §3.8.6.3, id="cos-element-consistent"): if a model group's {particles}
// contains, directly, indirectly, or ·implicitly·, two or more element
// declarations with the same expanded name, their declared {type definition}s
// must be one named top-level definition and their {type table}s must agree
// (clauses 1-4); and if a strict or lax wildcard in the group — or in the
// containing complex type's {open content} — ·matches· that name and a top-level
// declaration carries it, that declaration's {type table} must agree too.
const ruleCosElementConsistent xsderr.Rule = "cos-element-consistent"

// This file decides cos-element-consistent. It is a collect-then-group pass, not
// the pairwise walk cos-nonambig runs: the constraint is about the SET of
// declarations sharing an expanded name, so the declarations are gathered first
// and then partitioned.
//
// Scope is every Model Group at every depth, per §3.8.6's unqualified chapeau,
// with two consequences the code depends on. Clause 2.1's wildcard is scoped to
// the group being checked, so a nested group is not discharged by checking its
// parent; and clause 2.2's {open content} belongs to the complex type whose
// content model the group is the {term} of, so the containing complex type is
// threaded as an explicit parameter (matching wildcardadmit.go's pattern) and the
// {open content} is passed only at that one level. A Model Group Definition's
// {model group} is checked in its own right as well, whether or not any <group
// ref> points at it.
//
// COMPONENT IDENTITY is load-bearing here and is why every gathered declaration
// carries a key. The constraint speaks of "two or more element declarations",
// meaning two components — not one component reached twice. <element ref="a"/>
// twice in a sequence, or one <group ref="g"/> written twice where g declares an
// element inline, contains ONE declaration, and clause 1's "non-absent {name}"
// requirement must not fire on it just because its type is anonymous. Reaching a
// declaration twice is therefore de-duplicated by key before the clauses run.
//
// Nothing is memoized on any component: the gathered sets are finalize-scoped
// intermediates discarded when the check returns (STYLE D3), and one ModelGroup
// value reachable from two content models could not be memoized soundly anyway.

// containedElement is one element declaration a model group's {particles}
// ·contains·, paired with a key identifying the COMPONENT it was reached as.
//
// The key is not a name: two distinct local declarations may share an expanded
// name (that is exactly the case the constraint is about), while one declaration
// may be reached by several paths. A top-level declaration is keyed by its
// expanded name, which sch-props-correct clause 2 keeps unique; an inline one by
// its position under the nearest enclosing named root — the Model Group
// Definition it lies in, or the content model — so that two <group ref>s to one
// definition key its inline declarations identically.
type containedElement struct {
	decl ElementDeclaration
	key  string
}

// groupContents is what one model group ·contains·, gathered in document order:
// every element declaration reached directly, indirectly, or ·implicitly·, and
// every Wildcard {term}, one per particle occurrence, whatever its {process
// contents}. Clause 2.1 names strict and lax wildcards only, so its one
// consumer, wildcardAdmitsName, passes over the skip ones; ContainedWildcards
// reads them all.
type groupContents struct {
	elements  []containedElement
	wildcards []Wildcard
	seen      map[string]bool
}

// checkElementDeclarationsConsistent is the cos-element-consistent half of Phase
// C. It walks the compiled set in document order (STYLE D2) and returns the first
// violation.
//
// The complex types are complextypewalk.go's, not s.types: §3.8.6's chapeau
// binds the content model of an ANONYMOUS type a declaration owns exactly as it
// binds a named one's, and ranging s.types reached none of them (#438).
func (s *Schema) checkElementDeclarationsConsistent() error {
	w := complexTypeWalk{
		complexType:          s.checkContentModelElementsConsistent,
		modelGroupDefinition: s.checkModelGroupDefinitionConsistent,
	}
	return w.schema(s)
}

// checkContentModelElementsConsistent charges cos-element-consistent against one
// complex type's content model. An empty or simple {content type} holds no
// particle and so no model group for the constraint to bind.
func (s *Schema) checkContentModelElementsConsistent(c ComplexType) error {
	ec, ok := c.ContentType().(ElementContent)
	if !ok {
		return nil
	}
	// The content model's {term} is the one model group for which clause 2.2's
	// "the Model Group is the {term} of the ·content model· of some Complex Type
	// Definition CTD" holds, so {open content} is passed here and nowhere deeper.
	return s.checkTermConsistent(ec.Particle.Term(), c, ec.OpenContent, QName{}, "")
}

// checkModelGroupDefinitionConsistent charges cos-element-consistent against one
// Model Group Definition's {model group}, which is a Model Group component in
// its own right (§3.7.1), so §3.8.6 binds it whether or not a <group ref> points
// at it. It has no containing complex type and no {open content}: the zero
// ComplexType is the correct "no containing type" context, since its content
// model contains no declaration and so the sibling keyword excludes nothing —
// which is the right reading, cvc-wildcard clause 3's remaining preconditions
// (3.3-3.6) naming a containing complex type that does not exist here.
func (s *Schema) checkModelGroupDefinitionConsistent(mgd ModelGroupDefinition) error {
	return s.checkGroupConsistent(mgd.ModelGroup(), ComplexType{}, nil, mgd.Name(), "")
}

// checkTermConsistent checks the model group at a particle's {term}, if the
// {term} is one. An <element ref> or an inline declaration or wildcard is a leaf:
// the constraint binds Model Groups, not particles.
func (s *Schema) checkTermConsistent(t TermOrRef, containing ComplexType, oc *OpenContent, root QName, path string) error {
	switch t := t.(type) {
	case ResolvedTerm:
		g, ok := t.Term.(ModelGroup)
		if !ok {
			return nil
		}
		return s.checkGroupConsistent(g, containing, oc, root, path)
	case ModelGroupRef:
		mgd, ok := s.modelGroupIndex[t.Name]
		if !ok {
			return nil // a dangling <group ref> was already rejected by Phase A
		}
		// Crossing into a definition re-roots the identity path, so the same inline
		// declaration reached through two <group ref>s keys identically.
		return s.checkGroupConsistent(mgd.ModelGroup(), containing, oc, t.Name, "")
	case ElementDeclarationRef:
		return nil
	default:
		panic("xsd: checkTermConsistent: non-exhaustive TermOrRef switch")
	}
}

// checkGroupConsistent checks one model group and then every model group nested
// below it. {open content} is passed only to the group it was given for: clause
// 2.2 speaks of the content model's {term}, not of every group inside it.
func (s *Schema) checkGroupConsistent(g ModelGroup, containing ComplexType, oc *OpenContent, root QName, path string) error {
	if err := s.checkGroupElementsConsistent(g, containing, oc, root, path); err != nil {
		return err
	}
	for i, p := range g.particles {
		if err := s.checkTermConsistent(p.Term(), containing, nil, root, path+"/"+strconv.Itoa(i)); err != nil {
			return err
		}
	}
	return nil
}

// checkGroupElementsConsistent applies both halves of cos-element-consistent to
// one model group. Names are visited in first-seen document order; the by-name
// buckets are a map used only as a lookup index, never ranged, so which violation
// is reported does not depend on map iteration (STYLE D1/D2).
func (s *Schema) checkGroupElementsConsistent(g ModelGroup, containing ComplexType, oc *OpenContent, root QName, path string) error {
	var c groupContents
	s.gatherGroupContents(g, root, path, &c)
	s.gatherImplicitElements(&c)

	var order []QName
	buckets := map[QName][]ElementDeclaration{}
	for _, e := range c.elements {
		name := e.decl.Name()
		if _, ok := buckets[name]; !ok {
			order = append(order, name)
		}
		buckets[name] = append(buckets[name], e.decl)
	}
	for _, name := range order {
		if err := checkSameTypeDefinition(name, buckets[name]); err != nil {
			return err
		}
		if err := s.checkTopLevelTypeTable(name, buckets[name], c, containing, oc); err != nil {
			return err
		}
	}
	return nil
}

// checkSameTypeDefinition is the first half of cos-element-consistent, clauses
// 1-4, over the declarations sharing one expanded name. It is vacuous for a
// single declaration: the constraint's antecedent is "two or more element
// declarations with the same expanded name".
//
// The first declaration in document order is the reference every later one is
// compared against, so the reported failure is deterministic (STYLE D1).
//
// Clause 1 is decided by WHICH ARM of TypeDefinitionOrRef each declaration
// carries, not by comparing a QName against zero: only the by-name
// TypeDefinitionRef arm has a ·non-absent· {name} at all. Each of the other three
// states fails clause 1, and each for its own reason — the distinctions the old
// zero-QName encoding conflated:
//
//   - a nil slot has no {type definition} to name;
//   - an InlineTypeDefinition is anonymous by construction;
//   - a SubstitutionGroupHeadTypeRef inherits the head's ANONYMOUS type
//     (§3.3.2.1 dcl.elt.common clause 3, the anonymous-head case), which has no
//     {name} either — its Head field names an element declaration, not a type,
//     so reading it as a type name here would compare two different symbol
//     spaces.
//
// The last is reached by the same fall-through as the first two, deliberately:
// clause 1 is about the TYPE's {name}, and the inherited type genuinely has
// none.
func checkSameTypeDefinition(name QName, decls []ElementDeclaration) error {
	if len(decls) < 2 {
		return nil
	}
	typeNames := make([]QName, len(decls))
	for i, d := range decls {
		ref, byName := d.TypeDefinition().(TypeDefinitionRef)
		if !byName {
			return xsderr.New(ruleCosElementConsistent, xsderr.Loc{},
				"a content model contains two or more element declarations named %s, but one of them has an anonymous or absent declared {type definition}, and cos-element-consistent clause 1 requires every such {type definition} to have a ·non-absent· {name}", name)
		}
		typeNames[i] = ref.Name
	}
	for _, tn := range typeNames[1:] {
		// {name} and {target namespace} (clauses 2 and 3) are bundled as one QName
		// by this package's expanded-name convention, so one comparison covers both.
		if tn != typeNames[0] {
			return xsderr.New(ruleCosElementConsistent, xsderr.Loc{},
				"a content model contains element declarations named %s with different declared {type definition}s (%s and %s), but cos-element-consistent clauses 2-3 require the same {name} and {target namespace}",
				name, typeNames[0], tn)
		}
	}
	return checkTypeTablesAgree(name, decls,
		"cos-element-consistent clause 4 requires the {type table}s of same-named element declarations to be either all ·absent· or all present and ·equivalent·")
}

// checkTopLevelTypeTable is the second half of cos-element-consistent: when a
// strict or lax wildcard admits the expanded name Q — either a ·wildcard
// particle· the group contains (clause 2.1) or the containing complex type's
// {open content} wildcard (clause 2.2) — and a top-level declaration G carries Q
// (clause 3), G's {type table} must agree with those of the contained
// declarations EDS.
//
// G is folded into EDS only when it is not already one of them; the contained
// declarations were de-duplicated by component key, and G reached through an
// <element ref> IS one of them.
func (s *Schema) checkTopLevelTypeTable(name QName, eds []ElementDeclaration, c groupContents, containing ComplexType, oc *OpenContent) error {
	if len(eds) == 0 {
		return nil
	}
	if !s.wildcardAdmitsName(c, containing, oc, name) {
		return nil // clause 2 unsatisfied
	}
	g, ok := s.Element(name)
	if !ok {
		return nil // clause 3 unsatisfied
	}
	if c.seen[topLevelKey(name)] {
		return nil // G is already among EDS; clause 4 above covered it
	}
	return checkTypeTablesAgree(name, append(append([]ElementDeclaration(nil), eds...), g),
		"cos-element-consistent's second constraint requires the {type table}s of the contained declarations and of the top-level declaration of the same name to be either all ·absent· or all present and ·equivalent·")
}

// wildcardAdmitsName reports whether some strict or lax wildcard ·matches· the
// expanded name (clause 2.1's ·wildcard particles·, then clause 2.2's {open
// content} wildcard). ·match· for a wildcard is cvc-wildcard (§3.10.4.1,
// key-wc-match), so the one canonical implementation is reused rather than a
// second matcher built (STYLE T4); the containing complex type is what
// cvc-wildcard clause 3 needs to resolve the sibling keyword.
//
// Wildcards are consulted in document order, and the {open content} wildcard
// last, so the answer does not depend on which one is looked at first.
func (s *Schema) wildcardAdmitsName(c groupContents, containing ComplexType, oc *OpenContent, name QName) bool {
	for _, w := range c.wildcards {
		if w.ProcessContents() == ProcessSkip {
			continue // clause 2.1 names strict and lax only
		}
		if s.allowsElementWildcardName(w, containing, name) {
			return true // clause 2.1
		}
	}
	if oc == nil {
		return false
	}
	w := oc.Wildcard()
	if w.ProcessContents() == ProcessSkip {
		return false // clause 2.2 names strict and lax only
	}
	return s.allowsElementWildcardName(w, containing, name) // clause 2.2
}

// checkTypeTablesAgree enforces "all ·absent· or all present and ·equivalent·"
// over a set of declarations, the shape both halves of cos-element-consistent
// end in. The first declaration in document order is the reference, so the
// reported failure is deterministic (STYLE D1); reason names the clause charged.
func checkTypeTablesAgree(name QName, decls []ElementDeclaration, reason string) error {
	head, headPresent := decls[0].TypeTable()
	for _, d := range decls[1:] {
		tt, present := d.TypeTable()
		if present != headPresent {
			return xsderr.New(ruleCosElementConsistent, xsderr.Loc{},
				"element declarations named %s disagree on whether a {type table} is present, but %s", name, reason)
		}
		if !present {
			continue
		}
		if !typeTablesEquivalent(head, tt) {
			return xsderr.New(ruleCosElementConsistent, xsderr.Loc{},
				"element declarations named %s have {type table}s that are not ·equivalent·, but %s", name, reason)
		}
	}
	return nil
}

// typeTablesEquivalent is ·equivalent· for Type Tables (§3.8.6.3, key-equiv-tt):
// the {alternatives} lists have the same length with ·equivalent· CORRESPONDING
// entries — {alternatives} is an ordered list and clause 1 pairs entries by
// position, not as sets — and the {default type definition}s are ·equivalent·.
func typeTablesEquivalent(a, b TypeTable) bool {
	if len(a.alternatives) != len(b.alternatives) {
		return false
	}
	for i := range a.alternatives {
		if !typeAlternativesEquivalent(a.alternatives[i], b.alternatives[i]) {
			return false
		}
	}
	return typeAlternativesEquivalent(a.defaultTypeDefinition, b.defaultTypeDefinition)
}

// typeAlternativesEquivalent is ·equivalent· for Type Alternatives (§3.8.6.3,
// key-equiv-ta). The definition's own terms — "{test}s are true for the same set
// of input element information items" — are undecidable in general, so the spec
// gives a five-clause minimum every processor must detect and then says: "A
// processor may treat two type alternatives as non-equivalent if they do not
// satisfy the conditions just given and the processor does not detect that they
// are nonetheless equivalent." This implements exactly the five-clause minimum
// and takes that license for everything else — a legal implementation choice, not
// an unimplemented gap.
//
// Clause 5 ("the same type definition") is typeDefinitionsEquivalent, which
// answers it per ARM of the {type definition} slot. That never rejects a schema
// for reaching one declaration twice: a declaration reached twice is one
// component, de-duplicated by key before these clauses run, which is how the
// "Any Type Alternative is equivalent to itself" case is discharged
// (TypeAlternative holds slices and so is not ==-comparable, leaving component
// identity as the only sound reading of "itself").
//
// Clauses 1-4 read {test}. They presuppose a present {test}: the {default type
// definition} has none by construction (e-props-correct clause 6), and two absent
// {test}s leave clause 5 as the whole test.
func typeAlternativesEquivalent(a, b TypeAlternative) bool {
	if !typeDefinitionsEquivalent(a.typeDefinition, b.typeDefinition) {
		return false // clause 5
	}
	ta, aPresent := a.Test()
	tb, bPresent := b.Test()
	if aPresent != bPresent {
		return false
	}
	if !aPresent {
		return true // two "otherwise" alternatives naming one top-level type
	}
	if !namespaceBindingsEquivalent(ta, tb) {
		return false // clause 1
	}
	an, anPresent := ta.DefaultNamespace()
	bn, bnPresent := tb.DefaultNamespace()
	if !optionalStringsEqual(an, anPresent, bn, bnPresent) {
		return false // clause 2
	}
	au, auPresent := ta.BaseURI()
	bu, buPresent := tb.BaseURI()
	if !optionalStringsEqual(au, auPresent, bu, buPresent) {
		return false // clause 3
	}
	return ta.Expression() == tb.Expression() // clause 4
}

// typeDefinitionsEquivalent decides key-equiv-ta clause 5 — "T1.{type
// definition} and T2.{type definition} are the same type definition" — over the
// TypeDefinitionOrRef sum, per arm. It is a COMPONENT-IDENTITY test, not a
// structural one, so the arms do not all answer the same way:
//
//   - two TypeDefinitionRefs are the same component exactly when they name it,
//     since a name resolves in one {type definitions} bucket (§3.17.6.2 clause
//     1.1);
//   - two SubstitutionGroupHeadTypeRefs likewise, through the OWNER they name:
//     §3.4.6.5's no-identity Note settles that a member and its head share one
//     component, so two members of one head hold one type definition;
//   - an InlineTypeDefinition on EITHER side is never equivalent to anything,
//     itself included. Anonymous-type identity is what the spec's own change log
//     records as underspecified, and key-equiv-ta's preamble licenses exactly
//     this fallback: "A processor may treat two type alternatives as
//     non-equivalent if they do not satisfy the conditions just given and the
//     processor does not detect that they are nonetheless equivalent." Two
//     independently produced anonymous types are two components; one component
//     read through two slots is one Type Alternative reached twice, which the
//     de-duplication above has already collapsed.
//   - a nil slot names no component, so it is the same as nothing.
//
// The default arm asserts the sealed sum (STYLE T2) and is unreachable:
// typeDefinitionOrRef is unexported, so no fourth variant exists.
func typeDefinitionsEquivalent(a, b TypeDefinitionOrRef) bool {
	switch ra := a.(type) {
	case nil:
		return false
	case TypeDefinitionRef:
		rb, same := b.(TypeDefinitionRef)
		return same && ra.Name == rb.Name
	case InlineTypeDefinition:
		return false
	case SubstitutionGroupHeadTypeRef:
		rb, same := b.(SubstitutionGroupHeadTypeRef)
		return same && ra.Head == rb.Head
	default:
		panic("xsd: typeDefinitionsEquivalent: non-exhaustive TypeDefinitionOrRef switch")
	}
}

// optionalStringsEqual compares two Optional anyURI properties: they agree when
// both are ·absent· or both are present with the same value (key-equiv-ta clauses
// 2 and 3, "either are both ·absent· or have the same value").
func optionalStringsEqual(a string, aPresent bool, b string, bPresent bool) bool {
	if aPresent != bPresent {
		return false
	}
	return !aPresent || a == b
}

// namespaceBindingsEquivalent is key-equiv-ta clause 1: the two {namespace
// bindings} have the same number of entries, and every entry of one has a
// counterpart in the other with the same {prefix} and {namespace}. The property
// is a SET, so the comparison is by membership, not by position — two expressions
// whose parsers emitted the same bindings in different orders are equivalent.
func namespaceBindingsEquivalent(a, b XPathExpression) bool {
	if len(a.namespaceBindings) != len(b.namespaceBindings) {
		return false
	}
	for _, ab := range a.namespaceBindings {
		found := false
		for _, bb := range b.namespaceBindings {
			if ab == bb {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// gatherGroupContents appends everything a model group ·contains· — element
// declarations directly and indirectly, and ·wildcard particles· of every
// {process contents} — in document order. root and path carry the component
// identity of inline declarations; a <group ref> re-roots them so one
// definition's inline declarations key identically however many references
// reach them.
func (s *Schema) gatherGroupContents(g ModelGroup, root QName, path string, c *groupContents) {
	for i, p := range g.particles {
		s.gatherTermContents(p.Term(), root, path+"/"+strconv.Itoa(i), c)
	}
}

// gatherTermContents appends what one particle's {term} contributes.
func (s *Schema) gatherTermContents(t TermOrRef, root QName, path string, c *groupContents) {
	switch t := t.(type) {
	case ResolvedTerm:
		s.gatherResolvedTermContents(t.Term, root, path, c)
	case ElementDeclarationRef:
		d, ok := s.Element(t.Name)
		if !ok {
			return // a dangling <element ref> was already rejected by Phase A
		}
		c.add(d, topLevelKey(t.Name))
	case ModelGroupRef:
		mgd, ok := s.modelGroupIndex[t.Name]
		if !ok {
			return // a dangling <group ref> was already rejected by Phase A
		}
		s.gatherGroupContents(mgd.ModelGroup(), t.Name, "", c)
	default:
		panic("xsd: gatherTermContents: non-exhaustive TermOrRef switch")
	}
}

// gatherResolvedTermContents appends what an inline {term} contributes,
// exhaustively over the sealed Term sum.
func (s *Schema) gatherResolvedTermContents(t Term, root QName, path string, c *groupContents) {
	switch t := t.(type) {
	case ElementDeclaration:
		// A global declaration written inline is still the one component the
		// {element declarations} table holds, so it keys by name.
		if t.ScopeVariety() == ScopeGlobal {
			c.add(t, topLevelKey(t.Name()))
			return
		}
		c.add(t, "local:"+root.String()+path)
	case ModelGroup:
		s.gatherGroupContents(t, root, path, c)
	case Wildcard:
		c.wildcards = append(c.wildcards, t)
	default:
		panic("xsd: gatherResolvedTermContents: non-exhaustive Term switch")
	}
}

// gatherImplicitElements adds the ·implicitly contained· declarations
// (§3.8.6.3, key-impl-cont: "A list of particles implicitly contains an element
// declaration if and only if a member of the list contains that element
// declaration in its ·substitution group·").
//
// Members are enumerated from the document-order {element declarations} slice,
// never from elementIndex, and heads in the order they were gathered, so the
// result is deterministic (STYLE D2). Membership is inSubstitutionGroupOf, which
// decides cos-equiv-derived-ok-rec exactly (substitutiongroup.go), so the set
// gathered here is the spec's ·substitution group· — an implicitly contained
// declaration only ever adds obligations, and none of them is invented. A head is
// in its own group (clause 1) and so is re-offered here; add drops it as already
// gathered.
func (s *Schema) gatherImplicitElements(c *groupContents) {
	heads := append([]containedElement(nil), c.elements...)
	for _, head := range heads {
		if head.decl.ScopeVariety() != ScopeGlobal {
			continue // a local declaration heads no substitution group
		}
		for _, e := range s.elements {
			if !s.inSubstitutionGroupOf(e.Name(), head.decl.Name()) {
				continue
			}
			c.add(e, topLevelKey(e.Name()))
		}
	}
}

// add appends a declaration unless the same COMPONENT was already gathered. The
// seen map is a membership index only; order comes from the slice (STYLE D2).
func (c *groupContents) add(d ElementDeclaration, key string) {
	if c.seen[key] {
		return
	}
	if c.seen == nil {
		c.seen = map[string]bool{}
	}
	c.seen[key] = true
	c.elements = append(c.elements, containedElement{decl: d, key: key})
}

// topLevelKey is the component key of a top-level element declaration: its
// expanded name, which sch-props-correct clause 2 keeps unique among {element
// declarations}. The prefix keeps it disjoint from an inline declaration's
// position-derived key.
func topLevelKey(name QName) string {
	return "top:" + name.String()
}
