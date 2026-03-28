// Package errors provides utilities for creating, wrapping, combining,
// and inspecting errors.
//
// It is 100% dependency-free and works perfectly with the standard
// library's errors package (Go 1.20+). Multi-errors support errors.Is,
// errors.As, and errors.Unwrap out of the box.
package errors

import (
	"errors"
	"fmt"
	"strings"
)

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
	b.WriteString(fmt.Sprintf("%d errors occurred:\n", len(m)))
	for i, err := range m {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("  - ")
		b.WriteString(err.Error())
	}
	return b.String()
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
func Wrapf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(format+": %w", append(args, err)...)
}

// Append combines multiple errors into a multi-error.
// Returns nil if all errors are nil.
func Append(err error, errs ...error) error {
	return joinNonNil(append([]error{err}, errs...)...)
}

// Join is an alias for errors.Join (stdlib).
func Join(errs ...error) error {
	return errors.Join(errs...)
}

// Flatten returns a single error if the multi-error contains only one error.
// Otherwise returns the multi-error unchanged.
func Flatten(err error) error {
	if err == nil {
		return nil
	}
	if m, ok := err.(multiError); ok && len(m) == 1 {
		return m[0]
	}
	return err
}

// Prefix adds the same prefix to every error inside err
// (works for both single errors and multi-errors).
func Prefix(err error, prefix string) error {
	if err == nil {
		return nil
	}

	// If it's already a multi-error, prefix every inner error
	if m, ok := err.(multiError); ok {
		newErrs := make(multiError, len(m))
		for i, e := range m {
			newErrs[i] = Wrap(e, prefix)
		}
		return newErrs
	}

	// Single error
	return Wrap(err, prefix)
}

// Errors returns the list of underlying errors.
// If err is not a multi-error, returns a single-element slice.
func Errors(err error) []error {
	if err == nil {
		return nil
	}
	if m, ok := err.(multiError); ok {
		return m
	}
	return []error{err}
}

// WithMessage adds msg as a sibling error.
// If err is nil, returns a plain error with msg.
func WithMessage(err error, msg string) error {
	if err == nil {
		return New(msg)
	}
	return Append(err, New(msg))
}

// Unwrap, Is, and As are re-exported for convenience.
func Unwrap(err error) error        { return errors.Unwrap(err) }
func Is(err, target error) bool     { return errors.Is(err, target) }
func As(err error, target any) bool { return errors.As(err, target) }

// joinNonNil is the internal helper that builds our multiError.
func joinNonNil(errs ...error) error {
	var nonNil []error
	for _, err := range errs {
		if err != nil {
			nonNil = append(nonNil, err)
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
