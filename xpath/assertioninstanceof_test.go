package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// The fixtures below drive xpath20.md §3.10.1's `instance of` with an atomic
// SequenceType (ctaInstanceOf) and fn:data as its operand
// (ctaParser.instanceofExpr).

// ioEval compiles expr for an E whose {content type} is content, whose
// attribute types are uses and whose child types are elems, read through
// types, or fails the test, and evaluates it under attrs, children and bound.
func ioEval(t *testing.T, expr string, types xsd.TypeResolver, content xsd.ContentType, uses AttributeTypes, elems ElementTypes, attrs TypedAttributes, children ChildElements, bound ValueBinding) bool {
	t.Helper()
	test, ok := CompileAssertionTest(asRecord(expr), types, content, uses, elems)
	if !ok {
		t.Fatalf("CompileAssertionTest(%q): declined, want compiled", expr)
	}
	return test.Evaluate(backend(), types, asElem, attrs, children, nil, bound, time.Time{})
}

// `$value instance of T` matches `$value`'s XDM representation against T
// (xpath20.md §2.5.4): its dynamic type is the {simple type definition}'s
// nearest builtin (Datatypes dt-xdmrep clause 2), and an AtomicType matches an
// item whose type derives from it (§2.5.4.2) — so an xs:date `$value` is an
// xs:date and an xs:anyAtomicType, not an xs:string, not an xs:untypedAtomic,
// and never cast; an xs:integer is an xs:decimal and an xs:decimal is no
// xs:integer, whatever its value; a user restriction of xs:date is an xs:date.
// A ·special· `$value` is xs:untypedAtomic (dt-xdmrep clause 1). The
// unprefixed `date` resolves in the {default namespace}. Every row declines
// with ctaFacade.instanceOf answering false for the assertion façade; the
// xs:integer-against-xs:decimal row fails with ctaTypes.itemMatches comparing
// the operand's type to T for equality instead of walking its ancestors.
func TestAssertionInstanceOfValue(t *testing.T) {
	myDate := csRestriction(t, "MyDate", ctaBuiltin("date"))
	types := asTypesWith(myDate)
	for _, tc := range []struct {
		expr, defaultNS string
		st              *xsd.SimpleType
		lexical         string
		want            bool
	}{
		{"$value instance of xs:date", "", asBuiltin(t, "date"), "2008-01-01", true},
		{"$value instance of xs:date", "", asBuiltin(t, "string"), "2008-01-01", false},
		{"not($value instance of xs:date)", "", asBuiltin(t, "string"), "2008-01-01", true},
		{"$value instance of xs:string", "", asBuiltin(t, "date"), "2008-01-01", false},
		{"$value instance of xs:anyAtomicType", "", asBuiltin(t, "date"), "2008-01-01", true},
		{"$value instance of xs:untypedAtomic", "", asBuiltin(t, "date"), "2008-01-01", false},
		{"$value instance of date", xsd.XMLSchemaNS, asBuiltin(t, "date"), "2008-01-01", true},
		{"$value instance of xs:decimal", "", asBuiltin(t, "integer"), "5", true},
		{"$value instance of xs:integer", "", asBuiltin(t, "decimal"), "5", false},
		{"$value instance of xs:int", "", asBuiltin(t, "integer"), "5", false},
		{"$value instance of xs:date", "", myDate, "2008-01-01", true},
		{"$value instance of xs:untypedAtomic", "", asBuiltin(t, "anySimpleType"), "x", true},
		{"$value instance of xs:anyAtomicType", "", asBuiltin(t, "anySimpleType"), "x", true},
		{"$value instance of xs:string", "", asBuiltin(t, "anySimpleType"), "x", false},
		{"$value instance of xs:date = true()", "", asBuiltin(t, "date"), "2008-01-01", true},
		{"$value instance of xs:date and $value instance of xs:anyAtomicType", "", asBuiltin(t, "date"), "2008-01-01", true},
	} {
		bound := BindValue(tc.lexical, Untyped(tc.lexical))
		if !tc.st.IsSpecial() {
			v, err := value.ValidateLexical(backend(), types, tc.st, tc.lexical, nil, FacetAssertions(time.Time{}))
			if err != nil {
				t.Fatalf("mapping %q against %s: %v", tc.lexical, tc.st.Name(), err)
			}
			bound = BindValue(tc.lexical, Typed(v))
		}
		test, ok := CompileAssertionTest(ctaExprRecord(tc.expr, tc.defaultNS, "xs", xsd.XMLSchemaNS), types, xsd.SimpleContent{SimpleType: tc.st}, asUses(t, nil), asNoElems)
		if !ok {
			t.Errorf("CompileAssertionTest(%q) over %s: declined, want compiled", tc.expr, tc.st.Name().Local)
			continue
		}
		if got := test.Evaluate(backend(), types, asElem, asValues(t), asNoChildren, nil, bound, time.Time{}); got != tc.want {
			t.Errorf("Evaluate(%q) over %s %q = %v, want %v", tc.expr, tc.st.Name().Local, tc.lexical, got, tc.want)
		}
	}
}

// The occurrence indicator decides the empty and multi-item cases (xpath20.md
// §2.5.4.1): a bare AtomicType matches exactly one item, `?` zero or one, `*`
// any number and `+` one or more, and the empty sequence matches only `?` and
// `*`. A `$value` over a list is the sequence of its items (Datatypes
// dt-xdmrep clause 3), each an xs:int and so an xs:integer; the zero
// ValueBinding is the empty sequence clause 2.3.2 gives an invalid E, and so is
// `$value` under a {content type} that is not simple. The two-item rows under
// the bare type and `?`, and the empty rows under it and `+`, are true with
// ctaInstanceOfItem's occurrence check removed. With occurrenceIndicator
// reading none, an indicator is a token no production takes, and the first
// row carrying one declines, failing the test.
func TestAssertionInstanceOfOccurrence(t *testing.T) {
	list := asList(t, "IntList", ctaBuiltin("int"))
	types := asTypesWith(list)
	for _, tc := range []struct {
		expr, lexical string
		want          bool
	}{
		{"$value instance of xs:int", "1", true},
		{"$value instance of xs:int?", "1", true},
		{"$value instance of xs:int*", "1", true},
		{"$value instance of xs:int+", "1", true},
		{"$value instance of xs:int", "1 2", false},
		{"$value instance of xs:int?", "1 2", false},
		{"$value instance of xs:int*", "1 2", true},
		{"$value instance of xs:int+", "1 2", true},
		{"$value instance of xs:integer+", "1 2", true},
		{"$value instance of xs:string*", "1 2", false},
		{"$value instance of xs:int", "", false},
		{"$value instance of xs:int?", "", true},
		{"$value instance of xs:int*", "", true},
		{"$value instance of xs:int+", "", false},
		{"$value instance of xs:string*", "", true},
	} {
		v, err := value.ValidateLexical(backend(), types, list, tc.lexical, nil, FacetAssertions(time.Time{}))
		if err != nil {
			t.Fatalf("mapping %q against the list: %v", tc.lexical, err)
		}
		if got := ioEval(t, tc.expr, types, xsd.SimpleContent{SimpleType: list}, asUses(t, nil), asNoElems, asValues(t), asNoChildren, BindValue(tc.lexical, Typed(v))); got != tc.want {
			t.Errorf("Evaluate(%q) over %q = %v, want %v", tc.expr, tc.lexical, got, tc.want)
		}
	}
	date := asBuiltin(t, "date")
	for _, tc := range []struct {
		expr    string
		content xsd.ContentType
		want    bool
	}{
		{"$value instance of xs:date", xsd.SimpleContent{SimpleType: date}, false},
		{"$value instance of xs:date?", xsd.SimpleContent{SimpleType: date}, true},
		{"$value instance of xs:date*", xsd.SimpleContent{SimpleType: date}, true},
		{"$value instance of xs:date+", xsd.SimpleContent{SimpleType: date}, false},
		{"$value instance of xs:date", xsd.EmptyContent{}, false},
		{"$value instance of xs:date?", xsd.EmptyContent{}, true},
	} {
		if got := ioEval(t, tc.expr, seededTypes, tc.content, asUses(t, nil), asNoElems, asValues(t), asNoChildren, ValueBinding{}); got != tc.want {
			t.Errorf("Evaluate(%q) over the empty sequence under %T = %v, want %v", tc.expr, tc.content, got, tc.want)
		}
	}
}

// fn:data atomizes its argument (xpath-functions.md §2.4, xpath20.md §2.4.2):
// an attribute typed xs:date yields its xs:date typed value, an attribute whose
// type is ·special· one xs:untypedAtomic, an absent one the empty sequence; a
// typed child each child's typed value, a ·nilled· one contributing none; and
// `.` over simple content one xs:untypedAtomic, E's string value, whatever
// `$value` is (cvc-assertion clause 2.3.1's Note) — so assert014's
// `not(data(.) instance of xs:date)` holds over xs:date content. Every row
// calls fn:data, and with instanceofExpr not reading it the first row declines,
// failing the test; the three `data(.)` rows flip with dataInstanceOf
// compiling `data(.)` to `$value`'s ctaValueVar; the bare-type rows over a
// ·nilled· child flip with the count taken over nodes (ctaSequenceLength)
// instead of atoms.
func TestAssertionInstanceOfData(t *testing.T) {
	uses := asUses(t, map[string]string{"d": "date", "u": "anySimpleType"})
	elems := asElems(map[xsd.QName]xsd.TypeDefinition{uq("e"): asBuiltin(t, "date")})
	date := asBuiltin(t, "date")
	attr := func(name, typ, lexical string) []asTyped { return []asTyped{{uq(name), typ, lexical}} }
	for _, tc := range []struct {
		expr     string
		attrs    []asTyped
		children []asChild
		want     bool
	}{
		{"data(@d) instance of xs:date", attr("d", "date", "2008-01-01"), nil, true},
		{"data(@u) instance of xs:date", attr("u", "anySimpleType", "2008-01-01"), nil, false},
		{"data(@u) instance of xs:untypedAtomic", attr("u", "anySimpleType", "2008-01-01"), nil, true},
		{"data(@d) instance of xs:date", nil, nil, false},
		{"data(@d) instance of xs:date?", nil, nil, true},
		{"data(e) instance of xs:date*", nil, []asChild{{uq("e"), "date", "2008-01-01", false}, {uq("e"), "date", "2008-01-02", false}}, true},
		{"data(e) instance of xs:date", nil, []asChild{{uq("e"), "date", "2008-01-01", false}, {uq("e"), "date", "2008-01-02", false}}, false},
		{"data(e) instance of xs:date", nil, []asChild{{name: uq("e"), nilled: true}}, false},
		{"data(e) instance of xs:date?", nil, []asChild{{name: uq("e"), nilled: true}}, true},
		{"data(e) instance of xs:date+", nil, []asChild{{name: uq("e"), nilled: true}, {uq("e"), "date", "2008-01-01", false}}, true},
		{"data(e) instance of xs:date", nil, []asChild{{name: uq("e"), nilled: true}, {uq("e"), "date", "2008-01-01", false}}, true},
		{"data(.) instance of xs:untypedAtomic", nil, nil, true},
		{"data(.) instance of xs:date", nil, nil, false},
		{"not(data(.) instance of xs:date)", nil, nil, true},
		{"data($value) instance of xs:date", nil, nil, true},
	} {
		got := ioEval(t, tc.expr, seededTypes, xsd.SimpleContent{SimpleType: date}, uses, elems, asValues(t, tc.attrs...), asChildren(t, tc.children...), asBind(t, date, "2008-01-01"))
		if got != tc.want {
			t.Errorf("Evaluate(%q) over %v %v = %v, want %v", tc.expr, tc.attrs, tc.children, got, tc.want)
		}
	}
}

// AssertionTest.ReadsChild reports a child fn:data reads under `instance of`,
// so the caller keeps its value. It reports false with ctaInstanceOf's
// readsChild answering false.
func TestAssertionInstanceOfReadsChild(t *testing.T) {
	elems := asElems(map[xsd.QName]xsd.TypeDefinition{uq("e"): asBuiltin(t, "date")})
	test, ok := CompileAssertionTest(asRecord("data(e) instance of xs:date*"), seededTypes, xsd.EmptyContent{}, asUses(t, nil), elems)
	if !ok {
		t.Fatal("CompileAssertionTest: declined, want compiled")
	}
	if !test.ReadsChild(uq("e")) {
		t.Error("ReadsChild(e) = false, want true")
	}
}

// An assertions facet decides `$value instance of T` over the value's XDM
// representation, and fn:data over the absent context item raises
// err:XPDY0002 (cvc-assertions-valid clause 1.2's Note), failing the facet
// under fn:not too, which an `instance of` answering false would make hold.
// The fn:not row holds with ctaInstanceOfItem answering false where its
// operand raises. An AttributeTest over an attribute step declines, the step
// reading no attribute node (ctaFacetFacade.instanceOf's GAP(xpath)): with
// attributeInstanceOf admitting any operand, the `@d instance of attribute()`
// row fails the facet instead.
func TestFacetAssertionsInstanceOf(t *testing.T) {
	str := asBuiltin(t, "string")
	for _, tc := range []struct {
		test string
		want value.AssertionOutcome
	}{
		{"$value instance of xs:string", value.AssertionHolds},
		{"not($value instance of xs:string)", value.AssertionFails},
		{"$value instance of xs:date", value.AssertionFails},
		{"data(.) instance of xs:untypedAtomic", value.AssertionFails},
		{"not(data(.) instance of xs:date)", value.AssertionFails},
		{"@d instance of attribute()", value.AssertionDeclined},
		{"not(./@d instance of attribute(*, xs:string))", value.AssertionDeclined},
	} {
		got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, str, ctaExprRecord(tc.test, "", "xs", xsd.XMLSchemaNS), fcValue(t, str, "abc"))
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %d, want %d", tc.test, got, tc.want)
		}
	}
}

// An [60] AttributeTest matches the attribute NODE one attribute step of E
// selects (xpath20.md §2.5.4.5): `attribute()` and `attribute(*)` any,
// `attribute(N)` one named N whatever its type, and a TypeName one whose type
// annotation — its {attribute use}'s type — derives from it, walked up a
// user-defined type's chain too; `./@d` and `attribute::d` decide as `@d` does.
// An absent attribute is the empty sequence, false without an occurrence
// indicator and true under `?` (§2.5.4.1). A ·special·-typed attribute is
// annotated by its type, xs:anySimpleType or xs:anyAtomicType, and only its
// typed value is xs:untypedAtomic (xpath-datamodel §3.3.1.1, §3.3.1.2), so
// `attribute(*, xs:untypedAtomic)` matches it no more than `attribute(*,
// xs:dateTime)` does. `@d` in the body of a quantifier over `$value` is E's
// attribute too. With instanceofExpr not dispatching to attributeInstanceOf
// the first row declines, failing the test; with attributeKindTest's name
// check removed the `attribute(s)` and `attribute(s, xs:dateTime)` rows over
// @d hold; with a ctaAttr read as annotated xs:untypedAtomic the four @u and @a
// rows naming xs:untypedAtomic or xs:anySimpleType flip; and with
// annotationDerives comparing the annotation for equality instead of walking
// its chain the @d xs:anyAtomicType and xs:anySimpleType rows and the @m
// xs:date row are false.
func TestAssertionInstanceOfAttributeTest(t *testing.T) {
	myDate := csRestriction(t, "MyDate", ctaBuiltin("date"))
	types := asTypesWith(myDate)
	builtin := asUses(t, map[string]string{"d": "dateTime", "s": "string", "u": "anySimpleType", "a": "anyAtomicType"})
	uses := func(path []xsd.QName, name xsd.QName) (*xsd.SimpleType, bool) {
		if len(path) == 0 && name == uq("m") {
			return myDate, true
		}
		return builtin(path, name)
	}
	m, err := value.ValidateLexical(backend(), types, myDate, "2008-01-01", nil, FacetAssertions(time.Time{}))
	if err != nil {
		t.Fatalf("mapping 2008-01-01 against MyDate: %v", err)
	}
	carried := asValues(t, asTyped{uq("d"), "dateTime", "2008-01-01T00:00:00"}, asTyped{uq("s"), "string", "x"},
		asTyped{uq("u"), "anySimpleType", "x"}, asTyped{uq("a"), "anyAtomicType", "x"})
	present := TypedAttributes(func(yield func(xsd.QName, TypedValue) bool) {
		for name, v := range carried {
			if !yield(name, v) {
				return
			}
		}
		yield(uq("m"), Typed(m))
	})
	absent := asValues(t)
	date := asBuiltin(t, "date")
	for _, tc := range []struct {
		expr  string
		attrs TypedAttributes
		want  bool
	}{
		{"@d instance of attribute(*, xs:dateTime)", present, true},
		{"@s instance of attribute(*, xs:dateTime)", present, false},
		{"@d instance of attribute(*, xs:dateTime)", absent, false},
		{"@d instance of attribute(*, xs:dateTime)?", absent, true},
		{"@d instance of attribute(*, xs:dateTime)?", present, true},
		{"@d instance of attribute(*, xs:dateTime)+", absent, false},
		{"not(@d instance of attribute(*, xs:dateTime))", present, false},
		{"./@d instance of attribute(*, xs:dateTime)", present, true},
		{"./@s instance of attribute(*, xs:dateTime)", present, false},
		{"./@d instance of attribute(*, xs:dateTime)", absent, false},
		{"attribute::d instance of attribute(*, xs:dateTime)", present, true},
		{"@d instance of attribute()", present, true},
		{"@s instance of attribute()", present, true},
		{"@d instance of attribute()", absent, false},
		{"@d instance of attribute(*)", present, true},
		{"@d instance of attribute(d)", present, true},
		{"@s instance of attribute(s)", present, true},
		{"@d instance of attribute(s)", present, false},
		{"@d instance of attribute(d, xs:dateTime)", present, true},
		{"@d instance of attribute(d, xs:string)", present, false},
		{"@d instance of attribute(s, xs:dateTime)", present, false},
		{"@d instance of attribute(*, xs:anyAtomicType)", present, true},
		{"@d instance of attribute(*, xs:anySimpleType)", present, true},
		{"@d instance of attribute(*, xs:untypedAtomic)", present, false},
		{"@d instance of attribute(*, xs:date)", present, false},
		{"@m instance of attribute(*, xs:date)", present, true},
		{"@m instance of attribute(*, xs:dateTime)", present, false},
		{"@u instance of attribute()", present, true},
		{"@u instance of attribute(*, xs:untypedAtomic)", present, false},
		{"@u instance of attribute(*, xs:dateTime)", present, false},
		{"@u instance of attribute(*, xs:anySimpleType)", present, true},
		{"@a instance of attribute(*, xs:anySimpleType)", present, true},
		{"@a instance of attribute(*, xs:untypedAtomic)", present, false},
		{"some $x in data($value) satisfies @d instance of attribute(*, xs:dateTime)", present, true},
	} {
		if got := ioEval(t, tc.expr, types, xsd.SimpleContent{SimpleType: date}, uses, asNoElems, tc.attrs, asNoChildren, asBind(t, date, "2008-01-01")); got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// Guard: what `instance of` does not admit declines rather than answering. A
// Type Alternative's {test} declines it — §3.12.6's grammar has no
// InstanceofExpr — and so does a value predicate. In an assertion, a KindTest,
// `item()` and `empty-sequence()` SequenceType decline under sequenceType's
// GAP(xpath) (#1042) — an AttributeTest is read over one attribute step alone,
// never over an atomized operand, fn:data's or fn:distinct-values' argument, a
// child read for its value, `$value` or a range variable — and so does a name
// that is no builtin atomic type: a list or xs:anySimpleType (err:XPST0051), a
// complex type, a user-defined type, a name resolving to nothing. Over an
// attribute step, every SequenceType but an AttributeTest declines, an
// AtomicType and `element()`, `node()` and `item()` among them, and so does an
// AttributeTest whose TypeName is user-defined (err:XPST0008), xs:anyType or
// resolves to nothing, or is xs:anyAtomicType over a ·special· attribute. A
// node operand without fn:data — `.`, a child step — declines, and so does an
// operand whose static type is not its dynamic one, a numeric literal and
// arithmetic, and a typed child, directly or through fn:distinct-values,
// against an AtomicType its compiled type does not derive from: an xsi:type
// can make an xs:int child an xs:short; fn:data anywhere but as the operand of
// `instance of`, or with two arguments, declines. The fn:data row compiles for
// a Type Alternative with ctaTypeAlternativeFacade.instanceOf answering true,
// and the AttributeTest row there with instanceofExpr's AttributeTest arm not
// asking the façade; the value predicate comparing `.` with an `instance of`
// compiles with ctaPredicateFacade.instanceOf answering true; the `.` and
// child-step rows compile without instanceofExpr's node check, and `@d
// instance of xs:date` with attributeInstanceOf handing its tail to
// instanceTail; the `data(@event)` and `data(@d)` rows compile with an
// AttributeTest arm in instanceTail over a ctaTypedAttr operand, and the six
// rows from `data(@event)` to `$value` with sequenceType admitting
// `attribute(...)`; the `$x` row compiles with an AttributeTest arm before
// instanceItem, and the two `$v` rows only with instanceofExpr's node check
// removed too; the `element()`, `node()` and `item()` rows over @d compile
// with attributeKindTest admitting any name before '('; the a:MyDate,
// xs:anyType and xs:nonesuch rows with annotationDerives admitting any
// TypeName, and the xs:anyAtomicType row over @u with annotationDerives
// answering it over a ctaAttr; the three typed-child rows compile without
// instanceTail's derived check, the IntegerLiteral row without instanceItem's
// literal check, and the arithmetic row with instanceItem admitting ctaArith.
func TestInstanceOfDeclines(t *testing.T) {
	for _, expr := range []string{"@a instance of xs:string", "not(@a instance of xs:string)", "data(@a) instance of xs:untypedAtomic", "@a instance of attribute()"} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, "", "xs", xsd.XMLSchemaNS), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
	}
	myDate := csRestriction(t, "MyDate", ctaBuiltin("date"))
	types := asTypesWith(myDate)
	date := asBuiltin(t, "date")
	uses := asUses(t, map[string]string{"event": "dateTime", "d": "date", "u": "anySimpleType"})
	elems := asElems(map[xsd.QName]xsd.TypeDefinition{uq("e"): date, uq("n"): asBuiltin(t, "int")})
	for _, expr := range []string{
		"data(@event) instance of attribute(*, xs:dateTime)",
		"data(@d) instance of attribute()",
		"distinct-values(@d) instance of attribute()",
		"data(n) instance of attribute()",
		"data(n[1]) instance of attribute()",
		"$value instance of attribute()",
		"some $x in data($value) satisfies $x instance of attribute()",
		"some $v in n satisfies $v instance of attribute()",
		"some $v in n satisfies $v/following-sibling::n[1] instance of attribute()",
		"count(n[@d instance of attribute()]) = 1",
		"@* instance of attribute()",
		"@d instance of element()",
		"@d instance of node()",
		"@d instance of item()",
		"@d instance of attribute(*, xs:date?)",
		"@d instance of attribute(*, a:MyDate)",
		"@d instance of attribute(*, xs:anyType)",
		"@d instance of attribute(*, xs:nonesuch)",
		"@u instance of attribute(*, xs:anyAtomicType)",
		"$value instance of element()",
		"$value instance of item()",
		"$value instance of empty-sequence()",
		"$value instance of xs:IDREFS",
		"$value instance of xs:anySimpleType",
		"$value instance of xs:anyType",
		"$value instance of xs:nonesuch",
		"$value instance of a:MyDate",
		"@d instance of xs:date",
		". instance of xs:untypedAtomic",
		"e instance of xs:date",
		"data(n) instance of xs:short",
		"not(data(n) instance of xs:short)",
		"distinct-values(n) instance of xs:short",
		"1 instance of xs:integer",
		"data(count(e) + count(e)) instance of xs:integer",
		"data(@d) = xs:date('2008-01-01')",
		"data(@d, @d) instance of xs:date",
		"$value instance of xs:date instance of xs:boolean",
		"count(n[. instance of xs:int]) = 1",
		"count(n[. = 1 and . = 'a' instance of xs:string]) = 1",
	} {
		if _, ok := CompileAssertionTest(asRecord(expr), types, xsd.SimpleContent{SimpleType: date}, uses, elems); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined", expr)
		}
	}
}
