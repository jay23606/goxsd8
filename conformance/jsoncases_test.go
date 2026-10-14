package conformance

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/validate/jsonsrc"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// curatedForTest is the committed curated corpus, or the test ends.
func curatedForTest(t *testing.T) []caseSpec {
	t.Helper()
	cases, err := curatedJSONCases(jsonCorpusIndex)
	if err != nil {
		t.Fatalf("curatedJSONCases: %v", err)
	}
	return cases
}

// TestCuratedJSONCorpus pins the corpus's shape: kindJSON cases only, with IDs
// an expectation line and the arbiter's superseded list can carry (no ',', ':'
// or white space, parseSupersededAssertions), a valid and an invalid case for
// each arm's group, and every case agreeing with its declared validity under
// the json executor — while the opposite declaration fails, so the agreement
// is the executor's observation and not a constant.
func TestCuratedJSONCorpus(t *testing.T) {
	cases := curatedForTest(t)
	if len(cases) < 6 {
		t.Fatalf("the corpus holds %d cases, want at least 6", len(cases))
	}
	exec := newJSONExec()
	polarity := map[string][2]int{}
	for _, c := range cases {
		if c.kind != kindJSON {
			t.Errorf("%s: kind %v, want kindJSON", c.id, c.kind)
		}
		if strings.ContainsFunc(c.id, func(r rune) bool { return r == ',' || r == ':' || unicode.IsSpace(r) }) {
			t.Errorf("%s: the ID carries ',', ':' or white space", c.id)
		}
		group := strings.Split(c.id, "/")[1]
		p := polarity[group]
		if c.expect.wantsValid() {
			p[0]++
		} else {
			p[1]++
		}
		polarity[group] = p
		if got, why := exec(c); !got.IsPass() {
			t.Errorf("%s: %v (refusal %q), want pass", c.id, got, why)
		}
		flipped := c
		flipped.expect = expectValidity(!c.expect.wantsValid())
		if got, _ := exec(flipped); got.IsPass() {
			t.Errorf("%s: passes declared the other way too", c.id)
		}
	}
	for _, group := range []string{"scalar", "child", "array", "attribute", "nil", "type"} {
		if p := polarity[group]; p[0] == 0 || p[1] == 0 {
			t.Errorf("group %s holds %d valid and %d invalid cases, want at least one of each", group, p[0], p[1])
		}
	}
}

// TestNoOtherLaneClaimsACuratedCase pins that the curated corpus reaches the
// json lane alone, which is what keeps every other lane's total unmoved by it:
// the read-only gate cannot see such a leak, a leaked case being New.
func TestNoOtherLaneClaimsACuratedCase(t *testing.T) {
	cases := curatedForTest(t)
	for _, l := range defaultLanes() {
		for _, c := range cases {
			if got, want := l.selects(c), l.name == "json"; got != want {
				t.Errorf("lane %s selects %s = %v, want %v", l.name, c.id, got, want)
			}
		}
	}
}

// corpusAnnotations mirrors the curated testSet's instanceTests with the one
// field the suite's structs do not read: the documentation that names an
// invalid case's expected rule.
type corpusAnnotations struct {
	Groups []struct {
		Name  string `xml:"name,attr"`
		Tests []struct {
			Name string `xml:"name,attr"`
			Doc  string `xml:"annotation>documentation"`
		} `xml:"instanceTest"`
	} `xml:"testGroup"`
}

// TestJSONCorpusChargesItsNamedRules holds each invalid case to the rule its
// documentation names ("expects <rule>"): that rule is among the
// Violations() the assessment charges it.
func TestJSONCorpusChargesItsNamedRules(t *testing.T) {
	raw, err := os.ReadFile(jsonCorpusIndex)
	if err != nil {
		t.Fatalf("reading %s: %v", jsonCorpusIndex, err)
	}
	var ann corpusAnnotations
	if err := xml.Unmarshal(raw, &ann); err != nil {
		t.Fatalf("decoding %s: %v", jsonCorpusIndex, err)
	}
	named := map[string]string{}
	for _, g := range ann.Groups {
		for _, it := range g.Tests {
			rule, ok := strings.CutPrefix(strings.TrimSpace(it.Doc), "expects ")
			if !ok {
				continue
			}
			rule, _, _ = strings.Cut(rule, ":")
			named[g.Name+"/"+it.Name] = rule
		}
	}
	backend := strict.New()
	for _, c := range curatedForTest(t) {
		if c.expect.wantsValid() {
			continue
		}
		parts := strings.Split(c.id, "/")
		rule, ok := named[parts[1]+"/"+parts[3]]
		if !ok {
			t.Errorf("%s: invalid, and its documentation names no expected rule", c.id)
			continue
		}
		schema, _, _, perr := assembleCase(backend, c.schemaDoc, c.schemaExtraDocs)
		if perr != nil {
			t.Fatalf("%s: assembling %s: %v", c.id, c.schemaDoc, perr)
		}
		v, err := validate.New(schema, backend)
		if err != nil {
			t.Fatalf("validate.New: %v", err)
		}
		f, err := os.Open(c.doc)
		if err != nil {
			t.Fatalf("%s: %v", c.id, err)
		}
		res, err := jsonsrc.Validate(v, f, jsonsrc.WithURI(c.doc))
		_ = f.Close()
		if err != nil {
			t.Fatalf("%s: jsonsrc.Validate: %v", c.id, err)
		}
		var charged []xsderr.Rule
		for _, e := range res.Violations() {
			charged = append(charged, e.Rule)
		}
		if !slices.Contains(charged, xsderr.Rule(rule)) {
			t.Errorf("%s: expects %s, charged %v", c.id, rule, charged)
		}
	}
}

// declinedSchema declares root over a {content type} xsd.Schema.ContentMatcher
// declines: two repeating sibling groups, over a and c, around a required b,
// past the matcher's region ceiling (xsd.maxPartitionStates). b's {min occurs}
// of 1 makes the empty sequence not ·valid·.
const declinedSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="root">
    <xs:complexType>
      <xs:sequence>
        <xs:sequence maxOccurs="1000"><xs:element name="a" maxOccurs="unbounded"/></xs:sequence>
        <xs:element name="b"/>
        <xs:sequence maxOccurs="1000"><xs:element name="c" maxOccurs="unbounded"/></xs:sequence>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
</xs:schema>`

// TestJSONExecutorRefusesAnUndecidedEmptySequence pins that execJSONCase banks
// no "valid" for an object-valued element with no members under a {content
// type} ContentMatcher declines: {"root":{}} is not valid against
// declinedSchema, the walk decides nothing about its empty sequence, and the
// executor refuses on the Unevaluated validate records for it (cvc-complex-type
// clause 1.4, validate's contentCheck.sequenceEnd).
func TestJSONExecutorRefusesAnUndecidedEmptySequence(t *testing.T) {
	dir := t.TempDir()
	schemaDoc, doc := filepath.Join(dir, "declined.xsd"), filepath.Join(dir, "root.json")
	if err := os.WriteFile(schemaDoc, []byte(declinedSchema), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(doc, []byte(`{"root":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c := caseSpec{id: "declined", kind: kindJSON, doc: doc, schemaDoc: schemaDoc, expect: expectValid()}
	want := refuseUnevaluated + ":cvc-complex-content"
	if got, why := newJSONExec()(c); got.IsPass() || why != want {
		t.Fatalf("execJSONCase = %v (refusal %q), want a fail refused as %q", got, why, want)
	}
}

// divergentSchema gives a wildcard's child a governing type lacking an
// attribute use of the declaration jsonsrc classifies it by. r's sequence
// takes a lax wildcard and then a local a of type base, so the first of two a
// children is ·attributed to· the wildcard and governed by the top-level a, of
// type narrow: a restriction of base prohibiting its uses n (unqualified,
// resolving no top-level declaration) and {urn:t}m (resolving one), with a
// strict {attribute wildcard} admitting both.
const divergentSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:t" xmlns:t="urn:t" elementFormDefault="qualified">
  <xs:complexType name="base">
    <xs:attribute name="n" type="xs:string"/>
    <xs:attribute ref="t:m"/>
    <xs:anyAttribute processContents="strict"/>
  </xs:complexType>
  <xs:complexType name="narrow">
    <xs:complexContent>
      <xs:restriction base="t:base">
        <xs:attribute name="n" use="prohibited"/>
        <xs:attribute ref="t:m" use="prohibited"/>
        <xs:anyAttribute processContents="strict"/>
      </xs:restriction>
    </xs:complexContent>
  </xs:complexType>
  <xs:attribute name="m" type="xs:string"/>
  <xs:element name="r">
    <xs:complexType>
      <xs:sequence>
        <xs:any processContents="lax" namespace="##targetNamespace"/>
        <xs:element name="a" type="t:base"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="a" type="t:narrow"/>
</xs:schema>`

// TestJSONExecutorRefusesDivergentAttributeGovernance pins that execJSONCase
// banks no "valid" where an attribute jsonsrc presents under one type is
// assessed under another (attributeGovernanceDiverges): each JSON document's
// assessment is an empty Result, and its XML twin is one assessedSubtreeRoot
// refuses, as strict-attribute-unresolved and as attribute-ldt.
func TestJSONExecutorRefusesDivergentAttributeGovernance(t *testing.T) {
	dir := t.TempDir()
	schemaDoc := filepath.Join(dir, "divergent.xsd")
	if err := os.WriteFile(schemaDoc, []byte(divergentSchema), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := strict.New()
	schema, report, _, perr := assembleCase(backend, schemaDoc, nil)
	if perr != nil {
		t.Fatalf("assembling divergentSchema: %v", perr)
	}
	for _, tc := range []struct {
		name, json, xml string
		gate            refusal
	}{
		{"strict wildcard, no declaration", `{"r":{"a":{"n":"x"},"a":{}}}`,
			`<r xmlns="urn:t"><a n="x"/><a/></r>`, refuseStrictAttribute},
		{"locally declared attribute type", `{"r":{"a":{"m":"x"},"a":{}}}`,
			`<r xmlns="urn:t" xmlns:t="urn:t"><a t:m="x"/><a/></r>`, refuseAttributeLDT},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, twin := filepath.Join(dir, tc.name+".json"), filepath.Join(dir, tc.name+".xml")
			if err := os.WriteFile(doc, []byte(tc.json), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(twin, []byte(tc.xml), 0o600); err != nil {
				t.Fatal(err)
			}
			if why := assessedSubtreeRoot(backend, schema, report, twin); why != tc.gate {
				t.Fatalf("assessedSubtreeRoot(%s) = %q, want %q: the twin does not have the shape the test claims", tc.xml, why, tc.gate)
			}
			c := caseSpec{id: "divergent", kind: kindJSON, doc: doc, schemaDoc: schemaDoc, expect: expectValid()}
			if got, why := newJSONExec()(c); got.IsPass() || why != refuseJSONAttributeGovernance {
				t.Fatalf("execJSONCase = %v (refusal %q), want a fail refused as %q", got, why, refuseJSONAttributeGovernance)
			}
		})
	}
}

// walkOwnedSchema reaches the attribute refusals of assessedSubtreeRoot that
// the walk, not execJSONCase, answers for. r and s take a lax wildcard and
// then a local child of type base, so the first of two such children is
// governed by the top-level declaration of its name: a, of type narrow, which
// prohibits base's n and has no {attribute wildcard}; b, of type xs:string.
const walkOwnedSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:t" xmlns:t="urn:t" elementFormDefault="qualified">
  <xs:complexType name="base">
    <xs:attribute name="n" type="xs:string"/>
  </xs:complexType>
  <xs:complexType name="narrow">
    <xs:complexContent>
      <xs:restriction base="t:base">
        <xs:attribute name="n" use="prohibited"/>
      </xs:restriction>
    </xs:complexContent>
  </xs:complexType>
  <xs:element name="r">
    <xs:complexType>
      <xs:sequence>
        <xs:any processContents="lax" namespace="##targetNamespace"/>
        <xs:element name="a" type="t:base"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="a" type="t:narrow"/>
  <xs:element name="s">
    <xs:complexType>
      <xs:sequence>
        <xs:any processContents="lax" namespace="##targetNamespace"/>
        <xs:element name="b" type="t:base"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="b" type="xs:string"/>
</xs:schema>`

// TestJSONExecutorReadsWalkOwnedAttributeRefusals pins the attribute refusals
// of assessedSubtreeRoot an attribute jsonsrc presents reaches and the walk
// charges: each JSON document's XML twin is refused as one, and execJSONCase
// reads the JSON as "not valid", the walk charging the rules the row names.
func TestJSONExecutorReadsWalkOwnedAttributeRefusals(t *testing.T) {
	dir := t.TempDir()
	schemaDoc := filepath.Join(dir, "walkowned.xsd")
	if err := os.WriteFile(schemaDoc, []byte(walkOwnedSchema), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := strict.New()
	schema, report, _, perr := assembleCase(backend, schemaDoc, nil)
	if perr != nil {
		t.Fatalf("assembling walkOwnedSchema: %v", perr)
	}
	for _, tc := range []struct {
		name, json, xml string
		gate            refusal
		charges         []xsderr.Rule
	}{
		{"attribute-unadmitted", `{"r":{"a":{"n":"x"},"a":{}}}`,
			`<r xmlns="urn:t"><a n="x"/><a/></r>`, refuseAttributeUnadmitted, []xsderr.Rule{"cvc-complex-type"}},
		{"simple-attribute", `{"s":{"b":{"n":"x"},"b":{}}}`,
			`<s xmlns="urn:t"><b n="x"/><b/></s>`, refuseSimpleAttribute, []xsderr.Rule{"cvc-type", "cvc-complex-type"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, twin := filepath.Join(dir, tc.name+".json"), filepath.Join(dir, tc.name+".xml")
			if err := os.WriteFile(doc, []byte(tc.json), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(twin, []byte(tc.xml), 0o600); err != nil {
				t.Fatal(err)
			}
			if why := assessedSubtreeRoot(backend, schema, report, twin); why != tc.gate {
				t.Fatalf("assessedSubtreeRoot(%s) = %q, want %q: the twin does not have the shape the test claims", tc.xml, why, tc.gate)
			}
			c := caseSpec{id: tc.name, kind: kindJSON, doc: doc, schemaDoc: schemaDoc, expect: expectValid()}
			if got, _ := newJSONExec()(c); got.IsPass() {
				t.Fatalf("execJSONCase = pass on a \"valid\" expectation")
			}
			c.expect = expectValidity(false)
			if got, why := newJSONExec()(c); !got.IsPass() {
				t.Fatalf("execJSONCase = %v (refusal %q) on an \"invalid\" expectation, want pass", got, why)
			}
			charged := chargedRules(t, schema, backend, doc)
			for _, r := range tc.charges {
				if !slices.Contains(charged, r) {
					t.Errorf("charged %v, want %s among them", charged, r)
				}
			}
		})
	}
}

// chargedRules is the rule IDs jsonsrc.Validate's assessment of doc against
// schema charges.
func chargedRules(t *testing.T, schema *xsd.Schema, backend value.Backend, doc string) []xsderr.Rule {
	t.Helper()
	v, err := validate.New(schema, backend)
	if err != nil {
		t.Fatalf("validate.New: %v", err)
	}
	f, err := os.Open(doc)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }() // read-only handle: close error cannot affect the test
	res, err := jsonsrc.Validate(v, f, jsonsrc.WithURI(doc))
	if err != nil {
		t.Fatalf("jsonsrc.Validate: %v", err)
	}
	var out []xsderr.Rule
	for _, e := range res.Violations() {
		out = append(out, e.Rule)
	}
	return out
}

// jsonFixture writes the schema document whose body is schemaBody and the
// JSON instance doc into a fresh directory, and returns their case.
func jsonFixture(t *testing.T, schemaBody, doc string, valid bool) caseSpec {
	t.Helper()
	dir := t.TempDir()
	schemaDoc, instance := filepath.Join(dir, "s.xsd"), filepath.Join(dir, "i.json")
	writeFixture(t, schemaDoc, `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">`+schemaBody+`</xs:schema>`)
	writeFixture(t, instance, doc)
	return caseSpec{id: "fixture", kind: kindJSON, doc: instance, schemaDoc: schemaDoc, expect: expectValidity(valid)}
}

// knownError declares a {nillable} top-level known whose {type definition} is
// ·xs:error·.
const knownError = `<xs:element name="known" type="xs:error" nillable="true"/>`

// TestJSONExecutorDeclinesNilledErrorType pins refuseJSONNilledErrorType, as
// TestInstanceExecutorDeclinesNilledErrorType pins refuseErrorType for the
// instance lane: ·xs:error· "has no valid instances" (§3.16.7.3, key-error),
// and the walk charges cvc-type clause 3.1.3 for an element it governs that is
// not ·nilled· — the control row, decided INVALID — but nothing for the
// ·nilled· element {"known":null} maps to, which clause 3.1.3 does not reach.
func TestJSONExecutorDeclinesNilledErrorType(t *testing.T) {
	exec := newJSONExec()
	if st, why := exec(jsonFixture(t, knownError, `{"known":""}`, false)); !st.IsPass() {
		t.Errorf("an empty element ·xs:error· governs: the walk charges cvc-type clause 3.1.3; the executor must agree with an invalid case (refusal %q)", why)
	}
	for _, valid := range []bool{true, false} {
		st, why := exec(jsonFixture(t, knownError, `{"known":null}`, valid))
		if st.IsPass() || why != refuseJSONNilledErrorType {
			t.Errorf("a ·nilled· element ·xs:error· governs, expectValid=%v: %v (refusal %q), want a fail refused as %q", valid, st, why, refuseJSONNilledErrorType)
		}
	}
}

// TestNillableErrorType pins where nillableErrorType finds a {nillable}
// declaration of ·xs:error·: at the top level, below a top-level
// declaration's anonymous type, in a named type, in a named model group and
// its inner model group, below the anonymous type of a group's declaration,
// and below a redefined type's declaration (its inline {base type
// definition}); and that it finds none where xs:error or {nillable} is
// missing. The two recursive rows end without a visited set: a named type
// reached again by reference, and a group reached again by <group ref>, are no
// ownership edge. Each row writes base.xsd beside its schema document;
// only the redefine row reads it.
func TestNillableErrorType(t *testing.T) {
	const (
		local     = `<xs:element name="k" type="xs:error" nillable="true" minOccurs="0"/>`
		notNil    = `<xs:element name="k" type="xs:error" minOccurs="0"/>`
		stringNil = `<xs:element name="k" type="xs:string" nillable="true" minOccurs="0"/>`
	)
	root := func(seq string) string {
		return `<xs:element name="r"><xs:complexType><xs:sequence>` + seq + `</xs:sequence></xs:complexType></xs:element>`
	}
	inner := func(name, seq string) string {
		return `<xs:element name="` + name + `" minOccurs="0"><xs:complexType><xs:sequence>` + seq + `</xs:sequence></xs:complexType></xs:element>`
	}
	for _, tc := range []struct {
		why, schemaBody string
		want            bool
	}{
		{"top level", knownError, true},
		{"top level, not nillable", `<xs:element name="known" type="xs:error"/>`, false},
		{"local, in a top-level declaration's type", root(local), true},
		{"local, not nillable", root(notNil), false},
		{"local, nillable but not xs:error", root(stringNil), false},
		{"two anonymous types down", root(inner("a", inner("b", local))), true},
		{"in a named type", `<xs:complexType name="T"><xs:sequence>` + inner("a", local) + `</xs:sequence></xs:complexType>`, true},
		{"in a named group", `<xs:group name="G"><xs:sequence>` + local + `</xs:sequence></xs:group>` + root(`<xs:group ref="G"/>`), true},
		{"in a named group's inner choice", `<xs:group name="G"><xs:sequence><xs:choice>` + local + `</xs:choice></xs:sequence></xs:group>`, true},
		{"below a group's declaration", `<xs:group name="G"><xs:sequence>` + inner("a", local) + `</xs:sequence></xs:group>` + root(`<xs:group ref="G"/>`), true},
		{
			"below a named base's declaration, under an anonymous extension",
			`<xs:complexType name="B"><xs:sequence>` + inner("x", local) + `</xs:sequence></xs:complexType>` +
				`<xs:element name="r"><xs:complexType><xs:complexContent><xs:extension base="B"/></xs:complexContent></xs:complexType></xs:element>`,
			true,
		},
		{
			"a named type holding itself by reference",
			`<xs:complexType name="B"><xs:sequence>` + inner("x", `<xs:element name="y" type="B" minOccurs="0"/>`+stringNil) +
				`</xs:sequence></xs:complexType><xs:element name="r" type="B"/>`,
			false,
		},
		{
			"a group's declaration whose type holds the group again, without xs:error",
			`<xs:group name="G"><xs:sequence>` + inner("a", `<xs:group ref="G" minOccurs="0"/>`) + `</xs:sequence></xs:group>` +
				root(`<xs:group ref="G"/>`),
			false,
		},
		{
			"below the redefined type's declaration, under its redefining extension",
			`<xs:redefine schemaLocation="base.xsd"><xs:complexType name="T"><xs:complexContent>` +
				`<xs:extension base="T"/></xs:complexContent></xs:complexType></xs:redefine>`,
			true,
		},
	} {
		c := jsonFixture(t, tc.schemaBody, `{}`, true)
		writeFixture(t, filepath.Join(filepath.Dir(c.schemaDoc), "base.xsd"),
			`<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"><xs:complexType name="T"><xs:sequence>`+
				inner("a", local)+`</xs:sequence></xs:complexType></xs:schema>`)
		schema, _, decidable, err := assembleCase(strict.New(), c.schemaDoc, nil)
		if err != nil || !decidable {
			t.Fatalf("%s: assembling the schema: decidable %v, err %v", tc.why, decidable, err)
		}
		if got := nillableErrorType(schema); got != tc.want {
			t.Errorf("%s: nillableErrorType = %v, want %v", tc.why, got, tc.want)
		}
	}
}

// wildcardAttributeSchemas give a wildcard-named attribute jsonsrc presents at
// a LOCAL element its own declaration governs each attribute refusal of
// assessedSubtreeRoot. Each is r with one optional local child, under a
// ##targetNamespace {attribute wildcard}: in ldt, c of type narrow, a
// restriction of base prohibiting base's use of the top-level {urn:t}m, the
// wildcard lax; in strict, s, the wildcard strict, and q resolving no
// top-level declaration; in unadmitted, e, the wildcard lax and rejecting
// {urn:t}p by name.
var wildcardAttributeSchemas = map[string]string{
	"ldt": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:t" xmlns:t="urn:t" elementFormDefault="qualified">
  <xs:complexType name="base">
    <xs:attribute ref="t:m"/>
    <xs:anyAttribute namespace="##targetNamespace" processContents="lax"/>
  </xs:complexType>
  <xs:complexType name="narrow">
    <xs:complexContent>
      <xs:restriction base="t:base">
        <xs:attribute ref="t:m" use="prohibited"/>
        <xs:anyAttribute namespace="##targetNamespace" processContents="lax"/>
      </xs:restriction>
    </xs:complexContent>
  </xs:complexType>
  <xs:attribute name="m" type="xs:string"/>
  <xs:element name="r">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="c" type="t:narrow" minOccurs="0"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
</xs:schema>`,
	"strict": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:t" xmlns:t="urn:t" elementFormDefault="qualified">
  <xs:element name="r">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="s" minOccurs="0">
          <xs:complexType>
            <xs:anyAttribute namespace="##targetNamespace" processContents="strict"/>
          </xs:complexType>
        </xs:element>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
</xs:schema>`,
	"unadmitted": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:t" xmlns:t="urn:t" elementFormDefault="qualified">
  <xs:element name="r">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="e" minOccurs="0">
          <xs:complexType>
            <xs:anyAttribute namespace="##targetNamespace" notQName="t:p" processContents="lax"/>
          </xs:complexType>
        </xs:element>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:attribute name="p" type="xs:string"/>
</xs:schema>`,
}

// TestJSONExecutorReadsWildcardAttributes pins the attribute refusals of
// assessedSubtreeRoot a wildcard-named attribute reaches at an element its own
// local declaration governs: each JSON document's XML twin is refused as the
// row's gate, and execJSONCase either refuses a "valid" reading as
// json-attribute-governance (no charges: the walk charges nothing for
// strict-attribute-unresolved or attribute-ldt) or reads "not valid" off the
// rules the walk charges (attribute-unadmitted).
func TestJSONExecutorReadsWildcardAttributes(t *testing.T) {
	backend := strict.New()
	for _, tc := range []struct {
		schema, json, xml string
		gate              refusal
		charges           []xsderr.Rule
	}{
		{"ldt", `{"r":{"c":{"m":"x"}}}`,
			`<r xmlns="urn:t" xmlns:t="urn:t"><c t:m="x"/></r>`, refuseAttributeLDT, nil},
		{"strict", `{"r":{"s":{"q":"x"}}}`,
			`<r xmlns="urn:t" xmlns:t="urn:t"><s t:q="x"/></r>`, refuseStrictAttribute, nil},
		{"unadmitted", `{"r":{"e":{"p":"x"}}}`,
			`<r xmlns="urn:t" xmlns:t="urn:t"><e t:p="x"/></r>`, refuseAttributeUnadmitted, []xsderr.Rule{"cvc-complex-type"}},
	} {
		t.Run(tc.schema, func(t *testing.T) {
			dir := t.TempDir()
			schemaDoc, doc, twin := filepath.Join(dir, "s.xsd"), filepath.Join(dir, "i.json"), filepath.Join(dir, "i.xml")
			writeFixture(t, schemaDoc, wildcardAttributeSchemas[tc.schema])
			writeFixture(t, doc, tc.json)
			writeFixture(t, twin, tc.xml)
			schema, report, _, perr := assembleCase(backend, schemaDoc, nil)
			if perr != nil {
				t.Fatalf("assembling %s: %v", tc.schema, perr)
			}
			if why := assessedSubtreeRoot(backend, schema, report, twin); why != tc.gate {
				t.Fatalf("assessedSubtreeRoot(%s) = %q, want %q: the twin does not have the shape the test claims", tc.xml, why, tc.gate)
			}
			if charged := chargedRules(t, schema, backend, doc); !slices.Equal(charged, tc.charges) {
				t.Fatalf("jsonsrc.Validate charges %v, want %v", charged, tc.charges)
			}
			c := caseSpec{id: tc.schema, kind: kindJSON, doc: doc, schemaDoc: schemaDoc, expect: expectValid()}
			got, why := newJSONExec()(c)
			if got.IsPass() {
				t.Fatalf("execJSONCase = pass on a \"valid\" expectation")
			}
			if tc.charges == nil && why != refuseJSONAttributeGovernance {
				t.Fatalf("execJSONCase refusal = %q, want %q", why, refuseJSONAttributeGovernance)
			}
			if tc.charges == nil {
				return
			}
			c.expect = expectValidity(false)
			if got, why := newJSONExec()(c); !got.IsPass() {
				t.Fatalf("execJSONCase = %v (refusal %q) on an \"invalid\" expectation, want pass", got, why)
			}
		})
	}
}

// instanceTypeWildcardSchema declares r of type base, which has no {attribute
// wildcard}, and the named type wild, an extension of base that adds a strict
// one of the target namespace; no declaration carries wild, so only a
// "$type" reaches it.
const instanceTypeWildcardSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:t" xmlns:t="urn:t" elementFormDefault="qualified">
  <xs:complexType name="base"/>
  <xs:complexType name="wild">
    <xs:complexContent>
      <xs:extension base="t:base">
        <xs:anyAttribute namespace="##targetNamespace" processContents="strict"/>
      </xs:extension>
    </xs:complexContent>
  </xs:complexType>
  <xs:element name="r" type="t:base"/>
</xs:schema>`

// TestJSONExecutorRefusesInstanceTypeWildcard pins attributeGovernanceDiverges
// over the named types a "$type" can name: wild ·overrides· base and governs
// r, jsonsrc names q by wild's {attribute wildcard}, and the walk charges
// nothing for the strict-attribute-unresolved twin, so the "valid" reading is
// refused though no declaration's type has an {attribute wildcard}.
func TestJSONExecutorRefusesInstanceTypeWildcard(t *testing.T) {
	backend := strict.New()
	dir := t.TempDir()
	schemaDoc, doc, twin := filepath.Join(dir, "s.xsd"), filepath.Join(dir, "i.json"), filepath.Join(dir, "i.xml")
	writeFixture(t, schemaDoc, instanceTypeWildcardSchema)
	writeFixture(t, doc, `{"r":{"$xmlns":{"t":"urn:t"},"$type":"t:wild","q":"x"}}`)
	writeFixture(t, twin, `<r xmlns="urn:t" xmlns:t="urn:t" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:type="t:wild" t:q="x"/>`)
	schema, report, _, perr := assembleCase(backend, schemaDoc, nil)
	if perr != nil {
		t.Fatalf("assembling instanceTypeWildcardSchema: %v", perr)
	}
	if why := assessedSubtreeRoot(backend, schema, report, twin); why != refuseStrictAttribute {
		t.Fatalf("assessedSubtreeRoot = %q, want %q: the twin does not have the shape the test claims", why, refuseStrictAttribute)
	}
	if charged := chargedRules(t, schema, backend, doc); charged != nil {
		t.Fatalf("jsonsrc.Validate charges %v, want none", charged)
	}
	c := caseSpec{id: "instance-type-wildcard", kind: kindJSON, doc: doc, schemaDoc: schemaDoc, expect: expectValid()}
	if got, why := newJSONExec()(c); got.IsPass() || why != refuseJSONAttributeGovernance {
		t.Fatalf("execJSONCase = %v (refusal %q), want a fail refused as %q", got, why, refuseJSONAttributeGovernance)
	}
}
