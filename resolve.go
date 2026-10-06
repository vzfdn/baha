package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
)

// AssetLookup contains the market data resolved for requested assets,
// along with the names that could not be found.
type AssetLookup struct {
	GoldCurrency []GoldCurrencyMatch
	Crypto       []CryptoMatch
	Missing      []string
}

// resolveAssets resolves requested holdings against the available gold,
// currency, and cryptocurrency market data. Assets that cannot be resolved
// are returned in AssetLookup.Missing.
//
// Holdings are expected to be deduplicated by normalized name before being
// passed in; parseHoldings enforces this. Duplicate holdings are not detected
// here and the last one wins.
func resolveAssets(ctx context.Context, client *http.Client, apiKey string, holdings []Holding) (AssetLookup, error) {
	byName := make(map[string]Holding, len(holdings))
	names := make([]string, len(holdings))
	for i, h := range holdings {
		byName[normalizeName(h.name)] = h
		names[i] = h.name
	}

	var result AssetLookup
	found := make(map[string]struct{}, len(holdings))

	goldCurrency, err := fetchGoldCurrency(ctx, client, goldCurrencyBaseURL, apiKey, names)
	if err != nil {
		return AssetLookup{}, fmt.Errorf("fetching gold/currency: %w", err)
	}
	for _, item := range goldCurrency {
		key := normalizeName(item.NameEn)
		h, err := lookupHolding(byName, key, item.NameEn)
		if err != nil {
			return AssetLookup{}, err
		}
		result.GoldCurrency = append(result.GoldCurrency, GoldCurrencyMatch{Holding: h, Item: item})
		found[key] = struct{}{}
	}

	crypto, err := fetchCrypto(ctx, client, cryptoBaseURL, apiKey, names)
	if err != nil {
		return AssetLookup{}, fmt.Errorf("fetching crypto: %w", err)
	}
	for _, item := range crypto {
		key := normalizeName(item.NameEn)
		h, err := lookupHolding(byName, key, item.NameEn)
		if err != nil {
			return AssetLookup{}, err
		}
		m, err := newCryptoMatch(h, item)
		if err != nil {
			return AssetLookup{}, fmt.Errorf("processing crypto data: %w", err)
		}
		result.Crypto = append(result.Crypto, m)
		found[key] = struct{}{}
	}

	for _, h := range holdings {
		if _, ok := found[normalizeName(h.name)]; !ok {
			result.Missing = append(result.Missing, h.name)
		}
	}
	return result, nil
}

func lookupHolding(byName map[string]Holding, key, itemName string) (Holding, error) {
	h, ok := byName[key]
	if !ok {
		// Fires if a matched name is missing from byName, indicating a
		// call-site bug.
		return Holding{}, fmt.Errorf("internal error: matched item %q has no corresponding holding", itemName)
	}
	return h, nil
}

// GoldCurrencyMatch pairs a requested Holding with the GoldCurrencyItem
// that resolved it.
type GoldCurrencyMatch struct {
	Holding Holding
	Item    GoldCurrencyItem
}

func (gcm GoldCurrencyMatch) value() (toman float64, ok bool) {
	if !gcm.Holding.hasAmount {
		return 0, false
	}
	return gcm.Holding.amount * float64(gcm.Item.Price), true
}

func (gcm GoldCurrencyMatch) formatAmount() string {
	return numPrinter.Sprintf("%.0f", gcm.Holding.amount)
}

// CryptoMatch pairs a requested Holding with the Crypto item that resolved
// it. priceToman stores Item.PriceToman parsed as a number during construction.
type CryptoMatch struct {
	Holding    Holding
	Item       Crypto
	priceToman float64
}

// newCryptoMatch creates a CryptoMatch after parsing the item's Toman price.
func newCryptoMatch(h Holding, item Crypto) (CryptoMatch, error) {
	priceToman, err := strconv.ParseFloat(string(item.PriceToman), 64)
	if err != nil {
		return CryptoMatch{}, fmt.Errorf("parsing price_toman for %s: %w", item.NameEn, err)
	}
	return CryptoMatch{Holding: h, Item: item, priceToman: priceToman}, nil
}

func (cm CryptoMatch) value() (toman float64, ok bool) {
	if !cm.Holding.hasAmount {
		return 0, false
	}
	return cm.Holding.amount * cm.priceToman, true
}

func (cm CryptoMatch) formatAmount() string {
	return numPrinter.Sprintf("%g", cm.Holding.amount)
}
