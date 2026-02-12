package engine

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/bekci/internal/store"
)

type Engine struct {
	store *store.Store
}

func New(st *store.Store) *Engine {
	return &Engine{store: st}
}

// Evaluate evaluates all rules that reference the given check.
func (e *Engine) Evaluate(checkID string) {
	rules, err := e.store.GetRulesByCheckID(checkID)
	if err != nil {
		slog.Error("Engine: failed to get rules for check", "check_id", checkID, "error", err)
		return
	}

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		e.evaluateRule(rule)
	}
}

func (e *Engine) evaluateRule(rule store.Rule) {
	conds, err := e.store.ListRuleConditions(rule.ID)
	if err != nil {
		slog.Error("Engine: failed to list conditions", "rule_id", rule.ID, "error", err)
		return
	}
	if len(conds) == 0 {
		_ = e.store.TouchRuleEvaluated(rule.ID)
		return
	}

	// Evaluate each condition
	results := make([]bool, len(conds))
	for i, cond := range conds {
		results[i] = e.evaluateCondition(cond)
	}

	// Combine: AND = all true, OR = any true
	var combined bool
	if rule.Operator == "OR" {
		for _, r := range results {
			if r {
				combined = true
				break
			}
		}
	} else { // AND
		combined = true
		for _, r := range results {
			if !r {
				combined = false
				break
			}
		}
	}

	newState := "healthy"
	if combined {
		newState = "unhealthy"
	}

	// Compare with current state
	currentState, err := e.store.GetRuleState(rule.ID)
	if err != nil {
		slog.Error("Engine: failed to get rule state", "rule_id", rule.ID, "error", err)
		return
	}

	oldState := "healthy"
	if currentState != nil {
		oldState = currentState.CurrentState
	}

	if newState != oldState {
		if err := e.store.UpdateRuleState(rule.ID, newState); err != nil {
			slog.Error("Engine: failed to update rule state", "rule_id", rule.ID, "error", err)
			return
		}
		slog.Warn("Rule state changed", "rule_id", rule.ID, "rule_name", rule.Name,
			"from", oldState, "to", newState, "severity", rule.Severity)
	} else {
		_ = e.store.TouchRuleEvaluated(rule.ID)
	}
}

// evaluateCondition checks whether a single condition is "triggered" (unhealthy).
func (e *Engine) evaluateCondition(cond store.RuleCondition) bool {
	if cond.FailWindow > 0 {
		// Window-based: count matching results within the window
		results, err := e.store.GetRecentResultsByWindow(cond.CheckID, cond.FailWindow)
		if err != nil {
			slog.Error("Engine: failed to get results by window", "check_id", cond.CheckID, "error", err)
			return false
		}
		matchCount := 0
		for _, r := range results {
			actual := extractField(r, cond.Field)
			if compare(actual, cond.Comparator, cond.Value) {
				matchCount++
			}
		}
		return matchCount >= cond.FailCount
	}

	// Single-result check
	last, err := e.store.GetLastResult(cond.CheckID)
	if err != nil || last == nil {
		return false
	}
	actual := extractField(*last, cond.Field)
	return compare(actual, cond.Comparator, cond.Value)
}

// extractField pulls a value from a CheckResult by field name.
func extractField(r store.CheckResult, field string) string {
	switch field {
	case "status":
		return r.Status
	case "response_ms":
		return fmt.Sprint(r.ResponseMs)
	default:
		if strings.HasPrefix(field, "metrics.") {
			key := strings.TrimPrefix(field, "metrics.")
			var m map[string]any
			if err := json.Unmarshal([]byte(r.Metrics), &m); err != nil {
				return ""
			}
			if v, ok := m[key]; ok {
				return fmt.Sprint(v)
			}
		}
	}
	return ""
}

// compare performs the comparison between actual and expected values.
func compare(actual, comparator, expected string) bool {
	switch comparator {
	case "eq":
		return actual == expected
	case "neq":
		return actual != expected
	case "gt", "lt", "gte", "lte":
		a, errA := strconv.ParseFloat(actual, 64)
		e, errE := strconv.ParseFloat(expected, 64)
		if errA != nil || errE != nil {
			return false
		}
		switch comparator {
		case "gt":
			return a > e
		case "lt":
			return a < e
		case "gte":
			return a >= e
		case "lte":
			return a <= e
		}
	}
	return false
}
