package jsonsrc

import (
	"bytes"
	"errors"
	"io"
)

// positions is the reader the json.Decoder pulls from, which keeps the line
// and column of every token it is asked to locate. json.Decoder reports a
// byte offset alone (InputOffset), so positions holds the bytes read past the
// last token located — the decoder's read-ahead and no more — and counts the
// newlines in them as the walk moves past. It never holds the document (STYLE
// P4).
//
// Lines are counted at '\n' and columns in bytes from the start of the line,
// both from 1, as parser/xmltree counts them. A leading byte order mark is
// never counted: r drops it before any byte reaches positions, as xmltree's
// decoding discards it before tokenizing.
type positions struct {
	r *bomless
	// pending is every byte read from r at or after base, the offset of the
	// last token located.
	pending []byte
	base    int64
	// line and col are base's position.
	line, col int
	// fault is the first error other than io.EOF that r returned: a token
	// error that is r's own is a fault reading the input, and any other is
	// the decoder's verdict on the bytes, whatever its type (#2655).
	fault error
}

func newPositions(r io.Reader) *positions {
	return &positions{r: &bomless{r: r}, line: 1, col: 1}
}

func (p *positions) Read(b []byte) (int, error) {
	n, err := p.r.read(b)
	p.pending = append(p.pending, b[:n]...)
	if err != nil && !errors.Is(err, io.EOF) && p.fault == nil {
		p.fault = err
	}
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

// utf8BOM is U+FEFF, the byte order mark, in UTF-8.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// bomless is r with one leading UTF-8 byte order mark dropped, which RFC 8259
// §8.1 lets a parser ignore. Only the document's first three bytes are the
// mark: a U+FEFF anywhere else reaches the decoder, and [Validate] says what
// it answers for one. It reads r a byte at a time until the mark is ruled in
// or out, so a mark split across reads is dropped whatever sizes r returns,
// and an error r returns meanwhile is returned unchanged, after the bytes
// read before it that are no part of a mark.
type bomless struct {
	r io.Reader
	// ruled reports whether the leading bytes have been read far enough to
	// rule the mark in or out.
	ruled bool
	// lead is the leading bytes read while ruling that read has yet to
	// return; once ruled, it never holds the whole mark.
	lead []byte
	// err is the error r returned while ruling, returned once lead is.
	err error
}

// read is io.Reader's Read over r with the mark dropped.
func (b *bomless) read(p []byte) (int, error) {
	if !b.ruled {
		b.rule()
	}
	if len(b.lead) > 0 {
		n := copy(p, b.lead)
		b.lead = b.lead[n:]
		return n, nil
	}
	if b.err != nil {
		return 0, b.err
	}
	return b.r.Read(p)
}

// rule reads r while every byte read is the mark's next, until lead holds the
// whole mark or a byte that is not the mark's, or r returns an error, which it
// keeps as err. It drops the whole mark, and the beginning of one that a fault
// other than io.EOF cut off, so that fault is the first thing read returns
// rather than a byte the decoder rejects before reading on to it.
func (b *bomless) rule() {
	b.ruled = true
	var c [1]byte
	for len(b.lead) < len(utf8BOM) && bytes.HasPrefix(utf8BOM, b.lead) {
		n, err := b.r.Read(c[:])
		b.lead = append(b.lead, c[:n]...)
		if err != nil {
			b.err = err
			break
		}
	}
	cut := b.err != nil && !errors.Is(b.err, io.EOF) && bytes.HasPrefix(utf8BOM, b.lead)
	if cut || bytes.Equal(b.lead, utf8BOM) {
		b.lead = nil
	}
}
