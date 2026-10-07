package queue

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/domain/post"
)

func TestTaskIDIsDeterministicAndDistinguishesJobs(t *testing.T) {
	target := uuid.New()
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	j1 := post.Job{ID: uuid.New(), PostTargetID: target, RunAt: at}
	first, second := TaskID(j1), TaskID(j1)
	if first != second {
		t.Fatal("task id must be stable so duplicate enqueues collapse")
	}
	if !strings.HasPrefix(TaskID(j1), target.String()+":") {
		t.Fatalf("id should start with the target: %s", TaskID(j1))
	}
	// A rescheduled job for the same target (new run_at or new job row) never collides with an old retained task.
	j2 := j1
	j2.RunAt = at.Add(time.Hour)
	j3 := j1
	j3.ID = uuid.New()
	if TaskID(j1) == TaskID(j2) || TaskID(j1) == TaskID(j3) {
		t.Fatalf("ids must differ: %s %s %s", TaskID(j1), TaskID(j2), TaskID(j3))
	}
	// Sub-second differences do not matter, whole seconds do.
	j4 := j1
	j4.RunAt = at.Add(500 * time.Millisecond)
	if TaskID(j1) != TaskID(j4) {
		t.Fatal("run_at is truncated to seconds")
	}
}
