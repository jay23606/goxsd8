package codec

import (
	"log/slog"
	"reflect"
	"slices"
)

// Option configures [Decode] and [CheckDecode]. The zero set of options is a complete,
// usable configuration: options only replace defaults.
type Option func(*config)

// config is the resolved [Decode] configuration.
type config struct {
	uri      string
	branches branchSets
	decimals decimalArmSets
	log      *slog.Logger
}

// newConfig applies opts over the defaults: the instance is unnamed, so
// every instance Loc Decode's errors carry renders "?" for its URI, no
// interface has a branch set or an arm set, and nothing is logged
// ([slog.DiscardHandler]).
func newConfig(opts []Option) config {
	cfg := config{log: slog.New(slog.DiscardHandler)}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// WithLogger sets the logger [Decode] traces on at debug level. A nil l
// selects the silent default ([slog.DiscardHandler]), and a later WithLogger
// replaces an earlier one. Decode hands l to package validate unchanged
// (validate.WithLogger), and the records validate logs on it are that
// package's, none of those below.
//
// Decode logs one Debug record per stage it runs on a value: whiteSpace, then,
// for an atomic Go type other than string, the lexical mapping and the
// conversion, which on a list value run on each list item. A value of a
// string Go type therefore logs its whiteSpace record alone. A union value
// logs its ·active basic member·'s records, as a value of that member logs
// them, and a list item of a union its member's lexical mapping and
// conversion; identifying the member logs no record of its own. Each record's
// attributes sit in the "codec" group, its message outside it:
//
//   - "whiteSpace": rule "rf-whiteSpace" (Datatypes §4.3.6), input the
//     characters Decode reads the value from, before whiteSpace.
//   - "lexical mapping": rule "cvc-datatype-valid" and clause "2.1" (§4.1.4),
//     input the normalized value, or for a list the list item, as handed to
//     the governing mapping's Parse, logged before Parse runs, so none where
//     no mapping of the backend governs the type.
//   - "conversion": rule "dt-canonical-mapping" (§2.3.1), input the canonical
//     form the governing mapping's Canonical rendered, logged once it has;
//     for yearMonthDuration's datatype struct, QName's and PrecisionDecimal's,
//     which Decode's doc states are converted from the string Parse accepted,
//     rule "dt-lexical-mapping" (§2.3), input that string, logged once Parse
//     has accepted it.
//
// Every record also carries type, the type as item 8 of Decode's doc names it,
// and loc, the instance Loc of the value's source item 8 names, the whole
// value's for a list item.
func WithLogger(l *slog.Logger) Option {
	if l == nil {
		l = slog.New(slog.DiscardHandler)
	}
	return func(c *config) { c.log = l }
}

// WithURI names the instance document data holds, so every instance
// [xsderr.Loc] an error of [Decode] carries identifies the file a reader has
// to open. [Decode] takes a []byte, which carries no name of its own.
func WithURI(uri string) Option {
	return func(c *config) { c.uri = uri }
}

// WithBranches supplies the branch types of I, a sealed interface codegen
// emitted, which [Decode] cannot find from T: a Go interface names none of its
// implementers. Pass one value of each branch or variant, in any order, such
// as codec.WithBranches[order.Pay](order.PayCard{}, order.PayCash{}); only
// each value's dynamic type is read. Decode uses the set wherever T holds I as
// a choice's, a substitution group's or a union's interface, at any depth: T
// itself, a field, a list's element, a nested element's struct or interface,
// a branch, or a variant, and ignores a set for any type T does not hold so,
// so one option list may serve every document element of a package; a set for
// a type that is no interface is never used. A set for I serves every
// interface type of I's method set, such as the named type codegen emits over
// I for a top-level element of I's union type, and a later WithBranches for
// an interface of I's method set replaces an earlier one's set. The Value
// interface of codegen's PrecisionDecimal takes its arms from
// [WithPrecisionDecimalArms] alone. With no WithBranches for I, I's set is
// empty. Decode's doc, item 6, states how a set is matched and when it is
// [ErrTypeMismatch].
func WithBranches[I any](branches ...I) Option {
	set := branchSet{iface: reflect.TypeFor[I](), branches: make([]reflect.Type, len(branches))}
	for i, b := range branches {
		set.branches[i] = reflect.TypeOf(b)
	}
	return func(c *config) { c.branches = c.branches.with(set) }
}

// branchSet is the branch types one WithBranches supplies for the interface
// iface, in the order given, each nil for a nil value.
type branchSet struct {
	iface    reflect.Type
	branches []reflect.Type
}

// branchSets holds one branchSet per method set, in the order a WithBranches
// first named each; it is searched by sameMethods and never ranged over into
// output (STYLE D2).
type branchSets []branchSet

// with returns s with set in place of the entry whose interface has its
// interface's method set, or appended when s has none; it may reuse s's
// array.
func (s branchSets) with(set branchSet) branchSets {
	if i := slices.IndexFunc(s, func(e branchSet) bool { return sameMethods(e.iface, set.iface) }); i >= 0 {
		s[i] = set
		return s
	}
	return append(s, set)
}

// of returns the branch types s holds for the interface it, those of the
// entry whose interface has its method set, and whether any WithBranches
// supplied them.
func (s branchSets) of(it reflect.Type) ([]reflect.Type, bool) {
	i := slices.IndexFunc(s, func(e branchSet) bool { return sameMethods(e.iface, it) })
	if i < 0 {
		return nil, false
	}
	return s[i].branches, true
}

// sameMethods reports whether a and b are one type or two interface types of
// one method set, each implementing the other, so that a set WithBranches
// supplies for one serves the other: codegen's named type over a union's
// interface, such as type Pin PinUnion, is such an interface. An unexported
// method is its package's, so two packages' interfaces never share one.
func sameMethods(a, b reflect.Type) bool {
	if a == b {
		return true
	}
	return a.Kind() == reflect.Interface && b.Kind() == reflect.Interface && a.Implements(b) && b.Implements(a)
}

// WithPrecisionDecimalArms supplies the four arms of I, the sealed interface
// codegen emits as the type of the one field Value of its PrecisionDecimal
// datatype struct (codegen's package doc, "# Simple types"), which [Decode]
// cannot find from T: a Go interface names none of its implementers. Pass
// one value of each arm in its role, numerical the arm holding a finite
// value, positiveInfinity, negativeInfinity and notANumber the arms of the
// special values (precisionDecimal §3.1 vp-pd-numVal), such as
// codec.WithPrecisionDecimalArms[m.PrecisionDecimalValue](m.PrecisionDecimalValueNumerical{},
// m.PrecisionDecimalValuePositiveInfinity{},
// m.PrecisionDecimalValueNegativeInfinity{},
// m.PrecisionDecimalValueNotANumber{}); only each value's dynamic type is
// read. A later WithPrecisionDecimalArms for the same I replaces an earlier
// one's arms. Decode reads them only where T holds a PrecisionDecimal whose
// Value is of type I, at any depth, and ignores them elsewhere, a choice's or
// a substitution group's interface included; Decode's doc, item 6, states
// when they are [ErrTypeMismatch]. Decode matches each arm by its shape
// alone, so it cannot tell the three field-less arms apart: passed in one
// another's roles, they decode the special values into the wrong arm, the
// one misuse Decode cannot report.
func WithPrecisionDecimalArms[I any](numerical, positiveInfinity, negativeInfinity, notANumber I) Option {
	set := decimalArms{
		iface:            reflect.TypeFor[I](),
		numerical:        reflect.TypeOf(numerical),
		positiveInfinity: reflect.TypeOf(positiveInfinity),
		negativeInfinity: reflect.TypeOf(negativeInfinity),
		notANumber:       reflect.TypeOf(notANumber),
	}
	return func(c *config) { c.decimals = c.decimals.with(set) }
}

// decimalArms is the arm types one WithPrecisionDecimalArms supplies for the
// interface iface, one per role, each nil for a nil value.
type decimalArms struct {
	iface, numerical, positiveInfinity, negativeInfinity, notANumber reflect.Type
}

// decimalArmSets holds one decimalArms per interface, in the order a
// WithPrecisionDecimalArms first named each; it is searched by identity and
// never ranged over into output (STYLE D2).
type decimalArmSets []decimalArms

// with returns s with set in place of the entry for its interface, or
// appended when s has none; it may reuse s's array.
func (s decimalArmSets) with(set decimalArms) decimalArmSets {
	if i := slices.IndexFunc(s, func(e decimalArms) bool { return e.iface == set.iface }); i >= 0 {
		s[i] = set
		return s
	}
	return append(s, set)
}

// of returns the arm types s holds for the interface it, and whether any
// WithPrecisionDecimalArms supplied them.
func (s decimalArmSets) of(it reflect.Type) (decimalArms, bool) {
	i := slices.IndexFunc(s, func(e decimalArms) bool { return e.iface == it })
	if i < 0 {
		return decimalArms{}, false
	}
	return s[i], true
}
