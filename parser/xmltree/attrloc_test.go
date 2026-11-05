package xmltree_test

import (
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
