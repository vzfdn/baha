package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

// AssetLookup contains resolved market data and classifies requested names
// that could not be resolved.
//
// Missing contains names absent from all successfully queried sources.
// Unverified contains names that could not be fully checked because a
// relevant source failed. A name cannot appear in both.
type AssetLookup struct {
	GoldCurrency []GoldCurrencyMatch
	Crypto       []CryptoMatch
	Missing      []string
	Unverified   []string
}

// resolveAssets resolves requested holdings against the available gold,
// currency, and cryptocurrency market data. It retains partial results
// when either source fails and returns the collected errors.
//
// Holdings must be deduplicated by normalized name before being passed in;
// parseHoldings enforces this. Duplicate holdings are not detected here and
// the last one wins.
func resolveAssets(ctx context.Context, client *http.Client, apiKey string, holdings []Holding) (AssetLookup, error) {
	byName := make(map[string]Holding, len(holdings))
	names := make([]string, len(holdings))
	for i, h := range holdings {
		byName[normalizeName(h.name)] = h
		names[i] = h.name
	}

	var result AssetLookup
	found := make(map[string]struct{}, len(holdings))

	goldCurrency, goldErr := fetchGoldCurrency(ctx, client, goldCurrencyBaseURL, apiKey, names)
	if goldErr != nil {
		goldErr = fmt.Errorf("fetching gold/currency: %w", goldErr)
	}
	for _, item := range goldCurrency {
		key := normalizeName(item.NameEn)
		result.GoldCurrency = append(result.GoldCurrency, GoldCurrencyMatch{Holding: byName[key], Item: item})
		found[key] = struct{}{}
	}

	crypto, cryptoErr := fetchCrypto(ctx, client, cryptoBaseURL, apiKey, names)
	if cryptoErr != nil {
		cryptoErr = fmt.Errorf("fetching crypto: %w", cryptoErr)
	}
	for _, item := range crypto {
		key := normalizeName(item.NameEn)
		m, err := newCryptoMatch(byName[key], item)
		if err != nil {
			return AssetLookup{}, fmt.Errorf("processing crypto data: %w", err)
		}
		result.Crypto = append(result.Crypto, m)
		found[key] = struct{}{}
	}

	for _, h := range holdings {
		if _, ok := found[normalizeName(h.name)]; !ok {
			if goldErr == nil && cryptoErr == nil {
				result.Missing = append(result.Missing, h.name)
			} else {
				result.Unverified = append(result.Unverified, h.name)
			}
		}
	}
	return result, errors.Join(goldErr, cryptoErr)
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
	priceToman, err := strconv.ParseFloat(item.PriceToman, 64)
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
	return strconv.FormatFloat(cm.Holding.amount, 'f', -1, 64)
}
