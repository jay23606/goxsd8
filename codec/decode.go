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

// Decode decodes the XML instance data into a new value of T, the type
// package codegen emits for the declaration of data's document element, and
// returns it. A value is returned exactly when the error is nil.
//
// T is the caller's choice: nothing dispatches a document element to its
// generated type, and item 6 below never compares T's own name. For a complex
// {type definition} item 6 compares T's fields with those codegen emits for
// the declaration in name, order and Go type alone, a field's type as item 6
// reads it, so a T whose fields differ returns [ErrTypeMismatch] where item 6
// is the first that applies, but a T of the same shape, such as the struct
// codegen emits for another top-level element of the same type, passes item 6
// and decodes as the declaration's own struct does, an error only where that
// one's is. For a simple one item 6 reads T's underlying type alone, so any T
// over the Go type codegen gives that simple type, its atomic Go type A or,
// for a list, []A, passes it, such as the type codegen emits for another
// document element of the same simple type. A caller decoding documents of
// several roots therefore reads the document element's name itself, through
// parser/xmltree or encoding/xml, before choosing T.
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
// simple type, T's own, is read from the item's ·normalized value·
// (Structures §3.1.4 key-nv) under its declaration's simple type: the string
// [value.ApplyWhiteSpace] returns for the item's characters, an attribute's as
// XML 1.0 §3.3.3 normalized them, an element's the concatenation of its
// character children. Where the post-schema-validation infoset supplies a value
// it is read from that value, the {lexical form} of a value constraint under
// the same normalization: for an absent attribute whose use has an effective
// value constraint (§3.5.4 key-evc; §3.4.4.2 key-dflt-att, §3.4.5.1
// sic-attrDefault), and for an element with neither element nor character
// children whose declaration has a {value constraint} (§3.3.4.3 cvc-elt clause
// 5.1, §3.3.5.4 sic-eltType). For a simple type whose atomic Go type is string
// the value is that string. For every other it is the value backend's
// governing mapping of the type ([value.GoverningMapping]) maps that string
// to, through Parse with a nil value.Context (Datatypes §4.1.4
// cvc-datatype-valid clause 2.1), read back through that mapping's Canonical
// and converted to the Go type: a boolean's canonical form exactly "true" or
// "false" (§E.4 f-booleanCanmap), a float's and a double's by
// strconv.ParseFloat at the Go type's width, and a decimal's into big.Rat,
// big.Int or a bounded Go integer at its width. backend's governing mapping of
// each such type must therefore have a non-nil Canonical, as builtin/strict's
// do; item 8 states the error where one fails. For a list type, whose
// whiteSpace is collapse (Datatypes §4.3.6, §2.4.1.2), the normalized string
// is split at each U+0020 into its list items before any is mapped, an empty
// one into none (§4.1.1 dt-list), and each item is read as above under the
// list's {item type definition}, a restriction of a list taking its base's
// (Structures §3.16.2.3); the value is the []A of the items in order. So the
// value is the instance's own even where a fixed value constraint holds it to
// an equal one that is not identical (§3.5.4 cvc-au, cvc-elt clause 5.2.2.2.2):
// -0 against fixed 0 on xs:double decodes to negative zero. The field of an
// optional attribute use with an effective value constraint is the Go type
// codegen gives its simple type itself, as codegen emits it, holding the
// supplied value when the attribute is absent; a whitespace-only element has a
// character child and holds its own value; and an element absent from the
// instance supplies nothing, its field nil or not appended to.
//
// Every instance Loc an error of Decode carries — a source fault's, a
// violation's, an undecided check's, a decline's and item 8's — names the
// document [WithURI] gives; without it, or with an empty uri, the URI
// renders "?".
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
//     for a list its {item type definition}'s, whose construct the list
//     declines by, after which nothing further is examined, or a complex
//     type's content, then the occurrence of its sequence or choice, each
//     particle of its content in order — a nested model group or wildcard,
//     then, of an <element ref>, the referenced declaration's substitution
//     group, then the particle's element declaration's {type table} and
//     {nillable}, its {type definition}, any complex one declining and a
//     simple one as the document element's does, and an expanded name an
//     earlier particle shares; of a choice, each alternative's own
//     constructs, then a branch field of the name and Go type, as item 6
//     reads it, an earlier alternative's has, which declines as two choice
//     alternatives whose branch structs have one shape — each of its
//     attribute uses in order, and its {attribute wildcard}. A component of
//     the declaration or its type that does not resolve, or a particle with
//     {max occurs} 0, is a plain error in place of a decline, at its point in
//     this order.
//  6. T does not match the type codegen emits for the declaration, as Decode
//     derives it from the schema: for a complex {type definition}, T is not a
//     struct whose fields are exactly those codegen emits, in name, order and
//     Go type, or, where codegen emits the choice's sealed interface as the
//     type itself, T is not an interface; for a simple one, T's underlying
//     type is not the Go type codegen gives it (the Simple types section of
//     codegen's package doc): its atomic Go type A, string, bool, float32,
//     float64, big.Rat, big.Int or a bounded Go integer, or for a list []A,
//     A its {item type definition}'s. A field's Go type is V, *V or []V, V the
//     Go type codegen gives its simple type, A or []A, for a local element
//     declaration's particle and an attribute use; for an <element ref>
//     particle it is N, *N or []N, N the named type codegen emits for the
//     referenced declaration, matched as any type other than V whose
//     underlying type is V, never by name; V itself, a local particle's type,
//     does not pass, so a ref's field and a local one's never accept one Go
//     type. A sealed interface, T or the struct's first field, does not match
//     when it is not an interface whose one method is unexported, or when its
//     branch set, as [WithBranches] supplied it, is not one struct of one
//     field per alternative, that field the name and Go type of the
//     alternative's branch field: when no WithBranches supplies it for a
//     choice with {particles}, or its count differs, or it holds a nil value,
//     a type that is no struct, a type twice, or a branch matching no
//     alternative an earlier branch did not match. It is an error errors.Is
//     reports as [ErrTypeMismatch], no *DeclineError, so that a T that does
//     not match the schema is never filled. Decode never reads the fields
//     from T, so for a type codegen emitted from this schema the error is a
//     defect of Decode's, or a branch set WithBranches did not supply as
//     codegen emitted it, and for any other T it is a T that does not match.
//  7. An element carrying xsi:type, the document element or a child: a
//     *DeclineError at that element's instance Loc, the first in document
//     order, whose [DeclineError.Construct] is xsi:type's. A document element
//     no top-level declaration declares, which validation accepts only
//     through its xsi:type (§3.3.4.6 cvc-assess-elt), declines so in place of
//     item 5.
//  8. The lexical mapping or the conversion to the Go type failing on a value
//     validate accepted on the same backend: no mapping of backend governs
//     the type, its Parse rejects the normalized value, or its Canonical is
//     nil, fails, or renders a form the Go type does not read; for a list,
//     the type is its {item type definition} and the value one list item. It
//     is a fault of the backend, not of the instance. The error names the
//     stage, the type — its QName, or for an anonymous type the declaration
//     it is local to and that declaration's schema Loc, and for a list's item
//     type the list's too — the normalized value, or the list item and its
//     1-based position among the value's items, and the value's source at its
//     instance Loc, for a list the whole value's and never an item's: an
//     element's character content, at its first character data, with that
//     data's byte offset; a present attribute, at its Loc
//     (xmltree.Attribute.Loc), with no offset; or a supplied value, at its
//     element's start, with the declaration whose value constraint supplied
//     it. It wraps nothing: errors.As finds no *xsderr.Error, *DeclineError or
//     *UndecidedError in it, xsderr.RuleOf no rule, and errors.Is neither
//     errors.ErrUnsupported nor [ErrTypeMismatch]; and [ViolationsOf] returns
//     nil for it.
//
// Items 5 and 6 read no instance; [CheckDecode] returns them for a
// declaration before an instance arrives.
//
// Items 7 and 8 are both found while filling T, and the first in document
// order is returned: an element's xsi:type, then its attributes' values,
// present attributes in start-tag order and then the supplied values of
// absent ones in attribute-use order, then its children. A supplied value
// whose attribute is present is never mapped.
func Decode[T any](schema *xsd.Schema, backend value.Backend, data []byte, opts ...Option) (*T, error) {
	cfg := newConfig(opts)
	v, err := validate.New(schema, backend, validate.WithLogger(cfg.log))
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
	b, err := bind(&pipeline{schema: schema, backend: backend, log: cfg.log.WithGroup("codec")}, root)
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

// CheckDecode returns, reading no instance, the error items 5 and 6 of
// [Decode]'s doc state for a document element of the top-level element
// declaration name in schema decoded into T under opts, or nil when neither
// applies. Of opts it reads only the sets [WithBranches] supplies. The first
// of these that applies is returned:
//
//  1. A nil schema: a plain error.
//  2. No top-level element declaration of schema has the expanded name name:
//     a plain error naming name.
//  3. The declaration has {abstract} true: a plain error, since no document
//     element validates against it (Structures §3.3.4.3 cvc-elt clause 2).
//  4. Item 5 of Decode's doc: its *DeclineError, charged to no instance Loc
//     ([DeclineError.Loc]), or the plain error it states in a decline's
//     place.
//  5. Item 6 of Decode's doc: its error, which errors.Is reports as
//     [ErrTypeMismatch].
//
// A plain error is no *DeclineError, wraps neither [ErrTypeMismatch] nor
// errors.ErrUnsupported, and carries no xsderr rule. nil means Decode returns
// neither its item 5 nor its item 6 for a document element of name; Decode's
// items 2 to 4, 7 and 8 depend on the instance.
func CheckDecode[T any](schema *xsd.Schema, name xsd.QName, opts ...Option) error {
	if schema == nil {
		return errors.New("codec: CheckDecode: nil *xsd.Schema")
	}
	decl, ok := schema.Element(name)
	if !ok {
		return fmt.Errorf("codec: no top-level element declaration has the expanded name %q", name.String())
	}
	// An answer about the schema, no violation of an instance, so it carries
	// no xsderr rule (STYLE E2).
	if decl.Abstract() {
		return fmt.Errorf("codec: %s has {abstract} true, so no document element validates against it (cvc-elt clause 2)", declSubject(decl))
	}
	b, err := bindDecl(&pipeline{schema: schema}, decl)
	if err != nil {
		return err
	}
	_, err = b.check(reflect.TypeFor[T](), newConfig(opts).branches)
	return err
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
// tells the two apart. It carries only the first, in document order, of the
// Result's validate.Unevaluated checks; the full list is
// validate.Result.Unevaluated, which a caller reaches by validating with
// package validate directly (validate.New, then validate/xmlsrc.Validate).
type UndecidedError struct{ u validate.Unevaluated }

// Unevaluated returns the one check e carries: its rule, its instance Loc and
// its message.
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

// shape is the occurrence a field holds its value in, by codegen's
// occurrence rules: one value, an optional one, or a repeated one. The zero
// value is invalid (STYLE T1).
type shape uint8

const (
	shapeOne      shape = iota + 1 // V
	shapeOptional                  // *V
	shapeRepeated                  // []V
)

// store stores v, a decoded value of the Go type codegen gives a simple type
// (scalar.goType), in dst, a value of a Go type of shape s over a type v
// converts to: a repeated one appends it, an optional one points at a copy of
// it, each converted to dst's own element type, and shapeOne sets dst to it
// converted to dst's type.
func (s shape) store(dst, v reflect.Value) {
	switch s {
	case shapeOptional:
		p := reflect.New(dst.Type().Elem())
		p.Elem().Set(v.Convert(dst.Type().Elem()))
		dst.Set(p)
	case shapeRepeated:
		dst.Set(reflect.Append(dst, v.Convert(dst.Type().Elem())))
	default:
		dst.Set(v.Convert(dst.Type()))
	}
}

// valueType is how the Go type V a field's shape holds its value in relates
// to the Go type codegen gives the field's simple type (scalar.goType), by
// codegen's field rules: that type itself for a local element declaration's
// particle and an attribute use, and, for an <element ref> particle, the
// named type codegen emits for the referenced top-level declaration, whose
// underlying type it is. The zero value is invalid (STYLE T1).
type valueType uint8

const (
	valueBare  valueType = iota + 1 // the simple type's Go type itself
	valueNamed                      // a named type over the simple type's Go type
)

// field is one field of a struct codegen emits: of the document element's
// declaration's struct, or of a choice's branch struct. It holds its Go name,
// its shape, its value type, and the scalar its value decodes as.
type field struct {
	name  string
	shape shape
	value valueType
	scalar
}

// matches reports whether got, a struct field of T, is f as codegen emits it:
// the same name, a Go type f accepts, and not embedded.
func (f field) matches(got reflect.StructField) bool {
	return got.Name == f.name && f.accepts(got.Type) && !got.Anonymous
}

// accepts reports whether t is the Go type codegen gives f, as Decode's doc,
// item 6, reads it: f's shape over a value type that is the Go type codegen
// gives f's simple type (scalar.goType) itself for valueBare, and for
// valueNamed a type other than it whose underlying type it is, its name never
// compared. Equal Kinds of two scalar types mean one underlying type; for
// big.Rat and big.Int, ConvertibleTo means an identical underlying type, which
// math/big's unexported fields keep any other struct from having; and for two
// slice types, Kind Slice and ConvertibleTo mean an identical underlying type,
// since Go converts between slice types only then.
func (f field) accepts(t reflect.Type) bool {
	switch f.shape {
	case shapeOne:
	case shapeOptional:
		if t.Kind() != reflect.Pointer {
			return false
		}
		t = t.Elem()
	case shapeRepeated:
		if t.Kind() != reflect.Slice {
			return false
		}
		t = t.Elem()
	default:
		// The zero shape is an internal bug, never input.
		panic(fmt.Sprintf("codec: invalid shape %d", f.shape))
	}
	want := f.goType()
	switch f.value {
	case valueBare:
		return t == want
	case valueNamed:
		return t != want && t.Kind() == want.Kind() && t.ConvertibleTo(want)
	}
	// The zero valueType is an internal bug, never input.
	panic(fmt.Sprintf("codec: invalid value type %d", f.value))
}

// sharesBranch reports whether f and g, two choice alternatives' branch
// fields, accept a common Go type, as accepts reads one: the same name and
// value type, and then, for valueBare, the same Go type of shape over
// scalar.goType, a repeated xs:int and a list of xs:int each []int32; for
// valueNamed, the same shape and the same goType separately, since the named
// type sits between them and []N over int32 is no N over []int32.
func (f field) sharesBranch(g field) bool {
	if f.name != g.name || f.value != g.value {
		return false
	}
	if f.value == valueNamed {
		return f.shape == g.shape && f.goType() == g.goType()
	}
	return f.shaped() == g.shaped()
}

// shaped returns the Go type of f's shape over scalar.goType: V, *V or []V.
func (f field) shaped() reflect.Type {
	switch f.shape {
	case shapeOne:
		return f.goType()
	case shapeOptional:
		return reflect.PointerTo(f.goType())
	case shapeRepeated:
		return reflect.SliceOf(f.goType())
	}
	// The zero shape is an internal bug, never input.
	panic(fmt.Sprintf("codec: invalid shape %d", f.shape))
}

// describe spells the Go type codegen gives f for an ErrTypeMismatch message:
// V, *V or []V, V the Go type codegen gives f's simple type, A or for a list
// []A, or the same over N for a named type N over V.
func (f field) describe() string {
	var prefix string
	switch f.shape {
	case shapeOne:
	case shapeOptional:
		prefix = "*"
	case shapeRepeated:
		prefix = "[]"
	}
	if f.value == valueNamed {
		return prefix + "N for a named type N over " + f.goType().String()
	}
	return prefix + f.goType().String()
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
	p       *pipeline
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

// bind returns the binder of root's declaration in p's schema, or the first
// decline Decode's doc states for it, item 5, charged to root's instance Loc.
// Item 5 is bindDecl's, which CheckDecode calls too.
func bind(p *pipeline, root *xmltree.StartElement) (binder, error) {
	name := expanded(root.Name())
	decl, ok := p.schema.Element(name)
	if !ok {
		// Only an xsi:type lets such a document element be assessed
		// (cvc-assess-elt), so it declines as one, in place of item 5.
		if err := checkXsiType(root); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("codec: %s: the document element %s has no top-level declaration", root.Loc(), name)
	}
	b, err := bindDecl(p, decl)
	// bindDecl returns its declines unwrapped and uncharged, and no other
	// error of its wraps one, so the charged copy replaces err whole.
	var de *DeclineError
	if errors.As(err, &de) {
		return nil, de.at(root.Loc())
	}
	return b, err
}

// bindDecl returns the binder of decl, a top-level element declaration in p's
// schema, or the first decline Decode's doc states for it, item 5, charged to
// no instance Loc, or the plain error item 5 states in a decline's place. It
// reads only p's schema.
func bindDecl(p *pipeline, decl xsd.ElementDeclaration) (binder, error) {
	schema := p.schema
	if d := declarationDecline(decl); d != 0 {
		return nil, decline(d, declSubject(decl))
	}
	td, ok := schema.ResolvedType(decl.TypeDefinition())
	if !ok {
		return nil, fmt.Errorf("codec: the {type definition} of %s does not resolve", declSubject(decl))
	}
	typeSubject := fmt.Sprintf("the {type definition} of %s, a type defined at %s", declSubject(decl), td.Loc())
	// TypeDefinition is a closed sum of the two, so the assertions are STYLE
	// T2's sealed-sum exception.
	if st, ok := td.(*xsd.SimpleType); ok {
		return bindSimpleRoot(p, decl, st, typeSubject)
	}
	ct, _ := td.(xsd.ComplexType)
	content, group, d := contentGroup(ct)
	if d != 0 {
		return nil, decline(d, typeSubject)
	}
	b := &binding{p: p, attributes: map[xsd.QName]int{}}
	// names is the struct's Namer: the content's fields claim from it in
	// particle order, then the attribute fields (codegen/doc.go "# Particles").
	// A bare sealed interface has no struct and discards it.
	names := goname.NewNamer()
	// ch is the content's choice, nil for a sequence: the one content a bare
	// sealed interface can bind.
	var ch *choice
	var err error
	b.content, ch, err = bindContent(p, group, names, typeSubject)
	if err != nil {
		return nil, err
	}
	for _, u := range ct.AttributeUses() {
		f, err := attributeUse(schema, u)
		if err != nil {
			return nil, err
		}
		f.name = names.Claim(u.DeclarationName().Local)
		b.attributes[u.DeclarationName()] = len(b.attrs)
		b.attrs = append(b.attrs, f)
	}
	if _, has := ct.AttributeWildcard(); has {
		return nil, decline(constructAttributeWildcard, typeSubject)
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
// first decline Decode's doc states for its particles, item 5, typeSubject
// naming the type.
func bindContent(p *pipeline, group xsd.ModelGroup, names *goname.Namer, typeSubject string) (elementContent, *choice, error) {
	if group.Compositor() == xsd.CompositorChoice {
		ch, err := bindChoice(p, group.Particles(), names, typeSubject)
		if err != nil {
			return nil, nil, err
		}
		return ch, ch, nil
	}
	s, err := bindSequence(p, group.Particles(), names, typeSubject)
	if err != nil {
		return nil, nil, err
	}
	return s, nil, nil
}

// sequence is a sequence's particles as the struct codegen emits holds them:
// one field per particle, in particle order.
type sequence struct {
	p      *pipeline
	fields []field
	// children maps a child's expanded name to its field's index; it is a
	// lookup index only and never iterated (STYLE D2).
	children map[xsd.QName]int
}

// bindSequence returns the sequence of particles, a content sequence's, each
// field named from names by its element's local name in particle order, or the
// first decline Decode's doc states for them, item 5, typeSubject naming the
// type.
func bindSequence(pl *pipeline, particles []xsd.Particle, names *goname.Namer, typeSubject string) (*sequence, error) {
	s := &sequence{p: pl, children: map[xsd.QName]int{}}
	for _, p := range particles {
		el, d, err := particleElement(pl.schema, p)
		if err != nil {
			return nil, err
		}
		if d != 0 {
			return nil, decline(d, "a particle of "+typeSubject)
		}
		f, err := elementField(pl.schema, s.children, el, p.Occurs())
		if err != nil {
			return nil, err
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
			return nil, fmt.Errorf("%w: field %d of %s is %s %s where codegen emits %s %s", ErrTypeMismatch, i, t, got.Name, got.Type, want.name, want.describe())
		}
	}
	return s, nil
}

// child sets the field of the child element start, reading its character
// children from r up to its end tag: the value of their characters, or of
// the field's supplied value when there are none, as Decode's doc states.
func (s *sequence) child(r *xmltree.Reader, start *xmltree.StartElement, out reflect.Value) error {
	i, ok := s.children[expanded(start.Name())]
	if !ok {
		return unparticled(start)
	}
	v, err := elementValue(r, start, s.p, s.fields[i].scalar)
	if err != nil {
		return err
	}
	s.fields[i].shape.store(out.Field(i), v)
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
	p    *pipeline
	name string
	alts []field
	// children maps a child's expanded name to its alternative's index in
	// alts; it is a lookup index only and never iterated (STYLE D2).
	children map[xsd.QName]int
}

// bindChoice returns the choice of particles, a content choice's, its field
// named from names by the local name choice before any alternative is read
// (codegen/doc.go "# Particles"), or the first decline Decode's doc states for
// them, item 5, typeSubject naming the type: each alternative's as a
// sequence's particle declines, then an alternative whose branch struct has an
// earlier one's shape, which no branch type could tell apart: one whose branch
// field accepts a Go type the earlier one's does (field.sharesBranch).
func bindChoice(pl *pipeline, particles []xsd.Particle, names *goname.Namer, typeSubject string) (*choice, error) {
	c := &choice{p: pl, name: names.Claim("choice"), children: map[xsd.QName]int{}}
	for _, p := range particles {
		el, d, err := particleElement(pl.schema, p)
		if err != nil {
			return nil, err
		}
		if d != 0 {
			return nil, decline(d, "a particle of "+typeSubject)
		}
		f, err := elementField(pl.schema, c.children, el, p.Occurs())
		if err != nil {
			return nil, err
		}
		// A branch's {min occurs} 0 reads as 1: the empty match is the nil
		// interface, so no branch holds an absent value (cos-group-emptiable).
		if f.shape == shapeOptional {
			f.shape = shapeOne
		}
		f.name = goname.NewNamer().Claim(el.Name().Local)
		for _, earlier := range c.alts {
			if earlier.sharesBranch(f) {
				return nil, decline(constructSharedBranch, declSubject(el))
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
// of start's alternative holds the value of their characters, or of the
// field's supplied value when there are none, as Decode's doc states, a
// repeated alternative's appended to the branch dst already holds.
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
	v, err := elementValue(r, start, bc.choice.p, alt.scalar)
	if err != nil {
		return err
	}
	alt.shape.store(branch.Field(0), v)
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
// states for st, item 5, subject naming st.
func bindSimpleRoot(p *pipeline, decl xsd.ElementDeclaration, st *xsd.SimpleType, subject string) (binder, error) {
	g, err := simpleDecline(p.schema, st, subject)
	if err != nil {
		return nil, err
	}
	vc, has := decl.ValueConstraint()
	return &simpleRoot{p: p, scalar: newScalar(g, st, declSubject(decl), vc, has)}, nil
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

// particleElement returns the element declaration p's term holds, a local
// one or the top-level one an <element ref> names (§3.3.2.4
// ref.elt.global), followed through [xsd.Schema.Element], or the construct
// Decode declines in its place: a model group, inline or through <group ref>,
// is a nested model group particle. An <element ref> naming no top-level
// declaration is a plain error.
func particleElement(schema *xsd.Schema, p xsd.Particle) (xsd.ElementDeclaration, construct, error) {
	switch term := p.Term().(type) {
	case xsd.ElementDeclarationRef:
		decl, ok := schema.Element(term.Name)
		if !ok {
			return xsd.ElementDeclaration{}, 0, fmt.Errorf("codec: the <element ref> particle to %s does not resolve", term.Name)
		}
		return decl, 0, nil
	case xsd.ResolvedTerm:
		switch inner := term.Term.(type) {
		case xsd.ElementDeclaration:
			return inner, 0, nil
		case xsd.Wildcard:
			return xsd.ElementDeclaration{}, constructWildcardParticle, nil
		}
	}
	return xsd.ElementDeclaration{}, constructNestedGroup, nil
}

// elementField returns the field, but for its name, which its caller claims,
// of the element declaration el in a particle occurring as occurs, or as the
// error the *DeclineError Decode's doc states for it, item 5, naming el, or
// the plain error that item states in a decline's place, in the order that
// doc states: for a top-level el, which an <element ref> named, its
// substitution group, then its own properties, its type, and an expanded name
// an earlier particle of its model group shares, children indexing the
// earlier particles' names. A local el's field holds the Go type codegen
// gives its type, and a top-level one's the named type codegen emits for el
// over it (valueNamed).
func elementField(schema *xsd.Schema, children map[xsd.QName]int, el xsd.ElementDeclaration, occurs xsd.Occurs) (field, error) {
	subject := declSubject(el)
	value := valueBare
	if el.ScopeVariety() == xsd.ScopeGlobal {
		if substitutable(schema, el) {
			return field{}, decline(constructSubstitutionRef, subject)
		}
		value = valueNamed
	}
	if d := declarationDecline(el); d != 0 {
		return field{}, decline(d, subject)
	}
	td, ok := schema.ResolvedType(el.TypeDefinition())
	if !ok {
		return field{}, fmt.Errorf("codec: the {type definition} of %s does not resolve", subject)
	}
	st, ok := td.(*xsd.SimpleType)
	if !ok {
		return field{}, decline(constructComplexElement, subject)
	}
	g, err := simpleDecline(schema, st, subject)
	if err != nil {
		return field{}, err
	}
	if _, shared := children[el.Name()]; shared {
		return field{}, decline(constructSharedName, subject)
	}
	s, err := occursShape(occurs, el)
	if err != nil {
		return field{}, err
	}
	vc, has := el.ValueConstraint()
	return field{shape: s, value: value, scalar: newScalar(g, st, subject, vc, has)}, nil
}

// substitutable reports whether an element other than one of head, a
// top-level element declaration, may stand where an <element ref> to head
// does: whether head's ·substitution group· (§3.3.6.4 key-eq,
// [xsd.Schema.SubstitutionGroup], which decides cos-equiv-derived-ok-rec
// clauses 1 to 2.3, a head blocking substitution by clause 2.1 holding itself
// alone) has members with {abstract} false (cvc-elt clause 2 admits no other
// in an instance) that are not head alone. That is codegen's test for a field
// holding the group's sealed interface rather than head's own type, so an
// abstract head, whatever its other properties, answers true.
func substitutable(schema *xsd.Schema, head xsd.ElementDeclaration) bool {
	members := slices.DeleteFunc(schema.SubstitutionGroup(head.Name()), xsd.ElementDeclaration.Abstract)
	return len(members) != 1 || members[0].Name() != head.Name()
}

// attributeUse returns the field, but for its name, which its caller claims,
// of attribute use u, or as the error the *DeclineError Decode's doc states
// for it, item 5, naming its attribute declaration, or a plain error. The
// field's shape is codegen's, as the package doc's "# What decodes" states,
// and needs no xsi: test here: an xsi: use declines before its shape is
// picked.
func attributeUse(schema *xsd.Schema, u xsd.AttributeUse) (field, error) {
	decl, ok := schema.ResolvedAttributeDeclaration(u)
	if !ok {
		return field{}, fmt.Errorf("codec: the {attribute declaration} of attribute use %s does not resolve", u.DeclarationName())
	}
	subject := fmt.Sprintf("attribute %s declared at %s", decl.Name(), decl.Loc())
	if decl.Name().Space == xsd.XMLSchemaInstanceNS {
		return field{}, decline(constructXsiAttribute, subject)
	}
	st, ok := schema.ResolvedSimpleType(decl.TypeDefinition())
	if !ok {
		return field{}, fmt.Errorf("codec: the {type definition} of %s does not resolve", subject)
	}
	g, err := simpleDecline(schema, st, subject)
	if err != nil {
		return field{}, err
	}
	shape := shapeOptional
	vc, has := schema.EffectiveValueConstraint(u)
	if u.Required() || has {
		shape = shapeOne
	}
	return field{shape: shape, value: valueBare, scalar: newScalar(g, st, subject, vc, has)}, nil
}

// simpleDecline returns the Go type codegen gives the simple type st of the
// item subject names (codegen/doc.go "# Simple types"), or else an error:
// the *DeclineError Decode's doc states for st, item 5, subject naming it,
// or a plain error. For an atomic st the decline is atomicDecline's; for a
// list, that of its {item type definition} (Structures §3.16.2.3, a
// restriction of a list taking its base's), which cos-st-restricts clause 2.1
// (checkListGraph) holds to an atomic or union type, so its construct always
// answers, the detail naming the item after subject.
func simpleDecline(r xsd.TypeResolver, st *xsd.SimpleType, subject string) (simpleGo, error) {
	variety, err := st.Variety(r)
	if err != nil {
		return simpleGo{}, err
	}
	if _, isList := variety.(xsd.List); !isList {
		atom, d, err := atomicDecline(r, st)
		if err != nil {
			return simpleGo{}, err
		}
		if d != 0 {
			return simpleGo{}, decline(d, subject)
		}
		return simpleGo{atom: atom}, nil
	}
	item, err := st.Item(r)
	if err != nil {
		return simpleGo{}, err
	}
	if item == nil {
		return simpleGo{}, fmt.Errorf("codec: the list type of %s has no {item type definition}", subject)
	}
	atom, d, err := atomicDecline(r, item)
	if err != nil {
		return simpleGo{}, err
	}
	if d != 0 {
		return simpleGo{}, decline(d, itemSubject(subject, item))
	}
	return simpleGo{atom: atom, item: item}, nil
}

// itemSubject names, after subject, item, the {item type definition} of the
// list type whose item declines, and item's schema Loc.
func itemSubject(subject string, item *xsd.SimpleType) string {
	if item.Name() == (xsd.QName{}) {
		return fmt.Sprintf("%s, through the list type's anonymous {item type definition}, a type defined at %s", subject, item.Loc())
	}
	return fmt.Sprintf("%s, through the list type's {item type definition} %s, a type defined at %s", subject, item.Name(), item.Loc())
}

// atomicDecline returns the atomic Go type codegen gives the simple type st,
// an atomic type whose {primitive type definition} is xs:string, xs:anyURI,
// xs:boolean, xs:float, xs:double or xs:decimal (codegen/doc.go "# Simple
// types"), or else the construct Decode declines st for: a union's, or that of
// its primitive. A list st is a plain error: simpleDecline routes a list to its
// {item type definition}, which cos-st-restricts clause 2.1 holds to no list.
// {variety} is a closed sum, so the switch is STYLE T2's sealed-sum exception.
func atomicDecline(r xsd.TypeResolver, st *xsd.SimpleType) (atomic, construct, error) {
	variety, err := st.Variety(r)
	if err != nil {
		return 0, 0, err
	}
	switch variety.(type) {
	case xsd.Union:
		return 0, constructUnion, nil
	case xsd.List:
		return 0, 0, fmt.Errorf("codec: the simple type defined at %s is a list where an atomic or union type is required, as a list's {item type definition} (cos-st-restricts clause 2.1)", st.Loc())
	}
	primitive, err := st.Primitive(r)
	if err != nil {
		return 0, 0, err
	}
	if primitive == nil {
		// xs:anySimpleType, of absent {variety}, and xs:anyAtomicType.
		return 0, constructSpecial, nil
	}
	switch primitive.Name() {
	case stringPrimitive, anyURIPrimitive:
		return atomicString, 0, nil
	case booleanPrimitive:
		return atomicBool, 0, nil
	case floatPrimitive:
		return atomicFloat32, 0, nil
	case doublePrimitive:
		return atomicFloat64, 0, nil
	case decimalPrimitive:
		atom, err := decimalAtomic(r, st)
		return atom, 0, err
	}
	return 0, constructOtherPrimitive, nil
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
			return nil, fmt.Errorf("%w: field %d of %s is %s %s where codegen emits %s %s", ErrTypeMismatch, width+i, t, got.Name, got.Type, want.name, want.describe())
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

// fill sets out's fields as a filler does, in the order Decode's doc states
// for items 7 and 8: root's xsi:type, its present attributes in start-tag
// order, the supplied values of the absent ones in attribute-use order, then
// its children. An attribute field with a supplied value holds it unless its
// attribute is present, and its supplied value is then never decoded.
func (b *boundBinding) fill(r *xmltree.Reader, root *xmltree.StartElement, out reflect.Value) error {
	if err := checkXsiType(root); err != nil {
		return err
	}
	bd := b.binding
	width := bd.content.width()
	present := make([]bool, len(bd.attrs))
	for _, a := range root.Attributes() {
		i, ok := bd.attributes[expanded(a.Name())]
		if !ok {
			if a.Name().Space() != xsd.XMLSchemaInstanceNS {
				return unattributed(a)
			}
			continue
		}
		v, err := bd.attrs[i].decode(bd.p, a.Value(), attributeSource{a: a})
		if err != nil {
			return err
		}
		bd.attrs[i].shape.store(out.Field(width+i), v)
		present[i] = true
	}
	for i, f := range bd.attrs {
		if present[i] || f.supplied == nil {
			continue
		}
		v, err := f.decode(bd.p, f.supplied.lexical, suppliedSource{start: root, subject: f.supplied.subject})
		if err != nil {
			return err
		}
		f.shape.store(out.Field(width+i), v)
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

// elementValue reads the character children of the element start from r up
// to its end tag and returns s's value of their characters, or of s's
// supplied {lexical form} when it is not nil and there are none, the value
// the element's {value constraint} supplies (cvc-elt clause 5.1), as Decode's
// doc states.
func elementValue(r *xmltree.Reader, start *xmltree.StartElement, p *pipeline, s scalar) (reflect.Value, error) {
	var text bytes.Buffer
	var first *xmltree.CharData
	for {
		tok, err := r.Token()
		if err != nil {
			return reflect.Value{}, err
		}
		switch tok := tok.(type) {
		case *xmltree.CharData:
			if first == nil {
				first = tok
			}
			text.WriteString(tok.Data())
		case *xmltree.StartElement:
			return reflect.Value{}, fmt.Errorf("codec: %s: element %s of simple type holds element %s", tok.Loc(), expanded(start.Name()), expanded(tok.Name()))
		case *xmltree.EndElement:
			if s.supplied != nil && text.Len() == 0 {
				return s.decode(p, s.supplied.lexical, suppliedSource{start: start, subject: s.supplied.subject})
			}
			return s.decode(p, text.String(), contentSource{start: start, first: first})
		}
	}
}

// simpleRoot is the type codegen emits for a document element declaration
// whose {type definition} is a simple type Decode decodes, as Decode reads
// it: a type over the Go type codegen gives that simple type (scalar.goType).
type simpleRoot struct {
	p *pipeline
	scalar
}

// check returns s, which fills t, when t's underlying type is the Go type
// codegen gives s's simple type, and otherwise the ErrTypeMismatch error
// Decode's doc states, item 6. t's name is never compared; equal Kinds and
// ConvertibleTo mean one underlying type, as field.accepts states.
func (s *simpleRoot) check(t reflect.Type, _ branchSets) (filler, error) {
	want := s.goType()
	if t.Kind() != want.Kind() || !t.ConvertibleTo(want) {
		return nil, fmt.Errorf("%w: the underlying type of %s is not %s", ErrTypeMismatch, t, want)
	}
	return s, nil
}

// fill sets out, whose underlying type is the Go type codegen gives s's
// simple type, as a filler does.
func (s *simpleRoot) fill(r *xmltree.Reader, root *xmltree.StartElement, out reflect.Value) error {
	if err := checkXsiType(root); err != nil {
		return err
	}
	if err := noAttributes(root); err != nil {
		return err
	}
	v, err := elementValue(r, root, s.p, s.scalar)
	if err != nil {
		return err
	}
	shapeOne.store(out, v)
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
			return decline(constructXsiType, fmt.Sprintf("element %s with xsi:type %q", expanded(start.Name()), a.Value())).at(start.Loc())
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
