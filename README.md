# errors

**Zero-dependency, modern Go error utilities** with first-class multi-error support, perfect `errors.Is`/`As`/`Unwrap` interop, and a beautiful API.

Built as a drop-in replacement for the standard `errors` package + `github.com/hashicorp/go-multierror` (or `go.uber.org/multierr`), but **without any external dependencies**.

---

## Features

- `New`, `Errorf`, `Wrap`, **and** the missing `Wrapf` everyone wants
- Full multi-error support via a tiny internal `multiError` type
- `Append`, `Join`, `Flatten`, `Prefix`, `WithMessage`, and `Errors()`
- 100% compatible with `fmt.Errorf("%w", ...)`, `errors.Is`, `errors.As`, and `errors.Join`
- Excellent error formatting (`%v` and `%+v` look great)
- Full godoc examples and table-tested behavior
- No runtime allocation surprises

## Installation

If this package lives inside your own module:

```bash
go get -u github.com/jwm1rr0rb10/go-errors
```

---

## Quick Start

```go
package main

import (
    "fmt"
    "yourmodule/errors" // or "github.com/yourusername/errors"
)

func main() {
    err1 := errors.New("permission denied")
    err2 := errors.New("disk full")

    // Combine multiple errors
    err := errors.Append(err1, err2)
    fmt.Println(err)
    // Output:
    // 2 errors occurred:
    //   - permission denied
    //   - disk full

    // Add context to everything
    err = errors.Prefix(err, "backup failed")
    fmt.Println(err)
    // Output:
    // 2 errors occurred:
    //   - backup failed: permission denied
    //   - backup failed: disk full

    // Causal wrapping (the stdlib way)
    dbErr := errors.New("connection refused")
    err = errors.Wrapf(dbErr, "failed to connect to %s:%d", "db.example.com", 5432)
    fmt.Println(err)
    // Output: failed to connect to db.example.com:5432: connection refused

    // Check errors the normal way
    if errors.Is(err, dbErr) {
        fmt.Println("original db error is still in the chain")
    }
}
```

## API Overview
| Function                                             | Description                                                  |
|:-----------------------------------------------------|:-------------------------------------------------------------|
| New(msg string) error                                | Same as errors.New (never returns nil)                       |
| "Errorf(format string, args ...any) error"           | Same as fmt.Errorf                                           |
| "Wrap(err error, msg string) error"                  | Wrap with context (nil → nil)                                |
| "Wrapf(err error, format string, args ...any) error" | Formatted wrap (nil → nil)                                   |
| "Append(err error, errs ...error) error"             | Multi-error builder (nil if all nil)                         |
| Join(errs ...error) error                            | Alias for errors.Join                                        |
| Flatten(err error) error                             | Collapse single-error multi-errors                           |
| "Prefix(err error, prefix string) error"             | Add prefix to every error inside                             |
| "WithMessage(err error, msg string) error"           | Add sibling message (like old Combine)                       |
| Errors(err error) []error,                           | Extract all underlying errors                                |
| "Is, As, Unwrap"                                     | Re-exported stdlib helpers|

---

## Why this library?

- **No dependency hell** — unlike `go-multierror` or `multierr`
- **Modern Go** — fully leverages Go 1.20+ `errors.Join` and `%w` unwrapping
- **Better UX** than both HashiCorp and Uber versions
- **Beautiful output** — multi-errors print cleanly by default
- **Zero surprises** — every function has clear, documented nil-handling

---

## License
[MIT License](https://github.com/jwm1rr0rb10/go-errors/blob/main/LICENSE) – © Raman Zaitsau [@jwm1rrr0rb10](https://github.com/jwm1rr0rb10)

Made with ❤️ for cleaner Go error handling