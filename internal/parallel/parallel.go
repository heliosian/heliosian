package parallel

import "sync"

type Result[T any] struct {
	Value T
	Err   error
}

func Run[T any](n, workers int, f func(int) (T, error)) []Result[T] {
	results := make([]Result[T], n)
	next := make(chan int)
	var wg sync.WaitGroup
	for range min(n, workers) {
		wg.Go(func() {
			for i := range next {
				results[i].Value, results[i].Err = f(i)
			}
		})
	}
	for i := range n {
		next <- i
	}
	close(next)
	wg.Wait()
	return results
}
