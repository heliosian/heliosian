package parallel

import (
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
)

func TestRunKeepsResultsAndFailuresInOrderWithinTheWorkerLimit(t *testing.T) {
	for _, workers := range []int{1, 2, 8} {
		t.Run(strconv.Itoa(workers), func(t *testing.T) {
			const n = 6
			width := min(n, workers)
			entered := make(chan struct{}, width)
			release := make(chan struct{})
			done := make(chan []Result[string, int])
			failed := errors.New("failed task")
			var active, peak atomic.Int32
			go func() {
				inputs := []string{"0", "1", "2", "3", "4", "5"}
				done <- Run(inputs, workers, func(input string) (int, error) {
					i, _ := strconv.Atoi(input)
					current := active.Add(1)
					defer active.Add(-1)
					for previous := peak.Load(); current > previous; previous = peak.Load() {
						if peak.CompareAndSwap(previous, current) {
							break
						}
					}
					if i < width {
						entered <- struct{}{}
						<-release
					}
					if i%2 == 1 {
						return i, failed
					}
					return i, nil
				})
			}()
			for range width {
				<-entered
			}
			close(release)
			results := <-done
			if len(results) != n || peak.Load() != int32(width) {
				t.Fatalf("got %d results with %d concurrent tasks, want %d and %d", len(results), peak.Load(), n, width)
			}
			for i, result := range results {
				if result.Input != strconv.Itoa(i) || result.Value != i || errors.Is(result.Err, failed) != (i%2 == 1) {
					t.Fatalf("task %d: %+v", i, result)
				}
			}
		})
	}
}

func TestRunWithNoTasks(t *testing.T) {
	results := Run([]int{}, 0, func(int) (int, error) {
		t.Error("ran a task")
		return 0, nil
	})
	if len(results) != 0 {
		t.Fatalf("got %v", results)
	}
}
