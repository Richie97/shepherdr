package push

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// Device is a registered phone.
type Device struct {
	Token string `json:"token"`
	// Sandbox is true for development builds (installed from Xcode).
	Sandbox bool      `json:"sandbox"`
	Name    string    `json:"name,omitempty"`
	Added   time.Time `json:"added"`
}

var tokenRE = regexp.MustCompile(`^[0-9a-fA-F]{32,200}$`)

// Devices is a small JSON-file-backed set of devices, keyed by token.
type Devices struct {
	path string
	mu   sync.Mutex
	list []Device
}

// OpenDevices loads path, or starts empty if it doesn't exist.
func OpenDevices(path string) (*Devices, error) {
	d := &Devices{path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return d, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &d.list); err != nil {
		return nil, err
	}
	return d, nil
}

// Add registers or updates a device.
func (d *Devices) Add(dev Device) error {
	if !tokenRE.MatchString(dev.Token) {
		return errors.New("device token must be hex")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if dev.Added.IsZero() {
		dev.Added = time.Now().UTC()
	}
	for i, existing := range d.list {
		if existing.Token == dev.Token {
			d.list[i] = dev
			return d.save()
		}
	}
	d.list = append(d.list, dev)
	return d.save()
}

// Remove forgets a token.
func (d *Devices) Remove(token string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, existing := range d.list {
		if existing.Token == token {
			d.list = append(d.list[:i], d.list[i+1:]...)
			return d.save()
		}
	}
	return nil
}

// List returns a copy of the registered devices.
func (d *Devices) List() []Device {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]Device(nil), d.list...)
}

// save writes atomically (temp file + rename). Callers hold mu.
func (d *Devices) save() error {
	if err := os.MkdirAll(filepath.Dir(d.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(d.list, "", "  ")
	if err != nil {
		return err
	}
	tmp := d.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, d.path)
}
