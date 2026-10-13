package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// The fixtures below drive a cast of a StringLiteral to xs:QName
// (xpath-functions.md §5.3) and an xs:QName operand read typed, compared by
// op:QName-equal (xpath20.md B.2): Structures' assert024, `@name eq
// xsd:QName('xsd:element')`.

// qnRecord is expr's XPath Expression property record, binding xsd to the XSD
// namespace and z to urn:z, with {default namespace} defaultNS where it is
// non-empty.
func qnRecord(expr, defaultNS string) xsd.XPathExpression {
	return ctaExprRecord(expr, defaultNS, "xsd", xsd.XMLSchemaNS, "z", "urn:z")
}

// qnAttr is one attribute of E: typ is the builtin its use declares, and an
// xs:QName lexical is mapped against bindings, the instance's own in-scope
// namespaces, prefix to namespace name, the default namespace under "" — the
// mapping validate performs before an assertion reads the value.
type qnAttr struct {
	name     string
	typ      string
	lexical  string
	bindings map[string]string
}

// qnContext is the instance's [value.Context]: its bindings, the empty prefix
// in no namespace where none binds it. It is not ctaNames, so a mutation of the
// expression's context leaves the instance's values as they are.
type qnContext map[string]string

func (c qnContext) LookupNamespace(prefix string) (string, bool) {
	ns, ok := c[prefix]
	return ns, ok || prefix == ""
}

// qnValues is the [TypedAttributes] over attrs in the order written.
func qnValues(t *testing.T, attrs ...qnAttr) TypedAttributes {
	t.Helper()
	vs := make([]TypedValue, 0, len(attrs))
	for _, a := range attrs {
		st := asBuiltin(t, a.typ)
		if st.IsSpecial() {
			vs = append(vs, Untyped(a.lexical))
			continue
		}
		v, err := value.ValidateLexical(backend(), seededTypes, st, a.lexical, qnContext(a.bindings), FacetAssertions(time.Time{}))
		if err != nil {
			t.Fatalf("mapping %q as xs:%s: %v", a.lexical, a.typ, err)
		}
		vs = append(vs, Typed(v))
	}
	return func(yield func(xsd.QName, TypedValue) bool) {
		for i, a := range attrs {
			if !yield(uq(a.name), vs[i]) {
				return
			}
		}
	}
}

// qnUses types @name and @other xs:QName, @s xs:string and @u
// xs:anySimpleType.
func qnUses(t *testing.T) AttributeTypes {
	t.Helper()
	return asUses(t, map[string]string{"name": "QName", "other": "QName", "s": "string", "u": "anySimpleType"})
}

// The instance's prefixes, none of them the expression's xsd: qnXSD binds q to
// the XSD namespace, qnOther binds q to urn:other, and qnBoth binds q and r
// both to the XSD namespace.
var (
	qnXSD   = map[string]string{"q": xsd.XMLSchemaNS}
	qnOther = map[string]string{"q": "urn:other"}
	qnBoth  = map[string]string{"q": xsd.XMLSchemaNS, "r": xsd.XMLSchemaNS}
)

// TestAssertionQNameEqual pins op:QName-equal over a StringLiteral's cast to
// xs:QName: equality of (namespace name, local part), prefixes ignored
// (xpath-functions.md §11.2.1). Every instance prefix differs from the
// literal's `xsd`, so a comparison that read prefixes would answer false on
// each true row. An unprefixed literal takes the expression's {default
// namespace}, else no namespace (§5.3). An absent @name makes `eq` the empty
// sequence, false, and true under fn:not. A literal outside xs:QName's lexical
// space raises err:FORG0001, false under fn:not too.
//
// Mutations run against this table:
//   - castTarget declining an xs:QName target again: every row declines;
//     ctaTypes.typedAtomic declining xs:QName: every row but the two castable
//     ones declines.
//   - LookupNamespace answering a prefix as its own namespace: the eq row over
//     q:element bound to the XSD namespace, the ne row over the same, the
//     padded literal, the cast spelling and the xsd:QName(@name) row fail.
//   - xs:QName's value equality reading the local part alone: the eq and ne
//     rows over q bound to urn:other and the unprefixed literal with no
//     {default namespace} fail; reading the namespace alone: the q:attribute
//     row fails.
//   - the cast's names not set (ctaParser.cast) or not scoped onto its operand
//     (ctaCastItem): every true row fails but fn:not over an absent @name.
//   - LookupNamespace without its `xml` arm: the xml:lang row declines; with
//     the empty prefix ignoring the {default namespace}: the unprefixed literal
//     under the XSD default fails.
func TestAssertionQNameEqual(t *testing.T) {
	uses := qnUses(t)
	element := func(lexical string, bindings map[string]string) []qnAttr {
		return []qnAttr{{"name", "QName", lexical, bindings}}
	}
	for _, tc := range []struct {
		expr, defaultNS string
		attrs           []qnAttr
		want            bool
	}{
		{"@name eq xsd:QName('xsd:element')", "", element("q:element", qnXSD), true},
		{"@name eq xsd:QName('xsd:element')", "", element("q:element", qnOther), false},
		{"@name eq xsd:QName('xsd:element')", "", element("q:attribute", qnXSD), false},
		{"@name eq xsd:QName('xsd:element')", "", nil, false},
		{"not(@name eq xsd:QName('xsd:element'))", "", nil, true},
		{"@name ne xsd:QName('xsd:element')", "", element("q:element", qnXSD), false},
		{"@name ne xsd:QName('xsd:element')", "", element("q:element", qnOther), true},
		{"@name = xsd:QName(' xsd:element ')", "", element("q:element", qnXSD), true},
		{"'xsd:element' cast as xsd:QName eq @name", "", element("q:element", qnXSD), true},
		{"xsd:QName(@name) eq xsd:QName('xsd:element')", "", element("q:element", qnXSD), true},
		{"@name eq xsd:QName('element')", xsd.XMLSchemaNS, element("q:element", qnXSD), true},
		{"@name eq xsd:QName('element')", "", element("q:element", qnXSD), false},
		{"@name eq xsd:QName('element')", "", element("element", nil), true},
		{"@name eq xsd:QName('xml:lang')", "", element("x:lang", map[string]string{"x": "http://www.w3.org/XML/1998/namespace"}), true},
		{"@name eq xsd:QName('xsd:1bad')", "", element("q:element", qnXSD), false},
		{"not(@name eq xsd:QName('xsd:1bad'))", "", element("q:element", qnXSD), false},
		{"'xsd:element' castable as xsd:QName", "", nil, true},
		{"'xsd:1bad' castable as xsd:QName", "", nil, false},
	} {
		c, ok := CompileAssertionTest(qnRecord(tc.expr, tc.defaultNS), seededTypes, xsd.EmptyContent{}, uses, asNoElems)
		if !ok {
			t.Errorf("CompileAssertionTest(%q): declined, want compiled", tc.expr)
			continue
		}
		got := c.Evaluate(backend(), seededTypes, asElem, qnValues(t, tc.attrs...), asNoChildren, nil, ValueBinding{}, time.Time{})
		if got != tc.want {
			t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.attrs, got, tc.want)
		}
	}
}

// TestAssertionQNameOperands pins what an xs:QName operand is compared with
// beside another one: B.2 gives xs:QName eq and ne alone, so an ordering
// comparison is err:XPTY0004; an xs:QName against an xs:string is no B.2 row
// either, by value comparison or general; and an xs:untypedAtomic operand is
// cast to xs:QName (xpath20.md §3.5.2 clause 2.4), which F&O §17.1 makes
// err:XPTY0004 for any operand but a StringLiteral. Each raised row is false,
// and false under fn:not, which a decided false would invert.
//
// With ctaTypes.typedAtomic declining xs:QName, every row but the two over @u
// declines; with castTarget declining an xs:QName target, those two decline.
// With the static context reaching every datatype validation rather than the
// literal's cast alone, `@u = xsd:QName('xsd:element')` is true. With
// xs:QName's value equality reading the namespace alone, the q:attribute
// `@name eq @other` row is true.
func TestAssertionQNameOperands(t *testing.T) {
	uses := qnUses(t)
	pair := []qnAttr{{"name", "QName", "q:element", qnBoth}, {"other", "QName", "r:element", qnBoth}}
	for _, tc := range []struct {
		expr  string
		attrs []qnAttr
		want  bool
	}{
		{"@name eq @other", pair, true},
		{"@name = @other", pair, true},
		{"@name eq @other", []qnAttr{pair[0], {"other", "QName", "q:attribute", qnBoth}}, false},
		{"@name lt @other", pair, false},
		{"not(@name lt @other)", pair, false},
		{"@name eq 'xsd:element'", pair, false},
		{"not(@name eq 'xsd:element')", pair, false},
		{"@name = 'xsd:element'", pair, false},
		{"not(@name = 'xsd:element')", pair, false},
		{"@u = xsd:QName('xsd:element')", []qnAttr{{"u", "anySimpleType", "xsd:element", nil}}, false},
		{"not(@u = xsd:QName('xsd:element'))", []qnAttr{{"u", "anySimpleType", "xsd:element", nil}}, false},
	} {
		c, ok := CompileAssertionTest(qnRecord(tc.expr, ""), seededTypes, xsd.EmptyContent{}, uses, asNoElems)
		if !ok {
			t.Errorf("CompileAssertionTest(%q): declined, want compiled", tc.expr)
			continue
		}
		got := c.Evaluate(backend(), seededTypes, asElem, qnValues(t, tc.attrs...), asNoChildren, nil, ValueBinding{}, time.Time{})
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// TestAssertionQNameDeclines pins the casts to xs:QName that WITHHOLD the
// assertion — never charged and never satisfied. A StringLiteral whose prefix
// the static context does not bind is err:FONS0004, a static error no
// assertion can be charged with. Every other operand but one of type xs:QName
// — an xs:string attribute, a computed xs:string, an xs:untypedAtomic
// attribute, a numeric or boolean literal — is §17.1's err:XPTY0004, and
// declines (ctaTypes.castsFrom). An xs:QName operand is never cast to another
// type, fn:string included.
//
// With ctaNames.resolvesLiteral's check skipped, the three zz rows compile;
// with castsFrom's xs:QName rule removed, the xsd:QName(@s), concat,
// xsd:QName(@u) and `@s cast as` rows compile.
func TestAssertionQNameDeclines(t *testing.T) {
	uses := qnUses(t)
	for _, tc := range []struct{ expr, why string }{
		{"@name eq xsd:QName('zz:element')", "an unbound prefix in the literal: err:FONS0004"},
		{"'zz:element' cast as xsd:QName eq @name", "the cast spelling of the same"},
		{"'zz:element' castable as xsd:QName", "castable over the same cast"},
		{"xsd:QName(@s) eq xsd:QName('xsd:element')", "an xs:string attribute is computed: err:XPTY0004"},
		{"@name eq xsd:QName(concat('xsd:', 'element'))", "a computed xs:string likewise"},
		{"xsd:QName(@u) eq xsd:QName('xsd:element')", "an xs:untypedAtomic attribute likewise"},
		{"@s cast as xsd:QName eq @name", "the cast spelling over an xs:string attribute"},
		{"xsd:QName(1) eq @name", "a numeric literal"},
		{"xsd:QName(true()) eq @name", "a boolean"},
		{"@name cast as xsd:string = 'q:element'", "a cast from an xs:QName to another type"},
		{"string(@name) = 'q:element'", "fn:string over an xs:QName"},
	} {
		if _, ok := CompileAssertionTest(qnRecord(tc.expr, ""), seededTypes, xsd.EmptyContent{}, uses, asNoElems); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined (%s)", tc.expr, tc.why)
		}
	}
}

// TestCTAQNameCast pins the Type Alternative façade's answer: a StringLiteral's
// cast to xs:QName evaluates there too (#888), so B.2's eq and ne rows over two
// such casts decide, by expanded name, and its missing ordering rows are
// err:XPTY0004, false under fn:not. An attribute is untyped there
// (key-cta-ta-select clause 1's Note): compared with such a cast it is cast to
// xs:QName and raises, and as the cast's own operand it declines, so it is
// never true. An unbound prefix in the literal declines and is no static error
// CTATestStaticError reports.
//
// With castTarget declining an xs:QName target, the first row declines. With
// LookupNamespace answering a prefix as its own namespace, the first two rows
// fail; with the cast's names not set or not scoped onto its operand, the
// three true rows fail; with the static context reaching every datatype
// validation, `@k = xs:QName('p:a')` is true; with xs:QName's value equality
// reading the local part alone, the two r:a rows fail. With castsFrom's
// xs:QName rule removed, the two attribute-operand declines compile; with
// ctaNames.resolvesLiteral's check skipped, the zz one does.
func TestCTAQNameCast(t *testing.T) {
	bindings := []string{"xs", xsd.XMLSchemaNS, "p", "urn:p", "q", "urn:p", "r", "urn:r"}
	attrs := ctaAttrs(at("k", "p:a"))
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"xs:QName('p:a') = xs:QName('q:a')", true},
		{"'p:a' cast as xs:QName = xs:QName('q:a')", true},
		{"xs:QName('p:a') = xs:QName('r:a')", false},
		{"xs:QName('p:a') != xs:QName('r:a')", true},
		{"xs:QName('p:a') < xs:QName('q:a')", false},
		{"not(xs:QName('p:a') < xs:QName('q:a'))", false},
		{"@k = xs:QName('p:a')", false},
		{"not(@k = xs:QName('p:a'))", false},
	} {
		got := compile(t, tc.expr, bindings...).Evaluate(backend(), seededTypes, attrs)
		if got != tc.want {
			t.Errorf("CTA Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
	for _, expr := range []string{"xs:QName(@k) = xs:QName('p:a')", "@k cast as xs:QName = xs:QName('p:a')", "xs:QName('zz:a') = xs:QName('p:a')"} {
		record := ctaExprRecord(expr, "", bindings...)
		if _, ok := CompileCTATest(record, seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
		if err := CTATestStaticError(record, seededTypes); err != nil {
			t.Errorf("CTATestStaticError(%q) = %v, want nil: the decline is no charge", expr, err)
		}
	}
}
