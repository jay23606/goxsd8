package parser

import (
	"errors"
	"fmt"
	"io"
	"net/url"

	"github.com/kud360/goxsd8/parser/xmltree"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// Document is a parsed schema document: its own URI and the root of the element
// tree ReadDocument built from it. The document URI is where the document was
// read from and the base of its xml:base chain; it is distinct from any
// individual element's own base URI, which differs once an xml:base attribute
// intervenes.
type Document struct {
	uri  string
	root *Element
}

// URI returns the document's own URI — the value passed to ReadDocument and the
// base of the xml:base composition chain. The root element's own base URI may
// differ from it when the root itself carries an xml:base attribute.
func (d *Document) URI() string { return d.uri }

// Root returns the root element of the document's tree. It is never nil for a
// Document returned by ReadDocument: a rootless input is a malformed-XML error.
func (d *Document) Root() *Element { return d.root }

// IsSchema reports whether the document's root element is the XSD <schema>
// element — expanded name {http://www.w3.org/2001/XMLSchema}schema (Glossary
// "schema document"; §3.17.2). Recognition here is purely nominal and is a
// derived predicate computed on each call, never a stored field (STYLE D3):
// "not a schema document" is not an error at this raw layer but a later
// component-production concern, and §3.17.2 allows <schema> not to be the
// document element, so a root-only reject would overreach.
func (d *Document) IsSchema() bool {
	name := d.root.Name()
	return name.Space() == xsd.XMLSchemaNS && name.Local() == "schema"
}

// rootSubject names the root element for a "not a <schema> document root"
// message. A <schema> outside the XSD namespace gets its namespace appended,
// because the bare name would read as the schema it is not (#2574); every other
// root keeps the bare local name.
func (d *Document) rootSubject() string {
	name := d.root.Name()
	switch {
	case name.Local() != "schema" || name.Space() == xsd.XMLSchemaNS:
		return "<" + name.Local() + ">"
	case name.Space() == "":
		return "<schema> in no namespace"
	default:
		return fmt.Sprintf("<schema> in namespace %q", name.Space())
	}
}

// ReadDocument reads a well-formed XML document from r into a schema-document
// tree, folding the parser/xmltree token stream through an open-element stack
// and composing each element's base URI top-down as it is built. uri names the
// document for locations and is the base of its xml:base chain; it is not
// opened or resolved here (it may even be a filesystem path).
//
// It errors only on XML well-formedness faults (delegated to xmltree's reader,
// carrying an xsderr.Loc), on I/O, on a rootless document, or on a document the
// reader refuses (see parser/xmltree's Contract); it never panics on malformed
// input, and it produces no xsd components — recognizing the root as a <schema>
// (IsSchema) and producing components are later concerns.
//
// The Document it returns is S1, the RAW document. ·Conditional-inclusion
// pre-processing· (§4.2.2) is not applied here: it is applied by [Parse] to
// every document of the assembly and by [Produce] to the one it is handed, so
// that the src-cip verdict it can reach stays a schema-validity error rather
// than arriving through this reader's well-formedness channel.
func ReadDocument(uri string, r io.Reader) (*Document, error) {
	reader := xmltree.NewReader(uri, r)
	var root *Element
	var stack []*Element
	for {
		tok, err := reader.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch n := tok.(type) {
		case *xmltree.StartElement:
			elem, err := buildElement(n, stack, uri)
			if err != nil {
				return nil, err
			}
			if len(stack) == 0 {
				root = elem
			}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, elem)
			}
			stack = append(stack, elem)
		case *xmltree.EndElement:
			// xmltree emits an EndElement only for a matching open element, so
			// the stack it mirrors is never empty here.
			stack = stack[:len(stack)-1]
		case *xmltree.CharData:
			// Character data outside any element has no element to attach to;
			// only element content becomes Text nodes. The reader has already
			// refused any such run that is not S (XML 1.0 [1] document, [27]
			// Misc), so nothing a document holds is lost here.
			if len(stack) == 0 {
				continue
			}
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, &Text{data: n.Data(), loc: n.Loc()})
		}
	}
	if root == nil {
		return nil, xsderr.New(xsderr.RuleXMLWellFormed, xsderr.Loc{URI: uri}, "document has no root element")
	}
	return &Document{uri: uri, root: root}, nil
}

// buildElement builds an Element from a resolved start tag, linking it to its
// parent (the innermost open element, or nil at the root) and composing its
// base URI against the parent's — the root's parent base being the document
// URI. Children are filled in by ReadDocument as later tokens arrive.
func buildElement(src *xmltree.StartElement, stack []*Element, docURI string) (*Element, error) {
	var parent *Element
	parentBase := docURI
	if len(stack) > 0 {
		parent = stack[len(stack)-1]
		parentBase = parent.baseURI
	}
	base, err := composeBaseURI(parentBase, xmlBaseOf(src))
	if err != nil {
		return nil, xsderr.Wrap(xsderr.RuleXMLWellFormed, src.Loc(), err)
	}
	return &Element{src: src, parent: parent, attrs: src.Attributes(), baseURI: base}, nil
}

// xmlBaseOf returns the value of the element's xml:base attribute
// ({http://www.w3.org/XML/1998/namespace}base, matched via
// xmltree.XMLNamespaceURI rather than a duplicated literal), or "" when the
// element declares none.
func xmlBaseOf(src *xmltree.StartElement) string {
	for _, a := range src.Attributes() {
		name := a.Name()
		if name.Space() == xmltree.XMLNamespaceURI && name.Local() == "base" {
			return a.Value()
		}
	}
	return ""
}

// composeBaseURI composes an element's base URI from its parent's base URI and
// its own xml:base attribute value, per [XML Base] over RFC 3986: an empty
// value inherits the parent base unchanged; otherwise the value is resolved as
// a relative reference against the parent base (net/url.URL.ResolveReference).
//
// This composition rule is NOT locally spec-grounded. Structures never mentions
// "xml:base" and never spells out the algorithm — it only names [[base URI]]
// (e.g. §3.13.2, §4.3.2), and xml-infoset.md defers the mechanism to [XML Base]
// without reproducing it. So this is external, standard XML infrastructure
// knowledge; no src-*/cos-* citation is attached deliberately, since inventing
// one would be a fabricated citation.
//
// The result is only a string: docURI may be a filesystem path rather than a
// real URL, and actual location resolution/fetching belongs to the
// loader/Resolver (parse phase 2), not to this raw layer.
func composeBaseURI(parentBase, value string) (string, error) {
	if value == "" {
		return parentBase, nil
	}
	base, err := url.Parse(parentBase)
	if err != nil {
		return "", fmt.Errorf("parsing base URI %q: %w", parentBase, err)
	}
	ref, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("parsing xml:base %q: %w", value, err)
	}
	return base.ResolveReference(ref).String(), nil
}
