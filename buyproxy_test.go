package buyproxy_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	buyproxy "github.com/DishantSinghDev2/buyproxy-go"
)

func newClient(t *testing.T, h http.HandlerFunc, opts ...buyproxy.Option) *buyproxy.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return buyproxy.New(append([]buyproxy.Option{buyproxy.WithAPIKey("bp_key_x"), buyproxy.WithBaseURL(srv.URL)}, opts...)...)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func TestParseProxy(t *testing.T) {
	p, err := buyproxy.ParseProxy("user-country-us:p@ss:w0rd@gw.buyproxy.org:6969", buyproxy.SOCKS5)
	if err != nil {
		t.Fatal(err)
	}
	if p.Host != "gw.buyproxy.org" || p.Port != 6969 || p.Username != "user-country-us" || p.Password != "p@ss:w0rd" {
		t.Fatalf("parsed wrong: %+v", p)
	}
	if got := p.URL().String(); got != "socks5://user-country-us:p%40ss%3Aw0rd@gw.buyproxy.org:6969" {
		t.Fatalf("url = %s", got)
	}
	if p.String() != "user-country-us:p@ss:w0rd@gw.buyproxy.org:6969" {
		t.Fatalf("string = %s", p.String())
	}
	for _, bad := range []string{"", "nope", "u:p@host", "u:p@host:99999", ":p@host:80", "u@host:80"} {
		if _, err := buyproxy.ParseProxy(bad, buyproxy.HTTP); err == nil {
			t.Errorf("expected an error for %q", bad)
		}
	}
}

func TestGenerateSendsKeyBodyAndParses(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "bp_key_x" || r.URL.Path != "/v1/plans/plan 1/generate" {
			t.Errorf("bad request: %s %s key=%s", r.Method, r.URL.Path, r.Header.Get("X-API-Key"))
		}
		if ua := r.Header.Get("User-Agent"); len(ua) < 12 || ua[:12] != "buyproxy-go/" {
			t.Errorf("user agent = %q", ua)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"protocol":"socks5","count":1}` {
			t.Errorf("body = %s", body)
		}
		writeJSON(w, 200, map[string]any{"lines": []string{"u:p@gw.buyproxy.org:6969"}})
	})
	out, err := c.Plans.Generate(context.Background(), "plan 1", &buyproxy.GenerateParams{Protocol: buyproxy.SOCKS5, Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Proxies) != 1 || out.Proxies[0].Protocol != buyproxy.SOCKS5 {
		t.Fatalf("proxies = %+v", out.Proxies)
	}
}

func TestErrorsMapToSentinels(t *testing.T) {
	for status, want := range map[int]error{401: buyproxy.ErrAuthentication, 402: buyproxy.ErrInsufficientBandwidth, 403: buyproxy.ErrForbidden, 404: buyproxy.ErrNotFound, 400: buyproxy.ErrValidation} {
		c := newClient(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, status, map[string]string{"error": "nope"}) })
		_, err := c.Account.Get(context.Background())
		if !errors.Is(err, want) {
			t.Errorf("status %d: errors.Is(%v, %v) = false", status, err, want)
		}
		var api *buyproxy.APIError
		if !errors.As(err, &api) || api.Message != "nope" || api.Status != status {
			t.Errorf("status %d: APIError = %+v", status, api)
		}
	}
}

func TestRetries429ThenSucceeds(t *testing.T) {
	var calls int32
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			writeJSON(w, 429, map[string]string{"error": "slow"})
			return
		}
		writeJSON(w, 200, map[string]any{"plan": "pro"})
	})
	acc, err := c.Account.Get(context.Background())
	if err != nil || acc.Plan != "pro" || calls != 2 {
		t.Fatalf("acc=%+v err=%v calls=%d", acc, err, calls)
	}
}

func TestRateLimitedAfterRetries(t *testing.T) {
	var calls int32
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Retry-After", "0")
		writeJSON(w, 429, map[string]string{"error": "slow"})
	})
	_, err := c.Account.Get(context.Background())
	if !errors.Is(err, buyproxy.ErrRateLimited) || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestPostNotRetriedOn503UnlessIdempotent(t *testing.T) {
	var calls int32
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if r.Header.Get("Idempotency-Key") == "k1" && n >= 2 {
			writeJSON(w, 201, map[string]any{"gb": 1})
			return
		}
		writeJSON(w, 503, map[string]string{"error": "down"})
	}, buyproxy.WithMaxRetries(2))

	_, err := c.Reseller.Customers.Allocate(context.Background(), "c", buyproxy.AllocateParams{ProductKey: buyproxy.ProductPremium, GB: 1})
	if !errors.Is(err, buyproxy.ErrServer) || calls != 1 {
		t.Fatalf("plain: err=%v calls=%d", err, calls)
	}
	atomic.StoreInt32(&calls, 0)
	if _, err := c.Reseller.Customers.Allocate(context.Background(), "c", buyproxy.AllocateParams{ProductKey: buyproxy.ProductPremium, GB: 1, IdempotencyKey: "k1"}); err != nil || calls != 2 {
		t.Fatalf("keyed: err=%v calls=%d", err, calls)
	}
}

func TestMissingKeyAndPublicCatalog(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		writeJSON(w, 200, map[string]any{"products": []any{}})
	}))
	defer srv.Close()
	t.Setenv("BUYPROXY_API_KEY", "")
	c := buyproxy.New(buyproxy.WithBaseURL(srv.URL))
	if _, err := c.Account.Get(context.Background()); !errors.Is(err, buyproxy.ErrAuthentication) {
		t.Fatalf("err = %v", err)
	}
	if _, err := c.Products.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d (the keyless account call must not hit the network)", calls)
	}
}

func TestWaitPollsUntilSettled(t *testing.T) {
	var n int32
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		status := "pending"
		if atomic.AddInt32(&n, 1) >= 3 {
			status = "succeeded"
		}
		writeJSON(w, 200, map[string]any{"id": "st_1", "status": status})
	})
	tx, err := c.Transactions.Wait(context.Background(), "st_1", time.Millisecond)
	if err != nil || tx.Status != "succeeded" || n != 3 {
		t.Fatalf("tx=%+v err=%v n=%d", tx, err, n)
	}
}

func TestContextCancellation(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		writeJSON(w, 200, nil)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := c.Account.Get(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestKeyKind(t *testing.T) {
	if buyproxy.New(buyproxy.WithAPIKey("bp_live_x")).KeyKind() != "reseller" || buyproxy.New(buyproxy.WithAPIKey("bp_key_x")).KeyKind() != "personal" {
		t.Fatal("key kind wrong")
	}
}
