// Package par runs independent work items on GOMAXPROCS workers.
package par

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// Do runs fn(0..n-1) on up to GOMAXPROCS workers and waits. Callers index
// results by i, so output never depends on scheduling.
func Do(n int, fn func(i int)) {
	DoW(n, func(_, i int) { fn(i) })
}

// DoW is Do with the worker's index passed too, for per-worker scratch;
// Workers bounds it.
func DoW(n int, fn func(worker, i int)) {
	workers := Workers(n)
	if workers <= 1 {
		for i := 0; i < n; i++ {
			fn(0, i)
		}
		return
	}
	var next atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= n {
					return
				}
				fn(w, i)
			}
		}(w)
	}
	wg.Wait()
}

// Inline runs fn(0, 0..n-1) on the calling goroutine: DoW for work too
// small to hand out.
func Inline(n int, fn func(worker, i int)) {
	for i := 0; i < n; i++ {
		fn(0, i)
	}
}

// Workers is how many workers DoW uses for n items.
func Workers(n int) int {
	return max(min(runtime.GOMAXPROCS(0), n), 1)
}
