package jsonsrc

import "io"

// positions is the reader the json.Decoder pulls from, which keeps the line
// and column of every token it is asked to locate. json.Decoder reports a
// byte offset alone (InputOffset), so positions holds the bytes read past the
// last token located — the decoder's read-ahead and no more — and counts the
// newlines in them as the walk moves past. It never holds the document (STYLE
// P4).
//
// Lines are counted at '\n' and columns in bytes from the start of the line,
// both from 1, as parser/xmltree counts them.
type positions struct {
	r io.Reader
	// pending is every byte read from r at or after base, the offset of the
	// last token located.
	pending []byte
	base    int64
	// line and col are base's position.
	line, col int
}

func newPositions(r io.Reader) *positions {
	return &positions{r: r, line: 1, col: 1}
}

func (p *positions) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.pending = append(p.pending, b[:n]...)
	return n, err
}

// start locates the token that follows offset off, the decoder's position
// before it was read: the first byte at or after off that is neither JSON
// white space nor the ':' or ',' separator Token consumes silently. It
// returns that byte's line and column, and forgets every byte before it. off
// is never before the last token located, because the decoder's offset only
// advances. Past the bytes read so far — the input ended — it locates the end.
func (p *positions) start(off int64) (line, col int) {
	i := int(off - p.base)
	for i < len(p.pending) && skipped(p.pending[i]) {
		i++
	}
	if i > len(p.pending) {
		i = len(p.pending)
	}
	for _, c := range p.pending[:i] {
		if c == '\n' {
			p.line, p.col = p.line+1, 1
			continue
		}
		p.col++
	}
	p.pending = p.pending[i:]
	p.base += int64(i)
	return p.line, p.col
}

// skipped reports whether c is JSON white space (RFC 8259 §2, ws) or a
// separator between tokens.
func skipped(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', ':', ',':
		return true
	}
	return false
}
