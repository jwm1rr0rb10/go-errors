// Package errors provides utilities for creating, wrapping, combining,
// and inspecting errors.
//
// It has no dependencies outside the standard library and interoperates
// with the standard errors package: multi-errors implement Unwrap() []error
// (the convention used by errors.Join), so errors.Is and errors.As work out
// of the box, and errors produced by errors.Join or fmt.Errorf are accepted
// everywhere.
//
// # Errors vs. Leaves
//
// Errors returns the items inside a multi-error or stdlib joined error
// (nested joins are flattened). For a single fmt.Errorf("%w") chain it
// returns the outer wrapper as one element, not the inner cause. Multi-errors
// of other types (anything else with Unwrap() []error) are kept as one item,
// so their type and message are preserved.
//
// Leaves returns the root causes: it follows both single (%w) and multi
// (Unwrap() []error) wrapping at any depth, so it also finds the causes of a
// wrapped multi-error. Use it for logging or error reporting.
//
// # Differences from the standard library
//
//   - Join flattens nested multi-errors and stdlib joined errors into one
//     level and returns the error itself (not a wrapper) when only one
//     non-nil error is given. The standard errors.Join keeps the nesting.
//     Other Unwrap() []error types are not flattened.
//   - Multi-errors print as "N errors occurred": followed by one line per
//     error; errors.Join separates messages with newlines only.
//
// Like errors.Join, nothing is deduplicated: joining the same error twice
// keeps both, so the count of failures is preserved.
//
// # Single-line messages
//
// The Error method of a multi-error spans several lines. For plain-text logs
// and metrics labels use OneLine, which renders any error tree on one line:
// "sync failed: timeout; dial: connection refused".
package errors

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
)

// maxUnwrapDepth bounds how deep Leaves follows a wrapping chain.
const maxUnwrapDepth = 100

// maxLeavesNodes bounds the total number of errors Leaves visits, so that
// pathological graphs (e.g., an error whose Unwrap() []error returns itself
// twice) cannot cause exponential work.
const maxLeavesNodes = 10000

// ErrUnsupported is errors.ErrUnsupported, re-exported so this package can
// replace the standard errors package in imports (together with Is, As,
// AsType, and Unwrap).
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
// verbose formats of the contained errors (e.g., stack traces) are kept.
//
// Write errors are ignored on purpose: Format cannot return them, and fmt
// reports its own errors to the caller of Printf/Sprintf.
func (m *multiError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			if len(m.errs) == 1 {
				_, _ = fmt.Fprintf(s, "%+v", m.errs[0])
				return
			}
			for i, err := range m.errs {
				if i > 0 {
					_, _ = io.WriteString(s, "\n")
				}
				_, _ = fmt.Fprintf(s, "  - %+v", err)
			}
			return
		}
		verb = 's'
	}
	formatString(s, verb, m.Error())
}

// Unwrap returns the contained errors. The slice must not be modified.
//
// The capacity of the returned slice equals its length, so appending to it
// always copies and can never overwrite errors of another multi-error that
// shares the backing array.
func (m *multiError) Unwrap() []error { return m.errs[:len(m.errs):len(m.errs)] }

// wrapError is the type returned by Wrap and Wrapf. It is cheaper than
// fmt.Errorf("%s: %w", ...): one allocation, no format parsing.
//
// The message is built lazily, so errors that are only checked with
// errors.Is / errors.As and never printed cost nothing extra.
type wrapError struct {
	msg string
	err error
}

// Error returns "msg: cause". A chain of Wrap calls is rendered in one pass
// with a single allocation, in time linear in the length of the message.
// (Concatenating level by level would allocate once per level and copy
// O(depth²) bytes.)
func (w *wrapError) Error() string {
	// wrapError values are immutable and can only wrap errors that already
	// exist, so a chain of them cannot form a cycle.
	n := 0
	var tail error = w
	for {
		ww, ok := tail.(*wrapError)
		if !ok {
			break
		}
		n += len(ww.msg) + len(": ")
		tail = ww.err
	}
	tailMsg := tail.Error()

	var b strings.Builder
	b.Grow(n + len(tailMsg))
	// Walk by type assertion only: comparing error values (e != tail) would
	// panic when the root cause has a non-comparable dynamic type.
	for e := error(w); ; {
		ww, ok := e.(*wrapError)
		if !ok {
			break
		}
		b.WriteString(ww.msg)
		b.WriteString(": ")
		e = ww.err
	}
	b.WriteString(tailMsg)
	return b.String()
}

func (w *wrapError) Unwrap() error { return w.err }

// Format implements fmt.Formatter. %+v is passed down to the wrapped error,
// so stack traces added by other libraries are not lost.
func (w *wrapError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			_, _ = fmt.Fprintf(s, "%s: %+v", w.msg, w.err)
			return
		}
		verb = 's'
	}
	formatString(s, verb, w.Error())
}

// formatString prints msg with the verb, flags, width, and precision of the
// current directive, so %q, %x, %X, %-20s, and so on behave as they do for
// plain errors.
func formatString(s fmt.State, verb rune, msg string) {
	if verb == 's' && !hasFlagsOrWidth(s) {
		_, _ = io.WriteString(s, msg)
		return
	}
	_, _ = fmt.Fprintf(s, fmt.FormatString(s, verb), msg)
}

func hasFlagsOrWidth(s fmt.State) bool {
	if _, ok := s.Width(); ok {
		return true
	}
	if _, ok := s.Precision(); ok {
		return true
	}
	return s.Flag('-') || s.Flag('+') || s.Flag('#') || s.Flag(' ') || s.Flag('0')
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
// Err is always the wrapped cause; a format must not contain %w (go vet
// reports it). Use %% for a literal percent sign.
func Wrapf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return &wrapError{msg: fmt.Sprintf(format, args...), err: err}
}

// Append combines errors into a multi-error. Nested multi-errors of this
// package and stdlib joined errors are flattened at any depth; other
// Unwrap() []error types are kept as one item; nil errors are skipped. Returns nil
// if all errors are nil, and the error itself if only one is non-nil.
//
// Appending to a multi-error in a loop (err = Append(err, e)) is amortized
// O(1) per call, and results of appending to the same value never share
// state, so it is safe to append to one base from several goroutines.
//
// Each call still allocates a new multi-error value. On hot paths, collect
// errors in a []error and call Join once: it allocates only for the slice
// growth and the final value.
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
	switch e := err.(type) {
	case *multiError:
		return err // always holds at least two errors
	default:
		if _, ok := asStdJoin(e); !ok {
			return err
		}
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

// Prefix wraps every top-level error inside err with a prefix
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

// Errors returns the items inside err. Multi-errors of this package and
// values produced by errors.Join are flattened at any depth; other
// Unwrap() []error types and wrapped errors are not unwrapped.
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

// Count returns the number of errors contained in err, i.e., len(Errors(err)),
// without allocating.
func Count(err error) int {
	return countOne(err)
}

// AppendMessage adds msg as a separate, sibling error: the result is a
// multi-error of err and a new error with msg. If err is nil, it returns a
// plain error with msg. To add context to err (causal wrapping), use Wrap.
func AppendMessage(err error, msg string) error {
	if err == nil {
		return New(msg)
	}
	return Append(err, New(msg))
}

// WithMessage is AppendMessage.
//
// Deprecated: the name suggests the behavior of pkg/errors.WithMessage,
// which wraps err, while this function adds a sibling error. Use
// AppendMessage for the same behavior or Wrap to add context.
func WithMessage(err error, msg string) error {
	return AppendMessage(err, msg)
}

// OneLine renders err on a single line, for plain-text logs and metrics.
//
// Multi-errors (this package's and errors.Join values) are rendered as
// their items separated by "; ", wrapped errors as "context: cause", at any
// depth:
//
//	Wrap(Join(timeout, Wrap(refused, "dial")), "sync failed")
//	→ "sync failed: timeout; dial: connection refused"
//
// Wrappers from other packages, such as fmt.Errorf("ctx: %w", err), are
// handled when their message ends with the message of the wrapped error.
// Other Unwrap() []error types are rendered by their own message. Any line
// breaks left in messages are replaced with "; ". The walk
// is bounded like Leaves, so cyclic or huge error graphs are safe; output
// that hits the limit ends with "...". Returns "" for nil.
func OneLine(err error) string {
	if err == nil {
		return ""
	}
	o := oneLineWriter{budget: maxLeavesNodes}
	o.write(err, 0)
	return o.b.String()
}

type oneLineWriter struct {
	b         strings.Builder
	budget    int
	truncated bool
}

func (o *oneLineWriter) write(err error, depth int) {
	if o.truncated {
		return
	}
	o.budget--
	if o.budget < 0 || depth >= maxUnwrapDepth {
		o.b.WriteString("...")
		o.truncated = true
		return
	}

	switch e := err.(type) {
	case *wrapError:
		o.writeFlat(e.msg)
		o.b.WriteString(": ")
		o.write(e.err, depth+1)

	case multiUnwrapper:
		if _, ok := err.(*multiError); !ok {
			if _, ok := asStdJoin(err); !ok {
				// Another library's multi-error has its own message.
				o.writeFlat(err.Error())
				return
			}
		}
		first := true
		for _, c := range e.Unwrap() {
			if c == nil {
				continue
			}
			if !first {
				o.b.WriteString("; ")
			}
			first = false
			o.write(c, depth+1)
			if o.truncated {
				return
			}
		}
		if first { // no non-nil children
			o.writeFlat(err.Error())
		}

	case interface{ Unwrap() error }:
		inner := e.Unwrap()
		if inner != nil {
			msg, innerMsg := err.Error(), inner.Error()
			if prefix, ok := strings.CutSuffix(msg, innerMsg); ok {
				o.writeFlat(prefix)
				o.write(inner, depth+1)
				return
			}
			o.writeFlat(msg)
			return
		}
		o.writeFlat(err.Error())

	default:
		o.writeFlat(err.Error())
	}
}

// writeFlat writes msg, replacing line breaks with "; " and dropping
// blank lines and surrounding spaces.
func (o *oneLineWriter) writeFlat(msg string) {
	if !strings.ContainsAny(msg, "\r\n") {
		o.b.WriteString(msg)
		return
	}
	first := true
	for _, line := range strings.FieldsFunc(msg, func(r rune) bool { return r == '\n' || r == '\r' }) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !first {
			o.b.WriteString("; ")
		}
		first = false
		o.b.WriteString(line)
	}
}

// IsAny reports whether any of the targets is present in an errs tree
// (including multi-error siblings).
func IsAny(err error, targets ...error) bool {
	for _, t := range targets {
		if errors.Is(err, t) {
			return true
		}
	}
	return false
}

// AsAny tries targets in order and stores the first match found in an errs
// tree (including multi-error siblings). Returns true if any target matched.
func AsAny(err error, targets ...any) bool {
	for _, t := range targets {
		if errors.As(err, t) {
			return true
		}
	}
	return false
}

// AsType finds the first error in an errs tree that matches the type E and
// returns it. It is the generic form of As, with the same semantics as
// errors.AsType from Go 1.26, but available from Go 1.21:
//
//	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
//		fmt.Println("failed at path:", pathErr.Path)
//	}
//
// The tree is walked depth-first, like errors.As: err itself, then the
// errors returned by Unwrap() error or Unwrap() []error, including
// multi-errors of this package and errors.Join values. An error matches if
// it is assignable to E, or if it has a method As(any) bool that returns
// true for a pointer to E.
//
// Unlike As, AsType uses no reflection and cannot panic on a wrong target
// type: E is checked at compile time. Like As (and unlike Leaves), it does
// not protect against cyclic error graphs.
func AsType[E error](err error) (E, bool) {
	if err == nil {
		var zero E
		return zero, false
	}
	// The pointer handed to As methods is allocated only when an error with
	// an As method is met, so the common case does not allocate.
	var p *E
	return asType[E](err, &p)
}

func asType[E error](err error, p **E) (E, bool) {
	for {
		if e, ok := err.(E); ok {
			return e, true
		}
		if x, ok := err.(interface{ As(any) bool }); ok {
			if *p == nil {
				*p = new(E)
			}
			if x.As(*p) {
				return **p, true
			}
		}
		switch x := err.(type) {
		case interface{ Unwrap() error }:
			err = x.Unwrap()
			if err == nil {
				var zero E
				return zero, false
			}
		case interface{ Unwrap() []error }:
			for _, e := range x.Unwrap() {
				if e == nil {
					continue
				}
				if found, ok := asType[E](e, p); ok {
					return found, true
				}
			}
			var zero E
			return zero, false
		default:
			var zero E
			return zero, false
		}
	}
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

// stdJoinType is the dynamic type of errors.Join results (*errors.joinError).
var stdJoinType = reflect.TypeOf(errors.Join(errors.ErrUnsupported))

// asStdJoin returns err as a multiUnwrapper if it was produced by
// errors.Join. Only this package's multi-errors and stdlib joined errors are
// flattened: any other Unwrap() []error type (a validation error, a
// multi-error from another library, fmt.Errorf with several %w) may carry
// its own message and fields, so it is kept as one item and stays visible
// to errors.As.
func asStdJoin(err error) (multiUnwrapper, bool) {
	// The interface check is much cheaper than reflect.TypeOf and rules out
	// plain errors, the common case.
	u, ok := err.(multiUnwrapper)
	if !ok || reflect.TypeOf(err) != stdJoinType {
		return nil, false
	}
	return u, true
}

// countOne returns the number of errors appendFlat would produce for err,
// without allocating.
func countOne(err error) int {
	switch e := err.(type) {
	case nil:
		return 0
	case *multiError:
		return len(e.errs)
	default:
		if j, ok := asStdJoin(err); ok {
			budget := maxLeavesNodes
			return countNested(j, 0, &budget)
		}
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
		default:
			if j, ok := asStdJoin(x); ok {
				n += countNested(j, depth+1, budget)
			} else {
				n++
			}
		}
	}
	return n
}

// appendFlat appends the non-nil errors of err to dst, flattening nested
// multi-errors (this package's and errors.Join values) at any depth. Other
// Unwrap() []error types and wrapped errors are not unwrapped:
// Wrap(Join(a, b), "x") is one item.
//
// This package's multi-errors are always flat (they are only built by
// appendFlat), so their elements are copied as is. Huge stdlib joins are
// bounded by maxUnwrapDepth and maxLeavesNodes; beyond the limits a joined
// error is kept as one item.
func appendFlat(dst []error, err error) []error {
	switch e := err.(type) {
	case nil:
		return dst
	case *multiError:
		return append(dst, e.errs...)
	default:
		if j, ok := asStdJoin(err); ok {
			budget := maxLeavesNodes
			return appendNested(dst, j, 0, &budget)
		}
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
		default:
			if j, ok := asStdJoin(x); ok {
				dst = appendNested(dst, j, depth+1, budget)
			} else {
				dst = append(dst, x)
			}
		}
	}
	return dst
}

// joinNonNil flattens first and the rest into a new multi-error.
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
	// Safe: err is a pointer, so each comparison is either between two
	// pointers or between different dynamic types (false). It can only
	// panic for two values of the same non-comparable type.
	return slices.Contains(w.path, err)
}

func isPointer(err error) bool {
	t := reflect.TypeOf(err)
	return t != nil && t.Kind() == reflect.Pointer
}

// singleChainLeaf follows Unwrap() error links while there is no multi-error
// and returns the root. Ok is false if a multi-error is met (the caller must
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
