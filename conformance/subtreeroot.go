package conformance

import (
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kud360/goxsd8/internal/xmldecl"
	"github.com/kud360/goxsd8/internal/xmlenc"
	"github.com/kud360/goxsd8/internal/xmltok"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/parser/xmltree"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xpath"
	"github.com/kud360/goxsd8/xsd"
)

// This file holds the instance lane's one shape gate for the "valid"
// observation: an ASSESSED SUBTREE ROOT (#1841), a validation root, with or
// without content (#1855), whose every element the walk strictly assessed
// against a type it really has, through a declaration, through an xsi:type for
// a wildcard's child resolving none (#1978) or for a root resolving none
// (#2156), or through its ·locally declared type· or an xsi:type ·overriding·
// it for an {open content}'s child with one (#2080), or laxly assessed against
// ·xs:anyType· (#1911), and every attribute against its ·governing attribute
// declaration· where it has one (key-sva clause 2), with no clause of key-sva
// (§3.3.4.6), cvc-elt (§3.3.4.3), cvc-type (§3.3.4.4) or cvc-complex-type
// (§3.4.4.2) left undecided unrecorded. instance.go's "Why an EMPTY Result is
// evidence of validity for ONE shape only" states which clause each condition
// discharges; this file is only the conditions.
//
// The gate is computed per case and stored nowhere. It re-reads the instance
// with internal/xmltok, decoding a byte-order-marked UTF-16 document and admitting
// a 1.x version label as parser/xmltree does (rawDecoder), and re-derives every
// child's ·attribution· through xsd.Schema.ContentMatcher, independently of the
// walk, so it never rests on what the walk did or did not record for a
// descendant — save for a strict ·wildcard particle·'s child resolving to no
// declaration and carrying no xsi:type, which it admits unread because the walk
// charges that child's parent for it (subtreeGate.resolvedChild).

// versioningNS is the XML Schema versioning namespace §4.2.2 reads vc:minVersion,
// vc:maxVersion, vc:typeAvailable, vc:typeUnavailable, vc:facetAvailable and
// vc:facetUnavailable from. parser keeps its own copy unexported; the lane
// declares the name here rather than parser exporting it for one harness gate,
// closureVersioned, whose GAP(conformance) marker states why it is held.
const versioningNS = "http://www.w3.org/2007/XMLSchema-versioning"

// assessedSubtreeRoot admits the instance document at doc, against schema as
// assembled into report, where it has the assessed-subtree-root shape an empty
// validate.Result may be read as "valid" for: a root, with content or without,
// whose subtree meets every condition subtreeGate.root names, in a document
// whose DTD, if any, defaults no attribute (rootStart), against an assembly no
// version condition touched. It answers the zero refusal where it admits the
// document, and otherwise the refusal naming the first condition it could not
// establish — a versioned assembly, an unreadable document, a decoder error,
// an unresolvable component — never a guess.
//
// backend is the one the walk was given (execInstanceCase's validate.New), in
// which a Type Alternative's {test} evaluates (subtreeGate.selectedType).
func assessedSubtreeRoot(backend value.Backend, schema *xsd.Schema, report *parser.AssemblyReport, doc string) refusal {
	if closureVersioned(report, doc) {
		return refuseVersioned
	}
	rc, _, err := loader.Dir(filepath.Dir(doc)).Resolve("", filepath.Base(doc))
	if err != nil {
		return refuseGateUnresolved
	}
	defer func() { _ = rc.Close() }() // read-only handle: close error cannot affect the verdict
	dec := rawDecoder(rc)
	root, why := rootStart(dec)
	if why != "" {
		return why
	}
	g := subtreeGate{schema: schema, backend: backend, dec: dec}
	if why := g.root(root); why != "" {
		return why
	}
	return documentEnd(dec)
}

// root reads the document element, whose start tag is start, through to its
// end tag, and refuses unless it meets element's conditions against its
// ·governing element declaration· or, with none, undeclaredRoot's. cvc-elt
// clause 1 (§3.3.4.6 ·governing element declaration· clause 4): the root's
// declaration is the top-level one its ·expanded name· resolves to. The
// ·validation root· has no [inherited attributes] (key-p-inherited clause 2).
func (g *subtreeGate) root(start xml.StartElement) refusal {
	d, ok := g.schema.Element(expandedName(start.Name))
	if !ok {
		return g.undeclaredRoot(start)
	}
	return g.element(start, d, nil)
}

// undeclaredRoot reads through to its end tag a document element whose start
// tag is start and which has no ·governing element declaration·, and refuses
// unless it carries an xsi:type naming a top-level type definition T against
// its own namespace declarations (instanceType), and it and its subtree meet
// governed's conditions against T. T is its ·governing type definition·
// (key-governing-type-elem clause 8, key-itd), so it is ·strictly assessed·
// against T (cvc-assess-elt clause 1) and cvc-type decides it, as the walk
// assesses it: the gate's reading against T is the one a declared root's
// governing type gets. With no declaration there is no ·selected type
// definition· for T to ·override·, so governingType's cvc-elt clause 4 test has
// no counterpart here, and no cvc-elt clause is evaluated for it.
//
// It is never ·nilled·: key-nilled needs a declaration whose {nillable} is
// true, so an xsi:nil it carries has no effect, whatever its value, and
// cvc-type and cvc-complex-type clause 1 read it as if absent. An xsi:nil with
// no ·actual value· is the walk's cvc-attribute clause 3 charge (nilValue). A
// T that is ·xs:error· is the walk's cvc-type clause 3.1.3 charge, there being
// no ·nilled· reading to except it, so element's refuseErrorType has no
// counterpart here either.
//
// A root with no xsi:type, or one naming no type definition, has no ·governing
// type definition· and is refused (refuseUndeclaredRoot). The walk charges
// cvc-assess-elt for such a root, so no empty Result reaches the gate with one,
// and the refusal keeps the gate from reading "valid" off a root it cannot
// type.
func (g *subtreeGate) undeclaredRoot(start xml.StartElement) refusal {
	i := slices.IndexFunc(start.Attr, func(a xml.Attr) bool { return a.Name == xsiType })
	if i < 0 {
		return refuseUndeclaredRoot
	}
	defer g.enter(start)()
	t, why := g.instanceType(start.Attr[i].Value)
	if why != "" {
		return refuseUndeclaredRoot
	}
	return g.governed(start, t, false, nil)
}

// documentEnd reads dec past the document element to the end of the document,
// refusing a directive (refuseEpilogDirective) or a decoder error
// (refuseDecode) there.
func documentEnd(dec *xmltok.Decoder) refusal {
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return ""
		}
		if err != nil {
			return refuseDecode
		}
		if _, ok := tok.(xml.Directive); ok {
			return refuseEpilogDirective
		}
	}
}

// subtreeGate is one assessedSubtreeRoot reading: the schema the subtree is
// checked against, the backend a {test} evaluates in, the decoder positioned
// inside it, and the namespace declarations of every element open above that
// position, outermost first, which resolveQName reads.
//
// Each of its reading methods answers a refusal: the zero value where the
// conditions its doc comment names hold, and otherwise the token
// (instance.go's refuse* constants) naming the first exit the reading
// reached that refused. A decoder error is refuseDecode wherever it arrives.
//
// Every method that reads an element takes inherited, that element's
// [inherited attributes] (§3.3.5.6, e-inherited_attributes) as handedDown
// composes them, which only selectedType reads.
type subtreeGate struct {
	schema  *xsd.Schema
	backend value.Backend
	dec     *xmltok.Decoder
	scope   []xml.Attr
}

// element reads the element whose start tag is start, whose ·governing element
// declaration· is d and whose [inherited attributes] are inherited, through to
// its end tag, and refuses unless every condition below holds for it and,
// recursively, for every element under it:
//
//   - an xsi:nil it carries has an ·actual value· (nilValue,
//     refuseNilLexical);
//   - its ·selected type definition· is determined (selectedType): d.{type
//     definition}, or the type d.{type table} ·conditionally selects·;
//   - its ·governing type definition· is determined: an xsi:type the element
//     carries meets governingType's conditions against the selected type, the
//     type it names then standing in for it;
//   - that type is not ·xs:error· (refuseErrorType): §3.16.7.3 says an item it
//     governs "will be invalid", which the walk charges as cvc-type clause
//     3.1.3 for an element that is not ·nilled· and, as that clause reads, not
//     for one that is, so the gate claims neither;
//   - the element and its subtree meet governed's conditions against that
//     type, the element being ·nilled· (key-nilled) where d.{nillable} is true
//     and its xsi:nil's ·actual value· is true.
//
// An xsi:nil is otherwise the walk's: validate's nilCheck charges cvc-elt
// clause 3.1 for one on a declaration whose {nillable} is false, whatever its
// value, and clause 3.2.3.2 for a ·nilled· element under a fixed {value
// constraint}; its contentCheck charges clause 3.2.3.1 for a ·nilled· element's
// character or element [[child]]. An xsi:nil whose ·actual value· is false is
// clause 3.2.2, which holds as 3.2.1 does, and the element is read as if it
// carried none.
//
// No property of d is refused by itself. {abstract} is cvc-elt clause 2, which
// the walk charges at every element (validate's walk.abstractDeclaration), so
// an abstract d never reaches the gate in an empty Result. A {type table} is
// read through selectedType, and clause 4 against the type it selects through
// governingType. A {value constraint} of either variety is clause 5.1 (an
// element with no [[children]]) or 5.2.2 (a fixed one over [[children]]),
// which the walk settles (validate's contentCheck.defaultValid and
// contentCheck.fixedValue), recording the one comparison it declines.
// {nillable} is clause 3, the walk's as above. {identity-constraint
// definitions} are clause 6 (cvc-identity-constraint, §3.11.4), the walk's,
// which records every check it declines.
func (g *subtreeGate) element(start xml.StartElement, d xsd.ElementDeclaration, inherited []xml.Attr) refusal {
	isNil, ok := nilValue(start.Attr)
	if !ok {
		return refuseNilLexical
	}
	defer g.enter(start)()
	selected, why := g.selectedType(start, d, inherited)
	if why != "" {
		return why
	}
	td, why := g.governingType(start, d, selected)
	if why != "" {
		return why
	}
	if td.Name() == errorTypeName {
		return refuseErrorType
	}
	return g.governed(start, td, isNil && d.Nillable(), inherited)
}

// governed reads through to its end tag an element whose start tag is start,
// whose ·governing type definition· is td and whose [inherited attributes] are
// inherited, ·nilled· where nilled is true, and refuses unless:
//
//   - for a Simple Type Definition, the element carries no attribute but the
//     four xsi: ones cvc-type clause 3.1.1 excepts (refuseSimpleAttribute),
//     and no element [[child]] (leaf);
//   - for a Complex Type Definition, it meets complex's conditions.
//
// A td that is neither is refuseTypeKind.
func (g *subtreeGate) governed(start xml.StartElement, td xsd.TypeDefinition, nilled bool, inherited []xml.Attr) refusal {
	switch t := td.(type) {
	case *xsd.SimpleType:
		if slices.ContainsFunc(start.Attr, notExcepted) {
			return refuseSimpleAttribute
		}
		return g.leaf()
	case xsd.ComplexType:
		return g.complex(start, t, nilled, inherited)
	}
	return refuseTypeKind
}

// enter pushes the namespace declarations start carries onto g.scope, and
// returns the function that pops them again at start's end tag.
func (g *subtreeGate) enter(start xml.StartElement) func() {
	mark := len(g.scope)
	for _, a := range start.Attr {
		if isNamespaceDeclaration(a) {
			g.scope = append(g.scope, a)
		}
	}
	return func() { g.scope = g.scope[:mark] }
}

// selectedType is the ·selected type definition· (§3.3.4.1, key-selected-type)
// of the element whose start tag is start, whose ·governing element
// declaration· is d and whose [inherited attributes] are inherited, determined
// here independently of the walk — validate's walk.selectedType and
// walk.conditionallySelected are its counterparts — and a refusal wherever the
// gate does not determine it. Clause 2: d with no {type table} selects d.{type
// definition}. Clause 1: d.{type table} ·conditionally selects· it
// (key-cta-select): the {alternatives} are tried in order, the first whose
// {test} evaluates to true (key-cta-ta-select) supplies it, and with none, the
// {default type definition}'s {type definition} does. Every {test} reads
// ctaAttributes' sequence, through xpath.CompileCTATest and
// xpath.CTATest.Evaluate, the pair the walk evaluates with, in the walk's
// backend; a dynamic or type error inside an evaluable {test} is the false
// key-cta-ta-select clause 2 makes it, inside Evaluate.
//
// The scan stops at the first alternative whose {test} is true, so one behind
// it is never compiled and costs nothing. One CompileCTATest declines before
// any succeeds leaves the selection undetermined, the walk's withhold, and the
// gate refuses it (refuseTypeTableUndecided) rather than fall through to a
// type the table may not select. So does an attribute value ctaAttributes
// cannot read as its [[normalized value]] (refuseTypeTableWhitespace), at the
// first {test} evaluated. A selected {type definition} that does not resolve
// is refuseTypeUnresolved.
func (g *subtreeGate) selectedType(start xml.StartElement, d xsd.ElementDeclaration, inherited []xml.Attr) (xsd.TypeDefinition, refusal) {
	table, tabled := d.TypeTable()
	if !tabled {
		return g.resolvedType(d.TypeDefinition())
	}
	attrs, normalized := ctaAttributes(start, inherited)
	for _, alt := range table.Alternatives() {
		test, _ := alt.Test()
		compiled, evaluable := xpath.CompileCTATest(test, g.schema)
		if !evaluable {
			return nil, refuseTypeTableUndecided
		}
		if !normalized {
			return nil, refuseTypeTableWhitespace
		}
		if compiled.Evaluate(g.backend, g.schema, attrs) {
			return g.resolvedType(alt.TypeDefinition())
		}
	}
	return g.resolvedType(table.DefaultTypeDefinition().TypeDefinition())
}

// resolvedType is the type definition ref names in g.schema, and
// refuseTypeUnresolved where it names none.
func (g *subtreeGate) resolvedType(ref xsd.TypeDefinitionOrRef) (xsd.TypeDefinition, refusal) {
	t, ok := g.schema.ResolvedType(ref)
	if !ok {
		return nil, refuseTypeUnresolved
	}
	return t, ""
}

// ctaAttributes is the attribute sequence a {test} evaluates against for the
// element whose start tag is start and whose [inherited attributes] are
// inherited — validate's ctaAttributes, spelled the same: first the element's
// own [[attributes]] in source order, xsi: ones included and namespace
// declarations not (key-cta-ta-select clause 1.1.2), then each member of
// inherited whose ·expanded name· none of those has (clause 1.1.3).
//
// normalized is false where a value in the sequence holds #x9, #xA or #xD. A
// {test} reads the [[normalized value]] (XML 1.0 §3.3.3), which maps a literal
// white-space character to #x20 and keeps one a character reference names,
// and encoding/xml hands both over alike, unmapped, so such a value is not
// read as either.
func ctaAttributes(start xml.StartElement, inherited []xml.Attr) (xpath.Attributes, bool) {
	var seq []xml.Attr
	for _, a := range start.Attr {
		if !isNamespaceDeclaration(a) {
			seq = append(seq, a)
		}
	}
	own := len(seq)
	for _, a := range inherited {
		if !attributeNamed(seq[:own], a.Name) {
			seq = append(seq, a)
		}
	}
	normalized := !slices.ContainsFunc(seq, func(a xml.Attr) bool { return strings.ContainsAny(a.Value, "\t\n\r") })
	return func(yield func(xsd.QName, string) bool) {
		for _, a := range seq {
			if !yield(expandedName(a.Name), a.Value) {
				return
			}
		}
	}, normalized
}

// attributeNamed reports whether attrs holds an attribute named n.
func attributeNamed(attrs []xml.Attr, n xml.Name) bool {
	return slices.ContainsFunc(attrs, func(a xml.Attr) bool { return a.Name == n })
}

// governingType is the ·governing type definition· (key-governing-type-elem) of
// the element whose start tag is start, whose ·governing element declaration·
// is d and whose ·selected type definition· is selected (selectedType),
// determined here independently of the walk, and a refusal wherever the gate
// does not determine it.
//
// With no xsi:type the selected type governs (clause 4). With one, the gate
// follows its ·instance-specified type definition· T (clause 3) where all of
// these hold, and refuses otherwise:
//
//   - the lexical names a type: it resolves as a QName against the namespace
//     bindings in scope (§3.17.6.3, cvc-resolve-instance) and the schema has a
//     top-level type definition of that name. A lexical that does not is the
//     walk's, charged under cvc-attribute clause 3 or 5 or, where the walk
//     withholds clause 3, recorded in Result.Unevaluated, so no empty Result
//     reaches the gate with it (refuseXsiTypeUnresolved);
//   - xsd.Schema.ValidlySubstitutable answers that T ·overrides· the selected
//     type under d.{disallowed substitutions} (§3.3.4.2, key-overrides), which
//     is cvc-elt clause 4. A false is the walk's cvc-elt charge
//     (refuseXsiTypeNotOverride). An error is refused
//     (refuseXsiTypeUndecided): validate's instanceOverride records it in
//     Result.Unevaluated and leaves the governing type undetermined.
func (g *subtreeGate) governingType(start xml.StartElement, d xsd.ElementDeclaration, selected xsd.TypeDefinition) (xsd.TypeDefinition, refusal) {
	i := slices.IndexFunc(start.Attr, func(a xml.Attr) bool { return a.Name == xsiType })
	if i < 0 {
		return selected, ""
	}
	t, why := g.instanceType(start.Attr[i].Value)
	if why != "" {
		return nil, why
	}
	overrides, err := g.schema.ValidlySubstitutable(t, selected, d.DisallowedSubstitutions())
	if err != nil {
		return nil, refuseXsiTypeUndecided
	}
	if !overrides {
		return nil, refuseXsiTypeNotOverride
	}
	return t, ""
}

// instanceType is the top-level type definition an xsi:type whose value is
// lexical names, resolved against the namespace bindings in g.scope
// (resolveQName), and refuseXsiTypeUnresolved where lexical resolves to no
// QName or the schema has no type of that name.
func (g *subtreeGate) instanceType(lexical string) (xsd.TypeDefinition, refusal) {
	name, ok := resolveQName(g.scope, lexical)
	if !ok {
		return nil, refuseXsiTypeUnresolved
	}
	t, ok := g.schema.Type(name)
	if !ok {
		return nil, refuseXsiTypeUnresolved
	}
	return t, ""
}

// resolveQName maps lexical, an xs:QName lexical, to the ·expanded name· the
// namespace declarations in scope bind it to after whiteSpace collapse, the
// last declaration of a prefix winning: an unprefixed name takes the default
// namespace, or none where no default namespace declaration is in scope. A
// lexical with an empty prefix or local part, or whose prefix no declaration
// in scope binds, answers false; any other malformation yields a name no type
// definition carries. Only namespace declarations in scope are read, so a
// start tag's whole attribute list may be passed.
func resolveQName(scope []xml.Attr, lexical string) (xsd.QName, bool) {
	prefix, local, prefixed := strings.Cut(strings.Trim(lexical, " \t\r\n"), ":")
	decl := xml.Name{Space: "xmlns", Local: prefix}
	if !prefixed {
		prefix, local = "", prefix
		decl = xml.Name{Local: "xmlns"}
	}
	if local == "" || (prefixed && prefix == "") {
		return xsd.QName{}, false
	}
	for i := len(scope) - 1; i >= 0; i-- {
		if scope[i].Name == decl {
			return xsd.QName{Space: scope[i].Value, Local: local}, true
		}
	}
	return xsd.QName{Local: local}, !prefixed
}

// xsiType and xsiNil are the ·expanded names· of the built-in xsi:type and
// xsi:nil attribute declarations (§3.2.7).
var (
	xsiType = xml.Name{Space: xsd.XMLSchemaInstanceNS, Local: "type"}
	xsiNil  = xml.Name{Space: xsd.XMLSchemaInstanceNS, Local: "nil"}
)

// anyTypeName and errorTypeName are the ·expanded names· of ·xs:anyType· and
// ·xs:error· (§3.16.7.3, key-error).
var (
	anyTypeName   = xsd.QName{Space: xsd.XMLSchemaNS, Local: "anyType"}
	errorTypeName = xsd.QName{Space: xsd.XMLSchemaNS, Local: "error"}
)

// nilValue reports the ·actual value· of the xsi:nil attrs, one start tag's
// attribute list, carries — false where it carries none — and false for ok
// where its lexical has no ·actual value·: after the whiteSpace collapse
// xs:boolean fixes, it is none of boolean-lexical-mapping's four literals
// (Datatypes §3.3.2.2), the type §3.2.7's built-in declaration gives xsi:nil.
//
// The walk charges such a lexical on every element it assesses, so no empty
// Result reaches the gate with one: under a ·governing element declaration· as
// cvc-elt clause 3.1 or 3.2 (validate's nilCheck), and on an element with none
// — ·laxly assessed·, or ·strictly assessed· against an xsi:type or a ·locally
// declared type· — as cvc-attribute clause 3 against the built-in declaration
// that governs the attribute (key-governing-ad, validate's
// walk.instanceNilLexical). An element it leaves undecided is assessed against
// nothing and charged nothing, and arrives only beside a decline already in the
// Result (#2159, #1892). element alone reads the value, for key-nilled, and
// refuses a lexical it cannot read rather than guess one.
func nilValue(attrs []xml.Attr) (value, ok bool) {
	i := slices.IndexFunc(attrs, func(a xml.Attr) bool { return a.Name == xsiNil })
	if i < 0 {
		return false, true
	}
	switch strings.Trim(attrs[i].Value, " \t\r\n") {
	case "true", "1":
		return true, true
	case "false", "0":
		return false, true
	}
	return false, false
}

// notExcepted reports whether a is an attribute information item that neither
// a namespace declaration is nor one of the four xsi: attributes (§3.2.7)
// cvc-type clause 3.1.1 and cvc-complex-type clause 2 except by name.
func notExcepted(a xml.Attr) bool {
	if isNamespaceDeclaration(a) || isLocationHint(a) {
		return false
	}
	return a.Name.Space != xsd.XMLSchemaInstanceNS || (a.Name.Local != "type" && a.Name.Local != "nil")
}

// complex reads an element governed by the Complex Type Definition t, ·nilled·
// where nilled is true, through to its end tag, and refuses unless:
//
//   - every one of t.{attribute uses} resolves to an {attribute declaration}
//     (refuseAttributeUse) that recordedAttributeType admits, present on the
//     element or not;
//   - every attribute the element carries that notExcepted names matches one
//     of those uses by ·expanded name· (cvc-complex-type clause 2.1) or meets
//     wildcardAttribute's conditions (clause 2.2);
//   - for a ·nilled· element, there is no element [[child]]: cvc-complex-type
//     clause 1 applies only to an element that is not ·nilled·, so the
//     {content type} is not read, and cvc-elt clause 3.2.3.1, no character or
//     element [[child]], is the walk's (validate's contentCheck);
//   - otherwise, under a simple or an empty {content type}, there is no
//     element [[child]];
//   - otherwise, under an element-only or mixed one, xsd.Schema.ContentMatcher
//     decides it (refuseContentMatcher) and every element [[child]] meets
//     child's conditions, each child's [inherited attributes] being what
//     handedDown composes from the element's start tag, t and inherited, the
//     element's own.
//
// A {content type} of none of these kinds is refuseContentType.
func (g *subtreeGate) complex(start xml.StartElement, t xsd.ComplexType, nilled bool, inherited []xml.Attr) refusal {
	uses := t.AttributeUses()
	for _, u := range uses {
		ad, ok := g.schema.ResolvedAttributeDeclaration(u)
		if !ok {
			return refuseAttributeUse
		}
		if why := g.recordedAttributeType(ad); why != "" {
			return why
		}
	}
	for _, a := range start.Attr {
		if !notExcepted(a) {
			continue
		}
		n := expandedName(a.Name)
		if slices.ContainsFunc(uses, func(u xsd.AttributeUse) bool { return u.DeclarationName() == n }) {
			continue
		}
		if why := g.wildcardAttribute(t, n); why != "" {
			return why
		}
	}
	if nilled {
		return g.leaf()
	}
	switch t.ContentType().(type) {
	case xsd.SimpleContent, xsd.EmptyContent:
		return g.leaf()
	case xsd.ElementContent:
		m, ok := g.schema.ContentMatcher(t)
		if !ok {
			return refuseContentMatcher
		}
		return g.children(t, m, g.handedDown(start, t, inherited))
	}
	return refuseContentType
}

// handedDown is the [inherited attributes] of every element [[child]] of the
// element whose start tag is start, whose ·governing type definition· is t and
// whose own [inherited attributes] are inherited (§3.3.5.6,
// e-inherited_attributes) — validate's walk.handedDown, composed in its order
// and with its shadowing: first the element's own attributes that are
// ·potentially inherited· (key-p-inherited, inheritable), then each ·defaulted
// attribute· (key-dflt-att) whose use's {inheritable} is true, then each member
// of inherited whose ·expanded name· none of those has. An attribute that is not
// ·potentially inherited· shadows nothing.
//
// A ·laxly assessed· element reaches here with t ·xs:anyType·, whose empty
// {attribute uses} and lax {attribute wildcard} (§3.4.7) leave inheritable the
// top-level reading the walk gives an element with no ·governing type
// definition·.
func (g *subtreeGate) handedDown(start xml.StartElement, t xsd.ComplexType, inherited []xml.Attr) []xml.Attr {
	var own []xml.Attr
	for _, a := range start.Attr {
		if !isNamespaceDeclaration(a) && g.inheritable(t, a.Name) {
			own = append(own, a)
		}
	}
	for _, u := range t.AttributeUses() {
		if !g.schema.ResolvedInheritable(u) {
			continue
		}
		vc, defaulted := g.defaulted(start, u)
		if !defaulted {
			continue
		}
		n := u.DeclarationName()
		own = append(own, xml.Attr{Name: xml.Name{Space: n.Space, Local: n.Local}, Value: vc.LexicalForm()})
	}
	nearest := len(own)
	for _, a := range inherited {
		if !attributeNamed(own[:nearest], a.Name) {
			own = append(own, a)
		}
	}
	return own
}

// inheritable is key-p-inherited clause 3 for an attribute named n on an
// element governed by t — validate's walk.inheritable: the {inheritable} of
// the t.{attribute uses} member n matches (clause 3.1), false where t's skip
// {attribute wildcard} admits it unmatched (·skipped·, key-skipped, assessed
// against no declaration), and otherwise the {inheritable} of the top-level
// declaration n ·resolves· to, false where it resolves to none (clause 3.2).
func (g *subtreeGate) inheritable(t xsd.ComplexType, name xml.Name) bool {
	n := expandedName(name)
	uses := t.AttributeUses()
	if i := slices.IndexFunc(uses, func(u xsd.AttributeUse) bool { return u.DeclarationName() == n }); i >= 0 {
		return g.schema.ResolvedInheritable(uses[i])
	}
	if wild, ok := t.AttributeWildcard(); ok && wild.ProcessContents() == xsd.ProcessSkip && g.schema.AllowsAttributeWildcardName(wild, n) {
		return false
	}
	d, ok := g.schema.Attribute(n)
	return ok && d.Inheritable()
}

// defaulted reports whether u, a member of the {attribute uses} of the type
// governing the element whose start tag is start, supplies that element a
// ·defaulted attribute· (key-dflt-att clauses 2 to 5), and if so its
// ·effective value constraint· — validate's walk.defaultedConstraint.
func (g *subtreeGate) defaulted(start xml.StartElement, u xsd.AttributeUse) (xsd.ValueConstraint, bool) {
	if u.Required() {
		return xsd.ValueConstraint{}, false
	}
	vc, constrained := g.schema.EffectiveValueConstraint(u)
	if !constrained {
		return xsd.ValueConstraint{}, false
	}
	n := u.DeclarationName()
	name := xml.Name{Space: n.Space, Local: n.Local}
	// Clause 4: none of the four xsi: names (§3.2.7), which notExcepted
	// excepts.
	if !notExcepted(xml.Attr{Name: name}) {
		return xsd.ValueConstraint{}, false
	}
	if attributeNamed(start.Attr, name) { // clause 5
		return xsd.ValueConstraint{}, false
	}
	return vc, true
}

// recordedAttributeType refuses an ad whose {type definition} does not resolve
// to a simple type (refuseAttributeType): the one condition the gate puts on
// an attribute declaration the walk assesses an attribute against, an
// {attribute uses} member's or a wildcard-resolved one's.
func (g *subtreeGate) recordedAttributeType(ad xsd.AttributeDeclaration) refusal {
	if _, ok := g.schema.ResolvedSimpleType(ad.TypeDefinition()); !ok {
		return refuseAttributeType
	}
	return ""
}

// wildcardAttribute admits an attribute named n that matches none of
// t.{attribute uses} where its assessment is one the walk decides or records
// (validate's walk.unmatchedAttribute and walk.wildcardAttribute): t has an
// {attribute wildcard} that admits n (cvc-complex-type clause 2.2, cvc-wildcard
// §3.10.4.1, xsd.Schema.AllowsAttributeWildcardName), and one of these holds:
//
//   - its {process contents} is skip: the attribute is ·skipped· (key-skipped),
//     assessed against nothing, and key-sva (§3.3.4.6) clause 2.2 leaves it
//     unassessed with nothing to decide;
//   - n ·resolves· to a top-level attribute declaration (key-governing-ad
//     clause 3) that recordedAttributeType admits, and the attribute's
//     ·locally declared type· within t is ·absent· (locallyDeclaredAttribute
//     answers false), which makes cvc-complex-type clause 5 vacuous for it:
//     the walk charges or records cvc-attribute clauses 3 and 4 against the
//     declaration, under lax and strict alike;
//   - n resolves to none under lax: the attribute has no ·governing attribute
//     declaration· and is not assessed, which charges nothing (sic-e-outcome
//     clause 1.1.3 reaches only a ·wildcard particle·), and clause 5 is
//     vacuous for want of a ·governing type definition·.
//
// It refuses n resolving to a declaration where the ·locally declared type· is
// not ·absent· — a base's attribute use a restriction prohibited (key-ldt-att
// case 3): clause 5 then asks the declaration's {type definition} to be
// ·validly substitutable· for that use's, which the walk never checks
// (refuseAttributeLDT). It refuses n resolving to none under strict
// (refuseStrictAttribute). The walk charges nothing there either, on the same
// reading, and records nothing, a reading the suite does not share for seven
// of its invalid cases (#1912). A name the wildcard does not admit, or a type
// with no {attribute wildcard}, is refused too (refuseAttributeUnadmitted),
// though the walk charges clause 2 for both.
func (g *subtreeGate) wildcardAttribute(t xsd.ComplexType, n xsd.QName) refusal {
	wild, ok := t.AttributeWildcard()
	if !ok || !g.schema.AllowsAttributeWildcardName(wild, n) {
		return refuseAttributeUnadmitted
	}
	pc := wild.ProcessContents()
	if pc == xsd.ProcessSkip {
		return ""
	}
	ad, ok := g.schema.Attribute(n)
	if !ok {
		if pc == xsd.ProcessLax {
			return ""
		}
		return refuseStrictAttribute
	}
	if why := g.recordedAttributeType(ad); why != "" {
		return why
	}
	if g.locallyDeclaredAttribute(t, n) {
		return refuseAttributeLDT
	}
	return ""
}

// leaf reads an element's content through to its end tag and refuses an
// element started inside it (refuseElementChild).
func (g *subtreeGate) leaf() refusal {
	for {
		tok, err := g.dec.Token()
		if err != nil {
			return refuseDecode
		}
		switch tok.(type) {
		case xml.StartElement:
			return refuseElementChild
		case xml.EndElement:
			return ""
		}
	}
}

// children reads the content of an element governed by t through to its end
// tag, advancing m, t's ContentMatcher, over each element [[child]] in document
// order, and refuses unless every child meets child's conditions and m accepts
// the whole sequence (refuseContentIncomplete).
func (g *subtreeGate) children(t xsd.ComplexType, m *xsd.Matcher, inherited []xml.Attr) refusal {
	for {
		tok, err := g.dec.Token()
		if err != nil {
			return refuseDecode
		}
		switch s := tok.(type) {
		case xml.StartElement:
			if why := g.child(t, m, s, inherited); why != "" {
				return why
			}
		case xml.EndElement:
			if !m.Accepting() {
				return refuseContentIncomplete
			}
			return ""
		}
	}
}

// child reads one element [[child]] whose start tag is start, of an element
// governed by t, through to its end tag, and refuses unless m ·attributes· it
// (§3.4.4.4; refuseContentRejected) to one of these, and the child meets that
// arm's conditions:
//
//   - an element particle whose {term} carries the child's own ·expanded
//     name·: that {term} is its ·context-determined declaration· (§3.3.4.6
//     key-governing-ed clause 2), which with the child's subtree must meet
//     element's conditions. The ·locally declared type· (key-ldt-elem case 2)
//     is that declaration's own {type definition}, so cvc-complex-type clause
//     5 holds wherever cvc-elt clause 4 does for a declaration with no {type
//     table}. Under a {type table} it need not: a selection of ·xs:error·
//     (e-props-correct clause 7.2) fails it, and so can an xsi:type
//     ·overriding· a selection that is not the declared type. The walk charges
//     both (validate's walk.locallyDeclaredType), so no empty Result reaches
//     the gate with such a child;
//   - an element particle whose {term} D carries another name: cvc-accept
//     clause 2.3.2 admitted the child as a member of D's ·substitution group·,
//     the Matcher deciding D top-level, D.{disallowed substitutions}, and
//     cos-equiv-derived-ok-rec (§3.3.6.3) clauses 2.1 to 2.3 for the top-level
//     declaration S the child's name ·resolves· to (refuseMemberUnresolved
//     where it resolves to none). S is the child's ·context-determined
//     declaration· (key-governing-ed clause 2), and S with the child's subtree
//     must meet element's conditions. The ·locally declared type·
//     (key-ldt-elem case 2, S ·implicitly contained·, key-impl-cont) is S's
//     own {type definition}, so cvc-complex-type clause 5 holds wherever
//     cvc-elt clause 4 does, on the first arm's terms. A child carrying D's
//     own name is the first arm's, cvc-accept clause 2.3.1 attributing it to
//     D, where the walk charges an ·abstract· D (cvc-elt clause 2);
//   - a skip Wildcard, or the {open content} with a skip {wildcard}: the child
//     is ·skipped· with its whole subtree (key-sva clause 3.2, cvc-assess-elt
//     clause 2), which is read past unchecked. A skipped child has no
//     [validity], so it cannot block its parent's `valid` (sic-e-outcome
//     clauses 1.1.2 and 1.1.3, the latter naming a strict particle alone), and
//     with no ·governing type definition· it binds no ID or IDREF (key-eas
//     clause 3). Its NAME is still checked: m.Next admits it only where the
//     wildcard's namespace constraint does (cvc-wildcard clause 1, and
//     cvc-complex-content clause 2.4 or 3.4 for the {open content}). The
//     {open content} half is validate's reading, not the spec's words
//     (validate's walk.childGoverning, #1969);
//   - a strict or lax Wildcard, or the {open content} with a strict or lax
//     {wildcard} where the child's ·locally declared type· within t is ·absent·
//     (xsd.Schema.LocallyDeclaredElementType answers false): resolvedChild's
//     conditions;
//   - the {open content} with a strict or lax {wildcard} where that ·locally
//     declared type· is non-·absent·: localTyped's conditions against it,
//     resolved name or not, key-governing-ed clause 4.3 giving the child no
//     ·governing element declaration· (validate's walk.childGoverning).
//
// An attribution of none of these kinds is refuseAttribution.
func (g *subtreeGate) child(t xsd.ComplexType, m *xsd.Matcher, start xml.StartElement, inherited []xml.Attr) refusal {
	name := expandedName(start.Name)
	a, ok := m.Next(name)
	if !ok {
		return refuseContentRejected
	}
	switch at := a.(type) {
	case xsd.ElementDeclaration:
		if at.Name() == name {
			return g.element(start, at, inherited)
		}
		s, ok := g.schema.Element(name)
		if !ok {
			return refuseMemberUnresolved
		}
		return g.element(start, s, inherited)
	case xsd.Wildcard:
		pc := at.ProcessContents()
		if pc == xsd.ProcessSkip {
			return g.skip()
		}
		return g.resolvedChild(start, pc == xsd.ProcessStrict, inherited)
	case *xsd.OpenContent:
		if at.Wildcard().ProcessContents() == xsd.ProcessSkip {
			return g.skip()
		}
		if ldt, local := g.schema.LocallyDeclaredElementType(t, name); local {
			return g.localTyped(start, ldt, inherited)
		}
		return g.resolvedChild(start, false, inherited)
	}
	return refuseAttribution
}

// skip reads past the element whose start tag the decoder just returned,
// subtree and end tag included, unchecked, refusing a decoder error there.
func (g *subtreeGate) skip() refusal {
	if g.dec.Skip() != nil {
		return refuseDecode
	}
	return ""
}

// resolvedChild reads through to its end tag a child whose start tag is start,
// which is ·attributed to· a strict or lax Wildcard or to an {open content}
// with a strict or lax {wildcard} and an ·absent· ·locally declared type·
// within the parent's type, and refuses unless one of these holds:
//
//   - its ·expanded name· ·resolves· to a top-level element declaration d
//     (key-governing-ed clauses 3 and 4), and d and the child's subtree meet
//     element's conditions. cvc-complex-type clause 5 is the walk's: it
//     charges a child whose ·governing type definition· is neither the same
//     as nor ·validly substitutable· for its non-·absent· ·locally declared
//     type· within the parent's type (validate's walk.locallyDeclaredType),
//     so no empty Result reaches the gate with such a child;
//   - its name resolves to none and it carries an xsi:type: instanceTyped's
//     conditions, under a strict and a lax wildcard alike, its ·locally
//     declared type· ·absent· or not (key-governing-type-elem clause 8 or 6,
//     the latter read soundly through the walk's clause 5 charge);
//   - strictParticle holds — the child is ·attributed to· a strict ·wildcard
//     particle· — its name resolves to none, and it carries no xsi:type: the
//     child is ·laxly assessed· with a ·governing type definition· of none, so
//     clause 5 is vacuous, and the walk charges e-validity clause 1.1.3
//     (validate's walk.unresolvedStrictWildcardChild) for the parent, so no
//     empty Result reaches the gate with it and the gate reads nothing below
//     it;
//   - strictParticle does not hold — the child is ·attributed to· a lax
//     Wildcard or {open content} — its name resolves to none, it carries no
//     xsi:type, and laxlyAssessed holds for it (#1911).
func (g *subtreeGate) resolvedChild(start xml.StartElement, strictParticle bool, inherited []xml.Attr) refusal {
	d, ok := g.schema.Element(expandedName(start.Name))
	if ok {
		return g.element(start, d, inherited)
	}
	if i := slices.IndexFunc(start.Attr, func(a xml.Attr) bool { return a.Name == xsiType }); i >= 0 {
		return g.instanceTyped(start, start.Attr[i].Value, inherited)
	}
	if strictParticle {
		return g.skip()
	}
	return g.laxlyAssessed(start, inherited)
}

// instanceTyped reads through to its end tag a wildcard's child whose start tag
// is start, which has no ·governing element declaration· and whose xsi:type
// carries lexical, and refuses unless lexical names a top-level type
// definition T against the namespace bindings in scope at start
// (instanceType), and the child and its subtree meet governed's conditions
// against T, never ·nilled·: key-nilled is relative to a declaration, and it
// has none.
//
// T is the child's ·governing type definition·, so the child is ·strictly
// assessed· against it (cvc-assess-elt clause 1) under a strict and a lax
// wildcard alike, and e-validity reads its own [validity] (key-sva clause
// 1.2): under strict the walk's e-validity clause 1.1.3 charge does not arise
// for it, and under lax it is not ·laxly assessed· (key-lva clause 1). Where
// the child's ·locally declared type· within the parent's type is ·absent·,
// key-governing-type-elem clause 8 selects T, which is validate's
// walk.childGoverning reading (instanceGovernance). A child ·attributed to· a
// Wildcard reaches here with a non-·absent· one too — child asks
// LocallyDeclaredElementType only on its {open content} arm — and clause 6
// selects T only where T ·overrides· that type, which with no declaration is
// key-overrides clause 2's ·validly substitutable without limitation·. That is
// exactly cvc-complex-type clause 5's condition, which the walk charges for a
// T failing it (validate's walk.locallyDeclaredType), so no empty Result
// reaches the gate with such a child unless clause 6 selects T: the gate's
// reading against T is the walk's. With no declaration there is no ·selected
// type definition· for T to ·override·, so governingType's cvc-elt clause 4
// test has no counterpart here.
//
// An xsi:type naming no type definition leaves the child with no ·governing
// type definition·. The walk charges cvc-attribute clause 5 for it, but the
// gate refuses it too rather than read a subtree it cannot type.
func (g *subtreeGate) instanceTyped(start xml.StartElement, lexical string, inherited []xml.Attr) refusal {
	defer g.enter(start)()
	t, why := g.instanceType(lexical)
	if why != "" {
		return why
	}
	return g.governed(start, t, false, inherited)
}

// localTyped reads through to its end tag an {open content}'s child whose start
// tag is start, which has no ·governing element declaration· because its
// ·locally declared type· ldt within the parent's type is non-·absent·
// (key-governing-ed clause 4.3), and refuses unless localType determines its
// ·governing type definition·, and the child and its subtree meet governed's
// conditions against that type, never ·nilled·: key-nilled is relative to a
// declaration, and it has none. The child is ·strictly assessed· against that
// type (key-sva (§3.3.4.6) clause 1.2), as validate's walk.localGovernance
// assesses it, so cvc-complex-type clause 5 holds for it by construction, and
// cvc-type clause 2, for an abstract complex type, is the walk's charge.
func (g *subtreeGate) localTyped(start xml.StartElement, ldt xsd.TypeDefinition, inherited []xml.Attr) refusal {
	defer g.enter(start)()
	td, why := g.localType(start, ldt)
	if why != "" {
		return why
	}
	return g.governed(start, td, false, inherited)
}

// localType is the ·governing type definition· (key-governing-type-elem) of the
// element whose start tag is start, which has no ·governing element
// declaration· and the non-·absent· ·locally declared type· ldt, and a refusal
// wherever the gate does not determine it. With no xsi:type, ldt governs
// (clause 7). With one, the type T it names (instanceType;
// refuseXsiTypeUnresolved where it names none) governs where it ·overrides· ldt
// (clause 6), which with no declaration known is ·validly substitutable without
// limitation· (key-overrides clause 2): xsd.Schema.ValidlySubstitutable under
// no blocking keywords. Otherwise ldt governs (clause 7): with no declaration,
// no cvc-elt clause is live to charge an xsi:type that does not override, so
// the walk charges nothing for it and the gate admits it, unlike
// governingType's refuseXsiTypeNotOverride. An error from that predicate is
// refused (refuseXsiTypeUndecided): the walk records it in Result.Unevaluated
// and assesses the child against nothing.
func (g *subtreeGate) localType(start xml.StartElement, ldt xsd.TypeDefinition) (xsd.TypeDefinition, refusal) {
	i := slices.IndexFunc(start.Attr, func(a xml.Attr) bool { return a.Name == xsiType })
	if i < 0 {
		return ldt, ""
	}
	t, why := g.instanceType(start.Attr[i].Value)
	if why != "" {
		return nil, why
	}
	overrides, err := g.schema.ValidlySubstitutable(t, ldt, nil)
	if err != nil {
		return nil, refuseXsiTypeUndecided
	}
	if !overrides {
		return ldt, ""
	}
	return t, ""
}

// laxlyAssessed reads through to its end tag an element whose start tag is
// start, which has neither a ·governing element declaration· nor a ·governing
// type definition· and is not ·skipped·, and refuses unless it and its subtree
// meet complex's conditions against ·xs:anyType· (refuseAnyType where
// the schema has none), never ·nilled·: key-nilled is relative to a
// declaration, and it has none. Such an element is ·laxly assessed·
// (cvc-assess-elt clause 3, key-lva): locally validated against ·xs:anyType·
// and its [[attributes]] and [[children]] assessed by key-sva clauses 2 and 3,
// which is validate's walk.child and walk.attribute for a laxly assessed
// parent (#1823, #1891). Its [validity] is notKnown (e-validity clause 2),
// which blocks no ancestor's valid: e-validity clause 1.1.3 counts notKnown
// only under a strict ·wildcard particle·. What it does decide is every charge
// at or below it, which instance.go's "Charges at depth" reads as "not valid"
// (§2.5 key-deep-valid-doc, #1911).
func (g *subtreeGate) laxlyAssessed(start xml.StartElement, inherited []xml.Attr) refusal {
	td, ok := g.schema.Type(anyTypeName)
	if !ok {
		return refuseAnyType
	}
	anyType, ok := td.(xsd.ComplexType)
	if !ok {
		return refuseAnyType
	}
	defer g.enter(start)()
	return g.complex(start, anyType, false, inherited)
}

// locallyDeclaredAttribute reports whether the ·locally declared type·
// (key-ldt-att) within t of an attribute named n may be non-·absent·: some
// Complex Type Definition on onChain's walk from t has an {attribute uses}
// member whose {attribute declaration} is named n (case 2). For an attribute
// matching none of t.{attribute uses}, that is a base's use the restriction
// t, or a restriction between t and that base, prohibited.
func (g *subtreeGate) locallyDeclaredAttribute(t xsd.ComplexType, n xsd.QName) bool {
	return g.onChain(t, func(c xsd.ComplexType) bool {
		return slices.ContainsFunc(c.AttributeUses(), func(u xsd.AttributeUse) bool { return u.DeclarationName() == n })
	})
}

// onChain reports whether holds answers true for t or for a Complex Type
// Definition on t's {base type definition} chain, which the ·locally declared
// type· recursion (key-ldtype case 3) walks, stopping at ·xs:anyType· (case 1,
// never offered to holds) or at a Simple Type Definition, which declares no
// element and no attribute. A {base type definition} that does not resolve
// answers true, so an unreadable chain is refused rather than read as ·absent·.
// The walk needs no visited set: a finalized Schema's base chains are acyclic
// but for ·xs:anyType·'s own, where it stops.
func (g *subtreeGate) onChain(t xsd.ComplexType, holds func(xsd.ComplexType) bool) bool {
	for c := t; c.Name() != anyTypeName; {
		if holds(c) {
			return true
		}
		base, ok := g.schema.ResolvedType(c.Base())
		if !ok {
			return true
		}
		next, complex := base.(xsd.ComplexType)
		if !complex {
			return false
		}
		c = next
	}
	return false
}

// rootStart reads dec up to the document element's start tag. It refuses
// (refuseDoctype) a DOCTYPE whose DTD could default an attribute the reader
// does not see (defaultsNoAttribute), and any other directive, character data
// before that tag that is not white space (refuseProlog), and a decoder error
// before that tag (refuseDecode). XML 1.0 [22] prolog admits only S as
// character data, and a second U+FEFF after the mark rawDecoder drops is none.
// A CDATA section or character reference whose text is white space reads as
// white space here, since encoding/xml marks neither in what it delivers;
// parser/xmltree refuses both, and no verdict rests on this read without that
// one (rawDecoder). The directive refusal also covers a DOCTYPE encoding/xml
// delimits short of its real end, as a quote inside a processing instruction
// can make it: every markup declaration left over arrives as a directive of its
// own, and a parameter-entity reference left over needs a declaration, which
// carries a '%' either inside the DOCTYPE or in a directive left over too.
//
// The DOCTYPE that survives declares, in its internal subset alone, general
// entities, notations, element types, comments and processing instructions.
// None of these defaults an attribute (XML 1.0 §3.3.2), so the attributes the
// reader sees are the [attributes] the walk assesses. Its entities still
// change the infoset (§4.4), and neither change is read here: an unparsed
// entity it declares is one an ENTITY value may name, which the walk decides
// through parser/xmltree's own read of the subset (key-vde, cvc-simple-type
// clause 3); a reference to a general entity it declares is included text
// (§4.4.2), which parser/xmltree includes and encoding/xml, knowing no entity
// but the five predefined ones, refuses, so a document holding one reaches no
// verdict through either reader of rootStart (rawDecoder): each admits the
// document only once dec has read the whole of it, and refuses it on dec's
// error.
func rootStart(dec *xmltok.Decoder) (xml.StartElement, refusal) {
	for {
		tok, err := dec.Token()
		if err != nil {
			return xml.StartElement{}, refuseDecode
		}
		switch t := tok.(type) {
		case xml.Directive:
			if !defaultsNoAttribute(t) {
				return xml.StartElement{}, refuseDoctype
			}
		case xml.CharData:
			if strings.Trim(string(t), " \t\r\n") != "" {
				return xml.StartElement{}, refuseProlog
			}
		case xml.StartElement:
			return t, ""
		}
	}
}

// defaultsNoAttribute reports whether d is a DOCTYPE whose DTD can default no
// attribute: an AttlistDecl is the one declaration that does (XML 1.0 §3.3.2),
// and d answers false wherever one could hide. That is an ExternalID after the
// document type name, whose external subset the reader does not read (§2.8
// doctypedecl), an <!ATTLIST in the internal subset, and any '%' there: a
// parameter-entity reference can expand to an AttlistDecl no literal scan sees
// (§4.4.8). A '%' that is no reference — one in a SystemLiteral — is refused
// with them. The header is the text before the first '[', so a '[' inside an
// ExternalID's literal leaves the literal's opening half in the header, where
// it is a third field.
func defaultsNoAttribute(d xml.Directive) bool {
	header, subset, _ := strings.Cut(string(d), "[")
	fields := strings.Fields(header)
	if len(fields) != 2 || fields[0] != "DOCTYPE" {
		return false
	}
	return !strings.Contains(subset, "<!ATTLIST") && !strings.Contains(subset, "%")
}

// rawDecoder is the one internal/xmltok reader the lane's raw re-reads —
// assessedSubtreeRoot, and instanceHints through recordingDecoder — take over
// a document's bytes. It reads the leading byte-order mark through
// internal/xmlenc, the decoding parser/xmltree's reader takes (XML 1.0 §4.3.3,
// Appendix F.1): a UTF-16 document, either byte order, is transcoded to UTF-8,
// a UTF-8 mark is dropped as the encoding signature it is, and an encoding
// declaration that disagrees with the mark fails the read through the mark's
// CharsetReader. The mark is thereby consumed before xmldecl.As10 meets the
// declaration, as As10 requires, and As10 then admits a 1.x version label as
// xmltree admits it — so a document's label is admitted here exactly when
// xmltree admits it, in either encoding, with the mark or without.
//
// One disagreement xmltree rejects is read here: a UTF-16 mark under
// encoding="UTF-8", which encoding/xml never hands to a CharsetReader and
// xmltree's checkDeclaration catches on its own. No verdict rests on that
// read: assessedSubtreeRoot runs only after assessInstance has read the
// instance through xmltree, and a case whose instance instanceHints read is
// declined by that same assessInstance when xmltree rejects it.
//
// A read failure in the peek for the mark is reported by the decoder's first
// read, as xmlenc.Decode latches it.
func rawDecoder(r io.Reader) *xmltok.Decoder {
	return recordingDecoder(r, io.Discard)
}

// recordingDecoder is rawDecoder writing to w every byte of the decoded UTF-8
// stream the decoder reads, in order, so byte k written to w is the byte at
// the decoder's InputOffset k. The mark's CharsetReader hands back the stream
// it is given, so a declaration naming UTF-16 changes no offset. instanceHints
// cuts an inline xs:schema out of the instance through it (hintReader).
func recordingDecoder(r io.Reader, w io.Writer) *xmltok.Decoder {
	body, mark := xmlenc.Decode(r)
	dec := xmltok.NewDecoder(io.TeeReader(xmldecl.As10(body), w))
	dec.CharsetReader = mark.CharsetReader
	return dec
}

// expandedName is n as the ·expanded name· the schema indexes by. encoding/xml
// has already translated a bound prefix to its namespace name.
func expandedName(n xml.Name) xsd.QName {
	return xsd.QName{Space: n.Space, Local: n.Local}
}

// isNamespaceDeclaration reports whether a is a namespace declaration, which
// the infoset carries in [namespace attributes] and not in [attributes].
func isNamespaceDeclaration(a xml.Attr) bool {
	return a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns")
}

// isLocationHint reports whether a is xsi:schemaLocation or
// xsi:noNamespaceSchemaLocation.
func isLocationHint(a xml.Attr) bool {
	return a.Name.Space == xsd.XMLSchemaInstanceNS &&
		(a.Name.Local == "schemaLocation" || a.Name.Local == "noNamespaceSchemaLocation")
}

// closureVersioned reports whether any schema document the assembly read
// carries an attribute in versioningNS, on any element.
//
// GAP(conformance): the harness declines every versioned assembly, and no
// parser defect now backs the decline — parser applies all six of §4.2.2's
// attributes, so the components it assembles are S2's. Its two consumers each
// charge in the withhold direction alone, returning Fail with refuseVersioned:
// assessedSubtreeRoot withholds the instance lane's "valid" reading, which it
// is asked for only after the walk charged nothing, and execJSONCase withholds
// the json lane's verdict either way, since it asks before validating. Neither
// can produce a pass; each can cost one on a case the suite declares valid.
// Owned by #2805, which retires the gate or re-justifies it.
//
// Each document is re-read from its parser.AssembledDocument.Location, which
// for the pinnedResolver assembleCase uses is an on-disk path. An inline
// xs:schema of the instance at doc has a location naming no file
// (inlineLocation), and the instance itself is read in its place: every
// attribute of the inline document is one of the instance's, so that reading
// stays a superset. A document that will not open or decode answers true.
func closureVersioned(report *parser.AssemblyReport, doc string) bool {
	for _, d := range report.Documents() {
		path := d.Location
		if isInlineLocation(doc, path) {
			path = doc
		}
		if documentCarries(path, isVersioningAttr) {
			return true
		}
	}
	return false
}

// isVersioningAttr reports whether a is in versioningNS.
func isVersioningAttr(a xmltree.Attribute) bool {
	return a.Name().Space() == versioningNS
}

// documentCarries reports whether any element of the document at path carries
// an attribute satisfying is, or the document cannot be read to say: one that
// will not open or read answers true.
//
// It reads through parser/xmltree, the reader the assembly read the document
// with, and not through rawDecoder: an element or attribute that a reference
// to an internal general entity includes (XML 1.0 §4.4.2) is one the assembly
// saw, and encoding/xml, which knows no entity but the five predefined ones,
// would refuse the document instead of reading it.
func documentCarries(path string, is func(xmltree.Attribute) bool) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer func() { _ = f.Close() }() // read-only handle: close error cannot affect the verdict
	r := xmltree.NewReader(path, f)
	for {
		tok, err := r.Token()
		if errors.Is(err, io.EOF) {
			return false
		}
		if err != nil {
			return true
		}
		start, ok := tok.(*xmltree.StartElement)
		if !ok {
			continue
		}
		if slices.ContainsFunc(start.Attributes(), is) {
			return true
		}
	}
}
