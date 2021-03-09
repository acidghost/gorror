package main

type Err string

const ErrSome = Err("failed for {{c.Field[0] MyStruct %s}}")

type MyStruct struct {
	Field []string
}

func main() {
	var s MyStruct
	s.Field = []string{"qwerty"}
	e := newErrSome(s)
	if e.c.Field[0] != "qwerty" {
		panic("Invalid field value")
	}
}
