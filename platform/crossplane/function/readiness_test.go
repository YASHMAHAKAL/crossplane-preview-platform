package main

import "testing"

func TestComposedReady(t *testing.T) {
	tests := []struct {
		name   string
		object map[string]any
		want   bool
	}{
		{"namespace", map[string]any{"metadata": map[string]any{"uid": "1"}, "status": map[string]any{"phase": "Active"}}, true},
		{"quota", map[string]any{"metadata": map[string]any{"uid": "1"}, "status": map[string]any{"hard": map[string]any{"pods": "4"}}}, true},
		{"service", map[string]any{"metadata": map[string]any{"uid": "1"}, "spec": map[string]any{"clusterIP": "10.96.0.1"}}, true},
		{"ingress", map[string]any{"metadata": map[string]any{"uid": "1"}, "spec": map[string]any{"rules": []any{map[string]any{"host": "preview.localhost"}}}}, true},
		{"deployment", map[string]any{"metadata": map[string]any{"uid": "1", "generation": float64(2)}, "spec": map[string]any{"replicas": float64(1)}, "status": map[string]any{"observedGeneration": float64(2), "availableReplicas": float64(1), "updatedReplicas": float64(1), "conditions": []any{map[string]any{"type": "Available", "status": "True"}}}}, true},
		{"deployment", map[string]any{"metadata": map[string]any{"uid": "1", "generation": float64(2)}, "spec": map[string]any{"replicas": float64(1)}, "status": map[string]any{"observedGeneration": float64(1), "availableReplicas": float64(1), "updatedReplicas": float64(1), "conditions": []any{map[string]any{"type": "Available", "status": "True"}}}}, false},
		{"vcluster-release", map[string]any{"metadata": map[string]any{"uid": "1"}, "status": map[string]any{"conditions": []any{map[string]any{"type": "Ready", "status": "True"}}}}, true},
		{"vcluster-release", map[string]any{"metadata": map[string]any{"uid": "1"}, "status": map[string]any{"conditions": []any{map[string]any{"type": "Ready", "status": "False"}}}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := composedReady(tc.name, tc.object); got != tc.want {
				t.Fatalf("composedReady = %v, want %v", got, tc.want)
			}
		})
	}
}
