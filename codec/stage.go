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
// the backend validate assessed the instance on, and the logger Decode traces
// each stage on, under the "codec" group.
type pipeline struct {
	schema  *xsd.Schema
	backend value.Backend
	log     *slog.Logger
}

// scalar is one item Decode decodes a value for: atom, the atomic Go type of
// its simple type or, for a list, of its {item type definition}; st, the
// simple type its value is decoded under; label, the type as a stage error and
// a trace name it (typeLabel); item, st's {item type definition} where st is a
// list and nil otherwise; and supplied, the value Decode's doc states the
// post-schema-validation infoset supplies for it, nil exactly when there is no
// value constraint: an element's {value constraint}, an attribute use's
// effective one (§3.5.4 key-evc).
type scalar struct {
	atom     atomic
	st       *xsd.SimpleType
	label    string
	item     *listItem
	supplied *supplied
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
// for a datatype struct (atomic.bare).
func (s scalar) goType() reflect.Type {
	if s.item == nil {
		return s.atom.bare()
	}
	return reflect.SliceOf(s.atom.bare())
}

// underlies reports whether v is a type over the Go type codegen gives s's
// simple type, its name never compared, as Decode's doc, item 6, reads a
// simple document element's T. For an atomic Go type other than a datatype
// struct, v's underlying type is goType's: equal Kinds of two scalar types
// mean one underlying type; for big.Rat and big.Int, ConvertibleTo means an
// identical underlying type, which math/big's unexported fields keep any
// other struct from having; and for two slice types, Kind Slice and
// ConvertibleTo mean an identical underlying type, since Go converts between
// slice types only then. For a datatype struct, which codegen declares in the
// caller's package, v is ConvertibleTo codec's mirror, which only a struct of
// its exported fields exactly, in name, order and Go type, struct tags aside,
// is, or for a list v is a slice of such a struct: []D converts to no
// []mirror.
func (s scalar) underlies(v reflect.Type) bool {
	if !s.atom.mirrored() {
		want := s.goType()
		return v.Kind() == want.Kind() && v.ConvertibleTo(want)
	}
	if s.item != nil {
		if v.Kind() != reflect.Slice {
			return false
		}
		v = v.Elem()
	}
	return v.ConvertibleTo(s.atom.bare())
}

// spell spells the Go type codegen gives s's simple type for an error
// message, A as atomic.spell spells it: A, or []A for a list.
func (s scalar) spell() string {
	if s.item == nil {
		return s.atom.spell()
	}
	return "[]" + s.atom.spell()
}

// simpleGo is the Go type codegen gives a simple type Decode decodes, as
// simpleDecline reads it: atom, its atomic Go type or, for a list, its {item
// type definition}'s, and item, that {item type definition} for a list and nil
// otherwise.
type simpleGo struct {
	atom atomic
	item *xsd.SimpleType
}

// supplied is the {lexical form} of the value constraint of the item subject
// names, its declaration and that declaration's schema Loc.
type supplied struct {
	lexical string
	subject string
}

// supply returns what the value constraint vc of the item subject names
// supplies, or nil where has is false and there is none.
func supply(vc xsd.ValueConstraint, has bool, subject string) *supplied {
	if !has {
		return nil
	}
	return &supplied{lexical: vc.LexicalForm(), subject: subject}
}

// newScalar returns the scalar of the item subject names, of simple type st,
// whose Go type is g, its value constraint vc where has is true.
func newScalar(g simpleGo, st *xsd.SimpleType, subject string, vc xsd.ValueConstraint, has bool) scalar {
	s := scalar{atom: g.atom, st: st, label: typeLabel(st, subject), supplied: supply(vc, has, subject)}
	if g.item != nil {
		s.item = &listItem{st: g.item, label: itemLabel(g.item, s.label)}
	}
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
// for a conversion of the lexical (atomic.readsLexical), §2.3
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
// only when there is an item. It traces one Debug record per stage it runs. A
// whiteSpace fault is a plain error; a lexical-mapping or conversion failure
// is the *stageError Decode's doc states, item 8.
func (s scalar) decode(p *pipeline, raw string, src source) (reflect.Value, error) {
	p.trace("whiteSpace", ruleWhiteSpace, s.label, raw, src)
	normalized, err := value.ApplyWhiteSpace(p.schema, s.st, raw)
	if err != nil {
		return reflect.Value{}, fmt.Errorf("codec: %s: normalizing %s: %w", src.loc(), src.where(), err)
	}
	if s.item == nil {
		ls, err := newLexicalStages(p, s.atom, s.st, s.label, src, normalized, 0)
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
	ls, err := newLexicalStages(p, s.atom, s.item.st, s.item.label, src, tokens[0], 1)
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
// value.GoverningMapping returns) and the conversion to the Go type
// (atomic.convert, of the form m's Canonical renders or, where the atomic
// readsLexical, of the lexical Parse accepted). m is the zero Mapping for
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
// struct, which shape.store converts to the caller's type.
func (ls lexicalStages) run(p *pipeline, lexical string, token int) (reflect.Value, error) {
	if ls.atom == atomicString {
		return reflect.ValueOf(lexical), nil
	}
	p.trace("lexical mapping", ruleDatatypeValid, ls.label, lexical, ls.src, slog.String("clause", "2.1"))
	v, err := ls.m.Parse(lexical, nil)
	if err != nil {
		return reflect.Value{}, ls.fail(stageLexicalMapping, lexical, token, err)
	}
	if ls.atom.readsLexical() {
		p.trace("conversion", ruleLexicalMapping, ls.label, lexical, ls.src)
		out, err := ls.atom.convert(lexical)
		if err != nil {
			return reflect.Value{}, ls.fail(stageConversion, lexical, token, fmt.Errorf("it is no %s: %w", ls.atom.spell(), err))
		}
		return out, nil
	}
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

// stage is a value stage Decode tags its errors with: the lexical mapping
// and the conversion to the Go type, its canonical rendering included. The
// zero value is invalid (STYLE T1).
type stage uint8

const (
	stageLexicalMapping stage = iota + 1
	stageConversion
)

// name returns the stage's name.
func (s stage) name() string {
	switch s {
	case stageLexicalMapping:
		return "the lexical mapping"
	case stageConversion:
		return "the conversion to the Go type"
	}
	// The zero stage is an internal bug, never input.
	panic(fmt.Sprintf("codec: invalid stage %d", s))
}

// stageError is the error Decode's doc states in item 8: stage failed on
// fragment, the normalized lexical of a value of the type label names, from
// src, for cause. For a list value, fragment is one list item, label names
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

// attributeSource is the present attribute a, which has no byte offset of its
// own (xmltree.Attribute).
type attributeSource struct{ a xmltree.Attribute }

// loc returns the attribute's own Loc (xmltree.Attribute.Loc).
func (a attributeSource) loc() xsderr.Loc { return a.a.Loc() }

// where names the attribute.
func (a attributeSource) where() string {
	return fmt.Sprintf("attribute %s", expanded(a.a.Name()))
}

// suppliedSource is the value the value constraint of the item subject names
// supplies to the element start or to an attribute of it.
type suppliedSource struct {
	start   *xmltree.StartElement
	subject string
}

// loc returns start's Loc.
func (s suppliedSource) loc() xsderr.Loc { return s.start.Loc() }

// where names the item whose value constraint supplied the value, and the
// element it is supplied on.
func (s suppliedSource) where() string {
	return fmt.Sprintf("the value the value constraint of %s supplies on element %s", s.subject, expanded(s.start.Name()))
}
