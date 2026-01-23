package utils

import "testing"

// RequireNoError fails the test immediately if err is not nil.
func RequireNoError(t *testing.T, err error, msg string) {
	t.Helper()
	if err != nil {
		if msg == "" {
			t.Fatalf("unexpected error: %v", err)
		}
		t.Fatalf("%s: %v", msg, err)
	}
}
