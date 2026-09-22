// baha retrieves market data for requested assets and displays
// the results in the terminal.
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
		log.Fatal(`usage: go run . asset1=amount asset2=amount ...`)
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
	if err != nil {
		log.Fatalf("could not resolve assets: %v", err)
	}

	if len(lookup.GoldCurrency) > 0 || len(lookup.Crypto) > 0 {
		fmt.Println(renderResult(lookup))
	}

	if len(lookup.Missing) > 0 {
		if len(lookup.GoldCurrency) == 0 && len(lookup.Crypto) == 0 {
			log.Fatalf("none of the requested assets were found: %s", strings.Join(lookup.Missing, ", "))
		}
		log.Printf("not found: %s", strings.Join(lookup.Missing, ", "))
	}
}
