package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSelectAssets(t *testing.T) {
	pool := []GoldCurrencyItem{
		{NameEn: "gold", Symbol: "XAU"},
		{NameEn: "silver", Symbol: "XAG"},
		{NameEn: "USD", Symbol: "USD"},
		{NameEn: "Euro", Symbol: "EUR"},
	}
	dupPool := []GoldCurrencyItem{
		{NameEn: "gold", Symbol: "FIRST"},
		{NameEn: "gold", Symbol: "SECOND"},
	}

	tests := []struct {
		name string
		pool []GoldCurrencyItem
		req  []string
		want []string // expected NameEn values, in order; nil means expect a nil slice
	}{
		{name: "empty names returns nil", pool: pool, req: nil, want: nil},
		{name: "exact name match", pool: pool, req: []string{"gold"}, want: []string{"gold"}},
		{name: "case-insensitive match", pool: pool, req: []string{"GOLD"}, want: []string{"gold"}},
		{name: "request name is trimmed", pool: pool, req: []string{"  Euro  "}, want: []string{"Euro"}},
		{name: "pool item is normalized for matching but preserved in result", pool: pool, req: []string{"usd"}, want: []string{"USD"}},
		{name: "duplicate requested names produce one result", pool: pool, req: []string{"gold", "Gold", " GOLD "}, want: []string{"gold"}},
		{name: "multiple matches return in pool order", pool: pool, req: []string{"silver", "gold"}, want: []string{"gold", "silver"}},
		{
			// The API's uniqueness assumption is documented but not enforced.
			// A second pool item with the same name cannot match, guarding
			// against two cards being rendered for one holding if the API
			// returns a duplicate.
			name: "duplicate pool names do not produce duplicate matches",
			pool: dupPool, req: []string{"gold"}, want: []string{"gold"},
		},
		{name: "non-matching pool items are skipped", pool: pool, req: []string{"gold", "platinum"}, want: []string{"gold"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectAssets(tt.pool, tt.req)

			if tt.want == nil {
				if got != nil {
					t.Errorf("selectAssets() = %#v, want nil", got)
				}
				return
			}

			if len(got) != len(tt.want) {
				t.Fatalf("len(selectAssets()) = %d, want %d", len(got), len(tt.want))
			}
			for i, wantName := range tt.want {
				if got[i].NameEn != wantName {
					t.Errorf("got[%d].NameEn = %q, want %q", i, got[i].NameEn, wantName)
				}
			}
		})
	}
}

func TestFetchGoldCurrency(t *testing.T) {
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

	t.Run("success decodes both arrays with all fields", func(t *testing.T) {
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

		// Matched by name rather than index: this test asserts which items
		// came back, not the order they came back in.
		find := func(name string) (GoldCurrencyItem, bool) {
			for _, item := range got {
				if item.NameEn == name {
					return item, true
				}
			}
			return GoldCurrencyItem{}, false
		}
		g, ok := find("Gold 18K")
		if !ok {
			t.Fatal("Gold 18K not in result")
		}
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

		c, ok := find("US Dollar")
		if !ok {
			t.Fatal("US Dollar not in result")
		}
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

	// Each requests one name against the two-item fixture, so both fail if
	// fetchGoldCurrency ever drops the selectAssets call and returns the
	// whole pool unfiltered.
	t.Run("only a gold match returns one item", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(body))
		}))
		defer srv.Close()

		got, err := fetchGoldCurrency(context.Background(), srv.Client(), srv.URL, "test-key", []string{"Gold 18K"})
		if err != nil {
			t.Fatalf("fetchGoldCurrency() error = %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("len(got) = %d, want 1", len(got))
		}
		if got[0].NameEn != "Gold 18K" {
			t.Errorf("got[0].NameEn = %q, want %q", got[0].NameEn, "Gold 18K")
		}
	})

	t.Run("only a currency match returns one item", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(body))
		}))
		defer srv.Close()

		got, err := fetchGoldCurrency(context.Background(), srv.Client(), srv.URL, "test-key", []string{"US Dollar"})
		if err != nil {
			t.Fatalf("fetchGoldCurrency() error = %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("len(got) = %d, want 1", len(got))
		}
		if got[0].NameEn != "US Dollar" {
			t.Errorf("got[0].NameEn = %q, want %q", got[0].NameEn, "US Dollar")
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

		_, err := fetchGoldCurrency(context.Background(), srv.Client(), srv.URL, "test-key", []string{"Gold 18K"})
		if err == nil {
			t.Fatal("fetchGoldCurrency() error = nil, want error")
		}

		// A *json.SyntaxError in the chain proves two things at once: the
		// failure came from the decode step (not fetchBody), and the error
		// was wrapped with %w rather than flattened into a string.
		if _, ok := errors.AsType[*json.SyntaxError](err); !ok {
			t.Errorf("error = %v, want it to wrap a *json.SyntaxError", err)
		}
	})

	t.Run("non-200 status returns error", func(t *testing.T) {
		// The specific error text comes from fetchBody and is tested there.
		// Here we only verify that fetchGoldCurrency propagates it.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "server error", http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := fetchGoldCurrency(context.Background(), srv.Client(), srv.URL, "test-key", []string{"Gold 18K"})
		if err == nil {
			t.Fatal("fetchGoldCurrency() error = nil, want error")
		}
	})
}
