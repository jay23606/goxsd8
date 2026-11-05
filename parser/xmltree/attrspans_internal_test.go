package xmltree

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/internal/xmltok"
	"github.com/kud360/goxsd8/xsderr"
)

// attrTag is one generated start tag: its source and, for each attribute the
// decoder reads, namespace declarations included, the offset of its name.
type attrTag struct {
	src   string
	names []int
}

// genAttrTag builds a start tag of up to six attributes, in no namespace or
// prefixed, with xmlns and xmlns:p declarations among them, separated and
// surrounded by white space drawn from space, tab, CR, LF and CRLF — none at all
// between one value's closing quote and the next name, which the decoder
// admits — and quoted with either quote around values holding '>', '=' and the
// other quote.
func genAttrTag(rng *rand.Rand) attrTag {
	spaces := []string{" ", "\t", "\r", "\n", "\r\n", " \t\r\n "}
	ws := func(none bool) string {
		if none && rng.IntN(3) == 0 {
			return ""
		}
		return spaces[rng.IntN(len(spaces))]
	}
	values := []string{"", "v", ">", "a=b", "x>y=z", "'", `"`, "'>'", "&amp;", "&#9;"}
	var b strings.Builder
	b.WriteString("<p:e")
	var tag attrTag
	declared := false
	used := map[string]bool{}
	for i := range 1 + rng.IntN(6) {
		b.WriteString(ws(i > 0))
		var name string
		switch rng.IntN(5) {
		case 0:
			name = "xmlns:p"
			if declared {
				name = fmt.Sprintf("xmlns:q%d", i)
			}
			declared = true
		case 1:
			name = "xmlns"
		case 2:
			name = fmt.Sprintf("p:a%d", i)
		default:
			name = fmt.Sprintf("a%d", i)
		}
		if used[name] {
			name = fmt.Sprintf("b%d", i)
		}
		used[name] = true
		tag.names = append(tag.names, b.Len())
		b.WriteString(name)
		b.WriteString(ws(true))
		b.WriteString("=")
		b.WriteString(ws(true))
		v := values[rng.IntN(len(values))]
		q := `"`
		if strings.Contains(v, `"`) || (!strings.Contains(v, "'") && rng.IntN(2) == 0) {
			q = "'"
		}
		if strings.HasPrefix(name, "xmlns") {
			v = "urn:x"
		}
		b.WriteString(q + v + q)
	}
	if !declared {
		tag.names = append(tag.names, b.Len()+1)
		b.WriteString(` xmlns:p="urn:p"`)
	}
	b.WriteString(ws(true) + "/>")
	tag.src = b.String()
	return tag
}

// TestAttrSpansAgreeWithTheDecoder is the differential behind attrSpans: on
// every generated start tag the decoder reads, attrSpans finds as many
// attributes as the decoder, each name beginning where the decoder's own
// name, prefix included, is spelled and where the generator wrote it, and the
// Reader reads the tag, locating each Attribute at its name.
func TestAttrSpansAgreeWithTheDecoder(t *testing.T) {
	rng := rand.New(rand.NewPCG(2402, 1))
	const tags = 20000
	for range tags {
		tag := genAttrTag(rng)
		tok, err := xmltok.NewDecoder(strings.NewReader(tag.src)).RawToken()
		if err != nil {
			t.Fatalf("decoder refused generated tag %q: %v", tag.src, err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			t.Fatalf("decoder read %q as %T", tag.src, tok)
		}
		spans := attrSpans(tag.src)
		if len(spans) != len(start.Attr) || len(spans) != len(tag.names) {
			t.Fatalf("%q: attrSpans found %d attributes, the decoder %d, the generator wrote %d", tag.src, len(spans), len(start.Attr), len(tag.names))
		}
		for i, a := range start.Attr {
			if spans[i].name != tag.names[i] || !strings.HasPrefix(tag.src[spans[i].name:], rawName(a.Name)) {
				t.Fatalf("%q: attribute %d %s at byte %d, generated at %d", tag.src, i, rawName(a.Name), spans[i].name, tag.names[i])
			}
		}
		r := NewReader("t.xml", strings.NewReader(tag.src))
		n, err := r.Token()
		if err != nil {
			t.Fatalf("Reader refused %q: %v", tag.src, err)
		}
		se := n.(*StartElement)
		k := 0
		for i, a := range start.Attr {
			if isDeclaration(a) {
				continue
			}
			before := tag.src[:tag.names[i]]
			line, col := 1+strings.Count(before, "\n"), len(before)-strings.LastIndexByte(before, '\n')
			if got := se.Attributes()[k].Loc(); got.Line != line || got.Col != col {
				t.Fatalf("%q: attribute %s at %s, want %d:%d", tag.src, rawName(a.Name), got, line, col)
			}
			k++
		}
		if _, err := r.Token(); err != nil {
			t.Fatalf("Reader after %q: %v", tag.src, err)
		}
		if _, err := r.Token(); !errors.Is(err, io.EOF) {
			t.Fatalf("Reader after %q: %v, want io.EOF", tag.src, err)
		}
	}
}

// TestAttrScanDisagreementIsRefused pins expandAttrs' invariant on a start tag
// the decoder never hands it, since the differential above finds none: a
// source whose attributes attrSpans counts otherwise than the decoder's, or
// spells under another name, is refused at the tag as RuleXMLWellFormed
// wrapping errAttrScan, a limit of the reader and no verdict.
func TestAttrScanDisagreementIsRefused(t *testing.T) {
	loc := xsderr.Loc{URI: "t.xml", Line: 1, Col: 1}
	decoded := xml.StartElement{Name: xml.Name{Local: "e"}, Attr: []xml.Attr{{Name: xml.Name{Local: "a"}, Value: "1"}}}
	for _, c := range []struct{ raw, want string }{
		{`<e/>`, `t.xml:1:1: [xml-wf] start tag re-read disagrees with the decoder: found 0 attribute values where the decoder read 1`},
		{`<e b="1"/>`, `t.xml:1:1: [xml-wf] start tag re-read disagrees with the decoder: the decoder read attribute 1 as a, where the source spells "b=\"1\"/>"`},
	} {
		r := NewReader("t.xml", strings.NewReader(""))
		_, err := r.expandAttrs(decoded, c.raw, true, func(i int) int64 { return int64(i) }, loc, nil)
		if !errors.Is(err, errAttrScan) {
			t.Errorf("%s: error = %v, want one wrapping errAttrScan", c.raw, err)
		}
		if err == nil || err.Error() != c.want {
			t.Errorf("%s: error = %v, want %s", c.raw, err, c.want)
		}
	}
}
