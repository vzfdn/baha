package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFormatNumericString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"small integer", "12", "12"},
		{"zero", "0", "0"},
		{"thousands separated", "1234567", "1,234,567"},
		{"negative", "-1234567", "-1,234,567"},
		{"with fraction", "1234.56", "1,234.56"},
		{"negative with fraction", "-1234.56", "-1,234.56"},
		{"trailing dot", "123.", "123."},
		{"fraction only passthrough", ".5", ".5"},
		{"non-numeric passthrough", "abc", "abc"},
		{"empty passthrough", "", ""},
		{"overflow passthrough", "99999999999999999999", "99999999999999999999"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatNumericString(tt.input); got != tt.want {
				t.Errorf("formatNumericString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		name  string
		input string
		width int
		want  string
	}{
		{"shorter padded", "abc", 5, "abc  "},
		{"exact length unchanged", "abc", 3, "abc"},
		{"longer unchanged", "abcdef", 3, "abcdef"},
		{"empty padded", "", 3, "   "},
		{"zero width empty", "", 0, ""},
		{"unicode shorter padded", "日本", 4, "日本  "},
		{"unicode exact length", "日本", 2, "日本"},
		{"unicode longer unchanged", "日本語", 2, "日本語"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := padRight(tt.input, tt.width); got != tt.want {
				t.Errorf("padRight(%q, %d) = %q, want %q", tt.input, tt.width, got, tt.want)
			}
		})
	}
}

func TestFormatToman(t *testing.T) {
	tests := []struct {
		name   string
		amount float64
		want   string
	}{
		{"zero", 0, "0 Toman"},
		{"whole", 1000, "1,000 Toman"},
		{"large", 1234567, "1,234,567 Toman"},
		{"below one rounds to zero", 0.4, "0 Toman"},
		{"above one half rounds to one", 0.6, "1 Toman"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatToman(tt.amount); got != tt.want {
				t.Errorf("formatToman(%v) = %q, want %q", tt.amount, got, tt.want)
			}
		})
	}
}

func TestRenderBox(t *testing.T) {
	t.Run("all lines have uniform rune width", func(t *testing.T) {
		// The box has to line up vertically regardless of content length,
		// so every line should be the same number of runes. This is what
		// the padding is for; if padRight or the border widths drift, this
		// catches it without asserting on the specific box characters.
		got := renderBox("header", []string{"row one", "row two"}, 10)
		lines := strings.Split(got, "\n")

		wantWidth := utf8.RuneCountInString(lines[0])
		for i, line := range lines {
			if w := utf8.RuneCountInString(line); w != wantWidth {
				t.Errorf("line %d width = %d, want %d: %q", i, w, wantWidth, line)
			}
		}
	})

	t.Run("header and rows appear inside the box", func(t *testing.T) {
		got := renderBox("header", []string{"row one", "row two"}, 10)
		for _, want := range []string{"header", "row one", "row two"} {
			if !strings.Contains(got, want) {
				t.Errorf("output does not contain %q\noutput:\n%s", want, got)
			}
		}
	})

	t.Run("line count is border plus content", func(t *testing.T) {
		got := renderBox("h", []string{"a", "b", "c"}, 5)
		// Top border, header row, separator, three content rows, bottom border.
		if n := strings.Count(got, "\n") + 1; n != 7 {
			t.Errorf("line count = %d, want 7\noutput:\n%s", n, got)
		}
	})

	t.Run("no rows", func(t *testing.T) {
		got := renderBox("h", nil, 5)
		// Top border, header row, separator, bottom border.
		if n := strings.Count(got, "\n") + 1; n != 4 {
			t.Errorf("line count = %d, want 4\noutput:\n%s", n, got)
		}
	})
}

func TestRenderCards(t *testing.T) {
	t.Run("empty input produces empty string", func(t *testing.T) {
		if got := renderCards(nil); got != "" {
			t.Errorf("renderCards(nil) = %q, want %q", got, "")
		}
	})

	t.Run("single card has no trailing newline", func(t *testing.T) {
		got := renderCards([]card{{Header: "h", Rows: []string{"r"}}})
		if strings.HasSuffix(got, "\n") {
			t.Errorf("output ends with newline: %q", got)
		}
		if !strings.Contains(got, "h") {
			t.Error("output does not contain header")
		}
		if !strings.Contains(got, "r") {
			t.Error("output does not contain row")
		}
	})

	t.Run("multiple cards separated by one blank line", func(t *testing.T) {
		got := renderCards([]card{
			{Header: "one", Rows: []string{"r1"}},
			{Header: "two", Rows: []string{"r2"}},
		})
		if n := strings.Count(got, "\n\n"); n != 1 {
			t.Errorf("blank-line separator count = %d, want 1\noutput:\n%s", n, got)
		}
		if !strings.Contains(got, "one") || !strings.Contains(got, "two") {
			t.Error("output missing one or both headers")
		}
	})

	t.Run("width is common across cards", func(t *testing.T) {
		// Both boxes should share the wider card's width so they align.
		got := renderCards([]card{
			{Header: "short", Rows: []string{"x"}},
			{Header: "much-longer-header", Rows: []string{"y"}},
		})
		lines := strings.Split(got, "\n")

		var wantWidth int
		for _, line := range lines {
			if line == "" {
				continue // separator blank line
			}
			w := utf8.RuneCountInString(line)
			if wantWidth == 0 {
				wantWidth = w
				continue
			}
			if w != wantWidth {
				t.Errorf("line width = %d, want %d: %q", w, wantWidth, line)
			}
		}
	})
}

func TestRenderResult(t *testing.T) {
	t.Run("empty lookup produces empty string", func(t *testing.T) {
		if got := renderResult(AssetLookup{}); got != "" {
			t.Errorf("renderResult(empty) = %q, want empty", got)
		}
	})

	t.Run("gold/currency match is rendered", func(t *testing.T) {
		lookup := AssetLookup{
			GoldCurrency: []GoldCurrencyMatch{{
				Holding: Holding{name: "gold"},
				Item: GoldCurrencyItem{
					NameEn: "Gold 18K",
					Symbol: "XAU18K",
					Price:  12345678,
					Unit:   "Toman/gram",
				},
			}},
		}
		got := renderResult(lookup)
		if !strings.Contains(got, "Gold 18K") {
			t.Errorf("output missing asset name\noutput:\n%s", got)
		}
		if !strings.Contains(got, "XAU18K") {
			t.Errorf("output missing symbol\noutput:\n%s", got)
		}
	})

	t.Run("crypto match is rendered", func(t *testing.T) {
		lookup := AssetLookup{
			Crypto: []CryptoMatch{{
				Holding:    Holding{name: "bitcoin"},
				Item:       Crypto{NameEn: "Bitcoin", Price: "12345.67", PriceToman: "3000000000"},
				priceToman: 3000000000,
			}},
		}
		got := renderResult(lookup)
		if !strings.Contains(got, "Bitcoin") {
			t.Errorf("output missing asset name\noutput:\n%s", got)
		}
	})

	t.Run("holding amount is rendered as computed value", func(t *testing.T) {
		lookup := AssetLookup{
			GoldCurrency: []GoldCurrencyMatch{{
				Holding: Holding{name: "gold", amount: 2, hasAmount: true},
				Item:    GoldCurrencyItem{NameEn: "Gold", Price: 1000},
			}},
		}
		got := renderResult(lookup)
		if !strings.Contains(got, "2,000 Toman") {
			t.Errorf("output missing computed value\noutput:\n%s", got)
		}
	})

	t.Run("mixed matches render both cards", func(t *testing.T) {
		lookup := AssetLookup{
			GoldCurrency: []GoldCurrencyMatch{{
				Holding: Holding{name: "gold"},
				Item:    GoldCurrencyItem{NameEn: "Gold", Symbol: "XAU"},
			}},
			Crypto: []CryptoMatch{{
				Holding:    Holding{name: "bitcoin"},
				Item:       Crypto{NameEn: "Bitcoin", PriceToman: "3000000000"},
				priceToman: 3000000000,
			}},
		}
		got := renderResult(lookup)
		if !strings.Contains(got, "Gold") {
			t.Error("output missing gold card")
		}
		if !strings.Contains(got, "Bitcoin") {
			t.Error("output missing crypto card")
		}
	})
}

func TestGoldCurrencyCard(t *testing.T) {
	baseItem := GoldCurrencyItem{
		NameEn: "Gold",
		Symbol: "XAU",
		Price:  1000,
		Unit:   "Toman/gram",
	}

	t.Run("header matches item name", func(t *testing.T) {
		got := goldCurrencyCard(GoldCurrencyMatch{
			Holding: Holding{name: "gold"},
			Item:    baseItem,
		})
		if got.Header != "Gold" {
			t.Errorf("Header = %q, want %q", got.Header, "Gold")
		}
	})

	t.Run("market cap row omitted when zero", func(t *testing.T) {
		got := goldCurrencyCard(GoldCurrencyMatch{
			Holding: Holding{name: "gold"},
			Item:    baseItem,
		})
		joined := strings.Join(got.Rows, "\n")
		if strings.Contains(joined, "Market Cap") {
			t.Errorf("Market Cap row should be omitted\nrows:\n%s", joined)
		}
	})

	t.Run("market cap row shown when non-zero", func(t *testing.T) {
		item := baseItem
		item.MarketCap = 5000000
		got := goldCurrencyCard(GoldCurrencyMatch{
			Holding: Holding{name: "gold"},
			Item:    item,
		})
		joined := strings.Join(got.Rows, "\n")
		if !strings.Contains(joined, "Market Cap") {
			t.Errorf("Market Cap row should be present\nrows:\n%s", joined)
		}
	})

	t.Run("your amount row omitted when holding has no amount", func(t *testing.T) {
		got := goldCurrencyCard(GoldCurrencyMatch{
			Holding: Holding{name: "gold"},
			Item:    baseItem,
		})
		joined := strings.Join(got.Rows, "\n")
		if strings.Contains(joined, "Your Amount") {
			t.Errorf("Your Amount row should be omitted\nrows:\n%s", joined)
		}
	})

	t.Run("your amount row shown when holding has an amount", func(t *testing.T) {
		got := goldCurrencyCard(GoldCurrencyMatch{
			Holding: Holding{name: "gold", amount: 2, hasAmount: true},
			Item:    baseItem,
		})
		joined := strings.Join(got.Rows, "\n")
		if !strings.Contains(joined, "Your Amount") {
			t.Errorf("Your Amount row should be present\nrows:\n%s", joined)
		}
		if !strings.Contains(joined, "Your Amount(2)") {
			t.Errorf("entered amount should display as 2\nrows:\n%s", joined)
		}
		if !strings.Contains(joined, "2,000 Toman") {
			t.Errorf("computed value should be present\nrows:\n%s", joined)
		}
	})

}

func TestCryptoCard(t *testing.T) {
	baseItem := Crypto{
		NameEn:     "Bitcoin",
		Price:      "12345.67",
		PriceToman: "3000000000",
	}

	t.Run("header matches item name", func(t *testing.T) {
		got := cryptoCard(CryptoMatch{
			Holding: Holding{name: "bitcoin"},
			Item:    baseItem,
		})
		if got.Header != "Bitcoin" {
			t.Errorf("Header = %q, want %q", got.Header, "Bitcoin")
		}
	})

	t.Run("market cap row omitted when zero", func(t *testing.T) {
		got := cryptoCard(CryptoMatch{
			Holding: Holding{name: "bitcoin"},
			Item:    baseItem,
		})
		joined := strings.Join(got.Rows, "\n")
		if strings.Contains(joined, "Market Cap") {
			t.Errorf("Market Cap row should be omitted\nrows:\n%s", joined)
		}
	})

	t.Run("market cap row shown when non-zero", func(t *testing.T) {
		item := baseItem
		item.MarketCap = 987654321
		got := cryptoCard(CryptoMatch{
			Holding: Holding{name: "bitcoin"},
			Item:    item,
		})
		joined := strings.Join(got.Rows, "\n")
		if !strings.Contains(joined, "Market Cap") {
			t.Errorf("Market Cap row should be present\nrows:\n%s", joined)
		}
	})

	t.Run("your amount row omitted when holding has no amount", func(t *testing.T) {
		got := cryptoCard(CryptoMatch{
			Holding: Holding{name: "bitcoin"},
			Item:    baseItem,
		})
		joined := strings.Join(got.Rows, "\n")
		if strings.Contains(joined, "Your Amount") {
			t.Errorf("Your Amount row should be omitted\nrows:\n%s", joined)
		}
	})

	t.Run("your amount row shown when holding has an amount", func(t *testing.T) {
		got := cryptoCard(CryptoMatch{
			Holding:    Holding{name: "bitcoin", amount: 0.5, hasAmount: true},
			Item:       baseItem,
			priceToman: 3000000000,
		})
		joined := strings.Join(got.Rows, "\n")
		if !strings.Contains(joined, "Your Amount") {
			t.Errorf("Your Amount row should be present\nrows:\n%s", joined)
		}
		if !strings.Contains(joined, "Your Amount(0.5)") {
			t.Errorf("entered amount should display as 0.5\nrows:\n%s", joined)
		}
		if !strings.Contains(joined, "1,500,000,000 Toman") {
			t.Errorf("computed value should be present\nrows:\n%s", joined)
		}
	})
}
