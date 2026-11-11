//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"
)

// installTask registers the bridge as a per-user Task Scheduler task that
// fires every minute and starts the bridge only when it isn't running
// (MultipleInstancesPolicy IgnoreNew). That one task starts it after logon
// and restarts it within a minute if it crashes or is killed. It has no time
// limit (the default kills tasks after 72 hours) and ignores battery state.
//
// Task Scheduler rather than a Run key or a direct start: processes started
// from an ssh or `hc exec` session live in that session's job object and are
// killed when it ends. A task the user owns runs outside the job and needs no
// admin rights; a logon trigger would, so the repeating trigger covers logon.
func installTask(bin string, args []string) error {
	xmlPath := filepath.Join(os.TempDir(), "herdr-bridge-task.xml")
	if err := os.WriteFile(xmlPath, utf16File(taskXML(bin, args, time.Now())), 0o644); err != nil {
		return err
	}
	defer os.Remove(xmlPath)
	return runSteps([][]string{
		{"schtasks", "/Create", "/F", "/TN", startTaskName, "/XML", xmlPath},
		{"schtasks", "/Run", "/TN", startTaskName},
	})
}

// pauseTask stops the watchdog from restarting the bridge while install
// replaces the binary. It fails harmlessly when the task doesn't exist yet.
func pauseTask() {
	exec.Command("schtasks", "/Change", "/TN", startTaskName, "/DISABLE").Run()
}

func taskXML(bin string, args []string, now time.Time) string {
	e := html.EscapeString
	return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Shepherdr hub: keeps herdr-bridge running. Fires every minute and does nothing while the bridge is already running.</Description>
  </RegistrationInfo>
  <Triggers>
    <TimeTrigger>
      <StartBoundary>` + now.Format("2006-01-02T15:04:05") + `</StartBoundary>
      <Enabled>true</Enabled>
      <Repetition>
        <Interval>PT1M</Interval>
        <StopAtDurationEnd>false</StopAtDurationEnd>
      </Repetition>
    </TimeTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <IdleSettings>
      <StopOnIdleEnd>false</StopOnIdleEnd>
      <RestartOnIdle>false</RestartOnIdle>
    </IdleSettings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>7</Priority>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>` + e(bin) + `</Command>
      <Arguments>` + e(quoteArgs(args)) + `</Arguments>
    </Exec>
  </Actions>
</Task>
`
}

// utf16File encodes s as UTF-16LE with a BOM, the encoding schtasks /XML
// reliably accepts.
func utf16File(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2+2*len(u))
	b[0], b[1] = 0xFF, 0xFE
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2+2*i:], c)
	}
	return b
}

func runSteps(steps [][]string) error {
	for _, s := range steps {
		fmt.Println("run  ", s)
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
