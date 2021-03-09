package main

type Err string

const ErrSome = Err("nowrap:some error")

func main() {
	e := newErrSome()
	if e.Error() != "some error" {
		panic("wrong error message")
	}
}
