package denohttpworker

import (
    "net/http"
)

// processHeaders adds X-Deno-Worker headers and preserves host/connection.
func processHeaders(h http.Header, url string) http.Header {
    if h == nil {
        h = http.Header{}
    }
    h.Set("X-Deno-Worker-URL", url)
    // Clear potentially user-specified internal headers
    h.Del("X-Deno-Worker-Host")
    h.Del("X-Deno-Worker-Connection")

    if host := h.Get("Host"); host != "" {
        h.Set("X-Deno-Worker-Host", host)
    }
    if conn := h.Get("Connection"); conn != "" {
        h.Set("X-Deno-Worker-Connection", conn)
    }
    return h
}
