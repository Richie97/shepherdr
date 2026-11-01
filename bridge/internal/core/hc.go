package core

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// FindHC locates the hc binary: PATH (hc, herdr-core), then the default
// install locations.
func FindHC() (string, error) {
	for _, name := range []string{"hc", "herdr-core"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	home, _ := os.UserHomeDir()
	candidates := []string{filepath.Join(home, ".local", "bin", "herdr-core")}
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			candidates = append([]string{filepath.Join(local, "Programs", "herdr-core", "hc.exe")}, candidates...)
		}
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("hc not found on PATH or in %v", candidates)
}

// hcConn is a running `hc connect` whose stdin/stdout carry the protocol.
type hcConn struct {
	cmd *exec.Cmd
	w   io.WriteCloser
	r   io.ReadCloser
}

func dialHC(path string) (io.ReadWriteCloser, error) {
	cmd := exec.Command(path, "connect")
	hideWindow(cmd)
	w, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	r, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s connect: %w", path, err)
	}
	return &hcConn{cmd: cmd, w: w, r: r}, nil
}

func (c *hcConn) Read(p []byte) (int, error)  { return c.r.Read(p) }
func (c *hcConn) Write(p []byte) (int, error) { return c.w.Write(p) }

// Close ends the session: closing stdin lets hc exit; kill it if it doesn't.
func (c *hcConn) Close() error {
	c.w.Close()
	if c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}
	c.cmd.Wait()
	return nil
}
