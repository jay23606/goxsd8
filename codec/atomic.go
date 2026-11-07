package codec

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"

	"github.com/kud360/goxsd8/internal/xmlqname"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// atomic is the atomic Go type codegen gives an atomic simple type Decode
// decodes (codegen/doc.go "# Simple types"), and how its value is read:
// string, bool, float32, float64, big.Rat, big.Int, a bounded Go integer,
// []byte read from xs:hexBinary's canonical form or from xs:base64Binary's,
// the datatype struct of a date, time or duration datatype, QName's
// datatype struct, which NOTATION shares, or PrecisionDecimal's, read from
// the lexical into codec's decimalValue. The zero value is invalid (STYLE
// T1).
type atomic uint8

const (
	atomicString atomic = iota + 1
	atomicBool
	atomicFloat32
	atomicFloat64
	atomicBigRat
	atomicBigInt
	atomicInt8
	atomicInt16
	atomicInt32
	atomicInt64
	atomicUint8
	atomicUint16
	atomicUint32
	atomicUint64
	atomicDateTime
	atomicDateTimeStamp
	atomicTime
	atomicDate
	atomicGYearMonth
	atomicGYear
	atomicGMonthDay
	atomicGDay
	atomicGMonth
	atomicDuration
	atomicDayTimeDuration
	atomicYearMonthDuration
	atomicHexBinary
	atomicBase64Binary
	atomicQName
	atomicPrecisionDecimal
)

// The mirrors of the thirteen datatype structs codegen emits other than
// PrecisionDecimal (codegen/doc.go "# Simple types"), the twelve date, time
// and duration ones and QName's: each holds codegen's exported fields
// exactly, in name, order and Go type, so that the struct codegen emits in
// the caller's package, which codec cannot name, converts to it and back
// (mirrored). TestDatatypeMirrorsAreCodegens pins each to codegen's.
// PrecisionDecimal has none: its one field is of the caller's own sealed
// interface, so no struct of codec's converts to it, and codec mirrors its
// numerical arm alone (decimalNumerical).
type (
	dateTimeMirror struct {
		Year           big.Int
		Month          uint8
		Day            uint8
		Hour           uint8
		Minute         uint8
		Second         big.Rat
		TimezoneOffset *int16
	}
	dateTimeStampMirror struct {
		Year           big.Int
		Month          uint8
		Day            uint8
		Hour           uint8
		Minute         uint8
		Second         big.Rat
		TimezoneOffset int16
	}
	timeMirror struct {
		Hour           uint8
		Minute         uint8
		Second         big.Rat
		TimezoneOffset *int16
	}
	dateMirror struct {
		Year           big.Int
		Month          uint8
		Day            uint8
		TimezoneOffset *int16
	}
	gYearMonthMirror struct {
		Year           big.Int
		Month          uint8
		TimezoneOffset *int16
	}
	gYearMirror struct {
		Year           big.Int
		TimezoneOffset *int16
	}
	gMonthDayMirror struct {
		Month          uint8
		Day            uint8
		TimezoneOffset *int16
	}
	gDayMirror struct {
		Day            uint8
		TimezoneOffset *int16
	}
	gMonthMirror struct {
		Month          uint8
		TimezoneOffset *int16
	}
	durationMirror struct {
		Months  big.Int
		Seconds big.Rat
	}
	dayTimeDurationMirror struct {
		Seconds big.Rat
	}
	yearMonthDurationMirror struct {
		Months big.Int
	}
	qNameMirror struct {
		Space string
		Local string
	}
)

// atomicTypes holds, indexed by atomic, each one's Go type: for a datatype
// struct, codec's mirror of it, and for PrecisionDecimal's, which has none,
// codec's decimalValue.
var atomicTypes = [...]reflect.Type{
	atomicString:            reflect.TypeFor[string](),
	atomicBool:              reflect.TypeFor[bool](),
	atomicFloat32:           reflect.TypeFor[float32](),
	atomicFloat64:           reflect.TypeFor[float64](),
	atomicBigRat:            reflect.TypeFor[big.Rat](),
	atomicBigInt:            reflect.TypeFor[big.Int](),
	atomicInt8:              reflect.TypeFor[int8](),
	atomicInt16:             reflect.TypeFor[int16](),
	atomicInt32:             reflect.TypeFor[int32](),
	atomicInt64:             reflect.TypeFor[int64](),
	atomicUint8:             reflect.TypeFor[uint8](),
	atomicUint16:            reflect.TypeFor[uint16](),
	atomicUint32:            reflect.TypeFor[uint32](),
	atomicUint64:            reflect.TypeFor[uint64](),
	atomicDateTime:          reflect.TypeFor[dateTimeMirror](),
	atomicDateTimeStamp:     reflect.TypeFor[dateTimeStampMirror](),
	atomicTime:              reflect.TypeFor[timeMirror](),
	atomicDate:              reflect.TypeFor[dateMirror](),
	atomicGYearMonth:        reflect.TypeFor[gYearMonthMirror](),
	atomicGYear:             reflect.TypeFor[gYearMirror](),
	atomicGMonthDay:         reflect.TypeFor[gMonthDayMirror](),
	atomicGDay:              reflect.TypeFor[gDayMirror](),
	atomicGMonth:            reflect.TypeFor[gMonthMirror](),
	atomicDuration:          reflect.TypeFor[durationMirror](),
	atomicDayTimeDuration:   reflect.TypeFor[dayTimeDurationMirror](),
	atomicYearMonthDuration: reflect.TypeFor[yearMonthDurationMirror](),
	atomicHexBinary:         reflect.TypeFor[[]byte](),
	atomicBase64Binary:      reflect.TypeFor[[]byte](),
	atomicQName:             reflect.TypeFor[qNameMirror](),
	atomicPrecisionDecimal:  decimalValueType,
}

// datatypeNames holds, indexed by an atomic whose Go type is a datatype
// struct (datatypeStruct), the local name of the XSD datatype whose datatype
// struct it is, the one encoding of that pairing (datatypeAtomics indexes it
// by name); codegen names the struct from it (codegen/doc.go, the datatype
// struct names): dateTime's is DateTime and QName's QName. xs:NOTATION,
// which shares QName's struct, is no entry: primitiveAtomics keys it to
// atomicQName.
var datatypeNames = [...]string{
	atomicDateTime:          "dateTime",
	atomicDateTimeStamp:     "dateTimeStamp",
	atomicTime:              "time",
	atomicDate:              "date",
	atomicGYearMonth:        "gYearMonth",
	atomicGYear:             "gYear",
	atomicGMonthDay:         "gMonthDay",
	atomicGDay:              "gDay",
	atomicGMonth:            "gMonth",
	atomicDuration:          "duration",
	atomicDayTimeDuration:   "dayTimeDuration",
	atomicYearMonthDuration: "yearMonthDuration",
	atomicQName:             "QName",
	atomicPrecisionDecimal:  "precisionDecimal",
}

// bare returns a's Go type itself, unnamed, or for a datatype struct codec's
// mirror of it, or decimalValue for PrecisionDecimal's, which converts to no
// caller type.
func (a atomic) bare() reflect.Type {
	if a == 0 || int(a) >= len(atomicTypes) {
		// The zero atomic is an internal bug, never input.
		panic(fmt.Sprintf("codec: invalid atomic %d", a))
	}
	return atomicTypes[a]
}

// mirrored reports whether the Go type codegen gives a is a datatype struct
// that codec holds a mirror of (bare), a struct of the identical underlying
// type, since codegen declares the datatype struct in the caller's package.
// It is read from the mirror's package and kind: big.Rat and big.Int are
// structs too, of math/big, and decimalValue, PrecisionDecimal's bare type,
// is codec's but no struct.
func (a atomic) mirrored() bool {
	return a.bare().PkgPath() == reflect.TypeFor[atomic]().PkgPath() && a.bare().Kind() == reflect.Struct
}

// datatypeStruct reports whether the Go type codegen gives a is a datatype
// struct, mirrored or PrecisionDecimal's: whether datatypeNames names its
// datatype.
func (a atomic) datatypeStruct() bool {
	return int(a) < len(datatypeNames) && datatypeNames[a] != ""
}

// shapes reports whether v is a struct of the shape of the datatype struct
// codegen gives a, a datatypeStruct atomic, its name never compared: for a
// mirrored a, one that converts to the mirror, so of its exported fields
// exactly, in name, order and Go type, struct tags aside; for
// PrecisionDecimal's, one decimalShape holds.
func (a atomic) shapes(v reflect.Type) bool {
	if a == atomicPrecisionDecimal {
		return decimalShape(v)
	}
	return v.ConvertibleTo(a.bare())
}

// reading is what an atomic's value is read from once its governing
// mapping's Parse has accepted the normalized lexical. The zero value is
// invalid (STYLE T1).
type reading uint8

const (
	// readCanonical is the form the governing mapping's Canonical renders the
	// value in (Datatypes §2.3.1 dt-canonical-mapping).
	readCanonical reading = iota + 1
	// readLexical is the normalized lexical Parse accepted, never Canonical:
	// yearMonthDuration's, whose zero value has no canonical representation
	// (§3.4.26.1, §2.3.1 dt-canonical-mapping's "where possible"), and
	// precisionDecimal's, whose canonical mapping keeps neither the ·scale·
	// of zero nor its ·sign· (precisionDecimal §6 f-precDecCanmap), read by
	// the lexical mapping (f-precDecLexmap).
	readLexical
	// readExpanded is the expanded name the normalized lexical resolves to
	// against the bindings Parse was given, never Canonical: QName's, which
	// NOTATION shares, neither having a canonical representation (§3.3.18,
	// §3.3.19).
	readExpanded
)

// reading returns what a's value is read from.
func (a atomic) reading() reading {
	if a == atomicYearMonthDuration || a == atomicPrecisionDecimal {
		return readLexical
	}
	if a == atomicQName {
		return readExpanded
	}
	return readCanonical
}

// spell spells a's Go type for an error message: its own name, []byte for
// either binary arm (reflect spells it []uint8), or for a datatype struct "D
// for the datatype struct D of" the datatype's QName and the struct's fields,
// never the mirror's name, PrecisionDecimal's Value field by its role, its
// type being the caller's interface.
func (a atomic) spell() string {
	if a == atomicHexBinary || a == atomicBase64Binary {
		return "[]byte"
	}
	if a == atomicPrecisionDecimal {
		return "D for the datatype struct D of xs:precisionDecimal (Value I, I a sealed interface whose arms WithPrecisionDecimalArms supplies)"
	}
	if !a.mirrored() {
		return a.bare().String()
	}
	t := a.bare()
	fields := make([]string, t.NumField())
	for i := range fields {
		fields[i] = t.Field(i).Name + " " + t.Field(i).Type.String()
	}
	return fmt.Sprintf("D for the datatype struct D of xs:%s (%s)", datatypeNames[a], strings.Join(fields, ", "))
}

// convert returns the value of a's bare type that form denotes, read as
// Decode's doc states, or the error for a form a's Go type does not read:
// form is a canonical lexical form the governing mapping's Canonical
// rendered, or where a's reading is readLexical the normalized lexical Parse
// accepted. A boolean is never read by strconv.ParseBool, which accepts more
// than f-booleanCanmap's two forms. PrecisionDecimal's is convertDecimal's,
// and every atomic convert does not name is convertTemporal's, which converts
// the date, time and duration ones and panics for the rest, atomicString and
// atomicQName, read by expandedName, having no conversion.
func (a atomic) convert(form string) (reflect.Value, error) {
	switch a {
	case atomicPrecisionDecimal:
		return convertDecimal(form)
	case atomicBool:
		switch form {
		case "true":
			return reflect.ValueOf(true), nil
		case "false":
			return reflect.ValueOf(false), nil
		}
		return reflect.Value{}, errors.New("a boolean's canonical form is true or false (f-booleanCanmap)")
	case atomicFloat32, atomicFloat64:
		f, err := strconv.ParseFloat(form, a.bare().Bits())
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(f).Convert(a.bare()), nil
	case atomicBigRat:
		r, ok := new(big.Rat).SetString(form)
		if !ok {
			return reflect.Value{}, errors.New("big.Rat does not read it")
		}
		return reflect.ValueOf(r).Elem(), nil
	case atomicBigInt:
		i, ok := new(big.Int).SetString(form, 10)
		if !ok {
			return reflect.Value{}, errors.New("big.Int does not read it in base 10")
		}
		return reflect.ValueOf(i).Elem(), nil
	case atomicInt8, atomicInt16, atomicInt32, atomicInt64:
		n, err := strconv.ParseInt(form, 10, a.bare().Bits())
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(n).Convert(a.bare()), nil
	case atomicUint8, atomicUint16, atomicUint32, atomicUint64:
		n, err := strconv.ParseUint(form, 10, a.bare().Bits())
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(n).Convert(a.bare()), nil
	case atomicHexBinary:
		// form is f-hexBinaryCanonical's (Datatypes §E.4.1); its letter case
		// is not compared, hex.DecodeString reading either.
		b, err := hex.DecodeString(form)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(b), nil
	case atomicBase64Binary:
		// form is Canonical-base64Binary's (Datatypes §3.3.16.2).
		b, err := base64.StdEncoding.DecodeString(form)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(b), nil
	default:
		return convertTemporal(a, form)
	}
}

// expandedName returns the value of QName's datatype struct, codec's mirror
// of it, that lexical denotes: the expanded name it resolves to against ctx,
// the bindings the governing mapping's Parse accepted it under (Datatypes
// §3.3.18, §3.3.19), Space its namespace name, "" for none, and Local its
// local part, or the error where it resolves to none.
func expandedName(lexical string, ctx value.Context) (reflect.Value, error) {
	space, local, ok := xmlqname.Resolve(lexical, ctx.LookupNamespace)
	if !ok {
		return reflect.Value{}, errors.New("it resolves to no expanded name against the bindings in scope")
	}
	return reflect.ValueOf(qNameMirror{Space: space, Local: local}), nil
}

// chainAtomics maps each integer builtin that decides an atomic type's
// atomic from anywhere on its {base type definition} chain to that atomic:
// the eight bounded integer builtins (Datatypes §3.4.16–§3.4.19,
// §3.4.21–§3.4.24) to the Go integer whose range is their value space, and
// xs:integer (§3.4.13) to big.Int. It is a lookup index only and never
// iterated (STYLE D2).
var chainAtomics = map[xsd.QName]atomic{
	{Space: xsd.XMLSchemaNS, Local: "long"}:          atomicInt64,
	{Space: xsd.XMLSchemaNS, Local: "int"}:           atomicInt32,
	{Space: xsd.XMLSchemaNS, Local: "short"}:         atomicInt16,
	{Space: xsd.XMLSchemaNS, Local: "byte"}:          atomicInt8,
	{Space: xsd.XMLSchemaNS, Local: "unsignedLong"}:  atomicUint64,
	{Space: xsd.XMLSchemaNS, Local: "unsignedInt"}:   atomicUint32,
	{Space: xsd.XMLSchemaNS, Local: "unsignedShort"}: atomicUint16,
	{Space: xsd.XMLSchemaNS, Local: "unsignedByte"}:  atomicUint8,
	{Space: xsd.XMLSchemaNS, Local: "integer"}:       atomicBigInt,
}

// datatypeAtomics maps the QName of each datatype of datatypeNames to its
// datatype struct's atomic, built from datatypeNames once; it is a lookup
// index only and never iterated (STYLE D2).
var datatypeAtomics = func() map[xsd.QName]atomic {
	m := map[xsd.QName]atomic{}
	for a, local := range datatypeNames {
		if local != "" {
			m[xsd.QName{Space: xsd.XMLSchemaNS, Local: local}] = atomic(a)
		}
	}
	return m
}()

// primitiveAtomics maps each {primitive type definition} Decode decodes that
// datatypeAtomics does not hold to the atomic it gives a type no builtin
// chainAtomic meets decides (codegen/doc.go "# Simple types"): xs:NOTATION
// among them, to QName's datatype struct, which it shares. It is a lookup
// index only and never iterated (STYLE D2).
var primitiveAtomics = map[xsd.QName]atomic{
	{Space: xsd.XMLSchemaNS, Local: "string"}:       atomicString,
	{Space: xsd.XMLSchemaNS, Local: "anyURI"}:       atomicString,
	{Space: xsd.XMLSchemaNS, Local: "boolean"}:      atomicBool,
	{Space: xsd.XMLSchemaNS, Local: "float"}:        atomicFloat32,
	{Space: xsd.XMLSchemaNS, Local: "double"}:       atomicFloat64,
	{Space: xsd.XMLSchemaNS, Local: "decimal"}:      atomicBigRat,
	{Space: xsd.XMLSchemaNS, Local: "hexBinary"}:    atomicHexBinary,
	{Space: xsd.XMLSchemaNS, Local: "base64Binary"}: atomicBase64Binary,
	{Space: xsd.XMLSchemaNS, Local: "NOTATION"}:     atomicQName,
}

// chainAtomic returns the atomic of st as codegen/doc.go "# Simple types"
// picks it from st's {base type definition} chain: walking it from st itself,
// the first builtin met that chainAtomics or datatypeAtomics holds gives its
// atomic, and false where the walk meets none. Every bounded builtin lies
// below xs:integer, and dateTimeStamp, dayTimeDuration and yearMonthDuration
// below the primitive they restrict, so the nearest wins over it; a primitive
// of datatypeAtomics is met only at the top of its chain, and no chain meets
// both an integer builtin and a temporal one.
func chainAtomic(r xsd.TypeResolver, st *xsd.SimpleType) (atomic, bool, error) {
	for step := st; step != nil; {
		if a, ok := chainAtomics[step.Name()]; ok {
			return a, true, nil
		}
		if a, ok := datatypeAtomics[step.Name()]; ok {
			return a, true, nil
		}
		base, err := step.Base(r)
		if err != nil {
			return 0, false, err
		}
		step = base
	}
	return 0, false, nil
}
