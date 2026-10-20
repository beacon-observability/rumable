//go:build integration

package main

import (
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
)

func main() {
	var mu sync.Mutex
	var records []map[string]string
	http.HandleFunc("/records", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == "DELETE" {
			records = nil
		}
		_ = json.NewEncoder(w).Encode(records)
	})
	http.HandleFunc("/otel/v1/traces", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/x-protobuf" {
			http.Error(w, "expected OTLP/protobuf POST", 400)
			return
		}
		var reader io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			g, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "bad gzip", 400)
				return
			}
			defer g.Close()
			reader = g
		}
		b, err := io.ReadAll(io.LimitReader(reader, 8<<20))
		if err != nil {
			http.Error(w, "read failed", 400)
			return
		}
		mu.Lock()
		records = append(records, map[string]string{"bodyHex": hex.EncodeToString(b), "token": r.Header.Get("X-Token")})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/x-protobuf")
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/headers" {
			header := r.Header.Clone()
			header.Set("Host", r.Host)
			_ = json.NewEncoder(w).Encode(header)
			return
		}
		if r.URL.Path == "/api/value" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"ok":true}`)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<html><head><title>OTel acceptance</title></head><body><button onclick="fetch('/api/value').then(r=>r.text()).then(v=>document.getElementById('result').textContent=JSON.stringify({response:v,rum:window.__RUMABLE__||null,sdk:!!window.DATAFLUX_RUM}))">Request</button><pre id="result"></pre></body></html>`)
	})
	log.Fatal(http.ListenAndServe(":19781", nil))
}
