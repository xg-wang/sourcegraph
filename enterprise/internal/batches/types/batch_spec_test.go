package types

import (
	"testing"
)

func TestComputeBatchSpecState(t *testing.T) {
	tests := []struct {
		stats BatchSpecStats
		want  BatchSpecState
	}{
		{
			stats: BatchSpecStats{Workspaces: 5},
			want:  BatchSpecStatePending,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Queued: 3},
			want:  BatchSpecStateQueued,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Queued: 2, Processing: 1},
			want:  BatchSpecStateProcessing,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Queued: 1, Processing: 1, Completed: 1},
			want:  BatchSpecStateProcessing,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Queued: 1, Processing: 0, Completed: 2},
			want:  BatchSpecStateProcessing,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Queued: 0, Processing: 0, Completed: 3},
			want:  BatchSpecStateCompleted,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Queued: 1, Processing: 1, Failed: 1},
			want:  BatchSpecStateProcessing,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Queued: 1, Processing: 0, Failed: 2},
			want:  BatchSpecStateProcessing,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Queued: 0, Processing: 0, Failed: 3},
			want:  BatchSpecStateFailed,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Queued: 0, Completed: 1, Failed: 2},
			want:  BatchSpecStateFailed,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Canceling: 3},
			want:  BatchSpecStateCanceling,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Canceling: 2, Completed: 1},
			want:  BatchSpecStateCanceling,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Canceling: 2, Failed: 1},
			want:  BatchSpecStateCanceling,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Canceling: 1, Queued: 2},
			want:  BatchSpecStateProcessing,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Canceling: 1, Processing: 2},
			want:  BatchSpecStateProcessing,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Canceled: 3},
			want:  BatchSpecStateCanceled,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Canceled: 1, Failed: 2},
			want:  BatchSpecStateCanceled,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Canceled: 1, Completed: 2},
			want:  BatchSpecStateCanceled,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Canceled: 1, Canceling: 2},
			want:  BatchSpecStateCanceling,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Canceled: 1, Canceling: 1, Queued: 1},
			want:  BatchSpecStateProcessing,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Canceled: 1, Processing: 2},
			want:  BatchSpecStateProcessing,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Canceled: 1, Canceling: 1, Processing: 1},
			want:  BatchSpecStateProcessing,
		},
		{
			stats: BatchSpecStats{Workspaces: 5, Executions: 3, Canceled: 1, Queued: 2},
			want:  BatchSpecStateProcessing,
		},
	}

	for idx, tt := range tests {
		have := ComputeBatchSpecState(tt.stats)

		if have != tt.want {
			t.Errorf("test %d/%d: unexpected batch spec state. want=%s, have=%s", idx+1, len(tests), tt.want, have)
		}
	}
}
