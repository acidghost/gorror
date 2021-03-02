package main

import "errors"

type Err string

const (
	ErrOpen = Err("failed to open file")
)

func main() {
	var e error = NewErrOpen()
	ErrOpen.IsIn(e)
	ee := NewErrOpen()
	external := errors.New("some other error")
	var eee error = ee.Wrap(external)
	if !ErrOpen.IsIn(eee) {
		panic("ErrOpen.IsIn()")
	}
	if !errors.Is(eee, external) {
		panic("errors.Is(errOpen, external")
	}
}
