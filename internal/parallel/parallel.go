package parallel

import "sync"

type Result[S, T any] struct {
	Input S
	Value T
	Err   error
}

func Run[S, T any](inputs []S, workers int, f func(S) (T, error)) []Result[S, T] {
	results := make([]Result[S, T], len(inputs))
	next := make(chan *Result[S, T])
	var wg sync.WaitGroup
	for range min(len(inputs), workers) {
		wg.Go(func() {
			for result := range next {
				result.Value, result.Err = f(result.Input)
			}
		})
	}
	for i, input := range inputs {
		results[i].Input = input
		next <- &results[i]
	}
	close(next)
	wg.Wait()
	return results
}
