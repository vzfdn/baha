package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const resolveTimeout = 10 * time.Second

func main() {
	flag.Parse()

	holdings, err := parseHoldings(flag.Args())
	if err != nil {
		log.Fatalf("invalid input: %v", err)
	}
	if len(holdings) == 0 {
		log.Fatal(`usage: baha asset1[=amount] asset2[=amount] ...`)
	}

	if err := godotenv.Load(); err != nil {
		if os.IsNotExist(err) {
			log.Println("no .env file found, relying on OS environment variables")
		} else {
			log.Fatalf("error loading .env file: %v", err)
		}
	}
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		log.Fatal("API_KEY not set")
	}

	// The deadline applies to the entire asset-resolution operation,
	// including all market-data requests.
	ctx, cancel := context.WithTimeout(context.Background(), resolveTimeout)
	defer cancel()

	lookup, err := resolveAssets(ctx, &http.Client{}, apiKey, holdings)

	// Keep partial results when a source fails; exit nonzero only if
	// nothing could be resolved.
	hasResults := len(lookup.GoldCurrency) > 0 || len(lookup.Crypto) > 0

	// Missing and Unverified are mutually exclusive, so report Unverified
	// before the fatal branches to preserve the names on total failure.
	if len(lookup.Unverified) > 0 {
		// Softer than Missing: these names might still exist in a source
		// that could not be queried.
		log.Printf("could not check: %s", strings.Join(lookup.Unverified, ", "))
	}

	if err != nil {
		if !hasResults {
			log.Fatalf("could not resolve assets: %v", err)
		}
		// Source failures and unusable prices alike are warning-level here
		// because there are still results to show.
		log.Printf("warnings while resolving assets: %v", err)
	}

	if hasResults {
		fmt.Println(renderResult(lookup))
	}
	if len(lookup.Missing) > 0 {
		if !hasResults {
			log.Fatalf("none of the requested assets were found: %s", strings.Join(lookup.Missing, ", "))
		}
		log.Printf("not found: %s", strings.Join(lookup.Missing, ", "))
	}
}
