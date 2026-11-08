package progress

import (
	"testing"
	"time"
)

func TestReporter_ProcessesPhasesAndFinishes(t *testing.T) {
	r, ch := New()
	r.Start()

	for i := 1; i <= 3; i++ {
		ch <- Update{Phase: "Rasterizing", Current: i, Total: 3}
	}
	ch <- Update{Phase: "Encoding", Current: 1, Total: 1}
	close(ch)

	done := make(chan struct{})
	go func() { r.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait() did not return after channel close")
	}
	if r.currentPhase != "Encoding" {
		t.Errorf("currentPhase = %q, want Encoding", r.currentPhase)
	}
}

func TestReporter_NoUpdates(_ *testing.T) {
	r, ch := New()
	r.Start()
	close(ch)
	r.Wait()
}
