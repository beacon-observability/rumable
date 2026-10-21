package rumable

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func TestIISFirstEnableRecoveryRetainsMapping(t *testing.T) {
	root := t.TempDir()
	s := testSite()
	s.Nginx, s.IIS = nil, &IIS{Site: "Website", Application: "/"}
	var unhealthy atomic.Bool
	unhealthy.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if unhealthy.Load() {
			w.WriteHeader(503)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<html><head></head></html>")
	}))
	defer server.Close()
	s.PublicURL = server.URL + "/"
	st, err := prepare(root, s)
	if err != nil {
		t.Fatal(err)
	}
	original := json.RawMessage(`{"rules":[],"preconditions":[],"virtualDirectory":null,"compressionPresent":false,"compressionValue":null}`)
	after := json.RawMessage(`{"rules":["<rule name=\"rumable-test-inject\" />"],"preconditions":["<preCondition name=\"rumable-test-html\" />"],"virtualDirectory":"<virtualDirectory path=\"/__rumable__\" physicalPath=\"C:\\Rumable\\sites\\test\\public\" />","compressionPresent":true,"compressionValue":"false"}`)
	st.Current.Native, st.OriginalIIS = original, original
	if err = saveState(root, st); err != nil {
		t.Fatal(err)
	}
	target, public, err := render(root, s, true)
	if err != nil {
		t.Fatal(err)
	}
	target.Native = after
	restored, err := retainedIISRecovery(transaction{Before: st, After: target, NativeCommitted: true})
	if err != nil {
		t.Fatal(err)
	}
	if sameJSON(original, restored) || sameJSON(after, restored) {
		t.Fatal("fixture must have a distinct retained-mapping state")
	}
	live, restores := original, 0
	bridge := iisCommand
	t.Cleanup(func() { iisCommand = bridge })
	iisCommand = func(_ string, _ Site, action string, extra map[string]any) (json.RawMessage, error) {
		switch action {
		case "inspect":
			return live, nil
		case "apply", "restore":
			if !sameJSON(extra["expected"].(json.RawMessage), live) {
				t.Fatal("lost compare-before-write protection")
			}
			if action == "apply" {
				live = after
			} else {
				live = restored
				restores++
			}
			return live, nil
		default:
			return nil, errors.New("unexpected action: " + action)
		}
	}
	if err = transact(root, st, target, public, false); err == nil || !strings.Contains(err.Error(), "recovery incomplete") {
		t.Fatalf("expected incomplete HTTP recovery: %v", err)
	}
	if !sameJSON(live, restored) {
		t.Fatal("rollback lost retained mapping")
	}
	journal, err := os.ReadFile(journalPath(root, s))
	if err != nil {
		t.Fatal(err)
	}
	unhealthy.Store(false)
	// A retry must still reject edits to any other owned field, including a
	// different virtual directory, not merely accept any retained-mapping state.
	for _, field := range []string{"virtualDirectory", "compressionValue", "rules", "preconditions", "otel"} {
		var foreign map[string]json.RawMessage
		json.Unmarshal(restored, &foreign)
		foreign[field] = json.RawMessage(`"external change"`)
		live, _ = json.Marshal(foreign)
		before := restores
		if err = rollback(root, st, io.Discard); err == nil || restores != before {
			t.Fatalf("overwrote external %s: %v", field, err)
		}
	}
	live = restored
	if err = rollback(root, st, io.Discard); err != nil {
		t.Fatal("could not retry partial recovery", err)
	}
	if _, err = os.Stat(journalPath(root, s)); !os.IsNotExist(err) {
		t.Fatal("completed recovery retained journal", err)
	}
	got, err := loadState(root, s.ID)
	if err != nil || !sameJSON(got.Current.Native, restored) || got.Current.Enabled || got.Previous != nil {
		t.Fatal("incorrect recovered state", err)
	}
	// Simulate process loss after IIS commits restore, before Go sees its return.
	// The original journal alone must suffice; no later checkpoint is required.
	if err = os.WriteFile(journalPath(root, s), journal, 0600); err != nil {
		t.Fatal(err)
	}
	if err = rollback(root, got, io.Discard); err != nil {
		t.Fatal("could not recover old journal after commit/return interruption", err)
	}
}
