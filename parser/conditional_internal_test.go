package parser

import (
	"strings"
	"testing"
)

// readDoc reads doc into a tree, failing the test on a well-formedness fault.
// These tests are package-internal because conditionalInclude and the tree
// fields they inspect are unexported (STYLE T5) — what §4.2.2's transform leaves
// BEHIND on the <schema>-root stub, and what it carries across when it rebuilds,
// is invisible in the produced components.
func readDoc(t *testing.T, doc string) *Document {
	t.Helper()
	d, err := ReadDocument("mem://cip.xsd", strings.NewReader(doc))
	if err != nil {
		t.Fatalf("ReadDocument: %v", err)
	}
	return d
}

// TestConditionalIncludeRootStubAttributes pins §4.2.2's carve-out at the level
// only this package can see: a <schema> the transform ignores keeps
// targetNamespace, vc:minVersion and vc:maxVersion, "any attributes other than"
// those three are removed, and [children] is the empty sequence.
//
// vc:maxVersion is on the fixture deliberately, at a value that retains. The
// carve-out names it unconditionally, so it survives on a stub this processor
// reached through the MIN arm alone: which arm ignored the root never changes
// what the stub carries.
func TestConditionalIncludeRootStubAttributes(t *testing.T) {
	d := readDoc(t, `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"`+
		` xmlns:vc="http://www.w3.org/2007/XMLSchema-versioning"`+
		` targetNamespace="urn:a" id="root" elementFormDefault="qualified"`+
		` vc:minVersion="3.2" vc:maxVersion="9.9" vc:typeAvailable="xs:integer">`+
		`<xs:element name="e" type="xs:string"/></xs:schema>`)

	s2, err := conditionalInclude(d)
	if err != nil {
		t.Fatalf("conditionalInclude: %v", err)
	}
	root := s2.Root()
	if got := len(root.Children()); got != 0 {
		t.Fatalf("stub [children] holds %d nodes, want the empty sequence", got)
	}
	// A slice compared in order: §4.2.2 removes attributes and reorders nothing.
	want := []struct{ space, local, value string }{
		{"", "targetNamespace", "urn:a"},
		{versioningNS, "minVersion", "3.2"},
		{versioningNS, "maxVersion", "9.9"},
	}
	attrs := root.attrs
	if len(attrs) != len(want) {
		t.Fatalf("stub carries %d attributes, want %d: %v", len(attrs), len(want), attrs)
	}
	for i, w := range want {
		got := attrs[i]
		if got.Name().Space() != w.space || got.Name().Local() != w.local || got.Value() != w.value {
			t.Fatalf("stub attribute %d = {%s}%s=%q, want {%s}%s=%q",
				i, got.Name().Space(), got.Name().Local(), got.Value(), w.space, w.local, w.value)
		}
	}
	// The INPUT is untouched: S1 is a value the caller may still hold, and the
	// stub narrows a copy of its attribute list rather than the list itself.
	if got := len(d.Root().attrs); got != 6 {
		t.Fatalf("S1's root carries %d attributes after the transform, want its original 6", got)
	}
}

// TestConditionalIncludeRebuildCarriesTheTreeAcross pins what the rebuild
// preserves where a document IS transformed: character data (§4.2.2 removes
// elements and attributes, never text), each retained element's own attributes,
// and the parent link a later src-resolve phase walks up (§3.17.6.2). A rebuild
// that re-parented children onto the ORIGINAL tree would leave the producer
// reading a <schema> ancestor whose children still hold the ignored elements.
func TestConditionalIncludeRebuildCarriesTheTreeAcross(t *testing.T) {
	d := readDoc(t, `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"`+
		` xmlns:vc="http://www.w3.org/2007/XMLSchema-versioning">`+
		`<xs:annotation><xs:documentation>kept verbatim</xs:documentation></xs:annotation>`+
		`<xs:element name="gone" type="xs:string" vc:minVersion="3.2"/>`+
		`<xs:element name="kept" type="xs:string"/></xs:schema>`)

	s2, err := conditionalInclude(d)
	if err != nil {
		t.Fatalf("conditionalInclude: %v", err)
	}
	root := s2.Root()
	if root == d.Root() {
		t.Fatalf("the root was returned unrebuilt, but this document carries an element to ignore")
	}
	var names []string
	for _, child := range root.Children() {
		el, ok := child.(*Element)
		if !ok {
			continue
		}
		names = append(names, el.Name().Local()+"/"+attrOr(el, "name"))
		if el.parent != root {
			t.Fatalf("child %s is parented onto another element, so an ancestor walk reads S1", el.Name().Local())
		}
	}
	if len(names) != 2 || names[0] != "annotation/" || names[1] != "element/kept" {
		t.Fatalf("S2's element children = %v, want the <annotation> and <element name=\"kept\"> in document order", names)
	}
	ann, ok := root.Children()[0].(*Element)
	if !ok {
		t.Fatalf("first child is not an element")
	}
	doc, ok := ann.Children()[0].(*Element)
	if !ok {
		t.Fatalf("<annotation>'s first child is not an element")
	}
	text, ok := doc.Children()[0].(*Text)
	if !ok {
		t.Fatalf("<documentation>'s first child is %T, want the retained *Text run", doc.Children()[0])
	}
	if text.Data() != "kept verbatim" {
		t.Fatalf("retained character data = %q, want it byte for byte", text.Data())
	}
}
