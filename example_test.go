package errors_test

import (
	stderrors "errors"
	"fmt"

	"github.com/jwm1rr0rb10/go-errors"
)

func ExampleAppend_loop() {
	var err error
	for _, name := range []string{"alice", "", "bob", ""} {
		if name == "" {
			// Amortized O(1) per call, safe for thousands of errors.
			err = errors.Append(err, errors.New("empty name"))
		}
	}
	fmt.Println(errors.Count(err))
	fmt.Println(err)
	// Output:
	// 2
	// 2 errors occurred:
	//   - empty name
	//   - empty name
}

func ExampleLeaves_wrappedMultiError() {
	timeout := errors.New("timeout")
	refused := errors.New("connection refused")

	// A multi-error wrapped with context: Errors sees one item,
	// Leaves finds every root cause.
	err := errors.Wrap(errors.Join(timeout, errors.Wrap(refused, "dial")), "sync failed")

	fmt.Println(len(errors.Errors(err)))
	for _, leaf := range errors.Leaves(err) {
		fmt.Println(leaf)
	}
	// Output:
	// 1
	// timeout
	// connection refused
}

func ExampleJoin_flattening() {
	a, b, c := errors.New("a"), errors.New("b"), errors.New("c")

	nested := stderrors.Join(stderrors.Join(a, b), c)
	fmt.Println(errors.Count(nested), errors.Count(errors.Join(nested)))
	// Output: 3 3
}

// validationErrors is a slice type, like validator.ValidationErrors.
type validationErrors []string

func (v validationErrors) Error() string { return fmt.Sprint([]string(v)) }

func ExampleAs_nonComparableErrors() {
	err := errors.Append(validationErrors{"name is required"}, errors.New("db down"))

	var ve validationErrors
	fmt.Println(errors.As(err, &ve), ve)
	// Output: true [name is required]
}
