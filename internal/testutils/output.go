package testutils

import (
	"fmt"
	"os"
	"testing"
	"text/tabwriter"
)

// TestCase represents a single unit test scenario.
type TestCase struct {
	Name     string
	Input    string
	Expected string
	Actual   string
	Pass     bool
}

// PrintTestTable prints a formatted table of comparison results, mimicking the legacy Bash output.
// It fails the test if any case has Pass=false.
func PrintTestTable(t *testing.T, cases []TestCase) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)

	// Define colors/styles roughly matching legacy output
	// Using ANSI codes directly for simplicity in test output
	const (
		Reset = "\033[0m"
		Red   = "\033[31m"
		Green = "\033[32m"
		Blue  = "\033[34m"
	)

	// Header
	fmt.Fprintf(w, "Input\tExpected Value\tReturned Value\t\n")

	anyFailed := false

	for _, tc := range cases {
		inputColor := Reset
		expectedColor := Reset
		actualColor := Green
		leftPtr := " "
		rightPtr := " "

		if !tc.Pass {
			anyFailed = true
			inputColor = Red
			expectedColor = Red
			actualColor = Red
			leftPtr = Red + ">" + Reset
			rightPtr = Red + "<" + Reset
		}

		// Format columns similar to bash: input | expected | returned
		// We add the pointers to the left and right visually?
		// The bash table was: LeftPointer Input Expected Returned RightPointer
		// effectively: Ptr "Input" "Expected" "Returned" Ptr

		fmt.Fprintf(w, "%s %s%s%s\t%s%s%s\t%s%s%s\t%s\n",
			leftPtr,
			inputColor, tc.Input, Reset,
			expectedColor, tc.Expected, Reset,
			actualColor, tc.Actual, Reset,
			rightPtr,
		)
	}

	w.Flush()
	fmt.Println() // Newline after table

	if anyFailed {
		t.Fail()
	}
}
