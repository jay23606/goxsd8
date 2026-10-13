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
// shape, a key no top-level declaration or several carry, and an array or null
// root value decline: there is no no-namespace root to fall back to.
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
//   - A scalar value is the element's simple content: {"size":10} ≡
//     <size>10</size>. Its lexical form is the token's source text — a
//     number's own digits, never a float64 round trip; a string's decoded
//     characters; true or false — and the empty string is no text at all,
//     as in <size/>.
//   - An array value is one child per item under the member's key, in item
//     order: an object or scalar item is a child as a member value would be,
//     and an empty array is no child.
//
// The element's default (empty-prefix) namespace is its declaration's target
// namespace, and "xml" is bound; no other prefix is.
//
// Two shapes are not expressible, and the mapping does not reorder to reach
// them. The ORDER TRAP: an array's items are contiguous repeats, so a content
// model interleaving repeats (a, b, a) is reached only through duplicate keys,
// which map in member order like any other members. The NAMESPACE TRAP: a key
// is a local name, while cvc-accept and cvc-complex-type clause 2 match
// expanded names; the namespace comes from the matched declaration or use, so
// a key two expanded names carry — among one content model's declarations, or
// among one type's attribute uses — declines. A member is classified under its
// matched declaration's type, which need not be the child's ·governing type
// definition·: where the engine attributes the child to a wildcard instead,
// that type is the engine's to determine — through the top-level declaration
// the child's name resolves to, for one — and it assesses the infoset built
// under the matched declaration, attributes included.
//
// # Declines
//
// An input the implemented arms do not map is an error from [Validate]
// naming the arm it is not — never a silent drop and never a guessed mapping.
// The GAP(jsonsrc) markers at classify, useNamed and members.key track the
// first, second, fourth and fifth entries, the attribute member after a child
// member in the third, and the attribute-use half of the NAMESPACE TRAP:
//
//   - a key no contained declaration and no attribute use carries, which could
//     be a wildcard's element or attribute; a key an element wildcard or an
//     {attribute wildcard} would admit declines with it;
//   - a key both a contained declaration and an attribute use carry, which no
//     spec rule decides between;
//   - an attribute member after a child member, a repeated attribute member,
//     and an attribute member whose value is null, an object or an array;
//   - an attribute use in the xsi namespace, which the null and "$type" arms
//     below are to present;
//   - an attribute member of an element whose type has simple content: the
//     mapping spells no text beside attribute members, so an object value
//     carries no simple content and a scalar value no attribute;
//   - a "$"-prefixed reserved member;
//   - a null, which would be xsi:nil — an empty element would be a mis-map
//     under cvc-elt clause 5.1;
//   - an array item that is itself an array;
//   - a matched declaration carrying a {type table}: which type governs its
//     element turns on its attributes, the xsi: ones included, which this
//     mapping does not all present;
//   - a {type definition} that does not resolve, under an object value.
//
// # Locations
//
// Every Loc is a line and a byte column from 1, at the member's key for an
// element (at the item for an array's item) and for an attribute, and at the
// scalar token for its text.
//
// # Planned arms (later M8 slices — not yet implemented)
//
//   - JSON null ⇒ xsi:nil="true" (nillability itself stays the engine's
//     cvc-elt concern).
//   - Reserved members: "$type" ⇒ xsi:type; "$xmlns" ⇒ {prefix: uri}
//     bindings for QName-valued content, inherited by descendants through an
//     immutable parent-linked scope chain; the element's default
//     (empty-prefix) namespace stays its matched declaration's target
//     namespace and deliberately shadows any "$xmlns" override of "".
package jsonsrc
