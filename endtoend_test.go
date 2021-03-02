// (c) Copyright 2021, Gorror Authors.
//
// Licensed under the terms of the GNU GPL License version 3.

package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEndToEnd(t *testing.T) {
	tmpdir, exePath := buildGorror(t)

	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}

	errorsSource := filepath.Join(tmpdir, "errors.go")
	for _, entry := range entries {
		t.Logf("run: %s %s\n", exePath, entry.Name())
		source := filepath.Join(tmpdir, entry.Name())
		err = copyFile(source, filepath.Join("testdata", entry.Name()))
		if err != nil {
			t.Fatalf("copying file to temporary directory: %s", err)
		}
		// Run gorror in temporary directory.
		err = run(exePath, "-type", "Err", "-output", errorsSource, source)
		if err != nil {
			t.Fatal(err)
		}
		// Run the binary in the temporary directory.
		err = run("go", "run", errorsSource, source)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func buildGorror(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "gorror.exe")
	err := run("go", "build", "-o", file)
	if err != nil {
		t.Fatalf("building gorror: %v", err)
	}
	return dir, file
}

func run(name string, arg ...string) error {
	cmd := exec.Command(name, arg...)
	cmd.Dir = "."
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func copyFile(to, from string) error {
	toFd, err := os.Create(to)
	if err != nil {
		return err
	}
	defer toFd.Close()
	fromFd, err := os.Open(from)
	if err != nil {
		return err
	}
	defer fromFd.Close()
	_, err = io.Copy(toFd, fromFd)
	return err
}
