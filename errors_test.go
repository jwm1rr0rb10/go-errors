package errors_test

import (
	"fmt"
	"testing"

	"github.com/jwm1rr0rb10/go-errors"
)

func ExampleWrapf() {
	err := errors.New("connection refused")
	err = errors.Wrapf(err, "failed to dial %s:%d", "db.example.com", 5432)
	fmt.Println(err)
	// Output: failed to dial db.example.com:5432: connection refused
}

func ExamplePrefix() {
	err1 := errors.New("permission denied")
	err2 := errors.New("disk full")
	err := errors.Append(err1, err2)
	err = errors.Prefix(err, "backup failed")
	fmt.Println(err)
	// Output:
	// 2 errors occurred:
	//   - backup failed: permission denied
	//   - backup failed: disk full
}

func ExampleErrors() {
	err := errors.Append(
		errors.New("validation failed"),
		errors.New("user already exists"),
	)
	for _, e := range errors.Errors(err) {
		fmt.Println(e)
	}
	// Output:
	// validation failed
	// user already exists
}

func TestCombineAndFlatten(t *testing.T) {
	t.Run("nil handling", func(t *testing.T) {
		if got := errors.Append(nil, nil, nil); got != nil {
			t.Error("expected nil")
		}
	})

	t.Run("Flatten single error", func(t *testing.T) {
		err := errors.New("boom")
		if got := errors.Flatten(errors.Append(err)); got.Error() != "boom" {
			t.Error("Flatten should return the single error")
		}
	})
}
