package ifax

import (
	"context"
	"net/http"
)

// Me returns the account balance, limits and recent usage for the API key.
func (c *Client) Me(ctx context.Context) (*Account, error) {
	var acct Account
	if err := c.do(ctx, http.MethodGet, "/v1/me", nil, "", nil, &acct); err != nil {
		return nil, err
	}
	return &acct, nil
}

// Pricing returns the current price and limits.
func (c *Client) Pricing(ctx context.Context) (*Pricing, error) {
	var p Pricing
	if err := c.do(ctx, http.MethodGet, "/v1/pricing", nil, "", nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
