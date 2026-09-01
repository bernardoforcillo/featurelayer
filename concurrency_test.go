package featurelayer

import (
	"context"
	"sync"
	"testing"
)

// Run with -race: Apply concurrent with Evaluate and Consume.
func TestApplyConcurrentWithEvaluate(t *testing.T) {
	e, _, _ := testEngine(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					e.Evaluate(ctx, "export.csv", EvalContext{TenantID: "acme"})
					e.Consume(ctx, "api.calls", EvalContext{TenantID: "acme"}, 1)
				}
			}
		}()
	}
	for i := 0; i < 200; i++ {
		snap, err := NewSnapshot(fullTestConfig())
		if err != nil {
			t.Fatal(err)
		}
		e.Apply(snap)
	}
	close(stop)
	wg.Wait()
}
