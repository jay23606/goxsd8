package xmltree

import "github.com/kud360/goxsd8/xsderr"

// Node is one item in the token stream produced by a Reader: a start tag, an
// end tag, or a run of character data. It is a closed set (STYLE T2 sealed
// sum) — consumers type-switch over *StartElement, *EndElement, and
// *CharData — sealed by the unexported node method so no other package can
// add a case. Every Node answers Loc.
type Node interface {
	// Loc reports where the node begins: the document URI plus the 1-based
	// line and column of its first byte.
	Loc() xsderr.Loc
	node()
}

// StartElement is a resolved start tag (including the start half of an empty
// element such as <e/>). Its name and attribute names are namespace-resolved
// against the bindings in scope at the tag; unbound prefixes never reach a
// StartElement, they are reported as errors by Reader.Token.
type StartElement struct {
	name  Name
	attrs []Attribute
	scope *scope
	loc   xsderr.Loc
}

// Name returns the resolved (namespace, local) name of the element.
func (e *StartElement) Name() Name { return e.name }

// Attributes returns the element's attributes in document order, excluding
// namespace declarations (xmlns / xmlns:p), which are not attributes but
// scope. The slice is the reader's own; its Attribute values are immutable.
func (e *StartElement) Attributes() []Attribute { return e.attrs }

// LookupPrefix resolves prefix to a namespace URI using the bindings in
// scope at this element, so a consumer can later resolve a QName-valued
// lexical that occurred here (e.g. xsi:type="p:foo") per Datatypes §3.3.18.
// The empty prefix yields the default namespace ("" when none is in scope);
// "xml" always resolves without a declaration. ok is false for an unbound
// prefix.
func (e *StartElement) LookupPrefix(prefix string) (uri string, ok bool) {
	return e.scope.lookup(prefix)
}

// Loc reports the position of the element's opening "<".
func (e *StartElement) Loc() xsderr.Loc { return e.loc }

func (e *StartElement) node() {}

// EndElement is a resolved end tag (including the implicit end of an empty
// element). Its Name equals the matching StartElement's Name; a tag whose
// name does not match its open element is reported as an error, never
// emitted.
type EndElement struct {
	name Name
	loc  xsderr.Loc
}

// Name returns the resolved (namespace, local) name of the closed element.
func (e *EndElement) Name() Name { return e.name }

// Loc reports the position of the end tag's "<". For the synthesized end of
// an empty element <e/>, it reports the position just past the element, since
// the empty element has no distinct end tag.
func (e *EndElement) Loc() xsderr.Loc { return e.loc }

func (e *EndElement) node() {}

// CharData is a run of character data (text or CDATA) with entity and
// character references already expanded. Its bytes are copied out of the
// decoder, so they stay valid after the reader advances.
type CharData struct {
	data   string
	offset int64
	loc    xsderr.Loc
}

// Data returns the decoded character content.
func (c *CharData) Data() string { return c.data }

// Offset returns the byte offset of the content's first byte in the input
// stream, so a downstream decode error on this content can cite the exact
// position (the contract's "byte offset" for character content). Content
// beginning in an entity's replacement text answers the offset of the
// outermost reference that includes it (XML 1.0 §4.4.2).
func (c *CharData) Offset() int64 { return c.offset }

// Loc reports the position of the content's first byte, or, for content
// beginning in an entity's replacement text, of the outermost reference that
// includes it (XML 1.0 §4.4.2).
func (c *CharData) Loc() xsderr.Loc { return c.loc }

func (c *CharData) node() {}

// Attribute is one resolved attribute of a start tag.
type Attribute struct {
	name  Name
	value string
	loc   xsderr.Loc
}

// Name returns the resolved (namespace, local) name of the attribute. An
// unprefixed attribute is always in no namespace: the default namespace
// applies to element names, never to attribute names.
func (a Attribute) Name() Name { return a.name }

// Value returns the attribute's normalized value per XML 1.0 §3.3.3: a
// character reference is the character it names, a reference to a general
// entity is the normalized value of its replacement text, and each literal
// white-space character is #x20, a line end (§2.11) one #x20. When the
// DOCTYPE's internal subset defines the attribute, on its element type and
// under the names the tag spells, with an AttType other than CDATA, leading
// and trailing #x20 are then discarded and each run of #x20 is one #x20; an
// attribute it defines as CDATA, or not at all, keeps every #x20. A definition
// the reader does not process (XML 1.0 §5.1, see the package's contract) is
// none.
func (a Attribute) Value() string { return a.value }

// Loc reports the position of the first byte of the attribute's name as its
// start tag spells it, prefix included, or, for a start tag in an entity's
// replacement text, of the outermost reference that includes it (XML 1.0
// §4.4.2). The column counts bytes, from 1.
func (a Attribute) Loc() xsderr.Loc { return a.loc }
