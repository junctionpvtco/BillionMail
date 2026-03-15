<template>
	<div class="max-w-5xl mx-auto px-4 py-8">
		<!-- Webhook Endpoints Card -->
		<n-card class="mb-8" :bordered="false">
			<template #header>
				<div class="flex items-center justify-between">
					<div class="flex items-center">
						<n-icon class="mr-2 text-primary" size="20">
							<i class="i-mdi-webhook"></i>
						</n-icon>
						<span class="text-lg font-medium">{{ t('webhook.title') }}</span>
					</div>
					<n-button type="primary" size="small" @click="openCreateModal">
						<template #icon>
							<n-icon><i class="i-mdi-plus"></i></n-icon>
						</template>
						{{ t('webhook.addWebhook') }}
					</n-button>
				</div>
			</template>
			<template #header-extra>
				<n-text depth="3" class="text-sm">{{ t('webhook.description') }}</n-text>
			</template>

			<n-data-table
				:columns="webhookColumns"
				:data="webhooks"
				:loading="loading"
				:bordered="false"
				size="small"
			/>
		</n-card>

		<!-- Webhook Logs Card -->
		<n-card :bordered="false">
			<template #header>
				<div class="flex items-center">
					<n-icon class="mr-2 text-primary" size="20">
						<i class="i-mdi-format-list-bulleted"></i>
					</n-icon>
					<span class="text-lg font-medium">{{ t('webhook.logs.title') }}</span>
				</div>
			</template>

			<div class="mb-4">
				<n-select
					v-model:value="logFilter.webhookId"
					:options="webhookFilterOptions"
					:placeholder="t('webhook.logs.filterByWebhook')"
					clearable
					size="small"
					style="width: 250px"
					@update:value="fetchLogs"
				/>
			</div>

			<n-data-table
				:columns="logColumns"
				:data="logs"
				:loading="logsLoading"
				:bordered="false"
				size="small"
				:pagination="logPagination"
				@update:page="handleLogPageChange"
			/>
		</n-card>

		<!-- Create/Edit Webhook Modal -->
		<n-modal v-model:show="showModal" :title="isEditing ? t('webhook.editWebhook') : t('webhook.addWebhook')" preset="dialog" style="width: 600px">
			<n-form ref="formRef" :model="formData" :rules="formRules" label-placement="left" label-width="auto">
				<n-form-item :label="t('webhook.form.url')" path="url">
					<n-input v-model:value="formData.url" :placeholder="t('webhook.form.urlPlaceholder')" />
				</n-form-item>
				<n-form-item :label="t('webhook.form.secret')" path="secret">
					<n-input v-model:value="formData.secret" :placeholder="t('webhook.form.secretPlaceholder')" type="password" show-password-on="click" />
				</n-form-item>
				<n-form-item :label="t('webhook.form.events')" path="events">
					<n-checkbox-group v-model:value="formData.events">
						<n-space>
							<n-checkbox v-for="evt in eventTypes" :key="evt" :value="evt" :label="evt" />
						</n-space>
					</n-checkbox-group>
				</n-form-item>
				<n-form-item :label="t('webhook.form.description')" path="description">
					<n-input v-model:value="formData.description" :placeholder="t('webhook.form.descriptionPlaceholder')" type="textarea" :rows="2" />
				</n-form-item>
				<n-form-item v-if="isEditing" :label="t('webhook.form.active')">
					<n-switch v-model:value="formData.active" :checked-value="1" :unchecked-value="0" />
				</n-form-item>
			</n-form>
			<template #action>
				<n-button @click="showModal = false">{{ t('common.actions.cancel') }}</n-button>
				<n-button type="primary" :loading="submitting" @click="handleSubmit">{{ t('common.actions.save') }}</n-button>
			</template>
		</n-modal>
	</div>
</template>

<script lang="ts" setup>
import { NButton, NTag, NSpace, NPopconfirm, type DataTableColumns, type FormInst } from 'naive-ui'
import {
	listWebhooks,
	createWebhook,
	updateWebhook,
	deleteWebhook,
	getWebhookLogs,
	getEventTypes,
	type WebhookEndpoint,
	type WebhookLog,
} from '@/api/modules/settings/webhook'
import { isObject } from '@/utils'

const { t } = useI18n()

// State
const webhooks = ref<WebhookEndpoint[]>([])
const logs = ref<WebhookLog[]>([])
const eventTypes = ref<string[]>([])
const loading = ref(false)
const logsLoading = ref(false)
const showModal = ref(false)
const isEditing = ref(false)
const submitting = ref(false)
const formRef = ref<FormInst | null>(null)

const logFilter = reactive({
	webhookId: null as number | null,
})

const logPagination = reactive({
	page: 1,
	pageSize: 10,
	itemCount: 0,
	showSizePicker: false,
})

const formData = reactive({
	id: 0,
	url: '',
	secret: '',
	events: [] as string[],
	active: 1,
	description: '',
})

const formRules = {
	url: [{ required: true, message: t('webhook.form.urlRequired'), trigger: 'blur' }],
	events: [
		{
			type: 'array' as const,
			required: true,
			message: t('webhook.form.eventsRequired'),
			trigger: 'change',
		},
	],
}

// Computed
const webhookFilterOptions = computed(() => {
	const options = [{ label: t('webhook.logs.allWebhooks'), value: 0 }]
	webhooks.value.forEach((w) => {
		options.push({ label: w.description || w.url, value: w.id })
	})
	return options
})

const webhookColumns: DataTableColumns<WebhookEndpoint> = [
	{
		title: () => t('webhook.table.url'),
		key: 'url',
		ellipsis: { tooltip: true },
		width: 250,
	},
	{
		title: () => t('webhook.table.events'),
		key: 'events',
		width: 250,
		render(row) {
			return h(
				NSpace,
				{ size: 'small' },
				{
					default: () =>
						(row.events || []).map((evt: string) =>
							h(NTag, { size: 'small', type: 'info', bordered: false }, { default: () => evt })
						),
				}
			)
		},
	},
	{
		title: () => t('webhook.table.status'),
		key: 'active',
		width: 100,
		render(row) {
			return h(
				NTag,
				{ type: row.active === 1 ? 'success' : 'default', size: 'small' },
				{ default: () => (row.active === 1 ? t('webhook.table.enabled') : t('webhook.table.disabled')) }
			)
		},
	},
	{
		title: () => t('webhook.table.description'),
		key: 'description',
		ellipsis: { tooltip: true },
	},
	{
		title: () => t('webhook.table.actions'),
		key: 'actions',
		width: 160,
		render(row) {
			return h(NSpace, { size: 'small' }, {
				default: () => [
					h(
						NButton,
						{ size: 'tiny', onClick: () => openEditModal(row) },
						{ default: () => t('common.actions.edit') }
					),
					h(
						NPopconfirm,
						{ onPositiveClick: () => handleDelete(row.id) },
						{
							trigger: () =>
								h(
									NButton,
									{ size: 'tiny', type: 'error' },
									{ default: () => t('common.actions.delete') }
								),
							default: () => t('webhook.deleteConfirm'),
						}
					),
				],
			})
		},
	},
]

const logColumns: DataTableColumns<WebhookLog> = [
	{ title: () => t('webhook.logs.eventType'), key: 'event_type', width: 120 },
	{
		title: () => t('webhook.logs.status'),
		key: 'status',
		width: 100,
		render(row) {
			const typeMap: Record<number, 'success' | 'warning' | 'error'> = {
				0: 'warning',
				1: 'success',
				2: 'error',
			}
			const labelMap: Record<number, string> = {
				0: t('webhook.logs.pending'),
				1: t('webhook.logs.success'),
				2: t('webhook.logs.failed'),
			}
			return h(
				NTag,
				{ type: typeMap[row.status] || 'default', size: 'small' },
				{ default: () => labelMap[row.status] || String(row.status) }
			)
		},
	},
	{ title: () => t('webhook.logs.httpStatus'), key: 'response_status', width: 100 },
	{ title: () => t('webhook.logs.attempts'), key: 'attempts', width: 80 },
	{
		title: () => t('webhook.logs.time'),
		key: 'create_time',
		width: 180,
		render(row) {
			return new Date(row.create_time * 1000).toLocaleString()
		},
	},
]

// Methods
const fetchWebhooks = async () => {
	loading.value = true
	try {
		const res = await listWebhooks({ page: 1, page_size: 100 })
		if (isObject(res) && Array.isArray(res.list)) {
			webhooks.value = res.list
		}
	} finally {
		loading.value = false
	}
}

const fetchLogs = async () => {
	logsLoading.value = true
	try {
		const res = await getWebhookLogs({
			webhook_id: logFilter.webhookId || 0,
			page: logPagination.page,
			page_size: logPagination.pageSize,
		})
		if (isObject(res)) {
			logs.value = (res as any).list || []
			logPagination.itemCount = (res as any).total || 0
		}
	} finally {
		logsLoading.value = false
	}
}

const fetchEventTypes = async () => {
	try {
		const res = await getEventTypes()
		if (Array.isArray(res)) {
			eventTypes.value = res
		}
	} catch {
		eventTypes.value = ['delivery', 'bounce', 'open', 'click', 'unsubscribe', 'complaint', 'deferral']
	}
}

const handleLogPageChange = (page: number) => {
	logPagination.page = page
	fetchLogs()
}

const openCreateModal = () => {
	isEditing.value = false
	formData.id = 0
	formData.url = ''
	formData.secret = ''
	formData.events = []
	formData.active = 1
	formData.description = ''
	showModal.value = true
}

const openEditModal = (row: WebhookEndpoint) => {
	isEditing.value = true
	formData.id = row.id
	formData.url = row.url
	formData.secret = row.secret
	formData.events = [...row.events]
	formData.active = row.active
	formData.description = row.description
	showModal.value = true
}

const handleSubmit = async () => {
	try {
		await formRef.value?.validate()
	} catch {
		return
	}

	submitting.value = true
	try {
		if (isEditing.value) {
			await updateWebhook({
				id: formData.id,
				url: formData.url,
				secret: formData.secret,
				events: formData.events,
				active: formData.active,
				description: formData.description,
			})
		} else {
			await createWebhook({
				url: formData.url,
				secret: formData.secret,
				events: formData.events,
				description: formData.description,
			})
		}
		showModal.value = false
		await fetchWebhooks()
	} finally {
		submitting.value = false
	}
}

const handleDelete = async (id: number) => {
	await deleteWebhook({ id })
	await fetchWebhooks()
}

// Init
onMounted(() => {
	fetchWebhooks()
	fetchLogs()
	fetchEventTypes()
})
</script>
