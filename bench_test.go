package errors

import (
	stderrors "errors"
	"fmt"
	"testing"
)

var sinkErr error
var sinkErrs []error

func BenchmarkWrap(b *testing.B) {
	base := New("base")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkErr = Wrap(base, "ctx")
	}
}

func BenchmarkWrapf(b *testing.B) {
	base := New("base")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkErr = Wrapf(base, "user %d", 42)
	}
}

func BenchmarkStdFmtErrorfWrap(b *testing.B) {
	base := New("base")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkErr = fmt.Errorf("ctx: %w", base)
	}
}

func BenchmarkAppendTwo(b *testing.B) {
	x, y := New("x"), New("y")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkErr = Append(x, y)
	}
}

func BenchmarkStdJoinTwo(b *testing.B) {
	x, y := New("x"), New("y")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkErr = stderrors.Join(x, y)
	}
}

// Appending one error at a time to an accumulator, 1000 errors per op.
func BenchmarkAppendLoop1000(b *testing.B) {
	errs := make([]error, 1000)
	for i := range errs {
		errs[i] = fmt.Errorf("e%d", i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		for _, e := range errs {
			err = Append(err, e)
		}
		sinkErr = err
	}
}

func BenchmarkLeavesWrappedChain(b *testing.B) {
	err := Wrap(Wrap(Wrap(New("root"), "l1"), "l2"), "l3")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkErrs = Leaves(err)
	}
}

func BenchmarkLeavesMulti(b *testing.B) {
	err := Append(Wrap(Wrap(New("root"), "l1"), "l2"), New("other"), Wrap(New("x"), "y"))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkErrs = Leaves(err)
	}
}

func BenchmarkCount(b *testing.B) {
	err := Append(New("a"), New("b"), New("c"), New("d"), New("e"))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Count(err)
	}
}

func BenchmarkErrorString(b *testing.B) {
	err := Append(New("a"), New("b"), New("c"))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = err.Error()
	}
}
