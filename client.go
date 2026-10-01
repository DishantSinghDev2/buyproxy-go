// Package buyproxy is the official Go SDK for BuyProxy — residential, mobile, ISP and
// datacenter proxies over a typed API.
//
//	client := buyproxy.New() // reads BUYPROXY_API_KEY
//	plans, _ := client.Plans.List(ctx)
//	res, _ := client.Plans.Generate(ctx, plans[0].ID, &buyproxy.GenerateParams{Country: "US", Count: 5})
//	resp, _ := res.Proxies[0].HTTPClient().Get("https://api.ipify.org")
package buyproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Version is the SDK version, sent in the User-Agent.
const Version = "1.0.0"

// DefaultBaseURL is the production API.
const DefaultBaseURL = "https://www.buyproxy.org"

// Client talks to the BuyProxy API. It is safe for concurrent use.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	maxRetries int

	Products     *ProductsService
	Auth         *AuthService
	Account      *AccountService
	Balances     *BalancesService
	Plans        *PlansService
	Topup        *TopupService
	Transactions *TransactionsService
	Reseller     *ResellerService
}

// Option configures a Client.
type Option func(*Client)

// WithAPIKey sets the key (bp_key_… personal, bp_live_… reseller). Default: $BUYPROXY_API_KEY.
func WithAPIKey(key string) Option { return func(c *Client) { c.apiKey = key } }

// WithBaseURL overrides the API origin (default https://www.buyproxy.org).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }

// WithHTTPClient supplies your own *http.Client (timeouts, tracing, proxies).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }

// WithMaxRetries sets retries for rate limits and transient failures (default 2).
func WithMaxRetries(n int) Option { return func(c *Client) { c.maxRetries = n } }

// New creates a Client.
func New(opts ...Option) *Client {
	c := &Client{
		apiKey:     os.Getenv("BUYPROXY_API_KEY"),
		baseURL:    DefaultBaseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		maxRetries: 2,
	}
	for _, opt := range opts {
		opt(c)
	}
	c.Products = &ProductsService{c}
	c.Auth = &AuthService{c}
	c.Account = &AccountService{c}
	c.Balances = &BalancesService{c}
	c.Plans = &PlansService{c}
	c.Topup = &TopupService{c}
	c.Transactions = &TransactionsService{c}
	c.Reseller = &ResellerService{c: c, Customers: &CustomersService{c}, Pool: &PoolService{c}}
	return c
}

// KeyKind returns "reseller" for bp_live_ keys, "personal" for bp_key_ keys, and "" with no key.
func (c *Client) KeyKind() string {
	switch {
	case c.apiKey == "":
		return ""
	case strings.HasPrefix(c.apiKey, "bp_live_"):
		return "reseller"
	default:
		return "personal"
	}
}

type request struct {
	method   string
	path     string
	query    url.Values
	body     any
	headers  map[string]string
	public   bool // works without an API key
	retry5xx bool // safe to retry on 5xx
}

func (c *Client) do(ctx context.Context, r request, out any) error {
	if !r.public && c.apiKey == "" {
		return &APIError{Status: 401, Message: "no API key — use WithAPIKey or set BUYPROXY_API_KEY (create one at https://www.buyproxy.org/dashboard/api-keys)"}
	}

	var payload []byte
	if r.body != nil {
		var err error
		if payload, err = json.Marshal(r.body); err != nil {
			return err
		}
	}
	target := c.baseURL + r.path
	if len(r.query) > 0 {
		target += "?" + r.query.Encode()
	}

	for attempt := 0; ; attempt++ {
		var body io.Reader
		if payload != nil {
			body = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, r.method, target, body)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "buyproxy-go/"+Version)
		if c.apiKey != "" {
			req.Header.Set("X-API-Key", c.apiKey)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range r.headers {
			req.Header.Set(k, v)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if r.retry5xx && attempt < c.maxRetries {
				if sleepCtx(ctx, backoff(attempt)) != nil {
					return ctx.Err()
				}
				continue
			}
			return &ConnectionError{Err: err}
		}
		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return &ConnectionError{Err: readErr}
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if out == nil || len(data) == 0 {
				return nil
			}
			if err := json.Unmarshal(data, out); err != nil {
				return errors.New("buyproxy: unexpected response: " + err.Error())
			}
			return nil
		}

		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		wait := time.Duration(-1)
		switch {
		case attempt >= c.maxRetries:
		case resp.StatusCode == 429:
			wait = backoff(attempt)
			if retryAfter >= 0 {
				wait = retryAfter
			}
		case r.retry5xx && (resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504):
			wait = backoff(attempt)
		}
		if wait >= 0 {
			if sleepCtx(ctx, wait) != nil {
				return ctx.Err()
			}
			continue
		}
		return newAPIError(resp.StatusCode, data, retryAfter)
	}
}

func newAPIError(status int, data []byte, retryAfter time.Duration) *APIError {
	msg := "BuyProxy API error"
	var parsed struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &parsed) == nil && parsed.Error != "" {
		msg = parsed.Error
	}
	e := &APIError{Status: status, Message: msg, Body: data}
	if retryAfter > 0 {
		e.RetryAfter = retryAfter
	}
	return e
}

func parseRetryAfter(h string) time.Duration {
	secs, err := strconv.ParseFloat(h, 64)
	if err != nil || secs < 0 {
		return -1
	}
	return time.Duration(math.Min(secs, 30) * float64(time.Second))
}

func backoff(attempt int) time.Duration {
	base := math.Min(500*math.Pow(2, float64(attempt)), 8000)
	return time.Duration(base*(0.75+rand.Float64()*0.5)) * time.Millisecond
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
