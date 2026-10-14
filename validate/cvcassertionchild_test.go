package validate

import (
	"strings"
	"testing"

	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// The fixtures below drive an assertion {test} that reads <root>'s element
// [[children]] (cvc-assertion clause 1.2): RootType carries the assertions
// given, an attribute x of xs:int, and the content
//
//	sequence( e1?, n*, any(skip)* )
//
// with e1 a {nillable} xs:string and n an xs:int, so a second <e1> is
// ·attributed to· the skip wildcard and ·skipped·. StrExt is a complex type
// whose simple content EXTENDS xs:string, which an xsi:type can put on <e1>:
// it keeps xs:string as its {simple type definition} (cos-ct-extends clause
// 2.1), so <e1>'s value is read as one.

// acSchema builds RootType over the assertions exprs, and declares <root>.
func acSchema(t *testing.T, exprs ...string) *xsd.Schema {
	t.Helper()
	unbounded, err := xsd.NewUnboundedOccurs(xsderr.Loc{}, 0)
	if err != nil {
		t.Fatalf("NewUnboundedOccurs: %v", err)
	}
	skip, err := xsd.NewParticle(xsderr.Loc{}, unbounded, xsd.ResolvedTerm{Term: *anyWildcard(t, xsd.ProcessSkip)})
	if err != nil {
		t.Fatalf("NewParticle: %v", err)
	}
	content := icContent(t,
		icOptional(t, icLocal(t, "RootType", local("e1"), icBuiltin("string"), true, nil)),
		icRepeated(t, icLocal(t, "RootType", local("n"), icBuiltin("int"), false, nil)),
		skip)
	ct, err := xsd.NewComplexType(xsderr.Loc{}, local("RootType"), xsd.QName{}, nil,
		xsd.DerivationRestriction, false, attrContent([]xsd.AttributeUse{icUse(t, local("x"), "int")}),
		nil, nil, content, nil, aAssertions(exprs...))
	if err != nil {
		t.Fatalf("building RootType: %v", err)
	}
	str := icSeeded(t)["string"]
	ext, err := xsd.NewComplexType(xsderr.Loc{}, local("StrExt"), icBuiltin("string"), nil,
		xsd.DerivationExtension, false, nil, nil, nil, xsd.SimpleContent{SimpleType: str}, nil, nil)
	if err != nil {
		t.Fatalf("building StrExt: %v", err)
	}
	return cSchemaFrom(t, ct, func(b *xsd.SchemaBuilder) {
		aTypes(t, b)
		b.AddType(ext)
	})
}

// acKid is a child element named name at line, holding text and carrying
// attrs, with the xs prefix bound for an xsi:type.
func acKid(name string, line int, text string, attrs ...Attribute) Child {
	e := &testElement{name: local(name), attrs: attrs, loc: loc(line, 3),
		bindings: map[string]string{"xs": xsd.XMLSchemaNS}}
	if text != "" {
		e.kids = []Child{TextChild(&testText{data: text, loc: loc(line, 8)})}
	}
	return ElementChild(e)
}

// acRoot is <root> at 1:1 over kids.
func acRoot(kids ...Child) *testElement {
	return &testElement{name: local("root"), kids: kids, loc: loc(1, 1)}
}

// acDeclined fails unless res recorded exactly one Unevaluated, under
// cvc-assertion at <root>, whose message names want — whatever else res
// charged.
func acDeclined(t *testing.T, res *Result, want string) {
	t.Helper()
	got := res.Unevaluated()
	if len(got) != 1 {
		t.Fatalf("Unevaluated() = %v, want one cvc-assertion decline", messages(got))
	}
	if got[0].Rule() != ruleCvcAssertion || got[0].Loc() != loc(1, 1) {
		t.Errorf("Unevaluated()[0] = %s at %s, want cvc-assertion at %s", got[0].Rule(), got[0].Loc(), loc(1, 1))
	}
	if !strings.Contains(got[0].Msg(), want) {
		t.Errorf("Unevaluated()[0].Msg() = %q, want it to name %q", got[0].Msg(), want)
	}
	for _, v := range res.Violations() {
		if v.Rule == ruleCvcAssertion {
			t.Errorf("Violations() holds %v: a declined assertion is never charged", v)
		}
	}
}

// An assertion reads <root>'s element [[children]] TYPED, over the instance
// cvc-assertion clause 1 builds once they are validated: `e1 = 'present'` is
// satisfied over <e1>present</e1> and charged over <e1>absent</e1>; `e1 and n`
// is charged where n is absent, the empty node sequence being false (xpath20.md
// §2.4.3); `n > 9` compares xs:int values, so 10 holds where the string "10"
// would not. A ·nilled· <e1> is a node with no value (xpath-datamodel §6.2.4):
// `e1` holds and `e1 = ”` is charged, where an empty <e1> satisfies it. An
// xsi:type restricting xs:string to xs:token is read through the child's own
// whiteSpace, so "  present " is "present", and one naming StrExt, which is
// ·validly substitutable· for xs:string by extension, is read as xs:string
// too: both StrExt rows decline instead with walk.childValue blocking
// extension. Every row is declined instead, and fails, with xpath's
// ctaAssertionFacade.child declining every child step.
func TestAssertionReadsChildElementValues(t *testing.T) {
	nilled := &testAttribute{name: xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: "nil"}, value: "true", loc: loc(2, 6)}
	token := xsiTypeAttr("xs:token")
	for _, tc := range []struct {
		expr    string
		kids    []Child
		charged bool
	}{
		{"e1 = 'present'", []Child{acKid("e1", 2, "present")}, false},
		{"e1 = 'present'", []Child{acKid("e1", 2, "absent")}, true},
		{"e1 = 'present'", nil, true},
		{"e1 and n", []Child{acKid("e1", 2, "x"), acKid("n", 3, "1")}, false},
		{"e1 and n", []Child{acKid("e1", 2, "x")}, true},
		{"n > 9", []Child{acKid("n", 2, "10")}, false},
		{"n = 2", []Child{acKid("n", 2, "1"), acKid("n", 3, "2")}, false},
		{"e1", []Child{acKid("e1", 2, "", nilled)}, false},
		{"e1 = 'present'", []Child{acKid("e1", 2, "", nilled)}, true},
		{"e1 = ''", []Child{acKid("e1", 2, "", nilled)}, true},
		{"e1 = ''", []Child{acKid("e1", 2, "")}, false},
		{"e1 = 'present'", []Child{acKid("e1", 2, "  present ", token)}, false},
		{"e1 = 'present'", []Child{acKid("e1", 2, "present", xsiTypeAttr("StrExt"))}, false},
		{"e1 = 'present'", []Child{acKid("e1", 2, "absent", xsiTypeAttr("StrExt"))}, true},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			res := aAssess(t, acSchema(t, tc.expr), acRoot(tc.kids...))
			if !tc.charged {
				wantSatisfied(t, res, tc.expr)
				return
			}
			wantAssertionCharge(t, res, "the element root is not ·valid· with respect to assertion 1 of 1 in the {assertions} of the ·governing type definition· RootType, whose {test} is ")
		})
	}
}

// A "/"-rooted {test} raises err:XPDY0050: <root> is the root of the instance
// cvc-assertion clause 1.3 builds, with no document node above it (xpath20.md
// §3.2), so `/root = 'present'` is charged whatever <root>'s child holds. It
// is declined instead, and fails, with xpath's assertion façade declining a
// rooted path.
func TestAssertionRootedPathIsCharged(t *testing.T) {
	for _, e1 := range []string{"present", "absent"} {
		res := aAssess(t, acSchema(t, "/root = 'present'"), acRoot(acKid("e1", 2, e1)))
		wantAssertionCharge(t, res, `the element root is not ·valid· with respect to assertion 1 of 1 in the {assertions} of the ·governing type definition· RootType, whose {test} is "/root = 'present'",`)
	}
}

// A child a {test} reads but whose typed value this package does not read
// DECLINES every assertion of <root> (walk.childValue), and is never charged:
// an <n> String Valid rejects is invalid in the partial ·PSVI·, and so is an
// <n> whose value maps but which carries an attribute its simple type admits
// none of (cvc-type clause 3.1.1) — the row that is satisfied instead with
// childValue's recorded check removed — and so is an <n> whose xsi:type,
// xs:string, does not ·override· its declared xs:int (cvc-elt clause 4), which
// is still read under xs:int, the row that is satisfied instead with
// walk.child's main arm taking <n>'s mark after walk.childGoverning; a second
// <e1> is ·skipped· by the wildcard, which is satisfied instead with
// assertionCheck.skipped removed. A child no {test} reads declines nothing,
// which is the retention gate (assertionCheck.reads): `@x = 1` and `e1 =
// 'present'` are each evaluated beside an invalid <n>, which only its own
// charge reports. With reads answering true for every name, both of those rows
// decline instead.
func TestAssertionOverAnUnreadableChildIsDeclined(t *testing.T) {
	x := &testAttribute{name: local("x"), value: "1", loc: loc(1, 10)}
	for _, tc := range []struct {
		why, expr string
		kids      []Child
		want      string
	}{
		{"an invalid child", "n > 0", []Child{acKid("n", 2, "abc")}, "the child element n of the element root"},
		{"a child invalid for an attribute", "n > 0", []Child{acKid("n", 2, "5", &testAttribute{name: local("stray"), value: "1", loc: loc(2, 6)})},
			"a violation or an unevaluated check was recorded for it"},
		{"a child invalid by its xsi:type", "n > 0", []Child{acKid("n", 2, "5", xsiTypeAttr("xs:string"))},
			"a violation or an unevaluated check was recorded for it"},
		{"a skipped child", "e1 = 'present'", []Child{acKid("e1", 2, "present"), acKid("e1", 3, "present")}, "it is ·skipped·"},
	} {
		t.Run(tc.why, func(t *testing.T) {
			acDeclined(t, aAssess(t, acSchema(t, tc.expr), acRoot(tc.kids...)), tc.want)
		})
	}
	for _, expr := range []string{"@x = 1", "e1 = 'present'"} {
		t.Run("unread invalid child beside "+expr, func(t *testing.T) {
			root := acRoot(acKid("e1", 2, "present"), acKid("n", 3, "abc"))
			root.attrs = []Attribute{x}
			res := aAssess(t, acSchema(t, expr), root)
			if got := res.Unevaluated(); len(got) != 0 {
				t.Errorf("Unevaluated() = %v, want none: no {test} reads <n>", messages(got))
			}
			for _, v := range res.Violations() {
				if v.Rule == ruleCvcAssertion {
					t.Errorf("Violations() holds %v, want the assertion satisfied", v)
				}
			}
		})
	}
}

// validate.valueTypeOf and xpath's ctaChildValueType each classify a
// ·locally declared type· for the value of a child a {test} reads, and the
// [xpath.ChildElements] contract makes them agree. This pins the agreement
// across the package boundary through the seam alone: <c>'s ·locally declared
// type· CInt is complex with simple content over xs:int, so `c > 9` is
// satisfied over 10 and charged over 5. Either side dropping the
// simple-content arm declines both rows instead — validate's valueTypeOf
// returning nil for it, or xpath's ctaChildValueType answering false; the
// simple-type arm is TestAssertionReadsChildElementValues' rows.
func TestAssertionChildValueTypeAgreesAcrossPackages(t *testing.T) {
	schema := parsedSchema(t, map[string]string{"main.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="CInt">
    <xs:simpleContent>
      <xs:extension base="xs:int"><xs:attribute name="a" type="xs:int"/></xs:extension>
    </xs:simpleContent>
  </xs:complexType>
  <xs:complexType name="RootType">
    <xs:sequence><xs:element name="c" type="CInt"/></xs:sequence>
    <xs:assert test="c &gt; 9"/>
  </xs:complexType>
  <xs:element name="root" type="RootType"/>
</xs:schema>`})
	a := &testAttribute{name: local("a"), value: "1", loc: loc(2, 6)}
	wantSatisfied(t, aAssess(t, schema, acRoot(acKid("c", 2, "10", a))), "c > 9")
	wantAssertionCharge(t, aAssess(t, schema, acRoot(acKid("c", 2, "5", a))),
		`the element root is not ·valid· with respect to assertion 1 of 1 in the {assertions} of the ·governing type definition· RootType, whose {test} is "c > 9",`)
}

// A child whose ·governing type definition· is not ·validly substitutable· for
// its ·locally declared type· DECLINES <root>'s assertions on walk.childValue's
// own repeat of cvc-complex-type clause 5, which walk.child charges before the
// child's frame opens, outside the record childValue reads. wild062's shape:
// the second <e1> is ·attributed to· the lax wildcard, resolves to no top-level
// declaration, and is governed by its xsi:type xs:time alone, which is not
// ·validly substitutable· for the local e1's xs:string. Its "12:20:02" maps
// under xs:string too, so with childValue's ValidlySubstitutable check removed
// `e1 = '12:20:02'` is satisfied instead.
func TestAssertionOverANonSubstitutableChildIsDeclined(t *testing.T) {
	schema := parsedSchema(t, map[string]string{"main.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="RootType">
    <xs:sequence>
      <xs:element name="e1" type="xs:string"/>
      <xs:any namespace="##local" processContents="lax"/>
    </xs:sequence>
    <xs:assert test="e1 = '12:20:02'"/>
  </xs:complexType>
  <xs:element name="root" type="RootType"/>
</xs:schema>`})
	res := aAssess(t, schema, acRoot(acKid("e1", 2, "x"), acKid("e1", 3, "12:20:02", xsiTypeAttr("xs:time"))))
	acDeclined(t, res, "its ·governing type definition· {"+xsd.XMLSchemaNS+"}time is neither its ·locally declared type· {"+xsd.XMLSchemaNS+"}string nor ·validly substitutable· for it")
}

// fn:count over a child step whose predicate reads the child's value counts
// the children the predicate is true for, each read typed as a child step
// naming it reads it (walk.keepChild): `count(n[. = 3]) eq 1` holds over 3
// and 4 and is charged over 3 and 3, which compares xs:int values; a ·nilled·
// <e1> has no value, so `count(e1[. = 'x']) eq 0` holds over it. A ·skipped·
// second <e1> under a value predicate declines through lackingChild, as a
// value step's does. With ctaMatchingChildren.readsChild false, which keeps no
// child's value, the first and last rows are charged and the ·skipped· row is
// evaluated instead.
func TestAssertionCountsChildrenFilteredByValue(t *testing.T) {
	nilled := &testAttribute{name: xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: "nil"}, value: "true", loc: loc(2, 6)}
	for _, tc := range []struct {
		expr    string
		kids    []Child
		charged bool
	}{
		{"count(n[. = 3]) eq 1", []Child{acKid("n", 2, "3"), acKid("n", 3, "4")}, false},
		{"count(n[. = 3]) eq 1", []Child{acKid("n", 2, "3"), acKid("n", 3, "03")}, true},
		{"count(e1[. = 'x']) eq 0", []Child{acKid("e1", 2, "", nilled)}, false},
		{"count(e1[. = 'x']) eq 1", []Child{acKid("e1", 2, "x"), acKid("n", 3, "1")}, false},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			res := aAssess(t, acSchema(t, tc.expr), acRoot(tc.kids...))
			if !tc.charged {
				wantSatisfied(t, res, tc.expr)
				return
			}
			wantAssertionCharge(t, res, "the element root is not ·valid· with respect to assertion 1 of 1 in the {assertions} of the ·governing type definition· RootType, whose {test} is ")
		})
	}
	acDeclined(t, aAssess(t, acSchema(t, "count(e1[. = 'x']) le 2"), acRoot(acKid("e1", 2, "x"), acKid("e1", 3, "x"))),
		"the child element e1 of the element root at instance.xml:3:3, which a {test} reads, has no typed value for the data model instance cvc-assertion clause 1 builds: it is ·skipped·")
}

// An `instance of` over a typed child matches the child's OWN type annotation
// (xpath20.md §2.5.4.2), its ·governing type definition·'s (cvc-assertion
// clause 1.2: E's [[children]]' properties "are defined in the usual way"),
// where xpath compiles it against the ·locally declared type·. <e1> is
// declared xs:string and carries xsi:type xs:token, so `data(e1) instance of
// xs:string` and `distinct-values(e1) instance of xs:string` hold — an
// xs:token is an xs:string — while every `instance of xs:token` over it,
// directly, under fn:not, through fn:distinct-values or fn:data over one,
// DECLINES: the ·locally declared type· does not decide it. With xpath's
// instanceTail answering where derived holds and matches does not, or
// ctaTypes.instanceItem's ctaTypedChild arm reporting derived false, the
// first, third, fourth and fifth declined rows are charged and the second is
// satisfied; with its ctaDistinctValues arm dropping derived, the last three
// are. With the ctaTypedChild arm declining outright, the two xs:string rows
// decline too.
func TestAssertionInstanceOfReadsTheChildsOwnType(t *testing.T) {
	token := xsiTypeAttr("xs:token")
	for _, tc := range []struct {
		expr     string
		declined bool
	}{
		{"data(e1) instance of xs:string", false},
		{"distinct-values(e1) instance of xs:string", false},
		{"data(e1) instance of xs:token", true},
		{"not(data(e1) instance of xs:token)", true},
		{"distinct-values(e1) instance of xs:token", true},
		{"data(distinct-values(e1)) instance of xs:token", true},
		{"not(distinct-values(e1) instance of xs:token)", true},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			schema := parsedSchema(t, map[string]string{"main.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="RootType">
    <xs:sequence><xs:element name="e1" type="xs:string"/></xs:sequence>
    <xs:assert test="` + tc.expr + `"/>
  </xs:complexType>
  <xs:element name="root" type="RootType"/>
</xs:schema>`})
			res := aAssess(t, schema, acRoot(acKid("e1", 2, "present", token)))
			if !tc.declined {
				wantSatisfied(t, res, tc.expr)
				return
			}
			acDeclined(t, res, tc.expr)
		})
	}
}
