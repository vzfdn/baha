package main

import (
	"context"
	"errors"
	"fmt"
	"math"
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

	// Both fetch functions select items by comparing normalizeName of the
	// item's NameEn against these same normalized holding names, so every item
	// they return has a byName entry and the lookups below cannot miss.
	goldCurrency, goldErr := fetchGoldCurrency(ctx, client, goldCurrencyBaseURL, apiKey, names)
	if goldErr != nil {
		goldErr = fmt.Errorf("fetching gold/currency: %w", goldErr)
	}
	// Price failures are collected rather than aborting the loop. Each item
	// is still appended and its name recorded as found: an unusable price
	// does not make the asset Missing or Unverified.
	var itemErrs []error
	for _, item := range goldCurrency {
		key := normalizeName(item.NameEn)
		m, err := newGoldCurrencyMatch(byName[key], item)
		if err != nil {
			itemErrs = append(itemErrs, fmt.Errorf("price unavailable: %w", err))
		}
		result.GoldCurrency = append(result.GoldCurrency, m)
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
			itemErrs = append(itemErrs, fmt.Errorf("price unavailable: %w", err))
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
	return result, errors.Join(append([]error{goldErr, cryptoErr}, itemErrs...)...)
}

// GoldCurrencyMatch pairs a requested Holding with the GoldCurrencyItem
// that resolved it.
type GoldCurrencyMatch struct {
	Holding Holding
	Item    GoldCurrencyItem
}

// priceUsable reports whether the price is usable. Zero and negative values
// are treated as missing data, not as a real valuation.
func (gcm GoldCurrencyMatch) priceUsable() bool {
	return gcm.Item.Price > 0
}

// value returns the holding's Toman value. An unusable price, a price
// denominated in another currency (Gold Ounce is quoted in dollars), or a
// missing amount has no Toman value, so it returns false.
func (gcm GoldCurrencyMatch) value() (toman float64, ok bool) {
	if !gcm.priceUsable() || !gcm.Holding.hasAmount || gcm.Item.Unit != "تومان" {
		return 0, false
	}
	return gcm.Holding.amount * float64(gcm.Item.Price), true
}

// newGoldCurrencyMatch pairs h with item. An unusable price is reported as
// an error, but the match is still returned so the caller can keep the
// resolved asset and record the error separately instead of discarding it.
func newGoldCurrencyMatch(h Holding, item GoldCurrencyItem) (GoldCurrencyMatch, error) {
	m := GoldCurrencyMatch{Holding: h, Item: item}
	if !m.priceUsable() {
		return m, fmt.Errorf("price for %s must be a positive number, got %d", item.NameEn, item.Price)
	}
	return m, nil
}

func (gcm GoldCurrencyMatch) formatAmount() string {
	return strconv.FormatFloat(gcm.Holding.amount, 'f', -1, 64)
}

// CryptoMatch pairs a requested Holding with the Crypto item that resolved
// it. priceToman stores Item.PriceToman parsed as a number, only when the
// price is usable. priceUnavailable marks an item that was resolved without
// a usable price; value() never computes with such a price, so an
// unavailable price is distinguishable from a real calculated value of zero.
type CryptoMatch struct {
	Holding          Holding
	Item             Crypto
	priceToman       float64
	priceUnavailable bool
}

// checkPriceToman parses and validates the item's Toman price. Parsing
// alone is not enough: strconv.ParseFloat also accepts NaN, infinities,
// negatives, and zero, none of which are usable prices. Only a positive,
// finite price is accepted: the API sample pairs Tether's non-zero USD price
// ("0.9989") with price_toman "0", which is more consistent with missing or
// unconverted data than with a real zero valuation.
func checkPriceToman(item Crypto) (float64, error) {
	p, err := strconv.ParseFloat(item.PriceToman, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing price_toman for %s: %w", item.NameEn, err)
	}
	if math.IsNaN(p) || math.IsInf(p, 0) || p <= 0 {
		return 0, fmt.Errorf("price_toman for %s must be a positive, finite number, got %v", item.NameEn, p)
	}
	return p, nil
}

// newCryptoMatch creates a CryptoMatch after validating the item's Toman
// price. When validation fails, the returned match is still complete with
// priceUnavailable set, so the caller can keep it and record the error
// separately instead of discarding the resolved asset.
func newCryptoMatch(h Holding, item Crypto) (CryptoMatch, error) {
	m := CryptoMatch{Holding: h, Item: item, priceUnavailable: true}
	priceToman, err := checkPriceToman(item)
	if err != nil {
		return m, err
	}
	m.priceToman = priceToman
	m.priceUnavailable = false
	return m, nil
}

func (cm CryptoMatch) value() (toman float64, ok bool) {
	if cm.priceUnavailable || !cm.Holding.hasAmount {
		return 0, false
	}
	return cm.Holding.amount * cm.priceToman, true
}

func (cm CryptoMatch) formatAmount() string {
	return strconv.FormatFloat(cm.Holding.amount, 'f', -1, 64)
}
