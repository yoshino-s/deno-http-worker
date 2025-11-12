# Go deno-http-worker

A Go port of the Node/TypeScript `deno-http-worker`. It launches a `deno` subprocess running a tiny bootstrap server over a Unix domain socket, then proxies your HTTP requests and WebSocket upgrades through that socket.

## Install

This module requires a local `deno` binary in PATH.

```
go get github.com/yoshino-s/deno-http-worker/denohttpworker
```

## Quick start

```go
w, err := denohttpworker.NewFromScript(`export default { async fetch(req: Request) { return Response.json({ url: req.url }) } }`, nil)
if err != nil { panic(err) }
defer w.Terminate()

ctx := context.Background()
resp, err := w.Request(ctx, denohttpworker.RequestOptions{URL: "https://example.com/hi", Method: http.MethodGet})
if err != nil { panic(err) }

defer resp.Body.Close()
body, _ := io.ReadAll(resp.Body)
fmt.Println(string(body))
```

## API

- NewFromScript(source string, opts *Options) (*Worker, error)
- NewFromImport(importURL string, opts *Options) (*Worker, error)
- (*Worker) Request(ctx, RequestOptions) (*ResponseData, error)
- (*Worker) WebSocket(ctx, url string, headers http.Header) (*websocket.Conn, *http.Response, error)
- (*Worker) Shutdown(ctx) error
- (*Worker) Terminate() error
- (*Worker) AddEventListener(fn OnExitListener)

Options mirror the TS version: DenoExecutable, DenoBootstrapScriptPath, RunFlags, PrintCommand, PrintOutput, WorkingDir, Env, OnSpawn.

## Notes

- The bootstrap script is loaded from `deno-http-worker/deno-bootstrap/index.ts`. Ensure your working directory contains the TS project or set `DenoBootstrapScriptPath` explicitly.
- WebSocket support uses `nhooyr.io/websocket` and works through the same Unix socket transport.
