package xmltree_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/parser/xmltree"
	"github.com/kud360/goxsd8/xsderr"
)

// render writes nodes as one line: a start tag as <{uri}local a="v">, an end
// tag as </local>, and character data quoted.
func render(nodes []xmltree.Node) string {
	var b strings.Builder
	for _, n := range nodes {
		switch n := n.(type) {
		case *xmltree.StartElement:
			b.WriteString("<" + nameOf(n.Name()))
			for _, a := range n.Attributes() {
				fmt.Fprintf(&b, " %s=%q", nameOf(a.Name()), a.Value())
			}
			b.WriteString(">")
		case *xmltree.EndElement:
			b.WriteString("</" + n.Name().Local() + ">")
		case *xmltree.CharData:
			fmt.Fprintf(&b, "%q", n.Data())
		}
	}
	return b.String()
}

// nameOf renders n as {uri}local, or local in no namespace.
func nameOf(n xmltree.Name) string {
	if n.Space() == "" {
		return n.Local()
	}
	return "{" + n.Space() + "}" + n.Local()
}

// TestInternalEntityIsIncluded reads a reference to an internal general entity
// as its replacement text (XML 1.0 §4.4.2, §4.4.5, §3.3.3): in content, parsed
// as content, so markup in it is elements resolved against the scope in force
// at the reference; in an attribute value, normalized, each white-space
// character in the replacement text a #x20 while a character reference in the
// value's own source stays the character it names; nested, expanded at
// inclusion and not at declaration (§4.5); and under the FIRST declaration of
// a name (§4.2).
func TestInternalEntityIsIncluded(t *testing.T) {
	for _, tc := range []struct {
		name, subset, root, want string
	}{
		{"content", `<!ENTITY e "text">`, `<r>a&e;b</r>`, `<r>"atextb"</r>`},
		{"content markup, scoped at the reference", `<!ENTITY e "x<p:b c='1'>y</p:b>z">`,
			`<r xmlns:p="urn:p">a&e;b</r>`, `<r>"ax"<{urn:p}b c="1">"y"</b>"zb"</r>`},
		{"attribute value normalized", `<!ENTITY e "a&#10;b	c">`,
			`<r v="[&e;]" w="&#10;&e;"/>`, `<r v="[a b c]" w="\na b c"></r>`},
		{"nested, in content", `<!ENTITY d "[0-9]"><!ENTITY h "(&d;|A)">`, `<r>&h;</r>`, `<r>"([0-9]|A)"</r>`},
		{"nested, in an attribute value", `<!ENTITY d "[0-9]"><!ENTITY h "(&d;|A)">`,
			`<r v="&h;"/>`, `<r v="([0-9]|A)"></r>`},
		{"nested, in an attribute of an included element", `<!ENTITY d "[0-9]"><!ENTITY b "<b v='&d;'/>">`,
			`<r>&b;</r>`, `<r><b v="[0-9]"></b></r>`},
		{"first declaration binds", `<!ENTITY a "1"><!ENTITY a "2">`, `<r v="&a;">&a;</r>`, `<r v="1">"1"</r>`},
		{"predefined entity left to inclusion", `<!ENTITY e "a&amp;b&lt;c/>">`, `<r v="&e;">&e;</r>`,
			`<r v="a&b<c/>">"a&b<c/>"</r>`},
		{"doubly escaped '<' in an attribute value", `<!ENTITY e "&#38;#60;">`, `<r v="&e;"/>`, `<r v="<"></r>`},
		{"CDATA in replacement text", `<!ENTITY e "<![CDATA[&x;]]>">`, `<r>&e;</r>`, `<r>"&x;"</r>`},
		{"line end in the literal", "<!ENTITY e \"a\r\nb&#65;\rc\">", `<r>&e;</r>`, `<r>"a\nbA\nc"</r>`},
		{"line end in the source beside a reference", `<!ENTITY e "x">`, "<r v=\"a\r\n&e;\">a\r\n&e;</r>",
			`<r v="a x">"a\nx"</r>`},
		{"character reference to #xD in the literal", `<!ENTITY e "a&#13;b">`, `<r>&e;</r>`, `<r>"a\rb"</r>`},
		{"empty replacement text", `<!ENTITY e "">`, `<r>&e;</r>`, `<r></r>`},
		{"after an unread parameter entity, standalone", `<!ENTITY % p SYSTEM "p.ent"> %p; <!ENTITY e "x">`,
			`<r>&e;</r>`, `<r>"x"</r>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decl := `<?xml version="1.0"?>`
			if strings.Contains(tc.name, "standalone") {
				decl = `<?xml version="1.0" standalone="yes"?>`
			}
			nodes, err := collect(t, "doc.xml", decl+`<!DOCTYPE r [`+tc.subset+`]>`+tc.root)
			if err != nil {
				t.Fatalf("Token: %v", err)
			}
			if got := render(nodes); got != tc.want {
				t.Errorf("read %s\n want %s", got, tc.want)
			}
		})
	}
}

// TestIncludedNodesAreLocatedAtTheReference locates every node an inclusion
// produces where the reference is, and the text after the reference where it
// is in the source.
func TestIncludedNodesAreLocatedAtTheReference(t *testing.T) {
	nodes, err := collect(t, "doc.xml", "<!DOCTYPE r [<!ENTITY e \"x<b/>\">]><r>a\n  &e;b</r>")
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got, want := render(nodes), `<r>"a\n  x"<b></b>"b"</r>`; got != want {
		t.Fatalf("read %s, want %s", got, want)
	}
	wantLoc(t, nodes[1], 1, 38)
	wantLoc(t, nodes[2], 2, 3)
	wantLoc(t, nodes[3], 2, 3)
	wantLoc(t, nodes[4], 2, 6)
}

// TestEntityInclusionFaults charges what XML 1.0 makes not well-formed in an
// inclusion as a fault the reader charges itself, wrapping no cause: a
// recursive pair (WFC No Recursion), in content and in an attribute value; a
// '<' in replacement text an attribute value includes, at any depth (WFC No <
// in Attribute Values); replacement text that is not balanced content (§4.3.2);
// and a reference outside the document element, to an internal entity or to an
// external one, which the decoder does not refuse first.
func TestEntityInclusionFaults(t *testing.T) {
	for _, tc := range []struct {
		name, subset, root, msg string
	}{
		{"recursive pair in content", `<!ENTITY a "x&b;"><!ENTITY b "&a;">`, `<r>&a;</r>`, "entity a references itself"},
		{"recursive pair in an attribute value", `<!ENTITY a "x&b;"><!ENTITY b "&a;">`, `<r v="&b;"/>`, "entity b references itself"},
		{"'<' in an attribute value", `<!ENTITY e "&#60;">`, `<r v="&e;"/>`, "No < in Attribute Values"},
		{"'<' one level down", `<!ENTITY i "&#60;"><!ENTITY o "x&i;">`, `<r v="&o;"/>`, "entity i, referenced in an attribute value"},
		{"element left open", `<!ENTITY e "<b>">`, `<r>&e;</r>`, "element b opened in the replacement text of entity e does not close"},
		{"element closed outside", `<!ENTITY e "</r>">`, `<r>&e;</r>`, "closes an element the entity did not open"},
		{"reference after the document element", `<!ENTITY e "x">`, `<r/>&e;`, "entity reference outside the document element"},
		{"external reference after the document element", `<!ENTITY x SYSTEM "x.ent">`, `<r/>&x;`, "entity reference outside the document element"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect(t, "doc.xml", `<!DOCTYPE r [`+tc.subset+`]>`+tc.root)
			wantWellFormednessError(t, err)
			var e *xsderr.Error
			if !errors.As(err, &e) || e.Err != nil {
				t.Errorf("error %v: want a charge wrapping no cause", err)
			}
			if !strings.Contains(fmt.Sprint(err), tc.msg) {
				t.Errorf("error %v: want it to say %q", err, tc.msg)
			}
		})
	}
}

// TestEntityReferenceRefusedUnread refuses, and charges as no fault of the
// document's (a wrapped cause), a reference to an entity the reader did not
// read the declaration of: one declared after a reference to a parameter
// entity it did not read, outside a standalone document (XML 1.0 §5.1). Such
// a document may be well-formed (WFC Entity Declared binds only a document
// with no unread parameter-entity reference or a standalone one), so the
// refusal is the reader's policy and names no constraint; the standalone row of
// TestInternalEntityIsIncluded reads the same subset. A reference in replacement
// text included in content to an entity that is not internal, and one past the
// expansion bounds, are refused alike; one in replacement text an attribute
// value includes is TestAttributeValueExternalEntity's fault.
func TestEntityReferenceRefusedUnread(t *testing.T) {
	laughs := `<!ENTITY l0 "lol">`
	for i := 1; i <= 9; i++ {
		laughs += fmt.Sprintf(`<!ENTITY l%d "%s">`, i, strings.Repeat(fmt.Sprintf("&l%d;", i-1), 10))
	}
	// sizeBound is the reader's maxGEExpansion: the bytes of replacement text
	// one document may include.
	const sizeBound = 1 << 20
	big := `<!ENTITY big "` + strings.Repeat("x", sizeBound) + `">`
	chain := func(n int) string {
		var b strings.Builder
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, `<!ENTITY c%d "&c%d;">`, i, i+1)
		}
		fmt.Fprintf(&b, `<!ENTITY c%d "end">`, n)
		return b.String()
	}
	for _, tc := range []struct {
		name, subset, root, msg string
	}{
		{"declared after an unread parameter entity", `<!ENTITY % p SYSTEM "p.ent"> %p; <!ENTITY e "x">`, `<r>&e;</r>`, "invalid character entity &e;"},
		{"external entity in replacement text", `<!ENTITY x SYSTEM "x.ent"><!ENTITY e "&x;">`, `<r>&e;</r>`, "entity &x;, which is not an internal entity"},
		{"billion laughs in content", laughs, `<r>&l9;</r>`, "expansion bound"},
		{"billion laughs in an attribute value", laughs, `<r v="&l9;"/>`, "expansion bound"},
		{"nesting past the depth bound", chain(64), `<r>&c0;</r>`, "expansion bound"},
		{"past the size bound, summed over the document", big, `<r>&big;<b v="&big;"/></r>`, "expansion bound"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect(t, "doc.xml", `<!DOCTYPE r [`+tc.subset+`]>`+tc.root)
			wantWellFormednessError(t, err)
			var e *xsderr.Error
			if !errors.As(err, &e) || e.Err == nil {
				t.Errorf("error %v: want a refusal wrapping its cause", err)
			}
			if !strings.Contains(fmt.Sprint(err), tc.msg) {
				t.Errorf("error %v: want it to say %q", err, tc.msg)
			}
			if strings.Contains(fmt.Sprint(err), "Entity Declared") || strings.Contains(fmt.Sprint(err), "entdeclared") {
				t.Errorf("error %v names WFC Entity Declared, which the document need not break", err)
			}
		})
	}
	t.Run("replacement text at the size bound", func(t *testing.T) {
		nodes, err := collect(t, "doc.xml", `<!DOCTYPE r [`+big+`]><r>&big;</r>`)
		if err != nil {
			t.Fatalf("Token: %v", err)
		}
		if got := len(nodes[1].(*xmltree.CharData).Data()); got != sizeBound {
			t.Errorf("read %d bytes, want %d", got, sizeBound)
		}
	})
	t.Run("nesting at the depth bound", func(t *testing.T) {
		nodes, err := collect(t, "doc.xml", `<!DOCTYPE r [`+chain(63)+`]><r>&c0;</r>`)
		if err != nil {
			t.Fatalf("Token: %v", err)
		}
		if got, want := render(nodes), `<r>"end"</r>`; got != want {
			t.Errorf("read %s, want %s", got, want)
		}
	})
}

// TestEntityDeclaredOnlyInParameterEntity charges, in a standalone="yes"
// document, a reference in content, in an attribute value and in replacement
// text to a general entity declared only in a parameter entity's replacement
// text (XML 1.0 WFC Entity Declared, which counts only a declaration outside
// every parameter entity), as a fault the reader charges itself, whether the
// entity is internal, external or unparsed: the clause keys on where the
// declaration stands, so the decoder's refusal of an entity that is not
// internal does not answer first. A reference in replacement text is charged
// only where the binding declaration (§4.2) of the innermost entity whose text
// holds it stands outside every parameter entity: through f, bound in one, to
// g, declared outside, the reference to x in g's text is charged. Each control
// is a fault row's document less one difference: no standalone="yes", where
// the parameter-entity reference read lifts the constraint, or a second
// declaration of the name outside every parameter entity, after the first or
// before it. Two more read a reference to x, declared only in a parameter
// entity, in the replacement text of f, whose binding declaration stands in
// that parameter entity though a later one of f stands outside, so the
// reference occurs within a parameter entity and is not charged (#2365), in
// content and in an attribute value. Each refused row is such a control for an
// external entity, referenced in content, which the reader does not read:
// refused, wrapping a cause, and charging no Entity Declared; the last is the
// content control with x external, whose attribute-value twin breaks WFC No
// External Entity References (TestAttributeValueExternalEntity).
func TestEntityDeclaredOnlyInParameterEntity(t *testing.T) {
	const alone = `<?xml version="1.0" standalone="yes"?>`
	const pe = `<!ENTITY % p "<!ENTITY e 'x'>">%p;`
	const xpe = `<!ENTITY % p "<!ENTITY x SYSTEM 'x.ent'>">%p;`
	// f's binding declaration (§4.2) stands in %p;, so a reference f's
	// replacement text holds occurs within a parameter entity; g's stands
	// outside every one, so one g's holds does not, though f references g.
	const peBound = `<!ENTITY % p "<!ENTITY f '[&x;]'><!ENTITY x 'y'>">%p;<!ENTITY f "z">`
	const peBoundExt = `<!ENTITY % p "<!ENTITY f '[&x;]'><!ENTITY x SYSTEM 'x.ent'>">%p;<!ENTITY f "z">`
	const viaG = `<!ENTITY % p "<!ENTITY f '&g;'><!ENTITY x 'y'>">%p;<!ENTITY f "z"><!ENTITY g "[&x;]">`
	const msg = `[xml-wf] reference to entity &%s; in a standalone="yes" document, where no general entity declaration outside every parameter entity declares it (XML 1.0 WFC Entity Declared)`
	for _, tc := range []struct {
		subset, root, want string
	}{
		{pe, `<r>&e;</r>`, `d.xml:1:91: ` + fmt.Sprintf(msg, "e")},
		{pe, `<r a="&e;"/>`, `d.xml:1:91: ` + fmt.Sprintf(msg, "e")},
		{pe + `<!ENTITY f "[&e;]">`, `<r>&f;</r>`, `d.xml:1:110: ` + fmt.Sprintf(msg, "e")},
		{xpe, `<r>&x;</r>`, `d.xml:1:102: ` + fmt.Sprintf(msg, "x")},
		{xpe, `<r a="&x;"/>`, `d.xml:1:102: ` + fmt.Sprintf(msg, "x")},
		{`<!NOTATION n SYSTEM 'n'><!ENTITY % p "<!ENTITY x SYSTEM 'x.ent' NDATA n>">%p;`, `<r>&x;</r>`, `d.xml:1:134: ` + fmt.Sprintf(msg, "x")},
		{viaG, `<r>&f;</r>`, `d.xml:1:142: ` + fmt.Sprintf(msg, "x")},
		{viaG, `<r a="&f;"/>`, `d.xml:1:142: ` + fmt.Sprintf(msg, "x")},
	} {
		t.Run(tc.subset+tc.root, func(t *testing.T) {
			_, err := collect(t, "d.xml", alone+`<!DOCTYPE r [`+tc.subset+`]>`+tc.root)
			wantWellFormednessError(t, err)
			var e *xsderr.Error
			if !errors.As(err, &e) || e.Err != nil {
				t.Errorf("error %v: want a charge wrapping no cause", err)
			}
			if fmt.Sprint(err) != tc.want {
				t.Errorf("error = %q, want %q", err, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		decl, subset, root, want string
	}{
		{`<?xml version="1.0"?>`, pe, `<r a="&e;">&e;</r>`, `<r a="x">"x"</r>`},
		{alone, pe + `<!ENTITY e "y">`, `<r a="&e;">&e;</r>`, `<r a="x">"x"</r>`},
		{alone, `<!ENTITY e "y">` + pe, `<r a="&e;">&e;</r>`, `<r a="y">"y"</r>`},
		{alone, peBound, `<r>&f;</r>`, `<r>"[y]"</r>`},
		{alone, peBound, `<r a="&f;"/>`, `<r a="[y]"></r>`},
	} {
		t.Run(tc.decl+tc.subset+tc.root, func(t *testing.T) {
			nodes, err := collect(t, "d.xml", tc.decl+`<!DOCTYPE r [`+tc.subset+`]>`+tc.root)
			if err != nil {
				t.Fatalf("Token: %v", err)
			}
			if got := render(nodes); got != tc.want {
				t.Errorf("read %s\n want %s", got, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		decl, subset, root string
	}{
		{alone, `<!ENTITY x SYSTEM 'x.ent'>`, `<r>&x;</r>`},
		{`<?xml version="1.0"?>`, xpe, `<r>&x;</r>`},
		{alone, xpe + `<!ENTITY x SYSTEM 'y.ent'>`, `<r>&x;</r>`},
		{alone, peBoundExt, `<r>&f;</r>`},
	} {
		t.Run(tc.decl+tc.subset+tc.root, func(t *testing.T) {
			_, err := collect(t, "d.xml", tc.decl+`<!DOCTYPE r [`+tc.subset+`]>`+tc.root)
			wantWellFormednessError(t, err)
			var e *xsderr.Error
			if !errors.As(err, &e) || e.Err == nil {
				t.Errorf("error %v: want a refusal wrapping its cause", err)
			}
			if strings.Contains(fmt.Sprint(err), "Entity Declared") {
				t.Errorf("error %v names WFC Entity Declared, which the document does not break", err)
			}
		})
	}
}

// TestAttributeValueExternalEntity charges a reference in an attribute value,
// directly or through replacement text, to an external entity whose
// declaration the reader read (XML 1.0 WFC No External Entity References) as
// a fault the reader charges itself, wrapping no cause and located at the
// attribute's name: outside a standalone document, in one whose external entity is
// declared in a parameter entity (the constraint keys on neither), in the
// replacement text of an entity bound in a parameter entity, where WFC Entity
// Declared is not charged (#2365), and in an attribute of an element
// replacement text included in content opens, located at that reference. A
// reference to an unparsed entity there is WFC Parsed Entity, checked first as
// entityGraph's walk of a default value checks it, and charged so in a
// standalone="yes" document whose declaration of it follows a parameter-entity
// reference the reader did not read, which §5.1 has the reader process (#2459);
// one in content is TestContentUnparsedEntity's fault. Each guard is refused,
// wrapping a cause and naming neither constraint: the same external entity
// referenced in content, where §4.4.3 lets the reader decline to include it, and,
// outside a standalone document, an external one declared after a parameter-entity
// reference the reader did not read (§5.2), referenced in an attribute value
// directly and through an entity declared before that reference, which reaches the
// charge with no declaration recorded.
func TestAttributeValueExternalEntity(t *testing.T) {
	const alone = `<?xml version="1.0" standalone="yes"?>`
	const plain = `<?xml version="1.0"?>`
	const ext = `<!ENTITY x SYSTEM 'x.ent'>`
	const ndata = `<!NOTATION n SYSTEM 'n'><!ENTITY u SYSTEM 'u.bin' NDATA n>`
	const unread = `<!ENTITY % q SYSTEM "q.ent"> %q; `
	const peBoundExt = `<!ENTITY % p "<!ENTITY f '[&x;]'><!ENTITY x SYSTEM 'x.ent'>">%p;<!ENTITY f "z">`
	const noExt = `[xml-wf] attribute value that references, directly or indirectly, the external entity x (XML 1.0 WFC: No External Entity References)`
	const parsed = `[xml-wf] attribute value that references, directly or indirectly, the unparsed entity u (XML 1.0 WFC: Parsed Entity)`
	for _, tc := range []struct {
		decl, subset, root, want string
	}{
		{plain, ext, `<r a="&x;"/>`, `d.xml:1:66: ` + noExt},
		{plain, ext + `<!ENTITY e "&x;">`, `<r a="&e;"/>`, `d.xml:1:83: ` + noExt},
		{plain, `<!ENTITY % p "` + ext + `">%p;`, `<r a="&x;"/>`, `d.xml:1:85: ` + noExt},
		{alone, peBoundExt, `<r a="&f;"/>`, `d.xml:1:136: ` + noExt},
		{plain, ext + `<!ENTITY b "<b a='&x;'/>">`, `<r>&b;</r>`, `d.xml:1:92: ` + noExt},
		{plain, ndata, `<r a="&u;"/>`, `d.xml:1:98: ` + parsed},
		{plain, ndata + `<!ENTITY e "&u;">`, `<r a="&e;"/>`, `d.xml:1:115: ` + parsed},
		{alone, unread + ndata, `<r a="&u;"/>`, `d.xml:1:148: ` + parsed},
	} {
		t.Run(tc.decl+tc.subset+tc.root, func(t *testing.T) {
			_, err := collect(t, "d.xml", tc.decl+`<!DOCTYPE r [`+tc.subset+`]>`+tc.root)
			wantWellFormednessError(t, err)
			var e *xsderr.Error
			if !errors.As(err, &e) || e.Err != nil {
				t.Errorf("error %v: want a charge wrapping no cause", err)
			}
			if fmt.Sprint(err) != tc.want {
				t.Errorf("error = %q, want %q", err, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		subset, root string
	}{
		{ext, `<r>&x;</r>`},
		{unread + ext, `<r a="&x;"/>`},
		{`<!ENTITY e "&x;">` + unread + ext, `<r a="&e;"/>`},
	} {
		t.Run(tc.subset+tc.root, func(t *testing.T) {
			_, err := collect(t, "d.xml", plain+`<!DOCTYPE r [`+tc.subset+`]>`+tc.root)
			wantWellFormednessError(t, err)
			var e *xsderr.Error
			if !errors.As(err, &e) || e.Err == nil {
				t.Errorf("error %v: want a refusal wrapping its cause", err)
			}
			if s := fmt.Sprint(err); strings.Contains(s, "No External Entity References") || strings.Contains(s, "Parsed Entity") {
				t.Errorf("error %v names a constraint the reader does not decide", err)
			}
		})
	}
}

// TestContentUnparsedEntity charges a reference in content, directly or through
// replacement text, to an unparsed entity whose declaration the reader read
// (XML 1.0 WFC Parsed Entity) as a fault the reader charges itself, wrapping no
// cause and located at the outermost reference: outside a standalone document,
// declared in a parameter entity the reader read, declared before an internal
// entity of the same name, which does not bind (§4.2), and whatever its
// notation, whose declaration is a validity constraint (VC Notation Declared).
// In a standalone="yes" document it is charged so too where the declaration
// follows a parameter-entity reference the reader did not read, directly and
// through replacement text, since §5.1 has the reader process that
// declaration (#2459), as where it precedes that reference or stands in an
// internal subset beside an external one, which it binds before (§2.8, §4.2).
// §4.4.3, which lets the reader decline to include an external parsed entity,
// does not cover an unparsed one. Each guard keeps its answer: outside a
// standalone document, an unparsed entity declared after a parameter-entity
// reference the reader did not read is refused, wrapping a cause and naming
// no constraint (§5.2), and an EntityValue that references one is well-formed
// where nothing includes it (§4.4.4).
func TestContentUnparsedEntity(t *testing.T) {
	const plain = `<?xml version="1.0"?><!DOCTYPE r`
	const alone = `<?xml version="1.0" standalone="yes"?><!DOCTYPE r`
	const ndata = `<!NOTATION n SYSTEM 'n'><!ENTITY u SYSTEM 'u.bin' NDATA n>`
	const unread = `<!ENTITY % q SYSTEM "q.ent"> %q; `
	const parsed = `[xml-wf] content references, directly or through replacement text, the unparsed entity u, which XML 1.0 WFC Parsed Entity forbids`
	for _, tc := range []struct {
		head, subset, root, want string
	}{
		{plain, ndata, `<r>&u;</r>`, `d.xml:1:98: ` + parsed},
		{plain, ndata + `<!ENTITY e "&u;">`, `<r>&e;</r>`, `d.xml:1:115: ` + parsed},
		{plain, ndata + `<!ENTITY b "<b>&u;</b>">`, `<r>&b;</r>`, `d.xml:1:122: ` + parsed},
		{plain, `<!NOTATION n SYSTEM 'n'><!ENTITY % p "<!ENTITY u SYSTEM 'u.bin' NDATA n>">%p;`, `<r>&u;</r>`, `d.xml:1:117: ` + parsed},
		{plain, ndata + `<!ENTITY u "x">`, `<r>&u;</r>`, `d.xml:1:113: ` + parsed},
		{plain, `<!ENTITY u SYSTEM 'u.bin' NDATA n>`, `<r>&u;</r>`, `d.xml:1:74: ` + parsed},
		{alone, unread + ndata, `<r>&u;</r>`, `d.xml:1:148: ` + parsed},
		{alone, unread + ndata + `<!ENTITY e "&u;">`, `<r>&e;</r>`, `d.xml:1:165: ` + parsed},
		{alone, ndata + unread, `<r>&u;</r>`, `d.xml:1:148: ` + parsed},
		{alone + ` SYSTEM "ext.dtd"`, ndata, `<r>&u;</r>`, `d.xml:1:132: ` + parsed},
	} {
		doc := tc.head + ` [` + tc.subset + `]>` + tc.root
		t.Run(doc, func(t *testing.T) {
			_, err := collect(t, "d.xml", doc)
			wantWellFormednessError(t, err)
			var e *xsderr.Error
			if !errors.As(err, &e) || e.Err != nil {
				t.Errorf("error %v: want a charge wrapping no cause", err)
			}
			if fmt.Sprint(err) != tc.want {
				t.Errorf("error = %q, want %q", err, tc.want)
			}
		})
	}
	t.Run("declared after an unread parameter entity", func(t *testing.T) {
		_, err := collect(t, "d.xml", plain+` [`+unread+ndata+`]><r>&u;</r>`)
		wantWellFormednessError(t, err)
		var e *xsderr.Error
		if !errors.As(err, &e) || e.Err == nil {
			t.Errorf("error %v: want a refusal wrapping its cause", err)
		}
		if strings.Contains(fmt.Sprint(err), "Parsed Entity") {
			t.Errorf("error %v names WFC Parsed Entity, which the reader does not decide", err)
		}
	})
	t.Run("referenced only in an EntityValue", func(t *testing.T) {
		nodes, err := collect(t, "d.xml", plain+` [`+ndata+`<!ENTITY e "&u;">]><r/>`)
		if err != nil {
			t.Fatalf("Token: %v", err)
		}
		if got, want := render(nodes), `<r></r>`; got != want {
			t.Errorf("read %s, want %s", got, want)
		}
	})
}

// TestEntityAmpersandBeginsNoReference charges a '&' in included replacement
// text that begins no Reference (XML 1.0 §4.4.2, §4.4.5, [67] Reference) as a
// fault the reader charges itself, wrapping no cause, at the attribute's name
// in an attribute value and at the outermost reference in content, and naming
// the entity whose replacement text holds it: a literal spelling it `&#38;` or
// `&#x26;`, in an attribute value and in content; a ';' after it that closes no
// Name; one an included entity's replacement text holds; and one in an attribute
// value of a start tag replacement text holds. Each control reads: a '&' its
// literal spelled `&#38;` that begins a CharRef, to '<' or to '&', or an
// EntityRef to a declared entity, and one inside a comment or a CDATA section.
// An EntityRef so spelled to an undeclared entity is the reader's refusal,
// wrapping a cause, as TestEntityReferenceRefusedUnread refuses one. A '&' in a
// comment, a processing instruction or an end tag the decoder fails to read is
// no such charge: the decoder's own syntax error is wrapped as the cause.
func TestEntityAmpersandBeginsNoReference(t *testing.T) {
	const msg = "[xml-wf] the replacement text of entity %s holds a '&' that begins no Reference, '&' Name ';' or a character reference (XML 1.0 §4.4.2, [67] Reference, [68] EntityRef, [66] CharRef)"
	for _, tc := range []struct {
		subset, root, want string
	}{
		{`<!ENTITY e "a&#38;b">`, `<r a="&e;"/>`, "d.xml:1:40: " + fmt.Sprintf(msg, "e")},
		{`<!ENTITY e "a&#38;b">`, `<r>&e;</r>`, "d.xml:1:40: " + fmt.Sprintf(msg, "e")},
		{`<!ENTITY e "a&#x26;b">`, `<r a="&e;"/>`, "d.xml:1:41: " + fmt.Sprintf(msg, "e")},
		{`<!ENTITY e "a&#x26;b">`, `<r>&e;</r>`, "d.xml:1:41: " + fmt.Sprintf(msg, "e")},
		{`<!ENTITY e "a&#38;b c;">`, `<r a="&e;"/>`, "d.xml:1:43: " + fmt.Sprintf(msg, "e")},
		{`<!ENTITY e "a&#38;b c;">`, `<r>&e;</r>`, "d.xml:1:43: " + fmt.Sprintf(msg, "e")},
		{`<!ENTITY f "a&#38;b"><!ENTITY e "x&f;">`, `<r a="&e;"/>`, "d.xml:1:58: " + fmt.Sprintf(msg, "f")},
		{`<!ENTITY f "a&#38;b"><!ENTITY e "x&f;">`, `<r>&e;</r>`, "d.xml:1:58: " + fmt.Sprintf(msg, "f")},
		{`<!ENTITY e "<b v='a&#38;b'/>">`, `<r>&e;</r>`, "d.xml:1:49: " + fmt.Sprintf(msg, "e")},
	} {
		t.Run(tc.subset+tc.root, func(t *testing.T) {
			_, err := collect(t, "d.xml", `<!DOCTYPE r [`+tc.subset+`]>`+tc.root)
			wantWellFormednessError(t, err)
			if errors.Unwrap(err) != nil {
				t.Errorf("error %v wraps the cause %v, want a charge wrapping none", err, errors.Unwrap(err))
			}
			if fmt.Sprint(err) != tc.want {
				t.Errorf("error = %q, want %q", err, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		subset, root, want string
	}{
		{`<!ENTITY e "a&#38;#60;b">`, `<r a="&e;">&e;</r>`, `<r a="a<b">"a<b"</r>`},
		{`<!ENTITY e "a&#38;#38;b">`, `<r a="&e;">&e;</r>`, `<r a="a&b">"a&b"</r>`},
		{`<!ENTITY b "v"><!ENTITY e "a&#38;b;">`, `<r a="&e;">&e;</r>`, `<r a="av">"av"</r>`},
		{`<!ENTITY e "<!-- &#38; --><![CDATA[&#38;]]>x">`, `<r>&e;</r>`, `<r>"&x"</r>`},
	} {
		t.Run(tc.subset+tc.root, func(t *testing.T) {
			nodes, err := collect(t, "d.xml", `<!DOCTYPE r [`+tc.subset+`]>`+tc.root)
			if err != nil {
				t.Fatalf("Token: %v", err)
			}
			if got := render(nodes); got != tc.want {
				t.Errorf("read %s\n want %s", got, tc.want)
			}
		})
	}
	for _, root := range []string{`<r a="&e;"/>`, `<r>&e;</r>`} {
		t.Run(root, func(t *testing.T) {
			_, err := collect(t, "d.xml", `<!DOCTYPE r [<!ENTITY e "a&#38;b;">]>`+root)
			wantWellFormednessError(t, err)
			if errors.Unwrap(err) == nil || !strings.Contains(fmt.Sprint(err), "&b;") {
				t.Errorf("error %v: want the refusal of &b;, wrapping its cause", err)
			}
		})
	}
	for _, tc := range []struct {
		subset, cause string
	}{
		{`<!ENTITY e "<!-- -- 'a&#38;b' -->">`, `invalid sequence "--" not allowed in comments`},
		{`<!ENTITY e "<?xml version='a&#38;b'?>">`, `unsupported version "a&b"`},
		{`<!ENTITY e "<b></b x='a&#38;b'>">`, `invalid characters between </b and >`},
	} {
		t.Run(tc.subset+"<r>&e;</r>", func(t *testing.T) {
			_, err := collect(t, "d.xml", `<!DOCTYPE r [`+tc.subset+`]><r>&e;</r>`)
			wantWellFormednessError(t, err)
			if strings.Contains(fmt.Sprint(err), "begins no Reference") {
				t.Errorf("error %v charges a '&' that begins no Reference in a comment, processing instruction or end tag", err)
			}
			if cause := errors.Unwrap(err); cause == nil || !strings.Contains(cause.Error(), tc.cause) {
				t.Errorf("error %v: want it to wrap the decoder's cause %q", err, tc.cause)
			}
		})
	}
}
