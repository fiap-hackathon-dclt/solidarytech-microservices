package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

var ErrNGONotFound = errors.New("ONG não encontrada")

type HTTPNGOClient struct {
	baseURL string
	http    *http.Client
}

func (c *HTTPNGOClient) Cause(ctx context.Context, ngoID int) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/ngos/%d", c.baseURL, ngoID), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var ngo struct {
			Cause string `json:"cause"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&ngo); err != nil {
			return "", fmt.Errorf("resposta ilegível do ngo-service: %w", err)
		}
		return ngo.Cause, nil
	case http.StatusNotFound:
		return "", ErrNGONotFound
	default:
		return "", fmt.Errorf("ngo-service respondeu %d", resp.StatusCode)
	}
}
