package sudu

import (
	"fmt"
	"testing"
	"time"
)

func TestSudu01(t *testing.T) {
	sd := NewSudu(nil)

	var cntb int
	sd.Go(func(cg *ConditionGroup) {
		cntb++
		a := cg.Require("A").(int)
		time.Sleep(1 * time.Millisecond)
		cg.Satisfy("B", a*2)
	})

	var cntc int
	sd.Go(func(cg *ConditionGroup) {
		cntc++
		a := cg.Require("A").(int)
		time.Sleep(2 * time.Millisecond)
		cg.Satisfy("C", a*3)
	})

	var cntd int
	sd.Go(func(cg *ConditionGroup) {
		cntd++
		b := cg.Require("B").(int)
		c := cg.Require("C").(int)

		cg.Satisfy("d", b*c)
	})

	var a = 3
	sd.Satisfy("A", a)
	sd.SatisfyLegacy("B", 2*a, "C", 3*a)
	sd.Wait()

	if cntb != 1 || cntc != 1 || cntd != 1 || sd.Require("d").(int) != 2*a*3*a {
		t.Errorf("error cnt")
	}

	a = 4
	sd.Satisfy("A", a)
	sd.Wait()

	if cntb != 2 || cntc != 2 || cntd != 3 || sd.Require("d").(int) != 2*a*3*a {
		t.Errorf("error cnt2, %d, %d, %d", cntb, cntc, cntd)
	}
}

func TestSudu02(t *testing.T) {
	sd := NewSudu(nil)

	var cntb int
	sd.Go(func(cg *ConditionGroup) {
		cntb++
		a := cg.Require("A").(int)
		time.Sleep(1 * time.Millisecond)
		cg.Satisfy("B", a*2)
	})

	var cntc int
	sd.Go(func(cg *ConditionGroup) {
		cntc++
		a := cg.Require("A").(int)
		time.Sleep(2 * time.Millisecond)
		cg.Satisfy("C", a*3)
	})

	var cntd int
	sd.Go(func(cg *ConditionGroup) {
		cntd++
		b := cg.Require("B").(int)
		c := cg.Require("C").(int)

		cg.Satisfy("d", b*c)
	})

	var a = 19
	sd.Satisfy("A", a)
	sd.SatisfyLegacy("B", 2*a, "C", 4*a)
	sd.Wait()

	if cntb != 1 || cntc != 1 || cntd != 2 || sd.Require("d").(int) != 2*a*3*a {
		t.Errorf("error cnt")
	}

	a = 91
	sd.Satisfy("A", a)
	sd.Wait()

	if cntb != 2 || cntc != 2 || cntd != 4 || sd.Require("d").(int) != 2*a*3*a {
		t.Errorf("error cnt2, %d, %d, %d", cntb, cntc, cntd)
	}
}

func TestSudu03(t *testing.T) {
	cache := NewCache("metadata", "xyz")
	{
		sd := NewSudu(cache)
		sd.Go(func(cg *ConditionGroup) {
			cg.Satisfy(
				"str", "test",
			)
		})

		sd.Go(func(cg *ConditionGroup) {
			str := cg.Require("str").(string)
			if str != "test" {
				panic(fmt.Errorf("ddddddd"))
			}
		})

		if err := sd.Wait(); err != nil {
			fmt.Printf("fail %#v\n", err)
		}
	}
	{
		sd := NewSudu(cache)
		sd.Go(func(cg *ConditionGroup) {
			time.Sleep(10 * time.Millisecond)
			cg.Satisfy(
				"str", "test2",
			)
		})

		sd.Go(func(cg *ConditionGroup) {
			str := cg.Require("str").(string)
			if str == "test" {
				panic(fmt.Errorf("ddddddd"))
			}
		})

		if err := sd.Wait(); err != nil {
			t.Errorf("error cache")
		}
	}
}
