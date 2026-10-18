package parser

import (
	"strings"

	"github.com/kud360/goxsd8/builtin"
	"github.com/kud360/goxsd8/parser/xmltree"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// ruleSrcCIP is the Conditional Inclusion Constraints Schema Representation
// Constraint (§4.2.2, src-cip): the lexical requirements the six versioning
// attributes must meet before their verdicts can be read at all. It is declared
// here rather than in produce.go's block because this file is the only place in
// the package that reads the versioning namespace.
const ruleSrcCIP xsderr.Rule = "src-cip"

// versioningNS is the schema-versioning namespace §1.3.1.3 defines and §1.3.2
// lists among the namespaces with special status. An attribute in it is foreign
// markup to every content model in Appendix A — the schema for schema documents
// admits "{any attributes with non-schema namespace}" on every element — which
// is exactly why nothing but this file looks at one.
const versioningNS = "http://www.w3.org/2007/XMLSchema-versioning"

// processorVersion is V, "a decimal value representing the version of XSD
// supported by the processor": §4.2.2 fixes it at 1.1 "for processors
// conforming to this version of this specification", which this one does.
//
// It is deliberately NOT shared with the conformance harness's
// supportedVersionTokens, whose doc holds the two apart in as many words: that
// slice says which xsts.xsd `version` tokens the harness claims, a
// suite-metadata mechanism defined by testdata/xsdtests/common/xsts.xsd alone.
// The two facts happen to be spelled with the same number; they are not one
// fact with two encodings (STYLE D3), and merging them would make a harness
// applicability decision govern which elements of a schema document exist.
var processorVersion = decimal{integer: "1", fraction: "1"}

// conditionalInclude applies ·conditional-inclusion pre-processing· to doc and
// returns S2 = ci(S1), the schema document §4.2.2 requires every later rule to
// be read against: "It is S2, not S1, which is required to conform to this
// specification", and "references to ·schema documents· elsewhere in this
// specification invariably refer to the result of the pre-processing step
// described here, not to its input". Every Schema Representation Constraint
// this package charges — sch-props-correct clause 2 above all, which a
// declaration §4.2.2 removes must not reach — therefore runs on what this
// returns.
//
// It is applied to each document as it is READ, before any of its own
// <include>/<override>/<redefine>/<import> children are followed, because
// §4.2.2 is the pre-processing that "is always performed first"
// (key-pre-processing): chameleon, redefinition and override pre-processing all
// compose documents that have already been through it, and a directive element
// the transform removes is not a directive of S2 at all.
//
// An element §4.2.2 ignores is removed "along with all its attributes and
// descendants", so the whole subtree is absent from S2 and no rule — src-cip
// included — reaches anything inside it.
//
// A document carrying no versioning attribute at all is returned as it stands,
// unrebuilt: §4.2.2's own note, "if S1 contains no elements or attributes to be
// ignored, then S1 and S2 are identical". Where one does appear the tree is
// rebuilt whole, because an [Element]'s parent link is fixed at construction
// and a rebuilt ancestor cannot keep an original child.
//
// All six of §4.2.2's attributes decide the verdict, read by [ignoredElement].
func conditionalInclude(doc *Document) (*Document, error) {
	if !carriesVersioningAttr(doc.root) {
		return doc, nil
	}
	ignore, err := ignoredElement(doc.root)
	if err != nil {
		return nil, err
	}
	if ignore {
		return &Document{uri: doc.uri, root: schemaStub(doc.root)}, nil
	}
	root, err := retainedElement(doc.root, nil)
	if err != nil {
		return nil, err
	}
	return &Document{uri: doc.uri, root: root}, nil
}

// retainedElement rebuilds el — an element §4.2.2 does NOT ignore, its own
// verdict already read by the caller — into its S2 form under parent: the same
// start tag, attributes and base URI, with every ignored child (and with it
// that child's whole subtree) dropped from [Element.Children] and every
// retained one rebuilt in turn, in document order.
//
// Character data is carried across as the same immutable [Text] node: §4.2.2
// removes elements and attributes, never text, and an <xs:documentation> whose
// element survives keeps its content byte for byte.
func retainedElement(el *Element, parent *Element) (*Element, error) {
	kept := &Element{src: el.src, parent: parent, attrs: el.attrs, baseURI: el.baseURI}
	for _, child := range el.children {
		sub, ok := child.(*Element)
		if !ok {
			kept.children = append(kept.children, child)
			continue
		}
		ignore, err := ignoredElement(sub)
		if err != nil {
			return nil, err
		}
		if ignore {
			continue
		}
		retained, err := retainedElement(sub, kept)
		if err != nil {
			return nil, err
		}
		kept.children = append(kept.children, retained)
	}
	return kept, nil
}

// schemaStub is S2 for the case §4.2.2 singles out — the document's own root
// element is ignored. S2 is then not an absent or empty document, since a
// schema document has a root: it "is identical to S1 except that any attributes
// other than targetNamespace, vc:minVersion or vc:maxVersion are removed from
// its [attributes], and its [children] is the empty sequence".
//
// §4.2.2 writes the carve-out for the <schema> element information item. A
// document rooted at anything else is not a schema document at all — [Parse]
// and [Produce] both reject it on that ground before reading further — so it
// takes the same narrowing rather than a second shape invented for it, and the
// alternative (returning no document) is not representable.
//
// The stub keeps the root's own base URI, which nothing can read: base URIs are
// consulted to resolve the schemaLocation of a directive element, and the stub
// has no children to hold one.
func schemaStub(root *Element) *Element {
	kept := make([]xmltree.Attribute, 0, len(root.attrs))
	for _, a := range root.attrs {
		if retainedOnStub(a.Name()) {
			kept = append(kept, a)
		}
	}
	return &Element{src: root.src, attrs: kept, baseURI: root.baseURI}
}

// retainedOnStub reports whether an attribute survives on the stub §4.2.2
// leaves where an ignored root stood: targetNamespace, vc:minVersion and
// vc:maxVersion, and nothing else. targetNamespace is unprefixed, hence in no
// namespace, as every attribute the schema for schema documents declares is
// (§5.1, Appendix A).
//
// vc:maxVersion is retained unconditionally, exactly as written: the carve-out
// names the three attributes S2 keeps and conditions none of them on what the
// verdict was reached over.
func retainedOnStub(name xmltree.Name) bool {
	if name.Space() == "" {
		return name.Local() == "targetNamespace"
	}
	return name.Space() == versioningNS && (name.Local() == "minVersion" || name.Local() == "maxVersion")
}

// carriesVersioningAttr reports whether el or any of its descendants carries an
// attribute in the versioning namespace — whether, that is, §4.2.2 has anything
// to say about this document at all. Every clause of the transform and every
// clause of src-cip is conditioned on one of those attributes appearing, so a
// document with none is S2 already.
func carriesVersioningAttr(el *Element) bool {
	for _, a := range el.attrs {
		if a.Name().Space() == versioningNS {
			return true
		}
	}
	for _, child := range el.children {
		sub, ok := child.(*Element)
		if ok && carriesVersioningAttr(sub) {
			return true
		}
	}
	return false
}

// ignoredElement reads §4.2.2's verdict on one element: whether the element "is
// to be ignored, along with all its attributes and descendants".
//
// The conditions are a disjunction, tested here in the order §4.2.2 states them
// rather than in the order the author happened to write the attributes. That
// choice is user-visible wherever an ill-formed value shares an element with
// another versioning attribute — whether src-cip is charged, and for which
// attribute, follows SPEC order: a condition firing earlier in it ends the scan
// (below), and of two ill-formed values the first in it is charged — and it
// keeps the verdict independent of tag layout (STYLE D1).
//
// A condition that fires ends the scan, so the ill-formed value of a LATER
// versioning attribute on the same element goes unreported. That is not
// leniency: the element is absent from S2, and src-cip, like every Schema
// Representation Constraint, is "enforced after, not before, the
// ·conditional-inclusion pre-processing·" (§4.2.1) — it does not reach an
// attribute S2 does not carry.
//
// An attribute in the versioning namespace that is none of the six is left
// alone. src-cip states that case with a "should" and its own note spells out
// the consequence: "it is not an error for such attributes to appear in a
// ·schema document·", the rule being written that way "to preserve the ability
// of future versions of this specification to add new attributes to the
// schema-versioning namespace". Warnings are encouraged there, not required,
// and this parser issues none — a lowercase vc:minversion is silently no
// attribute of §4.2.2's, which is the outcome the suite's own fixtures depend
// on.
//
// The two version attributes are compared against V as ·actual values·, so
// the retained band is vc:minVersion ≤ V < vc:maxVersion: minVersion is
// inclusive and maxVersion exclusive, as §4.2.2 calls the semantics of the
// vc:maxVersion attribute. A band with vc:maxVersion ≤ vc:minVersion retains
// nothing, and needs no test of its own: whichever arm fires ignores the
// element.
func ignoredElement(el *Element) (bool, error) {
	if lexical, ok := versioningAttr(el, "minVersion"); ok {
		lower, err := conditionalDecimal(el, "minVersion", lexical)
		if err != nil {
			return false, err
		}
		// "If V is less than the value of vc:minVersion … the element … is to be
		// ignored": minVersion is INCLUSIVE, so V = minVersion retains.
		if decimalCmp(processorVersion, lower) < 0 {
			return true, nil
		}
	}
	if lexical, ok := versioningAttr(el, "maxVersion"); ok {
		upper, err := conditionalDecimal(el, "maxVersion", lexical)
		if err != nil {
			return false, err
		}
		// "… or if V is greater than or equal to the value of vc:maxVersion":
		// maxVersion is EXCLUSIVE, so V = maxVersion ignores.
		if decimalCmp(processorVersion, upper) >= 0 {
			return true, nil
		}
	}
	if lexical, ok := versioningAttr(el, "typeAvailable"); ok {
		names, err := conditionalQNames(el, "typeAvailable", lexical)
		if err != nil {
			return false, err
		}
		// Clause 1: ignored where ANY item is not the expanded name of a type
		// ·automatically known· to the processor.
		if !allNamed(names, automaticallyKnownType) {
			return true, nil
		}
	}
	if lexical, ok := versioningAttr(el, "typeUnavailable"); ok {
		names, err := conditionalQNames(el, "typeUnavailable", lexical)
		if err != nil {
			return false, err
		}
		// Clause 2: ignored where EVERY item names a type known to the processor.
		if allNamed(names, automaticallyKnownType) {
			return true, nil
		}
	}
	if lexical, ok := versioningAttr(el, "facetAvailable"); ok {
		names, err := conditionalQNames(el, "facetAvailable", lexical)
		if err != nil {
			return false, err
		}
		// Clause 3, clause 1's shape over facets.
		if !allNamed(names, supportedFacet) {
			return true, nil
		}
	}
	if lexical, ok := versioningAttr(el, "facetUnavailable"); ok {
		names, err := conditionalQNames(el, "facetUnavailable", lexical)
		if err != nil {
			return false, err
		}
		// Clause 4, clause 2's shape over facets.
		if allNamed(names, supportedFacet) {
			return true, nil
		}
	}
	return false, nil
}

// versioningAttr returns the value of el's {versioningNS}local attribute,
// reporting whether it is present. The namespace test is what distinguishes a
// §4.2.2 attribute from an identically-named one in no namespace, which the
// schema for schema documents would govern instead.
func versioningAttr(el *Element, local string) (string, bool) {
	for _, a := range el.attrs {
		if a.Name().Space() == versioningNS && a.Name().Local() == local {
			return a.Value(), true
		}
	}
	return "", false
}

// allNamed reports whether every name satisfies known — whether every item of a
// vc:typeAvailable / vc:typeUnavailable / vc:facetAvailable /
// vc:facetUnavailable list names something this processor has.
//
// One predicate serves all four clauses because the *Available pair is exactly
// its negation: clause 1 ignores where NOT every named type is known, clause 2
// where every one is. That is also where §4.2.2's documented inversion falls
// out rather than being coded twice — "if the ·actual value· of
// vc:typeAvailable is the empty list … the corresponding element is not
// ignored. Conversely, if the ·actual value· of vc:typeUnavailable is the empty
// list, then the corresponding element is ignored" — since the empty list
// satisfies allNamed vacuously.
func allNamed(names []xsd.QName, known func(xsd.QName) bool) bool {
	for _, qn := range names {
		if !known(qn) {
			return false
		}
	}
	return true
}

// automaticallyKnownType reports whether qn is "the expanded name of some type
// definition ·automatically known· to and supported by the processor" — of a
// type "about which a processor possesses prior knowledge, and which the
// processor can support without any declaration of the type being supplied by
// the user" (key-automatic, §3.16.7.4).
//
// For this processor that set is exactly what [builtin.Seed] puts into every
// assembly before a single schema document is produced: the generated
// [builtin.Types] table, plus the two components Seed prepends because they
// have no row of their own — xs:anySimpleType (no facets, not a restriction
// base, §3.2.1.3) and xs:error (§3.16.7.3's tableau) — plus xs:anyType, the
// ur-type definition every assembly starts from, which is a Complex Type
// Definition and so outside a table of datatypes but is no less automatically
// known.
//
// A type the schema DECLARES is not automatically known: it is supplied by the
// user, which is the whole of what key-automatic excludes. That reading is what
// makes vc:typeAvailable="my:myInt" prune the declaration carrying it even
// where the same document defines my:myInt — the suite's vc_008 and vc_009 are
// valid only under it, since retaining both alternants leaves two declarations
// of one expanded name.
//
// A name outside the XSD namespace is never automatically known here: this
// processor defines no ·implementation-defined· datatype of its own and imports
// no other namespace's components without a declaration.
func automaticallyKnownType(qn xsd.QName) bool {
	if qn.Space != xsd.XMLSchemaNS {
		return false
	}
	switch qn.Local {
	case "anyType", "anySimpleType", "error":
		return true
	}
	for i := range builtin.Types {
		if builtin.Types[i].Name == qn.Local {
			return true
		}
	}
	return false
}

// supportedFacet reports whether qn is "the expanded name of some facet known
// to and supported by the processor" (§4.2.2 clauses 3 and 4).
//
// §4.2.2's own note fixes what that expanded name is: "the expanded names of
// the facets (built-in or ·implementation-defined·) are the expanded names of
// the elements used in XSD schema documents to apply the facets, e.g.
// xs:pattern for the pattern facet". So the test is over facet-applying ELEMENT
// names, and the name↔kind bridge is consulted for them rather than retyped as
// a third list (STYLE T4, the same reuse facetKindOf records).
//
// Element name and facet name coincide for every constraining facet but one:
// the assertions facet (§4.3.13) is applied by an <xs:assertion> element, so
// "assertion" is supported and "assertions" — which names no element of any
// content model — is not. builtin.FacetKindByName is keyed by the facet name
// and so answers the pair exactly backwards; the two arms below are that single
// correction, not a parallel table.
//
// Every name the bridge holds is supported: this processor implements each
// constraining facet of §4.3 and precisionDecimal's maxScale/minScale
// (xsd-precisionDecimal.md §4.2/§4.3).
func supportedFacet(qn xsd.QName) bool {
	if qn.Space != xsd.XMLSchemaNS {
		return false
	}
	if qn.Local == "assertion" {
		return true
	}
	if qn.Local == "assertions" {
		return false
	}
	_, ok := builtin.FacetKindByName(builtin.FacetName(qn.Local))
	return ok
}

// conditionalQNames maps the ·initial value· of one of the four availability
// attributes to the expanded names it holds, charging src-cip where it is not
// "locally ·valid· with respect to a simple type definition with {variety} =
// list and {item type definition} = xs:QName".
//
// Items are split by xmlSpaceFields, on XML white space alone, the list
// separator §3.16.4's list mapping uses, which subsumes the whiteSpace =
// collapse xs:QName carries: an empty or all-whitespace value is the empty
// list, the case §4.2.2 calls out.
//
// Two faults are charged, and both are String Valid failures against xs:QName
// rather than anything src-resolve governs: an item outside the ·lexical space·
// of xs:QName, and an item whose prefix has no in-scope binding, which leaves
// §3.3.18's context-dependent lexical mapping with nothing to map it to.
// Neither is a resolution failure — nothing here has to resolve to a component,
// and a well-formed QName naming no known type is the ordinary
// retained-or-ignored case, not an error.
//
// The lexical test is qnameLexical's three colon-and-emptiness shapes AND the
// full NCName pattern on each half. [producer.bindQName] stops short of the
// pattern (its GAP(parser) marker); stopping short here would accept "23" as a
// QName — which is exactly the item src-cip is asked about in the suite's
// vc905.
//
// The unprefixed item takes the default namespace in scope at el, or no
// namespace where none is declared. It is deliberately NOT defaulted to the
// document's targetNamespace, and no chameleon coercion is applied: §4.2.2 runs
// before §F.1's transformation has been applied to anything.
func conditionalQNames(el *Element, attr, lexical string) ([]xsd.QName, error) {
	items := xmlSpaceFields(lexical)
	names := make([]xsd.QName, 0, len(items))
	for _, item := range items {
		prefix, local, fault := qnameLexical(item)
		if fault == "" && !ncNameRE.MatchString(local) {
			fault = "a QName's local part is an NCName, and " + local + " is not one"
		}
		if fault == "" && prefix != "" && !ncNameRE.MatchString(prefix) {
			fault = "a QName's prefix is an NCName, and " + prefix + " is not one"
		}
		if fault != "" {
			return nil, xsderr.New(ruleSrcCIP, el.Loc(),
				"vc:%s item %q on <%s> is not in the ·lexical space· of xs:QName, but src-cip requires this attribute's value to be a list of xs:QName: %s (Datatypes §3.3.18, §3.4.7.1)",
				attr, item, el.Name().Local(), fault)
		}
		uri, bound := el.lookupPrefix(prefix)
		if prefix != "" && !bound {
			return nil, xsderr.New(ruleSrcCIP, el.Loc(),
				"the QName prefix %q of vc:%s item %q on <%s> does not resolve to an in-scope namespace, so the item is not ·valid· with respect to xs:QName as src-cip requires",
				prefix, attr, item, el.Name().Local())
		}
		names = append(names, xsd.QName{Space: uri, Local: local})
	}
	return names, nil
}

// conditionalDecimal maps the ·initial value· of vc:minVersion or vc:maxVersion
// to the decimal §4.2.2 compares V against, charging src-cip where it is not
// "locally ·valid· with respect to xs:decimal as per String Valid (§3.16.4)".
//
// String Valid runs the whiteSpace = collapse xs:decimal carries (§3.3.3)
// before the lexical mapping, so leading and trailing whitespace is stripped
// here; any whitespace INSIDE the value survives collapse as a space and is
// outside the lexical space either way.
func conditionalDecimal(el *Element, attr, lexical string) (decimal, error) {
	d, ok := parseDecimal(collapseTrim(lexical))
	if !ok {
		return decimal{}, xsderr.New(ruleSrcCIP, el.Loc(),
			"vc:%s value %q on <%s> is not in the ·lexical space· of xs:decimal, which src-cip requires it to be valid against (Datatypes §3.3.3.1)",
			attr, lexical, el.Name().Local())
	}
	return d, nil
}

// decimal is one xs:decimal ·actual value· in the only form §4.2.2's version
// comparison needs: a sign and the integer and fractional DIGIT strings, each
// stripped of the leading and trailing zeros that carry no value. Two values
// compare by comparing those digit strings, which is exact for every lexical
// xs:decimal admits and needs no arithmetic — "1.10" and "1.1" reduce to the
// same pair, and a lexical of any length is compared without overflow or
// rounding.
//
// This is NOT the module's decimal value space and must not grow into one: that
// is builtin/strict's, reached through a [value.Backend]. Conditional inclusion
// deliberately does not go through a backend, because a caller's replacement
// mapping for xs:decimal would then decide which elements of a schema document
// exist. V is a fixed property of the specification this processor conforms to
// (§4.2.2), not a value-space policy.
type decimal struct {
	// negative is the sign. A negative zero ("-0.0") is representable and is the
	// same value as zero; nothing here distinguishes them, since both compare the
	// same way against every positive V.
	negative bool
	// integer holds the integer digits with leading zeros removed, so the empty
	// string is the integer part of every value below 1.
	integer string
	// fraction holds the fractional digits with trailing zeros removed, so the
	// empty string is an integral value.
	fraction string
}

// parseDecimal maps a whitespace-collapsed lexical to its decimal value,
// reporting whether it is in the ·lexical space· of xs:decimal at all.
// Datatypes §3.3.3.1 admits an optional sign followed by either an unsigned
// numeral with no decimal point or one with a point and digits on at least one
// side of it:
//
//	decimalLexicalRep ::= ('+' | '-')? ('.' digit+ | digit+ ('.' digit*)?)
//
// A bare sign, an empty value, a lone ".", exponent notation and any other
// character are outside it and report ok false — no leniency, since src-cip
// makes them a schema-document error rather than a value to guess at.
func parseDecimal(lexical string) (decimal, bool) {
	rest := lexical
	negative := false
	if strings.HasPrefix(rest, "+") || strings.HasPrefix(rest, "-") {
		negative = rest[0] == '-'
		rest = rest[1:]
	}
	integer, fraction, _ := strings.Cut(rest, ".")
	if !allDigits(integer) || !allDigits(fraction) {
		return decimal{}, false
	}
	if integer == "" && fraction == "" {
		// "", ".", "+" and "-." alike: a numeral needs at least one digit. A lexical
		// with no point at all is covered by this same test, strings.Cut having left
		// the whole of it in integer and fraction empty.
		return decimal{}, false
	}
	return decimal{
		negative: negative,
		integer:  strings.TrimLeft(integer, "0"),
		fraction: strings.TrimRight(fraction, "0"),
	}, true
}

// allDigits reports whether s is made up entirely of ASCII digits, the empty
// string included. §3.3.3.1's digit is [0-9] alone: no other Unicode decimal
// digit is in the lexical space, so this is a byte scan rather than a
// rune-class test.
func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// decimalCmp compares two decimal values, returning a negative number when a is
// less than b, zero when they are equal, and a positive number when a is
// greater. Equality here is value-space equality, which is what §4.2.2 asks
// for: it compares ·actual values·, so "1.10" and "1.1" are one value and a
// lexical comparison of the two attribute strings would read the band boundary
// wrong.
func decimalCmp(a, b decimal) int {
	if a.negative != b.negative {
		if a.negative {
			return -1
		}
		return 1
	}
	magnitude := magnitudeCmp(a, b)
	if a.negative {
		return -magnitude
	}
	return magnitude
}

// magnitudeCmp compares the magnitudes of two decimals, ignoring their signs.
// The integer parts carry no leading zeros, so the longer digit string is the
// larger number and equal lengths compare lexicographically; the fractional
// parts are then compared digit by digit, a missing digit counting as zero.
func magnitudeCmp(a, b decimal) int {
	if len(a.integer) != len(b.integer) {
		return len(a.integer) - len(b.integer)
	}
	if a.integer != b.integer {
		return strings.Compare(a.integer, b.integer)
	}
	return strings.Compare(padFraction(a.fraction, len(b.fraction)), padFraction(b.fraction, len(a.fraction)))
}

// padFraction right-pads a fractional digit string with zeros to at least n
// digits, so two fractions of different lengths compare digit by digit under a
// plain string comparison.
func padFraction(fraction string, n int) string {
	if len(fraction) >= n {
		return fraction
	}
	return fraction + strings.Repeat("0", n-len(fraction))
}
