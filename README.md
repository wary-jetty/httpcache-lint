# httpcache-lint

A command-line validator for HTTP cache headers. It reads a raw set of
response headers, parses `Cache-Control` directives properly (quoted field
lists, delta-seconds, repeated/contradictory directives), and reports what's
wrong in either plain text or JSON.

## Why

`Cache-Control` looks like a simple comma-separated list but has enough
sharp edges that hand-rolled parsing (`strings.Contains(header, "no-cache")`)
gets it wrong in ways that only show up in production:

- `no-cache="set-cookie, x-internal"` has commas inside the value, which
  breaks a naive `strings.Split(header, ",")`.
- `public` and `private` are mutually exclusive but nothing stops you from
  sending both.
- `max-age` needs an integer; `max-age=soon` is accepted silently by a lot
  of tooling.
- `immutable` with `max-age=0` and `no-store` with `max-age=3600` are each
  internally contradictory.
- `Pragma: no-cache` is a legacy HTTP/1.0 header that's easy to assume does
  something it doesn't in HTTP/1.1+.

This tool parses the directives correctly and flags exactly these problems,
with a severity (`error` / `warning` / `info`) attached to each one.

## Usage

Feed it raw headers, one `Name: value` pair per line - the output of
`curl -I`, a saved response dump, or anything shaped like that:

```
$ curl -sI https://example.com/ | ./httpcache-lint
Cache-Control: max-age=604800
Expires: Thu, 01 Jan 1970 00:00:00 GMT

directives:
  max-age = 604800

no issues found
```

A header set with problems:

```
$ printf 'Cache-Control: public, private, max-age=oops, immutable\n' | ./httpcache-lint
Cache-Control: public, private, max-age=oops, immutable

directives:
  public
  private
  max-age = oops
  immutable

issues:
  [error]   Cache-Control "public" and "private" are mutually exclusive
  [error]   Cache-Control directive "max-age" value "oops" is not an integer number of seconds
```

The exit code is 1 if any diagnostic has severity `error`, 0 otherwise -
usable as a CI check against a saved header dump.

### JSON output

Pass `--json` for a machine-readable report instead of the text above:

```
$ printf 'Cache-Control: public, private\n' | ./httpcache-lint --json
{
  "headers": {
    "Cache-Control": "public, private"
  },
  "cacheControlDirectives": [
    { "name": "public", "hasValue": false },
    { "name": "private", "hasValue": false }
  ],
  "diagnostics": [
    {
      "severity": "error",
      "header": "Cache-Control",
      "message": "\"public\" and \"private\" are mutually exclusive"
    }
  ]
}
```

### Reading from a file

```
$ ./httpcache-lint headers.txt
$ ./httpcache-lint --json headers.txt
```

## What it checks today

- `Cache-Control`: known directive table with correct value shapes
  (flag, required delta-seconds, optional delta-seconds, optional quoted
  field list), unrecognized directives, repeated directives, and the
  cross-directive contradictions listed above.
- `Age`: must be a non-negative integer.
- `Expires`: must be a valid HTTP-date (or the literal `0`).
- `Pragma: no-cache`: flagged as legacy/redundant.

## Building

Standard library only, no third-party dependencies.

```
go build ./...
```

## License

MIT, see LICENSE.
