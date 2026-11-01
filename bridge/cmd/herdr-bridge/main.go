// Command herdr-bridge exposes a local herdr-core to the Shepherdr iOS app
// over Tailscale. See docs/PLAN.md.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	"herdr-ui/bridge/internal/core"
	"herdr-ui/bridge/internal/machines"
	"herdr-ui/bridge/internal/pair"
	"herdr-ui/bridge/internal/push"
	"herdr-ui/bridge/internal/server"
	"herdr-ui/bridge/internal/tailnet"
)

var version = "0.1.0-dev"

const (
	defaultListen = "127.0.0.1:7777"
	servePort     = "8443"
	launchdLabel  = "dev.herdr-ui.bridge"
)

func main() {
	cmd := "serve"
	args := os.Args[1:]
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve(args)
	case "install":
		err = install(args)
	case "pair":
		err = pairCmd(args)
	case "version":
		fmt.Println(version)
	default:
		fmt.Fprintf(os.Stderr, "usage: herdr-bridge [serve|install|pair|version] [flags]\n")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "herdr-bridge:", err)
		os.Exit(1)
	}
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	listen := fs.String("listen", defaultListen, "address to listen on; must be loopback")
	transport := fs.String("transport", "auto", "how to reach herdr-core: socket, hc (`hc connect`), or auto (hc on Windows, socket elsewhere)")
	socket := fs.String("socket", core.DefaultSocketPath(), "herdr-core socket (transport=socket)")
	hcPath := fs.String("hc", "", "hc binary (transport=hc; default: found on PATH or install dir)")
	owner := fs.String("owner", "", "Tailscale login allowed to use the bridge (default: this node's owner)")
	noAuth := fs.Bool("no-auth", false, "skip the Tailscale identity check (local development only)")
	logFile := fs.String("log", "", "append logs to this file instead of stderr")
	fs.Parse(args)

	out := io.Writer(os.Stderr)
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	log := slog.New(slog.NewTextHandler(out, nil))

	client := &core.Client{}
	if *transport == "auto" {
		*transport = "socket"
		if runtime.GOOS == "windows" {
			*transport = "hc"
		}
	}
	switch *transport {
	case "socket":
		client.SocketPath = *socket
	case "hc":
		if *hcPath == "" {
			p, err := core.FindHC()
			if err != nil {
				return err
			}
			*hcPath = p
		}
		client.HCPath = *hcPath
	default:
		return fmt.Errorf("unknown -transport %q", *transport)
	}

	host, _, err := net.SplitHostPort(*listen)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("refusing to listen on %s: use a loopback address and expose it with tailscale serve", *listen)
	}

	if !*noAuth && *owner == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		st, err := tailnet.GetStatus(ctx)
		cancel()
		if err != nil {
			return fmt.Errorf("detect owner (pass -owner to skip): %w", err)
		}
		if *owner, err = st.OwnerLogin(); err != nil {
			return err
		}
	}
	if *noAuth {
		log.Warn("identity check disabled (-no-auth)")
	}

	pusher, err := setupPush(client, log)
	if err != nil {
		return err
	}

	h := server.New(server.Config{
		Push:    pusher,
		Core:    client,
		Version: version,
		Owner:   *owner,
		NoAuth:  *noAuth,
		Logger:  log,
	})
	srv := &http.Server{Addr: *listen, Handler: h, ReadHeaderTimeout: 10 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	watcher := &push.Watcher{Core: client, Notify: pusher.Alert, Logger: log, Cooldown: 20 * time.Second}
	go watcher.Run(ctx)
	go (&machines.Healer{Core: client, Logger: log}).Run(ctx)
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()

	log.Info("herdr-bridge listening", "addr", *listen, "transport", *transport, "socket", client.SocketPath, "hc", client.HCPath, "owner", *owner, "version", version)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

var plistTmpl = template.Must(template.New("plist").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>{{.Label}}</string>
  <key>ProgramArguments</key>
  <array>
    <string>{{.Binary}}</string>
    <string>serve</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>{{.Log}}</string>
  <key>StandardErrorPath</key><string>{{.Log}}</string>
</dict>
</plist>
`))

// install makes `herdr-bridge serve` start automatically and publishes it to
// the tailnet with `tailscale serve`: a launchd agent on macOS, a
// per-user Run key on Windows (no admin needed).
func install(args []string) error {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "print what would be done without doing it")
	fs.Parse(args)

	bin, err := os.Executable()
	if err != nil {
		return err
	}
	if bin, err = filepath.EvalSymlinks(bin); err != nil {
		return err
	}
	ts, err := tailnet.CLIPath()
	if err != nil {
		return err
	}
	serveStep := []string{ts, "serve", "--bg", "--https=" + servePort, "http://" + defaultListen}

	switch runtime.GOOS {
	case "darwin":
		err = installLaunchd(bin, *dryRun)
	case "windows":
		err = installWindows(bin, *dryRun)
	default:
		err = fmt.Errorf("install is not supported on %s; run `herdr-bridge serve` under your init system", runtime.GOOS)
	}
	if err != nil {
		return err
	}

	fmt.Println("run  ", serveStep)
	if *dryRun {
		return nil
	}
	// tailscale serve waits indefinitely when Serve isn't enabled on the
	// tailnet, after printing the URL to enable it. Don't hang with it.
	sctx, scancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer scancel()
	out, err := exec.CommandContext(sctx, serveStep[0], serveStep[1:]...).CombinedOutput()
	if sctx.Err() != nil {
		return fmt.Errorf("tailscale serve did not finish; it said:\n%s\nEnable Serve at that URL, then run install again", out)
	}
	if err != nil {
		return fmt.Errorf("%v: %w\n%s", serveStep, err, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if st, err := tailnet.GetStatus(ctx); err == nil {
		hub := fmt.Sprintf("https://%s:%s", st.HostName(), servePort)
		fmt.Printf("\nbridge available at %s/v1/health\n", hub)
		fmt.Printf("pair your phone: run `herdr-bridge pair`, or open %s/pair in a browser on your tailnet\n", hub)
	}
	return nil
}

// pairCmd prints a QR code the phone scans to find this hub.
func pairCmd(args []string) error {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	hub := fs.String("hub", "", "hub URL (default: this machine's tailnet name on port "+servePort+")")
	fs.Parse(args)
	if *hub == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		st, err := tailnet.GetStatus(ctx)
		if err != nil {
			return fmt.Errorf("find this machine's tailnet name (or pass -hub): %w", err)
		}
		*hub = fmt.Sprintf("https://%s:%s", st.HostName(), servePort)
	}
	link := pair.Link(*hub)
	code, err := pair.Terminal(link)
	if err != nil {
		return err
	}
	fmt.Print("\nScan with your iPhone camera (Tailscale on):\n\n", code, "\n")
	fmt.Printf("Hub:  %s\nLink: %s\nOr open %s/pair in a browser on your tailnet.\n", *hub, link, *hub)
	return nil
}

func installLaunchd(bin string, dryRun bool) error {
	home, _ := os.UserHomeDir()
	plistPath := filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
	logPath := filepath.Join(home, "Library", "Logs", "herdr-bridge.log")
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	steps := [][]string{
		{"launchctl", "bootout", domain + "/" + launchdLabel}, // may not be loaded yet
		{"launchctl", "bootstrap", domain, plistPath},
	}
	fmt.Printf("write %s (runs %s serve, logs to %s)\n", plistPath, bin, logPath)
	for _, s := range steps {
		fmt.Println("run  ", s)
	}
	if dryRun {
		return nil
	}
	f, err := os.Create(plistPath)
	if err != nil {
		return err
	}
	err = plistTmpl.Execute(f, map[string]string{"Label": launchdLabel, "Binary": bin, "Log": logPath})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	for i, s := range steps {
		if out, err := exec.Command(s[0], s[1:]...).CombinedOutput(); err != nil && i != 0 {
			return fmt.Errorf("%v: %w\n%s", s, err, out)
		}
	}
	return nil
}

const windowsRunKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

func installWindows(bin string, dryRun bool) error {
	// Always install to one place, so the Run key never changes. Upgrade by
	// uploading a new build under another name (herdr-bridge.new.exe) and
	// running its install: a running .exe can't be overwritten, so stop it
	// first, then copy this binary over it.
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return errors.New("LOCALAPPDATA is not set")
	}
	target := filepath.Join(local, "Programs", "herdr-bridge", "herdr-bridge.exe")
	logPath := filepath.Join(filepath.Dir(target), "herdr-bridge.log")
	serveArgs := []string{"serve", "-log", logPath}
	command := fmt.Sprintf(`"%s" serve -log "%s"`, target, logPath)
	reg := []string{"reg", "add", windowsRunKey, "/v", "herdr-bridge", "/t", "REG_SZ", "/d", command, "/f"}
	kill := []string{"taskkill", "/F", "/IM", "herdr-bridge.exe", "/FI", fmt.Sprintf("PID ne %d", os.Getpid())}

	fmt.Println("run  ", kill)
	if !strings.EqualFold(bin, target) {
		fmt.Printf("copy %s -> %s\n", bin, target)
	}
	fmt.Println("run  ", reg)
	fmt.Printf("start %s %v via Task Scheduler\n", target, serveArgs)
	if dryRun {
		return nil
	}

	exec.Command(kill[0], kill[1:]...).Run() // fails harmlessly when none is running
	if !strings.EqualFold(bin, target) {
		if err := copyFile(bin, target); err != nil {
			return err
		}
	}
	if out, err := exec.Command(reg[0], reg[1:]...).CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %w\n%s", reg, err, out)
	}
	return startDetached(target, serveArgs)
}

// copyFile copies src to dst, retrying briefly while a killed process
// releases dst.
func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	for i := 0; ; i++ {
		err = os.WriteFile(dst, b, 0o755)
		if err == nil || i == 10 {
			return err
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// setupPush loads the APNs key and device list from the config dir. Without
// apns.json the bridge still accepts registrations and logs alerts.
func setupPush(client *core.Client, log *slog.Logger) (*push.Pusher, error) {
	dir, err := push.ConfigDir()
	if err != nil {
		return nil, err
	}
	devices, err := push.OpenDevices(filepath.Join(dir, "devices.json"))
	if err != nil {
		return nil, fmt.Errorf("devices.json: %w", err)
	}
	sender, err := push.LoadSender(dir)
	if err != nil {
		return nil, err
	}
	if sender == nil {
		log.Warn("push disabled: no apns.json in " + dir)
	} else {
		log.Info("push enabled", "topic", sender.Topic, "devices", len(devices.List()))
	}

	var (
		mu  sync.Mutex
		hub string
	)
	hubName := func() string {
		mu.Lock()
		defer mu.Unlock()
		if hub == "" {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if raw, err := client.Call(ctx, "server.status", nil); err == nil {
				var st struct {
					Core struct {
						Name string `json:"name"`
					} `json:"core"`
				}
				if json.Unmarshal(raw, &st) == nil {
					hub = st.Core.Name
				}
			}
		}
		return hub
	}
	return &push.Pusher{Sender: sender, Devices: devices, Hub: hubName, Logger: log}, nil
}
