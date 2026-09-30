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

var sinkString string

func wrapChain(depth int) error {
	err := New("connection refused")
	for i := 0; i < depth; i++ {
		err = Wrap(err, "handler step")
	}
	return err
}

func fmtChain(depth int) error {
	err := New("connection refused")
	for i := 0; i < depth; i++ {
		err = fmt.Errorf("handler step: %w", err)
	}
	return err
}

func BenchmarkErrorStringWrapChain5(b *testing.B) {
	err := wrapChain(5)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkString = err.Error()
	}
}

func BenchmarkErrorStringWrapChain20(b *testing.B) {
	err := wrapChain(20)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkString = err.Error()
	}
}

// Typical request path: build a 5-level chain and log it once.
func BenchmarkCreateAndLogWrap5(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkString = wrapChain(5).Error()
	}
}

func BenchmarkStdCreateAndLogFmt5(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkString = fmtChain(5).Error()
	}
}

// Error is only checked with errors.Is and never printed.
func BenchmarkCreateAndIsWrap5(b *testing.B) {
	target := New("target")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Is(wrapChain(5), target)
	}
}

func BenchmarkStdCreateAndIsFmt5(b *testing.B) {
	target := New("target")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Is(fmtChain(5), target)
	}
}

// Recommended hot-path pattern: collect into a slice, Join once.
func BenchmarkCollectThenJoin1000(b *testing.B) {
	errs := make([]error, 1000)
	for i := range errs {
		errs[i] = fmt.Errorf("e%d", i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var acc []error
		//lint:ignore S1011 simulates collecting errors one at a time
		for _, e := range errs {
			acc = append(acc, e)
		}
		sinkErr = Join(acc...)
	}
}

func BenchmarkOneLine(b *testing.B) {
	err := Wrap(Join(New("timeout"), Wrap(New("connection refused"), "dial")), "sync failed")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkString = OneLine(err)
	}
}
