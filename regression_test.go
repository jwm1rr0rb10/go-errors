package errors

import (
	stderrors "errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// validationErrors is a slice-based error type, like
// github.com/go-playground/validator's ValidationErrors. Its values are not
// comparable, so using them as map keys panics.
type validationErrors []string

func (v validationErrors) Error() string { return strings.Join(v, ", ") }

// fieldErr is a value type with a map inside: not comparable either.
type fieldErr struct{ fields map[string]string }

func (fieldErr) Error() string { return "field error" }

// valueWrapper is a comparable struct type whose interface field may hold a
// non-comparable value: == on two such values can panic at run time.
type valueWrapper struct{ err error }

func (w valueWrapper) Error() string { return "wrapped: " + w.err.Error() }
func (w valueWrapper) Unwrap() error { return w.err }

func mustNotPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%s panicked: %v", name, r)
		}
	}()
	fn()
}

func TestNonComparableErrorsDoNotPanic(t *testing.T) {
	a, b := New("a"), New("b")
	v := validationErrors{"name is required"}
	f := fieldErr{fields: map[string]string{"x": "y"}}
	vw := valueWrapper{err: v}

	cases := map[string]func(){
		"Leaves(Wrap(Join(a,b)))":           func() { _ = Leaves(Wrap(Join(a, b), "ctx")) },
		"Join(stdlib.Join(Join(a,b),c))":    func() { _ = Join(stderrors.Join(Join(a, b), New("c"))) },
		"Append(slice error, a)":            func() { _ = Append(v, a) },
		"Append(a, slice error, slice)":     func() { _ = Append(a, v, v) },
		"Join(a, map-struct error)":         func() { _ = Join(a, f) },
		"Flatten(stdlib.Join(slice, a))":    func() { _ = Flatten(stderrors.Join(v, a)) },
		"Leaves(Wrap(slice error))":         func() { _ = Leaves(Wrap(v, "ctx")) },
		"Leaves(value wrapper x2)":          func() { _ = Leaves(Join(vw, valueWrapper{err: vw})) },
		"Prefix(Join(slice, map-struct))":   func() { _ = Prefix(Join(v, f), "p") },
		"Count/Errors(Join(slice, slice))":  func() { _ = Count(Join(v, v)) + len(Errors(Join(v, v))) },
		"WithMessage(slice error)":          func() { _ = WithMessage(v, "m") },
		"IsAny/AsAny with slice error":      func() { var x validationErrors; _ = IsAny(Join(v, a), a) && AsAny(Join(v, a), &x) },
		"Leaves(stdlib.Join(multi, multi))": func() { _ = Leaves(stderrors.Join(Join(a, b), Join(a, b))) },
	}
	for name, fn := range cases {
		mustNotPanic(t, name, fn)
	}

	var got validationErrors
	if !As(Append(a, v), &got) || len(got) != 1 {
		t.Fatal("errors.As must find the slice-based error inside a multi-error")
	}
}

func TestLeavesWalksNestedMultiErrors(t *testing.T) {
	a, b, c := New("a"), New("b"), New("c")

	got := Leaves(Wrap(stderrors.Join(Wrap(a, "x"), b), "ctx"))
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("want [a b] through a wrapped stdlib join, got %q", got)
	}

	got = Leaves(Append(Wrap(Join(a, Wrap(Join(b, c), "inner")), "outer"), New("d")))
	if len(got) != 4 || got[0] != a || got[1] != b || got[2] != c || got[3].Error() != "d" {
		t.Fatalf("want [a b c d] at any depth, got %q", got)
	}
}

// selfJoin returns itself from Unwrap() []error: a pathological type that
// would recurse forever (and exponentially) without protection.
type selfJoin struct{ n int }

func (s *selfJoin) Error() string   { return "self" }
func (s *selfJoin) Unwrap() []error { return []error{s, s} }

func TestLeavesTerminatesOnPathologicalGraphs(t *testing.T) {
	done := make(chan []error, 1)
	go func() { done <- Leaves(&selfJoin{}) }()
	select {
	case got := <-done:
		if len(got) == 0 {
			t.Fatal("Leaves must return something for a cyclic multi-error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Leaves did not terminate on a cyclic multi-error")
	}
}

func TestFlattensNestedJoinsAtAnyDepth(t *testing.T) {
	a, b, c := New("a"), New("b"), New("c")
	nested := stderrors.Join(stderrors.Join(Join(a, b)), c)
	if got := Errors(nested); len(got) != 3 || got[0] != a || got[2] != c {
		t.Fatalf("want [a b c], got %q", got)
	}
	if Count(nested) != 3 || Count(Join(stderrors.Join(Join(a, b)))) != 2 {
		t.Fatal("Count must match Errors for nested joins")
	}
	// Wrapped multi-errors stay one item for Errors (Leaves goes inside).
	if Count(Join(Wrap(Join(a, b), "x"), c)) != 2 {
		t.Fatal("a wrapped multi-error is one item")
	}
}

func TestFlattenTerminatesOnPathologicalGraphs(t *testing.T) {
	done := make(chan int, 1)
	go func() { done <- Count(Join(&selfJoin{}, New("x"))) }()
	select {
	case n := <-done:
		if n == 0 {
			t.Fatal("expected a bounded, non-empty result")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Join did not terminate on a cyclic multi-error")
	}
}

func TestNoDeduplication(t *testing.T) {
	notFound := New("not found")
	if n := Count(Join(notFound, notFound)); n != 2 {
		t.Fatalf("Join must keep every error like errors.Join; Count = %d", n)
	}
	if n := Count(Append(Append(notFound, notFound), notFound)); n != 3 {
		t.Fatalf("Append must keep every error; Count = %d", n)
	}
}

func TestAppendInLoopIsLinear(t *testing.T) {
	const n = 20000
	start := time.Now()
	var err error
	for i := 0; i < n; i++ {
		err = Append(err, fmt.Errorf("e%d", i))
	}
	elapsed := time.Since(start)
	if Count(err) != n {
		t.Fatalf("want %d errors, got %d", n, Count(err))
	}
	// v1.0.2 needed ~25s for 20k appends (quadratic); linear takes a few ms.
	if elapsed > time.Second {
		t.Fatalf("Append in a loop is too slow: %v for %d appends", elapsed, n)
	}
}

func TestAppendDoesNotAliasSharedBase(t *testing.T) {
	base := Append(New("a"), New("b"))
	x := Append(base, New("x"))
	y := Append(base, New("y")) // must not overwrite x's last element

	if got := Errors(x); len(got) != 3 || got[2].Error() != "x" {
		t.Fatalf("x corrupted: %q", got)
	}
	if got := Errors(y); len(got) != 3 || got[2].Error() != "y" {
		t.Fatalf("y wrong: %q", got)
	}
	if Count(base) != 2 {
		t.Fatalf("base must not change, got %d", Count(base))
	}
}

func TestAppendConcurrentFromSharedBase(t *testing.T) {
	base := Append(New("a"), New("b"))
	var wg sync.WaitGroup
	results := make([]error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = Append(base, fmt.Errorf("g%d", i))
		}(i)
	}
	wg.Wait()
	for i, r := range results {
		got := Errors(r)
		if len(got) != 3 || got[2].Error() != fmt.Sprintf("g%d", i) {
			t.Fatalf("result %d corrupted: %q", i, got)
		}
	}
}

func TestErrorsReturnsCopy(t *testing.T) {
	err := Append(New("a"), New("b"))
	Errors(err)[0] = New("mutated")
	if Errors(err)[0].Error() != "a" {
		t.Fatal("mutating the result of Errors must not change the error")
	}
}

func TestFormatVerbs(t *testing.T) {
	m := Join(New("a"), New("b"))
	if got := fmt.Sprintf("%q", m); got != fmt.Sprintf("%q", m.Error()) {
		t.Fatalf("%%q must quote the message, got %s", got)
	}
	if got := fmt.Sprintf("%s", m); got != m.Error() {
		t.Fatalf("%%s = %q", got)
	}

	w := Wrap(New("root"), "ctx")
	if got := fmt.Sprintf("%v|%s|%q", w, w, w); got != `ctx: root|ctx: root|"ctx: root"` {
		t.Fatalf("wrap format: %s", got)
	}
	// %+v is passed down so stack-trace formatters of wrapped errors work.
	inner := verboseErr{}
	if got := fmt.Sprintf("%+v", Wrap(inner, "ctx")); got != "ctx: VERBOSE" {
		t.Fatalf("%%+v must reach the wrapped error, got %q", got)
	}
}

type verboseErr struct{}

func (verboseErr) Error() string { return "plain" }
func (verboseErr) Format(s fmt.State, verb rune) {
	if verb == 'v' && s.Flag('+') {
		fmt.Fprint(s, "VERBOSE")
		return
	}
	fmt.Fprint(s, "plain")
}

func TestErrUnsupportedReexported(t *testing.T) {
	if ErrUnsupported != stderrors.ErrUnsupported {
		t.Fatal("ErrUnsupported must be the stdlib sentinel")
	}
}
