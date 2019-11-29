package sudu

import (
	"testing"
	"time"
)

func TestConditionGroup(t *testing.T) {
	cg := NewConditionGroup()

	a := 2019
	x := 0

	go func() {
		a := cg.Require("A").(int)

		cg.Satisfy("B", 2*a)
	}()

	go func() {
		a := cg.Require("A").(int)

		cg.Satisfy("C", 3*a)
	}()

	go func() {
		c := cg.Require("C").(int)

		cg.Cancel("D", 4*c)
	}()

	go func() {
		b := cg.Require("B").(int)
		c := cg.Require("C").(int)
		_, d := cg.Want("D")

		cg.Satisfy("X", b*c*d.Message.(int))
	}()

	time.Sleep(1 * time.Millisecond)

	if x != 0 {
		t.Errorf("in = %d, out = %d, expect = %d", a, x, 0)
	}

	cg.Satisfy("A", a)
	x = cg.Require("X").(int)

	if x != 2*a*3*a*4*3*a {
		t.Errorf("in = %d, out = %d, expect = %d", a, x, 2*a*3*a*4*3*a)
	}
}
