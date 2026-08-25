package converter

import "strings"

func Redact(text string, nodes []Node) string {
	redacted := text
	for _, node := range nodes {
		values := []struct {
			value       string
			replacement string
		}{
			{node.RawLink, "[VLESS_LINK_REDACTED]"},
			{node.UUID, mask(node.UUID, "UUID")},
			{strings.ToUpper(node.UUID), mask(node.UUID, "UUID")},
			{node.PublicKey, mask(node.PublicKey, "PUBLIC_KEY")},
		}
		for _, item := range values {
			if item.value != "" {
				redacted = strings.ReplaceAll(redacted, item.value, item.replacement)
			}
		}
	}
	return redacted
}

func mask(value, label string) string {
	if len(value) <= 8 {
		return "[" + label + "_REDACTED]"
	}
	return value[:4] + "..." + value[len(value)-4:] + " [" + label + "]"
}
