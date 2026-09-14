package main

import (
	"net/http"
	"strconv"
	"strings"
)

type Severity string

const (
	SevError   Severity = "error"
	SevWarning Severity = "warning"
	SevInfo    Severity = "info"
)

type Diagnostic struct {
	Severity Severity `json:"severity"`
	Header   string   `json:"header"`
	Message  string   `json:"message"`
}

type Report struct {
	Headers     map[string]string `json:"headers"`
	Directives  []Directive       `json:"cacheControlDirectives,omitempty"`
	Diagnostics []Diagnostic      `json:"diagnostics"`
}

// BuildReport validates the cache-related headers in h. Header name lookups
// are case-insensitive; headers unrelated to caching are ignored.
func BuildReport(h map[string]string) Report {
	report := Report{Headers: map[string]string{}, Diagnostics: []Diagnostic{}}

	normalized := map[string]string{}
	for name, value := range h {
		normalized[strings.ToLower(name)] = value
	}

	for _, name := range []string{"cache-control", "expires", "age", "pragma", "vary", "etag"} {
		if value, ok := normalized[name]; ok {
			report.Headers[canonicalHeaderName(name)] = value
		}
	}

	if cc, ok := normalized["cache-control"]; ok {
		directives, diags := ParseCacheControl(cc)
		report.Directives = directives
		report.Diagnostics = append(report.Diagnostics, diags...)
	}

	if age, ok := normalized["age"]; ok {
		report.Diagnostics = append(report.Diagnostics, checkAge(age)...)
	}

	if expires, ok := normalized["expires"]; ok {
		report.Diagnostics = append(report.Diagnostics, checkExpires(expires)...)
	}

	if pragma, ok := normalized["pragma"]; ok {
		report.Diagnostics = append(report.Diagnostics, checkPragma(pragma, normalized)...)
	}

	if len(report.Headers) == 0 {
		report.Diagnostics = append(report.Diagnostics, Diagnostic{Severity: SevInfo, Message: "no cache-related headers found"})
	}

	return report
}

func canonicalHeaderName(lower string) string {
	switch lower {
	case "cache-control":
		return "Cache-Control"
	case "expires":
		return "Expires"
	case "age":
		return "Age"
	case "pragma":
		return "Pragma"
	case "vary":
		return "Vary"
	case "etag":
		return "ETag"
	}
	return lower
}

func checkAge(value string) []Diagnostic {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return []Diagnostic{{Severity: SevError, Header: "Age", Message: "value \"" + value + "\" is not an integer number of seconds"}}
	}
	if n < 0 {
		return []Diagnostic{{Severity: SevError, Header: "Age", Message: "must not be negative"}}
	}
	return nil
}

func checkExpires(value string) []Diagnostic {
	trimmed := strings.TrimSpace(value)
	if trimmed == "0" {
		// A common, deliberate shorthand for "already expired".
		return nil
	}
	if _, err := http.ParseTime(trimmed); err != nil {
		return []Diagnostic{{Severity: SevError, Header: "Expires", Message: "value \"" + value + "\" is not a valid HTTP-date"}}
	}
	return nil
}

func checkPragma(value string, headers map[string]string) []Diagnostic {
	found := false
	for _, tok := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(tok), "no-cache") {
			found = true
		}
	}
	if !found {
		return nil
	}
	if _, hasCC := headers["cache-control"]; hasCC {
		return []Diagnostic{{Severity: SevInfo, Header: "Pragma", Message: "\"Pragma: no-cache\" is a legacy HTTP/1.0 fallback and is redundant next to Cache-Control"}}
	}
	return []Diagnostic{{Severity: SevInfo, Header: "Pragma", Message: "\"Pragma: no-cache\" only affects HTTP/1.0 caches; consider adding Cache-Control"}}
}
