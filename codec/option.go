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
	log      *slog.Logger
}

// newConfig applies opts over the defaults: the instance is unnamed, so
// every instance Loc Decode's errors carry renders "?" for its URI, no
// interface has a branch set, and nothing is logged
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
// string Go type therefore logs its whiteSpace record alone. Each record's
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
//     for yearMonthDuration's datatype struct, which Decode's doc states is
//     converted from the string Parse accepted, rule "dt-lexical-mapping"
//     (§2.3), input that string, logged once Parse has accepted it.
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
// implementers. Pass one value of each branch, in any order, such as
// codec.WithBranches[order.Pay](order.PayCard{}, order.PayCash{}); only each
// value's dynamic type is read. A later WithBranches for the same I replaces
// an earlier one's set. Decode uses the set wherever T holds I, at any depth:
// T itself, a field, a nested element's struct or interface, or a branch, and
// ignores a set for any type T does not hold, so one option list may serve
// every document element of a package; a set for a type that is no interface
// is never used. With no WithBranches for I, I's set is empty. Decode's doc,
// item 6, states when a set is [ErrTypeMismatch].
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

// branchSets holds one branchSet per interface, in the order a WithBranches
// first named each; it is searched by identity and never ranged over into
// output (STYLE D2).
type branchSets []branchSet

// with returns s with set in place of the entry for its interface, or
// appended when s has none; it may reuse s's array.
func (s branchSets) with(set branchSet) branchSets {
	if i := slices.IndexFunc(s, func(e branchSet) bool { return e.iface == set.iface }); i >= 0 {
		s[i] = set
		return s
	}
	return append(s, set)
}

// of returns the branch types s holds for the interface it, and whether any
// WithBranches supplied them.
func (s branchSets) of(it reflect.Type) ([]reflect.Type, bool) {
	i := slices.IndexFunc(s, func(e branchSet) bool { return e.iface == it })
	if i < 0 {
		return nil, false
	}
	return s[i].branches, true
}
