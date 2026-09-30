package errors

import (
	stderrors "errors"
	"fmt"
	"strings"
	"testing"
)

// Regression: appending to the slice returned by Unwrap must not overwrite
// an error of another multi-error that shares the backing array.
func TestUnwrapDoesNotAlias(t *testing.T) {
	var base error
	for i := 0; i < 3; i++ {
		base = Append(base, New("e"))
	}
	m2 := Append(base, New("second")) // may extend base's array in place

	u := base.(interface{ Unwrap() []error }).Unwrap()
	if len(u) != cap(u) {
		t.Fatalf("Unwrap: len %d != cap %d", len(u), cap(u))
	}
	_ = append(u, New("oops"))

	errs := Errors(m2)
	if got := errs[len(errs)-1].Error(); got != "second" {
		t.Fatalf("last error of m2 = %q, want %q", got, "second")
	}
}

func TestWrapChainError(t *testing.T) {
	root := New("connection refused")
	err := Wrapf(Wrap(Wrap(root, "dial"), "db"), "user %d", 42)
	if got, want := err.Error(), "user 42: db: dial: connection refused"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}

	// A foreign wrapper in the middle ends the fast path correctly.
	mixed := Wrap(fmt.Errorf("mid: %w", Wrap(root, "inner")), "outer")
	if got, want := mixed.Error(), "outer: mid: inner: connection refused"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}

	// Empty messages are kept as is, like fmt.Errorf("%s: %w", "", err).
	if got, want := Wrap(root, "").Error(), ": connection refused"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestWrapChainErrorAllocatesOnce(t *testing.T) {
	err := error(stderrors.New("root"))
	for i := 0; i < 20; i++ {
		err = Wrap(err, "layer")
	}
	if n := testing.AllocsPerRun(100, func() { _ = err.Error() }); n != 1 {
		t.Fatalf("Error() on a 20-level chain: %v allocs, want 1", n)
	}
}

func TestFormatFlagsAndVerbs(t *testing.T) {
	w := Wrap(New("cause"), "ctx")
	m := Join(New("a"), New("b"))

	tests := []struct {
		format string
		arg    error
		want   string
	}{
		{"%s", w, "ctx: cause"},
		{"%v", w, "ctx: cause"},
		{"%+v", w, "ctx: cause"},
		{"%q", w, `"ctx: cause"`},
		{"%x", w, fmt.Sprintf("%x", "ctx: cause")},
		{"%X", w, fmt.Sprintf("%X", "ctx: cause")},
		{"%12s|", w, "  ctx: cause|"},
		{"%-12s|", w, "ctx: cause  |"},
		{"%.3s", w, "ctx"},
		{"%s", m, m.Error()},
		{"%q", m, fmt.Sprintf("%q", m.Error())},
		{"%x", m, fmt.Sprintf("%x", m.Error())},
		{"%+v", m, "  - a\n  - b"},
	}
	for _, tt := range tests {
		if got := fmt.Sprintf(tt.format, tt.arg); got != tt.want {
			t.Errorf("Sprintf(%q) = %q, want %q", tt.format, got, tt.want)
		}
	}
}

func TestOneLine(t *testing.T) {
	timeout := New("timeout")
	refused := New("connection refused")

	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"plain", timeout, "timeout"},
		{"wrap", Wrap(timeout, "fetch"), "fetch: timeout"},
		{"multi", Join(timeout, refused), "timeout; connection refused"},
		{
			"wrapped multi",
			Wrap(Join(timeout, Wrap(refused, "dial")), "sync failed"),
			"sync failed: timeout; dial: connection refused",
		},
		{"stdlib join", stderrors.Join(timeout, refused), "timeout; connection refused"},
		{"fmt.Errorf wrap of multi", fmt.Errorf("ctx: %w", Join(timeout, refused)), "ctx: timeout; connection refused"},
		{"foreign message with newlines", New("line one\n\n  line two  \r\nline three"), "line one; line two; line three"},
		{"wrap message with newline", Wrap(timeout, "a\nb"), "a; b: timeout"},
		{"prefix", Prefix(Join(timeout, refused), "backup"), "backup: timeout; backup: connection refused"},
		{"fmt wrapper whose message does not end with the cause", fmt.Errorf("%w (retrying)", timeout), "timeout (retrying)"},
		{"empty foreign join", emptyJoin{}, "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OneLine(tt.err); got != tt.want {
				t.Errorf("OneLine = %q, want %q", got, tt.want)
			}
		})
	}
}

type emptyJoin struct{}

func (emptyJoin) Error() string   { return "empty" }
func (emptyJoin) Unwrap() []error { return []error{nil} }

func TestOneLinePathological(t *testing.T) {
	// selfJoin (regression_test.go) returns itself twice from Unwrap.
	// Foreign multi-errors are rendered by their own message, so this
	// must not recurse at all.
	if got := OneLine(&selfJoin{}); got != "self" {
		t.Fatalf("OneLine(selfJoin) = %q, want %q", got, "self")
	}

	// A chain deeper than the limit is truncated.
	deep := New("root")
	for i := 0; i < 150; i++ {
		deep = Wrap(deep, "w")
	}
	got := OneLine(deep)
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncated output, got %d bytes ending %q", len(got), got[max(0, len(got)-20):])
	}
	if strings.ContainsAny(got, "\r\n") {
		t.Fatal("OneLine produced a line break")
	}

	// Cyclic single-wrap chain from another package.
	a := &cyclicErr{msg: "a"}
	b := &cyclicErr{msg: "b", next: a}
	a.next = b
	if got := OneLine(a); got == "" || len(got) > 10_000 {
		t.Fatalf("OneLine on a cycle returned %d bytes", len(got))
	}
}

func TestWithMessageIsAppendMessage(t *testing.T) {
	base := New("original")
	if got, want := WithMessage(base, "extra").Error(), AppendMessage(base, "extra").Error(); got != want {
		t.Fatalf("WithMessage = %q, AppendMessage = %q", got, want)
	}
}

// Regression (found by fuzzing): Error of a Wrap chain must not compare
// error values, which panics for non-comparable root causes.
func TestWrapChainNonComparableRoot(t *testing.T) {
	root := validationErrors{"name is required"}
	err := Wrap(Wrap(root, "validate"), "create user")
	want := "create user: validate: " + root.Error()
	if got := err.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if got := OneLine(err); got != want {
		t.Fatalf("OneLine = %q, want %q", got, want)
	}
}
