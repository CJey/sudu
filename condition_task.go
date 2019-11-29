package sudu

// 快速组合
// sudu的一种简单的降级使用方式
// 不涉及条件的legacy状态变更问题
// 所有条件只区分有和没有
// 可以尽最大努力在满足条件的前提下进行任务并发
type ConditionTask struct {
	ConditionGroup
	TaskGroup
}

func NewConditionTask() *ConditionTask {
	cg := NewConditionGroup()
	tg := NewTaskGroup()

	tg.panicFilter = func(p interface{}) interface{} {
		cg.cancelAll(p)
		return p
	}

	return &ConditionTask{
		*cg,
		*tg,
	}
}
