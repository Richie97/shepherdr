//go:build !windows

package main

import "errors"

func startDetached(string, []string) error { return errors.New("startDetached is Windows-only") }
