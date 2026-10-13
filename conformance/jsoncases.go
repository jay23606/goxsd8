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
//   - those an xsi: attribute reaches (nil-lexical and the three xsi-type
//     ones): unrepresentable, jsonsrc presenting no xsi: attribute — it
//     declines a null, a "$type" member and a key naming an attribute use in
//     the xsi namespace;
//   - type-table-whitespace: unrepresentable while jsonsrc declines a matched
//     declaration carrying a {type table}. A wildcard's child that a top-level
//     declaration governs in its place has none either: cos-element-
//     consistent's wildcard condition gives the two equivalent {type table}s;
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
//   - charged by the walk: error-type (cvc-type clause 3.1.3, no element being
//     ·nilled· without an xsi:nil), element-child (cvc-type clause 3.1.2,
//     cvc-complex-type clauses 1.1 and 1.2), simple-attribute (cvc-type clause
//     3.1.1), attribute-unadmitted (cvc-complex-type clause 2.2.1 or 2.2.2,
//     validate's walk.unmatchedAttribute), content-incomplete and
//     content-rejected (cvc-complex-content);
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
	return decideAgreement(true, c.expect.wantsValid()), ""
}

// attributeGovernanceDiverges reports whether schema has a top-level element
// declaration G whose {type definition} T is a Complex Type Definition with a
// strict or lax {attribute wildcard} and lacks an attribute use some type on
// its {base type definition} chain carries (subtreeGate.onChain, which answers
// true for a base that does not resolve, as this does for a G whose type does
// not). It is the condition, over the schema alone, without which neither
// refuseStrictAttribute nor refuseAttributeLDT can arise in an infoset
// jsonsrc builds:
//
//   - jsonsrc presents an attribute only under a use of the {type definition}
//     of the declaration D its element's key matched, so the attribute
//     matches no use (cvc-complex-type clause 2.2, where both refusals sit)
//     only where some other type governs the element;
//   - at the root and under an element particle D governs (cos-element-
//     consistent gives same-named contained declarations one type), and under
//     an {open content} the ·locally declared type· does, which is D's type;
//   - which leaves a child ·attributed to· a strict or lax Wildcard, governed
//     by the top-level G its name ·resolves· to, jsonsrc presenting no
//     xsi:type. The walk charges cvc-complex-type clause 5 unless T is
//     ·validly substitutable· for D's type, which carries the use T lacks,
//     and a skip {attribute wildcard} or none admits nothing either refusal
//     reads.
//
// The walk charges and records nothing for either shape (subtreeGate's
// wildcardAttribute), so a "valid" reading is refused wherever the condition
// holds, whether or not the document reaches it.
func attributeGovernanceDiverges(schema *xsd.Schema) bool {
	g := subtreeGate{schema: schema}
	for _, d := range schema.Elements() {
		td, ok := schema.ResolvedType(d.TypeDefinition())
		if !ok {
			return true
		}
		t, ok := td.(xsd.ComplexType)
		if !ok {
			continue
		}
		wild, ok := t.AttributeWildcard()
		if !ok || wild.ProcessContents() == xsd.ProcessSkip {
			continue
		}
		own := t.AttributeUses()
		lacks := func(c xsd.ComplexType) bool {
			return slices.ContainsFunc(c.AttributeUses(), func(u xsd.AttributeUse) bool {
				return !slices.ContainsFunc(own, func(o xsd.AttributeUse) bool { return o.DeclarationName() == u.DeclarationName() })
			})
		}
		if g.onChain(t, lacks) {
			return true
		}
	}
	return false
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
