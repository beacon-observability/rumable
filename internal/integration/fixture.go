//go:build integration

package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// This local receiver proves that a browser sends events; it does not emulate Guance ingestion.
func main() {
	var mu sync.Mutex
	types := map[string]int{}
	requests := 0
	compressedRequests := 0
	cacheRequests := 0
	replaySegments, replayBytes := 0, 0
	http.HandleFunc("/v1/write/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", r.Header.Get("Access-Control-Request-Headers"))
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		b, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		mu.Lock()
		requests++
		var compressed io.ReadCloser
		if reader, err := gzip.NewReader(bytes.NewReader(b)); err == nil {
			compressed = reader
		} else if reader, err := zlib.NewReader(bytes.NewReader(b)); err == nil {
			compressed = reader
		} else {
			compressed = flate.NewReader(bytes.NewReader(b))
		}
		if compressed != nil {
			if decoded, err := io.ReadAll(io.LimitReader(compressed, 8<<20)); err == nil {
				b = decoded
				compressedRequests++
			}
			compressed.Close()
		}
		if kind, params, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err == nil && kind == "multipart/form-data" {
			reader := multipart.NewReader(bytes.NewReader(b), params["boundary"])
			for {
				part, err := reader.NextPart()
				if err != nil {
					break
				}
				if part.FormName() == "segment" {
					segment, _ := io.ReadAll(part)
					if len(segment) > 0 {
						replaySegments++
						replayBytes += len(segment)
					}
				}
				part.Close()
			}
		}
		for _, line := range strings.Split(string(b), "\n") {
			// The official SDK sends DataKit line protocol by default.
			if i := strings.IndexByte(line, ','); i > 0 {
				switch line[:i] {
				case "view", "resource", "action", "error", "long_task":
					types[line[:i]]++
					continue
				}
			}
			var event map[string]any
			if json.Unmarshal([]byte(line), &event) == nil {
				if kind, ok := event["type"].(string); ok {
					types[kind]++
				}
			}
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"code":200,"success":true}`)
	})
	http.HandleFunc("/__stats", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"requests": requests, "events": types, "replaySegments": replaySegments, "replayBytes": replayBytes, "compressedRequests": compressedRequests})
	})
	http.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"value": "</head>", "ok": true, "traceHeader": r.Header.Get("traceparent") != "", "visitorHeader": r.Header.Get("x-rum-visitor-id") != ""})
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if name := os.Getenv("RUMABLE_FIXTURE_FAILURE_FILE"); name != "" {
			if _, err := os.Stat(name); err == nil {
				http.Error(w, "intentional fixture failure", http.StatusServiceUnavailable)
				return
			}
		}
		if r.URL.Path == "/headers" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"host": r.Host, "authorization": r.Header.Get("Authorization"), "tenant": r.Header.Get("X-Tenant"), "encoding": r.Header.Get("Accept-Encoding"), "etag": r.Header.Get("If-None-Match"), "modified": r.Header.Get("If-Modified-Since")})
			return
		}
		if r.URL.Path == "/payload.json" {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"literal":"</HeAd>","ok":true}`)
			return
		}
		if r.URL.Path == "/status/503" {
			http.Error(w, "intentional fixture failure", http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		if r.URL.Path == "/favicon.ico" {
			w.WriteHeader(204)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("ETag", `"original"`)
		w.Header().Set("Last-Modified", "Mon, 19 Oct 2026 00:00:00 GMT")
		w.Header().Set("X-Backend-Secret", "fixture-internal")
		cdn := ""
		if os.Getenv("RUMABLE_FIXTURE_ALLOW_CDN") == "1" {
			cdn = " https://static.guance.com"
		}
		w.Header().Set("Content-Security-Policy", "script-src 'self'"+cdn+"; worker-src blob:; connect-src 'self' http://127.0.0.1:19181 http://localhost:19181; object-src 'none'")
		if strings.HasPrefix(r.URL.Path, "/cache") {
			mu.Lock()
			cacheRequests++
			w.Header().Set("X-Fixture-Request", strconv.Itoa(cacheRequests))
			mu.Unlock()
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		if r.URL.Path == "/missinghead" {
			io.WriteString(w, "<html><body>No head</body></html>")
			return
		}
		if r.URL.Path == "/download" {
			w.Header().Set("Content-Disposition", "attachment; filename=page.html")
		}
		if r.URL.Path == "/download" || r.URL.Path == "/api/html" {
			io.WriteString(w, "<html><head></HeAd><body>Byte-preserved HTML</body></html>")
			return
		}
		if r.URL.Path == "/app.js" {
			w.Header().Set("Content-Type", "application/javascript")
			io.WriteString(w, `
document.querySelector('#fetch').onclick=function(){fetch('/api/data').then(function(r){return r.json();}).then(function(r){document.querySelector('#result').textContent='Fetched';document.querySelector('#network-state').textContent=JSON.stringify({traceHeader:r.traceHeader,visitorHeader:r.visitorHeader});});};
document.querySelector('#error').onclick=function(){setTimeout(function(){throw new Error('Rumable intentional regression-test error');},0);};
document.querySelector('#inspect').onclick=function(){
  var sdk=window.DATAFLUX_RUM, config=sdk&&sdk.getInitConfiguration(), selected={};
  ['trackUserInteractions','compressIntakeRequests','defaultPrivacyLevel','sessionOnErrorSampleRate','sessionReplaySampleRate','sessionReplayOnErrorSampleRate','traceType','enablePrivacyForActionName','replayCanvasEnabled','sessionPersistence'].forEach(function(key){if(config)selected[key]=config[key];});
  document.querySelector('#sdk-state').textContent=JSON.stringify({rumable:window.__RUMABLE__,recording:sdk&&sdk.isRecording(),config:selected},null,2);
};
document.querySelector('#new-session').onclick=function(){window.DATAFLUX_RUM.stopSession();location.reload();};
document.querySelector('#canvas').onclick=function(){var c=document.querySelector('canvas').getContext('2d');c.fillStyle='blue';c.fillRect(0,0,160,30);};
`)
			return
		}
		if r.URL.Path == "/forced-gzip" {
			w.Header().Set("Content-Encoding", "gzip")
			g := gzip.NewWriter(w)
			io.WriteString(g, "<html><head></head><body>Compressed upstream</body></html>")
			g.Close()
			return
		}
		if r.URL.Path == "/conditional" && (r.Header.Get("If-None-Match") == `"original"` || r.Header.Get("If-Modified-Since") != "") {
			w.WriteHeader(304)
			return
		}
		if r.URL.Path == "/stream" {
			io.WriteString(w, "<html><head><title>Chunked</title>")
			w.(http.Flusher).Flush()
			time.Sleep(20 * time.Millisecond)
			io.WriteString(w, "</head><body>Streamed</body></html>")
			return
		}
		if r.URL.Path == "/split-head" {
			for _, part := range []string{"<html><He", "Ad></head><body>BUSINESS_ORIGINAL</body></html>"} {
				io.WriteString(w, part)
				w.(http.Flusher).Flush()
				time.Sleep(20 * time.Millisecond)
			}
			return
		}
		if r.URL.Path == "/script-head" {
			w.Header().Set("Content-Security-Policy", "script-src 'self' 'nonce-dGVzdA=='; worker-src blob:; connect-src 'self' http://127.0.0.1:19181")
			io.WriteString(w, `<!doctype html><html><head><title>Head anchor regression</title><script nonce="dGVzdA==">window.template="</head>";window.businessRan=true;</script></head><body><h1>Head anchor regression</h1><p id="business"></p><button id="inspect">Inspect SDK</button><pre id="sdk"></pre><script nonce="dGVzdA==">document.querySelector('#business').textContent=window.businessRan&&window.template==="</head>"?'Business script passed':'Business script FAILED';document.querySelector('#inspect').onclick=function(){document.querySelector('#sdk').textContent=JSON.stringify({rumable:window.__RUMABLE__,sdk:!!window.DATAFLUX_RUM})};</script></body></html>`)
			return
		}
		if r.URL.Path == "/business" {
			io.WriteString(w, "<html><head></head><body>BUSINESS_ORIGINAL</body></html>")
			return
		}
		fmt.Fprint(w, `<!doctype html><html><head><meta charset="utf-8"><title>Rumable verification</title><script defer src="/app.js"></script></head><body><h1>Rumable verification</h1><button id="fetch">Fetch resource</button><button id="error">Trigger test error</button><button id="inspect">Inspect recording</button><button id="new-session">New session</button><button id="canvas">Draw canvas</button><p id="result">Ready</p><pre id="network-state"></pre><pre id="sdk-state"></pre><canvas width="200" height="50"></canvas></body></html>`)
	})
	fmt.Println("fixture listening on :19181")
	if err := http.ListenAndServe(":19181", nil); err != nil {
		panic(err)
	}
}
