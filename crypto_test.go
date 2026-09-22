package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchCrypto(t *testing.T) {
	t.Run("empty names", func(t *testing.T) {
		called := false

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		}))
		defer srv.Close()

		got, err := fetchCrypto(
			context.Background(),
			srv.Client(),
			srv.URL,
			"test-key",
			nil,
		)
		if err != nil {
			t.Fatalf("fetchCrypto() error = %v", err)
		}
		if got != nil {
			t.Errorf("fetchCrypto() = %#v, want nil", got)
		}
		if called {
			t.Error("server was called for empty names")
		}
	})

	t.Run("matches requested names case-insensitively and ignoring whitespace", func(t *testing.T) {
		items := []Crypto{
			{
				NameEn:     "bitcoin",
				Price:      "123.45",
				PriceToman: "10000000000",
			},
			{
				NameEn:     "Ethereum",
				Price:      "2345.67",
				PriceToman: "200000000",
			},
			{
				NameEn: "solana",
			},
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewEncoder(w).Encode(items); err != nil {
				t.Errorf("encoding response: %v", err)
			}
		}))
		defer srv.Close()

		got, err := fetchCrypto(
			context.Background(),
			srv.Client(),
			srv.URL,
			"test-key",
			[]string{" Bitcoin ", "ETHEREUM"},
		)
		if err != nil {
			t.Fatalf("fetchCrypto() error = %v", err)
		}

		if len(got) != 2 {
			t.Fatalf("len(fetchCrypto()) = %d, want 2", len(got))
		}

		if got[0].NameEn != "bitcoin" {
			t.Errorf("got[0].NameEn = %q, want %q", got[0].NameEn, "bitcoin")
		}
		if got[1].NameEn != "Ethereum" {
			t.Errorf("got[1].NameEn = %q, want %q", got[1].NameEn, "Ethereum")
		}
	})

	t.Run("missing names are ignored", func(t *testing.T) {
		items := []Crypto{
			{NameEn: "bitcoin"},
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewEncoder(w).Encode(items); err != nil {
				t.Errorf("encoding response: %v", err)
			}
		}))
		defer srv.Close()

		got, err := fetchCrypto(
			context.Background(),
			srv.Client(),
			srv.URL,
			"test-key",
			[]string{"bitcoin", "ethereum"},
		)
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
		items := []Crypto{
			{NameEn: "bitcoin"},
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewEncoder(w).Encode(items); err != nil {
				t.Errorf("encoding response: %v", err)
			}
		}))
		defer srv.Close()

		got, err := fetchCrypto(
			context.Background(),
			srv.Client(),
			srv.URL,
			"test-key",
			[]string{"bitcoin", "Bitcoin", " BITCOIN "},
		)
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

		_, err := fetchCrypto(
			context.Background(),
			srv.Client(),
			srv.URL,
			"test-key",
			[]string{"bitcoin"},
		)
		if err == nil {
			t.Fatal("fetchCrypto() error = nil, want error")
		}
	})

	t.Run("invalid JSON before array", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`not json`))
		}))
		defer srv.Close()

		_, err := fetchCrypto(
			context.Background(),
			srv.Client(),
			srv.URL,
			"test-key",
			[]string{"bitcoin"},
		)
		if err == nil {
			t.Fatal("fetchCrypto() error = nil, want error")
		}
	})

	t.Run("response is not an array", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"name_en":"bitcoin"}`))
		}))
		defer srv.Close()

		_, err := fetchCrypto(
			context.Background(),
			srv.Client(),
			srv.URL,
			"test-key",
			[]string{"bitcoin"},
		)
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

		_, err := fetchCrypto(
			context.Background(),
			srv.Client(),
			srv.URL,
			"test-key",
			[]string{"bitcoin", "ethereum"},
		)
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

		got, err := fetchCrypto(
			context.Background(),
			srv.Client(),
			srv.URL,
			"test-key",
			[]string{"bitcoin", "ethereum"},
		)
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
		// A response body that promises 10000 bytes and delivers fewer must
		// surface as an error, not as silent partial success. Which internal
		// path reports the truncation (the loop's Decode, or the post-loop
		// Token guard) depends on decoder buffering and is not part of the
		// contract being tested.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "10000")
			w.Write([]byte(`[{"name_en":"bitcoin"}`))
		}))
		defer srv.Close()

		got, err := fetchCrypto(
			context.Background(),
			srv.Client(),
			srv.URL,
			"test-key",
			[]string{"bitcoin", "ethereum"},
		)
		if err == nil {
			t.Fatalf("fetchCrypto() error = nil, want error; got %+v", got)
		}
	})
}
