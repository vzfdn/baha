package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const cryptoBaseURL = "https://Api.BrsApi.ir/Market/Cryptocurrency.php"

// Crypto represents a cryptocurrency market item returned by the API.
type Crypto struct {
	Date          string  `json:"date"`
	Time          string  `json:"time"`
	NameEn        string  `json:"name_en"`
	Price         string  `json:"price"`
	PriceToman    string  `json:"price_toman"`
	ChangePercent float64 `json:"change_percent"`
	MarketCap     int64   `json:"market_cap"`
}

// fetchCrypto retrieves the requested cryptocurrency market items.
// Names are matched case-insensitively and ignoring surrounding whitespace.
// It returns only items that were found; an absent item is not an error, and
// it stops reading once all requested items have been found.
func fetchCrypto(ctx context.Context, client *http.Client, baseURL, apiKey string, names []string) ([]Crypto, error) {
	wanted := make(map[string]struct{}, len(names))
	for _, n := range names {
		wanted[normalizeName(n)] = struct{}{}
	}
	if len(wanted) == 0 {
		return nil, nil
	}

	body, err := fetchBody(ctx, client, baseURL, apiKey)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	dec := json.NewDecoder(body)
	t, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("reading array start: %w", err)
	}
	if delim, ok := t.(json.Delim); !ok || delim != '[' {
		return nil, fmt.Errorf("expected JSON array, got %v", t)
	}

	found := make([]Crypto, 0, len(wanted))
	for dec.More() && len(wanted) > 0 {
		var item Crypto
		if err := dec.Decode(&item); err != nil {
			return nil, fmt.Errorf("decoding item: %w", err)
		}

		key := normalizeName(item.NameEn)
		if _, ok := wanted[key]; ok {
			found = append(found, item)
			delete(wanted, key)
		}
	}

	// More() returns false both for a clean end-of-array and for a read
	// error, so a truncation between items would otherwise go unnoticed.
	// Decode catches truncations inside a value; this catches the rest.
	if len(wanted) > 0 {
		if _, err := dec.Token(); err != nil {
			return nil, fmt.Errorf("reading array end: %w", err)
		}
	}

	return found, nil
}
