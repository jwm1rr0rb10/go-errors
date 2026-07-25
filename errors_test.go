package errors

import (
	stderrors "errors"
	"fmt"
	"strings"
	"testing"
)

type customErr struct{ msg string }

func (e *customErr) Error() string { return e.msg }

func ExampleWrapf() {
	err := New("connection refused")
	err = Wrapf(err, "failed to dial %s:%d", "db.example.com", 5432)
	fmt.Println(err)
	// Output: failed to dial db.example.com:5432: connection refused
}

func ExamplePrefix() {
	err1 := New("permission denied")
	err2 := New("disk full")
	err := Append(err1, err2)
	err = Prefix(err, "backup failed")
	fmt.Println(err)
	// Output:
	// 2 errors occurred:
	//   - backup failed: permission denied
	//   - backup failed: disk full
}

func ExampleErrors() {
	err := Append(
		New("validation failed"),
		New("user already exists"),
	)
	for _, e := range Errors(err) {
		fmt.Println(e)
	}
	// Output:
	// validation failed
	// user already exists
}

func ExampleLeaves() {
	root := New("connection refused")
	wrapped := Wrap(root, "dial failed")
	err := Append(wrapped, New("disk full"))

	for _, leaf := range Leaves(err) {
		fmt.Println(leaf)
	}
	// Output:
	// connection refused
	// disk full
}

func TestAppendNilHandling(t *testing.T) {
	if got := Append(nil, nil, nil); got != nil {
		t.Fatal("expected nil")
	}
}

func TestJoinNilHandling(t *testing.T) {
	if got := Join(nil, nil); got != nil {
		t.Fatal("expected nil")
	}
}

func TestWrapNil(t *testing.T) {
	if got := Wrap(nil, "context"); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
	if got := Wrapf(nil, "context %s", "x"); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestPrefixNil(t *testing.T) {
	if got := Prefix(nil, "prefix"); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestErrorfWithWrapVerb(t *testing.T) {
	root := New("root")
	wrapped := Errorf("outer: %w", root)

	if !Is(wrapped, root) {
		t.Fatal("expected Errorf %w chain to preserve root")
	}
}

func TestFlattenSingleError(t *testing.T) {
	err := New("boom")
	if got := Flatten(Append(err)); got.Error() != "boom" {
		t.Fatalf("Flatten should return the single error, got %q", got.Error())
	}
}

func TestFlattenJoinedErrors(t *testing.T) {
	err := Join(New("one"))
	if got := Flatten(err); got.Error() != "one" {
		t.Fatalf("expected flattened single error, got %q", got.Error())
	}
}

func TestAppendFlattensNestedMultiErrors(t *testing.T) {
	err1 := New("first")
	err2 := New("second")
	err3 := New("third")

	combined := Append(err1, Append(err2, err3))
	got := Errors(combined)

	if len(got) != 3 {
		t.Fatalf("expected 3 errors, got %d: %v", len(got), got)
	}
}

func TestJoinFlattensNestedMultiErrors(t *testing.T) {
	err := Join(
		New("a"),
		Join(New("b"), New("c")),
	)

	got := Errors(err)
	if len(got) != 3 {
		t.Fatalf("expected flattened join with 3 errors, got %d: %v", len(got), got)
	}
}

func TestJoinDiffersFromStdlibJoin(t *testing.T) {
	err1 := New("a")
	err2 := New("b")
	err3 := New("c")

	pkgJoined := Join(err1, Join(err2, err3))
	stdJoined := stderrors.Join(err1, stderrors.Join(err2, err3))

	if len(Errors(pkgJoined)) != 3 {
		t.Fatal("package Join should flatten nested joins")
	}

	type multiUnwrapper interface {
		Unwrap() []error
	}

	u, ok := stdJoined.(multiUnwrapper)
	if !ok {
		t.Fatal("expected stdlib join to expose Unwrap() []error")
	}
	if len(u.Unwrap()) != 2 {
		t.Fatal("stdlib Join should keep nested structure")
	}
}

func TestStdlibJoinInterop(t *testing.T) {
	err1 := New("alpha")
	err2 := New("beta")
	stdJoined := stderrors.Join(err1, err2)

	got := Errors(stdJoined)
	if len(got) != 2 {
		t.Fatalf("expected 2 errors from stdlib join, got %d", len(got))
	}

	combined := Append(stdJoined, New("gamma"))
	if len(Errors(combined)) != 3 {
		t.Fatalf("expected append with stdlib join to flatten to 3, got %d", len(Errors(combined)))
	}
}

func TestJoinMatchesAppendBehavior(t *testing.T) {
	err1 := New("alpha")
	err2 := New("beta")

	appendErr := Append(err1, err2)
	joinErr := Join(err1, err2)

	if appendErr.Error() != joinErr.Error() {
		t.Fatalf("Append and Join should format the same:\nappend=%q\njoin=%q", appendErr, joinErr)
	}

	if len(Errors(appendErr)) != len(Errors(joinErr)) {
		t.Fatal("Append and Join should expose the same number of errors")
	}
}

func TestPrefixWorksForJoin(t *testing.T) {
	err := Join(
		New("permission denied"),
		New("disk full"),
	)
	err = Prefix(err, "backup failed")

	output := err.Error()
	if !strings.Contains(output, "backup failed: permission denied") {
		t.Fatalf("unexpected output: %q", output)
	}
	if !strings.Contains(output, "backup failed: disk full") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestWrapAndIs(t *testing.T) {
	root := New("root cause")
	wrapped := Wrap(Wrap(root, "middle"), "outer")

	if !Is(wrapped, root) {
		t.Fatal("expected wrapped error chain to match root cause")
	}
}

func TestErrorsOnWrappedChain(t *testing.T) {
	root := New("root")
	wrapped := Wrap(root, "outer")

	got := Errors(wrapped)
	if len(got) != 1 {
		t.Fatalf("expected one top-level wrapper, got %d", len(got))
	}
	if got[0] != wrapped {
		t.Fatal("Errors should return the outer wrapper, not the root")
	}
}

func TestLeavesOnWrappedChain(t *testing.T) {
	root := New("root")
	wrapped := Wrap(root, "outer")

	got := Leaves(wrapped)
	if len(got) != 1 || got[0] != root {
		t.Fatalf("expected leaf root, got %v", got)
	}
}

func TestLeavesOnMultiError(t *testing.T) {
	root := New("root")
	wrapped := Wrap(root, "outer")
	err := Append(wrapped, New("disk full"))

	got := Leaves(err)
	if len(got) != 2 {
		t.Fatalf("expected 2 leaves, got %d", len(got))
	}
	if got[0] != root || got[1].Error() != "disk full" {
		t.Fatalf("unexpected leaves: %v", got)
	}
}

func TestMultiErrorSupportsStdlibIs(t *testing.T) {
	target := New("target")
	err := Append(New("other"), target)

	if !stderrors.Is(err, target) {
		t.Fatal("expected stdlib errors.Is to work with multi-error")
	}
}

func TestMultiErrorSupportsStdlibAs(t *testing.T) {
	target := &customErr{msg: "typed"}
	err := Append(New("other"), target)

	var extracted *customErr
	if !stderrors.As(err, &extracted) {
		t.Fatal("expected stdlib errors.As to work with multi-error")
	}
	if extracted.msg != "typed" {
		t.Fatalf("unexpected extracted value: %+v", extracted)
	}
}

func TestWithMessage(t *testing.T) {
	err := WithMessage(New("original"), "extra")
	got := Errors(err)

	if len(got) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(got))
	}
}

func TestCount(t *testing.T) {
	if Count(nil) != 0 {
		t.Fatal("expected zero count for nil")
	}

	err := Append(New("one"), Append(New("two"), New("three")))
	if Count(err) != 3 {
		t.Fatalf("expected count 3, got %d", Count(err))
	}
}

func TestMultiErrorVerboseFormat(t *testing.T) {
	err := Append(
		New("first"),
		New("second"),
	)

	formatted := fmt.Sprintf("%+v", err)
	if !strings.Contains(formatted, "first") || !strings.Contains(formatted, "second") {
		t.Fatalf("expected verbose format to include nested errors, got %q", formatted)
	}
}

func TestMultiErrorSingleElementFormat(t *testing.T) {
	err := Append(New("solo"))
	if err.Error() != "solo" {
		t.Fatalf("single-element multi-error should print like plain error, got %q", err.Error())
	}

	formatted := fmt.Sprintf("%+v", err)
	if !strings.Contains(formatted, "solo") {
		t.Fatalf("expected verbose single format, got %q", formatted)
	}
}

func TestWrapfPercentLiteral(t *testing.T) {
	root := New("root")
	wrapped := Wrapf(root, "success rate %d%%", 100)
	if wrapped.Error() != "success rate 100%: root" {
		t.Fatalf("unexpected wrapf output: %q", wrapped.Error())
	}
}

func TestReexportedHelpers(t *testing.T) {
	root := New("root")
	wrapped := Wrap(root, "outer")

	if !Is(wrapped, root) {
		t.Fatal("expected re-exported Is to work")
	}
	if Unwrap(wrapped) == nil {
		t.Fatal("expected re-exported Unwrap to work")
	}

	var extracted *customErr
	target := &customErr{msg: "typed"}
	err := Append(New("other"), target)
	if !As(err, &extracted) {
		t.Fatal("expected re-exported As to work")
	}
}

func TestWithMessageNilErr(t *testing.T) {
	err := WithMessage(nil, "solo")
	if err == nil || err.Error() != "solo" {
		t.Fatalf("expected plain error, got %v", err)
	}
}

func TestLeavesNil(t *testing.T) {
	if got := Leaves(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestMultiErrorPlainFormat(t *testing.T) {
	err := Append(New("first"), New("second"))
	formatted := fmt.Sprintf("%v", err)
	if !strings.Contains(formatted, "2 errors occurred") {
		t.Fatalf("unexpected plain format: %q", formatted)
	}
}

func TestFlattenMultiError(t *testing.T) {
	err := Append(New("one"), New("two"))
	flat := Flatten(err)
	if len(Errors(flat)) != 2 {
		t.Fatalf("expected multi-error unchanged, got %v", Errors(flat))
	}
}

type cyclicErr struct {
	msg  string
	next error
}

func (e *cyclicErr) Error() string { return e.msg }
func (e *cyclicErr) Unwrap() error { return e.next }

func TestLeafErrorCycle(t *testing.T) {
	// Create a cyclic error chain: a unwraps to b, b unwraps to a.
	a := &cyclicErr{msg: "a"}
	b := &cyclicErr{msg: "b", next: a}
	a.next = b

	got := leafError(a)
	if got == nil {
		t.Fatal("leafError returned nil on cyclic chain")
	}
	// It should return one of the cycle members.
	if got != a && got != b {
		t.Fatalf("unexpected leaf: %v", got)
	}
}

func BenchmarkAppend(b *testing.B) {
	errs := make([]error, 10)
	for i := range errs {
		errs[i] = New("err")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Append(errs[0], errs[1:]...)
	}
}

func BenchmarkAppendNested(b *testing.B) {
	var err error
	for i := 0; i < 20; i++ {
		err = Append(err, New("err"))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Append(err, New("extra"))
	}
}

func BenchmarkLeaves(b *testing.B) {
	root := New("root")
	wrapped := Wrap(Wrap(Wrap(root, "l1"), "l2"), "l3")
	err := Append(wrapped, New("other"), Wrap(New("x"), "y"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Leaves(err)
	}
}

func BenchmarkFlatten(b *testing.B) {
	err := Append(New("a"), New("b"), New("c"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Flatten(err)
	}
}

func BenchmarkErrors(b *testing.B) {
	err := Append(New("a"), New("b"), New("c"), New("d"), New("e"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Errors(err)
	}
}
