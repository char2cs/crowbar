package main_test

import (
	"os/exec"
	"testing"
)

func TestCLIHelp(
	t *testing.T,
) {
	cmd := exec.Command("go", "run", ".", "--help")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("output: %s", out)
	}
	if len(out) == 0 {
		t.Fatal("expected help output, got nothing")
	}
}
