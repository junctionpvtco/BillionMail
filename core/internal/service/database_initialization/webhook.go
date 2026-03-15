package database_initialization

import (
	"context"
	"github.com/gogf/gf/v2/frame/g"
)

func init() {
	registerHandler(func() {
		sqlList := []string{
			`-- Webhook endpoints configuration
			CREATE TABLE IF NOT EXISTS bm_webhooks (
				id SERIAL PRIMARY KEY,
				url TEXT NOT NULL,
				secret VARCHAR(255) NOT NULL DEFAULT '',
				events TEXT NOT NULL DEFAULT '[]',
				active SMALLINT NOT NULL DEFAULT 1,
				description VARCHAR(255) NOT NULL DEFAULT '',
				create_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
				update_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())
			)`,

			`-- Webhook event log
			CREATE TABLE IF NOT EXISTS bm_webhook_logs (
				id SERIAL PRIMARY KEY,
				webhook_id INTEGER NOT NULL,
				event_type VARCHAR(50) NOT NULL,
				payload TEXT NOT NULL DEFAULT '',
				response_status INTEGER NOT NULL DEFAULT 0,
				response_body TEXT NOT NULL DEFAULT '',
				attempts INTEGER NOT NULL DEFAULT 0,
				next_retry INTEGER NOT NULL DEFAULT 0,
				status SMALLINT NOT NULL DEFAULT 0,
				create_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
				update_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
				FOREIGN KEY (webhook_id) REFERENCES bm_webhooks(id) ON DELETE CASCADE
			)`,

			`CREATE INDEX IF NOT EXISTS idx_bm_webhook_logs_webhook_id ON bm_webhook_logs(webhook_id)`,
			`CREATE INDEX IF NOT EXISTS idx_bm_webhook_logs_status ON bm_webhook_logs(status, next_retry)`,
			`CREATE INDEX IF NOT EXISTS idx_bm_webhook_logs_event_type ON bm_webhook_logs(event_type)`,
			`CREATE INDEX IF NOT EXISTS idx_bm_webhook_logs_create_time ON bm_webhook_logs(create_time)`,
		}

		for _, sql := range sqlList {
			_, err := g.DB().Exec(context.Background(), sql)
			if err != nil {
				g.Log().Error(context.Background(), "Failed to create webhook tables:", err)
				return
			}
		}

		g.Log().Info(context.Background(), "Webhook tables initialized successfully")
	})
}
