package routines

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Routine struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Steps       []string `json:"steps"`
}

type RunResult struct {
	RoutineID string         `json:"routineId"`
	Status    string         `json:"status"`
	Output    map[string]any `json:"output,omitempty"`
	StartedAt string         `json:"startedAt"`
	EndedAt   string         `json:"endedAt"`
}

type Manager struct {
	mu       sync.RWMutex
	routines map[string]Routine
}

func NewManager() *Manager {
	m := &Manager{
		routines: map[string]Routine{},
	}
	m.registerDefaultRoutines()
	return m
}

func (m *Manager) registerDefaultRoutines() {
	m.routines["edge-wasm-smoke"] = Routine{
		ID:          "edge-wasm-smoke",
		Name:        "Edge WASM Smoke",
		Description: "Run baseline wasm runtime smoke checks",
		Steps:       []string{"load module", "evaluate sandbox policy", "execute test payload"},
	}
	m.routines["edge-router-plan"] = Routine{
		ID:          "edge-router-plan",
		Name:        "Edge Router Plan",
		Description: "Compute a deterministic route plan for multi-model execution",
		Steps:       []string{"inspect request", "score routes", "select primary+fallback"},
	}
	m.routines["edge-security-audit"] = Routine{
		ID:          "edge-security-audit",
		Name:        "Edge Security Audit",
		Description: "Run policy and guardrail checks for edge execution paths",
		Steps:       []string{"snapshot policies", "scan risky params", "emit remediation notes"},
	}
}

func (m *Manager) List() []Routine {
	m.mu.RLock()
	out := make([]Routine, 0, len(m.routines))
	for _, routine := range m.routines {
		out = append(out, routine)
	}
	m.mu.RUnlock()
	return out
}

func (m *Manager) Run(_ context.Context, routineID string, params map[string]any) (RunResult, error) {
	canonical := strings.ToLower(strings.TrimSpace(routineID))
	if canonical == "" {
		return RunResult{}, errors.New("routine id is required")
	}
	m.mu.RLock()
	routine, ok := m.routines[canonical]
	m.mu.RUnlock()
	if !ok {
		return RunResult{}, fmt.Errorf("routine %q not found", routineID)
	}

	started := time.Now().UTC()
	output := map[string]any{
		"routine": routine.Name,
		"steps":   routine.Steps,
		"params":  params,
	}
	return RunResult{
		RoutineID: routine.ID,
		Status:    "completed",
		Output:    output,
		StartedAt: started.Format(time.RFC3339),
		EndedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}
