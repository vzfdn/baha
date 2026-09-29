package main

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Holding represents a requested asset and, optionally, the amount held.
type Holding struct {
	name      string
	amount    float64
	hasAmount bool
}

// parseHoldings parses asset arguments in the form "name" or "name=amount".
// Asset names are compared case-insensitively and surrounding whitespace is
// ignored. A bare name produces a holding without an amount. Invalid
// arguments are collected and returned as a single error.
func parseHoldings(args []string) ([]Holding, error) {
	var list []Holding
	seen := make(map[string]struct{}, len(args))
	var errs []error
	for _, a := range args {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		name, amountStr, found := strings.Cut(a, "=")
		name = strings.TrimSpace(name)
		if name == "" {
			errs = append(errs, fmt.Errorf("asset name cannot be empty in %q", a))
			continue
		}
		key := normalizeName(name)
		if _, exists := seen[key]; exists {
			errs = append(errs, fmt.Errorf("duplicate entry for %q", name))
			continue
		}
		seen[key] = struct{}{}
		if !found {
			list = append(list, Holding{name: name})
			continue
		}
		amount, err := strconv.ParseFloat(strings.TrimSpace(amountStr), 64)
		if err != nil {
			errs = append(errs, fmt.Errorf("parsing amount for %q: %w", name, err))
			continue
		}
		if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 {
			errs = append(errs, fmt.Errorf("amount must be a finite, non-negative number for %q, got %v", name, amount))
			continue
		}
		list = append(list, Holding{name: name, amount: amount, hasAmount: true})
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return list, nil
}

// normalizeName returns s with surrounding whitespace removed and letters
// converted to lowercase for case-insensitive asset-name matching.
func normalizeName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
