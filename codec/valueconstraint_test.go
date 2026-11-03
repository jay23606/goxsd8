package codec

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
)

// decodesTo fails the test unless validate accepts the instance of attrs and
// content against the schema of decls, and Decode decodes it into want.
func decodesTo[T any](t *testing.T, decls, attrs, content string, want T) {
	t.Helper()
	schema := schemaOf(t, decls)
	data := instanceOf(attrs, content)
	mustAccept(t, schema, data)
	got, err := Decode[T](schema, strict.New(), data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("Decode = %s, want %s", deref(*got), deref(want))
	}
}

// deref prints s with its pointer fields followed.
func deref(s any) string {
	v := reflect.ValueOf(s)
	out := "{"
	for i := range v.NumField() {
		f := v.Field(i)
		out += v.Type().Field(i).Name + ":"
		switch {
		case f.Kind() == reflect.Pointer && f.IsNil():
			out += "nil"
		case f.Kind() == reflect.Pointer:
			out += fmt.Sprintf("&%q", f.Elem().Interface())
		default:
			out += fmt.Sprintf("%q", f.Interface())
		}
		out += " "
	}
	return out + "}"
}

// TestDecodeElementValueConstraint pins Decode's doc on a child element whose
// declaration has a {value constraint}, default or fixed (cvc-elt clause
// 5.1): one row per occurrence shape and per kind of children, a comment, a
// whitespace run and none.
func TestDecodeElementValueConstraint(t *testing.T) {
	str := func(s string) *string { return &s }
	type one struct{ A string }
	rows := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"default, present", func(t *testing.T) {
			decodesTo(t, sequenceOf(`<xs:element name="a" type="xs:string" default="d"/>`), "", `<s:a>x</s:a>`, one{A: "x"})
		}},
		{"default, present and empty", func(t *testing.T) {
			decodesTo(t, sequenceOf(`<xs:element name="a" type="xs:string" default="d"/>`), "", `<s:a/>`, one{A: "d"})
		}},
		{"fixed, present", func(t *testing.T) {
			decodesTo(t, sequenceOf(`<xs:element name="a" type="xs:string" fixed="f"/>`), "", `<s:a>f</s:a>`, one{A: "f"})
		}},
		{"fixed, present and empty", func(t *testing.T) {
			decodesTo(t, sequenceOf(`<xs:element name="a" type="xs:string" fixed="f"/>`), "", `<s:a></s:a>`, one{A: "f"})
		}},
		{"optional default, present and empty", func(t *testing.T) {
			decodesTo(t, sequenceOf(`<xs:element name="a" type="xs:string" default="d" minOccurs="0"/>`), "", `<s:a/>`, struct{ A *string }{A: str("d")})
		}},
		{"optional default, absent", func(t *testing.T) {
			decodesTo(t, sequenceOf(`<xs:element name="a" type="xs:string" default="d" minOccurs="0"/>`), "", ``, struct{ A *string }{})
		}},
		{"repeated default, each empty occurrence", func(t *testing.T) {
			decodesTo(t, sequenceOf(`<xs:element name="a" type="xs:string" default="d" maxOccurs="unbounded"/>`), "", `<s:a>p</s:a><s:a/><s:a></s:a>`,
				struct{ A []string }{A: []string{"p", "d", "d"}})
		}},
		{"default, a comment its only child", func(t *testing.T) {
			decodesTo(t, sequenceOf(`<xs:element name="a" type="xs:string" default="d"/>`), "", `<s:a><!--c--></s:a>`, one{A: "d"})
		}},
		{"default, whitespace its only child", func(t *testing.T) {
			decodesTo(t, sequenceOf(`<xs:element name="a" type="xs:token" default="d"/>`), "", `<s:a> </s:a>`, one{A: ""})
		}},
		{"default normalized by the type's whiteSpace", func(t *testing.T) {
			decodesTo(t, sequenceOf(`<xs:element name="a" type="xs:token" default="  d   e "/>`), "", `<s:a/>`, one{A: "d e"})
		}},
	}
	for _, row := range rows {
		t.Run(row.name, row.run)
	}
}

// TestDecodeAttributeValueConstraint pins Decode's doc on an attribute use
// with an effective value constraint, default or fixed (key-dflt-att), whose
// field is a string: the attribute present, present and empty, and absent,
// the constraint the use's own or its declaration's (§3.5.4 key-evc), and a
// *string field ErrTypeMismatch (item 6).
func TestDecodeAttributeValueConstraint(t *testing.T) {
	const a = `<s:a>x</s:a>`
	type val struct{ A, B string }
	uses := func(attrs string) string {
		return root(`<xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence>` + attrs)
	}
	global := `<xs:attribute name="b" type="xs:string" default="g"/>`
	rows := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"default, present", func(t *testing.T) {
			decodesTo(t, uses(`<xs:attribute name="b" type="xs:string" default="d"/>`), ` b="v"`, a, val{A: "x", B: "v"})
		}},
		{"default, present and empty", func(t *testing.T) {
			decodesTo(t, uses(`<xs:attribute name="b" type="xs:string" default="d"/>`), ` b=""`, a, val{A: "x", B: ""})
		}},
		{"default, absent", func(t *testing.T) {
			decodesTo(t, uses(`<xs:attribute name="b" type="xs:string" default="d"/>`), "", a, val{A: "x", B: "d"})
		}},
		{"fixed, present", func(t *testing.T) {
			decodesTo(t, uses(`<xs:attribute name="b" type="xs:string" fixed="f"/>`), ` b="f"`, a, val{A: "x", B: "f"})
		}},
		{"fixed, absent", func(t *testing.T) {
			decodesTo(t, uses(`<xs:attribute name="b" type="xs:string" fixed="f"/>`), "", a, val{A: "x", B: "f"})
		}},
		{"required and fixed, present", func(t *testing.T) {
			decodesTo(t, uses(`<xs:attribute name="b" type="xs:string" use="required" fixed="f"/>`), ` b="f"`, a, val{A: "x", B: "f"})
		}},
		{"the declaration's default, absent", func(t *testing.T) {
			decodesTo(t, global+uses(`<xs:attribute ref="s:b"/>`), "", a, val{A: "x", B: "g"})
		}},
		{"the use's own default over the declaration's, absent", func(t *testing.T) {
			decodesTo(t, global+uses(`<xs:attribute ref="s:b" default="u"/>`), "", a, val{A: "x", B: "u"})
		}},
		{"default normalized by the type's whiteSpace, absent", func(t *testing.T) {
			decodesTo(t, uses(`<xs:attribute name="b" type="xs:token" default="  x  y "/>`), "", a, val{A: "x", B: "x y"})
		}},
		{"default, the field a *string", func(t *testing.T) {
			type opt struct {
				A string
				B *string
			}
			schema := schemaOf(t, uses(`<xs:attribute name="b" type="xs:string" default="d"/>`))
			got, err := Decode[opt](schema, strict.New(), instanceOf("", a))
			const want = "codec: T is not the struct codegen emits for the document element: field 1 of codec.opt is B *string where codegen emits B string"
			if got != nil || !errors.Is(err, ErrTypeMismatch) || err.Error() != want {
				t.Errorf("Decode = (%v, %v), want no value and %q", got, err, want)
			}
		}},
	}
	for _, row := range rows {
		t.Run(row.name, row.run)
	}
}

// TestDecodeAnyURI pins that an xs:anyURI element and attribute decode as a
// string, the identity lexical mapping (Datatypes §3.3.17) applied after
// whiteSpace collapse, which xs:anyURI fixes.
func TestDecodeAnyURI(t *testing.T) {
	str := func(s string) *string { return &s }
	decls := root(`<xs:sequence><xs:element name="a" type="xs:anyURI"/></xs:sequence><xs:attribute name="b" type="xs:anyURI"/>`)
	decodesTo(t, decls, ` b="  http://e.x/b &#9; c  "`, "<s:a>  http://e.x/a \t b  </s:a>",
		struct {
			A string
			B *string
		}{A: "http://e.x/a b", B: str("http://e.x/b c")})
}
