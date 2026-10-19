package jsonsrc

import (
	"encoding/json"
	"fmt"

	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// The engine meets this package only through these views. An element
// implements neither optional document capability: a JSON document has no
// DTD, so [unparsedEntities] is not supported and every ENTITY value fails as
// validate.UnparsedEntities says, and [all declarations processed] is true,
// which is what its absence is taken to mean.
var (
	_ validate.Element   = (*element)(nil)
	_ validate.Children  = (*members)(nil)
	_ validate.Children  = (*scalarChildren)(nil)
	_ validate.Text      = (*text)(nil)
	_ validate.Attribute = (*attribute)(nil)
)

// xmlNamespace is the namespace name the prefix "xml" is bound to in every
// element's [in-scope namespaces] (Namespaces in XML §3).
const xmlNamespace = "http://www.w3.org/XML/1998/namespace"

// xsiNil is the expanded name of the xsi:nil attribute a null value presents
// (walker.nilled), and xsiType that of the xsi:type attribute a "$type"
// member presents (walker.typeMember).
var (
	xsiNil  = xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: "nil"}
	xsiType = xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: "type"}
)

// scope is the "$xmlns" bindings in scope at an element: those of the nearest
// object carrying a "$xmlns" member, the element's own or an ancestor's, and
// through parent those further out, to nil above the root. It is never
// changed once built, so every element below shares it.
type scope struct {
	parent   *scope
	bindings []binding
}

// binding is one member of a "$xmlns" object: prefix bound to uri. An
// empty-prefix binding is held, so that a repeat of it declines as any
// repeated prefix does, and is never read: element.LookupPrefix answers ""
// before it consults the scope.
type binding struct {
	prefix, uri string
}

// lookup is the namespace name prefix is bound to in s, the innermost binding
// winning, and false where none binds it.
func (s *scope) lookup(prefix string) (string, bool) {
	for ; s != nil; s = s.parent {
		for _, b := range s.bindings {
			if b.prefix == prefix {
				return b.uri, true
			}
		}
	}
	return "", false
}

// bound is the namespace name the non-empty prefix is bound to in s: "xml"
// everywhere, as Namespaces in XML §3 binds it, and any other prefix through
// lookup. It is what a prefixed key (walker.keyName) and an element's
// prefixed QNames (element.LookupPrefix) resolve through.
func (s *scope) bound(prefix string) (string, bool) {
	if prefix == "xml" {
		return xmlNamespace, true
	}
	return s.lookup(prefix)
}

// cursor is an element's [[children]] as the walk drives them.
type cursor interface {
	validate.Children
	// drain reads the cursor to its end, classifying every member it passes,
	// so the stream stands past the element's value.
	drain()
}

// element is one element information item: a member of a JSON object, or an
// item of a member's array, under the expanded name its key was classified to.
type element struct {
	w *walker
	// name is the element's [namespace name] and [local name]: the matched
	// declaration's expanded name, the one a prefixed key spells
	// (walker.keyName), or the one a wildcard's single enumerated namespace,
	// the namespace of a parent whose type has no wildcard, or, at the root,
	// none gives the key (walker.wildcardMember, walker.rootMember).
	name xsd.QName
	// key is the member's key as spelled, an array item's being its array's,
	// for an object-valued element, which a note on a member of its object
	// names it by (walker.noteChild); empty for a scalar or null one.
	key string
	// loc is where the member's key, or the array item, begins.
	loc xsderr.Loc
	// attrs is the element's [[attributes]], read whole as the element was
	// built: an object value's xsi:type, from its "$type" member, and its
	// leading attribute members (walker.attributes), or a null value's one
	// xsi:nil (walker.nilled).
	attrs []validate.Attribute
	// scope is the "$xmlns" bindings in scope at the element: its own
	// object's, or else its parent's.
	scope *scope
	// kids is the element's [[children]]: a members cursor for an object
	// value, a scalarChildren for a scalar or null one.
	kids cursor
	// finished latches that the parent's cursor moved past this element (or,
	// for the root, that Validate did), so a late Children call cannot read
	// a later element's members as this one's.
	finished bool
}

// Name is the matched declaration's expanded name, the one a prefixed key
// spells, or the one a wildcard's single enumerated namespace gives the key:
// an unprefixed key is a local name, and its namespace is the declaration's or
// the wildcard's (package doc, "The mapping" and its NAMESPACE TRAP).
func (e *element) Name() xsd.QName { return e.name }

// Attributes is the element's xsi:type, where its object carries "$type",
// then its attribute members in member order, the same slice at every call: a
// scalar value has none, and a null value has its one xsi:nil.
func (e *element) Attributes() []validate.Attribute { return e.attrs }

// Children reports the element's cursor. It panics once the walk has moved
// past the element, for the reason xmlsrc's element.Children does: the
// tokens it would read belong to a later element.
func (e *element) Children() validate.Children {
	if e.finished {
		panic(fmt.Sprintf("validate/jsonsrc: Children called after the walk left %s", e.Name()))
	}
	return e.kids
}

// LookupPrefix answers from the bindings a JSON document implies: the empty
// prefix is bound to the element's own namespace name, as its XML twin <k
// xmlns="N"> binds it, whatever a "$xmlns" binds "" to (package doc); "xml"
// is bound as everywhere; and any other prefix answers through the scope
// chain, the innermost "$xmlns" binding it winning (scope.bound).
func (e *element) LookupPrefix(prefix string) (string, bool) {
	if prefix == "" {
		return e.name.Space, true
	}
	return e.scope.bound(prefix)
}

func (e *element) Loc() xsderr.Loc { return e.loc }

// finish moves the stream past the element's value, reading whatever of its
// subtree the engine did not.
func (e *element) finish() {
	e.kids.drain()
	e.finished = true
}

// attribute is an attribute member — the expanded name of the attribute use its
// key matched, or the one the {attribute wildcard}'s single enumerated
// namespace gives it; its scalar's lexical form; and its key's location — a
// "$type" member's xsi:type, located at its key, or a null value's xsi:nil,
// located at the null.
type attribute struct {
	name  xsd.QName
	value string
	loc   xsderr.Loc
}

func (a *attribute) Name() xsd.QName { return a.name }
func (a *attribute) Value() string   { return a.value }
func (a *attribute) Loc() xsderr.Loc { return a.loc }

// text is a scalar member's value as the run of character information items
// its element holds.
type text struct {
	data string
	loc  xsderr.Loc
}

func (t *text) Data() string    { return t.data }
func (t *text) Loc() xsderr.Loc { return t.loc }

// scalarChildren is a scalar-valued element's [[children]]: its one text run,
// or none for the empty string or a null.
type scalarChildren struct {
	text *text
	done bool
}

func (c *scalarChildren) Next() (validate.Child, bool) {
	if c.done || c.text == nil {
		return validate.Child{}, false
	}
	c.done = true
	return validate.TextChild(c.text), true
}

// Err is nil: the scalar was read whole when its element was built.
func (c *scalarChildren) Err() error { return nil }

func (c *scalarChildren) drain() { c.done = true }

// members is a cursor over an object-valued element's [[children]]: one
// element per scalar- or object-valued child member, and one per item of an
// array-valued one, in member order, past the leading attribute members
// walker.attributes read.
type members struct {
	w *walker
	// owner is the element whose members these are, whose scope its children
	// inherit.
	owner *element
	// object is what the owner's members are classified against, read once
	// per element.
	object *object
	// ahead is what walker.attributes read past the leading attribute run, in
	// member order and not yet classified: the member or close it ended the
	// run on, then any member walker.readAhead read after that one. Reads
	// take from it first, and from the stream once it is empty.
	ahead []peeked
	// run is the array-valued member the stream stands inside, and nil
	// outside one.
	run *arrayRun
	// last is the element Next yielded last, finished before the stream
	// moves on: the engine may not have descended into it.
	last *element
	// done latches the owner's closing brace.
	done bool
}

// peeked is a member read ahead of its cursor: a key, its location, its
// value as far as walker.value read it, and, once members.key has classified
// it, its classification as a child element, nil for an array value, whose
// items are classified one by one; or the object's close, with member nil
// and no value.
type peeked struct {
	tok    json.Token
	loc    xsderr.Loc
	member elementMember
	val    value
}

// arrayRun is one array-valued member: its key, which each of its items is an
// element under, classified over each item (walker.item); the key's location,
// loc, and the array's, at; and whether an item has been read.
type arrayRun struct {
	key     string
	loc, at xsderr.Loc
	items   bool
}

func (m *members) Next() (validate.Child, bool) {
	if m.done || m.w.err != nil {
		return validate.Child{}, false
	}
	if m.last != nil {
		m.last.finish()
		m.last = nil
		if m.w.err != nil {
			return validate.Child{}, false
		}
	}
	e, err := m.next()
	if err != nil {
		m.w.fail(err)
		return validate.Child{}, false
	}
	if e == nil {
		m.done = true
		return validate.Child{}, false
	}
	m.last = e
	return validate.ElementChild(e), true
}

// next reads up to the next element of the owner's children, or past its
// closing brace, which it reports as a nil element.
func (m *members) next() (*element, error) {
	for {
		if m.run != nil {
			v, err := m.w.value(m.owner.scope, m.run.key)
			if err != nil {
				return nil, err
			}
			if v.tok == json.Delim(']') {
				if !m.run.items {
					if _, err := m.w.item(m.object, m.run, m.owner.scope, false); err != nil {
						return nil, err
					}
				}
				m.run = nil
				continue
			}
			m.run.items = true
			child, err := m.w.item(m.object, m.run, v.scope, scalarValue(v.tok))
			if err != nil {
				return nil, err
			}
			return m.w.element(child, v.loc, m.run.key, v, m)
		}
		p, err := m.key()
		if err != nil {
			return nil, err
		}
		key, ok := p.tok.(string)
		if !ok {
			// Token yields a key or the object's close here and nothing else.
			return nil, nil
		}
		if p.val.tok == json.Delim('[') {
			m.run = &arrayRun{key: key, loc: p.loc, at: p.val.loc}
			continue
		}
		return m.w.element(p.member, p.loc, key, p.val, m)
	}
}

// key reads the owner's next member, its key classified as a child over its
// value (walker.classify, but for an array value), or its close: first those
// the attribute run left ahead, then from the stream (walker.entry). A key
// classified as an attribute there DECLINES: it follows a child member, and
// the owner's [[attributes]] were read whole before its first child
// (walker.attributes).
//
// GAP(jsonsrc): an attribute member after a child member declines, where its
// XML twin, attribute order being insignificant, carries the attribute. RULED
// permanent by #2625: presenting it would mean holding the owner's members
// until its close, a whole subtree and at the root the whole document, where
// the walk holds none of the document (STYLE P4); the one bounded lift, a
// seekable pre-scan, needs an exported io.ReadSeeker entry point with no
// consumer (STYLE T5) and re-reads each object once per enclosing object.
// Attribute members leading the object is the notation's own ordering rule,
// as the ORDER TRAP is (package doc); the decline names the reorder, since
// encoding/json marshals a struct in field order and so puts an attribute
// field declared after a child field after its child member (#2687).
func (m *members) key() (peeked, error) {
	p, err := m.read()
	if err != nil {
		return peeked{}, err
	}
	key, ok := p.tok.(string)
	if !ok || p.val.tok == json.Delim('[') {
		return p, nil
	}
	mem, err := m.w.classify(m.object, key, p.val.scope, p.loc, scalarValue(p.val.tok))
	if err != nil {
		return peeked{}, err
	}
	child, ok := mem.(elementMember)
	if !ok {
		return peeked{}, m.w.declinef(p.loc, "attribute member %q follows a child member of %s; move attribute members ahead of the first child member, or declare a struct's attribute fields before its child fields, as an element's attributes are read before its children",
			key, m.object.owner)
	}
	p.member = child
	return p, nil
}

// read is the owner's next member, its value read as far as walker.value
// reads it, or its close, unclassified: the first of m.ahead, or else the
// stream's (walker.entry).
func (m *members) read() (peeked, error) {
	if len(m.ahead) > 0 {
		p := m.ahead[0]
		m.ahead = m.ahead[1:]
		return p, nil
	}
	tok, loc, err := m.w.token()
	if err != nil {
		return peeked{}, unexpectedEOF(err, loc)
	}
	key, ok := tok.(string)
	if !ok {
		return peeked{tok: tok, loc: loc}, nil
	}
	v, err := m.w.entry(m.owner.scope, key, loc)
	if err != nil {
		return peeked{}, err
	}
	return peeked{tok: tok, loc: loc, val: v}, nil
}

// Err reports the walk's one latched fault (STYLE D3).
func (m *members) Err() error { return m.w.err }

func (m *members) drain() {
	for {
		if _, ok := m.Next(); !ok {
			return
		}
	}
}
