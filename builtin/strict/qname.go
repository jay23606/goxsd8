package strict

import (
	"regexp"
	"strings"

	"github.com/kud360/goxsd8/builtin"
	"github.com/kud360/goxsd8/regex"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsderr"
)

// ncNameRE matches the NCName production ([XML Namespaces] NT-NCName): a
// NameStartChar-minus-colon followed by NameChar-minus-colon, whole-string
// anchored. It is compiled once, with the XSD flavor via regex.Translate, from
// xs:NCName's pattern "[\i-[:]][\c-[:]]*" as the generated builtin row carries
// it (builtin.NCNamePattern), and the NameStartChar/NameChar code-point sets
// are the ones the regex package already owns — neither is a second table here
// (PRINCIPLES 26/27). Those sets are hand-typed rather than generated (#989).
// FlavorXSD output is whole-string anchored (\A(?:…)\z), so a match means the
// entire string is an NCName — in particular an empty string and any string
// containing ':' fail.
var ncNameRE = func() *regexp.Regexp {
	goRE, err := regex.Translate(builtin.NCNamePattern(), regex.FlavorXSD, "")
	if err != nil {
		panic("strict: translating the NCName pattern: " + err.Error())
	}
	return regexp.MustCompile(goRE)
}()

// qnameVal is an xs:QName value (§3.3.18.1): the tuple {namespace name, local
// part}, a resolved expanded name — space is the namespace name (an anyURI,
// empty when the governing binding maps the prefix to no namespace) and local
// is the NCName local part. It is value.Eq and value.Lengthed, and deliberately
// NOT value.Ordered (ordered=false, §3.3.18 fundamental facets) nor
// value.Canonical (the spec defines no canonical representation for QName, since
// the available lexical forms vary with context — §3.3.18). It is a DISTINCT Go
// type from notationVal even though the two share this shape, so value.Eq never
// cross-matches a QName against a NOTATION (mirrors hexBinaryVal/base64BinaryVal).
type qnameVal struct{ space, local string }

// notationVal is an xs:NOTATION value (§3.3.19.1): the same {namespace name,
// local part} tuple as qnameVal, since NOTATION's lexical mapping rules are "as
// given for QName" (§3.3.19). Same capability set and rationale as qnameVal —
// value.Eq and value.Lengthed, NOT value.Ordered (ordered=false, §3.3.19) nor
// value.Canonical (no canonical representation is defined, §3.3.19). Kept a
// distinct Go type from qnameVal so value.Eq never treats a NOTATION as equal to
// a same-tuple QName.
type notationVal struct{ space, local string }

// parseQName maps a QName lexical to its value (§3.3.18.2). The mapping is
// context-dependent: it resolves the prefix (or, for an unprefixed name, the
// empty prefix = default namespace) against ctx's in-scope namespace bindings.
func parseQName(lexical string, ctx value.Context) (value.Value, error) {
	space, local, err := resolveQNameLexical(lexical, ctx, "QName")
	if err != nil {
		return nil, err
	}
	return qnameVal{space: space, local: local}, nil
}

// parseNOTATION maps a NOTATION lexical to its value using the identical
// grammar+resolution as QName (§3.3.19: "the lexical mapping rules for NOTATION
// are as given for QName"). It accepts a QName whether or not it names a
// notation declared in the current schema, which NOTATION's lexical and value
// spaces require (§3.3.19): a leaf mapping holds no schema, so that check is
// value.ValidateLexical's, over a lexical this mapping accepted, wherever its
// resolver holds the schema.
func parseNOTATION(lexical string, ctx value.Context) (value.Value, error) {
	space, local, err := resolveQNameLexical(lexical, ctx, "NOTATION")
	if err != nil {
		return nil, err
	}
	return notationVal{space: space, local: local}, nil
}

// resolveQNameLexical is the shared QName grammar+resolution used by both QName
// and NOTATION (§3.3.18.2). It splits the lexical per the [Namespaces in XML]
// QName production, validates each part as an NCName, then resolves the prefix
// to a namespace name via ctx. typ names the type for error messages. Every
// rejection — malformed grammar, or an unresolvable prefix — is an *xsderr.Error
// with rule cvc-datatype-valid (§4.1.4), never a fabricated value with an
// empty/wrong namespace.
func resolveQNameLexical(lexical string, ctx value.Context, typ string) (space, local string, err error) {
	prefix, local, err := splitQName(lexical, typ)
	if err != nil {
		return "", "", err
	}
	// A nil context binds nothing, not even the default namespace for an
	// unprefixed name ([value.Context]); reject cleanly rather than
	// dereferencing nil.
	if ctx == nil {
		return "", "", xsderr.New(ruleDatatypeValid, xsderr.Loc{},
			"%q has no namespace bindings in scope to resolve it against, so it maps to no %s value, but cvc-datatype-valid clause 2.1 requires it to map to one", lexical, typ)
	}
	// Resolve the prefix (empty prefix = default namespace, §3.3.18): the
	// context models "no namespace in scope" as an ok binding to the empty
	// string, so ok==false is a genuinely unbound prefix and a lexical-space
	// rejection, never a value fabricated with an empty namespace.
	space, ok := ctx.LookupNamespace(prefix)
	if !ok {
		return "", "", xsderr.New(ruleDatatypeValid, xsderr.Loc{},
			"%q has the prefix %q, which no namespace binding in scope declares, so it maps to no %s value, but cvc-datatype-valid clause 2.1 requires it to map to one", lexical, prefix, typ)
	}
	return space, local, nil
}

// splitQName splits a QName lexical into its prefix and local part per the
// [Namespaces in XML] QName production: PrefixedName ('prefix:local') or
// UnprefixedName ('local'). An unprefixed name yields an empty prefix. Both
// parts (and the sole part of an unprefixed name) must be NCNames, so a missing
// part, a leading/trailing colon, more than one colon, or an invalid NCName
// character is outside the lexical space — an *xsderr.Error (cvc-datatype-valid).
func splitQName(lexical, typ string) (prefix, local string, err error) {
	idx := strings.IndexByte(lexical, ':')
	if idx < 0 {
		if !ncNameRE.MatchString(lexical) {
			return "", "", xsderr.New(ruleDatatypeValid, xsderr.Loc{},
				"%q is not an NCName, so it is not in the lexical space of %s, which cvc-datatype-valid clause 2.1 requires it to be in", lexical, typ)
		}
		return "", lexical, nil
	}
	prefix, local = lexical[:idx], lexical[idx+1:]
	if !ncNameRE.MatchString(prefix) || !ncNameRE.MatchString(local) {
		return "", "", xsderr.New(ruleDatatypeValid, xsderr.Loc{},
			"%q does not split at its colon into a prefix and a local part that are each an NCName, so it is not in the lexical space of %s, which cvc-datatype-valid clause 2.1 requires it to be in", lexical, typ)
	}
	return prefix, local, nil
}

// Eq is value-space tuple equality (§3.3.18.2): two QName values are equal iff
// their namespace names and local parts are equal. A non-qnameVal argument —
// including a notationVal with the identical tuple — is unequal.
func (q qnameVal) Eq(other value.Value) bool {
	o, ok := other.(qnameVal)
	if !ok {
		return false
	}
	return q == o
}

// Eq is value-space tuple equality for NOTATION (§3.3.19.1), keyed on its own
// type so a NOTATION never compares equal to a same-tuple QName.
func (n notationVal) Eq(other value.Value) bool {
	o, ok := other.(notationVal)
	if !ok {
		return false
	}
	return n == o
}

// Len satisfies value.Lengthed so the length/minLength/maxLength facets — which
// ARE applicable to QName (cos-applicable-facets §4.1.5), though deprecated —
// have a measurement to call. Per §4.3.1.3/§4.3.2.3/§4.3.3.3 clause 1.3, ANY
// QName value is facet-valid for these facets regardless of the value, so this
// count never gates validity; it reports the rune count of the local NCName, the
// value's only intrinsic character sequence (the namespace name is a
// context-resolved URI, not part of the name).
func (q qnameVal) Len() int { return len([]rune(q.local)) }

// Len is the NOTATION counterpart of qnameVal.Len; §4.3.1.3 clause 1.3 makes it
// equally non-gating (any value is length-facet-valid).
func (n notationVal) Len() int { return len([]rune(n.local)) }
