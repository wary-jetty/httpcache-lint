package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

func PrintJSON(w io.Writer, report Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func PrintHuman(w io.Writer, report Report) {
	if len(report.Headers) == 0 {
		fmt.Fprintln(w, "no cache-related headers found")
	} else {
		names := make([]string, 0, len(report.Headers))
		for name := range report.Headers {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(w, "%s: %s\n", name, report.Headers[name])
		}
	}

	if len(report.Directives) > 0 {
		fmt.Fprintln(w, "\ndirectives:")
		for _, d := range report.Directives {
			if d.HasValue {
				fmt.Fprintf(w, "  %s = %s\n", d.Name, d.Value)
			} else {
				fmt.Fprintf(w, "  %s\n", d.Name)
			}
		}
	}

	if len(report.Diagnostics) == 0 {
		fmt.Fprintln(w, "\nno issues found")
		return
	}

	fmt.Fprintln(w, "\nissues:")
	for _, d := range report.Diagnostics {
		label := "[" + string(d.Severity) + "]"
		if d.Header != "" {
			fmt.Fprintf(w, "  %-9s %-13s %s\n", label, d.Header, d.Message)
		} else {
			fmt.Fprintf(w, "  %-9s %s\n", label, d.Message)
		}
	}
}
