package buyproxy

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"time"
)

func seg(s string) string { return url.PathEscape(s) }

// --- Public ---------------------------------------------------------------

// ProductsService is the public catalog.
type ProductsService struct{ c *Client }

// List returns every product with its key, minimum GB and live price. No API key needed.
func (s *ProductsService) List(ctx context.Context) (*Catalog, error) {
	var out Catalog
	err := s.c.do(ctx, request{method: "GET", path: "/v1/products", public: true, retry5xx: true}, &out)
	return &out, err
}

// AuthService creates an account without a browser: Signup, then VerifyOTP with the emailed code.
type AuthService struct{ c *Client }

func (s *AuthService) Signup(ctx context.Context, p SignupParams) (*SignupResult, error) {
	var out SignupResult
	err := s.c.do(ctx, request{method: "POST", path: "/api/auth/signup", body: p, public: true}, &out)
	return &out, err
}

func (s *AuthService) VerifyOTP(ctx context.Context, email, code string) (*VerifyResult, error) {
	var out VerifyResult
	err := s.c.do(ctx, request{method: "POST", path: "/api/auth/verify-otp", body: map[string]string{"email": email, "code": code}, public: true}, &out)
	return &out, err
}

func (s *AuthService) ResendOTP(ctx context.Context, email string) error {
	return s.c.do(ctx, request{method: "POST", path: "/api/auth/resend-otp", body: map[string]string{"email": email}, public: true}, nil)
}

// --- Personal API -----------------------------------------------------------

type AccountService struct{ c *Client }

func (s *AccountService) Get(ctx context.Context) (*Account, error) {
	var out Account
	err := s.c.do(ctx, request{method: "GET", path: "/v1/account", retry5xx: true}, &out)
	return &out, err
}

type BalancesService struct{ c *Client }

// List returns spendable GB per product, including GB still being added after a payment.
func (s *BalancesService) List(ctx context.Context) ([]Balance, error) {
	var out struct {
		Balances []Balance `json:"balances"`
	}
	err := s.c.do(ctx, request{method: "GET", path: "/v1/balances", retry5xx: true}, &out)
	return out.Balances, err
}

type PlansService struct{ c *Client }

func (s *PlansService) List(ctx context.Context) ([]Plan, error) {
	var out struct {
		Plans []Plan `json:"plans"`
	}
	err := s.c.do(ctx, request{method: "GET", path: "/v1/plans", retry5xx: true}, &out)
	return out.Plans, err
}

// Generate creates proxy lines against one of your plans. Lines are on-demand — don't store them.
func (s *PlansService) Generate(ctx context.Context, planID string, p *GenerateParams) (*GeneratedProxies, error) {
	if p == nil {
		p = &GenerateParams{}
	}
	var out struct {
		Lines []string `json:"lines"`
	}
	if err := s.c.do(ctx, request{method: "POST", path: "/v1/plans/" + seg(planID) + "/generate", body: p}, &out); err != nil {
		return nil, err
	}
	return newGenerated(out.Lines, p.Protocol)
}

type TopupService struct{ c *Client }

// Card starts a Stripe checkout — a top-up or a plan subscription. Open PayURL to pay.
func (s *TopupService) Card(ctx context.Context, p CardCheckoutParams) (*CardCheckout, error) {
	var out CardCheckout
	err := s.c.do(ctx, request{method: "POST", path: "/v1/topup/card", body: p}, &out)
	return &out, err
}

// Crypto starts a crypto top-up (pay-as-you-go only). Open URL to pay.
func (s *TopupService) Crypto(ctx context.Context, product ProductKey, gb int) (*CryptoCheckout, error) {
	body := map[string]any{"kind": "topup", "productKey": product, "gb": gb}
	var out CryptoCheckout
	err := s.c.do(ctx, request{method: "POST", path: "/v1/topup/crypto", body: body}, &out)
	return &out, err
}

type TransactionsService struct{ c *Client }

// List returns recent payments. limit <= 0 uses the API default.
func (s *TransactionsService) List(ctx context.Context, limit int) ([]Transaction, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var out struct {
		Transactions []Transaction `json:"transactions"`
	}
	err := s.c.do(ctx, request{method: "GET", path: "/v1/transactions", query: q, retry5xx: true}, &out)
	return out.Transactions, err
}

func (s *TransactionsService) Get(ctx context.Context, id string) (*Transaction, error) {
	var out Transaction
	err := s.c.do(ctx, request{method: "GET", path: "/v1/transactions/" + seg(id), retry5xx: true}, &out)
	return &out, err
}

// Wait polls until the payment settles (succeeded or failed) or ctx ends.
func (s *TransactionsService) Wait(ctx context.Context, id string, interval time.Duration) (*Transaction, error) {
	if interval <= 0 {
		interval = 3 * time.Second
	}
	for {
		tx, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if tx.Status != "pending" {
			return tx, nil
		}
		if err := sleepCtx(ctx, interval); err != nil {
			return nil, errors.New("buyproxy: payment " + id + " is still pending: " + err.Error())
		}
	}
}

// --- Reseller API (bp_live_ keys, Business plan or higher) -----------------

type ResellerService struct {
	c         *Client
	Customers *CustomersService
	Pool      *PoolService
}

func (s *ResellerService) Me(ctx context.Context) (*ResellerSummary, error) {
	var out ResellerSummary
	err := s.c.do(ctx, request{method: "GET", path: "/v1/me", retry5xx: true}, &out)
	return &out, err
}

type CustomersService struct{ c *Client }

// List returns your customers; pass a status ("active"/"suspended") to filter, or "" for all.
func (s *CustomersService) List(ctx context.Context, status CustomerStatus) ([]Customer, error) {
	q := url.Values{}
	if status != "" {
		q.Set("status", string(status))
	}
	var out struct {
		Customers []Customer `json:"customers"`
	}
	err := s.c.do(ctx, request{method: "GET", path: "/v1/customers", query: q, retry5xx: true}, &out)
	return out.Customers, err
}

// Create makes an isolated sub-account with no bandwidth — fund it with Allocate.
func (s *CustomersService) Create(ctx context.Context, label string) (*Customer, error) {
	var out Customer
	err := s.c.do(ctx, request{method: "POST", path: "/v1/customers", body: map[string]string{"label": label}}, &out)
	return &out, err
}

func (s *CustomersService) Get(ctx context.Context, username string) (*CustomerDetail, error) {
	var out CustomerDetail
	err := s.c.do(ctx, request{method: "GET", path: "/v1/customers/" + seg(username), retry5xx: true}, &out)
	return &out, err
}

// CustomerUpdate renames and/or suspends/reactivates; empty fields are left unchanged.
type CustomerUpdate struct {
	Label  string         `json:"label,omitempty"`
	Status CustomerStatus `json:"status,omitempty"`
}

func (s *CustomersService) Update(ctx context.Context, username string, u CustomerUpdate) (*Customer, error) {
	var out Customer
	err := s.c.do(ctx, request{method: "PATCH", path: "/v1/customers/" + seg(username), body: u}, &out)
	return &out, err
}

func (s *CustomersService) Usage(ctx context.Context, username string) (*CustomerUsage, error) {
	var out CustomerUsage
	err := s.c.do(ctx, request{method: "GET", path: "/v1/customers/" + seg(username) + "/usage", retry5xx: true}, &out)
	return &out, err
}

// Allocate moves GB from your pool onto a customer. Set IdempotencyKey to make retries safe.
func (s *CustomersService) Allocate(ctx context.Context, username string, p AllocateParams) (*Allocation, error) {
	r := request{method: "POST", path: "/v1/customers/" + seg(username) + "/allocate", body: p}
	if p.IdempotencyKey != "" {
		r.headers = map[string]string{"Idempotency-Key": p.IdempotencyKey}
		r.retry5xx = true
	}
	var out Allocation
	err := s.c.do(ctx, r, &out)
	return &out, err
}

func (s *CustomersService) Generate(ctx context.Context, username string, p CustomerGenerateParams) (*GeneratedProxies, error) {
	var out struct {
		Lines []string `json:"lines"`
	}
	if err := s.c.do(ctx, request{method: "POST", path: "/v1/customers/" + seg(username) + "/generate", body: p}, &out); err != nil {
		return nil, err
	}
	return newGenerated(out.Lines, p.Protocol)
}

type PoolService struct{ c *Client }

// Get returns your unallocated bandwidth, per product.
func (s *PoolService) Get(ctx context.Context) ([]PoolEntry, error) {
	var out struct {
		Pool []PoolEntry `json:"pool"`
	}
	err := s.c.do(ctx, request{method: "GET", path: "/v1/pool", retry5xx: true}, &out)
	return out.Pool, err
}

func (s *PoolService) TopupCard(ctx context.Context, p PoolTopupParams) (*CardCheckout, error) {
	var out CardCheckout
	err := s.c.do(ctx, request{method: "POST", path: "/v1/pool/topup/card", body: p}, &out)
	return &out, err
}

func (s *PoolService) TopupCrypto(ctx context.Context, p PoolTopupParams) (*CryptoCheckout, error) {
	var out CryptoCheckout
	err := s.c.do(ctx, request{method: "POST", path: "/v1/pool/topup/crypto", body: p}, &out)
	return &out, err
}
