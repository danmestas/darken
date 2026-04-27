package main

import (
	"os"
	"os/exec"
)

func runImages(args []string) error {
	if len(args) == 0 {
		args = []string{"all"}
	}
	c := exec.Command("make", append([]string{"-C", "images"}, args...)...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
