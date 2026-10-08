package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchCrypto(t *testing.T) {
	t.Run("empty names", func(t *testing.T) {
		// Buffered channel, not a bool: if the server ever were called, the
		// handler would write from another goroutine, and an unsynchronized
		// write would race with the receive below.
		called := make(chan struct{}, 1)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called <- struct{}{}
		}))
		defer srv.Close()

		if _, err := fetchCrypto(context.Background(), srv.Client(), srv.URL, "test-key", nil); err != nil {
			t.Fatalf("fetchCrypto() error = %v", err)
		}
		select {
		case <-called:
			t.Error("server was called for empty names")
		default:
		}
	})

	t.Run("matches requested names case-insensitively and ignoring whitespace", func(t *testing.T) {
		// Raw JSON, not json.Marshal of Crypto, so the name_en, price, and
		// price_toman tags are actually verified by the assertions below; a
		// wrong tag round-trips fine when we encode our own struct.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`[
				{"name_en":"bitcoin","price":"123.45","price_toman":"10000000000"},
				{"name_en":"Ethereum","price":"2345.67","price_toman":"200000000"},
				{"name_en":"solana"}
			]`))
		}))
		defer srv.Close()

		got, err := fetchCrypto(context.Background(), srv.Client(), srv.URL, "test-key", []string{" Bitcoin ", "ETHEREUM"})
		if err != nil {
			t.Fatalf("fetchCrypto() error = %v", err)
		}

		if len(got) != 2 {
			t.Fatalf("len(fetchCrypto()) = %d, want 2", len(got))
		}

		if got[0].NameEn != "bitcoin" {
			t.Errorf("got[0].NameEn = %q, want %q", got[0].NameEn, "bitcoin")
		}
		if got[0].Price != "123.45" {
			t.Errorf("got[0].Price = %q, want %q", got[0].Price, "123.45")
		}
		if got[0].PriceToman != "10000000000" {
			t.Errorf("got[0].PriceToman = %q, want %q", got[0].PriceToman, "10000000000")
		}
		if got[1].NameEn != "Ethereum" {
			t.Errorf("got[1].NameEn = %q, want %q", got[1].NameEn, "Ethereum")
		}
	})

	t.Run("missing names are ignored", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`[{"name_en":"bitcoin"}]`))
		}))
		defer srv.Close()

		got, err := fetchCrypto(context.Background(), srv.Client(), srv.URL, "test-key", []string{"bitcoin", "ethereum"})
		if err != nil {
			t.Fatalf("fetchCrypto() error = %v", err)
		}

		if len(got) != 1 {
			t.Fatalf("len(fetchCrypto()) = %d, want 1", len(got))
		}
		if got[0].NameEn != "bitcoin" {
			t.Errorf("got[0].NameEn = %q, want %q", got[0].NameEn, "bitcoin")
		}
	})

	t.Run("duplicate requested names produce one result", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`[{"name_en":"bitcoin"}]`))
		}))
		defer srv.Close()

		got, err := fetchCrypto(context.Background(), srv.Client(), srv.URL, "test-key", []string{"bitcoin", "Bitcoin", " BITCOIN "})
		if err != nil {
			t.Fatalf("fetchCrypto() error = %v", err)
		}

		if len(got) != 1 {
			t.Fatalf("len(fetchCrypto()) = %d, want 1", len(got))
		}
		if got[0].NameEn != "bitcoin" {
			t.Errorf("got[0].NameEn = %q, want %q", got[0].NameEn, "bitcoin")
		}
	})

	t.Run("fetch error is returned", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "server error", http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := fetchCrypto(context.Background(), srv.Client(), srv.URL, "test-key", []string{"bitcoin"})
		if err == nil {
			t.Fatal("fetchCrypto() error = nil, want error")
		}
	})

	t.Run("invalid JSON before array", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`not json`))
		}))
		defer srv.Close()

		_, err := fetchCrypto(context.Background(), srv.Client(), srv.URL, "test-key", []string{"bitcoin"})
		if err == nil {
			t.Fatal("fetchCrypto() error = nil, want error")
		}
	})

	t.Run("response is not an array", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"name_en":"bitcoin"}`))
		}))
		defer srv.Close()

		_, err := fetchCrypto(context.Background(), srv.Client(), srv.URL, "test-key", []string{"bitcoin"})
		if err == nil {
			t.Fatal("fetchCrypto() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "expected JSON array") {
			t.Errorf("error = %q, want JSON array error", err)
		}
	})

	t.Run("malformed item returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`[
				{"name_en":"bitcoin"},
				{"name_en":
			]`))
		}))
		defer srv.Close()

		_, err := fetchCrypto(context.Background(), srv.Client(), srv.URL, "test-key", []string{"bitcoin", "ethereum"})
		if err == nil {
			t.Fatal("fetchCrypto() error = nil, want error")
		}
	})

	t.Run("stops reading after all requested assets are found", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`[
				{"name_en":"bitcoin"},
				{"name_en":"ethereum"},
				{"name_en":
			]`))
		}))
		defer srv.Close()

		got, err := fetchCrypto(context.Background(), srv.Client(), srv.URL, "test-key", []string{"bitcoin", "ethereum"})
		if err != nil {
			t.Fatalf("fetchCrypto() error = %v", err)
		}

		if len(got) != 2 {
			t.Fatalf("len(fetchCrypto()) = %d, want 2", len(got))
		}
		if got[0].NameEn != "bitcoin" {
			t.Errorf("got[0].NameEn = %q, want %q", got[0].NameEn, "bitcoin")
		}
		if got[1].NameEn != "ethereum" {
			t.Errorf("got[1].NameEn = %q, want %q", got[1].NameEn, "ethereum")
		}
	})

	t.Run("truncated response returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "10000")
			w.Write([]byte(`[{"name_en":"bitcoin"}`))
		}))
		defer srv.Close()

		got, err := fetchCrypto(context.Background(), srv.Client(), srv.URL, "test-key", []string{"bitcoin", "ethereum"})
		if err == nil {
			t.Fatalf("fetchCrypto() error = nil, want error; got %+v", got)
		}
	})

	t.Run("truncation after a comma returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "10000")
			w.Write([]byte(`[{"name_en":"solana"},`))
		}))
		defer srv.Close()

		_, err := fetchCrypto(context.Background(), srv.Client(), srv.URL, "test-key", []string{"bitcoin"})
		if err == nil {
			t.Fatal("fetchCrypto() error = nil, want error")
		}
	})

	t.Run("truncation in trailing whitespace returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`[{"name_en":"solana"}  `))
		}))
		defer srv.Close()

		_, err := fetchCrypto(context.Background(), srv.Client(), srv.URL, "test-key", []string{"bitcoin"})
		if err == nil {
			t.Fatal("fetchCrypto() error = nil, want error")
		}
	})

	t.Run("empty array with unmatched names returns empty result", func(t *testing.T) {
		// A complete (not truncated) empty array is a successful response:
		// the post-loop Token consumes ']' cleanly and nothing is an error.
		// Absent items are Missing-list concerns, not fetch errors.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`[]`))
		}))
		defer srv.Close()

		got, err := fetchCrypto(context.Background(), srv.Client(), srv.URL, "test-key", []string{"bitcoin"})
		if err != nil {
			t.Fatalf("fetchCrypto() error = %v, want nil", err)
		}
		if len(got) != 0 {
			t.Errorf("fetchCrypto() = %+v, want empty", got)
		}
	})
}
