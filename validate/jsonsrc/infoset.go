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
	_ validate.Element  = (*element)(nil)
	_ validate.Children = (*members)(nil)
	_ validate.Children = (*scalarChildren)(nil)
	_ validate.Text     = (*text)(nil)
)

// xmlNamespace is the namespace name the prefix "xml" is bound to in every
// element's [in-scope namespaces] (Namespaces in XML §3).
const xmlNamespace = "http://www.w3.org/XML/1998/namespace"

// cursor is an element's [[children]] as the walk drives them.
type cursor interface {
	validate.Children
	// drain reads the cursor to its end, classifying every member it passes,
	// so the stream stands past the element's value.
	drain()
}

// element is one element information item: a member of a JSON object, or an
// item of a member's array, under the declaration its key matched.
type element struct {
	w    *walker
	decl xsd.ElementDeclaration
	// loc is where the member's key, or the array item, begins.
	loc xsderr.Loc
	// kids is the element's [[children]]: a members cursor for an object
	// value, a scalarChildren for a scalar one.
	kids cursor
	// finished latches that the parent's cursor moved past this element (or,
	// for the root, that Validate did), so a late Children call cannot read
	// a later element's members as this one's.
	finished bool
}

// Name is the matched declaration's expanded name: a JSON key is a local
// name, and its namespace is the declaration's (package doc, "The namespace
// trap").
func (e *element) Name() xsd.QName { return e.decl.Name() }

// Attributes is empty: this slice maps no member to an attribute.
func (e *element) Attributes() []validate.Attribute { return nil }

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
// prefix is bound to the element's own namespace name, as the matched
// declaration's target namespace is its default namespace (package doc),
// "xml" is bound as everywhere, and no other prefix is bound.
func (e *element) LookupPrefix(prefix string) (string, bool) {
	switch prefix {
	case "":
		return e.decl.Name().Space, true
	case "xml":
		return xmlNamespace, true
	}
	return "", false
}

func (e *element) Loc() xsderr.Loc { return e.loc }

// finish moves the stream past the element's value, reading whatever of its
// subtree the engine did not.
func (e *element) finish() {
	e.kids.drain()
	e.finished = true
}

// text is a scalar member's value as the run of character information items
// its element holds.
type text struct {
	data string
	loc  xsderr.Loc
}

func (t *text) Data() string    { return t.data }
func (t *text) Loc() xsderr.Loc { return t.loc }

// scalarChildren is a scalar-valued element's [[children]]: its one text run,
// or none for the empty string.
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
// element per scalar- or object-valued member, and one per item of an
// array-valued member, in member order.
type members struct {
	w     *walker
	owner xsd.QName
	// contained is what the owner's type contains, read once per element.
	contained []xsd.ElementDeclaration
	// run is the array-valued member the stream stands inside, and nil
	// outside one.
	run *arrayRun
	// last is the element Next yielded last, finished before the stream
	// moves on: the engine may not have descended into it.
	last *element
	// done latches the owner's closing brace.
	done bool
}

// arrayRun is one array-valued member: the declaration its key matched, and
// the key, which each of its items is an element under.
type arrayRun struct {
	decl xsd.ElementDeclaration
	key  string
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
			tok, loc, err := m.w.token()
			if err != nil {
				return nil, unexpectedEOF(err, loc)
			}
			if tok == json.Delim(']') {
				m.run = nil
				continue
			}
			return m.w.element(m.run.decl, loc, m.run.key, tok, loc)
		}
		tok, loc, err := m.w.token()
		if err != nil {
			return nil, unexpectedEOF(err, loc)
		}
		key, ok := tok.(string)
		if !ok {
			// Token yields a key or the object's close here and nothing else.
			return nil, nil
		}
		decl, err := m.w.child(m.contained, m.owner, key, loc)
		if err != nil {
			return nil, err
		}
		vtok, vloc, err := m.w.token()
		if err != nil {
			return nil, unexpectedEOF(err, vloc)
		}
		if vtok == json.Delim('[') {
			m.run = &arrayRun{decl: decl, key: key}
			continue
		}
		return m.w.element(decl, loc, key, vtok, vloc)
	}
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
