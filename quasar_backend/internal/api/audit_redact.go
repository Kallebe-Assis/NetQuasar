package api

import "strings"

func auditSetStr(m map[string]any, key string, v *string) {
	if v != nil {
		m[key] = *v
	}
}

func auditSetMasked(m map[string]any, key string, v *string) {
	if v != nil {
		m[key] = maskSecret(v)
	}
}

func auditSetSecret(m map[string]any, key string, v *string) {
	if v != nil && strings.TrimSpace(*v) != "" {
		m[key] = "***"
	}
}

func auditSetOIDFields(m map[string]any, fields map[string]*string) {
	for k, v := range fields {
		auditSetStr(m, k, v)
	}
}
