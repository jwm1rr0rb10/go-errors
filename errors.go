// Package errors provides utilities for creating, wrapping, combining,
// and inspecting errors.
//
// It is 100% dependency-free and works perfectly with the standard
// library's errors package (Go 1.20+). Multi-errors support errors.Is,
// errors.As, and errors.Unwrap out of the box.
//
// # Errors vs Leaves
//
// Errors returns the top-level items inside a multi-error or stdlib joined
// error. For a single fmt.Errorf("%w") chain it returns the outer wrapper as
// one element — not the inner cause.
//
// Leaves walks each extracted item to the root of its %w chain. Use Leaves
// when you need every underlying cause (for example, logging or Sentry).
package errors

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const maxUnwrapDepth = 100 // защита от бесконечных циклов

// multiError is our internal multi-error type. It implements the
// same Unwrap() []error convention that errors.Join uses, so stdlib
// functions work seamlessly.
type multiError []error

func (m multiError) Error() string {
	if len(m) == 0 {
		return ""
	}
	if len(m) == 1 {
		return m[0].Error()
	}
	var b strings.Builder
	b.WriteString(strconv.Itoa(len(m)))
	b.WriteString(" errors occurred:\n")
	for i, err := range m {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("  - ")
		b.WriteString(err.Error())
	}
	return b.String()
}

func (m multiError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			if len(m) == 1 {
				fmt.Fprintf(s, "%+v", m[0])
				return
			}
			for i, err := range m {
				if i > 0 {
					fmt.Fprint(s, "\n")
				}
				fmt.Fprint(s, "  - ")
				fmt.Fprintf(s, "%+v", err)
			}
			return
		}
	}
	fmt.Fprint(s, m.Error())
}

func (m multiError) Unwrap() []error { return m }

// New creates a new error with the given message (never returns nil).
func New(msg string) error {
	return errors.New(msg)
}

// Errorf creates a formatted error. Use %w to wrap another error.
func Errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

// Wrap wraps err with additional context.
// If err is nil, returns nil.
func Wrap(err error, msg string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", msg, err)
}

// Wrapf wraps err with a formatted message.
// If err is nil, returns nil.
// format may contain any verbs; %w is added automatically and safely.
func Wrapf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s: %w", msg, err)
}

// Append combines multiple errors into a multi-error.
// Nested multi-errors and joined errors are flattened.
// Returns nil if all errors are nil.
func Append(err error, errs ...error) error {
	return joinNonNil(append([]error{err}, errs...)...)
}

// Join combines multiple errors into a multi-error.
// Unlike [errors.Join], nested multi-errors and stdlib joined errors are
// flattened into a single level (same behavior as Append).
// Returns nil if all errors are nil.
func Join(errs ...error) error {
	return joinNonNil(errs...)
}

// Flatten returns a single error if err contains only one underlying error.
// Otherwise returns the multi-error unchanged.
func Flatten(err error) error {
	if err == nil {
		return nil
	}
	errs := extractErrors(err)
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return joinNonNil(errs...)
	}
}

// Prefix adds the same prefix to every error inside err
// (works for both single errors and multi-errors).
func Prefix(err error, prefix string) error {
	if err == nil {
		return nil
	}
	errs := extractErrors(err)
	if len(errs) == 0 {
		return nil
	}
	prefixed := make([]error, len(errs))
	for i, e := range errs {
		prefixed[i] = Wrap(e, prefix)
	}
	return joinNonNil(prefixed...)
}

// Errors returns the top-level items inside err.
// Multi-errors and values produced by [errors.Join] are flattened one level.
// A single fmt.Errorf("%w") chain is returned as a one-element slice
// containing the outer wrapper. Use [Leaves] to reach root causes.
func Errors(err error) []error {
	return extractErrors(err)
}

// Leaves returns the root cause of each item returned by [Errors].
// For multi-errors this is one leaf per sibling; for a lone %w chain it is
// the innermost wrapped error.
func Leaves(err error) []error {
	errs := extractErrors(err)
	if len(errs) == 0 {
		return nil
	}
	leaves := make([]error, 0, len(errs))
	for _, e := range errs {
		if leaf := leafError(e); leaf != nil {
			leaves = append(leaves, leaf)
		}
	}
	return leaves
}

// Count returns the number of top-level errors contained in err (see [Errors]).
func Count(err error) int {
	return len(extractErrors(err))
}

// WithMessage adds msg as a sibling error.
// If err is nil, returns a plain error with msg.
func WithMessage(err error, msg string) error {
	if err == nil {
		return New(msg)
	}
	return Append(err, New(msg))
}

// IsAny reports whether any of the targets is present in err's chain
// (including multi-error siblings).
func IsAny(err error, targets ...error) bool {
	for _, t := range targets {
		if Is(err, t) {
			return true
		}
	}
	return false
}

// AsAny finds the first target that matches in err's chain
// (including multi-error siblings) and stores it.
// Returns true if any target matched.
func AsAny(err error, targets ...any) bool {
	for _, t := range targets {
		if As(err, t) {
			return true
		}
	}
	return false
}

// Unwrap, Is, and As are re-exported for convenience.
func Unwrap(err error) error        { return errors.Unwrap(err) }
func Is(err, target error) bool     { return errors.Is(err, target) }
func As(err error, target any) bool { return errors.As(err, target) }

func leafError(err error) error {
	visited := make(map[error]struct{})
	for depth := 0; err != nil && depth < maxUnwrapDepth; depth++ {
		if _, ok := visited[err]; ok {
			return err // cycle detected
		}
		visited[err] = struct{}{}
		next := errors.Unwrap(err)
		if next == nil {
			return err
		}
		err = next
	}
	return err // depth exceeded or nil
}

// extractErrors returns the top-level items inside err (see [Errors]).
// Protected against cycles and pathological Unwrap implementations.
func extractErrors(err error) []error {
	if err == nil {
		return nil
	}

	// Fast path for our own type
	if m, ok := err.(multiError); ok {
		return append([]error(nil), m...)
	}

	// Stdlib Join / any type that implements Unwrap() []error
	if u, ok := err.(interface{ Unwrap() []error }); ok {
		errs := u.Unwrap()
		if len(errs) == 0 {
			return nil
		}
		// Защита от "Unwrap returns self"
		out := make([]error, 0, len(errs))
		seen := make(map[error]struct{})
		for _, e := range errs {
			if e == nil {
				continue
			}
			if _, ok := seen[e]; ok {
				continue // cycle / duplicate
			}
			seen[e] = struct{}{}
			out = append(out, e)
		}
		return out
	}

	return []error{err}
}

// joinNonNil is the internal helper that builds our multiError.
func joinNonNil(errs ...error) error {
	var nonNil []error
	seen := make(map[error]struct{})

	for _, err := range errs {
		for _, extracted := range extractErrors(err) {
			if extracted == nil {
				continue
			}
			if _, ok := seen[extracted]; ok {
				continue
			}
			seen[extracted] = struct{}{}
			nonNil = append(nonNil, extracted)
		}
	}

	switch len(nonNil) {
	case 0:
		return nil
	case 1:
		return nonNil[0]
	default:
		return multiError(nonNil)
	}
}
