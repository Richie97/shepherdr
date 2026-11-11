//go:build windows

package main

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"
)

func TestTaskXML(t *testing.T) {
	bin := `C:\Users\A & B\herdr-bridge.exe`
	doc := taskXML(bin, []string{"serve", "-log", `C:\Users\A & B\herdr-bridge.log`}, time.Date(2026, 11, 6, 17, 0, 0, 0, time.Local))

	var task struct {
		Interval string `xml:"Triggers>TimeTrigger>Repetition>Interval"`
		Start    string `xml:"Triggers>TimeTrigger>StartBoundary"`
		Policy   string `xml:"Settings>MultipleInstancesPolicy"`
		Limit    string `xml:"Settings>ExecutionTimeLimit"`
		Battery  string `xml:"Settings>DisallowStartIfOnBatteries"`
		Command  string `xml:"Actions>Exec>Command"`
		Args     string `xml:"Actions>Exec>Arguments"`
	}
	// The declaration says UTF-16 for schtasks; the string itself is UTF-8.
	d := xml.NewDecoder(strings.NewReader(strings.Replace(doc, `encoding="UTF-16"`, "", 1)))
	if err := d.Decode(&task); err != nil {
		t.Fatal(err)
	}
	want := map[string][2]string{
		"interval": {task.Interval, "PT1M"},
		"start":    {task.Start, "2026-11-06T17:00:00"},
		"policy":   {task.Policy, "IgnoreNew"},
		"limit":    {task.Limit, "PT0S"},
		"battery":  {task.Battery, "false"},
		"command":  {task.Command, bin},
		"args":     {task.Args, `serve -log "C:\Users\A & B\herdr-bridge.log"`},
	}
	for k, v := range want {
		if v[0] != v[1] {
			t.Errorf("%s = %q, want %q", k, v[0], v[1])
		}
	}
}

func TestUTF16File(t *testing.T) {
	b := utf16File("<a/>")
	if want := []byte{0xFF, 0xFE, '<', 0, 'a', 0, '/', 0, '>', 0}; string(b) != string(want) {
		t.Errorf("got % x", b)
	}
}
