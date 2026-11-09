// Command tokenproxy is a reverse proxy that injects a bearer token and a
// session ID into every request. Manual A2A tests use it where OpenShell's
// supervisor would normally inject credentials, but the token under test must
// come from a controllable issuer.
//
// Usage:
//
//	tokenproxy -listen 127.0.0.1:7000 -target http://127.0.0.1:8081 \
//	  -token "$(curl -s 'http://127.0.0.1:9099/token' | jq -r .token)"
//
// An empty token forwards the request without an Authorization header, which
// exercises the missing_token path. -token-file is re-read on every request so
// the token can be swapped between runs without restarting the proxy.
package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:7000", "listen address")
	target := flag.String("target", "http://127.0.0.1:8081", "upstream base URL")
	token := flag.String("token", "", "bearer token to inject (empty forwards no Authorization)")
	tokenFile := flag.String("token-file", "", "file re-read per request for the bearer token")
	sessionID := flag.String("session-id", "", "x-session-id header to inject")
	flag.Parse()

	targetURL, err := url.Parse(*target)
	if err != nil || !targetURL.IsAbs() {
		log.Fatalf("invalid -target %q", *target)
	}

	var mu sync.Mutex
	current := func() string {
		if *tokenFile == "" {
			return *token
		}
		mu.Lock()
		defer mu.Unlock()
		data, err := os.ReadFile(*tokenFile)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(data))
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(targetURL)
			pr.Out.Host = targetURL.Host
			if tok := current(); tok != "" {
				pr.Out.Header.Set("Authorization", "Bearer "+tok)
			} else {
				pr.Out.Header.Del("Authorization")
			}
			if *sessionID != "" {
				pr.Out.Header.Set("x-session-id", *sessionID)
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			log.Printf("%s %s -> %d", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode)
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("%s %s -> transport error: %v", r.Method, r.URL.Path, err)
			w.WriteHeader(http.StatusBadGateway)
		},
	}

	log.Printf("tokenproxy listening on %s -> %s", *listen, *target)
	log.Fatal(http.ListenAndServe(*listen, proxy))
}
