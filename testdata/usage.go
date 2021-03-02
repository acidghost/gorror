package main

import "errors"

type Err string

const (
	ErrOpen = Err("failed to open {{file string %q}}")
	ErrRead = Err("failed to read from {{file string %q}} (code={{code uint %d}})")
)

func main() {
	e1 := errors.New("some external error")
	e2 := NewErrOpen("filename.txt").Wrap(e1)
	e3 := NewErrRead("filename.txt", 42).Wrap(e2)
	if !ErrOpen.IsIn(e3) {
		panic("unexpected stuff!")
	}
	if !errors.Is(e3, e1) {
		panic("unexpected stuff!")
	}
}
