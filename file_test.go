//go:build windows

package winio

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestErrTimeout(t *testing.T) {
	if got, want := ErrTimeout.Error(), "i/o timeout"; got != want {
		t.Fatalf("ErrTimeout.Error() = %q; want %q", got, want)
	}

	var timeout interface {
		Timeout() bool
		Temporary() bool
	}
	if !errors.As(ErrTimeout, &timeout) {
		t.Fatal("ErrTimeout does not implement timeout methods")
	}
	if !timeout.Timeout() {
		t.Error("ErrTimeout.Timeout() = false; want true")
	}
	if !timeout.Temporary() {
		t.Error("ErrTimeout.Temporary() = false; want true")
	}

	for _, target := range []error{
		ErrTimeout,
		os.ErrDeadlineExceeded,
		context.DeadlineExceeded,
	} {
		if !errors.Is(ErrTimeout, target) {
			t.Errorf("errors.Is(ErrTimeout, %v) = false; want true", target)
		}
	}

	if errors.Is(ErrTimeout, context.Canceled) {
		t.Error("errors.Is(ErrTimeout, context.Canceled) = true; want false")
	}
}
