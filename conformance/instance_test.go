package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/validate/xmlsrc"
	"github.com/kud360/goxsd8/xsderr"
)

// knownRoot is the one top-level element declaration most cases below assemble,
// so an instance rooted at <known> is DECLARED and one rooted at anything else is
// not — the dispatch the whole lane turns on.
const knownRoot = `<xs:element name="known" type="xs:string"/>`

// instanceCase writes one schema document and one instance document into a fresh
// directory and returns the caseSpec discovery builds for an instanceTest of that
// pair: the instance is the document under test, the schema is the group's.
// schemaBody is wrapped in a no-targetNamespace <schema>, so an instance root in
// no namespace is the one that can resolve to a declaration.
func instanceCase(t *testing.T, schemaBody, instance string, valid bool) caseSpec {
	t.Helper()
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "s.xsd")
	instancePath := filepath.Join(dir, "i.xml")
	writeFixture(t, schemaPath, `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">`+schemaBody+`</xs:schema>`)
	writeFixture(t, instancePath, instance)
	return caseSpec{
		kind:      kindInstance,
		doc:       instancePath,
		schemaDoc: schemaPath,
		expect:    expectValidity(valid),
	}
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// declinesBothPolarities asserts the instance lane's executor records Fail for
// c AND for c with its declared outcome flipped — the exact test declines.go's
// census applies, and the only honest reading of "no verdict was reached" —
// naming the refusal want under both (#2008). Asserting one polarity would
// pass for an executor that decided the case wrongly; asserting no refusal
// would pass for one that attributed the decline to the wrong exit.
func declinesBothPolarities(t *testing.T, c caseSpec, why string, want refusal) {
	t.Helper()
	exec := newInstanceExec()
	for _, valid := range []bool{true, false} {
		c.expect = expectValidity(valid)
		st, got := exec(c)
		if st.IsPass() {
			t.Errorf("%s: must Fail (decline) regardless of expectValid=%v", why, valid)
		}
		if got != want {
			t.Errorf("%s: declined as %q under expectValid=%v, want %q", why, got, valid, want)
		}
	}
}

// status is e's Status alone, for a test asserting what the executor decided.
func (e laneExecutor) status() executor {
	return func(c caseSpec) Status {
		st, _ := e(c)
		return st
	}
}

// TestInstanceExecutorDecidesUndeclaredRoot proves the one verdict this
// bring-up slice really reaches: a validation root with no top-level element
// declaration and no xsi:type determines neither a ·governing element
// declaration· nor a ·governing type definition· (cvc-assess-elt, §3.3.4.6), so
// under §5.2 ·strict wildcard validation· the document is NOT VALID whatever
// else it holds. The executor must agree with a suite-invalid case and disagree
// with a suite-valid one — never pass under both, which is what a decline looks
// like.
func TestInstanceExecutorDecidesUndeclaredRoot(t *testing.T) {
	exec := newInstanceExec().status()
	if !exec(instanceCase(t, knownRoot, `<unknown/>`, false)).IsPass() {
		t.Error("an undeclared root is not valid: the executor must agree with a suite-invalid case")
	}
	if exec(instanceCase(t, knownRoot, `<unknown/>`, true)).IsPass() {
		t.Error("the executor must Fail under a flipped expectation (it decides for real)")
	}
}

// TestInstanceExecutorDecidesAbstractRoot proves the slice's SECOND verdict is
// reachable from a schema DOCUMENT: a root whose declaration has {abstract} true
// is locally invalid by cvc-elt clause 2 (§3.3.4.3), so e-validity clause 1.1.1.1
// fails and the document is NOT VALID whatever else it holds. It decides nothing
// unless producer.produceElement maps {abstract} off the attribute (#761).
func TestInstanceExecutorDecidesAbstractRoot(t *testing.T) {
	exec := newInstanceExec().status()
	const abstractRoot = `<xs:element name="known" type="xs:string" abstract="true"/>`
	if !exec(instanceCase(t, abstractRoot, `<known/>`, false)).IsPass() {
		t.Error("an abstract root is not valid: the executor must agree with a suite-invalid case")
	}
	if exec(instanceCase(t, abstractRoot, `<known/>`, true)).IsPass() {
		t.Error("the executor must Fail under a flipped expectation (it decides for real)")
	}
	// The control that the verdict turns on {abstract} and not on the shape of the
	// instance is TestInstanceExecutorDecidesContentLessRoot: the same document
	// under a non-abstract declaration charges nothing and is decided VALID.
}

// TestInstanceExecutorDecidesXsiTypedUndeclaredRoot proves a root with no
// top-level declaration whose xsi:type ·resolves· is decided against that type
// (#2156): it is the root's ·governing type definition· (key-governing-type-elem
// clause 8), the root is ·strictly assessed· against it (cvc-assess-elt clause
// 1), so it is NOT the undeclared-root case above, and cvc-type decides it. An
// xsi:nil on it has no effect, whatever its value: key-nilled needs a
// declaration whose {nillable} is true, so cvc-type clause 3.1.3 reads its empty
// content against xs:int as if xsi:nil were absent. Every valid row declines as
// undeclared-root with subtreeGate.undeclaredRoot refusing unconditionally.
func TestInstanceExecutorDecidesXsiTypedUndeclaredRoot(t *testing.T) {
	const schemaBody = knownRoot + `<xs:complexType name="T"/>` +
		`<xs:simpleType name="I"><xs:restriction base="xs:int"/></xs:simpleType>`
	exec := newInstanceExec()
	for _, tc := range []struct {
		why, instance string
		valid         bool
	}{
		{"an undeclared root typed by a resolved complex xsi:type",
			`<unknown ` + xsiNS + ` xsi:type="T"/>`, true},
		{"an undeclared root typed by a resolved simple xsi:type",
			`<unknown ` + xsiNS + ` xsi:type="I">1</unknown>`, true},
		{"an undeclared root whose content its simple xsi:type rejects",
			`<unknown ` + xsiNS + ` xsi:type="I">x</unknown>`, false},
		{"an xsi:nil true on an undeclared root, its content valid",
			`<unknown ` + xsiNS + ` xsi:type="I" xsi:nil="true">1</unknown>`, true},
		{"an xsi:nil true on an undeclared root does not nil its empty content",
			`<unknown ` + xsiNS + ` xsi:type="I" xsi:nil="true"/>`, false},
	} {
		for _, expect := range []bool{true, false} {
			st, why := exec(instanceCase(t, schemaBody, tc.instance, expect))
			if why != "" {
				t.Errorf("%s: declined as %q under expectValid=%v, want decided %s", tc.why, why, expect, validityWord(tc.valid))
			}
			if st.IsPass() != (expect == tc.valid) {
				t.Errorf("%s: under expectValid=%v the executor answered pass=%v, want it decided %s", tc.why, expect, st.IsPass(), validityWord(tc.valid))
			}
		}
	}
}

// TestInstanceExecutorDeclinesUndecidableShapes proves every shape this slice
// cannot decide is DECLINED in BOTH directions rather than guessed.
func TestInstanceExecutorDeclinesUndecidableShapes(t *testing.T) {
	cases := []struct {
		why        string
		schemaBody string
		instance   string
		refused    refusal
	}{
		{
			"an instance document the reader rejects is a gap, not a well-formedness verdict",
			knownRoot, `<unknown`, refuseInstanceUnread,
		},
		{
			// The witness must be a shape the producer BUILDS WITHOUT ERROR, or
			// perr alone declines the case and the row stops testing the
			// decidable-subset gate it names. An <override> child carrying no
			// name= is one: §F.2 clause 1 makes it a top-level declaration of the
			// overridden document, so overrideDecidable declines it, while the
			// producer neither rejects nor substitutes it: the directive names a
			// document that does not resolve, and src-override clause 1 is
			// guarded on the schemaLocation resolving, so ParseReport returns a
			// nil error and only !decidable can decline. The instance is one the
			// executor DOES decide once a schema is built (an undeclared root
			// charges cvc-assess-elt), so disabling the gate makes this row FAIL
			// rather than fall through to another decline reason.
			"a schema document outside the producer's decidable subset (an <override> child with no name=)",
			knownRoot + `<xs:override schemaLocation="lib.xsd">` +
				`<xs:element type="xs:string"/>` +
				`</xs:override>`,
			`<unknown/>`,
			refuseGroupAssembly,
		},
		{
			"a schema the assembly REJECTED is not the schema the suite declared",
			knownRoot + `<xs:simpleType name="T"><xs:restriction base="xs:string"/></xs:simpleType>` +
				`<xs:simpleType name="T"><xs:restriction base="xs:int"/></xs:simpleType>`,
			`<unknown/>`,
			refuseSchemaError,
		},
	}
	for _, tc := range cases {
		declinesBothPolarities(t, instanceCase(t, tc.schemaBody, tc.instance, false), tc.why, tc.refused)
	}

	// The non-<schema> schema document needs a root instanceCase's wrapper cannot
	// produce, so it is written out here rather than joining the table.
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "s.xsd")
	instancePath := filepath.Join(dir, "i.xml")
	writeFixture(t, schemaPath, `<notaschema/>`)
	writeFixture(t, instancePath, `<unknown/>`)
	declinesBothPolarities(t, caseSpec{kind: kindInstance, doc: instancePath, schemaDoc: schemaPath},
		"a schema document that is not <schema>-rooted", refuseGroupAssembly)
}

// TestInstanceExecutorDeclinesCaseWithNoGroupSchema proves a case with no
// group schema reference (groupSchemaDocs) and no hint on its root is DECLINED
// wherever the built-in components alone leave its root ·laxly assessed· and
// its [validity] notKnown (#2013, #2151), each arm under builtinsSchema's own
// refusal, in caseSchema and in the executor. Handed the built-ins schema,
// every row is decided "not valid" on the walk's cvc-assess-elt charge for a
// root with no top-level declaration and no resolving xsi:type, a ·strict
// assessment· the spec does not make there. TestInstanceExecutorDecidesBuiltinTypedRoot
// pins the shape that is decided.
func TestInstanceExecutorDeclinesCaseWithNoGroupSchema(t *testing.T) {
	const xsdNS = `xmlns:xsd="http://www.w3.org/2001/XMLSchema"`
	for _, tc := range []struct {
		why      string
		instance string
		refused  refusal
	}{
		{"a root with no xsi:type", `<unknown/>`, refuseNoHint},
		// xsi:nil is one of §3.2.7's four, and a plain attribute is in no
		// namespace: neither is unknownXsi.
		{"a root carrying xsi:nil and no xsi:type",
			`<unknown ` + xsiNS + ` xsi:nil="false"/>`, refuseNoHint},
		{"a root carrying a no-namespace attribute and no xsi:type",
			`<unknown blah="x"/>`, refuseNoHint},
		// addB199: key-itd clause 3 fails, cvc-assess-elt clause 3.
		{"a root whose xsi:type names no built-in type",
			`<a ` + xsiNS + ` ` + xsdNS + ` xsi:type="xsd:str&#x0400;ing">x</a>`, refuseNoHintXsiTypeUnresolved},
		{"a root whose xsi:type prefix no declaration binds",
			`<a ` + xsiNS + ` xsi:type="xsd:int">1</a>`, refuseNoHintXsiTypeUnresolved},
		// An unprefixed name with no default namespace declaration is in no
		// namespace, where no built-in type is.
		{"a root whose xsi:type is unprefixed, with no default namespace",
			`<a ` + xsiNS + ` xsi:type="int">1</a>`, refuseNoHintXsiTypeUnresolved},
		// attMd001-attMd011: cvc-complex-type clause 2 excepts the four names
		// case-sensitively; xs:anyType's lax wildcard admits the rest.
		{"a root carrying xsi:Type and no xsi:type",
			`<doc ` + xsiNS + ` ` + xsdNS + ` xsi:Type="xsd:int">1</doc>`, refuseNoHintUnknownXsi},
		{"a root carrying xsi:blah and no xsi:type",
			`<doc ` + xsiNS + ` xsi:blah="foo.xsd">abc</doc>`, refuseNoHintUnknownXsi},
	} {
		c := hintedCase(t, nil, tc.instance, false)
		declinesBothPolarities(t, c, tc.why, tc.refused)
		if _, why, _ := caseSchema(strict.New(), c); why != tc.refused {
			t.Errorf("%s: caseSchema refused it as %q, want %q", tc.why, why, tc.refused)
		}
	}
}

// abstractRootValidator is a validator over a schema whose one top-level element
// declaration has {abstract} true. It assembles that schema from a DOCUMENT,
// through the same gate the lane itself uses, because the producer maps
// {abstract} from the attribute (§3.3.2.1 dcl.elt.common, #761) — so the cvc-elt
// branch this exercises is the one a real case reaches, not a hand-built
// declaration's.
func abstractRootValidator(t *testing.T) *validate.Validator {
	t.Helper()
	schemaPath := filepath.Join(t.TempDir(), "s.xsd")
	writeFixture(t, schemaPath, `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">`+
		`<xs:element name="e" type="xs:string" abstract="true"/></xs:schema>`)
	schema, _, _, err := assembleCase(strict.New(), schemaPath, nil)
	if err != nil {
		t.Fatalf("assembling the schema: %v", err)
	}
	v, err := validate.New(schema, strict.New())
	if err != nil {
		t.Fatalf("validate.New: %v", err)
	}
	return v
}

// TestAssessInstanceDeclinesFaultedWalk proves the validate.Result.Err gate is
// load-bearing rather than decorative. An abstract root is the one Assess branch
// that charges a violation and STILL walks the subtree, so a source fault below it
// yields a Result that carries a decidable violation AND stopped early. Reading
// that as a verdict would score a document the walk never finished reading.
//
// The well-formed row is the control: the same schema and the same charge, with
// the walk reaching the end, is accepted and IS decidable — so the faulted row
// fails for the fault and not for the shape.
func TestAssessInstanceDeclinesFaultedWalk(t *testing.T) {
	v := abstractRootValidator(t)
	dir := t.TempDir()

	whole := filepath.Join(dir, "whole.xml")
	writeFixture(t, whole, `<e><a/></e>`)
	result, why := assessInstance(v, whole)
	if why != "" {
		t.Fatalf("a walk that reached the end of the document must be accepted, refused as %q", why)
	}
	if !decidedNotValid(result.Violations()) {
		t.Errorf("an abstract root charges cvc-elt clause 2, which is decidable; got %d violation(s)", len(result.Violations()))
	}

	faulted := filepath.Join(dir, "faulted.xml")
	writeFixture(t, faulted, `<e><a></b></e>`)
	if _, why := assessInstance(v, faulted); why != refuseWalkStopped {
		t.Errorf("a walk stopped by a source fault mid-document must be DECLINED, whatever it charged before stopping: refused as %q, want %q", why, refuseWalkStopped)
	}
}

// TestDecidedNotValidEnumeratesTheDecidableCharges pins the gate itself: a
// non-empty violation set whose every rule is one Assess charges is evidence the
// document is not valid, and every other shape declines rather than being read as
// a verdict a later, wider Assess might charge under an approximation.
func TestDecidedNotValidEnumeratesTheDecidableCharges(t *testing.T) {
	charge := func(rule xsderr.Rule) *xsderr.Error {
		return xsderr.New(rule, xsderr.Loc{}, "charged")
	}
	cases := []struct {
		name       string
		violations []*xsderr.Error
		want       bool
	}{
		{"no violation at all", nil, false},
		{"cvc-assess-elt alone", []*xsderr.Error{charge(ruleCvcAssessElt)}, true},
		{"cvc-elt alone", []*xsderr.Error{charge(ruleCvcElt)}, true},
		{"cvc-type alone", []*xsderr.Error{charge(ruleCvcType)}, true},
		{"cvc-complex-type alone", []*xsderr.Error{charge(ruleCvcComplexType)}, true},
		{"cvc-attribute alone", []*xsderr.Error{charge(ruleCvcAttribute)}, true},
		{"cvc-au alone", []*xsderr.Error{charge(ruleCvcAu)}, true},
		{"cvc-assertion alone", []*xsderr.Error{charge(ruleCvcAssertion)}, true},
		{"cvc-assertions-valid alone", []*xsderr.Error{charge(ruleCvcAssertionsValid)}, true},
		{"a rule outside the enumeration", []*xsderr.Error{charge("cvc-datatype-valid")}, false},
		{"two charges, both enumerated", []*xsderr.Error{charge(ruleCvcAttribute), charge(ruleCvcAu)}, true},
		{"one enumerated, one not", []*xsderr.Error{charge(ruleCvcElt), charge("cvc-datatype-valid")}, false},
	}
	for _, tc := range cases {
		if got := decidedNotValid(tc.violations); got != tc.want {
			t.Errorf("%s: decidedNotValid = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestInstanceExecutorDecidesEvaluatedAssertions proves the lane reads
// cvc-assertion as a verdict (#2232): an {assertions} member whose {test}
// validate evaluates decides the case both ways, at the root and below it —
// "not valid" where the {test} is false or raises a type error, "valid" where
// it holds. With cvc-assertion outside decidableRules the invalid rows decline
// as undecided-rule and this test fails.
func TestInstanceExecutorDecidesEvaluatedAssertions(t *testing.T) {
	exec := newInstanceExec().status()
	const typed = `<xs:complexType name="T"><xs:attribute name="x" type="xs:integer"/><xs:attribute name="b" type="xs:boolean"/>` +
		`<xs:assert test="@x > 300"/></xs:complexType>`
	const raises = `<xs:complexType name="T"><xs:attribute name="b" type="xs:boolean"/><xs:assert test="@b = 'true'"/></xs:complexType>`
	for _, tc := range []struct {
		why, schemaBody, instance string
		valid                     bool
	}{
		{"a false {test} at the root", `<xs:element name="known" type="T"/>` + typed, `<known x="200"/>`, false},
		{"a {test} raising err:XPTY0004 at the root", `<xs:element name="known" type="T"/>` + raises, `<known b="true"/>`, false},
		{
			"a false {test} below the root",
			`<xs:element name="known"><xs:complexType><xs:sequence><xs:element name="a" type="T"/></xs:sequence></xs:complexType></xs:element>` + typed,
			`<known><a x="1"/></known>`, false,
		},
		{"a true {test} at the root", `<xs:element name="known" type="T"/>` + typed, `<known x="500"/>`, true},
	} {
		if !exec(instanceCase(t, tc.schemaBody, tc.instance, tc.valid)).IsPass() {
			t.Errorf("%s: the executor must agree with a suite-%s case", tc.why, map[bool]string{true: "valid", false: "invalid"}[tc.valid])
		}
		if exec(instanceCase(t, tc.schemaBody, tc.instance, !tc.valid)).IsPass() {
			t.Errorf("%s: the executor must Fail under a flipped expectation", tc.why)
		}
	}
}

// TestParsedAnonymousExtensionIsNotFalselyRejected proves the shape whose two
// attribute properties are folded through the OWNING SLOT alone (#414) is
// reachable from a PARSED document and not only from a hand-assembled
// xsd.Schema: produceComplexType dispatches on the
// <complexContent>/<simpleContent> children before it considers whether the type
// is named, so an inline <complexContent><extension> under an <element> is
// produced exactly as a top-level one is.
//
// The document below is VALID — @fromBase is the base's own attribute use and
// @own the extension's — so the assessment has to get both of the anonymous
// type's folded attribute properties right or charge cvc-complex-type clause 2
// for an attribute the base declares. This test lives here rather than in
// validate because it needs the parser, which validate's import closure excludes
// (validate/imports_test.go); it goes through parser.Parse and xmlsrc.Validate
// directly rather than through the lane executor, since assembleCase's
// decidability gate declines this schema before validate ever sees it — which is
// why no suite case scores this shape either way.
func TestParsedAnonymousExtensionIsNotFalselyRejected(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, filepath.Join(dir, "s.xsd"), `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
	  <xs:complexType name="B">
	    <xs:attribute name="fromBase" type="xs:string"/>
	    <xs:anyAttribute namespace="##any"/>
	  </xs:complexType>
	  <xs:element name="root"><xs:complexType><xs:complexContent>
	    <xs:extension base="B"><xs:attribute name="own" type="xs:string"/></xs:extension>
	  </xs:complexContent></xs:complexType></xs:element>
	  <xs:element name="plain"><xs:complexType>
	    <xs:attribute name="a" type="xs:string" use="required"/>
	  </xs:complexType></xs:element>
	</xs:schema>`)
	schema, err := parser.Parse("s.xsd", parser.WithResolver(loader.Dir(dir)), parser.WithBackend(strict.New()))
	if err != nil {
		t.Fatalf("parsing the schema: %v", err)
	}
	v, err := validate.New(schema, strict.New())
	if err != nil {
		t.Fatalf("validate.New: %v", err)
	}
	assess := func(instance string) []*xsderr.Error {
		t.Helper()
		result, err := xmlsrc.Validate(v, strings.NewReader(instance))
		if err != nil {
			t.Fatalf("xmlsrc.Validate(%s): %v", instance, err)
		}
		if result.Err() != nil {
			t.Fatalf("Err() = %v, want nil", result.Err())
		}
		return result.Violations()
	}
	if got := assess(`<root fromBase="x" own="y"/>`); len(got) != 0 {
		t.Errorf("Violations() = %v, want none: the anonymous extension's {attribute uses} hold both @fromBase and @own once §3.4.2.4 clause 3 has folded them, so cvc-complex-type clause 2 matches each of them to a use", got)
	}
	// The control, over an anonymous type of the other shape: the sibling
	// implicit-content one IS a restriction of xs:anyType, both folds are the
	// identity on it, and it charges its own {required} use — so the assertion
	// above is about the FOLDS and not about which anonymous shapes the lane
	// admits, which since #1126 is all of them (conformance/schema.go's
	// complexTypeDecidable).
	if got := assess(`<plain/>`); len(got) != 1 {
		t.Fatalf("Violations() = %v, want exactly one: an implicit-content anonymous type is still assessed", got)
	}
}

// TestInstanceExecutorAgreesWithSuite drives the real executor over a real suite
// instanceTest fixture and its real sibling schema, so the discovery-side wiring
// and the executor are proved together on the shape the lane actually meets.
// Skips when the submodule is absent.
func TestInstanceExecutorAgreesWithSuite(t *testing.T) {
	skipWithoutSuite(t)
	exec := newInstanceExec().status()
	dir := filepath.Join(suiteRoot, "sunData", "ElemDecl", "typeDef", "typeDef00201m")
	c := caseSpec{
		kind:      kindInstance,
		doc:       filepath.Join(dir, "typeDef00201m1_p.xml"),
		schemaDoc: filepath.Join(dir, "typeDef00201m.xsd"),
		expect:    expectValid(),
	}
	// The root is an assessed subtree root the walk charges nothing for, so the case is
	// decided valid, and a flipped expectation disagrees.
	if !exec(c).IsPass() {
		t.Error("a declared assessed subtree root against a real suite schema: the executor must agree with the suite-valid case")
	}
	c.expect = expectValidity(false)
	if exec(c).IsPass() {
		t.Error("the executor must Fail under a flipped expectation (it decides for real)")
	}
}
