package v1

import (
	"billionmail-core/utility/types/api_v1"

	"github.com/gogf/gf/v2/frame/g"
)

// CreateWebhookReq Create webhook endpoint
type CreateWebhookReq struct {
	g.Meta        `path:"/webhook/create" tags:"Webhook" method:"post" summary:"Create a webhook endpoint"`
	Authorization string   `json:"authorization" dc:"Authorization" in:"header"`
	URL           string   `json:"url" v:"required|url" dc:"Webhook URL"`
	Secret        string   `json:"secret" dc:"Signing secret for HMAC-SHA256 verification"`
	Events        []string `json:"events" v:"required" dc:"Event types to subscribe to"`
	Description   string   `json:"description" dc:"Description"`
}

type CreateWebhookRes struct {
	api_v1.StandardRes
	Data struct {
		ID int `json:"id" dc:"Created webhook ID"`
	} `json:"data" dc:"Created webhook data"`
}

// UpdateWebhookReq Update webhook endpoint
type UpdateWebhookReq struct {
	g.Meta        `path:"/webhook/update" tags:"Webhook" method:"post" summary:"Update a webhook endpoint"`
	Authorization string   `json:"authorization" dc:"Authorization" in:"header"`
	ID            int      `json:"id" v:"required|min:1" dc:"Webhook ID"`
	URL           string   `json:"url" v:"required|url" dc:"Webhook URL"`
	Secret        string   `json:"secret" dc:"Signing secret for HMAC-SHA256 verification"`
	Events        []string `json:"events" v:"required" dc:"Event types to subscribe to"`
	Active        int      `json:"active" dc:"Active status (0: disabled, 1: enabled)"`
	Description   string   `json:"description" dc:"Description"`
}

type UpdateWebhookRes struct {
	api_v1.StandardRes
}

// DeleteWebhookReq Delete webhook endpoint
type DeleteWebhookReq struct {
	g.Meta        `path:"/webhook/delete" tags:"Webhook" method:"post" summary:"Delete a webhook endpoint"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	ID            int    `json:"id" v:"required|min:1" dc:"Webhook ID"`
}

type DeleteWebhookRes struct {
	api_v1.StandardRes
}

// ListWebhooksReq List webhook endpoints
type ListWebhooksReq struct {
	g.Meta        `path:"/webhook/list" tags:"Webhook" method:"get" summary:"List webhook endpoints"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	Page          int    `json:"page" dc:"Page number" d:"1"`
	PageSize      int    `json:"page_size" dc:"Page size" d:"20"`
}

type ListWebhooksRes struct {
	api_v1.StandardRes
	Data api_v1.StandardPagination `json:"data" dc:"Webhook list data"`
}

// GetWebhookLogsReq Get webhook delivery logs
type GetWebhookLogsReq struct {
	g.Meta        `path:"/webhook/logs" tags:"Webhook" method:"get" summary:"Get webhook delivery logs"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	WebhookID     int    `json:"webhook_id" dc:"Filter by webhook ID (0 for all)"`
	Page          int    `json:"page" dc:"Page number" d:"1"`
	PageSize      int    `json:"page_size" dc:"Page size" d:"20"`
}

type GetWebhookLogsRes struct {
	api_v1.StandardRes
	Data api_v1.StandardPagination `json:"data" dc:"Webhook logs data"`
}

// GetEventTypesReq Get supported event types
type GetEventTypesReq struct {
	g.Meta        `path:"/webhook/event_types" tags:"Webhook" method:"get" summary:"Get supported webhook event types"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
}

type GetEventTypesRes struct {
	api_v1.StandardRes
	Data []string `json:"data" dc:"List of supported event types"`
}
