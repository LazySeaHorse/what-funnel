package proxy

import (
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

const (
	upstreamDialTimeout           = 10 * time.Second
	upstreamResponseHeaderTimeout = 30 * time.Second
)

// newTransport returns an upstream transport that bounds connection setup and
// time-to-first-byte but, unlike http.Client.Timeout, never limits how long a
// response body may stream.
func newTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: upstreamDialTimeout, KeepAlive: 30 * time.Second}).DialContext
	transport.ResponseHeaderTimeout = upstreamResponseHeaderTimeout
	return transport
}

// normalizeUpstream maps ws/wss schemes to http/https, which is what the transport dials.
func normalizeUpstream(upstream *url.URL) *url.URL {
	target := *upstream
	switch target.Scheme {
	case "ws":
		target.Scheme = "http"
	case "wss":
		target.Scheme = "https"
	}
	return &target
}

// newReverseProxy builds a ReverseProxy to upstream. Hop-by-hop headers are
// handled by httputil, upstream redirects are returned to the client rather than
// followed, WebSocket upgrades are proxied natively, and client-supplied internal
// and forwarding headers are replaced with trusted values. mutate, when non-nil,
// runs last to apply route-specific rewrites.
func newReverseProxy(upstream *url.URL, logger *slog.Logger, mutate func(*httputil.ProxyRequest)) *httputil.ReverseProxy {
	target := normalizeUpstream(upstream)
	return &httputil.ReverseProxy{
		Transport: newTransport(),
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			for key := range pr.Out.Header {
				if IsInternalHeader(key) {
					pr.Out.Header.Del(key)
				}
			}
			// X-Forwarded-* from the client were already removed by ReverseProxy.
			pr.SetXForwarded()
			pr.Out.Header.Del("X-Real-IP")
			if host, _, err := net.SplitHostPort(pr.In.RemoteAddr); err == nil {
				pr.Out.Header.Set("X-Real-IP", host)
			} else if pr.In.RemoteAddr != "" {
				pr.Out.Header.Set("X-Real-IP", pr.In.RemoteAddr)
			}
			if mutate != nil {
				mutate(pr)
			}
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			logger.Error("proxy: upstream error", "target", target.Redacted(), "path", r.URL.Path, "error", err)
			http.Error(w, "gateway error: upstream unavailable", http.StatusBadGateway)
		},
	}
}
