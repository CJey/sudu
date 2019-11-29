package sudu

import (
	"sync"
)

type CancelMessage struct {
	Message interface{}
}

// 条件依赖模型
// 有点类似于pub/sub模型
// 但对于条件而言，只有满足与不满足
// 条件满足后，所有对该条件的引用全部不会阻塞，且一定成功，不用像pub/sub模型中先行订阅
type ConditionGroup struct {
	father *ConditionGroup

	raw    map[interface{}]interface{}
	lsn    map[interface{}]chan struct{}
	cmsgs  map[interface{}]*CancelMessage
	legacy map[interface{}]bool
	cmsg   *CancelMessage

	read_cbs  []func(name interface{})
	write_cbs []func(names ...interface{})

	// in legacy mode, Satisfy & Cancel will set the condition as legacy
	legacy_mode bool

	lock       *sync.Mutex
	event_lock *sync.Mutex
}

func NewConditionGroup() *ConditionGroup {
	return &ConditionGroup{
		raw:    make(map[interface{}]interface{}),
		lsn:    make(map[interface{}]chan struct{}),
		cmsgs:  make(map[interface{}]*CancelMessage),
		legacy: make(map[interface{}]bool),

		lock:       &sync.Mutex{},
		event_lock: &sync.Mutex{},
	}
}

func (cg *ConditionGroup) clone() *ConditionGroup {
	var cg2 ConditionGroup = *cg
	cg2.father = cg
	cg2.read_cbs = nil
	cg2.write_cbs = nil
	cg2.legacy_mode = false
	cg2.event_lock = &sync.Mutex{}
	return &cg2
}

func (cg *ConditionGroup) listenReadEvent(fx func(interface{})) {
	cg.event_lock.Lock()
	defer cg.event_lock.Unlock()
	if cg.read_cbs == nil {
		cg.read_cbs = make([]func(interface{}), 0)
	}

	cg.read_cbs = append(cg.read_cbs, fx)
}

func (cg *ConditionGroup) listenWriteEvent(fx func(...interface{})) {
	cg.event_lock.Lock()
	defer cg.event_lock.Unlock()
	if cg.write_cbs == nil {
		cg.write_cbs = make([]func(...interface{}), 0)
	}

	cg.write_cbs = append(cg.write_cbs, fx)
}

func (cg *ConditionGroup) emitReadEvent(name interface{}) {
	for _, cb := range cg.read_cbs {
		cb(name)
	}

	if cg.father != nil {
		cg.father.emitReadEvent(name)
	}
}

func (cg *ConditionGroup) emitWriteEvent(names ...interface{}) {
	for _, cb := range cg.write_cbs {
		cb(names...)
	}

	if cg.father != nil {
		cg.father.emitWriteEvent(names...)
	}
}

func (cg *ConditionGroup) conditions() (names []interface{}) {
	names = make([]interface{}, 0)
	for name := range cg.raw {
		names = append(names, name)
	}
	for name := range cg.cmsgs {
		if _, ok := cg.raw[name]; ok == false {
			names = append(names, name)
		}
	}
	return names
}

func (cg *ConditionGroup) inspect(name interface{}) (interface{}, *CancelMessage, bool) {
	if v, ok := cg.raw[name]; ok {
		return v, nil, true
	}
	if cmsg, ok := cg.cmsgs[name]; ok {
		return nil, cmsg, true
	}
	if cg.cmsg != nil {
		return nil, cg.cmsg, true
	}
	return nil, nil, false
}

// read from raw map
func (cg *ConditionGroup) Inspect(name interface{}) (interface{}, *CancelMessage, bool) {
	cg.lock.Lock()
	defer cg.lock.Unlock()

	return cg.inspect(name)
}

// raw map first, otherwise waiting for Satisfy()
func (cg *ConditionGroup) Want(name interface{}) (interface{}, *CancelMessage) {
	cg.lock.Lock()
	if v, cmsg, ok := cg.inspect(name); ok {
		cg.emitReadEvent(name)
		cg.lock.Unlock()
		return v, cmsg
	}

	lsn, ok := cg.lsn[name]
	if !ok {
		lsn = make(chan struct{}, 0)
		cg.lsn[name] = lsn
	}
	cg.lock.Unlock()

	<-lsn
	return cg.Want(name)
}

// got or panic
func (cg *ConditionGroup) Require(name interface{}) interface{} {
	v, c := cg.Want(name)
	if c != nil {
		panic(c)
	}
	return v
}

// fill raw map or cancel it, then wakeup listeners
// name, value, [name, value, ...]
func (cg *ConditionGroup) satisfy(canceled, legacy bool, nvs ...interface{}) {
	if len(nvs)%2 == 1 {
		nvs = append(nvs, nil)
	}

	names := make([]interface{}, 0, len(nvs)/2)
	for i := 0; i < len(nvs); i += 2 {
		name, value := nvs[i], nvs[i+1]

		if legacy {
			if v, ok := cg.legacy[name]; ok {
				if v == false {
					continue
				}
			} else {
				cg.legacy[name] = true
			}
		} else {
			cg.legacy[name] = false
		}

		if canceled {
			delete(cg.raw, name)
			cg.cmsgs[name] = &CancelMessage{value}
		} else {
			delete(cg.cmsgs, name)
			cg.raw[name] = value
		}
		if lsn, ok := cg.lsn[name]; ok {
			close(lsn) // wakeup
			delete(cg.lsn, name)
		}
		names = append(names, name)
	}

	cg.emitWriteEvent(names...)
}

// name, value, [name, value, ...]
func (cg *ConditionGroup) Satisfy(nvs ...interface{}) {
	cg.lock.Lock()
	defer cg.lock.Unlock()

	cg.satisfy(false, cg.legacy_mode, nvs...)
}

func (cg *ConditionGroup) satisfyLegacy(nvs ...interface{}) {
	cg.lock.Lock()
	defer cg.lock.Unlock()

	cg.satisfy(false, true, nvs...)
}

// name, msg, [name, msg, ...]
func (cg *ConditionGroup) Cancel(nvs ...interface{}) {
	cg.lock.Lock()
	defer cg.lock.Unlock()

	cg.satisfy(true, cg.legacy_mode, nvs...)
}

func (cg *ConditionGroup) cancelAll(msg interface{}) {
	cg.lock.Lock()
	defer cg.lock.Unlock()

	cg.cmsg = &CancelMessage{msg}

	names := make([]interface{}, 0, len(cg.lsn))
	for name, lsn := range cg.lsn {
		close(lsn) // wakeup
		delete(cg.lsn, name)
		names = append(names, name)
	}

	cg.emitWriteEvent(names...)
}
