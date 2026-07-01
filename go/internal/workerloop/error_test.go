package workerloop

import (
	"errors"
	"testing"
)

func TestFatalWrapsNonRetryableLoopErrors(t *testing.T) {
	wantErr := errors.New("bad loop configuration")
	err := Fatal(wantErr)

	if !IsFatal(err) {
		t.Fatal("IsFatal() = false, want true")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("Fatal() error = %v, want wrapping %v", err, wantErr)
	}
	if IsFatal(wantErr) {
		t.Fatal("IsFatal() = true for unwrapped error")
	}
}

func TestFatalNilReturnsNil(t *testing.T) {
	if Fatal(nil) != nil {
		t.Fatal("Fatal(nil) is not nil")
	}
}
