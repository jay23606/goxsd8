package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/loader"
)

// TestPinnedResolverServesXLink pins what pinnedResolver serves for
// xlinkLocation: the committed copy, byte for byte as its doc records, under
// xlinkPinned as its resolved location.
func TestPinnedResolverServesXLink(t *testing.T) {
	const wantSHA = "c83df86c7fdc16eb9c862b83dfb53fc1b1a4bcafd6e1d1217199e0188b82f24a"
	r := pinnedResolver{dir: loader.Dir(t.TempDir())}
	rc, resolved, err := r.Resolve("http://www.w3.org/1999/xlink", xlinkLocation)
	if err != nil {
		t.Fatalf("Resolve(%q): %v", xlinkLocation, err)
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("reading %q: %v", xlinkLocation, err)
	}
	sum := sha256.Sum256(b)
	if got := hex.EncodeToString(sum[:]); len(b) != 9386 || got != wantSHA {
		t.Errorf("served %d bytes, sha256 %s; want 9386 bytes, sha256 %s", len(b), got, wantSHA)
	}
	if resolved != xlinkPinned {
		t.Errorf("resolved = %q, want %q", resolved, xlinkPinned)
	}
}

// TestPinnedResolverDelegates pins that every other location reaches the
// wrapped loader.Dir, the case's own directory.
func TestPinnedResolverDelegates(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "s.xsd"), []byte("<s/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := pinnedResolver{dir: loader.Dir(dir)}
	rc, resolved, err := r.Resolve("", "s.xsd")
	if err != nil {
		t.Fatalf("Resolve(s.xsd): %v", err)
	}
	_ = rc.Close()
	if want := filepath.Join(dir, "s.xsd"); resolved != want {
		t.Errorf("resolved = %q, want %q", resolved, want)
	}
}

// TestAssembleCaseResolvesXLinkImport pins the xsts.xsd shape end to end: an
// <import> of the XLink namespace from xlinkLocation, and references to
// xlink:type and xlink:href (src-resolve clause 1.2), assemble and are decided.
// Without pinnedResolver the import is unfollowed, the references fail
// src-resolve, and fabricatedRejection declines the case.
func TestAssembleCaseResolvesXLinkImport(t *testing.T) {
	dir := t.TempDir()
	const src = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
  xmlns:xlink="http://www.w3.org/1999/xlink">
  <xs:import namespace="http://www.w3.org/1999/xlink"
    schemaLocation="http://www.w3.org/XML/2008/06/xlink.xsd"/>
  <xs:element name="ref">
    <xs:complexType>
      <xs:attribute ref="xlink:type" default="locator"/>
      <xs:attribute ref="xlink:href"/>
    </xs:complexType>
  </xs:element>
</xs:schema>`
	doc := filepath.Join(dir, "s.xsd")
	if err := os.WriteFile(doc, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, decidable, perr := assembleCase(strict.New(), doc, nil)
	if !decidable || perr != nil {
		t.Fatalf("assembleCase = decidable %v, error %v; want decidable, nil", decidable, perr)
	}
}
