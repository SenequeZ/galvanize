package ansible

import (
	"strconv"
	"strings"
)

var knownDockerProtocols = map[string]struct{}{
	"tcp":  {},
	"udp":  {},
	"sctp": {},
}

// normalizePublishedPorts supports optional protocol hints in published_ports,
// e.g. "22/ssh" or "8080:80/http".
//
// It returns:
// - a shallow-copied deploy params map where published_ports entries are made Docker-compatible,
// - a target-port to connection-scheme map used when building connection info.
func normalizePublishedPorts(params map[string]interface{}) (map[string]interface{}, map[int]string) {
	normalized := make(map[string]interface{}, len(params))
	for k, v := range params {
		normalized[k] = v
	}

	rawPorts, ok := params["published_ports"]
	if !ok {
		return normalized, map[int]string{}
	}

	ports, ok := rawPorts.([]interface{})
	if !ok {
		return normalized, map[int]string{}
	}

	normalizedPorts := make([]interface{}, 0, len(ports))
	hints := make(map[int]string)

	for _, p := range ports {
		switch v := p.(type) {
		case int:
			normalizedPorts = append(normalizedPorts, v)
		case int64:
			normalizedPorts = append(normalizedPorts, v)
		case float64:
			normalizedPorts = append(normalizedPorts, int(v))
		case string:
			composePort, targetPort, hint := splitPortHint(v)
			normalizedPorts = append(normalizedPorts, composePort)
			if targetPort > 0 && hint != "" {
				hints[targetPort] = hint
			}
		default:
			// Keep unknown types untouched to avoid breaking existing challenge files.
			normalizedPorts = append(normalizedPorts, p)
		}
	}

	normalized["published_ports"] = normalizedPorts
	return normalized, hints
}

func splitPortHint(raw string) (composePort string, targetPort int, hint string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw, 0, ""
	}

	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 {
		return trimmed, parseTargetPort(trimmed), ""
	}

	maybeHint := strings.ToLower(strings.TrimSpace(parts[len(parts)-1]))
	if maybeHint == "" {
		return trimmed, parseTargetPort(trimmed), ""
	}
	if _, isDockerProtocol := knownDockerProtocols[maybeHint]; isDockerProtocol {
		return trimmed, parseTargetPort(trimmed), ""
	}

	base := strings.Join(parts[:len(parts)-1], "/")
	base = strings.TrimSpace(base)
	if base == "" {
		return trimmed, 0, ""
	}

	return base, parseTargetPort(base), maybeHint
}

func parseTargetPort(portDef string) int {
	trimmed := strings.TrimSpace(portDef)
	if trimmed == "" {
		return 0
	}
	segments := strings.Split(trimmed, ":")
	last := strings.TrimSpace(segments[len(segments)-1])
	if last == "" || strings.Contains(last, "-") {
		return 0
	}
	value, err := strconv.Atoi(last)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}
