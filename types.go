package buyproxy

// ProductKey is accepted by every top-up and allocation call — see Products.List.
type ProductKey string

const (
	ProductResidential ProductKey = "residential"
	ProductPremium     ProductKey = "premium"
	ProductISP         ProductKey = "isp"
	ProductMobile      ProductKey = "mobile"
	ProductIPv4        ProductKey = "ipv4"
	ProductIPv6        ProductKey = "ipv6"
)

// PaidPlan is a plan you can buy by card.
type PaidPlan string

const (
	PlanPro      PaidPlan = "pro"
	PlanBusiness PaidPlan = "business"
	PlanAdvanced PaidPlan = "advanced"
)

// BillingInterval is "month" or "year".
type BillingInterval string

const (
	Monthly BillingInterval = "month"
	Yearly  BillingInterval = "year"
)

// CustomerStatus is "active" or "suspended".
type CustomerStatus string

const (
	CustomerActive    CustomerStatus = "active"
	CustomerSuspended CustomerStatus = "suspended"
)

// Product is one item in the catalog.
type Product struct {
	ProductKey      ProductKey `json:"productKey"`
	ProductID       string     `json:"productId"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	MinGB           int        `json:"minGb"`
	PricePerGBCents *int       `json:"pricePerGbCents"` // nil if pricing is momentarily unavailable
	Currency        string     `json:"currency"`
}

// Catalog is the public product catalog with live prices.
type Catalog struct {
	Products    []Product `json:"products"`
	VolumeBonus struct {
		ThresholdGB int     `json:"thresholdGb"`
		Rate        float64 `json:"rate"`
	} `json:"volumeBonus"`
	Plans []struct {
		Plan              PaidPlan `json:"plan"`
		MonthlyPriceCents int      `json:"monthlyPriceCents"`
		IncludedGB        float64  `json:"includedGb"`
	} `json:"plans"`
}

// Account is the account behind the API key.
type Account struct {
	Email         string  `json:"email"`
	Name          string  `json:"name"`
	Plan          string  `json:"plan"`
	PlanExpiresAt *string `json:"planExpiresAt"`
	// BalanceGB is the lifetime GB credited. Spendable per-product balances come from Balances.List.
	BalanceGB float64 `json:"balanceGb"`
	CreatedAt string  `json:"createdAt"`
}

// Balance is the spendable bandwidth on one product.
type Balance struct {
	ProductKey *ProductKey `json:"productKey"`
	ProductID  string      `json:"productId"`
	// PlanID is what to pass to Plans.Generate; nil until the product has been funded once.
	PlanID    *string `json:"planId"`
	ActiveGB  float64 `json:"activeGb"`
	PendingGB float64 `json:"pendingGb"` // paid for but still being added
	TotalGB   float64 `json:"totalGb"`
	ExpiresAt *string `json:"expiresAt"`
}

// GenerateCapabilities describes what a plan's generate call accepts.
type GenerateCapabilities struct {
	MaxCount         int        `json:"maxCount"`
	MaxStickyMinutes int        `json:"maxStickyMinutes"` // 0 means no sticky sessions on this product
	Protocols        []Protocol `json:"protocols"`
	Targeting        struct {
		Country bool `json:"country"`
		State   bool `json:"state"`
		City    bool `json:"city"`
		ISP     bool `json:"isp"`
	} `json:"targeting"`
}

// Plan is bandwidth you hold on one product.
type Plan struct {
	ID              string                `json:"id"`
	ProductID       string                `json:"productId"`
	ProductKey      *ProductKey           `json:"productKey"`
	Name            string                `json:"name"`
	Label           *string               `json:"label"`
	DataGBRemaining float64               `json:"dataGbRemaining"`
	DataGBTotal     float64               `json:"dataGbTotal"`
	ExpiresAt       *string               `json:"expiresAt"`
	CreatedAt       string                `json:"createdAt"`
	Generate        *GenerateCapabilities `json:"generate"`
}

// GenerateParams controls proxy generation. Zero values are omitted, so the API's defaults apply.
type GenerateParams struct {
	Protocol Protocol `json:"protocol,omitempty"` // default "http"
	Count    int      `json:"count,omitempty"`    // 1–1000, default 1
	Country  string   `json:"country,omitempty"`  // ISO 3166-1 alpha-2, e.g. "US"
	State    string   `json:"state,omitempty"`
	City     string   `json:"city,omitempty"`
	ISP      string   `json:"isp,omitempty"`
	// StickyMinutes keeps the same IP for that long. Leave 0 for a fresh IP per request (rotating).
	StickyMinutes int `json:"stickyMinutes,omitempty"`
}

// CardCheckoutParams starts a card checkout: either a top-up (Kind "topup", ProductKey, GB) or a
// subscription (Kind "plan", Plan, Interval). Use TopupCardParams / PlanCardParams to build one.
type CardCheckoutParams struct {
	Kind       string          `json:"kind"`
	ProductKey ProductKey      `json:"productKey,omitempty"`
	GB         int             `json:"gb,omitempty"`
	Plan       PaidPlan        `json:"plan,omitempty"`
	Interval   BillingInterval `json:"interval,omitempty"`
}

// TopupCardParams builds the params for a pay-as-you-go card top-up.
func TopupCardParams(product ProductKey, gb int) CardCheckoutParams {
	return CardCheckoutParams{Kind: "topup", ProductKey: product, GB: gb}
}

// PlanCardParams builds the params for a plan subscription. An empty interval means monthly.
func PlanCardParams(plan PaidPlan, interval BillingInterval) CardCheckoutParams {
	return CardCheckoutParams{Kind: "plan", Plan: plan, Interval: interval}
}

// CardCheckout is a Stripe checkout to open in a browser.
type CardCheckout struct {
	PayURL string `json:"payUrl"`
	// TransactionID is nil when PayURL is the subscription portal rather than a new payment.
	TransactionID *string `json:"transactionId"`
}

// CryptoCheckout is a crypto invoice to open in a browser.
type CryptoCheckout struct {
	URL           string `json:"url"`
	TransactionID string `json:"transactionId"`
}

// Transaction is one payment.
type Transaction struct {
	ID            string           `json:"id"`
	Kind          string           `json:"kind"`
	Status        string           `json:"status"` // pending | succeeded | failed
	PaymentMethod string           `json:"paymentMethod"`
	Interval      *BillingInterval `json:"interval"`
	AmountCents   int              `json:"amountCents"`
	Currency      string           `json:"currency"`
	GB            *float64         `json:"gb"` // including any volume bonus
	ProductKey    *ProductKey      `json:"productKey"`
	Plan          *string          `json:"plan"`
	ResellerPool  bool             `json:"resellerPool"`
	CreatedAt     string           `json:"createdAt"`
}

// PoolEntry is unallocated reseller bandwidth on one product.
type PoolEntry struct {
	ProductKey ProductKey `json:"productKey"`
	ProductID  string     `json:"productId"`
	Label      string     `json:"label"`
	GB         float64    `json:"gb"`
}

// ResellerSummary is the result of Reseller.Me.
type ResellerSummary struct {
	Email               string      `json:"email"`
	CustomerCount       int         `json:"customerCount"`
	ActiveCustomerCount int         `json:"activeCustomerCount"`
	Pool                []PoolEntry `json:"pool"`
}

// Customer is a reseller sub-account.
type Customer struct {
	Username  string         `json:"username"`
	Label     string         `json:"label"`
	Status    CustomerStatus `json:"status"`
	CreatedAt string         `json:"createdAt"`
}

// PendingAllocation is GB allocated but still being added to a plan.
type PendingAllocation struct {
	ProductID  string      `json:"productId"`
	ProductKey *ProductKey `json:"productKey"`
	GB         float64     `json:"gb"`
}

// CustomerPlan is a Plan plus the customer's remaining GB.
type CustomerPlan struct {
	Plan
	RemainingGB float64 `json:"remainingGb"`
}

// CustomerDetail is a customer with their plans.
type CustomerDetail struct {
	Customer
	Plans   []CustomerPlan      `json:"plans"`
	Pending []PendingAllocation `json:"pending"`
}

// CustomerUsage is a snapshot of a customer's consumption.
type CustomerUsage struct {
	Plans []struct {
		ID          string      `json:"id"`
		ProductID   string      `json:"productId"`
		ProductKey  *ProductKey `json:"productKey"`
		RemainingGB float64     `json:"remainingGb"`
		TotalGB     float64     `json:"totalGb"`
		UsedGB      float64     `json:"usedGb"`
		ExpiresAt   *string     `json:"expiresAt"`
	} `json:"plans"`
	Pending []PendingAllocation `json:"pending"`
}

// AllocateParams moves GB from your pool onto a customer.
type AllocateParams struct {
	ProductKey ProductKey `json:"productKey"`
	GB         int        `json:"gb"` // whole GB, at least the product's minimum
	// IdempotencyKey makes retries safe: repeating a key returns AlreadyApplied and moves nothing.
	IdempotencyKey string `json:"-"`
}

// Allocation is the result of an allocation.
type Allocation struct {
	ProductKey      ProductKey `json:"productKey"`
	ProductID       string     `json:"productId"`
	GB              int        `json:"gb"`
	AlreadyApplied  bool       `json:"alreadyApplied"`
	PoolRemainingGB float64    `json:"poolRemainingGb"`
}

// CustomerGenerateParams is GenerateParams plus the customer's plan.
type CustomerGenerateParams struct {
	PlanID string `json:"planId"`
	GenerateParams
}

// PoolTopupParams buys bandwidth into the reseller pool.
type PoolTopupParams struct {
	ProductKey ProductKey `json:"productKey"`
	GB         int        `json:"gb"`
}

// SignupParams creates an account (no API key needed).
type SignupParams struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name,omitempty"`
}

// SignupResult is the result of Auth.Signup.
type SignupResult struct {
	OK              bool `json:"ok"`
	CooldownSeconds int  `json:"cooldownSeconds"`
}

// VerifyResult is the result of Auth.VerifyOTP.
type VerifyResult struct {
	OK    bool `json:"ok"`
	IsNew bool `json:"isNew"`
	User  struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	} `json:"user"`
}
