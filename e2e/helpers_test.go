package e2e

import (
	"fmt"
	"os/exec"
	"strings"
)

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command %s failed: %v, output: %s", name, err, string(output))
	}
	return nil
}

func sanitizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")

	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}

	s := result.String()
	if len(s) > 63 {
		s = s[:63]
	}
	s = strings.TrimRight(s, "-")
	return s
}

func parseWhoamiHeaders(body string) map[string]string {
	headers := make(map[string]string)
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		if idx := strings.Index(line, ": "); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+2:])
			headers[key] = value
		}
	}
	return headers
}
