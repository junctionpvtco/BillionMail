package webhook

import (
	"billionmail-core/api/webhook"
)

type ControllerV1 struct{}

func NewV1() webhook.IWebhookV1 {
	return &ControllerV1{}
}
