package api

import (
	"fmt"
	"net/http"
	"testing"
)

// Free tags are uppercased with Turkish rules (i → İ, ı → I), and matched
// without caring which of i, ı, İ, I was typed — the user's choice after
// review item D-20 (13/10/2026).
func TestFreeTagsTurkishCasingAndMatching(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "admin", "adminpass1234", "admin")
	client := loginAs(t, ts, "admin", "adminpass1234")

	createTag := func(value string) (int, map[string]any) {
		t.Helper()
		resp := doJSON(t, client, http.MethodPost, ts.URL+"/api/tags", map[string]string{"group": "tag", "value": value})
		status := resp.StatusCode
		return status, parseJSON(t, resp)
	}

	// Creation: Turkish uppercase; what is typed in capitals is kept.
	for _, tc := range []struct{ typed, stored string }{
		{"şişli", "ŞİŞLİ"},
		{"IT", "IT"},
		{"KRİTİK", "KRİTİK"},
	} {
		status, body := createTag(tc.typed)
		if status != http.StatusCreated || body["value"] != tc.stored {
			t.Fatalf("create %q: status %d value %v, want 201 %q", tc.typed, status, body["value"], tc.stored)
		}
	}

	// A value that differs from an existing tag only in i/ı/İ/I is a duplicate.
	for _, typed := range []string{"İT", "ŞIŞLI", "kritik"} {
		if status, body := createTag(typed); status != http.StatusConflict {
			t.Errorf("create %q next to its look-alike: status %d (%v), want 409", typed, status, body)
		}
	}

	// Target tags given in any casing resolve to the stored tag.
	payload := map[string]any{
		"name": "tr-tags", "host": "10.0.0.3", "category": "Other", "operator": "AND",
		"tags": []string{"şişli", "it", "kritik"},
		"conditions": []map[string]any{{
			"check_type": "ping", "check_name": "ping", "config": "{}", "interval_s": 300,
			"field": "status", "comparator": "eq", "value": "down", "fail_count": 1,
			"condition_group": 0, "group_operator": "AND",
		}},
	}
	resp := doJSON(t, client, http.MethodPost, ts.URL+"/api/targets", payload)
	created := parseJSON(t, resp)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create target with lowercase tags: status %d (%v), want 201", resp.StatusCode, created["error"])
	}
	resp = doJSON(t, client, http.MethodGet, ts.URL+"/api/targets/"+created["id"].(string), nil)
	got := fmt.Sprint(parseJSON(t, resp)["tags"])
	if want := "[IT KRİTİK ŞİŞLİ]"; got != want {
		t.Errorf("target tags = %s, want %s", got, want)
	}

	// Renaming a tag to its own look-alike keeps it and applies Turkish casing;
	// renaming onto another tag's look-alike is a duplicate.
	ids := map[string]int{}
	resp = doJSON(t, client, http.MethodGet, ts.URL+"/api/tags?group=tag", nil)
	for _, tag := range parseJSONArray(t, resp) {
		ids[tag["value"].(string)] = int(tag["id"].(float64))
	}
	rename := func(id int, value string) int {
		t.Helper()
		resp := doJSON(t, client, http.MethodPut, fmt.Sprintf("%s/api/tags/%d", ts.URL, id), map[string]string{"value": value})
		resp.Body.Close()
		return resp.StatusCode
	}
	if status := rename(ids["KRİTİK"], "kritik"); status != http.StatusOK {
		t.Errorf("rename KRİTİK to kritik: status %d, want 200", status)
	}
	if status := rename(ids["IT"], "şişli"); status != http.StatusConflict {
		t.Errorf("rename IT onto ŞİŞLİ's look-alike: status %d, want 409", status)
	}
	resp = doJSON(t, client, http.MethodGet, ts.URL+"/api/tags?group=tag", nil)
	var values []string
	for _, tag := range parseJSONArray(t, resp) {
		values = append(values, tag["value"].(string))
	}
	if got, want := fmt.Sprint(values), "[IT KRİTİK ŞİŞLİ]"; got != want {
		t.Errorf("tag catalog after renames = %s, want %s", got, want)
	}
}

// The category list carries each category's SLA settings key, so the browser
// does not derive it: JavaScript lowercases "İ" to "i" + U+0307 where Go gives
// "i", and the Settings save was rejected as an unknown key once a category
// name contained İ.
func TestCategoryListCarriesSLAKey(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "admin", "adminpass1234", "admin")
	client := loginAs(t, ts, "admin", "adminpass1234")

	resp := doJSON(t, client, http.MethodPost, ts.URL+"/api/tags", map[string]string{"group": "category", "value": "İSG"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create category: status %d", resp.StatusCode)
	}

	keys := map[string]any{}
	resp = doJSON(t, client, http.MethodGet, ts.URL+"/api/tags?group=category", nil)
	for _, c := range parseJSONArray(t, resp) {
		keys[c["value"].(string)] = c["sla_key"]
	}
	if keys["İSG"] != "sla_isg" || keys["Other"] != "sla_other" {
		t.Fatalf("sla_key for İSG = %v, Other = %v; want sla_isg, sla_other", keys["İSG"], keys["Other"])
	}

	resp = doJSON(t, client, http.MethodPut, ts.URL+"/api/settings", map[string]string{"sla_isg": "99.5"})
	body := parseJSON(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save sla_isg: status %d (%v), want 200", resp.StatusCode, body["error"])
	}

	// Other groups are unchanged: no sla_key.
	createdTag := doJSON(t, client, http.MethodPost, ts.URL+"/api/tags", map[string]string{"group": "location", "value": "DC-1"})
	createdTag.Body.Close()
	resp = doJSON(t, client, http.MethodGet, ts.URL+"/api/tags?group=location", nil)
	for _, l := range parseJSONArray(t, resp) {
		if _, ok := l["sla_key"]; ok {
			t.Errorf("location %v has an sla_key", l["value"])
		}
	}
}
