# BillionMail Webhook Implementation Guide

## Overview

This document provides a roadmap for implementing webhook/callback functionality in BillionMail based on the existing event tracking infrastructure.

---

## Part 1: Recommended Webhook Architecture

### 1.1 Core Webhook System Components

#### New Database Tables Required

```sql
-- Webhook configurations per user/account
CREATE TABLE IF NOT EXISTS bm_webhooks (
    id SERIAL PRIMARY KEY,
    user_id INTEGER,
    url VARCHAR(2048) NOT NULL,
    events TEXT NOT NULL,  -- JSON array: ["open", "click", "bounce", "deferred", "complaint"]
    secret VARCHAR(256),   -- For HMAC signature
    active SMALLINT DEFAULT 1,
    retry_policy JSONB,    -- {"max_attempts": 5, "backoff": "exponential"}
    create_time INTEGER,
    update_time INTEGER,
    UNIQUE(user_id, url)
);

-- Webhook event delivery log
CREATE TABLE IF NOT EXISTS bm_webhook_logs (
    id SERIAL PRIMARY KEY,
    webhook_id INTEGER NOT NULL,
    event_type VARCHAR(50),  -- open, click, bounce, deferred, complaint
    campaign_id INTEGER,
    recipient VARCHAR(320),
    payload JSONB,
    http_status_code INTEGER,
    response_body TEXT,
    attempt_count INTEGER DEFAULT 1,
    last_attempt_time INTEGER,
    next_retry_time INTEGER,
    status VARCHAR(20),  -- pending, success, failed, retry
    create_time INTEGER,
    FOREIGN KEY (webhook_id) REFERENCES bm_webhooks(id) ON DELETE CASCADE
);

-- Failed webhook delivery queue for retry
CREATE TABLE IF NOT EXISTS bm_webhook_queue (
    id SERIAL PRIMARY KEY,
    webhook_log_id INTEGER NOT NULL,
    scheduled_time INTEGER,  -- When to retry
    priority INTEGER DEFAULT 1,
    FOREIGN KEY (webhook_log_id) REFERENCES bm_webhook_logs(id) ON DELETE CASCADE
);
```

### 1.2 Event Payload Structure

```go
// Standard event payload
type WebhookEvent struct {
    // Event metadata
    EventId       string      `json:"event_id"`        // UUID for idempotency
    EventType     string      `json:"event_type"`      // open, click, bounce, deferred, complaint
    Timestamp     int64       `json:"timestamp"`       // Unix milliseconds
    
    // Campaign/Campaign identifier
    CampaignId    int         `json:"campaign_id"`
    CampaignName  string      `json:"campaign_name,omitempty"`
    
    // Recipient information
    Recipient     string      `json:"recipient"`
    MessageId     string      `json:"message_id"`
    
    // Event-specific data
    EventData     map[string]interface{} `json:"event_data"`
    // Examples:
    // open:     {"user_agent": "...", "ip_address": "...", "client": "Gmail"}
    // click:    {"url": "...", "user_agent": "...", "ip_address": "..."}
    // bounce:   {"bounce_type": "hard/soft", "dsn": "...", "error": "..."}
    // deferred: {"delay": 120, "dsn": "...", "reason": "..."}
    // complaint: {"fbl_type": "...", "complaint_feedback_type": "..."}
}

// Webhook delivery request
type WebhookDelivery struct {
    Signature   string       `json:"signature"`       // HMAC-SHA256 of payload
    Event       WebhookEvent `json:"event"`
    Attempt     int          `json:"attempt"`         // Retry attempt number
}
```

### 1.3 Signature Generation (HMAC-SHA256)

```go
// Using webhook secret for verification
signature := hmac_sha256(secret, json.Marshal(event))
// Include in header: X-BillionMail-Signature

// Validation at customer endpoint:
expectedSig := hmac_sha256(secret, body)
actualSig := request.Header.Get("X-BillionMail-Signature")
if !constantTimeCompare(expectedSig, actualSig) {
    return 401 Unauthorized
}
```

---

## Part 2: Implementation Plan

### Phase 1: Database & Models

1. **Create migration file** in `core/internal/service/database_initialization/webhooks.go`
   - Implement tables from section 1.1
   - Add necessary indexes for performance

2. **Create entity models** in `core/internal/model/entity/`
   - `webhook.go`: Webhook configuration
   - `webhook_log.go`: Delivery log
   - `webhook_event.go`: Event payload structure

### Phase 2: Service Layer

1. **Create webhook service** in `core/internal/service/webhooks/`

```go
// webhook_service.go - Main service
type WebhookService struct{}

func (ws *WebhookService) CreateWebhook(ctx, url, events, secret) (*entity.Webhook, error)
func (ws *WebhookService) UpdateWebhook(ctx, webhookId, updates) error
func (ws *WebhookService) DeleteWebhook(ctx, webhookId) error
func (ws *WebhookService) ListWebhooks(ctx, userId) ([]*entity.Webhook, error)
func (ws *WebhookService) GetWebhook(ctx, webhookId) (*entity.Webhook, error)

// webhook_dispatcher.go - Event dispatch logic
func (ws *WebhookService) DispatchEvent(ctx, event *WebhookEvent) error {
    // Find matching webhooks
    // Send to all registered endpoints
    // Log delivery attempts
    // Queue retries if failed
}

// webhook_retry.go - Retry management
func (ws *WebhookService) RetryFailedDeliveries(ctx) error {
    // Query bm_webhook_queue
    // Process scheduled retries
    // Exponential backoff strategy
}

// webhook_signature.go - HMAC generation
func (ws *WebhookService) GenerateSignature(secret string, payload []byte) string {
    // HMAC-SHA256
}
```

2. **Integrate with existing event handlers** in `core/internal/service/maillog_stat/tracker.go`

```go
// Modify CampaignEventHandler to dispatch webhook events
func CampaignEventHandler(r *ghttp.Request, encStr string) {
    // ... existing code ...
    
    // NEW: Dispatch webhook event
    event := &WebhookEvent{
        EventType: "open",  // or "click"
        CampaignId: data.CampaignId,
        Recipient: data.Recipient,
        MessageId: data.MessageId,
        Timestamp: time.Now().UnixMilli(),
        EventData: map[string]interface{}{
            "user_agent": r.Header.Get("User-Agent"),
            "ip_address": r.RemoteAddr,
        },
    }
    webhookService.DispatchEvent(ctx, event)
}
```

3. **Integrate with abnormal_recipient scan** in `core/internal/service/abnormal_recipient/`

```go
// After upserting bounced recipients
func AbnormalRecipientAutoStat(ctx context.Context) {
    // ... existing code ...
    
    // NEW: Dispatch bounce events
    for _, detail := range recipientDetails {
        event := &WebhookEvent{
            EventType: "bounce",
            Recipient: detail.Email,
            EventData: map[string]interface{}{
                "bounce_type": "hard",
                "error": detail.ErrorReason,
            },
        }
        webhookService.DispatchEvent(ctx, event)
    }
}
```

### Phase 3: Controller API

Create `core/internal/controller/webhooks/` with endpoints:

```go
// webhooks_v1_create.go
type CreateWebhookReq struct {
    g.Meta     `path:"/webhooks" method:"post"`
    URL        string   `json:"url" v:"required|url"`
    Events     []string `json:"events" v:"required|in:open,click,bounce,deferred,complaint"`
    Secret     string   `json:"secret" v:"required|min-length:20"`
}

// webhooks_v1_list.go
type ListWebhooksReq struct {
    g.Meta `path:"/webhooks" method:"get"`
    Page   int `json:"page" v:"min:1"`
    Size   int `json:"size" v:"min:1|max:100"`
}

// webhooks_v1_delete.go
type DeleteWebhookReq struct {
    g.Meta    `path:"/webhooks/:id" method:"delete"`
    ID        int `json:"id" v:"required|min:1"`
}

// webhooks_v1_test.go
type TestWebhookReq struct {
    g.Meta `path:"/webhooks/:id/test" method:"post"`
    ID     int `json:"id" v:"required|min:1"`
}
// Sends sample event to verify webhook works
```

### Phase 4: Timer for Webhook Retries

Add to `core/internal/service/timers/timers.go`:

```go
// Webhook retry processing (every 5 minutes)
gtimer.Add(5*time.Minute, func() {
    webhookService.RetryFailedDeliveries(ctx)
})
```

### Phase 5: API Registration

Register in `core/internal/cmd/cmd.go`:

```go
group.Bind(
    // ... existing controllers ...
    webhooks.NewV1(),  // Add this
)
```

---

## Part 3: Implementation Details

### 3.1 Event Triggering Points

Modify these files to dispatch webhook events:

| File | Event Type | Location |
|------|-----------|----------|
| `maillog_stat/tracker.go` | `open` | CampaignEventHandler, case "open" |
| `maillog_stat/tracker.go` | `click` | CampaignEventHandler, case "click" |
| `abnormal_recipient/abnormal_recipient.go` | `bounce` | AbnormalRecipientAutoStat() |
| `maillog_stat/maillog_stat.go` | `deferred` | When processing deferred records |
| `maillog_stat/maillog_stat.go` | `complaint` | When processing FBL records |

### 3.2 Retry Logic

```go
type RetryStrategy struct {
    MaxAttempts  int
    BackoffType  string  // "exponential", "linear", "fixed"
    InitialDelay int     // seconds
    MaxDelay     int     // seconds
}

// Example: exponential backoff
// Attempt 1: immediate
// Attempt 2: 5 seconds
// Attempt 3: 25 seconds (5 * 5)
// Attempt 4: 125 seconds (25 * 5)
// Attempt 5: 625 seconds (2+ hours, cap at max)
```

### 3.3 Idempotency

Use EventId (UUID) to allow webhook consumers to deduplicate:

```go
event.EventId = uuid.New().String()
// Consumer stores eventId, rejects duplicates
```

### 3.4 Security Considerations

1. **HMAC Signature**: Verify authenticity
2. **Rate Limiting**: Limit webhooks per user (e.g., max 10,000 calls/day)
3. **Timeout**: 30-second HTTP timeout per delivery
4. **Validation**: Verify URL is accessible (DNS rebinding attack)
5. **Dead Letter Queue**: Store permanently failed webhooks

---

## Part 4: Database Schema Details

### Indexes for Performance

```sql
CREATE INDEX idx_webhooks_user_active ON bm_webhooks(user_id, active);
CREATE INDEX idx_webhooks_events ON bm_webhooks USING GIN(events);
CREATE INDEX idx_webhook_logs_webhook_status ON bm_webhook_logs(webhook_id, status);
CREATE INDEX idx_webhook_logs_create_time ON bm_webhook_logs(create_time);
CREATE INDEX idx_webhook_queue_scheduled ON bm_webhook_queue(scheduled_time);
```

---

## Part 5: Frontend Integration

Add to settings/configuration page:

```
Dashboard → Settings → Webhooks
├─ View/Manage Webhooks
│  ├─ Add New Webhook (URL, event types, secret)
│  ├─ Test Webhook (send sample event)
│  ├─ View Delivery Logs
│  └─ Edit/Delete
└─ Webhook Documentation
   ├─ Event types
   ├─ Payload examples
   └─ Signature verification code samples
```

---

## Part 6: API Documentation

### Webhook Event Types

| Event | When | Payload | Campaign Type |
|-------|------|---------|---------------|
| `open` | Pixel loaded | user_agent, ip | Both |
| `click` | Link clicked | url, user_agent, ip | Both |
| `bounce` | Hard failure | bounce_type, dsn, error | Both |
| `deferred` | Temp failure | delay, dsn, reason | Both |
| `complaint` | FBL report | fbl_type, reason | Both |

### HTTP Request Format

```
POST {webhook_url}
Content-Type: application/json
X-BillionMail-Signature: sha256=hmac_result
X-BillionMail-Event-Type: open|click|bounce|deferred|complaint
X-BillionMail-Attempt: 1

{
  "signature": "sha256=...",
  "event": {
    "event_id": "uuid",
    "event_type": "open",
    "timestamp": 1634567890000,
    "campaign_id": 123,
    "campaign_name": "Q4 Campaign",
    "recipient": "user@example.com",
    "message_id": "<msg-id@domain.com>",
    "event_data": {...}
  }
}
```

### Success Response

```
HTTP 200 OK

Any response with status 2xx is considered success.
Response body is logged but not required.
```

### Failure Response

```
HTTP 500+ or timeout → Retry scheduled
HTTP 400-499 → Logged as failed (no retry)
No response (timeout) → Retry scheduled
```

---

## Part 7: Testing Strategy

### Unit Tests

- Test event payload generation
- Test HMAC signature generation/validation
- Test webhook filtering by event type
- Test retry logic (exponential backoff)

### Integration Tests

- Test event dispatching end-to-end
- Test webhook delivery with mock endpoint
- Test failure scenarios and retries
- Test idempotency with duplicate events

### Load Tests

- Test with high event volume (1000+ events/sec)
- Verify no queue buildup
- Monitor database write performance

---

## Part 8: Monitoring & Analytics

Store in `bm_webhook_logs`:

- Delivery success rate per webhook
- Average delivery time
- Retry statistics
- Failed webhooks requiring manual intervention

Provide dashboard:
```
Webhooks Dashboard
├─ Total Events: X
├─ Success Rate: Y%
├─ Failed Deliveries: Z
├─ Retry Queue Size: N
└─ Top Failed Webhooks
```

---

## Part 9: Configuration

Add to `bm_options`:

| Option Name | Default | Purpose |
|-------------|---------|---------|
| `webhook_max_retries` | 5 | Max retry attempts |
| `webhook_retry_backoff` | exponential | Retry strategy |
| `webhook_timeout_seconds` | 30 | HTTP timeout |
| `webhook_rate_limit_per_day` | 10000 | Max webhooks/user/day |
| `webhook_max_concurrent` | 100 | Max parallel deliveries |

---

## Part 10: Migration Path

### For Existing Users

Webhooks are opt-in. No existing functionality changes.

1. Send email notification about new webhook feature
2. Provide setup guide in documentation
3. Offer webhook templates for common integrations

### For New Users

Include webhook setup in onboarding flow.

---

## Timeline Estimate

- **Phase 1-2** (Database + Service): 2-3 days
- **Phase 3** (API Controllers): 1-2 days
- **Phase 4-5** (Integration): 1-2 days
- **Phase 6-7** (Documentation + Frontend): 1-2 days
- **Phase 8-10** (Testing + Monitoring): 2-3 days

**Total: 10-14 days** for full implementation

---

## Summary

The webhook implementation leverages BillionMail's existing event tracking infrastructure. Key integration points are:

1. **tracker.go** - Dispatch open/click events
2. **abnormal_recipient.go** - Dispatch bounce events
3. **maillog_stat.go** - Dispatch deferred/complaint events
4. **timers.go** - Add retry processing timer
5. **cmd.go** - Register webhook API endpoints

All events use existing data structures, so minimal refactoring needed. The webhook system is self-contained with separate tables for configuration and delivery logging.

