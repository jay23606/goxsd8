package codec

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
	"strings"

	"github.com/kud360/goxsd8/parser/xmltree"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// pipeline is what Decode's value stages read beside the item: the schema,
// the backend validate assessed the instance on, the logger Decode traces
// each stage on, under the "codec" group, decimals, the arm sets
// [WithPrecisionDecimalArms] supplied, and branches, the sets [WithBranches]
// supplied, each from the config field the item-6 check reads
// (checkContext), from which the fill builds a PrecisionDecimal's Value
// (pipeline.decimal) and a union's variant (pipeline.union).
type pipeline struct {
	schema   *xsd.Schema
	backend  value.Backend
	log      *slog.Logger
	decimals decimalArmSets
	branches branchSets
}

// scalar is one item Decode decodes a value for: elem, the element of the Go
// type codegen gives its simple type, of the type itself or, for a list, of
// its {item type definition}; st, the simple type its value is decoded under;
// label, the type as a stage error and a trace name it (typeLabel); item,
// st's {item type definition} where st is a list and nil otherwise; and
// supplied, the value Decode's doc states the post-schema-validation infoset
// supplies for it, nil exactly when there is no value constraint: an
// element's {value constraint}, an attribute use's effective one (§3.5.4
// key-evc).
type scalar struct {
	elem     scalarElem
	st       *xsd.SimpleType
	label    string
	item     *listItem
	supplied *supplied
}

// scalarElem is the element of the Go type codegen gives a scalar's simple
// type, as Decode decodes it: an atomic, the atomic Go type of an atomic type
// or of a list's atomic {item type definition}, or a *unionScalar, the basic
// members of a union or of a list's union {item type definition}. It is a
// closed sum, sealed by its unexported methods, so a union's scalar holds no
// atomic Go type. list, in each method, reports whether the scalar's simple
// type is a list of the element.
type scalarElem interface {
	// bare returns the Go type codec decodes the element's value as: an
	// atomic's bare, and for a union, whose interface codec holds none of,
	// unionValue, which pipeline.union builds the caller's variant from.
	bare() reflect.Type
	// spell spells the element's Go type for an error message, as
	// scalar.spell states.
	spell() string
	// datatypeStruct reports whether the element's Go type is a datatype
	// struct (atomic.datatypeStruct); a union's interface is none.
	datatypeStruct() bool
	// underlies answers scalar.underlies for a scalar of this element.
	underlies(v reflect.Type, list bool) bool
	// holdSets answers scalar.holdSets for a scalar of this element.
	holdSets(v reflect.Type, list bool, site string, cx *checkContext) error
}

// listItem is the {item type definition} of a list type (Datatypes §2.4.1.2):
// st, the type each list item is mapped under, and label, which names st and
// the list for a stage error and a trace.
type listItem struct {
	st    *xsd.SimpleType
	label string
}

// goType returns the Go type codegen gives s's simple type: s's atomic Go type
// A, or []A for a list (codegen/doc.go "# Simple types"), A codec's mirror
// for a datatype struct (atomic.bare), and for a union, whose interface codec
// holds none of, unionValue, which pipeline.union builds the caller's variant
// from.
func (s scalar) goType() reflect.Type {
	if s.item == nil {
		return s.elem.bare()
	}
	return reflect.SliceOf(s.elem.bare())
}

// underlies reports whether v is a type over the Go type codegen gives s's
// simple type, its name never compared, as Decode's doc, item 6, reads a
// simple document element's T: atomic.underlies' answer, or for a union
// unionScalar.underlies'.
func (s scalar) underlies(v reflect.Type) bool { return s.elem.underlies(v, s.item != nil) }

// underlies reports whether v is a type over a's Go type, or for a list over
// []A, as scalar.underlies reads it. For an atomic Go type other than a
// datatype struct, v's underlying type is that Go type: equal Kinds of two
// scalar types mean one underlying type; for big.Rat and big.Int,
// ConvertibleTo means an identical underlying type, which math/big's
// unexported fields keep any other struct from having; and for two slice
// types, a list's or []byte's, Kind Slice and ConvertibleTo mean an identical
// underlying type, since Go converts between slice types only then. For a
// datatype struct, which codegen declares in the caller's package, v is a
// struct of its shape (atomic.shapes), ConvertibleTo codec's mirror, which
// only a struct of its exported fields exactly, in name, order and Go type,
// struct tags aside, is, or for PrecisionDecimal's, which has no mirror, a
// struct decimalShape holds, or for a list v is a slice of such a struct: []D
// converts to no []mirror.
func (a atomic) underlies(v reflect.Type, list bool) bool {
	if !a.datatypeStruct() {
		want := a.bare()
		if list {
			want = reflect.SliceOf(want)
		}
		return v.Kind() == want.Kind() && v.ConvertibleTo(want)
	}
	if list {
		if v.Kind() != reflect.Slice {
			return false
		}
		v = v.Elem()
	}
	return a.shapes(v)
}

// underlies reports whether v is a type over a union's interface, or for a
// list of a union over []I, as scalar.underlies reads it: codegen declares
// the interface in the caller's package, so v is any interface of one
// unexported method (sealedShape), or for a list a slice of one.
func (u *unionScalar) underlies(v reflect.Type, list bool) bool {
	if list {
		if v.Kind() != reflect.Slice {
			return false
		}
		v = v.Elem()
	}
	return sealedShape(v)
}

// holdSets holds v, a type underlies holds, at the site site names, to the
// sets cx supplies for the interfaces it holds, item 6 of Decode's doc:
// atomic.holdSets' answer, or for a union unionScalar.holdSets'.
func (s scalar) holdSets(v reflect.Type, site string, cx *checkContext) error {
	return s.elem.holdSets(v, s.item != nil, site, cx)
}

// holdSets returns nil unless a is PrecisionDecimal's atomic, and otherwise
// checkDecimalArms' answer for the Value interface of v, a type underlies
// holds, or of its element type for a list, at the site site names: so a
// PrecisionDecimal's V holds, beside its shape, the arms cx supplies for its
// interface (Decode's doc, item 6).
func (a atomic) holdSets(v reflect.Type, list bool, site string, cx *checkContext) error {
	if a != atomicPrecisionDecimal {
		return nil
	}
	if list {
		v = v.Elem()
	}
	return checkDecimalArms(v.Field(0).Type, site, cx)
}

// holdSets holds v, a type underlies holds, or its element type for a list,
// at the site site names, to u's variants (unionScalar.check).
func (u *unionScalar) holdSets(v reflect.Type, list bool, site string, cx *checkContext) error {
	if list {
		v = v.Elem()
	}
	return u.check(v, site, cx)
}

// spell spells the Go type codegen gives s's simple type for an error
// message, A as atomic.spell spells it, or for a union I as
// unionScalar.spell does: A or I, or []A or []I for a list.
func (s scalar) spell() string {
	if s.item == nil {
		return s.elem.spell()
	}
	return "[]" + s.elem.spell()
}

// callerDeclared reports whether codegen declares the element of the Go type
// it gives s's simple type in the caller's package, so that codec holds no Go
// type to compare a caller's with: a datatype struct, or a union's interface.
func (s scalar) callerDeclared() bool {
	// scalarElem is a closed sum, so the assertion is STYLE T2's sealed-sum
	// exception.
	_, isUnion := s.elem.(*unionScalar)
	return isUnion || s.elem.datatypeStruct()
}

// simpleGo is the Go type codegen gives a simple type Decode decodes, as
// simpleDecline reads it: elem, the element of that Go type, of the type
// itself or, for a list, of its {item type definition}; and item, that {item
// type definition} for a list and nil otherwise.
type simpleGo struct {
	elem goElem
	item *xsd.SimpleType
}

// goElem is the element of the Go type codegen gives a simple type, as
// simpleDecline reads it: an atomic, the atomic Go type of an atomic type or
// of a list's atomic {item type definition}, or a *unionGo, the sealed
// interface of a union or of a list's union {item type definition}. It is a
// closed sum, sealed by its unexported methods, so a union's simpleGo holds
// no atomic Go type.
type goElem interface {
	// bare returns the Go type codec decodes the element's value as, the one
	// the scalarElem forScalar returns answers.
	bare() reflect.Type
	// forScalar returns the element of a scalar of this element, labelled
	// label: an atomic itself, or a union's unionScalar (unionGo.forScalar).
	forScalar(label string) scalarElem
}

// forScalar returns a itself.
func (a atomic) forScalar(string) scalarElem { return a }

// oneGoType reports whether g and h, the Go types of two basic members of one
// union, are one Go type as item 6 of Decode's doc compares a variant's, by
// shape alone, so that no variant could tell them apart: the same A or []A
// (goType), so that xs:string and xs:token are one, and so are xs:QName and
// xs:NOTATION, or both lists of a union, each a slice of any sealed
// interface, whose element's bare type is unionValue, no atomic's. No basic
// member is a union (basicMembers), and the datatype structs of two
// datatypes have different fields, so no variant is over both.
func (g simpleGo) oneGoType(h simpleGo) bool { return g.goType() == h.goType() }

// sealed reports whether g is a sealed interface, a union's own, which
// codegen holds in an optional field bare, never behind a pointer
// (codegen/doc.go "# Particles"); a list of a union is a slice.
func (g simpleGo) sealed() bool {
	// goElem is a closed sum, so the assertion is STYLE T2's sealed-sum
	// exception.
	_, isUnion := g.elem.(*unionGo)
	return isUnion && g.item == nil
}

// goType returns the Go type codec decodes a value of g as, as scalar.goType
// does: g's element's bare type, or a slice of it for a list.
func (g simpleGo) goType() reflect.Type {
	if g.item == nil {
		return g.elem.bare()
	}
	return reflect.SliceOf(g.elem.bare())
}

// supplied is the value constraint vc of the item subject names, its
// declaration and that declaration's schema Loc, whose {lexical form} the
// post-schema-validation infoset supplies.
type supplied struct {
	vc      xsd.ValueConstraint
	subject string
}

// supply returns what the value constraint vc of the item subject names
// supplies, or nil where has is false and there is none.
func supply(vc xsd.ValueConstraint, has bool, subject string) *supplied {
	if !has {
		return nil
	}
	return &supplied{vc: vc, subject: subject}
}

// newScalar returns the scalar of the item subject names, of simple type st,
// whose Go type is g, its value constraint vc where has is true.
func newScalar(g simpleGo, st *xsd.SimpleType, subject string, vc xsd.ValueConstraint, has bool) scalar {
	s := scalarOf(g, st, typeLabel(st, subject))
	s.supplied = supply(vc, has, subject)
	return s
}

// scalarOf returns the scalar of simple type st, whose Go type is g, labelled
// label, with no supplied value: for a union, or a list of one, each basic
// member's scalar too, labelled by memberLabel after the union's label.
func scalarOf(g simpleGo, st *xsd.SimpleType, label string) scalar {
	s := scalar{st: st, label: label}
	elemLabel := label
	if g.item != nil {
		s.item = &listItem{st: g.item, label: itemLabel(g.item, label)}
		elemLabel = s.item.label
	}
	s.elem = g.elem.forScalar(elemLabel)
	return s
}

// itemLabel names item, the {item type definition} of the list type list
// names, for a stage error and a trace: its QName and then "the item type of "
// and list, or for an anonymous item that phrase alone.
func itemLabel(item *xsd.SimpleType, list string) string {
	if item.Name() != (xsd.QName{}) {
		return item.Name().String() + ", the item type of " + list
	}
	return "the item type of " + list
}

// typeLabel names st for a stage error and a trace: its QName, or for an
// anonymous st "the anonymous simple type of " and subject, which names the
// declaration st is local to and its schema Loc.
func typeLabel(st *xsd.SimpleType, subject string) string {
	if st.Name() != (xsd.QName{}) {
		return st.Name().String()
	}
	return "the anonymous simple type of " + subject
}

// The rules each stage's trace record names: Datatypes §4.3.6 rf-whiteSpace,
// §4.1.4 cvc-datatype-valid clause 2.1, §2.3.1 dt-canonical-mapping, and,
// for a conversion of the lexical (readLexical, readExpanded), §2.3
// dt-lexical-mapping.
const (
	ruleWhiteSpace       = "rf-whiteSpace"
	ruleDatatypeValid    = "cvc-datatype-valid"
	ruleCanonicalMapping = "dt-canonical-mapping"
	ruleLexicalMapping   = "dt-lexical-mapping"
)

// decode runs Decode's value stages on raw, the item's characters or its
// supplied {lexical form}, from src, and returns the value as a value of
// s.goType(): whiteSpace (value.ApplyWhiteSpace) under s.st, then, for an
// atomic type, the lexical stages on the normalized value. For a list, whose
// whiteSpace is collapse (Datatypes §4.3.6 rf-whiteSpace, §2.4.1.2), the
// normalized value is split at each U+0020 before any item is mapped
// (§2.4.1.2), an empty one into no items (§4.1.1 dt-list), and the lexical
// stages run on each item under s.item.st, the governing mapping read once,
// only when there is an item. For a union, the ·active basic member· is
// identified on raw first (unionScalar.identify), and the value is that
// member's, decoded by these same stages under it, whiteSpace included, as a
// unionValue; for a list of a union, it is identified on each list item, and
// the lexical stages run on the item under it. Identification runs no stage
// of its own and traces nothing. It traces one Debug record per stage it
// runs. An identification, lexical-mapping or conversion failure is the
// *stageError Decode's doc states, item 8.
//
// A whiteSpace fault never reaches Decode's return. On a schema Finalize
// accepted it is an error wrapping the *xsderr.Error of rule
// xsderr.RuleComponentInvariant that value.ApplyWhiteSpace's doc states, of
// an atomic or list s.st (§4.1.5 cos-applicable-facets). Finalize does
// not diagnose it, but validate's String Valid runs the same
// value.ApplyWhiteSpace on the same simple type for every value Decode decodes
// — an element's character content, a present attribute, or a supplied value
// — and records the fault, which is no datatype verdict, as Unevaluated, so
// Decode returns its item 4 first.
func (s scalar) decode(p *pipeline, raw string, src source) (reflect.Value, error) {
	// scalarElem is a closed sum, so the assertion is STYLE T2's sealed-sum
	// exception.
	u, isUnion := s.elem.(*unionScalar)
	if isUnion && s.item == nil {
		member, err := u.identify(p, s.st, s.label, raw, src, 0)
		if err != nil {
			return reflect.Value{}, err
		}
		v, err := member.decode(p, raw, src)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(unionValue{member: member, v: v}), nil
	}
	p.trace("whiteSpace", ruleWhiteSpace, s.label, raw, src)
	normalized, err := value.ApplyWhiteSpace(p.schema, s.st, raw)
	if err != nil {
		return reflect.Value{}, fmt.Errorf("codec: %s: normalizing %s: %w", src.loc(), src.where(), err)
	}
	// Past isUnion, s.elem is the sum's other arm, an atomic.
	if s.item == nil {
		ls, err := newLexicalStages(p, s.elem.(atomic), s.st, s.label, src, normalized, 0)
		if err != nil {
			return reflect.Value{}, err
		}
		return ls.run(p, normalized, 0)
	}
	var tokens []string
	if normalized != "" {
		tokens = strings.Split(normalized, " ")
	}
	out := reflect.MakeSlice(s.goType(), 0, len(tokens))
	if len(tokens) == 0 {
		return out, nil
	}
	if isUnion {
		for i, tok := range tokens {
			v, err := u.item(p, s.item, tok, src, i+1)
			if err != nil {
				return reflect.Value{}, err
			}
			out = reflect.Append(out, v)
		}
		return out, nil
	}
	ls, err := newLexicalStages(p, s.elem.(atomic), s.item.st, s.item.label, src, tokens[0], 1)
	if err != nil {
		return reflect.Value{}, err
	}
	for i, tok := range tokens {
		v, err := ls.run(p, tok, i+1)
		if err != nil {
			return reflect.Value{}, err
		}
		out = reflect.Append(out, v)
	}
	return out, nil
}

// lexicalStages are Decode's value stages after whiteSpace for lexicals of
// one atomic simple type, label naming it, from src: for every atomic other
// than string, the lexical mapping (m's Parse, the governing mapping
// value.GoverningMapping returns, given src's bindings) and the conversion to
// the Go type, by the atomic's reading: atomic.convert of the form m's
// Canonical renders, or of the lexical Parse accepted, or expandedName of
// that lexical under the same bindings. m is the zero Mapping for
// atomicString, whose value is the lexical itself.
type lexicalStages struct {
	atom  atomic
	label string
	src   source
	m     value.Mapping
}

// newLexicalStages returns the lexicalStages of st, of atomic Go type atom,
// or the error for a governing mapping of st that cannot be read, the
// *stageError of item 8 where no mapping of p's backend governs st, charged to
// first, the first lexical, at position token as stageError states it.
//
// A fault reading the governing mapping never reaches Decode's return:
// value.GoverningMapping fails only on a {variety} or {base type definition}
// hop of st, Finalize refuses every such hop that does not resolve
// (Structures §3.17.6.2 src-resolve clause 1.1) or is absent
// (xsderr.RuleComponentInvariant), and value.ApplyWhiteSpace (an atomic st)
// or atomicDecline (a list's item type) has read the same hops on the same
// schema first.
func newLexicalStages(p *pipeline, atom atomic, st *xsd.SimpleType, label string, src source, first string, token int) (lexicalStages, error) {
	ls := lexicalStages{atom: atom, label: label, src: src}
	if atom == atomicString {
		return ls, nil
	}
	m, ok, err := value.GoverningMapping(p.backend, p.schema, st)
	if err != nil {
		return lexicalStages{}, fmt.Errorf("codec: %s: reading the governing mapping of %s for %s: %w", src.loc(), label, src.where(), err)
	}
	if !ok {
		return lexicalStages{}, ls.fail(stageLexicalMapping, first, token, errors.New("no mapping of the backend governs the type"))
	}
	ls.m = m
	return ls, nil
}

// run returns the value of lexical, at position token as stageError states
// it, as a value of ls.atom's bare type, codec's mirror for a datatype
// struct, which shape.store converts to the caller's type, or decimalValue,
// which shape.store builds the caller's PrecisionDecimal from.
func (ls lexicalStages) run(p *pipeline, lexical string, token int) (reflect.Value, error) {
	if ls.atom == atomicString {
		return reflect.ValueOf(lexical), nil
	}
	p.trace("lexical mapping", ruleDatatypeValid, ls.label, lexical, ls.src, slog.String("clause", "2.1"))
	ctx := ls.src.bindings()
	v, err := ls.m.Parse(lexical, ctx)
	if err != nil {
		return reflect.Value{}, ls.fail(stageLexicalMapping, lexical, token, err)
	}
	switch ls.atom.reading() {
	case readLexical:
		p.trace("conversion", ruleLexicalMapping, ls.label, lexical, ls.src)
		out, err := ls.atom.convert(lexical)
		if err != nil {
			return reflect.Value{}, ls.fail(stageConversion, lexical, token, fmt.Errorf("it is no %s: %w", ls.atom.spell(), err))
		}
		return out, nil
	case readExpanded:
		p.trace("conversion", ruleLexicalMapping, ls.label, lexical, ls.src)
		out, err := expandedName(lexical, ctx)
		if err != nil {
			return reflect.Value{}, ls.fail(stageConversion, lexical, token, err)
		}
		return out, nil
	case readCanonical:
		return ls.canonical(p, v, lexical, token)
	}
	// The zero reading is an internal bug, never input.
	panic(fmt.Sprintf("codec: atomic %s has no reading", ls.atom.bare()))
}

// canonical returns the value of v, which Parse mapped lexical to, at
// position token, as run does, read from the form ls.m's Canonical renders v
// in.
func (ls lexicalStages) canonical(p *pipeline, v value.Value, lexical string, token int) (reflect.Value, error) {
	if ls.m.Canonical == nil {
		return reflect.Value{}, ls.fail(stageConversion, lexical, token, errors.New("the governing mapping has no canonical mapping"))
	}
	canonical, err := ls.m.Canonical(v)
	if err != nil {
		return reflect.Value{}, ls.fail(stageConversion, lexical, token, err)
	}
	p.trace("conversion", ruleCanonicalMapping, ls.label, canonical, ls.src)
	out, err := ls.atom.convert(canonical)
	if err != nil {
		return reflect.Value{}, ls.fail(stageConversion, lexical, token, fmt.Errorf("canonical form %q is no %s: %w", canonical, ls.atom.spell(), err))
	}
	return out, nil
}

// fail returns the *stageError of stage st on fragment, at position token,
// for cause.
func (ls lexicalStages) fail(st stage, fragment string, token int, cause error) error {
	return &stageError{stage: st, label: ls.label, fragment: fragment, token: token, src: ls.src, cause: cause}
}

// trace logs one Debug record for the stage named msg on the value from src,
// with the stage's rule, the type's label, the stage's input and src's Loc,
// then extra.
func (p *pipeline) trace(msg, rule, label, input string, src source, extra ...slog.Attr) {
	ctx := context.Background()
	if !p.log.Enabled(ctx, slog.LevelDebug) {
		return
	}
	attrs := append([]slog.Attr{slog.String("rule", rule)}, extra...)
	attrs = append(attrs, slog.String("type", label), slog.String("input", input), slog.String("loc", src.loc().String()))
	p.log.LogAttrs(ctx, slog.LevelDebug, msg, attrs...)
}

// stage is a value stage Decode tags its errors with: a union's
// identification of its ·active basic member·, the lexical mapping and the
// conversion to the Go type, its canonical rendering included. The zero value
// is invalid (STYLE T1).
type stage uint8

const (
	stageIdentification stage = iota + 1
	stageLexicalMapping
	stageConversion
)

// name returns the stage's name.
func (s stage) name() string {
	switch s {
	case stageIdentification:
		return "the identification of the ·active basic member·"
	case stageLexicalMapping:
		return "the lexical mapping"
	case stageConversion:
		return "the conversion to the Go type"
	}
	// The zero stage is an internal bug, never input.
	panic(fmt.Sprintf("codec: invalid stage %d", s))
}

// stageError is the error Decode's doc states in item 8: stage failed on
// fragment, the normalized lexical of a value of the type label names, or for
// a union's identification the characters it was identified on, from src, for
// cause. For a list value, fragment is one list item, label names
// the list's {item type definition}, and token is the item's 1-based position
// among the value's items; for an atomic value token is 0. It has no Unwrap:
// validate accepted the value on the same backend, so no rule cause carries is
// charged to the instance.
type stageError struct {
	stage    stage
	label    string
	fragment string
	token    int
	src      source
	cause    error
}

// Error names src's Loc, the stage, the fragment, for a list item its
// position, the type and src, then the cause, and states whose fault it is.
func (e *stageError) Error() string {
	fragment := strconv.Quote(e.fragment)
	if e.token > 0 {
		fragment = fmt.Sprintf("list item %d, %s", e.token, fragment)
	}
	return fmt.Sprintf("codec: %s: %s failed on %s, a value of %s in %s: %v; validate accepted the value on the same backend, so this is a fault of the backend, not of the instance",
		e.src.loc(), e.stage.name(), fragment, e.label, e.src.where(), e.cause)
}

// A source is where a value Decode decodes comes from, as a stage error and
// a trace name it: an element's character content, a present attribute, or a
// value constraint's supplied value.
type source interface {
	// loc returns the instance Loc the value is charged to.
	loc() xsderr.Loc
	// where names the value's item, and the byte offset where it has one.
	where() string
	// bindings returns the namespace bindings the value's lexical is mapped
	// under and a QName or NOTATION lexical resolves its prefix against, the
	// ones Decode's doc names for each source.
	bindings() value.Context
}

// elementBindings is the value.Context of the namespace bindings in scope at
// the element e (xmltree.StartElement.LookupPrefix).
type elementBindings struct{ e *xmltree.StartElement }

// LookupNamespace resolves prefix against the bindings in scope at e.
func (b elementBindings) LookupNamespace(prefix string) (string, bool) {
	return b.e.LookupPrefix(prefix)
}

// contentSource is the character content of the element start, first its
// first character data, nil when it has none.
type contentSource struct {
	start *xmltree.StartElement
	first *xmltree.CharData
}

// loc returns the first character data's Loc, or start's when there is none.
func (c contentSource) loc() xsderr.Loc {
	if c.first == nil {
		return c.start.Loc()
	}
	return c.first.Loc()
}

// where names the element and the byte offset of its first character data.
func (c contentSource) where() string {
	if c.first == nil {
		return fmt.Sprintf("the character content of element %s, which has none", expanded(c.start.Name()))
	}
	return fmt.Sprintf("the character content of element %s at byte offset %d", expanded(c.start.Name()), c.first.Offset())
}

// bindings returns the bindings in scope at start.
func (c contentSource) bindings() value.Context { return elementBindings{c.start} }

// attributeSource is the present attribute a of the element owner; a has no
// byte offset of its own (xmltree.Attribute).
type attributeSource struct {
	a     xmltree.Attribute
	owner *xmltree.StartElement
}

// loc returns the attribute's own Loc (xmltree.Attribute.Loc).
func (a attributeSource) loc() xsderr.Loc { return a.a.Loc() }

// where names the attribute.
func (a attributeSource) where() string {
	return fmt.Sprintf("attribute %s", expanded(a.a.Name()))
}

// bindings returns the bindings in scope at owner.
func (a attributeSource) bindings() value.Context { return elementBindings{a.owner} }

// suppliedSource is the value supplied supplies to the element start or to
// an attribute of it.
type suppliedSource struct {
	start    *xmltree.StartElement
	supplied *supplied
}

// loc returns start's Loc.
func (s suppliedSource) loc() xsderr.Loc { return s.start.Loc() }

// where names the item whose value constraint supplied the value, and the
// element it is supplied on.
func (s suppliedSource) where() string {
	return fmt.Sprintf("the value the value constraint of %s supplies on element %s", s.supplied.subject, expanded(s.start.Name()))
}

// bindings returns the value constraint's own, those in scope at the schema
// document element that wrote it (value.ConstraintContext), never start's.
func (s suppliedSource) bindings() value.Context { return value.ConstraintContext(s.supplied.vc) }
