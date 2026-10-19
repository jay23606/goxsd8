package conformance

import (
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
)

// ctaTypes declares the two types a {type table} below chooses between: A
// requires one child <a>, B one child <b>, each of type xs:int, so an instance
// valid against the one is not valid against the other, and each carries an
// optional attribute kind.
const ctaTypes = `<xs:complexType name="A"><xs:sequence><xs:element name="a" type="xs:int"/></xs:sequence>` +
	`<xs:attribute name="kind" type="xs:string"/></xs:complexType>` +
	`<xs:complexType name="B"><xs:sequence><xs:element name="b" type="xs:int"/></xs:sequence>` +
	`<xs:attribute name="kind" type="xs:string"/></xs:complexType>`

// ctaRoot declares <known> with a {type table} whose alternatives carry tests,
// in order, each selecting A, and whose {default type definition} is B.
func ctaRoot(tests ...string) string {
	alts := ""
	for _, test := range tests {
		alts += `<xs:alternative test="` + test + `" type="A"/>`
	}
	return ctaTypes + `<xs:element name="known">` + alts + `<xs:alternative type="B"/></xs:element>`
}

// ctaInherited declares the top-level inheritable attribute kind and <known>,
// whose type declares attrs, over one child <inner> whose {type table} selects
// A where @kind = 'a' and B otherwise; A and B each take an optional kind.
func ctaInherited(attrs string) string {
	return `<xs:complexType name="A"><xs:sequence><xs:element name="a" type="xs:int"/></xs:sequence><xs:attribute ref="kind"/></xs:complexType>` +
		`<xs:complexType name="B"><xs:sequence><xs:element name="b" type="xs:int"/></xs:sequence><xs:attribute ref="kind"/></xs:complexType>` +
		`<xs:attribute name="kind" type="xs:string" inheritable="true"/>` +
		`<xs:element name="known"><xs:complexType><xs:sequence><xs:element name="inner">` +
		`<xs:alternative test="@kind = 'a'" type="A"/><xs:alternative type="B"/>` +
		`</xs:element></xs:sequence>` + attrs + `</xs:complexType></xs:element>`
}

// kindUse is an {attribute uses} member referencing kind.
const kindUse = `<xs:attribute ref="kind"/>`

// TestInstanceExecutorDecidesTypeTable proves the gate admits a declaration
// carrying a {type table}, at the root and below it, reading the element
// against the type the table ·conditionally selects· (§3.3.4.1
// key-selected-type clause 1, key-cta-select) and not against the declared
// {type definition} (#2126): each row is decided VALID, and under the flipped
// expectation it Fails; with subtreeGate.element refusing a {type table} again,
// every row declines. Each row choosing between A and B is valid against the
// selected one alone, so a gate selecting the other refuses it at
// content-rejected: the inherited rows select A only through [inherited
// attributes] (§3.3.5.6, key-cta-ta-select clause 1.1.3), and select B only
// where the child's own attribute shadows the inherited one (clause 1.1.3) or
// the parent's attribute is ·skipped· and so not ·potentially inherited·
// (key-p-inherited clause 3.2).
func TestInstanceExecutorDecidesTypeTable(t *testing.T) {
	exec := newInstanceExec().status()
	for _, tc := range []struct{ why, schemaBody, instance string }{
		{"a {test} true selects its alternative's type", ctaRoot("@kind = 'a'"), `<known kind="a"><a>1</a></known>`},
		{"no {test} true selects the {default type definition}", ctaRoot("@kind = 'a'"), `<known kind="b"><b>1</b></known>`},
		{"a {test} reading an absent attribute is false", ctaRoot("@kind = 'a'"), `<known><b>1</b></known>`},
		{
			// key-cta-ta-select clause 2: the failed cast is a dynamic error,
			// which makes the {test} false and not undecided.
			"a {test} raising a dynamic error is false", ctaRoot("xs:integer(@kind) = 1"), `<known kind="x"><b>1</b></known>`,
		},
		{
			// Both {test}s are true and select different types, so a scan
			// taking the later one reads <a> against B.
			"the first true {test} wins",
			ctaTypes + `<xs:element name="known"><xs:alternative test="@kind = 'a'" type="A"/>` +
				`<xs:alternative test="@kind" type="B"/><xs:alternative type="B"/></xs:element>`,
			`<known kind="a"><a>1</a></known>`,
		},
		{
			"a {type table} on a content-less root",
			`<xs:complexType name="E"/><xs:element name="known" type="E"><xs:alternative type="E"/></xs:element>`, `<known/>`,
		},
		{
			"a {type table} below the root",
			`<xs:element name="known"><xs:complexType><xs:sequence><xs:element name="a" type="xs:int">` +
				`<xs:alternative type="xs:int"/></xs:element></xs:sequence></xs:complexType></xs:element>`,
			`<known><a>1</a></known>`,
		},
		{"an inherited attribute selects a child's type", ctaInherited(kindUse), `<known kind="a"><inner><a>1</a></inner></known>`},
		{"a defaulted inheritable attribute selects a child's type", ctaInherited(`<xs:attribute ref="kind" default="a"/>`), `<known><inner><a>1</a></inner></known>`},
		{"an inherited attribute leaves the {test} false", ctaInherited(kindUse), `<known kind="b"><inner><b>1</b></inner></known>`},
		{"the child's own attribute shadows an inherited one", ctaInherited(kindUse), `<known kind="a"><inner kind="b"><b>1</b></inner></known>`},
		{
			// key-p-inherited clause 3.2 reads no declaration for a ·skipped·
			// attribute, so it is not ·potentially inherited·.
			"a skip {attribute wildcard}'s attribute is not inherited",
			ctaInherited(`<xs:anyAttribute processContents="skip"/>`), `<known kind="a"><inner><b>1</b></inner></known>`,
		},
		{
			// key-p-inherited clause 3.2: no {attribute uses} member of known's
			// type matches kind, and the lax {attribute wildcard} admitting it
			// resolves it to the inheritable top-level declaration.
			"a wildcard-admitted top-level inheritable attribute is inherited",
			ctaInherited(`<xs:anyAttribute processContents="lax"/>`), `<known kind="a"><inner><a>1</a></inner></known>`,
		},
	} {
		if !exec(instanceCase(t, tc.schemaBody, tc.instance, true)).IsPass() {
			t.Errorf("%s: the walk selects the type and the gate reads the element against it; the executor must agree with a suite-valid case", tc.why)
		}
		if exec(instanceCase(t, tc.schemaBody, tc.instance, false)).IsPass() {
			t.Errorf("%s: the executor must Fail under a flipped expectation (it decides for real)", tc.why)
		}
	}
}

// TestAssessedSubtreeRootTypeTable pins, at the gate itself, the exits
// selectedType and governingType take for a {type table} that no executor row
// reaches, because the walk records or charges each first: a {test} the
// §3.12.6 evaluator declines before any alternative succeeds is
// refuseTypeTableUndecided (validate records key-cta-ta-select), while one
// behind a true {test} is never compiled; an xsi:type that ·overrides· the
// declared type but not the selected one is refuseXsiTypeNotOverride (cvc-elt
// clause 4, which validate charges); a value whose [[normalized value]]
// encoding/xml cannot give is refuseTypeTableWhitespace.
func TestAssessedSubtreeRootTypeTable(t *testing.T) {
	const declined = "count(@kind) &gt; 0"
	for _, tc := range []struct {
		why, schemaBody, instance string
		want                      refusal
	}{
		{"an undecidable {test} first", ctaRoot(declined), `<known kind="a"><a>1</a></known>`, refuseTypeTableUndecided},
		{"an undecidable {test} behind a false one", ctaRoot("@kind = 'z'", declined), `<known kind="a"><a>1</a></known>`, refuseTypeTableUndecided},
		{"an undecidable {test} behind a true one", ctaRoot("@kind = 'a'", declined), `<known kind="a"><a>1</a></known>`, ""},
		{
			"an xsi:type naming the declared type, not the selected one",
			ctaTypes + `<xs:element name="known" type="xs:anyType"><xs:alternative test="@kind = 'a'" type="A"/><xs:alternative type="B"/></xs:element>`,
			`<known ` + xsiNS + ` xsi:type="B" kind="a"><b>1</b></known>`,
			refuseXsiTypeNotOverride,
		},
		{"a literal tab in a value a {test} reads", ctaRoot("@kind = 'a'"), "<known kind=\"a\t\"><a>1</a></known>", refuseTypeTableWhitespace},
		{"a character reference to a tab in a value a {test} reads", ctaRoot("@kind = 'a'"), `<known kind="a&#9;"><b>1</b></known>`, refuseTypeTableWhitespace},
	} {
		c := instanceCase(t, tc.schemaBody, tc.instance, true)
		schema, _, decidable, err := assembleCase(strict.New(), c.schemaDoc, nil)
		if err != nil || !decidable {
			t.Fatalf("%s: assembling the schema: decidable %v, err %v", tc.why, decidable, err)
		}
		if got := assessedSubtreeRoot(strict.New(), schema, c.doc); got != tc.want {
			t.Errorf("%s: assessedSubtreeRoot = %q, want %q", tc.why, got, tc.want)
		}
	}
}

// ctaError declares a {nillable} <known> whose {type table} selects ·xs:error·
// for an element carrying no kind attribute and xs:string otherwise.
const ctaError = `<xs:element name="known" nillable="true"><xs:alternative test="not(@kind)" type="xs:error"/>` +
	`<xs:alternative type="xs:string"/></xs:element>`

// TestInstanceExecutorDeclinesNilledErrorType pins refuseErrorType: ·xs:error·
// "has no valid instances" (§3.16.7.3, key-error), and the walk charges cvc-type
// clause 3.1.3 for an element it governs that is not ·nilled· — the control
// row, decided INVALID — but nothing for a ·nilled· one, which clause 3.1.3
// does not reach. The gate claims no verdict for that one: without
// refuseErrorType it is decided valid.
func TestInstanceExecutorDeclinesNilledErrorType(t *testing.T) {
	exec := newInstanceExec().status()
	if !exec(instanceCase(t, ctaError, `<known/>`, false)).IsPass() {
		t.Error("an empty element ·xs:error· governs: the walk charges cvc-type clause 3.1.3; the executor must agree with a suite-invalid case")
	}
	declinesBothPolarities(t, instanceCase(t, ctaError, `<known `+xsiNS+` xsi:nil="true"/>`, false),
		"a ·nilled· element ·xs:error· governs", refuseErrorType)
}
