package jsonsrc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"

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
// implemented arms do not map (a key no child element declaration and no
// attribute use carries, a key two expanded names carry, a key both a child
// declaration and an attribute use carry, an attribute member after a child
// member or repeated, an attribute member whose value is null, an object or an
// array, a "$"-prefixed reserved member, a nested array, a declaration with a
// {type table}; the package doc lists them all). A decline names the arm it is
// not, and is never a silent drop or a guessed mapping: a member that would
// map to a wildcard's element or attribute or to xsi:type is not part of the
// document the assessment saw, so its result is discarded rather than
// returned. The whole document is read either way, so a fault after the point
// the assessment stopped at is still reported.
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

// object is what an object-valued element's members are classified against,
// read once per element from its declaration's {type definition}: the owner's
// expanded name, the child element declarations the type contains, the type's
// {attribute uses}, and whether its {content type} is simple. A simple type
// contains none and has none.
type object struct {
	owner     xsd.QName
	contained []xsd.ElementDeclaration
	uses      []xsd.AttributeUse
	// simpleContent is the type's {content type} being simple: the mapping
	// spells no text beside attribute members, so it declines one there.
	simpleContent bool
}

// member is one key's classification: an attribute use of the enclosing type
// where use is non-nil, and otherwise the child element declaration decl.
type member struct {
	decl xsd.ElementDeclaration
	use  *xsd.AttributeUse
}

// classify resolves a member's key, at loc, against o: to the child element
// declaration among o.contained whose local name it is, or to the attribute
// use among o.uses whose {attribute declaration}'s local name it is
// (cvc-complex-type clause 2.1 then matches the attribute to that use by
// expanded name). The first declaration of the name stands for every other
// (xsd.Schema.ContainedElementDeclarations says why that is sound).
//
// It DECLINES a reserved key; a key two expanded names carry among the
// declarations, or among the uses; a key both a declaration and a use carry,
// which no spec rule decides between; a use in the xsi namespace, xsi:nil
// being the null arm's to present and xsi:type the reserved-member arm's; an
// attribute use of a type with simple content, beside which the mapping spells
// no text; and a key neither carries, whatever an {attribute wildcard} or an
// element wildcard of the type would admit, a wildcard match being the arm this
// slice does not take.
//
// GAP(jsonsrc): a key no child element declaration and no attribute use
// carries declines, the wildcard member arm not being implemented. That covers
// a key an element wildcard of the type admits (cvc-accept), and a key its
// {attribute wildcard} admits, whose attribute cvc-complex-type clause 2.2
// would match to that wildcard.
//
// GAP(jsonsrc): a key both a child element declaration and an attribute use
// carry declines. RULED permanent by #2613: no spec rule decides between them,
// and the mapping has no warning channel to report a choice through.
//
// The xsi-namespace, two-expanded-name and simple-content declines among the
// uses are useNamed's, under its GAP(jsonsrc) markers.
func (w *walker) classify(o *object, key string, loc xsderr.Loc) (member, error) {
	if isReserved(key) {
		return member{}, w.reserved(key, loc)
	}
	decl, declared, err := w.childNamed(o, key, loc)
	if err != nil {
		return member{}, err
	}
	use, used, err := w.useNamed(o, key, loc)
	if err != nil {
		return member{}, err
	}
	if declared && used {
		return member{}, w.declinef(loc, "member %q names both the child element declaration %s and the attribute use %s of the type of %s; the mapping does not choose between them",
			key, decl.Name(), use.DeclarationName(), o.owner)
	}
	if used {
		return member{use: &use}, nil
	}
	if !declared {
		return member{}, w.declinef(loc, "member %q names no child element declaration and no attribute use of the type of %s; the wildcard member arm is not implemented",
			key, o.owner)
	}
	return member{decl: decl}, w.typeTable(decl, loc)
}

// childNamed is the first of o.contained whose local name is key, declining
// a key two expanded names carry.
func (w *walker) childNamed(o *object, key string, loc xsderr.Loc) (xsd.ElementDeclaration, bool, error) {
	var first xsd.ElementDeclaration
	found := false
	for _, d := range o.contained {
		if d.Name().Local != key {
			continue
		}
		if !found {
			first, found = d, true
			continue
		}
		if d.Name() != first.Name() {
			return xsd.ElementDeclaration{}, false, w.declinef(loc, "member %q names child element declarations of two expanded names in the type of %s (%s and %s); a JSON key carries no namespace to choose between them",
				key, o.owner, first.Name(), d.Name())
		}
	}
	return first, found, nil
}

// useNamed is the member of o.uses whose {attribute declaration}'s local name
// is key. It declines a key uses of two expanded names carry, a use in the xsi
// namespace, and a use of a type with simple content.
//
// GAP(jsonsrc): a key attribute uses of two expanded names carry declines:
// cvc-complex-type clause 2.1 matches an attribute to a use by expanded name,
// and a JSON key carries no namespace to choose between them.
//
// GAP(jsonsrc): a key naming an xsi:type attribute use declines: it is the
// "$type" arm's to present, owned by #2615.
//
// GAP(jsonsrc): a key naming an xsi:schemaLocation or
// xsi:noNamespaceSchemaLocation attribute use declines, owned by #2627, which
// rules it a permanent decline or gives it a key spelling.
//
// GAP(jsonsrc): a key naming an xsi:nil attribute use declines. RULED
// permanent by #2614: a null value is xsi:nil's one spelling (walker.nilled),
// and a second would let one object present it twice.
//
// GAP(jsonsrc): an attribute use of a type whose {content type} is simple
// declines. RULED permanent by #2613: the XML twin, simple content beside
// attributes, is valid under cvc-complex-type clauses 1.2 and 2, and the
// mapping spells no text beside attribute members.
func (w *walker) useNamed(o *object, key string, loc xsderr.Loc) (xsd.AttributeUse, bool, error) {
	var first xsd.AttributeUse
	found := false
	for _, u := range o.uses {
		if u.DeclarationName().Local != key {
			continue
		}
		if !found {
			first, found = u, true
			continue
		}
		if u.DeclarationName() != first.DeclarationName() {
			return xsd.AttributeUse{}, false, w.declinef(loc, "member %q names attribute uses of two expanded names in the type of %s (%s and %s); a JSON key carries no namespace to choose between them",
				key, o.owner, first.DeclarationName(), u.DeclarationName())
		}
	}
	if !found {
		return xsd.AttributeUse{}, false, nil
	}
	if first.DeclarationName().Space == xsd.XMLSchemaInstanceNS {
		return xsd.AttributeUse{}, false, w.declinef(loc, "member %q names the attribute use %s of the type of %s; an xsi: attribute is presented only by the null => xsi:nil arm and the \"$type\" arm, which is not implemented",
			key, first.DeclarationName(), o.owner)
	}
	if o.simpleContent {
		return xsd.AttributeUse{}, false, w.declinef(loc, "member %q names an attribute use of the type of %s, whose {content type} is simple; the mapping spells no text beside attribute members",
			key, o.owner)
	}
	return first, true, nil
}

// typeTable declines a declaration carrying a {type table}: which type governs
// its element turns on the element's attributes (§3.12.4), xsi: ones included,
// and this mapping does not present those, so the content it classifies by
// would be a guess.
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
		return w.nilled(decl, loc, tokLoc), nil
	case json.Delim:
		if v != json.Delim('{') {
			// '[' alone reaches here: Token never yields a closing delimiter
			// where a value is due.
			return nil, w.declinef(tokLoc, "an array is an item of the array of member %q; only an object or a scalar maps to one element", key)
		}
		o, err := w.object(decl, tokLoc)
		if err != nil {
			return nil, err
		}
		attrs, head, err := w.attributes(o)
		if err != nil {
			return nil, err
		}
		return &element{w: w, decl: decl, loc: loc, attrs: attrs, kids: &members{w: w, object: o, head: &head}}, nil
	}
	lexical, ok := scalarLexical(tok)
	if !ok {
		panic(fmt.Sprintf("jsonsrc: Token yielded %T", tok))
	}
	return w.scalar(decl, loc, lexical, tokLoc), nil
}

// scalarLexical is the lexical form of a scalar token — a string's decoded
// characters; a number's source text, never a float64 round trip; true or
// false — and false for a token that is no scalar: null or a delimiter.
func scalarLexical(tok json.Token) (string, bool) {
	switch v := tok.(type) {
	case string:
		return v, true
	case json.Number:
		return string(v), true
	case bool:
		if v {
			return "true", true
		}
		return "false", true
	}
	return "", false
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

// nilled builds a null-valued element: one attribute, xsi:nil with the
// lexical form true, located at the null token tokLoc, and no [[children]]
// (package doc, "The mapping"). It reads no property of decl: whether the
// element is ·nilled· (key-nilled) and whether cvc-elt clause 3.1 or 3.2.3.2
// charges it are the engine's, as for the XML twin <e xsi:nil="true"/>. Where
// it is ·nilled·, the xsi:nil keeps it from cvc-elt clause 5.1's {value
// constraint}, which an empty element without one would take.
func (w *walker) nilled(decl xsd.ElementDeclaration, loc, tokLoc xsderr.Loc) *element {
	attrs := []validate.Attribute{&attribute{name: xsiNil, value: "true", loc: tokLoc}}
	return &element{w: w, decl: decl, loc: loc, attrs: attrs, kids: &scalarChildren{}}
}

// object is what an object-valued element of decl classifies its members
// against, from its {type definition}: a complex type's contained declarations
// and {attribute uses}; a simple type has neither, so every member of its
// object declines and an empty object is an element with no children.
func (w *walker) object(decl xsd.ElementDeclaration, loc xsderr.Loc) (*object, error) {
	td, ok := w.schema.ResolvedType(decl.TypeDefinition())
	if !ok {
		return nil, w.declinef(loc, "the {type definition} of %s does not resolve; its members cannot be classified", decl.Name())
	}
	switch t := td.(type) {
	case xsd.ComplexType:
		_, simple := t.ContentType().(xsd.SimpleContent)
		return &object{owner: decl.Name(), contained: w.schema.ContainedElementDeclarations(t), uses: t.AttributeUses(), simpleContent: simple}, nil
	case *xsd.SimpleType:
		return &object{owner: decl.Name()}, nil
	}
	panic(fmt.Sprintf("jsonsrc: TypeDefinition arm %T", td))
}

// attributes reads the leading members of o's object that name attribute uses
// (classify), each an attribute of the element, and returns them with the
// token that ends the run — the first key naming a child, or the object's
// close — for the element's members cursor to start from.
//
// The engine reads an element's [[attributes]] at its entry, before its
// [[children]], and again after them, so they must be whole and unchanged
// from the first read. Reading the leading run as the element is built makes
// them so without holding any of the document (STYLE P4); an attribute member
// after a child member therefore declines, under members.key's GAP(jsonsrc)
// marker.
//
// Each attribute is named by its use's {attribute declaration}'s expanded name
// and located at its key. Its [[normalized value]] is its scalar's lexical
// form (scalarLexical) unchanged, and the empty string is an attribute whose
// value is empty. XML 1.0 §3.3.3 maps a literal #x9, #xA or #xD to #x20 and
// keeps one a character reference names; a JSON string holds none literally
// (RFC 8259 §7 requires a control character to be escaped), and its escape is
// read as the character reference would be. It declines a null, object or
// array value, and a key repeated in the run: an element carries one attribute
// of each expanded name (XML 1.0 §3.1, Unique Att Spec).
func (w *walker) attributes(o *object) ([]validate.Attribute, peeked, error) {
	var attrs []validate.Attribute
	for {
		tok, loc, err := w.token()
		if err != nil {
			return nil, peeked{}, unexpectedEOF(err, loc)
		}
		key, ok := tok.(string)
		if !ok {
			return attrs, peeked{tok: tok, loc: loc}, nil
		}
		m, err := w.classify(o, key, loc)
		if err != nil {
			return nil, peeked{}, err
		}
		if m.use == nil {
			return attrs, peeked{tok: tok, loc: loc, member: m}, nil
		}
		name := m.use.DeclarationName()
		if slices.ContainsFunc(attrs, func(a validate.Attribute) bool { return a.Name() == name }) {
			return nil, peeked{}, w.declinef(loc, "attribute member %q repeats in the object of %s; an element carries one attribute of each expanded name", key, o.owner)
		}
		vtok, vloc, err := w.token()
		if err != nil {
			return nil, peeked{}, unexpectedEOF(err, vloc)
		}
		lexical, ok := scalarLexical(vtok)
		if !ok {
			return nil, peeked{}, w.declinef(vloc, "attribute member %q of %s is not a string, number or boolean; an attribute's value is a scalar", key, o.owner)
		}
		attrs = append(attrs, &attribute{name: name, value: lexical, loc: loc})
	}
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
