//go:build ignore

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSyncRejectsUnpublishedOrIncompatibleSDK(t *testing.T) {
	bundle := []byte(`sdkVersion:"3.3.21",sdkName:"df_web_rum_sdk";DATAFLUX_RUM.startSessionReplayRecording()`)
	for _, tc := range []struct {
		metadata string
		ok       bool
	}{
		{`{"version":"3.3.21","license":"MIT"}`, true},
		{`{"version":"3.3.22","license":"MIT"}`, false},
		{`{"version":"3.3.21-rc.1","license":"MIT"}`, false},
		{`{"version":"4.0.0","license":"MIT"}`, false},
		{`{"version":"3.3.21","license":"changed"}`, false},
	} {
		m, err := sdkMetadata(bundle, []byte(tc.metadata))
		if (err == nil) != tc.ok {
			t.Fatalf("%s: %v", tc.metadata, err)
		}
		if tc.ok && !strings.Contains(string(m), sdkSource) {
			t.Fatal("official provenance lost")
		}
	}
	if _, err := sdkMetadata([]byte(`<html>error</html>`), []byte(`{"version":"3.3.21","license":"MIT"}`)); err == nil {
		t.Fatal("HTML accepted as SDK")
	}
}

func TestSyncDownloadRejectsRedirectAndWrongResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/sdk", 302)
			return
		}
		if r.URL.Path == "/sdk" {
			w.Header().Set("Content-Type", "application/javascript")
			w.Write([]byte("/* sdk */"))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html>error</html>"))
	}))
	defer server.Close()
	if _, err := download(server.URL+"/sdk", "application/javascript"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/redirect", "/wrong"} {
		if _, err := download(server.URL+p, "application/javascript"); err == nil {
			t.Fatal("unexpected response accepted", p)
		}
	}
}
