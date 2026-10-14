package xpath

import (
	"errors"
	"math/big"
	"strconv"
	"strings"

	"github.com/kud360/goxsd8/xsd"
)

// This file is the COMPILE-TIME type knowledge of the §3.12.6 subset: which
// builtin datatype a Literal carries, which type a comparison's two operands
// are converted into (xpath20.md §3.5.2's casting rules and B.1's type
// promotions), and whether B.2 admits the operator over that type at all.
// Nothing here runs at evaluation time — the compiled tree holds the resolved
// *[xsd.SimpleType] components and no resolver (ARCHITECTURE), so
// [CTATest.Evaluate] takes one again.

// ctaBuiltin is the ·expanded name· of a builtin datatype, which is the key an
// [xsd.TypeResolver] holds it under. Every call site names a literal builtin
// local (STYLE T4's parallel-table concern does not arise: there is no second
// table, only QName construction for an identity comparison the caller
// already knows the answer to) — this is not a general classifier for an
// arbitrary resolved name, and a schema is free to declare its OWN types with
// `targetNamespace` set to the XSD namespace (produce_typetable.go's
// ctaStaticTypes resolves against the fixed builtin set for exactly that
// reason, not against whatever a schema document declares).
func ctaBuiltin(local string) xsd.QName {
	return xsd.QName{Space: xsd.XMLSchemaNS, Local: local}
}

// ctaTypes is the type knowledge one [CompileCTATest] call reads: the resolver
// itself, plus the three builtin datatypes the compiler names outright — the
// two Literal kinds' types, and the xs:double §3.5.2 clause 2.1 answers with.
//
// It holds the resolver for the duration of the compile and puts it on no
// compiled node, which is ARCHITECTURE's rule for every reader above xsd that
// walks a {base type definition} chain.
type ctaTypes struct {
	resolver xsd.TypeResolver
	str      *xsd.SimpleType
	decimal  *xsd.SimpleType
	double   *xsd.SimpleType
}

// ctaResolveTypes reads the three builtin datatypes the compiler names, or
// reports false where the resolver answers for none of them — a schema whose
// {type definitions} were assembled without the builtins seeded. That is a
// whole-expression decline and not an error: no comparison in this grammar can
// be evaluated without xs:string, so the caller withholds on
// [CompileCTATest]'s own terms.
func ctaResolveTypes(r xsd.TypeResolver) (ctaTypes, bool) {
	t := ctaTypes{resolver: r}
	var resolved bool
	if t.str, resolved = t.simple(ctaBuiltin("string")); !resolved {
		return ctaTypes{}, false
	}
	if t.decimal, resolved = t.simple(ctaBuiltin("decimal")); !resolved {
		return ctaTypes{}, false
	}
	if t.double, resolved = t.simple(ctaBuiltin("double")); !resolved {
		return ctaTypes{}, false
	}
	return t, true
}

// simple resolves name to a simple type definition, reporting false where the
// {type definitions} hold none under that name or hold a COMPLEX one.
//
// The default arm is unreachable: [xsd.TypeDefinition] is a sealed sum of
// exactly the two variants named (STYLE T2's schema-closed-set exception), and
// the two are matched with the receiver kinds that sum's doc fixes — by value
// for a complex type, by pointer for a simple one.
func (t ctaTypes) simple(name xsd.QName) (*xsd.SimpleType, bool) {
	td, declared := t.resolver.Type(name)
	if !declared {
		return nil, false
	}
	switch d := td.(type) {
	case *xsd.SimpleType:
		return d, true
	case xsd.ComplexType:
		return nil, false
	default:
		return nil, false
	}
}

// literal is the builtin datatype a [16] ta-SimpleValue Literal carries: a
// DoubleLiteral (the one with an exponent, xpath20.md [73]) is xs:double, and
// an IntegerLiteral or DecimalLiteral is xs:decimal — xs:integer's primitive
// base, and so the type any two of them are compared in. A StringLiteral does
// not reach here; its type is xs:string with no text to inspect.
func (t ctaTypes) literal(text string) *xsd.SimpleType {
	for _, r := range text {
		if r == 'e' || r == 'E' {
			return t.double
		}
	}
	return t.decimal
}

// primitive resolves st's {primitive type definition}, reporting false where
// the chain cannot be walked or the property is ·absent· — xs:anySimpleType,
// xs:anyAtomicType, and the list and union varieties, none of which any
// operand of this grammar can carry once the cast targets are classified.
func (t ctaTypes) primitive(st *xsd.SimpleType) (*xsd.SimpleType, bool) {
	p, err := st.Primitive(t.resolver)
	if err != nil || p == nil {
		return nil, false
	}
	return p, true
}

// ancestor reports the ancestor of st named name — st ITSELF included — on the
// {base type definition} chain, or nil where the chain holds no such type. An
// unwalkable chain is the error, never a nil answer, so a caller cannot read
// "could not resolve the base" as "is not derived from it" (STYLE S3).
//
// TERMINATION: the walk carries no visited set (STYLE D4), on
// [xsd.SimpleType.Variety]'s terms — every base chain a finalized Schema holds
// is acyclic, established in Phase B before any pass that walks one runs.
func (t ctaTypes) ancestor(st *xsd.SimpleType, name xsd.QName) (*xsd.SimpleType, error) {
	for at := st; at != nil; {
		if at.Name() == name {
			return at, nil
		}
		base, err := at.Base(t.resolver)
		if err != nil {
			return nil, err
		}
		at = base
	}
	return nil, nil
}

// castTarget is the datatype [15] ta-CastExpr's tail and [18]
// ta-ConstructorFunction cast to, or the defect that keeps this engine from
// casting to it. The zero ctaDefect comes with the target, and every other
// defect with none.
//
// The target classes xpath20.md §3.10.2 separates are separated here, on the
// oracle's #894 ruling over xpath-valid clause 2.2.5, which puts only "those
// components that are present in every schema by definition" in the
// ·in-scope schema definitions· — so a schema's own type definitions never
// are, and a target naming one is no valid XPath outside the required subset
// but a static error:
//
//   - a BUILTIN atomic datatype, xs:QName included, is the required subset's
//     own case, which §3.12.6 clause 4 fixes for the cast spelling and clause 3
//     for the constructor spelling, and it is admitted. §3.10.2 excludes only
//     xs:NOTATION and xs:anyAtomicType by name; xs:QName's cast is
//     context-dependent (PRINCIPLES 19), and which operands it admits is
//     castsFrom's to judge (qnameTarget).
//   - xs:anyAtomicType and xs:NOTATION are err:XPST0080, a ctaStaticError
//     defect: "the target type cannot be xs:NOTATION or xs:anyAtomicType".
//   - every other name is err:XPST0051, a ctaStaticError defect: "The target
//     type must be an atomic type that is in the in-scope schema types". That
//     is a name resolving to nothing — ctaUnresolvedName among them, whose
//     err:XPST0081 the parser recorded first — a name outside the XSD
//     namespace, a complex type, and a non-atomic builtin (xs:anySimpleType,
//     and the list builtins xs:IDREFS, xs:NMTOKENS, xs:ENTITIES).
//   - xs:untypedAtomic declines, a ctaUnsupported defect, and is not
//     charged: XPath's predefined types include it (xpath20.md §2.1.1) where
//     clause 2.2.5's list does not, and the ruling holds the target to no
//     static error.
//
// A base chain this resolver cannot walk declines too, and is no verdict.
//
// The two STATIC arms are the CAST spelling's charge alone: the constructor
// spelling names a function, not a target type, and declines on any defect
// here (ctaParser.constructorFunction).
func (t ctaTypes) castTarget(name xsd.QName) (*xsd.SimpleType, ctaDefect) {
	if name == ctaBuiltin("untypedAtomic") {
		return nil, ctaDefect{kind: ctaUnsupported}
	}
	if name == ctaBuiltin("anyAtomicType") || name == ctaBuiltin("NOTATION") {
		return nil, ctaStaticDefect(ruleXPST0080, "the cast target %q is xs:NOTATION or xs:anyAtomicType, which err:XPST0080 forbids", name)
	}
	st, declared := t.simple(name)
	if !declared || name.Space != xsd.XMLSchemaNS {
		return nil, ctaStaticDefect(ruleXPST0051, "the cast target %q is no simple type in the in-scope schema types, which err:XPST0051 forbids", name)
	}
	// §3.10.2's "the target type must be an atomic type" is decided on the
	// {primitive type definition} and not on the {variety}, because the
	// property is ·absent· for EXACTLY the targets that rule excludes: the
	// list and union varieties, xs:anySimpleType's ·absent· {variety}, and
	// xs:anyAtomicType, which is excluded by name above.
	p, err := st.Primitive(t.resolver)
	if err != nil {
		return nil, ctaDefect{kind: ctaUnsupported}
	}
	if p == nil {
		return nil, ctaStaticDefect(ruleXPST0051, "the cast target %q is no atomic type, which err:XPST0051 forbids", name)
	}
	return st, ctaDefect{}
}

// qnameTarget reports whether target's {primitive type definition} is
// xs:QName, the one admitted cast target whose lexical mapping reads the
// static context's namespaces (xpath-functions.md §5.3).
func (t ctaTypes) qnameTarget(target *xsd.SimpleType) bool {
	p, resolved := t.primitive(target)
	return resolved && p.Name() == ctaBuiltin("QName")
}

// instanceItem is the static type of v's items (ctaStaticOf) as far as it
// decides the "actual type" xpath20.md §2.5.4.2 matches an AtomicType
// against — reporting ok false where it decides nothing, which declines an
// `instance of` over v ([CompileAssertionTest]'s withhold). It is each item's
// DYNAMIC type, and derived false, for an untyped read (xs:untypedAtomic: an
// attribute or `$value` whose type is ·special·, a mixed child, `.`), a typed
// attribute (its type annotation: no xsi:type reaches an attribute, so its
// {attribute use}'s type is its own), `$value` (Datatypes dt-xdmrep clause 2
// makes its dynamic type the ·nearest built-in datatype· T2, and every
// AtomicType itemMatches admits is builtin, so T2 and the {simple type
// definition} derive from it alike), a cast (its target), fn:count, an F&O
// function result and a castable or instance-of expression (each its result
// type), a StringLiteral, and the statically empty `$value`; and for a read of
// an absent context item or a rooted path, which raises before any item
// exists. An fn:distinct-values call answers as its operand does, each item
// keeping its own type (xpath-functions.md §15.1.6).
//
// For a typed child, through fn:distinct-values too, and for the k-th of them
// (ctaChildAt), derived is true: each item's dynamic type is item's or derived
// from it, and need not be item's. A child's annotation is its own ·governing
// type definition·'s (cvc-assertion clause 1.2 defines E's [[children]]'
// properties "in the usual way"), which an xsi:type can make a type derived
// from the ·locally declared type· the step was compiled against, and
// [ChildElements] carries a child only where its own simple type is that one or
// validly derived from it. So an AtomicType the compiled type derives from
// matches every item, and one it does not derive from decides nothing
// (instanceTail). So for an element a path of child steps reads for its value
// (ctaElementPath), whose annotation is its own governing type's on the same
// terms — an xsi:type, a {type table}, a substitution group member or a
// wildcard-resolved declaration can each make it differ from the type
// [ElementTypes] answered — and which [Tally.ElementValue] carries only where
// its own type is that one or validly derived from it.
//
// GAP(xpath): a numeric literal, an arithmetic result, a quantifier's range
// variable over `$value` (ctaRangeItem), and one over a typed child step,
// `$v/following-sibling::M[1]` over it and an attribute step over either, a
// function call ending it or not (ctaBoundChild, ctaBoundSibling,
// ctaBoundAttribute, ctaBoundAttributeCall), decline. An IntegerLiteral is
// xs:integer (xpath20.md §3.1.1) where ctaTypes.literal types it xs:decimal,
// and arithmetic's result type is B.2's over those operand types, so `1
// instance of xs:integer` would answer false; the range variables are left
// out until a {test} needs them. The direction is the withhold
// [CompileAssertionTest] reports. (#1042)
func (t ctaTypes) instanceItem(v ctaValue) (item ctaStatic, derived, ok bool) {
	switch n := v.(type) {
	case ctaLiteral:
		return ctaStaticOf(v), false, n.st == t.str
	case ctaDistinctValues:
		return t.instanceItem(n.operand)
	case ctaTypedChild, ctaChildAt, ctaElementPath:
		return ctaStaticOf(v), true, true
	case ctaAttr, ctaTypedAttr, ctaUntypedChild, ctaValueVar, ctaUntypedValue, ctaEmptyValue,
		ctaContextAtom, ctaNoContextItem, ctaNoDocumentRoot, ctaCast, ctaCastable, ctaInstanceOf, ctaCount,
		ctaMatch, ctaUnaryString, ctaPresence, ctaStringFunction, ctaConcat, ctaSubstringAfter, ctaNumber, ctaCurrentDate,
		ctaNoFocus, ctaNamespaceURI:
		return ctaStaticOf(v), false, true
	}
	return nil, false, false
}

// itemMatches reports whether an item whose dynamic type is item matches the
// AtomicType name (xpath20.md §2.5.4.2): "An AtomicType AtomicType matches an
// atomic value whose actual type is AT if derives-from(AT, AtomicType) is
// true", walked up item's {base type definition} chain (ancestor) — so an
// xs:integer matches xs:decimal, an xs:untypedAtomic matches only
// xs:untypedAtomic and xs:anyAtomicType, and nothing is cast. admitted is
// false where name is no builtin atomic type. xs:untypedAtomic, which XPath
// puts in the in-scope schema types (xpath20.md §2.1.1) and no resolver
// holds, is one; a name resolving to nothing, to a complex type, a list or
// xs:anySimpleType — err:XPST0051, the name is not "an atomic type that is in
// the in-scope schema types" (§2.5.4.2) — or to a user-defined type is not.
// item is ctaEmptySequence only for an operand with no item to match, whose
// answer is never read.
//
// GAP(xpath): a user-defined AtomicType declines, though it is in the
// in-scope schema types: `$value` carries its {simple type definition}'s
// nearest builtin as its dynamic type (dt-xdmrep clause 2) where a typed
// attribute carries its own type annotation, so the two answer such a name
// differently, and instanceItem keeps no record of which it read. The
// XPST0051 arm is folded into the same withhold. (#1042)
func (t ctaTypes) itemMatches(item ctaStatic, name xsd.QName) (matches, admitted bool) {
	if name == ctaBuiltin("untypedAtomic") {
		_, untyped := item.(ctaUntypedAtomic)
		return untyped, true
	}
	if name.Space != xsd.XMLSchemaNS {
		return false, false
	}
	st, declared := t.simple(name)
	if !declared {
		return false, false
	}
	if _, atomic := t.primitive(st); !atomic && name != ctaBuiltin("anyAtomicType") {
		return false, false
	}
	switch s := item.(type) {
	case ctaTyped:
		at, err := t.ancestor(s.st, name)
		if err != nil {
			return false, false
		}
		return at != nil, true
	case ctaUntypedAtomic:
		return name == ctaBuiltin("anyAtomicType"), true
	default:
		return false, true
	}
}

// annotationDerives reports derives-from(AT, name) (xpath20.md §2.5.4) for AT
// the type annotation of the attribute node v, which an [60] AttributeTest's
// TypeName asks (§2.5.4.5). v is an attribute of E's typed instance
// (ctaParser.attributeInstanceOf), whose annotation is its {attribute use}'s
// type, no xsi:type reaching an attribute: a ctaTypedAttr's st, or for a
// ctaAttr the ·special· type itself, xs:anySimpleType or xs:anyAtomicType —
// only its typed value is xs:untypedAtomic (xpath-datamodel §3.3.1.2). So no
// such attribute is annotated xs:untypedAtomic, the annotation of a
// skip-assessed one: [TypedAttributes] carries only attributes that matched an
// {attribute use}. derives-from walks AT's {base type definition} chain
// (ancestor), a user-defined AT's too, which is §2.5.4's
// implementation-dependent mechanism for an AT outside the in-scope schema
// definitions; neither ·special· type derives from any name but
// xs:anySimpleType and, for xs:anyAtomicType, itself.
//
// decided is false where name is neither xs:untypedAtomic nor a simple type in
// the XSD namespace — a user-defined type, which xpath-valid clause 2.2.5 keeps
// out of the in-scope schema types (err:XPST0008), xs:anyType, a name
// resolving to nothing — and for xs:anyAtomicType over a ctaAttr, which keeps
// no record of which ·special· type annotates it.
//
// GAP(xpath): each of those declines, the err:XPST0008 arm folded into the
// withhold. The direction is the withhold [CompileAssertionTest] reports.
// (#1042)
func (t ctaTypes) annotationDerives(v ctaValue, name xsd.QName) (derives, decided bool) {
	if _, simple := t.simple(name); name.Space != xsd.XMLSchemaNS || !simple && name != ctaBuiltin("untypedAtomic") {
		return false, false
	}
	switch n := v.(type) {
	case ctaTypedAttr:
		at, err := t.ancestor(n.st, name)
		if err != nil {
			return false, false
		}
		return at != nil, true
	case ctaAttr:
		if name == ctaBuiltin("anyAtomicType") {
			return false, false
		}
		return name == ctaBuiltin("anySimpleType"), true
	default:
		return false, false
	}
}

// castsFrom reports whether this engine casts the operand v to target, a type
// castTarget admitted, at all. A literal reaches castsFrom under either cast
// spelling and any target, and literalCastsTo judges it. Of every other
// operand, it is false for exactly two shapes, each an operand whose {primitive
// type definition} is not xs:string and whose type is neither target nor
// derived from it, nor an xs:date, xs:dateTime or xs:time one whose target is
// xs:string:
//
//   - a TYPED operand read off the instance or computed from it — an attribute
//     (ctaTypedAttr), a child element (ctaTypedChild), `$value` (ctaValueVar,
//     each item of a listed one), a count of its nodes (ctaCount, xs:integer),
//     the result of arithmetic (ctaArith, its B.2 result type,
//     arithmeticResult), of a castable or instance-of expression (ctaCastable,
//     ctaInstanceOf, xs:boolean) or of an F&O function call (ctaMatch and
//     ctaPresence, xs:boolean; ctaUnaryString, xs:integer or xs:string;
//     ctaStringFunction, ctaConcat and ctaSubstringAfter, xs:string;
//     ctaNumber, xs:double; ctaDistinctValues, its typed operand's type, a
//     literal's included; ctaCurrentDate, xs:date; ctaNamespaceURI,
//     xs:anyURI), or a cast of one of them, or of an operand the second shape
//     names, that is not in the string family (castSource, its target);
//   - a cast of any other shape whose target is xs:float or xs:double
//     (floatingSource): a cast to either over an untyped operand, a
//     string-family one or a literal other than a DoubleLiteral, as in
//     `string(xs:float('1.5'))` or `string(xs:double(@s))` over an xs:string
//     @s.
//
// Every other operand casts as [CompileCTATest] states, the statically empty
// `$value` (ctaEmptyValue) among them: it holds no item to convert. So does
// fn:position or fn:last over an absent focus (ctaNoFocus), which raises
// err:XPDY0002 before it yields one (xpath-functions.md §16.1, §16.2), so
// `string(position())` fails an assertions facet. So do the
// xs:untypedAtomic operands — an attribute (ctaAttr) or `$value`
// (ctaUntypedValue) of ·special· type, and a child of mixed content
// (ctaUntypedChild) — each item of which a cast to any target validates as a
// lexical of that target (F&O §17.1.1).
//
// Three typed operands are admitted:
//
//   - the string family, to any target, because xpath-functions.md §17.1.1
//     makes a cast from xs:string one datatype validation of the value's own
//     string, which ctaPromote performs exactly: the ·canonical
//     representation· of an xs:string value is that string (f-stringCanmap);
//   - an operand whose type is target itself or derived from it by
//     restriction, at any depth: F&O §17.2 case 4, "When SV is an instance of
//     the TT, the cast always succeeds (Identity cast)", and §17.3, "it is
//     always possible to cast a value of any atomic type to an atomic type from
//     which it is derived, directly or indirectly, by restriction ... The
//     result will have the same value as the original". ctaPromote hands the
//     value on unrendered wherever it already is one of target's
//     (ctaRepresents). Every typed operand above is atomic and its
//     base chain is restriction alone (typedAtomic, valueVariable), and
//     castTarget has already excluded xs:NOTATION, xs:anyAtomicType and every
//     non-atomic target, the exclusions §17.3 and §3.10.2 make. The relation is
//     the operand's type below the target and never the reverse, nor a shared
//     primitive: `xs:integer(@d)` over an xs:decimal @d is §17.4's, not §17.3's;
//   - an operand whose {primitive type definition} is xs:date, xs:dateTime or
//     xs:time (localValueSource), to xs:string itself: §17.1.2 makes TV "the
//     local value", its components rendered with the timezone "if present",
//     which is the ·canonical representation· ctaPromote renders — offset 0 as
//     `Z`, any other as its signed hh:mm (timezoneCanonicalFragmentMap), never
//     UTC-shifted, no trailing fractional zeros, and midnight as 00:00:00,
//     never "24".
//
// To a target whose {primitive type definition} is xs:QName (qnameTarget),
// §17.1's own rule replaces all three: "Casting is permitted from xs:string
// literals to xs:QName ... If the argument to such a cast is computed
// dynamically, [err:XPTY0004] is raised if the value is of any type other than
// xs:QName ... (including the case where it is an xs:string)". So a literal is
// admitted only as a StringLiteral (literalCastsTo's string-family rule, which
// ctaParser.cast resolves against the static context), and every other
// operand only where its static type is target or derived from it, the second
// rule's identity cast.
//
// The cast spelling and `castable as` take a [16] ta-SimpleValue operand, and
// so does the constructor spelling on a façade that calls no library function;
// elsewhere a constructor's operand is any argument a library call takes
// (ctaParser.constructorOperand). So a count, an arithmetic, castable or
// function result and a cast reach castsFrom as fn:string's argument or an
// fn:concat argument (ctaParser.stringOf), whose target is xs:string — of
// those, the second rule admits nothing the first does not, and the third
// admits one whose type has a date/time primitive, as `string(xs:date(@d))`
// and fn:distinct-values over a typed date/time operand do — and as a
// constructor's operand, to any target: `xs:date(concat(string($value),
// '!!!'))` by the first rule, its fn:concat result being an xs:string, and
// `xs:integer(count(e))` by the second.
//
// GAP(xpath): a cast from any OTHER typed operand, a literal aside, is
// declined — §17.4's cast within a branch of the hierarchy that is not to an
// ancestor, and §17.1's and §17.5's casts across primitives — because
// xpath-functions.md §17 defines those over the VALUE, not over a re-validated
// canonical lexical: xs:decimal to xs:integer truncates (§17.1.3.4) where the
// round-trip ctaPromote would perform raises err:FORG0001 for "3.5", and an
// assertion a raised cast makes false is a charge (cvc-assertion), so the
// round-trip would fabricate one. An xs:float or xs:double operand, a
// DoubleLiteral included (literalCastsTo), is the same case: §17.1.2 renders a
// value of absolute value in [0.000001,
// 1000000) as an xs:decimal, so `xs:string(1.5e0)` is "1.5" where ctaPromote
// would render "1.5E0", and §17.1.3 casts it to xs:decimal or xs:integer by its
// value, which the canonical "1.5E0" fails to validate as. Those value-defined
// casts are to the targets F&O §17.1's casting table marks Y or M from xs:float
// and xs:double: xs:untypedAtomic, xs:string, the numerics and xs:boolean
// (§17.1.6). To every target it marks N — the durations, the date/time and g*
// types, the binaries and xs:anyURI — §17.1 makes the cast err:XPTY0004, a type
// error with no value defined, and it declines too: a withhold where the
// round-trip had decided it, raising for `xs:date(1.5e0)` and succeeding for
// `xs:anyURI(1.5e0)`, "1.5E0" being an xs:anyURI lexical. fn:string over such an
// operand, a node of such a type included, is that cast to xs:string and
// declines with it. A date/time operand's cast to a target derived from
// xs:string, and a g* or duration operand's cast to xs:string, are withheld
// only for scope: §17.5 and §17.1.2 define each over the canonical
// representation ctaPromote renders, and admitting them is a widening of
// localValueSource's set or of its target test, not a new renderer (#1042).
// A cast to xs:QName from any operand but a StringLiteral or one of type
// xs:QName — an xs:untypedAtomic operand, a computed xs:string, any other
// typed one — is §17.1's err:XPTY0004 and declines too, a withhold where the
// error is defined; so does a cast from an xs:QName operand to any other
// target, and fn:string over one, which is that cast to xs:string here: §17.1.2
// builds its string from the value's retained prefix, which no ·canonical
// representation· renders (value.Mapping). The direction is the withhold
// [CompileAssertionTest] reports: the assertion is declined, never charged and
// never satisfied. (#1042)
func (t ctaTypes) castsFrom(v ctaValue, target *xsd.SimpleType) bool {
	if lit, isLiteral := v.(ctaLiteral); isLiteral {
		return t.literalCastsTo(lit, target)
	}
	if t.qnameTarget(target) {
		typed, isTyped := ctaStaticOf(v).(ctaTyped)
		if !isTyped {
			return false
		}
		at, err := t.ancestor(typed.st, target.Name())
		return err == nil && at != nil
	}
	st, judged := t.castSource(v)
	if !judged || t.stringSource(st) {
		return true
	}
	if t.localValueSource(st) && target.Name() == ctaBuiltin("string") {
		return true
	}
	at, err := t.ancestor(st, target.Name())
	return err == nil && at != nil
}

// literalCastsTo is castsFrom's answer for a literal: a StringLiteral, a
// NumericLiteral (xpath20.md [43]) or the xs:boolean an fn:true or fn:false
// call returns (constantCall). ctaPromote casts one by re-validating its
// ·canonical representation· as target, which is the cast F&O §17 defines in
// exactly these cases, and a literal is admitted in them alone:
//
//   - a StringLiteral, to any target: §17.1.1, castsFrom's string-family rule;
//   - to the literal's own type or an ancestor of it: §17.2 case 4 and §17.3,
//     castsFrom's ancestor rule. It is the one rule that admits a
//     DoubleLiteral, and it leaves that xs:double itself — `xs:double(1.5e0)`,
//     §17.2 case 4's identity cast: xs:double's two simple ancestors are never
//     a target, castTarget excluding xs:anyAtomicType by name and
//     xs:anySimpleType, xs:anyAtomicType's {base type definition}
//     (xmlschema11-2 §4.1.6), through its ·absent· {primitive type
//     definition};
//   - an IntegerLiteral or a DecimalLiteral, typed xs:decimal here (literal),
//     to a target whose {primitive type definition} is xs:string — §17.1.2
//     renders an xs:decimal by its canonical form, an integer-valued one as
//     an xs:integer, which is xs:decimal's ·canonical representation· — or is
//     xs:float or xs:double, which §17.1.3.1 and §17.1.3.2 define as "TV is
//     xs:double(SV cast as xs:string)";
//   - an IntegerLiteral, whose value xpath20.md §3.1.1 makes an xs:integer, to
//     xs:integer or a type derived from it: §17.1.3.4's "TV is SV" and §17.4's
//     cast down the branch, which checks the value against the target's facets
//     and its pattern against the source's canonical lexical — the round trip;
//   - an fn:true or fn:false, to a target whose primitive is xs:string:
//     §17.1.2's "In all other cases, TV is the ... canonical representation of
//     SV".
//
// GAP(xpath): every other cast of a literal is declined, because F&O §17
// defines it over the VALUE and not over a re-validated canonical lexical:
// a DecimalLiteral to xs:integer discards the fractional part (§17.1.3.4),
// where the round trip raises err:FORG0001 for "1.5" and so decides
// `xs:integer(1.5) = 1` false; a numeric literal to xs:boolean is true for
// every non-zero value (§17.1.6), where "2" fails to validate as an
// xs:boolean; and a DoubleLiteral to any target but its own type or an
// ancestor is castsFrom's GAP(xpath) case (floatingSource). To a target
// §17.1's casting table marks N from xs:decimal or xs:integer — the
// durations, the date/time and g* types, the binaries, xs:anyURI and xs:QName
// — the cast is err:XPTY0004, a type error with no value defined, where the
// round trip succeeds for `xs:anyURI(1.5)`, "1.5" being an xs:anyURI lexical,
// and it declines too; so does an fn:true or fn:false to xs:QName, which §17.1
// marks N from xs:boolean. A DecimalLiteral to a type derived from xs:integer declines
// with the cast to xs:integer itself. The direction is castsFrom's: the
// assertion is withheld, never charged and never satisfied. (#1042)
func (t ctaTypes) literalCastsTo(lit ctaLiteral, target *xsd.SimpleType) bool {
	if t.stringSource(lit.st) {
		return true
	}
	at, err := t.ancestor(lit.st, target.Name())
	if err != nil {
		return false
	}
	if at != nil {
		return true
	}
	source, sourceResolved := t.primitive(lit.st)
	tp, targetResolved := t.primitive(target)
	if !sourceResolved || !targetResolved {
		return false
	}
	switch source.Name() {
	case ctaBuiltin("decimal"):
		switch tp.Name() {
		case ctaBuiltin("string"), ctaBuiltin("float"), ctaBuiltin("double"):
			return true
		}
		if strings.ContainsRune(lit.text, '.') {
			return false
		}
		integer, err := t.ancestor(target, ctaBuiltin("integer"))
		return err == nil && integer != nil
	case ctaBuiltin("boolean"):
		return tp.Name() == ctaBuiltin("string")
	}
	return false
}

// castSource is the type castsFrom judges a cast from v by — the static type
// of a typed operand either of castsFrom's shapes names, the type it carries
// (ctaCarriedType) — or false where v casts whatever the target. A cast is
// such an operand itself where its own operand is one that is not in the
// string family: its value is then that operand's, under a new annotation, and
// casting it on is a cast from a typed instance value as much as the first
// one, so `string(xs:decimal(@d))` over an xs:decimal @d is §17.1.2's cast
// across primitives, from xs:decimal to xs:string, and declines under
// castsFrom's GAP(xpath) with `string(@d)`. A literal is not such an operand:
// castsFrom judges one by literalCastsTo, and as a cast's operand it leaves
// the cast judged by its target alone (floatingSource), the cast
// literalCastsTo admitted. Nor is fn:position or fn:last over an absent focus
// (ctaNoFocus), which raises before it yields an item to cast.
func (t ctaTypes) castSource(v ctaValue) (*xsd.SimpleType, bool) {
	switch n := v.(type) {
	case ctaLiteral, ctaNoFocus:
		return nil, false
	case ctaDistinctValues:
		typed, isTyped := ctaStaticOf(n).(ctaTyped)
		return typed.st, isTyped
	case ctaCast:
		inner, judged := t.castSource(n.operand)
		if !judged || t.stringSource(inner) {
			return t.floatingSource(n)
		}
		return n.target, true
	}
	return ctaCarriedType(v)
}

// floatingSource is castSource's answer for a cast n over an operand castSource
// does not judge or judges into the string family: n casts whatever the target
// unless its own target has a {primitive type definition} of xs:float or
// xs:double, as in `string(xs:float('1.5'))`, which it reports as judged, so
// castsFrom admits it only to its own type or an ancestor of it. A target
// whose primitive does not resolve is judged the same way, and castsFrom's
// ancestor rule declines it: castTarget admits no target whose primitive does
// not resolve.
func (t ctaTypes) floatingSource(n ctaCast) (*xsd.SimpleType, bool) {
	p, resolved := t.primitive(n.target)
	if resolved && p.Name() != ctaBuiltin("float") && p.Name() != ctaBuiltin("double") {
		return nil, false
	}
	return n.target, true
}

// stringSource reports whether st's {primitive type definition} is xs:string,
// the family castsFrom admits a cast from to any target.
func (t ctaTypes) stringSource(st *xsd.SimpleType) bool {
	p, resolved := t.primitive(st)
	return resolved && p.Name() == ctaBuiltin("string")
}

// localValueSource reports whether st's {primitive type definition} is
// xs:date, xs:dateTime or xs:time, a primitive §17.1.2 renders by its
// canonical representation: its cast to xs:string is the local value, which
// is the ·canonical representation· Datatypes dateCanonicalMap,
// dateTimeCanonicalMap and timeCanonicalMap give it. castsFrom admits a cast
// from such a type to xs:string itself.
func (t ctaTypes) localValueSource(st *xsd.SimpleType) bool {
	p, resolved := t.primitive(st)
	if !resolved {
		return false
	}
	switch p.Name() {
	case ctaBuiltin("date"), ctaBuiltin("dateTime"), ctaBuiltin("time"):
		return true
	}
	return false
}

// stringArgument converts v, one argument of an F&O function whose parameter
// is xs:string?, by xpath20.md §3.1.5's function conversion rules as far as
// v's static type settles them — atomization, then "each item of type
// xs:untypedAtomic is cast to the expected atomic type", then B.1's URI
// promotion, and err:XPTY0004 for an item that matches none of them:
//
//   - an xs:untypedAtomic operand is held under its cast to xs:string, the
//     `?` allowing the empty sequence and the cast raising err:XPTY0004 for two
//     or more items (ctaCastItem);
//   - a typed operand whose {primitive type definition} is xs:string or
//     xs:anyURI (ctaStringLike) is held as it is: subtype substitution and the
//     URI promotion convert it, and neither changes its string;
//   - the statically empty operand is held as it is: it holds no item;
//   - every other typed operand is held mistyped, and ctaStringOf raises
//     err:XPTY0004 for any item it yields. That is raised dynamically and not
//     here: an empty operand of any type matches xs:string?, so an absent
//     xs:integer attribute is the zero-length string.
func (t ctaTypes) stringArgument(v ctaValue) ctaStringArgument {
	switch s := ctaStaticOf(v).(type) {
	case ctaUntypedAtomic:
		return ctaStringArgument{operand: ctaCast{operand: v, target: t.str, allowsEmpty: true}}
	case ctaTyped:
		p, resolved := t.primitive(s.st)
		return ctaStringArgument{operand: v, mistyped: !resolved || !ctaStringLike(p)}
	case ctaEmptySequence:
		return ctaStringArgument{operand: v}
	}
	return ctaStringArgument{operand: v, mistyped: true} // ctaStatic has the three arms above; never reached
}

// typedAtomic reports whether this engine reads a value of type st off the
// instance as a typed operand — an attribute whose type [AttributeTypes]
// answered, an element whose type [ElementTypes] answered for its path, a child
// for the one-step path [N], or one item of `$value` (valueVariable) —
// classified the way castTarget classifies a cast target: by its {primitive
// type definition}, which is ·absent· for EXACTLY the types whose atomized
// value is not one atomic value of a type known at compile time:
//
//   - a list or union {variety}: a list atomizes to a sequence, and a union's
//     value takes the type of its ·validating· member, which only the instance
//     decides;
//   - the two ·special· types ([xsd.SimpleType.IsSpecial]), whose typed value
//     is xs:untypedAtomic instead — which a caller that reads one untyped asks
//     about before this, and never reaches here with.
//
// An xs:QName primitive is admitted although it has no ·canonical
// representation· (value.Mapping) for ctaPromote to convert through, because
// no conversion of such a value renders it: B.2 compares an xs:QName only with
// another (op:QName-equal, eq and ne alone), which shared settles in xs:QName
// itself and ctaPromote hands on unrendered (ctaRepresents); an
// xs:untypedAtomic operand against it is cast TO xs:QName (untypedAgainst),
// which raises the err:XPTY0004 xpath-functions.md §17.1 makes it, since no
// cast but a StringLiteral's carries the static context its mapping reads
// (ctaParser.cast); and castsFrom admits a cast from one only to its own type.
//
// GAP(xpath): the list and union varieties decline the whole assertion, never
// charge it and never satisfy it — the withhold [CompileAssertionTest]
// reports — and so does an xs:NOTATION primitive, which is not admitted on
// xs:QName's terms until a {test} needs it. An admitted xs:QName operand still
// declines wherever it is cast to another type, fn:string included, under
// castsFrom's marker. (#1042)
func (t ctaTypes) typedAtomic(st *xsd.SimpleType) bool {
	if st == nil {
		return false
	}
	p, resolved := t.primitive(st)
	if !resolved {
		return false
	}
	return p.Name() != ctaBuiltin("NOTATION")
}

// valueVariable classifies the {simple type definition} st of a simple
// {content type} as `$value`'s static type (cvc-assertion clause 2.3.1), which
// is the XDM representation of an ·actual value· of st (Datatypes dt-xdmrep):
//
//   - an st typedAtomic admits is one atomic value of st;
//   - a list whose {item type definition} typedAtomic admits is the sequence
//     of its items, each of that item type — "a sequence of one or more atomic
//     values" (cvc-assertion clause 2.3.1's Note), or none for an empty list;
//   - a ·special· st ([xsd.SimpleType.IsSpecial]) is one xs:untypedAtomic
//     value, E's [schema normalized value] (xpath-datamodel §3.3.1.2, Datatypes
//     dt-xdmrep clause 1), which is ctaUntypedValue;
//   - anything else declines: a union, whose value takes the type of its
//     ·active basic member·, which only the instance decides; a list of such a
//     union; and every other st typedAtomic declines.
//
// GAP(xpath): each of those declines the whole assertion on [CompileAssertionTest]'s
// withhold. (#1042)
func (t ctaTypes) valueVariable(st *xsd.SimpleType) (ctaValue, bool) {
	if st.IsSpecial() {
		return ctaUntypedValue{}, true
	}
	if t.typedAtomic(st) {
		return ctaValueVar{atom: st}, true
	}
	item, err := st.Item(t.resolver)
	if err != nil || item == nil || !t.typedAtomic(item) {
		return nil, false
	}
	return ctaValueVar{atom: item, listed: true}, true
}

// arithmetic builds the node of the binary arithmetic operator op over l and r
// (xpath20.md §3.4), settling at compile time the type both operands are
// converted into and the type of the result, or reports false where this
// engine declines the pair.
//
//   - A statically empty operand makes the whole expression the empty sequence
//     (§3.4: "If either operand is an empty sequence, the result of the
//     operation is an empty sequence"), which is ctaEmptyValue; the other
//     operand "need not be evaluated" (§3.4), so it is not kept.
//   - An xs:untypedAtomic operand is cast to xs:double (§3.4, "If the atomized
//     operand is of type xs:untypedAtomic, it is cast to xs:double").
//   - The operation type is then B.1's promotion of the two numeric primitives
//     to the wider one, which is ctaWider and keeps its xs:float/xs:double
//     withhold: an xs:untypedAtomic operand against an xs:float one reaches it
//     as xs:double against xs:float.
//   - The result type is B.2's (arithmeticResult).
//
// GAP(xpath): an operand whose {primitive type definition} is not numeric
// declines. B.2 gives `+`, `-`, `*` and `div` rows over the duration and
// date/time types, which this engine computes nothing in, and every other
// operand type is err:XPTY0004, which declining withholds rather than charges.
// The direction is the withhold [CompileAssertionTest] and [FacetAssertions]
// report: the assertion or the facet is declined, never charged and never
// satisfied. (#1042)
func (t ctaTypes) arithmetic(op ctaArithOp, l, r ctaValue) (ctaValue, bool) {
	if ctaIsEmpty(l) || ctaIsEmpty(r) {
		return ctaEmptyValue{}, true
	}
	lp, numeric := t.arithmeticOperand(l)
	if !numeric {
		return nil, false
	}
	rp, numeric := t.arithmeticOperand(r)
	if !numeric {
		return nil, false
	}
	operation, typing := ctaWider(lp, rp)
	if typing != ctaTypeSettled {
		return nil, false
	}
	// ctaWider answers one of two numeric primitives, so this arm never runs;
	// it declines rather than computing in a kind operation is not.
	kind, numeric := ctaNumericKindOf(operation)
	if !numeric {
		return nil, false
	}
	result, settled := t.arithmeticResult(op, kind, operation, l, r)
	if !settled {
		return nil, false
	}
	return ctaArith{op: op, kind: kind, operation: operation, st: result, left: l, right: r}, true
}

// arithmeticOperand is the numeric primitive one arithmetic operand is
// computed from: xs:double for an xs:untypedAtomic operand (§3.4), and its own
// {primitive type definition} for a typed one, reporting false where that is
// not numeric or does not resolve.
//
// GAP(xpath): an fn:sum call whose static type is xs:float or xs:double
// declines (ctaSum.floating), and so does an fn:distinct-values call over one,
// which takes its static type (ctaFloatingSum): over an empty argument it
// returns the xs:integer 0, which is not of that type, so the operation type
// B.1 settles from it would be wrong for that one value — `sum(a/@u) + 0.1 +
// 0.2 = 0.3` computed in xs:double where XPath computes it in xs:decimal,
// `sum(a/@u) div 0` NaN where XPath raises err:FOAR0001. The direction is the
// withhold [CompileAssertionTest] reports. (#1042)
func (t ctaTypes) arithmeticOperand(v ctaValue) (*xsd.SimpleType, bool) {
	if _, floating := ctaFloatingSum(v); floating {
		return nil, false
	}
	typed, isTyped := ctaStaticOf(v).(ctaTyped)
	if !isTyped {
		return t.double, true
	}
	p, resolved := t.primitive(typed.st)
	if !resolved || !ctaNumeric(p) {
		return nil, false
	}
	return p, true
}

// arithmeticResult is the type xpath20.md B.2 gives op over two operands
// computed in operation, the numeric primitive of the given kind: xs:integer
// for `idiv` whatever the operands (op:numeric-integer-divide); operation
// itself where that is xs:float or xs:double; and, in xs:decimal, xs:integer
// where both operands are derived from xs:integer and the operator is not
// `div` — whose two-xs:integer row is xs:decimal (xpath-functions.md §6.2.4)
// — and xs:decimal otherwise. It reports false where xs:integer does not
// resolve or a {base type definition} chain cannot be walked.
//
// An IntegerLiteral is typed xs:decimal here as everywhere in this grammar
// (ctaTypes.literal), so `$value mod 2` over an xs:int `$value` is xs:decimal
// where B.2 says xs:integer. The two are one value, and nothing this grammar
// applies to the result tells them apart: a comparison and fn:boolean run in
// the primitive, and a cast from the result is written only as fn:string,
// which declines it as a cast from outside the string family (castsFrom).
func (t ctaTypes) arithmeticResult(op ctaArithOp, kind ctaNumericKind, operation *xsd.SimpleType, l, r ctaValue) (*xsd.SimpleType, bool) {
	if op != ctaIntegerDivide && (kind != ctaDecimalKind || op == ctaDivide) {
		return operation, true
	}
	integer, resolved := t.simple(ctaBuiltin("integer"))
	if !resolved {
		return nil, false
	}
	if op == ctaIntegerDivide {
		return integer, true
	}
	for _, v := range []ctaValue{l, r} {
		typed, isTyped := ctaStaticOf(v).(ctaTyped)
		if !isTyped {
			return operation, true
		}
		at, err := t.ancestor(typed.st, integer.Name())
		if err != nil {
			return nil, false
		}
		if at == nil {
			return operation, true
		}
	}
	return integer, true
}

// sum builds the node of a call to fn:sum over arg (xpath-functions.md
// §15.4.5), settling at compile time the type arg's items are read in and the
// call's static type (ctaSum), or reports false where this engine declines it.
// The items are read in their own type, or in xs:double where they are
// xs:untypedAtomic, which §15.4.5 casts to xs:double. The static type is,
// where that item type has a numeric {primitive type definition}, the type
// §6.2's numeric operators return over two of them: xs:integer for one
// derived from xs:integer, and the primitive otherwise; and, where it does
// not, xs:integer, the type of the only value such a call returns without
// raising — the 0 of an empty argument — since a non-empty one raises
// err:FORG0006.
//
// GAP(xpath): an item type with a duration {primitive type definition}
// declines, though §15.4.5 sums xs:yearMonthDuration and xs:dayTimeDuration
// values: this engine computes no duration arithmetic (ctaTypes.arithmetic).
// The direction is the withhold [CompileAssertionTest] reports. (#1042)
func (t ctaTypes) sum(arg ctaValue) (ctaValue, bool) {
	integer, resolved := t.simple(ctaBuiltin("integer"))
	if !resolved {
		return nil, false
	}
	item := t.double
	if typed, isTyped := ctaStaticOf(arg).(ctaTyped); isTyped {
		item = typed.st
	}
	p, resolved := t.primitive(item)
	if !resolved || p.Name() == ctaBuiltin("duration") {
		return nil, false
	}
	st := p
	if !ctaNumeric(p) {
		st = integer
	}
	at, err := t.ancestor(item, integer.Name())
	if err != nil {
		return nil, false
	}
	if at != nil {
		st = integer
	}
	return ctaSum{arg: arg, item: item, st: st, integer: integer}, true
}

// ctaTyping is which of the three outcomes settling a comparison's type
// reached, and the three are three DIFFERENT directions rather than degrees of
// one (STYLE P3):
//
//   - ctaTypeSettled carries the type both operands are converted into.
//   - ctaTypeErrored is err:XPTY0004 — no one type serves both operands, which
//     is a raised type error and so a ctaTypeError node, decided false for the
//     whole {test} by key-cta-ta-select clause 2 or by cvc-assertion.
//   - ctaTypeDeclined is the compile-time WITHHOLD of [CompileCTATest] and
//     [CompileAssertionTest]: this engine will not decide the pair at all, and
//     the caller leaves the element's ·governing type definition· undetermined
//     or the assertion unevaluated rather than settled on a wrong answer.
//
// ctaTypeErrored is the zero value, so a fault that returns no type reports
// the error direction and never the withhold by accident.
type ctaTyping byte

const (
	ctaTypeErrored ctaTyping = iota
	ctaTypeDeclined
	ctaTypeSettled
)

// comparison settles the type a general comparison of l against r with
// operator op converts BOTH its operands into, per xpath20.md §3.5.2 clause
// 2's magnitude-relationship rules and B.1's type promotions, and reports
// ctaTypeErrored where B.2 admits no such comparison.
//
// The two error directions are ONE err:XPTY0004 and so one ctaTyping: §3.5.1
// raises it both where no single type serves the two operands and where the
// operand types "are not a valid combination for the given operator,
// according to the rules in B.2 Operator Mapping". Deciding both here is what
// makes a constructed ctaCompare B.2-legal by construction, so evaluation
// never meets an operand pair its operator does not admit.
func (t ctaTypes) comparison(op ctaComparator, l, r ctaValue) (*xsd.SimpleType, ctaTyping) {
	if !t.sumCompares(l, r) || !t.sumCompares(r, l) {
		return nil, ctaTypeDeclined
	}
	if st, empty := t.againstEmpty(l, r); empty {
		return st, ctaTypeSettled
	}
	st, typing := t.converted(l, r)
	if typing != ctaTypeSettled {
		return nil, typing
	}
	admitted, err := t.admitsComparison(op, st)
	if err != nil || !admitted {
		return nil, ctaTypeErrored
	}
	return st, ctaTypeSettled
}

// valueComparison settles the type a value comparison of l against r with
// operator op converts BOTH its operands into, per xpath20.md §3.5.1, and
// reports ctaTypeErrored where B.2 admits no such comparison — the same
// err:XPTY0004 comparison reports, decided here for the same reason.
//
// It differs from comparison in one rule only, and the difference is §3.5.1's
// own: step 4 casts an xs:untypedAtomic operand to xs:string whatever the
// other operand is, where §3.5.2 clause 2 chooses its target from the other
// operand (untypedAgainst). "The purpose of this rule is to make value
// comparisons transitive." Both operands are then typed, and "converted to
// their least common type by a combination of type promotion and subtype
// substitution", which is shared — and B.2's rows decide the rest.
//
// Only the two façades whose ctaFacade.comparesValues is true reach here, the
// assertion façade (ctaAssertionFacade) and the facet façade (ctaFacetFacade).
// The untyped operands they build are an attribute whose type is ·special·
// (ctaAssertionFacade.attribute), `$value` over a ·special· type
// (ctaUntypedValue), a child of mixed content (ctaUntypedChild), `.`
// (ctaContextAtom), and three that raise before they are compared: the rooted
// path (ctaNoDocumentRoot, err:XPDY0050), a read of the absent context item
// (ctaNoContextItem, err:XPDY0002, xpath20.md §3.1.4) and an fn:doc call
// (ctaNoDocument, err:FODC0005).
func (t ctaTypes) valueComparison(op ctaComparator, l, r ctaValue) (*xsd.SimpleType, ctaTyping) {
	if !t.sumCompares(l, r) || !t.sumCompares(r, l) {
		return nil, ctaTypeDeclined
	}
	if st, empty := t.againstEmpty(l, r); empty {
		return st, ctaTypeSettled
	}
	st, typing := t.shared(t.valueOperand(l), t.valueOperand(r))
	if typing != ctaTypeSettled {
		return nil, typing
	}
	admitted, err := t.admitsComparison(op, st)
	if err != nil || !admitted {
		return nil, ctaTypeErrored
	}
	return st, ctaTypeSettled
}

// sumCompares reports whether a comparison of v against other is decided in
// the comparison type B.1 settles from their static types for every value v
// can take, which is false for one case alone: v is an fn:sum call whose
// static type is xs:float or xs:double (ctaSum.floating), or an
// fn:distinct-values call over one, which takes its static type
// (ctaFloatingSum), and other is typed with an xs:decimal {primitive type
// definition}. Over an empty argument v is the xs:integer 0, which XPath
// compares with an xs:decimal in xs:decimal and this engine would compare in
// v's floating type, where a non-zero decimal of tiny magnitude rounds to
// zero: `sum(a/@u) = $tiny` would hold. A DecimalLiteral or IntegerLiteral
// other is admitted where its value is zero exactly when its rounding to v's
// type is, which settles every comparison of it with 0 alike in both types;
// every other such operand is not.
//
// GAP(xpath): such a comparison declines. The direction is the withhold
// [CompileAssertionTest] reports. (#1042)
func (t ctaTypes) sumCompares(v, other ctaValue) bool {
	sum, floating := ctaFloatingSum(v)
	if !floating {
		return true
	}
	typed, isTyped := ctaStaticOf(other).(ctaTyped)
	if !isTyped {
		return true
	}
	p, resolved := t.primitive(typed.st)
	if !resolved || p.Name() != ctaBuiltin("decimal") {
		return true
	}
	lit, isLiteral := other.(ctaLiteral)
	return isLiteral && ctaZeroAlike(lit.text, sum.st)
}

// ctaZeroAlike reports whether the decimal lexical text is zero exactly where
// its value rounded to the IEEE 754 type floating, xs:float or xs:double, is.
func ctaZeroAlike(text string, floating *xsd.SimpleType) bool {
	exact, parsed := new(big.Rat).SetString(text)
	if !parsed {
		return false
	}
	bits := 64
	if floating.Name() == ctaBuiltin("float") {
		bits = 32
	}
	rounded, err := strconv.ParseFloat(text, bits)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return false
	}
	return (exact.Sign() == 0) == (rounded == 0)
}

// valueOperand is the type one value-comparison operand is compared from: its
// own, or xs:string for an xs:untypedAtomic one (§3.5.1 step 4) — and for a
// statically empty one, which againstEmpty asks it of and which converts
// nothing in any type.
func (t ctaTypes) valueOperand(v ctaValue) *xsd.SimpleType {
	typed, isTyped := ctaStaticOf(v).(ctaTyped)
	if !isTyped {
		return t.str
	}
	return typed.st
}

// againstEmpty settles a comparison one of whose operands is the statically
// empty sequence (ctaEmptySequence), reporting false where neither is.
//
// No operator is ever applied there: a general comparison over an empty
// operand forms no pair and is false (§3.5.2), and a value comparison is the
// empty sequence (§3.5.1 step 2). So B.2 is not consulted, and the type
// answered is only what the OTHER operand is converted into before nothing is
// compared with it — valueOperand's answer for it: its own type, which
// converts nothing, or xs:string for an xs:untypedAtomic or empty one, which
// casts every lexical.
func (t ctaTypes) againstEmpty(l, r ctaValue) (*xsd.SimpleType, bool) {
	if ctaIsEmpty(l) {
		return t.valueOperand(r), true
	}
	if ctaIsEmpty(r) {
		return t.valueOperand(l), true
	}
	return nil, false
}

// ctaIsEmpty reports whether v is the statically empty sequence.
func ctaIsEmpty(v ctaValue) bool {
	_, empty := ctaStaticOf(v).(ctaEmptySequence)
	return empty
}

// converted settles the type alone, leaving B.2's operator rows to comparison.
//
// Two rules cover the three operand shapes this grammar builds, because an
// operand is either xs:untypedAtomic (an uncast untyped attribute, `$value`
// over ·special· content, a child of mixed content, `.`, or the rooted path
// and an fn:doc call, which raise before they are compared) or typed (a
// Literal, a cast, a constructor function, a typed attribute, a typed child
// element, a typed `$value`, an integer or string sequence):
//
//   - BOTH xs:untypedAtomic: clause 1, "the values are cast to the type
//     xs:string".
//   - EXACTLY ONE xs:untypedAtomic: clause 2 casts it to a type read off the
//     other operand's type T — untypedAgainst.
//   - NEITHER: one type must serve both, which is shared.
//
// B.1's URI promotion is NOT applied here. Answering xs:string for an
// xs:anyURI pair would change the type clause 2.4 casts the xs:untypedAtomic
// operand to, from xs:anyURI (whiteSpace collapse) to xs:string (preserve),
// and so change the answer: with @u=" http://a " and @v="http://a",
// `@u = @v cast as xs:anyURI` is true only if @u was collapsed. The promotion
// belongs where the comparison happens, and ctaCompare.eval applies it there
// by routing an xs:anyURI comparison type through the default collation
// exactly as it routes xs:string.
func (t ctaTypes) converted(l, r ctaValue) (*xsd.SimpleType, ctaTyping) {
	lt, lTyped := ctaStaticOf(l).(ctaTyped)
	rt, rTyped := ctaStaticOf(r).(ctaTyped)
	if !lTyped && !rTyped {
		return t.str, ctaTypeSettled
	}
	if !lTyped {
		return t.untypedAgainst(rt.st)
	}
	if !rTyped {
		return t.untypedAgainst(lt.st)
	}
	return t.shared(lt.st, rt.st)
}

// admitsComparison reports whether xpath20.md B.2 holds a row for op with both
// operands of type st, which is the question §3.5.1 makes err:XPTY0004 turn
// on: "If the types of the operands, after evaluation, are not a valid
// combination for the given operator, according to the rules in B.2 Operator
// Mapping, a type error is raised."
//
// It walks st's {base type definition} chain because B.2's rows are written
// over the types an operand can reach by SUBTYPE SUBSTITUTION, not over its
// own type: "that operator can be applied to an operand of type AT if type AT
// can be converted to type ET by a combination of type promotion and subtype
// substitution". That is how xs:yearMonthDuration reaches eq and ne, for which
// B.2 has an xs:duration row and none of its own.
//
// One chain answers for both operands, and the diagonal answers for every row:
// §3.5.2 converts both operands into the ONE type st, and numeric is B.2's
// only comparison macro expanding to rows whose two types differ — every such
// row's diagonal is in the table beside it.
//
// An unwalkable chain is the error, never a false (STYLE S3); the caller folds
// it into the same err:XPTY0004 direction ctaTypes.comparison gives every
// other type fault.
func (t ctaTypes) admitsComparison(op ctaComparator, st *xsd.SimpleType) (bool, error) {
	for at := st; at != nil; {
		if ctaB2Admits(op, at.Name(), at.Name()) {
			return true, nil
		}
		base, err := at.Base(t.resolver)
		if err != nil {
			return false, err
		}
		at = base
	}
	return false, nil
}

// ctaB2Row is one row of xpath20.md B.2's operator mapping table for one of
// the six comparison operators: the operator, and the LOCAL names of the two
// operand types it admits. The namespace is not carried because B.2 names no
// type outside the XSD namespace in a comparison row (gen_opmap.go).
type ctaB2Row struct {
	op   ctaComparator
	a, b string
}

// ctaB2Admits reports whether B.2 holds a row for op over operand types a and
// b, matched by ·expanded name· and exactly — subtype substitution is the
// CALLER's walk (admitsComparison), because a row admits the types it names
// and the derived types reach it rather than the row reaching them.
func ctaB2Admits(op ctaComparator, a, b xsd.QName) bool {
	if a.Space != xsd.XMLSchemaNS || b.Space != xsd.XMLSchemaNS {
		return false
	}
	for _, row := range ctaB2Comparisons {
		if row.op == op && row.a == a.Local && row.b == b.Local {
			return true
		}
	}
	return false
}

// untypedAgainst is xpath20.md §3.5.2 clause 2: the type an xs:untypedAtomic
// operand is cast to, chosen from the other operand's type T. All four
// sub-clauses are here, in their order.
//
//   - 2.1, "if T is a numeric type or is derived from a numeric type, then V
//     is cast to xs:double" — whatever the numeric type actually is, so a
//     comparison against an xs:integer cast runs in xs:double and not in
//     xs:decimal.
//   - 2.2 and 2.3, the two duration types, which are named rather than reduced
//     to their xs:duration primitive. The Note attached to them says why: "the
//     special treatment of the duration types is required to avoid errors that
//     may arise when comparing the primitive type xs:duration with any
//     duration type."
//   - 2.4, "in all other cases, V is cast to the primitive base type of T".
func (t ctaTypes) untypedAgainst(st *xsd.SimpleType) (*xsd.SimpleType, ctaTyping) {
	p, resolved := t.primitive(st)
	if !resolved {
		return nil, ctaTypeErrored
	}
	if ctaNumeric(p) {
		// GAP(xpath): clause 2.1 answers xs:double for an xs:float-primitive
		// T too, and B.1 rule 1.1 must then promote the T operand ITSELF to
		// xs:double — the promotion ctaWider declines, for the reason argued
		// there and under the same withhold. (#889)
		if p.Name() == ctaBuiltin("float") {
			return nil, ctaTypeDeclined
		}
		return t.double, ctaTypeSettled
	}
	dayTime, err := t.ancestor(st, ctaBuiltin("dayTimeDuration"))
	if err != nil {
		return nil, ctaTypeErrored
	}
	if dayTime != nil {
		return dayTime, ctaTypeSettled
	}
	yearMonth, err := t.ancestor(st, ctaBuiltin("yearMonthDuration"))
	if err != nil {
		return nil, ctaTypeErrored
	}
	if yearMonth != nil {
		return yearMonth, ctaTypeSettled
	}
	return p, ctaTypeSettled
}

// shared is the type serving two TYPED operands: their {primitive type
// definition} where they share one, narrowed to a named duration subtype where
// both carry one (sharedNamed), and otherwise B.1's two promotions — the
// wider of two numeric primitives, and xs:string for an xs:anyURI met by an
// xs:string. Those two are the only ways two DIFFERENT primitives are ever
// admitted, which is the reason a date cast compared against a string literal
// is err:XPTY0004 (the shape §3.5.2's untypedAtomic rules never reach, because
// a StringLiteral is xs:string and so no operand is untyped).
//
// The comparison is by ·expanded name· rather than by component identity: a
// primitive is always named, and a resolver is free to answer with a component
// this compile did not itself resolve.
func (t ctaTypes) shared(a, b *xsd.SimpleType) (*xsd.SimpleType, ctaTyping) {
	pa, resolved := t.primitive(a)
	if !resolved {
		return nil, ctaTypeErrored
	}
	pb, resolved := t.primitive(b)
	if !resolved {
		return nil, ctaTypeErrored
	}
	if pa.Name() == pb.Name() {
		return t.sharedNamed(a, b, pa)
	}
	if ctaNumeric(pa) && ctaNumeric(pb) {
		return ctaWider(pa, pb)
	}
	if ctaStringLike(pa) && ctaStringLike(pb) {
		return t.str, ctaTypeSettled
	}
	return nil, ctaTypeErrored
}

// sharedNamed narrows the primitive p two operands share to the NAMED duration
// subtype both derive from, where there is one, and answers p otherwise.
//
// The narrowing is what keeps B.2's rows reachable, and only the two duration
// subtypes need it: B.2 gives xs:yearMonthDuration and xs:dayTimeDuration the
// four ordering rows and gives their xs:duration primitive none, so reducing
// two xs:yearMonthDuration operands to xs:duration would make a comparison B.2
// admits by subtype substitution into err:XPTY0004. The two are named in
// §3.5.2 clause 2's own rules for the same reason, whose Note states it: "the
// special treatment of the duration types is required to avoid errors that may
// arise when comparing the primitive type xs:duration with any duration type."
//
// A pairing of the two subtypes with EACH OTHER settles on xs:duration, which
// is correct: B.2 has no row relating them and no ordering row for their
// primitive, so eq and ne are admitted through xs:duration and the four
// ordering operators are the type error §3.5.1 raises.
func (t ctaTypes) sharedNamed(a, b, p *xsd.SimpleType) (*xsd.SimpleType, ctaTyping) {
	if p.Name() != ctaBuiltin("duration") {
		return p, ctaTypeSettled
	}
	for _, name := range []xsd.QName{ctaBuiltin("dayTimeDuration"), ctaBuiltin("yearMonthDuration")} {
		at, err := t.ancestor(a, name)
		if err != nil {
			return nil, ctaTypeErrored
		}
		bt, err := t.ancestor(b, name)
		if err != nil {
			return nil, ctaTypeErrored
		}
		if at != nil && bt != nil {
			return at, ctaTypeSettled
		}
	}
	return p, ctaTypeSettled
}

// ctaNumeric reports whether p is one of the three primitives xpath20.md calls
// numeric: xs:decimal, xs:float and xs:double (§B.1's numeric promotions are
// written over exactly these three).
func ctaNumeric(p *xsd.SimpleType) bool {
	return ctaRank(p) >= 0
}

// ctaStringLike reports whether p is xs:anyURI or xs:string, the two
// primitives B.1's URI promotion relates. p need not be primitive:
// ctaCompare.eval asks it of a comparison type, which untypedAgainst may have
// answered as a named duration type, and the name comparison answers false for
// that as for anything else.
func ctaStringLike(p *xsd.SimpleType) bool {
	return p.Name() == ctaBuiltin("string") || p.Name() == ctaBuiltin("anyURI")
}

// ctaWider is the target of B.1's numeric promotions between two different
// numeric primitives: xs:float promotes to xs:double, and xs:decimal to either
// of them, so the wider of the pair is the one both reach.
//
// It answers both a comparison (shared) and an arithmetic operator
// (ctaTypes.arithmetic), whose operation type is this same promotion; the
// latter also asks it of two operands sharing one primitive, which is then
// the answer.
//
// GAP(xpath): the xs:float against xs:double pair is DECLINED rather than
// compared or computed, because reaching it needs B.1 rule 1.1 and this engine cannot
// perform rule 1.1. That rule promotes the xs:float operand to "the xs:double
// value that is the same as the original value" — the same point of the real
// line, not a re-parse of any lexical — and value exposes no such widening:
// over the strict backend an xs:float value and an xs:double value answer Eq
// false and Cmp value.Incomparable, correctly, because they are values of
// different types. ctaPromote's ·canonical representation· round-trip is not
// that widening either: xs:float's canonical is the shortest decimal that
// round-trips to the FLOAT, so reparsing it as an xs:double lands on a
// DIFFERENT xs:double. (Rule 1.2's xs:decimal promotion IS "created by
// casting", so the same round-trip is exactly right for the decimal pairs and
// they stay admitted.) The direction is [CompileCTATest]'s withhold: the
// {test} decides nothing rather than deciding it wrongly. (#889)
func ctaWider(a, b *xsd.SimpleType) (*xsd.SimpleType, ctaTyping) {
	wider, narrower := a, b
	if ctaRank(b) > ctaRank(a) {
		wider, narrower = b, a
	}
	if wider.Name() == ctaBuiltin("double") && narrower.Name() == ctaBuiltin("float") {
		return nil, ctaTypeDeclined
	}
	return wider, ctaTypeSettled
}

// ctaRank orders the three numeric primitives by which promotes to which, and
// reports -1 for a primitive that is not numeric at all.
func ctaRank(p *xsd.SimpleType) int {
	kind, numeric := ctaNumericKindOf(p)
	if !numeric {
		return -1
	}
	return int(kind)
}

// ctaNumericKind is one of the three numeric primitives xpath20.md B.1's
// numeric promotions are written over, in the order they promote:
// xs:decimal to either of the others, and xs:float to xs:double.
type ctaNumericKind byte

const (
	ctaDecimalKind ctaNumericKind = iota
	ctaFloatKind
	ctaDoubleKind
)

// ctaNumericKindOf is the numeric primitive p is, or false where p is none of
// the three.
func ctaNumericKindOf(p *xsd.SimpleType) (ctaNumericKind, bool) {
	switch p.Name() {
	case ctaBuiltin("decimal"):
		return ctaDecimalKind, true
	case ctaBuiltin("float"):
		return ctaFloatKind, true
	case ctaBuiltin("double"):
		return ctaDoubleKind, true
	}
	return 0, false
}
