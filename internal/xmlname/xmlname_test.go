package xmlname

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// specInterval is one alternative of a name-character production, as the spec
// text spells it.
type specInterval struct {
	lo, hi rune
}

// specProduction reads the alternatives of the production whose anchor is id
// from the local XML 1.0 5e text (docs/specs/md/xml.md §2.3), so the probes
// below compare the tables against the spec and not against a second hand copy.
// A reference to another production (NameChar's NameStartChar) is skipped.
func specProduction(t *testing.T, id string) []specInterval {
	t.Helper()
	src, err := os.ReadFile("../../docs/specs/md/xml.md")
	if err != nil {
		t.Fatalf("reading the XML spec: %v", err)
	}
	var row string
	for line := range strings.SplitSeq(string(src), "\n") {
		if strings.Contains(line, `<a id="`+id+`"></a>`) {
			row = line
			break
		}
	}
	if row == "" {
		t.Fatalf("xml.md has no production row %q", id)
	}
	cells := strings.Split(row, " | ")
	body := strings.Trim(cells[len(cells)-1], "| `")
	var out []specInterval
	for alt := range strings.SplitSeq(body, ` \| `) {
		out = append(out, parseAlternative(t, alt)...)
	}
	return out
}

// parseAlternative reads one alternative: "c", [a-z], [#xLO-#xHI], #xC, or a
// production name, which yields nothing.
func parseAlternative(t *testing.T, alt string) []specInterval {
	t.Helper()
	switch {
	case strings.HasPrefix(alt, `"`):
		r := []rune(strings.Trim(alt, `"`))
		if len(r) != 1 {
			t.Fatalf("alternative %q is not one character", alt)
		}
		return []specInterval{{r[0], r[0]}}
	case strings.HasPrefix(alt, "["):
		lo, hi, ok := strings.Cut(strings.Trim(alt, "[]"), "-")
		if !ok {
			t.Fatalf("alternative %q is not a range", alt)
		}
		return []specInterval{{specChar(t, lo), specChar(t, hi)}}
	case strings.HasPrefix(alt, "#x"):
		c := specChar(t, alt)
		return []specInterval{{c, c}}
	}
	return nil
}

// specChar reads one range endpoint: #xHEX or a literal character.
func specChar(t *testing.T, s string) rune {
	t.Helper()
	hex, ok := strings.CutPrefix(s, "#x")
	if !ok {
		r := []rune(s)
		if len(r) != 1 {
			t.Fatalf("endpoint %q is not one character", s)
		}
		return r[0]
	}
	n, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		t.Fatalf("endpoint %q: %v", s, err)
	}
	return rune(n)
}

func inSpec(set []specInterval, c rune) bool {
	for _, iv := range set {
		if iv.lo <= c && c <= iv.hi {
			return true
		}
	}
	return false
}

// tableIntervals flattens a table into its intervals, in table order.
func tableIntervals(tab *unicode.RangeTable) []specInterval {
	var out []specInterval
	for _, r := range tab.R16 {
		out = append(out, specInterval{rune(r.Lo), rune(r.Hi)})
	}
	for _, r := range tab.R32 {
		out = append(out, specInterval{rune(r.Lo), rune(r.Hi)})
	}
	return out
}

// TestTablesTranscribeTheProductions holds each table to its production
// alternative by alternative, and checks the shape unicode.Is relies on: stride
// 1 throughout, and LatinOffset counting the R16 entries at or below U+00FF.
func TestTablesTranscribeTheProductions(t *testing.T) {
	for _, tc := range []struct {
		id  string
		tab *unicode.RangeTable
	}{
		{"NT-NameStartChar", NameStartChar},
		{"NT-NameChar", NameCharExtra},
	} {
		want := specProduction(t, tc.id)
		got := tableIntervals(tc.tab)
		if len(got) != len(want) {
			t.Fatalf("%s: table has %d intervals, the production %d", tc.id, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: interval %d is [%#x-%#x], the production's [%#x-%#x]", tc.id, i, got[i].lo, got[i].hi, want[i].lo, want[i].hi)
			}
		}
		var latin int
		for _, r := range tc.tab.R16 {
			if r.Stride != 1 {
				t.Errorf("%s: [%#x-%#x] has stride %d, not 1", tc.id, r.Lo, r.Hi, r.Stride)
			}
			if r.Hi <= unicode.MaxLatin1 {
				latin++
			}
		}
		for _, r := range tc.tab.R32 {
			if r.Stride != 1 {
				t.Errorf("%s: [%#x-%#x] has stride %d, not 1", tc.id, r.Lo, r.Hi, r.Stride)
			}
		}
		if tc.tab.LatinOffset != latin {
			t.Errorf("%s: LatinOffset is %d, want %d", tc.id, tc.tab.LatinOffset, latin)
		}
	}
}

// TestEndpointsAndNeighbours probes every interval endpoint of [4] and [4a],
// and the code point either side of it, against the spec's own intervals, so
// an interval shifted or resized by one in either table fails here.
func TestEndpointsAndNeighbours(t *testing.T) {
	start := specProduction(t, "NT-NameStartChar")
	name := append(append([]specInterval(nil), start...), specProduction(t, "NT-NameChar")...)
	for _, set := range [][]specInterval{start, name} {
		for _, iv := range set {
			for _, c := range []rune{iv.lo - 1, iv.lo, iv.hi, iv.hi + 1} {
				if got, want := unicode.Is(NameStartChar, c), inSpec(start, c); got != want {
					t.Errorf("NameStartChar(%U) = %v, [4] says %v", c, got, want)
				}
				if got, want := unicode.In(c, NameStartChar, NameCharExtra), inSpec(name, c); got != want {
					t.Errorf("NameChar(%U) = %v, [4a] says %v", c, got, want)
				}
			}
		}
	}
}

// TestMultiplicationSignIsNoNameChar pins the gap [4] leaves between #xD6 and
// #xD8: U+00D7 '×' is neither a NameStartChar nor a NameChar.
func TestMultiplicationSignIsNoNameChar(t *testing.T) {
	if unicode.Is(NameStartChar, '×') {
		t.Error("U+00D7 is a NameStartChar; [4] skips it")
	}
	if unicode.In('×', NameStartChar, NameCharExtra) {
		t.Error("U+00D7 is a NameChar; [4a] skips it")
	}
}

// TestIsNCName pins Namespaces in XML [4]: a Name, so a NameStartChar first
// and NameChars after, with no ':' anywhere.
func TestIsNCName(t *testing.T) {
	for _, tc := range []struct {
		s    string
		want bool
	}{
		{"t", true},
		{"_x-1.é", true},
		{"", false},
		{"1x", false},
		{"-x", false},
		{"a b", false},
		{"a:b", false},
		{":a", false},
		{"a×", false},
	} {
		if got := IsNCName(tc.s); got != tc.want {
			t.Errorf("IsNCName(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}
