package wg

import "sync"

var wg *sync.WaitGroup

func Add(delta int) {
	if wg == nil {
		wg = &sync.WaitGroup{}
	}
	wg.Add(delta)
}

func Wait() {
	if wg != nil {
		wg.Wait()
		wg = nil
	}
}

func Done() {
	if wg != nil {
		wg.Done()
	}
}
