package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type DNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
	TTL     int    `json:"ttl"`
}

type DNSUpdate struct {
	Action          string    `json:"action"`
	RecordName      string    `json:"record_name"`
	RecordID        string    `json:"record_id,omitempty"`
	PreviousContent string    `json:"previous_content,omitempty"`
	NewContent      string    `json:"new_content"`
	Proxied         bool      `json:"proxied"`
	TTL             int       `json:"ttl"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type CloudflareDNS struct {
	APIBase string
	Token   string
	Client  *http.Client
}

type cfResponse[T any] struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Messages []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"messages"`
	Result T `json:"result"`
}

func NewCloudflareDNS(apiBase, token string) *CloudflareDNS {
	return &CloudflareDNS{
		APIBase: strings.TrimRight(apiBase, "/"),
		Token:   token,
		Client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *CloudflareDNS) UpsertARecord(ctx context.Context, zoneID, name, ip string, ttl int, proxied bool, comment string, create bool) (DNSUpdate, error) {
	records, err := c.ListARecords(ctx, zoneID, name)
	if err != nil {
		return DNSUpdate{}, err
	}

	payload := buildARecordPayload(name, ip, ttl, proxied, comment)
	now := time.Now().UTC()
	if len(records) == 0 {
		if !create {
			return DNSUpdate{}, fmt.Errorf("A 记录 %q 不存在，且未启用自动创建", name)
		}
		record, err := cfRequest[DNSRecord](ctx, c, http.MethodPost, fmt.Sprintf("/zones/%s/dns_records", zoneID), payload)
		if err != nil {
			return DNSUpdate{}, err
		}
		return DNSUpdate{
			Action:     "created",
			RecordName: record.Name,
			RecordID:   record.ID,
			NewContent: record.Content,
			Proxied:    record.Proxied,
			TTL:        record.TTL,
			UpdatedAt:  now,
		}, nil
	}

	record := records[0]
	update := DNSUpdate{
		Action:          "updated",
		RecordName:      record.Name,
		RecordID:        record.ID,
		PreviousContent: record.Content,
		NewContent:      ip,
		Proxied:         proxied,
		TTL:             ttl,
		UpdatedAt:       now,
	}
	if record.Content == ip && record.Proxied == proxied && record.TTL == ttl {
		update.Action = "unchanged"
		return update, nil
	}

	updated, err := cfRequest[DNSRecord](ctx, c, http.MethodPatch, fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, record.ID), payload)
	if err != nil {
		return DNSUpdate{}, err
	}
	update.RecordName = updated.Name
	update.NewContent = updated.Content
	update.Proxied = updated.Proxied
	update.TTL = updated.TTL
	return update, nil
}

func (c *CloudflareDNS) ListARecords(ctx context.Context, zoneID, name string) ([]DNSRecord, error) {
	values := url.Values{}
	values.Set("type", "A")
	values.Set("name", name)
	values.Set("per_page", "100")
	endpoint := fmt.Sprintf("/zones/%s/dns_records?%s", zoneID, values.Encode())
	return cfRequest[[]DNSRecord](ctx, c, http.MethodGet, endpoint, nil)
}

func cfRequest[T any](ctx context.Context, client *CloudflareDNS, method, endpoint string, body any) (T, error) {
	var zero T
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return zero, err
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, client.APIBase+endpoint, reader)
	if err != nil {
		return zero, err
	}
	req.Header.Set("Authorization", "Bearer "+client.Token)
	req.Header.Set("Content-Type", "application/json")

	httpClient := client.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return zero, err
	}

	var parsed cfResponse[T]
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return zero, fmt.Errorf("解析 Cloudflare 响应失败: %w，响应内容=%s", err, string(respBody))
	}
	if resp.StatusCode >= 300 {
		return zero, fmt.Errorf("Cloudflare API 返回 HTTP %d，响应内容=%s", resp.StatusCode, string(respBody))
	}
	if !parsed.Success {
		return zero, errors.New(formatCloudflareErrors(parsed))
	}
	return parsed.Result, nil
}

func buildARecordPayload(name, ip string, ttl int, proxied bool, comment string) map[string]any {
	payload := map[string]any{
		"type":    "A",
		"name":    name,
		"content": ip,
		"ttl":     ttl,
		"proxied": proxied,
	}
	if comment != "" {
		payload["comment"] = comment
	}
	return payload
}

func formatCloudflareErrors[T any](resp cfResponse[T]) string {
	var parts []string
	for _, err := range resp.Errors {
		parts = append(parts, fmt.Sprintf("[%d] %s", err.Code, err.Message))
	}
	if len(parts) == 0 {
		return "Cloudflare API 请求失败"
	}
	return strings.Join(parts, "; ")
}
