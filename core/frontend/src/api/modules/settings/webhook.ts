import { instance } from '@/api'
import i18n from '@/i18n'

const { t } = i18n.global

export interface WebhookEndpoint {
	id: number
	url: string
	secret: string
	events: string[]
	active: number
	description: string
	create_time: number
	update_time: number
}

export interface WebhookLog {
	id: number
	webhook_id: number
	event_type: string
	payload: string
	response_status: number
	response_body: string
	attempts: number
	status: number
	create_time: number
	update_time: number
}

export const listWebhooks = (params?: { page?: number; page_size?: number }) => {
	return instance.get('/webhook/list', { params })
}

export const createWebhook = (params: {
	url: string
	secret: string
	events: string[]
	description: string
}) => {
	return instance.post('/webhook/create', params, {
		fetchOptions: {
			loading: t('webhook.api.creating'),
			successMessage: true,
		},
	})
}

export const updateWebhook = (params: {
	id: number
	url: string
	secret: string
	events: string[]
	active: number
	description: string
}) => {
	return instance.post('/webhook/update', params, {
		fetchOptions: {
			loading: t('webhook.api.updating'),
			successMessage: true,
		},
	})
}

export const deleteWebhook = (params: { id: number }) => {
	return instance.post('/webhook/delete', params, {
		fetchOptions: {
			loading: t('webhook.api.deleting'),
			successMessage: true,
		},
	})
}

export const getWebhookLogs = (params?: {
	webhook_id?: number
	page?: number
	page_size?: number
}) => {
	return instance.get('/webhook/logs', { params })
}

export const getEventTypes = () => {
	return instance.get('/webhook/event_types')
}
