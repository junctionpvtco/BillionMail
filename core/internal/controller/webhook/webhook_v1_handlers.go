package webhook

import (
	v1 "billionmail-core/api/webhook/v1"
	"billionmail-core/internal/service/public"
	webhookService "billionmail-core/internal/service/webhook"
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
)

func (c *ControllerV1) CreateWebhook(ctx context.Context, req *v1.CreateWebhookReq) (res *v1.CreateWebhookRes, err error) {
	res = &v1.CreateWebhookRes{}

	// Validate events
	validEvents := webhookService.AllEventTypes()
	for _, e := range req.Events {
		if !isValidEvent(e, validEvents) {
			res.SetError(gerror.Newf("Invalid event type: %s", e))
			return
		}
	}

	id, err := webhookService.CreateWebhook(ctx, req.URL, req.Secret, req.Events, req.Description)
	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to create webhook: {}", err)))
		return
	}

	res.Data.ID = id
	res.SetSuccess(public.LangCtx(ctx, "Webhook created successfully"))
	return
}

func (c *ControllerV1) UpdateWebhook(ctx context.Context, req *v1.UpdateWebhookReq) (res *v1.UpdateWebhookRes, err error) {
	res = &v1.UpdateWebhookRes{}

	// Validate events
	validEvents := webhookService.AllEventTypes()
	for _, e := range req.Events {
		if !isValidEvent(e, validEvents) {
			res.SetError(gerror.Newf("Invalid event type: %s", e))
			return
		}
	}

	err = webhookService.UpdateWebhook(ctx, req.ID, req.URL, req.Secret, req.Events, req.Active, req.Description)
	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to update webhook: {}", err)))
		return
	}

	res.SetSuccess(public.LangCtx(ctx, "Webhook updated successfully"))
	return
}

func (c *ControllerV1) DeleteWebhook(ctx context.Context, req *v1.DeleteWebhookReq) (res *v1.DeleteWebhookRes, err error) {
	res = &v1.DeleteWebhookRes{}

	err = webhookService.DeleteWebhook(ctx, req.ID)
	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to delete webhook: {}", err)))
		return
	}

	res.SetSuccess(public.LangCtx(ctx, "Webhook deleted successfully"))
	return
}

func (c *ControllerV1) ListWebhooks(ctx context.Context, req *v1.ListWebhooksReq) (res *v1.ListWebhooksRes, err error) {
	res = &v1.ListWebhooksRes{}

	total, list, err := webhookService.ListWebhooks(ctx, req.Page, req.PageSize)
	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to list webhooks: {}", err)))
		return
	}

	res.Data.Total = total
	res.Data.List = list
	res.SetSuccess(public.LangCtx(ctx, "Success"))
	return
}

func (c *ControllerV1) GetWebhookLogs(ctx context.Context, req *v1.GetWebhookLogsReq) (res *v1.GetWebhookLogsRes, err error) {
	res = &v1.GetWebhookLogsRes{}

	total, list, err := webhookService.GetWebhookLogs(ctx, req.WebhookID, req.Page, req.PageSize)
	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to get webhook logs: {}", err)))
		return
	}

	res.Data.Total = total
	res.Data.List = list
	res.SetSuccess(public.LangCtx(ctx, "Success"))
	return
}

func (c *ControllerV1) GetEventTypes(ctx context.Context, req *v1.GetEventTypesReq) (res *v1.GetEventTypesRes, err error) {
	res = &v1.GetEventTypesRes{}
	res.Data = webhookService.AllEventTypes()
	res.SetSuccess(public.LangCtx(ctx, "Success"))
	return
}

func isValidEvent(event string, validEvents []string) bool {
	for _, e := range validEvents {
		if e == event {
			return true
		}
	}
	return false
}
