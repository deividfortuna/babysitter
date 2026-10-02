package remote

import (
	"os"
	"strings"
	"testing"
)

func TestTokenIsMadeOnceAndKept(t *testing.T) {
	dir := t.TempDir()

	first, err := Token(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Token(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || first != second {
		t.Fatalf("Token = %q then %q, want the same token twice", first, second)
	}
	info, err := os.Stat(TokenPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		t.Fatalf("token file mode %v, want only the owner to read it", mode)
	}
}

func TestRotateTokenReplacesTheToken(t *testing.T) {
	dir := t.TempDir()
	old, err := Token(dir)
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := RotateToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	now, err := Token(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rotated == old || now != rotated {
		t.Fatalf("old %q, rotated %q, now %q", old, rotated, now)
	}
}

func TestPairingLinkHoldsTheTokenInTheFragment(t *testing.T) {
	link := PairingLink("studio.local", 7420, "abc")
	if link != "http://studio.local:7420/#token=abc" {
		t.Fatalf("PairingLink = %q", link)
	}
	if strings.Contains(strings.Split(link, "#")[0], "abc") {
		t.Fatalf("the token leaks out of the fragment: %q", link)
	}
}
