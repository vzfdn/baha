package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSelectAssets(t *testing.T) {
	// Shared pool. selectAssets does not mutate its pool argument, so it is
	// safe to reuse across subtests.
	pool := []GoldCurrencyItem{
		{NameEn: "gold", Symbol: "XAU"},
		{NameEn: "silver", Symbol: "XAG"},
		{NameEn: "USD", Symbol: "USD"},
		{NameEn: "Euro", Symbol: "EUR"},
	}

	t.Run("empty names returns nil", func(t *testing.T) {
		got := selectAssets(pool, nil)
		if got != nil {
			t.Errorf("selectAssets() = %#v, want nil", got)
		}
	})

	t.Run("exact name match", func(t *testing.T) {
		got := selectAssets(pool, []string{"gold"})
		if len(got) != 1 {
			t.Fatalf("len(selectAssets()) = %d, want 1", len(got))
		}
		if got[0].NameEn != "gold" {
			t.Errorf("got[0].NameEn = %q, want %q", got[0].NameEn, "gold")
		}
	})

	t.Run("case-insensitive match", func(t *testing.T) {
		got := selectAssets(pool, []string{"GOLD"})
		if len(got) != 1 {
			t.Fatalf("len(selectAssets()) = %d, want 1", len(got))
		}
		if got[0].NameEn != "gold" {
			t.Errorf("got[0].NameEn = %q, want %q", got[0].NameEn, "gold")
		}
	})

	t.Run("request name is trimmed", func(t *testing.T) {
		got := selectAssets(pool, []string{"  Euro  "})
		if len(got) != 1 {
			t.Fatalf("len(selectAssets()) = %d, want 1", len(got))
		}
		if got[0].NameEn != "Euro" {
			t.Errorf("got[0].NameEn = %q, want %q", got[0].NameEn, "Euro")
		}
	})

	t.Run("pool item is normalized for matching but preserved in result", func(t *testing.T) {
		// The pool contains "USD" (uppercase). The request is lowercase.
		// Matching succeeds via normalizeName, but the returned item keeps
		// its original NameEn — normalization is a lookup concern, not a
		// mutation of API data.
		got := selectAssets(pool, []string{"usd"})
		if len(got) != 1 {
			t.Fatalf("len(selectAssets()) = %d, want 1", len(got))
		}
		if got[0].NameEn != "USD" {
			t.Errorf("got[0].NameEn = %q, want %q (original, not normalized)", got[0].NameEn, "USD")
		}
	})

	t.Run("duplicate requested names produce one result", func(t *testing.T) {
		got := selectAssets(pool, []string{"gold", "Gold", " GOLD "})
		if len(got) != 1 {
			t.Fatalf("len(selectAssets()) = %d, want 1", len(got))
		}
		if got[0].NameEn != "gold" {
			t.Errorf("got[0].NameEn = %q, want %q", got[0].NameEn, "gold")
		}
	})

	t.Run("duplicate pool names do not produce duplicate matches", func(t *testing.T) {
		// The API's uniqueness assumption is documented but not enforced.
		// selectAssets deletes each matched name from the wanted set, so a
		// second pool item with the same name cannot match. This guards the
		// resolver against rendering two cards for one holding if the API
		// ever returns a duplicate.
		dupPool := []GoldCurrencyItem{
			{NameEn: "gold", Symbol: "FIRST"},
			{NameEn: "gold", Symbol: "SECOND"},
		}
		got := selectAssets(dupPool, []string{"gold"})
		if len(got) != 1 {
			t.Fatalf("len(selectAssets()) = %d, want 1 (duplicates must not double-match)", len(got))
		}
	})

	t.Run("non-matching pool items are skipped", func(t *testing.T) {
		got := selectAssets(pool, []string{"gold", "platinum"})
		if len(got) != 1 {
			t.Fatalf("len(selectAssets()) = %d, want 1", len(got))
		}
		if got[0].NameEn != "gold" {
			t.Errorf("got[0].NameEn = %q, want %q", got[0].NameEn, "gold")
		}
	})
}

func TestFetchGoldCurrency(t *testing.T) {
	t.Run("success decodes both arrays with all fields", func(t *testing.T) {
		// Raw JSON, not json.Marshal of the struct, so this test actually
		// verifies the json tags on GoldCurrencyItem. If we encoded from the
		// struct, a wrong tag would round-trip fine and the test would still
		// pass.
		const body = `{
			"gold": [
				{
					"date": "1403/06/31",
					"time": "14:30:00",
					"name_en": "Gold 18K",
					"symbol": "XAU18K",
					"price": 12345678,
					"change_percent": 1.25,
					"market_cap": 987654321,
					"unit": "Toman/gram"
				}
			],
			"currency": [
				{
					"date": "1403/06/31",
					"time": "14:30:00",
					"name_en": "US Dollar",
					"symbol": "USD",
					"price": 900000,
					"change_percent": -0.5,
					"market_cap": 0,
					"unit": "Toman"
				}
			]
		}`

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(body))
		}))
		defer srv.Close()

		got, err := fetchGoldCurrency(context.Background(), srv.Client(), srv.URL, "test-key", []string{"Gold 18K", "US Dollar"})
		if err != nil {
			t.Fatalf("fetchGoldCurrency() error = %v", err)
		}

		if len(got) != 2 {
			t.Fatalf("len(got) = %d, want 2", len(got))
		}
		g := got[0]
		if g.Date != "1403/06/31" {
			t.Errorf("Gold.Date = %q, want %q", g.Date, "1403/06/31")
		}
		if g.Time != "14:30:00" {
			t.Errorf("Gold.Time = %q, want %q", g.Time, "14:30:00")
		}
		if g.NameEn != "Gold 18K" {
			t.Errorf("Gold.NameEn = %q, want %q", g.NameEn, "Gold 18K")
		}
		if g.Symbol != "XAU18K" {
			t.Errorf("Gold.Symbol = %q, want %q", g.Symbol, "XAU18K")
		}
		if g.Price != 12345678 {
			t.Errorf("Gold.Price = %d, want %d", g.Price, 12345678)
		}
		if g.ChangePercent != 1.25 {
			t.Errorf("Gold.ChangePercent = %v, want %v", g.ChangePercent, 1.25)
		}
		if g.MarketCap != 987654321 {
			t.Errorf("Gold.MarketCap = %d, want %d", g.MarketCap, 987654321)
		}
		if g.Unit != "Toman/gram" {
			t.Errorf("Gold.Unit = %q, want %q", g.Unit, "Toman/gram")
		}

		c := got[1]
		if c.Date != "1403/06/31" {
			t.Errorf("Currency.Date = %q, want %q", c.Date, "1403/06/31")
		}
		if c.NameEn != "US Dollar" {
			t.Errorf("Currency.NameEn = %q, want %q", c.NameEn, "US Dollar")
		}
		if c.Symbol != "USD" {
			t.Errorf("Currency.Symbol = %q, want %q", c.Symbol, "USD")
		}
		if c.Price != 900000 {
			t.Errorf("Currency.Price = %d, want %d", c.Price, 900000)
		}
		if c.ChangePercent != -0.5 {
			t.Errorf("Currency.ChangePercent = %v, want %v", c.ChangePercent, -0.5)
		}
		if c.Unit != "Toman" {
			t.Errorf("Currency.Unit = %q, want %q", c.Unit, "Toman")
		}
	})

	t.Run("empty arrays return no matches", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"gold":[],"currency":[]}`))
		}))
		defer srv.Close()

		got, err := fetchGoldCurrency(context.Background(), srv.Client(), srv.URL, "test-key", []string{"Gold 18K"})
		if err != nil {
			t.Fatalf("fetchGoldCurrency() error = %v", err)
		}
		if len(got) != 0 {
			t.Errorf("len(got) = %d, want 0", len(got))
		}
	})

	t.Run("malformed JSON returns wrapped error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{not json`))
		}))
		defer srv.Close()

		got, err := fetchGoldCurrency(context.Background(), srv.Client(), srv.URL, "test-key", []string{"Gold 18K"})
		if err == nil {
			t.Fatal("fetchGoldCurrency() error = nil, want error")
		}
		if got != nil {
			t.Errorf("fetchGoldCurrency() = %#v, want nil on error", got)
		}
		// The "decoding response" prefix is part of the function's contract,
		// not just an incidental message.
		if !strings.Contains(err.Error(), "decoding response") {
			t.Errorf("error = %q, want it to contain %q", err, "decoding response")
		}
	})

	t.Run("non-200 status returns error", func(t *testing.T) {
		// The specific error text comes from fetchBody and is tested there.
		// Here we only verify that fetchGoldCurrency propagates it and does
		// not return a partial response.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "server error", http.StatusInternalServerError)
		}))
		defer srv.Close()

		got, err := fetchGoldCurrency(context.Background(), srv.Client(), srv.URL, "test-key", []string{"Gold 18K"})
		if err == nil {
			t.Fatal("fetchGoldCurrency() error = nil, want error")
		}
		if got != nil {
			t.Errorf("fetchGoldCurrency() = %#v, want nil on error", got)
		}
	})
}
