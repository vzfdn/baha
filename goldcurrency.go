package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const goldCurrencyBaseURL = "https://Api.BrsApi.ir/Market/Gold_Currency.php"

// GoldCurrencyItem represents a gold or currency market item returned by the API.
type GoldCurrencyItem struct {
	Date          string  `json:"date"`
	Time          string  `json:"time"`
	NameEn        string  `json:"name_en"`
	Symbol        string  `json:"symbol"`
	Price         int64   `json:"price"`
	ChangePercent float64 `json:"change_percent"`
	MarketCap     int64   `json:"market_cap"`
	Unit          string  `json:"unit"`
}

type goldCurrencyResponse struct {
	Gold     []GoldCurrencyItem `json:"gold"`
	Currency []GoldCurrencyItem `json:"currency"`
}

// fetchGoldCurrency retrieves and decodes the gold and currency market data.
func fetchGoldCurrency(ctx context.Context, client *http.Client, baseURL, apiKey string) (*goldCurrencyResponse, error) {
	body, err := fetchBody(ctx, client, baseURL, apiKey)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	var gcr goldCurrencyResponse
	if err := json.NewDecoder(body).Decode(&gcr); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &gcr, nil
}

// selectAssets returns the items in pool whose names match those in names,
// ignoring case and surrounding whitespace. Each matching name is returned
// at most once.
func selectAssets(pool []GoldCurrencyItem, names []string) []GoldCurrencyItem {
	wanted := make(map[string]struct{}, len(names))
	for _, name := range names {
		wanted[normalizeName(name)] = struct{}{}
	}
	if len(wanted) == 0 {
		return nil
	}

	found := make([]GoldCurrencyItem, 0, len(wanted))
	for _, item := range pool {
		key := normalizeName(item.NameEn)
		if _, ok := wanted[key]; ok {
			found = append(found, item)
			delete(wanted, key)
		}
	}
	return found
}
