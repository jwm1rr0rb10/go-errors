package errors

import (
	stderrors "errors"
	"fmt"
	"io/fs"
	"reflect"
	"testing"
)

type timeoutErr struct{ op string }

func (e *timeoutErr) Error() string { return e.op + ": timeout" }
func (e *timeoutErr) Timeout() bool { return true }

type timeouter interface {
	error
	Timeout() bool
}

// asMethodErr matches *timeoutErr through an As method, without being one.
type asMethodErr struct{}

func (asMethodErr) Error() string { return "as-method" }
func (asMethodErr) As(target any) bool {
	if p, ok := target.(**timeoutErr); ok {
		*p = &timeoutErr{op: "from As"}
		return true
	}
	return false
}

func TestAsType(t *testing.T) {
	te := &timeoutErr{op: "dial"}
	ve := validationErrors{"name is required"}
	pe := &fs.PathError{Op: "open", Path: "/tmp/x", Err: fs.ErrNotExist}

	t.Run("nil", func(t *testing.T) {
		if got, ok := AsType[*timeoutErr](nil); ok || got != nil {
			t.Fatalf("AsType(nil) = %v, %v", got, ok)
		}
	})
	t.Run("direct", func(t *testing.T) {
		if got, ok := AsType[*timeoutErr](te); !ok || got != te {
			t.Fatalf("got %v, %v", got, ok)
		}
	})
	t.Run("not found returns zero value", func(t *testing.T) {
		got, ok := AsType[*timeoutErr](Wrap(New("x"), "y"))
		if ok || got != nil {
			t.Fatalf("got %v, %v", got, ok)
		}
	})
	t.Run("through Wrap and fmt.Errorf", func(t *testing.T) {
		err := Wrap(fmt.Errorf("mid: %w", Wrapf(te, "user %d", 1)), "outer")
		if got, ok := AsType[*timeoutErr](err); !ok || got != te {
			t.Fatalf("got %v, %v", got, ok)
		}
	})
	t.Run("inside multi-error, first match wins", func(t *testing.T) {
		other := &timeoutErr{op: "read"}
		err := Join(New("a"), Wrap(te, "ctx"), other)
		if got, ok := AsType[*timeoutErr](err); !ok || got != te {
			t.Fatalf("got %v, %v; want the first match", got, ok)
		}
	})
	t.Run("inside wrapped stdlib join", func(t *testing.T) {
		err := Wrap(stderrors.Join(nil, New("a"), te), "ctx")
		if got, ok := AsType[*timeoutErr](err); !ok || got != te {
			t.Fatalf("got %v, %v", got, ok)
		}
	})
	t.Run("interface type", func(t *testing.T) {
		got, ok := AsType[timeouter](Wrap(te, "ctx"))
		if !ok || got != te || !got.Timeout() {
			t.Fatalf("got %v, %v", got, ok)
		}
	})
	t.Run("non-comparable value type", func(t *testing.T) {
		got, ok := AsType[validationErrors](Join(New("db down"), Wrap(ve, "validate")))
		if !ok || !reflect.DeepEqual(got, ve) {
			t.Fatalf("got %v, %v", got, ok)
		}
	})
	t.Run("As method", func(t *testing.T) {
		got, ok := AsType[*timeoutErr](Wrap(asMethodErr{}, "ctx"))
		if !ok || got == nil || got.op != "from As" {
			t.Fatalf("got %v, %v", got, ok)
		}
	})
	t.Run("stdlib error type", func(t *testing.T) {
		got, ok := AsType[*fs.PathError](Wrap(pe, "load config"))
		if !ok || got != pe {
			t.Fatalf("got %v, %v", got, ok)
		}
	})
	t.Run("nil items in multi-errors are skipped", func(t *testing.T) {
		if _, ok := AsType[*timeoutErr](stderrors.Join(nil, nil)); ok {
			t.Fatal("found a match in an empty join")
		}
	})
}

// AsType must agree with errors.As on the same tree.
func TestAsTypeMatchesStdlibAs(t *testing.T) {
	te := &timeoutErr{op: "dial"}
	trees := []error{
		te,
		Wrap(te, "a"),
		Join(New("x"), te),
		stderrors.Join(New("x"), Wrap(asMethodErr{}, "y")),
		fmt.Errorf("%w and %w", New("x"), te),
		valueWrapper{err: Join(validationErrors{"v"}, te)},
		New("nothing here"),
	}
	for i, tree := range trees {
		checkAsTypeAgrees[*timeoutErr](t, i, tree)
		checkAsTypeAgrees[timeouter](t, i, tree)
		checkAsTypeAgrees[validationErrors](t, i, tree)
		checkAsTypeAgrees[valueWrapper](t, i, tree)
	}
}

func checkAsTypeAgrees[E error](t *testing.T, id any, err error) {
	t.Helper()
	var want E
	wantOK := stderrors.As(err, &want)
	got, gotOK := AsType[E](err)
	if gotOK != wantOK || !reflect.DeepEqual(got, want) {
		var e E
		t.Fatalf("tree %v, type %T: AsType = (%v, %v), errors.As = (%v, %v)", id, e, got, gotOK, want, wantOK)
	}
}

func TestAsTypeDoesNotAllocate(t *testing.T) {
	te := &timeoutErr{op: "dial"}
	err := Wrap(Join(New("a"), Wrap(te, "b")), "c")
	allocs := testing.AllocsPerRun(100, func() {
		if _, ok := AsType[*timeoutErr](err); !ok {
			t.Fatal("not found")
		}
	})
	if allocs != 0 {
		t.Fatalf("AsType allocated %v times, want 0", allocs)
	}
}

func BenchmarkAsType(b *testing.B) {
	err := Wrap(Join(New("a"), Wrap(&timeoutErr{op: "dial"}, "b")), "c")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = AsType[*timeoutErr](err)
	}
}

func BenchmarkStdErrorsAs(b *testing.B) {
	err := Wrap(Join(New("a"), Wrap(&timeoutErr{op: "dial"}, "b")), "c")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var target *timeoutErr
		_ = stderrors.As(err, &target)
	}
}
