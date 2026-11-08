package api

import (
	"net/http"
	"testing"
)

// A-072: a blanked Email or Phone clears the stored value; an omitted field keeps it.
func TestUpdateContactBlankClearsOmittedKeeps(t *testing.T) {
	type endpoint struct {
		name string
		url  func(base, id string) string
	}
	endpoints := []endpoint{
		{"me", func(base, id string) string { return base + "/api/me" }},
		{"admin_user", func(base, id string) string { return base + "/api/users/" + id }},
	}
	cases := []struct {
		name       string
		body       map[string]any
		wantStatus int
		wantEmail  string
		wantPhone  string
	}{
		{"blank email clears it", map[string]any{"email": "", "phone": "+44123456789"}, 200, "", "+44123456789"},
		{"blank phone clears it", map[string]any{"email": "old@example.com", "phone": ""}, 200, "old@example.com", ""},
		{"both blank clear both", map[string]any{"email": "", "phone": ""}, 200, "", ""},
		{"whitespace counts as blank", map[string]any{"email": "   ", "phone": "  "}, 200, "", ""},
		{"omitted phone keeps it", map[string]any{"email": "new@example.com"}, 200, "new@example.com", "+44123456789"},
		{"omitted email keeps it", map[string]any{"phone": "+44987654321"}, 200, "old@example.com", "+44987654321"},
		{"invalid email refused", map[string]any{"email": "not-an-email"}, 400, "old@example.com", "+44123456789"},
		{"invalid phone refused", map[string]any{"phone": "abc"}, 400, "old@example.com", "+44123456789"},
	}
	for _, ep := range endpoints {
		for _, tc := range cases {
			t.Run(ep.name+"/"+tc.name, func(t *testing.T) {
				ts, st := setupTestServer(t)
				admin := createUserWithContact(t, st, "ctadmin", "testpassword12345", "admin", "old@example.com", "+44123456789")
				client := loginAs(t, ts, "ctadmin", "testpassword12345")

				body := map[string]any{}
				for k, v := range tc.body {
					body[k] = v
				}
				if ep.name == "admin_user" {
					body["role"] = "admin"
				}
				resp := doJSON(t, client, http.MethodPut, ep.url(ts.URL, admin.ID), body)
				resp.Body.Close()
				if resp.StatusCode != tc.wantStatus {
					t.Fatalf("expected %d, got %d", tc.wantStatus, resp.StatusCode)
				}
				got, err := st.GetUserByID(admin.ID)
				if err != nil || got == nil {
					t.Fatal("user missing", err)
				}
				if got.Email != tc.wantEmail || got.Phone != tc.wantPhone {
					t.Fatalf("stored email=%q phone=%q, want email=%q phone=%q", got.Email, got.Phone, tc.wantEmail, tc.wantPhone)
				}
			})
		}
	}
}
