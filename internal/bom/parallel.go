package bom

import (
	"runtime"
	"sync"
)

// parallel appelle fn(0..n-1) sur au plus GOMAXPROCS goroutines et attend la fin de tous. Réservé aux calculs
// purs (ComputeTotals) : fn écrit seulement dans sa propre case, l'ordre des résultats ne dépend pas de
// l'ordonnancement (AGENTS §3). Jamais pour une lecture de base (un store câblé sur une transaction n'est pas
// partageable entre goroutines).
func parallel(n int, fn func(i int)) {
	workers := min(n, runtime.GOMAXPROCS(0))
	if workers <= 1 {
		for i := range n {
			fn(i)
		}
		return
	}
	next := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				fn(i)
			}
		}()
	}
	for i := range n {
		next <- i
	}
	close(next)
	wg.Wait()
}
