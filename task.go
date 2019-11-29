package sudu

import (
	"sync"
)

type task struct {
	id      int
	origin  *ConditionGroup
	do_lock *sync.Mutex
	doing   bool

	// 任务本体
	fx      func(*ConditionGroup)
	fx_lock *sync.Mutex
	// 将任务状态，以及是否是redo，回馈给调用方，根据返回值来决定此任务是否应当停止运行，不再重做
	// 任务会随着条件的状态变更而出现重复多次运行的情况
	fx_notify func(int, bool) bool

	cg *ConditionGroup
	// 追踪，此任务执行过程中，全部require或want的条件
	r_values map[interface{}]*cValue
	// 追踪，此任务明确require或want的条件，在执行过程中，却产生了变更，则记录在此
	// 用于和r_values比对，确认是否需要重做任务
	rw_values map[interface{}]*cValue
	// 追踪，此任务执行过程中，全部satisfy或cancle的条件
	// 当此任务的legacy状态发生变更时，需要进行状态传播
	w_values map[interface{}]*cValue
	// 标记当前任务的执行结果是否为legacy状态
	// 只要该任务依赖了一个legacy的条件，该任务的执行结果也必然是legacy的，其输出的条件也都是legacy的
	// 而一旦确认该任务依赖的所有条件都不再是legacy状态时，那么该任务的执行结果也必然是unlegacy的
	// 不仅如此，改任务所输出的条件也不再是legacy的
	legacy_mode bool
	// 标记此任务被禁止，不再有被执行的必要了
	disabled bool

	// 任务执行过程中panic的值
	// 如果数据类型为error，则认为是预期内的终止信号
	// 但是否终止，同样取决于此时任务是否处于unlegacy状态下
	fx_panic interface{}
}

// 临时记录每个任务内部读到的条件及当时的值 & 后续期间监测到这些条件的变化
type cValue struct {
	value  interface{}
	cmsg   *CancelMessage
	legacy bool
}

func newTask(id int, origin *ConditionGroup, fx func(*ConditionGroup)) *task {
	t := &task{
		id:      id,
		origin:  origin,
		do_lock: &sync.Mutex{},

		fx:      fx,
		fx_lock: &sync.Mutex{},
	}
	origin.listenWriteEvent(t.listenGlobalWrite)
	return t
}

const (
	task_state_start          = 1
	task_state_success_legacy = 2
	task_state_fail_legacy    = 3
	task_state_success        = 4
	task_state_fail           = 5
)

func (t *task) notify(state int, redo bool) bool {
	if t.fx_notify != nil {
		if t.fx_notify(state, redo) {
			return true
		} else {
			t.disabled = true
			return false
		}
	}
	return true
}

func (t *task) reset() {
	t.r_values = make(map[interface{}]*cValue)
	t.rw_values = make(map[interface{}]*cValue)
	t.w_values = make(map[interface{}]*cValue)
	t.legacy_mode = false
	t.fx_panic = nil
	t.disabled = false

	t.cg = t.origin.clone()
	t.cg.listenReadEvent(t.listenLocalRead)
	t.cg.listenWriteEvent(t.listenLocalWrite)
}

func (t *task) core() {
	t.reset()

	defer func() {
		// 避免任务自身的panic导致意外发生
		// 同时，任务如果panic的数据类型是error，则直接认为该任务常规出错
		// 在明确此任务的运行依赖条件都为unlegacy的状态时
		// 那么此error即表明任务的运行结果的确是出错了
		t.fx_panic = recover()

		// 检查一下依赖的条件是否存在值或者状态变更的情况
		// 如果有，则意味着此任务应当要重做
		t.redo_done()
	}()

	t.fx(t.cg)
}

// notify the task state & redo flag, and the returned true control it continue
func (t *task) do(notify func(int, bool) bool) {
	go func() {
		t.do_lock.Lock()
		defer t.do_lock.Unlock()
		if t.doing {
			return
		}

		t.fx_notify = notify

		// 任务首次开始
		if t.notify(task_state_start, false) {
			t.doing = true
			// 状态通知后，没什么意外就会正式开始执行任务
			go t.core()
		}
	}()
}

// 任务正常执行完成后，redo_done用于触发下一次可能的redo
func (t *task) redo_done() {
	t.do_lock.Lock()
	defer t.do_lock.Unlock()
	if t.disabled {
		return
	}

	var state int
	if t.legacy_mode {
		if t.fx_panic == nil {
			state = task_state_success_legacy
		} else {
			state = task_state_fail_legacy
		}
	} else {
		if t.fx_panic == nil {
			state = task_state_success
		} else {
			state = task_state_fail
		}
	}

	// race with Global Write Event
	t.fx_lock.Lock()
	redo := t.changed()
	t.fx_lock.Unlock()

	if redo == false {
		t.doing = false
	}

	if t.notify(state, redo) == false {
		return
	}

	if redo {
		go t.core()
	} else {
		t.try_unlegacy()
	}
}

// 任务已经执行结束了，但是其依赖的条件可能会继续发生变更
// 这样的变更，需要check该任务是否应当重新再执行一遍
func (t *task) redo_write() {
	t.do_lock.Lock()
	defer t.do_lock.Unlock()
	if t.doing || t.disabled {
		return
	}

	// race with Global Write Event
	t.fx_lock.Lock()
	redo := t.changed()
	t.fx_lock.Unlock()

	if redo {
		if t.notify(task_state_start, true) == true {
			t.doing = true
			go t.core()
		}
	} else {
		t.try_unlegacy()
	}
}

func (t *task) try_unlegacy() {
	if t.legacy_mode {
		// check legacy
		// 虽然值未改变，但是其legacy状态可能全部被移除
		for name, r_value := range t.r_values {
			if r_value.legacy {
				rw_value := t.rw_values[name]
				if rw_value != nil && rw_value.legacy {
					return
				}
			}
		}

		// 此时说明预测正确，需要执行翻转动作，将所有legacy的影响消除
		t.legacy_mode = false
		t.cg.legacy_mode = false

		// 没有start状态，只用于报告转变
		if t.fx_panic == nil {
			t.notify(task_state_success, false)
		} else {
			t.notify(task_state_fail, false)
		}

		// 翻转条件状态，通知条件链上关联的全部条件
		nvs1 := make([]interface{}, 0)
		nvs2 := make([]interface{}, 0)
		for name, w_value := range t.w_values {
			if w_value.legacy && t.cg.legacy[name] {
				if w_value.cmsg == nil {
					nvs1 = append(nvs1, name, w_value.value)
				} else {
					nvs2 = append(nvs2, name, w_value.cmsg)
				}
			}
		}
		if len(nvs1) > 0 {
			t.cg.satisfy(false, false, nvs1...)
		}
		if len(nvs2) > 0 {
			t.cg.satisfy(true, false, nvs2...)
		}
	}
}

// 检查所有依赖到（read: require，want）的条件
// 如果在执行期间，这些依赖条件的值发生过变更
// 那么就表明此任务需要被重做
func (t *task) changed() bool {
	for name, r_value := range t.r_values {
		rw_value := t.rw_values[name]
		if rw_value == nil {
			continue
		}
		if r_value.cmsg != rw_value.cmsg {
			return true
		}
		// do not support customized comapre rule yet
		if r_value.value != rw_value.value {
			return true
		}
	}
	return false
}

func (t *task) listenLocalRead(name interface{}) {
	// record read action
	// always record the first read
	if _, ok := t.r_values[name]; ok {
		return
	}

	legacy := t.cg.legacy[name]
	// 如果读到的条件为legacy（缓存注入），则将本任务的condition group置为legacy_mode
	// 也同样表示了，本次任务是legacy的，其输出也是legacy的
	if legacy {
		t.legacy_mode = true
		t.cg.legacy_mode = true
	}

	value, cmsg, _ := t.cg.inspect(name)
	t.r_values[name] = &cValue{
		value:  value,
		cmsg:   cmsg,
		legacy: legacy,
	}
}

func (t *task) listenLocalWrite(names ...interface{}) {
	for _, name := range names {
		value, cmsg, _ := t.cg.inspect(name)
		t.w_values[name] = &cValue{
			value:  value,
			cmsg:   cmsg,
			legacy: t.legacy_mode,
		}
	}
}

// 监控全局的条件变更行为，一旦发现变更的条件是此任务所依赖的
// 则触发一次redo
func (t *task) listenGlobalWrite(names ...interface{}) {
	// check my concerned conditions
	impact := 0
	t.fx_lock.Lock()
	for _, name := range names {
		if _, ok := t.r_values[name]; ok {
			impact++
			value, cmsg, _ := t.cg.inspect(name)
			t.rw_values[name] = &cValue{
				value:  value,
				cmsg:   cmsg,
				legacy: t.cg.legacy[name],
			}
		}
	}
	t.fx_lock.Unlock()

	if impact > 0 {
		t.redo_write()
	}
}
