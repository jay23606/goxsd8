package codec

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"

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

// scalar is one item Decode decodes a value for: its atomic Go type, the
// simple type its value is decoded under, label, the type as a stage error
// and a trace name it (typeLabel), and supplied, the value Decode's doc states
// the post-schema-validation infoset supplies for it, nil exactly when there
// is no value constraint: an element's {value constraint}, an attribute use's
// effective one (§3.5.4 key-evc).
type scalar struct {
	atom     atomic
	st       *xsd.SimpleType
	label    string
	supplied *supplied
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

// newScalar returns the scalar of the item subject names, of simple type st
// and atomic Go type atom, its value constraint vc where has is true.
func newScalar(atom atomic, st *xsd.SimpleType, subject string, vc xsd.ValueConstraint, has bool) scalar {
	return scalar{atom: atom, st: st, label: typeLabel(st, subject), supplied: supply(vc, has, subject)}
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
// §4.1.4 cvc-datatype-valid clause 2.1, and §2.3.1 dt-canonical-mapping.
const (
	ruleWhiteSpace       = "rf-whiteSpace"
	ruleDatatypeValid    = "cvc-datatype-valid"
	ruleCanonicalMapping = "dt-canonical-mapping"
)

// decode runs Decode's value stages on raw, the item's characters or its
// supplied {lexical form}, from src, and returns the value as a value of
// s.atom's bare type: whiteSpace (value.ApplyWhiteSpace), then, for every
// atomic other than string, the lexical mapping (the governing mapping's
// Parse, value.GoverningMapping) and the conversion to the Go type (its
// Canonical, then atomic.convert). It traces one Debug record per stage it
// runs. A whiteSpace fault is a plain error; a lexical-mapping or conversion
// failure is the *stageError Decode's doc states, item 8.
func (s scalar) decode(p *pipeline, raw string, src source) (reflect.Value, error) {
	p.trace("whiteSpace", ruleWhiteSpace, s.label, raw, src)
	normalized, err := value.ApplyWhiteSpace(p.schema, s.st, raw)
	if err != nil {
		return reflect.Value{}, fmt.Errorf("codec: %s: normalizing %s: %w", src.loc(), src.where(), err)
	}
	if s.atom == atomicString {
		return reflect.ValueOf(normalized), nil
	}
	m, ok, err := value.GoverningMapping(p.backend, p.schema, s.st)
	if err != nil {
		return reflect.Value{}, fmt.Errorf("codec: %s: reading the governing mapping of %s for %s: %w", src.loc(), s.label, src.where(), err)
	}
	fail := func(st stage, cause error) error {
		return &stageError{stage: st, label: s.label, fragment: normalized, src: src, cause: cause}
	}
	if !ok {
		return reflect.Value{}, fail(stageLexicalMapping, errors.New("no mapping of the backend governs the type"))
	}
	p.trace("lexical mapping", ruleDatatypeValid, s.label, normalized, src, slog.String("clause", "2.1"))
	v, err := m.Parse(normalized, nil)
	if err != nil {
		return reflect.Value{}, fail(stageLexicalMapping, err)
	}
	if m.Canonical == nil {
		return reflect.Value{}, fail(stageConversion, errors.New("the governing mapping has no canonical mapping"))
	}
	canonical, err := m.Canonical(v)
	if err != nil {
		return reflect.Value{}, fail(stageConversion, err)
	}
	p.trace("conversion", ruleCanonicalMapping, s.label, canonical, src)
	out, err := s.atom.convert(canonical)
	if err != nil {
		return reflect.Value{}, fail(stageConversion, fmt.Errorf("canonical form %q is no %s: %w", canonical, s.atom.bare(), err))
	}
	return out, nil
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
// src, for cause. It has no Unwrap: validate accepted the value on the same
// backend, so no rule cause carries is charged to the instance.
type stageError struct {
	stage    stage
	label    string
	fragment string
	src      source
	cause    error
}

// Error names src's Loc, the stage, the fragment, the type and src, then the
// cause, and states whose fault it is.
func (e *stageError) Error() string {
	return fmt.Sprintf("codec: %s: %s failed on %q, a value of %s in %s: %v; validate accepted the value on the same backend, so this is a fault of the backend, not of the instance",
		e.src.loc(), e.stage.name(), e.fragment, e.label, e.src.where(), e.cause)
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

// loc returns the attribute's Loc, its owning element's start.
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
