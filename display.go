package main

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var numPrinter = message.NewPrinter(language.English)

// renderResult renders resolved asset data as a collection of cards.
func renderResult(lookup AssetLookup) string {
	cards := make([]card, 0, len(lookup.GoldCurrency)+len(lookup.Crypto))
	for _, m := range lookup.GoldCurrency {
		cards = append(cards, goldCurrencyCard(m))
	}
	for _, m := range lookup.Crypto {
		cards = append(cards, cryptoCard(m))
	}
	return renderCards(cards)
}

// goldCurrencyCard creates a display card for a gold or currency asset.
func goldCurrencyCard(m GoldCurrencyMatch) card {
	item := m.Item
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "Symbol\t%s\n", item.Symbol)
	fmt.Fprintf(tw, "Price\t%s %s\n", numPrinter.Sprintf("%d", item.Price), item.Unit)
	fmt.Fprintf(tw, "Change\t%+.2f%%\n", item.ChangePercent)
	if item.MarketCap != 0 {
		fmt.Fprintf(tw, "Market Cap\t%s\n", numPrinter.Sprintf("$%d", item.MarketCap))
	}
	if toman, ok := m.value(); ok {
		fmt.Fprintf(tw, "Your Amount(%s)\t%s\n", m.formatAmount(), formatToman(toman))
	}
	fmt.Fprintf(tw, "Time\t%s\n", item.Time)
	fmt.Fprintf(tw, "Date\t%s\n", item.Date)
	tw.Flush()

	return card{
		Header: item.NameEn,
		Rows:   strings.Split(strings.TrimRight(buf.String(), "\n"), "\n"),
	}
}

// cryptoCard creates a display card for a cryptocurrency asset.
func cryptoCard(m CryptoMatch) card {
	item := m.Item
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "Price\t$%s\n", formatNumericString(item.Price))
	if m.priceUnavailable {
		fmt.Fprintf(tw, "Price (Toman)\tunavailable\n")
	} else {
		fmt.Fprintf(tw, "Price (Toman)\t%s\n", formatNumericString(item.PriceToman))
	}
	fmt.Fprintf(tw, "Change\t%+.2f%%\n", item.ChangePercent)
	if item.MarketCap != 0 {
		fmt.Fprintf(tw, "Market Cap\t%s\n", numPrinter.Sprintf("$%d", item.MarketCap))
	}
	if toman, ok := m.value(); ok {
		fmt.Fprintf(tw, "Your Amount(%s)\t%s\n", m.formatAmount(), formatToman(toman))
	}
	fmt.Fprintf(tw, "Time\t%s\n", item.Time)
	fmt.Fprintf(tw, "Date\t%s\n", item.Date)
	tw.Flush()

	return card{
		Header: item.NameEn,
		Rows:   strings.Split(strings.TrimRight(buf.String(), "\n"), "\n"),
	}
}

// card represents the text content of a rendered asset card.
type card struct {
	Header string
	Rows   []string
}

// renderCards renders cards using a common width based on the widest content.
func renderCards(cards []card) string {
	width := 0
	for _, c := range cards {
		if w := utf8.RuneCountInString(c.Header); w > width {
			width = w
		}
		for _, r := range c.Rows {
			if w := utf8.RuneCountInString(r); w > width {
				width = w
			}
		}
	}
	var out strings.Builder
	for i, c := range cards {
		if i > 0 {
			out.WriteString("\n")
		}
		out.WriteString(renderBox(c.Header, c.Rows, width))
		out.WriteString("\n")
	}
	return strings.TrimRight(out.String(), "\n")
}

// renderBox renders a single card using the specified content width.
func renderBox(header string, rows []string, width int) string {
	rule := strings.Repeat("─", width+2)

	var b strings.Builder
	b.WriteString("┌")
	b.WriteString(rule)
	b.WriteString("┐\n")

	b.WriteString("│ ")
	b.WriteString(padRight(header, width))
	b.WriteString(" │\n")

	b.WriteString("├")
	b.WriteString(rule)
	b.WriteString("┤\n")

	for _, r := range rows {
		b.WriteString("│ ")
		b.WriteString(padRight(r, width))
		b.WriteString(" │\n")
	}

	b.WriteString("└")
	b.WriteString(rule)
	b.WriteString("┘")
	return b.String()
}

// formatNumericString adds thousands separators to the integer portion of s
// while preserving its fractional portion. If the integer portion cannot be
// parsed, it returns s unchanged.
func formatNumericString(s string) string {
	intPart, frac, hasFrac := strings.Cut(s, ".")
	n, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return s
	}
	grouped := numPrinter.Sprintf("%d", n)
	if hasFrac {
		return grouped + "." + frac
	}
	return grouped
}

func padRight(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

// formatToman formats amount as a whole number of Toman, rounded to the nearest.
func formatToman(amount float64) string {
	return numPrinter.Sprintf("%.0f Toman", amount)
}
