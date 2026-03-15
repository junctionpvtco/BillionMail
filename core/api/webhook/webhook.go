// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package webhook

import (
	"context"

	"billionmail-core/api/webhook/v1"
)

type IWebhookV1 interface {
	CreateWebhook(ctx context.Context, req *v1.CreateWebhookReq) (res *v1.CreateWebhookRes, err error)
	UpdateWebhook(ctx context.Context, req *v1.UpdateWebhookReq) (res *v1.UpdateWebhookRes, err error)
	DeleteWebhook(ctx context.Context, req *v1.DeleteWebhookReq) (res *v1.DeleteWebhookRes, err error)
	ListWebhooks(ctx context.Context, req *v1.ListWebhooksReq) (res *v1.ListWebhooksRes, err error)
	GetWebhookLogs(ctx context.Context, req *v1.GetWebhookLogsReq) (res *v1.GetWebhookLogsRes, err error)
	GetEventTypes(ctx context.Context, req *v1.GetEventTypesReq) (res *v1.GetEventTypesRes, err error)
}
