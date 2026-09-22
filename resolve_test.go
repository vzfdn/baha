package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// roundTripperFunc adapts a plain function to http.RoundTripper.
//
// resolveAssets calls the two fetch helpers with the package-level base-URL
// constants, so an httptest.Server cannot be pointed at it without changing
// production code. Overriding the client's transport is the smallest way to
// intercept those requests from a test, and it keeps the seam confined to the
// test file rather than adding a parameter to resolveAssets that only exists
// for testing.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// fakeClient returns an *http.Client whose transport responds to the two BRS
// endpoints with the given JSON bodies. An empty body simulates a 500 response
// so callers can exercise the fetch-error paths.
func fakeClient(t *testing.T, goldBody, cryptoBody string) *http.Client {
	t.Helper()
	return &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			var body string
			switch r.URL.Path {
			case "/Market/Gold_Currency.php":
				body = goldBody
			case "/Market/Cryptocurrency.php":
				body = cryptoBody
			default:
				t.Errorf("unexpected request path: %s", r.URL.Path)
				return nil, fmt.Errorf("unexpected path %q", r.URL.Path)
			}
			if body == "" {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Status:     "500 Internal Server Error",
					Body:       io.NopCloser(strings.NewReader("")),
					Header:     make(http.Header),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}),
	}
}

// mustJSON marshals v into a JSON string, failing the test on error.
func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshaling fixture: %v", err)
	}
	return string(b)
}

func TestResolveAssets(t *testing.T) {
	t.Run("found in gold/currency only", func(t *testing.T) {
		client := fakeClient(
			t,
			mustJSON(t, goldCurrencyResponse{
				Gold: []GoldCurrencyItem{{NameEn: "gold", Symbol: "XAU", Price: 1000}},
			}),
			mustJSON(t, []Crypto{}),
		)

		got, err := resolveAssets(context.Background(), client, "test-key", []Holding{{name: "gold"}})
		if err != nil {
			t.Fatalf("resolveAssets() error = %v", err)
		}
		if len(got.GoldCurrency) != 1 {
			t.Fatalf("len(got.GoldCurrency) = %d, want 1", len(got.GoldCurrency))
		}
		if got.GoldCurrency[0].Item.Symbol != "XAU" {
			t.Errorf("matched symbol = %q, want %q", got.GoldCurrency[0].Item.Symbol, "XAU")
		}
		if got.GoldCurrency[0].Holding.name != "gold" {
			t.Errorf("holding name = %q, want %q", got.GoldCurrency[0].Holding.name, "gold")
		}
		if len(got.Crypto) != 0 {
			t.Errorf("len(got.Crypto) = %d, want 0", len(got.Crypto))
		}
		if len(got.Missing) != 0 {
			t.Errorf("len(got.Missing) = %d, want 0", len(got.Missing))
		}
	})

	t.Run("found in crypto only", func(t *testing.T) {
		client := fakeClient(
			t,
			mustJSON(t, goldCurrencyResponse{}),
			mustJSON(t, []Crypto{
				{NameEn: "bitcoin", PriceToman: "3000000000"},
			}),
		)

		holdings := []Holding{{name: "bitcoin", amount: 1, hasAmount: true}}
		got, err := resolveAssets(context.Background(), client, "test-key", holdings)
		if err != nil {
			t.Fatalf("resolveAssets() error = %v", err)
		}
		if len(got.Crypto) != 1 {
			t.Fatalf("len(got.Crypto) = %d, want 1", len(got.Crypto))
		}
		if got.Crypto[0].Item.NameEn != "bitcoin" {
			t.Errorf("matched name = %q, want %q", got.Crypto[0].Item.NameEn, "bitcoin")
		}
		// Integration check: resolveAssets must have constructed the match
		// via newCryptoMatch, otherwise value() returns zero here even though
		// TestNewCryptoMatch still passes on its own. Asserting through the
		// exported method keeps this test out of CryptoMatch's unexported
		// priceToman field, which is TestNewCryptoMatch's concern.
		v, ok := got.Crypto[0].value()
		if !ok {
			t.Fatal("value() ok = false, want true")
		}
		if v != 3000000000 {
			t.Errorf("value() = %v, want 3000000000", v)
		}
		if len(got.GoldCurrency) != 0 {
			t.Errorf("len(got.GoldCurrency) = %d, want 0", len(got.GoldCurrency))
		}
		if len(got.Missing) != 0 {
			t.Errorf("len(got.Missing) = %d, want 0", len(got.Missing))
		}
	})

	t.Run("one from each source", func(t *testing.T) {
		client := fakeClient(
			t,
			mustJSON(t, goldCurrencyResponse{
				Currency: []GoldCurrencyItem{{NameEn: "usd", Price: 900000}},
			}),
			mustJSON(t, []Crypto{
				{NameEn: "bitcoin", PriceToman: "3000000000"},
			}),
		)

		holdings := []Holding{{name: "usd"}, {name: "bitcoin"}}
		got, err := resolveAssets(context.Background(), client, "test-key", holdings)
		if err != nil {
			t.Fatalf("resolveAssets() error = %v", err)
		}
		if len(got.GoldCurrency) != 1 {
			t.Fatalf("len(got.GoldCurrency) = %d, want 1", len(got.GoldCurrency))
		}
		if got.GoldCurrency[0].Item.NameEn != "usd" {
			t.Errorf("gold/currency match = %q, want %q", got.GoldCurrency[0].Item.NameEn, "usd")
		}
		if len(got.Crypto) != 1 {
			t.Fatalf("len(got.Crypto) = %d, want 1", len(got.Crypto))
		}
		if got.Crypto[0].Item.NameEn != "bitcoin" {
			t.Errorf("crypto match = %q, want %q", got.Crypto[0].Item.NameEn, "bitcoin")
		}
		if len(got.Missing) != 0 {
			t.Errorf("len(got.Missing) = %d, want 0", len(got.Missing))
		}
	})

	t.Run("missing holdings are collected", func(t *testing.T) {
		client := fakeClient(
			t,
			mustJSON(t, goldCurrencyResponse{}),
			mustJSON(t, []Crypto{
				{NameEn: "bitcoin", PriceToman: "3000000000"},
			}),
		)

		holdings := []Holding{{name: "bitcoin"}, {name: "ethereum"}}
		got, err := resolveAssets(context.Background(), client, "test-key", holdings)
		if err != nil {
			t.Fatalf("resolveAssets() error = %v", err)
		}
		if len(got.Crypto) != 1 {
			t.Fatalf("len(got.Crypto) = %d, want 1", len(got.Crypto))
		}
		if len(got.Missing) != 1 {
			t.Fatalf("len(got.Missing) = %d, want 1", len(got.Missing))
		}
		if got.Missing[0] != "ethereum" {
			t.Errorf("got.Missing[0] = %q, want %q", got.Missing[0], "ethereum")
		}
	})

	t.Run("all missing", func(t *testing.T) {
		client := fakeClient(t, mustJSON(t, goldCurrencyResponse{}), mustJSON(t, []Crypto{}))

		holdings := []Holding{{name: "platinum"}, {name: "palladium"}}
		got, err := resolveAssets(context.Background(), client, "test-key", holdings)
		if err != nil {
			t.Fatalf("resolveAssets() error = %v", err)
		}
		if len(got.GoldCurrency) != 0 || len(got.Crypto) != 0 {
			t.Errorf("expected no matches, got %#v", got)
		}
		if len(got.Missing) != 2 {
			t.Fatalf("len(got.Missing) = %d, want 2", len(got.Missing))
		}
		// Missing preserves holdings order and the user-facing name.
		if got.Missing[0] != "platinum" || got.Missing[1] != "palladium" {
			t.Errorf("got.Missing = %#v, want [platinum palladium]", got.Missing)
		}
	})

	t.Run("gold fetch error is wrapped", func(t *testing.T) {
		// Empty gold body -> fakeClient returns 500 -> fetchBody returns
		// error -> fetchGoldCurrency propagates -> resolveAssets wraps.
		client := fakeClient(t, "", mustJSON(t, []Crypto{}))

		_, err := resolveAssets(context.Background(), client, "test-key", []Holding{{name: "gold"}})
		if err == nil {
			t.Fatal("resolveAssets() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "fetching gold/currency") {
			t.Errorf("error = %q, want it to contain %q", err, "fetching gold/currency")
		}
	})

	t.Run("crypto fetch error is wrapped", func(t *testing.T) {
		client := fakeClient(t, mustJSON(t, goldCurrencyResponse{}), "")

		_, err := resolveAssets(context.Background(), client, "test-key", []Holding{{name: "bitcoin"}})
		if err == nil {
			t.Fatal("resolveAssets() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "fetching crypto") {
			t.Errorf("error = %q, want it to contain %q", err, "fetching crypto")
		}
	})

	t.Run("malformed crypto price is wrapped", func(t *testing.T) {
		client := fakeClient(
			t,
			mustJSON(t, goldCurrencyResponse{}),
			mustJSON(t, []Crypto{
				{NameEn: "bitcoin", PriceToman: "not-a-number"},
			}),
		)

		_, err := resolveAssets(context.Background(), client, "test-key", []Holding{{name: "bitcoin"}})
		if err == nil {
			t.Fatal("resolveAssets() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "processing crypto data") {
			t.Errorf("error = %q, want it to contain %q", err, "processing crypto data")
		}
	})

	t.Run("case-insensitive match preserves API name", func(t *testing.T) {
		client := fakeClient(
			t,
			mustJSON(t, goldCurrencyResponse{
				Gold: []GoldCurrencyItem{{NameEn: "gold", Price: 1000}},
			}),
			mustJSON(t, []Crypto{}),
		)

		got, err := resolveAssets(context.Background(), client, "test-key", []Holding{{name: "  GOLD  "}})
		if err != nil {
			t.Fatalf("resolveAssets() error = %v", err)
		}
		if len(got.GoldCurrency) != 1 {
			t.Fatalf("len(got.GoldCurrency) = %d, want 1", len(got.GoldCurrency))
		}
		// Item name comes from the API, Holding name from the user. Neither
		// is replaced by the other, and neither is replaced by the normalized
		// form used internally for matching.
		if got.GoldCurrency[0].Item.NameEn != "gold" {
			t.Errorf("Item.NameEn = %q, want %q", got.GoldCurrency[0].Item.NameEn, "gold")
		}
		if got.GoldCurrency[0].Holding.name != "  GOLD  " {
			t.Errorf("Holding.name = %q, want the original user input %q",
				got.GoldCurrency[0].Holding.name, "  GOLD  ")
		}
	})
}

func TestNewCryptoMatch(t *testing.T) {
	t.Run("valid price parses", func(t *testing.T) {
		h := Holding{name: "bitcoin", amount: 1, hasAmount: true}
		item := Crypto{NameEn: "bitcoin", PriceToman: "3000000000"}

		got, err := newCryptoMatch(h, item)
		if err != nil {
			t.Fatalf("newCryptoMatch() error = %v", err)
		}
		if got.priceToman != 3000000000 {
			t.Errorf("priceToman = %v, want 3000000000", got.priceToman)
		}
		if got.Holding != h {
			t.Errorf("Holding = %+v, want %+v", got.Holding, h)
		}
		if got.Item.NameEn != "bitcoin" {
			t.Errorf("Item.NameEn = %q, want %q", got.Item.NameEn, "bitcoin")
		}
	})

	t.Run("empty price returns error", func(t *testing.T) {
		_, err := newCryptoMatch(Holding{}, Crypto{NameEn: "bitcoin", PriceToman: ""})
		if err == nil {
			t.Fatal("newCryptoMatch() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "parsing price_toman") {
			t.Errorf("error = %q, want it to contain %q", err, "parsing price_toman")
		}
		if !strings.Contains(err.Error(), "bitcoin") {
			t.Errorf("error = %q, want it to mention the item name", err)
		}
	})

	t.Run("non-numeric price returns error", func(t *testing.T) {
		_, err := newCryptoMatch(Holding{}, Crypto{NameEn: "ethereum", PriceToman: "abc"})
		if err == nil {
			t.Fatal("newCryptoMatch() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "ethereum") {
			t.Errorf("error = %q, want it to mention the item name", err)
		}
	})
}

func TestGoldCurrencyMatch(t *testing.T) {
	t.Run("value is not-ok when no amount", func(t *testing.T) {
		m := GoldCurrencyMatch{
			Holding: Holding{name: "gold"},
			Item:    GoldCurrencyItem{Price: 1000},
		}
		v, ok := m.value()
		if ok {
			t.Error("value() ok = true, want false")
		}
		if v != 0 {
			t.Errorf("value() = %v, want 0", v)
		}
	})

	t.Run("value is amount times price", func(t *testing.T) {
		m := GoldCurrencyMatch{
			Holding: Holding{name: "gold", amount: 2.5, hasAmount: true},
			Item:    GoldCurrencyItem{Price: 1000},
		}
		v, ok := m.value()
		if !ok {
			t.Fatal("value() ok = false, want true")
		}
		if v != 2500 {
			t.Errorf("value() = %v, want 2500", v)
		}
	})

	t.Run("formatAmount rounds to whole number", func(t *testing.T) {
		// Gold/currency amounts are typically whole units; the formatter
		// reflects that with %.0f. This test documents the choice.
		m := GoldCurrencyMatch{Holding: Holding{amount: 2.7, hasAmount: true}}
		if got := m.formatAmount(); got != "3" {
			t.Errorf("formatAmount() = %q, want %q", got, "3")
		}
	})
}

func TestCryptoMatch(t *testing.T) {
	t.Run("value is not-ok when no amount", func(t *testing.T) {
		m := CryptoMatch{
			Holding:    Holding{name: "bitcoin"},
			priceToman: 3000000000,
		}
		v, ok := m.value()
		if ok {
			t.Error("value() ok = true, want false")
		}
		if v != 0 {
			t.Errorf("value() = %v, want 0", v)
		}
	})

	t.Run("value is amount times stored toman price", func(t *testing.T) {
		m := CryptoMatch{
			Holding:    Holding{name: "bitcoin", amount: 0.5, hasAmount: true},
			priceToman: 3000000000,
		}
		v, ok := m.value()
		if !ok {
			t.Fatal("value() ok = false, want true")
		}
		if v != 1500000000 {
			t.Errorf("value() = %v, want 1500000000", v)
		}
	})

	t.Run("formatAmount preserves fractional amounts", func(t *testing.T) {
		// Crypto amounts are frequently fractional (e.g. 0.002 BTC); the
		// formatter uses %g so small values survive display intact.
		m := CryptoMatch{Holding: Holding{amount: 0.002, hasAmount: true}}
		if got := m.formatAmount(); got != "0.002" {
			t.Errorf("formatAmount() = %q, want %q", got, "0.002")
		}
	})
}
