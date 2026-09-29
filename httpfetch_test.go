package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestFetchBody(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("method = %q, want GET", r.Method)
			}
			if got := r.URL.Query().Get("key"); got != "test-key" {
				t.Errorf("key = %q, want %q", got, "test-key")
			}
			const wantUA = "Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36"
			if got := r.Header.Get("User-Agent"); got != wantUA {
				t.Errorf("User-Agent = %q, want %q", got, wantUA)
			}
			const wantAccept = "application/json, text/plain, */*"
			if got := r.Header.Get("Accept"); got != wantAccept {
				t.Errorf("Accept = %q, want %q", got, wantAccept)
			}
			fmt.Fprint(w, "test body")
		}))
		defer srv.Close()

		body, err := fetchBody(context.Background(), srv.Client(), srv.URL, "test-key")
		if err != nil {
			t.Fatalf("fetchBody() error = %v", err)
		}
		defer body.Close()

		got, err := io.ReadAll(body)
		if err != nil {
			t.Fatalf("reading body: %v", err)
		}
		if string(got) != "test body" {
			t.Errorf("body = %q, want %q", got, "test body")
		}
	})

	t.Run("api key is url-encoded", func(t *testing.T) {
		// "test-key" contains nothing that needs escaping, so the existing
		// success test does not actually exercise url.Values.Encode. Assert
		// the round trip (encode -> transmit -> decode == original) rather
		// than a specific escaped string, which would be brittle for no gain.
		const rawKey = "a b&c=d"

		gotKey := make(chan string, 1)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotKey <- r.URL.Query().Get("key")
		}))
		defer srv.Close()

		body, err := fetchBody(context.Background(), srv.Client(), srv.URL, rawKey)
		if err != nil {
			t.Fatalf("fetchBody() error = %v", err)
		}
		body.Close()

		if got := <-gotKey; got != rawKey {
			t.Errorf("key = %q, want %q", got, rawKey)
		}
	})

	t.Run("non-200 status returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		body, err := fetchBody(context.Background(), srv.Client(), srv.URL, "test-key")
		if err == nil {
			t.Fatal("fetchBody() error = nil, want error")
		}
		if body != nil {
			t.Error("expected nil body on error")
		}
	})

	t.Run("connection failure does not leak api key", func(t *testing.T) {
		const secretKey = "connection-failure-secret"

		// Bind an ephemeral port and close it immediately, so the address is
		// guaranteed to refuse connections rather than relying on a fixed port
		// like :1 happening to be closed in every environment this runs in.
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to set up listener: %v", err)
		}
		addr := l.Addr().String()
		l.Close()

		// Cheap insurance: if the closed port somehow doesn't refuse cleanly,
		// the test fails fast instead of hanging.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		_, err = fetchBody(ctx, &http.Client{}, "http://"+addr, secretKey)
		if err == nil {
			t.Fatal("fetchBody() error = nil, want error")
		}
		if strings.Contains(err.Error(), secretKey) {
			t.Errorf("error leaked API key: %v", err)
		}
	})

	t.Run("cancelled context does not leak api key", func(t *testing.T) {
		const secretKey = "cancelled-context-secret"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Block until the client cancels, rather than sleeping a fixed
			// duration — ties the test directly to real cancellation
			// propagation instead of an arbitrary timing assumption.
			<-r.Context().Done()
		}))
		defer srv.Close()

		// Give the cancellation enough time to avoid making the test sensitive
		// to scheduler delays on a busy machine.
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err := fetchBody(ctx, srv.Client(), srv.URL, secretKey)
		if err == nil {
			t.Fatal("fetchBody() error = nil, want error")
		}
		if strings.Contains(err.Error(), secretKey) {
			t.Errorf("error leaked API key: %v", err)
		}
	})
}

// TestBaseURLsParse validates the real production inputs that fetchBody's
// defensive url.Parse branch assumes are well-formed. It does not cover that
// branch and manufactures no inputs; a failure here means a base-URL
// constant was corrupted.
func TestBaseURLsParse(t *testing.T) {
	for name, raw := range map[string]string{
		"cryptoBaseURL":       cryptoBaseURL,
		"goldCurrencyBaseURL": goldCurrencyBaseURL,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := url.Parse(raw); err != nil {
				t.Errorf("url.Parse(%s = %q) error = %v, want nil", name, raw, err)
			}
		})
	}
}
