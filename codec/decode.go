package codec

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"

	"github.com/kud360/goxsd8/internal/goname"
	"github.com/kud360/goxsd8/parser/xmltree"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/validate/xmlsrc"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// ErrTypeMismatch is the error [Decode] wraps when T is not the struct codegen
// emits for the document element's declaration; Decode's doc states when.
var ErrTypeMismatch = errors.New("codec: T is not the struct codegen emits for the document element")

// The names of the primitives the slice decodes, read by identity.
var (
	stringPrimitive = xsd.QName{Space: xsd.XMLSchemaNS, Local: "string"}
	anyURIPrimitive = xsd.QName{Space: xsd.XMLSchemaNS, Local: "anyURI"}
)

// Decode decodes the XML instance data into a new value of T, the struct
// package codegen emits for the declaration of data's document element, and
// returns it. A value is returned exactly when the error is nil.
//
// The slice Decode covers is the package doc's "# What decodes". Decode
// first assesses data against schema with package validate and decodes only
// an instance whose Result has no source fault and stands
// validate.NotProvenInvalid; it then reads data a second time with
// parser/xmltree. Each decoded value is the item's ·normalized value·
// (Structures §3.1.4 key-nv) under its declaration's simple type: the string
// [value.ApplyWhiteSpace] returns for the item's characters, an attribute's as
// XML 1.0 §3.3.3 normalized them, an element's the concatenation of its
// character children. Where the post-schema-validation infoset supplies a
// value it is that value, the {lexical form} of a value constraint under the
// same normalization: for an absent attribute whose use has an effective value
// constraint (§3.5.4 key-evc; §3.4.4.2 key-dflt-att, §3.4.5.1
// sic-attrDefault), and for an element with neither element nor character
// children whose declaration has a {value constraint} (§3.3.4.3 cvc-elt clause
// 5.1, §3.3.5.4 sic-eltType). So an optional attribute use with an effective
// value constraint never decodes to a nil *string; a whitespace-only element
// has a character child and holds its own value; and an element absent from
// the instance supplies nothing, its field nil or not appended to.
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
//  3. A validate.Charged Result: an error wrapping every violation, so that
//     errors.As and xsderr.RuleOf reach the first violation's rule and
//     instance Loc. Decode never succeeds where validate does not accept.
//  4. A validate.Undecided Result: an error naming the rule and instance Loc
//     of the first validate.Unevaluated and wrapping errors.ErrUnsupported.
//     It is no *DeclineError: errors.As tells the two apart.
//  5. A construct of the document element's declaration or type outside the
//     slice: a *DeclineError at the document element's instance Loc, whose
//     [DeclineError.Construct] is that construct's. Each is found in this
//     order: the declaration's {type table} and {nillable}, its {type
//     definition} and that type's content, each particle of its content in
//     order, each of its attribute uses in order, and its {attribute
//     wildcard}.
//  6. T is not a struct whose fields are exactly those codegen emits for the
//     declaration, in name, order and Go type, as Decode derives them from
//     the schema: an error errors.Is reports as [ErrTypeMismatch], no
//     *DeclineError, so that a struct that does not match the schema is never
//     filled. Decode never reads the fields from T, so for a struct codegen
//     emitted from this schema the error is a defect of Decode's, and for any
//     other T it is a T that does not match.
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
	if err := b.check(reflect.TypeFor[T]()); err != nil {
		return nil, err
	}
	out := new(T)
	if err := b.fill(r, root, reflect.ValueOf(out).Elem()); err != nil {
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
		violations := res.Violations()
		errs := make([]error, len(violations))
		for i, v := range violations {
			errs[i] = v
		}
		return fmt.Errorf("codec: the instance is invalid against the schema: %w", errors.Join(errs...))
	case validate.Undecided:
		u := res.Unevaluated()[0]
		return fmt.Errorf("codec: %s: the validator left %s undecided, so the instance is not proven valid: %w", u.Loc(), u.Rule(), errors.ErrUnsupported)
	}
	return fmt.Errorf("codec: the validator returned Standing %d, which is no arm", res.Standing())
}

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

// field is one field of the struct codegen emits for the document element's
// declaration: its Go name, its shape, the simple type its value is
// normalized under, and supplied, the value Decode's doc states the
// post-schema-validation infoset supplies for it, already normalized under
// st, nil exactly when there is no value constraint: an element's {value
// constraint}, an attribute use's effective one (§3.5.4 key-evc).
type field struct {
	name     string
	shape    shape
	st       *xsd.SimpleType
	supplied *string
}

// binding is the struct codegen emits for one document element declaration,
// as Decode reads it: its fields in codegen's order, each element particle's
// first, then each attribute use's.
type binding struct {
	schema *xsd.Schema
	fields []field
	// firstAttribute is the index in fields of the first attribute use's
	// field, len(fields) when there is none.
	firstAttribute int
	// children and attributes map an expanded name to its field's index; each
	// is a lookup index only and never iterated (STYLE D2).
	children   map[xsd.QName]int
	attributes map[xsd.QName]int
}

// bind returns the binding of root's declaration, or the first decline
// Decode's doc states for it, item 5, at root's instance Loc.
func bind(schema *xsd.Schema, root *xmltree.StartElement) (*binding, error) {
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
	ct, ok := td.(xsd.ComplexType)
	if !ok {
		return nil, decline(loc, constructSimpleRoot, typeSubject)
	}
	particles, d := contentParticles(ct)
	if d != 0 {
		return nil, decline(loc, d, typeSubject)
	}
	b := &binding{schema: schema, children: map[xsd.QName]int{}, attributes: map[xsd.QName]int{}}
	var locals []string
	for _, p := range particles {
		el, d := particleElement(p)
		if d != 0 {
			return nil, decline(loc, d, "a particle of "+typeSubject)
		}
		f, d, err := b.localElement(el, p.Occurs())
		if err != nil {
			return nil, err
		}
		if d != 0 {
			return nil, decline(loc, d, declSubject(el))
		}
		b.children[el.Name()] = len(b.fields)
		b.fields = append(b.fields, f)
		locals = append(locals, el.Name().Local)
	}
	b.firstAttribute = len(b.fields)
	for _, u := range ct.AttributeUses() {
		f, d, subject, err := b.attributeUse(u)
		if err != nil {
			return nil, err
		}
		if d != 0 {
			return nil, decline(loc, d, subject)
		}
		b.attributes[u.DeclarationName()] = len(b.fields)
		b.fields = append(b.fields, f)
		locals = append(locals, u.DeclarationName().Local)
	}
	if _, has := ct.AttributeWildcard(); has {
		return nil, decline(loc, constructAttributeWildcard, typeSubject)
	}
	names := goname.NewNamer()
	for i, local := range locals {
		b.fields[i].name = names.Claim(local)
	}
	return b, nil
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

// contentParticles returns the particles of ct's content, an inline sequence
// occurring exactly once, or the construct Decode declines in its place.
// ContentType and the particle term are closed sums, so the switches are
// STYLE T2's sealed-sum exception.
func contentParticles(ct xsd.ComplexType) ([]xsd.Particle, construct) {
	var content xsd.ElementContent
	switch c := ct.ContentType().(type) {
	case xsd.EmptyContent:
		return nil, constructEmptyContent
	case xsd.SimpleContent:
		return nil, constructSimpleContent
	case xsd.ElementContent:
		content = c
	}
	if content.Mixed {
		return nil, constructMixedContent
	}
	if content.OpenContent != nil {
		return nil, constructOpenContent
	}
	if _, isRef := content.Particle.Term().(xsd.ModelGroupRef); isRef {
		return nil, constructGroupRefContent
	}
	resolved, _ := content.Particle.Term().(xsd.ResolvedTerm)
	group, _ := resolved.Term.(xsd.ModelGroup)
	switch group.Compositor() {
	case xsd.CompositorChoice:
		return nil, constructChoiceContent
	case xsd.CompositorAll:
		return nil, constructAllContent
	case xsd.CompositorSequence:
		occurs := content.Particle.Occurs()
		if upper, bounded := occurs.Max(); occurs.Min() != 1 || !bounded || upper != 1 {
			return nil, constructSequenceOccurs
		}
		return group.Particles(), 0
	}
	// The {particle} of element-only content is a model group (§3.4.2.3.3), so
	// the zero ModelGroup, no compositor, is a component no schema document
	// yields; decline it with the nearest construct rather than read it.
	return nil, constructNestedGroup
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

// localElement returns the field, its name unset, of the local element
// declaration el in a particle occurring as occurs, or the construct Decode
// declines it for, in the order Decode's doc states: its own properties, its
// type, and an expanded name an earlier particle of the sequence shares.
func (b *binding) localElement(el xsd.ElementDeclaration, occurs xsd.Occurs) (field, construct, error) {
	if d := declarationDecline(el); d != 0 {
		return field{}, d, nil
	}
	td, ok := b.schema.ResolvedType(el.TypeDefinition())
	if !ok {
		return field{}, 0, fmt.Errorf("codec: the {type definition} of %s does not resolve", declSubject(el))
	}
	st, ok := td.(*xsd.SimpleType)
	if !ok {
		return field{}, constructComplexElement, nil
	}
	d, err := simpleDecline(b.schema, st)
	if err != nil || d != 0 {
		return field{}, d, err
	}
	if _, shared := b.children[el.Name()]; shared {
		return field{}, constructSharedName, nil
	}
	s, err := occursShape(occurs, el)
	if err != nil {
		return field{}, 0, err
	}
	f := field{shape: s, st: st}
	if vc, has := el.ValueConstraint(); has {
		f.supplied, err = b.supply(st, vc, declSubject(el))
	}
	return f, 0, err
}

// attributeUse returns the field, its name unset, of attribute use u, or the
// construct Decode declines it for and the subject naming it.
func (b *binding) attributeUse(u xsd.AttributeUse) (field, construct, string, error) {
	decl, ok := b.schema.ResolvedAttributeDeclaration(u)
	if !ok {
		return field{}, 0, "", fmt.Errorf("codec: the {attribute declaration} of attribute use %s does not resolve", u.DeclarationName())
	}
	subject := fmt.Sprintf("attribute %s declared at %s", decl.Name(), decl.Loc())
	if decl.Name().Space == xsd.XMLSchemaInstanceNS {
		return field{}, constructXsiAttribute, subject, nil
	}
	st, ok := b.schema.ResolvedSimpleType(decl.TypeDefinition())
	if !ok {
		return field{}, 0, "", fmt.Errorf("codec: the {type definition} of %s does not resolve", subject)
	}
	d, err := simpleDecline(b.schema, st)
	if err != nil || d != 0 {
		return field{}, d, subject, err
	}
	f := field{shape: shapeOptional, st: st}
	if u.Required() {
		f.shape = shapeOne
	}
	if vc, has := b.schema.EffectiveValueConstraint(u); has {
		f.supplied, err = b.supply(st, vc, subject)
	}
	return f, 0, subject, err
}

// supply returns the ·schema normalized value· the post-schema-validation
// infoset supplies from the value constraint vc of the item subject names,
// whose simple type is st: vc's {lexical form} as [value.ApplyWhiteSpace]
// normalizes it under st (§3.3.5.4 sic-eltType clause 1.1, §3.4.5.1
// sic-attrDefault).
func (b *binding) supply(st *xsd.SimpleType, vc xsd.ValueConstraint, subject string) (*string, error) {
	v, err := value.ApplyWhiteSpace(b.schema, st, vc.LexicalForm())
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

// check returns nil when t is the struct codegen emits for b, and otherwise
// the ErrTypeMismatch error Decode's doc states, item 6.
func (b *binding) check(t reflect.Type) error {
	if t.Kind() != reflect.Struct {
		return fmt.Errorf("%w: %s is no struct", ErrTypeMismatch, t)
	}
	if t.NumField() != len(b.fields) {
		return fmt.Errorf("%w: %s has %d fields where codegen emits %d", ErrTypeMismatch, t, t.NumField(), len(b.fields))
	}
	for i, want := range b.fields {
		got := t.Field(i)
		if got.Name != want.name || got.Type != want.shape.goType() || got.Anonymous {
			return fmt.Errorf("%w: field %d of %s is %s %s where codegen emits %s %s", ErrTypeMismatch, i, t, got.Name, got.Type, want.name, want.shape.goType())
		}
	}
	return nil
}

// fill sets out's fields from the document element root, whose start tag r
// has just returned, and from r's tokens up to its end tag, or returns the
// xsi:type decline Decode's doc states, item 7. An attribute field with a
// supplied value holds it unless its attribute is present, as Decode's doc
// states.
func (b *binding) fill(r *xmltree.Reader, root *xmltree.StartElement, out reflect.Value) error {
	if err := checkXsiType(root); err != nil {
		return err
	}
	for i := b.firstAttribute; i < len(b.fields); i++ {
		if s := b.fields[i].supplied; s != nil {
			b.store(out, i, *s)
		}
	}
	for _, a := range root.Attributes() {
		i, ok := b.attributes[expanded(a.Name())]
		if ok {
			if err := b.set(out, i, a.Value()); err != nil {
				return err
			}
			continue
		}
		if a.Name().Space() != xsd.XMLSchemaInstanceNS {
			return fmt.Errorf("codec: %s: attribute %s is attributed to no attribute use", a.Loc(), expanded(a.Name()))
		}
	}
	for {
		tok, err := r.Token()
		if err != nil {
			return err
		}
		switch tok := tok.(type) {
		case *xmltree.EndElement:
			return nil
		case *xmltree.StartElement:
			if err := b.child(r, tok, out); err != nil {
				return err
			}
		}
	}
}

// child sets the field of the child element start, reading its character
// children from r up to its end tag: their ·normalized value·, or the
// field's supplied value when there are none, as Decode's doc states.
func (b *binding) child(r *xmltree.Reader, start *xmltree.StartElement, out reflect.Value) error {
	if err := checkXsiType(start); err != nil {
		return err
	}
	i, ok := b.children[expanded(start.Name())]
	if !ok {
		return fmt.Errorf("codec: %s: element %s is attributed to no particle", start.Loc(), expanded(start.Name()))
	}
	var text bytes.Buffer
	for {
		tok, err := r.Token()
		if err != nil {
			return err
		}
		switch tok := tok.(type) {
		case *xmltree.CharData:
			text.WriteString(tok.Data())
		case *xmltree.StartElement:
			return fmt.Errorf("codec: %s: element %s of simple type holds element %s", tok.Loc(), expanded(start.Name()), expanded(tok.Name()))
		case *xmltree.EndElement:
			if s := b.fields[i].supplied; s != nil && text.Len() == 0 {
				b.store(out, i, *s)
				return nil
			}
			return b.set(out, i, text.String())
		}
	}
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

// set stores the ·normalized value· of initial, under field i's simple type,
// in out's field i.
func (b *binding) set(out reflect.Value, i int, initial string) error {
	f := b.fields[i]
	v, err := value.ApplyWhiteSpace(b.schema, f.st, initial)
	if err != nil {
		return fmt.Errorf("codec: normalizing the value of field %s: %w", f.name, err)
	}
	b.store(out, i, v)
	return nil
}

// store stores v, a value already normalized, in out's field i by its shape:
// a repeated field appends it, and an optional one points at a copy of it.
func (b *binding) store(out reflect.Value, i int, v string) {
	dst := out.Field(i)
	switch b.fields[i].shape {
	case shapeOptional:
		dst.Set(reflect.ValueOf(&v))
	case shapeRepeated:
		dst.Set(reflect.Append(dst, reflect.ValueOf(v)))
	default:
		dst.SetString(v)
	}
}

// declSubject names the element declaration decl and its schema Loc.
func declSubject(decl xsd.ElementDeclaration) string {
	return fmt.Sprintf("element %s declared at %s", decl.Name(), decl.Loc())
}

// expanded returns the expanded name of an instance name.
func expanded(n xmltree.Name) xsd.QName {
	return xsd.QName{Space: n.Space(), Local: n.Local()}
}
