// (c) Copyright 2021, Gorror Authors.
//
// Licensed under the terms of the GNU GPL License version 3.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var golden = []Golden{
	{"simple", false, simpleIn, simpleOut},
	{"simpleCompatIs", true, simpleIn, simpleErrIsOut},
	{"oneField", false, oneFieldIn, oneFieldOut},
	{"multiFields", false, multiFieldsIn, multiFieldsOut},
}

// Golden represents a test case.
type Golden struct {
	name     string // name of the test case
	compatIs bool   // enables compatibility with errors.Is
	input    string // given input
	output   string // expected output
}

const simpleIn = `type Err string
const ErrOpen = Err("failed to open file")`

const simpleOut = `type errOpen struct {
	_errWrap
}

func NewErrOpen() *errOpen {
	return &errOpen{_errWrap{nil}}
}

func (e *errOpen) Error() string {
	if e.cause == nil {
		return fmt.Sprintf("failed to open file")
	}
	return fmt.Sprintf("failed to open file: %v", e.cause)
}

func (e *errOpen) Wrap(cause error) error {
	e.cause = cause
	return e
}

func (*errOpen) Is(e Err) bool { return e == ErrOpen }`

const simpleErrIsOut = `type errOpen struct {
	_errWrap
}

func NewErrOpen() *errOpen {
	return &errOpen{_errWrap{nil}}
}

func (e *errOpen) Error() string {
	if e.cause == nil {
		return fmt.Sprintf("failed to open file")
	}
	return fmt.Sprintf("failed to open file: %v", e.cause)
}

func (e *errOpen) Wrap(cause error) error {
	e.cause = cause
	return e
}

func (*errOpen) Is(e error) bool { return e == ErrOpen }`

const oneFieldIn = `type Err string
const ErrOpen = Err("failed to open {{filename string %q}}")`

const oneFieldOut = `type errOpen struct {
	_errWrap
	filename string
}

func NewErrOpen(filename string) *errOpen {
	return &errOpen{_errWrap{nil}, filename}
}

func (e *errOpen) Error() string {
	if e.cause == nil {
		return fmt.Sprintf("failed to open %q", e.filename)
	}
	return fmt.Sprintf("failed to open %q: %v", e.filename, e.cause)
}

func (e *errOpen) Wrap(cause error) error {
	e.cause = cause
	return e
}

func (*errOpen) Is(e Err) bool { return e == ErrOpen }`

const multiFieldsIn = `type Err string
const ErrFileOp = Err("failed to {{op string %s}} {{file string %q}} (code {{code int %d}})")`

const multiFieldsOut = `type errFileOp struct {
	_errWrap
	op   string
	file string
	code int
}

func NewErrFileOp(op string, file string, code int) *errFileOp {
	return &errFileOp{_errWrap{nil}, op, file, code}
}

func (e *errFileOp) Error() string {
	if e.cause == nil {
		return fmt.Sprintf("failed to %s %q (code %d)", e.op, e.file, e.code)
	}
	return fmt.Sprintf("failed to %s %q (code %d): %v", e.op, e.file, e.code, e.cause)
}

func (e *errFileOp) Wrap(cause error) error {
	e.cause = cause
	return e
}

func (*errFileOp) Is(e Err) bool { return e == ErrFileOp }`

func TestGolden(t *testing.T) {
	for _, test := range golden {
		t.Run(test.name, func(t *testing.T) {
			input := "package test\n" + test.input
			absFile := filepath.Join(t.TempDir(), test.name+".go")
			err := os.WriteFile(absFile, []byte(input), 0644)
			if err != nil {
				t.Error(err)
			}

			// Extract type declaration name from the first line.
			tokens := strings.SplitN(test.input, " ", 3)
			if len(tokens) != 3 {
				t.Fatalf("%s: need type declaration on first line", test.name)
			}

			g := Generator{typeName: tokens[1], compatIs: test.compatIs}
			g.loadPackage([]string{absFile})
			for _, e := range g.errors {
				g.generate(e)
			}
			got := string(g.format())
			expected := test.output + "\n\n"
			if got != expected {
				t.Errorf("%s: got(%d)\n====\n%q====\nexpected(%d)\n====\n%q",
					test.name, len(got), got, len(expected), expected)
			}
		})
	}
}
