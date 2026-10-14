package xmlname

import "unicode"

// NameStartChar is XML 1.0 5e production [4] NameStartChar (xml.md §2.3):
// ":" | [A-Z] | "_" | [a-z] | [#xC0-#xD6] | [#xD8-#xF6] | [#xF8-#x2FF] |
// [#x370-#x37D] | [#x37F-#x1FFF] | [#x200C-#x200D] | [#x2070-#x218F] |
// [#x2C00-#x2FEF] | [#x3001-#xD7FF] | [#xF900-#xFDCF] | [#xFDF0-#xFFFD] |
// [#x10000-#xEFFFF], one interval per alternative.
var NameStartChar = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: ':', Hi: ':', Stride: 1},
		{Lo: 'A', Hi: 'Z', Stride: 1},
		{Lo: '_', Hi: '_', Stride: 1},
		{Lo: 'a', Hi: 'z', Stride: 1},
		{Lo: 0xC0, Hi: 0xD6, Stride: 1},
		{Lo: 0xD8, Hi: 0xF6, Stride: 1},
		{Lo: 0xF8, Hi: 0x2FF, Stride: 1},
		{Lo: 0x370, Hi: 0x37D, Stride: 1},
		{Lo: 0x37F, Hi: 0x1FFF, Stride: 1},
		{Lo: 0x200C, Hi: 0x200D, Stride: 1},
		{Lo: 0x2070, Hi: 0x218F, Stride: 1},
		{Lo: 0x2C00, Hi: 0x2FEF, Stride: 1},
		{Lo: 0x3001, Hi: 0xD7FF, Stride: 1},
		{Lo: 0xF900, Hi: 0xFDCF, Stride: 1},
		{Lo: 0xFDF0, Hi: 0xFFFD, Stride: 1},
	},
	R32: []unicode.Range32{
		{Lo: 0x10000, Hi: 0xEFFFF, Stride: 1},
	},
	LatinOffset: 6,
}

// NameCharExtra is what XML 1.0 5e production [4a] NameChar (xml.md §2.3) adds
// to NameStartChar: "-" | "." | [0-9] | #xB7 | [#x0300-#x036F] |
// [#x203F-#x2040], one interval per alternative. NameChar is the union of the
// two tables.
var NameCharExtra = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: '-', Hi: '-', Stride: 1},
		{Lo: '.', Hi: '.', Stride: 1},
		{Lo: '0', Hi: '9', Stride: 1},
		{Lo: 0xB7, Hi: 0xB7, Stride: 1},
		{Lo: 0x300, Hi: 0x36F, Stride: 1},
		{Lo: 0x203F, Hi: 0x2040, Stride: 1},
	},
	LatinOffset: 4,
}

// IsNCName reports whether s matches Namespaces in XML production [4] NCName
// (docs/specs/md/xml-names.md), Name - (Char* ':' Char*): a NameStartChar
// other than ':' and then NameChars other than ':'.
func IsNCName(s string) bool {
	for i, r := range s {
		if r == ':' {
			return false
		}
		if i == 0 && !unicode.Is(NameStartChar, r) {
			return false
		}
		if !unicode.In(r, NameStartChar, NameCharExtra) {
			return false
		}
	}
	return s != ""
}
