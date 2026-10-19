package conformance

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
)

// fixtureFile is one document a hintedCase writes beside its instance.
type fixtureFile struct{ name, content string }

// hintedCase writes files and the instance document into a fresh directory and
// returns the caseSpec discovery builds for an instanceTest whose group
// declares no schemaTest: no group schema, so any schema comes from the
// instance's own hints (#2013).
func hintedCase(t *testing.T, files []fixtureFile, instance string, valid bool) caseSpec {
	t.Helper()
	dir := t.TempDir()
	for _, f := range files {
		writeFixture(t, filepath.Join(dir, f.name), f.content)
	}
	instancePath := filepath.Join(dir, "i.xml")
	writeFixture(t, instancePath, instance)
	return caseSpec{kind: kindInstance, doc: instancePath, expect: expectValidity(valid)}
}

// xsdDoc wraps body in a <schema>, with targetNamespace tns unless tns is "".
func xsdDoc(tns, body string) string {
	open := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"`
	if tns != "" {
		open += ` targetNamespace="` + tns + `"`
	}
	return open + `>` + body + `</xs:schema>`
}

// skipKnown declares <known> with one child of any name, ·skipped· by a skip
// Wildcard, so whatever that child carries the walk reads none of it.
const skipKnown = `<xs:element name="known"><xs:complexType><xs:sequence>` +
	`<xs:any processContents="skip"/></xs:sequence></xs:complexType></xs:element>`

// TestInstanceExecutorDecidesFromHints proves a case with no group schema is
// decided against the schema its root's hints locate (§4.3.2 clauses 3-5,
// #2013), in both directions and through both hint attributes. Each row
// declines with caseSchema's hint arm removed, as every no-group-schema case
// did before it.
func TestInstanceExecutorDecidesFromHints(t *testing.T) {
	exec := newInstanceExec().status()
	noNS := []fixtureFile{{"s.xsd", xsdDoc("", knownRoot)}}
	withNS := []fixtureFile{{"s.xsd", xsdDoc("urn:t", knownRoot)}}
	for _, tc := range []struct {
		why      string
		files    []fixtureFile
		instance string
		valid    bool
	}{
		{"noNamespaceSchemaLocation, a valid leaf root", noNS,
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd">x</known>`, true},
		// cvc-type clause 3.1.2: an element [[child]] under a simple type.
		{"noNamespaceSchemaLocation, a leaf root with an element child", noNS,
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd"><x/></known>`, false},
		// Namespaces in XML 1.1 lets a 1.1 document undeclare a prefix, so the
		// closure is read and decided like any other.
		{"noNamespaceSchemaLocation, an XML 1.1 schema undeclaring a prefix",
			[]fixtureFile{{"s.xsd", `<?xml version="1.1"?><xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:f="">` + knownRoot + `</xs:schema>`}},
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd">x</known>`, true},
		{"schemaLocation, a valid leaf root", withNS,
			`<t:known xmlns:t="urn:t" ` + xsiNS + ` xsi:schemaLocation="urn:t s.xsd">x</t:known>`, true},
		{"schemaLocation, a leaf root with an element child", withNS,
			`<t:known xmlns:t="urn:t" ` + xsiNS + ` xsi:schemaLocation="urn:t s.xsd"><x/></t:known>`, false},
	} {
		if !exec(hintedCase(t, tc.files, tc.instance, tc.valid)).IsPass() {
			t.Errorf("%s: decided against the hinted schema, the executor must agree with a suite-%s case", tc.why, validityWord(tc.valid))
		}
		if exec(hintedCase(t, tc.files, tc.instance, !tc.valid)).IsPass() {
			t.Errorf("%s: the executor must Fail under a flipped expectation (it decides for real)", tc.why)
		}
	}
}

// TestInstanceExecutorDecidesBuiltinTypedRoot proves a case with no group
// schema and no hint whose root's xsi:type names a built-in type definition is
// decided against the built-in components alone (builtinsSchema, #2151): that
// type governs the root (key-governing-type-elem clause 8, key-itd), which is
// ·strictly assessed· against it, and content outside its lexical space is
// invalid by cvc-type clause 3.1.3. Each decided row declines as no-hint with
// caseSchema's builtinsSchema arm removed. The prefix is resolved against the
// root's own namespace declarations, whichever prefix binds the XSD namespace.
//
// A root that is locally valid against its type is decided valid, the empty
// Result read through assessedSubtreeRoot as a declared root's is
// (subtreeGate.undeclaredRoot, #2156). The last assertion pins that decision:
// it declines as undeclared-root with that method refusing unconditionally.
func TestInstanceExecutorDecidesBuiltinTypedRoot(t *testing.T) {
	exec := newInstanceExec()
	for _, tc := range []struct {
		why      string
		instance string
	}{
		// addB202a.i.
		{"a hexBinary root with a non-hex character",
			`<a ` + xsiNS + ` xmlns:xsd="http://www.w3.org/2001/XMLSchema" xsi:type="xsd:hexBinary">adf&#x0400;789</a>`},
		// addB202b.i.
		{"a date root outside the date lexical space",
			`<a ` + xsiNS + ` xmlns:xsd="http://www.w3.org/2001/XMLSchema" xsi:type="xsd:date">2005-01-0&#x0400;1</a>`},
		{"an int root, the XSD namespace bound to another prefix",
			`<a ` + xsiNS + ` xmlns:p="http://www.w3.org/2001/XMLSchema" xsi:type="p:int">x</a>`},
		{"an int root, the XSD namespace the default namespace",
			`<a ` + xsiNS + ` xmlns="http://www.w3.org/2001/XMLSchema" xsi:type="int">x</a>`},
	} {
		for _, valid := range []bool{false, true} {
			st, why := exec(hintedCase(t, nil, tc.instance, valid))
			if why != "" {
				t.Errorf("%s: declined as %q under expectValid=%v, want decided not valid", tc.why, why, valid)
			}
			if st.IsPass() == valid {
				t.Errorf("%s: under expectValid=%v the executor answered pass=%v, want it decided not valid", tc.why, valid, st.IsPass())
			}
		}
	}

	const valid = `<a ` + xsiNS + ` xmlns:xsd="http://www.w3.org/2001/XMLSchema" xsi:type="xsd:int">1</a>`
	for _, expect := range []bool{true, false} {
		st, why := exec(hintedCase(t, nil, valid, expect))
		if why != "" {
			t.Errorf("a root locally valid against its built-in xsi:type: declined as %q under expectValid=%v, want decided valid", why, expect)
		}
		if st.IsPass() != expect {
			t.Errorf("a root locally valid against its built-in xsi:type: under expectValid=%v the executor answered pass=%v, want it decided valid", expect, st.IsPass())
		}
	}
}

func validityWord(valid bool) string {
	if valid {
		return "valid"
	}
	return "invalid"
}

// TestInstanceExecutorDeclinesUnreadableHints proves every hinted shape the
// lane cannot read completely is DECLINED in both directions (instanceHints,
// assembleHints). Each row is decided with the condition it names removed.
func TestInstanceExecutorDeclinesUnreadableHints(t *testing.T) {
	skip := []fixtureFile{{"s.xsd", xsdDoc("", skipKnown)}, {"o.xsd", xsdDoc("", `<xs:element name="other" type="xs:int"/>`)}}
	known := []fixtureFile{{"s.xsd", xsdDoc("", knownRoot)}}
	below := append([]fixtureFile{{"s.xsd", xsdDoc("", laxKnown)}}, hintedO...)
	for _, tc := range []struct {
		why      string
		files    []fixtureFile
		instance string
		refused  refusal
	}{
		// §4.3.2 clause 4: xml:base moves the base URI a hint below the root
		// resolves against, on the hinted element or an ancestor. Each is
		// decided invalid otherwise, against o.xsd's xs:int <o:c>.
		{"an xml:base on an ancestor of a hinted element", below,
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd"><a xml:base="./"><o:c xmlns:o="urn:o" xsi:schemaLocation="urn:o o.xsd">x</o:c></a></known>`,
			refuseXMLBase},
		{"an xml:base on a hinted element below the root", below,
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd"><o:c xmlns:o="urn:o" xml:base="./" xsi:schemaLocation="urn:o o.xsd">x</o:c></known>`,
			refuseXMLBase},
		// Decided valid otherwise, <o:c> laxly assessed with no declaration.
		{"an xsi:schemaLocation below the root with an odd member count", below,
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd"><o:c xmlns:o="urn:o" xsi:schemaLocation="urn:o">1</o:c></known>`,
			refuseOddLocation},
		// Decided valid otherwise, against a schema short of missing.xsd: a
		// hint below the root for a namespace no earlier hint supplied.
		{"a hint below the root resolving to no document", below,
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd"><o:c xmlns:o="urn:o" xsi:schemaLocation="urn:o missing.xsd">1</o:c></known>`,
			refuseHintUnfollowed},
		// Decided valid otherwise, the inner one read as the outer's content.
		{"an inline xs:schema inside another", skip,
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd"><xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">` +
				`<xs:annotation><xs:appinfo><xs:schema/></xs:appinfo></xs:annotation></xs:schema></known>`,
			refuseInlineSchema},
		// §4.3.2 clause 4: xml:base would move the base URI the inline
		// document's own locations resolve against. Decided valid otherwise.
		{"an xml:base on an ancestor of an inline xs:schema", skip,
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd"><a xml:base="sub/"><xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"/></a></known>`,
			refuseXMLBase},
		// A prefix undeclaration is legal in the XML 1.1 instance and not in
		// the inline document, which carries no declaration and is XML 1.0
		// (nsc-NoPrefixUndecl), so it does not read. Decided invalid otherwise,
		// the reader's charge read as wellFormednessFault's "not valid".
		{"an inline xs:schema that does not read as a schema document", skip,
			`<?xml version="1.1"?><known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd"><xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:p=""/></known>`,
			refuseInlineUnread},
		// Decided invalid otherwise (cvc-type clause 3.1.2). The ATTLIST
		// defaults nothing: the row pins rootStart's literal <!ATTLIST refusal
		// (defaultsNoAttribute), which refuses any ATTLIST whether or not it
		// defaults.
		{"a DOCTYPE whose internal subset holds an <!ATTLIST", known,
			`<!DOCTYPE known [<!ATTLIST known a CDATA #IMPLIED>]><known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd"><x/></known>`,
			refuseDoctype},
		{"an xml:base on the root", known,
			`<known ` + xsiNS + ` xml:base="sub/" xsi:noNamespaceSchemaLocation="s.xsd">x</known>`,
			refuseXMLBase},
		{"an xsi:schemaLocation with an odd member count", known,
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd" xsi:schemaLocation="urn:t">x</known>`,
			refuseOddLocation},
		// Decided valid otherwise, against a schema short of missing.xsd.
		{"a hint resolving to no document", known,
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd missing.xsd">x</known>`,
			refuseHintUnfollowed},
		// Decided invalid otherwise, where a root no declaration governs is
		// laxly assessed and notKnown (cvc-assess-elt clause 3, §3.3.5.1).
		{"a hinted schema declaring no top-level element for the root", known,
			`<unknown ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd"/>`,
			refuseHintUndeclared},
	} {
		declinesBothPolarities(t, hintedCase(t, tc.files, tc.instance, true), tc.why, tc.refused)
	}
}

// laxKnown declares <known> with any number of children of any name, each
// ·laxly assessed· by a lax Wildcard: a child no hint declares is valid, and
// one a followed hint declares is assessed against that declaration.
const laxKnown = `<xs:element name="known"><xs:complexType><xs:sequence>` +
	`<xs:any processContents="lax" minOccurs="0" maxOccurs="unbounded"/></xs:sequence></xs:complexType></xs:element>`

// hintedO is two documents for namespace urn:o: o.xsd declares <o:c> and
// o2.xsd <o:d>, each an xs:int, so content "x" is invalid wherever a
// declaration governs and valid where none does.
var hintedO = []fixtureFile{
	{"o.xsd", xsdDoc("urn:o", `<xs:element name="c" type="xs:int"/>`)},
	{"o2.xsd", xsdDoc("urn:o", `<xs:element name="d" type="xs:int"/>`)},
}

// TestInstanceExecutorReadsHintsBelowRoot proves a case with no group schema
// is decided against the hints of every element, in document order, the first
// hint for a namespace winning (instanceHints, §4.3.2 clauses 3-5, #2171). Each
// row is decided in both polarities and agrees with the validity it states.
func TestInstanceExecutorReadsHintsBelowRoot(t *testing.T) {
	files := append([]fixtureFile{{"s.xsd", xsdDoc("", laxKnown)}}, hintedO...)
	exec := newInstanceExec()
	for _, tc := range []struct {
		why      string
		instance string
		valid    bool
	}{
		// Declined hint-below-root before #2171; decided valid with
		// the hints of every element below the root left out of
		// hintReader.element's result.
		{"a hint below the root for a namespace no earlier hint supplied",
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd"><o:c xmlns:o="urn:o" xsi:schemaLocation="urn:o o.xsd">x</o:c></known>`,
			false},
		// A later hint for urn:o names o2.xsd, whose <o:d> would make "x"
		// invalid: decided invalid with hintsOf's covered test removed for
		// xsi:schemaLocation, following both documents. The later hint sits on
		// an element in urn:p, which no hint supplied: decided invalid with
		// the namespace taken from the element rather than the hint.
		{"a later hint below the root for a namespace an earlier one supplied",
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd"><o:c xmlns:o="urn:o" xsi:schemaLocation="urn:o o.xsd">1</o:c>` +
				`<p:e xmlns:p="urn:p" xsi:schemaLocation="urn:o o2.xsd"/><o:d xmlns:o="urn:o">x</o:d></known>`,
			true},
		// Declined hint-unfollowed with hintsOf's covered test removed.
		{"a later hint for a supplied namespace resolving to no document",
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd"><o:c xmlns:o="urn:o" xsi:schemaLocation="urn:o o.xsd">x</o:c>` +
				`<o:c xmlns:o="urn:o" xsi:schemaLocation="urn:o missing.xsd">1</o:c></known>`,
			false},
		{"a hint repeated below the root, the same location",
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd"><o:c xmlns:o="urn:o" xsi:schemaLocation="urn:o o.xsd">1</o:c>` +
				`<o:c xmlns:o="urn:o" xsi:schemaLocation="urn:o o.xsd">x</o:c></known>`,
			false},
		// An xml:base scopes its own element's subtree alone: declined
		// xml-base with hintReader.read's EndElement arm removed.
		{"an xml:base on an earlier sibling of a hinted element",
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd"><a xml:base="sub/"/><o:c xmlns:o="urn:o" xsi:schemaLocation="urn:o o.xsd">x</o:c></known>`,
			false},
	} {
		for _, expect := range []bool{tc.valid, !tc.valid} {
			st, why := exec(hintedCase(t, files, tc.instance, expect))
			if why != "" {
				t.Errorf("%s: declined as %q under expectValid=%v, want decided %s", tc.why, why, expect, validityWord(tc.valid))
				continue
			}
			if st.IsPass() != (expect == tc.valid) {
				t.Errorf("%s: under expectValid=%v the executor answered pass=%v, want it decided %s", tc.why, expect, st.IsPass(), validityWord(tc.valid))
			}
		}
	}
}

// xsOpen opens an inline xs:schema for namespace urn:o.
const xsOpen = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:o">`

// TestInstanceExecutorReadsInlineSchema proves a case with no group schema is
// decided against a schema its inline xs:schema documents are part of, below
// the root, beside a hint for the same namespace, and as the root itself
// (instanceHints, #2180, §4.3.2, §5.1). Each row is decided in both polarities
// and agrees with the validity it states. Every row declines inline-schema
// with the change reverted.
func TestInstanceExecutorReadsInlineSchema(t *testing.T) {
	files := append([]fixtureFile{{"s.xsd", xsdDoc("", laxKnown)}}, hintedO...)
	exec := newInstanceExec()
	for _, tc := range []struct {
		why      string
		files    []fixtureFile
		instance string
		valid    bool
	}{
		// Decided valid, <o:c> laxly assessed, with the inline document left
		// out of the assembly.
		{"an inline xs:schema below the root", files,
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd">` + xsOpen + `<xs:element name="c" type="xs:int"/></xs:schema><o:c xmlns:o="urn:o">x</o:c></known>`,
			false},
		{"an inline xs:schema below the root, its element valid", files,
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd">` + xsOpen + `<xs:element name="c" type="xs:int"/></xs:schema><o:c xmlns:o="urn:o">1</o:c></known>`,
			true},
		// The hint supplies urn:o and the inline document is read beside it:
		// decided valid with inline documents put through hintsOf's
		// first-wins test.
		{"an inline xs:schema beside a hint for the same namespace", files,
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd" xsi:schemaLocation="urn:o o.xsd">` + xsOpen + `<xs:element name="e" type="xs:int"/></xs:schema>` +
				`<o:c xmlns:o="urn:o">1</o:c><o:e xmlns:o="urn:o">x</o:e></known>`,
			false},
		{"an inline xs:schema beside a hint for the same namespace, both valid", files,
			`<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd" xsi:schemaLocation="urn:o o.xsd">` + xsOpen + `<xs:element name="e" type="xs:int"/></xs:schema>` +
				`<o:c xmlns:o="urn:o">1</o:c><o:e xmlns:o="urn:o">2</o:e></known>`,
			true},
		// t is bound on the root alone: declined with inheritedBindings'
		// result left out of the inline document.
		{"an inline xs:schema naming a type through an ancestor's prefix", files,
			`<known xmlns:t="http://www.w3.org/2001/XMLSchema" ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd">` + xsOpen + `<xs:element name="c" type="t:int"/></xs:schema>` +
				`<o:c xmlns:o="urn:o">x</o:c></known>`,
			false},
		// s.xsd imports a declaration of the root itself, so no
		// undeclared-root decline masks the row.
		{"an inline xs:schema as the root carrying a hint",
			[]fixtureFile{
				{"s.xsd", xsdDoc("", `<xs:import namespace="http://www.w3.org/2001/XMLSchema" schemaLocation="x.xsd"/>`)},
				{"x.xsd", xsdDoc("http://www.w3.org/2001/XMLSchema", `<xs:element name="schema"/>`)},
			},
			`<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd"/>`,
			true},
	} {
		for _, expect := range []bool{tc.valid, !tc.valid} {
			st, why := exec(hintedCase(t, tc.files, tc.instance, expect))
			if why != "" {
				t.Errorf("%s: declined as %q under expectValid=%v, want decided %s", tc.why, why, expect, validityWord(tc.valid))
				continue
			}
			if st.IsPass() != (expect == tc.valid) {
				t.Errorf("%s: under expectValid=%v the executor answered pass=%v, want it decided %s", tc.why, expect, st.IsPass(), validityWord(tc.valid))
			}
		}
	}
}

// TestInlineSchemaCollisionIsSchemaError proves an inline xs:schema declaring
// a component a hinted document for the same namespace declares too is read,
// not left out as a later hint is, so the assembly is the schema error
// sch-props-correct clause 2 makes it and the case declines (refuseSchemaError).
// The error cites the inline document at the instance's own line and column of
// the second declaration: it reads 2:5, the line counted from the inline
// document's own start, with closeSchema's padding removed.
func TestInlineSchemaCollisionIsSchemaError(t *testing.T) {
	c := hintedCase(t, append([]fixtureFile{{"s.xsd", xsdDoc("", laxKnown)}}, hintedO...),
		`<known `+xsiNS+` xsi:noNamespaceSchemaLocation="s.xsd" xsi:schemaLocation="urn:o o.xsd">`+"\n"+
			`  `+xsOpen+"\n"+`    <xs:element name="c" type="xs:int"/></xs:schema><o:c xmlns:o="urn:o">1</o:c></known>`, true)
	reachesPerrArm(t, c, "a collision")
	_, _, perr := caseSchema(strict.New(), c)
	if want := "i.xml#inline-1:3:5: [sch-props-correct]"; !strings.HasPrefix(perr.Error(), want) {
		t.Errorf("caseSchema error = %v, want it to open %q", perr, want)
	}
	declinesBothPolarities(t, c, "a collision", refuseSchemaError)
}

// TestHintedSchemaUndeclaringPrefixIsRejected pins addB139's shape: an XML 1.0
// hinted schema carrying xmlns:f="" is decidable, and its assembly is the
// parser's rejection under nsc-NoPrefixUndecl rather than a decline or a schema
// built as if the declaration were legal.
func TestHintedSchemaUndeclaringPrefixIsRejected(t *testing.T) {
	c := hintedCase(t,
		[]fixtureFile{{"s.xsd", `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:f="">` + knownRoot + `</xs:schema>`}},
		`<known `+xsiNS+` xsi:noNamespaceSchemaLocation="s.xsd">x</known>`, false)
	_, why, perr := caseSchema(strict.New(), c)
	if why != "" {
		t.Fatalf("caseSchema declined as %q, want the closure read and decided", why)
	}
	if perr == nil || !strings.Contains(perr.Error(), "[xml-wf] namespace declaration xmlns:f has an empty value") {
		t.Errorf("caseSchema error = %v, want the parser's xml-wf rejection of xmlns:f=\"\"", perr)
	}
}

// hintedRejection is one hinted case whose assembly caseSchema reads as
// decidable and rejected, so execInstanceCase's perr arm is the gate it meets,
// and the refusal that arm names where it declines the case — none for a case
// it decides.
type hintedRejection struct {
	why      string
	files    []fixtureFile
	instance string
	refused  refusal
}

// noNSHint is a <known> root hinting s.xsd through xsi:noNamespaceSchemaLocation.
const noNSHint = `<known ` + xsiNS + ` xsi:noNamespaceSchemaLocation="s.xsd">x</known>`

// reachesPerrArm fails t unless caseSchema reads c as decidable with a non-nil
// error, so no earlier gate declines it ahead of execInstanceCase's perr arm.
func reachesPerrArm(t *testing.T, c caseSpec, why string) {
	t.Helper()
	_, refused, perr := caseSchema(strict.New(), c)
	if refused != "" || perr == nil {
		t.Fatalf("%s: caseSchema = (refused %q, %v), want a decidable rejection", why, refused, perr)
	}
}

// TestInstanceExecutorDecidesNotWellFormedHintedSchema proves a hinted schema
// document the reader charged not well-formed is decided "not valid", the
// harness convention execInstanceCase states under §5.1 (#2058): addB138 and
// addB139's xmlns:f="" in an XML 1.0 document, and addB124's element left
// unclosed. Each row declines in both polarities with the decide arm removed.
func TestInstanceExecutorDecidesNotWellFormedHintedSchema(t *testing.T) {
	exec := newInstanceExec().status()
	for _, tc := range []hintedRejection{
		{"an XML 1.0 schema undeclaring a prefix",
			[]fixtureFile{{"s.xsd", `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:f="">` + knownRoot + `</xs:schema>`}},
			noNSHint, ""},
		{"a schema leaving an element unclosed",
			[]fixtureFile{{"s.xsd", xsdDoc("", `<xs:simpleType name="t"><xs:restriction base="xs:string"><xs:whiteSpace value="collapse"></xs:restriction></xs:simpleType>`+knownRoot)}},
			noNSHint, ""},
	} {
		c := hintedCase(t, tc.files, tc.instance, false)
		reachesPerrArm(t, c, tc.why)
		if !exec(c).IsPass() {
			t.Errorf("%s: must be decided not valid, agreeing with a suite-invalid case", tc.why)
		}
		c.expect = expectValidity(true)
		if exec(c).IsPass() {
			t.Errorf("%s: must Fail under a flipped expectation (it decides for real)", tc.why)
		}
	}
}

// TestInstanceExecutorDeclinesRejectedHintedSchema proves every other rejection
// of a hinted assembly is DECLINED in both polarities (#2058): a rejection of a
// document that read (schA8.i's src-import clause 3.1), a resolver fault on a
// composed document (#1201), a read failure wrapping a cause, and a prefix
// bound only by the namespace declaration an internal-subset ATTLIST defaults
// (unboundPrefixCharge). Every row reaches execInstanceCase's perr arm, and
// each is decided with wellFormednessFault removed from it.
func TestInstanceExecutorDeclinesRejectedHintedSchema(t *testing.T) {
	for _, tc := range []hintedRejection{
		{"a hinted document whose targetNamespace is not the hint's namespace",
			[]fixtureFile{{"s.xsd", xsdDoc("urn:other", knownRoot)}},
			`<t:known xmlns:t="urn:t" ` + xsiNS + ` xsi:schemaLocation="urn:t s.xsd">x</t:known>`,
			refuseSchemaError},
		// ENOTDIR from loader.Dir: a resolver fault, not loader.ErrNotFound.
		{"an <include> whose location the resolver faults on",
			[]fixtureFile{{"s.xsd", xsdDoc("", `<xs:include schemaLocation="s.xsd/x.xsd"/>`+knownRoot)}},
			noNSHint, refuseSchemaError},
		{"an encoding declaration the reader does not decode",
			[]fixtureFile{{"s.xsd", `<?xml version="1.0" encoding="ISO-8859-1"?>` + xsdDoc("", knownRoot)}},
			noNSHint, refuseSchemaError},
		// #2067's shape: a well-formed document (XML 1.0 §5.1) the reader
		// charges with an unbound prefix, as it applies no attribute default.
		{"a namespace declaration an internal-subset ATTLIST defaults",
			[]fixtureFile{{"s.xsd", `<!DOCTYPE xs:schema [` +
				`<!ATTLIST xs:schema xmlns:xs CDATA #FIXED "http://www.w3.org/2001/XMLSchema">]>` +
				`<xs:schema>` + knownRoot + `</xs:schema>`}},
			noNSHint, refuseUnboundPrefix},
	} {
		c := hintedCase(t, tc.files, tc.instance, true)
		reachesPerrArm(t, c, tc.why)
		declinesBothPolarities(t, c, tc.why, tc.refused)
	}
}
