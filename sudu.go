package sudu

import (
	//"fmt"
	"sync"
	//"time"
)

// Sudu，用于将存在条件依赖的并发调用，利用cache机制将依赖解开，变成并发调用
// 核心需要处理的，则是依赖条件值的legacy问题
// 使用legacy的值抢先进行预测，当依赖的条件由legacy状态转变为unlegacy时
// 对应抢先预测的任务需要检查自身刚做完的结果是否有必要重做
// 自身是否也受到unlegacy影响，也应当转变为unlegacy状态
type Sudu struct {
	ConditionGroup

	lock sync.Mutex
	wg   sync.WaitGroup

	// 任务panic结果，非legacy的任务发生了panic，将尽最大努力停止所有后续任务的开始
	// 同时，默认情况下会立即unblock Wait (controlled by WaitAll)
	// panic的值如果是error，则会作为Wait的返回值，否则Wait会触发同样的panic
	tasks    map[int]*task
	fx_panic interface{}
	fx_lock  sync.Mutex

	Tasks   int
	Running int
	WaitAll bool

	cache *Cache
}

func NewSudu(c *Cache) *Sudu {
	cg := NewConditionGroup()

	sd := &Sudu{
		ConditionGroup: *cg,

		lock: sync.Mutex{},
		wg:   sync.WaitGroup{},

		tasks:   make(map[int]*task),
		fx_lock: sync.Mutex{},

		cache: c,
	}
	sd.cacheRestore()
	return sd
}

// 预留，用于注册复杂类型的值比较方法
// 简单的内置比较直接采用==方式，无法适用于复杂结构
//func (sd *Sudu) CompareRule(name interface{}, fx func(v1, v2 interface{}) bool) {
//}

func (sd *Sudu) task(fx func(*ConditionGroup)) int {
	id := sd.Tasks

	task := newTask(id, &sd.ConditionGroup, fx)
	sd.tasks[id] = task

	sd.Tasks++
	sd.wg.Add(1)
	sd.Running++

	doing := true
	round := 0

	task.do(func(state int, redo bool) (do bool) {
		sd.lock.Lock()
		defer sd.lock.Unlock()
		//fmt.Printf("%s, id = %d, round = %d, state = %d\n", time.Now(), id, round, state)
		if state == task_state_fail && sd.fx_panic == nil {
			// found the first panic from unlegacy task, stop!
			sd.fx_panic = task.fx_panic
			if sd.WaitAll == false {
				sd.wg.Add(sd.Running * -1)
			}
		}

		if sd.fx_panic == nil || sd.WaitAll {
			if state == task_state_start {
				if round > 0 {
					sd.wg.Add(1)
					sd.Running++
					doing = true
				}
			} else if doing {
				if redo == false {
					doing = false
					sd.Running--
					sd.wg.Done()
				}
				round++
			}
			return true
		}

		if doing {
			doing = false
			sd.Running--
			round++
		}
		return false
	})
	return id
}

// 简单的集成了自带的cache，自动将依赖条件结果保存
func (sd *Sudu) cacheSave() {
	if sd.cache != nil {
		sd.cache.Set(sd.Conditions())
	}
}

// 简单的集成了自带的cache，自动将依赖条件结果还原
// 从cache中还原的条件，均会被置为legacy状态
func (sd *Sudu) cacheRestore() {
	if sd.cache != nil {
		sd.SatisfyLegacy(sd.cache.Get()...)
	}
}

func (sd *Sudu) Go(fxs ...func(*ConditionGroup)) {
	sd.lock.Lock()
	defer sd.lock.Unlock()

	for _, fx := range fxs {
		sd.task(fx)
	}
}

func (sd *Sudu) Wait() error {
	sd.wg.Wait()

	sd.lock.Lock()
	defer sd.lock.Unlock()

	if sd.fx_panic == nil {
		sd.cacheSave()
		return nil
	}

	if err, ok := sd.fx_panic.(error); ok {
		return err
	} else {
		panic(sd.fx_panic)
	}
}

// name, value, [name, value, ...]
func (sd *Sudu) SatisfyLegacy(nvs ...interface{}) {
	sd.ConditionGroup.satisfyLegacy(nvs...)
}

func (sd *Sudu) Conditions() []interface{} {
	cs := make([]interface{}, 0)
	for _, task := range sd.tasks {
		for name, cvalue := range task.w_values {
			if cvalue.cmsg == nil && cvalue.legacy == false {
				cs = append(cs, name, cvalue.value)
			}
		}
	}
	return cs
}
