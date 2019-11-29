package sudu

import (
	"testing"
)

func TestTaskGroup(t *testing.T) {
	tg := NewTaskGroup()

	var i int
	tg.Go(func() {
		i += tg.Tasks
	})

	tg.Go(func() {
		i += tg.Tasks
	})

	tg.Go(func() {
		i += tg.Tasks
	})

	tg.Go(func() {
		i += tg.Tasks
	})

	tg.Wait()

	if i != 1+2+3+4 {
		t.Errorf("i = %d, expect %d", i, 1+2+3+4)
	}

	if tg.Tasks != 4 {
		t.Errorf("tg.Tasks = %d, expect %d", tg.Tasks, 4)
	}

	if tg.Running != 0 {
		t.Errorf("tg.Running = %d, expect %d", tg.Running, 0)
	}
}
