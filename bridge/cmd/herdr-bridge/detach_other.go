//go:build !windows

package main

import "errors"

func installTask(string, []string) error { return errors.New("installTask is Windows-only") }

func pauseTask() {}
