package codec

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/kud360/goxsd8/internal/goname"
	"github.com/kud360/goxsd8/parser/xmltree"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/validate/xmlsrc"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// ErrTypeMismatch is the error [Decode] wraps when T does not match the type
// codegen emits for the document element's declaration; Decode's doc, item 6,
// states when.
var ErrTypeMismatch = errors.New("codec: T is not the type codegen emits for the document element")

// The names of the primitives the slice decodes, read by identity.
var (
	stringPrimitive = xsd.QName{Space: xsd.XMLSchemaNS, Local: "string"}
	anyURIPrimitive = xsd.QName{Space: xsd.XMLSchemaNS, Local: "anyURI"}
)

// Decode decodes the XML instance data into a new value of T, the type
// package codegen emits for the declaration of data's document element, and
// returns it. A value is returned exactly when the error is nil.
//
// T is the caller's choice: nothing dispatches a document element to its
// generated type, and item 6 below never compares T's own name. For a complex
// {type definition} item 6 compares T's fields with those codegen emits for
// the declaration in name, order and Go type alone, so a T whose fields
// differ returns [ErrTypeMismatch] where item 6 is the first that applies,
// but a T of the same shape, such as the struct codegen emits for another
// top-level element of the same type, passes item 6 and decodes as the
// declaration's own struct does, an error only where that one's is. For a
// simple one item 6 reads T's underlying type alone, so any T over string
// passes it, such as the type codegen emits for another document element of
// the same simple type. A caller decoding documents of several roots
// therefore reads the document element's name itself, through parser/xmltree
// or encoding/xml, before choosing T.
//
// T may be codegen's sealed interface for a choice, as the package doc's
// "# What decodes" states, and Decode then returns a *T pointing at it. The
// branch types reach Decode only through [WithBranches], and item 6 holds them
// to the alternatives by shape: neither T's, the interface's nor a branch's
// name is compared.
//
// The slice Decode covers is the package doc's "# What decodes". Decode
// first assesses data against schema with package validate and decodes only
// an instance whose Result has no source fault and stands
// validate.NotProvenInvalid; it then reads data a second time with
// parser/xmltree. Each decoded value, a field's or, for a document element of
// simple type, T's own, is the item's ·normalized value· (Structures §3.1.4
// key-nv) under its declaration's simple type: the string
// [value.ApplyWhiteSpace] returns for the item's characters, an attribute's as
// XML 1.0 §3.3.3 normalized them, an element's the concatenation of its
// character children. Where the post-schema-validation infoset supplies a value
// it is that value, the {lexical form} of a value constraint under the same
// normalization: for an absent attribute whose use has an effective value
// constraint (§3.5.4 key-evc; §3.4.4.2 key-dflt-att, §3.4.5.1 sic-attrDefault),
// and for an element with neither element nor character children whose
// declaration has a {value constraint} (§3.3.4.3 cvc-elt clause 5.1, §3.3.5.4
// sic-eltType). So the field of an optional attribute use with an effective
// value constraint is a string, as codegen emits it, holding the supplied value
// when the attribute is absent; a whitespace-only element has a character child
// and holds its own value; and an element absent from the instance supplies
// nothing, its field nil or not appended to.
//
// Every instance Loc an error of Decode carries — a source fault's, a
// violation's, an undecided check's and a decline's — names the document
// [WithURI] gives; without it, or with an empty uri, the URI renders "?".
//
// The first of these that applies is returned:
//
//  1. A nil schema: validate.New's plain error. A nil backend panics, on
//     validate.New's grounds. backend must be the one schema was compiled
//     with, as validate.New states.
//  2. A source fault, in either channel validate/xmlsrc.Validate carries one
//     in, whatever the Result's Standing: that error as it is, an
//     *xsderr.Error with rule xml-wf that xsderr.RuleOf reaches.
//  3. A validate.Charged Result: an error wrapping every violation, each an
//     *xsderr.Error, in the order Result.Violations lists them. errors.As to
//     an *xsderr.Error, and xsderr.RuleOf, reach the first violation alone.
//     [ViolationsOf] returns every one. Decode never succeeds where validate
//     does not accept.
//  4. A validate.Undecided Result: an [*UndecidedError] whose Unevaluated is
//     the first validate.Unevaluated. It wraps errors.ErrUnsupported, as a
//     *DeclineError does, so errors.Is does not tell the two apart; errors.As
//     does.
//  5. A construct of the document element's declaration or type outside the
//     slice: a *DeclineError at the document element's instance Loc, whose
//     [DeclineError.Construct] is that construct's. Each is found in this
//     order: the declaration's {type table} and {nillable}, then its {type
//     definition}: a simple type's {variety} and {primitive type definition},
//     after which nothing further is examined, or a complex type's content,
//     then the occurrence of its sequence or choice, each particle of its
//     content in order — of a choice, each alternative's own constructs, then
//     a branch field of the name and Go type an earlier alternative's has,
//     which declines as two choice alternatives whose branch structs have one
//     shape — each of its attribute uses in order, and its {attribute
//     wildcard}.
//  6. T does not match the type codegen emits for the declaration, as Decode
//     derives it from the schema: for a complex {type definition}, T is not a
//     struct whose fields are exactly those codegen emits, in name, order and
//     Go type, or, where codegen emits the choice's sealed interface as the
//     type itself, T is not an interface; for a simple one, T's underlying
//     type is not string. A sealed interface, T or the struct's first field,
//     does not match when it is not an interface whose one method is
//     unexported, or when its branch set, as [WithBranches] supplied it, is
//     not one struct of one field per alternative, that field the name and Go
//     type of the alternative's branch field: when no WithBranches supplies
//     it for a choice with {particles}, or its count differs, or it holds a
//     nil value, a type that is no struct, a type twice, or a branch matching
//     no alternative an earlier branch did not match. It is an error
//     errors.Is reports as [ErrTypeMismatch], no *DeclineError, so that a T
//     that does not match the schema is never filled. Decode never reads the
//     fields from T, so for a type codegen emitted from this schema the error
//     is a defect of Decode's, or a branch set WithBranches did not supply as
//     codegen emitted it, and for any other T it is a T that does not match.
//  7. An element carrying xsi:type, the document element or a child: a
//     *DeclineError at that element's instance Loc, the first in document
//     order, whose [DeclineError.Construct] is xsi:type's. A document element
//     no top-level declaration declares, which validation accepts only
//     through its xsi:type (§3.3.4.6 cvc-assess-elt), declines so in place of
//     item 5.
func Decode[T any](schema *xsd.Schema, backend value.Backend, data []byte, opts ...Option) (*T, error) {
	cfg := newConfig(opts)
	v, err := validate.New(schema, backend)
	if err != nil {
		return nil, err
	}
	res, err := xmlsrc.Validate(v, bytes.NewReader(data), xmlsrc.WithURI(cfg.uri))
	if err != nil {
		return nil, err
	}
	if err := verdict(res); err != nil {
		return nil, err
	}
	r := xmltree.NewReader(cfg.uri, bytes.NewReader(data))
	root, err := documentElement(r)
	if err != nil {
		return nil, err
	}
	b, err := bind(schema, root)
	if err != nil {
		return nil, err
	}
	f, err := b.check(reflect.TypeFor[T](), cfg.branches)
	if err != nil {
		return nil, err
	}
	out := new(T)
	if err := f.fill(r, root, reflect.ValueOf(out).Elem()); err != nil {
		return nil, err
	}
	return out, nil
}

// verdict returns nil for a Result Decode decodes, and otherwise the error
// Decode's doc states for it, items 2 to 4.
func verdict(res *validate.Result) error {
	if err := res.Err(); err != nil {
		return err
	}
	switch res.Standing() {
	case validate.NotProvenInvalid:
		return nil
	case validate.Charged:
		return &chargedError{violations: res.Violations()}
	case validate.Undecided:
		return &UndecidedError{u: res.Unevaluated()[0]}
	}
	return fmt.Errorf("codec: the validator returned Standing %d, which is no arm", res.Standing())
}

// ViolationsOf returns every violation of the charged error [Decode]
// returns for a validate.Charged Result (Decode's doc, item 3), in the
// order Result.Violations lists them, however many layers wrap that error.
// It returns nil for every other error, a nil err included. The slice is
// fresh on each call: mutating it does not affect err.
func ViolationsOf(err error) []*xsderr.Error {
	var ce *chargedError
	if !errors.As(err, &ce) {
		return nil
	}
	return slices.Clone(ce.violations)
}

// chargedError is the error Decode's doc states in item 3; violations is the
// Result's, never empty.
type chargedError struct{ violations []*xsderr.Error }

// Error opens with codec's statement of the charge and then gives each
// violation's text, one per line.
func (e *chargedError) Error() string {
	return "codec: the instance is invalid against the schema: " + errors.Join(e.Unwrap()...).Error()
}

// Unwrap returns the violations, each as an error, for the errors.As and
// xsderr.RuleOf reads Decode's doc states in item 3.
func (e *chargedError) Unwrap() []error {
	errs := make([]error, len(e.violations))
	for i, v := range e.violations {
		errs[i] = v
	}
	return errs
}

// An UndecidedError is the error [Decode] returns for a validate.Undecided
// Result (Decode's doc, item 4): the validator reached a check it did not
// perform, so the instance is not proven valid. It wraps
// errors.ErrUnsupported, as a [*DeclineError] does; errors.As to each type
// tells the two apart.
type UndecidedError struct{ u validate.Unevaluated }

// Unevaluated returns the first of the Result's validate.Unevaluated checks,
// in document order: its rule, its instance Loc and its message.
func (e *UndecidedError) Unevaluated() validate.Unevaluated { return e.u }

// Error names the Unevaluated's instance Loc and rule.
func (e *UndecidedError) Error() string {
	return fmt.Sprintf("codec: %s: the validator left %s undecided, so the instance is not proven valid: %v", e.u.Loc(), e.u.Rule(), errors.ErrUnsupported)
}

// Unwrap returns errors.ErrUnsupported.
func (e *UndecidedError) Unwrap() error { return errors.ErrUnsupported }

// documentElement returns the start tag of r's document element.
func documentElement(r *xmltree.Reader) (*xmltree.StartElement, error) {
	for {
		tok, err := r.Token()
		if err != nil {
			return nil, err
		}
		if start, ok := tok.(*xmltree.StartElement); ok {
			return start, nil
		}
	}
}

// shape is the occurrence a field holds its string in, by codegen's
// occurrence rules: one value, an optional one, or a repeated one. The zero
// value is invalid (STYLE T1).
type shape uint8

const (
	shapeOne      shape = iota + 1 // string
	shapeOptional                  // *string
	shapeRepeated                  // []string
)

// goType returns the Go type codegen gives a field of shape s over string.
func (s shape) goType() reflect.Type {
	switch s {
	case shapeOne:
		return reflect.TypeFor[string]()
	case shapeOptional:
		return reflect.TypeFor[*string]()
	case shapeRepeated:
		return reflect.TypeFor[[]string]()
	}
	// The zero shape is an internal bug, never input.
	panic(fmt.Sprintf("codec: invalid shape %d", s))
}

// field is one field of a struct codegen emits: of the document element's
// declaration's struct, or of a choice's branch struct. It holds its Go name,
// its shape, the simple type its value is normalized under, and supplied, the
// value Decode's doc states the post-schema-validation infoset supplies for
// it, already normalized under st, nil exactly when there is no value
// constraint: an element's {value constraint}, an attribute use's effective
// one (§3.5.4 key-evc).
type field struct {
	name     string
	shape    shape
	st       *xsd.SimpleType
	supplied *string
}

// set stores the ·normalized value· of initial, under f's simple type, in dst,
// a value of f's Go type.
func (f field) set(schema *xsd.Schema, dst reflect.Value, initial string) error {
	v, err := value.ApplyWhiteSpace(schema, f.st, initial)
	if err != nil {
		return fmt.Errorf("codec: normalizing the value of field %s: %w", f.name, err)
	}
	f.store(dst, v)
	return nil
}

// store stores v, a value already normalized, in dst, a value of f's Go type,
// by f's shape: a repeated field appends it, and an optional one points at a
// copy of it.
func (f field) store(dst reflect.Value, v string) {
	switch f.shape {
	case shapeOptional:
		dst.Set(reflect.ValueOf(&v))
	case shapeRepeated:
		dst.Set(reflect.Append(dst, reflect.ValueOf(v)))
	default:
		dst.SetString(v)
	}
}

// matches reports whether got, a struct field of T, is f as codegen emits it:
// the same name and Go type, and not embedded.
func (f field) matches(got reflect.StructField) bool {
	return got.Name == f.name && got.Type == f.shape.goType() && !got.Anonymous
}

// A binder is the type codegen emits for one document element declaration,
// as Decode reads it: check holds T to it, item 6 of Decode's doc, reading
// each sealed interface's branch types from sets, and returns the filler that
// sets a value of T from the document element. A *binding binds a complex
// {type definition} codegen gives a struct, a *choiceRoot one it gives a bare
// sealed interface, and a *simpleRoot a simple one.
type binder interface {
	check(t reflect.Type, sets branchSets) (filler, error)
}

// A filler sets out, a value of the T a binder's check held, from the
// document element root, whose start tag r has just returned, and from r's
// tokens up to its end tag, or returns the xsi:type decline Decode's doc
// states, item 7.
type filler interface {
	fill(r *xmltree.Reader, root *xmltree.StartElement, out reflect.Value) error
}

// binding is the struct codegen emits for one document element declaration
// of complex type, as Decode reads it: its fields in codegen's order, its
// content's first, then each attribute use's.
type binding struct {
	schema  *xsd.Schema
	content elementContent
	attrs   []field
	// attributes maps an attribute's expanded name to its field's index in
	// attrs; it is a lookup index only and never iterated (STYLE D2).
	attributes map[xsd.QName]int
}

// elementContent is the element content of the struct a *binding binds, the
// fields that precede its attribute fields, a closed sum: a *sequence, the
// fields of a sequence's particles, or a *choice, the one field holding a
// choice's sealed interface.
type elementContent interface {
	// width is how many of the struct's fields the content's are.
	width() int
	// check holds t's first width fields to the content's, item 6 of Decode's
	// doc, and returns the contentFiller that sets them.
	check(t reflect.Type, sets branchSets) (contentFiller, error)
}

// A contentFiller sets the content's fields of out, the struct an
// elementContent's check held, from the child element start, reading r up to
// its end tag.
type contentFiller interface {
	child(r *xmltree.Reader, start *xmltree.StartElement, out reflect.Value) error
}

// bind returns the binder of root's declaration, or the first decline
// Decode's doc states for it, item 5, at root's instance Loc.
func bind(schema *xsd.Schema, root *xmltree.StartElement) (binder, error) {
	loc := root.Loc()
	name := expanded(root.Name())
	decl, ok := schema.Element(name)
	if !ok {
		// Only an xsi:type lets such a document element be assessed
		// (cvc-assess-elt), so it declines as one, in place of item 5.
		if err := checkXsiType(root); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("codec: %s: the document element %s has no top-level declaration", loc, name)
	}
	if d := declarationDecline(decl); d != 0 {
		return nil, decline(loc, d, declSubject(decl))
	}
	td, ok := schema.ResolvedType(decl.TypeDefinition())
	if !ok {
		return nil, fmt.Errorf("codec: %s: the {type definition} of %s does not resolve", loc, declSubject(decl))
	}
	typeSubject := fmt.Sprintf("the {type definition} of %s, a type defined at %s", declSubject(decl), td.Loc())
	// TypeDefinition is a closed sum of the two, so the assertions are STYLE
	// T2's sealed-sum exception.
	if st, ok := td.(*xsd.SimpleType); ok {
		return bindSimpleRoot(schema, decl, st, loc, typeSubject)
	}
	ct, _ := td.(xsd.ComplexType)
	content, group, d := contentGroup(ct)
	if d != 0 {
		return nil, decline(loc, d, typeSubject)
	}
	b := &binding{schema: schema, attributes: map[xsd.QName]int{}}
	// names is the struct's Namer: the content's fields claim from it in
	// particle order, then the attribute fields (codegen/doc.go "# Particles").
	// A bare sealed interface has no struct and discards it.
	names := goname.NewNamer()
	// ch is the content's choice, nil for a sequence: the one content a bare
	// sealed interface can bind.
	var ch *choice
	var err error
	b.content, ch, err = bindContent(schema, group, names, loc, typeSubject)
	if err != nil {
		return nil, err
	}
	for _, u := range ct.AttributeUses() {
		f, d, subject, err := attributeUse(schema, u)
		if err != nil {
			return nil, err
		}
		if d != 0 {
			return nil, decline(loc, d, subject)
		}
		f.name = names.Claim(u.DeclarationName().Local)
		b.attributes[u.DeclarationName()] = len(b.attrs)
		b.attrs = append(b.attrs, f)
	}
	if _, has := ct.AttributeWildcard(); has {
		return nil, decline(loc, constructAttributeWildcard, typeSubject)
	}
	// codegen/emit.go's shape: a choice occurring once, in a type with no
	// attribute field, is the type itself unless it has {particles} and one
	// occurrence of it is emptiable (cos-group-emptiable).
	if ch != nil && len(b.attrs) == 0 && (len(ch.alts) == 0 || !schema.ParticleEmptiable(content)) {
		return &choiceRoot{choice: ch}, nil
	}
	return b, nil
}

// bindContent returns the elementContent of group, a content sequence or
// choice, its fields named from names, and for a choice that choice, or the
// first decline Decode's doc states for its particles, item 5, at the instance
// Loc loc, typeSubject naming the type.
func bindContent(schema *xsd.Schema, group xsd.ModelGroup, names *goname.Namer, loc xsderr.Loc, typeSubject string) (elementContent, *choice, error) {
	if group.Compositor() == xsd.CompositorChoice {
		ch, err := bindChoice(schema, group.Particles(), names, loc, typeSubject)
		if err != nil {
			return nil, nil, err
		}
		return ch, ch, nil
	}
	s, err := bindSequence(schema, group.Particles(), names, loc, typeSubject)
	if err != nil {
		return nil, nil, err
	}
	return s, nil, nil
}

// sequence is a sequence's particles as the struct codegen emits holds them:
// one field per particle, in particle order.
type sequence struct {
	schema *xsd.Schema
	fields []field
	// children maps a child's expanded name to its field's index; it is a
	// lookup index only and never iterated (STYLE D2).
	children map[xsd.QName]int
}

// bindSequence returns the sequence of particles, a content sequence's, each
// field named from names by its element's local name in particle order, or the
// first decline Decode's doc states for them, item 5, at the instance Loc loc,
// typeSubject naming the type.
func bindSequence(schema *xsd.Schema, particles []xsd.Particle, names *goname.Namer, loc xsderr.Loc, typeSubject string) (*sequence, error) {
	s := &sequence{schema: schema, children: map[xsd.QName]int{}}
	for _, p := range particles {
		el, d := particleElement(p)
		if d != 0 {
			return nil, decline(loc, d, "a particle of "+typeSubject)
		}
		f, d, err := localElement(schema, s.children, el, p.Occurs())
		if err != nil {
			return nil, err
		}
		if d != 0 {
			return nil, decline(loc, d, declSubject(el))
		}
		f.name = names.Claim(el.Name().Local)
		s.children[el.Name()] = len(s.fields)
		s.fields = append(s.fields, f)
	}
	return s, nil
}

// width is the particle count.
func (s *sequence) width() int { return len(s.fields) }

// check holds t's first fields to s's, in name, order and Go type, and
// returns s, which sets them.
func (s *sequence) check(t reflect.Type, _ branchSets) (contentFiller, error) {
	for i, want := range s.fields {
		if got := t.Field(i); !want.matches(got) {
			return nil, fmt.Errorf("%w: field %d of %s is %s %s where codegen emits %s %s", ErrTypeMismatch, i, t, got.Name, got.Type, want.name, want.shape.goType())
		}
	}
	return s, nil
}

// child sets the field of the child element start, reading its character
// children from r up to its end tag: their ·normalized value·, or the
// field's supplied value when there are none, as Decode's doc states.
func (s *sequence) child(r *xmltree.Reader, start *xmltree.StartElement, out reflect.Value) error {
	i, ok := s.children[expanded(start.Name())]
	if !ok {
		return unparticled(start)
	}
	v, err := elementValue(r, start, s.schema, s.fields[i].st, s.fields[i].supplied)
	if err != nil {
		return err
	}
	s.fields[i].store(out.Field(i), v)
	return nil
}

// choice is a choice occurring once as codegen emits it: a sealed interface,
// whose one method is unexported, and one branch struct per alternative, in
// particle order, each holding the one field its alternative's particle
// gives, its {min occurs} 0 read as 1 and its {max occurs} kept
// (codegen/doc.go "# Complex types"). The interface is the type itself
// (*choiceRoot) or the first field of the type's struct (*binding), named
// name. alts holds each alternative's branch field, in particle order.
type choice struct {
	schema *xsd.Schema
	name   string
	alts   []field
	// children maps a child's expanded name to its alternative's index in
	// alts; it is a lookup index only and never iterated (STYLE D2).
	children map[xsd.QName]int
}

// bindChoice returns the choice of particles, a content choice's, its field
// named from names by the local name choice before any alternative is read
// (codegen/doc.go "# Particles"), or the first decline Decode's doc states for
// them, item 5, at the instance Loc loc, typeSubject naming the type: each
// alternative's as a sequence's particle declines, then an alternative whose
// branch struct has an earlier one's shape, which no branch type could tell
// apart.
func bindChoice(schema *xsd.Schema, particles []xsd.Particle, names *goname.Namer, loc xsderr.Loc, typeSubject string) (*choice, error) {
	c := &choice{schema: schema, name: names.Claim("choice"), children: map[xsd.QName]int{}}
	for _, p := range particles {
		el, d := particleElement(p)
		if d != 0 {
			return nil, decline(loc, d, "a particle of "+typeSubject)
		}
		f, d, err := localElement(schema, c.children, el, p.Occurs())
		if err != nil {
			return nil, err
		}
		if d != 0 {
			return nil, decline(loc, d, declSubject(el))
		}
		// A branch's {min occurs} 0 reads as 1: the empty match is the nil
		// interface, so no branch holds an absent value (cos-group-emptiable).
		if f.shape == shapeOptional {
			f.shape = shapeOne
		}
		f.name = goname.NewNamer().Claim(el.Name().Local)
		for _, earlier := range c.alts {
			if earlier.name == f.name && earlier.shape == f.shape {
				return nil, decline(loc, constructSharedBranch, declSubject(el))
			}
		}
		c.children[el.Name()] = len(c.alts)
		c.alts = append(c.alts, f)
	}
	return c, nil
}

// width is the one field holding the interface.
func (c *choice) width() int { return 1 }

// check holds t's first field to the one holding c's interface, and returns
// the boundChoice that sets it.
func (c *choice) check(t reflect.Type, sets branchSets) (contentFiller, error) {
	got := t.Field(0)
	if got.Name != c.name || got.Anonymous {
		return nil, fmt.Errorf("%w: field 0 of %s is %s %s where codegen emits %s, a sealed interface", ErrTypeMismatch, t, got.Name, got.Type, c.name)
	}
	bc, err := c.checkSite(got.Type, fmt.Sprintf("field %s of %s", got.Name, t), sets)
	if err != nil {
		return nil, err
	}
	return bc, nil
}

// checkSite holds it, the Go type at the site site names, to c's sealed
// interface, item 6 of Decode's doc: an interface whose one method is
// unexported, whose branch types sets holds as one struct per alternative of
// the alternative's branch shape. It returns c bound to those branch types.
// Neither the interface's nor a branch's name is compared.
func (c *choice) checkSite(it reflect.Type, site string, sets branchSets) (*boundChoice, error) {
	if it.Kind() != reflect.Interface {
		return nil, fmt.Errorf("%w: %s is %s, no interface, where codegen emits a sealed interface", ErrTypeMismatch, site, it)
	}
	if it.NumMethod() != 1 || it.Method(0).IsExported() {
		return nil, fmt.Errorf("%w: %s is %s, which is not an interface of one unexported method, codegen's sealed interface", ErrTypeMismatch, site, it)
	}
	branches, supplied := sets.of(it)
	if !supplied && len(c.alts) > 0 {
		return nil, fmt.Errorf("%w: no WithBranches supplies the branches of %s, the interface of %s, where codegen emits %d", ErrTypeMismatch, it, site, len(c.alts))
	}
	if len(branches) != len(c.alts) {
		return nil, fmt.Errorf("%w: WithBranches supplies %d branches of %s where codegen emits %d", ErrTypeMismatch, len(branches), it, len(c.alts))
	}
	// As many branches as alternatives, each matching one no earlier branch
	// matched, leave no entry of matched nil.
	matched := make([]reflect.Type, len(c.alts))
	for i, bt := range branches {
		if bt == nil {
			return nil, fmt.Errorf("%w: WithBranches supplies a nil branch of %s", ErrTypeMismatch, it)
		}
		if bt.Kind() != reflect.Struct {
			return nil, fmt.Errorf("%w: WithBranches supplies %s, no struct, as a branch of %s", ErrTypeMismatch, bt, it)
		}
		if slices.Contains(branches[:i], bt) {
			return nil, fmt.Errorf("%w: WithBranches supplies %s twice as a branch of %s", ErrTypeMismatch, bt, it)
		}
		j := slices.IndexFunc(c.alts, func(a field) bool {
			return bt.NumField() == 1 && a.matches(bt.Field(0))
		})
		if j < 0 || matched[j] != nil {
			return nil, fmt.Errorf("%w: branch %s of %s has the shape of no branch struct codegen emits that an earlier branch did not match", ErrTypeMismatch, bt, it)
		}
		matched[j] = bt
	}
	return &boundChoice{choice: c, branches: matched}, nil
}

// boundChoice is a choice whose sealed interface checkSite held: branches[i]
// is the branch struct of choice.alts[i], none nil.
type boundChoice struct {
	choice   *choice
	branches []reflect.Type
}

// child sets the choice's field of out, the struct, from the child element
// start.
func (bc *boundChoice) child(r *xmltree.Reader, start *xmltree.StartElement, out reflect.Value) error {
	return bc.set(r, start, out.Field(0))
}

// set sets dst, a value of the choice's interface, from the child element
// start, reading its character children from r up to its end tag: the branch
// of start's alternative holds their ·normalized value·, or the field's
// supplied value when there are none, as Decode's doc states, a repeated
// alternative's appended to the branch dst already holds.
func (bc *boundChoice) set(r *xmltree.Reader, start *xmltree.StartElement, dst reflect.Value) error {
	i, ok := bc.choice.children[expanded(start.Name())]
	if !ok {
		return unparticled(start)
	}
	alt, bt := bc.choice.alts[i], bc.branches[i]
	branch := reflect.New(bt).Elem()
	if !dst.IsNil() {
		// Validation charges a child of a second alternative first
		// (cvc-model-group); this is the defensive check.
		if dst.Elem().Type() != bt {
			return fmt.Errorf("codec: %s: element %s matches a second alternative of the choice", start.Loc(), expanded(start.Name()))
		}
		branch.Set(dst.Elem())
	}
	v, err := elementValue(r, start, bc.choice.schema, alt.st, alt.supplied)
	if err != nil {
		return err
	}
	alt.store(branch.Field(0), v)
	dst.Set(branch)
	return nil
}

// choiceRoot is the bare sealed interface codegen emits for a document
// element declaration whose complex type's content is choice and which has no
// attribute field, as Decode reads it.
type choiceRoot struct {
	choice *choice
}

// check holds t, T itself, to the choice's sealed interface, and returns the
// boundChoiceRoot that fills it.
func (c *choiceRoot) check(t reflect.Type, sets branchSets) (filler, error) {
	bc, err := c.choice.checkSite(t, "T", sets)
	if err != nil {
		return nil, err
	}
	return boundChoiceRoot{choice: bc}, nil
}

// boundChoiceRoot is a choiceRoot whose T check held.
type boundChoiceRoot struct {
	choice *boundChoice
}

// fill sets out, a value of the interface, as a filler does. An empty match
// leaves out nil.
func (c boundChoiceRoot) fill(r *xmltree.Reader, root *xmltree.StartElement, out reflect.Value) error {
	if err := checkXsiType(root); err != nil {
		return err
	}
	if err := noAttributes(root); err != nil {
		return err
	}
	return eachChild(r, func(start *xmltree.StartElement) error { return c.choice.set(r, start, out) })
}

// bindSimpleRoot returns the simpleRoot of decl, a top-level element
// declaration whose {type definition} is st, or the decline Decode's doc
// states for st, item 5, at the instance Loc loc, subject naming st.
func bindSimpleRoot(schema *xsd.Schema, decl xsd.ElementDeclaration, st *xsd.SimpleType, loc xsderr.Loc, subject string) (binder, error) {
	d, err := simpleDecline(schema, st)
	if err != nil {
		return nil, err
	}
	if d != 0 {
		return nil, decline(loc, d, subject)
	}
	s := &simpleRoot{schema: schema, st: st}
	if vc, has := decl.ValueConstraint(); has {
		s.supplied, err = supply(schema, st, vc, declSubject(decl))
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// declarationDecline returns the construct of an element declaration's own
// properties Decode declines, in the order its doc states, or 0.
func declarationDecline(decl xsd.ElementDeclaration) construct {
	if _, has := decl.TypeTable(); has {
		return constructTypeTable
	}
	if decl.Nillable() {
		return constructNillable
	}
	return 0
}

// contentGroup returns the {particle} of ct's content and its model group, an
// inline sequence or choice occurring exactly once, or the construct Decode
// declines in its place. ContentType and the particle term are closed sums, so
// the switches are STYLE T2's sealed-sum exception.
func contentGroup(ct xsd.ComplexType) (xsd.Particle, xsd.ModelGroup, construct) {
	var content xsd.ElementContent
	switch c := ct.ContentType().(type) {
	case xsd.EmptyContent:
		return xsd.Particle{}, xsd.ModelGroup{}, constructEmptyContent
	case xsd.SimpleContent:
		return xsd.Particle{}, xsd.ModelGroup{}, constructSimpleContent
	case xsd.ElementContent:
		content = c
	}
	if content.Mixed {
		return xsd.Particle{}, xsd.ModelGroup{}, constructMixedContent
	}
	if content.OpenContent != nil {
		return xsd.Particle{}, xsd.ModelGroup{}, constructOpenContent
	}
	if _, isRef := content.Particle.Term().(xsd.ModelGroupRef); isRef {
		return xsd.Particle{}, xsd.ModelGroup{}, constructGroupRefContent
	}
	resolved, _ := content.Particle.Term().(xsd.ResolvedTerm)
	group, _ := resolved.Term.(xsd.ModelGroup)
	occurs := content.Particle.Occurs()
	upper, bounded := occurs.Max()
	once := occurs.Min() == 1 && bounded && upper == 1
	switch group.Compositor() {
	case xsd.CompositorChoice:
		if !once {
			return xsd.Particle{}, xsd.ModelGroup{}, constructChoiceOccurs
		}
		return content.Particle, group, 0
	case xsd.CompositorAll:
		return xsd.Particle{}, xsd.ModelGroup{}, constructAllContent
	case xsd.CompositorSequence:
		if !once {
			return xsd.Particle{}, xsd.ModelGroup{}, constructSequenceOccurs
		}
		return content.Particle, group, 0
	}
	// The {particle} of element-only content is a model group (§3.4.2.3.3), so
	// the zero ModelGroup, no compositor, is a component no schema document
	// yields; decline it with the nearest construct rather than read it.
	return xsd.Particle{}, xsd.ModelGroup{}, constructNestedGroup
}

// particleElement returns the local element declaration p's term holds, or
// the construct Decode declines in its place: a model group, inline or
// through <group ref>, is a nested model group particle.
func particleElement(p xsd.Particle) (xsd.ElementDeclaration, construct) {
	switch term := p.Term().(type) {
	case xsd.ElementDeclarationRef:
		return xsd.ElementDeclaration{}, constructElementRef
	case xsd.ResolvedTerm:
		switch inner := term.Term.(type) {
		case xsd.ElementDeclaration:
			return inner, 0
		case xsd.Wildcard:
			return xsd.ElementDeclaration{}, constructWildcardParticle
		}
	}
	return xsd.ElementDeclaration{}, constructNestedGroup
}

// localElement returns the field, but for its name, which its caller claims,
// of the local element declaration el in a particle occurring as occurs, or
// the construct Decode declines it for, in the order Decode's doc states:
// its own properties, its type, and an expanded name an earlier particle of
// its model group shares, children indexing the earlier particles' names.
func localElement(schema *xsd.Schema, children map[xsd.QName]int, el xsd.ElementDeclaration, occurs xsd.Occurs) (field, construct, error) {
	if d := declarationDecline(el); d != 0 {
		return field{}, d, nil
	}
	td, ok := schema.ResolvedType(el.TypeDefinition())
	if !ok {
		return field{}, 0, fmt.Errorf("codec: the {type definition} of %s does not resolve", declSubject(el))
	}
	st, ok := td.(*xsd.SimpleType)
	if !ok {
		return field{}, constructComplexElement, nil
	}
	d, err := simpleDecline(schema, st)
	if err != nil || d != 0 {
		return field{}, d, err
	}
	if _, shared := children[el.Name()]; shared {
		return field{}, constructSharedName, nil
	}
	s, err := occursShape(occurs, el)
	if err != nil {
		return field{}, 0, err
	}
	f := field{shape: s, st: st}
	if vc, has := el.ValueConstraint(); has {
		f.supplied, err = supply(schema, st, vc, declSubject(el))
	}
	return f, 0, err
}

// attributeUse returns the field, but for its name, which its caller claims,
// of attribute use u, or the construct Decode declines it for and the subject
// naming it. The field's shape is codegen's, as the package doc's "# What
// decodes" states, and needs no xsi: test here: an xsi: use declines before
// its shape is picked.
func attributeUse(schema *xsd.Schema, u xsd.AttributeUse) (field, construct, string, error) {
	decl, ok := schema.ResolvedAttributeDeclaration(u)
	if !ok {
		return field{}, 0, "", fmt.Errorf("codec: the {attribute declaration} of attribute use %s does not resolve", u.DeclarationName())
	}
	subject := fmt.Sprintf("attribute %s declared at %s", decl.Name(), decl.Loc())
	if decl.Name().Space == xsd.XMLSchemaInstanceNS {
		return field{}, constructXsiAttribute, subject, nil
	}
	st, ok := schema.ResolvedSimpleType(decl.TypeDefinition())
	if !ok {
		return field{}, 0, "", fmt.Errorf("codec: the {type definition} of %s does not resolve", subject)
	}
	d, err := simpleDecline(schema, st)
	if err != nil || d != 0 {
		return field{}, d, subject, err
	}
	f := field{shape: shapeOptional, st: st}
	vc, has := schema.EffectiveValueConstraint(u)
	if u.Required() || has {
		f.shape = shapeOne
	}
	if has {
		f.supplied, err = supply(schema, st, vc, subject)
	}
	return f, 0, subject, err
}

// supply returns the ·schema normalized value· the post-schema-validation
// infoset supplies from the value constraint vc of the item subject names,
// whose simple type is st: vc's {lexical form} as [value.ApplyWhiteSpace]
// normalizes it under st (§3.3.5.4 sic-eltType clause 1.1, §3.4.5.1
// sic-attrDefault).
func supply(schema *xsd.Schema, st *xsd.SimpleType, vc xsd.ValueConstraint, subject string) (*string, error) {
	v, err := value.ApplyWhiteSpace(schema, st, vc.LexicalForm())
	if err != nil {
		return nil, fmt.Errorf("codec: normalizing the {value constraint} of %s: %w", subject, err)
	}
	return &v, nil
}

// simpleDecline returns the construct Decode declines the simple type st
// for, or 0 for an atomic type whose {primitive type definition} is
// xs:string or xs:anyURI. {variety} is a closed sum, so the switch is STYLE
// T2's sealed-sum exception.
func simpleDecline(r xsd.TypeResolver, st *xsd.SimpleType) (construct, error) {
	variety, err := st.Variety(r)
	if err != nil {
		return 0, err
	}
	switch variety.(type) {
	case xsd.List:
		return constructList, nil
	case xsd.Union:
		return constructUnion, nil
	}
	primitive, err := st.Primitive(r)
	if err != nil {
		return 0, err
	}
	switch {
	case primitive == nil:
		// xs:anySimpleType, of absent {variety}, and xs:anyAtomicType.
		return constructSpecial, nil
	case primitive.Name() != stringPrimitive && primitive.Name() != anyURIPrimitive:
		return constructOtherPrimitive, nil
	}
	return 0, nil
}

// occursShape returns the shape codegen gives the field of a particle of el
// occurring as occurs, by codegen's package doc "# Particles" pointer and
// slice rules. Decode's check of T is what holds the two to one answer.
func occursShape(occurs xsd.Occurs, el xsd.ElementDeclaration) (shape, error) {
	upper, bounded := occurs.Max()
	switch {
	case !bounded || upper > 1:
		return shapeRepeated, nil
	case upper == 0:
		return 0, fmt.Errorf("codec: the particle of %s has {max occurs} 0, for which codegen emits no type", declSubject(el))
	case occurs.Min() == 0:
		return shapeOptional, nil
	}
	return shapeOne, nil
}

// check returns the boundBinding that fills t when t is the struct codegen
// emits for b, and otherwise the ErrTypeMismatch error Decode's doc states,
// item 6.
func (b *binding) check(t reflect.Type, sets branchSets) (filler, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%w: %s is no struct", ErrTypeMismatch, t)
	}
	width := b.content.width()
	if t.NumField() != width+len(b.attrs) {
		return nil, fmt.Errorf("%w: %s has %d fields where codegen emits %d", ErrTypeMismatch, t, t.NumField(), width+len(b.attrs))
	}
	cf, err := b.content.check(t, sets)
	if err != nil {
		return nil, err
	}
	for i, want := range b.attrs {
		if got := t.Field(width + i); !want.matches(got) {
			return nil, fmt.Errorf("%w: field %d of %s is %s %s where codegen emits %s %s", ErrTypeMismatch, width+i, t, got.Name, got.Type, want.name, want.shape.goType())
		}
	}
	return &boundBinding{binding: b, content: cf}, nil
}

// boundBinding is a binding whose T check held, content its content's
// contentFiller.
type boundBinding struct {
	binding *binding
	content contentFiller
}

// fill sets out's fields as a filler does. An attribute field with a supplied
// value holds it unless its attribute is present, as Decode's doc states.
func (b *boundBinding) fill(r *xmltree.Reader, root *xmltree.StartElement, out reflect.Value) error {
	if err := checkXsiType(root); err != nil {
		return err
	}
	bd := b.binding
	width := bd.content.width()
	for i, f := range bd.attrs {
		if f.supplied != nil {
			f.store(out.Field(width+i), *f.supplied)
		}
	}
	for _, a := range root.Attributes() {
		i, ok := bd.attributes[expanded(a.Name())]
		if ok {
			if err := bd.attrs[i].set(bd.schema, out.Field(width+i), a.Value()); err != nil {
				return err
			}
			continue
		}
		if a.Name().Space() != xsd.XMLSchemaInstanceNS {
			return unattributed(a)
		}
	}
	return eachChild(r, func(start *xmltree.StartElement) error { return b.content.child(r, start, out) })
}

// eachChild reads r's tokens up to the end tag of the element whose start tag
// r has just returned, and calls child for each child element's start tag,
// which reads r up to that child's end tag, once that child carries no
// xsi:type, or returns the xsi:type decline Decode's doc states, item 7.
func eachChild(r *xmltree.Reader, child func(*xmltree.StartElement) error) error {
	for {
		tok, err := r.Token()
		if err != nil {
			return err
		}
		switch tok := tok.(type) {
		case *xmltree.EndElement:
			return nil
		case *xmltree.StartElement:
			if err := checkXsiType(tok); err != nil {
				return err
			}
			if err := child(tok); err != nil {
				return err
			}
		}
	}
}

// unparticled is the error for the child element start when no particle of
// its parent's content attributes it.
func unparticled(start *xmltree.StartElement) error {
	return fmt.Errorf("codec: %s: element %s is attributed to no particle", start.Loc(), expanded(start.Name()))
}

// elementValue reads the character children of the element start, of simple
// type st, from r up to its end tag and returns their ·normalized value·
// under st, or supplied, already normalized, when it is not nil and there are
// none, the value the element's {value constraint} supplies (cvc-elt clause
// 5.1), as Decode's doc states.
func elementValue(r *xmltree.Reader, start *xmltree.StartElement, schema *xsd.Schema, st *xsd.SimpleType, supplied *string) (string, error) {
	var text bytes.Buffer
	for {
		tok, err := r.Token()
		if err != nil {
			return "", err
		}
		switch tok := tok.(type) {
		case *xmltree.CharData:
			text.WriteString(tok.Data())
		case *xmltree.StartElement:
			return "", fmt.Errorf("codec: %s: element %s of simple type holds element %s", tok.Loc(), expanded(start.Name()), expanded(tok.Name()))
		case *xmltree.EndElement:
			if supplied != nil && text.Len() == 0 {
				return *supplied, nil
			}
			v, err := value.ApplyWhiteSpace(schema, st, text.String())
			if err != nil {
				return "", fmt.Errorf("codec: normalizing the value of element %s: %w", expanded(start.Name()), err)
			}
			return v, nil
		}
	}
}

// simpleRoot is the type codegen emits for a document element declaration
// whose {type definition} is st, a simple type Decode decodes, as Decode reads
// it; supplied is the value its {value constraint} supplies, already
// normalized under st, nil exactly when there is none.
type simpleRoot struct {
	schema   *xsd.Schema
	st       *xsd.SimpleType
	supplied *string
}

// check returns s, which fills t, when t's underlying type is string, and
// otherwise the ErrTypeMismatch error Decode's doc states, item 6. t's name is
// never compared.
func (s *simpleRoot) check(t reflect.Type, _ branchSets) (filler, error) {
	if t.Kind() != reflect.String {
		return nil, fmt.Errorf("%w: the underlying type of %s is not string", ErrTypeMismatch, t)
	}
	return s, nil
}

// fill sets out, whose underlying type is string, as a filler does.
func (s *simpleRoot) fill(r *xmltree.Reader, root *xmltree.StartElement, out reflect.Value) error {
	if err := checkXsiType(root); err != nil {
		return err
	}
	if err := noAttributes(root); err != nil {
		return err
	}
	v, err := elementValue(r, root, s.schema, s.st, s.supplied)
	if err != nil {
		return err
	}
	out.SetString(v)
	return nil
}

// noAttributes returns the error for the first attribute of the document
// element root outside the xsi namespace, for a type with no attribute use.
func noAttributes(root *xmltree.StartElement) error {
	for _, a := range root.Attributes() {
		if a.Name().Space() != xsd.XMLSchemaInstanceNS {
			return unattributed(a)
		}
	}
	return nil
}

// unattributed is the error for the attribute a of the document element when
// no attribute use of its declaration's type attributes it.
func unattributed(a xmltree.Attribute) error {
	return fmt.Errorf("codec: %s: attribute %s is attributed to no attribute use", a.Loc(), expanded(a.Name()))
}

// checkXsiType returns the xsi:type decline for start when it carries one.
// It matches the attribute by expanded name alone: no QName is resolved.
func checkXsiType(start *xmltree.StartElement) error {
	for _, a := range start.Attributes() {
		if a.Name().Space() == xsd.XMLSchemaInstanceNS && a.Name().Local() == "type" {
			return decline(start.Loc(), constructXsiType, fmt.Sprintf("element %s with xsi:type %q", expanded(start.Name()), a.Value()))
		}
	}
	return nil
}

// declSubject names the element declaration decl and its schema Loc.
func declSubject(decl xsd.ElementDeclaration) string {
	return fmt.Sprintf("element %s declared at %s", decl.Name(), decl.Loc())
}

// expanded returns the expanded name of an instance name.
func expanded(n xmltree.Name) xsd.QName {
	return xsd.QName{Space: n.Space(), Local: n.Local()}
}
