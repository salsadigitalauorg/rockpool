package platform

import "sync"

var Wg *sync.WaitGroup

func WgAdd(delta int) {
	if Wg == nil {
		Wg = &sync.WaitGroup{}
	}
	Wg.Add(delta)
}

func WgWait() {
	if Wg != nil {
		Wg.Wait()
		Wg = nil
	}
}

func WgDone() {
	if Wg != nil {
		Wg.Done()
	}
}
