package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// The fixtures below drive the assertion façade's child-axis step and rooted
// path: an assertion reads E's element [[children]], typed (cvc-assertion
// clause 1.2), and E is the root of the instance it reads (clause 1.3).

// asComplex is a named complex type over content, which a child element of
// that type is read under.
func asComplex(t *testing.T, local string, content xsd.ContentType) xsd.ComplexType {
	t.Helper()
	ct, err := xsd.NewComplexType(xsderr.Loc{}, xsd.QName{Space: ctaUserNS, Local: local}, ctaBuiltin("anyType"), nil,
		xsd.DerivationRestriction, false, nil, nil, nil, content, nil, nil)
	if err != nil {
		t.Fatalf("building %s: %v", local, err)
	}
	return ct
}

// asElementContent is an element-only or mixed {content type} whose
// {particle} is an empty sequence.
func asElementContent(t *testing.T, mixed bool) xsd.ElementContent {
	t.Helper()
	g, err := xsd.NewModelGroup(xsderr.Loc{}, xsd.CompositorSequence, nil)
	if err != nil {
		t.Fatalf("NewModelGroup: %v", err)
	}
	o, err := xsd.NewOccurs(xsderr.Loc{}, 1, 1)
	if err != nil {
		t.Fatalf("NewOccurs: %v", err)
	}
	p, err := xsd.NewParticle(xsderr.Loc{}, o, xsd.ResolvedTerm{Term: g})
	if err != nil {
		t.Fatalf("NewParticle: %v", err)
	}
	return xsd.ElementContent{Mixed: mixed, Particle: p}
}

// asElems is the [ElementTypes] of an element whose governing type declares
// one child element per entry of elems, name to type, and no element below
// them: a path of any length but one answers false.
func asElems(elems map[xsd.QName]xsd.TypeDefinition) ElementTypes {
	return func(path []xsd.QName) (xsd.TypeDefinition, bool) {
		if len(path) != 1 {
			return nil, false
		}
		td, ok := elems[path[0]]
		return td, ok
	}
}

// asChild is one element child of E: its name, the builtin type its value is
// mapped under, and its lexical; nilled is a ·nilled· child, whose lexical is
// not read.
type asChild struct {
	name    xsd.QName
	typ     string
	lexical string
	nilled  bool
}

// asChildren is the [ChildElements] over children in the order WRITTEN, each
// [Typed] of the value the backend maps from its lexical, or nil for a
// ·nilled· one.
func asChildren(t *testing.T, children ...asChild) ChildElements {
	t.Helper()
	out := make([]ChildElement, 0, len(children))
	for _, c := range children {
		if c.nilled {
			out = append(out, Child(c.name, nil))
			continue
		}
		v, err := value.ValidateLexical(backend(), seededTypes, asBuiltin(t, c.typ), c.lexical, nil, FacetAssertions(time.Time{}))
		if err != nil {
			t.Fatalf("mapping %q as xs:%s: %v", c.lexical, c.typ, err)
		}
		out = append(out, Child(c.name, Typed(v)))
	}
	return func(yield func(ChildElement) bool) {
		for _, c := range out {
			if !yield(c) {
				return
			}
		}
	}
}

// asChildTypes is E's child element types for the child-axis rows: e1, a, b
// and d are xs:string, n is xs:int, and c is a complex type with simple
// content over xs:int.
func asChildTypes(t *testing.T) ElementTypes {
	t.Helper()
	str, i := asBuiltin(t, "string"), asBuiltin(t, "int")
	return asElems(map[xsd.QName]xsd.TypeDefinition{
		uq("e1"): str, uq("a"): str, uq("b"): str, uq("d"): str, uq("n"): i,
		uq("c"): asComplex(t, "IntContent", xsd.SimpleContent{SimpleType: i}),
	})
}

// A child-axis step reads E's element [[children]] (cvc-assertion clause 1.2)
// TYPED, under the simple type the child's ·locally declared type· has its
// value in (xpath-datamodel §6.2.4). `e1 = 'present'` holds over
// <e1>present</e1> and is false over <e1>absent</e1>. The values are typed: `n
// > 9` over n = 10 holds as xs:int, where an xs:string comparison would not.
// Two children of one name are a sequence: a general comparison is existential
// over it and a value comparison is err:XPTY0004 (§3.5.1 step 3), false under
// fn:not too. A ·nilled· child has no atom, so `e1 = 'present'` and `e1 = ”`
// are false and the first one's fn:not true. A step whose existence alone is
// asked, `a and b`, reads the [Tally] instead (assertionstep_test.go). Every
// row declines at CompileAssertionTest, and so fails, with
// ctaAssertionFacade.child declining.
func TestAssertionReadsChildElements(t *testing.T) {
	elems := asChildTypes(t)
	present := asChild{uq("e1"), "string", "present", false}
	absent := asChild{uq("e1"), "string", "absent", false}
	nilled := asChild{name: uq("e1"), nilled: true}
	two := asChild{uq("n"), "int", "2", false}
	for _, tc := range []struct {
		expr     string
		children []asChild
		want     bool
	}{
		{"e1 = 'present'", []asChild{present}, true},
		{"e1 = 'present'", []asChild{absent}, false},
		{"e1 = 'present'", nil, false},
		{"e1 eq 'present'", []asChild{present}, true},
		{"not(e1 = 'present')", []asChild{absent}, true},
		{"n > 9", []asChild{{uq("n"), "int", "10", false}}, true},
		{"n = 2", []asChild{{uq("n"), "int", "1", false}, two}, true},
		{"n eq 2", []asChild{two, two}, false},
		{"not(n eq 2)", []asChild{two, two}, false},
		{"c = 5", []asChild{{uq("c"), "int", "+5", false}}, true},
		{"e1 cast as xs:token = 'present'", []asChild{present}, true},
		{"e1 = 'present'", []asChild{nilled}, false},
		{"not(e1 = 'present')", []asChild{nilled}, true},
		{"e1 = ''", []asChild{nilled}, false},
		{"e1 = ''", []asChild{{uq("e1"), "string", "", false}}, true},
		{"e1 = 'present' and @x = 1", []asChild{present}, false},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			test, ok := CompileAssertionTest(asRecord(tc.expr), seededTypes, xsd.ElementContent{}, asUses(t, map[string]string{"x": "int"}), elems)
			if !ok {
				t.Fatalf("CompileAssertionTest(%q): declined, want compiled", tc.expr)
			}
			if got := test.Evaluate(backend(), seededTypes, asElem, asValues(t), asChildren(t, tc.children...), nil, ValueBinding{}, time.Time{}); got != tc.want {
				t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.children, got, tc.want)
			}
		})
	}
}

// A child's value of the wrong arm breaks the obligation [ChildElements]
// states, and every node reading it raises: an [Untyped] value under a typed
// name makes `e1 = 'present'`, its fn:not, and a cast of `e1` false.
func TestAssertionRaisesOnAnUntypedChild(t *testing.T) {
	children := func(yield func(ChildElement) bool) { yield(Child(uq("e1"), Untyped("present"))) }
	for _, expr := range []string{"e1 = 'present'", "not(e1 = 'present')", "e1 cast as xs:string"} {
		test, ok := CompileAssertionTest(asRecord(expr), seededTypes, xsd.ElementContent{}, asUses(t, nil), asChildTypes(t))
		if !ok {
			t.Fatalf("CompileAssertionTest(%q): declined, want compiled", expr)
		}
		if test.Evaluate(backend(), seededTypes, asElem, asValues(t), children, nil, ValueBinding{}, time.Time{}) {
			t.Errorf("Evaluate(%q) over an Untyped child = true, want false: the read raises", expr)
		}
	}
}

// A path opening with "/" or "//" raises err:XPDY0050: E is the root of the
// instance cvc-assertion clause 1.3 builds, an element node with no document
// node above it (xpath20.md §3.2). So `/r = 'present'` is false whatever E is
// named and whatever its children hold — over <r><e1>present</e1></r> and
// <r><e1>absent</e1></r> alike — and its fn:not is false too, the error
// propagating. Each row fails with ctaAssertionFacade.rooted declining, which
// is the compile path declining "/".
func TestAssertionRootedPathRaises(t *testing.T) {
	elems := asChildTypes(t)
	for _, expr := range []string{"/r = 'present'", "not(/r = 'present')", "/r", "//e1 = 'present'", "/e1 eq 'present'"} {
		test, ok := CompileAssertionTest(asRecord(expr), seededTypes, xsd.ElementContent{}, asUses(t, nil), elems)
		if !ok {
			t.Fatalf("CompileAssertionTest(%q): declined, want compiled", expr)
		}
		for _, e1 := range []string{"present", "absent"} {
			if test.Evaluate(backend(), seededTypes, asElem, asValues(t), asChildren(t, asChild{uq("e1"), "string", e1, false}), nil, ValueBinding{}, time.Time{}) {
				t.Errorf("Evaluate(%q) over <e1>%s</e1> = true, want false: the leading slash raises err:XPDY0050", expr, e1)
			}
		}
	}
}

// An unprefixed child NameTest takes the {default namespace} — the resolved
// xpathDefaultNamespace — and a prefixed one its binding (xpath20.md
// §3.2.1.2, PRINCIPLES 15). The parser resolves ##targetNamespace, an explicit
// URI, ##defaultNamespace and ##local to a present URI or to absent; a present
// one makes `e1` the child {urn:t}e1 and not the no-namespace e1, and an absent
// one the reverse.
func TestAssertionChildStepReadsTheDefaultNamespace(t *testing.T) {
	str := asBuiltin(t, "string")
	tns, local := xsd.QName{Space: "urn:t", Local: "e1"}, uq("e1")
	elems := asElems(map[xsd.QName]xsd.TypeDefinition{tns: str, local: str})
	present, err := value.ValidateLexical(backend(), seededTypes, str, "present", nil, FacetAssertions(time.Time{}))
	if err != nil {
		t.Fatalf("mapping present: %v", err)
	}
	for _, tc := range []struct {
		why       string
		record    xsd.XPathExpression
		childName xsd.QName
		want      bool
	}{
		{"a present default namespace names the qualified child", ctaExprRecord("e1 = 'present'", "urn:t"), tns, true},
		{"a present default namespace misses the unqualified child", ctaExprRecord("e1 = 'present'", "urn:t"), local, false},
		{"an absent default namespace names the unqualified child", ctaExprRecord("e1 = 'present'", ""), local, true},
		{"an absent default namespace misses the qualified child", ctaExprRecord("e1 = 'present'", ""), tns, false},
		{"a prefix names its binding whatever the default", ctaExprRecord("t:e1 = 'present'", "", "t", "urn:t"), tns, true},
	} {
		test, ok := CompileAssertionTest(tc.record, seededTypes, xsd.ElementContent{}, asUses(t, nil), elems)
		if !ok {
			t.Fatalf("%s: CompileAssertionTest(%q) declined, want compiled", tc.why, tc.record.Expression())
		}
		children := func(yield func(ChildElement) bool) { yield(Child(tc.childName, Typed(present))) }
		if got := test.Evaluate(backend(), seededTypes, asElem, asValues(t), children, nil, ValueBinding{}, time.Time{}); got != tc.want {
			t.Errorf("%s: Evaluate = %v, want %v", tc.why, got, tc.want)
		}
	}
}

// A child-axis step DECLINES where the child's type is not fixed at compile
// time, or is neither one simple type this engine reads nor of mixed content
// (ctaAssertionFacade.child; assertionmixed_test.go reads the mixed one),
// and every path but one step, rooted or not, declines in the grammar in a
// comparison's position — the one position a child path of two or more steps
// is admitted in is the whole operand of fn:exists, fn:empty or an ·effective
// boolean value· (assertionpath_test.go).
func TestCompileAssertionTestDeclinesChildSteps(t *testing.T) {
	union := asUnion(t)
	types := asTypesWith(union)
	elems := asElems(map[xsd.QName]xsd.TypeDefinition{
		uq("e1"):    asBuiltin(t, "string"),
		uq("n"):     asBuiltin(t, "int"),
		uq("s"):     asBuiltin(t, "anySimpleType"),
		uq("atom"):  asBuiltin(t, "anyAtomicType"),
		uq("l"):     asBuiltin(t, "NMTOKENS"),
		uq("u"):     union,
		uq("q"):     asBuiltin(t, "QName"),
		uq("sc"):    asComplex(t, "SpecialContent", xsd.SimpleContent{SimpleType: asBuiltin(t, "anySimpleType")}),
		uq("eo"):    asComplex(t, "ElementOnly", asElementContent(t, false)),
		uq("empty"): asComplex(t, "Empty", xsd.EmptyContent{}),
	})
	for _, tc := range []struct{ expr, why string }{
		{"zz = 'a'", "a name ElementTypes does not type"},
		{"a:e1 = 'a'", "the same name in another namespace"},
		{"p:e1 = 'a'", "an unbound prefix"},
		{"* = 'a'", "a wildcard NameTest"},
		{"a:* = 'a'", "a prefixed wildcard"},
		{"s = 'a'", "a ·special· type, which an xsi:type can make typed"},
		{"atom = 'a'", "xs:anyAtomicType likewise"},
		{"sc = 'a'", "simple content over a ·special· type likewise"},
		{"l = 'a'", "a list type atomizes to a sequence"},
		{"u = 'a'", "a union's value takes its validating member's type"},
		{"q cast as xs:string = 'a'", "a cast from an xs:QName child to another type"},
		{"eo = 'a'", "element-only content, whose atomization is a type error"},
		{"empty = 'a'", "empty content"},
		{"n cast as xs:string = '5'", "a cast from a typed non-string child"},
		{"child::e1 = 'a'", "the unabbreviated child axis"},
		{"e1/e1 = 'a'", "a path of two steps"},
		{"e1//e1 = 'a'", "a path through descendant-or-self"},
		{"/e1/e1 = 'a'", "a rooted path of two steps"},
		{"/ = 'a'", "a bare slash"},
		{"// = 'a'", "a bare double slash"},
		{"/p:r = 'a'", "an unbound prefix in a rooted step"},
	} {
		if _, ok := CompileAssertionTest(asRecord(tc.expr), types, xsd.ElementContent{}, asUses(t, nil), elems); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined (%s)", tc.expr, tc.why)
		}
	}
}

// ReadsChild reports a child-axis step naming the name, wherever the tree
// holds it — under a comparison, a cast, fn:not, and/or — and nothing else: an
// attribute-only test, `$value`, a rooted path, which raises before it takes a
// step, and a step whose existence alone is asked, `a` in `a and …`, read no
// child.
func TestAssertionTestReadsChild(t *testing.T) {
	elems := asChildTypes(t)
	content := xsd.SimpleContent{SimpleType: asBuiltin(t, "int")}
	uses := asUses(t, map[string]string{"x": "int"})
	for _, tc := range []struct {
		expr  string
		reads []string
	}{
		{"e1 = 'present'", []string{"e1"}},
		{"@x = 1 or not(a and b cast as xs:string = 'b')", []string{"b"}},
		{"e1 and exists(n)", nil},
		{"$value eq n", []string{"n"}},
		{"@x = 1", nil},
		{"$value eq 1", nil},
		{"/e1 = 'present'", nil},
	} {
		test, ok := CompileAssertionTest(asRecord(tc.expr), seededTypes, content, uses, elems)
		if !ok {
			t.Fatalf("CompileAssertionTest(%q): declined, want compiled", tc.expr)
		}
		for _, name := range []string{"e1", "a", "b", "d", "n"} {
			want := false
			for _, r := range tc.reads {
				want = want || r == name
			}
			if got := test.ReadsChild(uq(name)); got != want {
				t.Errorf("(%q).ReadsChild(%s) = %v, want %v", tc.expr, name, got, want)
			}
		}
	}
}
