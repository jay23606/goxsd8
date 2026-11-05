package codec

import (
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strconv"

	"github.com/kud360/goxsd8/xsd"
)

// atomic is the atomic Go type codegen gives an atomic simple type Decode
// decodes (codegen/doc.go "# Simple types"): string, bool, float32, float64,
// big.Rat, big.Int, or a bounded Go integer. The zero value is invalid (STYLE
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
)

// atomicTypes holds, indexed by atomic, each one's Go type.
var atomicTypes = [...]reflect.Type{
	atomicString:  reflect.TypeFor[string](),
	atomicBool:    reflect.TypeFor[bool](),
	atomicFloat32: reflect.TypeFor[float32](),
	atomicFloat64: reflect.TypeFor[float64](),
	atomicBigRat:  reflect.TypeFor[big.Rat](),
	atomicBigInt:  reflect.TypeFor[big.Int](),
	atomicInt8:    reflect.TypeFor[int8](),
	atomicInt16:   reflect.TypeFor[int16](),
	atomicInt32:   reflect.TypeFor[int32](),
	atomicInt64:   reflect.TypeFor[int64](),
	atomicUint8:   reflect.TypeFor[uint8](),
	atomicUint16:  reflect.TypeFor[uint16](),
	atomicUint32:  reflect.TypeFor[uint32](),
	atomicUint64:  reflect.TypeFor[uint64](),
}

// bare returns a's Go type itself, unnamed.
func (a atomic) bare() reflect.Type {
	if a == 0 || int(a) >= len(atomicTypes) {
		// The zero atomic is an internal bug, never input.
		panic(fmt.Sprintf("codec: invalid atomic %d", a))
	}
	return atomicTypes[a]
}

// convert returns the value of a's bare type that canonical, a canonical
// lexical form the governing mapping's Canonical rendered, denotes, read as
// Decode's doc states, or the error for a form a's Go type does not read. A
// boolean is never read by strconv.ParseBool, which accepts more than
// f-booleanCanmap's two forms. atomicString has no conversion and panics.
func (a atomic) convert(canonical string) (reflect.Value, error) {
	v := reflect.New(a.bare()).Elem()
	switch a {
	case atomicBool:
		switch canonical {
		case "true":
			v.SetBool(true)
		case "false":
		default:
			return reflect.Value{}, errors.New("a boolean's canonical form is true or false (f-booleanCanmap)")
		}
	case atomicFloat32, atomicFloat64:
		f, err := strconv.ParseFloat(canonical, a.bare().Bits())
		if err != nil {
			return reflect.Value{}, err
		}
		v.SetFloat(f)
	case atomicBigRat:
		r, ok := new(big.Rat).SetString(canonical)
		if !ok {
			return reflect.Value{}, errors.New("big.Rat does not read it")
		}
		v.Set(reflect.ValueOf(r).Elem())
	case atomicBigInt:
		i, ok := new(big.Int).SetString(canonical, 10)
		if !ok {
			return reflect.Value{}, errors.New("big.Int does not read it in base 10")
		}
		v.Set(reflect.ValueOf(i).Elem())
	case atomicInt8, atomicInt16, atomicInt32, atomicInt64:
		n, err := strconv.ParseInt(canonical, 10, a.bare().Bits())
		if err != nil {
			return reflect.Value{}, err
		}
		v.SetInt(n)
	case atomicUint8, atomicUint16, atomicUint32, atomicUint64:
		n, err := strconv.ParseUint(canonical, 10, a.bare().Bits())
		if err != nil {
			return reflect.Value{}, err
		}
		v.SetUint(n)
	default:
		// atomicString's value is the normalized lexical itself.
		panic(fmt.Sprintf("codec: atomic %s has no conversion", a.bare()))
	}
	return v, nil
}

// The builtins whose names decide an atomic type's atomic, read by identity.
var (
	stringPrimitive  = xsd.QName{Space: xsd.XMLSchemaNS, Local: "string"}
	anyURIPrimitive  = xsd.QName{Space: xsd.XMLSchemaNS, Local: "anyURI"}
	booleanPrimitive = xsd.QName{Space: xsd.XMLSchemaNS, Local: "boolean"}
	floatPrimitive   = xsd.QName{Space: xsd.XMLSchemaNS, Local: "float"}
	doublePrimitive  = xsd.QName{Space: xsd.XMLSchemaNS, Local: "double"}
	decimalPrimitive = xsd.QName{Space: xsd.XMLSchemaNS, Local: "decimal"}
	integerBuiltin   = xsd.QName{Space: xsd.XMLSchemaNS, Local: "integer"}
)

// boundedIntegers maps each of the eight bounded integer builtins (Datatypes
// §3.4.16–§3.4.19, §3.4.21–§3.4.24) to the Go integer whose range is its
// value space; it is a lookup index only and never iterated (STYLE D2).
var boundedIntegers = map[xsd.QName]atomic{
	{Space: xsd.XMLSchemaNS, Local: "long"}:          atomicInt64,
	{Space: xsd.XMLSchemaNS, Local: "int"}:           atomicInt32,
	{Space: xsd.XMLSchemaNS, Local: "short"}:         atomicInt16,
	{Space: xsd.XMLSchemaNS, Local: "byte"}:          atomicInt8,
	{Space: xsd.XMLSchemaNS, Local: "unsignedLong"}:  atomicUint64,
	{Space: xsd.XMLSchemaNS, Local: "unsignedInt"}:   atomicUint32,
	{Space: xsd.XMLSchemaNS, Local: "unsignedShort"}: atomicUint16,
	{Space: xsd.XMLSchemaNS, Local: "unsignedByte"}:  atomicUint8,
}

// decimalAtomic returns the atomic of st, whose {primitive type definition} is
// xs:decimal, as codegen/doc.go "# Simple types" picks it: walking st's {base
// type definition} chain from st itself, the nearest bounded integer builtin
// gives its Go integer, failing one xs:integer gives big.Int, and a chain
// reaching neither gives big.Rat. Every bounded builtin lies below xs:integer,
// so the nearest wins over it.
func decimalAtomic(r xsd.TypeResolver, st *xsd.SimpleType) (atomic, error) {
	for step := st; step != nil; {
		if a, ok := boundedIntegers[step.Name()]; ok {
			return a, nil
		}
		if step.Name() == integerBuiltin {
			return atomicBigInt, nil
		}
		base, err := step.Base(r)
		if err != nil {
			return 0, err
		}
		step = base
	}
	return atomicBigRat, nil
}
