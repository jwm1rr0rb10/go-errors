package errors

import (
	"fmt"
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
