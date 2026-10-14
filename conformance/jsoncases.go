package conformance

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/validate/jsonsrc"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// This file is the json lane: the curated JSON corpus's discovery and the
// executor that runs it through validate/jsonsrc (conformance/doc.go, "The
// curated JSON corpus").
//
// The corpus is a SECOND case root beside the W3C suite, and it reaches no
// lane but json. parseSuite never reads it, so the suite's discovery, the
// Catalog reader and the agreement test between them see the suite alone;
// TestConformance appends the curated cases to the suite's, and every curated
// case is kindJSON, which only the json lane's selector claims
// (TestNoOtherLaneClaimsACuratedCase).

// jsonCorpusIndex is the curated corpus's one testSet document, as `go test`
// sees it from the package directory.
const jsonCorpusIndex = "testdata/json/json.testSet"

// curatedJSONCases discovers the curated corpus at index: one kindJSON case per
// instanceTest, sorted by ID, each assessed against its group's schemaTest
// documents as an instance case is (groupSchemaDocs). A schemaTest is not a
// case: the schema lane scores the suite's schema documents, not this one.
//
// The testSet is read through the suite's own structs and makeCase, so a case
// carries an ID and an expectation built as a suite case's is. Unlike the
// suite's, a curated testSet scopes nothing away by `version`: every case it
// declares is the lane's to run.
func curatedJSONCases(index string) ([]caseSpec, error) {
	set, err := decodeTestSet(index)
	if err != nil {
		return nil, err
	}
	setDir := filepath.Dir(index)
	seen := map[string]struct{}{}
	var cases []caseSpec
	for _, g := range set.Groups {
		for _, it := range g.InstanceTests {
			c, err := makeCase(set.Name, kindJSON, it, g, setDir, seen)
			if err != nil {
				return nil, err
			}
			cases = append(cases, c)
		}
	}
	slices.SortFunc(cases, func(a, b caseSpec) int { return strings.Compare(a.id, b.id) })
	return cases, nil
}

// The json lane's refusals, beside the instance lane's it shares.
const (
	refuseJSONUnresolved refusal = "json-unresolved" // execJSONCase: the instance will not open
	refuseJSONDeclined   refusal = "json-declined"   // execJSONCase: jsonsrc.Validate returned an error
	// execJSONCase: attributeGovernanceDiverges, on a "valid" reading
	refuseJSONAttributeGovernance refusal = "json-attribute-governance"
	// execJSONCase: nillableErrorType, on a "valid" reading
	refuseJSONNilledErrorType refusal = "json-nilled-error-type"
)

// newJSONExec builds the json lane's executor over the strict backend, built
// once, as newInstanceExec builds the instance lane's.
func newJSONExec() laneExecutor {
	backend := strict.New()
	return func(c caseSpec) (Status, refusal) {
		return execJSONCase(backend, c)
	}
}

// execJSONCase decides one curated case, or declines it naming the refusal: it
// assembles the group's schema (assembleCase), assesses the JSON instance
// through jsonsrc.Validate, and reads the Result as the instance lane reads
// one. A set of decidable charges (decidedNotValid) is "not valid"; an empty
// Result with nothing in Result.Unevaluated is "valid".
//
// That second reading is wider than the instance lane's, which also holds the
// document to assessedSubtreeRoot's shape. It is sound for the infoset jsonsrc
// builds alone, because each refusal of that gate is enforced here, by the
// walk, or unrepresentable in that infoset:
//
//   - versioned: refused here, through closureVersioned as the gate does;
//   - the raw re-read's (gate-unresolved, decode, doctype, prolog-text,
//     epilog-directive): unrepresentable, jsonsrc building no DOCTYPE, prolog
//     or epilog, and refusing an instance that will not open
//     (refuseJSONUnresolved) or will not map (refuseJSONDeclined);
//   - nil-lexical: unrepresentable, the one xsi:nil jsonsrc presents being a
//     null's, with the lexical form true, which has an ·actual value·
//     (jsonsrc's walker.nilled): a "$type" member presents xsi:type alone
//     (walker.lead), and jsonsrc declines a key naming an attribute use in
//     the xsi namespace (walker.useNamed), and one an {attribute wildcard}
//     would name in it (walker.wildcardMember);
//   - the three xsi-type ones, at the xsi:type a "$type" member presents:
//     xsi-type-unresolved charged by the walk, cvc-attribute clause 5 for a
//     name no type definition carries (validate's walk.instanceTypeResolves,
//     the json case type/type-unresolved.i) and clause 3 for a lexical with
//     no ·actual value· (walk.instanceTypeLexical, jsonsrc's TestTypeMember
//     row "unbound prefix"), and recorded in Result.Unevaluated where the
//     walk declines either clause; xsi-type-not-overriding charged by the
//     walk, cvc-elt clause 4 (walk.instanceOverride, the json case
//     type/type-unrelated.i); and xsi-type-undecided recorded in
//     Result.Unevaluated by walk.instanceOverride and walk.localGovernance,
//     which no finalized Schema reaches (#2174). Where the xsi:type governs,
//     jsonsrc has classified the element's members under the type it names,
//     resolved through the walk's own resolver (internal/xmlqname, from
//     jsonsrc's walker.instanceType);
//   - error-type: refused here on a "valid" reading, through
//     nillableErrorType, which says why no other route reaches it; the walk
//     charges cvc-type clause 3.1.3 for an element ·xs:error· governs that is
//     not ·nilled·;
//   - type-table-whitespace: unrepresentable while jsonsrc declines a
//     declaration carrying a {type table} that it classifies an element by,
//     the top-level one a wildcard's child, or the child of a type with no
//     wildcard, is named for included. A child jsonsrc matched to a contained
//     declaration that a top-level declaration governs in its place has none
//     either: cos-element-consistent's wildcard condition gives the two
//     equivalent {type table}s. A child either names with no declaration,
//     ·laxly assessed· or governed by a ·locally declared type·, has no {type
//     table} to select through. A child classified under the declarations of
//     the type X its parent's "$type" names is governed by them wherever X
//     governs the parent; where X does not ·override· the type a ·governing
//     element declaration· supplies, the walk charges cvc-elt clause 4 at the
//     parent, so no "valid" reading stands on what the child's governing
//     declaration selects; and jsonsrc declines the "$type" wherever a
//     ·locally declared type· could supply it uncharged — where the type the
//     parent's members are classified under has a non-skip {open content}
//     under which the child's name has one, and where that type may not
//     govern the parent at all (walker.localType, walker.governs);
//   - strict-attribute-unresolved and attribute-ldt: refused here on a "valid"
//     reading, through attributeGovernanceDiverges, which says why no other
//     element reaches them;
//   - undeclared-root: unrepresentable, jsonsrc naming the root only by a
//     top-level declaration;
//   - recorded in Result.Unevaluated by the walk: type-table-undecided
//     (key-cta-ta-select), type-unresolved (cvc-elt clause 1, charged
//     instead for an ·absent· substitution group head), attribute-type
//     (cvc-complex-type clause 4 for a ·defaulted attribute·, the one use
//     whose absent attribute's type the walk reads, and cvc-attribute clause
//     3 for a present attribute, validate's walk.declaredAttribute, which no
//     schema document reaches: §3.2.2.2 types a typeless declaration
//     xs:anySimpleType, and src-resolve refuses a type= naming a complex
//     type), and content-matcher (cvc-complex-content, at each unmatched element
//     [[child]] or, with none, at the element: validate's
//     contentCheck.sequenceEnd);
//   - charged by the walk: element-child (cvc-type clause 3.1.2, cvc-complex-type clauses
//     1.1 and 1.2, a child jsonsrc names under a simple type or simple or empty content
//     included), simple-attribute (cvc-type clause 3.1.1), attribute-unadmitted
//     (cvc-complex-type clause 2.2.1 or 2.2.2, validate's walk.unmatchedAttribute, an
//     attribute jsonsrc names under an {attribute wildcard} that rejects it by name
//     included), content-incomplete and content-rejected (cvc-complex-content, a child
//     jsonsrc names under element wildcards that reject it, or under a type with no
//     wildcard, included);
//   - unreachable on a finalized Schema: attribute-use-unresolved,
//     member-unresolved and any-type; and type-kind, content-type and
//     attribution, each the default of a sealed sum.
//
// jsonsrc's error channel is a decline here, never a verdict: Validate returns
// (nil, err) for malformed JSON and for every input its arms do not map, and
// neither says anything of the document's validity.
func execJSONCase(backend value.Backend, c caseSpec) (Status, refusal) {
	schema, report, decidable, perr := assembleCase(backend, c.schemaDoc, c.schemaExtraDocs)
	if !decidable {
		return Fail(), refuseGroupAssembly
	}
	if perr != nil {
		return Fail(), refuseSchemaError
	}
	if closureVersioned(report, c.doc) {
		return Fail(), refuseVersioned
	}
	v, err := validate.New(schema, backend)
	if err != nil {
		return Fail(), refuseValidator
	}
	result, why := assessJSON(v, c.doc)
	if why != "" {
		return Fail(), why
	}
	if violations := result.Violations(); len(violations) > 0 {
		if !decidedNotValid(violations) {
			return Fail(), refuseUndecidedRule
		}
		return decideAgreement(false, c.expect.wantsValid()), ""
	}
	if unevaluated := result.Unevaluated(); len(unevaluated) > 0 {
		return Fail(), unevaluatedRefusal(unevaluated)
	}
	if attributeGovernanceDiverges(schema) {
		return Fail(), refuseJSONAttributeGovernance
	}
	if nillableErrorType(schema) {
		return Fail(), refuseJSONNilledErrorType
	}
	return decideAgreement(true, c.expect.wantsValid()), ""
}

// nillableErrorType reports whether schema has an element declaration D —
// one anyDeclaration reaches — whose {nillable} is true and whose {type
// definition} is ·xs:error·. It is the condition, over the schema alone,
// without which refuseErrorType cannot arise in an infoset jsonsrc builds:
// §3.16.7.3 says an item ·xs:error· governs "will be invalid", and the walk
// charges that as cvc-type clause 3.1.3 only for an element that is not
// ·nilled·, while a JSON null presents xsi:nil="true" (key-nilled needs
// D.{nillable} too). The other routes to ·xs:error· stay closed: jsonsrc
// declines a declaration carrying a {type table}; a wildcard's child, or a
// wildcard-free type's, governed by a top-level declaration is governed by
// one of those visited here; one governed by a ·locally declared type· or
// ·laxly assessed· has no declaration, so it is never ·nilled· and the walk
// charges clause 3.1.3; and an xsi:type naming ·xs:error· is presented only
// by a "$type" member, which rides an object value and never a null, so its
// element carries no xsi:nil, is never ·nilled·, and is charged cvc-type
// clause 3.1.3 wherever ·xs:error· governs it (jsonsrc's TestTypeMember row
// "xs:error is not nilled"); where it does not govern, another type does, and
// no ·xs:error· item arises from it. It refuses whether or not the document
// reaches D, as attributeGovernanceDiverges does.
func nillableErrorType(schema *xsd.Schema) bool {
	return anyDeclaration(schema, func(d xsd.ElementDeclaration) bool { return nilledError(schema, d) })
}

// anyDeclaration reports whether holds answers true for some element
// declaration of schema: a top-level one, one a named model group owns, or
// one the content model of a Complex Type Definition reached below contains.
// It also answers true where the walk meets an ElementScopeParent arm it does
// not know, refusing rather than leaving that arm's declarations unvisited.
// Those are every declaration jsonsrc can classify an element by: the root's,
// a contained one, and the top-level one a wildcard's child, or a
// wildcard-free type's, is named for.
//
// The types visited are every named one (Schema.Types) and every anonymous
// one an ownership edge reaches from them, from a top-level declaration, or
// from a declaration a named model group owns (Schema.ModelGroups): the inline
// {type definition} of a declaration whose {scope}.{parent} is the type being
// visited, and an inline {base type definition} (src-expredef clause 1.1).
// Each anonymous type has one owner, so those edges form a tree rooted at the
// top-level components and the walk needs no visited set (STYLE D4).
func anyDeclaration(schema *xsd.Schema, holds func(xsd.ElementDeclaration) bool) bool {
	for _, d := range schema.Elements() {
		if declaresWhere(schema, d, holds) {
			return true
		}
	}
	for _, td := range schema.Types() {
		if t, ok := td.(xsd.ComplexType); ok && containsWhere(schema, t, holds) {
			return true
		}
	}
	for _, g := range schema.ModelGroups() {
		if groupOwnsWhere(schema, g.ModelGroup(), holds) {
			return true
		}
	}
	return false
}

// declaresWhere reports whether holds answers true for d, or the walk below
// d's inline Complex Type Definition answers true.
func declaresWhere(schema *xsd.Schema, d xsd.ElementDeclaration, holds func(xsd.ElementDeclaration) bool) bool {
	if holds(d) {
		return true
	}
	t, ok := ownedComplexType(d.TypeDefinition())
	return ok && containsWhere(schema, t, holds)
}

// groupOwnsWhere is anyDeclaration's walk over the declarations a named model
// group's {model group} g owns: its inline element particles, through its
// inline model groups. A reference by name is no ownership edge: an <element
// ref> names a top-level declaration and a <group ref> a model group
// definition, each visited from its own loop in anyDeclaration.
func groupOwnsWhere(schema *xsd.Schema, g xsd.ModelGroup, holds func(xsd.ElementDeclaration) bool) bool {
	for _, p := range g.Particles() {
		resolved, ok := p.Term().(xsd.ResolvedTerm)
		if !ok {
			continue // an ElementDeclarationRef or a ModelGroupRef
		}
		switch term := resolved.Term.(type) {
		case xsd.ElementDeclaration:
			if declaresWhere(schema, term, holds) {
				return true
			}
		case xsd.ModelGroup:
			if groupOwnsWhere(schema, term, holds) {
				return true
			}
		case xsd.Wildcard:
			// declares nothing
		}
	}
	return false
}

// containsWhere is anyDeclaration's walk below the Complex Type Definition t:
// true if holds answers true for a declaration t's content model contains, or
// if the walk down an ownership edge from t answers true.
func containsWhere(schema *xsd.Schema, t xsd.ComplexType, holds func(xsd.ElementDeclaration) bool) bool {
	if base, ok := ownedComplexType(t.Base()); ok && containsWhere(schema, base, holds) {
		return true
	}
	for _, d := range schema.ContainedElementDeclarations(t) {
		if holds(d) {
			return true
		}
		inner, ok := ownedComplexType(d.TypeDefinition())
		if !ok {
			continue
		}
		parent, _ := d.Scope().Parent()
		switch p := parent.(type) {
		case nil:
			// A top-level declaration: anyDeclaration visits its type.
		case xsd.ModelGroupScopeParent:
			// A named model group's: visited from schema.ModelGroups().
		case xsd.ComplexTypeScopeParent:
			if p.Name == t.Name() && containsWhere(schema, inner, holds) {
				return true
			}
		case xsd.AnonymousComplexTypeScopeParent:
			ctx, ok := t.Context()
			if ok && ctx.ID() == p.Owner && containsWhere(schema, inner, holds) {
				return true
			}
		default:
			// An ElementScopeParent arm this walk does not know: refuse rather
			// than leave its declarations unvisited.
			return true
		}
	}
	return false
}

// nilledError reports whether d's {nillable} is true and its {type
// definition} is ·xs:error·.
func nilledError(schema *xsd.Schema, d xsd.ElementDeclaration) bool {
	if !d.Nillable() {
		return false
	}
	td, ok := schema.ResolvedType(d.TypeDefinition())
	return ok && td.Name() == errorTypeName
}

// ownedComplexType is the anonymous Complex Type Definition ref owns (its
// InlineTypeDefinition arm), and false for a reference by name, an ·absent·
// slot or a simple type.
func ownedComplexType(ref xsd.TypeDefinitionOrRef) (xsd.ComplexType, bool) {
	inline, ok := ref.(xsd.InlineTypeDefinition)
	if !ok {
		return xsd.ComplexType{}, false
	}
	t, ok := inline.Definition.(xsd.ComplexType)
	return t, ok
}

// attributeGovernanceDiverges reports whether schema has an element
// declaration D — one anyDeclaration reaches — whose {type definition} does
// not resolve, or a type T — D's {type definition} or a named one
// (Schema.Types) — that wildcardDiverges holds for. It is the condition, over
// the schema alone, without which neither refuseStrictAttribute nor
// refuseAttributeLDT can arise in an infoset jsonsrc builds, both refusals
// sitting at an attribute that matches none of its element's governing
// type's {attribute uses} (cvc-complex-type clause 2.2):
//
//   - jsonsrc presents an attribute under a use of the type its element's
//     members are classified under — the one its "$type" member names where
//     that resolves, a named type X, and otherwise the type of the
//     declaration D its element was classified by (jsonsrc's
//     walker.classifyingType) — or, where no use carries the key, under the
//     one namespace that type's {attribute wildcard} enumerates
//     (walker.wildcardMember). Wherever that type governs the element, the
//     second is ·attributed to· that wildcard, and
//     subtreeGate.wildcardAttribute refuses it where the wildcard is strict
//     and the name ·resolves· to no top-level declaration, and where a base's
//     use of the name gives it a non-·absent· ·locally declared type·: so the
//     strict and the lacks conditions bind every D, local ones included, and
//     every X;
//   - where a "$type" names X, X governs wherever it ·overrides· the type a
//     ·governing element declaration· supplies, and wherever the element has
//     neither that nor a ·locally declared type· (key-governing-type-elem
//     clause 8); where X does not ·override· a declaration's type, the walk
//     charges cvc-elt clause 4 (walk.instanceOverride), and no "valid"
//     reading stands, whatever the element's descendants are classified
//     under; and jsonsrc declines the "$type" wherever a non-·absent·
//     ·locally declared type· could govern instead (clauses 6 and 7): under
//     a non-skip {open content} of the parent's classifying type where the
//     name has one, and under a parent that type may not govern (walker.localType,
//     walker.governs). Where the "$type" names none, the walk charges cvc-attribute or
//     records it unevaluated (execJSONCase's xsi-type bullet), and the members are
//     classified under D's type, which governs;
//   - with no "$type", D's type governs the root, a child under an element particle
//     (cos-element-consistent gives same-named contained declarations one
//     type), and a child jsonsrc names, under a wildcard or a type with none,
//     for the top-level D its name ·resolves· to (key-governing-ed clauses 3
//     and 4). Under an {open content}, a child jsonsrc matched to
//     a contained D is governed by the ·locally declared type·, which is D's
//     type, and a wildcard-named one by a base's contained declaration's type
//     where that is non-·absent·: that declaration is visited here too, and
//     the child carries attributes presented under the top-level D its name
//     resolves to, or none;
//   - which leaves a child with no "$type" that jsonsrc matched to a
//     contained D but ·attributed to· a strict or lax Wildcard, governed by
//     the top-level G its name ·resolves· to. The walk charges
//     cvc-complex-type clause 5 unless G's type is ·validly substitutable·
//     for D's, which carries every use G's type lacks but one a restriction
//     on G's chain prohibited: G's lacks condition. A skip {attribute
//     wildcard} or none admits nothing either refusal reads.
//
// The walk charges and records nothing for either shape (subtreeGate's
// wildcardAttribute), so a "valid" reading is refused wherever the condition
// holds, whether or not the document reaches it.
func attributeGovernanceDiverges(schema *xsd.Schema) bool {
	g := subtreeGate{schema: schema}
	declared := anyDeclaration(schema, func(d xsd.ElementDeclaration) bool {
		td, ok := schema.ResolvedType(d.TypeDefinition())
		return !ok || wildcardDiverges(g, td)
	})
	return declared || slices.ContainsFunc(schema.Types(), func(td xsd.TypeDefinition) bool { return wildcardDiverges(g, td) })
}

// wildcardDiverges reports whether td is a Complex Type Definition T with an
// {attribute wildcard} that is strict, or lax where T lacks an attribute use
// some type on T's {base type definition} chain carries (subtreeGate.onChain,
// which answers true for a base that does not resolve).
func wildcardDiverges(g subtreeGate, td xsd.TypeDefinition) bool {
	t, ok := td.(xsd.ComplexType)
	if !ok {
		return false
	}
	wild, ok := t.AttributeWildcard()
	if !ok || wild.ProcessContents() == xsd.ProcessSkip {
		return false
	}
	if wild.ProcessContents() == xsd.ProcessStrict {
		return true
	}
	own := t.AttributeUses()
	lacks := func(c xsd.ComplexType) bool {
		return slices.ContainsFunc(c.AttributeUses(), func(u xsd.AttributeUse) bool {
			return !slices.ContainsFunc(own, func(o xsd.AttributeUse) bool { return o.DeclarationName() == u.DeclarationName() })
		})
	}
	return g.onChain(t, lacks)
}

// assessJSON opens the JSON instance at doc and assesses it against v, or
// declines, naming the refusal.
func assessJSON(v *validate.Validator, doc string) (*validate.Result, refusal) {
	f, err := os.Open(doc)
	if err != nil {
		return nil, refuseJSONUnresolved
	}
	defer func() { _ = f.Close() }() // read-only handle: close error cannot affect the verdict
	result, err := jsonsrc.Validate(v, f, jsonsrc.WithURI(doc))
	if err != nil {
		return nil, refuseJSONDeclined
	}
	return result, ""
}
