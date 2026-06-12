package executor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/soyunomas/ghostknock/internal/config"
)

var (
	safeParamKeyRegex   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
	safeParamValueRegex = regexp.MustCompile(`^[a-zA-Z0-9._][a-zA-Z0-9._-]*$`)
)

// ValidateParams verifies parameters before they can reach logs, hooks, or
// command templates.
func ValidateParams(params map[string]string) error {
	environmentKeys := make(map[string]string, len(params))
	for key, value := range params {
		if !safeParamKeyRegex.MatchString(key) {
			return fmt.Errorf("SEGURIDAD: nombre de parámetro inválido: %q", key)
		}

		environmentKey := strings.ToUpper(key)
		if existing, found := environmentKeys[environmentKey]; found {
			return fmt.Errorf(
				"SEGURIDAD: los parámetros %q y %q generan la misma variable de entorno",
				existing,
				key,
			)
		}
		environmentKeys[environmentKey] = key

		if !safeParamValueRegex.MatchString(value) {
			return fmt.Errorf(
				"SEGURIDAD: el valor del parámetro %q contiene caracteres inválidos o empieza con un guion",
				key,
			)
		}
		if value == ".." {
			return fmt.Errorf("SEGURIDAD: uso de '..' no permitido en el parámetro %q", key)
		}
	}
	return nil
}

func validateSensitiveParamNames(sensitive []string) error {
	environmentKeys := make(map[string]string, len(sensitive))
	for _, key := range sensitive {
		if !safeParamKeyRegex.MatchString(key) {
			return fmt.Errorf("SEGURIDAD: nombre inválido en sensitive_params: %q", key)
		}
		environmentKey := strings.ToUpper(key)
		if existing, found := environmentKeys[environmentKey]; found {
			return fmt.Errorf(
				"SEGURIDAD: sensitive_params contiene nombres equivalentes: %q y %q",
				existing,
				key,
			)
		}
		environmentKeys[environmentKey] = key
	}
	return nil
}

func cloneParams(params map[string]string) map[string]string {
	if params == nil {
		return nil
	}
	cloned := make(map[string]string, len(params))
	for key, value := range params {
		cloned[key] = value
	}
	return cloned
}

func requiredParamsForTemplate(tmpl *template.Template) ([]string, error) {
	return config.RequiredTemplateParams(tmpl)
}

func validateRequiredParams(requiredParams []string, params map[string]string) error {
	for _, paramName := range requiredParams {
		if _, ok := params[paramName]; !ok {
			return fmt.Errorf(
				"SEGURIDAD: el comando requiere el parámetro %q, pero no fue proporcionado",
				paramName,
			)
		}
	}
	return nil
}

func isSensitiveParam(key string, sensitive []string) bool {
	for _, candidate := range sensitive {
		if strings.EqualFold(candidate, key) {
			return true
		}
	}
	return false
}

func redactText(text string, params map[string]string, sensitive []string) string {
	values := make([]string, 0, len(sensitive))
	seen := make(map[string]struct{}, len(sensitive))
	for key, value := range params {
		if !isSensitiveParam(key, sensitive) || value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		return len(values[i]) > len(values[j])
	})

	redacted := text
	for _, value := range values {
		redacted = strings.ReplaceAll(redacted, value, "*****")
	}
	return redacted
}
