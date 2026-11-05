package codec

import (
	"errors"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/xsderr"
)

// TestDecodeURI pins Decode's URI paragraph: under WithURI every instance Loc
// an error carries names the document, and with no option, or WithURI(""),
// it renders "?". The violation, both source-fault channels and the
// undecided check read their Loc from validate/xmlsrc.Validate's read, and
// the two declines from codec's own parser/xmltree read, so each row fails
// when the URI is dropped at the read it names.
func TestDecodeURI(t *testing.T) {
	const nillable = `<xs:element name="r" nillable="true"><xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType></xs:element>`
	const xsiType = `<xs:complexType name="rt"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>
		<xs:element name="r" type="s:rt"/>`
	// viaLocOf reads the Loc xsderr.LocOf reaches.
	viaLocOf := func(err error) (string, bool) {
		loc, ok := xsderr.LocOf(err)
		return loc.String(), ok
	}
	// viaDecline reads the Loc of the *DeclineError err wraps.
	viaDecline := func(err error) (string, bool) {
		var de *DeclineError
		if !errors.As(err, &de) {
			return "", false
		}
		loc, charged := de.Loc()
		return loc.String(), charged
	}
	// viaUndecided reads the Loc of the *UndecidedError err wraps.
	viaUndecided := func(err error) (string, bool) {
		var ue *UndecidedError
		if !errors.As(err, &ue) {
			return "", false
		}
		return ue.Unevaluated().Loc().String(), true
	}
	rows := []struct {
		name  string
		decls string
		data  []byte
		loc   func(error) (string, bool)
		at    string // the Loc's line:col
	}{
		{"violation, the validator's read", shapesSchema, instanceOf(` req="r"`, "<s:one>x</s:one>"), viaLocOf, "1:1"},
		{"source fault in the walk, the validator's read", shapesSchema, []byte(`<s:r xmlns:s="urn:s" req="r"><s:one>x</s:one>`), viaLocOf, "1:1"},
		{"source fault after the walk, the validator's read", shapesSchema, []byte(`<s:r xmlns:s="urn:s" req="r"><s:one>x</s:one><s:tok>t</s:tok></s:r><bad`), viaLocOf, "1:72"},
		{"undecided, the validator's read", undecidedSchema, instanceOf("", "<s:a>x</s:a>"), viaUndecided, "1:1"},
		{"document element decline, codec's read", nillable, instanceOf("", "<s:a>x</s:a>"), viaDecline, "1:1"},
		{"child xsi:type decline, codec's read", xsiType, instanceOf("", `<s:a xsi:type="xs:token"> x  y </s:a>`), viaDecline, "1:120"},
	}
	uris := []struct {
		name string
		opts []Option
		want string
	}{
		{"WithURI", []Option{WithURI("u.xml")}, "u.xml"},
		{"no option", nil, "?"},
		{"WithURI empty", []Option{WithURI("")}, "?"},
	}
	for _, row := range rows {
		schema := schemaOf(t, row.decls)
		for _, u := range uris {
			t.Run(row.name+"/"+u.name, func(t *testing.T) {
				got, err := Decode[struct{ A string }](schema, strict.New(), row.data, u.opts...)
				if got != nil || err == nil {
					t.Fatalf("Decode = (%v, %v), want no value and an error", got, err)
				}
				loc, ok := row.loc(err)
				if !ok {
					t.Fatalf("error %v carries no Loc this row reads", err)
				}
				if want := u.want + ":" + row.at; loc != want {
					t.Errorf("Loc = %s, want %s: %v", loc, want, err)
				}
			})
		}
	}
}
