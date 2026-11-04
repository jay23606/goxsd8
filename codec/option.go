package codec

import (
	"reflect"
	"slices"
)

// Option configures [Decode]. The zero set of options is a complete,
// usable configuration: options only replace defaults.
type Option func(*config)

// config is the resolved [Decode] configuration.
type config struct {
	uri      string
	branches branchSets
}

// newConfig applies opts over the defaults: the instance is unnamed, so
// every instance Loc Decode's errors carry renders "?" for its URI, and no
// interface has a branch set.
func newConfig(opts []Option) config {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
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
// an earlier one's set. Decode uses the set wherever T holds I, and ignores a
// set for any type T does not hold, so one option list may serve every
// document element of a package; a set for a type that is no interface is
// never used. With no WithBranches for I, I's set is empty. Decode's doc,
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
