package jsonsrc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// Option configures [Validate]. The zero set of options is a complete,
// usable configuration: options only replace defaults (STYLE T1).
type Option func(*config)

// config is the resolved [Validate] configuration.
type config struct {
	uri string
}

// newConfig applies opts over the defaults: the document is unnamed, so
// every Loc it produces renders with "?" for its URI.
func newConfig(opts []Option) config {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// WithURI names the document being read, so every [xsderr.Loc] the
// assessment and every error of [Validate] cites identifies the file a reader
// has to open. [Validate] takes an [io.Reader], which carries no name of its
// own.
func WithURI(uri string) Option {
	return func(c *config) { c.uri = uri }
}

// Validate maps the JSON instance in r onto the validate infoset, on the
// mapping the package doc states, and assesses it against v's schema.
//
// It returns (nil, err) whenever no assessment of the whole document stands,
// and (result, nil) otherwise; a result it returns never carries
// [validate.Result.Err]. The faults are: a nil v or r; JSON that is malformed,
// or followed by further content; and every input the mapping DECLINES —
// a top level that is not an object with exactly one member naming a
// top-level element declaration, and, anywhere in the document, a member the
// implemented arms do not map (a key no child element declaration carries, a
// key two expanded names carry, a "$"-prefixed reserved member, a null, a
// nested array, a declaration with a {type table}). A decline names the arm it
// is not, and is never a silent drop or a guessed mapping: a member that would
// map to an attribute, to xsi:nil or to xsi:type is not part of the document
// the assessment saw, so its result is discarded rather than returned. The
// whole document is read either way, so a fault after the point the
// assessment stopped at is still reported.
//
// Every error is a plain one, prefixed "jsonsrc: " and, past the nil checks,
// the location of the construct it names. None is an [xsderr.Error]: none is a
// validity verdict about a document.
func Validate(v *validate.Validator, r io.Reader, opts ...Option) (*validate.Result, error) {
	if v == nil {
		return nil, fmt.Errorf("jsonsrc: Validate: nil *validate.Validator")
	}
	if r == nil {
		return nil, fmt.Errorf("jsonsrc: Validate: nil io.Reader")
	}
	cfg := newConfig(opts)
	w := newWalker(cfg.uri, r, v.Schema())
	root, err := w.root()
	if err != nil {
		return nil, err
	}
	res := v.Assess(root)
	if w.err != nil {
		return nil, w.err
	}
	if err := w.end(root); err != nil {
		return nil, err
	}
	return res, nil
}

// walker is the one token stream a whole mapping pulls from: a single
// json.Decoder over the positions its tokens are located by, the schema view
// members are classified against, and the one fault that can stop it.
type walker struct {
	dec    *json.Decoder
	pos    *positions
	uri    string
	schema validate.SchemaView
	// err is the walk's single latched fault, a malformed document or a
	// decline. Every cursor reports this one field and none keeps a copy of it
	// (STYLE D3).
	err error
}

func newWalker(uri string, r io.Reader, schema validate.SchemaView) *walker {
	pos := newPositions(r)
	dec := json.NewDecoder(pos)
	// A number's lexical form is its source text, never a float64 round trip:
	// 12345678901234567890 under xs:integer is that integer.
	dec.UseNumber()
	return &walker{dec: dec, pos: pos, uri: uri, schema: schema}
}

// token pulls the next token and the location it starts at.
func (w *walker) token() (json.Token, xsderr.Loc, error) {
	off := w.dec.InputOffset()
	tok, err := w.dec.Token()
	line, col := w.pos.start(off)
	loc := xsderr.Loc{URI: w.uri, Line: line, Col: col}
	if errors.Is(err, io.EOF) {
		return nil, loc, err
	}
	if err != nil {
		return nil, loc, fmt.Errorf("jsonsrc: %s: malformed JSON: %w", loc, err)
	}
	return tok, loc, nil
}

// fail latches err as the walk's fault, keeping the first one met.
func (w *walker) fail(err error) {
	if w.err == nil {
		w.err = err
	}
}

// declinef builds the error for an input the mapping declines at loc.
func (w *walker) declinef(loc xsderr.Loc, format string, args ...any) error {
	return fmt.Errorf("jsonsrc: %s: %s", loc, fmt.Sprintf(format, args...))
}

// root reads the top-level object up to the value of its one member and
// returns the element that member names: the validation root.
//
// The root is named by the member's key, matched by local name against the
// top-level element declarations; exactly one must carry it, and its expanded
// name is the root's (package doc, "The validation root").
func (w *walker) root() (*element, error) {
	tok, loc, err := w.token()
	if errors.Is(err, io.EOF) {
		return nil, w.declinef(loc, "the document holds no JSON value")
	}
	if err != nil {
		return nil, err
	}
	if tok != json.Delim('{') {
		return nil, w.declinef(loc, "the top-level value is not an object; the validation root is named by the one member of a top-level object")
	}
	tok, loc, err = w.token()
	if err != nil {
		return nil, unexpectedEOF(err, loc)
	}
	key, ok := tok.(string)
	if !ok {
		return nil, w.declinef(loc, "the top-level object has no member; its one member names the validation root")
	}
	decl, err := w.rootDeclaration(key, loc)
	if err != nil {
		return nil, err
	}
	vtok, vloc, err := w.token()
	if err != nil {
		return nil, unexpectedEOF(err, vloc)
	}
	if vtok == json.Delim('[') {
		return nil, w.declinef(vloc, "the value of root member %q is an array; the validation root is one element", key)
	}
	e, err := w.element(decl, loc, key, vtok, vloc)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// rootDeclaration resolves the root member's key to the one top-level element
// declaration whose local name it is.
func (w *walker) rootDeclaration(key string, loc xsderr.Loc) (xsd.ElementDeclaration, error) {
	if isReserved(key) {
		return xsd.ElementDeclaration{}, w.reserved(key, loc)
	}
	var match []xsd.ElementDeclaration
	for _, d := range w.schema.Elements() {
		if d.Name().Local == key {
			match = append(match, d)
		}
	}
	switch len(match) {
	case 0:
		return xsd.ElementDeclaration{}, w.declinef(loc, "root member %q names no top-level element declaration", key)
	case 1:
		return match[0], w.typeTable(match[0], loc)
	}
	return xsd.ElementDeclaration{}, w.declinef(loc, "root member %q names top-level element declarations of %d expanded names (%s and %s); a JSON key carries no namespace to choose between them",
		key, len(match), match[0].Name(), match[1].Name())
}

// child resolves a member's key to the child element declaration among
// contained, the declarations the enclosing element's type contains, whose
// local name it is. The first declaration of the name stands for every other
// (xsd.Schema.ContainedElementDeclarations says why that is sound).
//
// A key no declaration carries DECLINES whatever a wildcard of the content
// model would admit: the member could equally be an attribute, and a wildcard
// match is the arm this slice does not take.
func (w *walker) child(contained []xsd.ElementDeclaration, owner xsd.QName, key string, loc xsderr.Loc) (xsd.ElementDeclaration, error) {
	if isReserved(key) {
		return xsd.ElementDeclaration{}, w.reserved(key, loc)
	}
	var first xsd.ElementDeclaration
	found := false
	for _, d := range contained {
		if d.Name().Local != key {
			continue
		}
		if !found {
			first, found = d, true
			continue
		}
		if d.Name() != first.Name() {
			return xsd.ElementDeclaration{}, w.declinef(loc, "member %q names child element declarations of two expanded names in the type of %s (%s and %s); a JSON key carries no namespace to choose between them",
				key, owner, first.Name(), d.Name())
		}
	}
	if !found {
		return xsd.ElementDeclaration{}, w.declinef(loc, "member %q names no child element declaration of the type of %s; the attribute and wildcard member arms are not implemented",
			key, owner)
	}
	return first, w.typeTable(first, loc)
}

// typeTable declines a declaration carrying a {type table}: which type governs
// its element turns on the element's attributes (§3.12.4), and this mapping
// presents none, so the content it classifies by would be a guess.
func (w *walker) typeTable(d xsd.ElementDeclaration, loc xsderr.Loc) error {
	if _, ok := d.TypeTable(); ok {
		return w.declinef(loc, "element %s carries a {type table}; conditional type assignment is not implemented", d.Name())
	}
	return nil
}

// isReserved reports whether key is a reserved member name: "$type" and
// "$xmlns" are planned, and every "$"-prefixed name is held for them.
func isReserved(key string) bool { return len(key) > 0 && key[0] == '$' }

func (w *walker) reserved(key string, loc xsderr.Loc) error {
	return w.declinef(loc, "reserved member %q; the \"$type\" and \"$xmlns\" reserved-member arms are not implemented", key)
}

// element builds the element a member (or an item of a member's array) maps
// to, from its declaration, the location of its key or item, the key, and the
// value's first token and its location.
func (w *walker) element(decl xsd.ElementDeclaration, loc xsderr.Loc, key string, tok json.Token, tokLoc xsderr.Loc) (*element, error) {
	switch v := tok.(type) {
	case nil:
		return nil, w.declinef(tokLoc, "member %q is null; the null => xsi:nil arm is not implemented", key)
	case json.Delim:
		if v != json.Delim('{') {
			// '[' alone reaches here: Token never yields a closing delimiter
			// where a value is due.
			return nil, w.declinef(tokLoc, "an array is an item of the array of member %q; only an object or a scalar maps to one element", key)
		}
		contained, err := w.contained(decl, tokLoc)
		if err != nil {
			return nil, err
		}
		return &element{w: w, decl: decl, loc: loc, kids: &members{w: w, owner: decl.Name(), contained: contained}}, nil
	case string:
		return w.scalar(decl, loc, v, tokLoc), nil
	case json.Number:
		return w.scalar(decl, loc, string(v), tokLoc), nil
	case bool:
		if v {
			return w.scalar(decl, loc, "true", tokLoc), nil
		}
		return w.scalar(decl, loc, "false", tokLoc), nil
	}
	panic(fmt.Sprintf("jsonsrc: Token yielded %T", tok))
}

// scalar builds a scalar-valued element: lexical is its simple content. An
// empty lexical is no text at all, as <size/> carries none.
func (w *walker) scalar(decl xsd.ElementDeclaration, loc xsderr.Loc, lexical string, tokLoc xsderr.Loc) *element {
	kids := &scalarChildren{}
	if lexical != "" {
		kids.text = &text{data: lexical, loc: tokLoc}
	}
	return &element{w: w, decl: decl, loc: loc, kids: kids}
}

// contained is the child element declarations an object-valued element of
// decl classifies its members against: those its {type definition}'s content
// model contains. A simple type contains none, so every member of its object
// declines and an empty object is an element with no children.
func (w *walker) contained(decl xsd.ElementDeclaration, loc xsderr.Loc) ([]xsd.ElementDeclaration, error) {
	td, ok := w.schema.ResolvedType(decl.TypeDefinition())
	if !ok {
		return nil, w.declinef(loc, "the {type definition} of %s does not resolve; its members cannot be classified", decl.Name())
	}
	switch t := td.(type) {
	case xsd.ComplexType:
		return w.schema.ContainedElementDeclarations(t), nil
	case *xsd.SimpleType:
		return nil, nil
	}
	panic(fmt.Sprintf("jsonsrc: TypeDefinition arm %T", td))
}

// end reads what follows the root's value once the assessment returns: the
// rest of the root's subtree the assessment did not read, classified as the
// walk would have, then the top-level object's close and the end of the input.
func (w *walker) end(root *element) error {
	root.finish()
	if w.err != nil {
		return w.err
	}
	tok, loc, err := w.token()
	if err != nil {
		return unexpectedEOF(err, loc)
	}
	if key, ok := tok.(string); ok {
		return w.declinef(loc, "the top-level object has a second member %q; its one member names the validation root", key)
	}
	_, loc, err = w.token()
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return w.declinef(loc, "content follows the top-level object")
}

// unexpectedEOF turns the io.EOF token reports at the end of the input into
// the malformed-JSON error it is anywhere a value or a close is still due,
// and passes any other error through.
func unexpectedEOF(err error, loc xsderr.Loc) error {
	if errors.Is(err, io.EOF) {
		return fmt.Errorf("jsonsrc: %s: malformed JSON: %w", loc, io.ErrUnexpectedEOF)
	}
	return err
}
