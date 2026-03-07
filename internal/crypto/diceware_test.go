package crypto

import (
	"strings"
	"testing"
)

func TestGeneratePassphrase(t *testing.T) {
	p, err := GeneratePassphrase(4)
	if err != nil {
		t.Fatalf("GeneratePassphrase: %v", err)
	}

	words := strings.Split(p, "-")
	if len(words) != 4 {
		t.Fatalf("expected 4 words, got %d: %q", len(words), p)
	}

	for _, w := range words {
		if len(w) == 0 {
			t.Fatalf("empty word in passphrase: %q", p)
		}
	}
}

func TestGeneratePassphraseUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		p, err := GeneratePassphrase(4)
		if err != nil {
			t.Fatalf("GeneratePassphrase: %v", err)
		}
		if seen[p] {
			t.Fatalf("duplicate passphrase generated: %q", p)
		}
		seen[p] = true
	}
}

func TestGeneratePassphraseWordCount(t *testing.T) {
	for _, n := range []int{1, 2, 3, 5, 6} {
		p, err := GeneratePassphrase(n)
		if err != nil {
			t.Fatalf("GeneratePassphrase(%d): %v", n, err)
		}
		words := strings.Split(p, "-")
		if len(words) != n {
			t.Fatalf("expected %d words, got %d: %q", n, len(words), p)
		}
	}
}

func TestGeneratePassphraseDefaultCount(t *testing.T) {
	p, err := GeneratePassphrase(0)
	if err != nil {
		t.Fatalf("GeneratePassphrase(0): %v", err)
	}
	words := strings.Split(p, "-")
	if len(words) != 4 {
		t.Fatalf("expected 4 words for default, got %d", len(words))
	}
}
