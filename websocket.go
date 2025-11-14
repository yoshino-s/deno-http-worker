package denohttpworker

import (
	"context"
	"net/http"

	"github.com/coder/websocket"
)

// WebSocket dials a WebSocket URL through the worker's UNIX-socket-backed
// HTTP client. The headers map can be nil; it will be populated with internal
// X-Deno-Worker-* headers and Connection: upgrade as needed.
func (w *Worker) WebSocket(ctx context.Context, url string, headers http.Header) (string, *websocket.Conn, *http.Response, error) {
	if headers == nil {
		headers = http.Header{}
	}

	executionID, headers := processHeaders(headers, url)
	headers.Set("X-Deno-Worker-Connection", "upgrade")
	// Use the same HTTP client (with unix socket transport)
	opts := &websocket.DialOptions{HTTPClient: w.httpClient, HTTPHeader: headers}
	conn, res, err := websocket.Dial(ctx, url, opts)

	return executionID, conn, res, err
}
