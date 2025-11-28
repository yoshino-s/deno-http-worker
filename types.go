package denohttpworker

import (
	"io"
	"net/http"
)

// RequestOptions models the request sent to the worker via Request. URL should
// be an absolute URL (e.g. https://localhost/path). Body may be nil for
// methods that do not send a payload.
type RequestOptions struct {
	URL     string
	Method  string
	Body    io.Reader // optional
	Headers http.Header
}

// ResponseData mirrors important pieces of http.Response returned by the
// worker. The caller is responsible for closing Body.
type ResponseData struct {
	StatusCode  int
	Header      http.Header
	Body        io.ReadCloser
	Trailers    http.Header
	ExecutionID string
}

// OnExitListener is notified when the Deno process exits. It receives the
// exit code and, if applicable, a signal name.
type OnExitListener func(code int, signal string)

// Options controls worker process spawning and behavior.
type Options struct {
	// DenoExecutable can be either a single string command ("deno") or a slice where the first element is the binary
	// and the rest are pre-args (like a wrapper). If empty, defaults to "deno".
	DenoExecutable []string

	// DenoBootstrapScriptPath optionally overrides the bootstrap TS path.
	DenoBootstrapScriptPath string

	// DenoBootstrapScriptContent optionally overrides the bootstrap TS content.
	DenoBootstrapScriptContent string

	// RunFlags are passed to `deno run`.
	RunFlags []string

	// PrintCommand prints the command and args before spawn.
	PrintCommand bool

	// WorkingDir sets cmd.Dir when spawning deno.
	WorkingDir string

	// Env overrides environment variables. If nil, inherits parent.
	Env []string

	// OnSpawn is invoked right after the process starts.
	OnSpawn func(pid int)

	// OnStdout is invoked for each line of stdout.
	OnStdout func(line string)

	// OnStderr is invoked for each line of stderr.
	OnStderr func(line string)
}
