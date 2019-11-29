package sudu

import (
	"testing"
	"time"
)

func TestConditionTask(t *testing.T) {
	ct := NewConditionTask()

	a := 2019
	x := 0

	ct.Go(func() {
		a := ct.Require("A").(int)

		ct.Satisfy("B", 2*a)
	})

	ct.Go(func() {
		a := ct.Require("A").(int)

		ct.Satisfy("C", 3*a)
	})

	ct.Go(func() {
		c := ct.Require("C").(int)

		ct.Cancel("D", 4*c)
	})

	ct.Go(func() {
		b := ct.Require("B").(int)
		c := ct.Require("C").(int)
		_, d := ct.Want("D")

		x = b * c * d.Message.(int)
	})

	time.Sleep(1 * time.Millisecond)

	if x != 0 {
		t.Errorf("in = %d, out = %d, expect = %d", a, x, 0)
	}

	ct.Satisfy("A", a)
	ct.Wait()

	if x != 2*a*3*a*4*3*a {
		t.Errorf("in = %d, out = %d, expect = %d", a, x, 2*a*3*a*4*3*a)
	}
}
