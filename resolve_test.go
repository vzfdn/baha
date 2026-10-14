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
		// Pairs the match with the Holding the user actually asked for.
		if got.Crypto[0].Holding.name != "bitcoin" {
			t.Errorf("holding name = %q, want %q", got.Crypto[0].Holding.name, "bitcoin")
		}
		// Integration check: resolveAssets must have constructed the match
		// via newCryptoMatch, otherwise value() returns zero here even though
		// TestNewCryptoMatch still passes on its own. Asserting through
		// value() keeps this test out of CryptoMatch's unexported
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

	t.Run("gold fetch error preserves crypto results as partial", func(t *testing.T) {
		// Empty gold body -> fakeClient returns 500 -> fetchBody returns
		// error -> fetchGoldCurrency propagates -> resolveAssets wraps, but
		// still queries crypto and keeps whatever it found there.
		client := fakeClient(t, "", mustJSON(t, []Crypto{
			{NameEn: "bitcoin", PriceToman: "3000000000"},
		}))

		holdings := []Holding{{name: "bitcoin", amount: 0.5, hasAmount: true}, {name: "gold"}}
		got, err := resolveAssets(context.Background(), client, "test-key", holdings)
		if err == nil {
			t.Fatal("resolveAssets() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "fetching gold/currency") {
			t.Errorf("error = %q, want it to contain %q", err, "fetching gold/currency")
		}
		if len(got.Crypto) != 1 || got.Crypto[0].Item.NameEn != "bitcoin" {
			t.Fatalf("got.Crypto = %+v, want the bitcoin match retained", got.Crypto)
		}
		v, ok := got.Crypto[0].value()
		if !ok || v != 1500000000 {
			t.Errorf("value() = (%v, %v), want (1500000000, true)", v, ok)
		}
		// "gold" was never checked against the (failed) gold/currency source,
		// so it cannot be classified as missing.
		if len(got.Unverified) != 1 || got.Unverified[0] != "gold" {
			t.Errorf("got.Unverified = %+v, want [gold]", got.Unverified)
		}
		if len(got.Missing) != 0 {
			t.Errorf("got.Missing = %+v, want none", got.Missing)
		}
	})

	t.Run("crypto fetch error preserves gold results as partial", func(t *testing.T) {
		client := fakeClient(t, mustJSON(t, goldCurrencyResponse{
			Gold: []GoldCurrencyItem{{NameEn: "gold", Symbol: "XAU", Price: 1000}},
		}), "")

		holdings := []Holding{{name: "gold"}, {name: "bitcoin"}}
		got, err := resolveAssets(context.Background(), client, "test-key", holdings)
		if err == nil {
			t.Fatal("resolveAssets() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "fetching crypto") {
			t.Errorf("error = %q, want it to contain %q", err, "fetching crypto")
		}
		if len(got.GoldCurrency) != 1 || got.GoldCurrency[0].Item.NameEn != "gold" {
			t.Fatalf("got.GoldCurrency = %+v, want the gold match retained", got.GoldCurrency)
		}
		if len(got.Unverified) != 1 || got.Unverified[0] != "bitcoin" {
			t.Errorf("got.Unverified = %+v, want [bitcoin]", got.Unverified)
		}
		if len(got.Missing) != 0 {
			t.Errorf("got.Missing = %+v, want none", got.Missing)
		}
	})

	t.Run("both fetch errors are joined", func(t *testing.T) {
		client := fakeClient(t, "", "")

		holdings := []Holding{{name: "gold"}, {name: "bitcoin"}}
		got, err := resolveAssets(context.Background(), client, "test-key", holdings)
		if err == nil {
			t.Fatal("resolveAssets() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "fetching gold/currency") || !strings.Contains(err.Error(), "fetching crypto") {
			t.Errorf("error = %q, want both source failures reported", err)
		}
		if len(got.GoldCurrency) != 0 || len(got.Crypto) != 0 {
			t.Errorf("expected empty results on total failure, got %+v", got)
		}
		// No source to verify against, so nothing is Missing; Unverified
		// keeps holdings order.
		if len(got.Missing) != 0 {
			t.Errorf("got.Missing = %+v, want none", got.Missing)
		}
		if len(got.Unverified) != 2 || got.Unverified[0] != "gold" || got.Unverified[1] != "bitcoin" {
			t.Errorf("got.Unverified = %+v, want [gold bitcoin]", got.Unverified)
		}
	})

	t.Run("errors do not leak the API key", func(t *testing.T) {
		client := fakeClient(t, "", "")

		_, err := resolveAssets(context.Background(), client, "secret-test-key", []Holding{{name: "gold"}})
		if err == nil {
			t.Fatal("resolveAssets() error = nil, want error")
		}
		if strings.Contains(err.Error(), "secret-test-key") {
			t.Errorf("error leaks the API key: %q", err)
		}
	})

	t.Run("both sources succeed: unmatched asset is Missing, never Unverified", func(t *testing.T) {
		client := fakeClient(
			t,
			mustJSON(t, goldCurrencyResponse{
				Gold: []GoldCurrencyItem{{NameEn: "gold", Price: 1000}},
			}),
			mustJSON(t, []Crypto{}),
		)

		// Both sources responded successfully, so "ethereum" was fully
		// checked and can be declared Missing; it must not land in Unverified.
		got, err := resolveAssets(context.Background(), client, "test-key", []Holding{{name: "gold"}, {name: "ethereum"}})
		if err != nil {
			t.Fatalf("resolveAssets() error = %v", err)
		}
		if len(got.Missing) != 1 || got.Missing[0] != "ethereum" {
			t.Errorf("got.Missing = %+v, want [ethereum]", got.Missing)
		}
		if len(got.Unverified) != 0 {
			t.Errorf("got.Unverified = %+v, want none", got.Unverified)
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

	t.Run("unusable crypto price is reported without aborting", func(t *testing.T) {
		client := fakeClient(
			t,
			mustJSON(t, goldCurrencyResponse{}),
			mustJSON(t, []Crypto{
				{NameEn: "bitcoin", PriceToman: "not-a-number"},
			}),
		)

		holdings := []Holding{{name: "bitcoin", amount: 1, hasAmount: true}}
		got, err := resolveAssets(context.Background(), client, "test-key", holdings)
		if err == nil {
			t.Fatal("resolveAssets() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "price unavailable") {
			t.Errorf("error = %q, want it to contain %q", err, "price unavailable")
		}
		// The resolved asset is kept as a match even though its price is
		// unusable; nothing else exists to keep it company, and the error is
		// warning-level in main because matches still render.
		if len(got.Crypto) != 1 || got.Crypto[0].Item.NameEn != "bitcoin" || !got.Crypto[0].priceUnavailable {
			t.Fatalf("got.Crypto = %+v, want the bitcoin match retained with priceUnavailable", got.Crypto)
		}
		// value() must not return a successful result from an unusable price.
		if _, ok := got.Crypto[0].value(); ok {
			t.Error("value() ok = true, want false for an unusable price")
		}
		// Found means found: no Missing, no Unverified, despite the item error.
		if len(got.Missing) != 0 || len(got.Unverified) != 0 {
			t.Errorf("got.Missing = %+v, got.Unverified = %+v, want neither", got.Missing, got.Unverified)
		}
	})

	t.Run("invalid price between valid items preserves matches", func(t *testing.T) {
		client := fakeClient(
			t,
			mustJSON(t, goldCurrencyResponse{}),
			mustJSON(t, []Crypto{
				{NameEn: "bitcoin", PriceToman: "3000000000"},
				{NameEn: "ethereum", PriceToman: "NaN"},
				{NameEn: "solana", PriceToman: "15000000"},
			}),
		)

		holdings := []Holding{{name: "bitcoin", amount: 1, hasAmount: true}, {name: "ethereum", amount: 2, hasAmount: true}, {name: "solana", amount: 1, hasAmount: true}}
		got, err := resolveAssets(context.Background(), client, "test-key", holdings)
		if err == nil {
			t.Fatal("resolveAssets() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "price unavailable") {
			t.Errorf("error = %q, want it to contain %q", err, "price unavailable")
		}
		// Empty or malformed -> parse failure; NaN -> validation failure; the
		// error carries the reason either way.
		if !strings.Contains(err.Error(), "price_toman for ethereum") {
			t.Errorf("error = %q, want it to name the offending item", err)
		}
		// Valid matches before and after the invalid item are retained; the
		// invalid item is kept as a match too, with value() failing.
		if len(got.Crypto) != 3 {
			t.Fatalf("len(got.Crypto) = %d, want 3", len(got.Crypto))
		}
		if got.Crypto[0].priceUnavailable || got.Crypto[2].priceUnavailable {
			t.Errorf("items 0/2 marked unavailable: %+v", got.Crypto)
		}
		if !got.Crypto[1].priceUnavailable {
			t.Errorf("item 1 (ethereum) should be unavailable: %+v", got.Crypto[1])
		}
		v, _ := got.Crypto[0].value()
		if v != 3000000000 {
			t.Errorf("bitcoin value() = %v, want 3000000000", v)
		}
		v, _ = got.Crypto[2].value()
		if v != 15000000 {
			t.Errorf("solana value() = %v, want 15000000", v)
		}
		if len(got.Missing) != 0 || len(got.Unverified) != 0 {
			t.Errorf("got.Missing = %+v, got.Unverified = %+v, want neither", got.Missing, got.Unverified)
		}
	})

	t.Run("item errors and source errors coexist", func(t *testing.T) {
		// Gold source fails (500) and bitcoin has an invalid price: both
		// item-level and source-level problems must appear in the error, and
		// the bitcoin match must survive.
		client := fakeClient(
			t,
			"",
			mustJSON(t, []Crypto{
				{NameEn: "bitcoin", PriceToman: "not-a-number"},
				{NameEn: "solana", PriceToman: "15000000"},
			}),
		)

		holdings := []Holding{{name: "bitcoin"}, {name: "solana"}}
		got, err := resolveAssets(context.Background(), client, "test-key", holdings)
		if err == nil {
			t.Fatal("resolveAssets() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "fetching gold/currency") {
			t.Errorf("error = %q, want the source failure reported", err)
		}
		if !strings.Contains(err.Error(), "price unavailable") {
			t.Errorf("error = %q, want the item failure reported", err)
		}
		if len(got.Crypto) != 2 {
			t.Fatalf("len(got.Crypto) = %d, want 2", len(got.Crypto))
		}
		if !got.Crypto[0].priceUnavailable || got.Crypto[1].priceUnavailable {
			t.Errorf("availability flags wrong: %+v", got.Crypto)
		}
		if len(got.Unverified) != 0 || len(got.Missing) != 0 {
			t.Errorf("got.Missing = %+v, got.Unverified = %+v, want neither", got.Missing, got.Unverified)
		}
	})

	t.Run("zero price is unavailable but the asset is retained", func(t *testing.T) {
		client := fakeClient(
			t,
			mustJSON(t, goldCurrencyResponse{}),
			mustJSON(t, []Crypto{
				{NameEn: "bitcoin", PriceToman: "0"},
				{NameEn: "solana", PriceToman: "15000000"},
			}),
		)

		holdings := []Holding{{name: "bitcoin", amount: 10, hasAmount: true}, {name: "solana", amount: 1, hasAmount: true}}
		got, err := resolveAssets(context.Background(), client, "test-key", holdings)
		if err == nil {
			t.Fatal("resolveAssets() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "price unavailable") || !strings.Contains(err.Error(), "price_toman for bitcoin") {
			t.Errorf("error = %q, want the item-level price context for bitcoin", err)
		}
		// The zero-priced asset is still resolved, just not usable.
		if len(got.Crypto) != 2 || !got.Crypto[0].priceUnavailable {
			t.Fatalf("got.Crypto = %+v, want bitcoin retained with priceUnavailable", got.Crypto)
		}
		if v, ok := got.Crypto[0].value(); ok || v != 0 {
			t.Errorf("value() = (%v, %v), want (0, false) despite amount 10", v, ok)
		}
		// A valid zero price is no longer possible: an unavailable price must
		// never look like a real calculated value of zero.
		v, ok := got.Crypto[1].value()
		if !ok || v != 15000000 {
			t.Errorf("solana value() = (%v, %v), want (15000000, true)", v, ok)
		}
		// Found means found, regardless of price usability.
		if len(got.Missing) != 0 || len(got.Unverified) != 0 {
			t.Errorf("got.Missing = %+v, got.Unverified = %+v, want neither", got.Missing, got.Unverified)
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
	t.Run("zero price is rejected as unavailable", func(t *testing.T) {
		got, err := newCryptoMatch(Holding{name: "bitcoin", amount: 10, hasAmount: true}, Crypto{NameEn: "bitcoin", PriceToman: "0"})
		if err == nil {
			t.Fatal("newCryptoMatch() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "price_toman") || !strings.Contains(err.Error(), "bitcoin") {
			t.Errorf("error = %q, want it to mention price_toman and the item name", err)
		}
		// Rejected prices still yield a populated match per the documented
		// contract, and value() cannot compute from it.
		if !got.priceUnavailable {
			t.Error("priceUnavailable = false, want true")
		}
		if got.Item.NameEn != "bitcoin" {
			t.Errorf("Item.NameEn = %q, want %q", got.Item.NameEn, "bitcoin")
		}
		if v, ok := got.value(); ok || v != 0 {
			t.Errorf("value() = (%v, %v), want (0, false) despite amount 10", v, ok)
		}
	})

	t.Run("invalid numeric prices are rejected", func(t *testing.T) {
		tests := []struct {
			name       string
			priceToman string
		}{
			{"NaN", "NaN"},
			{"lowercase nan", "nan"},
			{"positive infinity", "Inf"},
			{"negative infinity", "-Inf"},
			{"negative finite", "-5"},
			{"zero", "0"},
			{"negative zero", "-0"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := newCryptoMatch(Holding{}, Crypto{NameEn: "bitcoin", PriceToman: tt.priceToman})
				if err == nil {
					t.Fatalf("newCryptoMatch(%q) error = nil, want error", tt.priceToman)
				}
				// Name and failure context, not exact wording.
				if !strings.Contains(err.Error(), "bitcoin") {
					t.Errorf("error = %q, want it to mention the item name", err)
				}
				if !strings.Contains(err.Error(), "price_toman") {
					t.Errorf("error = %q, want it to mention price_toman", err)
				}
				// Documented contract: the match is still returned so the
				// caller can keep the resolved asset.
			})
		}
	})

	t.Run("unavailable-price error still returns a usable match", func(t *testing.T) {
		h := Holding{name: "bitcoin", amount: 1, hasAmount: true}
		item := Crypto{NameEn: "bitcoin", PriceToman: "-1"}

		got, err := newCryptoMatch(h, item)
		if err == nil {
			t.Fatal("newCryptoMatch() error = nil, want error")
		}
		if !got.priceUnavailable {
			t.Error("priceUnavailable = false, want true")
		}
		if got.priceToman != 0 {
			t.Errorf("priceToman = %v, want the zero value, not a stale or garbage price", got.priceToman)
		}
		if got.Holding != h {
			t.Errorf("Holding = %+v, want %+v", got.Holding, h)
		}
		if got.Item.NameEn != "bitcoin" {
			t.Errorf("Item.NameEn = %q, want %q", got.Item.NameEn, "bitcoin")
		}
		if _, ok := got.value(); ok {
			t.Error("value() ok = true, want false on the returned match")
		}
	})

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

	t.Run("missing amount makes value not-ok, available price or not", func(t *testing.T) {
		valid, err := newCryptoMatch(Holding{name: "bitcoin"}, Crypto{NameEn: "bitcoin", PriceToman: "3000000000"})
		if err != nil {
			t.Fatalf("newCryptoMatch() error = %v", err)
		}
		if _, ok := valid.value(); ok {
			t.Error("value() ok = true, want false without an amount")
		}

		unavail, err := newCryptoMatch(Holding{name: "bitcoin", amount: 1, hasAmount: true}, Crypto{NameEn: "bitcoin", PriceToman: "-1"})
		if err == nil {
			t.Fatal("newCryptoMatch() error = nil, want error")
		}
		if _, ok := unavail.value(); ok {
			t.Error("value() ok = true, want false for an unusable price")
		}
	})

	t.Run("zero price value is not-ok, distinguishable from a real zero", func(t *testing.T) {
		// Zero price: unavailable, so value() fails even with a supplied
		// amount — it must never masquerade as a zero valuation.
		m, err := newCryptoMatch(Holding{amount: 10, hasAmount: true}, Crypto{NameEn: "bitcoin", PriceToman: "0"})
		if err == nil {
			t.Fatal("newCryptoMatch() error = nil, want error")
		}
		if v, ok := m.value(); ok || v != 0 {
			t.Errorf("value() = (%v, %v), want (0, false)", v, ok)
		}

		// A real calculated zero (positive price, zero amount) is ok:
		// (0, true) vs (0, false) is the required distinction.
		zeroAmount, err := newCryptoMatch(Holding{amount: 0, hasAmount: true}, Crypto{NameEn: "ethereum", PriceToman: "1500000"})
		if err != nil {
			t.Fatalf("newCryptoMatch() error = %v", err)
		}
		if v, ok := zeroAmount.value(); !ok || v != 0 {
			t.Errorf("value() = (%v, %v), want (0, true)", v, ok)
		}

		// Positive prices with supplied amounts still calculate correctly.
		good, err := newCryptoMatch(Holding{amount: 10, hasAmount: true}, Crypto{NameEn: "ethereum", PriceToman: "1500000"})
		if err != nil {
			t.Fatalf("newCryptoMatch() error = %v", err)
		}
		if v, ok := good.value(); !ok || v != 15000000 {
			t.Errorf("value() = (%v, %v), want (15000000, true)", v, ok)
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
			Item:    GoldCurrencyItem{Price: 1000, Unit: "تومان"},
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
			Item:    GoldCurrencyItem{Price: 1000, Unit: "تومان"},
		}
		v, ok := m.value()
		if !ok {
			t.Fatal("value() ok = false, want true")
		}
		if v != 2500 {
			t.Errorf("value() = %v, want 2500", v)
		}
	})

	t.Run("dollar-denominated price has no Toman valuation", func(t *testing.T) {
		// The Gold Ounce item is found and priced, but in dollars; its value
		// printed as Toman cannot be computed without an exchange-rate
		// policy, so value() reports unavailable rather than a Toman figure.
		m := GoldCurrencyMatch{
			Holding: Holding{name: "gold ounce", amount: 1, hasAmount: true},
			Item:    GoldCurrencyItem{NameEn: "Gold Ounce", Price: 4143, Unit: "دلار"},
		}
		if v, ok := m.value(); ok || v != 0 {
			t.Errorf("value() = (%v, %v), want (0, false)", v, ok)
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

	t.Run("formatAmount", func(t *testing.T) {
		tests := []struct {
			name   string
			amount float64
			want   string
		}{
			{
				name:   "fractional amount",
				amount: 0.002,
				want:   "0.002",
			},
			{
				name:   "small amount without scientific notation",
				amount: 0.00001,
				want:   "0.00001",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				m := CryptoMatch{
					Holding: Holding{amount: tt.amount, hasAmount: true},
				}
				if got := m.formatAmount(); got != tt.want {
					t.Errorf("formatAmount() = %q, want %q", got, tt.want)
				}
			})
		}
	})
}
