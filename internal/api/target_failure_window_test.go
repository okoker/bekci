package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/bekci/internal/store"
)

func TestTargetFailureWindowValidation(t *testing.T) {
	cases := []struct {
		name                                string
		count, interval, window             int
		wantCount, wantInterval, wantWindow int
		reject                              bool
	}{
		{name: "impossible_omitted_window", count: 10, interval: 300, reject: true},
		{name: "impossible_explicit_window", count: 10, interval: 300, window: 3000, reject: true},
		{name: "just_over_limit", count: 2, interval: 901, reject: true},
		{name: "long_interval", count: 2, interval: 3600, reject: true},
		{name: "exact_limit", count: 6, interval: 300, wantCount: 6, wantInterval: 300, wantWindow: 1800},
		{name: "two_failures_at_limit", count: 2, interval: 900, wantCount: 2, wantInterval: 900, wantWindow: 1800},
		{name: "default_window", count: 3, interval: 300, wantCount: 3, wantInterval: 300, wantWindow: 900},
		{name: "raise_short_window", count: 3, interval: 300, window: 1, wantCount: 3, wantInterval: 300, wantWindow: 900},
		{name: "cap_large_window", count: 3, interval: 300, window: 7200, wantCount: 3, wantInterval: 300, wantWindow: 1800},
		{name: "single_daily_check", count: 1, interval: 86400, wantCount: 1, wantInterval: 86400},
		{name: "default_count", interval: 3600, wantCount: 1, wantInterval: 3600},
		{name: "normalized_count", count: 101, interval: 18, wantCount: 100, wantInterval: 18, wantWindow: 1800},
		{name: "normalized_interval", count: 100, interval: 0, wantCount: 100, wantInterval: 10, wantWindow: 1000},
	}
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		for _, tc := range cases {
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				st, err := store.New(t.TempDir() + "/test.db")
				if err != nil {
					t.Fatal(err)
				}
				defer st.Close()
				srv := &Server{store: st}
				request := targetRequest{
					Name: "SR13 target", Host: "example.invalid", Category: "Other", Operator: "AND",
					Conditions: []targetConditionRequest{
						{CheckType: "ping", CheckName: "First check", Config: "{}", IntervalS: 60, FailCount: 1},
						{CheckType: "http", CheckName: "Second check", Config: "{}", IntervalS: 60, FailCount: 1},
					},
				}
				invoke := func(method, id string) *httptest.ResponseRecorder {
					t.Helper()
					body, err := json.Marshal(request)
					if err != nil {
						t.Fatal(err)
					}
					r := httptest.NewRequest(method, "/api/targets/"+id, bytes.NewReader(body))
					r.Header.Set("Content-Type", "application/json")
					w := httptest.NewRecorder()
					if method == http.MethodPost {
						srv.handleCreateTarget(w, r)
					} else {
						r.SetPathValue("id", id)
						srv.handleUpdateTarget(w, r)
					}
					return w
				}
				var before *store.TargetDetail
				id := ""
				if method == http.MethodPut {
					w := invoke(http.MethodPost, "")
					if w.Code != http.StatusCreated {
						t.Fatalf("setup: %d %s", w.Code, w.Body.String())
					}
					if err := json.Unmarshal(w.Body.Bytes(), &before); err != nil {
						t.Fatal(err)
					}
					id = before.ID
					// Compare two store reads so JSON timezone normalization cannot
					// make unchanged timestamps look different to reflect.DeepEqual.
					before, err = st.GetTargetDetail(id)
					if err != nil {
						t.Fatal(err)
					}
					for i, c := range before.Conditions {
						request.Conditions[i].CheckID = c.CheckID
					}
					// Detect partial writes to both the target and an earlier valid check.
					request.Name = "Changed name"
					request.Conditions[0].IntervalS = 120
				}
				request.Conditions[1].FailCount = tc.count
				request.Conditions[1].IntervalS = tc.interval
				request.Conditions[1].FailWindow = tc.window
				w := invoke(method, id)
				if tc.reject {
					if w.Code != http.StatusBadRequest {
						t.Errorf("expected 400 for impossible failure threshold, got %d: %s", w.Code, w.Body.String())
					} else {
						var response map[string]string
						if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
							t.Fatal(err)
						}
						for _, fragment := range []string{"Second check", "1800", "fail_count", "interval_s"} {
							if !strings.Contains(response["error"], fragment) {
								t.Errorf("error should identify the check, limit and fields to correct: %q", response["error"])
							}
						}
					}
					if method == http.MethodPut {
						after, err := st.GetTargetDetail(id)
						if err != nil {
							t.Fatal(err)
						}
						if !reflect.DeepEqual(before, after) {
							t.Error("rejected update changed the saved target or its conditions")
						}
					} else {
						targets, err := st.ListTargetSummaries()
						if err != nil {
							t.Fatal(err)
						}
						if len(targets) != 0 {
							t.Error("rejected creation left a saved target")
						}
					}
					return
				}
				wantStatus := http.StatusCreated
				if method == http.MethodPut {
					wantStatus = http.StatusOK
				}
				if w.Code != wantStatus {
					t.Fatalf("expected %d, got %d: %s", wantStatus, w.Code, w.Body.String())
				}
				var detail store.TargetDetail
				if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
					t.Fatal(err)
				}
				got := detail.Conditions[1]
				if got.FailCount != tc.wantCount || got.IntervalS != tc.wantInterval || got.FailWindow != tc.wantWindow {
					t.Errorf("got count=%d interval=%d window=%d; want %d/%d/%d", got.FailCount, got.IntervalS, got.FailWindow, tc.wantCount, tc.wantInterval, tc.wantWindow)
				}
			})
		}
	}
}
