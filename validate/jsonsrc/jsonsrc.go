package jsonsrc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/kud360/goxsd8/internal/xmlname"
	"github.com/kud360/goxsd8/internal/xmlqname"
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
// [validate.Result.Err]. Its faults are the four error classes listed below.
// The inputs the mapping DECLINES are a top level that is not an object with
// exactly one member, and, anywhere in the document, a member the implemented
// arms do not map (a key whose colon makes no QName or whose prefix no
// "$xmlns" in scope binds; a key no child element declaration and no attribute
// use carries whose namespace neither a prefix nor the type's wildcards fix,
// or whose kind a type with both an element wildcard and an {attribute
// wildcard} leaves open, or which a wildcard puts in the xsi or xml namespace,
// or a prefix in the xsi namespace or, as a child element, in the xml
// namespace; a member of an object under a child no contained and no top-level
// declaration names; an unprefixed key two expanded names carry, a key both a
// child declaration and an attribute use carry, an attribute member after a
// child member or repeated, an attribute member whose value is null, an object
// or an array, a "$type" or "$xmlns" member out of its leading place or of the
// wrong shape, a "$type" whose parent's ·governing type definition· the
// mapping does not determine or that the types a {type table} below the root
// can select for it settle differently, any other "$"-prefixed reserved
// member, a nested array, a member of an element below the root whose
// declaration carries a {type table} that its declared {type definition} and a
// type the table can select classify differently, and, at such a root, the
// leading attribute members no run settles by the type the table
// ·conditionally selects· for them; the package doc lists them all). A decline
// names the arm it is not, and for an input with no XML twin the spelling the
// mapping takes instead where one exists; it is never a silent drop or a
// guessed mapping: a member the mapping cannot name is not part of the
// document the assessment saw, so its result is discarded rather than
// returned. The whole document is read either way, so a fault after the point
// the assessment stopped at is still reported.
//
// An attribute member after a child member is the order encoding/json
// marshals a struct whose child field is declared before an attribute field:
// declare a struct's attribute fields first, or the document declines.
//
// Every error it returns is of one of four classes. Only a decline has a type
// of its own; the other three are plain errors, by design, and no sentinel
// value marks any of them:
//
//   - A nil argument: "jsonsrc: Validate: nil *validate.Validator" or
//     "jsonsrc: Validate: nil io.Reader". It wraps nothing.
//   - Malformed JSON: an input that is not exactly one JSON value (RFC 8259
//     §2), spelled "malformed JSON: …", "the document holds no JSON value; …"
//     or "content follows the top-level object". The first may wrap an error,
//     whose identity is not part of this contract; the other two wrap
//     nothing.
//   - A fault reading r: "fault reading the input: …", wrapping the error r
//     returned, which [errors.Is] reaches. Because r may return any error,
//     io.ErrUnexpectedEOF among them, no matchable value separates this class
//     from malformed JSON: a caller that needs the split checks whether its
//     own r returned an error.
//   - A decline: a [*DeclineError], matched with [errors.As]. It wraps
//     nothing.
//
// Every error is prefixed "jsonsrc: " and, past the nil checks, the location of
// the construct it names. None is an [xsderr.Error]: none is a validity verdict
// about a document.
//
// Read the verdict off a returned result as the "Contract" section of package
// [validate] says, but for the one difference from an xmlsrc result stated
// above: every fault comes back as err, and [validate.Result.Err] is nil. The
// rest of that caveat applies unchanged: an empty [validate.Result.Violations]
// with an empty [validate.Result.Unevaluated] means not proven invalid, not
// valid, and a non-empty Unevaluated leaves the document undecided against the
// rules its entries name.
func Validate(v *validate.Validator, r io.Reader, opts ...Option) (*validate.Result, error) {
	if v == nil {
		return nil, fmt.Errorf("jsonsrc: Validate: nil *validate.Validator")
	}
	if r == nil {
		return nil, fmt.Errorf("jsonsrc: Validate: nil io.Reader")
	}
	cfg := newConfig(opts)
	w := newWalker(cfg.uri, r, v.Schema(), v.ConditionallySelected)
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

// DeclineError is the error [Validate] returns for an input the mapping
// declines: one the implemented arms do not map, as the package doc's
// "Declines" section lists. Validate is what builds one, and its doc states
// what a decline wraps and the other error classes it returns.
type DeclineError struct {
	loc xsderr.Loc
	msg string
}

// Error renders the decline as "jsonsrc: <loc>: <message>", the message naming
// the arm the input is not.
func (e *DeclineError) Error() string {
	return fmt.Sprintf("jsonsrc: %s: %s", e.loc, e.msg)
}

// Loc is the location of the construct the decline names, as the package doc's
// "Locations" section places it, under the URI [WithURI] gives: empty, and
// rendered "?", without one.
func (e *DeclineError) Loc() xsderr.Loc {
	return e.loc
}

// walker is the one token stream a whole mapping pulls from: a single
// json.Decoder over the positions its tokens are located by, the schema view
// members are classified against, the selection a {type table} makes, and the
// one fault that can stop it.
type walker struct {
	dec    *json.Decoder
	pos    *positions
	uri    string
	schema validate.SchemaView
	// selected is validate.Validator.ConditionallySelected, the engine's own
	// key-cta-select scan, which walker.attributes reads a root's {type table}
	// through.
	selected func(table xsd.TypeTable, attributes, inherited []validate.Attribute) (xsd.TypeDefinitionOrRef, bool)
	// err is the walk's single latched fault, a malformed document, a fault
	// reading the input or a decline. Every cursor reports this one field
	// and none keeps a copy of it (STYLE D3).
	err error
}

func newWalker(uri string, r io.Reader, schema validate.SchemaView, selected func(xsd.TypeTable, []validate.Attribute, []validate.Attribute) (xsd.TypeDefinitionOrRef, bool)) *walker {
	pos := newPositions(r)
	dec := json.NewDecoder(pos)
	// A number's lexical form is its source text, never a float64 round trip:
	// 12345678901234567890 under xs:integer is that integer.
	dec.UseNumber()
	return &walker{dec: dec, pos: pos, uri: uri, schema: schema, selected: selected}
}

// token pulls the next token and the location it starts at.
//
// A token error is a fault reading the input when it is the error the
// reader itself returned (positions.fault), and malformed JSON otherwise:
// the reader's error decides, never the error's type, because the decoder
// reports a document cut off mid-token as io.ErrUnexpectedEOF, no
// *json.SyntaxError, and a reader can return any error at all (#2655).
func (w *walker) token() (json.Token, xsderr.Loc, error) {
	off := w.dec.InputOffset()
	tok, err := w.dec.Token()
	line, col := w.pos.start(off)
	loc := xsderr.Loc{URI: w.uri, Line: line, Col: col}
	if errors.Is(err, io.EOF) {
		return nil, loc, err
	}
	if w.pos.fault != nil && errors.Is(err, w.pos.fault) {
		return nil, loc, fmt.Errorf("jsonsrc: %s: fault reading the input: %w", loc, err)
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

// declinef builds the [*DeclineError] for an input the mapping declines at loc.
func (w *walker) declinef(loc xsderr.Loc, format string, args ...any) error {
	return &DeclineError{loc: loc, msg: fmt.Sprintf(format, args...)}
}

// root reads the top-level object up to the value of its one member and
// returns the element that member names: the validation root.
//
// The root is named by the member's key, read through its own "$xmlns" once
// walker.value has read it (package doc, "The validation root"). The key's
// declines are rootMember's, under its GAP(jsonsrc) markers.
//
// It DECLINES a top-level value that is not an object, a top-level object with
// no member, a reserved root key, and a root member whose value is an array.
// None has an XML twin, so none carries a marker: an XML document has exactly
// one document element (XML 1.0 §2.1 [1]), and each of these spells none or
// several, or a name no XML Name is. An input holding no JSON value is no
// decline but malformed, JSON text being one value (RFC 8259 §2), and its
// error is a plain one.
func (w *walker) root() (*element, error) {
	tok, loc, err := w.token()
	if errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("jsonsrc: %s: the document holds no JSON value; a JSON instance is one object whose one member names the validation root, {\"<root>\":...}", loc)
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
	if key == memberType {
		return nil, w.declinef(loc, "root member %q is a reserved member; the top-level object's one member names the validation root, and neither \"$xmlns\" nor \"$type\" is a member of the top-level object: move it into the root element's own object, after any \"$xmlns\" and before every other member, {\"<root>\":{\"$xmlns\":...,%q:...}}", key, key)
	}
	if key == memberXmlns {
		return nil, w.declinef(loc, "root member %q is a reserved member; the top-level object's one member names the validation root, and neither \"$xmlns\" nor \"$type\" is a member of the top-level object: move it into the root element's own object, which it leads, {\"<root>\":{%q:...}}", key, key)
	}
	if isReserved(key) {
		return nil, w.declinef(loc, "root member %q is a reserved member; the top-level object's one member names the validation root, keyed by the root element's own name, which takes no \"$\"", key)
	}
	v, err := w.value(nil, key)
	if err != nil {
		return nil, err
	}
	if v.tok == json.Delim('[') {
		return nil, w.declinef(v.loc, "the value of root member %q is an array; the validation root is one element, so give it an object, a scalar or null", key)
	}
	m, err := w.rootMember(key, v.scope, v.tok != json.Delim('{'), loc)
	if err != nil {
		return nil, err
	}
	e, err := w.element(m, loc, key, v, nil)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// rootMember names the validation root by the root member's key, read through
// sc, the scope its own "$xmlns" makes (walker.keyName), scalar when its value
// is a scalar or null and so carries no "$xmlns": the top-level element
// declaration of the expanded name a prefixed key spells, or the one whose
// local name an unprefixed key is, where one carries it (declaredElement);
// and otherwise the element that name — the absent namespace's, for an
// unprefixed key — with no declaration (bareElement). The mapping does not
// test admission: an undeclared root is the engine's to assess, laxly or under
// its xsi:type (cvc-assess-elt clauses 1.2 and 3, §5.2).
//
// It DECLINES a key keyName declines and an unprefixed key top-level
// declarations of two expanded names carry. A reserved key, which walker.root
// declined before reading the value, has no XML twin and no marker: "$xmlns"
// and "$type" lead an element's object and do not name one, and any other "$"
// name is no XML Name (XML 1.0 §2.3 [4]).
//
// GAP(jsonsrc): an unprefixed root key top-level element declarations of two
// expanded names carry declines. RULED permanent by #2623: an unprefixed key
// names a declaration by its local name alone, two carry it, and the root's
// verdict turns on which (cvc-elt clause 1, cvc-assess-elt); a prefixed key
// "p:local" spells each one in a namespace, and one in no namespace beside one
// in a namespace is spelled by no key, since no prefix is bound to the absent
// namespace (Namespaces in XML §5, NSC: No Prefix Undeclaring; walker.xmlns),
// which the mapping declines rather than give the unprefixed key either
// meaning.
func (w *walker) rootMember(key string, sc *scope, scalar bool, loc xsderr.Loc) (elementMember, error) {
	k, err := w.keyName(key, sc, scalar, loc)
	if err != nil {
		return nil, err
	}
	var match []xsd.ElementDeclaration
	for _, d := range w.schema.Elements() {
		if k.carries(d.Name()) {
			match = append(match, d)
		}
	}
	switch len(match) {
	case 0:
		return bareElement{name: k.name}, nil
	case 1:
		return declaredElement{decl: match[0]}, nil
	}
	if !scalar {
		return nil, w.declinef(loc, "root member %q names top-level element declarations of %d expanded names (%s and %s); an unprefixed key does not choose between them, and a prefixed one, \"p:%s\", names one",
			key, len(match), match[0].Name(), match[1].Name(), key)
	}
	inXML := slices.IndexFunc(match, func(d xsd.ElementDeclaration) bool { return d.Name().Space == xmlNamespace })
	if inXML < 0 {
		return nil, w.declinef(loc, "root member %q names top-level element declarations of %d expanded names (%s and %s); an unprefixed key does not choose between them, and a root whose value is a scalar or null has no object for a \"$xmlns\" member to lead, so \"xml\" is the one prefix bound there and no key names any of them with this value",
			key, len(match), match[0].Name(), match[1].Name())
	}
	return nil, w.declinef(loc, "root member %q names top-level element declarations of %d expanded names (%s and %s); an unprefixed key does not choose between them, and a root whose value is a scalar or null has no object for a \"$xmlns\" member to lead, so \"xml\" is the one prefix bound there: key it \"xml:%s\" for %s, and no key names any other of them with this value",
		key, len(match), match[0].Name(), match[1].Name(), key, match[inXML].Name())
}

// keyName is a member's key as classification reads it: name.Local, the local
// name, and, for a prefixed key "p:local", name.Space, the namespace name p's
// binding gives it (prefixed). An unprefixed key's name.Space is empty and
// unread: such a key names a declaration or use by its local name alone.
type keyName struct {
	name     xsd.QName
	prefixed bool
}

// carries reports whether a declaration or use of the expanded name n carries
// the key: n itself for a prefixed key, any n of its local name for an
// unprefixed one.
func (k keyName) carries(n xsd.QName) bool {
	if k.prefixed {
		return n == k.name
	}
	return n.Local == k.name.Local
}

// keyName reads key, at loc, through sc, the bindings in scope at the element
// it names — its own object's "$xmlns" over its parent's — or, for an
// attribute, at the element it is an attribute of (package doc, "The
// mapping"). A key with no colon is unprefixed. A key "p:local" is resolved
// by [xmlqname.Resolve] through sc and the "xml" binding every element has;
// the empty prefix is never consulted, so neither a "$xmlns" binding of "" nor
// any default namespace reaches a key (Namespaces in XML §6.2).
//
// It DECLINES a key with a colon whose prefix or local part is empty or no
// NCName, a second colon among them, and a prefix sc does not bind, "xmlns"
// included. None has an XML twin, so no marker tracks them: such a name is not
// namespace-well-formed (Namespaces in XML §7), and an xmlns:p attribute is a
// namespace declaration, which a "$xmlns" member spells. The unbound-prefix
// decline names the "$xmlns" member that would bind the prefix, except under
// scalarRoot, a root whose value is a scalar or null: such a root has no
// object of its own and no ancestor, so no "$xmlns" scopes it and "xml" is the
// one prefix bound there; the decline names the "xml:" key and the unprefixed
// key (rootMember) instead.
func (w *walker) keyName(key string, sc *scope, scalarRoot bool, loc xsderr.Loc) (keyName, error) {
	prefix, local, prefixed := strings.Cut(key, ":")
	if !prefixed {
		return keyName{name: xsd.QName{Local: key}}, nil
	}
	if !xmlname.IsNCName(prefix) || !xmlname.IsNCName(local) {
		return keyName{}, w.declinef(loc, "member %q is no QName; a key with a colon is a prefix and a local name, each an NCName", key)
	}
	space, local, ok := xmlqname.Resolve(key, sc.bound)
	if !ok && prefix == "xmlns" {
		return keyName{}, w.declinef(loc, "member %q: no \"$xmlns\" in scope binds its prefix %q; a namespace declaration is a \"$xmlns\" member, {%q:\"<namespace name>\"}, leading the object of the element it scopes, and never an xmlns: key", key, prefix, key[len(prefix)+1:])
	}
	if !ok && scalarRoot {
		return keyName{}, w.declinef(loc, "member %q: no \"$xmlns\" in scope binds its prefix %q, and a root whose value is a scalar or null has no object for a \"$xmlns\" member to lead, so \"xml\" is the one prefix bound there; key it {\"xml:%s\":...} for the element of that local name in the namespace %s, or by its local name alone, {%q:...}, which names the top-level element declaration of that local name where exactly one carries it", key, prefix, key[len(prefix)+1:], xmlNamespace, key[len(prefix)+1:])
	}
	if !ok {
		return keyName{}, w.declinef(loc, "member %q: no \"$xmlns\" in scope binds its prefix %q; a prefixed key names the expanded name its prefix's binding gives it, so bind %q in a \"$xmlns\" member, {%q:\"<namespace name>\"}, leading the object of the element the key names or is an attribute of, or of an ancestor", key, prefix, prefix, prefix)
	}
	return keyName{name: xsd.QName{Space: space, Local: local}, prefixed: true}, nil
}

// object is what an object-valued element's members are classified against,
// read once per element from the type declaredElement.object chose — its
// declaration's {type definition}, or, under a "$type", the type
// walker.instanceGoverned settles — or, at a root whose declaration carries a
// {type table}, from the type walker.settled or walker.take reads the table
// to select: the owner's expanded name, the child element declarations the
// type contains, the type's {attribute uses}, its element wildcards and
// {attribute wildcard}, whether its {content type} is simple, the objects of
// the types the declaration's {type table} can select, and the type that
// governs the owner, with the matcher its children are ·attributed· by. A
// simple type contains none and has none.
type object struct {
	owner     xsd.QName
	contained []xsd.ElementDeclaration
	uses      []xsd.AttributeUse
	// elementWildcards is every Wildcard {term} the type's content model
	// contains (xsd.Schema.ContainedWildcards), whatever its {process
	// contents}, then its {open content} wildcard where it has one.
	elementWildcards []xsd.Wildcard
	// attributeWildcard is the type's {attribute wildcard}, nil where it has
	// none.
	attributeWildcard *xsd.Wildcard
	// simpleContent is the type's {content type} being simple: the mapping
	// spells no text beside attribute members, so it declines one there.
	simpleContent bool
	// selectable is the object of each type the owner's declaration's {type
	// table} can select — each alternative's {type definition}, then the
	// {default type definition}'s — where the members are classified under the
	// declaration's {type definition} (declaredElement.object), and nil
	// otherwise: each is a candidate for the root's attribute run
	// (walker.attributes), and below the root a key maps only where each
	// classifies it as the declaration's type does (walker.classify).
	selectable []*object
	// undeclared is the owner having no declaration: a child wildcardMember
	// names whose expanded name no top-level declaration carries (bareElement),
	// which has no type to classify a member against.
	undeclared bool
	// governing is each type that may be the owner's ·governing type
	// definition·, as the engine determines it from the owner's ·attribution·
	// (walker.governing) or its "$type" (walker.instanceGoverned), which need
	// not be the type the members are classified under: one where the mapping
	// determines it, the type a root's {type table} selects included
	// (walker.settled, walker.take); each type a {type table} can select where
	// the declaration that governs the owner below the root, or one an
	// ancestor's, carries one whose types differ (walker.declarationType), the
	// engine selecting among them by a {test} the mapping does not evaluate
	// there (selection); and none where the mapping does not determine it, or
	// the owner has no declaration. A child's "$type" is settled under each
	// (walker.instanceGoverned), and the children are ·attributed· under each
	// one's content model (object.attribute).
	governing []governingType
	// unattributed is the mapping attributing none of the owner's children
	// under governing: the element content of one of them is one
	// xsd.Schema.ContentMatcher declines, or the schema view lacks the
	// governor capability (walker.govern).
	unattributed bool
}

// governingType is one type that may govern an object's owner
// (object.governing), with matcher, its content model, fed the owner's child
// elements in document order (object.attribute): nil where it has no element
// content, and from the first child it rejects on, after which validate
// advances it no further (validate's contentCheck.element).
type governingType struct {
	typ     xsd.TypeDefinition
	matcher *xsd.Matcher
}

// attribute feeds name, the expanded name of the owner's next child element,
// to the matcher of each of o.governing, and returns what each ·attributes·
// that child to (§3.4.4.4), in o.governing's order: nil where nothing does —
// no element content, or a child at or before it rejected (cvc-complex-content
// charges the parent) — and false where the mapping cannot attribute it
// (object.unattributed, or o.governing empty). Every child element of the
// owner is fed in document order, whatever its value, as validate's walk feeds
// its matcher, so each attribution depends on the siblings before it alone
// (STYLE P4).
func (o *object) attribute(name xsd.QName) ([]xsd.Attribution, bool) {
	if len(o.governing) == 0 || o.unattributed {
		return nil, false
	}
	as := make([]xsd.Attribution, len(o.governing))
	for i := range o.governing {
		gt := &o.governing[i]
		if gt.matcher == nil {
			continue
		}
		a, ok := gt.matcher.Next(name)
		if !ok {
			gt.matcher = nil
		}
		as[i] = a
	}
	return as, true
}

// placement is where a child element stands among its parent's members:
// parent, the object it is a member of, nil for the root; a, what each of
// parent.governing ·attributes· it to, in its order, nil where nothing does;
// and attributed, false where the mapping cannot attribute it
// (object.attribute).
type placement struct {
	parent     *object
	a          []xsd.Attribution
	attributed bool
}

// member is one key's classification, a closed sum of two arms: an
// attributeMember, or an elementMember — a declaredElement or a bareElement.
type member interface {
	classified()
}

// attributeMember is a key that is an attribute of the enclosing element,
// named by the expanded name of the attribute use it matched, or by the one
// namespace the type's {attribute wildcard} enumerates.
type attributeMember struct {
	name xsd.QName
}

// elementMember is a key that is a child element: the name it carries, and
// what an object value under it classifies its own members against, given the
// element e, the xsi:type its "$type" member presents (nil where it has none),
// where e stands among its parent's members (the zero placement for the root)
// and the location of its '{'.
type elementMember interface {
	member
	elementName() xsd.QName
	object(w *walker, e *element, typ *attribute, at placement, loc xsderr.Loc) (*object, error)
	// typeTable is the {type table} of the element's declaration, false where
	// it has none or no declaration.
	typeTable() (xsd.TypeTable, bool)
}

// declaredElement is a child under the element declaration decl: one the
// enclosing type contains, the root's top-level one, or, where named is
// true, the top-level one the expanded name of a child wildcardMember names
// resolves to.
type declaredElement struct {
	decl  xsd.ElementDeclaration
	named bool
}

// bareElement is a child wildcardMember names whose expanded name no
// top-level declaration carries: it has no declaration, so an object value
// under it maps only empty.
type bareElement struct {
	name xsd.QName
}

func (attributeMember) classified() {}
func (declaredElement) classified() {}
func (bareElement) classified()     {}

func (d declaredElement) elementName() xsd.QName { return d.decl.Name() }
func (b bareElement) elementName() xsd.QName     { return b.name }

func (d declaredElement) typeTable() (xsd.TypeTable, bool) { return d.decl.TypeTable() }
func (bareElement) typeTable() (xsd.TypeTable, bool)       { return xsd.TypeTable{}, false }

// object classifies e's members, where e carries an xsi:type typ naming a type
// (instanceType), under the one walker.instanceGoverned settles, which then
// governs e; and otherwise under d's {type definition} (declaredType), whatever
// types walker.governing finds may govern e from its ·attribution·, and, where d
// carries a {type table}, under every type the table can select as well
// (walker.selectable, walker.classify), until walker.attributes reads, at the
// root, the one the table selects (selection).
//
// The "$type" arm reads no {type table}: an xsi:type naming a type T
// ·overrides· e's ·selected type definition· or is charged cvc-elt clause 4
// (key-governing-type-elem clauses 3 and 4), so T governs e wherever its
// verdict stands uncharged, whichever type the table selects.
func (d declaredElement) object(w *walker, e *element, typ *attribute, at placement, loc xsderr.Loc) (*object, error) {
	declared, err := w.declaredType(d.decl, loc)
	if err != nil {
		return nil, err
	}
	if typ != nil {
		if instance, ok := w.instanceType(e, typ.value); ok {
			td, err := w.instanceGoverned(e, typ, instance, at.parent)
			if err != nil {
				return nil, err
			}
			o := w.object(e.name, td)
			w.govern(o, []xsd.TypeDefinition{td})
			return o, nil
		}
	}
	o := w.object(e.name, declared)
	if o.selectable, err = w.selectable(d.decl, e.name, loc); err != nil {
		return nil, err
	}
	w.govern(o, w.governing(d.decl, declared, at))
	return o, nil
}

// object is the empty object a bareElement's value classifies against: {}
// maps, and every member declines (walker.wildcardMember), those after a
// "$type" included — whatever type it names, the element has no declaration
// for it to stand beside. No member being classified under any type, its
// "$type" is settled against none (walker.instanceGoverned), and no type is
// determined to govern it (object.governing).
func (b bareElement) object(*walker, *element, *attribute, placement, xsderr.Loc) (*object, error) {
	return &object{owner: b.name, undeclared: true}, nil
}

// classify resolves a member's key, at loc, against o, its prefix read through
// sc (keyName): to the child element declaration among o.contained that
// carries it — of the expanded name a prefixed key spells, or of the local
// name an unprefixed key is — or to the attribute use among o.uses whose
// {attribute declaration} carries it (cvc-complex-type clause 2.1 then
// matches the attribute to that use by expanded name), and, where neither
// carries it, to the element or attribute a wildcard of the type names, or to
// the child a type with no wildcard names (wildcardMember). A key a
// declaration or a use carries never reaches a wildcard: the mapping's own
// rule, which the spec shares only where the wildcard's one namespace is the
// declaration's or the use's own, so that both name one item (the Note under
// cvc-accept; the Note after cvc-complex-type clause 4; package doc, "The
// mapping"). The first declaration of the name stands for every other
// (xsd.Schema.ContainedElementDeclarations says why that is sound). The key
// is not reserved: walker.entry declined those, walker.value and
// walker.typeMember having read the ones that lead the object.
//
// It DECLINES a key keyName declines; an unprefixed key two expanded names
// carry among the declarations, or among the uses; a key both a declaration
// and a use carry, which no spec rule decides between; a use in the xsi
// namespace, xsi:nil being the null arm's to present, xsi:type the "$type"
// member's, and xsi:schemaLocation and xsi:noNamespaceSchemaLocation hints a
// JSON instance carries none of; an attribute use of a type with simple
// content, beside which the mapping spells no text; and every key
// wildcardMember declines.
//
// GAP(jsonsrc): a key both a child element declaration and an attribute use
// carry declines. RULED permanent by #2613: no spec rule decides between them,
// and the mapping has no warning channel to report a choice through.
//
// GAP(jsonsrc): a key a declaration or use carries maps under its name even
// where a wildcard of the type would name the key as the other kind, or, for
// an unprefixed key, (N,key) in another namespace N. RULED permanent by #2634
// as the mapping's own rule: the spec is silent, the Note under cvc-accept and
// the Note after cvc-complex-type clause 4 giving a declaration or use
// precedence only over a wildcard of its own kind that admits the same
// expanded name. A prefixed key spells (N,key) itself, which a declaration or
// use of another namespace does not carry.
//
// Under an owner whose declaration carries a {type table} and whose
// ·selected type definition· the mapping does not read (selection), the key is
// classified under the declaration's {type definition}, o, and again under
// each of o.selectable, and maps only where every one classifies it alike
// (walker.alike), scalar saying whether the member's value is a scalar or
// null (scalarValue). The element is assessed under its ·selected type
// definition· (key-selected-type clause 1, key-cta-select), so it maps only a
// key that classifies the same whichever type the table selects, and the
// infoset is the one any of them would give. It DECLINES a key any one of
// them declines or classifies otherwise. At the root, walker.attributes reads
// the selection instead, and the members after the attribute run are
// classified here under the selected type alone, o.selectable being nil.
//
// GAP(jsonsrc): a member of an element below the root whose declaration
// carries a {type table} declines where the declaration's {type definition}
// and a type the table can select classify it differently, or one of them
// declines it alone, where the engine assesses the XML twin under the type the
// table selects: a key that is a child under one type and an attribute under
// another, and, under an object value, a child under declarations alike does
// not equate. The mapping computes no [inherited attributes], which a {test}
// reads beside the element's own (selection). Owned by #2699, which computes them.
//
// The two-expanded-name decline among the declarations is childNamed's, and
// the xsi-namespace, two-expanded-name and simple-content declines among the
// uses useNamed's, each under its function's GAP(jsonsrc) markers.
func (w *walker) classify(o *object, key string, sc *scope, loc xsderr.Loc, scalar bool) (member, error) {
	k, err := w.keyName(key, sc, false, loc)
	if err != nil {
		return nil, err
	}
	ms, err := w.membersOf(append([]*object{o}, o.selectable...), key, k, loc)
	if err != nil {
		return nil, err
	}
	if !w.allAlike(ms, scalar) {
		return nil, w.notAlike(o, key, loc)
	}
	return ms[0], nil
}

// membersOf classifies the key, read as k, at loc, under each of objects, one
// owner's for which the mapping reads no ·selected type definition·: the first
// one's decline is the key's own, and another's is notAlike's. In a root's
// attribute run, which reads the selection, walker.candidates classifies the
// key instead.
func (w *walker) membersOf(objects []*object, key string, k keyName, loc xsderr.Loc) ([]member, error) {
	ms := make([]member, 0, len(objects))
	for i, o := range objects {
		m, err := w.memberOf(o, key, k, loc)
		if err != nil && i > 0 {
			return nil, w.notAlike(objects[0], key, loc)
		}
		if err != nil {
			return nil, err
		}
		ms = append(ms, m)
	}
	return ms, nil
}

// allAlike reports whether each of ms, one key's classifications, is alike
// the first (walker.alike).
func (w *walker) allAlike(ms []member, scalar bool) bool {
	for _, m := range ms[1:] {
		if !w.alike(ms[0], m, scalar) {
			return false
		}
	}
	return true
}

// notAlike declines the member key, at loc, of o's object, which the types
// o's owner's {type table} can select classify differently, or one of them
// declines alone, where the mapping reads no ·selected type definition· for
// the owner (selection).
func (w *walker) notAlike(o *object, key string, loc xsderr.Loc) error {
	return w.declinef(loc, "member %q of %s is not classified alike under its declaration's {type definition} and every type its {type table} can select; the mapping reads no ·selected type definition· for it",
		key, o.owner)
}

// undetermined declines the member key, at loc, of o's object, the attribute
// run of a root whose {type table} the mapping reads the selection of
// (walker.attributes), which a live candidate other than the first declines,
// the first classifying it, where walker.candidates may not drop the one that
// declines it: a type some reading of the run up to the member selects
// declines it, or the engine withholds one such reading's selection, under
// walker.candidates' GAP(jsonsrc) markers; or, where another member follows
// it, reading on settles no one reading, under walker.readAhead's.
func (w *walker) undetermined(o *object, key string, loc xsderr.Loc) error {
	return w.declinef(loc, "member %q of %s is declined under a type its {type table} can select and classified under another; the type the table ·conditionally selects· reads the whole run of attribute members, and the mapping maps the member only where no reading of the run up to it selects a type that declines it, and, reading on, one reading alone is classified as it reads by the type it selects",
		key, o.owner)
}

// memberOf classifies the key, read as k, at loc, against o alone: classify's
// arms but the {type table} one.
func (w *walker) memberOf(o *object, key string, k keyName, loc xsderr.Loc) (member, error) {
	decl, declared, err := w.childNamed(o, key, k, loc)
	if err != nil {
		return nil, err
	}
	use, used, err := w.useNamed(o, key, k, loc)
	if err != nil {
		return nil, err
	}
	if declared && used {
		return nil, w.declinef(loc, "member %q names both the child element declaration %s and the attribute use %s of the type of %s; the mapping does not choose between them",
			key, decl.Name(), use.DeclarationName(), o.owner)
	}
	if used {
		return attributeMember{name: use.DeclarationName()}, nil
	}
	if !declared {
		return w.wildcardMember(o, key, k, loc)
	}
	return declaredElement{decl: decl}, nil
}

// alike reports whether a and b, one key's classifications under two types,
// are one member, where scalar says whether the member's value is a scalar or
// null (scalarValue): an attribute of one expanded name; under a scalar or
// null value, a child of one expanded name, whatever declarations they are
// under; and otherwise a child with no declaration of one expanded name, or
// a child under declarations of one expanded name whose {type definition}s
// are one type (oneType) and neither of which carries a {type table}, so that
// the child's own members are classified alike too, under that type
// (declaredElement.object).
//
// A scalar or null value builds its element from the name alone
// (walker.scalar, walker.nilled): it carries no member to classify and no
// "$type", so two classifications naming one child give one infoset. The
// declarations behind them — the {nillable} cvc-elt clause 3 reads, the
// {type definition} or ·selected type definition· cvc-elt clause 5 and
// cvc-type assess the text under, the {identity-constraint definitions}
// cvc-elt clause 6 reads — are the engine's to read off the ·attribution·
// it computes, and the mapping reads none of them here.
//
// A declaration of an anonymous simple type is alike no other under a value
// that is not scalar: oneType reads no identity of one, and the decline that
// leaves is classify's GAP(jsonsrc) marker's.
func (w *walker) alike(a, b member, scalar bool) bool {
	if x, ok := a.(attributeMember); ok {
		y, ok := b.(attributeMember)
		return ok && x == y
	}
	x, ok := a.(elementMember)
	if !ok {
		return false
	}
	y, ok := b.(elementMember)
	if !ok || x.elementName() != y.elementName() {
		return false
	}
	if scalar {
		return true
	}
	switch x := x.(type) {
	case bareElement:
		_, ok := y.(bareElement)
		return ok
	case declaredElement:
		y, ok := y.(declaredElement)
		return ok && w.oneUntabledType(x.decl, y.decl)
	}
	return false
}

// oneUntabledType reports whether neither d nor e carries a {type table} and
// their {type definition}s resolve to one type (oneType).
func (w *walker) oneUntabledType(d, e xsd.ElementDeclaration) bool {
	if _, table := d.TypeTable(); table {
		return false
	}
	if _, table := e.TypeTable(); table {
		return false
	}
	s, ok := w.schema.ResolvedType(d.TypeDefinition())
	if !ok {
		return false
	}
	t, ok := w.schema.ResolvedType(e.TypeDefinition())
	return ok && oneType(s, t)
}

// oneType reports whether s and t, each a named type or the {type definition}
// of an element declaration, are one type: of one name, or each an anonymous
// complex type whose {context} (xsd.ComplexType.Context) is one declaration's
// identity, which owns that one {type definition} or, through a substitution
// group, shares its head's (§3.3.2.1 clause 3). An anonymous simple type is
// one with no other, the mapping reading no identity of one.
//
// It does not equate the types of a {type table}: the anonymous type of an
// <alternative> carries its declaration's {context} as the declaration's own
// type does (xsd.ComponentID, §3.4.2.1 dcl.ctd.common), so two of them compare
// alike by {context}; sameNamed equates those.
func oneType(s, t xsd.TypeDefinition) bool {
	if s.Name() != (xsd.QName{}) || t.Name() != (xsd.QName{}) {
		return s.Name() == t.Name()
	}
	sc, ok := s.(xsd.ComplexType)
	if !ok {
		return false
	}
	tc, ok := t.(xsd.ComplexType)
	if !ok {
		return false
	}
	sx, ok := sc.Context()
	if !ok {
		return false
	}
	tx, ok := tc.Context()
	return ok && sx.ID() == tx.ID()
}

// sameNamed reports whether s and t are one named type. Anonymous types are
// never the same here: oneType says why {context} identity does not tell the
// types a {type table} can select apart.
func sameNamed(s, t xsd.TypeDefinition) bool {
	return s.Name() != (xsd.QName{}) && s.Name() == t.Name()
}

// wildcardMember names a key, read as k, no contained declaration and no
// attribute use of o carries, where the key or the model fixes its namespace
// (package doc, "The mapping"). Element kind: the type has an element
// wildcard — a Wildcard {term} its content model contains, whatever its
// {process contents}, or its {open content} wildcard — and the key is the
// child of the expanded name a prefixed key spells, or, for an unprefixed key,
// the child (N,key), where every such wildcard enumerates the one namespace N;
// either is under the top-level declaration of that name where there is one
// (declaredElement) and a bareElement where there is none. Attribute kind: the
// type has only an {attribute wildcard}, and the key is the attribute a
// prefixed key spells, or (M,key), where it enumerates the one namespace M. No
// wildcard of either kind: the key is the child a prefixed key spells, or
// (O,key), O the owner's own namespace name, named as an element wildcard's
// child is (namedChild); every XML twin of the key, of whatever name and kind,
// is invalid — an element under cvc-complex-content, cvc-complex-type clause
// 1.1 or 1.2, or cvc-type clause 3.1.2 under a simple type, an attribute under
// cvc-complex-type clause 2.2.1 or cvc-type clause 3.1.1 — but the four xsi
// attributes cvc-complex-type clause 2 excepts, so the name decides no
// validity, and O keeps the child's default namespace its parent's.
//
// It does not test admission: a {disallowed names} entry, ##defined and
// ##definedSibling are the engine's to charge (cvc-wildcard clauses 1 to 3),
// and so is a prefixed key's namespace that a wildcard does not admit; a
// name the wildcards reject is soundly invalid, because the input spells it,
// or because every other namespace is outside every wildcard of that kind,
// and no declaration or use of the type carries the key. Which {process
// contents} applies, and whether the element is ·skipped·, ·laxly assessed· or
// charged under e-validity clause 1.1.3, is the engine's too.
//
// It DECLINES a member of a bareElement's object; a key the type has
// wildcards of both kinds for (xs:anyType has); for an unprefixed key, a
// wildcard of variety any or not, an enumeration of more than one namespace,
// or element wildcards whose namespaces differ; a name it would give in the
// xsi or the xml namespace, N or M; a prefixed key's in the xsi namespace,
// and its child element's in the xml namespace; and a key a simple-content
// type's {attribute wildcard} alone admits.
//
// GAP(jsonsrc): a wildcard-only key whose namespace or kind the model does
// not fix declines: for an unprefixed key, a wildcard of variety any or not,
// xs:anyType's included, an enumeration of several namespaces, and element
// wildcards enumerating different ones; for any key, a type with wildcards of
// both kinds, and a bareElement's members, which xs:anyType's lax wildcards of
// both kinds would classify. RULED permanent by #2634: the verdict of
// whichever (N,key) the mapping chose depends on N through declaration lookup
// (key-governing-ed clause 3), attribution (cvc-accept, cvc-wildcard,
// cvc-wildcard-name) and an ancestor's identity-constraint paths
// (c-selector-xpath, c-fields-xpaths) and assertions (cvc-assertion), which
// the mapping cannot fix without an ancestor census. A prefix fixes N, so it
// lifts the namespace declines; it does not say the kind, so it keeps the
// other two.
//
// GAP(jsonsrc): a key a wildcard's single namespace, of either kind, puts in
// the xml or the xsi namespace declines, and so does a prefixed key no
// declaration or use carries that spells a name in the xsi namespace, or a
// child element's in the xml namespace, under an element wildcard or no
// wildcard. RULED permanent by #2635: an element in the xml namespace would
// bind the default namespace to it, which no XML twin has, and the attribute
// kind is ruled with it; the four xsi attributes cvc-complex-type clause 2
// excepts must keep declining, xsi:nil and xsi:type having their own
// spellings, and the rest of the xsi namespace is not worth a half-arm. A
// prefixed key in the xml namespace under a type with only an {attribute
// wildcard} maps, to the attribute it spells: an attribute binds no default
// namespace, so the element's reason does not reach it.
//
// GAP(jsonsrc): a key a simple-content type's {attribute wildcard} alone
// admits declines, on the ground useNamed's simple-content marker states for
// an attribute use, RULED permanent by #2613: the mapping spells no text
// beside attribute members.
func (w *walker) wildcardMember(o *object, key string, k keyName, loc xsderr.Loc) (member, error) {
	if o.undeclared {
		return nil, w.declinef(loc, "member %q of %s, an element no contained and no top-level element declaration names; only an empty object maps under it",
			key, o.owner)
	}
	elemental := len(o.elementWildcards) > 0
	if elemental && o.attributeWildcard != nil {
		return nil, w.declinef(loc, "member %q names no child element declaration and no attribute use of the type of %s, which has both an element wildcard and an {attribute wildcard}; a JSON key does not say which it is",
			key, o.owner)
	}
	if !elemental && o.attributeWildcard != nil && o.simpleContent {
		return nil, w.declinef(loc, "member %q names only the {attribute wildcard} of the type of %s, whose {content type} is simple; the mapping spells no text beside attribute members",
			key, o.owner)
	}
	if k.prefixed {
		if k.name.Space == xsd.XMLSchemaInstanceNS {
			return nil, w.declinef(loc, "member %q names no child element declaration and no attribute use of the type of %s, and its prefix puts it in the namespace %s; the mapping names no key in the xsi namespace", key, o.owner, k.name.Space)
		}
		if o.attributeWildcard != nil {
			return attributeMember{name: k.name}, nil
		}
		if k.name.Space == xmlNamespace {
			return nil, w.declinef(loc, "member %q names no child element declaration and no attribute use of the type of %s, and its prefix puts it in the namespace %s; the mapping names no child element in the xml namespace", key, o.owner, k.name.Space)
		}
		return w.namedChild(k.name), nil
	}
	if elemental {
		ns, why, ok := fixedNamespace(o.elementWildcards)
		if !ok {
			return nil, w.declinef(loc, "member %q names no child element declaration and no attribute use of the type of %s, and its element wildcard set %s",
				key, o.owner, why)
		}
		return w.namedChild(xsd.QName{Space: ns, Local: key}), nil
	}
	if o.attributeWildcard == nil {
		return w.namedChild(xsd.QName{Space: o.owner.Space, Local: key}), nil
	}
	ns, why, ok := fixedNamespace([]xsd.Wildcard{*o.attributeWildcard})
	if !ok {
		return nil, w.declinef(loc, "member %q names no child element declaration and no attribute use of the type of %s, and its {attribute wildcard} set %s",
			key, o.owner, why)
	}
	return attributeMember{name: xsd.QName{Space: ns, Local: key}}, nil
}

// namedChild is the child element name, which no declaration the enclosing
// type contains carries: under the top-level declaration of that name where
// there is one (declaredElement), and as a bareElement where there is none.
func (w *walker) namedChild(name xsd.QName) member {
	d, ok := w.schema.Element(name)
	if !ok {
		return bareElement{name: name}
	}
	return declaredElement{decl: d, named: true}
}

// fixedNamespace is the one namespace name every wildcard of wildcards
// enumerates, read in document order, and ok. It is not ok where one is of
// variety any or not, where they enumerate more than one namespace, and where
// the one is the xsi or the xml namespace; why then completes the caller's
// decline, after the wildcards' noun.
//
// {variety} and {namespaces} are read here to name the key, not to admit it:
// admission stays cvc-wildcard's, the engine's (wildcardMember).
func fixedNamespace(wildcards []xsd.Wildcard) (uri, why string, ok bool) {
	var fixed []xsd.Namespace
	for _, wc := range wildcards {
		nc := wc.NamespaceConstraint()
		if nc.Variety() != xsd.NamespaceConstraintEnumeration {
			return "", fmt.Sprintf("includes one of variety %s, which fixes no namespace for it", nc.Variety()), false
		}
		for _, n := range nc.Namespaces() {
			if !slices.Contains(fixed, n) {
				fixed = append(fixed, n)
			}
		}
	}
	if len(fixed) != 1 {
		return "", fmt.Sprintf("enumerates %d namespaces; an unprefixed key does not choose between them", len(fixed)), false
	}
	uri, _ = fixed[0].URI() // "" for ·absent·, a QName's no-namespace Space
	if uri == xsd.XMLSchemaInstanceNS || uri == xmlNamespace {
		return "", fmt.Sprintf("enumerates the namespace %s alone; the mapping names no key in the xsi or xml namespace", uri), false
	}
	return uri, "", true
}

// childNamed is the first of o.contained that carries the key, read as k
// (keyName.carries), declining an unprefixed key two expanded names carry. A
// prefixed key is carried by one expanded name at most.
//
// GAP(jsonsrc): an unprefixed key child element declarations of two expanded
// names carry declines. RULED permanent by #2623: an unprefixed key names a
// declaration by its local name alone, two carry it, and the verdict turns on
// which (cvc-accept clause 2.3.1 matches an element to a declaration by
// expanded name); a prefixed key "p:local" spells each one in a namespace, and
// one in no namespace beside one in a namespace is spelled by no key, since no
// prefix is bound to the absent namespace (Namespaces in XML §5, NSC: No
// Prefix Undeclaring; walker.xmlns), which the mapping declines rather than
// give the unprefixed key either meaning.
func (w *walker) childNamed(o *object, key string, k keyName, loc xsderr.Loc) (xsd.ElementDeclaration, bool, error) {
	var first xsd.ElementDeclaration
	found := false
	for _, d := range o.contained {
		if !k.carries(d.Name()) {
			continue
		}
		if !found {
			first, found = d, true
			continue
		}
		if d.Name() != first.Name() {
			return xsd.ElementDeclaration{}, false, w.declinef(loc, "member %q names child element declarations of two expanded names in the type of %s (%s and %s); an unprefixed key does not choose between them, and a prefixed one, \"p:%s\", names one",
				key, o.owner, first.Name(), d.Name(), key)
		}
	}
	return first, found, nil
}

// useNamed is the member of o.uses whose {attribute declaration} carries the
// key, read as k (keyName.carries). It declines an unprefixed key uses of two
// expanded names carry, a use in the xsi namespace, and a use of a type with
// simple content.
//
// GAP(jsonsrc): an unprefixed key attribute uses of two expanded names carry
// declines. RULED permanent by #2623: an unprefixed key names a use by its
// local name alone, two carry it, and the verdict turns on which
// (cvc-complex-type clause 2.1 matches an attribute to a use by expanded
// name); a prefixed key "p:local" spells each one in a namespace, and one in
// no namespace beside one in a namespace is spelled by no key, since no prefix
// is bound to the absent namespace (Namespaces in XML §5, NSC: No Prefix
// Undeclaring; walker.xmlns), which the mapping declines rather than give the
// unprefixed key either meaning. The absent namespace an unprefixed XML
// attribute name has (§6.2) does not decide it: an unprefixed key is matched
// by local name, the rule that maps a key one use in a namespace carries.
//
// GAP(jsonsrc): a key naming an xsi:type attribute use declines. RULED
// permanent by #2615: a "$type" member is xsi:type's one spelling
// (walker.typeMember), and a second would let one object present it twice.
//
// GAP(jsonsrc): a key naming an xsi:schemaLocation or
// xsi:noNamespaceSchemaLocation attribute use declines. RULED permanent by
// #2627: the two are schema-location hints (§2.7.3, §4.3.2) a JSON instance
// carries none of and the mapping has no spelling for, and cvc-complex-type
// clause 2 excepts them, so a use of either is never attributed one.
//
// GAP(jsonsrc): a key naming an xsi:nil attribute use declines. RULED
// permanent by #2614: a null value is xsi:nil's one spelling (walker.nilled),
// and a second would let one object present it twice.
//
// GAP(jsonsrc): an attribute use of a type whose {content type} is simple
// declines. RULED permanent by #2613: the XML twin, simple content beside
// attributes, is valid under cvc-complex-type clauses 1.2 and 2, and the
// mapping spells no text beside attribute members.
func (w *walker) useNamed(o *object, key string, k keyName, loc xsderr.Loc) (xsd.AttributeUse, bool, error) {
	var first xsd.AttributeUse
	found := false
	for _, u := range o.uses {
		if !k.carries(u.DeclarationName()) {
			continue
		}
		if !found {
			first, found = u, true
			continue
		}
		if u.DeclarationName() != first.DeclarationName() {
			return xsd.AttributeUse{}, false, w.declinef(loc, "member %q names attribute uses of two expanded names in the type of %s (%s and %s); an unprefixed key does not choose between them, and a prefixed one, \"p:%s\", names one",
				key, o.owner, first.DeclarationName(), u.DeclarationName(), key)
		}
	}
	if !found {
		return xsd.AttributeUse{}, false, nil
	}
	if first.DeclarationName().Space == xsd.XMLSchemaInstanceNS {
		return xsd.AttributeUse{}, false, w.declinef(loc, "member %q names the attribute use %s of the type of %s; an xsi: attribute is presented only by the null => xsi:nil arm and the \"$type\" member",
			key, first.DeclarationName(), o.owner)
	}
	if o.simpleContent {
		return xsd.AttributeUse{}, false, w.declinef(loc, "member %q names an attribute use of the type of %s, whose {content type} is simple; the mapping spells no text beside attribute members",
			key, o.owner)
	}
	return first, true, nil
}

// selectable is the object, for an element named owner, of each type d's
// {type table} can select (key-cta-select): each {alternatives} member's {type
// definition}, in order, then the {default type definition}'s; and nil where
// d carries none. tokLoc locates the object classified under them.
//
// It DECLINES a {type definition} among them that does not resolve, under
// declaredType's GAP(jsonsrc) marker, RULED permanent by #2625: a finalized
// Schema reaches such a slot the ways it reaches a declaration's, and no
// other, xsd.NewTypeAlternative rejecting a nil one and admitting a
// substitution-group head only in the {default type definition} the
// declaration's own slot is copied to (§3.3.2.1).
func (w *walker) selectable(d xsd.ElementDeclaration, owner xsd.QName, tokLoc xsderr.Loc) ([]*object, error) {
	table, ok := d.TypeTable()
	if !ok {
		return nil, nil
	}
	alternatives := tableAlternatives(table)
	objects := make([]*object, 0, len(alternatives))
	for i, alt := range alternatives {
		t, ok := w.schema.ResolvedType(alt.TypeDefinition())
		if !ok {
			return nil, w.declinef(tokLoc, "the {type definition} of Type Alternative %d of %d in the {type table} of %s does not resolve; its members cannot be classified",
				i+1, len(alternatives), d.Name())
		}
		objects = append(objects, w.object(owner, t))
	}
	return objects, nil
}

// The two reserved members the mapping reads (walker.value, walker.typeMember).
const (
	memberXmlns = "$xmlns"
	memberType  = "$type"
)

// isReserved reports whether key is a reserved member name: every
// "$"-prefixed one. "$xmlns" and "$type" are mapped where they lead an object
// (walker.value, walker.typeMember), and every other one is held.
func isReserved(key string) bool { return len(key) > 0 && key[0] == '$' }

// reserved declines the reserved member key at loc, which walker.value and
// walker.typeMember did not read: a "$xmlns" or "$type" out of its place, or
// any other reserved name. Any other "$" name, and a repeated "$xmlns" or
// "$type", has no XML twin, so no marker tracks it: "$" is not an XML
// NameStartChar (XML 1.0 §2.3 [4]), and an element carries one attribute of
// each expanded name (XML 1.0 §3.1, Unique Att Spec).
//
// GAP(jsonsrc): a "$xmlns" or "$type" out of its leading place — "$type"
// before "$xmlns", or either after an attribute or child member — declines,
// where its XML twin, attribute order being insignificant, carries the
// namespace declarations or the xsi:type. RULED permanent by #2615: no spec
// text orders an element's attributes, so the decline is never a charge, and
// reading the two first is what lets the members after them be classified
// under the type the "$type" decides, resolved through the bindings "$xmlns" makes,
// without holding the object (STYLE P4), as members.key's marker says of an
// attribute member after a child.
func (w *walker) reserved(key string, loc xsderr.Loc) error {
	if key == memberXmlns || key == memberType {
		return w.declinef(loc, "reserved member %q is out of place; \"$xmlns\" and then \"$type\" lead an element's object, each at most once, before its other members", key)
	}
	return w.declinef(loc, "reserved member %q; only the \"$type\" and \"$xmlns\" reserved members are mapped, and a member naming an attribute or a child is keyed by its own name, which takes no \"$\"", key)
}

// value is the start of a member's value, or of an array's item, as far as
// the walk reads it before classifying the key over it: its first token and
// location, and scope, the bindings in scope at the element it makes — for an
// object, its leading "$xmlns" member's over the parent's, where it has one,
// and the parent's otherwise — with, for an object, next, the token after
// that member, and its location.
type value struct {
	tok     json.Token
	loc     xsderr.Loc
	scope   *scope
	next    json.Token
	nextLoc xsderr.Loc
}

// value reads the start of a value, the element it makes inheriting the scope
// parent: its first token, and, for an object, its leading "$xmlns" member
// where it has one (walker.xmlns), whose bindings scope the element, and the
// token after it. key is the member's key, which the "$xmlns" declines name.
//
// Reading the "$xmlns" before the key is classified is what lets a prefixed
// key resolve through the bindings in scope at the element it names, its own
// included (keyName), without holding any of the document (STYLE P4).
func (w *walker) value(parent *scope, key string) (value, error) {
	tok, loc, err := w.token()
	if err != nil {
		return value{}, unexpectedEOF(err, loc)
	}
	v := value{tok: tok, loc: loc, scope: parent}
	if tok != json.Delim('{') {
		return v, nil
	}
	if v.next, v.nextLoc, err = w.token(); err != nil {
		return value{}, unexpectedEOF(err, v.nextLoc)
	}
	if v.next != json.Token(memberXmlns) {
		return v, nil
	}
	bindings, err := w.xmlns(key)
	if err != nil {
		return value{}, err
	}
	v.scope = &scope{parent: parent, bindings: bindings}
	if v.next, v.nextLoc, err = w.token(); err != nil {
		return value{}, unexpectedEOF(err, v.nextLoc)
	}
	return v, nil
}

// entry reads the value of the member key, at loc, of an object in the scope
// sc, as far as walker.value does, for members.key to classify the key over
// it. It DECLINES a reserved key before reading its value (walker.reserved).
func (w *walker) entry(sc *scope, key string, loc xsderr.Loc) (value, error) {
	if isReserved(key) {
		return value{}, w.reserved(key, loc)
	}
	return w.value(sc, key)
}

// element builds the element a member (or an item of a member's array) maps
// to, from its classification m, the location of its key or item, the key,
// its value as walker.value read it, and in, the members cursor of the
// parent's object — nil for the root. Every element below the root is
// ·attributed· under its parent's governing type as it is built
// (object.attribute), whatever its value.
//
// It DECLINES an array item that is itself an array. That has no XML twin, so
// no marker tracks it: an item maps to exactly one element, and an array to
// zero or more.
func (w *walker) element(m elementMember, loc xsderr.Loc, key string, v value, in *members) (*element, error) {
	var at placement
	if in != nil {
		at.parent = in.object
		at.a, at.attributed = in.object.attribute(m.elementName())
	}
	switch t := v.tok.(type) {
	case nil:
		return w.nilled(m.elementName(), loc, v.loc, v.scope), nil
	case json.Delim:
		if t != json.Delim('{') {
			// '[' alone reaches here: Token never yields a closing delimiter
			// where a value is due.
			return nil, w.declinef(v.loc, "an array is an item of the array of member %q; only an object or a scalar maps to one element", key)
		}
		return w.objectElement(m, loc, v, at)
	}
	lexical, ok := scalarLexical(v.tok)
	if !ok {
		panic(fmt.Sprintf("jsonsrc: Token yielded %T", v.tok))
	}
	return w.scalar(m.elementName(), loc, lexical, v.loc, v.scope), nil
}

// objectElement builds an object-valued element from its value v, standing at
// at among its parent's members: its scope, v's, its "$type" (typeMember),
// then its attribute members (attributes), each classified under the type its
// members are (elementMember.object), and a cursor over the rest.
func (w *walker) objectElement(m elementMember, loc xsderr.Loc, v value, at placement) (*element, error) {
	e := &element{w: w, name: m.elementName(), loc: loc, scope: v.scope}
	typ, tok, tokAt, err := w.typeMember(e, v.next, v.nextLoc)
	if err != nil {
		return nil, err
	}
	o, err := m.object(w, e, typ, at, v.loc)
	if err != nil {
		return nil, err
	}
	attrs, ahead, o, err := w.attributes(o, e.scope, tok, tokAt, typ, selection(m, o, at))
	if err != nil {
		return nil, err
	}
	e.attrs = attrs
	e.kids = &members{w: w, owner: e, object: o, ahead: ahead}
	return e, nil
}

// selection is the {type table} walker.attributes reads the ·selected type
// definition· of an element off, where m is the element, o what its members
// are classified against, and at where it stands: m's declaration's, where o
// classifies them under its {type definition} and the types the table can
// select (object.selectable) and the element is the validation root, and nil
// otherwise. The root has no ancestor, so its [inherited attributes] are none
// (key-p-inherited clause 1) and its {test}s read its [attributes] alone
// (key-cta-ta-select clauses 1.1.2 and 1.1.3).
//
// GAP(jsonsrc): below the root, the mapping reads no element's ·selected type
// definition·: it computes no [inherited attributes] (e-inherited_attributes),
// which a {test} reads beside the element's own. A member of an element below
// the root whose declaration carries a {type table} therefore maps only where
// every type the table can select classifies it alike (walker.classify), and
// a "$type" below such an element only where each settles it alike
// (walker.instanceGoverned); either declines otherwise, where the engine
// assesses the XML twin under the type the table selects. Owned by
// #2699, which computes them.
func selection(m elementMember, o *object, at placement) *xsd.TypeTable {
	table, tabled := m.typeTable()
	if !tabled || len(o.selectable) == 0 || at.parent != nil {
		return nil
	}
	return &table
}

// typeMember reads the "$type" member of e's object where tok, at loc, the
// token after any "$xmlns" walker.value read, is its key: the reserved
// members lead the object in this order and each at most once. It returns the
// "$type" as e's xsi:type attribute, located at its key, whose [[normalized
// value]] is the member's string unchanged, and nil where there is none, with
// the token after it, a key or the object's close, and its location.
//
// Reading the two first is what lets the members after them be classified
// under the type the "$type" decides, and "$type" resolve against the bindings
// "$xmlns" makes, without holding any of the document (STYLE P4). A "$type"
// before a "$xmlns", a repeat of either, and either after another member are
// left to walker.entry, which declines them (walker.reserved): no spec text
// orders an element's attributes, so none of them is charged.
func (w *walker) typeMember(e *element, tok json.Token, loc xsderr.Loc) (*attribute, json.Token, xsderr.Loc, error) {
	if tok != json.Token(memberType) {
		return nil, tok, loc, nil
	}
	vtok, vloc, err := w.token()
	if err != nil {
		return nil, nil, vloc, unexpectedEOF(err, vloc)
	}
	lexical, ok := vtok.(string)
	if !ok {
		return nil, nil, vloc, w.declinef(vloc, "the \"$type\" member of %s is not a string; its value is the lexical form of the element's xsi:type, so give it as a string, \"local\" or \"p:local\"", e.name)
	}
	typ := &attribute{name: xsiType, value: lexical, loc: loc}
	if tok, loc, err = w.token(); err != nil {
		return nil, nil, loc, unexpectedEOF(err, loc)
	}
	return typ, tok, loc, nil
}

// xmlns reads the value of a "$xmlns" member of the object under key: an object
// whose every member binds its key, a prefix, to its string value, a namespace
// name. It declines any other value; a member whose value is not a string; a
// non-empty prefix that is no NCName, which no xmlns:prefix attribute can
// declare (Namespaces in XML [2] PrefixedAttName); a prefix bound twice; the
// prefixes "xml" and "xmlns", which Namespaces in XML §3 reserves, "xml" being
// bound already (element.LookupPrefix) — the decline says to drop an "xml"
// binding to that namespace name or to "", and to bind another prefix for any
// other; and a non-empty prefix bound to the empty string, which would
// undeclare it. The empty prefix may be bound, and is shadowed: the element's
// default namespace stays its own namespace name (package doc).
func (w *walker) xmlns(key string) ([]binding, error) {
	tok, loc, err := w.token()
	if err != nil {
		return nil, unexpectedEOF(err, loc)
	}
	if tok != json.Delim('{') {
		return nil, w.declinef(loc, "the \"$xmlns\" member of %q is not an object; its members bind prefixes to namespace names", key)
	}
	var bindings []binding
	for {
		tok, loc, err := w.token()
		if err != nil {
			return nil, unexpectedEOF(err, loc)
		}
		prefix, ok := tok.(string)
		if !ok {
			return bindings, nil
		}
		vtok, vloc, err := w.token()
		if err != nil {
			return nil, unexpectedEOF(err, vloc)
		}
		uri, ok := vtok.(string)
		if !ok {
			return nil, w.declinef(vloc, "the \"$xmlns\" member of %q binds %q to a value that is not a string; a namespace name is a string", key, prefix)
		}
		if prefix != "" && !xmlname.IsNCName(prefix) {
			return nil, w.declinef(loc, "the \"$xmlns\" member of %q binds %q, which is not an NCName; no xmlns:prefix attribute declares it, so bind an NCName prefix instead", key, prefix)
		}
		if prefix == "xml" && uri != xmlNamespace && uri != "" {
			return nil, w.declinef(loc, "the \"$xmlns\" member of %q binds the reserved prefix %q to %q, but every element binds it to %s and none to any other namespace name, so bind another prefix to %q and use it in place of %q on the keys meant to be in that namespace", key, prefix, uri, xmlNamespace, uri, prefix)
		}
		if prefix == "xml" {
			return nil, w.declinef(loc, "the \"$xmlns\" member of %q binds the reserved prefix %q, which every element binds already, so drop the binding", key, prefix)
		}
		if prefix == "xmlns" {
			return nil, w.declinef(loc, "the \"$xmlns\" member of %q binds the reserved prefix %q, which no element binds, so bind another prefix", key, prefix)
		}
		if prefix != "" && uri == "" {
			return nil, w.declinef(vloc, "the \"$xmlns\" member of %q binds %q to the empty string; a prefix cannot be undeclared, so bind it to a namespace name or drop the binding", key, prefix)
		}
		if slices.ContainsFunc(bindings, func(b binding) bool { return b.prefix == prefix }) {
			return nil, w.declinef(loc, "the \"$xmlns\" member of %q binds %q twice; bind each prefix once", key, prefix)
		}
		bindings = append(bindings, binding{prefix: prefix, uri: uri})
	}
}

// declaredType is decl's {type definition}, which must resolve, tokLoc
// locating the object classified under it.
//
// It DECLINES a {type definition} that does not resolve.
//
// GAP(jsonsrc): an object value under a declaration whose {type definition}
// does not resolve declines, where the engine assesses its XML twin. RULED
// permanent by #2625: a finalized Schema reaches such a slot two ways
// (validate's resolvedSelection) — a substitution-group head naming no
// declaration, which the parser never builds, or a nil slot built without the
// parser's defaulting, which validate itself declines under the ruling #2166
// records — so no schema document reaches either; and under lax assessment
// the members would classify against no type, the bareElement case, whose
// verdicts depend on the namespace chosen, as #2634's grounding found.
func (w *walker) declaredType(decl xsd.ElementDeclaration, tokLoc xsderr.Loc) (xsd.TypeDefinition, error) {
	declared, ok := w.schema.ResolvedType(decl.TypeDefinition())
	if !ok {
		return nil, w.declinef(tokLoc, "the {type definition} of %s does not resolve; its members cannot be classified", decl.Name())
	}
	return declared, nil
}

// instanceType is the top-level type definition an xsi:type of e whose value
// is lexical names, and false where it names none. The lexical is resolved
// against e's bindings (e.LookupPrefix, so an unprefixed name takes e's own
// namespace name) by [xmlqname.Resolve], the one encoding validate's walk
// resolves an xsi:type through too (cvc-resolve-instance, §3.17.6.3, there
// resolveInstanceQName; STYLE T4), and the name is looked up among the
// schema's top-level type definitions. Where it names none, the engine charges
// cvc-attribute (clause 3 or 5) and assesses e under its declared type.
func (w *walker) instanceType(e *element, lexical string) (xsd.TypeDefinition, bool) {
	space, local, ok := xmlqname.Resolve(lexical, e.LookupPrefix)
	if !ok {
		return nil, false
	}
	return w.schema.ResolvedType(xsd.TypeDefinitionRef{Name: xsd.QName{Space: space, Local: local}})
}

// governor is the three methods of *xsd.Schema the mapping reads beyond
// validate.SchemaView: xsd.Schema.LocallyDeclaredElementType, key-ldt-elem's
// base-chain walk; xsd.Schema.ValidlySubstitutable, key-val-sub-type, which
// validate's walk settles ·override· by (walk.instanceOverride,
// walk.localGovernance); and xsd.Schema.ContentMatcher, the cvc-accept walk
// validate's contentCheck.match ·attributes· a child by. Reading them through
// this capability keeps one encoding of each (STYLE T4) without widening
// SchemaView's exported surface. Under a view without it, which
// validate.Validator.Schema never returns, the mapping determines no type
// governing an element below the root (walker.govern), and every "$type"
// there declines (walker.instanceGoverned).
type governor interface {
	LocallyDeclaredElementType(within xsd.ComplexType, name xsd.QName) (xsd.TypeDefinition, bool)
	ValidlySubstitutable(sub, super xsd.TypeDefinition, blocked []xsd.DerivationMethod) (bool, error)
	ContentMatcher(t xsd.ComplexType) (*xsd.Matcher, bool)
}

// anyTypeRef names ·xs:anyType·, the type a ·laxly assessed· element is
// validated against (key-lva).
var anyTypeRef = xsd.TypeDefinitionRef{Name: xsd.QName{Space: xsd.XMLSchemaNS, Local: "anyType"}}

// govern makes ts, which may be empty, the types that may govern o's owner
// (object.governing), and readies the attribution of the owner's children
// under each: a matcher over its element content, where it has one
// (governingType.matcher). It marks the children unattributed
// (object.unattributed) where xsd.Schema.ContentMatcher declines the content
// of one of them, or the schema view lacks the governor capability.
func (w *walker) govern(o *object, ts []xsd.TypeDefinition) {
	if len(ts) == 0 {
		return
	}
	o.governing = make([]governingType, 0, len(ts))
	for _, t := range ts {
		o.governing = append(o.governing, governingType{typ: t})
	}
	g, ok := w.schema.(governor)
	if !ok {
		o.unattributed = true
		return
	}
	for i := range o.governing {
		ct, ok := o.governing[i].typ.(xsd.ComplexType)
		if !ok {
			continue
		}
		if _, element := ct.ContentType().(xsd.ElementContent); !element {
			continue
		}
		m, ok := g.ContentMatcher(ct)
		if !ok {
			o.unattributed = true
			return
		}
		o.governing[i].matcher = m
	}
}

// governing is each type that may be the ·governing type definition· of an
// element that carries no "$type" naming a type, as validate's walk
// determines it (walk.childGoverning), given d, the declaration its members
// are classified by, named as the element is, declared, d's {type
// definition}, and at, where it stands among its parent's members. The
// root's is d's, walker.declarationType: d is its ·governing element
// declaration·; where d carries a {type table}, walker.settled or walker.take
// replaces it with the one type the table selects once the root's attributes
// are read. Below the root it is, for each type P that may govern the parent,
// what governingUnder reads off what P ·attributes· it to (at.a,
// object.attribute), each type once (appendTypes).
//
// The ·attribution· is cvc-accept's, unique for each child sequence (the Note
// under cos-nonambig, key-vpath), so under one P one declaration or none
// governs; whether its verdict is charged — cvc-complex-type clause 5 against
// a ·locally declared type·, or the parent's cvc-complex-content — leaves it
// governing. The engine selects the one P that governs the parent, so the
// one that governs the element is among these. It is empty where the mapping
// cannot attribute the element (at.attributed) and where, under some P,
// walker.resolvedType or the declaration, the root's or an element
// particle's, gives no type.
func (w *walker) governing(d xsd.ElementDeclaration, declared xsd.TypeDefinition, at placement) []xsd.TypeDefinition {
	if at.parent == nil {
		return w.declarationType(d)
	}
	g, ok := w.schema.(governor)
	if !ok || !at.attributed {
		return nil
	}
	var types []xsd.TypeDefinition
	for i, p := range at.parent.governing {
		under := w.governingUnder(g, d.Name(), declared, p.typ, at.a[i])
		if len(under) == 0 {
			return nil
		}
		types = appendTypes(types, under)
	}
	return types
}

// governingUnder is each type that may govern an element named name, whose
// members are classified under declared, where p governs its parent and
// ·attributes· it to a:
//
//   - an ·element particle·'s declaration D: D's {type definition} where the
//     element's name is D's (key-governing-ed clause 2), and otherwise the
//     type of the ·substituting declaration· its name ·resolves· to
//     (cvc-accept clause 2.3.2), walker.resolvedType;
//   - a non-skip ·wildcard particle·: the type walker.resolvedType gives the
//     name (key-governing-ed clause 3, key-lva);
//   - a non-skip {open content}, or nothing — p simple, its {content type}
//     empty or simple, or the element or a sibling before it rejected, which
//     charges the parent: its ·locally declared type· within p where that is
//     non-·absent· (key-ldt-elem, key-governing-ed clause 4.3,
//     key-governing-type-elem clause 7), and the type walker.resolvedType
//     gives the name otherwise (key-governing-ed clause 4, key-lva);
//   - a skip wildcard or a skip {open content}: none, the element being
//     ·skipped· and nothing below it assessed (cvc-assess-elt clause 2); declared
//     stands for it, which only names its descendants.
//
// A declaration carrying a {type table} gives each type it can select
// (walker.declarationType).
func (w *walker) governingUnder(g governor, name xsd.QName, declared, p xsd.TypeDefinition, a xsd.Attribution) []xsd.TypeDefinition {
	switch a := a.(type) {
	case xsd.ElementDeclaration:
		if a.Name() != name {
			return w.resolvedType(name)
		}
		return w.declarationType(a)
	case xsd.Wildcard:
		if a.ProcessContents() == xsd.ProcessSkip {
			return []xsd.TypeDefinition{declared}
		}
		return w.resolvedType(name)
	case *xsd.OpenContent:
		if a.Wildcard().ProcessContents() == xsd.ProcessSkip {
			return []xsd.TypeDefinition{declared}
		}
	}
	if ct, ok := p.(xsd.ComplexType); ok {
		if ldt, local := g.LocallyDeclaredElementType(ct, name); local {
			return []xsd.TypeDefinition{ldt}
		}
	}
	return w.resolvedType(name)
}

// resolvedType is each type an element named name that carries no xsi:type
// may take from the declaration its name ·resolves· to (key-governing-ed
// clauses 3 and 4), walker.declarationType, or ·xs:anyType· where it resolves
// to none (key-lva).
func (w *walker) resolvedType(name xsd.QName) []xsd.TypeDefinition {
	if d, ok := w.schema.Element(name); ok {
		return w.declarationType(d)
	}
	t, ok := w.schema.ResolvedType(anyTypeRef)
	if !ok {
		return nil
	}
	return []xsd.TypeDefinition{t}
}

// declarationType is each type that governs an element d is the ·governing
// element declaration· of where it carries no xsi:type: d's {type
// definition}, and, where d carries a {type table}, each type the table can
// select instead (key-governing-type-elem clause 4, key-selected-type clause
// 1, key-cta-select) — each {alternatives} member's {type definition}, then
// the {default type definition}'s — each named type once (appendTypes). It is
// one type where every type the table can select is one named type, whichever
// the engine selects, and empty where a type does not resolve, which
// declaredType's marker rules unreachable.
//
// The mapping evaluates no {test} here: where the table can select several
// types, a child's "$type" is settled under each (walker.instanceGoverned),
// and the children are ·attributed· under each (object.attribute), but at the
// root, whose selection walker.settled or walker.take reads.
func (w *walker) declarationType(d xsd.ElementDeclaration) []xsd.TypeDefinition {
	table, ok := d.TypeTable()
	if !ok {
		t, ok := w.schema.ResolvedType(d.TypeDefinition())
		if !ok {
			return nil
		}
		return []xsd.TypeDefinition{t}
	}
	var types []xsd.TypeDefinition
	for _, alt := range tableAlternatives(table) {
		t, ok := w.schema.ResolvedType(alt.TypeDefinition())
		if !ok {
			return nil
		}
		types = appendTypes(types, []xsd.TypeDefinition{t})
	}
	return types
}

// tableAlternatives is every Type Alternative of table in the order
// key-cta-select tries them: its {alternatives}, then its {default type
// definition}.
func tableAlternatives(table xsd.TypeTable) []xsd.TypeAlternative {
	return append(table.Alternatives(), table.DefaultTypeDefinition())
}

// appendTypes appends to types each of ts no member of types is the same
// named type as (sameNamed), so that a set of types that may govern an
// element holds each named type once however deeply {type table}s nest above
// it. An anonymous type is appended however often it recurs. Every reader
// settles its answer under each member alike, so a repeat changes no answer.
func appendTypes(types, ts []xsd.TypeDefinition) []xsd.TypeDefinition {
	for _, t := range ts {
		if !slices.ContainsFunc(types, func(u xsd.TypeDefinition) bool { return sameNamed(t, u) }) {
			types = append(types, t)
		}
	}
	return types
}

// openContent reports whether t's {content type} has an {open content} whose
// {wildcard}'s {process contents} is not skip.
func openContent(t xsd.ComplexType) bool {
	ec, ok := t.ContentType().(xsd.ElementContent)
	return ok && ec.OpenContent != nil && ec.OpenContent.Wildcard().ProcessContents() != xsd.ProcessSkip
}

// instanceGoverned is the type the members of e are classified under where
// its xsi:type typ names instance: the ·governing type definition· e takes
// wherever its verdict stands uncharged. At the root, which has a ·governing
// element declaration·, that is instance, which ·overrides· its type or is
// charged under cvc-elt clause 4. Below it, it is the type instanceUnder
// settles on under each of parent.governing, the types that may be the
// parent's ·governing type definition·, and instance under each where every
// ·attribution· it may give e charges it — the cvc-elt clause 4 and
// cvc-complex-type clause 5 branches included — where every one settles on one
// type (oneType). The engine selects the one that governs the parent, so
// whichever it is, e's members are classified under that type.
//
// It DECLINES typ where parent.governing is empty, the mapping determining no
// type that governs the parent, and where two of parent.governing settle on
// different types, which a {type table} below the root makes several
// (walker.declarationType); a root's is the one type it selects
// (walker.settled, walker.take). It declines too where ·override· is not
// decided, an error of xsd.Schema.ValidlySubstitutable unreachable for a
// finalized Schema, which the engine declines for the XML twin as well
// (walk.instanceOverride, walk.localGovernance), so no marker tracks it.
//
// The parent's governing types are empty where the schema view lacks the
// governor capability, which validate.Validator.Schema never returns, and
// where xsd.Schema.ContentMatcher declines the content model of a type that
// may govern the grandparent (object.unattributed).
//
// GAP(jsonsrc): a "$type" declines where the types its parent's or an
// ancestor's {type table}, below the root, can select settle it on different
// types, where the engine assesses the XML twin under the one it selects: the
// mapping computes no [inherited attributes], which a {test} reads beside the
// element's own (selection). Owned by #2699, which computes them.
//
// GAP(jsonsrc): a "$type" declines where the content model of its
// grandparent's ·governing type definition· is one xsd.Schema.ContentMatcher
// declines (its GAP(xsd) marker, the maxPartitionStates ceiling RULED
// permanent there), so the parent's ·attribution· is not computed; validate
// walks the parent undecided (its GAP(validate) marker in walk.child), so the
// engine assigns the twin's subtree no ·governing type definition· either.
// Owned by #2670, which retires with #1601's ruling.
func (w *walker) instanceGoverned(e *element, typ *attribute, instance xsd.TypeDefinition, parent *object) (xsd.TypeDefinition, error) {
	if parent == nil {
		return instance, nil
	}
	g, ok := w.schema.(governor)
	if !ok || len(parent.governing) == 0 {
		return nil, w.declinef(typ.loc, "the \"$type\" member of %s: the mapping determines no type that governs %s, so the type governing %s is not settled",
			e.name, parent.owner, e.name)
	}
	var settled xsd.TypeDefinition
	for i, p := range parent.governing {
		td, uncharged, err := w.instanceUnder(g, p.typ, e.name, instance)
		if err != nil {
			return nil, w.declinef(typ.loc, "the \"$type\" member of %s: whether the type it names ·overrides· the one %s would otherwise take could not be settled: %v",
				e.name, e.name, err)
		}
		if !uncharged {
			td = instance
		}
		if i > 0 && !oneType(td, settled) {
			return nil, w.declinef(typ.loc, "the \"$type\" member of %s: the types the {type table} of %s or of an ancestor can select settle the type governing %s differently; the mapping reads no ·selected type definition· below the root",
				e.name, parent.owner, e.name)
		}
		settled = td
	}
	return settled, nil
}

// instanceUnder is the ·governing type definition· an element named name
// whose xsi:type names instance takes under p, its parent's ·governing type
// definition·, and true; or false where every ·attribution· p may give it
// charges it, the element particle's, the ·wildcard particle·'s and the {open
// content}'s alike. It reads none of them off the element's actual
// ·attribution·, so it settles the type of a charged element as of an
// uncharged one:
//
//   - with a non-·absent· ·locally declared type· L within p (key-ldt-elem):
//     instance where it ·overrides· L ·validly substitutable without
//     limitation· (key-overrides clause 2; no blocking keyword is passed, and
//     L's own {prohibited substitutions} join inside
//     xsd.Schema.ValidlySubstitutable), as the engine settles it for an {open
//     content}'s child (walk.localGovernance). An element particle's
//     declaration has L's type, and instance ·overrides· it under the
//     declaration's {disallowed substitutions} too or cvc-elt clause 4 is
//     charged; where that declaration carries a {type table}, instance
//     ·overrides· the type it selects or clause 4 is charged, and governs
//     where it does. A ·wildcard particle·'s top-level declaration is
//     ·overridden· by instance or clause 4 is charged. Where instance does not
//     ·override· L: L under a non-skip {open content} (key-governing-ed clause
//     4.3 leaves no declaration to charge clause 4 against,
//     key-governing-type-elem clause 7), and false otherwise — an element
//     particle charges cvc-elt clause 4, and a ·wildcard particle· clause 4,
//     or, wherever instance governs, a tabled element particle's included,
//     cvc-complex-type clause 5 is charged against L;
//   - with L ·absent·: false where the name ·resolves· to a top-level
//     declaration with no {type table}, whose {type definition} instance does
//     not ·override· under its {disallowed substitutions} — a wildcard's
//     ·attribution· makes it the ·governing element declaration·
//     (key-governing-ed clauses 3 and 4), and cvc-elt clause 4 is charged —
//     and instance otherwise, which then ·overrides· the type that
//     declaration selects, or is charged, or governs with no declaration
//     (key-governing-type-elem clause 8).
//
// Under a simple p it is false: the element is charged as its child (cvc-type
// clause 3.1.2). The error is ValidlySubstitutable's.
func (w *walker) instanceUnder(g governor, p xsd.TypeDefinition, name xsd.QName, instance xsd.TypeDefinition) (xsd.TypeDefinition, bool, error) {
	ct, ok := p.(xsd.ComplexType)
	if !ok {
		return nil, false, nil
	}
	if ldt, local := g.LocallyDeclaredElementType(ct, name); local {
		overrides, err := g.ValidlySubstitutable(instance, ldt, nil)
		if err != nil {
			return nil, false, err
		}
		if overrides {
			return instance, true, nil
		}
		if openContent(ct) {
			return ldt, true, nil
		}
		return nil, false, nil
	}
	d, ok := w.schema.Element(name)
	if !ok {
		return instance, true, nil
	}
	if _, table := d.TypeTable(); table {
		return instance, true, nil
	}
	declared, ok := w.schema.ResolvedType(d.TypeDefinition())
	if !ok {
		return instance, true, nil
	}
	overrides, err := g.ValidlySubstitutable(instance, declared, d.DisallowedSubstitutions())
	if err != nil {
		return nil, false, err
	}
	if !overrides {
		return nil, false, nil
	}
	return instance, true, nil
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

// scalarValue reports whether tok, the first token of a value, is a scalar or
// null: a value whose element walker.scalar or walker.nilled builds from its
// name alone.
func scalarValue(tok json.Token) bool {
	_, ok := scalarLexical(tok)
	return ok || tok == nil
}

// scalar builds a scalar-valued element named name in the scope sc: lexical
// is its simple content. An empty lexical is no text at all, as <size/>
// carries none.
func (w *walker) scalar(name xsd.QName, loc xsderr.Loc, lexical string, tokLoc xsderr.Loc, sc *scope) *element {
	kids := &scalarChildren{}
	if lexical != "" {
		kids.text = &text{data: lexical, loc: tokLoc}
	}
	return &element{w: w, name: name, loc: loc, scope: sc, kids: kids}
}

// nilled builds a null-valued element named name in the scope sc: one
// attribute, xsi:nil with the lexical form true, located at the null token
// tokLoc, and no [[children]] (package doc, "The mapping"). It reads no
// declaration: whether the element is ·nilled· (key-nilled) and whether
// cvc-elt clause 3.1 or 3.2.3.2 charges it are the engine's, as for the XML
// twin <e xsi:nil="true"/>. Where it is ·nilled·, the xsi:nil keeps it from
// cvc-elt clause 5.1's {value constraint}, which an empty element without one
// would take.
func (w *walker) nilled(name xsd.QName, loc, tokLoc xsderr.Loc, sc *scope) *element {
	attrs := []validate.Attribute{&attribute{name: xsiNil, value: "true", loc: tokLoc}}
	return &element{w: w, name: name, loc: loc, attrs: attrs, scope: sc, kids: &scalarChildren{}}
}

// object is what an object-valued element named owner classifies its members
// against, from td, the type declaredElement.object chose: a complex type's
// contained declarations, {attribute uses}, element wildcards and {attribute
// wildcard}; a simple type has none, so every member of its object is a child
// no wildcard names (wildcardMember) and an empty object is an element with
// no children.
func (w *walker) object(owner xsd.QName, td xsd.TypeDefinition) *object {
	switch t := td.(type) {
	case xsd.ComplexType:
		o := &object{owner: owner, contained: w.schema.ContainedElementDeclarations(t), uses: t.AttributeUses()}
		o.elementWildcards = w.schema.ContainedWildcards(t)
		switch ct := t.ContentType().(type) {
		case xsd.SimpleContent:
			o.simpleContent = true
		case xsd.ElementContent:
			if ct.OpenContent != nil {
				o.elementWildcards = append(o.elementWildcards, ct.OpenContent.Wildcard())
			}
		}
		if aw, ok := t.AttributeWildcard(); ok {
			o.attributeWildcard = &aw
		}
		return o
	case *xsd.SimpleType:
		return &object{owner: owner}
	}
	panic(fmt.Sprintf("jsonsrc: TypeDefinition arm %T", td))
}

// attributes reads the members of o's object that classify as attributes,
// their prefixes read through sc, the element's scope, from tok at loc, the
// first after its reserved members, each an attribute of the element after
// typ, its xsi:type where a "$type" presents one and nil where none does; and
// returns them, typ first, with what it read past the run, for the element's
// members cursor to start from — the member that ends the run, the first
// whose key names a child or whose value is an array, as far as walker.value
// read it, or the object's close, then any member walker.readAhead read after
// it — and the object the members after the run are classified against.
//
// The engine reads an element's [[attributes]] at its entry, before its
// [[children]], and again after them, so they must be whole and unchanged
// from the first read. Reading the leading run as the element is built makes
// them so without holding any of the document but the run and what
// walker.lookAhead reads past it (STYLE P4); an attribute member after a
// child member therefore declines, under members.key's GAP(jsonsrc) marker.
//
// Each attribute is named by its use's {attribute declaration}'s expanded name,
// or by the namespace the {attribute wildcard} fixes (wildcardMember), and
// located at its key. Its [[normalized value]] is its scalar's lexical
// form (scalarLexical) unchanged, and the empty string is an attribute whose
// value is empty. XML 1.0 §3.3.3 maps a literal #x9, #xA or #xD to #x20 and
// keeps one a character reference names; a JSON string holds none literally
// (RFC 8259 §7 requires a control character to be escaped), and its escape is
// read as the character reference would be. It declines a null, object or
// array value, and a key repeated in the run. Neither has an XML twin, so no
// marker tracks them: an attribute's value is a string, and an element carries
// one attribute of each expanded name (XML 1.0 §3.1, Unique Att Spec).
//
// Where table is nil, each member is classified under o and alike under each
// of o.selectable (walker.classify), and the members after the run are
// classified so against o. Where table is the root's {type table}
// (selection), the run is the one the table's own selection settles
// (walker.settled): each member is classified under every candidate still
// reading attributes — o and each of o.selectable at the start — a candidate
// that declines it dropping from the run where no reading of the run selects
// a type that declines it (walker.candidates), and where one classifies it as
// no attribute, the type the attributes before it select decides whether the
// run ends there. Where a member leaves more than one reading of the run open
// — a candidate dropping only by readings that read members after it, a
// candidate taking it as an attribute where the selected type ends the run
// on it, or candidates taking it as attributes of different names — the run
// is the one reading walker.readAhead settles by reading on.
func (w *walker) attributes(o *object, sc *scope, tok json.Token, loc xsderr.Loc, typ *attribute, table *xsd.TypeTable) ([]validate.Attribute, []peeked, *object, error) {
	r := &attributeRun{owner: o, table: table, typ: typ}
	r.live = r.candidates()
	for {
		key, ok := tok.(string)
		if !ok {
			final, _, err := w.settled(r, nil, loc)
			if err != nil {
				return nil, nil, nil, err
			}
			return r.attributes(), []peeked{{tok: tok, loc: loc}}, final, nil
		}
		if isReserved(key) {
			return nil, nil, nil, w.reserved(key, loc)
		}
		v, err := w.value(sc, key)
		if err != nil {
			return nil, nil, nil, err
		}
		next := &runMember{key: key, loc: loc, val: v}
		var unsettled error
		if v.tok != json.Delim('[') {
			if next.k, err = w.keyName(key, v.scope, false, loc); err != nil {
				return nil, nil, nil, err
			}
			if unsettled, err = w.candidates(r, next); err != nil {
				return nil, nil, nil, err
			}
		}
		if unsettled == nil && next.child() {
			var final *object
			if final, unsettled, err = w.settled(r, next, loc); err != nil {
				return nil, nil, nil, err
			}
			if final != nil {
				return r.attributes(), []peeked{{tok: tok, loc: loc, val: v}}, final, nil
			}
		}
		if unsettled == nil {
			if unsettled, err = w.extend(r, next); err != nil {
				return nil, nil, nil, err
			}
		}
		if unsettled != nil {
			return w.readAhead(r, next, sc, unsettled)
		}
		if tok, loc, err = w.token(); err != nil {
			return nil, nil, nil, unexpectedEOF(err, loc)
		}
	}
}

// attributeRun is the leading run of attribute members walker.attributes
// reads: owner, the object the element's members are classified against
// before any selection; live, the candidates still reading attributes, owner
// and each of owner.selectable at the start, less each one that took a member
// as no attribute (walker.extend) or declined one (walker.candidates); table,
// the {type table} whose selection settles the run, nil where none does
// (selection); typ, the element's xsi:type, nil where no "$type" presents
// one; and run, the members taken as attributes so far, which the selected
// type classifies again (walker.agrees, walker.judge).
type attributeRun struct {
	owner *object
	live  []*object
	table *xsd.TypeTable
	typ   *attribute
	run   []*runMember
}

// candidates is every type r's members are classified under at the start of
// its run: r.owner, then each of r.owner.selectable. Every type a {type
// table} selects is among them.
func (r *attributeRun) candidates() []*object {
	return append([]*object{r.owner}, r.owner.selectable...)
}

// attributes is the element's attributes so far: r.typ, where it is not nil,
// then the attribute each member of r.run is (runMember.asAttribute).
func (r *attributeRun) attributes() []validate.Attribute {
	attrs := make([]validate.Attribute, 0, len(r.run)+1)
	if r.typ != nil {
		attrs = append(attrs, r.typ)
	}
	for _, m := range r.run {
		attrs = append(attrs, m.asAttribute())
	}
	return attrs
}

// has reports whether a member of r's run is the attribute named name. r.typ
// is never that attribute: the mapping names no member an attribute in the
// xsi namespace (walker.classify's declines).
func (r *attributeRun) has(name xsd.QName) bool {
	return slices.ContainsFunc(r.run, func(m *runMember) bool { return m.attributeName() == name })
}

// runMember is one member walker.attributes or walker.lookAhead reads: its
// key, read as k, at loc; its value as far as walker.value read it; and ms,
// its classification under each of the run's live candidates, in their order,
// once walker.candidates has dropped those that decline it, or, where
// walker.take takes it into the run, under the type the reading it takes
// selects alone; nil for an array value, whose items are classified one by
// one, and for a member walker.lookAhead read that no reading takes.
type runMember struct {
	key string
	k   keyName
	loc xsderr.Loc
	val value
	ms  []member
}

// attributeName is the expanded name of the attribute m is where the run
// takes it: the one every candidate that takes it gives it (walker.extend),
// or the one the reading walker.take takes gives it.
func (m *runMember) attributeName() xsd.QName {
	for _, x := range m.ms {
		if a, ok := x.(attributeMember); ok {
			return a.name
		}
	}
	panic("jsonsrc: attributeName of a member no candidate takes as an attribute")
}

// asAttribute is the attribute m is where the run takes it (walker.extend,
// walker.take), named by attributeName, valued by its scalar's lexical
// form and located at its key.
func (m *runMember) asAttribute() *attribute {
	lexical, ok := scalarLexical(m.val.tok)
	if !ok {
		panic("jsonsrc: asAttribute of a member whose value is no scalar")
	}
	return &attribute{name: m.attributeName(), value: lexical, loc: m.loc}
}

// child reports whether some live candidate classifies m as no attribute, or
// none classifies it at all: an array value, which none classifies, and a
// member every live candidate declined (walker.candidates).
func (m *runMember) child() bool {
	return len(m.ms) == 0 || slices.ContainsFunc(m.ms, func(x member) bool {
		_, ok := x.(attributeMember)
		return !ok
	})
}

// attribute reports whether some live candidate classifies m as an attribute.
func (m *runMember) attribute() bool {
	return slices.ContainsFunc(m.ms, func(x member) bool {
		_, ok := x.(attributeMember)
		return ok
	})
}

// candidates classifies next, a member of r's run whose value is no array,
// under each live candidate of r, in their order, into next.ms.
//
// Where r reads no selection, it DECLINES next where one candidate declines
// it (walker.membersOf). Where r reads one, the candidates that decline next
// drop from r.live, and next.ms holds the others' classifications, where no
// reading of the run selects a type that declines next (walker.droppable):
// the root is assessed under the type its leading attributes select alone
// (key-selected-type clause 1, key-cta-select), so a candidate no reading
// selects governs no twin. Where every live candidate declines next, next.ms
// is empty and the run ends at it (runMember.child) under walker.settled's
// declines. Where next is a scalar another member follows, the readings that
// take next and members after it as attributes select their types from
// members not yet read, so it drops none and returns unsettled, the decline
// walker.readAhead gives where reading past next settles no one reading.
// Otherwise it DECLINES next, with the first live candidate's own decline
// where that one declines it, and undetermined's where it does not; that
// decline is unsettled's too.
//
// GAP(jsonsrc): in the attribute run of a root whose declaration carries a
// {type table}, a member some live candidate declines declines the root where
// a type some reading of the run selects declines it — a reading that ends
// the run at or before the member, or, for a scalar member, the one that
// takes it as an attribute too, under any name an attribute keyed so can
// take — though the engine assesses each XML twin, and that reading may place
// another member as its type does not, leaving one other reading the only
// consistent one. RULED permanent by #2703: the type a reading selects
// governs the root under it (key-governing-type-elem clause 4), so its
// decline of the member is a decline of that reading, and the mapping maps
// only where no such reading remains. A reading that takes a member after it
// as an attribute too is walker.readAhead's, which counts a reading only where
// its own selected type does not rule it out.
//
// GAP(jsonsrc): the same decline stands where the engine withholds the type
// one of those readings selects, a {test} it reaches being outside its
// §3.12.6 compiler's subset, the mapping then not knowing which type governs
// that reading. RULED permanent by #2702: key-cta-select tries the Type
// Alternatives in order and selects the first whose {test} succeeds, and
// key-cta-ta-select clause 2 makes a {test} false only where it raises an
// error, which a {test} outside the subset does not, so no alternative after
// the withheld one is tried and dropping the candidate would select past it.
func (w *walker) candidates(r *attributeRun, next *runMember) (unsettled, err error) {
	if r.table == nil {
		ms, err := w.membersOf(r.live, next.key, next.k, next.loc)
		next.ms = ms
		return nil, err
	}
	live := make([]*object, 0, len(r.live))
	ms := make([]member, 0, len(r.live))
	var first error
	for i, o := range r.live {
		m, err := w.memberOf(o, next.key, next.k, next.loc)
		if err != nil && i == 0 {
			first = err
		}
		if err != nil {
			continue
		}
		live, ms = append(live, o), append(ms, m)
	}
	next.ms = ms
	if len(live) == len(r.live) {
		return nil, nil
	}
	decline := first
	if decline == nil {
		decline = w.undetermined(r.owner, next.key, next.loc)
	}
	if !w.droppable(r, next) {
		return nil, decline
	}
	if _, scalar := scalarLexical(next.val.tok); scalar && w.dec.More() {
		return decline, nil
	}
	r.live = live
	return nil, nil
}

// droppable reports whether no reading of r's run up to next selects a type
// that declines next, the member at position j of the run, a reading k taking
// the first k members as the root's attributes, beside r.typ, and the rest as
// its children (key-cta-ta-select clause 1.1.2 reads [attributes] alone):
//
//   - each k up to j, whose selection reads no member from next on, and so
//     stands whatever next is;
//   - k = j+1, where next is a scalar, the one value an attribute takes.
//     Next's name under that reading is the type's that reading selects,
//     which may be the very type that declines next, so the selection is read
//     under each name attributeNames gives it.
//
// The readings past j+1, which read members after next, are walker.readAhead's
// (walker.candidates). It is false where the engine withholds a selection
// (walker.classifiedUnder).
func (w *walker) droppable(r *attributeRun, next *runMember) bool {
	attrs := r.attributes()
	for k := len(attrs) - len(r.run); k <= len(attrs); k++ {
		if !w.classifiedUnder(r, attrs[:k], next) {
			return false
		}
	}
	lexical, ok := scalarLexical(next.val.tok)
	if !ok {
		return true
	}
	for _, name := range attributeNames(*r.table, next.k) {
		taken := append(attrs[:len(attrs):len(attrs)], validate.Attribute(&attribute{name: name, value: lexical, loc: next.loc}))
		if !w.classifiedUnder(r, taken, next) {
			return false
		}
	}
	return true
}

// classifiedUnder reports whether the type r's {type table} selects for the
// attributes attrs (walker.selected, key-cta-select) classifies next, its
// decline being the one fact read: false where it declines next, and where the
// engine withholds the selection. Every type the table can select resolves,
// walker.selectable having declined the root before its run otherwise, so the
// false an unresolved one answers is never read.
func (w *walker) classifiedUnder(r *attributeRun, attrs []validate.Attribute, next *runMember) bool {
	slot, decided := w.selected(*r.table, attrs, nil)
	if !decided {
		return false
	}
	s, ok := w.schema.ResolvedType(slot)
	if !ok {
		return false
	}
	_, err := w.memberOf(w.object(r.owner.owner, s), next.key, next.k, next.loc)
	return err == nil
}

// attributeNames is an expanded name for an attribute keyed k in each class
// of names the {test}s of table cannot tell apart: the one a prefixed key
// spells; for an unprefixed key, its local name in no namespace, in each
// namespace a {test}'s {namespace bindings} bind — the xml namespace among
// them wherever a name test can spell it — and in one namespace none binds,
// longer than every one of them. Every name test a {test} the engine
// evaluates carries reads an attribute's namespace as no namespace (an
// unprefixed name) or one of those bindings, or does not read it (`*`,
// `*:local`) (xpath.CompileCTATest's subset), so each class selects one type.
func attributeNames(table xsd.TypeTable, k keyName) []xsd.QName {
	if k.prefixed {
		return []xsd.QName{k.name}
	}
	spaces := []string{""}
	for _, alt := range table.Alternatives() {
		test, ok := alt.Test()
		if !ok {
			continue
		}
		for _, b := range test.NamespaceBindings() {
			if !slices.Contains(spaces, b.Namespace()) {
				spaces = append(spaces, b.Namespace())
			}
		}
	}
	spaces = append(spaces, strings.Join(spaces, " ")+" unbound")
	names := make([]xsd.QName, 0, len(spaces))
	for _, space := range spaces {
		names = append(names, xsd.QName{Space: space, Local: k.name.Local})
	}
	return names
}

// extend takes next, a member some live candidate of r classifies as an
// attribute, into the run: the live candidates become those that do, and
// next the attribute of the name they give it.
//
// It DECLINES next where the candidates that take it give it different
// expanded names and r reads no selection (notAlike), and where its value is
// not a scalar (notScalar). One that takes it as no attribute has dropped
// from the run, where walker.settled let it go on; where r reads no
// selection, settled declines it first. Where r reads a selection and the
// candidates that take next give it different expanded names, the run holds
// no one name for it: it takes nothing and returns unsettled, the decline
// walker.readAhead gives where reading past next settles no one reading.
func (w *walker) extend(r *attributeRun, next *runMember) (unsettled, err error) {
	var live []*object
	var name xsd.QName
	for i, m := range next.ms {
		a, ok := m.(attributeMember)
		if !ok {
			continue
		}
		if len(live) > 0 && a.name != name && r.table == nil {
			return nil, w.notAlike(r.owner, next.key, next.loc)
		}
		if len(live) > 0 && a.name != name {
			return w.declinef(next.loc, "member %q of %s is an attribute of the expanded names %s and %s under two types its {type table} can select, and no one reading of its attribute members settles which",
				next.key, r.owner.owner, name, a.name), nil
		}
		live, name = append(live, r.live[i]), a.name
	}
	if r.has(name) {
		return nil, w.declinef(next.loc, "attribute member %q repeats in the object of %s; an element carries one attribute of each expanded name, so give it once", next.key, r.owner.owner)
	}
	if _, ok := scalarLexical(next.val.tok); !ok {
		return nil, w.notScalar(r.owner, next.key, next.val.loc)
	}
	r.live = live
	r.run = append(r.run, next)
	return nil, nil
}

// readAhead settles r's run, at a root whose {type table} it reads the
// selection of, where walker.candidates, walker.settled or walker.extend
// leaves more than one reading of it open at next: it reads the object on
// from next (walker.lookAhead), finds the one reading its own selected type
// classifies as it reads (walker.oneReading), and takes that one
// (walker.take). It returns what walker.attributes does, and unsettled, the
// decline of the step that called it, where no one reading is so settled.
//
// The type is selected once, against the element's whole [attributes]
// (key-cta-ta-select clauses 1.1.2 and 1.2), so each reading — the first k
// members taken as attributes, the rest as children — is an XML twin of its
// own, assessed under the type its attributes select (key-selected-type
// clause 1, key-cta-select). The mapping names a member an attribute only
// where the type the root is assessed under takes it as one, so a reading
// stands only where that type takes its attributes as it does, under their
// names, and the member after them as no attribute.
//
// GAP(jsonsrc): the root declines where two readings each stand, though the
// engine assesses each XML twin. RULED permanent by #2700: nothing in
// Structures ranks the readings, so naming either would guess a mapping.
//
// GAP(jsonsrc): the root declines where the type a reading selects declines
// a member that reading reads, its attributes or the member after them, and
// no other member rules that reading out, though the engine assesses each XML
// twin. RULED permanent by #2703: the type a reading selects governs the root
// under it (key-governing-type-elem clause 4), so its decline of the member
// is a decline of that reading.
//
// GAP(jsonsrc): the root declines where a reading its selected type
// classifies as it reads has no XML twin — that type takes the member after
// the reading's attributes as an attribute while its value is no scalar,
// which the mapping declines as an attribute (notScalar), or the reading's
// attributes repeat an expanded name (XML 1.0 §3.1, Unique Att Spec;
// Namespaces in XML §6.3) — even where another reading stands, though the
// engine assesses that other reading's XML twin. RULED permanent by #2700:
// a reading with no XML twin cannot be assessed by the engine and so cannot
// be excluded, and nothing in Structures ranks the readings.
//
// GAP(jsonsrc): the root declines where a candidate type takes no member of a
// reading otherwise than the reading does but declines so many of its
// attributes that their names, each in every class attributeNames gives it,
// make more than nameBudget assignments, though the engine assesses each XML
// twin. RULED permanent by #2700: the assignments multiply per declined
// member, and the mapping reads a selection at most nameBudget times per
// candidate and reading.
//
// GAP(jsonsrc): the root declines where the engine withholds the type a
// reading selects, a {test} it reaches being outside its §3.12.6 compiler's
// subset, the mapping then not knowing whether that reading stands, though
// the engine assesses each XML twin. RULED permanent by #2702: key-cta-select
// tries the Type Alternatives in order and selects the first whose {test}
// succeeds, and key-cta-ta-select clause 2 makes a {test} false only where it
// raises an error, which a {test} outside the subset does not, so no
// alternative after the withheld one is tried and standing the reading or
// ruling it out would select past it.
//
// GAP(jsonsrc): the root declines where no reading stands, though the engine
// assesses each XML twin, whichever members it takes as attributes: the
// mapping names a member an attribute only where the type the root is
// assessed under classifies it as one, as walker.settled's marker of the same
// ruling says. RULED permanent by #2701: Structures assesses an element whose
// [attributes] are given, each reading is an element of its own, and no clause
// ranks the readings, so the mapping has no reading to name.
func (w *walker) readAhead(r *attributeRun, next *runMember, sc *scope, unsettled error) ([]validate.Attribute, []peeked, *object, error) {
	la, err := w.lookAhead(r, next, sc)
	if err != nil {
		return nil, nil, nil, err
	}
	rd, ok := w.oneReading(r, la)
	if !ok {
		return nil, nil, nil, unsettled
	}
	return w.take(r, la, rd)
}

// nameBudget bounds the name assignments walker.oneReading reads one
// candidate's selection under for one reading (walker.readAhead's
// GAP(jsonsrc) marker).
const nameBudget = 64

// lookahead is what walker.lookAhead read of a root's object from the member
// after its run on: items, that member and each one after it, in order, each
// but perhaps the last valued by a scalar; and closed, the object's close,
// where the reading reached it, and nil otherwise.
type lookahead struct {
	items  []*runMember
	closed *peeked
}

// scalars is the number of la's leading items valued by a scalar: the members
// a reading can take as attributes.
func (la lookahead) scalars() int {
	for i, it := range la.items {
		if _, ok := scalarLexical(it.val.tok); !ok {
			return i
		}
	}
	return len(la.items)
}

// lookAhead reads the members of the root's object, of the scope sc, from
// next, the member after r's run, as far as a reading can take them as
// attributes: up to and including the first whose value is no scalar, or the
// object's close, but no further than the member by which every candidate
// type rules out every longer reading, having classified a member of the run
// otherwise than the run took it or one after the run as no attribute. Every
// type the table selects is one of r.candidates, so no reading past that
// member stands. A candidate's decline of a member rules nothing out, being
// the one fact read of it (walker.keepsRun). It holds only the members a
// reading can still take as attributes, and the one after them, as
// walker.attributes holds the run (STYLE P4).
//
// It DECLINES a reserved key and a key keyName declines, as walker.attributes
// does on reaching them.
func (w *walker) lookAhead(r *attributeRun, next *runMember, sc *scope) (lookahead, error) {
	cands := r.candidates()
	out := make([]bool, len(cands))
	for i, c := range cands {
		out[i] = !w.keepsRun(c, r.run)
	}
	la := lookahead{items: []*runMember{next}}
	for {
		last := la.items[len(la.items)-1]
		if _, ok := scalarLexical(last.val.tok); !ok {
			return la, nil
		}
		for i, c := range cands {
			m, err := w.memberOf(c, last.key, last.k, last.loc)
			if _, ok := m.(elementMember); err == nil && ok {
				out[i] = true
			}
		}
		if !slices.Contains(out, false) {
			return la, nil
		}
		tok, loc, err := w.token()
		if err != nil {
			return lookahead{}, unexpectedEOF(err, loc)
		}
		key, ok := tok.(string)
		if !ok {
			la.closed = &peeked{tok: tok, loc: loc}
			return la, nil
		}
		if isReserved(key) {
			return lookahead{}, w.reserved(key, loc)
		}
		v, err := w.value(sc, key)
		if err != nil {
			return lookahead{}, err
		}
		rm := &runMember{key: key, loc: loc, val: v}
		if v.tok != json.Delim('[') {
			if rm.k, err = w.keyName(key, v.scope, false, loc); err != nil {
				return lookahead{}, err
			}
		}
		la.items = append(la.items, rm)
	}
}

// reading is one reading of a root's object past its run: the first k of a
// lookahead's items taken as attributes, of names, and the rest as children,
// under typ, the type the run's attributes and those select.
type reading struct {
	k     int
	names []xsd.QName
	typ   xsd.TypeDefinition
}

// fit is how a reading's selected type classifies the members it reads, the
// worst of each member's: consistent, as the reading does; undecided, where
// the mapping cannot settle the reading — the type declines one, or takes the
// one after the attributes as an attribute while its value is no scalar, or
// the reading's selection is withheld, or its attributes repeat a name;
// excluded, where the type classifies one otherwise, which rules the reading
// out whatever any other member is.
type fit int

const (
	consistent fit = iota
	undecided
	excluded
)

// oneReading is the one reading of la, past r's run, that stands (walker.judge),
// and false where none does, where two do, where one is undecided, and where
// a candidate's assignments outnumber nameBudget. It reads each reading's
// selection under the names each candidate type gives its attributes
// (walker.namesUnder): a reading that stands is the one its own selected
// type, a candidate, names so, but for the names of members that type
// declines, which are read in each class attributeNames gives them
// (assignments).
func (w *walker) oneReading(r *attributeRun, la lookahead) (reading, bool) {
	var found []reading
	for k := 0; k <= la.scalars(); k++ {
		if k == len(la.items) && la.closed == nil {
			break
		}
		for _, c := range r.candidates() {
			names, free, ok := w.namesUnder(c, r, la, k)
			if !ok {
				continue
			}
			as, ok := assignments(*r.table, la.items[:k], names, free)
			if !ok {
				return reading{}, false
			}
			for _, names := range as {
				rd, f := w.judge(r, la, k, names)
				if f == undecided {
					return reading{}, false
				}
				if f == consistent && !slices.ContainsFunc(found, rd.same) {
					found = append(found, rd)
				}
			}
		}
	}
	if len(found) != 1 {
		return reading{}, false
	}
	return found[0], true
}

// same reports whether a and b read the same members as attributes, of the
// same names, which select the same type.
func (a reading) same(b reading) bool {
	return a.k == b.k && slices.Equal(a.names, b.names)
}

// keepsRun reports whether c takes each member of run as the attribute the run
// took it as, or declines it: a decline is the one fact read of it, as
// walker.classifiedUnder reads one.
func (w *walker) keepsRun(c *object, run []*runMember) bool {
	for _, rm := range run {
		m, err := w.memberOf(c, rm.key, rm.k, rm.loc)
		if err != nil {
			continue
		}
		if a, ok := m.(attributeMember); !ok || a.name != rm.attributeName() {
			return false
		}
	}
	return true
}

// namesUnder is the name c gives each of the first k items of la as an
// attribute, the zero name for each one c declines, whose index free lists;
// and false where c, as the type a reading k selects, would rule that reading
// out: it takes a member of r's run otherwise than the run took it, one of
// those items as no attribute, or the scalar-valued item after them as an
// attribute.
func (w *walker) namesUnder(c *object, r *attributeRun, la lookahead, k int) ([]xsd.QName, []int, bool) {
	if !w.keepsRun(c, r.run) {
		return nil, nil, false
	}
	names := make([]xsd.QName, k)
	var free []int
	for i, it := range la.items[:k] {
		m, err := w.memberOf(c, it.key, it.k, it.loc)
		if err != nil {
			free = append(free, i)
			continue
		}
		a, ok := m.(attributeMember)
		if !ok {
			return nil, nil, false
		}
		names[i] = a.name
	}
	if k < la.scalars() {
		after := la.items[k]
		m, err := w.memberOf(c, after.key, after.k, after.loc)
		if _, ok := m.(attributeMember); err == nil && ok {
			return nil, nil, false
		}
	}
	return names, free, true
}

// assignments is names, each of whose free indices into items is named in
// turn by each class attributeNames gives that item under table: one
// assignment where none is free, and false where they would number more than
// nameBudget.
func assignments(table xsd.TypeTable, items []*runMember, names []xsd.QName, free []int) ([][]xsd.QName, bool) {
	out := [][]xsd.QName{names}
	for _, i := range free {
		classes := attributeNames(table, items[i].k)
		if len(out)*len(classes) > nameBudget {
			return nil, false
		}
		grown := make([][]xsd.QName, 0, len(out)*len(classes))
		for _, as := range out {
			for _, n := range classes {
				as := slices.Clone(as)
				as[i] = n
				grown = append(grown, as)
			}
		}
		out = grown
	}
	return out, true
}

// judge is the reading of la taking its first k items as attributes, of
// names, beside r's run, and how the type their selection names
// (walker.selected, key-cta-select) fits it: undecided where the engine
// withholds that type or it does not resolve; else the worst fit of each
// member of the run and each of those items, which the type must take as the
// attributes the reading does (walker.fitsAttribute), and of the item after
// them, which it must take as no attribute (walker.fitsChild). A reading that
// fits but repeats an expanded name among its attributes is undecided: it has
// no XML twin, and walker.readAhead's GAP(jsonsrc) marker RULED permanent by
// #2700 tracks the decline, another reading standing or not.
func (w *walker) judge(r *attributeRun, la lookahead, k int, names []xsd.QName) (reading, fit) {
	attrs := r.attributes()
	for i, it := range la.items[:k] {
		lexical, _ := scalarLexical(it.val.tok) // k <= la.scalars
		attrs = append(attrs, &attribute{name: names[i], value: lexical, loc: it.loc})
	}
	slot, decided := w.selected(*r.table, attrs, nil)
	if !decided {
		return reading{}, undecided
	}
	s, ok := w.schema.ResolvedType(slot)
	if !ok {
		return reading{}, undecided
	}
	rd := reading{k: k, names: names, typ: s}
	so := w.object(r.owner.owner, s)
	f := consistent
	taken := make([]xsd.QName, 0, len(r.run)+k)
	for _, rm := range r.run {
		f = max(f, w.fitsAttribute(so, rm, rm.attributeName()))
		taken = append(taken, rm.attributeName())
	}
	for i, it := range la.items[:k] {
		f = max(f, w.fitsAttribute(so, it, names[i]))
		taken = append(taken, names[i])
	}
	if k < len(la.items) && la.items[k].val.tok != json.Delim('[') {
		f = max(f, w.fitsChild(so, la.items[k]))
	}
	if f == consistent && repeats(taken) {
		return rd, undecided
	}
	return rd, f
}

// fitsAttribute is how so classifies m, which a reading takes as the
// attribute name: consistent where so does too, undecided where it declines
// m, and excluded otherwise.
func (w *walker) fitsAttribute(so *object, m *runMember, name xsd.QName) fit {
	got, err := w.memberOf(so, m.key, m.k, m.loc)
	if err != nil {
		return undecided
	}
	if a, ok := got.(attributeMember); ok && a.name == name {
		return consistent
	}
	return excluded
}

// fitsChild is how so classifies m, the member after a reading's attributes:
// consistent where so takes it as no attribute; undecided where it declines
// m, or takes it as an attribute while its value is no scalar, which the
// mapping declines as an attribute (notScalar), a reading with no XML twin
// (walker.readAhead's GAP(jsonsrc) marker RULED permanent by #2700); and
// excluded where it takes a scalar-valued m as an attribute.
func (w *walker) fitsChild(so *object, m *runMember) fit {
	got, err := w.memberOf(so, m.key, m.k, m.loc)
	if err != nil {
		return undecided
	}
	if _, ok := got.(attributeMember); !ok {
		return consistent
	}
	if _, ok := scalarLexical(m.val.tok); ok {
		return excluded
	}
	return undecided
}

// repeats reports whether two of names are one expanded name.
func repeats(names []xsd.QName) bool {
	for i, n := range names {
		if slices.Contains(names[i+1:], n) {
			return true
		}
	}
	return false
}

// take settles r's run on rd, a reading of la: its attributes join the run,
// each named as rd names it, and the members after them are left, unread
// by the run, for the root's members cursor, classified under rd's type
// alone, which governs the root. It returns what walker.attributes does.
func (w *walker) take(r *attributeRun, la lookahead, rd reading) ([]validate.Attribute, []peeked, *object, error) {
	for i, it := range la.items[:rd.k] {
		it.ms = []member{attributeMember{name: rd.names[i]}}
		r.run = append(r.run, it)
	}
	so := w.object(r.owner.owner, rd.typ)
	w.govern(so, []xsd.TypeDefinition{rd.typ})
	ahead := make([]peeked, 0, len(la.items)-rd.k+1)
	for _, it := range la.items[rd.k:] {
		ahead = append(ahead, peeked{tok: it.key, loc: it.loc, val: it.val})
	}
	if la.closed != nil {
		ahead = append(ahead, *la.closed)
	}
	return r.attributes(), ahead, so, nil
}

// settled ends r's run at next, the member at loc that some live candidate
// classifies as no attribute, or at the object's close at loc, where next is
// nil. It returns the object the members from next on are classified against;
// or a nil object, where the run goes on under the candidates that take next
// as an attribute (walker.extend).
//
// Where r reads no selection, the run ends under r.owner (walker.unselected).
// Otherwise it ends where the type S the run's attributes select
// (walker.selected, key-cta-select) classifies each of them as the run did
// and next as no attribute (walker.agrees): the members after it are
// classified against S alone, which governs the root (key-selected-type clause
// 1, key-governing-type-elem clause 4) and is the one type both an XML twin
// of these members and its selection settle on. Where S classifies them
// otherwise, the run goes on under the candidates that take next as an
// attribute. Where S ends the run while a candidate takes next as an
// attribute, a longer reading may be consistent too, so it returns a nil
// object and unsettled, the decline walker.readAhead gives where reading past
// next settles no one reading.
//
// It DECLINES where S classifies the run otherwise and no candidate takes
// next as an attribute, or the object closes; and where the engine withholds
// S, but for a run every candidate classified alike, member by member, which
// ends under r.owner as where r reads no selection.
//
// GAP(jsonsrc): the members of a root whose declaration carries a {type
// table} decline where the type the run's attributes select classifies them,
// or the member that ends the run, otherwise, and no longer run is left,
// though the engine assesses each XML twin, whichever members it takes as
// attributes: the mapping names a member an attribute only where the type
// the root is assessed under classifies it as one, and here none does. RULED
// permanent by #2701: Structures assesses an element whose [attributes] are
// given, each reading is an element of its own, and no clause ranks the
// readings (key-selected-type clause 1 defers to the Type Alternative
// key-cta-select ranks first within one), so the mapping has no reading to
// name.
//
// GAP(jsonsrc): where the engine withholds the type a root's {type table}
// ·conditionally selects·, a {test} it reaches being outside its §3.12.6
// compiler's subset (validate's walk.conditionallySelected, recorded
// Unevaluated under key-cta-ta-select), the members of the root decline
// unless every type the table can select classifies them alike, where the
// engine assesses the XML twin under no ·governing type definition·. RULED
// permanent by #2702: key-cta-select tries the Type Alternatives in order and
// selects the first whose {test} succeeds, and key-cta-ta-select clause 2
// makes a {test} false only where it raises an error, which a {test} outside
// the subset does not, so no alternative after the withheld one is tried and
// classifying the members under any one candidate would select past it.
func (w *walker) settled(r *attributeRun, next *runMember, loc xsderr.Loc) (final *object, unsettled, err error) {
	if r.table == nil {
		return w.unselected(r, next)
	}
	slot, decided := w.selected(*r.table, r.attributes(), nil)
	if !decided {
		if len(r.live) < len(r.owner.selectable)+1 {
			return nil, nil, w.withheld(r.owner, loc)
		}
		return w.unselected(r, next)
	}
	s, ok := w.schema.ResolvedType(slot)
	if !ok {
		return nil, nil, w.declinef(loc, "the type the {type table} of %s ·conditionally selects· does not resolve; its members cannot be classified", r.owner.owner)
	}
	so := w.object(r.owner.owner, s)
	agrees, err := w.agrees(so, r.run, next)
	if err != nil {
		return nil, nil, err
	}
	if !agrees && next != nil && next.attribute() {
		return nil, nil, nil
	}
	if !agrees {
		return nil, nil, w.declinef(loc, "the members of %s settle no type its {type table} ·conditionally selects·: the type its leading attribute members select classifies them otherwise",
			r.owner.owner)
	}
	if next != nil && next.attribute() {
		return nil, w.declinef(loc, "member %q of %s is an attribute under a type its {type table} can select and a child under the one the attributes before it select, and no one reading of its attribute members settles which",
			next.key, r.owner.owner), nil
	}
	w.govern(so, []xsd.TypeDefinition{s})
	return so, nil, nil
}

// unselected ends r's run at next, or at the object's close where next is
// nil, under r.owner, every live candidate classifying next alike
// (walker.allAlike). It DECLINES next where they do not: notAlike's where r
// reads no selection, and withheld's where the engine withholds it.
func (w *walker) unselected(r *attributeRun, next *runMember) (final *object, unsettled, err error) {
	if next == nil || next.ms == nil {
		return r.owner, nil, nil
	}
	alike := w.allAlike(next.ms, scalarValue(next.val.tok))
	if !alike && r.table != nil {
		return nil, nil, w.withheld(r.owner, next.loc)
	}
	if !alike {
		return nil, nil, w.notAlike(r.owner, next.key, next.loc)
	}
	return r.owner, nil, nil
}

// withheld declines the members of o's object, at loc, a root's whose {type
// table} the types it can select classify differently, where the engine
// withholds the type the table ·conditionally selects· (walker.settled's
// GAP(jsonsrc) marker).
func (w *walker) withheld(o *object, loc xsderr.Loc) error {
	return w.declinef(loc, "the members of %s are classified differently under the types its {type table} can select, and the engine withholds the type the table ·conditionally selects·: a {test} it reaches is outside its compiler's subset",
		o.owner)
}

// agrees reports whether so classifies each member of run as the attribute
// the run took it as, and next, where it is not nil and its value is no
// array, as no attribute, and returns the decline of the first of them so
// declines.
func (w *walker) agrees(so *object, run []*runMember, next *runMember) (bool, error) {
	for _, rm := range run {
		m, err := w.memberOf(so, rm.key, rm.k, rm.loc)
		if err != nil {
			return false, err
		}
		a, ok := m.(attributeMember)
		if !ok || a.name != rm.attributeName() {
			return false, nil
		}
	}
	if next == nil || next.ms == nil {
		return true, nil
	}
	m, err := w.memberOf(so, next.key, next.k, next.loc)
	if err != nil {
		return false, err
	}
	_, ok := m.(elementMember)
	return ok, nil
}

// notScalar declines the attribute member key of o's object whose value, at
// loc, is null, an object or an array.
func (w *walker) notScalar(o *object, key string, loc xsderr.Loc) error {
	return w.declinef(loc, "attribute member %q of %s is not a string, number or boolean; an attribute's value is a scalar", key, o.owner)
}

// item classifies run's key over one of its items, whose element has the
// scope sc, the item's own "$xmlns" included (walker.value), and scalar says
// whether its value is a scalar or null (scalarValue); or, where the array is
// empty, sc being the owner's and scalar false, over none, so that the key's
// declines still stand. It DECLINES a key that classifies as an attribute,
// located at the array: an attribute's value is a scalar.
func (w *walker) item(o *object, run *arrayRun, sc *scope, scalar bool) (elementMember, error) {
	m, err := w.classify(o, run.key, sc, run.loc, scalar)
	if err != nil {
		return nil, err
	}
	child, ok := m.(elementMember)
	if !ok {
		return nil, w.notScalar(o, run.key, run.at)
	}
	return child, nil
}

// end reads what follows the root's value once the assessment returns: the
// rest of the root's subtree the assessment did not read, classified as the
// walk would have, then the top-level object's close and the end of the input.
//
// It DECLINES a second member of the top-level object, which has no XML twin,
// so no marker tracks it: it would be a second document element (XML 1.0 §2.1
// [1]). Content after the object is no decline but malformed, JSON text being
// one value (RFC 8259 §2), and its error is a plain one, whether the decoder
// reads that content as a value or as a syntax error.
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
	return fmt.Errorf("jsonsrc: %s: content follows the top-level object", loc)
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
