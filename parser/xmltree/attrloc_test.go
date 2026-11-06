package xmltree_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/parser/xmltree"
)

// TestAttributeIsLocatedAtItsName pins Attribute.Loc's contract on a start
// tag's first and later lines, past a namespace declaration, white space
// around '=' and values holding '>', '=' or the other quote, and on a start tag
// in an entity's replacement text.
func TestAttributeIsLocatedAtItsName(t *testing.T) {
	type at struct{ name, line, col int }
	for _, c := range []struct {
		name string
		doc  string
		elem int
		want []at
	}{
		{"one line", `<r a="1" b='2'/>`, 0, []at{{0, 1, 4}, {1, 1, 10}}},
		{"later lines", "<r a=\"1\"\n  xmlns:p='urn:p' p:b = '2'\r\n\tc\n=\n\"x>y='z'\"/>", 0, []at{{0, 1, 4}, {1, 2, 19}, {2, 3, 2}}},
		{"after a declaration", `<r xmlns="urn:d" a="&#9;"/>`, 0, []at{{0, 1, 18}}},
		{"in replacement text", "<!DOCTYPE r [<!ENTITY e \"<s x='1'\ny='2'/>\">]><r>\n  &e;</r>", 2, []at{{0, 3, 3}, {1, 3, 3}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			nodes, err := collect(t, "t.xml", c.doc)
			if err != nil {
				t.Fatalf("collect: %v", err)
			}
			start, ok := nodes[c.elem].(*xmltree.StartElement)
			if !ok {
				t.Fatalf("nodes[%d] = %T, want *StartElement", c.elem, nodes[c.elem])
			}
			attrs := start.Attributes()
			if len(attrs) != len(c.want) {
				t.Fatalf("%d attributes, want %d", len(attrs), len(c.want))
			}
			for _, w := range c.want {
				loc := attrs[w.name].Loc()
				if loc.URI != "t.xml" || loc.Line != w.line || loc.Col != w.col {
					t.Errorf("attribute %s at %s, want t.xml:%d:%d", attrs[w.name].Name().Local(), loc, w.line, w.col)
				}
			}
		})
	}
}

// TestAttributeFaultIsLocatedAtItsName pins that every charge the reader makes
// about one attribute of a start tag in document source answers that
// attribute's name, where Attribute.Loc places it, and not the tag's '<': the
// faulted attribute is the tag's second, on a later line than the '<', so a
// charge at the '<' or at the first attribute fails its row. The charges are
// an unbound attribute prefix, nsc-NoPrefixUndecl on xmlns:p="" in an XML 1.0
// document, and, for a value that references entity e, a '<' or a '&'
// beginning no Reference in e's replacement text, an illegal character
// reference there (WFC Legal Character), e external (WFC No External Entity
// References) or unparsed (WFC Parsed Entity), and e recursive (WFC No
// Recursion). Each guard is a charge against the element itself, an unbound
// or reserved prefix on its name, which stays at the '<' though the tag
// carries an attribute on a later line.
func TestAttributeFaultIsLocatedAtItsName(t *testing.T) {
	const tag = "<r a=\"1\"\n  b=\"&e;\"/>"
	doctype := func(decls string) string { return "<!DOCTYPE r [" + decls + "]>\n" + tag }
	for _, tc := range []struct {
		name, doc, want string
	}{
		{"unbound prefix", "<r a=\"1\"\n  p:b=\"2\"/>", `t.xml:2:3: [xml-wf] unbound namespace prefix "p" on attribute p:b`},
		{"prefix undeclared in 1.0", "<r a=\"1\"\n  xmlns:p=\"\"/>", `t.xml:2:3: [xml-wf] namespace declaration xmlns:p has an empty value`},
		{"'<' in replacement text", doctype(`<!ENTITY e "&#60;">`), `t.xml:3:3: [xml-wf] the replacement text of entity e, referenced in an attribute value, holds '<'`},
		{"'&' begins no Reference", doctype(`<!ENTITY e "a&#38;b">`), `t.xml:3:3: [xml-wf] the replacement text of entity e holds a '&' that begins no Reference`},
		{"Legal Character", doctype(`<!ENTITY e "&#38;#0;">`), `t.xml:3:3: [xml-wf] character reference &#0; names no XML character`},
		{"No External Entity References", doctype(`<!ENTITY e SYSTEM "e.ent">`), `t.xml:3:3: [xml-wf] attribute value that references, directly or indirectly, the external entity e`},
		{"Parsed Entity", doctype(`<!NOTATION n SYSTEM "n"><!ENTITY e SYSTEM "e.bin" NDATA n>`), `t.xml:3:3: [xml-wf] attribute value that references, directly or indirectly, the unparsed entity e`},
		{"No Recursion", doctype(`<!ENTITY e "x&f;"><!ENTITY f "&e;">`), `t.xml:3:3: [xml-wf] entity e references itself`},
		{"guard: unbound element prefix", "<p:r a=\"1\"\n  b=\"2\"/>", `t.xml:1:1: [xml-wf] unbound namespace prefix "p" on element <p:r>`},
		{"guard: reserved element prefix", "<xmlns:r a=\"1\"\n  b=\"2\"/>", `t.xml:1:1: [xml-wf] "xmlns" is a reserved prefix and cannot name an element`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect(t, "t.xml", tc.doc)
			wantWellFormednessError(t, err)
			if !strings.HasPrefix(fmt.Sprint(err), tc.want) {
				t.Errorf("error = %q, want it to open %q", err, tc.want)
			}
		})
	}
}
