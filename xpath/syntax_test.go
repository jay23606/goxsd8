package xpath

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/internal/xmldecl"
	"github.com/kud360/goxsd8/internal/xmlenc"
)

// TestXPathSyntaxAccepts pins texts A.1's grammar derives, the hard lexical
// corners among them: each must recognize, because rejecting one charges
// ta-props-correct against a schema whose {test} is legal XPath 2.0.
func TestXPathSyntaxAccepts(t *testing.T) {
	for _, tc := range []struct{ expr, why string }{
		{"()", "[46] ParenthesizedExpr with no Expr: the empty sequence (s3_12si04 line 58)"},
		{"() (: :)", "a Comment after the last terminal is a separator"},
		{"@a (: c :) = 1", "a Comment between terminals"},
		{"(: a (: nested :) one :) 1", "Comments nest when balanced"},
		{"5 instance (: odd :) of xs:integer", "a Comment between the words of a two-word operator"},
		{"for (: loop :) $i in $x return $i", "a Comment between 'for' and '$' (gn: parens)"},
		{"address (: may be empty :)", "a name before a Comment is no FunctionCall"},
		{"div div div", "'div' as NameTest, operator, NameTest"},
		{"and and and", "'and' as NameTest, operator, NameTest"},
		{"AND", "keywords are case sensitive, so AND is a name"},
		{"/", "a leading lone slash (xgc: leading-lone-slash)"},
		{"(/) * 5", "a lone slash parenthesized as an operand"},
		{"5 * /", "a lone slash as the right operand"},
		{"/ union", "'union' after a slash is a NameTest (xgc: leading-lone-slash)"},
		{"$x instance of item", "item without '()' is an AtomicType QName"},
		{"1.", "DecimalLiteral with no fraction digits"},
		{".5", "DecimalLiteral with no integer digits"},
		{"1e3", "DoubleLiteral"},
		{"1.5E-2", "DoubleLiteral with a signed exponent"},
		{"foo-foo", "one QName: '-' is a NameChar"},
		{"foo -foo", "a QName, the minus operator, a QName"},
		{"foo(: c :)- foo", "a Comment separates a name from '-'"},
		{"p:if(x)", "a prefixed FunctionCall may use a reserved local name"},
		{"p:element(1, 2)", "a prefixed KindTest keyword is a FunctionCall"},
		{"child::p:x", "a prefixed NameTest after an axis"},
		{"if (@a) then 1 else 2", "[7] IfExpr"},
		{"some $x in @a satisfies $x = 1", "[6] QuantifiedExpr"},
		{"every $x in (1, 2), $y in 3 satisfies $x lt $y", "QuantifiedExpr with two bindings"},
		{"for $x in 1 to 3 return $x * 2", "[4] ForExpr and [11] RangeExpr"},
		{"@a cast as xs:double", "[49] SingleType"},
		{"@a cast as xs:double? = 1", "SingleType with its '?'"},
		{"@kind cast as messageTypeString='string'", "a cast to a user type is legal syntax (s3_12ii06, #894)"},
		{"string cast as string", "a NameTest cast to an unprefixed QName"},
		{"double('3' cast as float > 2)", "a FunctionCall over a comparison"},
		{"@a castable as xs:int", "[18] CastableExpr"},
		{"$x treat as item()*", "[17] TreatExpr with an OccurrenceIndicator"},
		{"$x instance of empty-sequence()", "SequenceType empty-sequence()"},
		{"$x instance of element(*, xs:anyType?)+", "an ElementTest with a nillable TypeName"},
		{"$x instance of attribute(a, xs:string)", "an AttributeTest"},
		{"$x instance of document-node(schema-element(p:e))", "a DocumentTest over a SchemaElementTest"},
		{"$x instance of processing-instruction('pi')", "a PITest over a StringLiteral"},
		{"a << b", "[24] NodeComp '<<'"},
		{"a >> b", "[24] NodeComp '>>'"},
		{"a is b", "[24] NodeComp 'is'"},
		{"@a != 'x' and @b <= 2 or @c >= 3", "GeneralComp operators under and/or"},
		{"*", "the bare Wildcard"},
		{"p:*", "Wildcard NCName ':' '*'"},
		{"*:n", "Wildcard '*' ':' NCName"},
		{"child::a/descendant-or-self::node()/@b", "axis steps and a KindTest"},
		{"ancestor-or-self::*[1]", "a reverse axis with a predicate"},
		{"../@a", "the abbreviated parent step"},
		{".//a[@b = 'c'][2]", "a context item and two predicates"},
		{"a | b union c intersect d except e", "union and intersect/except chains"},
		{"- - +1", "[20] UnaryExpr with several signs"},
		{"10 idiv 3 mod 2", "idiv and mod"},
		{`"a""b" = 'c''d'`, "StringLiteral escapes"},
		{"\t@a\r\n=\t1 ", "XML S around and between terminals"},
		{"namespace::*", "the namespace axis is in the grammar"},
		{"text()", "a KindTest as a step"},
		{"@attribute", "a keyword as an attribute NameTest"},
		{"for", "'for' with no '$' after it is a NameTest"},
		{"if", "'if' with no '(' after it is a NameTest"},
	} {
		if at, bad := xpathSyntaxError(tc.expr); bad {
			t.Errorf("xpathSyntaxError(%q) rejects at offset %d, want accepted (%s)", tc.expr, at, tc.why)
		}
	}
}

// TestXPathSyntaxRejects pins texts no derivation of A.1's grammar covers,
// each with the byte offset the grammar stops at.
func TestXPathSyntaxRejects(t *testing.T) {
	for _, tc := range []struct {
		expr string
		at   int
		why  string
	}{
		{")(", 0, "no ExprSingle opens with ')'"},
		{">", 0, "no ExprSingle opens with a comparator"},
		{"((7>=6)", 7, "one ')' short"},
		{"@6='hi'", 1, "a NodeTest is no number"},
		{"@numberOfChildren < 5 AND @numberOfChildren > 0", 22, "AND is a name, and no name follows a complete operand"},
		{"(@type cast as xs1::double)='double'", 18, "a SingleType is one QName; '::' cannot follow it"},
		{"xs1::double", 0, "xs1 is no axis"},
		{"", 0, "Expr needs an ExprSingle"},
		{"(: :)", 5, "a Comment alone is no ExprSingle"},
		{"@p:kind = 'x' extra", 14, "trailing tokens"},
		{"@p:kind = ", 10, "a comparator with no right operand"},
		{"(: open", 0, "an unclosed Comment"},
		{"'open", 0, "an unclosed StringLiteral"},
		{"1 = 2 = 3", 6, "ComparisonExpr takes one comparator"},
		{"1 to 2 to 3", 7, "RangeExpr takes one 'to'"},
		{"/ * 5", 4, "xgc: leading-lone-slash reads '/ *' first"},
		{"10div 3", 2, "A.2.2: a number and a name need a separator"},
		{"10 div3", 3, "div3 is one name, which cannot follow an operand"},
		{"1.2.3", 3, "A.2.2: '.' beside a numeric literal needs a separator"},
		{"foo- foo", 5, "foo- is one QName, and a name cannot follow it"},
		{"if(@a)", 6, "'if(' opens an IfExpr, which needs 'then'"},
		{"a/if(x)", 2, "if is a reserved function name"},
		{"item()", 0, "item is a reserved function name"},
		{"empty-sequence()", 0, "empty-sequence is a reserved function name"},
		{"typeswitch(1)", 0, "typeswitch is a reserved function name"},
		{"@a cast as xs:double+", 21, "SingleType takes no '+', and '+' then needs an operand"},
		{"4 treat as item() + 5", 20, "xgc: occurrence-indicators takes the '+'"},
		{"@ a : b", 4, "no white space inside a QName"},
		{"p : *", 2, "no white space inside a Wildcard"},
		{"@p: b", 2, "a ':' with neither an NCName nor '*' after it is no terminal"},
		{"p:", 1, "a QName's ':' needs an NCName after it"},
		{"*:*", 1, "no Wildcard '*' ':' '*'"},
		{"a ! b", 2, "'!' only opens '!='"},
		{"@a = 1", 2, "white space is XML S only, never U+00A0"},
		{"element(*:a)", 8, "an ElementTest takes '*' or a QName, never '*:n'"},
		{"element(p:*)", 8, "an ElementTest takes '*' or a QName, never 'p:*'"},
		{"p:*(1)", 3, "a Wildcard is no FunctionCall name"},
		{"/ union /", 9, "'union' after a slash is a NameTest, so the second '/' needs a step"},
		{"(1, )", 4, "a comma needs an ExprSingle after it"},
		{"processing-instruction(p:x)", 23, "a PITest takes an NCName, never a prefixed QName"},
		{"p:child::x", 0, "an axis name is never prefixed"},
		{"a[1", 3, "an unclosed predicate"},
	} {
		at, bad := xpathSyntaxError(tc.expr)
		if !bad {
			t.Errorf("xpathSyntaxError(%q) accepts, want rejected at offset %d (%s)", tc.expr, tc.at, tc.why)
			continue
		}
		if at != tc.at {
			t.Errorf("xpathSyntaxError(%q) rejects at offset %d, want %d (%s)", tc.expr, at, tc.at, tc.why)
		}
	}
}

// TestXPathSyntaxAcceptsSuiteValidTests is the over-charge guard: it feeds the
// recognizer every alternative/@test the W3C suite carries in a schema
// document of a schemaTest it declares valid, under any version, and fails
// on any value rejected — each such rejection becomes a ta-props-correct
// charge against a schema the suite says is valid. It reads testdata/xsdtests
// at run time, so it holds for whatever suite revision is checked out, and an
// absent submodule skips it.
//
// Only the schemaTest's own schemaDocuments are read, never a document they
// include or import.
func TestXPathSyntaxAcceptsSuiteValidTests(t *testing.T) {
	const suite = "../testdata/xsdtests"
	if _, err := os.Stat(filepath.Join(suite, "suite.xml")); err != nil {
		t.Skip("suite submodule absent: git submodule update --init testdata/xsdtests")
	}
	var docs []string
	err := filepath.WalkDir(suite, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".testSet") {
			return nil
		}
		valid, err := suiteValidSchemaDocuments(path)
		if err != nil {
			return err
		}
		docs = append(docs, valid...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	tests := 0
	for _, doc := range docs {
		if seen[doc] {
			continue
		}
		seen[doc] = true
		values, err := alternativeTests(doc)
		if err != nil {
			t.Fatalf("%s: %v", doc, err)
		}
		for _, v := range values {
			tests++
			if at, bad := xpathSyntaxError(v); bad {
				t.Errorf("%s: xpathSyntaxError(%q) rejects at offset %d, want accepted: the suite declares this schema valid", doc, v, at)
			}
		}
	}
	// A floor, so a reader that silently finds nothing cannot pass: suite
	// 7bc3365 yields well over this many.
	if tests < 100 {
		t.Fatalf("read %d alternative/@test values from suite-valid schemas, want at least 100", tests)
	}
	t.Logf("%d alternative/@test values from %d suite-valid schema documents", tests, len(seen))
}

// suiteTSNamespace is the W3C test-suite metadata namespace.
const suiteTSNamespace = "http://www.w3.org/XML/2004/xml-schema-test-suite/"

// suiteValidSchemaDocuments reads one testSet and returns the paths of the
// schemaDocuments of every schemaTest with an <expected validity="valid"/>,
// in document order.
func suiteValidSchemaDocuments(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := xml.NewDecoder(strings.NewReader(string(raw)))
	var out, pending []string
	valid, inSchemaTest := false, false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		switch el := tok.(type) {
		case xml.StartElement:
			if el.Name.Space != suiteTSNamespace {
				continue
			}
			switch el.Name.Local {
			case "schemaTest":
				inSchemaTest, valid, pending = true, false, nil
			case "schemaDocument":
				if inSchemaTest {
					pending = append(pending, filepath.Join(filepath.Dir(path), suiteAttr(el, "href")))
				}
			case "expected":
				if inSchemaTest && suiteAttr(el, "validity") == "valid" {
					valid = true
				}
			}
		case xml.EndElement:
			if el.Name.Space == suiteTSNamespace && el.Name.Local == "schemaTest" {
				if valid {
					out = append(out, pending...)
				}
				inSchemaTest = false
			}
		}
	}
}

// suiteAttr is the value of el's attribute with the given local name.
func suiteAttr(el xml.StartElement, local string) string {
	for _, a := range el.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// alternativeTests returns the test attribute of every XSD alternative
// element in the schema document at path, in document order. It reads the
// document as parser/xmltree does: internal/xmlenc drops a byte-order mark and
// transcodes UTF-16, and internal/xmldecl reads a 1.x declaration as 1.0. A
// document declaring any other encoding, ISO-8859-1 among them, is xmlenc's
// limit (#361) and fails this read loudly, never silently, but only once it
// carries "alternative", which no suite-valid document at suite 7bc3365 does.
func alternativeTests(path string) ([]string, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	body, mark := xmlenc.Decode(bytes.NewReader(file))
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(string(raw), "alternative") {
		return nil, nil // no element can be named alternative
	}
	dec := xml.NewDecoder(xmldecl.As10(bytes.NewReader(raw)))
	dec.CharsetReader = mark.CharsetReader
	var out []string
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		el, ok := tok.(xml.StartElement)
		if !ok || el.Name.Space != "http://www.w3.org/2001/XMLSchema" || el.Name.Local != "alternative" {
			continue
		}
		for _, a := range el.Attr {
			if a.Name.Space == "" && a.Name.Local == "test" {
				out = append(out, a.Value)
			}
		}
	}
}
