# BillionMail Event Tracking & Webhook Infrastructure - Comprehensive Analysis

## Executive Summary

BillionMail has a sophisticated **event tracking system** built around email campaigns that records opens, clicks, bounces, and delivery status. The system uses:
- **Encrypted tracking URLs** for click tracking
- **Tracking pixels** for open tracking
- **Centralized maillog database** with multiple event tables
- **Real-time event handlers** via `/pmta/` endpoint
- **Automatic bounce detection** with abnormal recipient management
- **API-based email sending** with separate tracking for API campaigns

**Current Status**: No webhook/callback infrastructure exists yet. The system processes events internally. A webhook branch `copilot/add-email-webhooks-support` exists but only has initial planning commits.

---

## 1. EVENT TRACKING FLOW

### 1.1 Campaign Event Handler Endpoint
**File**: `core/internal/service/maillog_stat/tracker.go`
**Endpoint**: `/pmta/{encrypted_data}`

The handler processes three types of events:
- **open**: Email was opened by recipient
- **click**: Link in email was clicked
- (Fire-and-forget tracking)

```go
func CampaignEventHandler(r *ghttp.Request, encStr string) {
    // Decrypts encrypted event data
    // Validates: message_id, recipient (email), type, campaign_id, url
    
    // Handles "open" type:
    // - Inserts into mailstat_opened table
    // - Updates contact_activity (last_active_at)
    // - Returns 1x1 transparent PNG image
    
    // Handles "click" type:
    // - Inserts into mailstat_clicked table
    // - Updates contact_activity
    // - Redirects to target URL
}
```

### 1.2 Tracking URL Generation
The `MailTracker` class (tracker.go) generates:
1. **Click tracking URL**: Encrypted data with type="click", url, recipient, campaign_id, message_id
2. **Open tracking pixel URL**: Same structure but type="open" (no URL param)

**Encryption**: AES-256-CBC with PKCS7 padding, Base64 URL-safe encoding

```go
type MailTracker struct {
    originalMailHTML string
    campaignID       int
    messageID        string      // Trimmed <>, unique per email
    recipient        string
    baseURL          string      // e.g., "https://domain.com/pmta"
}

func (t *MailTracker) TrackLinks()        // Wraps all href links in tracking URLs
func (t *MailTracker) AppendTrackingPixel() // Adds hidden 1x1 pixel image
func (t *MailTracker) GetTrackingURL(url string) string
func (t *MailTracker) GetTrackingPixel() string
```

---

## 2. DATABASE SCHEMA

### 2.1 Event Tables (Maillog Statistics)
**Location**: `core/internal/service/database_initialization/maillog_stat.go`

#### Core Event Tables:

| Table | Purpose | Key Fields |
|-------|---------|-----------|
| `mailstat_send_mails` | Outbound email delivery status | postfix_message_id, recipient, status (bounced/deferred/sent), mail_provider, dsn, relay |
| `mailstat_receive_mails` | Inbound email delivery status | postfix_message_id, recipient, status, delay, dsn |
| `mailstat_opened` | Email open events | campaign_id, recipient, message_id, postfix_message_id, log_time_millis |
| `mailstat_clicked` | Link click events | campaign_id, recipient, url, message_id, postfix_message_id, log_time_millis |
| `mailstat_deferred_mails` | Deferred delivery records | postfix_message_id, delay, dsn, description |
| `mailstat_complaints` | FBL/complaint records | postfix_message_id, recipient |
| `mailstat_removed` | Queue removal records | postfix_message_id |

#### Supporting Tables:

| Table | Purpose |
|-------|---------|
| `mailstat_message_ids` | Maps postfix_message_id → message_id |
| `mailstat_senders` | Sender domain & email size tracking |

### 2.2 Campaign & Recipient Tables
**Location**: `core/internal/service/database_initialization/batch_mail.go`

| Table | Purpose | Key Fields |
|-------|---------|-----------|
| `email_tasks` | Campaign definitions | id, task_name, addresser, subject, template_id, **track_open**, **track_click**, group_id, tag_ids, tag_logic |
| `recipient_info` | Task recipients | task_id, recipient, message_id, is_sent (0=pending, 2=ready, 1=sent) |
| `email_templates` | Email content | id, content, render data |
| `bm_contact_groups` | Contact list groups | id, name, double_optin, welcome_mail_*, confirm_mail_*, unsubscribe_* |
| `bm_contacts` | Individual subscribers | email, group_id, active, status (0=unconfirmed, 1=confirmed), last_active_at |
| `abnormal_recipient` | Bounce/failure tracking | recipient, count, description, add_type (1=manual, 2=auto, 3=scan) |
| `unsubscribe_records` | Opt-out records | email, group_id, template_id, task_id |

### 2.3 API Email Tables
| Table | Purpose |
|-------|---------|
| `api_templates` | API-based email configs | api_key, template_id, addresser, track_open, track_click |
| `api_mail_logs` | API email send history | api_id, recipient, message_id, status (0=pending, 2=sent, 3=failed) |
| `api_ip_whitelist` | API IP restrictions | api_id, ip |

---

## 3. EVENT PROCESSING PIPELINE

### 3.1 Maillog Event Handler
**File**: `core/internal/service/maillog_stat/maillog_stat.go` (848 lines)

**Functions**:
- `NewMallogEventHandler()` - Creates event listener for Postfix logs
- `Start()` - Begins processing logs in real-time (1 second interval)
- `AggregateMaillogs()` - Consolidates logs every 1 minute (timer in timers.go line 68)

### 3.2 Maillog Aggregation
**File**: `core/internal/service/maillog_stat/aggregate.go`

Aggregates logs from Postfix:
1. Gets last aggregation time from `bm_options.abnormal_recipient_last_time`
2. Queries domains and matches sender/recipient to domain
3. Processes failed mails to identify bounces

### 3.3 Overview Analytics
**File**: `core/internal/service/maillog_stat/overview.go` (800+ lines)

**Key Methods**:
```go
func (o *Overview) Overview() // Dashboard data
func (o *Overview) FailedListBounced(campaignID, domain, startTime, endTime) []map[string]interface{}
func (o *Overview) overviewDashboard() // Summary stats
func (o *Overview) chartSendMail() // Send count chart
func (o *Overview) chartBounceRate()
func (o *Overview) chartOpenRate() // Based on mailstat_opened
func (o *Overview) chartClickRate() // Based on mailstat_clicked
```

**Query Pattern**:
- Joins `mailstat_send_mails` with `mailstat_senders`
- Filters by time range (max 1 year), domain, campaign_id
- Joins with `recipient_info` to link to campaign tasks

### 3.4 Bounce Detection & Abnormal Recipient Management
**File**: `core/internal/service/abnormal_recipient/abnormal_recipient.go`

**Timer**: Runs every 30 minutes (`timers.go` line 137)

```go
func AbnormalRecipientAutoStat(ctx context.Context) {
    // 1. Check if abnormal detection enabled (bm_options.abnormal_mail_check_switch)
    // 2. Get failed list via FailedListBounced(0, "", lastTime, now)
    // 3. Batch upsert into abnormal_recipient table:
    //    - New entries: count=1, add_type=2
    //    - Existing: count+1
    // 4. Update last stat time
}
```

**Abnormal Recipient Model**:
```go
type AbnormalRecipient struct {
    Id          int    // Primary key
    Recipient   string // Email address (UNIQUE)
    Count       int    // Bounce count
    Description string // Error reason
    AddType     int    // 1=Manual, 2=Auto-stat, 3=Manual-scan
    CreateTime  int64
}
```

---

## 4. CAMPAIGN TASK EXECUTION

### 4.1 Task Executor
**File**: `core/api/batch_mail/v1/task_executor.go`

**Components**:
- `TaskExecutor` - Manages email sending with rate limiting
- `TaskConfig` - Thread count, speed (emails/min), min/max speed
- `SpeedMeter` - Tracks current sending rate

### 4.2 Batch Mail Service
**File**: `core/internal/service/batch_mail/batch_mail.go` (150+ lines shown)

**Key Functions**:
```go
type CreateTaskArgs struct {
    Addresser   string  // Sender email
    Subject     string
    TemplateId  int
    TrackOpen   int     // Enable open tracking (0/1)
    TrackClick  int     // Enable click tracking (0/1)
    IsRecord    int     // Save to outbox
    Unsubscribe int     // Include unsubscribe link
    Threads     int     // Concurrency
    GroupId     int     // Contact group
    TagIds      []int   // Tag-based filtering
    TagLogic    string  // "AND" / "OR"
}

func CreateTask(ctx, args) (int, error)     // Returns task_id
func GetTasksWithPage(page, pageSize, status) // Paginated listing
func DeleteTask(taskId)
```

### 4.3 API Email Sending
**File**: `core/internal/service/batch_mail/api_mail_send.go`

**Architecture**:
```go
type WorkerPool struct {
    workers int
    jobs    chan ApiMailLog
    cache   *CacheData  // Caches templates, contacts
}

func (p *WorkerPool) worker() {
    // Processes from job queue
    // Fetches: ApiTemplate, EmailTemplate, Contact
    // Processes content/subject (spintax, variable substitution)
    // Sends via SMTP
    // Updates api_mail_logs status
}

const (
    StatusPending = 0
    StatusSuccess = 2
    StatusFailed  = 3
)
```

---

## 5. TRACKING ARCHITECTURE

### 5.1 Campaign ID Encoding
- **Marketing tasks**: campaign_id < 1,000,000,000 (stored in `email_tasks.id`)
- **API campaigns**: campaign_id > 1,000,000,000 (API-based)

Distinction is made in `tracker.go` lines 66-77:
```go
if data.CampaignId > 1000000000 {
    // API campaign - groupId = 0
} else {
    // Marketing task - fetch groupId from email_tasks
}
```

### 5.2 Contact Activity Tracking
**File**: `core/internal/service/contact_activity/`

When email is opened or clicked:
```go
contact_activity.UpdateActivityByEmailAndGroup(recipient, groupId)
// Updates: bm_contacts.last_active_at = NOW()
```

### 5.3 Message ID Lookup
**Function**: `SearchPostfixMessageIdByMessageId()` (maillog_stat.go:841)
```go
func SearchPostfixMessageIdByMessageId(messageId string) (string, error) {
    // Queries mailstat_message_ids.postfix_message_id
    // WHERE message_id = ?
    return val.String(), nil
}
```

---

## 6. TIMERS & BACKGROUND JOBS

**File**: `core/internal/service/timers/timers.go`

Key timers related to tracking:

| Timer | Interval | Function | Lines |
|-------|----------|----------|-------|
| Maillog analysis | 1 second | `NewMallogEventHandler().Start()` | 61-64 |
| Maillog aggregation | 1 minute | `AggregateMaillogsTask()` | 67-69 |
| Task processing | 5 seconds | `ProcessEmailTasks()` | 97-99 |
| API mail queue | 1 minute | `ProcessApiMailQueueWithLock()` | 120-123 |
| Abnormal recipient scan | 30 minutes | `AbnormalRecipientAutoStat()` | 137-139 |
| Task stats update | 1 minute | `UpdateTaskJoinMailstat()` | 142-144 |

---

## 7. API ROUTING

**File**: `core/internal/cmd/cmd.go`

### 7.1 Event Tracking Route
```go
// Line 354-357: Email Campaign Tracker
s.BindHandler("/pmta/*any", func(r *ghttp.Request) {
    maillog_stat.CampaignEventHandler(r, r.Get("any").String())
})
```

### 7.2 API Endpoints (Protected)
**Routes**: `/api` group with JWT middleware (line 236)

Registered controllers (lines 247-265):
- `batch_mail.NewV1()` - Campaign management
- `abnormal_recipient.NewV1()` - Bounce list management
- `overview.NewV1()` - Analytics dashboard
- `contact.NewV1()` - Contact management
- `email_template.NewV1()` - Template management
- `settings.NewV1()` - System configuration

---

## 8. ENCRYPTION FOR TRACKING

**File**: `core/internal/service/maillog_stat/encryption.go`

```go
func Encrypt(data interface{}) string {
    // 1. JSON marshal data
    // 2. Generate random 16-byte key & IV
    // 3. AES-256-CBC encrypt with PKCS7 padding
    // 4. Interleave key & IV: [k0, i0, k1, i1, ..., k7, i7, k8-15, i8-15]
    // 5. Append to ciphertext: key_iv + ciphertext + key_iv_tail
    // 6. Base64 URL-safe encode, trim padding
    return base64.URLEncoding.EncodeToString(result)
}

func Decrypt(encStr string, result interface{}) error {
    // Reverse process
    // Extract key & IV from encoded data
    // Decrypt ciphertext
    // JSON unmarshal
}
```

---

## 9. CONTROLLER API STRUCTURE

### 9.1 Batch Mail Controller
**Directory**: `core/internal/controller/batch_mail/`

Endpoints (26 files):
- `batch_mail_v1_create_task.go` - Create campaign
- `batch_mail_v1_list_tasks.go` - List campaigns
- `batch_mail_v1_delete_task.go`
- `batch_mail_v1_task_info.go` - Get task details
- `batch_mail_v1_get_task_mail_logs.go` - View events for task
- `batch_mail_v1_get_task_send_count.go` - Send count stats
- `batch_mail_v1_task_stat_chart.go` - Open/click charts
- `batch_mail_v1_task_overview.go` - Dashboard
- `batch_mail_v1_task_mail_provider_stat.go` - Provider breakdown
- `batch_mail_v1_pause_task.go` / `_resume_task.go`
- `batch_mail_v1_update_task_speed.go`
- `batch_mail_v1_api_mail_send.go` - API send endpoint
- `batch_mail_v1_api_mail_batch_send.go` - Batch API send
- `batch_mail_v1_send_test_email.go`
- And more...

### 9.2 Abnormal Recipient Controller
**Directory**: `core/internal/controller/abnormal_recipient/` (11 files)

Endpoints:
- `abnormal_recipient_v1_list_abnormal_recipient.go` - List bounces
- `abnormal_recipient_v1_add_abnormal_recipient.go` - Manual add
- `abnormal_recipient_v1_delete_abnormal_recipient.go`
- `abnormal_recipient_v1_clearabnormal_recipient.go` - Bulk clear
- `abnormal_recipient_v1_set_abnormal_switch.go` - Enable/disable scan
- `abnormal_recipient_v1_abnormal_switch.go` - Check status
- `abnormal_recipient_v1_get_scan_log.go` - Scan history

---

## 10. DATABASE OPTIONS TABLE

**Location**: `bm_options` (generic key-value store)

Used for configuration & state:

| Option Name | Purpose |
|-------------|---------|
| `abnormal_mail_check_switch` | Enable/disable automatic bounce detection (value="0" or "1") |
| `abnormal_recipient_last_time` | Last abnormal recipient scan timestamp |
| `last_maillog_aggregate_time_millis` | Last maillog aggregation timestamp |
| `rspamd_worker_controller_password` | Rspamd auth |
| `API_DOC_ENABLED` | Swagger UI enabled |

---

## 11. CURRENT TRACKING CAPABILITIES

✅ **Implemented**:
- Email open tracking via pixel image requests
- Link click tracking via URL wrapping
- Bounce/hard failure detection via Postfix logs
- Deferred delivery tracking
- Complaint/FBL tracking
- Campaign-level analytics (open rate, click rate, bounce rate)
- Provider-level breakdown (Gmail, Outlook, Yahoo, etc.)
- Contact activity tracking (last_active_at)
- Abnormal recipient management (manual + automatic)
- API-based email sending with separate tracking

❌ **Not Implemented**:
- Webhook/callback notifications to external systems
- Real-time event streaming
- Custom event types beyond open/click/bounce
- Event replay/retry logic
- Dead letter queue for failed webhook deliveries

---

## 12. WEBHOOK BRANCH STATUS

**Current State**: 
- Branch: `copilot/add-email-webhooks-support`
- Only 1 commit: "Initial plan"
- No code changes yet

**Recommendation**: This branch should be used to implement webhook infrastructure following the patterns established in the existing tracking system.

---

## 13. KEY FILES REFERENCE

| File | Purpose | Lines |
|------|---------|-------|
| `core/internal/service/maillog_stat/tracker.go` | Campaign event handler, tracking URL/pixel generation | 238 |
| `core/internal/service/maillog_stat/maillog_stat.go` | Event log processing, aggregation | 848 |
| `core/internal/service/maillog_stat/overview.go` | Analytics queries | 800+ |
| `core/internal/service/abnormal_recipient/abnormal_recipient.go` | Bounce management | 297 |
| `core/internal/service/batch_mail/batch_mail.go` | Campaign service | ~300 |
| `core/internal/service/batch_mail/api_mail_send.go` | API email worker pool | ~400 |
| `core/internal/service/timers/timers.go` | Background job scheduler | 199 |
| `core/internal/cmd/cmd.go` | API routing, server setup | 470 |
| `core/internal/service/database_initialization/batch_mail.go` | Campaign DB schema | 274 |
| `core/internal/service/database_initialization/maillog_stat.go` | Event DB schema | 159 |
| `core/internal/model/entity/batch_mail.go` | Campaign data models | 189 |

---

## 14. DESIGN PATTERNS OBSERVED

1. **Service Layer Pattern**: Business logic in `service/` packages, exposed via controllers
2. **Timer-based Processing**: Background jobs via GoFrame's gtimer
3. **Event Sourcing**: All email events stored in dedicated maillog tables
4. **Encryption**: URLs encrypted to prevent tampering/disclosure
5. **Rate Limiting**: Task executor with configurable speed control
6. **Caching**: Worker pool caches templates/contacts to reduce DB queries
7. **Pagination**: API results paginated (page, pageSize parameters)
8. **Options Pattern**: Flexible config via `bm_options` key-value table

