package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// The fixtures below drive a relative path of two or more child steps,
// `a/b` (xpath20.md §3.2, [26] RelativePathExpr), as the whole operand of
// fn:exists, fn:empty or an ·effective boolean value·: whether it selects a
// node of E's subtree is read off the [Tally] its caller fills, by the chain of
// names from E's child down to each reported element.

// apCompile compiles record for the assert003 shape — element-only content, an
// optional attribute y of xs:anySimpleType, a child a whose type is
// element-only and a child b of a mixed type standing in for xs:anyType — or
// fails the test. A child path consults no ElementTypes, so the types a and b
// carry decide nothing; they are there so the row is assert003's.
func apCompile(t *testing.T, record xsd.XPathExpression) AssertionTest {
	t.Helper()
	elems := asElems(map[xsd.QName]xsd.TypeDefinition{
		uq("a"): asComplex(t, "AOnly", asElementContent(t, false)),
		uq("b"): asComplex(t, "AnyLike", asElementContent(t, true)),
	})
	uses := asUses(t, map[string]string{"x": "anySimpleType", "y": "anySimpleType"})
	test, ok := CompileAssertionTest(record, seededTypes, asElementContent(t, false), uses, elems)
	if !ok {
		t.Fatalf("CompileAssertionTest(%q): declined, want compiled", record.Expression())
	}
	return test
}

// A child path selects the element whose chain below E is exactly its steps:
// fn:exists is true, fn:empty false and the ·effective boolean value· true
// where one such element is reported, and the converse where none is — a lacking
// b, no a at all, b under c rather than a, b one level too deep or too
// shallow, and an a/b in another namespace. Two such elements are one answer.
// `exists(@y) ne exists(a/b)` is assert003's test: invalid with both or
// neither, valid with exactly one. Every row declines at CompileAssertionTest,
// and so fails, with ctaParser.childPath's two call sites removed; the
// false rows over a reported chain hold with ctaChildPath.selectsElement
// answering true.
func TestAssertionChildPathExistence(t *testing.T) {
	y := []asTyped{{uq("y"), "anySimpleType", "204"}}
	ab := acPath("a", "b")
	for _, tc := range []struct {
		why    string
		expr   string
		record xsd.XPathExpression
		attrs  []asTyped
		nodes  []acNode
		holds  bool
	}{
		{why: "b under a", expr: "exists(a/b)", nodes: []acNode{acPath("a"), ab}, holds: true},
		{why: "a lacking b", expr: "exists(a/b)", nodes: []acNode{acPath("a")}},
		{why: "no a", expr: "exists(a/b)"},
		{why: "b under c", expr: "exists(a/b)", nodes: []acNode{acPath("c"), acPath("c", "b")}},
		{why: "b below a's b", expr: "exists(a/b)", nodes: []acNode{acPath("a", "c", "b")}},
		{why: "b as a child", expr: "exists(a/b)", nodes: []acNode{acPath("b")}},
		{why: "two b under a are one answer", expr: "exists(a/b)", nodes: []acNode{ab, ab}, holds: true},
		{why: "empty, b under a", expr: "empty(a/b)", nodes: []acNode{ab}},
		{why: "empty, a lacking b", expr: "empty(a/b)", nodes: []acNode{acPath("a")}, holds: true},
		{why: "empty, no a", expr: "empty(a/b)", holds: true},
		{why: "effective boolean value, b under a", expr: "a/b", nodes: []acNode{ab}, holds: true},
		{why: "effective boolean value, a lacking b", expr: "a/b", nodes: []acNode{acPath("a")}},
		{why: "fn:not over the path", expr: "not(a/b)", nodes: []acNode{acPath("a")}, holds: true},
		{why: "parenthesized", expr: "(a/b)", nodes: []acNode{ab}, holds: true},
		{why: "and over two paths", expr: "a/b and a/c", nodes: []acNode{ab, acPath("a", "c")}, holds: true},
		{why: "or over two paths, one present", expr: "a/b or a/c", nodes: []acNode{acPath("a", "c")}, holds: true},
		{why: "the path as a value comparison operand", expr: "exists(a/b) eq true()", nodes: []acNode{ab}, holds: true},
		{why: "three steps, assert020.v1", expr: "empty(temp/temp/temp)",
			nodes: []acNode{acPath("temp"), acPath("temp", "temp")}, holds: true},
		{why: "three steps, assert020.n1", expr: "empty(temp/temp/temp)",
			nodes: []acNode{acPath("temp"), acPath("temp", "temp"), acPath("temp", "temp", "temp")}},
		{why: "assert003.n1: y and a/b", expr: "exists(@y) ne exists(a/b)", attrs: y, nodes: []acNode{acPath("a"), ab}},
		{why: "assert003.n2: neither", expr: "exists(@y) ne exists(a/b)", nodes: []acNode{acPath("a")}},
		{why: "assert003.v1: y alone", expr: "exists(@y) ne exists(a/b)", attrs: y, nodes: []acNode{acPath("a")}, holds: true},
		{why: "assert003.v2: a/b alone", expr: "exists(@y) ne exists(a/b)", nodes: []acNode{acPath("a"), ab}, holds: true},
		{why: "a present default namespace names the qualified chain",
			record: ctaExprRecord("exists(a/b)", "urn:t"),
			nodes:  []acNode{{path: []xsd.QName{{Space: "urn:t", Local: "a"}, {Space: "urn:t", Local: "b"}}}}, holds: true},
		{why: "a present default namespace misses the unqualified chain",
			record: ctaExprRecord("exists(a/b)", "urn:t"), nodes: []acNode{ab}},
		{why: "a prefix names its binding",
			record: ctaExprRecord("exists(t:a/b)", "", "t", "urn:t"),
			nodes:  []acNode{{path: []xsd.QName{{Space: "urn:t", Local: "a"}, uq("b")}}}, holds: true},
	} {
		t.Run(tc.why, func(t *testing.T) {
			record := tc.record
			if tc.expr != "" {
				record = asRecord(tc.expr)
			}
			test := apCompile(t, record)
			got := test.Evaluate(backend(), seededTypes, asElem, asValues(t, tc.attrs...), asNoChildren, acTally(test, tc.nodes...), ValueBinding{}, time.Time{})
			if got != tc.holds {
				t.Errorf("Evaluate(%q) over %v = %v, want %v", record.Expression(), tc.nodes, got, tc.holds)
			}
		})
	}
}

// A ·nilled· element is a node all the same (xpath-datamodel §6.2.4): the
// caller reports it like any other, so it counts for a child path whether it is
// the path's last step or an intermediate one — what is reported is a chain of
// names, and no value is read.
func TestAssertionChildPathCountsNilledNodes(t *testing.T) {
	test := apCompile(t, asRecord("exists(a/b)"))
	if !test.Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, acTally(test, acPath("a"), acPath("a", "b")), ValueBinding{}, time.Time{}) {
		t.Error("Evaluate(exists(a/b)) over a reported a/b = false, want true")
	}
}

// A child path reads no child's value: ReadsChild is false for each of its
// names, so the caller keeps no child — and records no lack for an a of
// element-only type, whose value it could not read — while Tally is non-nil, so
// the caller reports E's subtree. With ctaChildPath.readsChild answering true
// for its first step the a row fails, and with ctaChildPath.counted appending
// nothing the Tally row does.
func TestAssertionChildPathReadsNoChild(t *testing.T) {
	test := apCompile(t, asRecord("exists(a/b)"))
	for _, name := range []string{"a", "b"} {
		if test.ReadsChild(uq(name)) {
			t.Errorf("(exists(a/b)).ReadsChild(%s) = true, want false", name)
		}
	}
	if test.Tally() == nil {
		t.Error("(exists(a/b)).Tally() = nil, want a Tally for the path")
	}
}

// A path written twice keeps one counter (ctaTallied.same), which the test's
// own Tally fits; a Tally made for another path, or for the one-step `a`, does
// not fit, and Evaluate answers false for it whatever the tree holds. With
// ctaChildPath.same answering false the path written twice takes two counters
// and its own Tally fits nothing; with it answering true for any ctaChildPath
// the a/c, b/a and a/b/c rows fit and hold.
func TestAssertionChildPathKeepsOneCounter(t *testing.T) {
	twice := apCompile(t, asRecord("exists(a/b) and not(empty(a/b))"))
	if c := twice.Tally(); c == nil || len(c.counters) != 1 {
		t.Errorf("(exists(a/b) and not(empty(a/b))).Tally() holds %d counters, want 1", len(c.counters))
	}
	if !twice.Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, acTally(twice, acPath("a", "b")), ValueBinding{}, time.Time{}) {
		t.Error("Evaluate over its own Tally with a/b reported = false, want true")
	}
	for _, tc := range []struct {
		why   string
		other string
	}{
		{"a Tally for a/c", "exists(a/c)"},
		{"a Tally for b/a", "exists(b/a)"},
		{"a Tally for a/b/c", "exists(a/b/c)"},
		{"a Tally for count(b)", "count(b) eq 1"},
		{"a Tally for count(.//b)", "count(.//b) eq 1"},
	} {
		other := apCompile(t, asRecord(tc.other))
		counts := acTally(other, acPath("a", "b"), acPath("a", "c"), acPath("b"), acPath("b", "a"), acPath("a", "b", "c"))
		if apCompile(t, asRecord("exists(a/b)")).Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, counts, ValueBinding{}, time.Time{}) {
			t.Errorf("%s: Evaluate(exists(a/b)) = true, want false", tc.why)
		}
	}
}

// ctaTallied's arms select on their own terms: a child path the chain equal to
// its steps, `a/*` any chain of a and one name, and never an attribute; `N`
// the chain [N] alone; `.//N` any non-empty chain ending in N. Nothing selects
// through an empty chain, which is E itself.
func TestTalliedSelects(t *testing.T) {
	a, b, c := uq("a"), uq("b"), uq("c")
	ab, ok := ctaChildPathOf([]xsd.QName{a}, ctaExactName{name: b})
	if !ok {
		t.Fatal("ctaChildPathOf([a], b) = false, want a path")
	}
	if _, ok := ctaChildPathOf(nil, ctaExactName{name: a}); ok {
		t.Error("ctaChildPathOf([], a) = true, want false: one step is ctaTypedChild's")
	}
	if _, ok := ctaChildPathOf([]xsd.QName{a}, nil); ok {
		t.Error("ctaChildPathOf([a], nil) = true, want false: selectsElement would call a nil last")
	}
	aAny, _ := ctaChildPathOf([]xsd.QName{a}, ctaAnyName{})
	child := ctaCountPath{axis: ctaCountChildren, name: b}
	desc := ctaCountPath{axis: ctaCountDescendants, name: b}
	for _, tc := range []struct {
		why  string
		key  ctaTallied
		path []xsd.QName
		want bool
	}{
		{"a/b selects [a b]", ab, []xsd.QName{a, b}, true},
		{"a/b misses [c b]", ab, []xsd.QName{c, b}, false},
		{"a/b misses [a]", ab, []xsd.QName{a}, false},
		{"a/b misses [b]", ab, []xsd.QName{b}, false},
		{"a/b misses [a b c]", ab, []xsd.QName{a, b, c}, false},
		{"a/b misses [c a b]", ab, []xsd.QName{c, a, b}, false},
		{"a/b misses []", ab, nil, false},
		{"a/* selects [a b]", aAny, []xsd.QName{a, b}, true},
		{"a/* selects [a a]", aAny, []xsd.QName{a, a}, true},
		{"a/* misses [a]", aAny, []xsd.QName{a}, false},
		{"a/* misses [c b]", aAny, []xsd.QName{c, b}, false},
		{"a/* misses [a b c]", aAny, []xsd.QName{a, b, c}, false},
		{"a/* misses []", aAny, nil, false},
		{"b selects [b]", child, []xsd.QName{b}, true},
		{"b misses [a b]", child, []xsd.QName{a, b}, false},
		{"b misses []", child, nil, false},
		{".//b selects [b]", desc, []xsd.QName{b}, true},
		{".//b selects [a c b]", desc, []xsd.QName{a, c, b}, true},
		{".//b misses [b a]", desc, []xsd.QName{b, a}, false},
		{".//b misses []", desc, []xsd.QName{}, false},
	} {
		if got := tc.key.selectsElement(tc.path, nil); got != tc.want {
			t.Errorf("%s: selectsElement = %v, want %v", tc.why, got, tc.want)
		}
	}
	if ab.selectsAttribute(1, b) || ab.selectsAttribute(2, b) {
		t.Error("a/b selects an attribute, want none")
	}
	again, _ := ctaChildPathOf([]xsd.QName{a}, ctaExactName{name: b})
	ac, _ := ctaChildPathOf([]xsd.QName{a}, ctaExactName{name: c})
	aAnyAgain, _ := ctaChildPathOf([]xsd.QName{a}, ctaAnyName{})
	cAny, _ := ctaChildPathOf([]xsd.QName{c}, ctaAnyName{})
	for _, tc := range []struct {
		why  string
		x, y ctaTallied
		want bool
	}{
		{"a/b same as a/b", ab, again, true},
		{"a/b not a/c", ab, ac, false},
		{"a/* same as a/*", aAny, aAnyAgain, true},
		{"a/* not a/b", aAny, ab, false},
		{"a/b not a/*", ab, aAny, false},
		{"a/* not c/*", aAny, cAny, false},
		{"a/b not b", ab, child, false},
		{"b not a/b", child, ab, false},
		{"b same as b", child, ctaCountPath{axis: ctaCountChildren, name: b}, true},
		{"b not .//b", child, desc, false},
	} {
		if got := tc.x.same(tc.y); got != tc.want {
			t.Errorf("%s: same = %v, want %v", tc.why, got, tc.want)
		}
	}
}

// A child path stands only as the whole operand of fn:exists, fn:empty or an
// ·effective boolean value·, and only as QName child steps, the last of which
// may be `*` (TestAssertionWildcardChildPath): every other position, axis,
// wildcard and separator declines.
func TestCompileAssertionTestDeclinesChildPaths(t *testing.T) {
	elems := asElems(map[xsd.QName]xsd.TypeDefinition{uq("a"): asBuiltin(t, "string"), uq("b"): asBuiltin(t, "int")})
	uses := asUses(t, map[string]string{"y": "anySimpleType"})
	for _, tc := range []struct{ expr, why string }{
		{"a/b = 1", "a general comparison operand"},
		{"a/b eq 1", "a value comparison operand"},
		{"1 = a/b", "a right comparison operand"},
		{"a/b + 1", "an arithmetic operand"},
		{"xs:string(a/b) = '1'", "a constructor argument"},
		{"a/b cast as xs:string = '1'", "a cast operand"},
		{"string(a/b) = '1'", "an fn:string argument"},
		{"contains(a/b, 'x')", "an fn:contains argument"},
		{"count(a/b) = 1", "an fn:count argument"},
		{"exists(a/b, a/b)", "two arguments"},
		{"exists(a/b = 1)", "a comparison inside fn:exists"},
		{"exists((a/b))", "a parenthesized argument"},
		{"a/@b", "an attribute step"},
		{"exists(a/@b)", "an attribute step inside fn:exists"},
		{"a//b", "descendant-or-self between steps"},
		{"exists(a//b)", "descendant-or-self inside fn:exists"},
		{"a/*/b", "a wildcard step before the last"},
		{"exists(a/*:b)", "a *:N wildcard step inside fn:exists"},
		{"exists(a/child::b)", "the unabbreviated child axis"},
		{"exists(a/b[1])", "a predicate"},
		{"exists(a/text())", "a kind test"},
		{"exists(a/)", "a slash with no step after it"},
		{"exists(./a/b)", "a context-item path"},
		{"exists(a/b", "an unclosed call"},
		{"exists(a/b c)", "a stray token before the call's ')'"},
		{"exists(a/p:b)", "an unbound prefix"},
	} {
		if _, ok := CompileAssertionTest(asRecord(tc.expr), seededTypes, asElementContent(t, false), uses, elems); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined (%s)", tc.expr, tc.why)
		}
	}
}

// A rooted path of child steps is still declined, as a "/" opening more than
// one step has always been; one opening one step raises err:XPDY0050 and is
// false whatever the subtree holds, inside fn:exists too.
func TestAssertionRootedChildPath(t *testing.T) {
	for _, expr := range []string{"/a/b", "//a/b", "exists(/a/b)", "exists(//a/b)"} {
		if _, ok := CompileAssertionTest(asRecord(expr), seededTypes, asElementContent(t, false), asUses(t, nil), asNoElems); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined", expr)
		}
	}
	for _, expr := range []string{"exists(/a)", "exists(//a)", "not(exists(//a))"} {
		test, ok := CompileAssertionTest(asRecord(expr), seededTypes, asElementContent(t, false), asUses(t, nil), asNoElems)
		if !ok {
			t.Fatalf("CompileAssertionTest(%q): declined, want compiled", expr)
		}
		if test.Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, nil, ValueBinding{}, time.Time{}) {
			t.Errorf("Evaluate(%q) = true, want false: the leading slash raises err:XPDY0050", expr)
		}
	}
}

// A child path is the assertion façade's alone: a Type Alternative's {test}
// declines it on ctaTypeAlternativeFacade.childPath, and an assertions facet's
// reads the absent context item and Fails, never Holds
// (ctaFacetFacade.childPath, err:XPDY0002), fn:not over it included, the error
// propagating. The facet rows decline instead with the facet's childPath
// declining; the Type Alternative rows compile with its childPath admitting
// the path.
func TestChildPathOutsideTheAssertionFacade(t *testing.T) {
	for _, expr := range []string{"a/b", "not(a/b)", "a/b and @x"} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, ""), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
	}
	str := asBuiltin(t, "string")
	for _, test := range []string{"a/b", "not(a/b)", "exists(a/b)", "empty(a/b)", "not(empty(a/b))"} {
		if got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, str, ctaExprRecord(test, ""), fcValue(t, str, "x")); got != value.AssertionFails {
			t.Errorf("FacetAssertions().Evaluate(%q) = %d, want Fails (%d): never Holds", test, got, value.AssertionFails)
		}
	}
}
