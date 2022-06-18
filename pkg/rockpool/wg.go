package rockpool

import "sync"

func (wg *Wg) WgAdd(delta int) {
	if wg.wg == nil {
		wg.wg = &sync.WaitGroup{}
	}
	wg.wg.Add(delta)
}

func (wg *Wg) WgWait() {
	if wg.wg != nil {
		wg.wg.Wait()
		wg.wg = nil
	}
}

func (wg *Wg) WgDone() {
	if wg.wg != nil {
		wg.wg.Done()
	}
}
