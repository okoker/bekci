package scheduler

import (
	"strings"
	"testing"

	"github.com/bekci/internal/store"
)

// T-SC9: null SNMP config uses defaults and still receives global credentials.
func TestRunCheckSNMPNullConfig(t *testing.T) {
	for _, checkType := range []string{"snmp_v2c", "snmp_v3"} {
		for _, config := range []string{"{}", "null", " \nnull\t"} {
			t.Run(checkType+"/"+config, func(t *testing.T) {
				defer func() {
					if p := recover(); p != nil {
						t.Fatalf("runCheck panicked: %v", p)
					}
				}()

				st := newTestStore(t)
				if err := st.SetSettings(map[string]string{
					"snmp_v3_username":       "test-user",
					"snmp_v3_security_level": "noAuthNoPriv",
				}); err != nil {
					t.Fatal(err)
				}
				target := &store.Target{
					Name: "snmp-test", Host: "127.0.0.1:invalid",
					Enabled: true, Operator: "AND", Category: "Network",
				}
				conds := []store.TargetCondition{{
					CheckType: checkType, CheckName: "SNMP", Config: config,
					IntervalS: 60, Field: "status", Comparator: "eq", Value: "down",
					FailCount: 1, GroupOperator: "AND",
				}}
				if err := st.CreateTargetWithConditions(target, conds, ""); err != nil {
					t.Fatal(err)
				}
				checks, err := st.ListChecksByTarget(target.ID)
				if err != nil || len(checks) != 1 {
					t.Fatalf("expected one check, got %v: %v", checks, err)
				}

				// The invalid address fails before network I/O, after config preparation.
				s := New(st)
				for range 2 {
					s.runCheck(checks[0].ID)
					result, err := st.GetLastResult(checks[0].ID)
					if err != nil || result == nil {
						t.Fatalf("expected saved result, got %v: %v", result, err)
					}
					if result.Status != "down" || !strings.HasPrefix(result.Message, "SNMP connect failed:") {
						t.Fatalf("expected normal connection failure, got %+v", result)
					}
				}
				check, err := st.GetCheck(checks[0].ID)
				if err != nil || check == nil || check.Config != config {
					t.Fatalf("SNMP preparation changed stored config: %v, %v", check, err)
				}
			})
		}
	}
}
