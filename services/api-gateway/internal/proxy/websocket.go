package proxy

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// WebSocket dials the upstream server and copies bytes bidirectionally to support WebSocket proxying.
func WebSocket(upstreamURL *url.URL, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Connection"), "upgrade") ||
			!strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			http.Error(w, "websocket proxy: not an upgrade request", http.StatusBadRequest)
			return
		}

		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
			return
		}
		clientConn, _, err := hj.Hijack()
		if err != nil {
			logger.Error("websocket proxy: hijack failed", "error", err)
			return
		}
		defer clientConn.Close()

		upstreamAddr := upstreamURL.Host
		if !strings.Contains(upstreamAddr, ":") {
			if upstreamURL.Scheme == "https" || upstreamURL.Scheme == "wss" {
				upstreamAddr += ":443"
			} else {
				upstreamAddr += ":80"
			}
		}
		upstreamConn, err := net.Dial("tcp", upstreamAddr)
		if err != nil {
			logger.Error("websocket proxy: dial upstream failed", "addr", upstreamAddr, "error", err)
			_, _ = clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
			return
		}
		defer upstreamConn.Close()

		path := r.URL.Path
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
		reqStr := fmt.Sprintf("%s %s HTTP/1.1\r\n", r.Method, path)
		reqStr += fmt.Sprintf("Host: %s\r\n", upstreamURL.Host)
		for k, vals := range r.Header {
			if strings.EqualFold(k, "Host") || IsInternalHeader(k) {
				continue
			}
			for _, v := range vals {
				reqStr += fmt.Sprintf("%s: %s\r\n", k, v)
			}
		}
		reqStr += "\r\n"

		_, err = upstreamConn.Write([]byte(reqStr))
		if err != nil {
			logger.Error("websocket proxy: write upstream failed", "error", err)
			return
		}

		errChan := make(chan error, 2)
		cp := func(dst io.Writer, src io.Reader) {
			_, err := io.Copy(dst, src)
			errChan <- err
		}
		go cp(clientConn, upstreamConn)
		go cp(upstreamConn, clientConn)

		<-errChan
	})
}
