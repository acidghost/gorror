package main

import "errors"

type Err string

const ErrFileOp = Err("failed to {{op string %s}} {{file string %q}} (code {{code int %d}})")

func main() {
	e := NewErrFileOp("create", "filename.txt", 42)
	if e.Error() != `failed to create "filename.txt" (code 42)` {
		panic("wrong error message: " + e.Error())
	}
	external := errors.New("some other error")
	ee := e.Wrap(external)
	if !ErrFileOp.IsIn(ee) {
		panic("ErrFileOp.IsIn(ee)")
	}
	if !errors.Is(ee, external) {
		panic("errors.Is(errFileOp, external)")
	}
}
