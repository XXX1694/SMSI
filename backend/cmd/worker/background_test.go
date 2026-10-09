package main

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func testBackground() *background {
	return &background{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestBackgroundWaitReturnsOnceJobsObserveContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	bg := testBackground()
	var stopped atomic.Int32
	for range 3 {
		bg.Go(func() {
			<-ctx.Done()
			time.Sleep(20 * time.Millisecond) // finishing the current iteration
			stopped.Add(1)
		})
	}
	cancel()
	if !bg.Wait(2 * time.Second) {
		t.Fatal("Wait timed out although every job stops on cancel")
	}
	if n := stopped.Load(); n != 3 {
		t.Fatalf("%d of 3 jobs finished before Wait returned", n)
	}
}

func TestBackgroundWaitIsBoundedWhenAJobIgnoresItsContext(t *testing.T) {
	bg := testBackground()
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	bg.Go(func() { <-release })
	start := time.Now()
	if bg.Wait(50 * time.Millisecond) {
		t.Fatal("Wait reported success for a job that is still running")
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("Wait took %s, want about the 50ms timeout", d)
	}
}
