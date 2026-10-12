package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// The fixtures below drive the assertion façade: the §3.12.6 grammar over an
// element whose attributes are TYPED (cvc-assertion clause 1, §3.13.4.1).

// asTyped is one attribute of the element an assertion is evaluated against:
// its ·expanded name·, the builtin type its use declares, and the lexical its
// typed value is read from — mapped to an ·actual value·, or taken as
// xs:untypedAtomic where the type is ·special·.
type asTyped struct {
	name    xsd.QName
	typ     string
	lexical string
}

// asBuiltin is the seeded builtin simple type named local, or a fatal error.
func asBuiltin(t *testing.T, local string) *xsd.SimpleType {
	t.Helper()
	td, declared := seededTypes.Type(ctaBuiltin(local))
	if !declared {
		t.Fatalf("the seeded builtins hold no xs:%s", local)
	}
	st, simple := td.(*xsd.SimpleType)
	if !simple {
		t.Fatalf("xs:%s is not a simple type", local)
	}
	return st
}

// asUses is the [AttributeTypes] of an element whose governing type declares
// one use per entry of uses, name to builtin type local name, and below which
// no element has a static type: every non-empty path answers false.
func asUses(t *testing.T, uses map[string]string) AttributeTypes {
	t.Helper()
	typed := make(map[xsd.QName]*xsd.SimpleType, len(uses))
	for local, typ := range uses {
		typed[uq(local)] = asBuiltin(t, typ)
	}
	return func(path []xsd.QName, name xsd.QName) (*xsd.SimpleType, bool) {
		if len(path) != 0 {
			return nil, false
		}
		st, ok := typed[name]
		return st, ok
	}
}

// asValues is the [TypedAttributes] over attrs in the order WRITTEN, on the
// terms [TypedAttributes] states: [Untyped] of the lexical where the builtin
// type is ·special·, and [Typed] of the value the backend maps from the lexical
// against it otherwise.
func asValues(t *testing.T, attrs ...asTyped) TypedAttributes {
	t.Helper()
	vs := make([]TypedValue, 0, len(attrs))
	for _, a := range attrs {
		st := asBuiltin(t, a.typ)
		if st.IsSpecial() {
			vs = append(vs, Untyped(a.lexical))
			continue
		}
		v, err := value.ValidateLexical(backend(), seededTypes, st, a.lexical, nil, FacetAssertions(time.Time{}))
		if err != nil {
			t.Fatalf("mapping %q as xs:%s: %v", a.lexical, a.typ, err)
		}
		vs = append(vs, Typed(v))
	}
	return func(yield func(xsd.QName, TypedValue) bool) {
		for i, a := range attrs {
			if !yield(a.name, vs[i]) {
				return
			}
		}
	}
}

// asNoElems is the [ElementTypes] of an element whose governing type declares
// no child element, so every child-axis step declines.
func asNoElems([]xsd.QName) (xsd.TypeDefinition, bool) { return nil, false }

// asNoChildren is the [ChildElements] of an element with no element child.
func asNoChildren(func(ChildElement) bool) {}

// asElement is a [ContextElement] built from literals: E's ·expanded name·
// and its in-scope bindings, prefix to namespace name, the default namespace
// under "". Its LookupPrefix answers ("", true) for the empty prefix where no
// default namespace is bound, as validate's source adapters do, and asked
// records each prefix it was asked about, in order, where it is non-nil.
type asElement struct {
	name     xsd.QName
	bindings map[string]string
	asked    *[]string
}

func (e asElement) Name() xsd.QName { return e.name }

func (e asElement) LookupPrefix(prefix string) (string, bool) {
	if e.asked != nil {
		*e.asked = append(*e.asked, prefix)
	}
	uri, ok := e.bindings[prefix]
	if !ok && prefix == "" {
		return "", true
	}
	return uri, ok
}

// asElem is the element an evaluation that reads neither E's name nor its
// bindings is handed: e in no namespace, binding nothing.
var asElem = asElement{name: uq("e")}

// asCompile compiles expr against uses, for an element with empty content, or
// fails the test.
func asCompile(t *testing.T, expr string, uses AttributeTypes) AssertionTest {
	t.Helper()
	return asCompileFor(t, expr, xsd.EmptyContent{}, uses)
}

// asCompileFor compiles expr against uses for an element whose {content type}
// is content, or fails the test.
func asCompileFor(t *testing.T, expr string, content xsd.ContentType, uses AttributeTypes) AssertionTest {
	t.Helper()
	c, ok := CompileAssertionTest(asRecord(expr), seededTypes, content, uses, asNoElems)
	if !ok {
		t.Fatalf("CompileAssertionTest(%q): declined, want compiled", expr)
	}
	return c
}

// asRecord is expr's XPath Expression property record, binding xs and a.
func asRecord(expr string) xsd.XPathExpression {
	return ctaExprRecord(expr, "", "xs", xsd.XMLSchemaNS, "a", "http://example.com/a")
}

// An assertion reads its attributes TYPED, so a comparison runs in the
// attributes' own types and not in xs:string. `@min <= @max` over min="10" and
// max="9" is the discriminating case: as xs:int values 10 <= 9 is false, while
// the untyped reading a Type Alternative takes compares the strings "10" and
// "9" and answers true (TestEvaluateUntypedReadingOfTypedNames pins that half).
func TestAssertionEvaluatesTypedAttributes(t *testing.T) {
	uses := asUses(t, map[string]string{"x": "integer", "min": "int", "max": "int", "s": "string", "d": "double"})
	for _, tc := range []struct {
		expr  string
		attrs []asTyped
		want  bool
	}{
		{"@x > 300", []asTyped{{uq("x"), "integer", "500"}}, true},
		{"@x > 300", []asTyped{{uq("x"), "integer", "200"}}, false},
		{"@x > 300", nil, false},
		{"@x = 300", []asTyped{{uq("x"), "integer", "+0300"}}, true},
		{"@min <= @max", []asTyped{{uq("min"), "int", "10"}, {uq("max"), "int", "9"}}, false},
		{"@min <= @max", []asTyped{{uq("min"), "int", "9"}, {uq("max"), "int", "10"}}, true},
		{"@d > 1.5", []asTyped{{uq("d"), "double", "2"}}, true},
		{"@s = 'abc'", []asTyped{{uq("s"), "string", "abc"}}, true},
		{"@x", []asTyped{{uq("x"), "integer", "0"}}, true},
		{"@x", nil, false},
		{"not(@x) or @x > 0", nil, true},
		{"@s cast as xs:integer > 3", []asTyped{{uq("s"), "string", " 5 "}}, true},
		{"xs:integer(@s) = 5", []asTyped{{uq("s"), "string", "5"}}, true},
	} {
		got := asCompile(t, tc.expr, uses).Evaluate(backend(), seededTypes, asElem, asValues(t, tc.attrs...), asNoChildren, nil, ValueBinding{}, time.Time{})
		if got != tc.want {
			t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.attrs, got, tc.want)
		}
	}
}

// A TYPE ERROR is reachable over typed operands that an untyped reading never
// raises: `@b = 'true'` with @b typed xs:boolean compares an xs:boolean with
// an xs:string, which is err:XPTY0004 (xpath20.md §3.5.2, B.2). cvc-assertion
// makes a {test} that raises false — "evaluates to true ... without raising
// any dynamic error or type error" — so Evaluate answers false and the caller
// charges cvc-assertion. Spec-correct under the typed reading; do not "fix" it
// into a cast.
func TestAssertionTypeErrorIsFalse(t *testing.T) {
	uses := asUses(t, map[string]string{"b": "boolean"})
	attrs := asValues(t, asTyped{uq("b"), "boolean", "true"})

	if asCompile(t, "@b = 'true'", uses).Evaluate(backend(), seededTypes, asElem, attrs, asNoChildren, nil, ValueBinding{}, time.Time{}) {
		t.Error("Evaluate(@b = 'true') over a typed xs:boolean = true, want false: the comparison raises err:XPTY0004")
	}
	if asCompile(t, "not(@b = 'true')", uses).Evaluate(backend(), seededTypes, asElem, attrs, asNoChildren, nil, ValueBinding{}, time.Time{}) {
		t.Error("Evaluate(not(@b = 'true')) = true, want false: fn:not propagates the raised error, and the {test} raised")
	}
	if !asCompile(t, "@b = @b", uses).Evaluate(backend(), seededTypes, asElem, attrs, asNoChildren, nil, ValueBinding{}, time.Time{}) {
		t.Error("Evaluate(@b = @b) = false, want true: two xs:boolean operands are B.2-comparable")
	}
}

// The same names read UNTYPED through the Type Alternative façade answer the
// other way round, which is what shows the two façades build different trees
// over one grammar: "10" <= "9" holds as xs:string, and @b = 'true' is a
// string comparison that holds.
func TestEvaluateUntypedReadingOfTypedNames(t *testing.T) {
	if !compile(t, "@min <= @max").Evaluate(backend(), seededTypes, ctaAttrs(at("min", "10"), at("max", "9"))) {
		t.Error(`CTA Evaluate(@min <= @max) over "10", "9" = false, want true: untyped operands compare as xs:string`)
	}
	if !compile(t, "@b = 'true'").Evaluate(backend(), seededTypes, ctaAttrs(at("b", "true"))) {
		t.Error("CTA Evaluate(@b = 'true') = false, want true: an untyped operand is cast to xs:string")
	}
}

// asUnion is a union of xs:int and xs:string, whose value takes the type of
// its ·validating· member and so has no type fixed at compile time.
func asUnion(t *testing.T) *xsd.SimpleType {
	t.Helper()
	st, err := xsd.NewSimpleType(xsderr.Loc{}, xsd.QName{Space: ctaUserNS, Local: "IntOrString"},
		xsd.UnionDerivation{Members: []xsd.SimpleTypeOrRef{
			xsd.SimpleTypeRef{Name: ctaBuiltin("int")}, xsd.SimpleTypeRef{Name: ctaBuiltin("string")},
		}}, xsd.SimpleTypeRef{Name: ctaBuiltin("anySimpleType")}, nil, nil)
	if err != nil {
		t.Fatalf("building the union: %v", err)
	}
	return st
}

// An attribute whose type is ·special· is read as xs:untypedAtomic — its
// [schema normalized value], xpath-datamodel §3.3.1.2 and Datatypes dt-xdmrep
// clause 1 — and compared as §3.5.2 compares an untyped operand: against an
// integer literal it is cast to xs:double (clause 2.1), so x="1e0" equals 1,
// which an xs:decimal cast would raise on; against another untypedAtomic both
// are cast to xs:string (clause 1), so "10" > "9" is false; and a lexical that
// does not cast raises err:FORG0001, which cvc-assertion charges as false and
// fn:not propagates. A value comparison casts it to xs:string whatever the
// other operand (§3.5.1 step 4). Each row fails with the ·special· arm of
// ctaAssertionFacade.attribute removed, which declines the {test} instead.
func TestAssertionReadsSpecialAttributesUntyped(t *testing.T) {
	uses := asUses(t, map[string]string{"x": "anySimpleType", "y": "anySimpleType", "atom": "anyAtomicType", "i": "integer"})
	for _, tc := range []struct {
		expr  string
		attrs []asTyped
		want  bool
	}{
		{"@x > 300", []asTyped{{uq("x"), "anySimpleType", "304"}}, true},
		{"@x > 300", []asTyped{{uq("x"), "anySimpleType", "204"}}, false},
		{"@x > 300", nil, false},
		{"@x = 1", []asTyped{{uq("x"), "anySimpleType", "1e0"}}, true},
		{"@x = 1", []asTyped{{uq("x"), "anySimpleType", " 1 "}}, true},
		{"@x > 300", []asTyped{{uq("x"), "anySimpleType", "abc"}}, false},
		{"not(@x > 300)", []asTyped{{uq("x"), "anySimpleType", "abc"}}, false},
		{"not(@x > 300)", []asTyped{{uq("x"), "anySimpleType", "204"}}, true},
		{"@x > @y", []asTyped{{uq("x"), "anySimpleType", "10"}, {uq("y"), "anySimpleType", "9"}}, false},
		{"@x > @y", []asTyped{{uq("x"), "anySimpleType", "9"}, {uq("y"), "anySimpleType", "10"}}, true},
		{"@x = @i", []asTyped{{uq("x"), "anySimpleType", "5.0"}, {uq("i"), "integer", "5"}}, true},
		{"@x eq '304'", []asTyped{{uq("x"), "anySimpleType", "304"}}, true},
		{"@x eq 304", []asTyped{{uq("x"), "anySimpleType", "304"}}, false},
		{"not(@x eq 304)", []asTyped{{uq("x"), "anySimpleType", "304"}}, false},
		{"@x cast as xs:integer gt 3", []asTyped{{uq("x"), "anySimpleType", " 5 "}}, true},
		{"@x", []asTyped{{uq("x"), "anySimpleType", ""}}, true},
		{"@x", nil, false},
		{"@atom = 'a'", []asTyped{{uq("atom"), "anyAtomicType", "a"}}, true},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			got := asCompile(t, tc.expr, uses).Evaluate(backend(), seededTypes, asElem, asValues(t, tc.attrs...), asNoChildren, nil, ValueBinding{}, time.Time{})
			if got != tc.want {
				t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.attrs, got, tc.want)
			}
		})
	}
}

// A value of the wrong arm breaks the obligation [TypedAttributes] states, and
// the node reading it raises rather than answering: a [Typed] value under a
// ·special· name, and an [Untyped] one under a typed name, make the {test}
// false and its fn:not false too.
func TestAssertionRaisesOnWrongArm(t *testing.T) {
	uses := asUses(t, map[string]string{"x": "anySimpleType", "i": "integer"})
	v, err := value.ValidateLexical(backend(), seededTypes, asBuiltin(t, "integer"), "5", nil, FacetAssertions(time.Time{}))
	if err != nil {
		t.Fatalf("mapping 5: %v", err)
	}
	for _, tc := range []struct {
		expr string
		name xsd.QName
		v    TypedValue
	}{
		{"@x = 5", uq("x"), Typed(v)},
		{"not(@x = 5)", uq("x"), Typed(v)},
		{"@x", uq("x"), Typed(v)},
		{"@i = 5", uq("i"), Untyped("5")},
		{"not(@i = 5)", uq("i"), Untyped("5")},
		{"@i", uq("i"), Untyped("5")},
	} {
		attrs := func(yield func(xsd.QName, TypedValue) bool) { yield(tc.name, tc.v) }
		t.Run(tc.expr, func(t *testing.T) {
			if asCompile(t, tc.expr, uses).Evaluate(backend(), seededTypes, asElem, attrs, asNoChildren, nil, ValueBinding{}, time.Time{}) {
				t.Errorf("Evaluate(%q) over the wrong arm = true, want false: the read raises", tc.expr)
			}
		})
	}
}

// CompileAssertionTest DECLINES — ok false, never a tree that answers false —
// every {test} whose operand types it cannot fix, every construct outside the
// §3.12.6 grammar and its value comparisons ($value among them), and every
// decline CompileCTATest itself makes.
func TestCompileAssertionTestDeclines(t *testing.T) {
	union := asUnion(t)
	types := asUses(t, map[string]string{
		"x": "integer", "l": "NMTOKENS",
		"q": "QName", "n": "NOTATION", "f": "float",
	})
	uses := func(path []xsd.QName, name xsd.QName) (*xsd.SimpleType, bool) {
		if len(path) == 0 && name == uq("u") {
			return union, true
		}
		return types(path, name)
	}
	for _, tc := range []struct{ expr, why string }{
		{"@y > 1", "a name with no attribute use, which AttributeTypes does not type"},
		{"@a:x > 1", "the same name in another namespace"},
		{"@* = 1", "a wildcard NameTest can match a wildcard-attributed attribute"},
		{"@a:* = 1", "a prefixed wildcard likewise"},
		{"@*:x = 1", "a local-name wildcard likewise"},
		{"@p:x = 1", "an unbound prefix"},
		{"@l = 'a'", "a list type atomizes to a sequence"},
		{"@u = 1", "a union's value takes its validating member's type"},
		{"@q = 'a'", "an xs:QName value has no canonical representation to convert through"},
		{"@n = 'a'", "an xs:NOTATION value likewise"},
		{"@x cast as xs:string = '5'", "a cast from a typed non-string attribute"},
		{"xs:string(@x) = '5'", "the constructor spelling of the same cast"},
		{"@f = 1e0", "B.1 rule 1.1's xs:float to xs:double promotion, CompileCTATest's own decline"},
		{"@x eq 5 eq 5", "ValueComp is non-associative, so a second one is an unparsed tail"},
		{"upper-case(@x) = '1'", "a function call outside fn:not, fn:count, the constructors and the string and sequence core"},
	} {
		if _, ok := CompileAssertionTest(asRecord(tc.expr), seededTypes, xsd.EmptyContent{}, uses, asNoElems); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined (%s)", tc.expr, tc.why)
		}
	}
}

// The value comparisons (xpath20.md §3.5.1) evaluate over typed attributes:
// `@min le @max` is §3.13.2's own example, decided in the attributes' xs:int
// values, so min="10" max="9" fails it where the string reading would hold.
// An EMPTY operand makes the comparison the empty sequence, whose effective
// boolean value is false, so fn:not over it is TRUE — the row that tells the
// empty sequence apart from err:XPTY0004, which fn:not propagates. A pair of
// operand types B.2 gives the operator no row for is err:XPTY0004, false under
// fn:not as well.
func TestAssertionEvaluatesValueComparisons(t *testing.T) {
	uses := asUses(t, map[string]string{"x": "integer", "min": "int", "max": "int", "s": "string", "b": "boolean", "dur": "duration"})
	for _, tc := range []struct {
		expr  string
		attrs []asTyped
		want  bool
	}{
		{"@min le @max", []asTyped{{uq("min"), "int", "6"}, {uq("max"), "int", "5"}}, false},
		{"@min le @max", []asTyped{{uq("min"), "int", "5"}, {uq("max"), "int", "6"}}, true},
		{"@min le @max", []asTyped{{uq("min"), "int", "10"}, {uq("max"), "int", "9"}}, false},
		{"@min le @max", []asTyped{{uq("min"), "int", "5"}, {uq("max"), "int", "5"}}, true},
		{"@min lt @max", []asTyped{{uq("min"), "int", "5"}, {uq("max"), "int", "5"}}, false},
		{"@x eq 5", []asTyped{{uq("x"), "integer", "+05"}}, true},
		{"@x ne 5", []asTyped{{uq("x"), "integer", "5"}}, false},
		{"@x gt 4.5", []asTyped{{uq("x"), "integer", "5"}}, true},
		{"@x ge 5", []asTyped{{uq("x"), "integer", "4"}}, false},
		{"@s eq 'abc'", []asTyped{{uq("s"), "string", "abc"}}, true},
		{"@b eq xs:boolean('1')", []asTyped{{uq("b"), "boolean", "true"}}, true},
		{"@x eq 5", nil, false},
		{"not(@x eq 5)", nil, true},
		{"@x eq 5 or @min le @max", []asTyped{{uq("min"), "int", "1"}, {uq("max"), "int", "2"}}, true},
		{"@x eq 'a'", []asTyped{{uq("x"), "integer", "5"}}, false},
		{"not(@x eq 'a')", []asTyped{{uq("x"), "integer", "5"}}, false},
		{"@dur lt @dur", []asTyped{{uq("dur"), "duration", "P1D"}}, false},
		{"not(@dur lt @dur)", []asTyped{{uq("dur"), "duration", "P1D"}}, false},
		{"@dur eq @dur", []asTyped{{uq("dur"), "duration", "P1D"}}, true},
	} {
		got := asCompile(t, tc.expr, uses).Evaluate(backend(), seededTypes, asElem, asValues(t, tc.attrs...), asNoChildren, nil, ValueBinding{}, time.Time{})
		if got != tc.want {
			t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.attrs, got, tc.want)
		}
	}
}

// asList is a list of xs:int, whose `$value` is the sequence of its items.
func asList(t *testing.T, name string, item xsd.QName) *xsd.SimpleType {
	t.Helper()
	st, err := xsd.NewSimpleType(xsderr.Loc{}, xsd.QName{Space: ctaUserNS, Local: name},
		xsd.ListDerivation{Item: xsd.SimpleTypeRef{Name: item}},
		xsd.SimpleTypeRef{Name: ctaBuiltin("anySimpleType")},
		[]xsd.Facet{xsd.NewFacet(xsd.FacetWhiteSpace, []string{"collapse"}, true)}, nil)
	if err != nil {
		t.Fatalf("building the %s list: %v", name, err)
	}
	return st
}

// asTypesWith is the seeded builtins plus extra, by name.
func asTypesWith(extra ...*xsd.SimpleType) ctaTestTypes {
	types := make(ctaTestTypes, len(seededTypes)+len(extra))
	for name, td := range seededTypes {
		types[name] = td
	}
	for _, st := range extra {
		types[st.Name()] = st
	}
	return types
}

// asBind maps lexical against st and binds it to $value, or fails the test.
func asBind(t *testing.T, st *xsd.SimpleType, lexical string) ValueBinding {
	t.Helper()
	v, err := value.ValidateLexical(backend(), seededTypes, st, lexical, nil, FacetAssertions(time.Time{}))
	if err != nil {
		t.Fatalf("mapping %q against %s: %v", lexical, st.Name(), err)
	}
	return BindValue(lexical, Typed(v))
}

// `$value` over a SIMPLE {content type} is E's ·actual value· under its {simple
// type definition} (cvc-assertion clause 2.3.1), typed — so `$value eq 5`
// compares xs:int values and "+05" is 5 — and `$value` in the zero
// ValueBinding is the empty sequence clause 2.3.2 gives an invalid or ·nilled·
// E, false under a value comparison and true under fn:not of one. With
// ctaEffectiveBoolean.eval's ctaCarriedType read deleted the bare `$value` row
// over "+05" fails.
func TestAssertionValueOverSimpleContent(t *testing.T) {
	content := xsd.SimpleContent{SimpleType: asBuiltin(t, "int")}
	uses := asUses(t, map[string]string{"max": "int"})
	five := asBind(t, asBuiltin(t, "int"), "+05")
	for _, tc := range []struct {
		expr  string
		bound ValueBinding
		attrs []asTyped
		want  bool
	}{
		{"$value eq 5", five, nil, true},
		{"$value eq 6", five, nil, false},
		{"$value gt 4", five, nil, true},
		{"$value = 5", five, nil, true},
		{"$value le @max", five, []asTyped{{uq("max"), "int", "4"}}, false},
		{"$value le @max", five, []asTyped{{uq("max"), "int", "5"}}, true},
		{"$value", five, nil, true},
		{"$value", asBind(t, asBuiltin(t, "int"), "0"), nil, false},
		{"$value eq 5", ValueBinding{}, nil, false},
		{"not($value eq 5)", ValueBinding{}, nil, true},
		{"$value", ValueBinding{}, nil, false},
	} {
		got := asCompileFor(t, tc.expr, content, uses).Evaluate(backend(), seededTypes, asElem, asValues(t, tc.attrs...), asNoChildren, nil, tc.bound, time.Time{})
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// `$value` under a {content type} that is NOT simple — empty, element-only,
// mixed — is the empty sequence (cvc-assertion clause 2.3.2), decided at
// compile time, so `$value eq 1` is the empty sequence and false and its
// negation true, whatever the evaluation binds: a stray binding is never read.
// With ctaEffectiveBoolean.eval's ctaEmptyValue arm answering true the bare
// `$value` and `not($value)` rows fail; with it deleted none does, the operand
// carrying no type (ctaCarriedType) and so answering the same false.
func TestAssertionValueOverNonSimpleContent(t *testing.T) {
	stray := asBind(t, asBuiltin(t, "int"), "1")
	for _, content := range []xsd.ContentType{
		xsd.EmptyContent{},
		xsd.ElementContent{Mixed: false},
		xsd.ElementContent{Mixed: true},
	} {
		for _, tc := range []struct {
			expr string
			want bool
		}{
			{"$value eq 1", false},
			{"not($value eq 1)", true},
			{"$value = 1", false},
			{"$value", false},
			{"not($value)", true},
			{"$value lt xs:hexBinary('00')", false},
			{"not($value lt xs:hexBinary('00'))", true},
		} {
			test := asCompileFor(t, tc.expr, content, asUses(t, nil))
			for _, bound := range []ValueBinding{{}, stray} {
				if got := test.Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, nil, bound, time.Time{}); got != tc.want {
					t.Errorf("Evaluate(%q) under %s content = %v, want %v", tc.expr, content.Variety(), got, tc.want)
				}
			}
		}
	}
}

// `$value` over a LIST {simple type definition} is the flattened sequence of
// its items (Datatypes dt-xdmrep). A value comparison over two or more items
// raises err:XPTY0004 (xpath20.md §3.5.1 step 3), which cvc-assertion charges
// as false and fn:not propagates; a general comparison is existential over the
// items instead; one item compares as itself, and an empty list is the empty
// sequence. fn:boolean of two or more atomic items raises err:FORG0006.
func TestAssertionValueOverListContent(t *testing.T) {
	list := asList(t, "IntList", ctaBuiltin("int"))
	content := xsd.SimpleContent{SimpleType: list}
	types := asTypesWith(list)
	for _, tc := range []struct {
		expr, lexical string
		want          bool
	}{
		{"$value eq 1", "1 2", false},
		{"not($value eq 1)", "1 2", false},
		{"$value = 2", "1 2", true},
		{"$value = 3", "1 2", false},
		{"$value eq 3", "3", true},
		{"$value eq 1", "", false},
		{"not($value eq 1)", "", true},
		{"$value", "1 2", false},
		{"not($value)", "1 2", false},
		{"$value", "7", true},
	} {
		test, ok := CompileAssertionTest(asRecord(tc.expr), types, content, asUses(t, nil), asNoElems)
		if !ok {
			t.Fatalf("CompileAssertionTest(%q) over a list: declined, want compiled", tc.expr)
		}
		v, err := value.ValidateLexical(backend(), types, list, tc.lexical, nil, FacetAssertions(time.Time{}))
		if err != nil {
			t.Fatalf("mapping %q against the list: %v", tc.lexical, err)
		}
		if got := test.Evaluate(backend(), types, asElem, asValues(t), asNoChildren, nil, BindValue("", Typed(v)), time.Time{}); got != tc.want {
			t.Errorf("Evaluate(%q) over %q = %v, want %v", tc.expr, tc.lexical, got, tc.want)
		}
	}
}

// `$value` over a ·special· {simple type definition} is E's [schema normalized
// value] as one xs:untypedAtomic value (Datatypes dt-xdmrep clause 1), cast as
// an untyped attribute is: to xs:string under a value comparison (xpath20.md
// §3.5.1 step 4), to xs:double against a numeric under a general comparison
// (§3.5.2 clause 2.1), raising err:FORG0001 where it does not cast; its effective boolean
// value is false only for the zero-length string (§2.4.3 rule 4). `$value` in the zero
// ValueBinding is the empty sequence, and a [Typed] binding breaks [BindValue]'s obligation
// and raises. Each row fails with ctaTypes.valueVariable declining a ·special· type.
// With ctaEffectiveBoolean.eval's ctaUntypedValue arm deleted, the `$value` row
// over "0" and the `not($value)` row over the [Typed] binding fail.
func TestAssertionValueOverSpecialContent(t *testing.T) {
	five, err := value.ValidateLexical(backend(), seededTypes, asBuiltin(t, "integer"), "5", nil, FacetAssertions(time.Time{}))
	if err != nil {
		t.Fatalf("mapping 5: %v", err)
	}
	for _, special := range []string{"anySimpleType", "anyAtomicType"} {
		content := xsd.SimpleContent{SimpleType: asBuiltin(t, special)}
		for _, tc := range []struct {
			expr  string
			bound ValueBinding
			want  bool
		}{
			{"$value eq 'x'", BindValue("x", Untyped("x")), true},
			{"$value eq 'x'", BindValue(" x", Untyped(" x")), false},
			{"$value = 5", BindValue("5.0", Untyped("5.0")), true},
			{"$value eq 5", BindValue("5", Untyped("5")), false},
			{"not($value eq 5)", BindValue("5", Untyped("5")), false},
			{"$value = 5", BindValue("five", Untyped("five")), false},
			{"not($value = 5)", BindValue("five", Untyped("five")), false},
			{"$value cast as xs:integer eq 5", BindValue(" 5 ", Untyped(" 5 ")), true},
			{"$value", BindValue("0", Untyped("0")), true},
			{"$value", BindValue("", Untyped("")), false},
			{"not($value)", BindValue("", Untyped("")), true},
			{"$value eq 'x'", ValueBinding{}, false},
			{"not($value eq 'x')", ValueBinding{}, true},
			{"$value", ValueBinding{}, false},
			{"$value eq 'x'", BindValue("", Typed(five)), false},
			{"not($value eq 'x')", BindValue("", Typed(five)), false},
			{"not($value)", BindValue("", Typed(five)), false},
		} {
			t.Run(special+" "+tc.expr, func(t *testing.T) {
				got := asCompileFor(t, tc.expr, content, asUses(t, nil)).Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, nil, tc.bound, time.Time{})
				if got != tc.want {
					t.Errorf("Evaluate(%q) over %s content = %v, want %v", tc.expr, special, got, tc.want)
				}
			})
		}
	}
}

// `$value` DECLINES where its static type is not fixed at compile time, or is
// one this engine does not read: a nil {content type}, a union, a list of a
// union, an xs:QName or xs:NOTATION primitive — and any variable but `$value`,
// which is not in scope (err:XPST0008), and a cast from a non-string `$value`,
// on a typed attribute's terms.
func TestCompileAssertionTestDeclinesValue(t *testing.T) {
	union := asUnion(t)
	unionList := asList(t, "UnionList", union.Name())
	types := asTypesWith(union, unionList)
	for _, tc := range []struct {
		expr    string
		content xsd.ContentType
		why     string
	}{
		{"$value eq 1", nil, "no {content type} to type $value by"},
		{"$value eq 1", xsd.SimpleContent{SimpleType: union}, "a union's value takes its active member's type"},
		{"$value = 1", xsd.SimpleContent{SimpleType: unionList}, "a list of a union likewise"},
		{"$value = 'a'", xsd.SimpleContent{SimpleType: asBuiltin(t, "QName")}, "an xs:QName value has no canonical representation"},
		{"$value = 'a'", xsd.SimpleContent{SimpleType: asBuiltin(t, "NOTATION")}, "an xs:NOTATION value likewise"},
		{"$other eq 1", xsd.SimpleContent{SimpleType: asBuiltin(t, "int")}, "no variable but $value is in scope"},
		{"$a:value eq 1", xsd.SimpleContent{SimpleType: asBuiltin(t, "int")}, "$value has no namespace"},
		{"$value cast as xs:string = '5'", xsd.SimpleContent{SimpleType: asBuiltin(t, "int")}, "a cast from a typed non-string $value"},
		{"$ eq 1", xsd.SimpleContent{SimpleType: asBuiltin(t, "int")}, "a '$' with no name"},
	} {
		if _, ok := CompileAssertionTest(asRecord(tc.expr), types, tc.content, asUses(t, nil), asNoElems); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined (%s)", tc.expr, tc.why)
		}
	}
}

// A Type Alternative's {test} still declines $value, the value comparators, a
// child-axis step and a rooted path: the assertion façade widened nothing in
// the grammar CompileCTATest admits.
func TestCompileCTATestStillDeclinesAssertionOnlyForms(t *testing.T) {
	for _, expr := range []string{"$value > 0", "@min le @max", "@x eq 5", "e1 = 'present'", "/r = 'present'", "//r"} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, ""), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
	}
}

// A general or value comparison whose comparison type is in the date/time
// family DECIDES at CompileAssertionTest, under the implicit timezone F&O §10.4
// assumes on an operand without one (ctaImplicitTimezone, Z):
//
//   - a timezone-MIXED pair is a total order, so `@d < @e or @d >= @e` over
//     2000-01-01 and 2000-01-01Z is true, and 2000-01-01 at Z equals
//     2000-01-01Z — including across the day boundary, where 2000-01-01 at Z is
//     after 2000-01-01+14:00. Every mixed row but `@d lt @e` and 1976-02
//     `eq` 1976-03Z, each false either way, fails with the date/time arm of
//     ctaHoldsPair removed, which leaves the pair value.Incomparable and
//     unequal.
//   - two untimezoned operands, and two timezoned ones, compare as before the
//     arm: the implicit timezone given to the left operand alone breaks the
//     untimezoned `@d = @e` row.
//   - the g* types have eq and ne alone (xpath20.md B.2), so an ordering over
//     one is err:XPTY0004 — false, and false under fn:not — whatever the
//     timezones; their equality compares starting instants.
//   - an xs:untypedAtomic operand is cast to xs:date against a date under a
//     general comparison (§3.5.2 clause 2.4) and decided at Z, and to xs:string
//     under a value comparison (§3.5.1 step 4), which is err:XPTY0004.
//
// xs:dateTimeStamp reaches the arm through its xs:dateTime primitive, and the
// 24:00:00 rows pin that the canonical round-trip needs no case of its own.
func TestAssertionDecidesDateTimeComparisons(t *testing.T) {
	uses := asUses(t, map[string]string{
		"d": "date", "e": "date", "dt": "dateTime", "dts": "dateTimeStamp", "tm": "time",
		"gym": "gYearMonth", "gy": "gYear", "gmd": "gMonthDay", "gd": "gDay", "gm": "gMonth",
		"x": "anySimpleType",
	})
	date := func(name, lexical string) asTyped { return asTyped{uq(name), "date", lexical} }
	for _, tc := range []struct {
		expr  string
		attrs []asTyped
		want  bool
	}{
		// Mixed: decided at Z.
		{"@d < @e or @d >= @e", []asTyped{date("d", "2000-01-01"), date("e", "2000-01-01Z")}, true},
		{"@d = @e", []asTyped{date("d", "2000-01-01"), date("e", "2000-01-01Z")}, true},
		{"@d != @e", []asTyped{date("d", "2000-01-01"), date("e", "2000-01-01Z")}, false},
		{"@d eq @e", []asTyped{date("d", "2000-01-01"), date("e", "2000-01-01Z")}, true},
		{"@d le @e", []asTyped{date("d", "2000-01-01"), date("e", "2000-01-01Z")}, true},
		{"@d lt @e", []asTyped{date("d", "2000-01-01"), date("e", "2000-01-01Z")}, false},
		{"@d > @e", []asTyped{date("d", "2000-01-01"), date("e", "2000-01-01+14:00")}, true},
		{"@e < @d", []asTyped{date("d", "2000-01-01"), date("e", "2000-01-01+14:00")}, true},
		{"@dt = xs:dateTime('2000-01-01T00:00:00Z')", []asTyped{{uq("dt"), "dateTime", "2000-01-01T00:00:00"}}, true},
		{"@dts = xs:dateTime('2000-01-01T00:00:00')", []asTyped{{uq("dts"), "dateTimeStamp", "2000-01-01T00:00:00Z"}}, true},
		{"@tm ge xs:time('12:00:00Z')", []asTyped{{uq("tm"), "time", "12:00:00"}}, true},
		{"@gym eq xs:gYearMonth('1976-02Z')", []asTyped{{uq("gym"), "gYearMonth", "1976-02"}}, true},
		{"@gy = xs:gYear('2000Z')", []asTyped{{uq("gy"), "gYear", "2000"}}, true},
		{"@gmd = xs:gMonthDay('--01-01Z')", []asTyped{{uq("gmd"), "gMonthDay", "--01-01"}}, true},
		{"@gd = xs:gDay('---01Z')", []asTyped{{uq("gd"), "gDay", "---01"}}, true},
		{"@gm ne xs:gMonth('--01Z')", []asTyped{{uq("gm"), "gMonth", "--01"}}, false},
		{"@gym eq xs:gYearMonth('1976-03Z')", []asTyped{{uq("gym"), "gYearMonth", "1976-02"}}, false},
		// Two untimezoned, and two timezoned: as before.
		{"@d = @e", []asTyped{date("d", "2000-01-01"), date("e", "2000-01-01")}, true},
		{"@d < @e", []asTyped{date("d", "2000-01-01"), date("e", "2000-01-02")}, true},
		{"@d = @e", []asTyped{date("d", "2000-01-01Z"), date("e", "2000-01-01+14:00")}, false},
		{"@d > @e", []asTyped{date("d", "2000-01-01Z"), date("e", "2000-01-01+14:00")}, true},
		{"@gd = xs:gDay('---02+14:00')", []asTyped{{uq("gd"), "gDay", "---01-10:00"}}, true},
		{"xs:time('08:00:00+09:00') eq xs:time('17:00:00-06:00')", nil, false},
		{"xs:time('24:00:00') lt xs:time('23:59:59')", nil, true},
		{"xs:time('24:00:00+01:00') eq xs:time('00:00:00+01:00')", nil, true},
		{"xs:dateTime('1999-12-31T24:00:00') eq xs:dateTime('2000-01-01T00:00:00')", nil, true},
		// g* orderings: err:XPTY0004 whatever the timezones.
		{"@gd < xs:gDay('---02')", []asTyped{{uq("gd"), "gDay", "---01"}}, false},
		{"not(@gd < xs:gDay('---02'))", []asTyped{{uq("gd"), "gDay", "---01"}}, false},
		{"not(@gym lt xs:gYearMonth('1976-03Z'))", []asTyped{{uq("gym"), "gYearMonth", "1976-02"}}, false},
		// xs:untypedAtomic: cast to xs:date by §3.5.2, to xs:string by §3.5.1.
		{"@x < xs:date('2000-01-01Z') or @x >= xs:date('2000-01-01Z')", []asTyped{{uq("x"), "anySimpleType", "2000-01-01"}}, true},
		{"@x = xs:date('2000-01-01Z')", []asTyped{{uq("x"), "anySimpleType", "2000-01-01"}}, true},
		{"@x eq xs:date('2000-01-01Z')", []asTyped{{uq("x"), "anySimpleType", "2000-01-01"}}, false},
		{"not(@x eq xs:date('2000-01-01Z'))", []asTyped{{uq("x"), "anySimpleType", "2000-01-01"}}, false},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			got := asCompile(t, tc.expr, uses).Evaluate(backend(), seededTypes, asElem, asValues(t, tc.attrs...), asNoChildren, nil, ValueBinding{}, time.Time{})
			if got != tc.want {
				t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.attrs, got, tc.want)
			}
		})
	}
}

// ctaImplicitTimezone is a legal timezoneFrag: an untimezoned xs:date lexical
// with it appended validates, and the value it maps to carries a timezone —
// which is the range check §3.3.7's timezoneFrag makes, so none is written here.
func TestImplicitTimezoneIsATimezone(t *testing.T) {
	v, err := value.ValidateLexical(backend(), seededTypes, asBuiltin(t, "date"), "2000-01-01"+ctaImplicitTimezone, nil, FacetAssertions(time.Time{}))
	if err != nil {
		t.Fatalf("validating 2000-01-01%s as xs:date: %v", ctaImplicitTimezone, err)
	}
	tz, aware := v.(value.TimezoneAware)
	if !aware || !tz.HasTimezone() {
		t.Errorf("2000-01-01%s as xs:date carries no timezone", ctaImplicitTimezone)
	}
}
