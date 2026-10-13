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
//   - those an instance attribute reaches (nil-lexical, type-table-whitespace,
//     the three xsi-type ones, simple-attribute, attribute-unadmitted,
//     strict-attribute-unresolved, attribute-ldt): unrepresentable, jsonsrc
//     presenting no attribute;
//   - undeclared-root: unrepresentable, jsonsrc naming the root only by a
//     top-level declaration;
//   - recorded in Result.Unevaluated by the walk: type-table-undecided
//     (key-cta-ta-select), type-unresolved (cvc-elt clause 1, charged
//     instead for an ·absent· substitution group head), attribute-type
//     (cvc-complex-type clause 4 for a ·defaulted attribute·, the one use whose
//     absent attribute's type the walk reads), and content-matcher
//     (cvc-complex-content, at each unmatched element [[child]] or, with none,
//     at the element: validate's contentCheck.sequenceEnd);
//   - charged by the walk: error-type (cvc-type clause 3.1.3, no element being
//     ·nilled· without an xsi:nil), element-child (cvc-type clause 3.1.2,
//     cvc-complex-type clauses 1.1 and 1.2), content-incomplete and
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
	return decideAgreement(true, c.expect.wantsValid()), ""
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
