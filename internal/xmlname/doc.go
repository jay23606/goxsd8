// Package xmlname holds the XML 1.0 Fifth Edition name-character productions,
// [4] NameStartChar and [4a] NameChar (docs/specs/md/xml.md §2.3), once for
// every library package that reads them: regex's \i and \c escapes (Datatypes
// §G.4.2.5), parser/xmltree's DOCTYPE name checks and internal/xmltok's
// element, attribute, PI-target and entity-reference name checks; and
// Namespaces in XML [4] NCName over them, which validate/jsonsrc's "$xmlns"
// prefix check reads.
//
// # Contract
//
//	var NameStartChar *unicode.RangeTable
//	var NameCharExtra *unicode.RangeTable
//	func IsNCName(s string) bool
//
// A rune is a NameStartChar when unicode.Is(NameStartChar, r) holds, and a
// NameChar when unicode.In(r, NameStartChar, NameCharExtra) holds: [4a] is
// [4] plus NameCharExtra's alternatives, and the tables keep that split so
// each one transcribes its production's alternatives in the spec's order. Both
// tables are read-only; no importer mutates them. IsNCName reports whether a
// string is a Name with no ':' (xml-names.md [4], Name - (Char* ':' Char*)).
//
// The edition is chosen here. Which XML edition supplies the productions is
// implementation-defined (Datatypes §H.1 clause 1); this module takes XML 1.0
// Fifth Edition, the edition the local spec corpus carries, and every reader
// of these tables inherits the one choice. The tables are Name's, not
// NCName's: ':' is a NameStartChar here, and IsNCName alone excludes it.
//
// It depends on nothing but the standard library.
package xmlname
