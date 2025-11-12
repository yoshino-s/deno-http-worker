// Package denohttpworker provides a tiny bridge for running a Deno-based HTTP
// or WebSocket handler from a Go program. It spawns a Deno process, exposes a
// UNIX-domain-socket-backed http.Client, and forwards requests through that
// socket to the Deno runtime using a small bootstrap script.
//
// Typical usage
//
//  w, err := denohttpworker.NewFromScript(tsSource, &denohttpworker.Options{})
//  if err != nil {
//      log.Fatal(err)
//  }
//  defer w.Terminate()
//
//  resp, err := w.Request(ctx, denohttpworker.RequestOptions{
//      URL:    "https://localhost/hello",
//      Method: http.MethodGet,
//  })
//  if err != nil { /* handle */ }
//  defer resp.Body.Close()
//
// Notes
//   - NewFromScript accepts TypeScript/TSX source code directly. Use
//     NewFromImport to provide an import URL (e.g. https://… or file://…).
//   - The worker listens on a private UNIX socket path created in the system
//     temp directory; requests are sent via an internal http.Client configured
//     with a custom DialContext.
//   - Shutdown attempts a graceful exit by sending SIGINT; Terminate force-kills
//     the process.
package denohttpworker
