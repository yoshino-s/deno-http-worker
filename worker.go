package denohttpworker

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

//go:embed bootstrap.ts
var bootstrapScript []byte

// EarlyExitError mirrors the EarlyExitDenoHTTPWorkerError used on the
// TypeScript side. It is returned when the Deno process exits before the
// worker becomes ready (e.g. bootstrap failure, syntax error) or a timeout
// occurs while waiting for the socket.
type EarlyExitError struct {
	Msg    string
	Stderr string
	Stdout string
	Code   int
	Signal string
}

func (e *EarlyExitError) Error() string { return e.Msg }

// Worker encapsulates a running Deno process plus an http.Client that routes
// requests over the worker's private UNIX domain socket. Use NewFromScript or
// NewFromImport to construct a Worker; remember to call Terminate or Shutdown
// when you are done to release resources and remove the socket file.
type Worker struct {
	socketPath string
	cmd        *exec.Cmd
	stdoutR    io.ReadCloser
	stderrR    io.ReadCloser
	httpClient *http.Client

	mu       sync.Mutex
	exited   bool
	exitOnce sync.Once
	onExit   []OnExitListener

	// buffers for early-exit diagnostics
	outBuf bytes.Buffer
	errBuf bytes.Buffer
}

// NewFromScript starts a worker from an in-memory TypeScript / TSX source
// string. The code is passed to the bootstrap via a data: URL. The caller is
// responsible for keeping a reference to the returned Worker and shutting it
// down.
func NewFromScript(script string, opts *Options) (*Worker, error) {
	return newWorker(script, false, opts)
}

// NewFromImport starts a worker from an import URL string (for example an
// HTTP(S) URL or local file path supported by Deno). The import string is
// forwarded verbatim to the bootstrap script which then loads it. Use this
// when the code already exists on disk or remotely.
func NewFromImport(importURL string, opts *Options) (*Worker, error) {
	return newWorker(importURL, true, opts)
}

func newWorker(source string, isImport bool, opts *Options) (*Worker, error) {
	if opts == nil {
		opts = &Options{}
	}
	denoExec := opts.DenoExecutable
	if len(denoExec) == 0 {
		denoExec = []string{"deno"}
	}

	// Generate unique socket path in tmp dir
	socketPath := filepath.Join(os.TempDir(), randomID()+"-deno-http.sock")

	var bs []byte
	var err error

	// Load bootstrap content from file
	if opts.DenoBootstrapScriptPath != "" {
		bs, err = os.ReadFile(opts.DenoBootstrapScriptPath)
		if err != nil {
			return nil, fmt.Errorf("read bootstrap script: %w", err)
		}
	} else {
		bs = bootstrapScript
	}

	if err != nil {
		return nil, fmt.Errorf("read bootstrap: %w", err)
	}
	dataURL := "data:text/typescript," + urlEncode(string(bs))

	// Build args
	args := append([]string{}, denoExec[1:]...)
	args = append(args, "run")

	// Ensure read/write permissions include socket (and script if import string file)
	runFlags := make([]string, 0, len(opts.RunFlags)+2)
	runFlags = append(runFlags, opts.RunFlags...)

	allowReadFound := false
	allowWriteFound := false
	for i, f := range runFlags {
		switch {
		case f == "--allow-read" || f == "--allow-all":
			allowReadFound = true
		case f == "--allow-write" || f == "--allow-all":
			allowWriteFound = true
		case strings.HasPrefix(f, "--allow-read="):
			allowReadFound = true
			runFlags[i] = f + "," + allowReadValue(source, isImport, socketPath)
		case strings.HasPrefix(f, "--allow-write="):
			allowWriteFound = true
			runFlags[i] = f + "," + socketPath
		}
	}
	if !allowReadFound {
		runFlags = append(runFlags, "--allow-read="+allowReadValue(source, isImport, socketPath))
	}
	if !allowWriteFound {
		runFlags = append(runFlags, "--allow-write="+socketPath)
	}
	args = append(args, runFlags...)
	args = append(args, dataURL)
	if isImport {
		args = append(args, socketPath, "import", source)
	} else {
		args = append(args, socketPath, "script", source)
	}

	if opts.PrintCommand {
		fmt.Println("Spawning deno process:", append([]string{denoExec[0]}, args...))
	}

	cmd := exec.Command(denoExec[0], args...)
	cmd.Dir = opts.WorkingDir
	if opts.Env != nil {
		cmd.Env = opts.Env
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	w := &Worker{socketPath: socketPath, cmd: cmd, stdoutR: stdout, stderrR: stderr}

	// Capture output for diagnostics and optional printing
	go w.pipe(stdout, &w.outBuf, opts.OnStdout)
	go w.pipe(stderr, &w.errBuf, opts.OnStderr)

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	if opts.OnSpawn != nil {
		opts.OnSpawn(cmd.Process.Pid)
	}

	// Exit watcher
	go func() {
		err := cmd.Wait()
		code, sig := exitStatus(err)
		w.mu.Lock()
		w.exited = true
		listeners := append([]OnExitListener{}, w.onExit...)
		w.mu.Unlock()
		for _, fn := range listeners {
			fn(code, sig)
		}
		// Best-effort cleanup
		_ = os.Remove(socketPath)
	}()

	// Wait until socket is created or process died
	deadline := time.Now().Add(30 * time.Second)
	for {
		if time.Now().After(deadline) {
			_ = w.Terminate()
			return nil, &EarlyExitError{Msg: "Timed out waiting for Deno to be ready", Stderr: w.errBuf.String(), Stdout: w.outBuf.String(), Code: -1, Signal: ""}
		}
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		// Check early exit
		if w.isExited() {
			code, sig := 1, ""
			if st := cmd.ProcessState; st != nil {
				code = st.ExitCode()
			}
			return nil, &EarlyExitError{Msg: "Deno exited before being ready", Stderr: w.errBuf.String(), Stdout: w.outBuf.String(), Code: code, Signal: sig}
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Create HTTP client using unix socket
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// always dial the worker unix socket
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
	}
	w.httpClient = &http.Client{Transport: transport}

	// Warm request
	_ = w.warmRequest()

	return w, nil
}

func (w *Worker) pipe(r io.Reader, buf *bytes.Buffer, hook func(line string)) {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			buf.Write(line)
			if hook != nil {
				hook(string(line))
			}
		}
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			return
		}
	}
}

// isExited reports whether the underlying Deno process has fully exited.
// Internal helper; not concurrency-safe for external callers.
func (w *Worker) isExited() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.exited
}

// Terminate force-kills (SIGKILL) the Deno process and removes the socket
// file. Prefer Shutdown for graceful termination when possible. Safe to call
// multiple times.
func (w *Worker) Terminate() error {
	if w == nil || w.cmd == nil || w.cmd.Process == nil {
		return nil
	}
	// SIGKILL
	_ = w.cmd.Process.Kill()
	_ = os.Remove(w.socketPath)
	return nil
}

// Shutdown sends SIGINT to the Deno process and waits for it to exit until
// the provided context is done. If the context expires first, ctx.Err() is
// returned. Use Terminate to forcefully kill instead.
func (w *Worker) Shutdown(ctx context.Context) error {
	if w == nil || w.cmd == nil || w.cmd.Process == nil {
		return nil
	}
	// SIGINT
	_ = w.cmd.Process.Signal(os.Interrupt)
	ch := make(chan struct{})
	go func() { _ = w.cmd.Wait(); close(ch) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ch:
		return nil
	}
}

// AddEventListener registers an OnExitListener which is invoked after the
// process exits. Listeners run in a goroutine after Wait() returns. They are
// best-effort; if Terminate removes the socket early they still fire.
func (w *Worker) AddEventListener(listener OnExitListener) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onExit = append(w.onExit, listener)
}

// Stdout returns a reader for the worker process's stdout stream. Lines are
// also optionally delivered to Options.OnStdout.
func (w *Worker) Stdout() io.Reader { return w.stdoutR }

// Stderr returns a reader for the worker process's stderr stream. Lines are
// also optionally delivered to Options.OnStderr.
func (w *Worker) Stderr() io.Reader { return w.stderrR }

// Request sends an HTTP request through the worker's UNIX socket transport.
// The URL should be absolute (scheme + host + path) as expected by the
// JavaScript handler. Headers are augmented with X-Deno-Worker-* metadata.
// The ResponseData contains Body; the caller must Close it when finished.
func (w *Worker) Request(ctx context.Context, opts RequestOptions) (*ResponseData, error) {
	if w == nil || w.httpClient == nil {
		return nil, fmt.Errorf("worker not initialized")
	}
	if opts.Method == "" {
		opts.Method = http.MethodGet
	}

	hdr := processHeaders(cloneHeader(opts.Headers), opts.URL)
	req, err := http.NewRequestWithContext(ctx, opts.Method, "http://deno/", opts.Body)
	if err != nil {
		return nil, err
	}
	req.Header = hdr

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	rd := &ResponseData{StatusCode: resp.StatusCode, Header: resp.Header, Body: resp.Body, Trailers: resp.Trailer}
	return rd, nil
}

func (w *Worker) warmRequest() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://deno/", nil)
	_, err := w.httpClient.Do(req)
	return err
}

func cloneHeader(h http.Header) http.Header {
	if h == nil {
		return http.Header{}
	}
	out := http.Header{}
	for k, vv := range h {
		cp := make([]string, len(vv))
		copy(cp, vv)
		out[k] = cp
	}
	return out
}

func allowReadValue(source string, isImport bool, socket string) string {
	if isImport {
		return socket + "," + source
	}
	return socket
}

func randomID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// urlEncode mimics JavaScript's encodeURIComponent
func urlEncode(s string) string {
	// Characters not escaped by encodeURIComponent
	const unreserved = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.!~*'()"
	isUnreserved := func(r rune) bool {
		return strings.ContainsRune(unreserved, r)
	}
	var b strings.Builder
	b.Grow(len(s) * 3)
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		if r == utf8.RuneError && size == 1 {
			// invalid byte, percent-encode
			b.WriteString("%FF")
			continue
		}
		if isUnreserved(r) {
			b.WriteRune(r)
			continue
		}
		// percent-encode as UTF-8 bytes
		var buf [utf8.UTFMax]byte
		n := utf8.EncodeRune(buf[:], r)
		for i := 0; i < n; i++ {
			b.WriteString(fmt.Sprintf("%%%02X", buf[i]))
		}
	}
	return b.String()
}

func exitStatus(waitErr error) (code int, signal string) {
	if waitErr == nil {
		return 0, ""
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			if status.Signaled() {
				return 1, status.Signal().String()
			}
			return status.ExitStatus(), ""
		}
	}
	return 1, ""
}
