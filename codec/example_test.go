package codec_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/codec"
	"github.com/kud360/goxsd8/codec/testdata/readme/order"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
)

// ExampleDecode parses README's order.xsd, decodes README's order.xml into
// the Order codegen emits for it, then decodes an instance the schema rejects
// and prints each of its violations by its Loc and rule.
func ExampleDecode() {
	readme := filepath.Join("..", "testdata", "readme")
	schema, err := parser.Parse("order.xsd", parser.WithResolver(loader.Dir(readme)))
	if err != nil {
		log.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(readme, "order.xml"))
	if err != nil {
		log.Fatal(err)
	}
	o, err := codec.Decode[order.Order](schema, strict.New(), data, codec.WithURI("order.xml"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(o.Item, *o.Kind)

	invalid := []byte(`<o:order xmlns:o="urn:o" kind="x" size="2">
  <o:note>n</o:note>
</o:order>`)
	_, err = codec.Decode[order.Order](schema, strict.New(), invalid, codec.WithURI("order.xml"))
	// Every violation, as Decode's item 3 reaches them.
	for _, v := range codec.ViolationsOf(err) {
		fmt.Println(v.Loc, v.Rule)
	}
	// Output:
	// [a b] x
	// order.xml:1:1 cvc-complex-type
	// order.xml:2:3 cvc-complex-content
}
