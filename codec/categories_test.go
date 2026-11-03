package codec

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/codec/testdata/readme/order"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/validate/xmlsrc"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// categoryRoutes are the routes Decode's doc gives a caller to its error
// categories, items 2 to 6, in that order.
var categoryRoutes = []struct {
	name  string
	match func(error) bool
}{
	{"source fault", func(err error) bool {
		rule, _ := xsderr.RuleOf(err)
		return rule == xsderr.RuleXMLWellFormed
	}},
	{"charged", func(err error) bool { return ViolationsOf(err) != nil }},
	{"undecided", func(err error) bool {
		var ue *UndecidedError
		return errors.As(err, &ue)
	}},
	{"decline", func(err error) bool {
		var de *DeclineError
		return errors.As(err, &de)
	}},
	{"T mismatch", func(err error) bool { return errors.Is(err, ErrTypeMismatch) }},
}

// TestDecodeErrorCategories pins that each of Decode's error categories,
// items 2 to 6 of its doc, is reached by its own route and by no other
// category's, wrapped once more by a caller or not.
func TestDecodeErrorCategories(t *testing.T) {
	shapesValid := instanceOf(` req="r"`, "<s:one>x</s:one><s:tok>t</s:tok>")
	rows := []struct {
		route  string
		decode func() error
	}{
		{"source fault", func() error {
			_, err := Decode[shapes](schemaOf(t, shapesSchema), strict.New(), []byte(`<s:r xmlns:s="urn:s" req="r"><s:one>x</s:one>`))
			return err
		}},
		{"charged", func() error {
			_, err := Decode[order.Order](readmeOrderSchema(t), strict.New(), invalidOrder, WithURI("order.xml"))
			return err
		}},
		{"undecided", func() error {
			_, err := Decode[struct{ A string }](schemaOf(t, undecidedSchema), strict.New(), instanceOf("", "<s:a>x</s:a>"))
			return err
		}},
		{"decline", func() error {
			_, err := Decode[struct{ A string }](schemaOf(t, `<xs:element name="r" nillable="true"><xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType></xs:element>`), strict.New(), instanceOf("", "<s:a>x</s:a>"))
			return err
		}},
		{"T mismatch", func() error {
			_, err := Decode[struct{ One string }](schemaOf(t, shapesSchema), strict.New(), shapesValid)
			return err
		}},
	}
	for _, row := range rows {
		err := row.decode()
		if err == nil {
			t.Fatalf("%s: Decode returned no error", row.route)
		}
		for _, wrapped := range []error{err, fmt.Errorf("caller: %w", err)} {
			for _, r := range categoryRoutes {
				if got := r.match(wrapped); got != (r.name == row.route) {
					t.Errorf("%s row: the %s route reports %v for %v", row.route, r.name, got, wrapped)
				}
			}
		}
	}
}

// invalidOrder is ExampleDecode's instance that README's order.xsd rejects
// with two violations at distinct Locs.
var invalidOrder = []byte(`<o:order xmlns:o="urn:o" kind="x" size="2">
  <o:note>n</o:note>
</o:order>`)

// undecidedSchema declares r with an assertion whose for expression the XPath
// subset declines, so validate leaves it undecided (cvc-assertion).
const undecidedSchema = `<xs:element name="r"><xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence>
		<xs:assert test="(for $x in s:a return string-length($x)) = 1"/></xs:complexType></xs:element>`

// readmeOrderSchema parses README's order.xsd.
func readmeOrderSchema(t *testing.T) *xsd.Schema {
	t.Helper()
	schema, err := parser.Parse("order.xsd", parser.WithResolver(loader.Dir(filepath.Join("..", "testdata", "readme"))))
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

// TestViolationsOf pins item 3 of Decode's doc and ViolationsOf's: every
// violation of a charged instance, in Result.Violations order, through any
// number of wraps, in a fresh slice, the error's text unchanged, and
// xsderr.RuleOf reaching the first violation, whose rule is no source fault's.
func TestViolationsOf(t *testing.T) {
	schema := readmeOrderSchema(t)
	v, err := validate.New(schema, strict.New())
	if err != nil {
		t.Fatal(err)
	}
	res, err := xmlsrc.Validate(v, bytes.NewReader(invalidOrder), xmlsrc.WithURI("order.xml"))
	if err != nil {
		t.Fatal(err)
	}
	want := res.Violations()
	if len(want) < 2 || want[0].Loc == want[1].Loc {
		t.Fatalf("the fixture yields violations %v, want two at distinct Locs", want)
	}
	_, err = Decode[order.Order](schema, strict.New(), invalidOrder, WithURI("order.xml"))
	for _, wrapped := range []error{err, fmt.Errorf("caller: %w", err), fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", err))} {
		got := ViolationsOf(wrapped)
		if len(got) != len(want) {
			t.Fatalf("ViolationsOf(%v) holds %d violations, want %d", wrapped, len(got), len(want))
		}
		for i := range want {
			if got[i].Rule != want[i].Rule || got[i].Loc != want[i].Loc || got[i].Error() != want[i].Error() {
				t.Errorf("ViolationsOf(%v)[%d] = %v, want %v", wrapped, i, got[i], want[i])
			}
		}
	}
	if rule, _ := xsderr.RuleOf(err); rule != want[0].Rule || rule == xsderr.RuleXMLWellFormed {
		t.Errorf("RuleOf = %q, want the first violation's %s", rule, want[0].Rule)
	}
	errs := make([]error, len(want))
	for i, w := range want {
		errs[i] = w
	}
	if text := fmt.Errorf("codec: the instance is invalid against the schema: %w", errors.Join(errs...)).Error(); err.Error() != text {
		t.Errorf("error = %q, want %q", err, text)
	}
	first := ViolationsOf(err)
	first[0] = nil
	if again := ViolationsOf(err); again[0] == nil {
		t.Error("mutating ViolationsOf's slice changed the error's violations")
	}
	if got := ViolationsOf(nil); got != nil {
		t.Errorf("ViolationsOf(nil) = %v, want nil", got)
	}
}

// TestUndecidedError pins item 4 of Decode's doc: the *UndecidedError's
// Unevaluated is the Result's first, through a caller's wrap, it carries no
// rule xsderr.RuleOf reaches, and a zero UndecidedError renders without
// panicking.
func TestUndecidedError(t *testing.T) {
	schema := schemaOf(t, undecidedSchema)
	data := instanceOf("", "<s:a>x</s:a>")
	v, err := validate.New(schema, strict.New())
	if err != nil {
		t.Fatal(err)
	}
	res, err := xmlsrc.Validate(v, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if res.Standing() != validate.Undecided {
		t.Fatalf("Standing = %d, want Undecided", res.Standing())
	}
	_, err = Decode[struct{ A string }](schema, strict.New(), data)
	var ue *UndecidedError
	if !errors.As(fmt.Errorf("caller: %w", err), &ue) {
		t.Fatalf("error %v holds no *UndecidedError", err)
	}
	if got, want := ue.Unevaluated(), res.Unevaluated()[0]; got != want {
		t.Errorf("Unevaluated = %+v, want %+v", got, want)
	}
	if rule, isRule := xsderr.RuleOf(err); isRule {
		t.Errorf("RuleOf = %q, want no rule", rule)
	}
	const zero = "codec: ?: the validator left  undecided, so the instance is not proven valid: unsupported operation"
	if got := (&UndecidedError{}).Error(); got != zero {
		t.Errorf("zero UndecidedError = %q, want %q", got, zero)
	}
}
