package codegen

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/kud360/goxsd8/internal/goname"
	"github.com/kud360/goxsd8/internal/goshape"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// The names of the components the slice reads by identity.
var (
	anyTypeName      = xsd.QName{Space: xsd.XMLSchemaNS, Local: "anyType"}
	errorTypeName    = xsd.QName{Space: xsd.XMLSchemaNS, Local: "error"}
	stringPrimitive  = xsd.QName{Space: xsd.XMLSchemaNS, Local: "string"}
	anyURIPrimitive  = xsd.QName{Space: xsd.XMLSchemaNS, Local: "anyURI"}
	booleanPrimitive = xsd.QName{Space: xsd.XMLSchemaNS, Local: "boolean"}
	floatPrimitive   = xsd.QName{Space: xsd.XMLSchemaNS, Local: "float"}
	doublePrimitive  = xsd.QName{Space: xsd.XMLSchemaNS, Local: "double"}
	decimalPrimitive = xsd.QName{Space: xsd.XMLSchemaNS, Local: "decimal"}

	hexBinaryPrimitive    = xsd.QName{Space: xsd.XMLSchemaNS, Local: "hexBinary"}
	base64BinaryPrimitive = xsd.QName{Space: xsd.XMLSchemaNS, Local: "base64Binary"}
	qNamePrimitive        = xsd.QName{Space: xsd.XMLSchemaNS, Local: "QName"}
	notationPrimitive     = xsd.QName{Space: xsd.XMLSchemaNS, Local: "NOTATION"}

	precisionDecimalPrimitive = xsd.QName{Space: xsd.XMLSchemaNS, Local: "precisionDecimal"}

	integerBuiltin = xsd.QName{Space: xsd.XMLSchemaNS, Local: "integer"}

	longBuiltin          = xsd.QName{Space: xsd.XMLSchemaNS, Local: "long"}
	intBuiltin           = xsd.QName{Space: xsd.XMLSchemaNS, Local: "int"}
	shortBuiltin         = xsd.QName{Space: xsd.XMLSchemaNS, Local: "short"}
	byteBuiltin          = xsd.QName{Space: xsd.XMLSchemaNS, Local: "byte"}
	unsignedLongBuiltin  = xsd.QName{Space: xsd.XMLSchemaNS, Local: "unsignedLong"}
	unsignedIntBuiltin   = xsd.QName{Space: xsd.XMLSchemaNS, Local: "unsignedInt"}
	unsignedShortBuiltin = xsd.QName{Space: xsd.XMLSchemaNS, Local: "unsignedShort"}
	unsignedByteBuiltin  = xsd.QName{Space: xsd.XMLSchemaNS, Local: "unsignedByte"}
)

// The math/big types "# Simple types" maps to; a field or named type of
// either makes the generated file import math/big (analysis.usesBig).
const (
	bigInt = "big.Int"
	bigRat = "big.Rat"
)

// datatypeStruct is the struct "# Simple types" maps one date, time or
// duration datatype, or QName and NOTATION, to (Datatypes §3.3.6–§3.3.14,
// §3.3.18, §3.3.19, §3.4.26–§3.4.28): one field per property its values carry,
// in the order of the seven-property model (§D.2.1), of duration's two
// properties (§3.3.6.1), or of QName's {namespace name, local part} (§3.3.18).
// doc is the header's line after its first, "" for none.
type datatypeStruct struct {
	datatype xsd.QName
	doc      string
	fields   []goField
}

// goField is one field of an emitted struct after its name is claimed: its Go
// name and its Go type. A datatypeStruct's field is named from its property,
// and claimFields names every other struct's.
type goField struct {
	name, goType string
}

// The header lines the datatype structs share.
const (
	optionalOffsetDoc = "TimezoneOffset is in minutes from UTC, nil when absent."
	requiredOffsetDoc = "TimezoneOffset is in minutes from UTC."
)

// The fields the datatype structs share: uint8 is the narrowest Go integer
// holding month 1–12, day 1–31, hour 0–23 and minute 0–59, int16 the narrowest
// holding a timezoneOffset of −840..840 (§D.2.1), and year, second, months and
// seconds are unbounded (§D.2.1, §3.3.6.1).
var (
	yearField           = goField{"Year", bigInt}
	monthField          = goField{"Month", "uint8"}
	dayField            = goField{"Day", "uint8"}
	hourField           = goField{"Hour", "uint8"}
	minuteField         = goField{"Minute", "uint8"}
	secondField         = goField{"Second", bigRat}
	optionalOffsetField = goField{"TimezoneOffset", "*int16"}
	requiredOffsetField = goField{"TimezoneOffset", "int16"}
	monthsField         = goField{"Months", bigInt}
	secondsField        = goField{"Seconds", bigRat}
)

// The thirteen datatype structs: one per date, time or duration primitive, one
// per builtin derived from one whose values carry fewer properties or a
// required one (§3.4.26 yearMonthDuration, §3.4.27 dayTimeDuration, §3.4.28
// dateTimeStamp), and one for QName, which NOTATION shares (§3.3.18,
// §3.3.19).
var (
	dateTimeStruct = datatypeStruct{xsd.QName{Space: xsd.XMLSchemaNS, Local: "dateTime"}, optionalOffsetDoc,
		[]goField{yearField, monthField, dayField, hourField, minuteField, secondField, optionalOffsetField}}
	dateTimeStampStruct = datatypeStruct{xsd.QName{Space: xsd.XMLSchemaNS, Local: "dateTimeStamp"}, requiredOffsetDoc,
		[]goField{yearField, monthField, dayField, hourField, minuteField, secondField, requiredOffsetField}}
	timeStruct = datatypeStruct{xsd.QName{Space: xsd.XMLSchemaNS, Local: "time"}, optionalOffsetDoc,
		[]goField{hourField, minuteField, secondField, optionalOffsetField}}
	dateStruct = datatypeStruct{xsd.QName{Space: xsd.XMLSchemaNS, Local: "date"}, optionalOffsetDoc,
		[]goField{yearField, monthField, dayField, optionalOffsetField}}
	gYearMonthStruct = datatypeStruct{xsd.QName{Space: xsd.XMLSchemaNS, Local: "gYearMonth"}, optionalOffsetDoc,
		[]goField{yearField, monthField, optionalOffsetField}}
	gYearStruct = datatypeStruct{xsd.QName{Space: xsd.XMLSchemaNS, Local: "gYear"}, optionalOffsetDoc,
		[]goField{yearField, optionalOffsetField}}
	gMonthDayStruct = datatypeStruct{xsd.QName{Space: xsd.XMLSchemaNS, Local: "gMonthDay"}, optionalOffsetDoc,
		[]goField{monthField, dayField, optionalOffsetField}}
	gDayStruct = datatypeStruct{xsd.QName{Space: xsd.XMLSchemaNS, Local: "gDay"}, optionalOffsetDoc,
		[]goField{dayField, optionalOffsetField}}
	gMonthStruct = datatypeStruct{xsd.QName{Space: xsd.XMLSchemaNS, Local: "gMonth"}, optionalOffsetDoc,
		[]goField{monthField, optionalOffsetField}}
	durationStruct = datatypeStruct{xsd.QName{Space: xsd.XMLSchemaNS, Local: "duration"}, "Months and Seconds share one sign.",
		[]goField{monthsField, secondsField}}
	dayTimeDurationStruct = datatypeStruct{xsd.QName{Space: xsd.XMLSchemaNS, Local: "dayTimeDuration"}, "",
		[]goField{secondsField}}
	yearMonthDurationStruct = datatypeStruct{xsd.QName{Space: xsd.XMLSchemaNS, Local: "yearMonthDuration"}, "",
		[]goField{monthsField}}
	qNameStruct = datatypeStruct{qNamePrimitive, "Space is the namespace name, \"\" for none.",
		[]goField{{"Space", "string"}, {"Local", "string"}}}
)

// emitter builds one target's declarations. decls holds each emitted type
// declaration in emission order: a top-level element's type, then the types
// its particles reach that no earlier element emitted — anonymous complex
// types nested in its type definition or in a model group definition a
// <group ref> particle names, and named complex types of local elements —
// depth first in particle order; the type for one occurrence of a model
// group particle (occurrenceType) is one such type, reached at its particle's
// place and emitted before the types its own particles reach, and a sealed
// interface's branch structs, a choice's or a flattened group's (flattened),
// follow it, each before the types its alternative or basic particle reaches.
// The sealed interface over a substitution group (substitutionType) is one
// such type too, reached at the first <element ref> particle to its head, its
// branch structs following it; so is the sealed interface over a local element
// declaration's governing types (localTable), each of its branch structs
// before the types its field reaches, as a top-level declaration's is at its
// own position (tableType).
type emitter struct {
	schema *xsd.Schema
	// types claims every type name of the package, by the walk emit runs it
	// for (surveyNamer or replayNamer).
	types typeNamer
	decls []declaration
	// emitted holds the type emitted for each named complex type a local
	// element reaches, so that every later local element of that type shares
	// it, and for each anonymous complex type nested in a named complex type
	// or in a model group definition, and each type for one occurrence of a
	// model group particle in either (occurrenceType), and each sealed
	// interface over the governing types of a local element declaration in
	// either (localTable), so that every element whose walk reaches that named
	// type, and every <group ref> particle to that group, shares it; it is a
	// lookup index only and never iterated (STYLE D2). scopedType,
	// occurrenceType and localTable store an entry before walking its type's
	// particles or branches, so a type that reaches itself reads it, and remove
	// it when that walk fails, so that it holds only types emitted whole or
	// still being walked. A top-level element's type is named from the element
	// and is never stored, nor is a type nested in an anonymous top-level type
	// outside a model group definition (scope.shared), and a branch struct is
	// never stored: it rides with its interface.
	emitted map[nestedKey]emittedType
	// elements holds the type emitted for each top-level element declaration,
	// keyed by its expanded name and stored by elementType when the walk first
	// reaches the declaration, at its own topLevel visit or at an <element ref>
	// particle to it, so that a ref field reads its name and whether it is a
	// sealed interface; it is a lookup index only and never iterated (STYLE
	// D2).
	elements map[xsd.QName]emittedType
	// acyclic holds the chain nodes whose value fields valueCycle followed to
	// the end with the node in its chain (valueChain), so that no value cycle
	// passes through them and valueCycle never walks one again; it is a lookup
	// index only and never iterated (STYLE D2), kept for a measured hot path:
	// without it a schema whose named types each reach the next twice costs
	// double per level (#3037).
	acyclic map[valueNode]struct{}
	// datatypes holds the name claimed for each datatype struct emitted, so
	// that each is emitted once per package; it is a lookup index only and
	// never iterated (STYLE D2). tailDecls holds those structs' declarations,
	// PrecisionDecimal's group (precisionDecimalType), the raw-content
	// structs' (anyTypeType), MixedChild's (mixedChildType),
	// Nillable's and its arms' (nillableTypes), each attributes struct's
	// (attributesType), and each union's interface and variants (unionType),
	// in first-use order, emitted after decls.
	datatypes map[xsd.QName]string
	tailDecls []declaration
	// unions holds the name of the sealed interface emitted for each union
	// simple type, keyed by component identity, so that each is emitted once
	// per package whatever reaches it; unionType stores an entry only once
	// every basic member has its variant. It is a lookup index only and never
	// iterated (STYLE D2).
	unions map[*xsd.SimpleType]string
	// substitutions holds the name of the sealed interface emitted for each
	// substitution-group head an <element ref> particle names, keyed by the
	// head's expanded name, so that each is emitted once per package;
	// substitutionType stores an entry only once every branch has its type. It
	// is a lookup index only and never iterated (STYLE D2).
	substitutions map[xsd.QName]string
	// raw holds the names claimed for the four raw-content structs, which
	// anyTypeType emits once per package; its zero value means they are
	// unclaimed and unemitted.
	raw rawNames
	// mixedChild holds the name claimed for the generic struct MixedChild,
	// which mixedChildType emits once per package; "" means it is unclaimed
	// and unemitted.
	mixedChild string
	// precisionDecimal holds the names claimed for the struct PrecisionDecimal,
	// its sealed interface and that interface's four arms, which
	// precisionDecimalType emits once per package; its zero value means they
	// are unclaimed and unemitted.
	precisionDecimal precisionDecimalNames
	// nillable holds the names claimed for the generic sealed interface
	// Nillable and its two arms, which nillableTypes emits once per package;
	// its zero value means they are unclaimed and unemitted.
	nillable nillableNames
	// attributes holds, keyed by the Go name of each struct body emits with an
	// attribute or {attribute wildcard} field, those fields as that struct
	// names them, so that the attributes struct of a nilled element of that
	// type holds the same (attributesType); it is a lookup index only and never
	// iterated (STYLE D2).
	attributes map[string]structAttributes
	// nilled holds each attributes struct attributesType claimed, in claim
	// order, each with its slot in tailDecls, which emit fills from attributes
	// once every turn is walked.
	nilled []nilledAttributes
}

// nillableNames is the names claimed for the generic sealed interface
// Nillable and its two arms, Nilled and Content, which
// "# Element declarations" gives a field over a nillable element
// declaration, in claim order.
type nillableNames struct {
	sum, nilled, content string
}

// precisionDecimalNames is the names claimed for the struct "# Simple types"
// gives xs:precisionDecimal (precisionDecimal §3.1 sec-vs-pD), the sealed
// interface its one field holds, and that interface's four arms, one per case
// of ·numericalValue· (vp-pd-numVal), in claim order.
type precisionDecimalNames struct {
	wrapper, value, numerical, positiveInfinity, negativeInfinity, notANumber string
}

// structAttributes is what body records for one struct whose type has
// attribute fields (hasAttributeFields): from, its header's rendering
// of what it is generated from, and fields, its attribute fields and then its
// wildcard field, named as the struct names them.
type structAttributes struct {
	from   provenance
	fields []goField
}

// nilledAttributes is one attributes struct attributesType claimed: its name,
// the Go name of the struct whose attribute fields it holds, and the index in
// e.tailDecls its declaration fills.
type nilledAttributes struct {
	name, of string
	slot     int
}

// rawNames is the names claimed for the four raw-content structs the package
// doc's "# xs:anyType", "# Particles" and "# Attributes" give, in that order,
// an element of type xs:anyType (§3.4.7 any-type-itself), a wildcard particle
// of a model group, and an {attribute wildcard} (§3.4.2.5 dcl.ctd.anyatt), in
// claim order.
type rawNames struct {
	anyType, attribute, child, element string
}

// isMixed reports whether the complex type t's {content type} has {variety}
// mixed (§3.4.1): element content whose effective mixed value (§3.4.2.3.3
// dcl.ctd.ctcc.common clause 1) is true, read from t's resolved {content type}
// alone, never its base's, so that an extension's (clause 4.2) and a
// restriction's are each its own.
func isMixed(t xsd.ComplexType) bool {
	content, isElement := t.ContentType().(xsd.ElementContent)
	return isElement && content.Mixed
}

// isAnyType reports whether td is xs:anyType itself, by identity: every other
// type definition is derived from it (§2.2.1.1 key-derived), whatever its
// shape, and so is never xs:anyType.
func isAnyType(td xsd.TypeDefinition) bool { return td.Name() == anyTypeName }

// emittedType is the Go type emitted for one complex type definition or
// top-level element declaration, or the Go type goType maps a simple type to:
// its name, and whether it is a sealed interface, a choice's or a union's,
// which a field holds bare rather than through a pointer.
type emittedType struct {
	name   string
	sealed bool
}

// scope is where the structs nested in one struct take their names from: the
// path of steps from their nearest named ancestor, or from the named complex
// type or model group definition that declares their element (nestedType),
// that ancestor's first, and the kind of component that ancestor is
// (scopeRoot). A struct is named from the path's local names and keyed by its
// root and its steps (nestedKey).
type scope struct {
	root scopeRoot
	path []pathStep
}

// pathStep is one step of a scope's path: the expanded name of the path's
// root or of an element declaration, slot being "", or a branch of the sealed
// interface tableType emits over the governing types of the element
// declaration the step before names, slot being the branch's (tableBranch.slot)
// and name, in no namespace, the branch's local name (tableBranch.local)
// followed by Type.
type pathStep struct {
	name xsd.QName
	slot string
}

// elementPath returns a path of element steps, one per name in order.
func elementPath(names ...xsd.QName) []pathStep {
	path := make([]pathStep, len(names))
	for i, name := range names {
		path[i] = pathStep{name: name}
	}
	return path
}

// scopeRoot is the kind of component a scope's path starts at: a top-level
// element declaration whose type is anonymous, a named complex type
// definition, or a model group definition. The three are separate symbol
// spaces that may share an expanded name (§3.3.2, §3.17.1). The zero value is
// invalid (STYLE T1).
type scopeRoot uint8

// The scopeRoot values.
const (
	elementRoot scopeRoot = iota + 1
	typeRoot
	groupRoot
)

// shared reports whether a type emitted under s is emitted once and shared by
// every walk that reaches s's path (e.emitted): under a named complex type,
// whichever elements have that type, and under a model group definition,
// whichever particles reference it, each element declared in the group being
// one component (§3.7.2). A top-level element's anonymous type is reached from
// that element alone.
func (s scope) shared() bool { return s.root != elementRoot }

// steps returns the path's steps as the namer reads them: each step's local
// name, a {type table} branch's step generated and every other declared.
func (s scope) steps() []nameStep {
	steps := make([]nameStep, len(s.path))
	for i, step := range s.path {
		steps[i] = declaredStep(step.name.Local)
		if step.slot != "" {
			steps[i] = generatedSteps(step.name.Local)[0]
		}
	}
	return steps
}

// nestedKey identifies a named complex type by its expanded name, and an
// anonymous complex type nested in one or in a model group definition by the
// steps along its path: the named type's or group's expanded name, then each
// element's {target namespace} and {name} (§3.3.2.3), or each {type table}
// branch's slot and local name after a "|", which no Go-quoted string starts
// with, each Go-quoted so that the concatenation is unambiguous and a named
// type's one-name key differs from every nested type's, all after the path's
// scopeRoot in decimal, so that a type and a group of one expanded name never
// share a key; and the type for one occurrence of a model group particle in a
// struct of either by that key and one quoted string more, its groupSite's
// ordinals (groupKey). The key of an element's type leaves out every group
// step between the element and its path's last element: two element
// declarations of one expanded name in one content model's particle tree,
// nested groups and referenced ones included, have one {type definition}
// (§3.8.6.3 cos-element-consistent clause 1, checked at Finalize), so two
// such declarations with anonymous types never share a path, and an
// occurrence type is keyed by its position alone, never by its structure, so
// two sibling groups never share one even when they are identical.
type nestedKey string

// key returns the nestedKey of the struct whose path is s's.
func (s scope) key() nestedKey {
	var b strings.Builder
	b.WriteString(strconv.Itoa(int(s.root)))
	for _, step := range s.path {
		if step.slot != "" {
			b.WriteString("|")
			b.WriteString(strconv.Quote(step.slot))
			b.WriteString(strconv.Quote(step.name.Local))
			continue
		}
		b.WriteString(strconv.Quote(step.name.Space))
		b.WriteString(strconv.Quote(step.name.Local))
	}
	return nestedKey(b.String())
}

// place names, in declines, the declaring place of an anonymous complex type
// whose struct's path is s's: the path innermost first, each local element
// declaration as "element N", each {type table} branch by its slot, the path's
// root by its kind (scopeRoot) as "element N", "complex type N" or "model
// group N", joined by " of ", so that anonymous types sharing a struct
// (nestedKey) share a place.
func (s scope) place() string {
	parts := make([]string, 0, len(s.path))
	for i := len(s.path) - 1; i > 0; i-- {
		if s.path[i].slot != "" {
			parts = append(parts, s.path[i].slot)
			continue
		}
		parts = append(parts, "element "+s.path[i].name.String())
	}
	return strings.Join(append(parts, s.root.noun()+" "+s.path[0].name.String()), " of ")
}

// noun names the kind of component a scope's path starts at in declines
// (scope.place).
func (r scopeRoot) noun() string {
	switch r {
	case elementRoot:
		return "element"
	case typeRoot:
		return "complex type"
	case groupRoot:
		return "model group"
	}
	return fmt.Sprintf("scopeRoot(%d)", r)
}

// groupSite is a position in the walk of one struct's content model: the
// struct's body, an occurrence type (occurrenceType), or a sealed interface's
// branch. ordinals holds one step per occurrence type or branch from the body
// to the position, the position's own last, and words the compositor,
// "sequence", "choice" or "all", of each occurrence type among them, in the same
// order. The zero value is the body, or a sealed interface emitted for a
// type's content particle; group and branch return every other, each a copy
// that never aliases its receiver.
//
// The steps are these. The content particle contentFields gives groupField's
// one field for is ordinal 0 of the body. Under any other struct, the leaves spliced returns
// whose term is a model group are numbered from 1 in particle order, a spliced sequence or
// all group being no leaf and taking no ordinal. Under a sealed interface each alternative
// k, from 1 in {particles} order, is a branch, and the group leaves inside it are numbered
// from 1 again.
type groupSite struct {
	ordinals []int
	words    []string
}

// group returns the site of the model group particle of compositor c at
// ordinal under g.
func (g groupSite) group(ordinal int, c xsd.Compositor) groupSite {
	return groupSite{ordinals: append(slices.Clone(g.ordinals), ordinal), words: append(slices.Clone(g.words), c.String())}
}

// branch returns the site of the branch for alternative index of the sealed
// interface at g.
func (g groupSite) branch(index int) groupSite {
	return groupSite{ordinals: append(slices.Clone(g.ordinals), index), words: slices.Clone(g.words)}
}

// groupKey returns the nestedKey of the type for one occurrence of the model
// group particle at site in the struct whose path is s's (occurrenceType): s's
// key followed by one Go-quoted string, site's ordinals in decimal joined by
// ".". A path key holds an even number of quoted strings, so the two never
// meet.
func (s scope) groupKey(site groupSite) nestedKey {
	steps := make([]string, len(site.ordinals))
	for i, ordinal := range site.ordinals {
		steps[i] = strconv.Itoa(ordinal)
	}
	return s.key() + nestedKey(strconv.Quote(strings.Join(steps, ".")))
}

// emit returns the declarations of schema's package in emission order, decls
// and then tailDecls (emitter), which a unit renders (newUnit), or the
// declines and the failure that is not a decline that Generate's doc states
// for one target's walk, one turn per top-level element declaration. It
// returns declarations only when it collects no decline, and err carries no
// decline.
//
// It walks schema twice, since a type's name is a function of every name the
// package claims ("# Naming"): the first walk records each type claim
// (surveyNamer) and its declines or failure are emit's, and only when it
// collects none does resolve name every claim and a second walk build the
// declarations with those names (replayNamer).
func emit(schema *xsd.Schema) (decls []declaration, declines []*DeclineError, err error) {
	survey := newSurveyNamer()
	if _, declines, err := walk(schema, survey); err != nil || len(declines) > 0 {
		return nil, declines, err
	}
	if survey.err != nil {
		return nil, nil, survey.err
	}
	replay := &replayNamer{claims: survey.claims, names: resolve(survey.claims)}
	e, declines, err := walk(schema, replay)
	if err != nil {
		return nil, nil, err
	}
	if replay.err != nil {
		return nil, nil, replay.err
	}
	if len(declines) > 0 || replay.next != len(replay.claims) {
		return nil, nil, fmt.Errorf("the second walk made %d type claims and %d declines, the first %d and none", replay.next, len(declines), len(replay.claims))
	}
	for _, n := range e.nilled {
		of, ok := e.attributes[n.of]
		if !ok {
			return nil, nil, fmt.Errorf("the attributes struct %s: no struct %s with attribute fields was emitted", n.name, n.of)
		}
		e.tailDecls[n.slot] = declare(n.name, of.from,
			fmt.Sprintf("// It holds the attribute fields of %s, which a nilled element keeps.\n", n.of),
			renderStruct(n.name, of.fields))
	}
	return slices.Concat(e.decls, e.tailDecls), nil, nil
}

// walk runs one turn per top-level element declaration of schema, claiming
// type names in types, and returns the emitter it built, the declines it
// collected in turn order, each once, and the first failure that is not a
// decline, which ends the walk.
func walk(schema *xsd.Schema, types typeNamer) (*emitter, []*DeclineError, error) {
	e := &emitter{schema: schema, types: types, emitted: map[nestedKey]emittedType{}, elements: map[xsd.QName]emittedType{}, acyclic: map[valueNode]struct{}{}, datatypes: map[xsd.QName]string{}, unions: map[*xsd.SimpleType]string{}, substitutions: map[xsd.QName]string{}, attributes: map[string]structAttributes{}}
	var declines []*DeclineError
	for _, decl := range schema.Elements() {
		err := e.topLevel(decl)
		if err == nil {
			continue
		}
		var de *DeclineError
		if !errors.As(err, &de) {
			return nil, declines, err
		}
		if !slices.ContainsFunc(declines, func(d *DeclineError) bool { return *d == *de }) {
			declines = append(declines, de)
		}
	}
	return e, declines, nil
}

// declaration is one emitted type declaration: the name it declares, its doc
// comment, and code, the declaration and any method on its type. The comment
// is before, then, when loc is non-nil, the one schema Loc its header renders
// and after; the Loc is rendered only when the source is (text), so that one
// declaration renders against any target's Dir. reaches is the type names its
// package must declare with it that its code does not name: Nillable for the
// type of a nillable top-level declaration, whose nilled value it is (topLevel).
type declaration struct {
	name          string
	before, after string
	loc           *xsderr.Loc
	code          string
	reaches       []string
}

// comment returns d's doc comment, rendering its Loc, when it has one, with
// renderLoc.
func (d declaration) comment(renderLoc func(xsderr.Loc) string) string {
	if d.loc == nil {
		return d.before
	}
	return d.before + renderLoc(*d.loc) + d.after
}

// text returns d's source, its Loc rendered with renderLoc.
func (d declaration) text(renderLoc func(xsderr.Loc) string) string {
	return d.comment(renderLoc) + d.code
}

// provenance is what a header renders a declaration as generated from: text,
// then, when loc is non-nil, " declared at " and that Loc.
type provenance struct {
	text string
	loc  *xsderr.Loc
}

// prefixed returns p with prefix before its text.
func (p provenance) prefixed(prefix string) provenance {
	return provenance{text: prefix + p.text, loc: p.loc}
}

// declare returns the declaration of the type name, generated from p, whose
// header is followed by the comment lines doc, each ending in a newline, and
// then by code.
func declare(name string, p provenance, doc, code string) declaration {
	d := declaration{name: name, after: ".\n" + doc, loc: p.loc, code: code}
	if p.loc == nil {
		d.before = fmt.Sprintf("// %s is generated from %s.\n%s", name, p.text, doc)
		d.after = ""
		return d
	}
	d.before = fmt.Sprintf("// %s is generated from %s declared at ", name, p.text)
	return d
}

// located returns a provenance of text declared at loc.
func located(text string, loc xsderr.Loc) provenance {
	return provenance{text: text, loc: &loc}
}

// elementFrom renders an element declaration for a header: its expanded name,
// whose namespace is its {target namespace} (for a local declaration, as form
// and elementFormDefault set it, §3.3.2.3), and its Loc, which headerLoc
// renders against the target's Dir. Only a header renders a Loc so: every
// decline and error names the Loc as the declaration carries it.
func (e *emitter) elementFrom(decl xsd.ElementDeclaration) provenance {
	return located("element "+decl.Name().String(), decl.Loc())
}

// origin is what one complex type's emitted declarations are generated from:
// from renders it for their headers, and subject names the complex type in
// declines.
type origin struct {
	from    provenance
	subject string
}

// elementOrigin is the origin of the type emitted for the top-level element
// declaration decl's complex {type definition} t and named from decl.
func (e *emitter) elementOrigin(decl xsd.ElementDeclaration, t xsd.ComplexType) origin {
	return origin{from: e.elementFrom(decl), subject: "complex type " + typeName(t) + " of element " + decl.Name().String()}
}

// typeOrigin is the origin of the type emitted for the named complex type t
// that a local element reaches and named from t: t's expanded name and its
// Loc, rendered as elementFrom renders an element's.
func (e *emitter) typeOrigin(t xsd.ComplexType) origin {
	return origin{
		from:    located("complex type "+t.Name().String(), t.Loc()),
		subject: "complex type " + typeName(t),
	}
}

// headerLoc renders loc for a header against the absolute directory base, by
// the rule the package doc's "Naming" states for a target's Dir.
func headerLoc(base string, loc xsderr.Loc) string {
	if !filepath.IsAbs(loc.URI) {
		return loc.String()
	}
	rel, err := filepath.Rel(base, loc.URI)
	if err != nil {
		return loc.String()
	}
	loc.URI = filepath.ToSlash(rel)
	return loc.String()
}

// topLevel emits the type of one top-level element declaration: a named type
// over the raw-content struct AnyType ("# xs:anyType") for xs:anyType, over
// string for a special one, over an atomic Go type for an atomic simple {type
// definition}, over a slice of one for a list, over a union's sealed interface
// for a union (unionType), a struct or sealed interface for an accepted
// complex one, named or anonymous, or the sealed interface tableType emits
// over its governing types when governingTypes answers a sum. A
// nillable declaration (nillable) gets that same type: after it, the visit
// claims, when nothing earlier did, the names nillableType claims for a field
// over decl, Nillable over that type being a nilled root's value, and records
// Nillable in the reaches of that type's declaration; a sum's
// branches wrap their own fields instead (tableType). That type is named from
// the element (elementType), and a local element of the same named type never
// shares it (nestedType), while every <element ref> particle to the element
// does; the types nested in it are named from the type when it is named, and
// shared with that type's every other use, and from the element when it is
// anonymous, but for those of the elements a model group definition declares,
// named from the group and shared with its every <group ref> particle
// (nestedType). A declaration with {abstract} true emits nothing: no element is
// valid against it (cvc-elt clause 2).
func (e *emitter) topLevel(decl xsd.ElementDeclaration) error {
	if decl.Abstract() {
		return nil
	}
	td, err := e.declaredType(decl)
	if err != nil {
		return err
	}
	branches, sum, err := e.governingTypes(decl)
	if err != nil {
		return err
	}
	typ, err := e.elementType(decl, td)
	if err != nil {
		return err
	}
	if sum {
		return e.tableType(typ, scope{root: elementRoot, path: elementPath(decl.Name())}, decl, branches)
	}
	if isAnyType(td) {
		e.decls = append(e.decls, declare(typ.name, e.elementFrom(decl), "", fmt.Sprintf("type %s %s\n", typ.name, e.raw.anyType)))
		return nil
	}
	switch t := td.(type) {
	case *xsd.SimpleType:
		underlying, err := e.simpleGoType("element "+decl.Name().String(), decl.Loc(), t, declaredUnion{declaredStep(decl.Name().Local)})
		if err != nil {
			return err
		}
		e.decls = append(e.decls, declare(typ.name, e.elementFrom(decl), "", fmt.Sprintf("type %s %s\n", typ.name, underlying.name)))
	case xsd.ComplexType:
		nested := structScope(scope{}, decl, t)
		chain, err := valueChain(nil, false, decl, t)
		if err != nil {
			return err
		}
		o := e.elementOrigin(decl, t)
		_, content, err := e.shape(typ.name, o, t)
		if err != nil {
			return err
		}
		if err := e.body(typ, nested, chain, o, t, content); err != nil {
			return err
		}
	default:
		return fmt.Errorf("element %s at %s: unexpected type definition %T", decl.Name(), decl.Loc(), td)
	}
	if nillable(decl) {
		e.nillableType(typ, td)
		i := slices.IndexFunc(e.decls, func(d declaration) bool { return d.name == typ.name })
		if i < 0 {
			return fmt.Errorf("element %s at %s: no declaration of its type %s", decl.Name(), decl.Loc(), typ.name)
		}
		e.decls[i].reaches = append(e.decls[i].reaches, e.nillable.sum)
	}
	return nil
}

// elementType returns the type emitted for the top-level element declaration
// decl, whose {type definition} declaredType resolved to td, claiming its name
// and checking its type when the walk first reaches decl: at its own topLevel
// visit, or at an <element ref> particle to it met earlier. For xs:anyType
// there is no check, and anyTypeType claims the raw-content structs' names
// before decl's is claimed ("# Naming"); for a simple td the check is
// simpleGoType's, run before the name is claimed so that a datatype struct or a
// union's interface and variants claim their names first, and the type it
// records is sealed exactly when td is a union, whose interface the named type
// is declared over; for a complex one it is shape's, run after. For a decl
// governingTypes answers a sum for, td is not read: the type is the sealed interface
// tableType emits, its name claimed first, and the check is shape's over each
// complex type other than xs:anyType among the branches, in branch order, run
// after; a simple branch type is checked, and its datatype structs and union
// interfaces claimed, only where tableType reaches its branch, after the
// branch's own name ("# Naming"). It emits no declaration of its own, only the
// datatype or raw-content structs, or the union interfaces and variants, it
// first needs: topLevel emits the type's declaration at decl's own visit, in
// document order, whichever reached decl first.
func (e *emitter) elementType(decl xsd.ElementDeclaration, td xsd.TypeDefinition) (emittedType, error) {
	if typ, ok := e.elements[decl.Name()]; ok {
		return typ, nil
	}
	branches, sum, err := e.governingTypes(decl)
	if err != nil {
		return emittedType{}, err
	}
	var typ emittedType
	st, isSimple := td.(*xsd.SimpleType)
	ct, isComplex := td.(xsd.ComplexType)
	switch {
	case sum:
		typ = emittedType{name: e.types.fixed(decl.Name().Local), sealed: true}
		ifc := scope{root: elementRoot, path: elementPath(decl.Name())}
		for _, b := range branches {
			bt, isComplex := b.td.(xsd.ComplexType)
			if !isComplex || isAnyType(b.td) {
				continue
			}
			if _, _, err := e.shape("", e.branchOrigin(branchScope(ifc, b), decl, b, bt), bt); err != nil {
				return emittedType{}, err
			}
		}
	case isAnyType(td):
		e.anyTypeType()
		typ = emittedType{name: e.types.fixed(decl.Name().Local)}
	case isSimple:
		underlying, err := e.simpleGoType("element "+decl.Name().String(), decl.Loc(), st, declaredUnion{declaredStep(decl.Name().Local)})
		if err != nil {
			return emittedType{}, err
		}
		typ = emittedType{name: e.types.fixed(decl.Name().Local), sealed: underlying.sealed}
	case isComplex:
		shaped, _, err := e.shape(e.types.fixed(decl.Name().Local), e.elementOrigin(decl, ct), ct)
		if err != nil {
			return emittedType{}, err
		}
		typ = shaped
	default:
		return emittedType{}, fmt.Errorf("element %s at %s: unexpected type definition %T", decl.Name(), decl.Loc(), td)
	}
	e.elements[decl.Name()] = typ
	return typ, nil
}

// declaredType returns the type definition the element declaration decl emits
// as: its one governing type when governingTypes answers no sum, and otherwise
// the resolved {type definition}, which decides nothing for a decl
// governingTypes answers a sum for: topLevel, elementType and field test sum
// ahead of td's cases, refType returns once elementType's type is sealed, and
// valueCycle skips such a decl before calling declaredType.
func (e *emitter) declaredType(decl xsd.ElementDeclaration) (xsd.TypeDefinition, error) {
	subject := "element " + decl.Name().String()
	branches, sum, err := e.governingTypes(decl)
	if err != nil {
		return nil, err
	}
	if !sum {
		return branches[0].td, nil
	}
	td, ok := e.schema.ResolvedType(decl.TypeDefinition())
	if !ok {
		return nil, fmt.Errorf("%s at %s: its {type definition} does not resolve", subject, decl.Loc())
	}
	return td, nil
}

// tableBranch is one governing type of an element declaration
// (governingTypes): td, resolved, and slot, the slot of the declaration it
// first occurs in, which holds td itself or, when td is a member of an abstract
// complex type (abstractMembers), that abstract type.
type tableBranch struct {
	slot branchSlot
	td   xsd.TypeDefinition
}

// local is the local name a branch is named from ("# Naming"): td's own when
// td is named, and otherwise its slot's (branchSlot.anonymous).
func (b tableBranch) local() string {
	if b.td.Name() != (xsd.QName{}) {
		return b.td.Name().Local
	}
	return b.slot.anonymous()
}

// branchSlot is the slot of an element declaration a governing type comes from
// (governingTypes): a {type table} alternative (alternativeSlot), its {default
// type definition} (defaultSlot), or, for a declaration with no {type table},
// its {type definition} (declaredSlot). §3.3.1 closes the set, so it is a
// sealed sum (STYLE T2's exception).
type branchSlot interface {
	// anonymous is the local name a branch of an anonymous type from the slot
	// is named from ("# Naming").
	anonymous() string
	// name is how declines and headers name the slot.
	name() string
}

// alternativeSlot is the {type table} alternative at this 1-based position in
// {alternatives}.
type alternativeSlot int

func (s alternativeSlot) anonymous() string { return "alternative" + strconv.Itoa(int(s)) }
func (s alternativeSlot) name() string      { return "{type table} alternative " + strconv.Itoa(int(s)) }

// defaultSlot is a {type table}'s {default type definition}.
type defaultSlot struct{}

func (defaultSlot) anonymous() string { return "default" }
func (defaultSlot) name() string      { return "{default type definition}" }

// declaredSlot is the {type definition} of a declaration with no {type table}.
// No branch is named from its anonymous name: an anonymous {type definition} is
// a local <complexType> or <simpleType>, never abstract (the schema for
// schemas' localComplexType prohibits the abstract attribute), so it is its
// declaration's one governing type, which is no sum.
type declaredSlot struct{}

func (declaredSlot) anonymous() string { return "typeDefinition" }
func (declaredSlot) name() string      { return "{type definition}" }

// governingTypes returns the types the package doc's "# Element declarations"
// lets govern an instance of the element declaration decl, in its order and by
// its identity, and sum, true exactly when "# Element declarations" gives decl
// a sealed interface over them. The slots are decl's {type table} alternatives
// in order then its {default type definition} (§3.3.4.1 key-selected-type,
// key-cta-select), or, with no {type table}, its {type definition} alone, each
// resolved through [xsd.Schema.ResolvedType]. A {type table} slot holding
// xs:error, matched by name (errorTypeName), gives nothing. A slot holding a
// complex type with {abstract} true gives its members (abstractMembers) in its
// place. Each type is kept at its first slot only, compared by name, so a
// {default type definition} that is a substitution-group head's type and an
// alternative naming that same type are one type, which xsd's
// typeDefinitionsEquivalent would call two, and two anonymous types are never
// one. sum is false for exactly one type, which declaredType returns in place
// of the {type definition}, and true for zero or two or more. A slot that does
// not resolve is an error, never a decline: finalize resolves every {type
// definition} slot (src-resolve); so is abstractMembers' error.
func (e *emitter) governingTypes(decl xsd.ElementDeclaration) (branches []tableBranch, sum bool, err error) {
	type slotRef struct {
		slot branchSlot
		ref  xsd.TypeDefinitionOrRef
	}
	slots := []slotRef{{declaredSlot{}, decl.TypeDefinition()}}
	table, hasTable := decl.TypeTable()
	if hasTable {
		slots = nil
		for i, alt := range table.Alternatives() {
			slots = append(slots, slotRef{alternativeSlot(i + 1), alt.TypeDefinition()})
		}
		slots = append(slots, slotRef{defaultSlot{}, table.DefaultTypeDefinition().TypeDefinition()})
	}
	for _, s := range slots {
		td, ok := e.schema.ResolvedType(s.ref)
		if !ok {
			return nil, false, fmt.Errorf("%s of element %s at %s: its {type definition} does not resolve", s.slot.name(), decl.Name(), decl.Loc())
		}
		if hasTable && td.Name() == errorTypeName {
			continue
		}
		types := []xsd.TypeDefinition{td}
		if ct, isComplex := td.(xsd.ComplexType); isComplex && ct.Abstract() {
			types, err = e.abstractMembers(decl, ct)
			if err != nil {
				return nil, false, err
			}
		}
		for _, td := range types {
			if td.Name() != (xsd.QName{}) && slices.ContainsFunc(branches, func(seen tableBranch) bool { return seen.td.Name() == td.Name() }) {
				continue
			}
			branches = append(branches, tableBranch{slot: s.slot, td: td})
		}
	}
	return branches, len(branches) != 1, nil
}

// abstractMembers returns the members "# Element declarations" gives the
// complex type t, which has {abstract} true, in a slot of the element
// declaration decl: each named complex type with {abstract} false among
// [xsd.Schema.Types], in its order, for which [xsd.Schema.ValidlySubstitutable]
// answers true against t under decl's {disallowed substitutions}, t's
// {prohibited substitutions} being the one it adds (§3.3.4.2 key-overrides
// clause 2). An error from ValidlySubstitutable is returned, never a decline.
func (e *emitter) abstractMembers(decl xsd.ElementDeclaration, t xsd.ComplexType) ([]xsd.TypeDefinition, error) {
	var members []xsd.TypeDefinition
	for _, td := range e.schema.Types() {
		s, isComplex := td.(xsd.ComplexType)
		if !isComplex || s.Name() == (xsd.QName{}) || s.Abstract() {
			continue
		}
		ok, err := e.schema.ValidlySubstitutable(s, t, decl.DisallowedSubstitutions())
		if err != nil {
			return nil, fmt.Errorf("element %s at %s: complex type %s as a member of abstract complex type %s: %w", decl.Name(), decl.Loc(), s.Name(), t.Name(), err)
		}
		if ok {
			members = append(members, s)
		}
	}
	return members, nil
}

// simpleGoType is a special type's string, the atomic Go type, the slice,
// or the union's sealed interface that a simple-typed element's or
// attribute's field, or an element's named type, is declared over, chosen
// by the package doc's "# Simple types" (goType). subject names the
// declaration the type is read from, and loc, its Loc, is a decline's;
// under is goType's.
func (e *emitter) simpleGoType(subject string, loc xsderr.Loc, t *xsd.SimpleType, under unionNaming) (emittedType, error) {
	return e.goType(subject, fmt.Sprintf("%s with {type definition} %s", subject, typeName(t)), loc, t, under)
}

// goType is the Go type "# Simple types" maps the simple type t to: for a
// special t (Datatypes §2.4.2 dt-special), xs:anySimpleType or xs:anyAtomicType
// known by identity ahead of its {variety}, string, holding the literal; for an
// atomic t, its atomic Go type, chosen through builtinType and then t's
// {primitive type definition} (Datatypes §4.1.1), a math/big type, a datatype
// struct holding one, or PrecisionDecimal among them; for a list (Datatypes
// §2.4.1.2, a finite sequence of its item type's values), a slice of the type
// its {item type definition} (§3.16.1) maps to, read through
// [xsd.SimpleType.Item], which takes a restriction's item from its base
// (§3.16.2.3), never by walking t to a builtin; for a union, the sealed
// interface unionType emits, the one answer that is sealed. typed names t in a
// decline, and a list's item, or a union's basic member, is named in it after
// the list or union, so an anonymous item or member still has a named context.
// under is what an anonymous union reached through t is named by ("# Naming");
// a list's item takes what listItemPath returns. subject and loc are
// simpleGoType's.
func (e *emitter) goType(subject, typed string, loc xsderr.Loc, t *xsd.SimpleType, under unionNaming) (emittedType, error) {
	if t.IsSpecial() {
		return emittedType{name: "string"}, nil
	}
	variety, err := t.Variety(e.schema)
	if err != nil {
		return emittedType{}, fmt.Errorf("%s at %s: %w", subject, loc, err)
	}
	switch variety.(type) {
	case xsd.Atomic:
	case xsd.List:
		item, err := t.Item(e.schema)
		if err != nil {
			return emittedType{}, fmt.Errorf("%s at %s: %w", subject, loc, err)
		}
		itemUnder, err := e.listItemPath(t, under)
		if err != nil {
			return emittedType{}, fmt.Errorf("%s at %s: %w", subject, loc, err)
		}
		elem, err := e.goType(subject, typed+", whose {item type definition} is "+typeName(item), loc, item, itemUnder)
		if err != nil {
			return emittedType{}, err
		}
		return emittedType{name: "[]" + elem.name}, nil
	case xsd.Union:
		return e.unionType(subject, typed, loc, t, under)
	default:
		return emittedType{}, fmt.Errorf("%s at %s: absent {variety}, which st-props-correct clause 1 (Structures §3.16.6.1) and the Datatypes §4.1.1 tableau give no simple type but xs:anySimpleType, a special type", typed, loc)
	}
	atomic, err := e.atomicType(subject, typed, loc, t)
	if err != nil {
		return emittedType{}, err
	}
	return emittedType{name: atomic}, nil
}

// listItemPath is what an anonymous union that is the {item type definition}
// of the list t is named by ("# Naming"): the item belongs to the list whose
// <list> declares it, the last type with {variety} list on t's {base type
// definition} chain (§3.16.1), t first, since a restriction of a list takes its
// base's item (§3.16.2.3). That list's local name alone, a declared step, when
// it is named, and under, the one t was handed, when it is anonymous. The walk stops
// at the first base whose {variety} is not list, xs:anySimpleType's absent one
// included, so it never asks xs:anySimpleType for its absent base.
func (e *emitter) listItemPath(t *xsd.SimpleType, under unionNaming) (unionNaming, error) {
	declaring := t
	for {
		base, err := declaring.Base(e.schema)
		if err != nil {
			return nil, err
		}
		variety, err := base.Variety(e.schema)
		if err != nil {
			return nil, err
		}
		if _, isList := variety.(xsd.List); !isList {
			break
		}
		declaring = base
	}
	if declaring.Name() == (xsd.QName{}) {
		return under, nil
	}
	return declaredUnion{declaredStep(declaring.Name().Local)}, nil
}

// atomicType is the atomic Go type goType maps the atomic simple type t to;
// its arguments are goType's.
func (e *emitter) atomicType(subject, typed string, loc xsderr.Loc, t *xsd.SimpleType) (string, error) {
	mapped, err := e.builtinType(t)
	if err != nil {
		return "", fmt.Errorf("%s at %s: %w", subject, loc, err)
	}
	if mapped != "" {
		return mapped, nil
	}
	primitive, err := t.Primitive(e.schema)
	if err != nil {
		return "", fmt.Errorf("%s at %s: %w", subject, loc, err)
	}
	if primitive == nil {
		return "", fmt.Errorf("%s at %s: atomic with no {primitive type definition}, which st-props-correct clause 1 (Structures §3.16.6.1) and the Datatypes §4.1.1 tableau give no atomic type but xs:anyAtomicType, a special type", typed, loc)
	}
	switch primitive.Name() {
	case stringPrimitive, anyURIPrimitive:
		return "string", nil
	case booleanPrimitive:
		return "bool", nil
	case floatPrimitive: // Datatypes §3.3.4.1, as "# Simple types" states
		return "float32", nil
	case doublePrimitive: // Datatypes §3.3.5.1, as "# Simple types" states
		return "float64", nil
	case decimalPrimitive: // Datatypes §3.3.3, as "# Simple types" states
		return bigRat, nil
	case hexBinaryPrimitive, base64BinaryPrimitive: // Datatypes §3.3.15.1, §3.3.16.1, as "# Simple types" states
		return "[]byte", nil
	case qNamePrimitive, notationPrimitive: // Datatypes §3.3.18, §3.3.19, as "# Simple types" states
		return e.datatypeType(qNameStruct), nil
	case dateTimeStruct.datatype: // Datatypes §3.3.7
		return e.datatypeType(dateTimeStruct), nil
	case timeStruct.datatype: // Datatypes §3.3.8
		return e.datatypeType(timeStruct), nil
	case dateStruct.datatype: // Datatypes §3.3.9
		return e.datatypeType(dateStruct), nil
	case gYearMonthStruct.datatype: // Datatypes §3.3.10
		return e.datatypeType(gYearMonthStruct), nil
	case gYearStruct.datatype: // Datatypes §3.3.11
		return e.datatypeType(gYearStruct), nil
	case gMonthDayStruct.datatype: // Datatypes §3.3.12
		return e.datatypeType(gMonthDayStruct), nil
	case gDayStruct.datatype: // Datatypes §3.3.13
		return e.datatypeType(gDayStruct), nil
	case gMonthStruct.datatype: // Datatypes §3.3.14
		return e.datatypeType(gMonthStruct), nil
	case durationStruct.datatype: // Datatypes §3.3.6
		return e.datatypeType(durationStruct), nil
	case precisionDecimalPrimitive: // precisionDecimal §3.1 sec-vs-pD, as "# Simple types" states
		return e.precisionDecimalType(), nil
	}
	return "", decline(loc, constructUncoveredPrimitive, typed, primitive.Name())
}

// unionType returns the sealed interface "# Simple types" gives the union simple
// type t (Datatypes §2.4.1.3), emitting it and its variants when no earlier
// declaration did: the interface's name is claimed first (unionName), then
// each variant's, the interface's name plus variantWord's, in goshape.BasicMembers' order,
// each before its member's Go type is computed with the variant's own name as what
// an anonymous union there is named by (variantUnion). The interface and its
// variants join e.tailDecls together at that first use, ahead of whatever their
// members' types first need. A basic member that declines declines t by the
// member's construct, typed naming the member after t, and t is stored in e.unions
// only once every member has its variant. A union with no basic member, which
// §3.16.1 admits though no schema document yields one
// (src-union-memberTypes-or-simpleTypes), gets its interface alone, which no
// variant implements. Facets on t are not represented. The arguments are goType's.
func (e *emitter) unionType(subject, typed string, loc xsderr.Loc, t *xsd.SimpleType, under unionNaming) (emittedType, error) {
	if name, ok := e.unions[t]; ok {
		return emittedType{name: name, sealed: true}, nil
	}
	members, err := goshape.BasicMembers(e.schema, t)
	if err != nil {
		return emittedType{}, fmt.Errorf("%s at %s: %w", subject, loc, err)
	}
	name := e.unionName(t, under)
	marker := "is" + name
	slot := len(e.tailDecls)
	e.tailDecls = append(e.tailDecls, make([]declaration, 1+len(members))...)
	e.tailDecls[slot] = declare(name, e.simpleFrom(t), "", fmt.Sprintf("type %s interface {\n\t%s()\n}\n", name, marker))
	for i, m := range members {
		word, err := e.variantWord(m)
		if err != nil {
			return emittedType{}, fmt.Errorf("%s at %s: %w", subject, loc, err)
		}
		variant := e.types.extend(name, word)
		under, err := e.goType(subject, typed+", whose basic member is "+typeName(m), loc, m, variantUnion(variant))
		if err != nil {
			return emittedType{}, err
		}
		e.tailDecls[slot+1+i] = declare(variant, e.simpleFrom(m), "", fmt.Sprintf("type %s %s\n\nfunc (%s) %s() {}\n", variant, under.name, variant, marker))
	}
	e.unions[t] = name
	return emittedType{name: name, sealed: true}, nil
}

// unionName claims the name of the sealed interface unionType emits for the
// union t: from t's local name when t is named, and by under when it is
// anonymous.
func (e *emitter) unionName(t *xsd.SimpleType, under unionNaming) string {
	if t.Name() != (xsd.QName{}) {
		return e.types.fixed(t.Name().Local)
	}
	return under.claimUnion(e.types)
}

// unionNaming is what an anonymous union reached through a simple type is
// named by ("# Naming"): a path of declared names (declaredUnion), or the
// name of the variant whose member it is (variantUnion).
type unionNaming interface {
	// claimUnion claims the union's interface name in types.
	claimUnion(types typeNamer) string
}

// declaredUnion is the path of names an anonymous complex type would take in
// the union's place; the union's name is that path with the generated step
// union last, claimed as a path.
type declaredUnion []nameStep

func (d declaredUnion) claimUnion(types typeNamer) string {
	return types.path(append(slices.Clone(d), generatedSteps("union")...))
}

// variantUnion is the name of the variant whose member's type the union is;
// the union's name extends it with union.
type variantUnion string

func (v variantUnion) claimUnion(types typeNamer) string {
	return types.extend(string(v), "union")
}

// variantWord is the local name a union's variant for its basic member m is
// named from after the interface's name ("# Naming"): m's own when m is named,
// list for an anonymous list, and otherwise that of the nearest named type on
// m's {base type definition} chain (§3.16.1). Every chain ends at the named
// xs:anySimpleType, so the walk always meets one.
func (e *emitter) variantWord(m *xsd.SimpleType) (string, error) {
	variety, err := m.Variety(e.schema)
	if err != nil {
		return "", err
	}
	if _, isList := variety.(xsd.List); isList && m.Name() == (xsd.QName{}) {
		return "list", nil
	}
	step := m
	for step.Name() == (xsd.QName{}) {
		base, err := step.Base(e.schema)
		if err != nil {
			return "", err
		}
		if base == nil {
			return "", fmt.Errorf("simple type (anonymous) at %s: its {base type definition} chain ends unnamed", m.Loc())
		}
		step = base
	}
	return step.Name().Local, nil
}

// simpleFrom renders the simple type t for a header: "simple type", its
// expanded name or (anonymous), and its Loc, as elementFrom renders an
// element's, which is omitted when t's Loc is zero, as a builtin's is.
func (e *emitter) simpleFrom(t *xsd.SimpleType) provenance {
	from := "simple type " + typeName(t)
	if t.Loc() == (xsderr.Loc{}) {
		return provenance{text: from}
	}
	return located(from, t.Loc())
}

// datatypeType returns the name of the struct ds, emitting it when no earlier
// declaration did: its name is claimed in e.types at that first use, by the
// package doc's "Naming", and its declaration joins e.tailDecls.
func (e *emitter) datatypeType(ds datatypeStruct) string {
	if name, ok := e.datatypes[ds.datatype]; ok {
		return name
	}
	name := e.types.fixed(ds.datatype.Local)
	e.datatypes[ds.datatype] = name
	doc := ""
	if ds.doc != "" {
		doc = "// " + ds.doc + "\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "type %s struct {\n", name)
	for _, f := range ds.fields {
		fmt.Fprintf(&b, "\t%s %s\n", f.name, f.goType)
	}
	b.WriteString("}\n")
	e.tailDecls = append(e.tailDecls, declare(name, provenance{text: "datatype " + ds.datatype.String()}, doc, b.String()))
	return name
}

// precisionDecimalType returns the name of the struct PrecisionDecimal, which
// "# Simple types" gives xs:precisionDecimal (precisionDecimal §3.1
// sec-vs-pD). When no earlier declaration needed them, it claims the six names
// in e.types, in precisionDecimalNames' order, by the package doc's "Naming",
// appends their declarations to e.tailDecls in that order.
func (e *emitter) precisionDecimalType() string {
	if e.precisionDecimal.wrapper != "" {
		return e.precisionDecimal.wrapper
	}
	local := precisionDecimalPrimitive.Local
	n := precisionDecimalNames{wrapper: e.types.fixed(local)}
	n.value = e.types.fixed(local, "value")
	n.numerical = e.types.extend(n.value, "numerical")
	n.positiveInfinity = e.types.extend(n.value, "positiveInfinity")
	n.negativeInfinity = e.types.extend(n.value, "negativeInfinity")
	n.notANumber = e.types.extend(n.value, "notANumber")
	e.precisionDecimal = n
	from := provenance{text: "datatype " + precisionDecimalPrimitive.String()}
	marker := "is" + n.value
	arm := func(name, doc, fields string) declaration {
		return declare(name, from, doc, fmt.Sprintf("type %s struct {%s}\n\nfunc (%s) %s() {}\n", name, fields, name, marker))
	}
	e.tailDecls = append(e.tailDecls,
		declare(n.wrapper, from, "", fmt.Sprintf("type %s struct {\n\tValue %s\n}\n", n.wrapper, n.value)),
		declare(n.value, from, "", fmt.Sprintf("type %s interface {\n\t%s()\n}\n", n.value, marker)),
		arm(n.numerical, "// Its value is Coefficient × 10^−Scale, negative when Negative; Coefficient is never negative.\n",
			fmt.Sprintf("\n\tCoefficient %s\n\tScale %s\n\tNegative bool\n", bigInt, bigInt)),
		arm(n.positiveInfinity, "", ""),
		arm(n.negativeInfinity, "", ""),
		arm(n.notANumber, "", ""))
	return n.wrapper
}

// anyTypeType returns the name of the raw-content struct AnyType, which holds
// the content of an element of type xs:anyType: mixed, a lax ##any wildcard
// any number of times, and a lax ##any {attribute wildcard} (§3.4.7
// any-type-itself). When no earlier declaration, wildcard particle or
// {attribute wildcard} needed them, it claims the four raw-content structs'
// names in e.types, in rawNames' order, by the package doc's "Naming", and
// appends their declarations to e.tailDecls in that order. No field names
// math/big.
func (e *emitter) anyTypeType() string {
	if e.raw.anyType != "" {
		return e.raw.anyType
	}
	n := rawNames{
		anyType:   e.types.fixed("anyType"),
		attribute: e.types.fixed("anyAttribute"),
		child:     e.types.fixed("anyChild"),
		element:   e.types.fixed("anyElement"),
	}
	e.raw = n
	from := provenance{text: "complex type " + anyTypeName.String()}
	e.tailDecls = append(e.tailDecls,
		declare(n.anyType, from,
			"// Text is the characters before the first of Children, \"\" for none.\n",
			fmt.Sprintf("type %s struct {\n\tAttributes []%s\n\tText string\n\tChildren []%s\n}\n", n.anyType, n.attribute, n.child)),
		declare(n.attribute, from, "",
			fmt.Sprintf("type %s struct {\n\tSpace, Local, Value string\n}\n", n.attribute)),
		declare(n.child, from,
			"// Tail is the characters after Element, before the next child or the end.\n",
			fmt.Sprintf("type %s struct {\n\tElement %s\n\tTail string\n}\n", n.child, n.element)),
		declare(n.element, from, "",
			fmt.Sprintf("type %s struct {\n\tSpace, Local string\n\tContent %s\n}\n", n.element, n.anyType)))
	return n.anyType
}

// mixedChildType returns the name of the generic struct MixedChild, which
// holds one child element of mixed content and the characters after it
// (§3.4.4.2 cvc-complex-type clause 1.4 validates the element [children]
// alone, and nothing checks the character data between them). When no
// earlier field needed it, it claims the name in e.types from the local name
// mixedChild, by the package doc's "Naming", and appends its declaration to
// e.tailDecls. No field names math/big.
func (e *emitter) mixedChildType() string {
	if e.mixedChild != "" {
		return e.mixedChild
	}
	e.mixedChild = e.types.fixed("mixedChild")
	e.tailDecls = append(e.tailDecls, declaration{
		name: e.mixedChild,
		before: fmt.Sprintf("// %s is generated for mixed content: one child element and the characters after it.\n", e.mixedChild) +
			"// Tail is the characters after Element, before the next child or the end.\n",
		code: fmt.Sprintf("type %s[T any] struct {\n\tElement T\n\tTail string\n}\n", e.mixedChild),
	})
	return e.mixedChild
}

// nillable reports whether a field over the element declaration decl holds
// the sum "# Element declarations" gives a nillable element: decl has
// {nillable} true (§3.3.1 ed-nillable) and no {value constraint} of {variety}
// fixed, which makes every nilled instance invalid (§3.3.4.3 cvc-elt clause
// 3.2.3.2), so that such a decl emits as one with {nillable} false. It is the
// one test every site that wraps a type in Nillable, or passes a field over
// decl as no struct value, reads: topLevel, field, substitutionType,
// tableType and valueCycle, and refType through field's value.
func nillable(decl xsd.ElementDeclaration) bool {
	if !decl.Nillable() {
		return false
	}
	vc, ok := decl.ValueConstraint()
	return !ok || vc.Kind() != xsd.ValueFixed
}

// nillableType returns the sealed interface Nillable over typ, the Go type a
// field over a nillable declaration whose type definition is td holds
// without the wrap, and A: the attributes struct attributesType names for a
// td that is a complex type with attribute fields (hasAttributeFields), and
// struct{} otherwise. It claims Nillable's names first (nillableTypes), then
// A's, each when nothing earlier did.
func (e *emitter) nillableType(typ emittedType, td xsd.TypeDefinition) emittedType {
	sum := e.nillableTypes()
	attributes := "struct{}"
	if ct, isComplex := td.(xsd.ComplexType); isComplex && hasAttributeFields(ct) {
		attributes = e.attributesType(typ.name)
	}
	return emittedType{name: sum + "[" + typ.name + ", " + attributes + "]", sealed: true}
}

// nillableTypes returns the name of the generic sealed interface Nillable,
// whose arms are Nilled, a nilled element holding its attributes (§3.3.4.3
// cvc-elt clause 3.2.3: no character or element children), and Content, an
// element that is not nilled holding its content. Its marker method takes
// both type parameters, so that an arm implements only the instantiation of
// its own type arguments. When no earlier field needed them, it claims the
// three names in e.types from the local names nillable, nilled and content,
// in that order, by the package doc's "Naming", and appends their
// declarations to e.tailDecls. No field names math/big.
func (e *emitter) nillableTypes() string {
	if e.nillable.sum != "" {
		return e.nillable.sum
	}
	n := nillableNames{sum: e.types.fixed("nillable"), nilled: e.types.fixed("nilled"), content: e.types.fixed("content")}
	e.nillable = n
	marker := "is" + n.sum
	e.tailDecls = append(e.tailDecls,
		declaration{
			name:   n.sum,
			before: fmt.Sprintf("// %s is generated for nillable elements: a nilled element or one with content.\n", n.sum),
			code:   fmt.Sprintf("type %s[T, A any] interface {\n\t%s(T, A)\n}\n", n.sum, marker),
		},
		declaration{
			name:   n.nilled,
			before: fmt.Sprintf("// %s is generated for nillable elements: a nilled element, with no character or element children, holding its attributes (cvc-elt 3.2.3).\n", n.nilled),
			code:   fmt.Sprintf("type %s[T, A any] struct {\n\tAttributes A\n}\n\nfunc (%s[T, A]) %s(T, A) {}\n", n.nilled, n.nilled, marker),
		},
		declaration{
			name:   n.content,
			before: fmt.Sprintf("// %s is generated for nillable elements: an element that is not nilled, holding its content.\n", n.content),
			code:   fmt.Sprintf("type %s[T, A any] struct {\n\tContent T\n}\n\nfunc (%s[T, A]) %s(T, A) {}\n", n.content, n.content, marker),
		})
	return n.sum
}

// attributesType returns the name of the attributes struct of a nilled
// element whose content is the struct of, which holds of's attribute fields
// and then its {attribute wildcard} field, named as of names them, and no
// other: when no earlier field or top-level visit needed it, it claims the
// name in e.types from of's name followed by the local name attributes, by the
// package doc's "Naming", and reserves its slot in e.tailDecls, which emit
// fills from what body records for of in e.attributes, since of's body may
// still be walking, or not yet reached, when the name is claimed. One struct
// is emitted per of, whatever reaches it.
func (e *emitter) attributesType(of string) string {
	if i := slices.IndexFunc(e.nilled, func(n nilledAttributes) bool { return n.of == of }); i >= 0 {
		return e.nilled[i].name
	}
	n := nilledAttributes{name: e.types.extend(of, "attributes"), of: of, slot: len(e.tailDecls)}
	e.tailDecls = append(e.tailDecls, declaration{})
	e.nilled = append(e.nilled, n)
	return n.name
}

// builtinType is the Go type "# Simple types" maps t to through the eight
// bounded builtin integer types (Datatypes §3.4.16–§3.4.19, §3.4.21–§3.4.24),
// xs:integer (§3.4.13), and the three builtins derived from a date, time or
// duration primitive that get a datatype struct of their own (§3.4.26–§3.4.28),
// walking t's {base type definition} chain (Structures §3.16.1) from t itself
// and stopping at the first of the twelve it meets, or "" when the walk ends
// at xs:anySimpleType's absent base without meeting one. Every bounded type
// lies below xs:integer on its chain, so the nearest bounded type wins over
// it; no chain meets both an integer builtin and a temporal one.
func (e *emitter) builtinType(t *xsd.SimpleType) (string, error) {
	for step := t; step != nil; {
		switch step.Name() {
		case integerBuiltin:
			return bigInt, nil
		case dateTimeStampStruct.datatype: // Datatypes §3.4.28
			return e.datatypeType(dateTimeStampStruct), nil
		case dayTimeDurationStruct.datatype: // Datatypes §3.4.27
			return e.datatypeType(dayTimeDurationStruct), nil
		case yearMonthDurationStruct.datatype: // Datatypes §3.4.26
			return e.datatypeType(yearMonthDurationStruct), nil
		case longBuiltin:
			return "int64", nil
		case intBuiltin:
			return "int32", nil
		case shortBuiltin:
			return "int16", nil
		case byteBuiltin:
			return "int8", nil
		case unsignedLongBuiltin:
			return "uint64", nil
		case unsignedIntBuiltin:
			return "uint32", nil
		case unsignedShortBuiltin:
			return "uint16", nil
		case unsignedByteBuiltin:
			return "uint8", nil
		}
		base, err := step.Base(e.schema)
		if err != nil {
			return "", err
		}
		step = base
	}
	return "", nil
}

// typeName names a type definition in a decline message.
func typeName(t xsd.TypeDefinition) string {
	if t.Name() == (xsd.QName{}) {
		return "(anonymous)"
	}
	return t.Name().String()
}

// shape checks the complex type t against the package doc's "# Complex types"
// shape, naming it in declines by o's subject, and returns the type it emits
// as name, sealed exactly when "# Complex types" gives the choice t's content
// particle holds as t itself — it reads t's mixedness (isMixed), then its
// attribute fields (hasAttributeFields) and its {open content}
// (hasOpenContent) from the finalized component, so inherited ones count, and
// then answers sealed for a choice with no {particles}, before asking
// termEmptiable about a choice with {particles} — and its content particle
// (contentGroup), nil for empty content. It emits nothing: body does, so that
// a caller can store the type in e.emitted before its body is walked.
func (e *emitter) shape(name string, o origin, t xsd.ComplexType) (emittedType, *groupParticle, error) {
	content, err := e.contentGroup(o.subject, t)
	if err != nil {
		return emittedType{}, nil, err
	}
	if content == nil || content.group.Compositor() != xsd.CompositorChoice || content.occurs != exactlyOnce || isMixed(t) {
		return emittedType{name: name}, content, nil
	}
	if hasAttributeFields(t) || hasOpenContent(t) {
		return emittedType{name: name}, content, nil
	}
	if len(content.group.Particles()) == 0 {
		return emittedType{name: name, sealed: true}, content, nil
	}
	emptiable, err := e.termEmptiable(o.subject, t.Loc(), xsd.ResolvedTerm{Term: content.group})
	if err != nil {
		return emittedType{}, nil, err
	}
	return emittedType{name: name, sealed: !emptiable}, content, nil
}

// hasAttributeFields reports whether the complex type t has an attribute use
// fieldAttributeUses keeps or an {attribute wildcard}, read from the finalized
// component, so inherited ones count: exactly when the struct body emits for t
// holds attribute fields, and so when shape never answers a sealed interface
// for t.
func hasAttributeFields(t xsd.ComplexType) bool {
	_, hasWildcard := t.AttributeWildcard()
	return hasWildcard || len(fieldAttributeUses(t)) > 0
}

// hasOpenContent reports whether the complex type t has an {open content},
// read from its finalized {content type} as the package doc's
// "# Open content" states: exactly when the struct body emits for t holds
// the open-content field, and so when shape never answers a sealed
// interface for t.
func hasOpenContent(t xsd.ComplexType) bool {
	content, isElement := t.ContentType().(xsd.ElementContent)
	return isElement && content.OpenContent != nil
}

// xsiNil is the expanded name of the built-in attribute declaration xsi:nil
// (§3.2.7.2).
var xsiNil = xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: "nil"}

// fieldAttributeUses returns t's {attribute uses} but a use of xsi:nil, the
// uses body gives a field each: a nillable element's nilled-ness is
// Nillable's arm (cvc-elt clause 3.2.3), and a declaration that is not
// nillable admits no xsi:nil (cvc-elt clause 3.1). It matches the use's
// {attribute declaration} by expanded name, which stands for the built-in's
// identity because no-xsi (§3.2.6.4) keeps a schema document out of the xsi
// namespace.
//
// GAP(xsd): a programmatically built schema is not kept out (#1462), so a
// declaration it builds named {xsi}nil matches too: body then emits no field
// for that use and hasAttributeFields does not count it, so the emitted type
// omits that attribute and nothing declines.
func fieldAttributeUses(t xsd.ComplexType) []xsd.AttributeUse {
	return slices.DeleteFunc(t.AttributeUses(), func(u xsd.AttributeUse) bool { return u.DeclarationName() == xsiNil })
}

// body emits typ, which shape returned for the complex type t with content,
// generated from o: a struct holding, for mixed content (isMixed), first the
// field named from the local name text holding a string, then the fields
// contentFields returns for content, or for simple content (§3.4.2.2
// dcl.ctd.ctsc) the value field, named from the local name value and holding,
// never through a pointer, what goType maps its {simple type definition} to,
// an anonymous union named by nested's path with value last, then, when t has
// an {open content} (hasOpenContent), one named from the local name
// openContent holding a slice of the raw-content struct AnyElement, or of
// AnyChild for mixed content (anyTypeType, which claims the raw-content
// structs' names there when nothing earlier did), then one per
// attribute use fieldAttributeUses keeps (attributeField), and then, when t
// has an {attribute wildcard} (§3.4.2.5 dcl.ctd.anyatt), its own, inherited or
// brought by an attribute group, one named from the local name anyAttribute
// holding a slice of the raw-content struct AnyAttribute (anyTypeType, which
// claims the raw-content structs' names there when nothing earlier did); or
// the sealed interface sealedType emits; then the types its fields reach, the
// anonymous ones named in nested. chain is valueChain's for a walk of t's
// particles. A struct with attribute or {attribute wildcard} fields records
// them in e.attributes under typ's name, as claimFields names them, for the
// attributes struct of a nilled element of typ (attributesType).
func (e *emitter) body(typ emittedType, nested scope, chain []valueNode, o origin, t xsd.ComplexType, content *groupParticle) error {
	if typ.sealed {
		return e.sealedType(typ.name, nested, groupSite{}, o, t, content.group.Particles())
	}
	slot := len(e.decls)
	e.decls = append(e.decls, declaration{})
	var fields []fieldSpec
	if simple, ok := t.ContentType().(xsd.SimpleContent); ok {
		goType, err := e.goType(o.subject, fmt.Sprintf("%s with {simple type definition} %s", o.subject, typeName(simple.SimpleType)), t.Loc(), simple.SimpleType, declaredUnion(append(nested.steps(), generatedSteps("value")...)))
		if err != nil {
			return err
		}
		fields = append(fields, fieldSpec{local: "value", goType: goType.name})
	}
	if isMixed(t) {
		fields = append(fields, fieldSpec{local: "text", goType: "string"})
	}
	if content != nil {
		contentFields, err := e.contentFields(nested, chain, o, t, *content)
		if err != nil {
			return err
		}
		fields = append(fields, contentFields...)
	}
	if hasOpenContent(t) {
		e.anyTypeType()
		item := e.raw.element
		if isMixed(t) {
			item = e.raw.child
		}
		fields = append(fields, fieldSpec{local: "openContent", goType: "[]" + item})
	}
	attributesFrom := len(fields)
	for _, u := range fieldAttributeUses(t) {
		field, err := e.attributeField(nested, o.subject, t.Loc(), u)
		if err != nil {
			return err
		}
		fields = append(fields, field)
	}
	if _, has := t.AttributeWildcard(); has {
		e.anyTypeType()
		fields = append(fields, fieldSpec{local: "anyAttribute", goType: "[]" + e.raw.attribute})
	}
	claimed := claimFields(fields)
	if attributesFrom < len(claimed) {
		e.attributes[typ.name] = structAttributes{from: o.from, fields: claimed[attributesFrom:]}
	}
	e.decls[slot] = declare(typ.name, o.from, "", renderStruct(typ.name, claimed))
	return nil
}

// contentFields returns the fields the content particle content of the
// complex type t generated from o gives in the struct body emits for t, whose
// nested types are named in nested and whose particles chain is valueChain's
// for: particleFields' for its group's particles when splices answers true,
// and otherwise the one field groupField returns.
func (e *emitter) contentFields(nested scope, chain []valueNode, o origin, t xsd.ComplexType, content groupParticle) ([]fieldSpec, error) {
	splice, err := e.splices(o.subject, t.Loc(), content.group, content.occurs)
	if err != nil {
		return nil, err
	}
	if splice {
		return e.particleFields(nested, groupSite{}, chain, o, t, content.group.Particles())
	}
	field, err := e.groupField(nested, groupSite{}, 0, o, t, content)
	if err != nil {
		return nil, err
	}
	return []fieldSpec{field}, nil
}

// groupField returns the field of the model-group particle c at ordinal under
// site (groupSite) of the complex type t generated from o, in a struct whose
// nested types are named in nested: named from c's compositor, "sequence",
// "choice" or "all". When flattens answers true for c, decided before any type
// of c's is built, the field holds a slice of the sealed interface
// occurrenceType emits flattened, whatever c's occurrence, a choice occurring
// exactly once included; otherwise it holds the type for one occurrence of c's
// group (occurrenceType) by c's occurrence (occursGoType). Its two callers are
// contentFields, for a content particle splices does not splice, and field,
// for a model group particle nested in a struct's or a branch's particles, so
// the walk tests a particle before any particle in its term, and a particle
// flattens answers true for is the outermost one: its term is the reach walk's
// (reach), and no particle inside it reaches groupField.
func (e *emitter) groupField(nested scope, site groupSite, ordinal int, o origin, t xsd.ComplexType, c groupParticle) (fieldSpec, error) {
	flat, err := e.flattens(o.subject, t.Loc(), c)
	if err != nil {
		return fieldSpec{}, err
	}
	typ, err := e.occurrenceType(nested, site, ordinal, o, t, c.group, flat)
	if err != nil {
		return fieldSpec{}, err
	}
	if flat {
		return fieldSpec{local: c.group.Compositor().String(), goType: "[]" + typ.name}, nil
	}
	goType, ok := occursGoType(typ, c.occurs)
	if !ok {
		return fieldSpec{}, fmt.Errorf("%s at %s: a %s model group particle with {max occurs} 0 reached groupField", o.subject, t.Loc(), c.group.Compositor())
	}
	return fieldSpec{local: c.group.Compositor().String(), goType: goType}, nil
}

// occurrenceType returns the type for one occurrence of the model group group,
// the term of the particle at ordinal under parent (groupSite), of the complex
// type t generated from o, in a struct whose nested types are named in nested:
// when flat is true, the sealed interface flattened emits; otherwise a struct
// of a field per particle, with no attribute field, for a sequence or an all group, and
// the sealed interface sealedType emits for a choice. It is named by nested's path
// followed by its site's words, the compositor of each enclosing occurrence type and then
// its own, generated from the declaration that path is named from — t when t is named
// (typeOrigin), and o's element otherwise — and emitted after the struct holding its
// field; its particles take their nested types' names from nested as if it were absent.
// Under a named complex type or a model group definition (scope.shared) it is stored in
// e.emitted under nested.groupKey of its site before its particles are walked, shared by
// every later walk of nested's path, and removed when that walk fails, as scopedType
// stores a type; flat is a function of the particle at that site (flattens), so every
// walk sharing the key reads one kind of type.
func (e *emitter) occurrenceType(nested scope, parent groupSite, ordinal int, o origin, t xsd.ComplexType, group xsd.ModelGroup, flat bool) (emittedType, error) {
	site := parent.group(ordinal, group.Compositor())
	key := nested.groupKey(site)
	if typ, ok := e.emitted[key]; ok && nested.shared() {
		return typ, nil
	}
	typ := emittedType{
		name:   e.types.path(append(nested.steps(), generatedSteps(site.words...)...)),
		sealed: flat || group.Compositor() == xsd.CompositorChoice,
	}
	if nested.shared() {
		e.emitted[key] = typ
	}
	if t.Name() != (xsd.QName{}) {
		o.from = e.typeOrigin(t).from
	}
	emitType := e.occurrence
	if flat {
		emitType = e.flattened
	}
	if err := emitType(typ, nested, site, o, t, group.Particles()); err != nil {
		if nested.shared() {
			delete(e.emitted, key)
		}
		return emittedType{}, err
	}
	return typ, nil
}

// occurrence emits typ, the type occurrenceType names for one occurrence of
// the model group of particles at site in the complex type t generated from
// o: the sealed interface sealedType emits, or a struct of particleFields'
// fields. The particles' walk starts with an empty chain (valueChain): the
// occurrence lies behind a slice, a pointer or an interface, so no value field
// passes through it.
func (e *emitter) occurrence(typ emittedType, nested scope, site groupSite, o origin, t xsd.ComplexType, particles []xsd.Particle) error {
	if typ.sealed {
		return e.sealedType(typ.name, nested, site, o, t, particles)
	}
	slot := len(e.decls)
	e.decls = append(e.decls, declaration{})
	fields, err := e.particleFields(nested, site, nil, o, t, particles)
	if err != nil {
		return err
	}
	e.decls[slot] = declare(typ.name, o.from, "", structDecl(typ.name, fields))
	return nil
}

// flattens reports whether groupField gives the model group particle c a
// flattened field (flattened), by the three gates the package doc's
// "# Particles" states: c's group is a choice one of whose alternatives'
// term is an emptiable model group (termEmptiable, which answers false for
// an element declaration or a wildcard), whatever c's occurrence; or c's
// {max occurs} is above 1 or unbounded and either its group is a sequence
// one occurrence of which is emptiable, or its group has a repeating edge
// (repeatingEdge). subject and loc name the complex type holding c in the
// error termEmptiable or repeatingEdge returns.
func (e *emitter) flattens(subject string, loc xsderr.Loc, c groupParticle) (bool, error) {
	if c.group.Compositor() == xsd.CompositorChoice {
		for _, p := range c.group.Particles() {
			emptiable, err := e.termEmptiable(subject, loc, p.Term())
			if err != nil {
				return false, err
			}
			if emptiable {
				return true, nil
			}
		}
	}
	if upper, bounded := c.occurs.Max(); bounded && upper <= 1 {
		return false, nil
	}
	if c.group.Compositor() == xsd.CompositorSequence {
		emptiable, err := e.termEmptiable(subject, loc, xsd.ResolvedTerm{Term: c.group})
		if err != nil {
			return false, err
		}
		if emptiable {
			return true, nil
		}
	}
	return e.repeatingEdge(subject, loc, c.group)
}

// flattened emits typ, the sealed interface occurrenceType names for a model
// group particle at site whose term holds particles and which flattens answers
// true for, of the complex type t generated from o, in the package doc's
// "# Particles" shape: the interface, then one branch struct per particle reach
// returns for particles, in that order, each followed by the types its field
// reaches, the anonymous ones named in nested as if the flattened group were
// absent; for none, the interface alone, which no branch implements. reach
// runs whole, and so declines a particle with {max occurs} 0 or errors on an
// all group, before the interface is emitted. A branch is named from the
// interface's name plus the basic particle's word and generated from what its
// header renders (alternative), its name claimed after the interface's and
// every earlier branch's and the names their fields' types first claim. Its one
// field is the one field gives the basic particle's term occurring exactly
// once, with an empty chain (valueChain): an element's, through any
// substitution group or governing types' interface and any Nillable or
// MixedChild wrap, or a wildcard's AnyElement or AnyChild, never a pointer or
// a slice.
func (e *emitter) flattened(typ emittedType, nested scope, site groupSite, o origin, t xsd.ComplexType, particles []xsd.Particle) error {
	basic, err := e.reach(o.subject, t.Loc(), particles)
	if err != nil {
		return err
	}
	marker := "is" + typ.name
	e.decls = append(e.decls, declare(typ.name, o.from, "", fmt.Sprintf("type %s interface {\n\t%s()\n}\n", typ.name, marker)))
	for _, p := range basic {
		word, from, err := e.alternative(o, t, p)
		if err != nil {
			return err
		}
		once, err := xsd.NewParticle(xsderr.Loc{}, exactlyOnce, p.Term())
		if err != nil {
			return fmt.Errorf("%s at %s: %w", o.subject, t.Loc(), err)
		}
		branch := e.types.extend(typ.name, word)
		slot := len(e.decls)
		e.decls = append(e.decls, declaration{})
		field, err := e.field(nested, site, 0, nil, o, t, once)
		if err != nil {
			return err
		}
		e.decls[slot] = declare(branch, from, "", structDecl(branch, []fieldSpec{field})+fmt.Sprintf("\nfunc (%s) %s() {}\n", branch, marker))
	}
	return nil
}

// reach returns the basic particles (Structures key-basic-particle) a
// flattened field's interface gets one branch each for, in a depth-first walk
// of particles in order: an element particle, local or an <element ref>, or a
// wildcard particle, is one, and a model group particle, inline or a <group
// ref> (modelGroupTerm), is replaced in place by its own group's. It never
// enters an element's type, keys each branch by the particle at its point, so
// a particle a <group ref> brings twice gives two, and dedups nothing. A
// particle with {max occurs} 0 declines, naming its term as field does, and a
// model group one before any of its own particles is reached; an all group is
// an error, never a decline: cos-all-limited (§3.8.6.2), checked at Finalize,
// admits an all group particle only as a content {particle} with {max occurs}
// 1 or among an all group's {particles}, and a flattened group is neither an
// all group nor inside one. subject and loc name the complex type holding
// particles.
func (e *emitter) reach(subject string, loc xsderr.Loc, particles []xsd.Particle) ([]xsd.Particle, error) {
	var basic []xsd.Particle
	for _, p := range particles {
		group, isGroup, err := e.modelGroupTerm(subject, loc, p.Term())
		if err != nil {
			return nil, err
		}
		upper, bounded := p.Occurs().Max()
		vacuous := bounded && upper == 0
		if !isGroup {
			if vacuous {
				return nil, e.vacuousBasic(subject, loc, p)
			}
			basic = append(basic, p)
			continue
		}
		if group.Compositor() == xsd.CompositorAll {
			return nil, fmt.Errorf("%s at %s: an all model group particle inside a flattened model group reached reach", subject, loc)
		}
		if vacuous {
			return nil, decline(loc, constructVacuousParticle, fmt.Sprintf("a %s model group", group.Compositor()), subject)
		}
		inner, err := e.reach(subject, loc, group.Particles())
		if err != nil {
			return nil, err
		}
		basic = append(basic, inner...)
	}
	return basic, nil
}

// vacuousBasic returns the decline of the element or wildcard particle p with
// {max occurs} 0 of the complex type subject names at loc, its term named as
// field names it: "a wildcard", or the element declaration's expanded name.
func (e *emitter) vacuousBasic(subject string, loc xsderr.Loc, p xsd.Particle) error {
	if wildcardTerm(p.Term()) {
		return decline(loc, constructVacuousParticle, "a wildcard", subject)
	}
	decl, err := e.elementTerm(subject, loc, p.Term())
	if err != nil {
		return err
	}
	return decline(loc, constructVacuousParticle, "element "+decl.Name().String(), subject)
}

// particleFields returns the fields of particles, one per particle spliced
// returns for them, in that order, in a struct at site (groupSite) whose
// nested types are named in nested and whose particles chain is valueChain's
// for, of the complex type t generated from o (field). It numbers the leaves
// whose term is a model group from 1 in that order, each leaf's ordinal under
// site.
func (e *emitter) particleFields(nested scope, site groupSite, chain []valueNode, o origin, t xsd.ComplexType, particles []xsd.Particle) ([]fieldSpec, error) {
	leaves, err := e.spliced(o.subject, t.Loc(), particles)
	if err != nil {
		return nil, err
	}
	fields := make([]fieldSpec, 0, len(leaves))
	ordinal := 0
	for _, p := range leaves {
		_, isGroup, err := e.modelGroupTerm(o.subject, t.Loc(), p.Term())
		if err != nil {
			return nil, err
		}
		if isGroup {
			ordinal++
		}
		field, err := e.field(nested, site, ordinal, chain, o, t, p)
		if err != nil {
			return nil, err
		}
		fields = append(fields, field)
	}
	return fields, nil
}

// spliced returns particles in order with each particle whose term is a model
// group (modelGroupTerm), inline or the {model group} of the definition a
// <group ref> names (§3.7.2), and which splices answers true for replaced,
// recursively, by that group's own particles: one occurrence of a sequence
// particle is one pass through its particles in order (§3.8.4.1.1
// sec-sequences), and one of an all particle one sequence per particle
// interleaved (§3.8.4.1.3 all-mg), so its fields are theirs; cos-all-limited
// (§3.8.6.2 clauses 1.3 and 2), checked at Finalize, admits an all group
// particle among particles only as an all group's member occurring exactly
// once, so every one is spliced. A complex-content extension's
// content particle (§3.4.2.3.3 dcl.ctd.ctcc.common clause 4.2.3.3) is one such
// sequence, nesting its base's when the base is itself an extension, so the
// splice recurses; the package doc's "# Particles" states the fields that
// gives. Every other particle is kept as is, and field gives a choice and a
// sequence splices does not splice, inline or referenced alike, the field
// groupField returns. subject and loc name the complex type holding
// particles in the error modelGroupTerm returns.
func (e *emitter) spliced(subject string, loc xsderr.Loc, particles []xsd.Particle) ([]xsd.Particle, error) {
	var leaves []xsd.Particle
	for _, p := range particles {
		group, isGroup, err := e.modelGroupTerm(subject, loc, p.Term())
		if err != nil {
			return nil, err
		}
		if !isGroup {
			leaves = append(leaves, p)
			continue
		}
		splice, err := e.splices(subject, loc, group, p.Occurs())
		if err != nil {
			return nil, err
		}
		if !splice {
			leaves = append(leaves, p)
			continue
		}
		inner, err := e.spliced(subject, loc, group.Particles())
		if err != nil {
			return nil, err
		}
		leaves = append(leaves, inner...)
	}
	return leaves, nil
}

// splices reports whether a model group particle of group under occurs, inline
// or a <group ref> to group's definition (§3.7.2), gives in its place the fields
// group's own particles give (spliced, contentFields): group is a sequence or an
// all group, and occurs is exactly once, or {min occurs} 0 and {max occurs} 1
// when one occurrence of group can match nothing (termEmptiable, §3.9.6.3
// cos-group-emptiable), as the package doc's "# Particles" states. subject and
// loc name the complex type holding the particle in the error termEmptiable
// returns.
func (e *emitter) splices(subject string, loc xsderr.Loc, group xsd.ModelGroup, occurs xsd.Occurs) (bool, error) {
	if group.Compositor() == xsd.CompositorChoice {
		return false, nil
	}
	if occurs == exactlyOnce {
		return true, nil
	}
	if occurs != optional {
		return false, nil
	}
	return e.termEmptiable(subject, loc, xsd.ResolvedTerm{Term: group})
}

// modelGroupTerm returns the model group a particle's term holds and true: an
// inline <sequence>, <choice> or <all>, or the {model group} of the top-level
// model group definition a <group ref> names (§3.7.2 declare-namedModelGroup),
// followed through [xsd.Schema.ModelGroup], so that every site reads the two
// alike. It returns false for an element declaration, an <element ref> or a
// wildcard. A <group ref> that does not resolve is an error naming subject and
// loc, the complex type holding the particle, and never a decline: finalize
// guarantees it resolves (src-resolve clause 1.5). The term is a closed sum,
// so the switch is STYLE T2's sealed-sum exception, as elementTerm's is.
func (e *emitter) modelGroupTerm(subject string, loc xsderr.Loc, term xsd.TermOrRef) (xsd.ModelGroup, bool, error) {
	switch term := term.(type) {
	case xsd.ModelGroupRef:
		def, ok := e.schema.ModelGroup(term.Name)
		if !ok {
			return xsd.ModelGroup{}, false, fmt.Errorf("%s at %s: its <group ref> particle to %s does not resolve", subject, loc, term.Name)
		}
		return def.ModelGroup(), true, nil
	case xsd.ResolvedTerm:
		group, isGroup := term.Term.(xsd.ModelGroup)
		return group, isGroup, nil
	}
	return xsd.ModelGroup{}, false, nil
}

// structDecl renders the declaration of the struct name holding fields in
// order, their names claimed by claimFields.
func structDecl(name string, fields []fieldSpec) string {
	return renderStruct(name, claimFields(fields))
}

// claimFields returns fields in order, each named by the identifier one
// goname.Namer of their own claims for its local name: the names one struct's
// fields take.
func claimFields(fields []fieldSpec) []goField {
	names := goname.NewNamer()
	claimed := make([]goField, len(fields))
	for i, f := range fields {
		claimed[i] = goField{name: names.Claim(f.local), goType: f.goType}
	}
	return claimed
}

// renderStruct renders the declaration of the struct name holding fields in
// order, spelled struct{} when there is no field.
func renderStruct(name string, fields []goField) string {
	if len(fields) == 0 {
		return fmt.Sprintf("type %s struct{}\n", name)
	}
	var b strings.Builder
	for _, f := range fields {
		fmt.Fprintf(&b, "\t%s %s\n", f.name, f.goType)
	}
	return fmt.Sprintf("type %s struct {\n%s}\n", name, b.String())
}

// sealedType emits the sealed interface name for the complex type t generated
// from o, a choice of particles at site (groupSite) (§3.8.4.1.2 sec-choices:
// one alternative per pass), the term of t's content particle or of a model
// group particle nested in it, in the package doc's "# Complex types" shape:
// the interface, then each branch struct in particle order, followed by the
// types its alternative reaches, the anonymous ones named in nested; for no
// particles, the interface alone, which no branch implements. A branch
// struct's fields are particleFields' for its one alternative particle, read
// through branchParticle, at site.branch(k), k its 1-based position in
// particles, with an empty chain: an element or a wildcard gives its one
// field, a sequence occurring exactly once the fields of its spliced
// particles, and any other model group the one field groupField returns. A
// branch is named from the interface's name plus the alternative's word and
// generated from what its header renders (alternative).
//
// No alternative's term is an emptiable model group, so no branch holds the
// empty match, which the nil interface stands for: shape answers no sealed
// interface for an emptiable content choice, and groupField flattens a nested
// choice holding one (flattens) rather than reaching occurrenceType's
// sealedType.
func (e *emitter) sealedType(name string, nested scope, site groupSite, o origin, t xsd.ComplexType, particles []xsd.Particle) error {
	marker := "is" + name
	e.decls = append(e.decls, declare(name, o.from, "", fmt.Sprintf("type %s interface {\n\t%s()\n}\n", name, marker)))
	for k, p := range particles {
		word, from, err := e.alternative(o, t, p)
		if err != nil {
			return err
		}
		raised, err := branchParticle(o.subject, t.Loc(), p)
		if err != nil {
			return err
		}
		branch := e.types.extend(name, word)
		slot := len(e.decls)
		e.decls = append(e.decls, declaration{})
		fields, err := e.particleFields(nested, site.branch(k+1), nil, o, t, []xsd.Particle{raised})
		if err != nil {
			return err
		}
		e.decls[slot] = declare(branch, from, "", structDecl(branch, fields)+fmt.Sprintf("\nfunc (%s) %s() {}\n", branch, marker))
	}
	return nil
}

// alternative returns, for the alternative p of a sealed interface emitted
// for the complex type t generated from o by sealedType, or the basic particle
// p of one flattened emits, the word its branch is named from after the
// interface's name and what its branch header renders as generated from: for
// an element declaration, local or the top-level one an <element ref>
// particle names (elementTerm), its local name and elementFrom's rendering;
// for a wildcard (wildcardTerm), which has no {name} (§3.10.1), the word any
// and o's from; for a model group, inline or a <group ref> (modelGroupTerm),
// which only sealedType passes, its compositor and o's from.
func (e *emitter) alternative(o origin, t xsd.ComplexType, p xsd.Particle) (word string, from provenance, err error) {
	if wildcardTerm(p.Term()) {
		return "any", o.from, nil
	}
	group, isGroup, err := e.modelGroupTerm(o.subject, t.Loc(), p.Term())
	if err != nil {
		return "", provenance{}, err
	}
	if isGroup {
		return group.Compositor().String(), o.from, nil
	}
	decl, err := e.elementTerm(o.subject, t.Loc(), p.Term())
	if err != nil {
		return "", provenance{}, err
	}
	return decl.Name().Local, e.elementFrom(decl), nil
}

// termEmptiable reports whether one occurrence of a particle's {term} can
// match nothing: [xsd.Schema.ParticleEmptiable] (§3.9.6.3 cos-group-emptiable)
// over a particle of term occurring exactly once, so that its clause 2 alone
// decides. It is false for an element declaration or a wildcard and true for
// a model group with no {particles}; codegen does no range arithmetic of its
// own. subject and loc name the complex type holding the term in the error a
// particle that cannot be built returns.
func (e *emitter) termEmptiable(subject string, loc xsderr.Loc, term xsd.TermOrRef) (bool, error) {
	p, err := xsd.NewParticle(xsderr.Loc{}, exactlyOnce, term)
	if err != nil {
		return false, fmt.Errorf("%s at %s: %w", subject, loc, err)
	}
	return e.schema.ParticleEmptiable(p), nil
}

// repeatingEdge reports whether group, the term of a model group particle with
// {max occurs} above 1 or unbounded, holds a particle that repeats (repeats)
// at both its edges, as the package doc's "# Particles" states
// (othersEmptiable reads a sequence's edges). It reads model group terms
// through modelGroupTerm, never enters an element's type, and stops at the
// first repeating edge particle; the <group ref> graph is acyclic after
// finalize, so the walk ends. No all group is reached: cos-all-limited
// (§3.8.6.2 clauses 1.2 and 1.3) admits a particle whose term is one only as a
// content particle with {max occurs} 1 or as an all group's member, neither of
// which is group's particle or inside it. subject and loc name the complex
// type holding group in the error modelGroupTerm returns.
func (e *emitter) repeatingEdge(subject string, loc xsderr.Loc, group xsd.ModelGroup) (bool, error) {
	particles := group.Particles()
	for i, p := range particles {
		if group.Compositor() != xsd.CompositorChoice && !e.othersEmptiable(particles, i) {
			continue
		}
		if repeats(p.Occurs()) {
			return true, nil
		}
		inner, isGroup, err := e.modelGroupTerm(subject, loc, p.Term())
		if err != nil {
			return false, err
		}
		if !isGroup {
			continue
		}
		edge, err := e.repeatingEdge(subject, loc, inner)
		if err != nil {
			return false, err
		}
		if edge {
			return true, nil
		}
	}
	return false, nil
}

// othersEmptiable reports whether every particle of particles but the one at
// index i is emptiable ([xsd.Schema.ParticleEmptiable]).
func (e *emitter) othersEmptiable(particles []xsd.Particle, i int) bool {
	for j, p := range particles {
		if j != i && !e.schema.ParticleEmptiable(p) {
			return false
		}
	}
	return true
}

// repeats reports whether a particle of occurs repeats, as the package doc's
// "# Particles" defines the word.
func repeats(occurs xsd.Occurs) bool {
	upper, bounded := occurs.Max()
	return !bounded || upper > 1 && upper != occurs.Min()
}

// branchParticle returns the particle a choice's branch struct holds for its
// alternative p: p with {min occurs} 0 read as 1 when {max occurs} is not 0,
// {max occurs} unchanged, and p itself otherwise. A branch is the
// alternative chosen, so it never holds the empty match the nil interface
// stands for: an element or a wildcard {0,1} gives a value field rather than
// a pointer, a sequence {0,1} is spliced, and a choice {0,1} gives the bare
// interface. subject and loc name the complex type holding p in the error a
// particle that cannot be built returns.
func branchParticle(subject string, loc xsderr.Loc, p xsd.Particle) (xsd.Particle, error) {
	occurs := p.Occurs()
	upper, bounded := occurs.Max()
	if occurs.Min() > 0 || bounded && upper == 0 {
		return p, nil
	}
	raised, err := xsd.NewUnboundedOccurs(xsderr.Loc{}, 1)
	if bounded {
		raised, err = xsd.NewOccurs(xsderr.Loc{}, 1, upper)
	}
	if err != nil {
		return xsd.Particle{}, fmt.Errorf("%s at %s: %w", subject, loc, err)
	}
	branch, err := xsd.NewParticle(xsderr.Loc{}, raised, p.Term())
	if err != nil {
		return xsd.Particle{}, fmt.Errorf("%s at %s: %w", subject, loc, err)
	}
	return branch, nil
}

// exactlyOnce is the {min occurs} = {max occurs} = 1 range under which a
// sequence or an all particle, a complex type's content particle or nested, is
// spliced whatever its emptiability (splices), and under which an element
// particle gives a value field.
var exactlyOnce = mustOccurs(xsd.NewOccurs(xsderr.Loc{}, 1, 1))

// optional is the {min occurs} 0, {max occurs} 1 range under which a sequence
// or an all particle whose one occurrence is emptiable is spliced (splices).
var optional = mustOccurs(xsd.NewOccurs(xsderr.Loc{}, 0, 1))

// mustOccurs unwraps a constructor result that cannot fail on constant
// arguments.
func mustOccurs(o xsd.Occurs, err error) xsd.Occurs {
	if err != nil {
		panic(err)
	}
	return o
}

// groupParticle is a model group particle as groupField reads it: group, the
// sequence, choice or all model group that is its term, inline or the {model
// group} a <group ref> names (§3.7.2), never the source <choice> itself, and
// occurs, its {min occurs} and {max occurs}, {max occurs} never 0. Its two
// construction sites, contentGroup for a complex type's content {particle}
// (§3.4.2.3.3 dcl.ctd.ctcc.common) and field's model group arm for a nested
// one, each screen a {max occurs} of 0 first, and field an all group, which
// spliced never returns because it splices each exactly-once one (§3.8.6.2
// cos-all-limited clauses 1 and 1.3).
type groupParticle struct {
	group  xsd.ModelGroup
	occurs xsd.Occurs
}

// contentGroup checks the complex type t, named in declines by subject, against
// the package doc's "# Complex types" complex-type shape. It reads t's finalized
// component alone, never its base, so a base that itself declines does not
// block t, and an extension is checked as a restriction is: its {content type}
// already holds what it takes from its base (§3.4.2.3.3 dcl.ctd.ctcc.common
// clause 4.2, §3.4.2.2 dcl.ctd.ctsc). For element-only or mixed content it
// returns t's content particle, whatever its {min occurs} and {max occurs} but
// a {max occurs} of 0, which declines, its group the particle's own model group
// or, for a <group ref>, the referenced definition's {model group}
// (modelGroupTerm), read before the {max occurs} test, so that a referenced
// group is checked and emitted as an inline one with the ref's {min occurs} and
// {max occurs} (§3.7.2), whatever t's {open content}, which body gives its own
// field (hasOpenContent); for {variety} empty (§3.4.1), which has no {particle},
// and for {variety} simple, whose value field body gives, it returns nil and a
// nil error. body and field read whether the content is mixed (isMixed).
// Particles and model groups carry no Loc, so their declines cite t's.
func (e *emitter) contentGroup(subject string, t xsd.ComplexType) (*groupParticle, error) {
	content, isElement := t.ContentType().(xsd.ElementContent)
	if !isElement {
		switch variety := t.ContentType().Variety(); variety {
		case xsd.ContentEmpty, xsd.ContentSimple:
			return nil, nil
		default:
			return nil, fmt.Errorf("%s at %s: unexpected content type %T of {variety} %s", subject, t.Loc(), t.ContentType(), variety)
		}
	}
	group, isGroup, err := e.modelGroupTerm(subject, t.Loc(), content.Particle.Term())
	if err != nil {
		return nil, err
	}
	if !isGroup {
		return nil, fmt.Errorf("%s at %s: unexpected content particle term %T", subject, t.Loc(), content.Particle.Term())
	}
	occurs := content.Particle.Occurs()
	if upper, bounded := occurs.Max(); bounded && upper == 0 {
		return nil, decline(t.Loc(), constructVacuousParticle, fmt.Sprintf("a %s model group", group.Compositor()), subject)
	}
	return &groupParticle{group: group, occurs: occurs}, nil
}

// fieldSpec is one struct field before its name is claimed.
type fieldSpec struct {
	local  string
	goType string
}

// field returns the field of particle p in a struct whose nested types are
// named in nested and whose particles chain is valueChain's for, of the complex
// type t generated from o, typed by the package doc's "# Complex types" and
// "# Particles" field rules. In a t of mixed content (isMixed), an element
// particle's field holds MixedChild (mixedChildType, claimed before the field's
// type is computed) over the type the rules below give, and a wildcard particle's
// AnyChild in place of AnyElement; a model group particle's field is not wrapped.
// The field of a wildcard particle (wildcardTerm) is named from the local name
// any and holds the raw-content struct AnyElement (anyTypeType) by p's occurrence
// (occursGoType), whatever the wildcard's {namespace constraint} and {process
// contents}, which it never reads; that path never calls shape, contentGroup,
// nestedType or valueChain, and gives a choice's wildcard alternative its
// branch struct's one field (sealedType). The field of a model group particle
// (modelGroupTerm), inline or a <group ref>, a choice or a sequence which
// spliced does not splice (splices), is the one groupField returns for p at
// ordinal under site, after a {max occurs} of 0 has declined naming the group,
// as contentGroup declines one, before any occurrence type is emitted; an all
// group is an error, never a decline: cos-all-limited (§3.8.6.2), checked at
// Finalize, admits an all group particle only as a content {particle}, which
// contentGroup and groupField handle, or exactly once among an all group's
// {particles}, which spliced splices. An element particle whose declaration
// abstractLocal answers true for declines, charged to that declaration, before
// its type is resolved. The field of an <element ref> particle, whose declaration is
// global, holds the type refType returns for the referenced declaration, after
// declaredType has resolved that declaration's type: the sealed interface over its
// substitution group's declarations with {abstract} false when goshape.SubstitutionSum
// answers a sum, and the type elementType returns for it otherwise; the walk does not
// enter that type's body, which topLevel emits, but valueCycle checks it when the
// particle occurs exactly once, the declaration is not nillable, and the type is a
// complex type other than xs:anyType emitted as a struct; valueCycle ends at once for a
// type whose content particle occurs other than exactly once. A local declaration
// governingTypes answers a sum for holds the sealed interface localTable returns. A
// local declaration of type xs:anyType holds the raw-content struct AnyType
// (anyTypeType). From field, neither a global declaration nor xs:anyType ever reaches
// nestedType. A nillable declaration (nillable) holds, inside any MixedChild wrap and
// by the same occurrence rules, Nillable over the type the rules above give
// (nillableType), whose names are claimed after that type's, unless that type is
// AnyType, which holds xsi:nil among its Attributes, or a sum over the declaration's
// substitution group or governing types, whose branches wrap their own fields
// (substitutionType, tableType); its particle is never a value field, whatever its
// occurrence, since the field holds an interface or AnyType, whose walk ends at once.
func (e *emitter) field(nested scope, site groupSite, ordinal int, chain []valueNode, o origin, t xsd.ComplexType, p xsd.Particle) (fieldSpec, error) {
	subject, loc := o.subject, t.Loc()
	if wildcardTerm(p.Term()) {
		e.anyTypeType()
		item := e.raw.element
		if isMixed(t) {
			item = e.raw.child
		}
		goType, ok := occursGoType(emittedType{name: item}, p.Occurs())
		if !ok {
			return fieldSpec{}, decline(loc, constructVacuousParticle, "a wildcard", subject)
		}
		return fieldSpec{local: "any", goType: goType}, nil
	}
	group, isGroup, err := e.modelGroupTerm(subject, loc, p.Term())
	if err != nil {
		return fieldSpec{}, err
	}
	if isGroup {
		if group.Compositor() == xsd.CompositorAll {
			return fieldSpec{}, fmt.Errorf("%s at %s: an all model group particle nested in a model group reached field", subject, loc)
		}
		if upper, bounded := p.Occurs().Max(); bounded && upper == 0 {
			return fieldSpec{}, decline(loc, constructVacuousParticle, fmt.Sprintf("a %s model group", group.Compositor()), subject)
		}
		return e.groupField(nested, site, ordinal, o, t, groupParticle{group: group, occurs: p.Occurs()})
	}
	decl, err := e.elementTerm(subject, loc, p.Term())
	if err != nil {
		return fieldSpec{}, err
	}
	if abstractLocal(decl) {
		return fieldSpec{}, decline(decl.Loc(), constructAbstractLocal, "local element "+decl.Name().String(), subject)
	}
	td, err := e.declaredType(decl)
	if err != nil {
		return fieldSpec{}, err
	}
	branches, sum, err := e.governingTypes(decl)
	if err != nil {
		return fieldSpec{}, err
	}
	if isMixed(t) {
		e.mixedChildType()
	}
	value := p.Occurs() == exactlyOnce && !nillable(decl)
	st, isSimple := td.(*xsd.SimpleType)
	ct, isComplex := td.(xsd.ComplexType)
	var typ emittedType
	switch {
	case decl.ScopeVariety() == xsd.ScopeGlobal:
		typ, err = e.refType(chain, value, decl, td)
	case sum:
		typ, err = e.localTable(nested, decl, branches)
	case isAnyType(td):
		typ.name = e.anyTypeType()
	case isSimple:
		typ, err = e.simpleGoType("element "+decl.Name().String(), decl.Loc(), st, declaredUnion(localScope(nested, decl).steps()))
	case isComplex:
		typ, err = e.nestedType(nested, chain, value, decl, ct)
	default:
		err = fmt.Errorf("element %s at %s: unexpected type definition %T", decl.Name(), decl.Loc(), td)
	}
	if err != nil {
		return fieldSpec{}, err
	}
	_, substitution := goshape.SubstitutionSum(e.schema, decl)
	if nillable(decl) && !sum && !isAnyType(td) && !substitution {
		typ = e.nillableType(typ, td)
	}
	if isMixed(t) {
		typ = emittedType{name: e.mixedChild + "[" + typ.name + "]"}
	}
	goType, ok := occursGoType(typ, p.Occurs())
	if !ok {
		return fieldSpec{}, decline(loc, constructVacuousParticle, "element "+decl.Name().String(), subject)
	}
	return fieldSpec{local: decl.Name().Local, goType: goType}, nil
}

// occursGoType is the Go type of a field holding typ under occurs, by the
// package doc's "# Particles" pointer and slice rules: a slice for {max
// occurs} above 1 or unbounded, a pointer for {min occurs} 0 with {max occurs}
// 1 unless typ is a sealed interface, a choice's or a union's, and typ itself
// otherwise. ok is false, and goType "", exactly when {max occurs} is 0; the
// caller declines that particle naming its own term.
func occursGoType(typ emittedType, occurs xsd.Occurs) (goType string, ok bool) {
	upper, bounded := occurs.Max()
	switch {
	case !bounded || upper > 1:
		return "[]" + typ.name, true
	case upper == 0:
		return "", false
	case upper == 1 && occurs.Min() == 0 && !typ.sealed:
		return "*" + typ.name, true
	}
	return typ.name, true
}

// wildcardTerm reports whether a particle's term is a wildcard (§3.10.1): an
// xsd.ResolvedTerm whose Term is an xsd.Wildcard. The particle term is a
// closed sum, so this assertion is STYLE T2's sealed-sum exception, as
// elementTerm's switch is.
func wildcardTerm(term xsd.TermOrRef) bool {
	resolved, ok := term.(xsd.ResolvedTerm)
	if !ok {
		return false
	}
	_, ok = resolved.Term.(xsd.Wildcard)
	return ok
}

// abstractLocal reports whether the element declaration decl is local with
// {abstract} true, the declaration field declines and valueCycle passes as a
// leaf; the package doc's "# Coverage" states why no schema document yields
// one.
func abstractLocal(decl xsd.ElementDeclaration) bool {
	return decl.ScopeVariety() == xsd.ScopeLocal && decl.Abstract()
}

// refType returns the type of the field of an <element ref> particle to the
// top-level element declaration decl, whose {type definition} declaredType
// resolved to td: the sealed interface substitutionType emits when
// goshape.SubstitutionSum answers a sum for decl, and elementType's otherwise. When
// value is true, the particle occurring exactly once over a declaration that
// is not nillable (field's value, which reads nillable), and that type is a struct
// emitted for a complex td other than xs:anyType, valueCycle walks td's body
// from the particles whose chain is chain, as it does whether or not decl's
// type was emitted earlier. A td of xs:anyType gets no walk: AnyType reaches
// itself only through slices. Nor does the substitution group's interface,
// nor the governing types' interface elementType records as sealed: no field
// holds either as a struct value.
func (e *emitter) refType(chain []valueNode, value bool, decl xsd.ElementDeclaration, td xsd.TypeDefinition) (emittedType, error) {
	if members, sum := goshape.SubstitutionSum(e.schema, decl); sum {
		return e.substitutionType(decl, members)
	}
	typ, err := e.elementType(decl, td)
	if err != nil {
		return emittedType{}, err
	}
	ct, isComplex := td.(xsd.ComplexType)
	if isAnyType(td) || !value || !isComplex || typ.sealed {
		return typ, nil
	}
	return typ, e.valueCycle(chain, decl, structScope(scope{}, decl, ct), ct)
}

// substitutionType returns the sealed interface a field over an <element ref>
// particle to the head declaration head holds, members being goshape.SubstitutionSum's
// for head, emitting it once per package in the package doc's "# Substitution groups"
// shape: the interface, named from head's local name plus SubstitutionGroup, then one
// branch struct per declaration of members in order, none when members is empty, named
// from the interface's name plus the declaration's local name and holding one field,
// named from that local name, of the type elementType returns for it. Each
// declaration's type is resolved by declaredType and claimed by elementType at its
// branch, after the branch's name, so the shape of its type, or of each of its
// governing types, declines the interface at the first such declaration in group
// order, with the same DeclineError its own topLevel turn returns first; a decline in
// a declaration's body is met only at its own turn. Each branch implements the
// interface's method; the declarations' own types carry no method, since one may be
// sealed or sit in several heads' groups. The field of a nillable declaration's branch
// (nillable), the head's when it has a branch, holds Nillable over that type
// (nillableType), its names claimed after the type's, unless the type is AnyType or
// the sum over the declaration's governing types, whose branches wrap their own
// fields; the interface itself is never wrapped, each declaration's own {nillable}
// governing its branch. It stores the interface's name in e.substitutions only once
// every branch is emitted.
func (e *emitter) substitutionType(head xsd.ElementDeclaration, members []xsd.ElementDeclaration) (emittedType, error) {
	if name, ok := e.substitutions[head.Name()]; ok {
		return emittedType{name: name, sealed: true}, nil
	}
	name := e.types.fixed(head.Name().Local, "SubstitutionGroup")
	marker := "is" + name
	e.decls = append(e.decls, declare(name, e.elementFrom(head).prefixed("the substitution group of "), "", fmt.Sprintf("type %s interface {\n\t%s()\n}\n", name, marker)))
	for _, member := range members {
		branch := e.types.extend(name, member.Name().Local)
		td, err := e.declaredType(member)
		if err != nil {
			return emittedType{}, err
		}
		typ, err := e.elementType(member, td)
		if err != nil {
			return emittedType{}, err
		}
		_, sum, err := e.governingTypes(member)
		if err != nil {
			return emittedType{}, err
		}
		if nillable(member) && !sum && !isAnyType(td) {
			typ = e.nillableType(typ, td)
		}
		fields := []fieldSpec{{local: member.Name().Local, goType: typ.name}}
		e.decls = append(e.decls, declare(branch, e.elementFrom(member), "", structDecl(branch, fields)+fmt.Sprintf("\nfunc (%s) %s() {}\n", branch, marker)))
	}
	e.substitutions[head.Name()] = name
	return emittedType{name: name, sealed: true}, nil
}

// localTable returns the sealed interface a field over the local element
// declaration decl holds when governingTypes answers a sum, branches being its
// answer, its parent's particles taking their names from parent: named and
// keyed by localScope's path, the one an anonymous complex type of decl would
// take, and emitted by tableType once per nestedKey under a shared scope
// (scope.shared), stored before its branches are walked and removed when they
// fail, as scopedType stores a struct. No struct is ever emitted at that
// path: decl's own {type definition} is emitted only as a branch, under the
// path one step longer.
func (e *emitter) localTable(parent scope, decl xsd.ElementDeclaration, branches []tableBranch) (emittedType, error) {
	ifc := localScope(parent, decl)
	key := ifc.key()
	if typ, ok := e.emitted[key]; ok && ifc.shared() {
		return typ, nil
	}
	typ := emittedType{name: e.types.path(ifc.steps()), sealed: true}
	if ifc.shared() {
		e.emitted[key] = typ
	}
	if err := e.tableType(typ, ifc, decl, branches); err != nil {
		if ifc.shared() {
			delete(e.emitted, key)
		}
		return emittedType{}, err
	}
	return typ, nil
}

// tableType emits typ, the sealed interface "# Element declarations" gives
// the element declaration decl that governingTypes answers a sum for with
// branches, its path ifc: the interface, generated from decl's {type table},
// or from its {type definition} when it has none, then one branch struct per
// branch in branches' order, named from the interface's name plus the
// branch's local name (tableBranch.local), generated from its slot
// (tableBranch.slot) and holding one field, named from that local name, of
// the type branchType returns, each followed by the types that field first
// needs. The branch's name is claimed before its field's type is computed.
// The branch types carry no method: a named one may be selected by several
// tables, or held by fields of its own. For a nillable decl (nillable), each
// branch's field holds Nillable over that type (nillableType), its names
// claimed after the type's, but a branch of xs:anyType, whose AnyType holds
// xsi:nil among its Attributes; the interface itself is not wrapped. With no
// branch, the interface is emitted alone.
func (e *emitter) tableType(typ emittedType, ifc scope, decl xsd.ElementDeclaration, branches []tableBranch) error {
	marker := "is" + typ.name
	slots := "the {type table} of "
	if _, hasTable := decl.TypeTable(); !hasTable {
		slots = "the " + declaredSlot{}.name() + " of "
	}
	e.decls = append(e.decls, declare(typ.name, e.elementFrom(decl).prefixed(slots), "", fmt.Sprintf("type %s interface {\n\t%s()\n}\n", typ.name, marker)))
	for _, b := range branches {
		branch := e.types.extend(typ.name, b.local())
		slot := len(e.decls)
		e.decls = append(e.decls, declaration{})
		field, err := e.branchType(ifc, decl, b)
		if err != nil {
			return err
		}
		if nillable(decl) && !isAnyType(b.td) {
			field = e.nillableType(field, b.td)
		}
		fields := []fieldSpec{{local: b.local(), goType: field.name}}
		e.decls[slot] = declare(branch, e.elementFrom(decl).prefixed(b.slot.name()+" of "), "", structDecl(branch, fields)+fmt.Sprintf("\nfunc (%s) %s() {}\n", branch, marker))
	}
	return nil
}

// branchType returns the type of the one field of the branch struct for b, a
// branch of the sealed interface whose path is ifc over the governing types
// of the element declaration decl: the type a local element declaration of b's
// type holds when it occurs exactly once. That is AnyType for xs:anyType
// (anyTypeType); for a simple type, simpleGoType's, named in declines by b's
// declaring place (scope.place) and charged to decl, an anonymous union named
// by branchScope's path; for a named complex type, nestedType's, emitted once
// per expanded name and shared with every local element of that type; and for
// an anonymous complex type, a struct or interface of its own, named and keyed
// by branchScope's path, which every type nested in it extends (scopedType). A
// complex branch type is walked as a value field from an empty chain, as a
// choice's branch is (valueChain): the interface lies between it and the
// fields over decl, so the chain starts with the step through decl to that
// type alone, whose node for decl itself no value field reaches.
func (e *emitter) branchType(ifc scope, decl xsd.ElementDeclaration, b tableBranch) (emittedType, error) {
	if isAnyType(b.td) {
		return emittedType{name: e.anyTypeType()}, nil
	}
	child := branchScope(ifc, b)
	switch t := b.td.(type) {
	case *xsd.SimpleType:
		return e.simpleGoType(child.place(), decl.Loc(), t, declaredUnion(child.steps()))
	case xsd.ComplexType:
		if t.Name() != (xsd.QName{}) {
			return e.nestedType(ifc, nil, true, decl, t)
		}
		return e.scopedType(child, e.branchOrigin(child, decl, b, t), nil, true, decl, t)
	}
	return emittedType{}, fmt.Errorf("%s of element %s at %s: unexpected type definition %T", b.slot.name(), decl.Name(), decl.Loc(), b.td)
}

// branchScope is the scope of the type emitted for b's anonymous type, a
// branch of the sealed interface whose path is ifc: ifc's path followed by
// b's branch step.
func branchScope(ifc scope, b tableBranch) scope {
	return scope{root: ifc.root, path: append(slices.Clone(ifc.path), pathStep{name: xsd.QName{Local: b.local() + "Type"}, slot: b.slot.name()})}
}

// branchOrigin is the origin of the type emitted for t, b's complex type, a
// branch of the sealed interface over the governing types of the element
// declaration decl, whose struct's scope is child (branchScope) when t is
// anonymous: typeOrigin's for a named t, and for an anonymous one generated
// from b's slot of decl and named in declines by its declaring place
// (scope.place), which names that slot.
func (e *emitter) branchOrigin(child scope, decl xsd.ElementDeclaration, b tableBranch, t xsd.ComplexType) origin {
	if t.Name() != (xsd.QName{}) {
		return e.typeOrigin(t)
	}
	return origin{from: e.elementFrom(decl).prefixed(b.slot.name() + " of "), subject: "complex type (anonymous) of " + child.place()}
}

// attributeField returns the field of attribute use u (§3.5.1) of the complex
// type at loc named in declines by subject, typed and declined by the package
// doc's "# Attributes" attribute-use rule. An attribute use carries no Loc, so
// its declines cite the declaration it resolves to, the global one for an
// <attribute ref>. An anonymous union type is named by attributePath, nested
// being the scope of the struct holding the field. A declaration is a built-in
// xsi: one, which that rule excludes from the effective value constraint's
// arm, exactly when it is in the xsi namespace (§3.2.6.4 no-xsi).
func (e *emitter) attributeField(nested scope, subject string, loc xsderr.Loc, u xsd.AttributeUse) (fieldSpec, error) {
	name := u.DeclarationName()
	attr := "attribute " + name.String() + " of " + subject
	decl, ok := e.schema.ResolvedAttributeDeclaration(u)
	if !ok {
		return fieldSpec{}, fmt.Errorf("%s at %s: its {attribute declaration} does not resolve", attr, loc)
	}
	td, ok := e.schema.ResolvedType(decl.TypeDefinition())
	if !ok {
		return fieldSpec{}, fmt.Errorf("%s at %s: its {type definition} does not resolve", attr, decl.Loc())
	}
	st, ok := td.(*xsd.SimpleType)
	if !ok {
		return fieldSpec{}, fmt.Errorf("%s at %s: unexpected type definition %T", attr, decl.Loc(), td)
	}
	typ, err := e.simpleGoType(attr, decl.Loc(), st, attributePath(nested, decl))
	if err != nil {
		return fieldSpec{}, err
	}
	_, constrained := e.schema.EffectiveValueConstraint(u)
	supplied := constrained && name.Space != xsd.XMLSchemaInstanceNS
	if !u.Required() && !supplied && !typ.sealed {
		return fieldSpec{local: name.Local, goType: "*" + typ.name}, nil
	}
	return fieldSpec{local: name.Local, goType: typ.name}, nil
}

// attributePath is the path of local names an anonymous union type of the
// attribute declaration decl is named from ("# Naming"): decl's local name
// alone for a global decl; after it, that of the named complex type or
// attribute group definition its {scope}.{parent} names (§3.2.2.2
// dcl.att.local); and for an anonymous complex type, decl's local name after
// the path of nested, the scope of the struct holding its field. The {parent}
// is a closed sum, so the switch is STYLE T2's sealed-sum exception, as
// localScope's is.
func attributePath(nested scope, decl xsd.AttributeDeclaration) declaredUnion {
	local := declaredStep(decl.Name().Local)
	sp, ok := decl.Scope().Parent()
	if !ok {
		return declaredUnion{local}
	}
	switch owner := sp.(type) {
	case xsd.AttributeComplexTypeScopeParent:
		return declaredUnion{declaredStep(owner.Name.Local), local}
	case xsd.AttributeGroupScopeParent:
		return declaredUnion{declaredStep(owner.Name.Local), local}
	}
	return append(nested.steps(), local)
}

// nestedType returns the type emitted for a local element declaration's complex
// type t, whose particle occurs exactly once when value is true and whose
// parent's particles chain is valueChain's for. A named t gets one type, named
// from t and generated from it, whose nested types take their names from the
// path at t, as under a top-level element of type t (topLevel). An anonymous t
// gets a type named by the path from the named complex type or the model group
// definition that declares decl ({scope}.{parent}, §3.3.2.3 dcl.elt.local)
// when decl has one, so that every type extending that complex type, and every
// particle referencing that group, shares it, and otherwise from its nearest
// named ancestor in parent (structScope). The type is scopedType's in that
// scope, generated from localOrigin's origin. branchType also calls it for a
// named t among an element declaration's governing types, decl being that
// declaration, local or top-level, which a named t's scope and origin do not
// read.
func (e *emitter) nestedType(parent scope, chain []valueNode, value bool, decl xsd.ElementDeclaration, t xsd.ComplexType) (emittedType, error) {
	child := structScope(parent, decl, t)
	return e.scopedType(child, e.localOrigin(decl, child, t), chain, value, decl, t)
}

// scopedType returns the type emitted for the complex type t, reached through
// the element declaration decl, whose struct's scope is child and whose
// declarations are generated from o, its arguments otherwise nestedType's.
// Each named type, and each anonymous type under one or under a model group
// definition, is emitted once per nestedKey, by the first element whose walk
// reaches it, and later ones share it; its entry is stored before its body is
// walked, and removed when that body fails, so that a later turn's walk, which
// emit runs after a decline, enters the type again rather than sharing a type
// never emitted, and so that a walk reaching the type again inside its own
// body, through a <group ref> to the group that declares decl, ends there. A
// shared type reached by a value field is walked again by valueCycle, which the
// emitting walk does not do. The emitted lookup, the store and the removal run
// only under a shared scope (scope.shared): a key whose path starts at a
// top-level element is reached from that element alone.
func (e *emitter) scopedType(child scope, o origin, chain []valueNode, value bool, decl xsd.ElementDeclaration, t xsd.ComplexType) (emittedType, error) {
	key := child.key()
	if typ, ok := e.emitted[key]; ok && child.shared() {
		if !value || typ.sealed {
			return typ, nil
		}
		return typ, e.valueCycle(chain, decl, child, t)
	}
	typ, content, err := e.shape(e.claimScope(child), o, t)
	if err != nil {
		return emittedType{}, err
	}
	next, err := valueChain(chain, value && !typ.sealed, decl, t)
	if err != nil {
		return emittedType{}, err
	}
	if child.shared() {
		e.emitted[key] = typ
	}
	if err := e.body(typ, child, next, o, t, content); err != nil {
		if child.shared() {
			delete(e.emitted, key)
		}
		return emittedType{}, err
	}
	return typ, nil
}

// claimScope claims the name of the struct or interface whose scope is child:
// a named complex type's one-step path is its declared name, claimed fixed,
// and every longer path is claimed as a path ("# Naming").
func (e *emitter) claimScope(child scope) string {
	if len(child.path) == 1 {
		return e.types.fixed(child.path[0].name.Local)
	}
	return e.types.path(child.steps())
}

// localScope is the scope an anonymous type of the local element declaration
// decl is named and keyed in, its parent's particles taking their names from
// parent: the path from the named complex type or the model group definition
// that declares decl ({scope}.{parent}, §3.3.2.3 dcl.elt.local) when decl has
// one, and parent's path followed by decl's expanded name otherwise. The
// {parent} is a closed sum, so the switch is STYLE T2's sealed-sum exception,
// as wildcardTerm's assertion is.
func localScope(parent scope, decl xsd.ElementDeclaration) scope {
	if sp, ok := decl.Scope().Parent(); ok {
		switch owner := sp.(type) {
		case xsd.ComplexTypeScopeParent:
			return scope{root: typeRoot, path: elementPath(owner.Name, decl.Name())}
		case xsd.ModelGroupScopeParent:
			return scope{root: groupRoot, path: elementPath(owner.Name, decl.Name())}
		}
	}
	return scope{root: parent.root, path: append(slices.Clone(parent.path), pathStep{name: decl.Name()})}
}

// structScope is the scope of the struct emitted for the element declaration
// decl's complex type t, its parent's particles taking their names from
// parent: the path at t for a named t, the path at decl for a top-level decl
// of an anonymous t, and localScope's otherwise, the one case that reads
// parent.
func structScope(parent scope, decl xsd.ElementDeclaration, t xsd.ComplexType) scope {
	switch {
	case t.Name() != (xsd.QName{}):
		return scope{root: typeRoot, path: elementPath(t.Name())}
	case decl.ScopeVariety() == xsd.ScopeGlobal:
		return scope{root: elementRoot, path: elementPath(decl.Name())}
	}
	return localScope(parent, decl)
}

// localOrigin is the origin of the type emitted for the local element decl's
// complex type t, whose struct's scope is nested (structScope): typeOrigin's
// for a named t, and for an anonymous one generated from decl and named in
// declines by its declaring place (scope.place).
func (e *emitter) localOrigin(decl xsd.ElementDeclaration, nested scope, t xsd.ComplexType) origin {
	if t.Name() != (xsd.QName{}) {
		return e.typeOrigin(t)
	}
	return origin{from: e.elementFrom(decl), subject: "complex type " + typeName(t) + " of " + nested.place()}
}

// symbolSpace is the symbol space a valueNode's name is in: top-level element
// declarations, type definitions, or the local element declarations of model
// group definitions, which may share an expanded name (§3.3.2, §3.17.1). The
// zero value is invalid (STYLE T1).
type symbolSpace uint8

// The symbolSpace values.
const (
	elementSpace symbolSpace = iota + 1
	typeSpace
	groupElementSpace
)

// valueNode is one node of a chain (valueChain): a top-level element
// declaration or a named complex type definition, by its expanded name, or
// the anonymous complex type of a local element declaration that a model
// group definition declares, by the group's expanded name and the element's,
// the path nestedType keys that type's struct by. Build one with elementNode,
// typeNode or groupElementNode; element is the zero QName but in the last.
type valueNode struct {
	space   symbolSpace
	name    xsd.QName
	element xsd.QName
}

// elementNode returns the valueNode of the top-level element declaration name.
func elementNode(name xsd.QName) valueNode { return valueNode{space: elementSpace, name: name} }

// typeNode returns the valueNode of the named complex type definition name.
func typeNode(name xsd.QName) valueNode { return valueNode{space: typeSpace, name: name} }

// groupElementNode returns the valueNode of the anonymous complex type of the
// local element declaration element that the model group definition group
// declares.
func groupElementNode(group, element xsd.QName) valueNode {
	return valueNode{space: groupElementSpace, name: group, element: element}
}

// String names the node in a struct-value recursion decline.
func (n valueNode) String() string {
	switch n.space {
	case elementSpace:
		return "element " + n.name.String()
	case typeSpace:
		return "complex type " + n.name.String()
	case groupElementSpace:
		return "element " + n.element.String() + " of model group " + n.name.String()
	}
	return fmt.Sprintf("valueNode(%d)", n.space)
}

// valueNodes returns the nodes a walk's step through the element declaration
// decl to its complex type t pushes, in order: a global decl, a top-level
// element or the declaration an <element ref> particle names, pushes its
// element node, and a named t its type node; a local decl of an anonymous t
// pushes its groupElementNode when a model group definition declares decl
// ({scope}.{parent} a xsd.ModelGroupScopeParent, §3.3.2.3 dcl.elt.local),
// and none when a complex type declares it. That node bounds a cycle that
// reaches decl again through a <group ref> to the group declaring it, which
// mg-props-correct clause 2 leaves legal: it bounds group containment alone,
// not the path from a group through an element's type back to it. It names
// decl as well as the group, since a step through a sibling of decl in the
// same group reaches no cycle by that alone. The {parent} is a closed sum, so
// the assertion is STYLE T2's sealed-sum exception, as localScope's switch is.
func valueNodes(decl xsd.ElementDeclaration, t xsd.ComplexType) []valueNode {
	var nodes []valueNode
	if decl.ScopeVariety() == xsd.ScopeGlobal {
		nodes = append(nodes, elementNode(decl.Name()))
	}
	if t.Name() != (xsd.QName{}) {
		return append(nodes, typeNode(t.Name()))
	}
	if sp, ok := decl.Scope().Parent(); ok {
		if owner, inGroup := sp.(xsd.ModelGroupScopeParent); inGroup {
			nodes = append(nodes, groupElementNode(owner.Name, decl.Name()))
		}
	}
	return nodes
}

// valueChain returns the chain for a walk of the particles of the complex
// type t, reached through the element declaration decl from particles whose
// chain is chain, or declines a struct-value recursion. A chain is the nodes
// (valueNodes) a walk has pushed through value fields only since its last
// other field, in path order: a value field is one "# Particles" types by the
// bare struct, which needs a particle that occurs exactly once (value) and a
// t emitted as a struct, not a sealed interface (shape). A value field appends
// the step's nodes, and declines on one already in chain, since the emitted Go
// types would hold one another as struct values, which Go rejects; it keeps chain
// when the step pushes none. Any other field starts afresh, holding the step's
// nodes alone. A top-level element's walk starts as a step that is not a value
// field, and each branch of a sealed interface and the particles of an
// occurrence type (occurrence) start empty. The decline names decl and the
// repeated node, and cites decl's Loc: for an <element ref> particle, which
// carries none, the Loc of the top-level declaration it names.
func valueChain(chain []valueNode, value bool, decl xsd.ElementDeclaration, t xsd.ComplexType) ([]valueNode, error) {
	nodes := valueNodes(decl, t)
	if !value {
		return nodes, nil
	}
	for _, n := range nodes {
		if slices.Contains(chain, n) {
			return nil, decline(decl.Loc(), constructValueCycle, reacher(decl), n)
		}
	}
	return slices.Concat(chain, nodes), nil
}

// reacher names the element declaration decl, reached by a value field, in a
// struct-value recursion decline: a global decl is reached so only through an
// <element ref> particle, since a top-level element's own step is not a value
// field.
func reacher(decl xsd.ElementDeclaration) string {
	if decl.ScopeVariety() == xsd.ScopeGlobal {
		return "<element ref> particle to element " + decl.Name().String()
	}
	return "local element " + decl.Name().String()
}

// valueCycle walks, through value fields only, the particles of the complex type t that the element declaration
// decl, local or the top-level one an <element ref> particle names, reaches by a value field from particles
// whose chain is chain, applying valueChain at each field; t's struct's scope is nested (structScope), and t is
// named to the checks it calls as decl's elementOrigin or localOrigin names it. It walks the particles spliced
// returns for t's content particle's group, a <group ref> content particle followed by contentGroup, the list
// particleFields gives fields for, so a sequence or an all group spliced into its parent, inline or through a
// <group ref>, is a value edge that pushes no node, and every other nested model group particle (modelGroupTerm)
// is a leaf, passed before elementTerm: its field holds a slice, a pointer to its occurrence struct, or a bare
// sealed interface (groupField), never a value. A choice t, a t whose content particle occurs other than exactly
// once, and a step whose first node (valueNodes) is in e.acyclic, end the walk: a content particle splices
// answers true for though it occurs other than exactly once is emptiable, so no element particle among its
// spliced particles occurs exactly once, and it never steps to an element of type xs:anyType, whose AnyType
// reaches itself only through slices. A wildcard particle (wildcardTerm) is a leaf, passed before elementTerm:
// its field holds AnyElement, which reaches AnyType, which reaches itself only through the slice Children. So is
// an <element ref> particle to a head goshape.SubstitutionSum answers a sum for, passed after elementTerm: its field
// holds the group's sealed interface (substitutionType). So is an element particle whose declaration governingTypes answers a
// sum for, passed after that: its field holds the governing types' sealed interface (tableType); a declaration with one
// governing type steps to that type, which declaredType returns. So is an element particle whose declaration is nillable
// (nillable), passed with it: its field holds the sealed interface Nillable (nillableType). So is one whose declaration
// abstractLocal answers true for, passed with it: field declines it, so no field holds its type. It reads the schema, never
// e.emitted, e.elements or e.substitutions, so a type the emitting walk shares, and the type of a declaration an <element
// ref> particle names, is walked again here unless e.acyclic holds its step's first node; that node joins e.acyclic once the
// walk of t's particles reaches the end, and a step that pushes none, a local decl of an anonymous t that a complex type
// declares, is never memoized. It returns only a struct-value recursion: every other failure on its path ends that path,
// since the emitting walk meets it.
func (e *emitter) valueCycle(chain []valueNode, decl xsd.ElementDeclaration, nested scope, t xsd.ComplexType) error {
	subject := e.localOrigin(decl, nested, t).subject
	if decl.ScopeVariety() == xsd.ScopeGlobal {
		subject = e.elementOrigin(decl, t).subject
	}
	nodes := valueNodes(decl, t)
	if len(nodes) > 0 {
		if _, done := e.acyclic[nodes[0]]; done {
			return nil
		}
	}
	content, err := e.contentGroup(subject, t)
	if err != nil || content != nil && (content.group.Compositor() == xsd.CompositorChoice || content.occurs != exactlyOnce) {
		//nolint:nilerr // a decline of t ends this path: the emitting walk meets it and returns it in walk order.
		return nil
	}
	chain, err = valueChain(chain, true, decl, t)
	if err != nil {
		return err
	}
	var particles []xsd.Particle
	if content != nil {
		particles, err = e.spliced(subject, t.Loc(), content.group.Particles())
	}
	if err != nil {
		//nolint:nilerr // a <group ref> that does not resolve ends this path: the emitting walk meets it in particleFields.
		return nil
	}
	for _, p := range particles {
		if p.Occurs() != exactlyOnce || wildcardTerm(p.Term()) {
			continue
		}
		if _, isGroup, err := e.modelGroupTerm(subject, t.Loc(), p.Term()); err != nil || isGroup {
			continue
		}
		el, err := e.elementTerm(subject, t.Loc(), p.Term())
		if err != nil {
			continue
		}
		if _, sum := goshape.SubstitutionSum(e.schema, el); sum {
			continue
		}
		if _, sum, err := e.governingTypes(el); err != nil || sum || nillable(el) || abstractLocal(el) {
			continue
		}
		td, err := e.declaredType(el)
		if err != nil {
			continue
		}
		ct, isComplex := td.(xsd.ComplexType)
		if isAnyType(td) || !isComplex {
			continue
		}
		if err := e.valueCycle(chain, el, structScope(nested, el, ct), ct); err != nil {
			return err
		}
	}
	if len(nodes) > 0 {
		e.acyclic[nodes[0]] = struct{}{}
	}
	return nil
}

// elementTerm returns the element declaration a particle of a model group
// holds as its term: a local declaration, or the top-level one an <element ref>
// particle names (§3.3.2.4 ref.elt.global), followed through
// [xsd.Schema.Element]. A wildcard (wildcardTerm) or a model group, inline or
// the {model group} a <group ref> names (modelGroupTerm), is an error, never a
// decline: every caller routes both elsewhere first (field's wildcard and
// group arms, alternative's, valueCycle's leaf skips). subject and loc name
// the complex type holding the particle.
func (e *emitter) elementTerm(subject string, loc xsderr.Loc, term xsd.TermOrRef) (xsd.ElementDeclaration, error) {
	group, isGroup, err := e.modelGroupTerm(subject, loc, term)
	if err != nil {
		return xsd.ElementDeclaration{}, err
	}
	if isGroup {
		return xsd.ElementDeclaration{}, fmt.Errorf("%s at %s: a %s model group particle reached elementTerm", subject, loc, group.Compositor())
	}
	switch term := term.(type) {
	case xsd.ElementDeclarationRef:
		// The named top-level declaration alone: the declarations an instance
		// may carry in its place are its substitution group (cvc-accept clause
		// 2.3.2), which field types the particle over through refType's
		// goshape.SubstitutionSum, an abstract declaration among them getting no
		// branch. A head blocking substitution admits only itself.
		decl, ok := e.schema.Element(term.Name)
		if !ok {
			return xsd.ElementDeclaration{}, fmt.Errorf("%s at %s: its <element ref> particle to %s does not resolve", subject, loc, term.Name)
		}
		return decl, nil
	case xsd.ResolvedTerm:
		switch inner := term.Term.(type) {
		case xsd.ElementDeclaration:
			return inner, nil
		case xsd.Wildcard:
			return xsd.ElementDeclaration{}, fmt.Errorf("%s at %s: a wildcard particle reached elementTerm", subject, loc)
		}
	}
	return xsd.ElementDeclaration{}, fmt.Errorf("%s at %s: unexpected particle term %T", subject, loc, term)
}
