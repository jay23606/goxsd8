// Package jsonsrc maps JSON documents onto the validate infoset so JSON
// instances can be assessed against a compiled schema set.
//
// The engine never imports a JSON package; this adapter builds infoset
// values over the stdlib streaming json.Decoder token walk — member order
// preserved, the document never held whole (STYLE P4) — and hands them to
// [validate.Validator.Assess] through [Validate]. Verdicts are the engine's,
// with the same cvc-* rule IDs as for the XML document the JSON maps to; the
// json conformance lane pins the mapping with curated cases
// (conformance/testdata/json).
//
// # The JSON shape in brief
//
// The top-level value is an object with exactly one member, whose key names
// the root element. Below it, each object is one element's attributes and
// children:
//
//   - A key names an attribute where the element's type has an attribute use
//     of that name, and a child element where the type contains a child
//     declaration of it; a key both carry is declined. An unprefixed key
//     matches by local name, and a prefixed key p:local by the namespace its
//     "$xmlns" binds p to.
//   - Attribute members are plain keys spelling the attribute's own name —
//     kind, not "@kind", "_kind" or "-kind" — and lead the object, before
//     every child member. encoding/json marshals a Go struct in field order,
//     so declare its attribute fields before its child fields: a child field
//     declared first yields a document that declines. It marshals a map, such
//     as map[string]any, with its keys sorted, which puts "$type" before
//     "$xmlns" where an object has both, and a child key before an attribute
//     key wherever one sorts first; both orders decline, so marshal a struct,
//     or write the members in order yourself, as a json.Marshaler of your own
//     can.
//   - "$xmlns" and "$type" are reserved and lead the object: a "$xmlns"
//     member, {"prefix":"uri",...}, comes first and binds the prefixes of keys
//     and QName content; a "$type" member, the element's xsi:type, follows
//     it; both precede every other member, and neither is a member of the
//     top-level object.
//   - A scalar is the element's simple content, null is xsi:nil="true", and an
//     array is one child per item, for repeated children.
//   - An input the mapping declines is a [*DeclineError] from [Validate], not
//     a verdict: it returns no [validate.Result].
//
// Against a schema declaring {urn:o}order with an attribute kind and a
// repeated child item, this instance maps to <o:order xmlns:o="urn:o"
// kind="x"><o:item>a</o:item><o:item>b</o:item></o:order>:
//
//	{"o:order":{"$xmlns":{"o":"urn:o"},"kind":"x","item":["a","b"]}}
//
// ExampleValidate, in example_test.go, assesses it beside an instance the
// engine charges and one the mapping declines.
//
// "The validation root" and "The mapping" below state each arm exactly —
// keys no declaration or use carries, wildcards and namespaces included — and
// "Declines" lists every input the mapping refuses.
//
// # The validation root
//
// The top-level value is an object with exactly one member. Its key names the
// validation root, read as every key is ("The mapping"): the top-level element
// declaration ([validate.SchemaView]'s Elements) of the expanded name a
// prefixed key spells, or the one whose local name an unprefixed key is, gives
// the root its expanded name. Where none carries the key, the root is the
// element a prefixed key spells, or, for an unprefixed one, the element of
// that local name in no namespace — the document's own default, as <x> with
// no xmlns has — and has no declaration: the engine assesses it laxly, or
// strictly under the xsi:type a "$type" presents (cvc-assess-elt clauses 1.2
// and 3), and its object's other members decline as every member of an
// element with no declaration does. Its value is the root's, as any member's
// value is its element's. A document holding no JSON value is malformed, not
// declined. A top level of any other shape, an unprefixed key
// top-level declarations of two expanded names carry, and an array root value
// decline, as "Declines" below says. A null root value maps as any member's
// null does, below.
//
// # The mapping
//
// The arms implemented, at every element below the root:
//
//   - A key is unprefixed, a local name, or prefixed, "p:local", which names
//     the expanded name {uri}local, uri the namespace name a "$xmlns" binds p
//     to (below) in scope at the element the key names — its own object's
//     "$xmlns" included, read before the key is classified, and each array
//     item's its own — or, for an attribute, at the element it is an attribute
//     of; "xml" is bound everywhere. A prefixed key is matched by its expanded
//     name, and an unprefixed one by its local name, at every step below: a
//     declaration, use or name it carries is one of that expanded name, or of
//     that local name in whatever namespace. The empty prefix is never read,
//     so no "$xmlns" binding of "" reaches a key, and an unprefixed key never
//     takes a namespace from a binding (Namespaces in XML §6.2 gives an
//     unprefixed attribute name none). Extending "$xmlns" from QName-valued
//     content to names is the mapping's own design: no spec text maps a JSON
//     key to an expanded name, and cvc-resolve-instance resolves content, not
//     names. A key whose colon makes no QName and a prefix no binding in scope
//     binds decline.
//   - A member whose key a child element declaration the enclosing element's
//     type contains carries — directly, indirectly or implicitly,
//     [xsd.Schema.ContainedElementDeclarations] — is that child, under that
//     declaration's expanded name. An object value carries the child's own
//     attributes and children; members map in member order.
//   - A member whose key an attribute use of the enclosing element's type
//     ([xsd.ComplexType.AttributeUses]) carries is that attribute, under the
//     use's {attribute declaration}'s expanded name:
//     {"shipment":{"id":"s1","dest":"Oslo"}} ≡ <shipment
//     id="s1"><dest>Oslo</dest></shipment>. Its value is a scalar, and its
//     lexical form, as below, is the attribute's [[normalized value]]
//     unchanged: a JSON string holds no literal tab, LF or CR (RFC 8259 §7),
//     and an escaped one is kept, as XML 1.0 §3.3.3 keeps a character
//     reference. The empty string is an attribute with an empty value.
//     Attribute members lead the object, before its first child member: the
//     engine reads an element's attributes whole at its entry.
//   - A member whose key neither carries, of a type with a wildcard, is named
//     by a wildcard of the type where the key or the model fixes its namespace, and only
//     there. Element kind: every element wildcard of the type — each Wildcard
//     {term} its content model contains ([xsd.Schema.ContainedWildcards]),
//     whatever its {process contents}, and its {open content} wildcard —
//     enumerates one and the same namespace N, and the key is the child
//     (N,key): {"box":{"w":5}} ≡ <box xmlns="urn:w"><w>5</w></box> under
//     <xs:any namespace="##targetNamespace"/>. It is under the top-level
//     declaration of (N,key) where there is one, and otherwise has no
//     declaration, and its object value then maps only empty, as {}. Attribute
//     kind: the type has no element wildcard and its {attribute wildcard}
//     enumerates the one namespace M, and the key is the attribute (M,key), as
//     an attribute use's member is. A prefixed key fixes its namespace itself:
//     it is the child it spells, or, under an {attribute wildcard} alone, the
//     attribute, whatever namespaces the wildcards enumerate: {"bag":
//     {"$xmlns":{"z":"urn:z"},"z:q":"x"}} ≡ <bag xmlns="urn:w"
//     xmlns:z="urn:z"><z:q>x</z:q></bag>, which the engine charges under a
//     ##local wildcard. The mapping does not test admission: a {disallowed
//     names} entry, ##defined and ##definedSibling are the engine's to charge
//     (cvc-wildcard), and so is a {process contents}, as for the XML twin. A
//     name the wildcards reject is soundly invalid: the input spells it, or
//     every other namespace is outside every wildcard of that kind, and no
//     declaration or use carries the key. A key a declaration or use carries is
//     named by it and never by a wildcard: that is the mapping's own rule, as
//     the NAMESPACE TRAP below says, and no spec text makes it in general; for
//     a prefixed key it only chooses the kind, the declaration's or use's
//     expanded name being the one it spells; it is RULED permanent by #2634
//     because the spec is silent between the two names, and because a decline
//     instead would refuse inputs it maps, the curated json cases wildcard-attribute.v
//     and .i among them. The spec makes the same choice for one item matched both ways,
//     which here is where the wildcard's one namespace is the declaration's or the use's
//     own: the Note under cvc-accept matches an element information item "accepted by
//     either a Wildcard or an Element Declaration" against the Element Declaration, and
//     the Note after cvc-complex-type clause 4 gives an attribute use "whose name and
//     target namespace match" precedence over the {attribute wildcard}. Neither Note
//     chooses between two expanded names, the declaration's or use's and the (N,key) a
//     wildcard would give, nor across kinds, a declaration over an {attribute wildcard}
//     or a use over an element wildcard.
//   - A member whose key neither carries, of a type with no wildcard of either
//     kind — a simple type included — is the child a prefixed key spells, or
//     (O,key), O the enclosing element's own namespace name, under the
//     top-level declaration of that name where there is one and otherwise with
//     none, as a wildcard's child is: {"order":{"size":1,"extra":"x"}} ≡
//     <order xmlns="urn:t"><size>1</size><extra>x</extra></order>. Every XML
//     twin of such a key, of whatever name and kind, is invalid
//     (cvc-complex-content; cvc-complex-type clauses 1.1, 1.2 and 2.2.1;
//     cvc-type clauses 3.1.1 and 3.1.2) but the four xsi attributes
//     cvc-complex-type clause 2 excepts, so the name decides no validity.
//   - A scalar value is the element's simple content: {"size":10} ≡
//     <size>10</size>. Its lexical form is the token's source text — a
//     number's own digits, never a float64 round trip; a string's decoded
//     characters; true or false — and the empty string is no text at all,
//     as in <size/>.
//   - A null value is an element whose one attribute is xsi:nil, in the XSI
//     namespace with the lexical form true, and which has no [[children]]:
//     {"qty":null} ≡ <qty xsi:nil="true"/>. The mapping reads no {nillable}:
//     whether the element is ·nilled· and cvc-elt clause 3 are the engine's,
//     which charges clause 3.1 where the declaration is not nillable and
//     3.2.3.2 where it carries a fixed {value constraint}, as for the XML
//     twin. An empty element without the xsi:nil would be a mis-map: cvc-elt
//     clause 5.1 would give it the {value constraint}. An object value is
//     never ·nilled·, {} included.
//   - An array value is one child per item under the member's key, in item
//     order: an object, scalar or null item is a child as a member value
//     would be, the key classified over each item in turn, and an empty array
//     is no child, its key classified in the parent's scope over none.
//   - A "$xmlns" member leading an object binds prefixes for the element and
//     its descendants: its value is an object of strings, {prefix: uri}, the
//     bindings of an xmlns:prefix="uri" on the element. Each element's
//     bindings are its own object's "$xmlns", else its parent's, through an
//     immutable parent-linked scope chain, the innermost binding of a prefix
//     winning. They are what QName-valued content, "$type" and a prefixed key
//     resolve against: {"names":{"$xmlns":{"t":"urn:t"},"q":"t:foo"}} ≡ <names
//     xmlns:t="urn:t"><q>t:foo</q></names>, and {"t:names":{"$xmlns":
//     {"t":"urn:t"}}} ≡ <t:names xmlns:t="urn:t"/>.
//   - A "$type" member, after any "$xmlns" and before every other member, is
//     the element's xsi:type, its string value the attribute's [[normalized
//     value]]: {"item":{"$type":"t:Derived",…}} ≡ <item
//     xsi:type="t:Derived">…</item>. The engine resolves it and charges
//     cvc-attribute (clause 3 or 5) where it names no type, and cvc-elt clause
//     4 where the type it names does not ·override· the one a ·governing
//     element declaration· supplied. The members after it are classified under
//     the type that governs the element wherever its verdict stands
//     uncharged: where the type it names resolves, that type where it
//     ·overrides· the element's ·locally declared type· ·without limitation·,
//     or where the element has none, and the ·locally declared type· where it
//     does not and an {open content} may take the element, which leaves it no
//     ·governing element declaration· to charge cvc-elt clause 4 against
//     (key-governing-type-elem clauses 6 to 8), and the type it names again
//     where every reading charges the element; and the declared type where it
//     names none. That is read under the parent's ·governing type definition·,
//     which the mapping determines as the engine does, from the type the
//     parent's own ·attribution· gives it — its parent's content model run
//     over the siblings before it ([xsd.Schema.ContentMatcher]) — and not
//     from the declaration its members are classified under; the root takes
//     the type it names, which ·overrides· its declaration's or is charged.
//     Under an element with no declaration the members decline as every
//     member there does. The type is resolved through the engine's own
//     resolver (cvc-resolve-instance, §3.17.6.3), so the two agree on it. A
//     "$type" rides an object value only: a scalar or null element carries no
//     xsi:type. On an element with a declaration it declines where the
//     mapping determines no type governing the parent, and where, below the
//     root, the types a {type table} can select for the parent settle it
//     differently (Declines).
//   - An object value under a declaration carrying a {type table} is
//     classified under each type it may be assessed under: the declaration's
//     {type definition} and every type the table can select — each
//     alternative's {type definition} and the {default type definition}'s.
//     At the validation root, which has no [inherited attributes]
//     (key-p-inherited), the mapping reads the ·selected type definition·
//     itself (key-selected-type clause 1, key-cta-select), through the
//     engine's own selection, validate.Validator.ConditionallySelected: a
//     member is an attribute only where the type the element is assessed under
//     takes it as one, and the attributes decide that type, so the leading
//     run of attribute members ends at the first member that the type the
//     attributes before it select classifies as no attribute, having
//     classified those as the run did; the members after it are classified
//     under that type alone, which governs the element. Where no run is so
//     settled, where one candidate type would carry the run on past the
//     member the selected type ends it at, and where the engine withholds the
//     selection while the candidate types classify the members differently,
//     the root's members decline (Declines); where it withholds the selection
//     and every candidate classifies them alike, they map as below the root.
//     Below the root each member maps only where every candidate classifies
//     it alike: an attribute of one expanded name; a child of one expanded
//     name whose value is a scalar or null, whatever declarations it is under,
//     its element being built from its name alone; and, under any other value,
//     a declaration-less child of one expanded name, or a child under
//     declarations of one expanded name and one type — of one name, or an
//     anonymous complex type of one declaration's {context} — neither carrying
//     a {type table}; the engine selects the type from the attributes the
//     mapping presents, xsi:type included, and the infoset is the one any
//     candidate gives. Its "$type" arm reads no table, the type it names
//     ·overriding· the selected one or being charged cvc-elt clause 4. A
//     "$type" on its child or below it is settled under the type the root's
//     table selects, and below the root under every type the table can select,
//     its children ·attributed· under each, so it maps where every one settles
//     on one type; where every type the table can select is one named type,
//     that type governs the element. A scalar or null value classifies no
//     member.
//   - No DTD-shaped declaration exists in JSON, so the adapter does not
//     implement [validate.UnparsedEntities] and every ·ENTITY value· fails
//     cvc-simple-type clause 3, as that interface's doc states.
//
// The element's default (empty-prefix) namespace is its own namespace name —
// its declaration's target namespace, the one its prefixed key spells, the N
// a wildcard gives it, or the O its parent has — and deliberately shadows any
// "$xmlns" binding of "": an unprefixed QName, in content or in "$type", takes
// that namespace, and no binding of "" ever names a key. "xml" is bound; any
// other prefix is bound by a "$xmlns" in scope or not at all.
//
// Three shapes are not expressible, and the mapping does not reorder to reach
// the first two. The ORDER TRAP: an array's items are contiguous repeats, so
// a content model interleaving repeats (a, b, a) is reached only through
// duplicate keys, which map in member order like any other members. The
// NAMESPACE TRAP: an unprefixed key is a local name, while cvc-accept and
// cvc-complex-type clause 2 match expanded names; its namespace comes from the
// matched declaration or use, from the one namespace a wildcard enumerates,
// under a type with no wildcard from the enclosing element, and at the root
// from none, so an unprefixed key two expanded names carry — among the
// top-level declarations, among one content model's declarations, or among
// one type's attribute uses — declines, and so does an unprefixed
// wildcard-only key whose wildcards admit more than one namespace. A prefixed
// key spells its namespace and escapes the trap, except that no prefix is
// bound to the absent namespace, so no key spells a declaration or use in no
// namespace beside one of its local name in a namespace. The NILLED
// ATTRIBUTES shape: a null carries no members, so no input spells a ·nilled·
// element with other attributes, <e a="1" xsi:nil="true"/>; this is a limit of
// the notation, not a decline. A member with no "$type" is classified under
// its matched declaration's type, which need not be the child's ·governing
// type definition·: where the engine attributes the child to a wildcard
// instead, it is governed through the top-level declaration the child's name
// resolves to, for one, or, under an {open content}, the ·locally declared
// type· a base's declaration of the name gives it — the type a "$type" below
// it is settled under — and the engine assesses the infoset built under the
// type the mapping classified by, attributes included; where its declaration
// carries a {type table}, it is governed by the type the table selects, under
// which its members are classified at the root and alike below it ("The
// mapping"). One with a "$type" is classified under the type that governs it
// wherever its verdict stands uncharged, as "The mapping" says.
//
// # Declines
//
// Each decline, listed in full below, and the change to the input or the
// schema that avoids it:
//
//	top level not {"<root>":value}        one member, keyed by the root's name
//	root value an array                   an object, a scalar or null
//	key whose colon makes no QName        local or p:local
//	prefix no "$xmlns" in scope binds     bind it in a "$xmlns" in scope
//	unprefixed key two names carry        prefix it; for a name in no namespace
//	                                        beside one in a namespace, none: a
//	                                        permanent limitation
//	wildcard-only key, namespace unfixed  prefix it
//	wildcard-only key, kind unfixed       none: a permanent limitation
//	key in the xsi or xml namespace       null for xsi:nil, "$type" for
//	                                        xsi:type; prefix it xml: for an
//	                                        attribute under an {attribute
//	                                        wildcard} alone; else none: a
//	                                        permanent limitation
//	member under an undeclared child      declare the child
//	key a declaration and a use carry     none: a permanent limitation
//	attribute member after a child        move attribute members first
//	attribute member repeated             give it once
//	attribute member null, object, array  give it a scalar
//	attribute use in the xsi namespace    null for xsi:nil, "$type" for
//	                                        xsi:type; else none: a permanent
//	                                        limitation
//	attribute on simple content           none: a permanent limitation
//	"$xmlns" or "$type" misplaced,        "$xmlns" then "$type", once each,
//	  repeated or in the top level          leading the element's own object
//	"$type" or "$xmlns" value malformed   a string "$type"; a "$xmlns" binding
//	                                        each NCName prefix once, not xml or
//	                                        xmlns, to a non-empty name
//	"$type" a {type table} settles        none in the input; an open gap
//	  differently, or under a parent
//	  whose type is undetermined
//	other "$" member                      key it by its own name, with no "$"
//	array item that is an array           an object, a scalar or null item
//	member a {type table}'s types         none in the input; an open gap
//	  classify differently
//	unresolved {type definition}          none: a permanent limitation
//
// An input the implemented arms do not map is a [*DeclineError] from
// [Validate], which [errors.As] matches, naming the arm it is not — never a
// silent drop and never a guessed mapping. Malformed or absent JSON, content
// after the top-level object and a fault reading the input are no declines,
// and no error but a decline is a DeclineError. Each entry below whose XML
// twin the engine assesses is tracked by a GAP(jsonsrc) marker at the function
// that declines it — rootMember, childNamed, useNamed, wildcardMember,
// classify, selection, attributes, extend, settled, members.key, reserved,
// instanceGoverned and declaredType; one more at classify tracks the
// mapping's own choice of a declaration or use over a wildcard, under "The
// mapping". A repeated attribute member, an attribute member whose value is
// null, an object or an array, an array item that is itself an array, a key
// whose colon makes no QName, a prefix no "$xmlns" in scope binds, a repeated
// "$xmlns" or "$type", a reserved member other than those two or at the top
// level, the malformed "$xmlns" and "$type" values below, and the top-level
// shapes and array root value that "The validation root" declines have no XML
// twin, and no marker; the error for each names the spelling the mapping takes
// instead, where one exists:
//
//   - an unprefixed key two expanded names carry: among the top-level
//     declarations at the root, among the declarations a type contains, or
//     among its attribute uses. RULED permanent by #2623: the verdict turns on
//     which expanded name the key is given, a prefixed key "p:local" spells
//     each one in a namespace, and one in no namespace beside one in a
//     namespace is spelled by no key, since no prefix is bound to the absent
//     namespace, which the mapping declines rather than give the unprefixed
//     key either meaning;
//   - a key no contained declaration and no attribute use carries, where the
//     key and the type's wildcards do not fix its namespace or kind: for an
//     unprefixed key, a wildcard of variety any or not (xs:anyType's included),
//     or several enumerated namespaces; a key a wildcard's single namespace, of
//     either kind, puts in the xsi or the xml namespace, and a prefixed key no
//     declaration or use carries that spells a name in the xsi namespace, or a
//     child element's in the xml namespace; a key of a type with both an
//     element wildcard and an {attribute wildcard}, whose kind the key does not
//     say, prefixed or not; and a member of an object under a child no
//     contained and no top-level declaration names. The xsi- and xml-namespace
//     class is RULED permanent by #2635: an element in the xml namespace would
//     bind the default namespace to it, which no XML twin has, and the
//     attribute kind is ruled with it; the four xsi attributes cvc-complex-type
//     clause 2 excepts must keep declining, xsi:nil and xsi:type having their
//     own spellings, and the rest of the xsi namespace is not worth a half-arm;
//     a prefixed key in the xml namespace under a type with only an {attribute
//     wildcard} maps, to the attribute it spells, which binds no default
//     namespace. The others are RULED permanent by #2634, and a prefix lifts
//     the namespace ones but not the kind ones: whichever (N,key) the mapping
//     chose, its verdict depends on N through declaration lookup, attribution
//     and an ancestor's identity-constraint paths and assertions. Conditions
//     under which the verdict is the same for every admitted N exist — no
//     {disallowed names} entry of the key's local name and ##defined and
//     ##definedSibling inert for it, skip or no global declaration of it in an
//     admitted namespace, one particle accepting every (N,key), no
//     ancestor-or-self assertion, identity constraint or {type table} — and
//     they are unbuilt by decision, not by oversight: testing the last needs a
//     census of every ancestor, which the walker does not keep;
//   - a key both a contained declaration and an attribute use carry, which no
//     spec rule decides between;
//   - an attribute member after a child member, a repeated attribute member,
//     and an attribute member whose value is null, an object or an array;
//   - an attribute use in the xsi namespace: xsi:nil is spelled by a null
//     value alone, xsi:type by a "$type" member alone, and
//     xsi:schemaLocation and xsi:noNamespaceSchemaLocation, schema-location
//     hints (§2.7.3, §4.3.2) a JSON instance carries none of, are RULED
//     permanent by #2627;
//   - an attribute member of an element whose type has simple content, a
//     use's or one its {attribute wildcard} alone admits: the mapping spells
//     no text beside attribute members, so an object value carries no simple
//     content and a scalar value no attribute;
//   - a "$xmlns" or "$type" out of its place — "$type" before "$xmlns",
//     either repeated, either after another member, either as the top-level
//     object's member — which no spec text orders, so none is charged; a
//     "$type" whose value is not a string; a "$xmlns" whose value is not an
//     object of strings, which binds a non-empty prefix that is no NCName (no
//     xmlns:prefix attribute declares one), binds a prefix twice, binds "xml"
//     or "xmlns", or binds a non-empty prefix to "";
//   - a "$type" naming a type, below the root, that the types a {type table}
//     of its parent's declaration, or of an ancestor's below the root, can
//     select settle on different types: below the root the mapping reads no
//     ·selected type definition·, computing no [inherited attributes] for a
//     {test} to read, so it maps only what settles alike whichever type is
//     selected; and one whose parent's ·governing type definition· the mapping does not
//     determine, where [xsd.Schema.ContentMatcher] declines the content model
//     the parent is attributed by, whose engine twin is walked undecided;
//   - any other "$"-prefixed reserved member, "$" being no XML NameStartChar;
//   - an array item that is itself an array;
//   - a member of an object under a declaration carrying a {type table}, below
//     the root, that the declaration's {type definition} and a type the table
//     can select classify differently, or that one of them declines: there the
//     mapping reads no ·selected type definition·, so it maps only what
//     classifies alike whichever type is selected; and, at the root, a member
//     of the attribute run any candidate type still reading attributes
//     declines, though the type the table selects may map it, since the
//     selection reads the whole run and the mapping drops no candidate on a
//     member it declines (#2703); one the candidates still reading attributes
//     take as attributes of different expanded names, one a candidate takes as
//     an attribute where the type the attributes before it select ends the run
//     on it, members no run of attributes settles, and members the candidate
//     types classify differently where the engine withholds the selection, a
//     {test} it reaches being outside its compiler's §3.12.6 subset;
//   - a {type definition} that does not resolve, under an object value.
//
// # Locations
//
// Every Loc is a line and a byte column from 1, at the member's key for an
// element (at the item for an array's item), for an attribute member and for
// a "$type" member's xsi:type, at the scalar token for its text, and at the
// null token for a null value's xsi:nil, which has no key of its own.
package jsonsrc
