package tunnel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/0990/gotun/tun"
)

const mtrTimeout = 10 * time.Minute

type mtrSpec struct {
	Tunnel   string
	Protocol string
	Mode     string
	Host     string
	Port     string
}

type sseEvent struct {
	name    string
	payload interface{}
}

func MTRStream(mgr *tun.Manager) func(w http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		flusher, ok := writer.(http.Flusher)
		if !ok {
			http.Error(writer, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("Cache-Control", "no-cache")
		writer.Header().Set("Connection", "keep-alive")
		writer.Header().Set("X-Accel-Buffering", "no")
		writer.WriteHeader(http.StatusOK)
		flusher.Flush()

		name := strings.TrimSpace(request.URL.Query().Get("name"))
		if name == "" {
			writeSSE(writer, flusher, "error", map[string]string{"message": "lose name"})
			return
		}

		service, ok := mgr.GetService(name)
		if !ok {
			writeSSE(writer, flusher, "error", map[string]string{"message": "tun not exist"})
			return
		}
		if service.Cfg().Disabled {
			writeSSE(writer, flusher, "error", map[string]string{"message": "tunnel disabled"})
			return
		}

		spec, err := buildMTRSpec(name, service.Cfg().Output)
		if err != nil {
			writeSSE(writer, flusher, "error", map[string]string{"message": err.Error()})
			return
		}

		mtrPath, err := exec.LookPath("mtr")
		if err != nil {
			writeSSE(writer, flusher, "error", map[string]string{"message": "mtr not installed"})
			return
		}

		ctx, cancel := context.WithTimeout(request.Context(), mtrTimeout)
		defer cancel()

		args := spec.args()
		ptmx, cmd, err := startMTRSession(ctx, mtrPath, args)
		if err != nil {
			writeSSE(writer, flusher, "error", map[string]string{"message": err.Error()})
			return
		}
		defer ptmx.Close()

		startedAt := time.Now()
		if err := writeSSE(writer, flusher, "start", map[string]interface{}{
			"name":        spec.Tunnel,
			"protocol":    spec.Protocol,
			"mode":        spec.Mode,
			"target_host": spec.Host,
			"target_port": spec.Port,
			"target":      net.JoinHostPort(spec.Host, spec.Port),
			"command":     spec.commandString(),
		}); err != nil {
			return
		}

		eventCh := make(chan sseEvent, 64)
		waitCh := make(chan error, 1)

		go streamMTRPTY(ctx, ptmx, eventCh)
		go func() {
			waitCh <- cmd.Wait()
		}()

		pipeDone := false
		waitReceived := false

		for !(waitReceived && pipeDone) {
			select {
			case <-request.Context().Done():
				return
			case ev := <-eventCh:
				if ev.name == "__pipe_done" {
					pipeDone = true
					continue
				}
				if err := writeSSE(writer, flusher, ev.name, ev.payload); err != nil {
					return
				}
			case err := <-waitCh:
				waitReceived = true
				exitCode := 0
				if err != nil {
					var exitErr *exec.ExitError
					if errors.As(err, &exitErr) {
						exitCode = exitErr.ExitCode()
					} else {
						exitCode = -1
					}
				}

				timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
				if !timedOut && err != nil && !errors.Is(ctx.Err(), context.Canceled) {
					if writeErr := writeSSE(writer, flusher, "error", map[string]string{
						"message": err.Error(),
					}); writeErr != nil {
						return
					}
				}

				if writeErr := writeSSE(writer, flusher, "done", map[string]interface{}{
					"exit_code":   exitCode,
					"duration_ms": time.Since(startedAt).Milliseconds(),
					"timed_out":   timedOut,
				}); writeErr != nil {
					return
				}
			}
		}
	}
}

func buildMTRSpec(name string, output string) (mtrSpec, error) {
	parts := strings.SplitN(strings.TrimSpace(output), "@", 2)
	if len(parts) != 2 {
		return mtrSpec{}, errors.New("invalid output format")
	}

	proto := strings.ToLower(strings.TrimSpace(parts[0]))
	addr := strings.TrimSpace(parts[1])

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return mtrSpec{}, fmt.Errorf("invalid output addr: %w", err)
	}
	if host == "" {
		return mtrSpec{}, errors.New("invalid output host")
	}
	portNum, err := strconv.Atoi(port)
	if err != nil || portNum <= 0 || portNum > 65535 {
		return mtrSpec{}, errors.New("invalid output port")
	}

	mode := ""
	switch proto {
	case "tcp", "tcp_mux", "tcpmux":
		mode = "tcp"
	case "udp", "quic", "kcp", "kcp_mux", "kcpmux", "kcpx", "kcpx_mux":
		mode = "udp"
	default:
		return mtrSpec{}, fmt.Errorf("mtr unsupported protocol: %s", proto)
	}

	return mtrSpec{
		Tunnel:   name,
		Protocol: proto,
		Mode:     mode,
		Host:     host,
		Port:     port,
	}, nil
}

func (m mtrSpec) args() []string {
	args := []string{"--curses", "--no-dns"}
	if m.Mode == "tcp" {
		args = append(args, "--tcp")
	} else {
		args = append(args, "--udp")
	}
	args = append(args, "-P", m.Port, m.Host)
	return args
}

func (m mtrSpec) commandString() string {
	parts := append([]string{"mtr"}, m.args()...)
	return strings.Join(parts, " ")
}

func streamMTRPTY(ctx context.Context, r io.Reader, ch chan<- sseEvent) {
	defer func() {
		sendPipeEvent(ctx, ch, sseEvent{name: "__pipe_done"})
	}()

	buf := make([]byte, 8192)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := base64.StdEncoding.EncodeToString(buf[:n])
			sendPipeEvent(ctx, ch, sseEvent{
				name: "terminal",
				payload: map[string]string{
					"chunk": chunk,
				},
			})
		}

		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) || errors.Is(ctx.Err(), context.Canceled) {
			return
		}
		sendPipeEvent(ctx, ch, sseEvent{
			name: "error",
			payload: map[string]string{
				"message": err.Error(),
			},
		})
		return
	}
}

func sendPipeEvent(ctx context.Context, ch chan<- sseEvent, event sseEvent) {
	select {
	case ch <- event:
	case <-ctx.Done():
	}
}

func writeSSE(writer io.Writer, flusher http.Flusher, eventName string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(writer, "event: %s\n", eventName); err != nil {
		return err
	}
	for _, line := range strings.Split(string(body), "\n") {
		if _, err := fmt.Fprintf(writer, "data: %s\n", line); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprint(writer, "\n"); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
