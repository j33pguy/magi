package secretstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type vaultBackend struct {
	addr      string
	token     string
	mount     string
	namespace string
	client    *http.Client
}

func newVaultBackend(cfg Config) *vaultBackend {
	return &vaultBackend{
		addr:      strings.TrimRight(cfg.VaultAddr, "/"),
		token:     cfg.VaultToken,
		mount:     strings.Trim(cfg.VaultMount, "/"),
		namespace: cfg.VaultNamespace,
		client:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (v *vaultBackend) Name() string { return "vault" }

func (v *vaultBackend) Put(ctx context.Context, path string, data map[string]string) error {
	body, err := json.Marshal(map[string]any{"data": data})
	if err != nil {
		return fmt.Errorf("marshal vault payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.addr+"/v1/"+v.mount+"/data/"+strings.Trim(path, "/"), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create vault request: %w", err)
	}
	v.applyHeaders(req)
	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("store secret in vault: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("vault write returned status %d", resp.StatusCode)
	}
	return nil
}

func (v *vaultBackend) Get(ctx context.Context, path string) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.addr+"/v1/"+v.mount+"/data/"+strings.Trim(path, "/"), nil)
	if err != nil {
		return nil, fmt.Errorf("create vault request: %w", err)
	}
	v.applyHeaders(req)
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("read secret from vault: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("vault read returned status %d", resp.StatusCode)
	}
	var payload struct {
		Data struct {
			Data map[string]string `json:"data"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode vault response: %w", err)
	}
	return payload.Data.Data, nil
}

func (v *vaultBackend) applyHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Vault-Token", v.token)
	if v.namespace != "" {
		req.Header.Set("X-Vault-Namespace", v.namespace)
	}
}
