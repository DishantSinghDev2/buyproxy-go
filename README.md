# BuyProxy Go SDK — Residential Proxy API Client for Golang

[![Go Reference](https://pkg.go.dev/badge/github.com/DishantSinghDev2/buyproxy-go.svg)](https://pkg.go.dev/github.com/DishantSinghDev2/buyproxy-go)
[![CI](https://github.com/DishantSinghDev2/buyproxy-go/actions/workflows/ci.yml/badge.svg)](https://github.com/DishantSinghDev2/buyproxy-go/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/DishantSinghDev2/buyproxy-go)](https://goreportcard.com/report/github.com/DishantSinghDev2/buyproxy-go)
[![license](https://img.shields.io/badge/license-MIT-green)](LICENSE)

The official **Go SDK** for [BuyProxy](https://www.buyproxy.org): generate **residential, mobile, ISP and datacenter proxies**, check bandwidth, top up by card or crypto, and manage **reseller** customers. Idiomatic Go — `context` on every call, `errors.Is` sentinels, and **zero third-party dependencies**.

**Keywords:** residential proxies · rotating proxies · mobile proxies · ISP proxies · datacenter proxies · SOCKS5 · proxy API · web scraping · proxy reseller

## Features

- Standard library only — no transitive dependencies
- `context.Context` on every call; safe for concurrent use
- `errors.Is(err, buyproxy.ErrInsufficientBandwidth)` and friends
- `Proxy.HTTPClient()` returns an `*http.Client` already routed through the proxy

## Installation

```bash
go get github.com/DishantSinghDev2/buyproxy-go
```

Go 1.21+. Standard library only.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"io"
	"log"

	buyproxy "github.com/DishantSinghDev2/buyproxy-go"
)

func main() {
	ctx := context.Background()
	client := buyproxy.New() // reads BUYPROXY_API_KEY — or buyproxy.WithAPIKey("bp_key_…")

	plans, err := client.Plans.List(ctx)
	if err != nil || len(plans) == 0 {
		log.Fatal("no plans yet — top up first: ", err)
	}

	res, err := client.Plans.Generate(ctx, plans[0].ID, &buyproxy.GenerateParams{Country: "US", City: "Austin", Count: 5})
	if err != nil {
		log.Fatal(err)
	}

	resp, err := res.Proxies[0].HTTPClient().Get("https://api.ipify.org")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	ip, _ := io.ReadAll(resp.Body)
	fmt.Println("exit IP:", string(ip))
}
```

Set `StickyMinutes: 10` to keep the same IP for ten minutes, or `Protocol: buyproxy.SOCKS5` for SOCKS5.

## Using the proxies with your stack

### net/http

```go
proxy := res.Proxies[0]

client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy.URL())}}
// or simply: client := proxy.HTTPClient()
```

### Colly

```go
c := colly.NewCollector()
c.SetProxyFunc(http.ProxyURL(res.Proxies[0].URL()))
```

### chromedp / Rod

```go
opts := append(chromedp.DefaultExecAllocatorOptions[:],
	chromedp.ProxyServer(res.Proxies[0].URL().Host)) // add auth via a fetch.AuthRequired handler
```

### curl & shell

```go
fmt.Println(res.Proxies[0].Curl())  // ready-to-run curl command
fmt.Println(res.Proxies[0].Env())   // HTTP_PROXY, HTTPS_PROXY, ALL_PROXY, …
```

## Buy bandwidth (card & crypto)

```go
// Buy 10 GB of Core Residential by card — open PayURL in a browser
checkout, _ := client.Topup.Card(ctx, buyproxy.TopupCardParams(buyproxy.ProductResidential, 10))
fmt.Println(checkout.PayURL)

// Or subscribe to a plan (monthly or yearly)
client.Topup.Card(ctx, buyproxy.PlanCardParams(buyproxy.PlanPro, buyproxy.Yearly))

// Crypto (pay-as-you-go top-ups only)
client.Topup.Crypto(ctx, buyproxy.ProductResidential, 50)

// Wait for the payment to settle — GB appears on client.Balances.List right after
tx, _ := client.Transactions.Wait(ctx, *checkout.TransactionID, 3*time.Second)
fmt.Println(tx.Status) // "succeeded"
```

Card payments run through Stripe Checkout; crypto is available for pay-as-you-go top-ups.

## Reseller API

Build a proxy product on top of BuyProxy: buy bandwidth in bulk into a **reseller pool**, create isolated **customers**, and allocate GB to them. Needs a `bp_live_` key on a Business plan or higher.

```go
reseller := buyproxy.New(buyproxy.WithAPIKey("bp_live_…"))

reseller.Reseller.Pool.TopupCard(ctx, buyproxy.PoolTopupParams{ProductKey: buyproxy.ProductResidential, GB: 500})
customer, _ := reseller.Reseller.Customers.Create(ctx, "Acme Inc") // isolated sub-account

reseller.Reseller.Customers.Allocate(ctx, customer.Username, buyproxy.AllocateParams{
	ProductKey: buyproxy.ProductResidential, GB: 20, IdempotencyKey: "order-1042", // safe to retry
})

detail, _ := reseller.Reseller.Customers.Get(ctx, customer.Username)
res, _ := reseller.Reseller.Customers.Generate(ctx, customer.Username, buyproxy.CustomerGenerateParams{
	PlanID: detail.Plans[0].ID, GenerateParams: buyproxy.GenerateParams{Country: "DE"},
})

reseller.Reseller.Customers.Update(ctx, customer.Username, buyproxy.CustomerUpdate{Status: buyproxy.CustomerSuspended})
```

## Error handling

```go
_, err := client.Plans.Generate(ctx, planID, nil)

switch {
case errors.Is(err, buyproxy.ErrInsufficientBandwidth):
	log.Println("Out of GB — top up first")
case errors.Is(err, buyproxy.ErrRateLimited):
	var apiErr *buyproxy.APIError
	errors.As(err, &apiErr)
	log.Printf("Slow down; retry in %s", apiErr.RetryAfter)
case errors.Is(err, buyproxy.ErrAuthentication):
	log.Println("Check your API key")
}
```
Non-2xx responses are `*buyproxy.APIError` (with `Status`, `Message`, `Body`); network failures are `*buyproxy.ConnectionError`.

## Configuration

```go
client := buyproxy.New(
	buyproxy.WithAPIKey("bp_key_…"),            // default: $BUYPROXY_API_KEY
	buyproxy.WithBaseURL("https://www.buyproxy.org"),
	buyproxy.WithHTTPClient(&http.Client{Timeout: 10 * time.Second}),
	buyproxy.WithMaxRetries(2),                  // 429s always; 5xx only for safe requests
)
```

## API overview

| Capability | Call |
|---|---|
| Catalog & prices | `client.Products.List(ctx)` |
| Account | `client.Account.Get(ctx)` |
| Spendable GB per product | `client.Balances.List(ctx)` |
| Plans | `client.Plans.List(ctx)` |
| Generate proxies | `client.Plans.Generate(ctx, planID, params)` |
| Card checkout | `client.Topup.Card(ctx, params)` |
| Crypto checkout | `client.Topup.Crypto(ctx, product, gb)` |
| Payments | `client.Transactions.List/Get/Wait` |
| Create an account | `client.Auth.Signup/VerifyOTP/ResendOTP` |
| Reseller summary | `client.Reseller.Me(ctx)` |
| Customers | `client.Reseller.Customers.List/Create/Get/Update/Usage/Allocate/Generate` |
| Reseller pool | `client.Reseller.Pool.Get/TopupCard/TopupCrypto` |

Full endpoint reference: <https://www.buyproxy.org/docs/api-reference/introduction> · OpenAPI spec: <https://www.buyproxy.org/openapi.yaml>

## FAQ

**How do I get an API key?**
Create a free account at [buyproxy.org](https://www.buyproxy.org/signup), then open **Dashboard → API keys**. Personal keys start with `bp_key_`; reseller keys (Business plan and above) start with `bp_live_`. You can create the *account* from code (`signup` + `verify otp`), but keys are issued from the dashboard.

**What's the difference between a personal and a reseller key?**
A personal key (`bp_key_…`) manages your own bandwidth: balances, plans, generating proxies, top-ups and payments. A reseller key (`bp_live_…`) manages *your customers*: create isolated sub-accounts, buy bandwidth into a pool, allocate it, suspend or reactivate, and generate proxies on their behalf.

**Which proxy types are available?**
Core Residential, Premium Residential, ISP Rotating, Mobile, Datacenter IPv4 and IPv6 — call the catalog endpoint for the live list and per-GB prices (from **$1.99/GB**).

**Do generated proxies expire, and when is bandwidth used?**
Generating a proxy is free and instant; bandwidth is only consumed when traffic flows through it. Lines are on-demand — generate fresh ones rather than storing them.

**What are the rate limits?**
60 requests per minute per API key. The SDK honours `Retry-After` and retries rate-limited requests automatically.

**Is it free to use the SDK?**
Yes — the SDK is MIT-licensed. You pay only for the proxy bandwidth you buy.

## Other BuyProxy SDKs

[Node.js](https://github.com/DishantSinghDev2/buyproxy-node) · [Python](https://github.com/DishantSinghDev2/buyproxy-python) · [Go](https://github.com/DishantSinghDev2/buyproxy-go) · [PHP](https://github.com/DishantSinghDev2/buyproxy-php) · [Ruby](https://github.com/DishantSinghDev2/buyproxy-ruby) · [Java](https://github.com/DishantSinghDev2/buyproxy-java) · [.NET](https://github.com/DishantSinghDev2/buyproxy-dotnet) · [Rust](https://github.com/DishantSinghDev2/buyproxy-rust) · [CLI](https://github.com/DishantSinghDev2/buyproxy-cli)

All SDKs are checked against the same request log, so behaviour is identical across languages — see [buyproxy-sdks](https://github.com/DishantSinghDev2/buyproxy-sdks).

## Development

```bash
go vet ./... && go test -race ./...
```

Issues and pull requests are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md). Security reports: [SECURITY.md](SECURITY.md).

## Links

- Website: <https://www.buyproxy.org>
- Documentation: <https://www.buyproxy.org/docs/sdks>
- Pricing: <https://www.buyproxy.org/pricing>
- Support: <support@buyproxy.org>

## License

[MIT](LICENSE) © BuyProxy
