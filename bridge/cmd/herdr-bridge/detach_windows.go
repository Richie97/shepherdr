//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strings"
)

const startTaskName = "herdr-bridge"

// startDetached starts the bridge through Task Scheduler. Processes started
// directly from an ssh or `hc exec` session live in that session's job
// object and are killed when it ends, and breakaway is not permitted there.
// A task the user owns runs outside the job and needs no admin rights.
func startDetached(bin string, args []string) error {
	tr := fmt.Sprintf(`"%s" %s`, bin, quoteArgs(args))
	steps := [][]string{
		{"schtasks", "/Create", "/F", "/TN", startTaskName, "/SC", "ONCE", "/ST", "00:00", "/TR", tr},
		{"schtasks", "/Run", "/TN", startTaskName},
	}
	for _, s := range steps {
		if out, err := exec.Command(s[0], s[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %w\n%s", s, err, out)
		}
	}
	return nil
}

func quoteArgs(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t") {
			a = `"` + a + `"`
		}
		q[i] = a
	}
	return strings.Join(q, " ")
}
