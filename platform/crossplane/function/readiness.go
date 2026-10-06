package main

// composedReady reports whether an observed resource is usable by the preview.
// The host ingress has no load balancer address in kind, so its existence is
// checked here while the evaluator's status API verifies the actual URL.
func composedReady(name string, object map[string]any) bool {
	metadata := field(object, "metadata")
	status := field(object, "status")
	if metadata["uid"] == nil {
		return false
	}
	switch name {
	case "namespace":
		return status["phase"] == "Active"
	case "quota":
		return len(field(status, "hard")) > 0
	case "service":
		return field(object, "spec")["clusterIP"] != nil
	case "ingress":
		return len(list(field(object, "spec")["rules"])) > 0
	case "deployment":
		spec := field(object, "spec")
		desired := number(spec["replicas"])
		return desired > 0 && number(status["observedGeneration"]) >= number(metadata["generation"]) &&
			number(status["availableReplicas"]) >= desired && number(status["updatedReplicas"]) >= desired &&
			conditionTrue(status, "Available")
	case "vcluster-release":
		return conditionTrue(status, "Ready")
	default:
		return false
	}
}

func field(object map[string]any, key string) map[string]any {
	value, _ := object[key].(map[string]any)
	return value
}

func list(value any) []any {
	items, _ := value.([]any)
	return items
}

func number(value any) float64 {
	switch value := value.(type) {
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case float64:
		return value
	default:
		return 0
	}
}

func conditionTrue(status map[string]any, kind string) bool {
	for _, raw := range list(status["conditions"]) {
		condition, ok := raw.(map[string]any)
		if ok && condition["type"] == kind && condition["status"] == "True" {
			return true
		}
	}
	return false
}
