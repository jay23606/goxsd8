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
// generated type, and item 6 below states which T match. A caller decoding
// documents of several roots therefore reads the document element's name
// itself, through parser/xmltree or encoding/xml, before choosing T.
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
// strconv.ParseFloat at the Go type's width, a decimal's into big.Rat,
// big.Int or one of the bounded Go integers int64, int32, int16, int8, uint64,
// uint32, uint16 and uint8 at its width, and a date, time or duration type's
// into its datatype struct, by that datatype's lexical grammar (§3.3.6.2,
// §3.3.7.2–§3.3.14.2, §D.2.2), each property the local value the form shows,
// never converted to UTC (§D.2.1), the timezoneOffset in minutes, and a
// duration's Months and Seconds both of the form's one sign (§3.3.6.1, §E.2).
// A type whose datatype struct is yearMonthDuration's alone is converted from
// the string Parse accepted, so that zero, which has no canonical
// representation (§3.4.26.1), decodes as Months 0; its Canonical is never
// consulted. A float or double literal beyond the type's range is the INF or
// -INF its mapping rounds it to (·floatingPointRound·, Datatypes §E.1), so it
// decodes as ±Inf where the backend's Canonical renders that, as
// builtin/strict's does, and is no item 8 error. backend's governing mapping of
// each such type but those must therefore have a non-nil Canonical, as
// builtin/strict's do; item 8 states the error where one fails. For a list
// type, whose whiteSpace is collapse (Datatypes §4.3.6, §2.4.1.2), the
// normalized string is split at each U+0020 into its list items before any is
// mapped, an empty one into none (§4.1.1 dt-list), and each item is read as
// above under the list's {item type definition}, a restriction of a list taking
// its base's (Structures §3.16.2.3); the value is the []A of the items in
// order. So the value is the instance's own even where a fixed value constraint
// holds it to an equal one that is not identical (§3.5.4 cvc-au, cvc-elt clause
// 5.2.2.2.2): -0 against fixed 0 on xs:double decodes to negative zero. The
// field of an optional attribute use with an effective value constraint is the
// Go type codegen gives its simple type itself, as codegen emits it, holding
// the supplied value when the attribute is absent; a whitespace-only element
// has a character child and holds its own value; and an element absent from the
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
//  5. A construct outside the slice of the document element's declaration or
//     type, or of a declaration or type its particles reach at any depth: a
//     *DeclineError at the document element's instance Loc, whose
//     [DeclineError.Construct] is that construct's, the first in the order
//     the list "Item 5 examines" below states. A component of a declaration
//     or type this item examines that does not resolve, or a particle with
//     {max occurs} 0, is a plain error in place of a decline, at its point in
//     that order.
//  6. T does not match the type codegen emits for the declaration, as Decode
//     derives it from the schema by the comparisons the list "Item 6
//     compares" below states: an error errors.Is reports as
//     [ErrTypeMismatch], no *DeclineError, so that a T that does not match
//     the schema is never filled.
//  7. An element carrying xsi:type, the document element or any descendant: a
//     *DeclineError at that element's instance Loc, the first in document
//     order, whose [DeclineError.Construct] is xsi:type's. A document element
//     no top-level declaration declares, which validation accepts only
//     through its xsi:type (§3.3.4.6 cvc-assess-elt), declines so in place of
//     item 5.
//  8. The lexical mapping or the conversion to the Go type failing on a value
//     validate accepted on the same backend: no mapping of backend governs
//     the type, its Parse rejects the normalized value, or its Canonical is
//     nil, fails, or renders a form the Go type does not read or a finite
//     form beyond the Go type's range, or, for yearMonthDuration's datatype
//     struct, which reads no Canonical, Parse accepts a form that struct does
//     not read; a datatype struct reads no form denoting no value of its
//     datatype, such as a month of 13, an hour of 24 or a dateTimeStamp
//     without a timezone. For a list, the type is its {item type definition}
//     and the value one list item. It is a fault of the backend,
//     not of the instance. The error names the stage, the type — its QName, or
//     for an anonymous type the declaration it is local to and that
//     declaration's schema Loc, and for a list's item type the list's too —
//     the normalized value, or the list item and its 1-based position among
//     the value's items, and the value's source at its instance Loc, for a
//     list the whole value's and never an item's: an element's character
//     content, at its first character data, with that data's byte offset; a
//     present attribute, at its Loc (xmltree.Attribute.Loc), with no offset;
//     or a supplied value, at its element's start, with the declaration whose
//     value constraint supplied it. It wraps nothing: errors.As finds no
//     *xsderr.Error, *DeclineError or *UndecidedError in it, xsderr.RuleOf no
//     rule, and errors.Is neither errors.ErrUnsupported nor [ErrTypeMismatch];
//     and [ViolationsOf] returns nil for it.
//
// Items 7 and 8 are both found while filling T, and the first in document
// order is returned: an element's xsi:type, then its attributes' values,
// present attributes in start-tag order and then the supplied values of
// absent ones in attribute-use order, then its children in document order,
// each child of complex type filled in this same order before the next
// child. A supplied value whose attribute is present is never mapped.
//
// Items 5 and 6 read no instance; [CheckDecode] returns them for a
// declaration before an instance arrives.
//
// Item 5 examines the document element's declaration in this order; the
// fourth and fifth bullets say how the particle step reads a substitution
// group's <element ref> and a choice's alternatives:
//
//   - The declaration's {type table} and {nillable}.
//   - Its {type definition}: a simple type's {variety} and {primitive type
//     definition}, for a list its {item type definition}'s, whose construct
//     the list declines by, after which nothing further is examined; or a
//     complex type's content, then the occurrence of its sequence or choice.
//   - Each particle of the complex type's content in order, the whole of one
//     before the next. A nested model group or wildcard declines first. Then
//     the particle's element declaration's {type table} and {nillable}, and
//     its {type definition}: a simple one is examined as the document
//     element's is; a complex one declines when it is xs:anyType, then when it
//     has {abstract} true, and is otherwise examined as the document element's
//     complex type is, from its content to its {attribute wildcard}. Then the
//     particle declines by an expanded name an earlier particle's field takes,
//     a particle's field taking its element's expanded name, or, for a
//     substitution group's interface, each declaration the next bullet
//     examines.
//   - Of an <element ref> particle whose field holds a substitution group's
//     sealed interface (item 6), in place of the referenced declaration's own
//     properties and type: each declaration of the group with {abstract}
//     false in group order, the head among them only where it has {abstract}
//     false, examined as a particle's element declaration is, each then
//     declining by a branch field of an earlier one's shape as a choice's
//     alternatives do, and then the first of their expanded names in group
//     order an earlier particle's field takes.
//   - Of a choice, each alternative's own constructs, then a branch field of
//     the name an earlier alternative's has where either alternative is of
//     complex type, holds a substitution group's interface or is of a simple
//     type whose Go type is a datatype struct or a list of one, and otherwise
//     of the name and Go type, as item 6 reads it, which declines as two
//     branches of a sealed interface whose structs have one shape.
//   - Each of the complex type's attribute uses in order, and its {attribute
//     wildcard}.
//
// The complex type of a top-level declaration, the document element's, one
// an <element ref> names, or a member of that one's substitution group, and
// a local declaration's named complex {type definition}, are each examined at
// the first reach of that declaration or that name only, and contribute
// nothing when reached again, so a type that reaches itself is examined once.
//
// Item 6 compares T with the type codegen emits by shape alone: neither T's,
// a sealed interface's nor a branch's name is compared. Decode never reads
// the fields from T, so for a type codegen emitted from this schema an item
// 6 error is a defect of Decode's, or a branch set [WithBranches] did not
// supply as codegen emitted it, and for any other T it is a T that does not
// match. Item 6 compares:
//
//   - T of a complex {type definition}: T is a struct whose fields are
//     exactly those codegen emits, in name, order and Go type, each field's
//     Go type as the bullets below read it, or, where codegen emits the
//     choice's sealed interface as the type itself, T is an interface, and
//     Decode then returns a *T pointing at it. So a T of the same shape, such
//     as the struct codegen emits for another top-level element of the same
//     type, passes and decodes as the declaration's own struct does, an error
//     only where that one's is.
//   - T of a simple {type definition}: T's underlying type alone is the Go
//     type codegen gives it (the Simple types section of codegen's package
//     doc): its atomic Go type A, one of string, bool, float32, float64,
//     big.Rat, big.Int, int64, int32, int16, int8, uint64, uint32, uint16,
//     uint8 and a date, time or duration datatype struct D, or for a list
//     []A, A its {item type definition}'s. So any T over that Go type passes,
//     such as the type codegen emits for another document element of the same
//     simple type. Codegen declares D in the caller's package, so D is matched
//     by shape: a struct of exactly D's exported fields, in name, order and Go
//     type, struct tags aside, whatever its name, and for a list a slice,
//     named or not, of such a struct.
//   - The field of a particle or attribute use of simple type: for a local
//     element declaration's particle and an attribute use it is V, *V or []V
//     by its occurrence, an attribute's by its use's {required} and its
//     effective value constraint (§3.5.4 key-evc), V the Go type codegen
//     gives that simple type, A or []A; for an <element ref> particle it is
//     N, *N or []N, N the named type codegen emits for the referenced
//     declaration, matched as any type other than V whose underlying type is
//     V, never by name; V itself, a local particle's type, does not pass, so
//     a ref's field of simple type and a local one's never accept one Go type,
//     except where V is a datatype struct D: V and N are then each a struct of
//     D's shape, as T is, so either field accepts both. For a list of D a
//     local field's V is an unnamed slice of such a struct and a ref's N a
//     defined slice type over one.
//   - The field of a particle whose element declaration, local or
//     referenced, has a complex {type definition}: S, *S or []S by its
//     occurrence, S a struct held to that type as T is to the document
//     element's, its fields exactly codegen's and its name never compared,
//     or, where codegen emits the type's choice's sealed interface as the
//     type itself, I or []I, I that interface, held as T is, the bare
//     interface for an optional one and never a pointer, nil meaning absent.
//   - The field of an <element ref> particle that codegen gives a sealed
//     interface over the referenced declaration's substitution group
//     (§3.3.6.4 key-eq; codegen's package doc, "# Substitution groups"
//     states which): in place of all these, I or []I, I that interface, the
//     bare interface for an optional one and never a pointer, nil meaning
//     absent, one interface value per child for []I; its alternatives are the
//     group's declarations with {abstract} false, in group order, each one's
//     branch field named from its local name and holding what the field of an
//     <element ref> particle occurring once to that declaration would hold
//     were its own substitution group that declaration alone: N, S or I, so a
//     group with no such declaration has no alternative and its field is
//     nil.
//   - A sealed interface, T, a struct's first field or a nested element's, a
//     choice's or a substitution group's, whose branch types reach Decode
//     only through [WithBranches]: it does not match when it is not an
//     interface whose one method is unexported, or when its branch set, as
//     WithBranches supplied it, is not one struct of one field per
//     alternative, that field the name and Go type of the alternative's
//     branch field, a branch whose field is named like an alternative of
//     complex type, or of a substitution group's interface, held to that
//     alternative alone: when no WithBranches supplies it for an interface
//     with alternatives, or its count differs, or it holds a nil value, a
//     type that is no struct, a type twice, or a branch matching no
//     alternative an earlier branch did not match.
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
	f, err := b.check(reflect.TypeFor[T](), newCheckContext(cfg.branches))
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
	_, err = b.check(reflect.TypeFor[T](), newCheckContext(newConfig(opts).branches))
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

// elem returns the Go type V that t holds by s, t itself for shapeOne and the
// element type of a pointer for shapeOptional and of a slice for
// shapeRepeated, and false when t is no type of shape s.
func (s shape) elem(t reflect.Type) (reflect.Type, bool) {
	switch s {
	case shapeOne:
		return t, true
	case shapeOptional:
		if t.Kind() != reflect.Pointer {
			return nil, false
		}
		return t.Elem(), true
	case shapeRepeated:
		if t.Kind() != reflect.Slice {
			return nil, false
		}
		return t.Elem(), true
	}
	// The zero shape is an internal bug, never input.
	panic(fmt.Sprintf("codec: invalid shape %d", s))
}

// over returns the Go type of shape s over v: V, *V or []V.
func (s shape) over(v reflect.Type) reflect.Type {
	switch s {
	case shapeOne:
		return v
	case shapeOptional:
		return reflect.PointerTo(v)
	case shapeRepeated:
		return reflect.SliceOf(v)
	}
	// The zero shape is an internal bug, never input.
	panic(fmt.Sprintf("codec: invalid shape %d", s))
}

// prefix spells what s puts before V in an ErrTypeMismatch message: nothing,
// "*" or "[]".
func (s shape) prefix() string {
	switch s {
	case shapeOne:
	case shapeOptional:
		return "*"
	case shapeRepeated:
		return "[]"
	}
	return ""
}

// store stores v, a decoded value of a type that converts to the V a field of
// shape s holds, in dst, a value of a Go type of shape s: a repeated one
// appends it, an optional one points at a copy of it, each converted to dst's
// own element type, and shapeOne sets dst to it converted to dst's type, each
// conversion convertTo's.
func (s shape) store(dst, v reflect.Value) {
	switch s {
	case shapeOptional:
		p := reflect.New(dst.Type().Elem())
		p.Elem().Set(convertTo(v, dst.Type().Elem()))
		dst.Set(p)
	case shapeRepeated:
		dst.Set(reflect.Append(dst, convertTo(v, dst.Type().Elem())))
	default:
		dst.Set(convertTo(v, dst.Type()))
	}
}

// convertTo returns v converted to t: a slice whose element type is not t's,
// a list of codec's mirror of a datatype struct, which converts to no slice
// of the caller's struct, element by element into a new slice of t of v's
// length, and any other value by Convert.
func convertTo(v reflect.Value, t reflect.Type) reflect.Value {
	if v.Kind() != reflect.Slice || v.Type().Elem() == t.Elem() {
		return v.Convert(t)
	}
	out := reflect.MakeSlice(t, v.Len(), v.Len())
	for i := range v.Len() {
		out.Index(i).Set(v.Index(i).Convert(t.Elem()))
	}
	return out
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

// field is one field a particle gives a struct codegen emits: a complex
// type's struct's, or a choice's branch struct's. It holds its Go name, its
// shape, and arm, the V its shape holds.
type field struct {
	name  string
	shape shape
	arm   fieldArm
}

// fieldArm is the V a field's shape holds, as Decode reads it, a closed sum:
// a scalarArm, the Go type codegen gives a simple type or a named type over
// it; a structArm, a nested complex type's struct; an ifaceArm, a nested
// complex type's bare sealed interface; or a groupArm, the sealed interface
// over a substitution group.
type fieldArm interface {
	// bind holds v, the V the field's shape holds at at, to the arm, item 6 of
	// Decode's doc, and returns the elementReader that reads the field's
	// element into a V, or at's mismatch.
	bind(v reflect.Type, at fieldAt, cx *checkContext) (elementReader, error)
	// describe spells V for an ErrTypeMismatch message.
	describe() string
}

// scalarArm is a field's V where the field's element or attribute is of a
// simple type: value, how V relates to the Go type codegen gives that type,
// and the scalar the value decodes as.
type scalarArm struct {
	value valueType
	scalar
}

// accepts reports whether v is the V codegen gives a, as Decode's doc, item 6,
// reads it: the Go type codegen gives a's simple type (scalar.goType) itself
// for valueBare, and for valueNamed a type other than it over it
// (scalar.underlies), its name never compared. A datatype struct D is
// declared in the caller's package, so codec holds no D to compare v with:
// for valueBare and valueNamed alike v is a struct over D, and for a list
// []D, an unnamed slice of such a struct, for valueBare, and a defined slice
// type over one, N for type N []D, for valueNamed.
func (a scalarArm) accepts(v reflect.Type) bool {
	if a.atom.mirrored() {
		if !a.underlies(v) {
			return false
		}
		if a.item == nil {
			return true
		}
		return (v.Name() == "") == (a.value == valueBare)
	}
	want := a.goType()
	switch a.value {
	case valueBare:
		return v == want
	case valueNamed:
		return v != want && a.underlies(v)
	}
	// The zero valueType is an internal bug, never input.
	panic(fmt.Sprintf("codec: invalid value type %d", a.value))
}

// bind returns a, which reads the field's element, when a accepts v, and
// otherwise at's mismatch.
func (a scalarArm) bind(v reflect.Type, at fieldAt, _ *checkContext) (elementReader, error) {
	if !a.accepts(v) {
		return nil, at.mismatch()
	}
	return a, nil
}

// read reads the character children of the element start from r up to its
// end tag and returns their value, or the supplied one's, as elementValue
// does.
func (a scalarArm) read(p *pipeline, r *xmltree.Reader, start *xmltree.StartElement) (reflect.Value, error) {
	return elementValue(r, start, p, a.scalar)
}

// describe spells V: the Go type codegen gives a's simple type, A or for a
// list []A, as scalar.spell spells it, or N for a named type N over it, which
// a datatype struct D, of no list, is not told apart from (accepts).
func (a scalarArm) describe() string {
	if a.value == valueNamed && (!a.atom.mirrored() || a.item != nil) {
		return "N for a named type N over " + a.spell()
	}
	return a.spell()
}

// structArm is a field's V where the field's element is of a complex type
// codegen gives a struct, unit's: S, matched by its fields as T is, its name
// never compared, whether the particle is local or an <element ref>.
type structArm struct{ unit *binding }

// bind holds v to a's struct, and returns the reader that fills a V from the
// field's element.
func (a structArm) bind(v reflect.Type, at fieldAt, cx *checkContext) (elementReader, error) {
	if v.Kind() != reflect.Struct {
		return nil, at.mismatch()
	}
	f, err := a.unit.check(v, cx)
	if err != nil {
		return nil, err
	}
	return nestedReader{t: v, fill: f}, nil
}

// describe spells V.
func (structArm) describe() string { return "S for a struct S" }

// ifaceArm is a field's V where the field's element is of a complex type
// codegen gives a bare sealed interface, unit's: I, held to unit's choice as
// T is, its name never compared. Its field's shape is never shapeOptional
// (choiceRoot.field).
type ifaceArm struct{ unit *choiceRoot }

// bind holds v to a's interface, and returns the reader that fills a V from
// the field's element.
func (a ifaceArm) bind(v reflect.Type, at fieldAt, cx *checkContext) (elementReader, error) {
	if v.Kind() != reflect.Interface {
		return nil, at.mismatch()
	}
	f, err := a.unit.checkAt(v, at.site(), cx)
	if err != nil {
		return nil, err
	}
	return nestedReader{t: v, fill: f}, nil
}

// describe spells V.
func (ifaceArm) describe() string { return "I for a sealed interface I" }

// groupArm is a field's V where the field is an <element ref> particle's to a
// head whose substitution group codegen gives a sealed interface (groupField):
// I, held to sealed's branches as a choice's interface is, its name never
// compared. Its field's shape is never shapeOptional.
type groupArm struct{ sealed *sealed }

// bind holds v to a's interface, and returns the reader that reads one
// member's element into a fresh V.
func (a groupArm) bind(v reflect.Type, at fieldAt, cx *checkContext) (elementReader, error) {
	if v.Kind() != reflect.Interface {
		return nil, at.mismatch()
	}
	bc, err := a.sealed.checkSite(v, at.site(), cx)
	if err != nil {
		return nil, err
	}
	return groupReader{t: v, bound: bc}, nil
}

// describe spells V.
func (groupArm) describe() string { return "I for a substitution group's sealed interface I" }

// groupReader reads a member's element into a new value of t, the interface
// a groupArm bound, through bound, so a repeated field holds one interface
// value per child.
type groupReader struct {
	t     reflect.Type
	bound *boundChoice
}

// read returns a new value of g.t holding the branch of start's member.
func (g groupReader) read(_ *pipeline, r *xmltree.Reader, start *xmltree.StartElement) (reflect.Value, error) {
	v := reflect.New(g.t).Elem()
	if err := g.bound.set(r, start, v); err != nil {
		return reflect.Value{}, err
	}
	return v, nil
}

// nestedReader reads an element of complex type into a new value of t, the
// V a structArm or ifaceArm bound, through fill.
type nestedReader struct {
	t    reflect.Type
	fill filler
}

// read returns a new value of n.t that fill sets from the element start.
func (n nestedReader) read(_ *pipeline, r *xmltree.Reader, start *xmltree.StartElement) (reflect.Value, error) {
	v := reflect.New(n.t).Elem()
	if err := n.fill.fill(r, start, v); err != nil {
		return reflect.Value{}, err
	}
	return v, nil
}

// An elementReader reads the element start, whose start tag r has just
// returned, from r up to its end tag, and returns its value, of the V a field
// holds, under p.
type elementReader interface {
	read(p *pipeline, r *xmltree.Reader, start *xmltree.StartElement) (reflect.Value, error)
}

// fieldAt is field index of the struct owner, the site item 6 holds want to.
type fieldAt struct {
	owner reflect.Type
	index int
	want  field
}

// mismatch is the ErrTypeMismatch error for the field at a, which is not
// a.want as codegen emits it.
func (a fieldAt) mismatch() error {
	got := a.owner.Field(a.index)
	return fmt.Errorf("%w: field %d of %s is %s %s where codegen emits %s %s", ErrTypeMismatch, a.index, a.owner, got.Name, got.Type, a.want.name, a.want.describe())
}

// site names the field at a by its Go name and owner, as a sealed
// interface's ErrTypeMismatch message names its site.
func (a fieldAt) site() string {
	return fmt.Sprintf("field %s of %s", a.owner.Field(a.index).Name, a.owner)
}

// check holds field index of owner to f, item 6 of Decode's doc: the same
// name, not embedded, a Go type of f's shape, and a V f's arm binds. It
// returns the elementReader that reads f's element.
func (f field) check(owner reflect.Type, index int, cx *checkContext) (elementReader, error) {
	at := fieldAt{owner: owner, index: index, want: f}
	got := owner.Field(index)
	v, shaped := f.shape.elem(got.Type)
	if !shaped || got.Name != f.name || got.Anonymous {
		return nil, at.mismatch()
	}
	return f.arm.bind(v, at, cx)
}

// matches reports whether got, a branch struct's one field, is f as codegen
// emits it where f's arm is a scalarArm, as check reads it.
func (f field) matches(got reflect.StructField) bool {
	a, scalar := f.arm.(scalarArm)
	if !scalar || got.Name != f.name || got.Anonymous {
		return false
	}
	v, shaped := f.shape.elem(got.Type)
	return shaped && a.accepts(v)
}

// sharesBranch reports whether f and g, two branch fields of one sealed
// interface, a choice's alternatives' or a substitution group's members', may
// accept a common Go type, as check reads one: the same name, and then either
// arm other than a scalarArm, or either scalarArm's of a datatype struct,
// which is conservative, since such an alternative's branch is selected by its
// field's name alone (sealed.alternative), and a datatype struct's V is no Go
// type codec holds (scalarArm.accepts); or the same value type and, for
// valueBare, the same Go type of shape over scalar.goType, a repeated xs:int
// and a list of xs:int each []int32; for valueNamed, the same shape and the
// same goType separately, since the named type sits between them and []N over
// int32 is no N over []int32.
func (f field) sharesBranch(g field) bool {
	fa, fScalar := f.arm.(scalarArm)
	ga, gScalar := g.arm.(scalarArm)
	if f.name != g.name {
		return false
	}
	if !fScalar || !gScalar || fa.atom.mirrored() || ga.atom.mirrored() {
		return true
	}
	if fa.value != ga.value {
		return false
	}
	if fa.value == valueNamed {
		return f.shape == g.shape && fa.goType() == ga.goType()
	}
	return f.shape.over(fa.goType()) == g.shape.over(ga.goType())
}

// describe spells the Go type codegen gives f for an ErrTypeMismatch message:
// V, *V or []V, V as f's arm spells it.
func (f field) describe() string {
	return f.shape.prefix() + f.arm.describe()
}

// attribute is the field an attribute use gives a complex type's struct: its
// Go name, its shape, and the scalar its value decodes as; its V is the Go
// type codegen gives the attribute's simple type itself.
type attribute struct {
	name  string
	shape shape
	scalar
}

// field returns a as item 6 of Decode's doc reads a field.
func (a attribute) field() field {
	return field{name: a.name, shape: a.shape, arm: scalarArm{value: valueBare, scalar: a.scalar}}
}

// A binder is the type codegen emits for one document element declaration,
// as Decode reads it: check holds T to it, item 6 of Decode's doc, reading
// each sealed interface's branch types from cx, and returns the filler that
// sets a value of T from the document element. A *binding binds a complex
// {type definition} codegen gives a struct, a *choiceRoot one it gives a bare
// sealed interface, and a *simpleRoot a simple one.
type binder interface {
	check(t reflect.Type, cx *checkContext) (filler, error)
}

// A complexUnit is the type codegen emits for a complex {type definition}, a
// document element's or a particle's element declaration's, as Decode reads
// it, a closed sum: a *binding, a struct, or a *choiceRoot, a bare sealed
// interface. It binds T as a binder does where it is the document element's.
type complexUnit interface {
	binder
	// field returns the field, but for its name, of a particle of an element
	// of the unit's type occurring in shape s: its V the unit's type, codegen's
	// shape for it.
	field(s shape) field
}

// checkContext is what one item-6 check of T reads beside the binder: sets,
// the branch types [WithBranches] supplied, and bound, the filler already
// bound for each complex unit and Go type the check has reached.
type checkContext struct {
	sets branchSets
	// bound ties the check's knot (STYLE D4): a unit is held to a Go type once,
	// its entry stored before its fields are checked, and a later reach of the
	// same pair, through a recursive type, links to that entry. It lives for
	// one check only, a lookup index never iterated (STYLE D2); an entry left
	// by a check that fails is never read, since the failure ends the check.
	bound map[boundKey]filler
}

// newCheckContext returns the checkContext of a check under sets.
func newCheckContext(sets branchSets) *checkContext {
	return &checkContext{sets: sets, bound: map[boundKey]filler{}}
}

// boundKey is a complex unit and the Go type item 6 holds it to.
type boundKey struct {
	unit complexUnit
	t    reflect.Type
}

// A filler sets out, a value of the T a binder's check held, from the
// element root, whose start tag r has just returned, and from r's tokens up
// to its end tag, or returns the xsi:type decline Decode's doc states, item 7.
type filler interface {
	fill(r *xmltree.Reader, root *xmltree.StartElement, out reflect.Value) error
}

// binding is the struct codegen emits for a complex type, a document
// element's or a particle's element declaration's, as Decode reads it: its
// fields in codegen's order, its content's first, then each attribute use's.
type binding struct {
	p       *pipeline
	content elementContent
	attrs   []attribute
	// attributes maps an attribute's expanded name to its field's index in
	// attrs; it is a lookup index only and never iterated (STYLE D2).
	attributes map[xsd.QName]int
}

// field returns the field of shape s whose V is b's struct.
func (b *binding) field(s shape) field { return field{shape: s, arm: structArm{unit: b}} }

// elementContent is the element content of the struct a *binding binds, the
// fields that precede its attribute fields, a closed sum: a *sequence, the
// fields of a sequence's particles, or a *choice, the one field holding a
// choice's sealed interface.
type elementContent interface {
	// width is how many of the struct's fields the content's are.
	width() int
	// check holds t's first width fields to the content's, item 6 of Decode's
	// doc, and returns the contentFiller that sets them.
	check(t reflect.Type, cx *checkContext) (contentFiller, error)
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
	if d := declarationDecline(decl); d != 0 {
		return nil, decline(d, declSubject(decl))
	}
	td, ok := p.schema.ResolvedType(decl.TypeDefinition())
	if !ok {
		return nil, fmt.Errorf("codec: the {type definition} of %s does not resolve", declSubject(decl))
	}
	// TypeDefinition is a closed sum of the two, so the assertions are STYLE
	// T2's sealed-sum exception.
	if st, ok := td.(*xsd.SimpleType); ok {
		return bindSimpleRoot(p, decl, st, typeSubject(decl, td))
	}
	ct, _ := td.(xsd.ComplexType)
	run := &bindRun{p: p, elements: unitIndex{}, types: unitIndex{}}
	return run.complex(ct, typeSubject(decl, td), run.elements, decl.Name())
}

// typeSubject names td, the {type definition} of decl, as a decline of one
// of td's constructs labels it: by decl and its schema Loc, then td's.
func typeSubject(decl xsd.ElementDeclaration, td xsd.TypeDefinition) string {
	return fmt.Sprintf("the {type definition} of %s, a type defined at %s", declSubject(decl), td.Loc())
}

// bindRun is one item-5 bind of a document element declaration: p, and the
// complex units it has reached by name. Codec descends into two kinds of
// term, an inline local element declaration and an <element ref>, the
// latter to the referenced declaration or to each member of its
// substitution group, and a local declaration's type is either inline, a tree
// it owns, or named, so every recursion through types passes one of the two
// names that key codegen's emitted types: a top-level element declaration,
// the document element's or one an <element ref> reaches, keyed by its
// expanded name in elements, and the named {type definition} of a local
// element declaration, keyed by its QName in types. An inline anonymous type
// is bound in place, under no key.
type bindRun struct {
	p        *pipeline
	elements unitIndex
	types    unitIndex
}

// unitIndex maps a name to the complex unit bound for it. Each unit's entry
// is stored when the unit is allocated, before its content is bound, so a
// later reach of the name, a recursive one included, links to the unit and
// examines nothing again: this is STYLE D4's named placeholder, no visited
// set. It lives for one bindRun only and is a lookup index never iterated
// (STYLE D2). No entry is removed when a bind fails: any item-5 error ends
// the whole bind.
type unitIndex map[xsd.QName]complexUnit

// complex returns the complex unit of ct, labelled by typeSubject, the one
// index already holds for key where it holds one, or else the first decline
// Decode's doc states for ct, item 5, in its order: ct's content, the
// occurrence of its sequence or choice, each particle in order, each
// attribute use in order, and its {attribute wildcard}. A nil index binds an
// inline anonymous ct under no key.
func (run *bindRun) complex(ct xsd.ComplexType, typeSubject string, index unitIndex, key xsd.QName) (complexUnit, error) {
	if u, ok := index[key]; ok {
		return u, nil
	}
	content, group, d := contentGroup(ct)
	if d != 0 {
		return nil, decline(d, typeSubject)
	}
	// names is the struct's Namer: the content's fields claim from it in
	// particle order, then the attribute fields (codegen/doc.go "# Particles").
	// A bare sealed interface has no struct and discards it.
	names := goname.NewNamer()
	// codegen/emit.go's shape: a choice occurring once, in a type with no
	// attribute field, is the type itself unless it has {particles} and one
	// occurrence of it is emptiable (cos-group-emptiable). Every attribute use
	// gives a field or declines, and so does every particle an alternative, so
	// the counts decide before either is bound.
	if group.Compositor() == xsd.CompositorChoice && len(ct.AttributeUses()) == 0 &&
		(len(group.Particles()) == 0 || !run.p.schema.ParticleEmptiable(content)) {
		c := &choiceRoot{}
		index.store(key, c)
		ch, err := bindChoice(run, group.Particles(), names, typeSubject)
		if err != nil {
			return nil, err
		}
		if _, has := ct.AttributeWildcard(); has {
			return nil, decline(constructAttributeWildcard, typeSubject)
		}
		c.sealed = ch.sealed
		return c, nil
	}
	b := &binding{p: run.p, attributes: map[xsd.QName]int{}}
	index.store(key, b)
	var err error
	b.content, err = bindContent(run, group, names, typeSubject)
	if err != nil {
		return nil, err
	}
	for _, u := range ct.AttributeUses() {
		a, err := attributeUse(run.p.schema, u)
		if err != nil {
			return nil, err
		}
		a.name = names.Claim(u.DeclarationName().Local)
		b.attributes[u.DeclarationName()] = len(b.attrs)
		b.attrs = append(b.attrs, a)
	}
	if _, has := ct.AttributeWildcard(); has {
		return nil, decline(constructAttributeWildcard, typeSubject)
	}
	return b, nil
}

// store stores u as key's unit, unless index is nil, an inline type's.
func (index unitIndex) store(key xsd.QName, u complexUnit) {
	if index != nil {
		index[key] = u
	}
}

// nested returns the complex unit of ct, the {type definition} of el, a
// particle's element declaration, or the first decline Decode's doc states
// for it, item 5: xs:anyType, then {abstract} true, then the constructs of ct
// in the order complex states, keyed by el's expanded name for a top-level
// el, by ct's QName for a local el whose {type definition} names it, and
// under no key for an inline one.
func (run *bindRun) nested(el xsd.ElementDeclaration, ct xsd.ComplexType) (complexUnit, error) {
	subject := typeSubject(el, ct)
	// xs:anyType is known by name, as codegen's isAnyType knows it: its
	// AnyType holds raw content, no struct the slice fills.
	if ct.Name() == (xsd.QName{Space: xsd.XMLSchemaNS, Local: "anyType"}) {
		return nil, decline(constructAnyTypeElement, subject)
	}
	// Codegen emits no struct for an abstract type: an element of one holds
	// its governing types' interface, or the struct of its one member type
	// (codegen/doc.go "# Element declarations").
	if ct.Abstract() {
		return nil, decline(constructAbstractTypeElement, subject)
	}
	if el.ScopeVariety() == xsd.ScopeGlobal {
		return run.complex(ct, subject, run.elements, el.Name())
	}
	// TypeDefinition is a closed sum, so the assertion is STYLE T2's sealed-sum
	// exception; a local declaration's is never a SubstitutionGroupHeadTypeRef.
	if ref, named := el.TypeDefinition().(xsd.TypeDefinitionRef); named {
		return run.complex(ct, subject, run.types, ref.Name)
	}
	return run.complex(ct, subject, nil, xsd.QName{})
}

// bindContent returns the elementContent of group, a content sequence or
// choice, its fields named from names, or the first decline Decode's doc
// states for its particles, item 5, typeSubject naming the type.
func bindContent(run *bindRun, group xsd.ModelGroup, names *goname.Namer, typeSubject string) (elementContent, error) {
	if group.Compositor() == xsd.CompositorChoice {
		return bindChoice(run, group.Particles(), names, typeSubject)
	}
	return bindSequence(run, group.Particles(), names, typeSubject)
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
func bindSequence(run *bindRun, particles []xsd.Particle, names *goname.Namer, typeSubject string) (*sequence, error) {
	s := &sequence{p: run.p, children: map[xsd.QName]int{}}
	for _, p := range particles {
		el, d, err := particleElement(run.p.schema, p)
		if err != nil {
			return nil, err
		}
		if d != 0 {
			return nil, decline(d, "a particle of "+typeSubject)
		}
		f, takes, err := elementField(run, s.children, el, p.Occurs())
		if err != nil {
			return nil, err
		}
		f.name = names.Claim(el.Name().Local)
		for _, n := range takes {
			s.children[n] = len(s.fields)
		}
		s.fields = append(s.fields, f)
	}
	return s, nil
}

// width is the particle count.
func (s *sequence) width() int { return len(s.fields) }

// check holds t's first fields to s's, in name, order and Go type, and
// returns the boundSequence that sets them.
func (s *sequence) check(t reflect.Type, cx *checkContext) (contentFiller, error) {
	readers := make([]elementReader, len(s.fields))
	for i, want := range s.fields {
		r, err := want.check(t, i, cx)
		if err != nil {
			return nil, err
		}
		readers[i] = r
	}
	return &boundSequence{sequence: s, readers: readers}, nil
}

// boundSequence is a sequence whose fields check held: readers[i] reads the
// element of sequence.fields[i].
type boundSequence struct {
	sequence *sequence
	readers  []elementReader
}

// child sets the field of the child element start from its value, which its
// field's elementReader reads from r up to its end tag.
func (b *boundSequence) child(r *xmltree.Reader, start *xmltree.StartElement, out reflect.Value) error {
	s := b.sequence
	i, ok := s.children[expanded(start.Name())]
	if !ok {
		return unparticled(start)
	}
	v, err := b.readers[i].read(s.p, r, start)
	if err != nil {
		return err
	}
	s.fields[i].shape.store(out.Field(i), v)
	return nil
}

// sealed is a sealed interface as codegen emits it, whose one method is
// unexported, and one branch struct per alternative, in order, each holding
// the one field alts holds for it, as Decode reads them.
type sealed struct {
	p    *pipeline
	alts []field
	// children maps a child's expanded name to its alternative's index in
	// alts; it is a lookup index only and never iterated (STYLE D2).
	children map[xsd.QName]int
}

// choice is a choice occurring once as codegen emits it: a sealed interface
// with one branch struct per alternative, in particle order, each holding the
// one field its alternative's particle gives, its {min occurs} 0 read as 1
// and its {max occurs} kept (codegen/doc.go "# Complex types"). The interface
// is the type itself (*choiceRoot) or the first field of the type's struct
// (*binding), named name.
type choice struct {
	name   string
	sealed *sealed
}

// bindChoice returns the choice of particles, a content choice's, its field
// named from names by the local name choice before any alternative is read
// (codegen/doc.go "# Particles"), or the first decline Decode's doc states for
// them, item 5, typeSubject naming the type: each alternative's as a
// sequence's particle declines, then an alternative whose branch struct has an
// earlier one's shape, which no branch type could tell apart: one whose branch
// field accepts a Go type the earlier one's does (field.sharesBranch).
func bindChoice(run *bindRun, particles []xsd.Particle, names *goname.Namer, typeSubject string) (*choice, error) {
	c := &choice{name: names.Claim("choice"), sealed: &sealed{p: run.p, children: map[xsd.QName]int{}}}
	alts := c.sealed
	for _, p := range particles {
		el, d, err := particleElement(run.p.schema, p)
		if err != nil {
			return nil, err
		}
		if d != 0 {
			return nil, decline(d, "a particle of "+typeSubject)
		}
		f, takes, err := elementField(run, alts.children, el, p.Occurs())
		if err != nil {
			return nil, err
		}
		// A branch's {min occurs} 0 reads as 1: the empty match is the nil
		// interface, so no branch holds an absent value (cos-group-emptiable).
		if f.shape == shapeOptional {
			f.shape = shapeOne
		}
		f.name = goname.NewNamer().Claim(el.Name().Local)
		for _, earlier := range alts.alts {
			if earlier.sharesBranch(f) {
				return nil, decline(constructSharedBranch, declSubject(el))
			}
		}
		for _, n := range takes {
			alts.children[n] = len(alts.alts)
		}
		alts.alts = append(alts.alts, f)
	}
	return c, nil
}

// width is the one field holding the interface.
func (c *choice) width() int { return 1 }

// check holds t's first field to the one holding c's interface, and returns
// the boundContentChoice that sets it.
func (c *choice) check(t reflect.Type, cx *checkContext) (contentFiller, error) {
	got := t.Field(0)
	if got.Name != c.name || got.Anonymous {
		return nil, fmt.Errorf("%w: field 0 of %s is %s %s where codegen emits %s, a sealed interface", ErrTypeMismatch, t, got.Name, got.Type, c.name)
	}
	bc, err := c.sealed.checkSite(got.Type, fieldAt{owner: t, index: 0}.site(), cx)
	if err != nil {
		return nil, err
	}
	return boundContentChoice{bound: bc}, nil
}

// boundContentChoice is a content choice whose check held, the contentFiller
// of the struct's first field.
type boundContentChoice struct{ bound *boundChoice }

// child sets the choice's field of out, the struct, from the child element
// start.
func (b boundContentChoice) child(r *xmltree.Reader, start *xmltree.StartElement, out reflect.Value) error {
	return b.bound.set(r, start, out.Field(0))
}

// checkSite holds it, the Go type at the site site names, to c's sealed
// interface, item 6 of Decode's doc: an interface whose one method is
// unexported, whose branch types cx's sets hold as one struct per alternative
// of the alternative's branch shape. It returns c bound to those branch
// types. Neither the interface's nor a branch's name is compared.
func (c *sealed) checkSite(it reflect.Type, site string, cx *checkContext) (*boundChoice, error) {
	if it.Kind() != reflect.Interface {
		return nil, fmt.Errorf("%w: %s is %s, no interface, where codegen emits a sealed interface", ErrTypeMismatch, site, it)
	}
	if it.NumMethod() != 1 || it.Method(0).IsExported() {
		return nil, fmt.Errorf("%w: %s is %s, which is not an interface of one unexported method, codegen's sealed interface", ErrTypeMismatch, site, it)
	}
	branches, supplied := cx.sets.of(it)
	if !supplied && len(c.alts) > 0 {
		return nil, fmt.Errorf("%w: no WithBranches supplies the branches of %s, the interface of %s, where codegen emits %d", ErrTypeMismatch, it, site, len(c.alts))
	}
	if len(branches) != len(c.alts) {
		return nil, fmt.Errorf("%w: WithBranches supplies %d branches of %s where codegen emits %d", ErrTypeMismatch, len(branches), it, len(c.alts))
	}
	// As many branches as alternatives, each matching one no earlier branch
	// matched, leave no entry of matched without its branch type.
	matched := make([]boundBranch, len(c.alts))
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
		j, read, err := c.alternative(bt, cx)
		if err != nil {
			return nil, err
		}
		if j < 0 || matched[j].t != nil {
			return nil, fmt.Errorf("%w: branch %s of %s has the shape of no branch struct codegen emits that an earlier branch did not match", ErrTypeMismatch, bt, it)
		}
		matched[j] = boundBranch{t: bt, read: read}
	}
	return &boundChoice{sealed: c, branches: matched}, nil
}

// alternative returns the index in c.alts of the alternative whose branch
// struct bt is, with the elementReader of its one field, or -1 for none, or
// that alternative's ErrTypeMismatch error: the alternative whose arm is no
// scalarArm whose field bt's one field is named like, which sharesBranch
// keeps alone of its name, its check's error the check's; or else the first
// alternative whose branch field bt's one field matches. So no nested check
// is tried and discarded.
func (c *sealed) alternative(bt reflect.Type, cx *checkContext) (int, elementReader, error) {
	if bt.NumField() != 1 {
		return -1, nil, nil
	}
	got := bt.Field(0)
	j := slices.IndexFunc(c.alts, func(a field) bool {
		_, scalar := a.arm.(scalarArm)
		return !scalar && a.name == got.Name
	})
	if j < 0 {
		j = slices.IndexFunc(c.alts, func(a field) bool { return a.matches(got) })
	}
	if j < 0 {
		return -1, nil, nil
	}
	read, err := c.alts[j].check(bt, 0, cx)
	return j, read, err
}

// boundChoice is a sealed interface checkSite held: branches[i] is the
// branch of sealed.alts[i], none without its type.
type boundChoice struct {
	sealed   *sealed
	branches []boundBranch
}

// boundBranch is one branch struct t of a choice's interface, read its one
// field's elementReader.
type boundBranch struct {
	t    reflect.Type
	read elementReader
}

// set sets dst, a value of the sealed interface, from the child element
// start, read from r up to its end tag: the branch of start's alternative
// holds the value its field's elementReader reads, a repeated alternative's
// appended to the branch dst already holds.
func (bc *boundChoice) set(r *xmltree.Reader, start *xmltree.StartElement, dst reflect.Value) error {
	i, ok := bc.sealed.children[expanded(start.Name())]
	if !ok {
		return unparticled(start)
	}
	alt, bb := bc.sealed.alts[i], bc.branches[i]
	branch := reflect.New(bb.t).Elem()
	if !dst.IsNil() {
		// Validation charges a child of a second alternative first
		// (cvc-model-group); this is the defensive check.
		if dst.Elem().Type() != bb.t {
			return fmt.Errorf("codec: %s: element %s matches a second alternative of the choice", start.Loc(), expanded(start.Name()))
		}
		branch.Set(dst.Elem())
	}
	v, err := bb.read.read(bc.sealed.p, r, start)
	if err != nil {
		return err
	}
	alt.shape.store(branch.Field(0), v)
	dst.Set(branch)
	return nil
}

// choiceRoot is the bare sealed interface codegen emits for a complex type,
// a document element's or a particle's element declaration's, whose content
// is a choice and which has no attribute field, as Decode reads it.
type choiceRoot struct {
	sealed *sealed
}

// field returns the field of shape s whose V is c's interface: an optional
// one is the bare interface, nil meaning absent, never a pointer to it
// (codegen/doc.go "# Particles").
func (c *choiceRoot) field(s shape) field {
	if s == shapeOptional {
		s = shapeOne
	}
	return field{shape: s, arm: ifaceArm{unit: c}}
}

// check holds t, T itself, to the choice's sealed interface, and returns the
// boundChoiceRoot that fills it.
func (c *choiceRoot) check(t reflect.Type, cx *checkContext) (filler, error) {
	return c.checkAt(t, "T", cx)
}

// checkAt holds t, the Go type at the site site names, to the choice's sealed
// interface, and returns the boundChoiceRoot that fills it, the one cx
// already bound for c and t where it holds one.
func (c *choiceRoot) checkAt(t reflect.Type, site string, cx *checkContext) (filler, error) {
	key := boundKey{unit: c, t: t}
	if f, ok := cx.bound[key]; ok {
		return f, nil
	}
	bc := &boundChoiceRoot{}
	cx.bound[key] = bc
	choice, err := c.sealed.checkSite(t, site, cx)
	if err != nil {
		return nil, err
	}
	bc.choice = choice
	return bc, nil
}

// boundChoiceRoot is a choiceRoot whose check held.
type boundChoiceRoot struct {
	choice *boundChoice
}

// fill sets out, a value of the interface, as a filler does. An empty match
// leaves out nil.
func (c *boundChoiceRoot) fill(r *xmltree.Reader, root *xmltree.StartElement, out reflect.Value) error {
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
// of the element declaration el in a particle occurring as occurs, and the
// expanded names of the instance children that field takes, or as the error
// the *DeclineError Decode's doc states for it, item 5, or the plain error
// that item states in a decline's place, in the order that doc states. For a
// top-level el, which an <element ref> named, whose substitution group
// codegen gives a sealed interface (substitutionMembers), it is groupField's.
// Otherwise it is declField's for el, the field taking el's name alone, and
// then an expanded name an earlier particle of its model group shares,
// children indexing the earlier particles' names, declines; a local el of
// simple type's field holds the Go type codegen gives its type, and a
// top-level one's the named type codegen emits for el over it (valueNamed).
func elementField(run *bindRun, children map[xsd.QName]int, el xsd.ElementDeclaration, occurs xsd.Occurs) (field, []xsd.QName, error) {
	if members, sum := substitutionMembers(run.p.schema, el); sum {
		return groupField(run, children, el, members, occurs)
	}
	value := valueBare
	if el.ScopeVariety() == xsd.ScopeGlobal {
		value = valueNamed
	}
	f, err := declField(run, el, value, func() (shape, error) {
		return particleShape(children, []xsd.ElementDeclaration{el}, el, occurs)
	})
	if err != nil {
		return field{}, nil, err
	}
	return f, []xsd.QName{el.Name()}, nil
}

// groupField returns the field, but for its name, of an <element ref>
// particle to head occurring as occurs, where codegen gives it the sealed
// interface over head's substitution group, members being
// substitutionMembers' for head, and the expanded names of members, the
// instance children that field takes, in group order (codegen/doc.go "#
// Substitution groups"). The interface has one branch per member, in group
// order, holding the one field declField gives the member, of the member's
// own type even where the member heads a group of its own, and occurring
// once. A head with {abstract} true is no member, so its own properties and
// type are not examined, and with no member the interface has no branch. The
// first of these in this order declines: each member in group order, by
// declField's constructs and then by a branch field the shape of an earlier
// member's (field.sharesBranch); then a member's expanded name an earlier
// particle of the model group shares, children indexing the earlier
// particles' names, the first in group order; then occursShape's plain error.
// The field's shape is occursShape's, an optional one the bare interface, nil
// meaning absent, as choiceRoot.field's is.
func groupField(run *bindRun, children map[xsd.QName]int, head xsd.ElementDeclaration, members []xsd.ElementDeclaration, occurs xsd.Occurs) (field, []xsd.QName, error) {
	group := &sealed{p: run.p, children: map[xsd.QName]int{}}
	names := make([]xsd.QName, 0, len(members))
	for _, m := range members {
		f, err := declField(run, m, valueNamed, func() (shape, error) { return shapeOne, nil })
		if err != nil {
			return field{}, nil, err
		}
		f.name = goname.NewNamer().Claim(m.Name().Local)
		for _, earlier := range group.alts {
			if earlier.sharesBranch(f) {
				return field{}, nil, decline(constructSharedBranch, declSubject(m))
			}
		}
		group.children[m.Name()] = len(group.alts)
		group.alts = append(group.alts, f)
		names = append(names, m.Name())
	}
	s, err := particleShape(children, members, head, occurs)
	if err != nil {
		return field{}, nil, err
	}
	if s == shapeOptional {
		s = shapeOne
	}
	return field{shape: s, arm: groupArm{sealed: group}}, names, nil
}

// declField returns the field, but for its name, of the element declaration
// el, its V by el's own type, or as the error the *DeclineError Decode's doc
// states for it, item 5, naming el, or the plain error that item states in a
// decline's place, in the order that doc states: el's own properties, then
// its type, then shapeOf's error, shapeOf returning the field's shape. It
// never examines el's substitution group. A field of simple type holds the Go
// type codegen gives el's type, or the named type over it (value); one of
// complex type holds the type codegen emits for that type, the unit
// run.nested binds.
func declField(run *bindRun, el xsd.ElementDeclaration, value valueType, shapeOf func() (shape, error)) (field, error) {
	schema := run.p.schema
	subject := declSubject(el)
	if d := declarationDecline(el); d != 0 {
		return field{}, decline(d, subject)
	}
	td, ok := schema.ResolvedType(el.TypeDefinition())
	if !ok {
		return field{}, fmt.Errorf("codec: the {type definition} of %s does not resolve", subject)
	}
	// TypeDefinition is a closed sum of the two, so the assertion is STYLE T2's
	// sealed-sum exception.
	st, simple := td.(*xsd.SimpleType)
	if !simple {
		ct, _ := td.(xsd.ComplexType)
		u, err := run.nested(el, ct)
		if err != nil {
			return field{}, err
		}
		s, err := shapeOf()
		if err != nil {
			return field{}, err
		}
		return u.field(s), nil
	}
	g, err := simpleDecline(schema, st, subject)
	if err != nil {
		return field{}, err
	}
	s, err := shapeOf()
	if err != nil {
		return field{}, err
	}
	vc, has := el.ValueConstraint()
	return field{shape: s, arm: scalarArm{value: value, scalar: newScalar(g, st, subject, vc, has)}}, nil
}

// particleShape returns the shape of the field of el's particle occurring as
// occurs, which takes the instance children named by decls, or the decline of
// the first of decls whose expanded name an earlier particle shares, children
// indexing the earlier particles' names, or occursShape's plain error, in
// that order.
func particleShape(children map[xsd.QName]int, decls []xsd.ElementDeclaration, el xsd.ElementDeclaration, occurs xsd.Occurs) (shape, error) {
	for _, d := range decls {
		if _, shared := children[d.Name()]; shared {
			return 0, decline(constructSharedName, declSubject(d))
		}
	}
	return occursShape(occurs, el)
}

// substitutionMembers reports whether codegen gives the field of an <element
// ref> particle to head, an element declaration, the sealed interface over
// head's ·substitution group· (§3.3.6.4 key-eq, [xsd.Schema.SubstitutionGroup],
// which decides cos-equiv-derived-ok-rec clauses 1 to 2.3, a head blocking
// substitution by clause 2.1 holding itself alone) rather than head's own
// type, and returns the group's declarations with {abstract} false, in group
// order, since cvc-elt clause 2 validates no element against an abstract one.
// sum is true exactly when head is top-level, a local declaration heading no
// group (cvc-accept clause 2.3.2), and members are not head alone, so an
// abstract head whose group holds no other declaration with {abstract} false
// answers a sum with no members. It mirrors codegen's substitutionSum.
func substitutionMembers(schema *xsd.Schema, head xsd.ElementDeclaration) (members []xsd.ElementDeclaration, sum bool) {
	if head.ScopeVariety() != xsd.ScopeGlobal {
		return nil, false
	}
	members = slices.DeleteFunc(schema.SubstitutionGroup(head.Name()), xsd.ElementDeclaration.Abstract)
	if len(members) == 1 && members[0].Name() == head.Name() {
		return nil, false
	}
	return members, true
}

// attributeUse returns the attribute field, but for its name, which its
// caller claims, of attribute use u, or as the error the *DeclineError
// Decode's doc states for it, item 5, naming its attribute declaration, or a
// plain error. The field's shape is codegen's, as the package doc's "# What
// decodes" states, and needs no xsi: test here: an xsi: use declines before
// its shape is picked.
func attributeUse(schema *xsd.Schema, u xsd.AttributeUse) (attribute, error) {
	decl, ok := schema.ResolvedAttributeDeclaration(u)
	if !ok {
		return attribute{}, fmt.Errorf("codec: the {attribute declaration} of attribute use %s does not resolve", u.DeclarationName())
	}
	subject := fmt.Sprintf("attribute %s declared at %s", decl.Name(), decl.Loc())
	if decl.Name().Space == xsd.XMLSchemaInstanceNS {
		return attribute{}, decline(constructXsiAttribute, subject)
	}
	st, ok := schema.ResolvedSimpleType(decl.TypeDefinition())
	if !ok {
		return attribute{}, fmt.Errorf("codec: the {type definition} of %s does not resolve", subject)
	}
	g, err := simpleDecline(schema, st, subject)
	if err != nil {
		return attribute{}, err
	}
	shape := shapeOptional
	vc, has := schema.EffectiveValueConstraint(u)
	if u.Required() || has {
		shape = shapeOne
	}
	return attribute{shape: shape, scalar: newScalar(g, st, subject, vc, has)}, nil
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
// an atomic type whose {primitive type definition} is one of
// primitiveAtomics' or datatypeAtomics', as codegen/doc.go "# Simple types"
// picks it: from st's {base type definition} chain (chainAtomic), which meets
// a date, time or duration primitive at its top, failing that from its
// primitive. Or else it returns the construct Decode declines st for: a
// union's, a special type's, or that of its primitive. A list st is a plain
// error: simpleDecline routes a list to its {item type definition}, which
// cos-st-restricts clause 2.1 holds to no list. {variety} is a closed sum, so
// the switch is STYLE T2's sealed-sum exception.
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
	atom, ok, err := chainAtomic(r, st)
	if err != nil {
		return 0, 0, err
	}
	if ok {
		return atom, 0, nil
	}
	if atom, ok := primitiveAtomics[primitive.Name()]; ok {
		return atom, 0, nil
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
// emits for b, the one cx already bound for b and t where it holds one, and
// otherwise the ErrTypeMismatch error Decode's doc states, item 6. Its fields
// are compared, never its name.
func (b *binding) check(t reflect.Type, cx *checkContext) (filler, error) {
	key := boundKey{unit: b, t: t}
	if f, ok := cx.bound[key]; ok {
		return f, nil
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%w: %s is no struct", ErrTypeMismatch, t)
	}
	width := b.content.width()
	if t.NumField() != width+len(b.attrs) {
		return nil, fmt.Errorf("%w: %s has %d fields where codegen emits %d", ErrTypeMismatch, t, t.NumField(), width+len(b.attrs))
	}
	bb := &boundBinding{binding: b}
	cx.bound[key] = bb
	cf, err := b.content.check(t, cx)
	if err != nil {
		return nil, err
	}
	for i, want := range b.attrs {
		if _, err := want.field().check(t, width+i, cx); err != nil {
			return nil, err
		}
	}
	bb.content = cf
	return bb, nil
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

// check returns s, which fills t, when t is a type over the Go type codegen
// gives s's simple type (scalar.underlies), and otherwise the ErrTypeMismatch
// error Decode's doc states, item 6. t's name is never compared.
func (s *simpleRoot) check(t reflect.Type, _ *checkContext) (filler, error) {
	if !s.underlies(t) {
		return nil, fmt.Errorf("%w: the underlying type of %s is not %s", ErrTypeMismatch, t, s.spell())
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

// noAttributes returns the error for the first attribute of the element root
// outside the xsi namespace, for a type with no attribute use.
func noAttributes(root *xmltree.StartElement) error {
	for _, a := range root.Attributes() {
		if a.Name().Space() != xsd.XMLSchemaInstanceNS {
			return unattributed(a)
		}
	}
	return nil
}

// unattributed is the error for the attribute a of an element when no
// attribute use of its declaration's type attributes it.
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
