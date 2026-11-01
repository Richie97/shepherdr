package core

// allowed lists the herdr-core methods the phone may call through the bridge.
// Widen it on purpose, in its own commit, with a reason (see AGENTS.md).
//
// Deliberately absent: server.stop/restart, terminal.kill/rm, terminal.attach
// (needs framed mode; proxied separately later), machine.add/rm/rename,
// machine.exec, file.*, doc.set.
var allowed = map[string]bool{
	"hello":              true,
	"server.status":      true,
	"templates.list":     true,
	"terminal.list":      true,
	"terminal.get":       true,
	"terminal.read":      true,
	"terminal.send":      true,
	"terminal.keys":      true,
	"terminal.interrupt": true,
	"terminal.wait":      true,
	"terminal.run":       true,
	"terminal.label":     true,
	"terminal.put":       true,
	"machine.list":       true,
	"machine.connect":    true,
	"doc.get":            true,
}

// Allowed reports whether the bridge forwards method to the core.
func Allowed(method string) bool { return allowed[method] }
