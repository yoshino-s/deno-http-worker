package denohttpworker

import (
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func denoExists() bool {
	_, err := exec.LookPath("deno")
	return err == nil
}

//go:embed test/echo-request.ts
var echoRequestScript string

//go:embed test/echo-websocket.ts
var echoWebSocketScript string

//go:embed test/echo-hono.ts
var echoHonoScript string

func TestJSONEcho(t *testing.T) {
	if !denoExists() {
		t.Skip("deno not found in PATH; skipping")
	}

	w, err := NewFromScript(string(echoRequestScript), &Options{})
	if err != nil {
		t.Fatalf("spawn worker: %v", err)
	}
	defer w.Terminate()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := w.Request(ctx, RequestOptions{URL: "https://localhost/hello?isee=you", Method: "POST"})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["url"] != "https://localhost/hello?isee=you" {
		t.Fatalf("unexpected url: %v", got["url"])
	}
}

func TestWebSocketEcho(t *testing.T) {
	if !denoExists() {
		t.Skip("deno not found in PATH; skipping")
	}

	w, err := NewFromScript(string(echoWebSocketScript), &Options{})
	if err != nil {
		t.Fatalf("spawn worker: %v", err)
	}
	defer w.Terminate()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws://localhost/echo"
	_, wsConn, resp, err := w.WebSocket(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial: %v (resp: %+v)", err, resp)
	}
	defer wsConn.Close(websocket.StatusNormalClosure, "bye")

	message := "hello websocket"
	if err := wsConn.Write(ctx, websocket.MessageText, []byte(message)); err != nil {
		t.Fatalf("websocket write: %v", err)
	}

	_, p, err := wsConn.Read(ctx)
	if err != nil {
		t.Fatalf("websocket read: %v", err)
	}
	if string(p) != message {
		t.Fatalf("unexpected websocket message: %q", p)
	}
}

func TestHonoEcho(t *testing.T) {
	if !denoExists() {
		t.Skip("deno not found in PATH; skipping")
	}

	w, err := NewFromScript(string(echoHonoScript), &Options{})
	if err != nil {
		t.Fatalf("spawn worker: %v", err)
	}
	defer w.Terminate()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := w.Request(ctx, RequestOptions{URL: "https://localhost/echo", Body: strings.NewReader("hello"), Method: "POST"})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	expected := "hello"
	if string(got) != expected {
		t.Fatalf("unexpected body: %q", got)
	}
}
