package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

// UpstreamBalance is deliberately separate from billing-rate snapshots: reading
// a balance must never change scheduling, local quotas, or billing multipliers.
type UpstreamBalance struct {
	Status    string    `json:"status"`
	Provider  string    `json:"provider,omitempty"`
	Scope     string    `json:"scope,omitempty"`
	Amount    *float64  `json:"amount,omitempty"`
	Unit      string    `json:"unit,omitempty"`
	Unlimited bool      `json:"unlimited,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// QueryUpstreamBalance uses the stored API key only, never a browser credential.
// Concurrency and duplicate requests share the existing upstream probe limiter.
func (s *UpstreamBillingProbeService) QueryUpstreamBalance(ctx context.Context, id int64) (*UpstreamBalance, error) {
	if s == nil || s.accountRepo == nil || s.accountTestService == nil || s.accountTestService.httpUpstream == nil {
		return nil, ErrUpstreamBillingProbeUnavailable
	}
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// Do not join a request for credentials that were edited while it was running.
	key := "balance:" + strconv.FormatInt(id, 10) + ":" + strconv.FormatInt(account.UpdatedAt.UnixNano(), 10)
	value, err, _ := s.probeGroup.Do(key, func() (any, error) {
		select {
		case s.probeSlots <- struct{}{}:
			defer func() { <-s.probeSlots }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		result := &UpstreamBalance{Status: "unavailable", CheckedAt: time.Now().UTC()}
		base := account.GetCredential("base_url")
		if account.Type != AccountTypeAPIKey || upstreamBillingProbeTargetIsOfficialAPI(base) || account.GetCredential("api_key") == "" {
			return result, nil
		}
		base, err := s.accountTestService.validateUpstreamBaseURL(base)
		if err != nil {
			return result, nil
		}
		probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		get := func(endpoint string, authenticated bool) ([]byte, error) {
			return s.fetchUpstreamBalance(probeCtx, account, endpoint, authenticated)
		}
		body, err := get(buildOpenAIEndpointURL(base, "/v1/usage"), true)
		if err == nil {
			if parsed := parseSub2APIBalance(body); parsed != nil {
				parsed.CheckedAt = result.CheckedAt
				return parsed, nil
			}
		}
		// New API's token endpoint identifies the response family and tells us
		// whether the billing API's large sentinel denotes an unlimited key.
		tokenBody, err := get(upstreamBalanceSiteURL(base, "/api/usage/token/"), true)
		if err != nil {
			return result, nil
		}
		var token struct {
			Success bool `json:"success"`
			Data    struct {
				Object    string `json:"object"`
				Unlimited bool   `json:"unlimited_quota"`
			} `json:"data"`
		}
		if json.Unmarshal(tokenBody, &token) != nil || !token.Success || token.Data.Object != "token_usage" {
			return result, nil
		}
		status, err := get(upstreamBalanceSiteURL(base, "/api/status"), false)
		if err != nil {
			return result, nil
		}
		subscription, err := get(buildOpenAIEndpointURL(base, "/v1/dashboard/billing/subscription"), true)
		if err != nil {
			return result, nil
		}
		usage, err := get(buildOpenAIEndpointURL(base, "/v1/dashboard/billing/usage"), true)
		if err != nil {
			return result, nil
		}
		if parsed := parseNewAPIBalance(status, subscription, usage, token.Data.Unlimited); parsed != nil {
			parsed.CheckedAt = result.CheckedAt
			return parsed, nil
		}
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	balance, ok := value.(*UpstreamBalance)
	if !ok {
		return nil, ErrUpstreamBillingProbeUnavailable
	}
	return balance, nil
}

func upstreamBalanceSiteURL(base, endpoint string) string {
	u, _ := url.Parse(base) // base has already passed URL validation
	u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/v1") + endpoint
	u.RawPath, u.RawQuery, u.Fragment = "", "", ""
	return u.String()
}

func (s *UpstreamBillingProbeService) fetchUpstreamBalance(ctx context.Context, account *Account, endpoint string, authenticated bool) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if account.Platform == PlatformOpenAI {
		ctx = WithHTTPUpstreamProfile(ctx, HTTPUpstreamProfileOpenAI)
	}
	req, err := http.NewRequestWithContext(WithHTTPUpstreamRedirectsDisabled(ctx), http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if authenticated {
		account.ApplyHeaderOverrides(req.Header)
		req.Header.Set("Authorization", "Bearer "+account.GetCredential("api_key"))
	}
	proxy := ""
	if account.ProxyID != nil {
		if account.Proxy == nil || account.Proxy.ID != *account.ProxyID {
			return nil, errors.New("proxy unavailable")
		}
		proxy = account.Proxy.URL()
	}
	var profile *tlsfingerprint.Profile
	if s.accountTestService.tlsFPProfileService != nil {
		profile = s.accountTestService.tlsFPProfileService.ResolveTLSProfile(account)
	}
	resp, err := s.accountTestService.httpUpstream.DoWithTLS(req, proxy, account.ID, account.Concurrency, profile)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Body == nil {
		return nil, errors.New("empty response")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("balance unavailable")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024+1))
	if err != nil || len(body) > 1024*1024 {
		return nil, errors.New("invalid response size")
	}
	return body, nil
}

func validBalanceAmount(value *float64) bool {
	return value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0)
}

func parseSub2APIBalance(body []byte) *UpstreamBalance {
	var data struct {
		Mode         string          `json:"mode"`
		Valid        *bool           `json:"isValid"`
		Balance      *float64        `json:"balance"`
		Remaining    *float64        `json:"remaining"`
		Unit         string          `json:"unit"`
		Subscription json.RawMessage `json:"subscription"`
		Quota        json.RawMessage `json:"quota"`
	}
	if json.Unmarshal(body, &data) != nil || data.Valid == nil || !*data.Valid || data.Unit != "USD" {
		return nil
	}
	result := &UpstreamBalance{Status: "ok", Provider: "sub2api", Unit: "USD"}
	switch {
	case data.Mode == "unrestricted" && validBalanceAmount(data.Balance):
		result.Scope, result.Amount = "wallet", data.Balance
	case data.Mode == "quota_limited" && len(data.Quota) > 0 && string(data.Quota) != "null" && validBalanceAmount(data.Remaining):
		result.Scope, result.Amount = "key", data.Remaining
	case data.Mode == "unrestricted" && len(data.Subscription) > 0 && string(data.Subscription) != "null" && validBalanceAmount(data.Remaining) && *data.Remaining >= 0:
		result.Scope, result.Amount = "subscription", data.Remaining
	default:
		return nil
	}
	return result
}

func parseNewAPIBalance(status, subscription, usage []byte, unlimited bool) *UpstreamBalance {
	var site struct {
		Success bool `json:"success"`
		Data    struct {
			Display  string `json:"quota_display_type"`
			Currency *bool  `json:"display_in_currency"`
		} `json:"data"`
	}
	var limit struct {
		Object string          `json:"object"`
		Limit  *float64        `json:"hard_limit_usd"`
		Error  json.RawMessage `json:"error"`
	}
	var used struct {
		Object string          `json:"object"`
		Usage  *float64        `json:"total_usage"`
		Error  json.RawMessage `json:"error"`
	}
	if json.Unmarshal(status, &site) != nil || !site.Success || json.Unmarshal(subscription, &limit) != nil || json.Unmarshal(usage, &used) != nil {
		return nil
	}
	if limit.Object != "billing_subscription" || used.Object != "list" || len(limit.Error) > 0 || len(used.Error) > 0 || !validBalanceAmount(limit.Limit) || !validBalanceAmount(used.Usage) || *used.Usage < 0 {
		return nil
	}
	unit := site.Data.Display
	if unit == "" {
		// Older versions convert currency to USD; otherwise they expose raw quota.
		if site.Data.Currency == nil {
			return nil
		}
		if *site.Data.Currency {
			unit = "USD"
		} else {
			unit = "QUOTA"
		}
	}
	if unit != "USD" && unit != "CNY" && unit != "TOKENS" && unit != "QUOTA" {
		return nil
	}
	result := &UpstreamBalance{Status: "ok", Provider: "newapi", Scope: "upstream", Unit: unit}
	if unlimited && *limit.Limit == 100000000 {
		result.Unlimited, result.Scope = true, "key"
		return result
	}
	amount := *limit.Limit - *used.Usage/100
	if !validBalanceAmount(&amount) {
		return nil
	}
	result.Amount = &amount
	return result
}
