package errors

import (
	stderrors "errors"
	"fmt"
	"reflect"
	"testing"
)

func FuzzJoinAppendFlatten(f *testing.F) {
	// Seed with various flag combinations
	f.Add(byte(0), byte(1), byte(2), "msg1", "msg2", "msg3")
	f.Add(byte(1), byte(0), byte(1), "a", "b", "c")
	f.Add(byte(255), byte(128), byte(0), "", "empty", "x")
	f.Add(byte(3), byte(5), byte(7), "wrap", "nested", "deep")

	f.Fuzz(func(t *testing.T, a, b, c byte, msg1, msg2, msg3 string) {
		// Limit message length to keep fuzzing reasonable
		if len(msg1) > 64 {
			msg1 = msg1[:64]
		}
		if len(msg2) > 64 {
			msg2 = msg2[:64]
		}
		if len(msg3) > 64 {
			msg3 = msg3[:64]
		}

		var errs [3]error
		msgs := []string{msg1, msg2, msg3}
		flags := []byte{a, b, c}

		for i, flag := range flags {
			if flag%2 == 0 {
				base := New(msgs[i])
				// Occasionally wrap to create chains
				if flag%4 == 0 && msgs[i] != "" {
					base = Wrap(base, fmt.Sprintf("wrap-%d", i))
				}
				if flag%8 == 0 {
					base = Wrapf(base, "fmt-%d-%s", i, msgs[i])
				}
				errs[i] = base
			}
			// else leave nil
		}

		// Mix Join and Append in different nesting orders
		joined := Join(errs[0], Join(errs[1], errs[2]))
		appended := Append(errs[0], Append(errs[1], errs[2]))

		// Also test with different nesting
		joined2 := Join(Join(errs[0], errs[1]), errs[2])
		appended2 := Append(Append(errs[0], errs[1]), errs[2])

		flatJoined := Flatten(joined)
		flatAppended := Flatten(appended)

		if (joined == nil) != (appended == nil) {
			t.Fatalf("Join and Append nil mismatch: join=%v append=%v", joined, appended)
		}

		if joined != nil && appended != nil && joined.Error() != appended.Error() {
			t.Fatalf("Join and Append should match: join=%q append=%q", joined.Error(), appended.Error())
		}

		if Count(joined) != Count(appended) {
			t.Fatalf("count mismatch: join=%d append=%d", Count(joined), Count(appended))
		}

		if Count(joined2) != Count(appended2) {
			t.Fatalf("nested count mismatch: join2=%d append2=%d", Count(joined2), Count(appended2))
		}

		if flatJoined != nil && flatAppended != nil && flatJoined.Error() != flatAppended.Error() {
			t.Fatalf("flatten mismatch: join=%q append=%q", flatJoined.Error(), flatAppended.Error())
		}

		// Leaves must never contain nil and must be consistent
		leaves := Leaves(joined)
		for _, leaf := range leaves {
			if leaf == nil {
				t.Fatal("Leaves must not contain nil")
			}
		}

		// Prefix and Flatten should not panic
		if joined != nil {
			_ = Prefix(joined, "pfx")
			_ = Flatten(Prefix(joined, "pfx"))
		}

		// WithMessage should work
		_ = WithMessage(joined, "extra")
	})
}

// buildTree turns fuzz bytes into an arbitrary error tree mixing comparable
// and non-comparable error types, stdlib joins, fmt wrapping and this
// package's multi-errors.
func buildTree(data []byte, pos *int, depth int) error {
	if *pos >= len(data) || depth > 6 {
		return nil
	}
	op := data[*pos]
	*pos++
	switch op % 10 {
	case 0:
		return nil
	case 1:
		return New(fmt.Sprintf("leaf-%d", op))
	case 2:
		return validationErrors{fmt.Sprintf("v-%d", op)} // slice: not comparable
	case 3:
		return fieldErr{fields: map[string]string{"k": "v"}} // map inside
	case 4:
		return Wrap(buildTree(data, pos, depth+1), "wrap")
	case 5:
		return fmt.Errorf("fmt: %w", buildTree(data, pos, depth+1))
	case 6:
		return stderrors.Join(buildTree(data, pos, depth+1), buildTree(data, pos, depth+1))
	case 7:
		return Join(buildTree(data, pos, depth+1), buildTree(data, pos, depth+1))
	case 8:
		return Append(buildTree(data, pos, depth+1), buildTree(data, pos, depth+1), buildTree(data, pos, depth+1))
	default:
		inner := buildTree(data, pos, depth+1)
		if inner == nil {
			return nil
		}
		return valueWrapper{err: inner} // comparable struct holding any error
	}
}

func FuzzErrorTrees(f *testing.F) {
	f.Add([]byte{7, 2, 3})
	f.Add([]byte{4, 7, 1, 2})
	f.Add([]byte{6, 7, 1, 1, 7, 1, 1})
	f.Add([]byte{9, 8, 2, 2, 3})
	f.Add([]byte{5, 6, 8, 1, 2, 3, 9, 7, 2, 3})

	f.Fuzz(func(t *testing.T, data []byte) {
		pos := 0
		a := buildTree(data, &pos, 0)
		b := buildTree(data, &pos, 0)
		c := buildTree(data, &pos, 0)

		joined := Join(a, b, c)
		appended := Append(Append(a, b), c)

		// Join and Append agree, and nothing is lost or deduplicated.
		want := Count(a) + Count(b) + Count(c)
		if Count(joined) != want || Count(appended) != want {
			t.Fatalf("count: join=%d append=%d want=%d", Count(joined), Count(appended), want)
		}
		if (joined == nil) != (want == 0) {
			t.Fatalf("nil mismatch: joined=%v want=%d", joined, want)
		}
		if joined != nil && joined.Error() != appended.Error() {
			t.Fatalf("message mismatch:\n%q\n%q", joined.Error(), appended.Error())
		}

		// Every top-level item of every input (multi-errors are flattened)
		// is still found by errors.Is, if it can be compared at all.
		for _, in := range []error{a, b, c} {
			for _, e := range Errors(in) {
				if reflect.TypeOf(e).Comparable() && isSafeComparable(e) && !Is(joined, e) {
					t.Fatalf("errors.Is lost %v", e)
				}
			}
		}
		var ve validationErrors
		if hasType[validationErrors](a, b, c) && !As(joined, &ve) {
			t.Fatal("errors.As lost validationErrors")
		}

		// None of these may panic, and results never contain nil.
		for _, l := range Leaves(joined) {
			if l == nil {
				t.Fatal("nil leaf")
			}
			if Count(l) > 1 {
				t.Fatalf("leaf is a multi-error: %v", l)
			}
		}
		for _, e := range Errors(joined) {
			if e == nil {
				t.Fatal("nil in Errors")
			}
		}
		_ = Flatten(joined)
		_ = Prefix(joined, "p")
		_ = WithMessage(joined, "m")
		_ = fmt.Sprintf("%v %+v %s %q", joined, joined, joined, joined)
	})
}

// isSafeComparable reports whether e == e cannot panic (comparable type
// whose interface fields do not hold non-comparable values).
func isSafeComparable(e error) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return e == e
}

func hasType[T error](errs ...error) bool {
	for _, e := range errs {
		var target T
		if e != nil && stderrors.As(e, &target) {
			return true
		}
	}
	return false
}
