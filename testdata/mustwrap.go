package main

import "errors"

type Err string

const ErrSome = Err("wrap:some error")

func main() {
	inner := errors.New("inner error")
	e := newErrSome(inner)
	if !errors.Is(e, inner) {
		panic("inner not in error")
	}
}
