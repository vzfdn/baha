# baha

A command-line tool that looks up gold, currency, and cryptocurrency
prices and, for prices denominated in Toman, calculates the value of a
holding.

You pass one or more asset names, optionally with an amount, and `baha` prints
a card for each asset it resolves:

```bash
go run . "us dollar"=2 bitcoin=0.0020
```

Two dollars held and a small Bitcoin position produce two cards — one from the
gold/currency feed, one from the cryptocurrency feed. Output in this README is
the exact rendering of the current code over real provider data; the prices
shown are snapshots (each card's `Date` row shows when they were taken), **not
current**:

```text
┌────────────────────────────────────────┐
│ US Dollar                              │
├────────────────────────────────────────┤
│ Symbol          USD                    │
│ Price           256,900 Toman          │
│ Change          -2.22%                 │
│ Your Amount(2)  513,800 Toman          │
│ Time            19:12                  │
│ Date            1405/07/21             │
└────────────────────────────────────────┘

┌────────────────────────────────────────┐
│ Bitcoin                                │
├────────────────────────────────────────┤
│ Price               $83,327            │
│ Price (Toman)       21,406,630,049     │
│ Change              +0.79%             │
│ Market Cap          $1,674,672,166,270 │
│ Your Amount(0.002)  42,813,260 Toman   │
│ Time                19:08              │
│ Date                1405/07/21         │
└────────────────────────────────────────┘
```

`baha` is a personal utility, not a service. It reads a single free market-data
provider and prints what that provider returns.

## Requirements

- **Go 1.27 or newer.** The version is declared in `go.mod`.
- **A BrsApi key.** Both market-data endpoints require one. Request a key at
  <https://brsapi.ir/>.

## Configuration

The program reads its key from the `API_KEY` environment variable. The simplest
way to set it is a local `.env` file, which the program loads on startup with
[`godotenv`](https://github.com/joho/godotenv):

```bash
cp .env.sample .env
```

Then edit `.env` and set the value:

```dotenv
# Get a key from https://brsapi.ir/
API_KEY=your-key-here
```

`.env` is git-ignored and must never be committed. If no `.env` file exists,
the program falls back to the environment it was started in, which is the
right choice in a container or CI job:

```bash
API_KEY=your-key-here go run . bitcoin
```

If `API_KEY` is empty, the program stops with `API_KEY not set`.

## Usage

```
go run . asset1[=amount] asset2[=amount] ...
```

- An asset may be given as a bare name to print its price, or as `name=amount`
  to also calculate the value of that amount (for items whose price is in
  Toman).
- Amounts are plain numbers, and fractional amounts such as `0.0020` are
  supported.
- Names are matched case-insensitively, and surrounding whitespace is ignored.
  Amounts must be finite and non-negative.
- Quote names that contain spaces.
- Entries that repeat the same asset (after case and whitespace are ignored)
  are rejected as duplicates.

Examples — each invocation immediately followed by its exact output
(snapshots, therefore not current):

A bare name prints only the provider's price:

```bash
go run . euro
```

```text
┌───────────────────────┐
│ Euro                  │
├───────────────────────┤
│ Symbol  EUR           │
│ Price   287,620 Toman │
│ Change  -1.91%        │
│ Time    19:12         │
│ Date    1405/07/21    │
└───────────────────────┘
```

An amount adds the computed value of the holding:

```bash
go run . "18K Gold"=5
```

```text
┌───────────────────────────────────┐
│ 18K Gold                          │
├───────────────────────────────────┤
│ Symbol          IR_GOLD_18K       │
│ Price           25,970,300 Toman  │
│ Change          +0.90%            │
│ Your Amount(5)  129,851,500 Toman │
│ Time            12:16             │
│ Date            1405/07/22        │
└───────────────────────────────────┘
```

`Gold Ounce` is priced in dollars, so its card omits the computed value row
(see the notes below):

```bash
go run . "gold ounce"=2
```

```text
┌──────────────────────┐
│ Gold Ounce           │
├──────────────────────┤
│ Symbol  XAUUSD       │
│ Price   4,143 Dollar │
│ Change  -0.37%       │
│ Time    12:15        │
│ Date    1405/07/22   │
└──────────────────────┘
```

Build a binary first if you prefer not to go through `go run`:

```bash
go build -o baha . && ./baha "tether dollar"=1500000
```

```text
┌───────────────────────────────────────────────┐
│ Tether Dollar                                 │
├───────────────────────────────────────────────┤
│ Symbol                  USDT_IRT              │
│ Price                   261,101 Toman         │
│ Change                  +1.86%                │
│ Your Amount(1,500,000)  391,651,500,000 Toman │
│ Time                    12:16:45              │
│ Date                    1405/07/22            │
└───────────────────────────────────────────────┘
```

### Notes on the output

- **The unit field is translated for display only.** The provider sends the
  unit in Persian (`تومان` for most items, `دلار` for `Gold Ounce`), and the
  gold/currency card renders exactly that field in English, so a card reads
  `Price  256,900 Toman` or `Price  4,143 Dollar`. The decoded data is left
  unchanged, and any other provider text is printed as sent.
- The `Date` row is in the provider's own calendar, a Jalali (Solar Hijri)
  date.
- `Change` is the percentage change the provider reports for the item.
- `Your Amount` is only printed when an amount was supplied and the item's
  price is denominated in Toman.
- **`Gold Ounce` gets its price but no `Your Amount`.** Its card ends after
  `Price  4,143 Dollar`. This is a deliberate scope boundary, not an error: a
  Toman value needs an exchange rate, and the same response carries two
  different dollar rates — `USD` and `USDT_IRT` (260,860 and 261,101 Toman in
  one snapshot) — with no policy for choosing between them. The dollar price
  is never converted.

## Asset names

Names must match the provider's English names (`name_en`). Case and surrounding
whitespace do not matter, e.g. `us dollar`, `US Dollar`, and `  us dollar  ` are
the same request.

A few names verified against the live provider, with their provider symbols
and the quantity the price refers to. Every gold and currency item is priced
in Toman except `Gold Ounce`, which is priced in US dollars:

| Argument             | Kind     | Provider symbol   | Quantity basis       | Notes                                     |
| -------------------- | -------- | ----------------- | -------------------- | ----------------------------------------- |
| `"us dollar"`        | currency | `USD`             | units of the currency |                                          |
| `euro`               | currency | `EUR`             | units of the currency |                                          |
| `"uae dirham"`       | currency | `AED`             | units of the currency |                                          |
| `"tether dollar"`    | currency | `USDT_IRT`        | units of the token    | quoted in Toman                           |
| `"18K Gold"`         | gold     | `IR_GOLD_18K`     | grams                 |                                           |
| `"24K Gold"`         | gold     | `IR_GOLD_24K`     | grams                 |                                           |
| `"Melted Gold"`      | gold     | `IR_GOLD_MELTED`  | grams                 | basis by convention, see below            |
| `"1g Coin"`          | gold     | `IR_COIN_1G`      | coins                 |                                           |
| `"Quarter Coin"`     | gold     | `IR_COIN_QUARTER` | coins                 |                                           |
| `"Half Coin"`        | gold     | `IR_COIN_HALF`    | coins                 |                                           |
| `"Emami Coin"`       | gold     | `IR_COIN_EMAMI`   | coins                 |                                           |
| `"Bahar Azadi Coin"` | gold     | `IR_COIN_BAHAR`   | coins                 |                                           |
| `"Gold Ounce"`       | gold     | `XAUUSD`          | troy ounces           | priced in dollars, see below              |
| `bitcoin`            | crypto   | —                 | units of the coin      | crypto rows have no symbol                |
| `ethereum`           | crypto   | —                 | units of the coin      |                                           |
| `solana`             | crypto   | —                 | units of the coin      |                                           |

**About the quantity basis.** The provider's `unit` field carries only the
denomination — Toman or Dollar, as printed on the cards — and nothing about
what quantity the price refers to. The bases above are a reading of the
provider's data and Iranian market convention, not something the API
documents:

- **Gold bullion** — `18K Gold`, `24K Gold`, `Melted Gold` — is quoted **per
  gram**. For two of the bullion rows the data confirms it: in one snapshot
  (1405/07/22), `Gold Ounce` converted at the provider's `USD` rate lands
  within 0.4% of the listed 24K per-gram price, and 18K is within 0.002% of
  exactly 18/24 of 24K — the ratios expected if both are gram prices. For
  `Melted Gold`, per gram is the market convention for gold sold by weight
  («طلای آب‌شده نقدی»); its price data alone does not confirm the basis.
- **Coins** — `1g Coin`, `Quarter Coin`, `Half Coin`, `Emami Coin`,
  `Bahar Azadi Coin` — are quoted **per coin**, so `=amount` is a number of
  coins.
- **`Gold Ounce`** is quoted per troy ounce, in dollars (see the output
  notes).
- **Currencies** are quoted per **unit** of the currency, so `"us dollar"=20`
  means twenty US dollars.

**What the cards do not include.** The gold prices the provider returns are
the raw metal price. A retail purchase of gold or coins in the Iranian market
typically adds seller-side charges on top of it:

- اجرت ساخت (manufacturing/making charge), a percentage;
- سود فروشنده (seller's profit), a percentage;
- مالیات بر ارزش افزوده (value-added tax), applied to the making charge and
  the seller's profit rather than to the raw metal value.

`Your Amount` reports `amount × price` and nothing else, so where these
charges apply, the card's figure is lower than what a shop would charge. They
are seller-side terms added on top of the feed price; the program has no
fields for them, and the feed does not say which items carry them.

This table is a convenience list, **not a catalog**. The gold and currency data
covers a few dozen items and the cryptocurrency endpoint covers roughly 3,000,
and any of those names works. To find others, look up the asset on
<https://brsapi.ir/> and use its English name. Nothing in the repository
contains a local copy of the name list, so a name that the provider renames will
stop resolving — the program reports it as not found.

## Tests and local verification

```bash
go test ./...                # unit tests (no network, no API key needed)
go test -race ./...          # same tests under the race detector
go vet ./...                 # static checks
gofmt -l .                   # formatting (prints nothing when clean)
go build ./...               # compile check
go mod verify                # module cache integrity
go mod tidy -diff            # module file drift (prints nothing when clean)

# For extra confidence:
go test -count=1 -shuffle=on ./...
go test -count=10 ./...
go test -count=1 -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

The tests do not call the real API and do not require `API_KEY`. They use
`httptest` servers for HTTP behavior and fakes for the rest, so they run offline.

Coverage is a diagnostic, not a target: the suite is behavior-focused, and the
percentage is not something the project is trying to raise.

## Design notes

A few behaviors are deliberate and worth knowing before reading the code:

- **Partial results are kept.** Gold/currency and cryptocurrency come from two
  separate endpoints. If one fails, the assets resolved from the other are still
  printed, and the failure is reported as a warning. The process exits non-zero
  only when *nothing* could be resolved.
- **`Missing` versus `Unverified`.** A requested name is `Missing` only when
  every relevant source was queried successfully and none of them contained it.
  If a source failed, the name is `Unverified` instead: "could not check". A name
  is never reported as both.
- **An unavailable price is not a zero price.** A cryptocurrency is counted as
  found even when its Toman price cannot be used, and the card shows
  `Price (Toman) unavailable` rather than discarding the asset or treating the
  price as zero. Only a positive, finite number is accepted as a usable price;
  the provider returns `0` for some assets whose USD price is non-zero, which
  looks like missing or unconverted data rather than a real zero valuation.
- **The API key is kept out of error text.** Request failures are reported by
  host name only, because the underlying `url.Error` would otherwise include the
  full request URL, which carries the key.
- **The whole lookup shares one deadline.** All requests for a single run share
  a ten-second timeout.

## Limitations

- **One provider.** Both endpoints belong to BrsApi, whose rate limits, names,
  and response shapes are outside this project's control. If any of them
  changes, the tool breaks; there is no fallback provider. At the time of
  writing the provider was not returning prices for any asset type, so the
  cards above are records of earlier runs and a fresh run may produce no usable
  output.
- **No cache and no offline mode.** Every run hits the network and prints a
  fresh snapshot; nothing is stored between runs.
- **Prices in non-Toman units.** `Gold Ounce` is priced in dollars and is not
  valued in Toman — a deliberate scope boundary; see the output notes.
- **Not a data source for decisions.** The output is a display of provider
  data, not a valuation or advice.
