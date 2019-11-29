package sudu

import (
	"sync"
)

// 一个waitgroup的高级封装
type TaskGroup struct {
	lock sync.Mutex
	wg   sync.WaitGroup

	fx_panic interface{}
	fx_lock  sync.Mutex

	panicFilter func(interface{}) interface{}

	Tasks   int
	Running int
	WaitAll bool
}

func NewTaskGroup() *TaskGroup {
	return &TaskGroup{
		lock: sync.Mutex{},
		wg:   sync.WaitGroup{},

		fx_lock: sync.Mutex{},
	}
}

// if fx panic()，the Wait will be unblock immediately by default (controled by WaitAll)
// if fx first panic(error), the Wait will return the error
// if fx first panic(anyother), the Wait will panic as the same
// otherwise, Wait return nil
func (tg *TaskGroup) Go(fxs ...func()) {
	tg.lock.Lock()
	defer tg.lock.Unlock()

	wg := sync.WaitGroup{}
	for _, fx := range fxs {
		wg.Add(1)
		go func() {
			defer func() {
				tg.fx_lock.Lock()
				defer tg.fx_lock.Unlock()

				tg.Running--

				if tg.fx_panic == nil || tg.WaitAll {
					tg.wg.Done()
				}

				if p := recover(); p != nil && tg.fx_panic == nil {
					if tg.panicFilter != nil {
						p = tg.panicFilter(p)
					}
					if p != nil {
						tg.fx_panic = p
						if !tg.WaitAll {
							tg.wg.Add(tg.Running * -1)
						}
					}
				}
			}()

			tg.fx_lock.Lock()
			if tg.fx_panic == nil || tg.WaitAll {
				tg.wg.Add(1)
			}
			tg.fx_lock.Unlock()

			tg.Tasks++
			tg.Running++
			wg.Done()

			fx()
		}()
	}
	wg.Wait()
}

func (tg *TaskGroup) Wait() error {
	tg.lock.Lock()
	defer tg.lock.Unlock()

	tg.wg.Wait()

	if tg.fx_panic == nil {
		return nil
	}

	if err, ok := tg.fx_panic.(error); ok {
		return err
	} else {
		panic(tg.fx_panic)
	}
}
