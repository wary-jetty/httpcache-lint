package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	jsonOutput := flag.Bool("json", false, "emit machine-readable JSON instead of the human-readable report")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [--json] [file]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Reads raw HTTP response headers (one \"Name: value\" pair per line) from\n"+
			"file, or from stdin if no file is given, and validates the cache-related\n"+
			"ones: Cache-Control, Expires, Age, Pragma, Vary, ETag.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	var r io.Reader = os.Stdin
	if flag.NArg() > 0 {
		f, err := os.Open(flag.Arg(0))
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		defer f.Close()
		r = f
	}

	headers, err := readHeaders(r)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	report := BuildReport(headers)

	if *jsonOutput {
		if err := PrintJSON(os.Stdout, report); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	} else {
		PrintHuman(os.Stdout, report)
	}

	os.Exit(exitCode(report))
}

// readHeaders parses "Name: value" lines, the shape `curl -I` or a raw
// response dump produces. Lines without a colon - a leading status line,
// blank lines - are skipped rather than rejected.
func readHeaders(r io.Reader) (map[string]string, error) {
	headers := map[string]string{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if name == "" {
			continue
		}
		headers[name] = value
	}
	return headers, scanner.Err()
}

func exitCode(report Report) int {
	for _, d := range report.Diagnostics {
		if d.Severity == SevError {
			return 1
		}
	}
	return 0
}
