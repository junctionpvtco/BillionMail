package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// Event types
const (
	EventDelivery    = "delivery"
	EventBounce      = "bounce"
	EventOpen        = "open"
	EventClick       = "click"
	EventUnsubscribe = "unsubscribe"
	EventComplaint   = "complaint"
	EventDeferral    = "deferral"
)

// Log status
const (
	LogStatusPending = 0
	LogStatusSuccess = 1
	LogStatusFailed  = 2
)

// AllEventTypes returns all supported event types
func AllEventTypes() []string {
	return []string{
		EventDelivery,
		EventBounce,
		EventOpen,
		EventClick,
		EventUnsubscribe,
		EventComplaint,
		EventDeferral,
	}
}

// WebhookPayload represents the payload sent to webhook endpoints
type WebhookPayload struct {
	Event     string      `json:"event"`
	Timestamp int64       `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// WebhookEndpoint represents a configured webhook
type WebhookEndpoint struct {
	ID          int      `json:"id"`
	URL         string   `json:"url"`
	Secret      string   `json:"secret"`
	Events      []string `json:"events"`
	Active      int      `json:"active"`
	Description string   `json:"description"`
	CreateTime  int      `json:"create_time"`
	UpdateTime  int      `json:"update_time"`
}

// webhookEndpointDB is used for database scanning
type webhookEndpointDB struct {
	ID          int    `json:"id"`
	URL         string `json:"url"`
	Secret      string `json:"secret"`
	Events      string `json:"events"`
	Active      int    `json:"active"`
	Description string `json:"description"`
	CreateTime  int    `json:"create_time"`
	UpdateTime  int    `json:"update_time"`
}

var (
	maxRetries     = 5
	retryIntervals = []int{60, 300, 900, 3600, 7200} // seconds: 1min, 5min, 15min, 1hr, 2hr
	httpClient     = &http.Client{Timeout: 10 * time.Second}
	dispatchMu     sync.Mutex
)

// Dispatch sends an event to all matching active webhooks
func Dispatch(ctx context.Context, eventType string, data interface{}) {
	go func() {
		dispatchMu.Lock()
		defer dispatchMu.Unlock()

		doDispatch(ctx, eventType, data)
	}()
}

func doDispatch(ctx context.Context, eventType string, data interface{}) {
	var endpoints []webhookEndpointDB
	err := g.DB().Model("bm_webhooks").
		Where("active", 1).
		Scan(&endpoints)
	if err != nil {
		g.Log().Warning(ctx, "Failed to query webhook endpoints:", err)
		return
	}

	now := time.Now()
	payload := WebhookPayload{
		Event:     eventType,
		Timestamp: now.Unix(),
		Data:      data,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		g.Log().Warning(ctx, "Failed to marshal webhook payload:", err)
		return
	}
	payloadStr := string(payloadBytes)

	for _, ep := range endpoints {
		var events []string
		if err := json.Unmarshal([]byte(ep.Events), &events); err != nil {
			continue
		}

		if !containsEvent(events, eventType) {
			continue
		}

		// Attempt immediate delivery
		respStatus, respBody, deliveryErr := sendWebhook(ep.URL, ep.Secret, payloadBytes)

		status := LogStatusSuccess
		nextRetry := 0
		if deliveryErr != nil || respStatus < 200 || respStatus >= 300 {
			status = LogStatusPending
			nextRetry = int(now.Unix()) + retryIntervals[0]
		}

		// Log the attempt
		_, logErr := g.DB().Model("bm_webhook_logs").Insert(g.Map{
			"webhook_id":      ep.ID,
			"event_type":      eventType,
			"payload":         payloadStr,
			"response_status": respStatus,
			"response_body":   truncate(respBody, 1000),
			"attempts":        1,
			"next_retry":      nextRetry,
			"status":          status,
			"create_time":     now.Unix(),
			"update_time":     now.Unix(),
		})
		if logErr != nil {
			g.Log().Warning(ctx, "Failed to log webhook delivery:", logErr)
		}
	}
}

// ProcessRetries retries failed webhook deliveries
func ProcessRetries(ctx context.Context) {
	now := time.Now().Unix()

	type logEntry struct {
		ID             int    `json:"id"`
		WebhookID      int    `json:"webhook_id"`
		Payload        string `json:"payload"`
		Attempts       int    `json:"attempts"`
		ResponseStatus int    `json:"response_status"`
	}

	var logs []logEntry
	err := g.DB().Model("bm_webhook_logs").
		Where("status", LogStatusPending).
		WhereLTE("next_retry", now).
		WhereGT("next_retry", 0).
		Limit(50).
		Scan(&logs)
	if err != nil {
		g.Log().Warning(ctx, "Failed to query retry webhook logs:", err)
		return
	}

	for _, entry := range logs {
		var ep webhookEndpointDB
		err := g.DB().Model("bm_webhooks").Where("id", entry.WebhookID).Scan(&ep)
		if err != nil || ep.Active != 1 {
			// Mark as failed if webhook is deleted or inactive
			_, _ = g.DB().Model("bm_webhook_logs").Where("id", entry.ID).Data(g.Map{
				"status":      LogStatusFailed,
				"update_time": now,
			}).Update()
			continue
		}

		respStatus, respBody, deliveryErr := sendWebhook(ep.URL, ep.Secret, []byte(entry.Payload))

		newAttempts := entry.Attempts + 1
		if deliveryErr == nil && respStatus >= 200 && respStatus < 300 {
			_, _ = g.DB().Model("bm_webhook_logs").Where("id", entry.ID).Data(g.Map{
				"response_status": respStatus,
				"response_body":   truncate(respBody, 1000),
				"attempts":        newAttempts,
				"status":          LogStatusSuccess,
				"update_time":     now,
			}).Update()
		} else if newAttempts >= maxRetries {
			_, _ = g.DB().Model("bm_webhook_logs").Where("id", entry.ID).Data(g.Map{
				"response_status": respStatus,
				"response_body":   truncate(respBody, 1000),
				"attempts":        newAttempts,
				"status":          LogStatusFailed,
				"update_time":     now,
			}).Update()
		} else {
			nextInterval := retryIntervals[0]
			if newAttempts < len(retryIntervals) {
				nextInterval = retryIntervals[newAttempts]
			}
			_, _ = g.DB().Model("bm_webhook_logs").Where("id", entry.ID).Data(g.Map{
				"response_status": respStatus,
				"response_body":   truncate(respBody, 1000),
				"attempts":        newAttempts,
				"next_retry":      int(now) + nextInterval,
				"update_time":     now,
			}).Update()
		}
	}
}

// sendWebhook sends a webhook HTTP request with HMAC-SHA256 signature
func sendWebhook(webhookURL string, secret string, payload []byte) (statusCode int, body string, err error) {
	req, err := http.NewRequest("POST", webhookURL, strings.NewReader(string(payload)))
	if err != nil {
		return 0, "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "BillionMail-Webhook/1.0")

	if secret != "" {
		sig := computeHMAC(payload, secret)
		req.Header.Set("X-BillionMail-Signature", sig)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return resp.StatusCode, string(bodyBytes), nil
}

// computeHMAC computes HMAC-SHA256 signature
func computeHMAC(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func containsEvent(events []string, event string) bool {
	for _, e := range events {
		if e == event {
			return true
		}
	}
	return false
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// CRUD Operations

// CreateWebhook creates a new webhook endpoint
func CreateWebhook(ctx context.Context, url string, secret string, events []string, description string) (int, error) {
	eventsJSON, err := json.Marshal(events)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal events: %w", err)
	}

	now := time.Now().Unix()
	result, err := g.DB().Model("bm_webhooks").InsertAndGetId(g.Map{
		"url":         url,
		"secret":      secret,
		"events":      string(eventsJSON),
		"active":      1,
		"description": description,
		"create_time": now,
		"update_time": now,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create webhook: %w", err)
	}

	return int(result), nil
}

// UpdateWebhook updates an existing webhook
func UpdateWebhook(ctx context.Context, id int, url string, secret string, events []string, active int, description string) error {
	eventsJSON, err := json.Marshal(events)
	if err != nil {
		return fmt.Errorf("failed to marshal events: %w", err)
	}

	_, err = g.DB().Model("bm_webhooks").Where("id", id).Data(g.Map{
		"url":         url,
		"secret":      secret,
		"events":      string(eventsJSON),
		"active":      active,
		"description": description,
		"update_time": time.Now().Unix(),
	}).Update()
	if err != nil {
		return fmt.Errorf("failed to update webhook: %w", err)
	}

	return nil
}

// DeleteWebhook deletes a webhook endpoint
func DeleteWebhook(ctx context.Context, id int) error {
	_, err := g.DB().Model("bm_webhooks").Where("id", id).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete webhook: %w", err)
	}
	return nil
}

// GetWebhook returns a single webhook by ID
func GetWebhook(ctx context.Context, id int) (*WebhookEndpoint, error) {
	var ep webhookEndpointDB
	err := g.DB().Model("bm_webhooks").Where("id", id).Scan(&ep)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook: %w", err)
	}

	var events []string
	_ = json.Unmarshal([]byte(ep.Events), &events)

	return &WebhookEndpoint{
		ID:          ep.ID,
		URL:         ep.URL,
		Secret:      ep.Secret,
		Events:      events,
		Active:      ep.Active,
		Description: ep.Description,
		CreateTime:  ep.CreateTime,
		UpdateTime:  ep.UpdateTime,
	}, nil
}

// ListWebhooks returns all webhooks with pagination
func ListWebhooks(ctx context.Context, page, pageSize int) (int, []*WebhookEndpoint, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	model := g.DB().Model("bm_webhooks").Safe()
	total, err := model.Count()
	if err != nil {
		return 0, nil, fmt.Errorf("failed to count webhooks: %w", err)
	}

	var dbList []webhookEndpointDB
	err = model.Page(page, pageSize).Order("id DESC").Scan(&dbList)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to list webhooks: %w", err)
	}

	list := make([]*WebhookEndpoint, 0, len(dbList))
	for _, ep := range dbList {
		var events []string
		_ = json.Unmarshal([]byte(ep.Events), &events)
		list = append(list, &WebhookEndpoint{
			ID:          ep.ID,
			URL:         ep.URL,
			Secret:      ep.Secret,
			Events:      events,
			Active:      ep.Active,
			Description: ep.Description,
			CreateTime:  ep.CreateTime,
			UpdateTime:  ep.UpdateTime,
		})
	}

	return total, list, nil
}

// GetWebhookLogs returns webhook delivery logs with pagination
func GetWebhookLogs(ctx context.Context, webhookID int, page, pageSize int) (int, []g.Map, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	model := g.DB().Model("bm_webhook_logs").Safe()
	if webhookID > 0 {
		model = model.Where("webhook_id", webhookID)
	}

	total, err := model.Count()
	if err != nil {
		return 0, nil, fmt.Errorf("failed to count webhook logs: %w", err)
	}

	result, err := model.Page(page, pageSize).Order("id DESC").All()
	if err != nil {
		return 0, nil, fmt.Errorf("failed to list webhook logs: %w", err)
	}

	list := make([]g.Map, 0, len(result))
	for _, record := range result.List() {
		list = append(list, record)
	}

	return total, list, nil
}
