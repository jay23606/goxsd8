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
// # The validation root
//
// The top-level value is an object with exactly one member. Its key names the
// validation root: it is matched by local name against the top-level element
// declarations ([validate.SchemaView]'s Elements), exactly one of which must
// carry it, and that declaration's expanded name is the root's. Its value is
// the root's, as any member's value is its element's. A top level of any other
// shape, a key no top-level declaration or several carry, and an array root
// value decline: there is no no-namespace root to fall back to; the two key
// declines are tracked as "Declines" below says. A null root
// value maps as any member's null does, below.
//
// # The mapping
//
// The arms implemented, at every element below the root:
//
//   - A member whose key is the local name of a child element declaration
//     the enclosing element's type contains — directly, indirectly or
//     implicitly, [xsd.Schema.ContainedElementDeclarations] — is that child,
//     under that declaration's expanded name. An object value carries the
//     child's own attributes and children; members map in member order.
//   - A member whose key is the local name of an attribute use of the
//     enclosing element's type ([xsd.ComplexType.AttributeUses]) is that
//     attribute, under the use's {attribute declaration}'s expanded name:
//     {"shipment":{"id":"s1","dest":"Oslo"}} ≡ <shipment
//     id="s1"><dest>Oslo</dest></shipment>. Its value is a scalar, and its
//     lexical form, as below, is the attribute's [[normalized value]]
//     unchanged: a JSON string holds no literal tab, LF or CR (RFC 8259 §7),
//     and an escaped one is kept, as XML 1.0 §3.3.3 keeps a character
//     reference. The empty string is an attribute with an empty value.
//     Attribute members lead the object, before its first child member: the
//     engine reads an element's attributes whole at its entry.
//   - A member whose key neither carries, of a type with a wildcard, is named
//     by a wildcard of the type where the model fixes its namespace, and only
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
//     an attribute use's member is. The mapping does not test admission: a
//     {disallowed names} entry, ##defined and ##definedSibling are the engine's
//     to charge (cvc-wildcard), and so is a {process contents}, as for the XML
//     twin. A name the wildcards reject is soundly invalid: every other
//     namespace is outside every wildcard of that kind, and no declaration or
//     use carries the key. A key a declaration or use carries is named by it
//     and never by a wildcard: that is the mapping's own rule, as the NAMESPACE
//     TRAP below says, and no spec text makes it in general; it is RULED
//     permanent by #2634 because the spec is silent between the two names, and
//     because a decline instead would refuse inputs it maps, the curated json
//     cases wildcard-attribute.v and .i among them. The spec makes the same
//     choice for one item matched both ways, which here is where the wildcard's
//     one namespace is the declaration's or the use's own: the Note under
//     cvc-accept matches an element information item "accepted by either a
//     Wildcard or an Element Declaration" against the Element Declaration, and
//     the Note after cvc-complex-type clause 4 gives an attribute use "whose
//     name and target namespace match" precedence over the {attribute
//     wildcard}. Neither Note chooses between two expanded names, the
//     declaration's or use's and the (N,key) a wildcard would give, nor across
//     kinds, a declaration over an {attribute wildcard} or a use over an
//     element wildcard.
//   - A member whose key neither carries, of a type with no wildcard of either
//     kind — a simple type included — is the child (O,key), O the enclosing
//     element's own namespace name, under the top-level declaration of that
//     name where there is one and otherwise with none, as a wildcard's child
//     is: {"order":{"size":1,"extra":"x"}} ≡ <order
//     xmlns="urn:t"><size>1</size><extra>x</extra></order>. Every XML twin of
//     such a key, of whatever name and kind, is invalid (cvc-complex-content;
//     cvc-complex-type clauses 1.1, 1.2 and 2.2.1; cvc-type clauses 3.1.1 and
//     3.1.2) but the four xsi attributes cvc-complex-type clause 2 excepts, so
//     the name decides no validity.
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
//     would be, and an empty array is no child.
//
// The element's default (empty-prefix) namespace is its own namespace name —
// its declaration's target namespace, the N a wildcard gives it, or the O its
// parent has — and "xml" is bound; no other prefix is.
//
// Three shapes are not expressible, and the mapping does not reorder to reach
// the first two. The ORDER TRAP: an array's items are contiguous repeats, so
// a content model interleaving repeats (a, b, a) is reached only through
// duplicate keys, which map in member order like any other members. The
// NAMESPACE TRAP: a key is a local name, while cvc-accept and cvc-complex-type
// clause 2 match expanded names; the namespace comes from the matched
// declaration or use, from the one namespace a wildcard enumerates, or, under
// a type with no wildcard, from the enclosing element, so a
// key two expanded names carry — among one content model's declarations, or
// among one type's attribute uses — declines, and so does a wildcard-only key
// whose wildcards admit more than one namespace. The NILLED ATTRIBUTES shape:
// a null carries no members, so no input spells a ·nilled· element with other
// attributes, <e a="1" xsi:nil="true"/>; this is a limit of the notation, not
// a decline. A member is classified under its matched declaration's type,
// which need not be the child's ·governing type definition·: where the engine
// attributes the child to a wildcard instead, that type is the engine's to
// determine — through the top-level declaration the child's name resolves to,
// for one, or, under an {open content}, the ·locally declared type· a base's
// declaration of the name gives it — and it assesses the infoset built under
// the matched declaration, attributes included.
//
// # Declines
//
// An input the implemented arms do not map is an error from [Validate]
// naming the arm it is not — never a silent drop and never a guessed mapping.
// Each entry below whose XML twin the engine assesses is tracked by a
// GAP(jsonsrc) marker at the function that declines it — rootDeclaration,
// wildcardMember, classify, members.key, useNamed, reserved, typeTable and
// object — and so are the NAMESPACE TRAP's declines among a type's child
// declarations and among its attribute uses, at childNamed and useNamed; one
// more at classify tracks the mapping's own choice of a declaration or use
// over a wildcard, under "The mapping". A repeated attribute member, an
// attribute member whose value is null, an object or an array, an array item
// that is itself an array, and the top-level shapes and array root value that
// "The validation root" declines have no XML twin, and no marker:
//
//   - a root key no top-level declaration carries, or that top-level
//     declarations of two expanded names carry;
//   - a key no contained declaration and no attribute use carries, where the
//     type's wildcards do not fix its namespace: a wildcard of variety any or
//     not (xs:anyType's included), or several enumerated namespaces; a key a
//     wildcard's single namespace, of either kind, puts in the xsi or the xml
//     namespace; a key of a type with both an element wildcard and an
//     {attribute wildcard}, whose kind the key does not say; and a member of
//     an object under a child no contained and no top-level declaration
//     names. The xsi- and xml-namespace class is RULED permanent by #2635: an
//     element in the xml namespace would bind the default namespace to it,
//     which no XML twin has, and the attribute kind is ruled with it; the four
//     xsi attributes cvc-complex-type clause 2 excepts must keep declining,
//     xsi:nil and xsi:type having their own spellings, and the rest of the xsi
//     namespace is not worth a half-arm. The others are RULED permanent by
//     #2634: whichever (N,key) the mapping chose, its verdict depends on N
//     through declaration lookup, attribution and an ancestor's
//     identity-constraint paths and assertions. Conditions under which the
//     verdict is the same for every admitted N exist — no {disallowed names}
//     entry of the key's local name and ##defined and ##definedSibling
//     inert for it, skip or no global declaration of it in an admitted
//     namespace, one particle accepting every (N,key), no ancestor-or-self
//     assertion, identity constraint or {type table} — and they are unbuilt by
//     decision, not by oversight: testing the last needs a census of every
//     ancestor, which the walker does not keep;
//   - a key both a contained declaration and an attribute use carry, which no
//     spec rule decides between;
//   - an attribute member after a child member, a repeated attribute member,
//     and an attribute member whose value is null, an object or an array;
//   - an attribute use in the xsi namespace: xsi:nil is spelled by a null
//     value alone, xsi:type is the planned "$type" arm's below, and
//     xsi:schemaLocation and xsi:noNamespaceSchemaLocation, schema-location
//     hints (§2.7.3, §4.3.2) a JSON instance carries none of, are RULED
//     permanent by #2627;
//   - an attribute member of an element whose type has simple content, a
//     use's or one its {attribute wildcard} alone admits: the mapping spells
//     no text beside attribute members, so an object value carries no simple
//     content and a scalar value no attribute;
//   - a "$"-prefixed reserved member: "$type" and "$xmlns" are the planned
//     arms below, and no other "$" name has an XML twin, "$" being no XML
//     NameStartChar;
//   - an array item that is itself an array;
//   - a matched declaration carrying a {type table}, the top-level one of a
//     child a wildcard or a wildcard-free type names included: which type
//     governs its element turns on its attributes, the xsi: ones included,
//     which this mapping does not all present;
//   - a {type definition} that does not resolve, under an object value.
//
// # Locations
//
// Every Loc is a line and a byte column from 1, at the member's key for an
// element (at the item for an array's item) and for an attribute member, at
// the scalar token for its text, and at the null token for a null value's
// xsi:nil, which has no key of its own.
//
// # Planned arms (later M8 slices — not yet implemented)
//
//   - Reserved members: "$type" ⇒ xsi:type; "$xmlns" ⇒ {prefix: uri}
//     bindings for QName-valued content, inherited by descendants through an
//     immutable parent-linked scope chain; the element's default
//     (empty-prefix) namespace stays its own namespace name and deliberately
//     shadows any "$xmlns" override of "".
package jsonsrc
