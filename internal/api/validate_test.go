package api

import (
	"net/http"
	"testing"
	"unicode/utf8"
)

// Review item D-20: limits on text fields are in characters, so multi-byte
// text (Turkish ç ş ğ ı İ ö ü are two bytes each) is neither cut early nor
// split mid-character.
func TestMaxLenCountsCharacters(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"abc", 5, "abc"},
		{"abcdef", 3, "abc"},
		{"ğğğ", 3, "ğğğ"},
		{"çşğıİöü", 3, "çşğ"},
		{"İstanbul Şişli Mecidiyeköy Büyükdere Çağlşyan Ö", 50, "İstanbul Şişli Mecidiyeköy Büyükdere Çağlşyan Ö"},
		{"", 3, ""},
	}
	for _, tc := range cases {
		got := maxLen(tc.in, tc.n)
		if got != tc.want {
			t.Errorf("maxLen(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}

// The limit seen through the API: a 47-character Turkish location (57 bytes)
// is stored whole under the 50 limit, and a longer one is cut to exactly 50
// characters of valid UTF-8.
func TestTagValueLimitCountsCharacters(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "admin", "adminpass1234", "admin")
	client := loginAs(t, ts, "admin", "adminpass1234")

	create := func(value string) string {
		t.Helper()
		resp := doJSON(t, client, http.MethodPost, ts.URL+"/api/tags", map[string]string{"group": "location", "value": value})
		if resp.StatusCode != http.StatusCreated {
			resp.Body.Close()
			t.Fatalf("create location %q: status %d", value, resp.StatusCode)
		}
		return parseJSON(t, resp)["value"].(string)
	}

	const fits = "İstanbul Şişli Mecidiyeköy Büyükdere Çağlşyan Ö"
	if got := create(fits); got != fits {
		t.Errorf("47-character value stored as %q, want it unchanged", got)
	}

	long := "Şişli Şişli Şişli Şişli Şişli Şişli Şişli Şişli Şişli Şişli" // 59 characters
	got := create(long)
	if !utf8.ValidString(got) {
		t.Fatalf("stored value %q is not valid UTF-8", got)
	}
	if want := "Şişli Şişli Şişli Şişli Şişli Şişli Şişli Şişli Şi"; got != want {
		t.Errorf("59-character value stored as %q, want %q", got, want)
	}
}
