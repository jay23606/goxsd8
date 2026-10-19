package conformance

import (
	"path/filepath"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/xsd"
)

// groupedCase writes files and the instance document into a fresh directory
// and returns the caseSpec discovery builds for an instanceTest whose group
// declares a schemaTest listing the file named g.xsd alone, so any other file
// is reached only through the instance's own hints (#771).
func groupedCase(t *testing.T, files []fixtureFile, instance string, valid bool) caseSpec {
	t.Helper()
	c := hintedCase(t, files, instance, valid)
	c.schemaDoc = filepath.Join(filepath.Dir(c.doc), "g.xsd")
	return c
}

// groupA is a group schema for urn:a declaring the root {urn:a}known, and an
// element {urn:a}open whose strict {attribute wildcard} admits any namespace
// but urn:a's.
var groupA = fixtureFile{"g.xsd", xsdDoc("urn:a", knownRoot+
	`<xs:element name="open"><xs:complexType><xs:anyAttribute namespace="##other"/></xs:complexType></xs:element>`)}

// hintB declares, for urn:b, a root {urn:b}known and a top-level attribute
// {urn:b}att, neither of which groupA's closure holds.
var hintB = fixtureFile{"h.xsd", xsdDoc("urn:b", knownRoot+`<xs:attribute name="att" type="xs:int"/>`)}

// hintNone declares, in no namespace, the root known.
var hintNone = fixtureFile{"n.xsd", xsdDoc("", knownRoot)}

// aRoot and bRoot open an instance root carrying the ElemDecl targetNS00101m1_p
// hint pair: urn:a's names the group's own document, urn:b's names h.xsd.
const (
	hintPairAB = xsiNS + ` xmlns:a="urn:a" xmlns:b="urn:b" xsi:schemaLocation="urn:a g.xsd urn:b h.xsd"`
	bRoot      = `<b:known ` + hintPairAB + `>`
	aOpen      = `<a:open ` + hintPairAB
)

// TestGroupSchemaDecidesFromUncoveredHint pins the shapes of #771's four
// decided-and-wrong cases: a group whose schema covers urn:a, and an instance
// whose root hints a document for a namespace the group's closure does not
// cover. caseSchema adds that document (groupSchema), so the case is decided
// against what it declares, in both polarities. The rows, against what each
// answers with groupSchema's hint arm removed (run: every row fails):
//
//   - a root in urn:b (targetNS00101m1_p, ST_targetNS00101m2_p): its
//     declaration is hint-only, so the valid row is charged cvc-assess-elt and
//     decided not valid; the invalid row is too, so both also check the
//     assembled schema declares the root.
//   - an attribute on a urn:a root that its strict {attribute wildcard}
//     matches, declared only in urn:b (attgD034.v, ctL021.v; an element under a
//     strict wildcard is particlesB013.v and schA1.v): both rows decline as
//     strict-attribute-unresolved.
//   - a root in no namespace hinted by xsi:noNamespaceSchemaLocation (schA1.v's
//     second hint attribute): decided not valid, as the first row.
func TestGroupSchemaDecidesFromUncoveredHint(t *testing.T) {
	exec := newInstanceExec()
	for _, tc := range []struct {
		why      string
		files    []fixtureFile
		instance string
		valid    bool
		declares xsd.QName
	}{
		{"a root declared only in the hinted urn:b document", []fixtureFile{groupA, hintB},
			bRoot + `x</b:known>`, true, xsd.QName{Space: "urn:b", Local: "known"}},
		// cvc-type clause 3.1.2: an element [[child]] under a simple type.
		{"a root declared only in the hinted urn:b document, with an element child", []fixtureFile{groupA, hintB},
			bRoot + `<x/></b:known>`, false, xsd.QName{Space: "urn:b", Local: "known"}},
		{"a wildcard-matched attribute declared only in the hinted urn:b document", []fixtureFile{groupA, hintB},
			aOpen + ` b:att="1"/>`, true, xsd.QName{Space: "urn:a", Local: "open"}},
		// cvc-attribute clause 3 against the hinted declaration's xs:int.
		{"a wildcard-matched attribute invalid against its hinted urn:b declaration", []fixtureFile{groupA, hintB},
			aOpen + ` b:att="x"/>`, false, xsd.QName{Space: "urn:a", Local: "open"}},
		{"a no-namespace root hinted by xsi:noNamespaceSchemaLocation", []fixtureFile{groupA, hintNone},
			`<known ` + xsiNS + ` xsi:schemaLocation="urn:a g.xsd" xsi:noNamespaceSchemaLocation="n.xsd">x</known>`,
			true, xsd.QName{Local: "known"}},
	} {
		c := groupedCase(t, tc.files, tc.instance, tc.valid)
		schema, refused, perr := caseSchema(strict.New(), c)
		if refused != "" || perr != nil {
			t.Fatalf("%s: caseSchema = (refused %q, %v), want the widened schema", tc.why, refused, perr)
		}
		if _, ok := schema.Element(tc.declares); !ok {
			t.Errorf("%s: the assembled schema declares no %v, want the hinted document added", tc.why, tc.declares)
		}
		for _, valid := range []bool{tc.valid, !tc.valid} {
			c.expect = expectValidity(valid)
			st, why := exec(c)
			if why != "" {
				t.Errorf("%s: declined as %q under expectValid=%v, want decided", tc.why, why, valid)
			}
			if st.IsPass() != (valid == tc.valid) {
				t.Errorf("%s: under expectValid=%v the executor answered pass=%v, want it decided %s", tc.why, valid, st.IsPass(), validityWord(tc.valid))
			}
		}
	}
}

// TestGroupSchemaWithholdsCoveredHint pins the stale-hint class: a root hint
// for a namespace a document of the group's closure already has as its
// targetNamespace is NOT added (closureNamespaces), though the document it
// names would add a declaration. Each row's schema declares that declaration
// with hintsOf's covered test removed (run: both rows fail).
func TestGroupSchemaWithholdsCoveredHint(t *testing.T) {
	stale := `<xs:element name="other" type="xs:string"/>`
	for _, tc := range []struct {
		why      string
		files    []fixtureFile
		instance string
		withheld xsd.QName
	}{
		{"an xsi:schemaLocation hint for the group's own urn:a",
			[]fixtureFile{groupA, {"s.xsd", xsdDoc("urn:a", stale)}},
			`<a:known xmlns:a="urn:a" ` + xsiNS + ` xsi:schemaLocation="urn:a s.xsd">x</a:known>`,
			xsd.QName{Space: "urn:a", Local: "other"}},
		{"an xsi:noNamespaceSchemaLocation hint for a no-namespace group",
			[]fixtureFile{{"g.xsd", xsdDoc("", knownRoot)}, {"s.xsd", xsdDoc("", stale)}},
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd">x</known>`,
			xsd.QName{Local: "other"}},
	} {
		schema, refused, perr := caseSchema(strict.New(), groupedCase(t, tc.files, tc.instance, true))
		if refused != "" || perr != nil {
			t.Fatalf("%s: caseSchema = (refused %q, %v), want the group's schema", tc.why, refused, perr)
		}
		if _, ok := schema.Element(tc.withheld); ok {
			t.Errorf("%s: the assembled schema declares %v, want the covered hint withheld", tc.why, tc.withheld)
		}
	}
}

// TestGroupSchemaReadsRootHintsAlone pins that a group case reads its root's
// hints alone (uncoveredHints, #771's GAP(conformance) in instance.go): an
// instance carrying a hint below the root still has its root's uncovered hint
// added, and the hint below the root is not. Run against each mutation: with
// the below-root hint instanceHints reads appended to uncoveredHints' result,
// <o:c> is assessed against o.xsd's xs:int and decided not valid; with
// instanceHints again refusing any instance that carries a hint below its
// root, h.xsd is not added.
func TestGroupSchemaReadsRootHintsAlone(t *testing.T) {
	group := fixtureFile{"g.xsd", xsdDoc("urn:a", laxKnown)}
	files := append([]fixtureFile{group, hintB}, hintedO...)
	instance := `<a:known ` + hintPairAB + `><o:c xmlns:o="urn:o" xsi:schemaLocation="urn:o o.xsd">x</o:c></a:known>`
	exec := newInstanceExec()
	for _, valid := range []bool{true, false} {
		c := groupedCase(t, files, instance, valid)
		schema, refused, perr := caseSchema(strict.New(), c)
		if refused != "" || perr != nil {
			t.Fatalf("caseSchema = (refused %q, %v), want the widened schema", refused, perr)
		}
		if _, ok := schema.Attribute(xsd.QName{Space: "urn:b", Local: "att"}); !ok {
			t.Errorf("the assembled schema declares no {urn:b}att, want the root's uncovered hint added")
		}
		st, why := exec(c)
		if why != "" {
			t.Errorf("declined as %q under expectValid=%v, want decided valid", why, valid)
		}
		if st.IsPass() != valid {
			t.Errorf("under expectValid=%v the executor answered pass=%v, want it decided valid, <o:c> laxly assessed", valid, st.IsPass())
		}
	}
}

// TestGroupSchemaFallsBackFromHints pins that a hint that cannot be followed,
// or a widened assembly that declines or errs, never declines a group case and
// never decides it invalid (§4.3.2 clause 3); a followed hint's components are
// assessed like any other. Where the widened assembly cannot be read the
// group's own schema is used, and a hint resolving to no document is skipped.
// Each row's root is declared by groupA and valid, so the case is decided
// valid. Against each mutation, run:
//
//   - the widened assembly's error returned instead of falling back: the
//     src-import clause 3.1 row declines as schema-error.
//   - the widened assembly's decline reported instead of falling back: the
//     out-of-subset row declines as group-assembly.
//   - an unfollowed hint falling back too: the unfollowed row is charged
//     cvc-assess-elt and decided not valid, the urn:c root left undeclared.
func TestGroupSchemaFallsBackFromHints(t *testing.T) {
	exec := newInstanceExec()
	for _, tc := range []struct {
		why      string
		files    []fixtureFile
		instance string
	}{
		// s3_8_6v01, s3_8_6ii01, d4_3_16ii06.
		{"a hinted document whose targetNamespace is not the hint's (src-import clause 3.1)",
			[]fixtureFile{groupA, {"h.xsd", xsdDoc("urn:c", knownRoot)}},
			`<a:known ` + hintPairAB + `>x</a:known>`},
		// schemaShapeDecidable: a top-level child outside the XSD namespace.
		{"a hinted document outside the decidable subset",
			[]fixtureFile{groupA, {"h.xsd", xsdDoc("urn:b", `<f:x xmlns:f="urn:f"/>`)}},
			`<a:known ` + hintPairAB + `>x</a:known>`},
		{"a hint resolving to no document, beside one that resolves",
			[]fixtureFile{groupA, {"c.xsd", xsdDoc("urn:c", knownRoot)}},
			`<c:known xmlns:c="urn:c" ` + xsiNS + ` xsi:schemaLocation="urn:b missing.xsd urn:c c.xsd">x</c:known>`},
	} {
		for _, valid := range []bool{true, false} {
			st, why := exec(groupedCase(t, tc.files, tc.instance, valid))
			if why != "" {
				t.Errorf("%s: declined as %q under expectValid=%v, want decided valid", tc.why, why, valid)
			}
			if st.IsPass() != valid {
				t.Errorf("%s: under expectValid=%v the executor answered pass=%v, want it decided valid", tc.why, valid, st.IsPass())
			}
		}
	}
}

// TestGroupSchemaRejectionIgnoresHints pins that a group assembly the parser
// rejected is read as without hints, though the instance hints an uncovered
// namespace: the rejection is the group's documents' own, and execInstanceCase
// declines it as schema-error. With groupSchema's perr arm removed, run, the
// widened assembly errs too, the fallback hands out the group's assembly
// without its error, and the case declines as validator instead.
func TestGroupSchemaRejectionIgnoresHints(t *testing.T) {
	// sch-props-correct clause 2: two top-level declarations of {urn:a}known.
	group := fixtureFile{"g.xsd", xsdDoc("urn:a", knownRoot+knownRoot)}
	declinesBothPolarities(t, groupedCase(t, []fixtureFile{group, hintB}, bRoot+`x</b:known>`, true),
		"a rejected group schema with an uncovered hint", refuseSchemaError)
}
