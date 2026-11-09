package parser

import (
	"fmt"
	"slices"

	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// This file implements <xs:redefine> (§4.2.4, src-redefine and src-expredef):
// including a schema document while REPLACING some of its top-level
// simpleType/complexType/group/attributeGroup definitions with new ones written
// inline, each of which may build on the definition it replaces.
//
// §4.2.4 marks the whole mechanism ·deprecated· — "may be removed from future
// versions of this specification. Schema authors are encouraged to avoid its use
// in cases where interoperability or compatibility with later versions of this
// specification are important" — and §4.2.5 says why: "existing XSD processors
// have implemented conflicting and non-interoperable interpretations of
// <redefine>". It is implemented here because §4.2.4 is still normative, not
// because it is recommended; nothing in this file is a behaviour change to
// <xs:override>, which exists to replace it.
//
// # The shape of the mechanism
//
// A <redefine> is an <include> plus a rewrite, so it enters at the DISCOVER
// phase alongside <include>/<import>/<override> and the rewrite is carried as
// DATA — a redefineSet threaded through discovery beside the effective target
// namespace and the ·override pre-processing· — exactly as parser/override.go
// carries §F.2. No tree is rewritten and no node is synthesized.
//
// Two documents meet over one redefineSet, and they read it from opposite sides:
//
//	D1, the REDEFINING document, holds the <redefine> element. Its producer
//	     maps the <redefine>'s children as top-level definitions of its own
//	     (src-expredef clause 1.2 / clause 2, §4.2.4 clause 4.1.1).
//	D2, the REDEFINED document, is the one the schemaLocation names. Its
//	     producer contributes every component it declares EXCEPT those the set
//	     names (§4.2.4 clause 4.1.2, "with the exception of those explicitly
//	     redefined"), and records each excepted declaration as the ORIGINAL a
//	     self-reference in D1 resolves to.
//
// # Self-reference: the two-component pairing (src-expredef)
//
// A redefining <simpleType>/<complexType> corresponds to TWO components: a
// hidden one with {name} ·absent· that is the ORIGINAL (clause 1.1), and the
// visible redefining one whose {base type definition} is that hidden original
// (clause 1.2). "This pairing ensures the coherence constraints on type
// definitions are respected, while at the same time achieving the desired
// effect, namely that references to names of redefined components in both the
// <redefine>ing and <redefine>d schema documents ·resolve· to the redefined
// component". A redefining <group>/<attributeGroup> corresponds to a SINGLE
// component, but "if and when a self-reference based on a ref [attribute] …
// is ·resolved·, a component which corresponds to the top-level definition item
// of that name and the appropriate kind in S2 is used" (clause 2).
//
// Getting that wrong produces a FALSE CIRCULARITY — a type or group appearing to
// derive from or contain itself — so the FOUR resolution sites that can meet a
// self-reference each consult the set first: resolveBase (a simple type's base=)
// and redefinedComplexBase (a complex type's, both produce.go),
// produceGroupRefParticle (a <group ref>) and attributeGroupMember (an
// <attributeGroup ref>, both produce_complex.go).
//
// # The complex-type pairing (#505)
//
// A redefining <complexType> is a NAMED component that OWNS its
// {name}-·absent· base outright, which xsd.ComplexType's {base type definition}
// slot expresses as the InlineTypeDefinition arm of a TypeDefinitionOrRef and
// xsd.NewComplexTypeOwningBase is the only entry point for. The producer mints
// ONE xsd.ComponentID per redefinition (namedComplexTypeIdentity, produce.go),
// builds clause 1.1's original from the REDEFINED document's own declaration and
// under that document's producer, and threads the identity into both halves so
// the original's {context} points back at the redefining component; the
// constructor checks the two agree.
//
// The pairing is built at buildComplexType, not here, because the redefining
// declaration is reachable by NAME as well: prescanRedefine registers it under
// its own expanded name, so a reference from either document arrives through
// resolveBaseType. One decision point means one component per DECLARATION, and
// the declaration that name reaches is the redefining one; together those make
// src-expredef's note ("references … in both the <redefine>ing and <redefine>d
// schema documents ·resolve· to the redefined component") hold.
//
// src-redefine clause 5 is charged BEFORE the pairing is attempted, so a
// redefining complex type that does not derive from itself is rejected for that
// rule rather than for a missing original.
//
// # The chain (#585)
//
// D2 may itself <redefine> a D3 for the SAME (kind, name). Clause 4.1.1 makes
// D2's redefining child a top-level definition of D2, so it is what D1's clause
// 1.1 pairs with — and clause 4.1.2, applied at D2's level, keeps D3's own
// declaration out of D2's component set entirely, so wiring D1 straight to D3
// would drop D2's redefinition from the hierarchy D1 sees. The pairing therefore
// nests: D1's hidden original IS D2's redefining declaration, built anonymously,
// owning D3's hidden original in turn. Each level mints its own identity for the
// edge below it, so the anonymous levels stay distinct components
// (redefineOriginalComplexType, produce_complex.go).
//
// Composing it needs no new resolution site. Both type-side sites already ask the
// producer of the document the declaration came from — resolveBase recursing
// through src.owner, and redefinedComplexBase through src.owner.produceComplexType
// — so a declaration that is itself redefining meets its OWN set there. What was
// missing was the recording: prescanRedefine now records a chained redefinition as
// the outer set's original, which chainedOriginal gates.

// redefineEntry is one non-<annotation> child of a <redefine> element paired
// with the key it redefines on. Entries are kept as a SLICE in document order,
// never as a bare map, because that order is the order the redefining components
// enter the builder and so reaches the user through the component set and
// through duplicate reports (STYLE D2).
type redefineEntry struct {
	key  componentKey
	elem *Element
}

// redefineSet is the ·redefinition· one <redefine> element declares: which
// top-level definitions of the document it names are replaced, by what, and —
// once that document has been pre-scanned — what each replaced definition was.
//
// The nil *redefineSet is the identity: an EMPTY <redefine> (no children, or
// <annotation> only) redefines nothing and is a plain <include>, which is also
// why src-redefine clause 1 does not require its schemaLocation to resolve.
type redefineSet struct {
	// el is the <redefine> element this set was read from. It is the set's
	// identity for the producer of the document that CONTAINS it, which looks its
	// own <redefine> children up by element rather than by position.
	el *Element

	// entries are the redefining declarations in document order (see
	// redefineEntry). Two entries may share a key: two same-named children of one
	// <redefine> correspond to two components with one expanded name, which
	// Finalize rejects under sch-props-correct (§3.17.6.1) clause 2 — the rule
	// that genuinely governs it — so both are produced rather than one being
	// discarded here.
	entries []redefineEntry

	// index is the membership set the REDEFINED document's producer consults to
	// decide which of its own top-level declarations §4.2.4 clause 4.1.2 excepts.
	// It is derived from entries at construction and never ranged to produce
	// output (STYLE D2/D3).
	index map[componentKey]struct{}

	// byElem is the reverse of index, derived from entries at construction: it
	// answers "is THIS element a redefining declaration, and of what", which is
	// what the three self-reference resolution sites ask after walking up from a
	// base=/ref= to its containing declaration.
	//
	// It has a SECOND writer, recordSubstitute, for the declaration §F.2 clause 1
	// substitutes for a redefining child — the one element that is a redefining
	// declaration of this set without being a child of its <redefine>.
	byElem map[*Element]componentKey

	// originals holds, per redefined key, the top-level declaration of the
	// REDEFINED document that the redefining one replaces — the component
	// src-expredef clause 1.1 makes the hidden {name}-·absent· base and clause 2
	// makes the target of a group/attributeGroup self-reference. §4.2.4 clause
	// 4.1.1 counts a <redefine> child of the redefined document among its
	// top-level definitions, so a CHAINED redefinition is recorded here too (see
	// producer.chainedOriginal).
	//
	// It is the ONE field filled after construction, by the redefined document's
	// pre-scan (producer.prescan and producer.prescanRedefine): it is not knowable
	// when the set is read, since that document has not been fetched yet. Filling
	// it is how the two producers meet, and it is written exactly once per key,
	// from a single document-order pass over one document.
	originals map[componentKey]typeSource
}

// redefinitions are the ·redefinition·s in force over ONE discovered document —
// one per reading of it the assembly merged into that discovery, in discovery
// order, a nil member being a reading that reached the document plainly. The
// empty slice is the [Produce] path, which follows no ·inter-schema-document
// reference· and so has no reading of anything.
//
// It is a slice because a document reached under two readings is composed once
// (docKey, parser/parse.go): §4.2.4 clause 4.1.2 subtracts per READING, so a
// definition is contributed unless every reading subtracts it, while each
// reading's own set still needs the original it subtracted.
type redefinitions []*redefineSet

// newRedefineSet reads the redefining declarations a <redefine> element declares
// (§4.2.4's XML Representation Summary). It returns nil for one that declares
// none, which makes an empty <redefine> a plain <include> in every particular:
// same load-once key, and no src-redefine clause 1 obligation on its
// schemaLocation.
//
// The content model is (annotation | (simpleType | complexType | group |
// attributeGroup))* — narrower than <override>'s, which also admits <element>,
// <attribute> and <notation>. A child outside it is a grammar fault with no
// dedicated Schema Representation Constraint (src-redefine's preamble
// incorporates the schema for schema documents by reference without restating
// it), reported as a plain error like an <include> with no schemaLocation, never
// silently ignored: ignoring it would drop a declaration the author wrote.
//
// A child INSIDE that content model is read against the grammar type its kind
// has here, which is the top level's own: §4.2.4's model reaches the global
// <group> and <attributeGroup> element declarations through xs:redefinable
// (xmlschema11-1.md:4465), so a redefining <group> is an xs:namedGroup and a
// redefining <attributeGroup> an xs:namedAttributeGroup, prohibited attributes
// and all. rejectProhibitedAttrs is the one encoding of those lists (STYLE T4)
// and runs BEFORE the name attribute is read, so <group ref="tns:G"/> — which
// writes no name — is reported for the ref it may not carry rather than for the
// name that ref displaced, and before produceRedefinition can charge
// src-expredef over the same child for naming no original.
//
// The child it charges is the one written in the <redefine> itself. A declaration
// §F.2 clause 1 SUBSTITUTES for that child is charged the same guard at
// producer.prescanRedefine, the only place the substitution is known: assembly.redefine
// is deliberately not passed the override set (parse.go).
func newRedefineSet(el *Element) (*redefineSet, error) {
	var entries []redefineEntry
	for _, child := range el.Children() {
		c, ok := child.(*Element)
		if !ok || c.Name().Space() != xsd.XMLSchemaNS {
			// Foreign-namespace markup is admitted only inside <annotation> (§A), so
			// anything here is already outside the content model; it is skipped
			// rather than charged, since it corresponds to no component either way.
			continue
		}
		if isXSD(c, "annotation") {
			continue
		}
		if !redefinableKind(c.Name().Local()) {
			return nil, fmt.Errorf("parser: <redefine> at %s has a <%s> child at %s, but §4.2.4's content model admits only (annotation | (simpleType | complexType | group | attributeGroup)) — <element>, <attribute> and <notation> are <override>'s (§4.2.5), not <redefine>'s", el.Loc(), c.Name().Local(), c.Loc())
		}
		if err := rejectProhibitedAttrs(c, formRedefining); err != nil {
			return nil, err
		}
		name, ok := c.Attr("name")
		if !ok {
			// The production is the top level's own, for the reason this function's
			// doc gives: xs:redefinable routes the child to the SAME global element
			// declaration, so topLevelGrammar (produce.go) answers for both positions.
			return nil, fmt.Errorf("parser: the <%s> child of <redefine> at %s has no name attribute, which the schema for schema documents requires (%s declares name use=\"required\" with type xs:NCName) and which src-expredef needs in order to pair it with a top-level definition of the redefined schema document", c.Name().Local(), c.Loc(), topLevelGrammar(c.Name().Local()))
		}
		entries = append(entries, redefineEntry{key: componentKey{kind: c.Name().Local(), name: name}, elem: c})
	}
	return buildRedefineSet(el, entries), nil
}

// buildRedefineSet completes a redefineSet from its document-ordered entries,
// deriving both indexes from them so neither is a second encoding of the same
// fact (STYLE D3). An empty entry list is the nil set.
func buildRedefineSet(el *Element, entries []redefineEntry) *redefineSet {
	if len(entries) == 0 {
		return nil
	}
	index := make(map[componentKey]struct{}, len(entries))
	byElem := make(map[*Element]componentKey, len(entries))
	for _, e := range entries {
		index[e.key] = struct{}{}
		byElem[e.elem] = e.key
	}
	return &redefineSet{
		el:        el,
		entries:   entries,
		index:     index,
		byElem:    byElem,
		originals: make(map[componentKey]typeSource, len(entries)),
	}
}

// mustResolve reports whether src-redefine clause 1 obliges this set's
// schemaLocation to resolve: "If there are any element information items among
// the [children] other than <annotation> then the ·actual value· of the
// schemaLocation [attribute] must successfully resolve". A non-nil set is
// exactly that condition, and it is what makes <redefine> differ from
// <include>, whose src-include clause 2.4 states the opposite in as many words
// ("It is not an error … to fail to resolve at all").
func (s *redefineSet) mustResolve() bool {
	return s != nil
}

// excepts reports whether el, a top-level declaration of the REDEFINED document,
// is one this set replaces — §4.2.4 clause 4.1.2's "with the exception of those
// explicitly redefined". Under this reading such a declaration contributes no
// component of its own; it survives as the hidden original a self-reference
// resolves to.
func (s *redefineSet) excepts(el *Element) bool {
	key, ok := declarationKey(el)
	if !ok {
		return false
	}
	return s.replaces(key)
}

// replaces reports whether this set replaces the top-level definition key
// names. The nil set — a plain reading, or an empty <redefine> — replaces none.
func (s *redefineSet) replaces(key componentKey) bool {
	if s == nil {
		return false
	}
	_, redefined := s.index[key]
	return redefined
}

// excepts reports whether EVERY reading of this document excepts el, which is
// the only case in which no reading contributes it: the readings are merged into
// one composition of the document (docKey, parser/parse.go), and §4.2.4 clause
// 4.1.2 subtracts a definition from the reading whose <redefine> names it alone.
// A document with no reading at all — [Produce] — excepts nothing.
func (rs redefinitions) excepts(el *Element) bool {
	if len(rs) == 0 {
		return false
	}
	for _, s := range rs {
		if !s.excepts(el) {
			return false
		}
	}
	return true
}

// recordOriginals notes src, the top-level declaration of this document that key
// names, as the original of EVERY reading whose <redefine> replaces key.
//
// It is called for every top-level declaration, not only for one that excepts
// reports no reading contributes: a reading needs the original it is paired with
// (src-expredef clauses 1.1 and 2) whether or not some OTHER reading of the same
// document still contributes that declaration under its own name. Withholding it
// there would charge src-expredef's closing requirement against a redefinition
// whose original is present (produceRedefinition).
func (rs redefinitions) recordOriginals(key componentKey, src typeSource) error {
	for _, s := range rs {
		if !s.replaces(key) {
			continue
		}
		if err := s.recordOriginal(key, src); err != nil {
			return err
		}
	}
	return nil
}

// recordOriginal notes the redefined document's own declaration for key, which
// src-expredef clause 1.1 makes the hidden {name}-·absent· base of the redefining
// type and clause 2 the target of a group/attributeGroup self-reference.
//
// TWO writers reach it, through redefinitions.recordOriginals, both from the
// redefined document's own pre-scan and both in that one document-order pass over
// that document's <schema> children: prescan, for a top-level declaration, and
// prescanRedefine, for the redefining child of a NESTED <redefine> that §4.2.4
// clause 4.1.1 makes a top-level definition of this document too (a chained
// redefine, #585). A document carrying BOTH for one key declares two top-level definitions
// of one expanded name, which sch-props-correct (§3.17.6.1) clause 2 forbids, so
// the SECOND write is rejected instead of clobbering the first.
//
// The rejection is charged here rather than at finalize because neither
// declaration ever becomes a named component: clause 4.1.2 excepts both from the
// components this document contributes, so indexByName (xsd/schema.go) — which
// charges the same clause for every pair that does reach the by-name symbol
// tables — never sees them. The pair is nonetheless two members of one property
// of schema(D2), the conforming schema src-redefine clause 4.1.1 requires the
// redefined document correspond to.
//
// The charge names the LATER declaration in document order and cites the first's
// location, the way indexByName's does. Document order is the order both writers
// walk, so no ordering work is needed (STYLE D2). ONE writer per key is the valid
// chained redefine and stays valid.
func (s *redefineSet) recordOriginal(key componentKey, src typeSource) error {
	if first, dup := s.originals[key]; dup {
		name := xsd.QName{Space: src.owner.target, Local: key.name}
		return xsderr.New(ruleSchPropsCorrect, src.elem.Loc(),
			"the <redefine>d document repeats the expanded name %s among its top-level <%s> definitions (first declared at %s), but sch-props-correct clause 2 forbids two components of the same kind sharing an expanded name", name, key.kind, first.elem.Loc())
	}
	s.originals[key] = src
	return nil
}

// recordSubstitute indexes decl — the declaration §F.2 clause 1 puts in the
// place of the redefining child this set holds under key — as a redefining
// declaration of this set in its own right, so a self-reference written inside it
// resolves to the original in S2 (src-expredef clause 2) exactly as the child's
// own would have.
//
// It is what stands in for the re-parenting §F.2's normative stylesheet performs
// and this parser does not: the `xs:schema | xs:redefine` template COPIES the
// <redefine> wrapper with its own attributes — hence its own schemaLocation, and
// so its own S2 — and puts the substitute "in the location corresponding to E2's
// place in D2", which is inside that copy. No tree is rewritten here (see
// parser/override.go), so the substitute stays physically parented under the
// <override> that declares it, where the walk up from a base=/ref= cannot reach
// the <redefine>. This index is where the spec's position is recorded instead.
//
// It is byElem's SECOND writer, and the only one that runs after construction:
// the ·override pre-processing· in force over the redefining document is not
// knowable when the set is read from its <redefine> element, since that document
// has not been reached yet. It is called from producer.prescanRedefine, and EVERY
// producer's pre-scan runs before ANY producer's run (assembly.compile), so the
// index is complete before the first self-reference is resolved — the same
// ordering originals depends on.
//
// The key is the REPLACED child's, never re-derived from decl: §F.2 clause 1
// matches on (element type, name), so the two agree by construction and deriving
// it twice would encode one fact twice (STYLE D3).
//
// One substitute can stand in for the children of TWO <redefine> elements of one
// document, which redefinitionOf then answers with the first of them in document
// order. That document declares two top-level definitions of one expanded name
// whichever it answers with, and finalize rejects it under sch-props-correct
// (§3.17.6.1) clause 2.
func (s *redefineSet) recordSubstitute(key componentKey, decl *Element) {
	s.byElem[decl] = key
}

// declarationKey identifies a top-level source declaration by the pair
// <redefine> matches on — the element type and the ·actual value· of its name
// attribute — the same key §F.2 clause 1 uses for <override> (componentKey).
func declarationKey(el *Element) (componentKey, bool) {
	if el.Name().Space() != xsd.XMLSchemaNS {
		return componentKey{}, false
	}
	name, ok := el.Attr("name")
	if !ok {
		return componentKey{}, false
	}
	return componentKey{kind: el.Name().Local(), name: name}, true
}

// chainedOriginal reports whether e, one of THIS document's own redefining
// declarations, is itself excepted by every ·redefinition· in force over this
// document — a CHAINED <redefine>, where D1 redefines D2 and D2 redefines D3 for
// one (kind, name). §4.2.4 clause 4.1.1 makes e a top-level definition of this
// document, so it is what D1's src-expredef clause 1.1 pairs with and what clause
// 4.1.2 excepts from the components this document contributes.
func (p *producer) chainedOriginal(e redefineEntry) bool {
	return chainedKind(e.key.kind) && p.rd.excepts(e.elem)
}

// chainedKind reports whether a redefining declaration of this element type
// composes when it is itself redefined.
//
// GAP(xsd): only the two kinds src-expredef clause 1 PAIRS compose — <simpleType>
// and <complexType>. A chained <group>/<attributeGroup>, whose clause 2 is a
// single-component substitution, is still refused on a schema §4.2.4 makes valid.
// What withholds it is no longer that src-redefine clauses 6.2.2 and 7.2.2 are
// fail-open — both are charged at finalize now (#503, #504) — but that a chained
// redefining declaration contributes NO component, while both clauses are charged
// over a pairing that adds one (xsd.SchemaBuilder.AddRedefiningModelGroup and its
// attributeGroup twin fuse the component and the obligation into one call). So on
// a middle level of a chain there is nothing to fuse the obligation to, and
// composing the chain today would ACCEPT the two suite cases in that shape —
// MS-Schema2006-07-15/schL10 and /schM5 are each invalid at the MIDDLE level, on
// 6.2.2 and 7.2.2 respectively, and satisfy their own clause at the outer level.
// Measured by collapsing this function and running the fixture, not predicted.
// #744 owns the retirement, and needs an xsd entry point for both kinds.
//
// The direction is fail-CLOSED, and the value withheld for the other two kinds —
// the outer set's originals entry — has exactly three readers, all of which see
// the miss: produceRedefinition REJECTS on it, charging src-expredef's closing
// requirement; redefinedGroupOriginal and redefinedAttributeGroupOriginal (through
// originalFor) would answer "not a self-reference", and neither runs, because that
// rejection precedes them.
func chainedKind(kind string) bool {
	switch kind {
	case "simpleType", "complexType":
		return true
	}
	return false
}

// redefinableKind reports whether local is one of the four element types
// §4.2.4's content model admits as a redefining declaration. It is deliberately
// NARROWER than overridableKind: <element>, <attribute> and <notation> are
// <override>'s three extra kinds and are the load-bearing structural difference
// between the two mechanisms.
func redefinableKind(local string) bool {
	switch local {
	case "simpleType", "complexType", "group", "attributeGroup":
		return true
	}
	return false
}

// redefineSetOf returns the set read from el, one of this document's own
// <redefine> children, or nil when the assembly never followed it. Nil is the
// [Produce] path: a single-document producer dereferences no
// ·inter-schema-document reference· at all, so its <redefine> children name no
// document and redefine nothing.
//
// The scan is over a SLICE in document order, so the answer does not depend on
// map iteration (STYLE D2).
func (p *producer) redefineSetOf(el *Element) *redefineSet {
	for _, rs := range p.redefines {
		if rs.el == el {
			return rs
		}
	}
	return nil
}

// redefinitionOf reports whether el is a redefining declaration of one of this
// document's own followed <redefine> elements, and under which key. It is the
// question every self-reference site asks after walking up from a base=/ref= to
// the declaration that contains it.
func (p *producer) redefinitionOf(el *Element) (*redefineSet, componentKey, bool) {
	for _, rs := range p.redefines {
		if key, ok := rs.byElem[el]; ok {
			return rs, key, true
		}
	}
	return nil, componentKey{}, false
}

// originalFor returns the redefined document's own declaration behind decl — the
// component src-expredef pairs the redefining one with — provided decl really is
// a redefining declaration of one of the kinds given and its own expanded name
// is qn, the name the reference wrote.
//
// A miss on any of the three conditions falls through to ordinary resolution,
// which is the correct outcome: a reference that is not the redefining
// declaration's own name is an ordinary reference and resolves to the visible
// (redefined) component, per src-expredef's note.
func (p *producer) originalFor(decl *Element, qn xsd.QName, kinds ...string) (typeSource, bool) {
	rs, key, ok := p.redefinitionOf(decl)
	if !ok {
		return typeSource{}, false
	}
	if !slices.Contains(kinds, key.kind) {
		return typeSource{}, false
	}
	if (xsd.QName{Space: p.target, Local: key.name}) != qn {
		return typeSource{}, false
	}
	src, recorded := rs.originals[key]
	return src, recorded
}

// enterOriginal marks decl — the <redefine>d document's declaration that
// src-expredef clause 1.1 pairs a redefining declaration with — as being built,
// and reports whether it was not already being built.
//
// A decl already on that stack closes a <redefine> CYCLE over one (kind, name):
// §4.2.4 clause 4.1.1 makes each level's redefining child the "top-level
// definition item of that name and kind in the <redefine>d schema document" the
// next level pairs with, so a cycle of <redefine>s over one name makes the
// pairing's own base chain re-enter itself. No hop of that chain passes through
// a named build: redefinedComplexBase goes DIRECT to produceComplexType and
// resolveBase to constructSimpleType, since buildComplexType's and
// buildSimpleType's memos are keyed by the declaration mapped and this one maps
// to two components — the ordinary named one and clause 1.1's original — that a
// single entry cannot hold (see redefinedComplexBase). buildComplexType's on-stack
// sentinel, the only other guard that could bound this chain, therefore never
// sees a hop of it. Its two callers charge the acyclicity rule for the kind they
// build.
//
// Every caller must pair it with leaveOriginal: the same declaration is a
// legitimate original of two DIFFERENT redefinitions, one after the other, and
// only a re-entry while it is still being built is the cycle.
func (p *producer) enterOriginal(decl *Element) bool {
	if _, building := p.symbols.redefineOriginals[decl]; building {
		return false
	}
	p.symbols.redefineOriginals[decl] = struct{}{}
	return true
}

// leaveOriginal unmarks decl once the original built from it is finished (see
// enterOriginal).
func (p *producer) leaveOriginal(decl *Element) {
	delete(p.symbols.redefineOriginals, decl)
}

// redefinedTypeBase returns the ORIGINAL type definition a base= written at at —
// a <restriction> or <extension> — names, when that base is the redefining
// type's own expanded name (src-expredef clause 1.1). ok is false for every
// other base=, which then resolves ordinarily.
func (p *producer) redefinedTypeBase(at *Element, qn xsd.QName) (typeSource, bool) {
	decl := redefinedTypeDeclaration(at)
	if decl == nil {
		return typeSource{}, false
	}
	return p.originalFor(decl, qn, "simpleType", "complexType")
}

// redefinedTypeDeclaration returns the <simpleType>/<complexType> whose OWN
// derivation the <restriction>/<extension> at states, at exactly the depth
// src-redefine clause 5 words: a <restriction> "among [a <simpleType>'s]
// children", a <restriction> or <extension> "among [a <complexType>'s]
// grand-children". A derivation nested deeper — inside an inline anonymous type,
// say — is not the redefining type's own, so a base= there names the VISIBLE
// (redefined) component like any other reference and is genuinely circular if it
// names the type it sits in.
func redefinedTypeDeclaration(at *Element) *Element {
	parent := at.parent
	if parent == nil {
		return nil
	}
	if isXSD(parent, "simpleType") {
		return parent
	}
	grand := parent.parent
	if grand != nil && isXSD(grand, "complexType") {
		return grand
	}
	return nil
}

// redefinedGroupOriginal returns the ORIGINAL model group definition a
// <group ref> at at resolves to under src-expredef clause 2: at sits inside a
// redefining <group> and its ref= names that group's own expanded name.
//
// Clause 2's resolution rule carries NO "<element> ancestor" condition — "if and
// when a self-reference based on a ref [attribute] whose ·actual value· is the
// same as the item's name plus target namespace is ·resolved·, a component which
// corresponds to the top-level definition item of that name and the appropriate
// kind in S2 is used", with no qualification. The <element>-ancestor exclusion
// belongs to src-redefine clause 6.1 alone, where it selects which self-references
// are COUNTED for 6.1.1/6.1.2 (see selfReferences); it does not change what any
// of them resolves to. Conflating the two would leave a self-reference under a
// local element resolving to the redefinition and reported as a circular
// <group ref> graph.
func (p *producer) redefinedGroupOriginal(at *Element, qn xsd.QName) (typeSource, bool) {
	decl := redefinedContainer(at, "group")
	if decl == nil {
		return typeSource{}, false
	}
	return p.originalFor(decl, qn, "group")
}

// redefinedAttributeGroupOriginal is redefinedGroupOriginal's attributeGroup
// twin (src-expredef clause 2 again). src-redefine clause 7.1 carries NEITHER of
// clause 6.1's two extra conditions — no minOccurs/maxOccurs constraint and no
// "<element> ancestor" exclusion — a verified asymmetry, not an omission, so
// neither is ported across to the counting either (see
// checkRedefinedAttributeGroup).
func (p *producer) redefinedAttributeGroupOriginal(at *Element, qn xsd.QName) (typeSource, bool) {
	decl := redefinedContainer(at, "attributeGroup")
	if decl == nil {
		return typeSource{}, false
	}
	return p.originalFor(decl, qn, "attributeGroup")
}

// redefinedContainer walks up from at to the nearest ancestor element of element
// type kind — the definition a nested ref= could be a self-reference of. A ref=
// with no such ancestor (a <group ref> in a redefining <complexType>, say) is an
// ordinary reference to the visible redefined component, not a self-reference.
func redefinedContainer(at *Element, kind string) *Element {
	for cur := at.parent; cur != nil; cur = cur.parent {
		if isXSD(cur, kind) {
			return cur
		}
	}
	return nil
}

// prescanRedefine registers the redefining declarations of one <redefine> child
// of THIS document in the assembly-wide symbol table, under their own expanded
// names and this producer as owner — they are children of this document, so
// their local declarations take this document's target namespace and
// schema-level defaults (§3.3.2.3 dcl.elt.local) and their unqualified
// references this document's §F.1 coercion.
//
// Registering them here is what makes references resolve the way src-expredef's
// note requires: "references to names of redefined components in both the
// <redefine>ing and <redefine>d schema documents ·resolve· to the redefined
// component". The redefined document's own pre-scan withholds the names it is
// excepted of (§4.2.4 clause 4.1.2), so there is no contest between the two.
//
// This function withholds on the same terms and for the same clause: a redefining
// declaration that some OUTER document redefines in turn is recorded as THAT
// document's original instead of being registered under its own name (see
// chainedOriginal), so a chain of any depth leaves exactly one visible component
// per expanded name — the outermost redefinition's.
//
// Each child is first passed through the ·override pre-processing· in force over
// this document: §F.2's governing sentence scopes its case analysis to "each
// element information item E2 in the [children] of any <schema> OR <redefine>
// element information item within D2", so clause 1 substitutes for a <redefine>
// child exactly as it does for a <schema> child. A substitute is recorded as a
// redefining declaration of the set in its own right (recordSubstitute), which is
// what keeps its self-reference resolving into S2.
//
// A SUBSTITUTE is charged rejectProhibitedAttrs here, and only a substitute. §F.2
// clause 1 puts "a copy of E1" — the <override> child, attributes and all — in
// the redefining child's place, and §4.2.5 src-override clause 3's closing Note
// binds the schema for schema documents to the transformed document: "It is Dold′
// and not Dold, which is required to correspond to a conforming schema"
// (xmlschema11-1.md:4171). So it is the substitute's own attributes that
// xs:namedGroup and xs:namedAttributeGroup prohibit ref, minOccurs and maxOccurs
// among. newRedefineSet charges the child written in the <redefine> and cannot
// reach this element, which is parented under the <override>. The two charges are
// ADDITIVE, not alternates: newRedefineSet runs over every <redefine> child
// unconditionally and before any prescan, so a substituted position is charged
// twice, the original child there and the substitute here, and an original
// carrying a prohibited attribute is rejected for it even though clause 1 keeps
// that original out of Dold′.
//
// Charging it at PRE-SCAN is what keeps the ordering the guard exists for: EVERY
// document's prescan runs before ANY document's run (produce.go), so the
// prohibited attribute is reported ahead of the src-expredef pairing miss
// produceRedefinition would otherwise charge first over the same entry.
func (p *producer) prescanRedefine(el *Element) error {
	rs := p.redefineSetOf(el)
	if rs == nil {
		return nil
	}
	for _, e := range rs.entries {
		decl := p.ov.replacement(e.elem)
		if decl != e.elem {
			if err := rejectProhibitedAttrs(decl, formRedefining); err != nil {
				return err
			}
			rs.recordSubstitute(e.key, decl)
		}
		p.prescanIdentityConstraints(decl)
		if chainedKind(e.key.kind) {
			// A CHAINED redefine: some outer document redefines THIS one for the same
			// (kind, name), and §4.2.4 clause 4.1.1 makes this redefining child a
			// top-level definition of this document — so it is the "top-level
			// definition item … in the <redefine>d schema document" src-expredef's
			// closing requirement demands, and clause 1.1's original the outer
			// redefinition is paired with. It is recorded for every outer reading that
			// names it, exactly as prescan records an ordinary top-level declaration.
			if err := p.rd.recordOriginals(e.key, typeSource{elem: decl, owner: p}); err != nil {
				return err
			}
		}
		if p.chainedOriginal(e) {
			// Clause 4.1.2 excepts it from the components this document contributes —
			// so it is NOT registered under its own name, the outer redefinition owning
			// that name — but only when EVERY reading excepts it, since a reading that
			// does not redefine this name still contributes the definition.
			continue
		}
		qn := xsd.QName{Space: p.target, Local: e.key.name}
		src := typeSource{elem: decl, owner: p}
		// An <attributeGroup> has no source index to enter: an <attributeGroup
		// ref> resolves against xsd's {attribute group definitions} at finalize,
		// which produceRedefinition's own build of this declaration enters.
		switch e.key.kind {
		case "simpleType":
			p.symbols.simpleTypes[qn] = src
		case "complexType":
			p.symbols.complexTypes[qn] = src
		case "group":
			p.symbols.modelGroups[qn] = src
		}
	}
	return nil
}

// produceRedefine maps the children of one <redefine> child of this document
// into components, in document order (§4.2.4 clause 4.1.1: "the schema
// corresponding to D1 includes … definitions or declarations corresponding to
// the appropriate members of its own [children]"). The components of the
// redefined document reach the builder through ITS producer, minus the ones this
// set excepts.
func (p *producer) produceRedefine(el *Element) error {
	rs := p.redefineSetOf(el)
	if rs == nil {
		// [Produce] follows no ·inter-schema-document reference· (§4.2.1), so its
		// <redefine> children name no document; there is no original to pair a
		// redefinition with, and the whole element is skipped exactly as <include>,
		// <import> and <override> are.
		return nil
	}
	for _, e := range rs.entries {
		if err := p.produceRedefinition(rs, e); err != nil {
			return err
		}
	}
	return nil
}

// produceRedefinition maps ONE redefining declaration. It charges, in order, the
// name's own lexical validity (declarationName, cvc-datatype-valid: a redefining
// declaration mints a top-level {name} like any other, and it is an xs:NCName
// like any other), then the closing requirement of src-expredef ("In all cases
// there must be a top-level definition item of the appropriate name and kind in
// the <redefine>d schema document"), then the src-redefine clause that
// constrains this kind's own shape (5 for a type, 6 for a group, 7 for an
// attribute group), and only then builds. The lexical fault comes first because
// both structural clauses presuppose a name that could address a component at
// all (PRINCIPLES 19).
//
// The closing requirement precedes the shape clause because every later step
// depends on the original existing: clause 5's self-derivation would
// otherwise resolve to the redefinition itself and be reported as a
// circularity, which is the wrong verdict for a redefinition of a name the
// redefined document never declared. It also subsumes src-redefine clauses
// 6.2.1 and 7.2.1, which say the same thing for the no-self-reference branch
// ("its own name attribute plus target namespace successfully ·resolves· to a
// model group definition in S2").
//
// Its message says "was recorded from" rather than "the redefined schema
// document declares no": the miss it reports is on rs.originals, which is filled
// by the redefined document's pre-scan, so it says what this producer knows
// rather than asserting what the other document contains. Every reading of that
// document records the originals of every set that reaches it
// (redefinitions.recordOriginals), including a set whose <redefine> landed on a
// document another directive had already composed — the dedup path that once
// left the second set unfilled and charged this rule against a declaration that
// was there (#1349).
func (p *producer) produceRedefinition(rs *redefineSet, e redefineEntry) error {
	qn, err := declarationName(e.elem, p.target)
	if err != nil {
		return err
	}
	if _, ok := rs.originals[e.key]; !ok {
		return xsderr.New(ruleSrcExpRedefine, e.elem.Loc(),
			"<redefine> redefines <%s> %s, but no top-level <%s> of that name was recorded from the redefined schema document: src-expredef requires \"a top-level definition item of the appropriate name and kind in the <redefine>d schema document\" in all cases",
			e.key.kind, qn, e.key.kind)
	}
	decl := p.ov.replacement(e.elem)
	if err := p.checkRedefinition(decl, qn); err != nil {
		return err
	}
	if p.chainedOriginal(e) {
		// A CHAINED redefine: an OUTER document redefines this one for the same
		// (kind, name), so §4.2.4 clause 4.1.2 excepts this redefinition from the
		// components this document contributes — the outer redefinition owns the
		// name. Every rule above is still charged, because this declaration is a
		// redefining one in its own right; only the component is withheld. It is
		// built instead as the outer redefinition's src-expredef clause 1.1
		// original, anonymous and under THIS producer, by the resolution site that
		// meets the outer self-reference (resolveBase, redefinedComplexBase).
		return nil
	}
	switch e.key.kind {
	case "simpleType":
		st, err := p.buildSimpleType(qn, decl)
		if err != nil {
			return err
		}
		p.builder.AddType(st)
		return nil
	case "complexType":
		// src-expredef clause 1.2. buildComplexType selects the redefining
		// identity from decl itself and memoises under decl, so this path and a
		// by-name reference to qn — which prescanRedefine points at this same
		// declaration — reach one component; the clause-1.1 original is built
		// under it (see this file's header).
		ct, err := p.buildComplexType(qn, decl)
		if err != nil {
			return err
		}
		p.builder.AddType(ct)
		return nil
	case "group":
		mgd, err := p.buildModelGroupDefinition(qn, decl)
		if err != nil {
			return err
		}
		original, restricts, err := p.redefinedGroupRestricted(decl, qn)
		if err != nil {
			return err
		}
		if !restricts {
			p.builder.AddModelGroup(mgd) // clause 6.1: a self-reference, no containment obligation
			return nil
		}
		p.builder.AddRedefiningModelGroup(mgd, original)
		return nil
	case "attributeGroup":
		ag, err := p.buildAttributeGroup(qn, decl)
		if err != nil {
			return err
		}
		original, restricts, err := p.redefinedAttributeGroupRestricted(decl, qn)
		if err != nil {
			return err
		}
		if !restricts {
			p.builder.AddAttributeGroup(ag) // clause 7.1: a self-reference, no restriction obligation
			return nil
		}
		p.builder.AddRedefiningAttributeGroup(ag, original)
		return nil
	}
	return fmt.Errorf("parser: <redefine> child <%s> at %s is outside §4.2.4's content model", e.key.kind, decl.Loc())
}

// checkRedefinition charges the src-redefine clause that constrains a redefining
// declaration's own shape, dispatched on its element type: clause 5 for a
// <simpleType>/<complexType>, clause 6 for a <group>, clause 7 for an
// <attributeGroup>.
func (p *producer) checkRedefinition(decl *Element, qn xsd.QName) error {
	switch decl.Name().Local() {
	case "simpleType":
		return p.checkRedefinedSimpleType(decl, qn)
	case "complexType":
		return p.checkRedefinedComplexType(decl, qn)
	case "group":
		return p.checkRedefinedGroup(decl, qn)
	case "attributeGroup":
		return p.checkRedefinedAttributeGroup(decl, qn)
	}
	return nil
}

// checkRedefinedSimpleType charges src-redefine clause 5's simple-type half:
// "each <simpleType> must have a <restriction> among its children … the ·actual
// value· of whose base [attribute] must be the same as the ·actual value· of its
// own name attribute plus target namespace".
func (p *producer) checkRedefinedSimpleType(decl *Element, qn xsd.QName) error {
	restriction := childElement(decl, xsd.XMLSchemaNS, "restriction")
	if restriction == nil {
		return xsderr.New(ruleSrcRedefine, decl.Loc(),
			"the redefining <simpleType> %s has no <restriction> child, but src-redefine clause 5 requires one whose base is %s itself", qn, qn)
	}
	same, err := p.namesSelf(restriction, qn)
	if err != nil {
		return err
	}
	if same {
		return nil
	}
	return xsderr.New(ruleSrcRedefine, restriction.Loc(),
		"the redefining <simpleType> %s does not restrict itself: src-redefine clause 5 requires its <restriction>'s base to be %s, its own name plus target namespace", qn, qn)
}

// checkRedefinedComplexType charges src-redefine clause 5's complex-type half:
// "each <complexType> must have a restriction or extension among its
// grand-children the ·actual value· of whose base [attribute] must be the same
// as the ·actual value· of its own name attribute plus target namespace". The
// depth is the spec's own — a <complexContent>/<simpleContent> child holding the
// derivation — so the implicit-content form, which states no derivation at all,
// fails the clause as it should.
//
// <annotation> subtrees are skipped, as selfReferences skips them and for the
// same reason: §3 states that "neither the correspondences described nor the XML
// Representation Constraints apply to elements in the Schema namespace which
// occur as descendants of <appinfo> or <documentation>", so a <restriction>
// written there is prose and discharges nothing. Only markup <complexType>'s own
// content model already forbids can reach it.
func (p *producer) checkRedefinedComplexType(decl *Element, qn xsd.QName) error {
	for _, child := range decl.Children() {
		c, ok := child.(*Element)
		if !ok || isXSD(c, "annotation") {
			continue
		}
		for _, grand := range c.Children() {
			d, ok := grand.(*Element)
			if !ok || d.Name().Space() != xsd.XMLSchemaNS {
				continue
			}
			if d.Name().Local() != "restriction" && d.Name().Local() != "extension" {
				continue
			}
			same, err := p.namesSelf(d, qn)
			if err != nil {
				return err
			}
			if same {
				return nil
			}
		}
	}
	return xsderr.New(ruleSrcRedefine, decl.Loc(),
		"the redefining <complexType> %s does not derive from itself: src-redefine clause 5 requires a <restriction> or <extension> among its grand-children whose base is %s, its own name plus target namespace", qn, qn)
}

// namesSelf reports whether the base attribute of derivation — a <restriction>
// or <extension> — resolves to qn, the redefining declaration's own expanded
// name. An absent base is not a match: src-redefine clause 5 reads the ·actual
// value· of an attribute that has to be there.
func (p *producer) namesSelf(derivation *Element, qn xsd.QName) (bool, error) {
	lexical, ok := derivation.Attr("base")
	if !ok {
		return false, nil
	}
	base, err := p.resolveQName(derivation, lexical, "base")
	if err != nil {
		return false, err
	}
	return base == qn, nil
}

// checkRedefinedGroup charges src-redefine clause 6 on a redefining <group>.
//
// Clause 6.1's precondition is a <group> among the redefinition's contents "at
// some level" whose ref= is the group's own name plus target namespace AND that
// "does not have an <element> ancestor". When one or more such self-references
// exist, 6.1.1 requires exactly one and 6.1.2 requires its minOccurs and
// maxOccurs both to be 1 (or ·absent·).
//
// It charges NOTHING on clause 6.2's no-self-reference branch, and nothing is
// missing there. 6.2.1 is src-expredef's closing requirement, already charged by
// produceRedefinition; 6.2.2 compares the element sequences two assembled model
// groups accept and is charged at finalize, over the pairing produceRedefinition
// hands xsd.SchemaBuilder.AddRedefiningModelGroup (redefinedGroupRestricted).
func (p *producer) checkRedefinedGroup(decl *Element, qn xsd.QName) error {
	refs, err := p.selfReferences(decl, qn, "group", true)
	if err != nil {
		return err
	}
	if len(refs) == 0 {
		return nil // clause 6.2, charged at finalize
	}
	if len(refs) > 1 {
		return xsderr.New(ruleSrcRedefine, refs[1].Loc(),
			"the redefining <group> %s references itself %d times (the first at %s), but src-redefine clause 6.1.1 permits exactly one", qn, len(refs), refs[0].Loc())
	}
	return checkSelfReferenceOccurs(refs[0], qn)
}

// checkRedefinedAttributeGroup charges src-redefine clause 7 on a redefining
// <attributeGroup>: clause 7.1 permits exactly one self-reference. It carries
// NEITHER of clause 6.1's extra conditions (see
// redefinedAttributeGroupOriginal).
//
// It charges NOTHING on clause 7.2's no-self-reference branch, and nothing is
// missing there. 7.2.1 is src-expredef's closing requirement, already charged by
// produceRedefinition; 7.2.2 compares two assembled components and is charged at
// finalize, over the pairing produceRedefinition hands
// xsd.SchemaBuilder.AddRedefiningAttributeGroup
// (redefinedAttributeGroupRestricted).
func (p *producer) checkRedefinedAttributeGroup(decl *Element, qn xsd.QName) error {
	refs, err := p.selfReferences(decl, qn, "attributeGroup", false)
	if err != nil {
		return err
	}
	if len(refs) <= 1 {
		return nil // one self-reference: clause 7.1; none: clause 7.2, charged at finalize
	}
	return xsderr.New(ruleSrcRedefine, refs[1].Loc(),
		"the redefining <attributeGroup> %s references itself %d times (the first at %s), but src-redefine clause 7.1 permits exactly one", qn, len(refs), refs[0].Loc())
}

// redefinedAttributeGroupRestricted returns the attribute group definition of S2
// — the redefined schema document — that a redefining <attributeGroup> must
// RESTRICT under src-redefine clause 7.2.2, built through the producer of the
// document that declares it.
//
// ok is false on clause 7.1's branch, where decl carries a self-reference: that
// branch states no restriction obligation at all, so the redefinition is added
// as an ordinary attribute group definition. Which branch applies is clause 7's
// own dispatch, and it is decided here the way checkRedefinedAttributeGroup
// decides it — by counting self-references, the same walk under the same
// exclusions, rather than by storing that count for a second reader (STYLE D3).
//
// A miss on originals is likewise reported as "no obligation", not as a fault:
// produceRedefinition has already charged src-expredef's closing requirement for
// exactly that miss, so this call is reached only when the entry is recorded.
// buildAttributeGroup is not memoised by name, so building the original under
// the same expanded name as the redefinition it pairs with is safe — the two
// components are separate values, and only the redefinition is registered.
func (p *producer) redefinedAttributeGroupRestricted(decl *Element, qn xsd.QName) (xsd.AttributeGroupDefinition, bool, error) {
	refs, err := p.selfReferences(decl, qn, "attributeGroup", false)
	if err != nil {
		return xsd.AttributeGroupDefinition{}, false, err
	}
	if len(refs) > 0 {
		return xsd.AttributeGroupDefinition{}, false, nil // clause 7.1
	}
	src, ok := p.originalFor(decl, qn, "attributeGroup")
	if !ok {
		return xsd.AttributeGroupDefinition{}, false, nil
	}
	original, err := src.owner.buildAttributeGroup(qn, src.elem)
	if err != nil {
		return xsd.AttributeGroupDefinition{}, false, err
	}
	return original, true, nil
}

// redefinedGroupRestricted returns the model group definition of S2 — the
// redefined schema document — a subset of whose element sequences a redefining
// <group> must accept under src-redefine clause 6.2.2, built through the producer
// of the document that declares it.
//
// ok is false on clause 6.1's branch, where decl carries a self-reference: that
// branch states no containment obligation at all, so the redefinition is added as
// an ordinary model group definition. Which branch applies is clause 6's own
// dispatch, decided here the way checkRedefinedGroup decides it — by counting
// self-references under the same <element>-ancestor exclusion, the same walk
// rather than a stored count (STYLE D3).
//
// A miss on originals is likewise reported as "no obligation", not as a fault:
// produceRedefinition has already charged src-expredef's closing requirement for
// exactly that miss, so this call is reached only when the entry is recorded.
//
// It differs from its attributeGroup twin (redefinedAttributeGroupRestricted) in
// two ways, each of which would otherwise be silent:
//
//   - it produces the original through discardingComponents, so the body's
//     registrations reach a throwaway builder.
//   - it calls produceModelGroupDefinition, never buildModelGroupDefinition, so
//     that throwaway build writes no entry to the ASSEMBLY-SHARED memo
//     (symbols.builtGroups). The memo is keyed by declaration, so an entry
//     written here would be keyed by S2's OWN top-level <group> — the very
//     declaration run dispatches whenever some reading does not except it
//     (§4.2.4 clause 4.1.2) — and run would then register a component whose
//     named <key>/<unique>/<keyref> children reached the throwaway builder and
//     nothing else. It is the unmemoised layer #505 drops to for its own
//     clause-1.1 original.
func (p *producer) redefinedGroupRestricted(decl *Element, qn xsd.QName) (xsd.ModelGroupDefinition, bool, error) {
	refs, err := p.selfReferences(decl, qn, "group", true)
	if err != nil {
		return xsd.ModelGroupDefinition{}, false, err
	}
	if len(refs) > 0 {
		return xsd.ModelGroupDefinition{}, false, nil // clause 6.1
	}
	src, ok := p.originalFor(decl, qn, "group")
	if !ok {
		return xsd.ModelGroupDefinition{}, false, nil
	}
	original, err := src.owner.discardingComponents().produceModelGroupDefinition(qn, src.elem)
	if err != nil {
		return xsd.ModelGroupDefinition{}, false, err
	}
	return original, true, nil
}

// discardingComponents returns a shallow copy of p whose builder is a throwaway,
// for mapping a subtree whose registrations must not reach the schema. Two
// callers want that:
//
//   - a declaration §4.2.4 clause 4.1.2 excepts from the components its document
//     contributes, where the component VALUE is wanted and every registration it
//     performs on the way is not (redefinedGroupRestricted);
//   - an elided <element>/<all>/<choice>/<sequence> subtree, which maps to no
//     component at all (§3.3.2.3, §3.8.2) and so contributes none either, while
//     §5.1's first bullet still binds its grammar (produceElementParticle,
//     produceGroupParticle, #883).
//
// The one registration reachable from a content model is AddIdentityConstraint —
// produceIdentityConstraint's name= arm, for a named <unique>/<key>/<keyref> on a
// local <element>; every other builder.Add* call site is run's top-level dispatch
// or produceRedefinition's own — and it is exactly the one that must not run. Let
// one out and it is the FIRST registration of that constraint, out of document
// order (STYLE D2) and colliding sch-props-correct (§3.17.6.1) clause 2 against a
// live definition of the same name — the redefinition's own copy of the redefined
// key, the commonest redefine shape, or a sibling of the elided <element>. Both
// are false rejects, and both have a false-accept twin: a <keyref> whose refer=
// resolves only to the constraint that should not be there.
//
// Everything else the copy shares, symbols above all. An on-demand build reached
// from the body runs through the DECLARING document's own producer (typeSource's
// owner — resolveModelGroup and resolveBaseType alike), which this copy does not
// touch, so it registers with the real builder exactly as it would have.
func (p *producer) discardingComponents() *producer {
	discarding := *p
	discarding.builder = xsd.NewSchemaBuilder()
	return &discarding
}

// selfReferences collects, in document order, every descendant of decl of
// element type kind whose ref= resolves to qn — the self-references src-redefine
// clauses 6.1 and 7.1 count.
//
// excludeElement stops the descent at an <element>, which is clause 6.1's "and
// that <group> does not have an <element> ancestor": a matching <group ref>
// under a local element is not counted, so a redefinition whose only
// self-reference sits there falls to clause 6.2 rather than satisfying 6.1 —
// which is why a naive "one ref= matching my own name" count gets clause 6
// wrong. It is a COUNTING rule only: such a reference still resolves to the
// original (redefinedGroupOriginal). Clause 7.1 has no such exclusion and passes
// false.
//
// <annotation> subtrees are never descended: §3 states that "neither the
// correspondences described nor the XML Representation Constraints apply to
// elements in the Schema namespace which occur as descendants of <appinfo> or
// <documentation>", so a <group ref> written there is prose and counts for
// nothing.
//
// Every ref= it meets is resolved, and a resolution failure is RETURNED rather
// than skipped (STYLE S3): an unbound prefix is a src-resolve verdict wherever
// it occurs.
func (p *producer) selfReferences(decl *Element, qn xsd.QName, kind string, excludeElement bool) ([]*Element, error) {
	var refs []*Element
	for _, child := range decl.Children() {
		c, ok := child.(*Element)
		if !ok || c.Name().Space() != xsd.XMLSchemaNS {
			continue
		}
		if isXSD(c, "annotation") {
			continue
		}
		if excludeElement && isXSD(c, "element") {
			continue
		}
		if c.Name().Local() == kind {
			match, err := p.namesSelfRef(c, qn)
			if err != nil {
				return nil, err
			}
			if match {
				refs = append(refs, c)
			}
		}
		nested, err := p.selfReferences(c, qn, kind, excludeElement)
		if err != nil {
			return nil, err
		}
		refs = append(refs, nested...)
	}
	return refs, nil
}

// namesSelfRef reports whether el's ref attribute resolves to qn. An element
// with no ref (a nested definition form, which the grammar does not admit here
// anyway) matches nothing.
func (p *producer) namesSelfRef(el *Element, qn xsd.QName) (bool, error) {
	lexical, ok := el.Attr("ref")
	if !ok {
		return false, nil
	}
	ref, err := p.resolveQName(el, lexical, "ref")
	if err != nil {
		return false, err
	}
	return ref == qn, nil
}

// checkSelfReferenceOccurs charges src-redefine clause 6.1.2: "The ·actual
// value· of both that group's minOccurs and maxOccurs [attribute] is 1 (or
// ·absent·)". An absent attribute defaults to 1 and passes; maxOccurs="unbounded"
// is not 1 and fails here rather than being read as an occurrence range, since
// clause 6.1.2 is about the attribute's actual value, not about the particle.
func checkSelfReferenceOccurs(ref *Element, qn xsd.QName) error {
	for _, attr := range []string{"minOccurs", "maxOccurs"} {
		a, present := ref.attr(attr)
		if !present {
			continue
		}
		lexical := a.Value()
		if collapseTrim(lexical) == "unbounded" {
			return xsderr.New(ruleSrcRedefine, ref.Loc(),
				"the self-reference of the redefining <group> %s has %s=%q, but src-redefine clause 6.1.2 requires both minOccurs and maxOccurs to be 1 (or absent)", qn, attr, lexical)
		}
		n, err := nonNegativeInt(lexical, a.Loc(), attr)
		if err != nil {
			return err
		}
		if n == 1 {
			continue
		}
		return xsderr.New(ruleSrcRedefine, ref.Loc(),
			"the self-reference of the redefining <group> %s has %s=%q, but src-redefine clause 6.1.2 requires both minOccurs and maxOccurs to be 1 (or absent)", qn, attr, lexical)
	}
	return nil
}
