package xsd

import (
	"errors"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/xsderr"
)

// These tests are package-internal: key-dft-binding and loc-testSubP are
// unexported finalize-phase helpers (STYLE T5). The component builders come from
// particleattribution_test.go and complexderivation_test.go (STYLE T4).

// bSchema finalizes a schema carrying xs:anyType, the named simple types the
// attribute helpers refer to, and whatever build adds, returning the *Schema the
// binding helpers are methods on.
func bSchema(t *testing.T, build func(*SchemaBuilder)) *Schema {
	t.Helper()
	b := NewSchemaBuilder()
	b.AddType(dAnyType(t))
	str := dPrimitive(t, uq("str"))
	b.AddType(str)
	b.AddType(dSimple(t, uq("narrow"), str))
	if build != nil {
		build(b)
	}
	s, err := b.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	return s
}

// TestAttributeDefaultBindingCases covers the key-dft-binding cases this
// rendering decides: case 2 (a matching {attribute use}), cases 4/5/6 (the
// {attribute wildcard}'s keyword), and the "no binding at all" answer the caller
// charges.
func TestAttributeDefaultBindingCases(t *testing.T) {
	s := bSchema(t, nil)
	use := dAttr(t, uq("a"), uq("str"))
	strict := uWildcard(t, NamespaceConstraintAny, nil, ProcessStrict)
	skip := uWildcard(t, NamespaceConstraintAny, nil, ProcessSkip)

	for _, tc := range []struct {
		name    string
		ct      ComplexType
		query   QName
		wantOK  bool
		wantUse bool            // expect an attributeUseBinding
		wantKey ProcessContents // expect a wildcardKeywordBinding with this keyword
	}{
		{"case 2: a matching attribute use wins",
			dType(t, uq("c"), anyTypeName, EmptyContent{}, []AttributeUse{use}, &strict),
			uq("a"), true, true, 0},
		{"case 4: a strict wildcard with no matching use yields the keyword",
			dType(t, uq("c"), anyTypeName, EmptyContent{}, []AttributeUse{use}, &strict),
			uq("z"), true, false, ProcessStrict},
		{"case 6: a skip wildcard yields skip",
			dType(t, uq("c"), anyTypeName, EmptyContent{}, nil, &skip),
			uq("z"), true, false, ProcessSkip},
		{"no use and no wildcard: no binding",
			dType(t, uq("c"), anyTypeName, EmptyContent{}, nil, nil),
			uq("z"), false, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := s.attributeDefaultBinding(complexTypeAttributeSide(tc.ct, typeDefinitionLabel(tc.ct)), tc.query)
			if ok != tc.wantOK {
				t.Fatalf("attributeDefaultBinding ok = %t, want %t", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if tc.wantUse {
				if _, isUse := got.(attributeUseBinding); !isUse {
					t.Fatalf("binding = %T, want attributeUseBinding", got)
				}
				return
			}
			k, isKeyword := got.(wildcardKeywordBinding)
			if !isKeyword {
				t.Fatalf("binding = %T, want wildcardKeywordBinding", got)
			}
			if k.keyword != tc.wantKey {
				t.Fatalf("keyword = %s, want %s", k.keyword, tc.wantKey)
			}
		})
	}
}

// TestBindingSubsumesKeywords covers loc-testSubP clauses 1-3 plus the
// documented fail-open on a strict G, which reaches an Attribute Use S and
// never a keyword one (#2546). The two complex types are only message context
// here, so a pair of trivial ones is enough.
func TestBindingSubsumesKeywords(t *testing.T) {
	s := bSchema(t, nil)
	tt := dType(t, uq("t"), anyTypeName, EmptyContent{}, nil, nil)
	bb := dType(t, uq("b"), anyTypeName, EmptyContent{}, nil, nil)
	use := attributeUseBinding{use: dAttr(t, uq("a"), uq("str"))}
	decl := elementDeclarationBinding{decl: uLocal(t, uq("a"), uq("str"))}

	strict := wildcardKeywordBinding{keyword: ProcessStrict}
	for _, tc := range []struct {
		name     string
		general  defaultBinding
		specific defaultBinding
		wantOK   bool
		// wantMsg, for a keyword refusal, is the message text naming G, then S,
		// then the clause — in that order, so a message that swaps them fails.
		wantMsg string
	}{
		{"clause 1: skip subsumes an attribute use",
			wildcardKeywordBinding{keyword: ProcessSkip}, use, true, ""},
		{"clause 1: skip subsumes skip",
			wildcardKeywordBinding{keyword: ProcessSkip}, wildcardKeywordBinding{keyword: ProcessSkip}, true, ""},
		{"clause 2: lax subsumes an attribute use",
			wildcardKeywordBinding{keyword: ProcessLax}, use, true, ""},
		{"clause 2: lax does NOT subsume skip",
			wildcardKeywordBinding{keyword: ProcessLax}, wildcardKeywordBinding{keyword: ProcessSkip}, false,
			"but {urn:upa}b binds attribute {urn:upa}a to a lax wildcard while the restriction binds it to a skip wildcard, and loc-testSubP clause 2 "},
		{"clause 3: strict subsumes strict",
			strict, strict, true, ""},
		{"clause 3: strict does NOT subsume skip",
			strict, wildcardKeywordBinding{keyword: ProcessSkip}, false,
			"but {urn:upa}b binds attribute {urn:upa}a to a strict wildcard while the restriction binds it to a skip wildcard, and loc-testSubP clause 3 "},
		{"clause 3: strict does NOT subsume lax",
			strict, wildcardKeywordBinding{keyword: ProcessLax}, false,
			"but {urn:upa}b binds attribute {urn:upa}a to a strict wildcard while the restriction binds it to a lax wildcard, and loc-testSubP clause 3 "},
		{"clause 1: skip subsumes lax",
			wildcardKeywordBinding{keyword: ProcessSkip}, wildcardKeywordBinding{keyword: ProcessLax}, true, ""},
		{"clause 2: lax subsumes lax",
			wildcardKeywordBinding{keyword: ProcessLax}, wildcardKeywordBinding{keyword: ProcessLax}, true, ""},
		{"clause 2: lax subsumes strict",
			wildcardKeywordBinding{keyword: ProcessLax}, strict, true, ""},
		{"strict over an attribute use is the documented fail-open",
			strict, use, true, ""},
		{"an Element Declaration does not subsume an Attribute Use",
			decl, use, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := s.checkBindingSubsumes(uq("a"), complexTypeAttributeRestriction(tt, bb), tc.general, tc.specific)
			if tc.wantOK && err != nil {
				t.Fatalf("expected ·subsumes·, got %v", err)
			}
			if tc.wantOK {
				return
			}
			expectRule(t, err, ruleDerivationOKRestriction)
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("error %q does not contain %q", err, tc.wantMsg)
			}
		})
	}
}

// TestBindingSubsumesChargesTheRestrictingType pins the POSITION of the two
// derivation-ok-restriction clause 3 (c-ran) rejections in checkBindingSubsumes
// and checkKeywordSubsumes: both are charged to the restricting complex type —
// the spec's T, whose own §3.4.6.3 obligation clause 3 is (#359).
//
// Through checkBindingSubsumes these two sites are unreachable from a parsed
// document, because checkAttributeRestriction (attributerestriction.go) always
// passes it an attributeUseBinding as the specific binding: no keyword general
// binding can then fail keywordSubsumes, and no pair can reach the
// mismatched-kind fallthrough. They are pinned here rather than in parser's
// end-to-end TestRestrictionAttributeRejectionsCiteTheRestrictingType for
// exactly that reason. checkKeywordSubsumes' other caller,
// checkAttributeRestrictionWildcard, does reach its keyword charge from a
// document; parser's TestRestrictionAttributeWildcardKeyword pins that one. b
// carries a DIFFERENT position, so charging the base instead fails.
func TestBindingSubsumesChargesTheRestrictingType(t *testing.T) {
	tLoc := xsderr.Loc{URI: "t.xsd", Line: 11, Col: 3}
	bLoc := xsderr.Loc{URI: "b.xsd", Line: 22, Col: 5}
	s := bSchema(t, nil)
	tt := dTypeAt(t, tLoc, uq("t"), anyTypeName, EmptyContent{}, nil, nil)
	bb := dTypeAt(t, bLoc, uq("b"), anyTypeName, EmptyContent{}, nil, nil)

	for _, tc := range []struct {
		name     string
		general  defaultBinding
		specific defaultBinding
	}{
		{"loc-testSubP fallthrough: an Element Declaration does not subsume an Attribute Use",
			elementDeclarationBinding{decl: uLocal(t, uq("a"), uq("str"))},
			attributeUseBinding{use: dAttr(t, uq("a"), uq("str"))}},
		{"loc-testSubP clause 2: lax does not subsume skip",
			wildcardKeywordBinding{keyword: ProcessLax},
			wildcardKeywordBinding{keyword: ProcessSkip}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := s.checkBindingSubsumes(uq("a"), complexTypeAttributeRestriction(tt, bb), tc.general, tc.specific)
			expectRule(t, err, ruleDerivationOKRestriction)
			var xe *xsderr.Error
			if !errors.As(err, &xe) {
				t.Fatalf("error %v is not an *xsderr.Error", err)
			}
			if xe.Loc != tLoc {
				t.Fatalf("loc = %s, want the restricting type's %s", xe.Loc, tLoc)
			}
		})
	}
}

// TestBindingSubsumesAttributeUses covers loc-testSubP clause 5's three
// sub-conditions and the pass case.
func TestBindingSubsumesAttributeUses(t *testing.T) {
	s := bSchema(t, nil)
	tt := dType(t, uq("t"), anyTypeName, EmptyContent{}, nil, nil)
	bb := dType(t, uq("b"), anyTypeName, EmptyContent{}, nil, nil)
	fixed := NewValueConstraint(ValueFixed, "7", nil, nil)
	other := NewValueConstraint(ValueDefault, "7", nil, nil)

	inheritable := func(u AttributeUse) AttributeUse {
		u.inheritable = inheritTrue
		return u
	}

	for _, tc := range []struct {
		name     string
		general  AttributeUse
		specific AttributeUse
		wantOK   bool
	}{
		{"identical uses subsume",
			dAttr(t, uq("a"), uq("str")), dAttr(t, uq("a"), uq("str")), true},
		{"5.1: a narrower type is validly derived",
			dAttr(t, uq("a"), uq("str")), dAttr(t, uq("a"), uq("narrow")), true},
		{"5.1: a wider type is not",
			dAttr(t, uq("a"), uq("narrow")), dAttr(t, uq("a"), uq("str")), false},
		{"5.2: keeping the base's fixed value is fine",
			dAttrUse(t, uq("a"), uq("str"), false, &fixed), dAttrUse(t, uq("a"), uq("str"), false, &fixed), true},
		{"5.2: dropping the base's fixed value is not",
			dAttrUse(t, uq("a"), uq("str"), false, &fixed), dAttr(t, uq("a"), uq("str")), false},
		{"5.2: replacing fixed with default is not",
			dAttrUse(t, uq("a"), uq("str"), false, &fixed), dAttrUse(t, uq("a"), uq("str"), false, &other), false},
		{"5.2.1: a base default constraint imposes nothing",
			dAttrUse(t, uq("a"), uq("str"), false, &other), dAttr(t, uq("a"), uq("str")), true},
		{"5.3: {inheritable} must be equal",
			dAttr(t, uq("a"), uq("str")), inheritable(dAttr(t, uq("a"), uq("str"))), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := s.checkBindingSubsumes(uq("a"), complexTypeAttributeRestriction(tt, bb),
				attributeUseBinding{use: tc.general}, attributeUseBinding{use: tc.specific})
			if tc.wantOK && err != nil {
				t.Fatalf("expected ·subsumes·, got %v", err)
			}
			if !tc.wantOK {
				expectRule(t, err, ruleDerivationOKRestriction)
			}
		})
	}
}

// TestEffectiveValueConstraintFallback pins key-evc's three-step fallback,
// including the Ref variant, which is why the helper lives on *Schema at all.
func TestEffectiveValueConstraintFallback(t *testing.T) {
	declFixed := NewValueConstraint(ValueFixed, "7", nil, nil)
	useDefault := NewValueConstraint(ValueDefault, "9", nil, nil)
	s := bSchema(t, func(b *SchemaBuilder) {
		global, err := NewAttributeDeclaration(xsderr.Loc{}, uq("g"), TypeDefinitionRef{Name: uq("str")}, NewAttributeGlobalScope(), &declFixed, false)
		if err != nil {
			t.Fatalf("NewAttributeDeclaration: %v", err)
		}
		b.AddAttribute(global)
	})

	own, err := NewAttributeUse(xsderr.Loc{}, false, AttributeDeclarationRef{Name: uq("g")}, &useDefault, nil)
	if err != nil {
		t.Fatalf("NewAttributeUse: %v", err)
	}
	if vc, ok := s.EffectiveValueConstraint(own); !ok || vc.Kind() != ValueDefault || vc.LexicalForm() != "9" {
		t.Fatalf("the use's OWN {value constraint} must win: got %+v ok=%t", vc, ok)
	}

	inherited, err := NewAttributeUse(xsderr.Loc{}, false, AttributeDeclarationRef{Name: uq("g")}, nil, nil)
	if err != nil {
		t.Fatalf("NewAttributeUse: %v", err)
	}
	if vc, ok := s.EffectiveValueConstraint(inherited); !ok || vc.Kind() != ValueFixed || vc.LexicalForm() != "7" {
		t.Fatalf("a use with no own constraint must fall back to the resolved declaration's: got %+v ok=%t", vc, ok)
	}

	bare := dAttr(t, uq("a"), uq("str"))
	if _, ok := s.EffectiveValueConstraint(bare); ok {
		t.Fatalf("a use whose declaration has no {value constraint} must be ·absent·")
	}
}

// TestAttributeUsesIdenticalComparesResolvedInheritable pins §3.5.1 property
// identity on the resolved {inheritable}: a ref use writing no inheritable over
// a declaration whose {inheritable} is false holds the same property as one
// writing inheritable="false" (§3.2.2.3 ref.att.local), and a different one
// from one writing "true" (#1682).
func TestAttributeUsesIdenticalComparesResolvedInheritable(t *testing.T) {
	s := bSchema(t, func(b *SchemaBuilder) {
		global, err := NewAttributeDeclaration(xsderr.Loc{}, uq("g"), TypeDefinitionRef{Name: uq("str")}, NewAttributeGlobalScope(), nil, false)
		if err != nil {
			t.Fatalf("NewAttributeDeclaration: %v", err)
		}
		b.AddAttribute(global)
	})
	use := func(own *bool) AttributeUse {
		u, err := NewAttributeUse(xsderr.Loc{}, false, AttributeDeclarationRef{Name: uq("g")}, nil, own)
		if err != nil {
			t.Fatalf("NewAttributeUse: %v", err)
		}
		return u
	}
	yes, no := true, false
	if !s.attributeUsesIdentical(use(nil), use(&no)) {
		t.Error(`absent inheritable and inheritable="false" over a false declaration: not identical, want identical`)
	}
	if s.attributeUsesIdentical(use(nil), use(&yes)) {
		t.Error(`absent inheritable and inheritable="true" over a false declaration: identical, want not identical`)
	}
}

// vcElem builds a global element declaration of the named type carrying vc as
// its {value constraint}.
func vcElem(t *testing.T, local string, typeName QName, vc *ValueConstraint) ElementDeclaration {
	t.Helper()
	e, err := NewElementDeclaration(xsderr.Loc{}, uq(local), TypeDefinitionRef{Name: typeName}, nil,
		NewGlobalScope(), vc, false, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("NewElementDeclaration(%s): %v", local, err)
	}
	return e
}

// TestElementDeclarationSubsumesFixedValues pins loc-testSubP clause 4.2's
// fourth outcome — both {value constraint}s fixed — as a VALUE-space test
// delegated to the installed ValueSpace, and pins that an UNDECIDED verdict
// still accepts. The two lexical forms differ, so a lexical comparison would
// decide where only the value space may.
func TestElementDeclarationSubsumesFixedValues(t *testing.T) {
	gvc := NewValueConstraint(ValueFixed, "7", nil, nil)
	svc := NewValueConstraint(ValueFixed, "07", nil, nil)

	for _, tc := range []struct {
		name string
		vs   *stubValueSpace
		want bool
	}{
		{"decided equal-or-identical subsumes", &stubValueSpace{same: true, decided: true}, true},
		{"decided NOT equal-or-identical does not", &stubValueSpace{same: false, decided: true}, false},
		{"undecided subsumes (fail-open)", &stubValueSpace{same: false, decided: false}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := vcSchema(t, tc.vs, nil)
			if err != nil {
				t.Fatalf("FinalizeWith: %v", err)
			}
			got := s.elementDeclarationSubsumes(vcElem(t, "g", uq("str"), &gvc), vcElem(t, "s", uq("narrow"), &svc))
			if got != tc.want {
				t.Fatalf("elementDeclarationSubsumes = %t, want %t", got, tc.want)
			}
			if tc.vs.calls == 0 {
				t.Fatal("clause 4.2 decided without consulting the ValueSpace")
			}
		})
	}
}

// TestElementDeclarationSubsumesFixedValuesSkipsUnresolvedType pins clause 4.2's
// other fail-open branch: a {type definition} that names no simple type — here
// xs:anyType, whose {content type} is mixed — leaves the clause with no value
// space to compare in, so it accepts without consulting the ValueSpace at all.
func TestElementDeclarationSubsumesFixedValuesSkipsUnresolvedType(t *testing.T) {
	gvc := NewValueConstraint(ValueFixed, "7", nil, nil)
	svc := NewValueConstraint(ValueFixed, "07", nil, nil)
	vs := &stubValueSpace{same: false, decided: true}
	s, err := vcSchema(t, vs, nil)
	if err != nil {
		t.Fatalf("FinalizeWith: %v", err)
	}
	if !s.fixedValueConstraintSubsumes(vcElem(t, "g", anyTypeName, &gvc), vcElem(t, "s", anyTypeName, &svc)) {
		t.Fatal("a mixed complex {type definition} names no value space, so clause 4.2 must accept")
	}
	if vs.calls != 0 {
		t.Fatalf("the ValueSpace was consulted %d time(s) with no simple type to compare in", vs.calls)
	}
}

// TestElementDeclarationSubsumesFixedValuesInSimpleContent pins that clause 4.2
// compares in a simple-content complex type's {content type}.{simple type
// definition}, the type cos-valid-default clause 1 names for it (#1379): a
// decided NOT-same rejects, and the ValueSpace is handed that simple type.
func TestElementDeclarationSubsumesFixedValuesInSimpleContent(t *testing.T) {
	gvc := NewValueConstraint(ValueFixed, "7", nil, nil)
	svc := NewValueConstraint(ValueFixed, "8", nil, nil)
	vs := &recordingValueSpace{stubValueSpace: stubValueSpace{same: false, decided: true}}
	s, err := vcSchema(t, vs, func(b *SchemaBuilder) {
		content := dPrimitive(t, uq("content"))
		b.AddType(content)
		b.AddType(dType(t, uq("sc"), anyTypeName, SimpleContent{SimpleType: content}, nil, nil))
	})
	if err != nil {
		t.Fatalf("FinalizeWith: %v", err)
	}
	if s.fixedValueConstraintSubsumes(vcElem(t, "g", uq("sc"), &gvc), vcElem(t, "s", uq("content"), &svc)) {
		t.Fatal("a decided NOT-same in the simple content's type must fail clause 4.2")
	}
	if vs.calls != 1 {
		t.Fatalf("the ValueSpace was consulted %d time(s), want 1", vs.calls)
	}
	if vs.ta == nil || vs.ta.Name() != uq("content") || vs.tb == nil || vs.tb.Name() != uq("content") {
		t.Fatalf("compared in (%v, %v), want both sides in content", vs.ta, vs.tb)
	}
}

// recordingValueSpace is stubValueSpace recording the two types the last
// EqualOrIdentical was handed.
type recordingValueSpace struct {
	stubValueSpace
	ta, tb *SimpleType
}

func (r *recordingValueSpace) EqualOrIdentical(res TypeResolver, ta *SimpleType, a ValueConstraint, tb *SimpleType, b ValueConstraint) (bool, bool) {
	r.ta, r.tb = ta, tb
	return r.stubValueSpace.EqualOrIdentical(res, ta, a, tb, b)
}

// TestAttributeValueConstraintSubsumesFixedValues pins loc-testSubP clause
// 5.2.2 the same way: two fixed ·effective value constraints· are compared in
// the value space, and an undecided verdict accepts.
func TestAttributeValueConstraintSubsumesFixedValues(t *testing.T) {
	gvc := NewValueConstraint(ValueFixed, "7", nil, nil)
	svc := NewValueConstraint(ValueFixed, "07", nil, nil)

	for _, tc := range []struct {
		name    string
		vs      *stubValueSpace
		wantErr bool
	}{
		{"decided equal-or-identical subsumes", &stubValueSpace{same: true, decided: true}, false},
		{"decided NOT equal-or-identical does not", &stubValueSpace{same: false, decided: true}, true},
		{"undecided subsumes (fail-open)", &stubValueSpace{same: false, decided: false}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := vcSchema(t, tc.vs, nil)
			if err != nil {
				t.Fatalf("FinalizeWith: %v", err)
			}
			tt := dType(t, uq("t"), anyTypeName, EmptyContent{}, nil, nil)
			bb := dType(t, uq("b"), anyTypeName, EmptyContent{}, nil, nil)
			err = s.checkBindingSubsumes(uq("a"), complexTypeAttributeRestriction(tt, bb),
				attributeUseBinding{use: dAttrUse(t, uq("a"), uq("str"), false, &gvc)},
				attributeUseBinding{use: dAttrUse(t, uq("a"), uq("narrow"), false, &svc)})
			if tc.vs.calls == 0 {
				t.Fatal("clause 5.2.2 decided without consulting the ValueSpace")
			}
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("expected ·subsumes·, got %v", err)
				}
				return
			}
			expectRule(t, err, ruleDerivationOKRestriction)
		})
	}
}
