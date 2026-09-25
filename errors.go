// Package errors provides utilities for creating, wrapping, combining,
// and inspecting errors.
//
// It has no dependencies outside the standard library and interoperates
// with the standard errors package: multi-errors implement Unwrap() []error
// (the convention used by errors.Join), so errors.Is and errors.As work out
// of the box, and errors produced by errors.Join or fmt.Errorf are accepted
// everywhere.
//
// # Errors vs Leaves
//
// Errors returns the items inside a multi-error or stdlib joined error
// (nested joins are flattened). For a single fmt.Errorf("%w") chain it returns the outer wrapper as
// one element, not the inner cause.
//
// Leaves returns the root causes: it follows both single (%w) and multi
// (Unwrap() []error) wrapping at any depth, so it also finds the causes of a
// wrapped multi-error. Use it for logging or error reporting.
//
// # Differences from the standard library
//
//   - Join flattens nested multi-errors and stdlib joined errors into one
//     level, and returns the error itself (not a wrapper) when only one
//     non-nil error is given. errors.Join keeps the nesting.
//   - Multi-errors print as "N errors occurred:" followed by one line per
//     error; errors.Join separates messages with newlines only.
//
// Like errors.Join, nothing is deduplicated: joining the same error twice
// keeps both, so the count of failures is preserved.
package errors

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
)

// maxUnwrapDepth bounds how deep Leaves follows a wrapping chain.
const maxUnwrapDepth = 100

// maxLeavesNodes bounds the total number of errors Leaves visits, so that
// pathological graphs (e.g. an error whose Unwrap() []error returns itself
// twice) cannot cause exponential work.
const maxLeavesNodes = 10000

// ErrUnsupported is errors.ErrUnsupported, re-exported so this package can
// replace the standard errors package in imports.
var ErrUnsupported = errors.ErrUnsupported

// multiError is the internal multi-error type. It implements the same
// Unwrap() []error convention that errors.Join uses.
//
// It is a pointer type, so it is always comparable and hashable.
//
// Appending reuses the backing array when possible, which makes
// err = Append(err, e) in a loop amortized O(1). copyNeeded makes this safe:
// only the first Append to a given value may extend its array in place;
// later Appends to the same value copy, so results never alias each other.
type multiError struct {
	errs       []error
	copyNeeded atomic.Bool
}

func (m *multiError) Error() string {
	switch len(m.errs) {
	case 0:
		return ""
	case 1:
		return m.errs[0].Error()
	}
	var b strings.Builder
	b.WriteString(strconv.Itoa(len(m.errs)))
	b.WriteString(" errors occurred:\n")
	for i, err := range m.errs {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("  - ")
		b.WriteString(err.Error())
	}
	return b.String()
}

// Format implements fmt.Formatter. %+v formats every error with %+v, so
// verbose formats of the contained errors (e.g. stack traces) are kept.
func (m *multiError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			if len(m.errs) == 1 {
				fmt.Fprintf(s, "%+v", m.errs[0])
				return
			}
			for i, err := range m.errs {
				if i > 0 {
					fmt.Fprint(s, "\n")
				}
				fmt.Fprintf(s, "  - %+v", err)
			}
			return
		}
		fmt.Fprint(s, m.Error())
	case 'q':
		fmt.Fprintf(s, "%q", m.Error())
	default:
		fmt.Fprint(s, m.Error())
	}
}

// Unwrap returns the contained errors. The slice must not be modified.
func (m *multiError) Unwrap() []error { return m.errs }

// wrapError is the type returned by Wrap and Wrapf. It is cheaper than
// fmt.Errorf("%s: %w", ...): one allocation, no format parsing.
type wrapError struct {
	msg string
	err error
}

func (w *wrapError) Error() string { return w.msg + ": " + w.err.Error() }

func (w *wrapError) Unwrap() error { return w.err }

// Format implements fmt.Formatter. %+v is passed down to the wrapped error,
// so stack traces added by other libraries are not lost.
func (w *wrapError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			fmt.Fprintf(s, "%s: %+v", w.msg, w.err)
			return
		}
		fmt.Fprint(s, w.Error())
	case 'q':
		fmt.Fprintf(s, "%q", w.Error())
	default:
		fmt.Fprint(s, w.Error())
	}
}

// New creates a new error with the given message (never returns nil).
func New(msg string) error {
	return errors.New(msg)
}

// Errorf creates a formatted error. Use %w to wrap another error.
func Errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

// Wrap wraps err with additional context: "msg: err".
// If err is nil, returns nil.
func Wrap(err error, msg string) error {
	if err == nil {
		return nil
	}
	return &wrapError{msg: msg, err: err}
}

// Wrapf wraps err with a formatted message: "format(args...): err".
// If err is nil, returns nil.
//
// err is always the wrapped cause; format must not contain %w (go vet
// reports it). Use %% for a literal percent sign.
func Wrapf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return &wrapError{msg: fmt.Sprintf(format, args...), err: err}
}

// Append combines errors into a multi-error. Nested multi-errors and stdlib
// joined errors are flattened at any depth; nil errors are skipped. Returns nil
// if all errors are nil, and the error itself if only one is non-nil.
//
// Appending to a multi-error in a loop (err = Append(err, e)) is amortized
// O(1) per call, and results of appending to the same value never share
// state, so it is safe to append to one base from several goroutines.
func Append(err error, errs ...error) error {
	if m, ok := err.(*multiError); ok && len(errs) > 0 {
		return m.append(errs)
	}
	return joinNonNil(err, errs)
}

// append extends m with errs, reusing m's backing array if nobody did yet.
func (m *multiError) append(errs []error) error {
	n := 0
	for _, e := range errs {
		n += countOne(e)
	}
	if n == 0 {
		return m
	}
	var out []error
	if !m.copyNeeded.Swap(true) {
		out = m.errs // first extension of m: grow in place (append may reallocate)
	} else {
		out = make([]error, len(m.errs), len(m.errs)+n)
		copy(out, m.errs)
	}
	for _, e := range errs {
		out = appendFlat(out, e)
	}
	return &multiError{errs: out}
}

// Join combines errors into a multi-error. It behaves exactly like Append:
// unlike errors.Join, nested multi-errors and stdlib joined errors are
// flattened into a single level at any depth, and a single non-nil error is returned
// as is. Returns nil if all errors are nil.
func Join(errs ...error) error {
	return joinNonNil(nil, errs)
}

// Flatten returns the single contained error if err holds exactly one
// top-level error, nil if it holds none, and err otherwise.
func Flatten(err error) error {
	if err == nil {
		return nil
	}
	switch countOne(err) {
	case 0:
		return nil
	case 1:
		return appendFlat(nil, err)[0]
	default:
		return err
	}
}

// Prefix wraps every top-level error inside err with prefix
// (works for both single errors and multi-errors).
func Prefix(err error, prefix string) error {
	if err == nil {
		return nil
	}
	errs := appendFlat(nil, err)
	if len(errs) == 0 {
		return nil
	}
	for i, e := range errs {
		errs[i] = Wrap(e, prefix)
	}
	return build(errs)
}

// Errors returns the items inside err. Multi-errors and values produced by
// errors.Join are flattened at any depth; wrapped errors are not unwrapped.
// A single fmt.Errorf("%w") chain is returned as a one-element slice
// containing the outer wrapper. Use Leaves to reach root causes.
// The returned slice is a copy and may be modified.
func Errors(err error) []error {
	return appendFlat(nil, err)
}

// Leaves returns the root causes of err: errors that wrap nothing. It
// follows both Unwrap() error and Unwrap() []error at any depth, so the
// causes of a wrapped multi-error are found too. Order is depth-first,
// left to right; nil is never included.
//
// Cyclic and pathologically large error graphs are handled: the walk is
// bounded, and an error that would repeat a cycle is returned as a leaf.
func Leaves(err error) []error {
	if err == nil {
		return nil
	}
	// Fast path: a plain chain of single wraps, the common case.
	if leaf, ok := singleChainLeaf(err); ok {
		return []error{leaf}
	}
	w := leavesWalker{budget: maxLeavesNodes, out: make([]error, 0, countOne(err))}
	w.walk(err)
	return w.out
}

// Count returns the number of errors contained in err, i.e. len(Errors(err)),
// without allocating.
func Count(err error) int {
	return countOne(err)
}

// WithMessage adds msg as a separate, sibling error: the result is a
// multi-error of err and a new error with msg. If err is nil, it returns a
// plain error with msg.
//
// Note: this differs from pkg/errors.WithMessage, which wraps. To add
// context to err (causal wrapping), use Wrap.
func WithMessage(err error, msg string) error {
	if err == nil {
		return New(msg)
	}
	return Append(err, New(msg))
}

// IsAny reports whether any of the targets is present in err's tree
// (including multi-error siblings).
func IsAny(err error, targets ...error) bool {
	for _, t := range targets {
		if errors.Is(err, t) {
			return true
		}
	}
	return false
}

// AsAny tries targets in order and stores the first match found in err's
// tree (including multi-error siblings). Returns true if any target matched.
func AsAny(err error, targets ...any) bool {
	for _, t := range targets {
		if errors.As(err, t) {
			return true
		}
	}
	return false
}

// Unwrap is errors.Unwrap. It returns nil for multi-errors; use Errors.
func Unwrap(err error) error { return errors.Unwrap(err) }

// Is is errors.Is.
func Is(err, target error) bool { return errors.Is(err, target) }

// As is errors.As.
func As(err error, target any) bool { return errors.As(err, target) }

// multiUnwrapper is implemented by errors.Join values and multi-errors from
// other libraries.
type multiUnwrapper interface{ Unwrap() []error }

// countOne returns the number of errors appendFlat would produce for err,
// without allocating.
func countOne(err error) int {
	switch e := err.(type) {
	case nil:
		return 0
	case *multiError:
		return len(e.errs)
	case multiUnwrapper:
		budget := maxLeavesNodes
		return countNested(e, 0, &budget)
	default:
		return 1
	}
}

func countNested(e multiUnwrapper, depth int, budget *int) int {
	*budget--
	if depth >= maxUnwrapDepth || *budget < 0 {
		return 1 // too deep or too large: kept as one opaque item
	}
	n := 0
	for _, x := range e.Unwrap() {
		switch c := x.(type) {
		case nil:
		case *multiError:
			n += len(c.errs)
		case multiUnwrapper:
			n += countNested(c, depth+1, budget)
		default:
			n++
		}
	}
	return n
}

// appendFlat appends the non-nil errors of err to dst, flattening nested
// multi-errors (this package's and any Unwrap() []error value) at any depth.
// Wrapped errors are not unwrapped: Wrap(Join(a, b), "x") is one item.
//
// This package's multi-errors are always flat (they are only built by
// appendFlat), so their elements are copied as is. Cyclic or huge foreign
// multi-errors are bounded by maxUnwrapDepth and maxLeavesNodes; beyond the
// limits a multi-error is kept as one item.
func appendFlat(dst []error, err error) []error {
	switch e := err.(type) {
	case nil:
		return dst
	case *multiError:
		return append(dst, e.errs...)
	case multiUnwrapper:
		budget := maxLeavesNodes
		return appendNested(dst, e, 0, &budget)
	default:
		return append(dst, err)
	}
}

func appendNested(dst []error, e multiUnwrapper, depth int, budget *int) []error {
	*budget--
	if depth >= maxUnwrapDepth || *budget < 0 {
		return append(dst, e.(error))
	}
	for _, x := range e.Unwrap() {
		switch c := x.(type) {
		case nil:
		case *multiError:
			dst = append(dst, c.errs...)
		case multiUnwrapper:
			dst = appendNested(dst, c, depth+1, budget)
		default:
			dst = append(dst, x)
		}
	}
	return dst
}

// joinNonNil flattens first and rest into a new multi-error.
func joinNonNil(first error, rest []error) error {
	n := countOne(first)
	for _, e := range rest {
		n += countOne(e)
	}
	if n == 0 {
		return nil
	}
	out := make([]error, 0, n)
	out = appendFlat(out, first)
	for _, e := range rest {
		out = appendFlat(out, e)
	}
	return build(out)
}

// build returns nil, the single error, or a multi-error owning errs.
func build(errs []error) error {
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return &multiError{errs: errs}
	}
}

// leavesWalker collects root causes with cycle and size protection.
//
// Cycle detection compares an error with its ancestors only when its
// dynamic type is a pointer. Comparing interface values whose dynamic types
// are equal but not comparable (slices, maps, structs holding them) panics,
// and a cycle can only be formed through pointers anyway. Comparing a
// pointer with an ancestor of a different type is always safe (false).
type leavesWalker struct {
	out    []error
	path   []error // ancestors of the current error; only pointers are compared
	budget int
}

func (w *leavesWalker) walk(err error) {
	w.budget--
	if w.budget < 0 || len(w.path) >= maxUnwrapDepth || w.onPath(err) {
		w.out = append(w.out, err)
		return
	}

	switch e := err.(type) {
	case multiUnwrapper:
		n := 0
		w.path = append(w.path, err)
		for _, c := range e.Unwrap() {
			if c == nil {
				continue
			}
			n++
			// A child that is a plain wrap chain needs no path bookkeeping,
			// unless it could cycle back to an ancestor (pointer check).
			if leaf, ok := singleChainLeaf(c); ok && !w.onPath(leaf) {
				w.budget--
				w.out = append(w.out, leaf)
				continue
			}
			w.walk(c)
		}
		w.path = w.path[:len(w.path)-1]
		if n == 0 {
			w.out = append(w.out, err)
		}
	case interface{ Unwrap() error }:
		next := e.Unwrap()
		if next == nil {
			w.out = append(w.out, err)
			return
		}
		w.path = append(w.path, err)
		w.walk(next)
		w.path = w.path[:len(w.path)-1]
	default:
		w.out = append(w.out, err)
	}
}

func (w *leavesWalker) onPath(err error) bool {
	if len(w.path) == 0 || !isPointer(err) {
		return false
	}
	for _, p := range w.path {
		if p == err {
			return true
		}
	}
	return false
}

func isPointer(err error) bool {
	t := reflect.TypeOf(err)
	return t != nil && t.Kind() == reflect.Pointer
}

// singleChainLeaf follows Unwrap() error links while there is no multi-error
// and returns the root. ok is false if a multi-error is met (the caller must
// do a full walk). Cycles are caught by comparing pointer-typed errors with
// the chain start and by the depth limit, without allocating.
func singleChainLeaf(err error) (leaf error, ok bool) {
	start := err
	startIsPtr := isPointer(start)
	for depth := 0; depth < maxUnwrapDepth; depth++ {
		switch e := err.(type) {
		case multiUnwrapper:
			return nil, false
		case interface{ Unwrap() error }:
			next := e.Unwrap()
			if next == nil {
				return err, true
			}
			if startIsPtr && depth > 0 && next == start {
				return next, true // cycle back to the start
			}
			if depth >= 8 {
				return nil, false // long chain: let the walker check cycles properly
			}
			err = next
		default:
			return err, true
		}
	}
	return nil, false
}

// leafError follows a single Unwrap() error chain to its root, stopping at
// cycles and at maxUnwrapDepth.
func leafError(err error) error {
	var w leavesWalker
	for depth := 0; err != nil && depth < maxUnwrapDepth; depth++ {
		if w.onPath(err) {
			return err
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return err
		}
		next := u.Unwrap()
		if next == nil {
			return err
		}
		w.path = append(w.path, err)
		err = next
	}
	return err
}
