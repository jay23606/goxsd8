package jsonsrc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"

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
// [validate.Result.Err]. The faults are: a nil v or r; JSON that is malformed,
// or followed by further content; and every input the mapping DECLINES —
// a top level that is not an object with exactly one member naming a
// top-level element declaration, and, anywhere in the document, a member the
// implemented arms do not map (a key no child element declaration and no
// attribute use carries whose namespace the type's wildcards fix no one of, or
// whose kind a type with both an element wildcard and an {attribute wildcard}
// leaves open, or which a wildcard puts in the xsi or xml namespace; a member
// of an object under a child no contained and no top-level declaration names; a
// key two expanded names carry, a key both a child declaration and an
// attribute use carry, an attribute member after a child member or repeated,
// an attribute member whose value is null, an object or an array, a "$type"
// or "$xmlns" member out of its leading place or of the wrong shape, a
// "$type" on an element an {open content} may take where its name has a
// ·locally declared type·, any other "$"-prefixed reserved member, a nested
// array, a declaration with a {type table}; the package doc lists them all).
// A decline names the arm it is not, and is never a silent drop or a guessed
// mapping: a member the mapping cannot name is not part of the document the
// assessment saw, so its result is discarded rather than returned. The whole
// document is read either way, so a fault after the point the assessment
// stopped at is still reported.
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
// name is the root's (package doc, "The validation root"). The key's declines
// are rootDeclaration's, under its GAP(jsonsrc) markers.
//
// It DECLINES an input holding no JSON value, a top-level value that is not an
// object, a top-level object with no member, and a root member whose value is
// an array. None has an XML twin, so none carries a marker: an XML document
// has exactly one document element (XML 1.0 §2.1 [1]), and each of these
// spells none or several.
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
	e, err := w.element(declaredElement{decl: decl}, loc, key, vtok, vloc, nil)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// rootDeclaration resolves the root member's key to the one top-level element
// declaration whose local name it is. It DECLINES a reserved key, which has no
// XML twin and no marker — "$xmlns" and "$type" lead an element's object and
// do not name one, and any other "$" name is no XML Name (XML 1.0 §2.3 [4]) —
// a declaration carrying a {type table}, under typeTable's GAP(jsonsrc)
// marker, and a key that no top-level declaration, or that declarations of two
// expanded names, carry.
//
// GAP(jsonsrc): a root key no top-level element declaration carries declines,
// where the engine assesses its XML twin, an element with no declaration
// (cvc-assess-elt), in whatever namespace the mapping gave it. Owned by #2623,
// the NAMESPACE TRAP with no declaration carrying the key.
//
// GAP(jsonsrc): a root key top-level element declarations of two expanded names
// carry declines: a JSON key carries no namespace to choose between them.
// Owned by #2623, the NAMESPACE TRAP.
func (w *walker) rootDeclaration(key string, loc xsderr.Loc) (xsd.ElementDeclaration, error) {
	if isReserved(key) {
		return xsd.ElementDeclaration{}, w.declinef(loc, "root member %q is a reserved member; the top-level object's one member names the validation root, and \"$xmlns\" and \"$type\" lead an element's object", key)
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
// read once per element from the type walker.classifyingType chose — its
// declaration's {type definition}, or the type its "$type" names: the owner's
// expanded name, the child element declarations the type contains, the type's
// {attribute uses}, its element wildcards and {attribute wildcard}, whether
// its {content type} is simple, the type itself where its {open content} is
// not skip, and whether the type is known to govern the owner. A simple type
// contains none and has none.
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
	// undeclared is the owner having no declaration: a child wildcardMember
	// names whose expanded name no top-level declaration carries (bareElement),
	// which has no type to classify a member against.
	undeclared bool
	// open is the type, where its {content type} has an {open content} whose
	// {wildcard}'s {process contents} is not skip, and nil otherwise: a child
	// that {open content} may take carries no "$type" where its name has a
	// ·locally declared type· within the type (walker.localType).
	open *xsd.ComplexType
	// governs is the type the members are classified under being the owner's
	// ·governing type definition· wherever the owner's verdict stands
	// uncharged (walker.governs). A child of an owner it is false for carries
	// no "$type" (walker.localType).
	governs bool
}

// reattributes reports whether o's type has an element wildcard whose
// {process contents} is not skip, a Wildcard {term} or its {open content}
// one: a child matched to a contained declaration may be ·attributed to· it
// instead of to the declaration's particle.
func (o *object) reattributes() bool {
	return slices.ContainsFunc(o.elementWildcards, func(wc xsd.Wildcard) bool { return wc.ProcessContents() != xsd.ProcessSkip })
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
// the object e is a member of (nil for the root) and the location of its '{'.
type elementMember interface {
	member
	elementName() xsd.QName
	object(w *walker, e *element, typ *attribute, parent *object, loc xsderr.Loc) (*object, error)
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

func (d declaredElement) object(w *walker, e *element, typ *attribute, parent *object, loc xsderr.Loc) (*object, error) {
	td, instance, err := w.classifyingType(d.decl, e, typ, loc)
	if err != nil {
		return nil, err
	}
	o := w.object(e.name, td)
	o.governs = w.governs(d, e.name, instance, parent)
	return o, nil
}

// object is the empty object a bareElement's value classifies against: {}
// maps, and every member declines (walker.wildcardMember), those after a
// "$type" included — whatever type it names, the element has no declaration
// for it to stand beside.
func (b bareElement) object(*walker, *element, *attribute, *object, xsderr.Loc) (*object, error) {
	return &object{owner: b.name, undeclared: true}, nil
}

// classify resolves a member's key, at loc, against o: to the child element
// declaration among o.contained whose local name it is, or to the attribute
// use among o.uses whose {attribute declaration}'s local name it is
// (cvc-complex-type clause 2.1 then matches the attribute to that use by
// expanded name), and, where neither carries it, to the element or attribute
// a wildcard of the type names, or to the child a type with no wildcard names
// (wildcardMember). A key a declaration or a use carries never reaches a
// wildcard: the mapping's own rule, which the spec shares only where the
// wildcard's one namespace is the declaration's or the use's own, so that
// both name one item (the Note under cvc-accept; the Note after
// cvc-complex-type clause 4; package doc, "The mapping"). The first
// declaration of the name stands for every other
// (xsd.Schema.ContainedElementDeclarations says why that is sound).
//
// It DECLINES a reserved key, walker.lead having read the ones that lead the
// object; a key two expanded names carry among the declarations, or among the
// uses; a key both a declaration and a use carry, which no spec rule decides
// between; a use in the xsi namespace, xsi:nil being the null arm's to
// present, xsi:type the "$type" member's, and xsi:schemaLocation and
// xsi:noNamespaceSchemaLocation hints a JSON instance carries none of; an
// attribute use of a type with simple content, beside which the mapping
// spells no text; and every key wildcardMember declines.
//
// GAP(jsonsrc): a key both a child element declaration and an attribute use
// carry declines. RULED permanent by #2613: no spec rule decides between them,
// and the mapping has no warning channel to report a choice through.
//
// GAP(jsonsrc): a key a declaration or use carries maps under its name even
// where a wildcard of the type would name the key (N,key) in another namespace
// N, or as the other kind. RULED permanent by #2634 as the mapping's own
// rule: the spec is silent, the Note under cvc-accept and the Note after
// cvc-complex-type clause 4 giving a declaration or use precedence only over a
// wildcard of its own kind that admits the same expanded name.
//
// The reserved-key decline is reserved's, the two-expanded-name decline among
// the declarations childNamed's, the {type table} decline typeTable's, and the
// xsi-namespace, two-expanded-name and simple-content declines among the uses
// useNamed's, each under its function's GAP(jsonsrc) markers.
func (w *walker) classify(o *object, key string, loc xsderr.Loc) (member, error) {
	if isReserved(key) {
		return nil, w.reserved(key, loc)
	}
	decl, declared, err := w.childNamed(o, key, loc)
	if err != nil {
		return nil, err
	}
	use, used, err := w.useNamed(o, key, loc)
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
		return w.wildcardMember(o, key, loc)
	}
	return declaredElement{decl: decl}, w.typeTable(decl, loc)
}

// wildcardMember names a key no contained declaration and no attribute use of
// o carries, where the model fixes its namespace (package doc, "The
// mapping"). Element kind: every element wildcard of the type — each Wildcard
// {term} its content model contains, whatever its {process contents}, and its
// {open content} wildcard — enumerates the one namespace N, and the key is the
// child (N,key), under the top-level declaration of that name where there is
// one (declaredElement, which typeTable vets) and as a bareElement where there
// is none. Attribute kind: the {attribute wildcard} enumerates the one
// namespace M, and the key is the attribute (M,key). No wildcard of either
// kind: the key is the child (O,key), O the owner's own namespace name, named
// as an element wildcard's child is (namedChild); every XML twin of the key,
// of whatever name and kind, is invalid — an element under cvc-complex-content,
// cvc-complex-type clause 1.1 or 1.2, or cvc-type clause 3.1.2 under a simple
// type, an attribute under cvc-complex-type clause 2.2.1 or cvc-type clause
// 3.1.1 — but the four xsi attributes cvc-complex-type clause 2 excepts, so
// the name decides no validity, and O keeps the child's default namespace its
// parent's.
//
// It does not test admission: a {disallowed names} entry, ##defined and
// ##definedSibling are the engine's to charge (cvc-wildcard clauses 1 to 3),
// and a name the wildcards reject is soundly invalid, because every other
// namespace is outside every wildcard of that kind and no declaration or use
// of the type carries the key. Which {process contents} applies, and whether
// the element is ·skipped·, ·laxly assessed· or charged under e-validity
// clause 1.1.3, is the engine's too.
//
// It DECLINES a member of a bareElement's object; a key the type has
// wildcards of both kinds for (xs:anyType has); a wildcard of variety any or
// not, an enumeration of more than one namespace, or element wildcards whose
// namespaces differ; N or M the xsi or the xml namespace; and a key a
// simple-content type's {attribute wildcard} alone admits.
//
// GAP(jsonsrc): a wildcard-only key whose namespace or kind the model does
// not fix declines: a wildcard of variety any or not, xs:anyType's included,
// an enumeration of several namespaces, element wildcards enumerating
// different ones, a type with wildcards of both kinds, and a bareElement's
// members, which xs:anyType's lax wildcards would classify. RULED permanent
// by #2634: the verdict of whichever (N,key) the mapping chose depends on N
// through declaration lookup (key-governing-ed clause 3), attribution
// (cvc-accept, cvc-wildcard, cvc-wildcard-name) and an ancestor's
// identity-constraint paths (c-selector-xpath, c-fields-xpaths) and
// assertions (cvc-assertion), which the mapping cannot fix without an
// ancestor census.
//
// GAP(jsonsrc): a key a wildcard's single namespace, of either kind, puts in
// the xml or the xsi namespace declines. RULED permanent by #2635: an element
// in the xml namespace would bind the default namespace to it, which no XML
// twin has, and the attribute kind is ruled with it; the four xsi attributes
// cvc-complex-type clause 2 excepts must keep declining, xsi:nil and xsi:type
// having their own spellings, and the rest of the xsi namespace is not worth a
// half-arm.
//
// GAP(jsonsrc): a key a simple-content type's {attribute wildcard} alone
// admits declines, on the ground useNamed's simple-content marker states for
// an attribute use, RULED permanent by #2613: the mapping spells no text
// beside attribute members.
func (w *walker) wildcardMember(o *object, key string, loc xsderr.Loc) (member, error) {
	if o.undeclared {
		return nil, w.declinef(loc, "member %q of %s, an element no contained and no top-level element declaration names; only an empty object maps under it",
			key, o.owner)
	}
	elemental := len(o.elementWildcards) > 0
	if elemental && o.attributeWildcard != nil {
		return nil, w.declinef(loc, "member %q names no child element declaration and no attribute use of the type of %s, which has both an element wildcard and an {attribute wildcard}; a JSON key does not say which it is",
			key, o.owner)
	}
	if elemental {
		ns, why, ok := fixedNamespace(o.elementWildcards)
		if !ok {
			return nil, w.declinef(loc, "member %q names no child element declaration and no attribute use of the type of %s, and its element wildcard set %s",
				key, o.owner, why)
		}
		return w.namedChild(xsd.QName{Space: ns, Local: key}, loc)
	}
	if o.attributeWildcard == nil {
		return w.namedChild(xsd.QName{Space: o.owner.Space, Local: key}, loc)
	}
	if o.simpleContent {
		return nil, w.declinef(loc, "member %q names only the {attribute wildcard} of the type of %s, whose {content type} is simple; the mapping spells no text beside attribute members",
			key, o.owner)
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
// there is one (declaredElement, which typeTable vets), and as a bareElement
// where there is none.
func (w *walker) namedChild(name xsd.QName, loc xsderr.Loc) (member, error) {
	d, ok := w.schema.Element(name)
	if !ok {
		return bareElement{name: name}, nil
	}
	return declaredElement{decl: d, named: true}, w.typeTable(d, loc)
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
		return "", fmt.Sprintf("enumerates %d namespaces; a JSON key carries no namespace to choose between them", len(fixed)), false
	}
	uri, _ = fixed[0].URI() // "" for ·absent·, a QName's no-namespace Space
	if uri == xsd.XMLSchemaInstanceNS || uri == xmlNamespace {
		return "", fmt.Sprintf("enumerates the namespace %s alone; the mapping names no key in the xsi or xml namespace", uri), false
	}
	return uri, "", true
}

// childNamed is the first of o.contained whose local name is key, declining
// a key two expanded names carry.
//
// GAP(jsonsrc): a key child element declarations of two expanded names carry
// declines: cvc-particle matches an element to a declaration by expanded name,
// and a JSON key carries no namespace to choose between them. Owned by #2623,
// the NAMESPACE TRAP.
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
// and a JSON key carries no namespace to choose between them. Owned by #2623,
// the NAMESPACE TRAP.
//
// GAP(jsonsrc): a key naming an xsi:type attribute use declines. RULED
// permanent by #2615: a "$type" member is xsi:type's one spelling
// (walker.lead), and a second would let one object present it twice.
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
		return xsd.AttributeUse{}, false, w.declinef(loc, "member %q names the attribute use %s of the type of %s; an xsi: attribute is presented only by the null => xsi:nil arm and the \"$type\" member",
			key, first.DeclarationName(), o.owner)
	}
	if o.simpleContent {
		return xsd.AttributeUse{}, false, w.declinef(loc, "member %q names an attribute use of the type of %s, whose {content type} is simple; the mapping spells no text beside attribute members",
			key, o.owner)
	}
	return first, true, nil
}

// typeTable declines a declaration carrying a {type table}: members are
// classified under a type the mapping reads off the declaration or a "$type"
// member, and such an element's ·selected type definition· is neither — it
// turns on the element's attributes (§3.12.4, key-selected-type clause 1), xsi:
// ones included, and is computed only inside validate, so the content
// classified would be a guess.
//
// GAP(jsonsrc): a declaration carrying a {type table} declines, where the
// engine assesses its XML twin under the type the table selects (§3.12.4).
// Owned by #2630, the lift that classifies under the ·selected type
// definition·.
func (w *walker) typeTable(d xsd.ElementDeclaration, loc xsderr.Loc) error {
	if _, ok := d.TypeTable(); ok {
		return w.declinef(loc, "element %s carries a {type table}; conditional type assignment is not implemented", d.Name())
	}
	return nil
}

// The two reserved members the mapping reads (walker.lead).
const (
	memberXmlns = "$xmlns"
	memberType  = "$type"
)

// isReserved reports whether key is a reserved member name: every
// "$"-prefixed one. "$xmlns" and "$type" are mapped where they lead an object
// (walker.lead), and every other one is held.
func isReserved(key string) bool { return len(key) > 0 && key[0] == '$' }

// reserved declines the reserved member key at loc, which walker.lead did not
// read: a "$xmlns" or "$type" out of its place, or any other reserved name.
// Any other "$" name, and a repeated "$xmlns" or "$type", has no XML twin, so
// no marker tracks it: "$" is not an XML NameStartChar (XML 1.0 §2.3 [4]), and
// an element carries one attribute of each expanded name (XML 1.0 §3.1, Unique
// Att Spec).
//
// GAP(jsonsrc): a "$xmlns" or "$type" out of its leading place — "$type"
// before "$xmlns", or either after an attribute or child member — declines,
// where its XML twin, attribute order being insignificant, carries the
// namespace declarations or the xsi:type. RULED permanent by #2615: no spec
// text orders an element's attributes, so the decline is never a charge, and
// reading the two first is what lets the members after them be classified
// under the type "$type" names, resolved through the bindings "$xmlns" makes,
// without holding the object (STYLE P4), as members.key's marker says of an
// attribute member after a child.
func (w *walker) reserved(key string, loc xsderr.Loc) error {
	if key == memberXmlns || key == memberType {
		return w.declinef(loc, "reserved member %q is out of place; \"$xmlns\" and then \"$type\" lead an element's object, each at most once, before its other members", key)
	}
	return w.declinef(loc, "reserved member %q; only the \"$type\" and \"$xmlns\" reserved members are mapped", key)
}

// element builds the element a member (or an item of a member's array) maps
// to, from its classification m, the location of its key or item, the key,
// the value's first token and its location, and in, the members cursor of the
// parent's object, whose owner's scope it inherits — nil for the root.
//
// It DECLINES an array item that is itself an array. That has no XML twin, so
// no marker tracks it: an item maps to exactly one element, and an array to
// zero or more.
func (w *walker) element(m elementMember, loc xsderr.Loc, key string, tok json.Token, tokLoc xsderr.Loc, in *members) (*element, error) {
	var parent *scope
	if in != nil {
		parent = in.owner.scope
	}
	switch v := tok.(type) {
	case nil:
		return w.nilled(m.elementName(), loc, tokLoc, parent), nil
	case json.Delim:
		if v != json.Delim('{') {
			// '[' alone reaches here: Token never yields a closing delimiter
			// where a value is due.
			return nil, w.declinef(tokLoc, "an array is an item of the array of member %q; only an object or a scalar maps to one element", key)
		}
		return w.objectElement(m, loc, tokLoc, in, parent)
	}
	lexical, ok := scalarLexical(tok)
	if !ok {
		panic(fmt.Sprintf("jsonsrc: Token yielded %T", tok))
	}
	return w.scalar(m.elementName(), loc, lexical, tokLoc, parent), nil
}

// objectElement builds an object-valued element whose '{' is at tokLoc, in
// the parent's members cursor in (nil for the root) and scope parent: its
// leading reserved members (lead), a "$type" among them vetted by localType,
// then its attribute members (attributes), each classified under the type its
// members are (elementMember.object), and a cursor over the rest.
func (w *walker) objectElement(m elementMember, loc, tokLoc xsderr.Loc, in *members, parent *scope) (*element, error) {
	e := &element{w: w, name: m.elementName(), loc: loc, scope: parent}
	typ, tok, tokAt, err := w.lead(e)
	if err != nil {
		return nil, err
	}
	var up *object
	if in != nil {
		up = in.object
	}
	if typ != nil && up != nil {
		if err := w.localType(up, e, typ); err != nil {
			return nil, err
		}
	}
	o, err := m.object(w, e, typ, up, tokLoc)
	if err != nil {
		return nil, err
	}
	attrs, head, err := w.attributes(o, tok, tokAt)
	if err != nil {
		return nil, err
	}
	if typ != nil {
		attrs = append([]validate.Attribute{typ}, attrs...)
	}
	e.attrs = attrs
	e.kids = &members{w: w, owner: e, object: o, head: &head}
	return e, nil
}

// lead reads the reserved members that lead e's object, in this order and
// each at most once: a "$xmlns", whose bindings become e's scope (xmlns), then
// a "$type", returned as e's xsi:type attribute, located at its key, whose
// [[normalized value]] is the member's string unchanged. It returns the token
// after them, a key or the object's close, and its location.
//
// Reading them first is what lets the members after them be classified under
// the type "$type" names, and "$type" resolve against the bindings "$xmlns"
// makes, without holding any of the document (STYLE P4). A "$type" before a
// "$xmlns", a repeat of either, and either after another member are left to
// classify, which declines them (walker.reserved): no spec text orders an
// element's attributes, so none of them is charged.
func (w *walker) lead(e *element) (*attribute, json.Token, xsderr.Loc, error) {
	tok, loc, err := w.token()
	if err != nil {
		return nil, nil, loc, unexpectedEOF(err, loc)
	}
	if tok == json.Token(memberXmlns) {
		bindings, err := w.xmlns(e.name)
		if err != nil {
			return nil, nil, loc, err
		}
		e.scope = &scope{parent: e.scope, bindings: bindings}
		if tok, loc, err = w.token(); err != nil {
			return nil, nil, loc, unexpectedEOF(err, loc)
		}
	}
	if tok != json.Token(memberType) {
		return nil, tok, loc, nil
	}
	vtok, vloc, err := w.token()
	if err != nil {
		return nil, nil, vloc, unexpectedEOF(err, vloc)
	}
	lexical, ok := vtok.(string)
	if !ok {
		return nil, nil, vloc, w.declinef(vloc, "the \"$type\" member of %s is not a string; its value is the lexical form of the element's xsi:type", e.name)
	}
	typ := &attribute{name: xsiType, value: lexical, loc: loc}
	if tok, loc, err = w.token(); err != nil {
		return nil, nil, loc, unexpectedEOF(err, loc)
	}
	return typ, tok, loc, nil
}

// xmlns reads the value of a "$xmlns" member of owner's object: an object
// whose every member binds its key, a prefix, to its string value, a namespace
// name. It declines any other value; a member whose value is not a string; a
// non-empty prefix that is no NCName, which no xmlns:prefix attribute can
// declare (Namespaces in XML [2] PrefixedAttName); a prefix bound twice; the
// prefixes "xml" and "xmlns", which Namespaces in XML §3 reserves, "xml" being
// bound already (element.LookupPrefix); and a non-empty prefix bound to the
// empty string, which would undeclare it. The empty prefix may be bound, and
// is shadowed: the element's default namespace stays its own namespace name
// (package doc).
func (w *walker) xmlns(owner xsd.QName) ([]binding, error) {
	tok, loc, err := w.token()
	if err != nil {
		return nil, unexpectedEOF(err, loc)
	}
	if tok != json.Delim('{') {
		return nil, w.declinef(loc, "the \"$xmlns\" member of %s is not an object; its members bind prefixes to namespace names", owner)
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
			return nil, w.declinef(vloc, "the \"$xmlns\" member of %s binds %q to a value that is not a string; a namespace name is a string", owner, prefix)
		}
		if prefix != "" && !xmlname.IsNCName(prefix) {
			return nil, w.declinef(loc, "the \"$xmlns\" member of %s binds %q, which is not an NCName; no xmlns:prefix attribute declares it", owner, prefix)
		}
		if prefix == "xml" || prefix == "xmlns" {
			return nil, w.declinef(loc, "the \"$xmlns\" member of %s binds the reserved prefix %q", owner, prefix)
		}
		if prefix != "" && uri == "" {
			return nil, w.declinef(vloc, "the \"$xmlns\" member of %s binds %q to the empty string; a prefix cannot be undeclared", owner, prefix)
		}
		if slices.ContainsFunc(bindings, func(b binding) bool { return b.prefix == prefix }) {
			return nil, w.declinef(loc, "the \"$xmlns\" member of %s binds %q twice", owner, prefix)
		}
		bindings = append(bindings, binding{prefix: prefix, uri: uri})
	}
}

// classifyingType is the type definition the members of e, an element under
// decl, are classified under, tokLoc locating its object, and whether it is
// the one e's xsi:type names: decl's {type definition}, which must resolve,
// or, where e carries an xsi:type typ whose lexical names a type
// (instanceType), that ·instance-specified type definition·. The second is
// taken whether or not it ·overrides· the type the engine would otherwise
// assess e under: where it does not, the engine assesses the members under
// that type and charges cvc-elt clause 4 where a ·governing element
// declaration· supplied it, and they are still the members of the type
// "$type" names. Where a ·locally declared type· could supply it instead,
// uncharged (key-governing-type-elem clause 7), localType declined the
// "$type" before this was reached.
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
func (w *walker) classifyingType(decl xsd.ElementDeclaration, e *element, typ *attribute, tokLoc xsderr.Loc) (xsd.TypeDefinition, bool, error) {
	declared, ok := w.schema.ResolvedType(decl.TypeDefinition())
	if !ok {
		return nil, false, w.declinef(tokLoc, "the {type definition} of %s does not resolve; its members cannot be classified", decl.Name())
	}
	if typ == nil {
		return declared, false, nil
	}
	if instance, ok := w.instanceType(e, typ.value); ok {
		return instance, true, nil
	}
	return declared, false, nil
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

// governs reports whether the type the members of the element name, under d,
// are classified under is its ·governing type definition· wherever its
// verdict stands uncharged — instance being that type's coming from its
// "$type" — given parent, the object it is a member of (nil for the root).
// It is true for the root, which d or an xsi:type ·overriding· d's type
// governs, cvc-elt clause 4 being charged otherwise. Below the root it needs
// the parent's own type to govern the parent (parent.governs), and then:
//
//   - with an xsi:type naming the type, localType having let it through: the
//     type ·overrides· whatever else would govern, or cvc-elt clause 4 is
//     charged, or no declaration and no ·locally declared type· is there and
//     the type governs (key-governing-type-elem clause 8);
//   - with d a declaration the parent's type contains: false where that type
//     has a non-skip element wildcard (object.reattributes) and a top-level
//     declaration of the name exists, which may govern the element in d's
//     place once it is ·attributed to· that wildcard (key-governing-ed
//     clauses 3 and 4) with no charge where its type is ·validly
//     substitutable· for d's (cvc-complex-type clause 5); an {open content}
//     with no such declaration leaves the ·locally declared type·, which is
//     d's type (key-ldt-elem case 2, cos-element-consistent);
//   - with d the top-level declaration a wildcard names: false where the
//     name has a ·locally declared type· within the parent's type under a
//     non-skip {open content}, which governs in d's place
//     (key-governing-ed clause 4.3, key-governing-type-elem clause 7).
//
// It is over-approximate in the declining direction: false wherever another
// type may govern, whether or not one does.
func (w *walker) governs(d declaredElement, name xsd.QName, instance bool, parent *object) bool {
	if parent == nil {
		return true
	}
	if !parent.governs {
		return false
	}
	if instance {
		return true
	}
	if d.named {
		return parent.open == nil || !w.locallyDeclared(*parent.open, name)
	}
	if !parent.reattributes() {
		return true
	}
	_, global := w.schema.Element(name)
	return !global
}

// localDeclarer is the one method of *xsd.Schema the mapping reads beyond
// validate.SchemaView: xsd.Schema.LocallyDeclaredElementType, key-ldt-elem's
// base-chain walk, which validate's walk governs an {open content} child by.
// Reading it through this capability keeps one encoding of that walk (STYLE
// T4) without widening SchemaView's exported surface.
type localDeclarer interface {
	LocallyDeclaredElementType(within xsd.ComplexType, name xsd.QName) (xsd.TypeDefinition, bool)
}

// locallyDeclared reports whether name has a non-·absent· ·locally declared
// type· within t (key-ldt-elem), as the engine reads it. A schema view
// without the capability, which validate.Validator.Schema never returns,
// answers true: the mapping cannot rule one out, and declines.
func (w *walker) locallyDeclared(t xsd.ComplexType, name xsd.QName) bool {
	ld, ok := w.schema.(localDeclarer)
	if !ok {
		return true
	}
	_, local := ld.LocallyDeclaredElementType(t, name)
	return local
}

// localType vets the xsi:type typ of e, a child of the object parent. It
// DECLINES typ, whatever it names, where the type the parent's members are
// classified under is not known to govern the parent (parent.governs, from
// walker.governs), and where that type P has an {open content} whose
// {wildcard} is not skip (parent.open) and e's expanded name has a ·locally
// declared type· within P (locallyDeclared, key-ldt-elem).
//
// Such an e may be ·attributed to· that {open content}, which the mapping
// cannot tell from a particle without running the content model. It then has
// no ·governing element declaration· (key-governing-ed clause 4.3), and the
// ·locally declared type· L governs it unless the type typ names ·overrides·
// L (key-governing-type-elem clauses 6 and 7), with no cvc-elt clause 4
// charge. The mapping classifies e's members under the type typ names
// (classifyingType), so where that type does not ·override· L, e's attributes
// and its descendants would be classified under one type and assessed under
// another, uncharged; validate.SchemaView cannot ask ·override·. Where P may
// not govern the parent, the type that does may have such an {open content},
// so the "$type" declines there too.
//
// GAP(jsonsrc): a "$type" on an element a non-skip {open content} may take,
// whose name has a ·locally declared type· within the type, declines, and so
// does one under a parent the mapping cannot show its classifying type
// governs, where the engine assesses the XML twin under the ·governing type
// definition·. Owned by #2647, the lift that classifies under the ·governing
// type definition·.
func (w *walker) localType(parent *object, e *element, typ *attribute) error {
	if !parent.governs {
		return w.declinef(typ.loc, "the \"$type\" member of %s: the type the members of %s are classified under may not govern it, and the one that does may give %s a ·locally declared type· under an {open content}, which governs it unless the type \"$type\" names ·overrides· it (key-governing-type-elem clauses 6 and 7); the mapping does not ask ·override·",
			e.name, parent.owner, e.name)
	}
	if parent.open == nil || !w.locallyDeclared(*parent.open, e.name) {
		return nil
	}
	return w.declinef(typ.loc, "the \"$type\" member of %s: the {open content} of the type of %s may take it, and its name has a ·locally declared type· there, which governs it unless the type \"$type\" names ·overrides· it (key-governing-type-elem clauses 6 and 7); the mapping does not ask ·override·",
		e.name, parent.owner)
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
// against, from td, the type classifyingType chose: a complex type's
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
				if ct.OpenContent.Wildcard().ProcessContents() != xsd.ProcessSkip {
					o.open = &t
				}
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

// attributes reads the members of o's object that classify as attributes
// (classify), from tok at loc, the first after its reserved members, each an
// attribute of the element; and returns them with the token that ends the run
// — the first key naming a child, or the object's close — for the element's
// members cursor to start from.
//
// The engine reads an element's [[attributes]] at its entry, before its
// [[children]], and again after them, so they must be whole and unchanged
// from the first read. Reading the leading run as the element is built makes
// them so without holding any of the document (STYLE P4); an attribute member
// after a child member therefore declines, under members.key's GAP(jsonsrc)
// marker.
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
func (w *walker) attributes(o *object, tok json.Token, loc xsderr.Loc) ([]validate.Attribute, peeked, error) {
	var attrs []validate.Attribute
	for {
		key, ok := tok.(string)
		if !ok {
			return attrs, peeked{tok: tok, loc: loc}, nil
		}
		m, err := w.classify(o, key, loc)
		if err != nil {
			return nil, peeked{}, err
		}
		a, ok := m.(attributeMember)
		if !ok {
			return attrs, peeked{tok: tok, loc: loc, member: m.(elementMember)}, nil
		}
		name := a.name
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
		if tok, loc, err = w.token(); err != nil {
			return nil, peeked{}, unexpectedEOF(err, loc)
		}
	}
}

// end reads what follows the root's value once the assessment returns: the
// rest of the root's subtree the assessment did not read, classified as the
// walk would have, then the top-level object's close and the end of the input.
//
// It DECLINES a second member of the top-level object and content after it.
// Neither has an XML twin, so no marker tracks them: a second member would be
// a second document element (XML 1.0 §2.1 [1]), and JSON text is one value
// (RFC 8259 §2), so content after it is malformed.
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
