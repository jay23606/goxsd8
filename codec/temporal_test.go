package codec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"log/slog"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"unicode"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// temporalSchema declares s:r of one local element per date, time and
// duration datatype struct codegen emits, a local list of xs:date, refs to w,
// of xs:date, and wl, a list of xs:date, an optional xs:date, and an optional
// attribute of xs:date; s:w and s:wl are simple roots too.
const temporalSchema = `<xs:element name="w" type="xs:date"/><xs:element name="wl"><xs:simpleType><xs:list itemType="xs:date"/></xs:simpleType></xs:element>
<xs:element name="r"><xs:complexType><xs:sequence>
<xs:element name="a" type="xs:dateTime"/><xs:element name="b" type="xs:dateTimeStamp"/><xs:element name="c" type="xs:time"/><xs:element name="d" type="xs:date"/>
<xs:element name="e" type="xs:gYearMonth"/><xs:element name="f" type="xs:gYear"/><xs:element name="g" type="xs:gMonthDay"/><xs:element name="h" type="xs:gDay"/>
<xs:element name="i" type="xs:gMonth"/><xs:element name="j" type="xs:duration"/><xs:element name="k" type="xs:dayTimeDuration"/><xs:element name="l" type="xs:yearMonthDuration"/>
<xs:element name="m"><xs:simpleType><xs:list itemType="xs:date"/></xs:simpleType></xs:element><xs:element ref="s:w"/><xs:element ref="s:wl"/><xs:element name="o" type="xs:date" minOccurs="0"/>
</xs:sequence><xs:attribute name="at" type="xs:date"/></xs:complexType></xs:element>`

// tDateTime to tYearMonthDuration are the twelve datatype structs codegen
// emits, DateTime to YearMonthDuration, and temporals, tW and tWl its R, W
// and Wl for temporalSchema, which TestTemporalTypesAreCodegens pins.
type (
	tDateTime struct {
		Year           big.Int
		Month          uint8
		Day            uint8
		Hour           uint8
		Minute         uint8
		Second         big.Rat
		TimezoneOffset *int16
	}
	tDateTimeStamp struct {
		Year           big.Int
		Month          uint8
		Day            uint8
		Hour           uint8
		Minute         uint8
		Second         big.Rat
		TimezoneOffset int16
	}
	tTime struct {
		Hour           uint8
		Minute         uint8
		Second         big.Rat
		TimezoneOffset *int16
	}
	tDate struct {
		Year           big.Int
		Month          uint8
		Day            uint8
		TimezoneOffset *int16
	}
	tGYearMonth struct {
		Year           big.Int
		Month          uint8
		TimezoneOffset *int16
	}
	tGYear struct {
		Year           big.Int
		TimezoneOffset *int16
	}
	tGMonthDay struct {
		Month          uint8
		Day            uint8
		TimezoneOffset *int16
	}
	tGDay struct {
		Day            uint8
		TimezoneOffset *int16
	}
	tGMonth struct {
		Month          uint8
		TimezoneOffset *int16
	}
	tDuration struct {
		Months  big.Int
		Seconds big.Rat
	}
	tDayTimeDuration   struct{ Seconds big.Rat }
	tYearMonthDuration struct{ Months big.Int }
	temporals          struct {
		A  tDateTime
		B  tDateTimeStamp
		C  tTime
		D  tDate
		E  tGYearMonth
		F  tGYear
		G  tGMonthDay
		H  tGDay
		I  tGMonth
		J  tDuration
		K  tDayTimeDuration
		L  tYearMonthDuration
		M  []tDate
		W  tW
		Wl tWl
		O  *tDate
		At *tDate
	}
	tW  tDate
	tWl []tDate
)

// datatypeTypes are the test's datatype structs, indexed by the atomic each
// is codegen's struct for.
var datatypeTypes = map[atomic]reflect.Type{
	atomicDateTime:          reflect.TypeFor[tDateTime](),
	atomicDateTimeStamp:     reflect.TypeFor[tDateTimeStamp](),
	atomicTime:              reflect.TypeFor[tTime](),
	atomicDate:              reflect.TypeFor[tDate](),
	atomicGYearMonth:        reflect.TypeFor[tGYearMonth](),
	atomicGYear:             reflect.TypeFor[tGYear](),
	atomicGMonthDay:         reflect.TypeFor[tGMonthDay](),
	atomicGDay:              reflect.TypeFor[tGDay](),
	atomicGMonth:            reflect.TypeFor[tGMonth](),
	atomicDuration:          reflect.TypeFor[tDuration](),
	atomicDayTimeDuration:   reflect.TypeFor[tDayTimeDuration](),
	atomicYearMonthDuration: reflect.TypeFor[tYearMonthDuration](),
	atomicQName:             reflect.TypeFor[tQName](),
}

// structBody renders t, a struct type, as codegen declares it under name:
// gofmt's "type name struct" with one line per field.
func structBody(t *testing.T, name string, st reflect.Type) string {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "type %s struct {\n", name)
	for i := range st.NumField() {
		fmt.Fprintf(&b, "\t%s %s\n", st.Field(i).Name, st.Field(i).Type)
	}
	b.WriteString("}\n")
	out, err := format.Source([]byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// TestDatatypeMirrorsAreCodegens pins each of codec's thirteen datatype struct
// mirrors (atomicTypes) to the struct codegen emits for its datatype, in
// field names, order and Go types, and each of this test's datatype structs
// to its mirror, so that a mirror drifting from codegen fails here.
func TestDatatypeMirrorsAreCodegens(t *testing.T) {
	gen := string(generated(t, schemaOf(t, temporalSchema))) + string(generated(t, schemaOf(t, qnameSchema)))
	mirrored := 0
	for a := atomic(1); int(a) < len(atomicTypes); a++ {
		if !a.mirrored() {
			continue
		}
		mirrored++
		local := datatypeNames[a]
		name := string(unicode.ToUpper(rune(local[0]))) + local[1:]
		if want := structBody(t, name, a.bare()); !strings.Contains(gen, want) {
			t.Errorf("codegen emits\n%s\nwhich does not declare the mirror of xs:%s,\n%s", gen, local, want)
		}
		if got, want := structBody(t, name, datatypeTypes[a]), structBody(t, name, a.bare()); got != want {
			t.Errorf("the test's struct for xs:%s is\n%s\nwhere the mirror is\n%s", local, got, want)
		}
	}
	if mirrored != len(datatypeTypes) {
		t.Errorf("%d mirrored atomics, want %d", mirrored, len(datatypeTypes))
	}
}

// TestTemporalTypesAreCodegens pins temporals, tW and tWl to codegen's R, W
// and Wl for temporalSchema.
func TestTemporalTypesAreCodegens(t *testing.T) {
	gen := generated(t, schemaOf(t, temporalSchema))
	for _, decl := range []string{
		"type W Date\n", "type Wl []Date\n",
		"type R struct {\n\tA  DateTime\n\tB  DateTimeStamp\n\tC  Time\n\tD  Date\n\tE  GYearMonth\n\tF  GYear\n\tG  GMonthDay\n\tH  GDay\n" +
			"\tI  GMonth\n\tJ  Duration\n\tK  DayTimeDuration\n\tL  YearMonthDuration\n\tM  []Date\n\tW  W\n\tWl Wl\n\tO  *Date\n\tAt *Date\n}\n",
	} {
		if !bytes.Contains(gen, []byte(decl)) {
			t.Errorf("codegen emits\n%s\nwhich does not declare\n%s", gen, decl)
		}
	}
}

// spellValue spells v for a comparison: a struct as {Field:value ...}, a big.Int
// and a big.Rat by their strings, a pointer as nil or its target, and a slice
// as [items].
func spellValue(v reflect.Value) string {
	switch x := v.Interface().(type) {
	case big.Int:
		return x.String()
	case big.Rat:
		return x.RatString()
	}
	switch v.Kind() {
	case reflect.Struct:
		parts := make([]string, v.NumField())
		for i := range parts {
			parts[i] = v.Type().Field(i).Name + ":" + spellValue(v.Field(i))
		}
		return "{" + strings.Join(parts, " ") + "}"
	case reflect.Pointer:
		if v.IsNil() {
			return "nil"
		}
		return spellValue(v.Elem())
	case reflect.Slice:
		parts := make([]string, v.Len())
		for i := range parts {
			parts[i] = spellValue(v.Index(i))
		}
		return "[" + strings.Join(parts, " ") + "]"
	default:
		return fmt.Sprint(v.Interface())
	}
}

// TestDecodeTemporals pins the lift of the date, time and duration
// primitives (Datatypes §3.3.6–§3.3.14) and of dateTimeStamp, dayTimeDuration
// and yearMonthDuration (§3.4.26–§3.4.28): each value decodes into codegen's
// datatype struct, with and without a timezone where its datatype has one,
// its properties the local ones the lexical shows (§D.2.1), a year beyond
// int64 and a negative one in Year's big.Int, a fractional second in
// Second's big.Rat, hour 24 as the next day's 00 (§E.3.1), a duration's
// Months and Seconds of one sign, and a zero yearMonthDuration, which has no
// canonical form, as Months 0. CheckDecode returns nil for each row's T.
func TestDecodeTemporals(t *testing.T) {
	rows := []struct {
		typ, lexical string
		into         target
		want         string
	}{
		{"dateTime", "2002-10-10T12:00:00-05:00", into[struct{ A tDateTime }](), "{Year:2002 Month:10 Day:10 Hour:12 Minute:0 Second:0 TimezoneOffset:-300}"},
		{"dateTime", " 2002-10-10T12:00:00.50 ", into[struct{ A tDateTime }](), "{Year:2002 Month:10 Day:10 Hour:12 Minute:0 Second:1/2 TimezoneOffset:nil}"},
		{"dateTime", "2000-12-31T24:00:00Z", into[struct{ A tDateTime }](), "{Year:2001 Month:1 Day:1 Hour:0 Minute:0 Second:0 TimezoneOffset:0}"},
		{"dateTime", "123456789012345678901234-01-01T00:00:00", into[struct{ A tDateTime }](), "{Year:123456789012345678901234 Month:1 Day:1 Hour:0 Minute:0 Second:0 TimezoneOffset:nil}"},
		{"dateTime", "-0044-03-15T23:59:59.999+14:00", into[struct{ A tDateTime }](), "{Year:-44 Month:3 Day:15 Hour:23 Minute:59 Second:59999/1000 TimezoneOffset:840}"},
		{"dateTimeStamp", "2002-10-10T12:00:00-14:00", into[struct{ A tDateTimeStamp }](), "{Year:2002 Month:10 Day:10 Hour:12 Minute:0 Second:0 TimezoneOffset:-840}"},
		{"dateTimeStamp", "2002-10-10T12:00:00Z", into[struct{ A tDateTimeStamp }](), "{Year:2002 Month:10 Day:10 Hour:12 Minute:0 Second:0 TimezoneOffset:0}"},
		{"time", "13:20:00Z", into[struct{ A tTime }](), "{Hour:13 Minute:20 Second:0 TimezoneOffset:0}"},
		{"time", "13:20:30.25", into[struct{ A tTime }](), "{Hour:13 Minute:20 Second:121/4 TimezoneOffset:nil}"},
		{"date", "2000-02-29+13:00", into[struct{ A tDate }](), "{Year:2000 Month:2 Day:29 TimezoneOffset:780}"},
		{"date", "0000-01-01", into[struct{ A tDate }](), "{Year:0 Month:1 Day:1 TimezoneOffset:nil}"},
		{"gYearMonth", "1999-05", into[struct{ A tGYearMonth }](), "{Year:1999 Month:5 TimezoneOffset:nil}"},
		{"gYearMonth", "1999-05+05:30", into[struct{ A tGYearMonth }](), "{Year:1999 Month:5 TimezoneOffset:330}"},
		{"gYear", "-0001", into[struct{ A tGYear }](), "{Year:-1 TimezoneOffset:nil}"},
		{"gYear", "12345-00:00", into[struct{ A tGYear }](), "{Year:12345 TimezoneOffset:0}"},
		{"gMonthDay", "--02-29", into[struct{ A tGMonthDay }](), "{Month:2 Day:29 TimezoneOffset:nil}"},
		{"gMonthDay", "--12-25-03:00", into[struct{ A tGMonthDay }](), "{Month:12 Day:25 TimezoneOffset:-180}"},
		{"gDay", "---01", into[struct{ A tGDay }](), "{Day:1 TimezoneOffset:nil}"},
		{"gDay", "---31Z", into[struct{ A tGDay }](), "{Day:31 TimezoneOffset:0}"},
		{"gMonth", "--05", into[struct{ A tGMonth }](), "{Month:5 TimezoneOffset:nil}"},
		{"gMonth", "--11+00:00", into[struct{ A tGMonth }](), "{Month:11 TimezoneOffset:0}"},
		{"duration", "P1Y2M3DT4H5M6.5S", into[struct{ A tDuration }](), "{Months:14 Seconds:547813/2}"},
		{"duration", "-P1Y2M3DT4H5M6.5S", into[struct{ A tDuration }](), "{Months:-14 Seconds:-547813/2}"},
		{"duration", "P0Y13M", into[struct{ A tDuration }](), "{Months:13 Seconds:0}"},
		{"duration", "-PT0S", into[struct{ A tDuration }](), "{Months:0 Seconds:0}"},
		{"dayTimeDuration", "P1DT0.5S", into[struct{ A tDayTimeDuration }](), "{Seconds:172801/2}"},
		{"dayTimeDuration", "-PT1M", into[struct{ A tDayTimeDuration }](), "{Seconds:-60}"},
		{"yearMonthDuration", "P1Y2M", into[struct{ A tYearMonthDuration }](), "{Months:14}"},
		{"yearMonthDuration", "-P1Y2M", into[struct{ A tYearMonthDuration }](), "{Months:-14}"},
		{"yearMonthDuration", "P0M", into[struct{ A tYearMonthDuration }](), "{Months:0}"},
		{"yearMonthDuration", "-P0Y", into[struct{ A tYearMonthDuration }](), "{Months:0}"},
	}
	for _, row := range rows {
		t.Run(row.typ+" "+row.lexical, func(t *testing.T) {
			schema := schemaOf(t, sequenceOf(`<xs:element name="a" type="xs:`+row.typ+`"/>`))
			data := instanceOf("", "<s:a>"+row.lexical+"</s:a>")
			mustAccept(t, schema, data)
			got, err := row.into.decode(schema, data)
			if err != nil {
				t.Fatal(err)
			}
			if r := spellValue(reflect.ValueOf(got).Field(0)); r != row.want {
				t.Errorf("A = %s, want %s", r, row.want)
			}
			if err := row.into.check(schema); err != nil {
				t.Errorf("CheckDecode = %v, want nil", err)
			}
		})
	}
}

// TestDecodeTemporalSites pins the datatype structs at each site that holds
// one, against codegen's R: a sequence field of each, a local list of
// xs:date into []Date, a ref to an xs:date element into its named type, a
// ref to a list-of-date element into its named slice type, an optional
// element and an optional attribute; and a simple root of xs:date, of
// dateTimeStamp, and of a list of xs:date, named or not.
func TestDecodeTemporalSites(t *testing.T) {
	schema := schemaOf(t, temporalSchema)
	data := instanceOf(` at="2001-01-01Z"`, "<s:a>2002-10-10T12:00:00-05:00</s:a><s:b>2002-10-10T12:00:00Z</s:b><s:c>13:20:00</s:c><s:d>2000-01-02</s:d>"+
		"<s:e>1999-05</s:e><s:f>1999</s:f><s:g>--05-06</s:g><s:h>---07</s:h><s:i>--08</s:i><s:j>P1MT1S</s:j><s:k>PT2S</s:k><s:l>P3M</s:l>"+
		"<s:m>2000-01-01 2000-01-02Z</s:m><s:w>2003-03-03</s:w><s:wl> 2004-04-04 </s:wl><s:o>2005-05-05</s:o>")
	mustAccept(t, schema, data)
	got, err := Decode[temporals](schema, strict.New(), data)
	if err != nil {
		t.Fatal(err)
	}
	const want = "{A:{Year:2002 Month:10 Day:10 Hour:12 Minute:0 Second:0 TimezoneOffset:-300} B:{Year:2002 Month:10 Day:10 Hour:12 Minute:0 Second:0 TimezoneOffset:0}" +
		" C:{Hour:13 Minute:20 Second:0 TimezoneOffset:nil} D:{Year:2000 Month:1 Day:2 TimezoneOffset:nil} E:{Year:1999 Month:5 TimezoneOffset:nil}" +
		" F:{Year:1999 TimezoneOffset:nil} G:{Month:5 Day:6 TimezoneOffset:nil} H:{Day:7 TimezoneOffset:nil} I:{Month:8 TimezoneOffset:nil}" +
		" J:{Months:1 Seconds:1} K:{Seconds:2} L:{Months:3}" +
		" M:[{Year:2000 Month:1 Day:1 TimezoneOffset:nil} {Year:2000 Month:1 Day:2 TimezoneOffset:0}] W:{Year:2003 Month:3 Day:3 TimezoneOffset:nil}" +
		" Wl:[{Year:2004 Month:4 Day:4 TimezoneOffset:nil}] O:{Year:2005 Month:5 Day:5 TimezoneOffset:nil} At:{Year:2001 Month:1 Day:1 TimezoneOffset:0}}"
	if r := spellValue(reflect.ValueOf(*got)); r != want {
		t.Errorf("Decode =\n%s\nwant\n%s", r, want)
	}
	if err := CheckDecode[temporals](schema, rootName); err != nil {
		t.Errorf("CheckDecode = %v, want nil", err)
	}
	w, err := Decode[tW](schema, strict.New(), []byte(`<s:w xmlns:s="urn:s">2003-03-03-01:00</s:w>`))
	if err != nil || spellValue(reflect.ValueOf(*w)) != "{Year:2003 Month:3 Day:3 TimezoneOffset:-60}" {
		t.Errorf("Decode[tW] = %v, %v", w, err)
	}
	for _, root := range []struct {
		name string
		do   func() (any, error)
	}{
		{"named", func() (any, error) {
			v, err := Decode[tWl](schema, strict.New(), []byte(`<s:wl xmlns:s="urn:s">2004-04-04 2004-04-05</s:wl>`))
			return v, err
		}},
		{"unnamed", func() (any, error) {
			v, err := Decode[[]tDate](schema, strict.New(), []byte(`<s:wl xmlns:s="urn:s">2004-04-04 2004-04-05</s:wl>`))
			return v, err
		}},
	} {
		v, err := root.do()
		if err != nil {
			t.Errorf("Decode %s list root: %v", root.name, err)
			continue
		}
		if r := spellValue(reflect.ValueOf(v)); r != "[{Year:2004 Month:4 Day:4 TimezoneOffset:nil} {Year:2004 Month:4 Day:5 TimezoneOffset:nil}]" {
			t.Errorf("Decode %s list root = %s", root.name, r)
		}
	}
	stamp := schemaOf(t, `<xs:element name="r" type="xs:dateTimeStamp"/>`)
	s, err := Decode[tDateTimeStamp](stamp, strict.New(), instanceOf("", "2002-10-10T12:00:00+01:00"))
	if err != nil || s.TimezoneOffset != 60 {
		t.Errorf("Decode[tDateTimeStamp] = %v, %v; want TimezoneOffset 60", s, err)
	}
}

// TestDecodeTemporalNames pins item 6's reading of a datatype struct's field:
// codec holds no struct of codegen's to compare a name with, so a local
// xs:date field accepts a named type over Date and a ref's field Date itself,
// and a struct of codegen's fields carrying struct tags passes; a list's
// field still tells an unnamed []Date, a local one's, from a named slice
// type, a ref's.
func TestDecodeTemporalNames(t *testing.T) {
	type tagged struct {
		Year           big.Int `xml:"y"`
		Month          uint8   `json:"m"`
		Day            uint8
		TimezoneOffset *int16 `x:"z"`
	}
	schema := schemaOf(t, `<xs:element name="w" type="xs:date"/>`+sequenceOf(`<xs:element name="d" type="xs:date"/><xs:element ref="s:w"/>`))
	data := instanceOf("", "<s:d>2000-01-01</s:d><s:w>2000-01-02</s:w>")
	mustAccept(t, schema, data)
	for _, row := range []struct {
		name string
		into target
	}{
		{"a local field of a named type, a ref's of Date", into[struct {
			D tW
			W tDate
		}]()},
		{"struct tags", into[struct {
			D tagged
			W tagged
		}]()},
	} {
		got, err := row.into.decode(schema, data)
		if err != nil {
			t.Errorf("%s: %v", row.name, err)
			continue
		}
		if r := spellValue(reflect.ValueOf(got)); r != "{D:{Year:2000 Month:1 Day:1 TimezoneOffset:nil} W:{Year:2000 Month:1 Day:2 TimezoneOffset:nil}}" {
			t.Errorf("%s: Decode = %s", row.name, r)
		}
		if err := row.into.check(schema); err != nil {
			t.Errorf("%s: CheckDecode = %v, want nil", row.name, err)
		}
	}
}

// TestDecodeTemporalTypeMismatch pins item 6 over the datatype structs: a
// struct whose field set, order or field types differ from codegen's, a
// dateTime's struct for a dateTimeStamp and the reverse, a list's local field
// of a named slice type and a ref's of []Date, and a simple root of another
// struct are each ErrTypeMismatch, no decline; the message spells codegen's
// struct by its datatype's QName and fields, never by codec's mirror.
func TestDecodeTemporalTypeMismatch(t *testing.T) {
	type wideMonth struct {
		Year           big.Int
		Month          uint16
		Day            uint8
		TimezoneOffset *int16
	}
	type swapped struct {
		Year           big.Int
		Day            uint8
		Month          uint8
		TimezoneOffset *int16
	}
	type missing struct {
		Year  big.Int
		Month uint8
		Day   uint8
	}
	type required struct {
		Year           big.Int
		Month          uint8
		Day            uint8
		TimezoneOffset int16
	}
	type wideMonthR struct{ A wideMonth }
	type swappedR struct{ A swapped }
	type missingR struct{ A missing }
	type requiredR struct{ A required }
	type stampForDateTime struct{ A tDateTimeStamp }
	type dateTimeForStamp struct{ A tDateTime }
	type namedList struct{ A tWl }
	type unnamedRef struct{ Wl []tDate }
	type refOther struct{ W tGYear }
	const date = "D for the datatype struct D of xs:date (Year big.Int, Month uint8, Day uint8, TimezoneOffset *int16)"
	dateElem := sequenceOf(`<xs:element name="a" type="xs:date"/>`)
	dateData := instanceOf("", "<s:a>2000-01-01</s:a>")
	listOfDate := `<xs:element name="wl"><xs:simpleType><xs:list itemType="xs:date"/></xs:simpleType></xs:element>`
	rows := []struct {
		name, decls string
		data        []byte
		into        target
		want        string
	}{
		{"Month uint16", dateElem, dateData, into[wideMonthR](), "field 0 of codec.wideMonthR is A codec.wideMonth where codegen emits A " + date},
		{"Month and Day swapped", dateElem, dateData, into[swappedR](), "field 0 of codec.swappedR is A codec.swapped where codegen emits A " + date},
		{"TimezoneOffset missing", dateElem, dateData, into[missingR](), "field 0 of codec.missingR is A codec.missing where codegen emits A " + date},
		{"TimezoneOffset int16", dateElem, dateData, into[requiredR](), "field 0 of codec.requiredR is A codec.required where codegen emits A " + date},
		{"a dateTimeStamp's struct for a dateTime", sequenceOf(`<xs:element name="a" type="xs:dateTime"/>`), instanceOf("", "<s:a>2000-01-01T00:00:00Z</s:a>"),
			into[stampForDateTime](), "field 0 of codec.stampForDateTime is A codec.tDateTimeStamp where codegen emits A D for the datatype struct D of xs:dateTime (Year big.Int, Month uint8, Day uint8, Hour uint8, Minute uint8, Second big.Rat, TimezoneOffset *int16)"},
		{"a dateTime's struct for a dateTimeStamp", sequenceOf(`<xs:element name="a" type="xs:dateTimeStamp"/>`), instanceOf("", "<s:a>2000-01-01T00:00:00Z</s:a>"),
			into[dateTimeForStamp](), "field 0 of codec.dateTimeForStamp is A codec.tDateTime where codegen emits A D for the datatype struct D of xs:dateTimeStamp (Year big.Int, Month uint8, Day uint8, Hour uint8, Minute uint8, Second big.Rat, TimezoneOffset int16)"},
		{"a local list field of a named slice type", sequenceOf(`<xs:element name="a"><xs:simpleType><xs:list itemType="xs:date"/></xs:simpleType></xs:element>`), dateData,
			into[namedList](), "field 0 of codec.namedList is A codec.tWl where codegen emits A []" + date},
		{"a ref's list field of []Date", listOfDate + sequenceOf(`<xs:element ref="s:wl"/>`), instanceOf("", "<s:wl>2000-01-01</s:wl>"),
			into[unnamedRef](), "field 0 of codec.unnamedRef is Wl []codec.tDate where codegen emits Wl N for a named type N over []" + date},
		{"a ref's field of another struct", `<xs:element name="w" type="xs:date"/>` + sequenceOf(`<xs:element ref="s:w"/>`), instanceOf("", "<s:w>2000-01-01</s:w>"),
			into[refOther](), "field 0 of codec.refOther is W codec.tGYear where codegen emits W " + date},
		{"a simple root of another struct", `<xs:element name="r" type="xs:date"/>`, instanceOf("", "2000-01-01"),
			into[tGYearMonth](), "the underlying type of codec.tGYearMonth is not " + date},
		{"a list root of another struct", `<xs:element name="r"><xs:simpleType><xs:list itemType="xs:date"/></xs:simpleType></xs:element>`, instanceOf("", "2000-01-01"),
			into[[]tGYearMonth](), "the underlying type of []codec.tGYearMonth is not []" + date},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			schema := schemaOf(t, row.decls)
			mustAccept(t, schema, row.data)
			got, err := row.into.decode(schema, row.data)
			if got != nil {
				t.Errorf("Decode = %v, want no value", got)
			}
			if want := ErrTypeMismatch.Error() + ": " + row.want; err == nil || err.Error() != want {
				t.Errorf("error = %v\nwant    %q", err, want)
			}
			if err != nil && strings.Contains(err.Error(), "Mirror") {
				t.Errorf("error %v names codec's mirror", err)
			}
			var de *DeclineError
			if !errors.Is(err, ErrTypeMismatch) || errors.As(err, &de) {
				t.Errorf("error %v: want ErrTypeMismatch alone", err)
			}
			assertProbeMismatch(t, row.into.check(schema), err)
		})
	}
}

// overDatatype is strict.New() with the mapping of the XSD datatype local
// replaced by m(strict's own).
func overDatatype(local string, m func(base value.Mapping) value.Mapping) value.Backend {
	name := xsd.QName{Space: xsd.XMLSchemaNS, Local: local}
	base, _ := strict.New().Mapping(name)
	return value.Override(strict.New(), oneMapping{name: name, m: m(base)})
}

// TestDecodeTemporalStageErrors pins item 8 over the datatype structs: a
// canonical form a datatype struct does not read, or one denoting no value
// of its datatype, is a conversion error spelling codegen's struct, never
// codec's mirror; and a yearMonthDuration is converted from the normalized
// lexical Parse accepted, never from Canonical, so a backend whose Parse
// accepts a form no duration grammar reads fails there, though its Canonical
// renders a readable form.
func TestDecodeTemporalStageErrors(t *testing.T) {
	const tail = "; validate accepted the value on the same backend, so this is a fault of the backend, not of the instance"
	month13 := overDatatype("date", func(base value.Mapping) value.Mapping {
		return value.Mapping{Parse: base.Parse, Canonical: func(value.Value) (string, error) { return "2000-13-01", nil }}
	})
	feb30 := overDatatype("date", func(base value.Mapping) value.Mapping {
		return value.Mapping{Parse: base.Parse, Canonical: func(value.Value) (string, error) { return "2000-02-30", nil }}
	})
	lexicalX := overDatatype("yearMonthDuration", func(base value.Mapping) value.Mapping {
		return value.Mapping{Parse: func(lexical string, ctx value.Context) (value.Value, error) {
			if lexical == "x" {
				return base.Parse("P1Y", ctx)
			}
			return base.Parse(lexical, ctx)
		}, Canonical: base.Canonical}
	})
	date := sequenceOf(`<xs:element name="a" type="xs:date"/>`)
	dateData := instanceOf("", "<s:a>2000-01-01</s:a>")
	offset := bytes.Index(dateData, []byte("<s:a>")) + len("<s:a>")
	ymd := sequenceOf(`<xs:element name="a" type="xs:yearMonthDuration"/>`)
	ymdData := instanceOf("", "<s:a>x</s:a>")
	const dateStruct = "D for the datatype struct D of xs:date (Year big.Int, Month uint8, Day uint8, TimezoneOffset *int16)"
	rows := []struct {
		name, decls string
		data        []byte
		backend     value.Backend
		decode      func(*xsd.Schema, value.Backend, []byte) error
		want        string
	}{
		{"a month of 13", date, dateData, month13, decodeErr[struct{ A tDate }],
			fmt.Sprintf(`codec: ?:1:%d: the conversion to the Go type failed on "2000-01-01", a value of {http://www.w3.org/2001/XMLSchema}date in the character content of element {urn:s}a at byte offset %d: canonical form "2000-13-01" is no %s: the month 13 is outside 1 to 12 (§D.2.1)`, offset+1, offset, dateStruct)},
		{"February 30", date, dateData, feb30, decodeErr[struct{ A tDate }],
			fmt.Sprintf(`codec: ?:1:%d: the conversion to the Go type failed on "2000-01-01", a value of {http://www.w3.org/2001/XMLSchema}date in the character content of element {urn:s}a at byte offset %d: canonical form "2000-02-30" is no %s: the day 30 is outside 1 to 29 (§D.2.1)`, offset+1, offset, dateStruct)},
		{"a yearMonthDuration read from its lexical", ymd, ymdData, lexicalX, decodeErr[struct{ A tYearMonthDuration }],
			fmt.Sprintf(`codec: ?:1:%d: the conversion to the Go type failed on "x", a value of {http://www.w3.org/2001/XMLSchema}yearMonthDuration in the character content of element {urn:s}a at byte offset %d: it is no D for the datatype struct D of xs:yearMonthDuration (Months big.Int): 'x' at byte 0 where 'P' is expected`, offset+1, offset)},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			schema := schemaOf(t, row.decls)
			err := row.decode(schema, row.backend, row.data)
			var se *stageError
			if !errors.As(err, &se) || se.stage != stageConversion {
				t.Fatalf("Decode error = %v, want a stage error of %s", err, stageConversion.name())
			}
			if want := row.want + tail; err.Error() != want {
				t.Errorf("error = %q\nwant    %q", err, want)
			}
			var xe *xsderr.Error
			if errors.As(err, &xe) || errors.Is(err, ErrTypeMismatch) || errors.Is(err, errors.ErrUnsupported) {
				t.Errorf("error %v wraps something", err)
			}
		})
	}
}

// TestConvertTemporalRejects pins convertTemporal's refusals: a form of no
// datatype struct's grammar, or one denoting no value of the datatype, is an
// error, never a panic and never a struct no value maps to.
func TestConvertTemporalRejects(t *testing.T) {
	for _, row := range []struct {
		a    atomic
		form string
	}{
		{atomicDate, "2000-13-01"}, {atomicDate, "2000-00-01"}, {atomicDate, "1900-02-29"}, {atomicDate, "2000-04-31"},
		{atomicDate, "02000-01-01"}, {atomicDate, "200-01-01"}, {atomicDate, "2000-1-01"}, {atomicDate, "2000-01-01x"},
		{atomicDate, "2000-01-01+14:01"}, {atomicDate, "2000-01-01+15:00"}, {atomicDate, "2000-01-01+01:60"}, {atomicDate, "2000-01-01+0100"},
		{atomicDate, ""}, {atomicDateTime, "2000-01-01T24:00:00"}, {atomicDateTime, "2000-01-01T12:60:00"},
		{atomicDateTime, "2000-01-01T12:00:60"}, {atomicDateTime, "2000-01-01T12:00:00."}, {atomicDateTime, "2000-01-01 12:00:00"},
		{atomicDateTimeStamp, "2000-01-01T12:00:00"}, {atomicTime, "12:00"}, {atomicGMonthDay, "--02-30"}, {atomicGMonthDay, "-02-01"},
		{atomicGDay, "---32"}, {atomicGDay, "--01"}, {atomicGMonth, "--13"}, {atomicGYear, "99"}, {atomicGYearMonth, "1999"},
		{atomicDuration, "P"}, {atomicDuration, "PT"}, {atomicDuration, "P1.5Y"}, {atomicDuration, "P1M1Y"}, {atomicDuration, "PT1D"},
		{atomicDuration, "P1S"}, {atomicDuration, "1Y"}, {atomicDuration, "P1"}, {atomicDuration, "P1Y-1M"}, {atomicDuration, "P."},
		{atomicDayTimeDuration, "P1M"}, {atomicYearMonthDuration, "PT1S"}, {atomicYearMonthDuration, "P1D"},
	} {
		if v, err := convertTemporal(row.a, row.form); err == nil {
			t.Errorf("convertTemporal(xs:%s, %q) = %s, want an error", datatypeNames[row.a], row.form, spellValue(v))
		}
	}
}

// TestWithLoggerYearMonthDuration pins the conversion record of a
// yearMonthDuration, read from its lexical: rule "dt-lexical-mapping", input
// the normalized value, here "P0M", which has no canonical form.
func TestWithLoggerYearMonthDuration(t *testing.T) {
	schema := schemaOf(t, sequenceOf(`<xs:element name="a" type="xs:yearMonthDuration"/>`))
	data := instanceOf("", "<s:a> P0M </s:a>")
	var buf bytes.Buffer
	if _, err := Decode[struct{ A tYearMonthDuration }](schema, strict.New(), data, WithLogger(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))); err != nil {
		t.Fatal(err)
	}
	var got []traceRecord
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var tr traceRecord
		if err := json.Unmarshal(line, &tr); err != nil {
			t.Fatalf("record %s: %v", line, err)
		}
		if tr.Codec != nil {
			got = append(got, tr)
		}
	}
	const ymd = "{http://www.w3.org/2001/XMLSchema}yearMonthDuration"
	loc := fmt.Sprintf("?:1:%d", bytes.Index(data, []byte("<s:a>"))+len("<s:a>")+1)
	want := []traceRecord{
		{"whiteSpace", map[string]string{"rule": "rf-whiteSpace", "type": ymd, "input": " P0M ", "loc": loc}},
		{"lexical mapping", map[string]string{"rule": "cvc-datatype-valid", "clause": "2.1", "type": ymd, "input": "P0M", "loc": loc}},
		{"conversion", map[string]string{"rule": "dt-lexical-mapping", "type": ymd, "input": "P0M", "loc": loc}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("codec records = %+v\nwant %+v", got, want)
	}
}
