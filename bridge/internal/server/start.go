package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"time"

	"herdr-ui/bridge/internal/core"
)

// startRequest starts an agent the way the Herdr app does: a kept shell
// labeled into a Herdr workspace and tab, with the agent command typed in.
type startRequest struct {
	// On is the machine; empty for the hub's own core.
	On  string `json:"on"`
	Cwd string `json:"cwd"`
	// Command is typed into the shell, e.g. "claude" or "codex"; empty for a plain shell.
	Command string `json:"command"`
	// Task is sent to the agent once it reports it is ready.
	Task string `json:"task"`
	Name string `json:"name"`
	// Space and Tab are Herdr ids local to that machine; empty makes new ones.
	Space string `json:"space"`
	Tab   string `json:"tab"`
}

var (
	nameRE = regexp.MustCompile(`^[a-z0-9_.-]{1,32}$`)
	idRE   = regexp.MustCompile(`^(sp|tb)[0-9a-f]{8,64}$`)
)

// newHerdrID makes an id in Herdr's format: a prefix and 32 hex digits.
func newHerdrID(prefix string) string {
	b := make([]byte, 16)
	rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

// taskWait bounds how long the bridge waits for a new agent to be ready for
// its task (it may first ask to trust the folder, which the user answers).
var taskWait = 10 * time.Minute

func (s *server) start(w http.ResponseWriter, r *http.Request) {
	var req startRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	switch {
	case req.Name != "" && !nameRE.MatchString(req.Name):
		writeError(w, http.StatusBadRequest, "bad_request", "name must be 1-32 of a-z 0-9 - _ .")
		return
	case req.Space != "" && !idRE.MatchString(req.Space), req.Tab != "" && !idRE.MatchString(req.Tab):
		writeError(w, http.StatusBadRequest, "bad_request", "space/tab must be Herdr ids")
		return
	}
	if req.Space == "" {
		req.Space = newHerdrID("sp")
	}
	if req.Tab == "" {
		req.Tab = newHerdrID("tb")
	}

	params := map[string]any{
		"keep":      true,
		"no_parent": true,
		"labels":    map[string]string{"herdr.space": req.Space, "herdr.tab": req.Tab},
	}
	for k, v := range map[string]string{"on": req.On, "cwd": req.Cwd, "name": req.Name} {
		if v != "" {
			params[k] = v
		}
	}
	s.cfg.Logger.Info("input", "method", "start", "on", req.On, "cwd", req.Cwd, "command", req.Command, "task_len", len(req.Task))

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	raw, err := s.cfg.Core.Call(ctx, "terminal.run", params)
	if err != nil {
		s.writeCoreError(w, err)
		return
	}
	var run struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal(raw, &run); err != nil || run.Address == "" {
		writeError(w, http.StatusBadGateway, "bad_response", "terminal.run returned no address")
		return
	}
	if req.Command != "" {
		if _, err := s.cfg.Core.Call(ctx, "terminal.send", map[string]any{"terminal": run.Address, "text": req.Command}); err != nil {
			s.writeCoreError(w, err)
			return
		}
	}
	if req.Task != "" {
		if req.Command == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "a task needs an agent command")
			return
		}
		go s.deliverTask(run.Address, req.Task)
	}
	writeJSON(w, http.StatusOK, map[string]any{"address": run.Address, "space": req.Space, "tab": req.Tab})
}

// deliverTask waits until the new agent reports idle, then sends the task.
func (s *server) deliverTask(address, task string) {
	ctx, cancel := context.WithTimeout(context.Background(), taskWait)
	defer cancel()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			s.cfg.Logger.Warn("task not delivered: agent never became ready", "terminal", address)
			return
		case <-tick.C:
		}
		raw, err := s.cfg.Core.Call(ctx, "terminal.get", map[string]string{"terminal": address})
		if err != nil {
			var rpcErr *core.RPCError
			if errors.As(err, &rpcErr) { // gone
				s.cfg.Logger.Warn("task not delivered", "terminal", address, "err", err)
				return
			}
			continue
		}
		var t struct {
			State  string `json:"state"`
			Status *struct {
				State string `json:"state"`
			} `json:"status"`
		}
		json.Unmarshal(raw, &t)
		if t.State != "" && t.State != "live" {
			return
		}
		if t.Status == nil || (t.Status.State != "idle" && t.Status.State != "done") {
			continue
		}
		if _, err := s.cfg.Core.Call(ctx, "terminal.send", map[string]any{"terminal": address, "text": task}); err != nil {
			s.cfg.Logger.Warn("task not delivered", "terminal", address, "err", err)
			return
		}
		s.cfg.Logger.Info("input", "method", "start.task", "terminal", address, "text_len", len(task))
		return
	}
}

func (s *server) writeCoreError(w http.ResponseWriter, err error) {
	var rpcErr *core.RPCError
	if errors.As(err, &rpcErr) {
		writeJSON(w, http.StatusOK, map[string]any{"error": rpcErr})
		return
	}
	writeError(w, http.StatusBadGateway, "core_unavailable", err.Error())
}
