package progress

import (
	"errors"
	"testing"
	"time"
)

func TestReporter_WaitReturnsAfterChannelClose(t *testing.T) {
	r, ch := New()
	r.Start()

	for i := 1; i <= 3; i++ {
		ch <- Update{Phase: PhaseRasterize, Current: i, Total: 3}
	}
	ch <- Update{Phase: PhaseEncode, Current: 1, Total: 1}
	close(ch)

	done := make(chan struct{})
	go func() { r.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait() did not return after channel close")
	}
}

func TestRun_ReturnsFnErrorAfterDraining(t *testing.T) {
	want := errors.New("boom")
	err := Run(func(ch chan<- Update) error {
		ch <- Update{Phase: PhaseIR, Current: 1, Total: 1}
		return want
	})
	if !errors.Is(err, want) {
		t.Errorf("Run() = %v, want %v", err, want)
	}
}
