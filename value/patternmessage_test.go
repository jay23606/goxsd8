package value_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin"
	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// ownedChain is the [xsd.TypeResolver] for a graph whose every slot is the owned
// arm, as builtin.Seed's and the restrictions below are: nothing is looked up by
// name, so it resolves nothing.
type ownedChain struct{}

func (ownedChain) Type(xsd.QName) (xsd.TypeDefinition, bool) { return nil, false }

// TestPatternRejectionNamesFailedStep pins what a cvc-pattern-valid rejection
// (§4.3.4.4) says: the whole OR-set of the derivation step the literal failed,
// each member as written save the XML references patternMemberEscaper puts for
// `&`, `"`, LF and CR, the type on the base chain that declares it, and the
// checked type when that type inherits it — so xs:int's "x3" fails a pattern
// facet xs:int inherits from xs:integer (Datatypes integer.pattern), not a
// pattern the schema author never wrote (#2310), and a list item or union
// member names the item or member type, not the list or union (#3335). With
// CheckLexical's message reverted to the one naming neither, every row fails;
// with its members unescaped again, the quote, line-break and ampersand rows
// fail (#2350), and the backslash rows fail under a %q rendering.
func TestPatternRejectionNamesFailedStep(t *testing.T) {
	b := strict.New()
	seeded, err := builtin.Seed(b)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	builtins := map[string]*xsd.SimpleType{}
	for _, st := range seeded {
		builtins[st.Name().Local] = st
	}
	restrict := func(name xsd.QName, base *xsd.SimpleType, patterns ...string) *xsd.SimpleType {
		t.Helper()
		st, err := xsd.NewSimpleType(xsderr.Loc{}, name, xsd.RestrictionDerivation{}, xsd.OwnedSimpleType{Definition: base},
			[]xsd.Facet{xsd.NewFacet(xsd.FacetPattern, patterns, false)}, nil)
		if err != nil {
			t.Fatalf("NewSimpleType(%s): %v", name, err)
		}
		if err := st.CheckDerivation(ownedChain{}); err != nil {
			t.Fatalf("CheckDerivation(%s): %v", name, err)
		}
		return st
	}
	code := restrict(xsd.QName{Space: "urn:t", Local: "Code"}, builtins["string"], "[A-Z]{3}")
	narrower := restrict(xsd.QName{Space: "urn:t", Local: "Narrower"}, code, "AB.")
	anon := restrict(xsd.QName{}, builtins["string"], "[0-9]+", "[a-z]+")
	pair := restrict(xsd.QName{Space: "urn:t", Local: "Pair"}, builtins["string"], "a", "b")
	quote := restrict(xsd.QName{Space: "urn:t", Local: "Q"}, builtins["string"], `a", "b`)
	lines := restrict(xsd.QName{Space: "urn:t", Local: "N"}, builtins["string"], "a\nb", "c\rd")
	amp := restrict(xsd.QName{Space: "urn:t", Local: "Amp"}, builtins["string"], "a&#10;b")
	// anonHeir is an anonymous restriction of anon that declares no pattern of
	// its own: both sides of a Declaring() == Name() test are the zero QName,
	// so only facetOrigin's kind-and-position rule reads the facet as
	// inherited (#552).
	anonHeir, err := xsd.NewSimpleType(xsderr.Loc{}, xsd.QName{}, xsd.RestrictionDerivation{}, xsd.OwnedSimpleType{Definition: anon},
		[]xsd.Facet{xsd.NewFacet(xsd.FacetMaxLength, []string{"5"}, false)}, nil)
	if err != nil {
		t.Fatalf("NewSimpleType(anonHeir): %v", err)
	}
	if err := anonHeir.CheckDerivation(ownedChain{}); err != nil {
		t.Fatalf("CheckDerivation(anonHeir): %v", err)
	}
	ints, err := xsd.NewSimpleType(xsderr.Loc{}, xsd.QName{Space: "urn:t", Local: "Ints"},
		xsd.ListDerivation{Item: xsd.OwnedSimpleType{Definition: builtins["int"]}},
		xsd.OwnedSimpleType{Definition: xsd.AnySimpleType()},
		[]xsd.Facet{xsd.NewFacet(xsd.FacetWhiteSpace, []string{"collapse"}, true)}, nil)
	if err != nil {
		t.Fatalf("NewSimpleType(Ints): %v", err)
	}
	intOrDate, err := xsd.NewSimpleType(xsderr.Loc{}, xsd.QName{Space: "urn:t", Local: "IntOrDate"},
		xsd.UnionDerivation{Members: []xsd.SimpleTypeOrRef{
			xsd.OwnedSimpleType{Definition: builtins["int"]},
			xsd.OwnedSimpleType{Definition: builtins["date"]},
		}},
		xsd.OwnedSimpleType{Definition: xsd.AnySimpleType()}, nil, nil)
	if err != nil {
		t.Fatalf("NewSimpleType(IntOrDate): %v", err)
	}

	for _, tc := range []struct {
		name    string
		st      *xsd.SimpleType
		lexical string
		rule    xsderr.Rule
		want    string
	}{
		{"a built-in type fails its ancestor's built-in pattern", builtins["int"], "x3", "cvc-pattern-valid",
			`"x3" matches no member of the pattern facet that the simple type {http://www.w3.org/2001/XMLSchema}int inherits from the simple type {http://www.w3.org/2001/XMLSchema}integer, whose {value} holds "[\-+]?[0-9]+", but cvc-pattern-valid requires one to match`},
		{"a user-derived type fails its own pattern", code, "abc", "cvc-pattern-valid",
			`"abc" matches no member of the pattern facet of the simple type {urn:t}Code, whose {value} holds "[A-Z]{3}", but cvc-pattern-valid requires one to match`},
		{"the derived step fails where its base's passes", narrower, "XYZ", "cvc-pattern-valid",
			`"XYZ" matches no member of the pattern facet of the simple type {urn:t}Narrower, whose {value} holds "AB.", but cvc-pattern-valid requires one to match`},
		// Narrower declares a pattern of its own, yet Code's earlier entry is
		// inherited.
		{"the base step fails under a type with its own pattern", narrower, "abc", "cvc-pattern-valid",
			`"abc" matches no member of the pattern facet that the simple type {urn:t}Narrower inherits from the simple type {urn:t}Code, whose {value} holds "[A-Z]{3}", but cvc-pattern-valid requires one to match`},
		{"an anonymous type's whole OR-set", anon, "A1", "cvc-pattern-valid",
			`"A1" matches no member of the pattern facet of an anonymous simple type, whose {value} holds "[0-9]+", "[a-z]+", but cvc-pattern-valid requires one to match`},
		{"an anonymous type inherits its anonymous base's pattern", anonHeir, "A1", "cvc-pattern-valid",
			`"A1" matches no member of the pattern facet that an anonymous simple type inherits from an anonymous simple type, whose {value} holds "[0-9]+", "[a-z]+", but cvc-pattern-valid requires one to match`},
		// Q's one member holds `", "`; it must not read as Pair's two members.
		{"two members a and b", pair, "z", "cvc-pattern-valid",
			`"z" matches no member of the pattern facet of the simple type {urn:t}Pair, whose {value} holds "a", "b", but cvc-pattern-valid requires one to match`},
		{"one member holding a quote", quote, "z", "cvc-pattern-valid",
			`"z" matches no member of the pattern facet of the simple type {urn:t}Q, whose {value} holds "a&quot;, &quot;b", but cvc-pattern-valid requires one to match`},
		{"members holding LF and CR stay on one line", lines, "z", "cvc-pattern-valid",
			`"z" matches no member of the pattern facet of the simple type {urn:t}N, whose {value} holds "a&#10;b", "c&#13;d", but cvc-pattern-valid requires one to match`},
		// A member spelling a reference itself must not read as the LF one.
		{"a member holding an ampersand", amp, "z", "cvc-pattern-valid",
			`"z" matches no member of the pattern facet of the simple type {urn:t}Amp, whose {value} holds "a&amp;#10;b", but cvc-pattern-valid requires one to match`},
		{"a list item fails its item type's ancestor's pattern", ints, "1 x3", "cvc-pattern-valid",
			`"x3" matches no member of the pattern facet that the simple type {http://www.w3.org/2001/XMLSchema}int inherits from the simple type {http://www.w3.org/2001/XMLSchema}integer, whose {value} holds "[\-+]?[0-9]+", but cvc-pattern-valid requires one to match`},
		// No member accepts, so the union charges cvc-datatype-valid and folds
		// each member's reason in (union.go dispatchUnion); xs:int's is this one.
		{"a union member's reason inside the union's rejection", intOrDate, "x3", "cvc-datatype-valid",
			`"x3" matches no member of the pattern facet that the simple type {http://www.w3.org/2001/XMLSchema}int inherits from the simple type {http://www.w3.org/2001/XMLSchema}integer, whose {value} holds "[\-+]?[0-9]+", but cvc-pattern-valid requires one to match`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := value.ValidateLexical(b, ownedChain{}, tc.st, tc.lexical, nil, declineEvery{})
			if err == nil {
				t.Fatalf("ValidateLexical(%s, %q) = nil, want a %s rejection", tc.st.Name(), tc.lexical, tc.rule)
			}
			if r, _ := xsderr.RuleOf(err); r != tc.rule {
				t.Errorf("ValidateLexical(%s, %q) charged %s, want %s: %v", tc.st.Name(), tc.lexical, r, tc.rule, err)
			}
			if !strings.Contains(err.Error(), "[cvc-pattern-valid] "+tc.want) {
				t.Errorf("ValidateLexical(%s, %q) =\n%v\nwant it to carry\n[cvc-pattern-valid] %s", tc.st.Name(), tc.lexical, err, tc.want)
			}
		})
	}
}

// TestPatternRejectionRendersWhole pins xs:int's "x3" cause by equality through
// [xsderr.Error.Error]: E5's order — the literal, the facet with its relation to
// xs:int and its {value}, then the inline citation — with no `value ` lead and
// no trailing `(rule, §N)` tail (#2369). The zero Loc a facet stage charges at
// renders as `?`.
func TestPatternRejectionRendersWhole(t *testing.T) {
	b := strict.New()
	seeded, err := builtin.Seed(b)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	var intType *xsd.SimpleType
	for _, st := range seeded {
		if st.Name().Local == "int" {
			intType = st
		}
	}
	_, err = value.ValidateLexical(b, ownedChain{}, intType, "x3", nil, declineEvery{})
	var xe *xsderr.Error
	if !errors.As(err, &xe) {
		t.Fatalf("ValidateLexical(xs:int, %q) = %v, want an *xsderr.Error", "x3", err)
	}
	const want = `?: [cvc-pattern-valid] "x3" matches no member of the pattern facet that the simple type {http://www.w3.org/2001/XMLSchema}int inherits from the simple type {http://www.w3.org/2001/XMLSchema}integer, whose {value} holds "[\-+]?[0-9]+", but cvc-pattern-valid requires one to match`
	if got := xe.Error(); got != want {
		t.Errorf("ValidateLexical(xs:int, %q).Error() =\n%s\nwant\n%s", "x3", got, want)
	}
}
