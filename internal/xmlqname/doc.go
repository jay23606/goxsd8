// Package xmlqname resolves a QName lexical against the namespace bindings in
// scope at an element, once for every library package that resolves an
// xsi:type: validate's walk (cvc-resolve-instance, §3.17.6.3) and
// validate/jsonsrc's "$type" member, which must name the type the walk will
// find, so that the members it classifies under that type are the ones the walk
// assesses.
//
// # Contract
//
//	func Resolve(lexical string, lookup func(prefix string) (string, bool)) (space, local string, ok bool)
//
// Resolve collapses lexical (whiteSpace = collapse, which xs:QName fixes:
// Datatypes §3.3.18 and §4.3.6, over a space, tab, carriage return and line
// feed only), splits it per the QName production of Namespaces in XML
// (docs/specs/md/xml-names.md [7]-[11]) and resolves its prefix through
// lookup, the caller's bindings in scope at the element. It is false for an
// empty lexical, a colon structure no QName has — an empty prefix or local
// part, or a second colon — and a prefix lookup does not bind, each of which
// leaves the QName with no ·actual value·. An unprefixed lexical takes the
// namespace lookup gives the empty prefix, and an empty prefix lookup does not
// bind is the ·absent· namespace name, not a failure.
//
// The two halves are checked for emptiness and not against the NCName
// production: true does not make the lexical a QName, which is cvc-attribute
// clause 3's question for validate, decided through its backend. A name that is
// no NCName is the {name} of no component, so the component lookup a caller
// makes with the result turns it away as it turns away a well-formed name
// nothing declares.
//
// It depends on nothing but the standard library.
package xmlqname
