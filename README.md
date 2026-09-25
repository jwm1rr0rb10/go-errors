# go-errors

[Русская версия](READMEru.md)

Error utilities for Go with first-class multi-error support and full
interoperability with the standard `errors` package. No dependencies outside
the standard library.

- `New`, `Errorf`, `Wrap`, `Wrapf`, and `Is` / `As` / `Unwrap` / `ErrUnsupported`
  re-exported, so the package can replace `errors` in imports
- Multi-errors: `Append`, `Join`, `Flatten`, `Prefix`, `WithMessage`, `Errors`, `Count`
- `Leaves` finds every root cause, at any depth, including behind wrapped multi-errors
- Works with `fmt.Errorf("%w")`, `errors.Is`, `errors.As` and `errors.Join` values
- Safe with any error type, including non-comparable ones (slices, maps)
- `err = Append(err, e)` in a loop is amortized O(1)
- Race-tested, fuzz-tested, 90%+ coverage

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
	var err error
	err = errors.Append(err, errors.New("permission denied"))
	err = errors.Append(err, errors.New("disk full"))
	fmt.Println(err)
	// 2 errors occurred:
	//   - permission denied
	//   - disk full

	// Add the same context to every error
	fmt.Println(errors.Prefix(err, "backup failed"))
	// 2 errors occurred:
	//   - backup failed: permission denied
	//   - backup failed: disk full

	// Causal wrapping
	dbErr := errors.New("connection refused")
	err = errors.Wrapf(dbErr, "connect to %s:%d", "db.example.com", 5432)
	fmt.Println(err)                    // connect to db.example.com:5432: connection refused
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

errors.Errors(err) // [sync failed: 2 errors occurred: ...]  (one wrapped item)
errors.Leaves(err) // [timeout, connection refused]
```

Use `Leaves` for logging and error reporting. Cyclic or pathologically large
error graphs are handled: the walk is bounded and never loops.

## Collecting errors in loops

```go
var err error
for _, item := range items {
	if e := validate(item); e != nil {
		err = errors.Append(err, e)
	}
}
return err // nil if nothing failed, the error itself if only one failed
```

Appending to a multi-error reuses its backing array, so each call is
amortized O(1). Results of appending to the same value never share state:
appending to one base from several goroutines is safe.

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
| `WithMessage(err, msg) error` | Add `msg` as a **sibling** error (a multi-error of `err` and `msg`). Unlike `pkg/errors.WithMessage`, this does not wrap; use `Wrap` for context. |
| `Errors(err) []error` | Contained errors (a copy, safe to modify). |
| `Leaves(err) []error` | Root causes at any depth. |
| `Count(err) int` | `len(Errors(err))` without allocating. |
| `IsAny(err, targets...) bool` | `errors.Is` for any of the targets. |
| `AsAny(err, targets...) bool` | `errors.As` for the first matching target. |
| `Is`, `As`, `Unwrap`, `ErrUnsupported` | Re-exported from `errors`. |

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

`%v` and `%s` print `Error()`, `%q` prints it quoted. `%+v` is passed down to
the contained errors, so verbose formats of other libraries (for example
stack traces) are kept.

## Performance

`make bench`. Go 1.26, Intel Xeon 2.1 GHz. v1.0.2 is the previous version.

| Operation | v1.0.2 | v1.1.0 |
|---|---|---|
| `Wrap(err, "ctx")` | 176 ns, 3 allocs | 29 ns, 1 alloc |
| `Wrapf(err, "user %d", 42)` | 256 ns, 4 allocs | 87 ns, 2 allocs |
| `Append(x, y)` | 239 ns, 6 allocs | 68 ns, 2 allocs |
| `Append` in a loop, 1000 errors | 50 ms, 74 MB | 48 µs, 67 KB |
| `Leaves`, chain of 3 wraps | 159 ns, 2 allocs | 39 ns, 1 alloc |
| `Leaves`, multi-error | 276 ns, 2 allocs | 121 ns, 2 allocs |
| `Count` | 63 ns, 1 alloc | 1.5 ns, 0 allocs |
| `Flatten` | 356 ns, 8 allocs | 2.4 ns, 0 allocs |

For reference: `fmt.Errorf("ctx: %w", err)` is 128 ns, 2 allocs;
`errors.Join(x, y)` is 60 ns, 2 allocs.

## Changelog

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
make lint   # golangci-lint
```

## License

[MIT](LICENSE) © Raman Zaitsau [@jwm1rr0rb10](https://github.com/jwm1rr0rb10)
