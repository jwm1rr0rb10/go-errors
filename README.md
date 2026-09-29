# go-errors

[Русская версия](READMEru.md)

Error utilities for Go with first-class multi-error support and full
interoperability with the standard `errors` package. No dependencies outside
the standard library.

- `New`, `Errorf`, `Wrap`, `Wrapf`, and `Is` / `As` / `Unwrap` / `ErrUnsupported`
  re-exported, so the package can replace `errors` in imports
- `Wrap` is about 5× cheaper than `fmt.Errorf("ctx: %w", err)`: one allocation,
  no format parsing, no stack traces; the message is built only when printed
- Multi-errors: `Append`, `Join`, `Flatten`, `Prefix`, `AppendMessage`, `Errors`, `Count`
- `Leaves` finds every root cause, at any depth, including behind wrapped multi-errors
- `Oneline` renders any error tree on one line for plain-text logs
- Works with `fmt.Errorf("%w")`, `errors.Is`, `errors.As` and `errors.Join` values
- Safe with any error type, including non-comparable ones (slices, maps)
- Race-tested, fuzz-tested in CI, 90%+ coverage

## Installation

```bash
go get github.com/jwm1rr0rb10/go-errors
```

Requires Go 1.21+.

## Quick start

```go
package main

import (
	"fmt"

	"github.com/jwm1rr0rb10/go-errors"
)

func main() {
	// Collect errors
	err := errors.Join(errors.New("permission denied"), errors.New("disk full"))
	fmt.Println(err)
	// 2 errors occurred:
	//   - permission denied
	//   - disk full

	// Same error on one line, for plain-text logs
	fmt.Println(errors.Oneline(err))
	// permission denied; disk full

	// Add the same context to every error
	fmt.Println(errors.Oneline(errors.Prefix(err, "backup failed")))
	// backup failed: permission denied; backup failed: disk full

	// Causal wrapping
	dbErr := errors.New("connection refused")
	err = errors.Wrapf(dbErr, "connect to %s:%d", "db.example.com", 5432)
	fmt.Println(err)                   // connect to db.example.com:5432: connection refused
	fmt.Println(errors.Is(err, dbErr)) // true
}
```

## Errors vs Leaves

| Function | Returns |
|---|---|
| `Errors(err)` | The items of a multi-error or `errors.Join` value, nested joins flattened. A wrapped error is one item: it is not unwrapped. |
| `Leaves(err)` | The root causes: follows both `%w` wrapping and multi-errors at any depth. |

```go
timeout := errors.New("timeout")
refused := errors.New("connection refused")
err := errors.Wrap(errors.Join(timeout, errors.Wrap(refused, "dial")), "sync failed")

errors.Errors(err)  // [sync failed: 2 errors occurred: ...]  (one wrapped item)
errors.Leaves(err)  // [timeout, connection refused]
errors.Oneline(err) // "sync failed: timeout; dial: connection refused"
```

Use `Leaves` for error reporting and `Oneline` for log messages. Cyclic or
pathologically large error graphs are handled by both: the walk is bounded
and never loops.

## Collecting errors

On hot paths, collect errors in a slice and join them once:

```go
var errs []error
for _, item := range items {
	if e := validate(item); e != nil {
		errs = append(errs, e)
	}
}
return errors.Join(errs...) // nil if nothing failed, the error itself if only one failed
```

`err = errors.Append(err, e)` in a loop also works and is amortized O(1): it
reuses the backing array, and results of appending to the same value never
share state, so appending to one base from several goroutines is safe. But
each call allocates a new multi-error value, so for 1000 errors it makes
about 1000 allocations against 13 for the slice-and-`Join` pattern (see
[Performance](#performance)).

## API

| Function | Description |
|---|---|
| `New(msg) error` | `errors.New` (never nil). |
| `Errorf(format, args...) error` | `fmt.Errorf`; use `%w` to wrap. |
| `Wrap(err, msg) error` | `"msg: err"`, keeps `err` as the cause. nil → nil. |
| `Wrapf(err, format, args...) error` | `Wrap` with a formatted message. nil → nil. `format` must not contain `%w` (`go vet` reports it); use `%%` for `%`. |
| `Append(err, errs...) error` | Combine errors; nested multi-errors flattened, nils skipped. All nil → nil; one non-nil → that error. |
| `Join(errs...) error` | Same as `Append` (see the differences from `errors.Join` below). |
| `Flatten(err) error` | The single contained error if there is exactly one; otherwise `err`. |
| `Prefix(err, prefix) error` | Wrap every contained error with `prefix`. |
| `AppendMessage(err, msg) error` | Add `msg` as a **sibling** error (a multi-error of `err` and `msg`). Use `Wrap` to add context. |
| `Errors(err) []error` | Contained errors (a copy, safe to modify). |
| `Leaves(err) []error` | Root causes at any depth. |
| `Count(err) int` | `len(Errors(err))` without allocating. |
| `Oneline(err) string` | Any error tree on one line: items joined with `"; "`, wraps as `"ctx: cause"`. |
| `IsAny(err, targets...) bool` | `errors.Is` for any of the targets. |
| `AsAny(err, targets...) bool` | `errors.As` for the first matching target. |
| `Is`, `As`, `Unwrap`, `ErrUnsupported` | Re-exported from `errors`. |
| `WithMessage(err, msg) error` | **Deprecated**, same as `AppendMessage`. Unlike `pkg/errors.WithMessage` it does not wrap, which the name wrongly suggests. |

## Differences from `errors.Join`

| | `Join` / `Append` | `errors.Join` |
|---|---|---|
| Nested joins | Flattened into one level | Nesting kept |
| One non-nil error | Returned as is | Wrapped in a join value |
| Duplicates | Kept (like `errors.Join`) | Kept |
| `errors.Is` / `errors.As` | Yes | Yes |
| Message | `N errors occurred:` + one `  - msg` line per error | Messages separated by newlines |

Values created by `errors.Join` (and any type with `Unwrap() []error`) are
accepted by every function of this package.

## Formatting

`%v` and `%s` print `Error()`. All string verbs and flags work as for a plain
error: `%q`, `%x`, `%X`, `%-20s`, `%.10s` and so on. `%+v` is passed down to the
contained errors, so verbose formats of other libraries (for example stack
traces) are kept.

`Error()` of a multi-error spans several lines. For plain-text logs, alerts
and metric labels use `Oneline(err)`. It also flattens multi-errors hidden
behind `Wrap` or `fmt.Errorf("ctx: %w", ...)`, and replaces line breaks left
in other messages with `"; "`.

## Performance

`make bench`. Go 1.22, linux/amd64.

| Operation | go-errors | Standard library |
|---|---|---|
| Wrap once | 36 ns, 1 alloc | `fmt.Errorf("ctx: %w")`: 210 ns, 2 allocs |
| `Wrapf(err, "user %d", 42)` | 137 ns, 2 allocs | — |
| Build a 5-level chain, check with `errors.Is` | 221 ns, 6 allocs | 1140 ns, 11 allocs |
| Build a 5-level chain, print it once | 288 ns, 7 allocs | 1090 ns, 11 allocs |
| `Error()` of a ready 5-level chain | 85 ns, 1 alloc | 2 ns, 0 allocs (message stored) |
| `Error()` of a ready 20-level chain | 205 ns, 1 alloc | 2 ns, 0 allocs |
| `Append(x, y)` | 93 ns, 2 allocs | `errors.Join`: 76 ns, 2 allocs |
| 1000 errors, `err = Append(err, e)` | 55 µs, 1010 allocs | — |
| 1000 errors, slice + one `Join` | 25 µs, 13 allocs | — |
| `Leaves`, chain of 3 wraps | 54 ns, 1 alloc | — |
| `Oneline`, wrapped multi-error | 259 ns, 3 allocs | — |
| `Count`, `Flatten` | 2–3 ns, 0 allocs | — |

`Wrap` builds the message lazily. This is the right trade-off for servers,
where most errors are only checked (`errors.Is`, retries, `ErrNotFound`) and
never printed. The cost is that each `Error()` call builds the string again,
in one allocation and linear time. If you print the same error many times,
save the string once.

## Changelog

### v1.2.0

Fixes:

- **Data corruption**: appending to the slice returned by a multi-error's
  `Unwrap()` could overwrite an error of another multi-error that shared the
  same backing array. `Unwrap()` now returns a slice whose capacity equals its
  length.
- `Error()` of a `Wrap` chain allocated once per level and copied O(depth²)
  bytes (20 levels: 1.6 µs, 20 allocs, 5 KB). Now one allocation in linear
  time (205 ns, 320 B).
- `%x`, `%X`, width, precision and flags were ignored for wrapped errors and
  multi-errors.

Added:

- `Oneline` for single-line messages.
- `AppendMessage`; `WithMessage` is deprecated because its name suggests
  wrapping, like `pkg/errors.WithMessage`.
- CI: gofmt, vet, race tests on Go 1.21 and stable, staticcheck, fuzzing.
- Benchmarks for real request paths and the slice-and-`Join` pattern.

Removed: unused internal `leafError`.

### v1.1.0

Fixes:

- **Panic** `hash of unhashable type` when an error of a non-comparable type
  (a slice such as `validator.ValidationErrors`, or a struct with a map) was
  passed to `Append`, `Join`, `Flatten` or `Leaves`, and in
  `Leaves(Wrap(Join(a, b), ...))`.
- `Append` in a loop was quadratic (20,000 appends took 32 s); now amortized O(1).
- `Leaves` did not look inside wrapped multi-errors.
- `%q` did not quote multi-errors.

Behavior changes:

- No deduplication: `Join(err, err)` keeps both, like `errors.Join`.
  Previously identical errors were merged, which lost the count of failures.
- Nested joins are flattened at any depth (was one level), so `Count`,
  `Errors` and `Join` always agree.
- `Wrap`/`Wrapf` return an internal type instead of `*fmt.wrapError`. Messages,
  `errors.Is/As/Unwrap` are unchanged.

Added: `ErrUnsupported`, runnable examples, fuzzing of arbitrary error trees,
benchmarks. Minimum Go version lowered from 1.25 to 1.21.

## Development

```bash
make test   # tests with the race detector
make cover  # coverage
make bench  # benchmarks
make fuzz   # fuzzing (FUZZTIME=5m make fuzz)
make lint   # staticcheck
```

## License

[MIT](LICENSE) © Raman Zaitsau [@jwm1rr0rb10](https://github.com/jwm1rr0rb10)
