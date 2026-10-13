package api

import (
	"net/http"
	"testing"
)

// Review item D-27: a recipient list naming the same user twice is a bad
// request (400), not a server error (it hit the table's primary key and came
// back as 500); the target's recipients stay as they were.
func TestSetRecipientsRejectsDuplicateIDs(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "admin", "adminpass1234", "admin")
	client := loginAs(t, ts, "admin", "adminpass1234")
	targetID, _ := createTargetViaAPI(t, client, ts.URL, "dup-host", "10.0.0.2", "Other", "ping")
	alice := createUserWithContact(t, st, "alice", "alicepass123456", "viewer", "alice@example.com", "")
	bob := createUserWithContact(t, st, "bob", "bobpass12345678", "viewer", "bob@example.com", "")

	url := ts.URL + "/api/targets/" + targetID + "/recipients"
	resp := doJSON(t, client, http.MethodPut, url, map[string]any{"user_ids": []string{alice.ID}})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setting one recipient: status %d", resp.StatusCode)
	}

	resp = doJSON(t, client, http.MethodPut, url, map[string]any{"user_ids": []string{bob.ID, alice.ID, bob.ID}})
	body := parseJSON(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("duplicate user ID: status %d (%v), want 400", resp.StatusCode, body["error"])
	}

	ids, err := st.ListTargetRecipientIDs(targetID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != alice.ID {
		t.Fatalf("recipients after rejected request = %v, want only alice (%s)", ids, alice.ID)
	}
}
