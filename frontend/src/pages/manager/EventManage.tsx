import axios from 'axios'
import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, XCircle, Globe, Edit, Trash2, Clock } from 'lucide-react'
import toast from 'react-hot-toast'
import api from '../../api/client'

type EventStatus = 'draft' | 'published' | 'closed' | 'ended'

type TicketTypeForm = {
  fieldKey: string
  name: string
  total_quota: number
}

type TicketTypeSummary = Omit<TicketTypeForm, 'fieldKey'> & {
  id?: string
  remaining?: number
}

type EventForm = {
  title: string
  description: string
  venue: string
  image_url: string
  document_url: string
  publish_time: string
  start_time: string
  apply_deadline: string
  end_time: string
  region_restriction: string
  max_tickets_per_person: number
  ticket_types: TicketTypeForm[]
}

type EventPayload = Omit<EventForm, 'region_restriction' | 'ticket_types'> & {
  region_restriction: string | null
  ticket_types: Array<Omit<TicketTypeForm, 'fieldKey'>>
}

type ManagedEvent = {
  id: string
  title: string
  description?: string
  venue?: string
  status: EventStatus
  image_url?: string
  document_url?: string
  publish_time?: string
  start_time?: string
  apply_deadline?: string
  end_time?: string
  region_restriction?: string
  max_tickets_per_person?: number
  ticket_types?: TicketTypeSummary[]
}

type ApiErrorResponse = {
  error?: {
    message?: string
  }
}

type TicketTypeField = keyof Omit<TicketTypeForm, 'fieldKey'>

const EVENT_STATUS_LABELS: Record<EventStatus, string> = {
  draft: '草稿',
  published: '發布中',
  closed: '已截止',
  ended: '已結束',
}

function getErrorMessage(error: unknown, fallback: string) {
  if (axios.isAxiosError<ApiErrorResponse>(error)) {
    return error.response?.data?.error?.message ?? fallback
  }

  if (typeof error === 'object' && error !== null && 'response' in error) {
    return (error as { response?: { data?: ApiErrorResponse } }).response?.data?.error?.message ?? fallback
  }

  return fallback
}

function getUploadErrorMessage(error: unknown, fallback: string) {
  const apiMessage = getErrorMessage(error, '')
  if (apiMessage) {
    return apiMessage
  }

  if (error instanceof Error && error.message) {
    return error.message
  }

  return fallback
}

function formatDateOnly(value?: string) {
  if (!value) {
    return ''
  }

  return new Date(value).toLocaleDateString('zh-TW')
}

function getRemainingColor(remaining = 0) {
  if (remaining > 5) {
    return 'var(--success)'
  }

  return 'var(--danger)'
}

function renderStatusBadge(status: EventStatus, publishTime?: string) {
  const publishDate = publishTime ? new Date(publishTime) : null
  const hasValidPublishDate = publishDate !== null && Number.isFinite(publishDate.getTime())
  const isScheduled = status === 'draft' && hasValidPublishDate && publishDate > new Date()
  const isPendingPublish = status === 'draft' && hasValidPublishDate && publishDate <= new Date()

  if (isScheduled) {
    return <span className="badge badge-draft">
      <Clock size={12} /> 排程中
    </span>
  }

  if (isPendingPublish) {
    return <span className="badge badge-pending">
      <Clock size={12} /> 發布處理中
    </span>
  }

  return <span className={`badge badge-${status}`}>{EVENT_STATUS_LABELS[status]}</span>
}

let ticketTypeFieldCounter = 0

function createTicketTypeForm(name: string, totalQuota: number, fieldKey?: string): TicketTypeForm {
  ticketTypeFieldCounter += 1
  return { fieldKey: fieldKey ?? `ticket-type-${ticketTypeFieldCounter}`, name, total_quota: totalQuota }
}

const EMPTY_FORM = {
  title: '', description: '', venue: '',
  image_url: '',
  document_url: '',
  publish_time: '', start_time: '', apply_deadline: '', end_time: '',
  region_restriction: '', max_tickets_per_person: 1,
  ticket_types: [createTicketTypeForm('一般票', 100)],
} satisfies EventForm

function createEmptyForm(): EventForm {
  return {
    ...EMPTY_FORM,
    ticket_types: [createTicketTypeForm('一般票', 100)],
  }
}

function validatePublishTimeline(event: Pick<ManagedEvent, 'apply_deadline' | 'start_time' | 'end_time'>, publishAt = new Date()): string | null {
  const applyDeadline = new Date(event.apply_deadline ?? '')
  const startTime = new Date(event.start_time ?? '')
  const endTime = new Date(event.end_time ?? '')

  if (publishAt >= applyDeadline) {
    return '發布時間必須早於申請截止時間'
  }

  if (startTime >= endTime) {
    return '活動結束時間必須晚於開始時間'
  }

  return null
}

export default function EventManage() {
  const qc = useQueryClient()
  const [showCreate, setShowCreate] = useState(false)
  const [editId, setEditId] = useState<string | null>(null)
  const [form, setForm] = useState<EventForm>(() => createEmptyForm())

  const closeEventModal = () => {
    setShowCreate(false)
  }

  const resetEventForm = () => {
    setEditId(null)
    setForm(createEmptyForm())
  }

  const openCreateModal = () => {
    resetEventForm()
    setShowCreate(true)
  }

  const formatToLocalDatetime = (isoString?: string) => {
    if (!isoString) return ''
    const date = new Date(isoString)
    if (!Number.isFinite(date.getTime())) return ''
    return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
  }

  const { data, isLoading } = useQuery({
    queryKey: ['events-manage'],
    queryFn: () => api.get<{ data: ManagedEvent[] }>('/events').then(r => r.data.data),
  })

  const createMutation = useMutation({
    mutationFn: (payload: EventPayload) => api.post('/events', payload),
    onSuccess: () => {
      toast.success('活動建立成功！')
      closeEventModal()
      resetEventForm()
      qc.invalidateQueries({ queryKey: ['events-manage'] })
      qc.invalidateQueries({ queryKey: ['events'] })
    },
    onError: error => toast.error(getErrorMessage(error, '建立失敗')),
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: EventPayload }) => api.put(`/events/${id}`, payload),
    onSuccess: () => {
      toast.success('活動更新成功！')
      closeEventModal()
      resetEventForm()
      qc.invalidateQueries({ queryKey: ['events-manage'] })
      qc.invalidateQueries({ queryKey: ['events'] })
    },
    onError: error => toast.error(getErrorMessage(error, '更新失敗')),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/events/${id}`),
    onSuccess: () => {
      toast.success('活動已刪除！')
      qc.invalidateQueries({ queryKey: ['events-manage'] })
    },
    onError: error => toast.error(getErrorMessage(error, '刪除失敗')),
  })

  const publishMutation = useMutation({
    mutationFn: (id: string) => api.patch(`/events/${id}/publish`),
    onSuccess: () => {
      toast.success('活動已發布！')
      qc.invalidateQueries({ queryKey: ['events-manage'] })
    },
    onError: error => toast.error(getErrorMessage(error, '發布失敗')),
  })

  const closeMutation = useMutation({
    mutationFn: (id: string) => api.patch(`/events/${id}/close`),
    onSuccess: () => {
      toast.success('已截止報名')
      qc.invalidateQueries({ queryKey: ['events-manage'] })
    },
  })

  const uploadImageMutation = useMutation({
    mutationFn: async (file: File) => {
      if (file.size > 5 * 1024 * 1024) throw new Error('檔案大小不能超過 5MB')
      if (!file.type.startsWith('image/')) throw new Error('請上傳圖片檔案')
      const fd = new FormData()
      fd.append('file', file)
      const res = await api.post<{ data: { url: string } }>('/upload', fd, { headers: { 'Content-Type': 'multipart/form-data' } })
      return res.data.data.url
    },
    onSuccess: (url) => {
      toast.success('圖片上傳成功！')
      setForm(f => ({ ...f, image_url: url }))
    },
    onError: error => toast.error(getUploadErrorMessage(error, '上傳失敗'))
  })

  const uploadDocMutation = useMutation({
    mutationFn: async (file: File) => {
      if (file.size > 5 * 1024 * 1024) throw new Error('檔案大小不能超過 5MB')
      if (file.type !== 'application/pdf') throw new Error('請上傳 PDF 檔案')
      const fd = new FormData()
      fd.append('file', file)
      const res = await api.post<{ data: { url: string } }>('/upload', fd, { headers: { 'Content-Type': 'multipart/form-data' } })
      return res.data.data.url
    },
    onSuccess: (url) => {
      toast.success('附件上傳成功！')
      setForm(f => ({ ...f, document_url: url }))
    },
    onError: error => toast.error(getUploadErrorMessage(error, '上傳失敗'))
  })

  const handleSave = () => {
    const payload: EventPayload = {
      ...form,
      publish_time: form.publish_time ? new Date(form.publish_time).toISOString() : '',
      start_time: form.start_time ? new Date(form.start_time).toISOString() : '',
      apply_deadline: form.apply_deadline ? new Date(form.apply_deadline).toISOString() : '',
      end_time: form.end_time ? new Date(form.end_time).toISOString() : '',
      region_restriction: form.region_restriction || null,
      ticket_types: form.ticket_types.map(tt => ({ name: tt.name, total_quota: Number(tt.total_quota) })),
    }
    if (editId) {
      updateMutation.mutate({ id: editId, payload })
    } else {
      createMutation.mutate(payload)
    }
  }

  const handleEdit = (event: ManagedEvent) => {
    setForm({
      title: event.title || '',
      description: event.description || '',
      venue: event.venue || '',
      image_url: event.image_url || '',
      document_url: event.document_url || '',
      publish_time: formatToLocalDatetime(event.publish_time),
      start_time: formatToLocalDatetime(event.start_time),
      apply_deadline: formatToLocalDatetime(event.apply_deadline),
      end_time: formatToLocalDatetime(event.end_time),
      region_restriction: event.region_restriction || '',
      max_tickets_per_person: event.max_tickets_per_person || 1,
      ticket_types: event.ticket_types && event.ticket_types.length > 0
        ? event.ticket_types.map(tt => createTicketTypeForm(tt.name, tt.total_quota, tt.id))
        : [createTicketTypeForm('一般票', 100)],
    })
    setEditId(event.id)
    setShowCreate(true)
  }

  const handleDelete = (id: string) => {
    if (!window.confirm('確定要刪除此草稿嗎？刪除後無法復原。')) {
      return
    }

    deleteMutation.mutate(id)
  }

  const handlePublish = (event: ManagedEvent) => {
    const error = validatePublishTimeline(event)
    if (error) {
      toast.error(error)
      return
    }

    publishMutation.mutate(event.id)
  }

  const handleImageFileChange = (files: FileList | null) => {
    const file = files?.[0]
    if (!file) {
      return
    }

    uploadImageMutation.mutate(file)
  }

  const handleDocumentFileChange = (files: FileList | null) => {
    const file = files?.[0]
    if (!file) {
      return
    }

    uploadDocMutation.mutate(file)
  }

  const addTicketType = () => setForm(f => ({ ...f, ticket_types: [...f.ticket_types, createTicketTypeForm('', 50)] }))
  const removeTicketType = (i: number) => setForm(f => ({ ...f, ticket_types: f.ticket_types.filter((_, idx) => idx !== i) }))
  const updateTicketType = <K extends TicketTypeField>(i: number, k: K, v: TicketTypeForm[K]) =>
    setForm(f => ({ ...f, ticket_types: f.ticket_types.map((tt, idx) => idx === i ? { ...tt, [k]: v } : tt) }))

  const events = data ?? []
  if (isLoading) return <div className="empty-state"><div className="spinner" /></div>

  return (
    <div>
      <div className="page-header" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <div>
          <h1 className="page-title">活動管理</h1>
          <p className="page-subtitle">建立、編輯、發布活動</p>
        </div>
        <button className="btn btn-primary" onClick={openCreateModal}>
          <Plus size={16} /> 建立新活動
        </button>
      </div>

      <div className="table-wrap">
        <table>
          <thead>
            <tr><th>活動名稱</th><th>地點</th><th>預計發布</th><th>申請截止</th><th>票種</th><th>狀態</th><th>操作</th></tr>
          </thead>
          <tbody>
            {events.map(e => (
              <tr key={e.id}>
                <td style={{ fontWeight: 500 }}>{e.title}</td>
                <td style={{ color: 'var(--text-secondary)' }}>{e.venue}</td>
                <td style={{ fontSize: 12, color: 'var(--text-muted)' }}>{formatDateOnly(e.publish_time)}</td>
                <td style={{ fontSize: 12, color: 'var(--text-muted)' }}>{formatDateOnly(e.apply_deadline)}</td>
                <td>
                  {(e.ticket_types ?? []).map(tt => (
                    <div key={tt.id ?? tt.name} style={{ fontSize: 12 }}>
                      {tt.name}: <strong style={{ color: getRemainingColor(tt.remaining) }}>{tt.remaining ?? 0}</strong>/{tt.total_quota}
                    </div>
                  ))}
                </td>
                <td>{renderStatusBadge(e.status, e.publish_time)}</td>
                <td>
                  <div style={{ display: 'flex', gap: 6 }}>
                    {e.status === 'draft' && (
                      <>
                        <button className="btn btn-success btn-sm" onClick={() => handlePublish(e)}>
                          <Globe size={13} /> 發布
                        </button>
                        <button className="btn btn-secondary btn-sm" onClick={() => handleEdit(e)}>
                          <Edit size={13} /> 編輯
                        </button>
                        <button className="btn btn-danger btn-sm" onClick={() => handleDelete(e.id)}>
                          <Trash2 size={13} /> 刪除
                        </button>
                      </>
                    )}
                    {e.status === 'published' && (
                      <button className="btn btn-secondary btn-sm" onClick={() => closeMutation.mutate(e.id)}>
                        <XCircle size={13} /> 截止
                      </button>
                    )}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {showCreate && (
        <div className="modal-overlay">
          <button
            type="button"
            aria-label="關閉活動表單"
            onClick={closeEventModal}
            style={{ position: 'absolute', inset: 0, border: 0, background: 'transparent', padding: 0 }}
          />
          <div className="modal" style={{ maxWidth: 640, position: 'relative', zIndex: 1 }}>
            <div className="modal-header">
              <span className="modal-title">{editId ? '編輯活動' : '建立新活動'}</span>
              <button className="modal-close" onClick={closeEventModal}>✕</button>
            </div>

            <div className="form-group">
              <label className="form-label" htmlFor="event-title">活動名稱 *</label>
              <input id="event-title" className="form-input" value={form.title} onChange={e => setForm(f => ({ ...f, title: e.target.value }))} />
            </div>
            <div className="form-group">
              <label className="form-label" htmlFor="event-description">描述</label>
              <textarea id="event-description" className="form-textarea" value={form.description} onChange={e => setForm(f => ({ ...f, description: e.target.value }))} />
            </div>
            <div className="form-group">
              <label className="form-label" htmlFor="event-venue">地點 *</label>
              <input id="event-venue" className="form-input" value={form.venue} onChange={e => setForm(f => ({ ...f, venue: e.target.value }))} />
            </div>
            <div className="form-group">
              <label className="form-label" htmlFor="event-image">活動圖片</label>
              <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                <input
                  id="event-image"
                  type="file"
                  accept="image/*"
                  onChange={e => handleImageFileChange(e.target.files)}
                  disabled={uploadImageMutation.isPending}
                />
                {uploadImageMutation.isPending && <span className="spinner" style={{ width: 16, height: 16 }}></span>}
              </div>
              {form.image_url && (
                <div style={{ marginTop: 8 }}>
                  <img src={form.image_url} alt="cover" style={{ height: 60, borderRadius: 4, objectFit: 'cover' }} />
                  <div style={{ fontSize: 12, color: 'var(--text-muted)' }}>圖片已上傳成功</div>
                </div>
              )}
            </div>

            <div className="form-group">
              <label className="form-label" htmlFor="event-document">活動文件 (僅限 PDF)</label>
              <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                <input
                  id="event-document"
                  type="file"
                  accept=".pdf"
                  onChange={e => handleDocumentFileChange(e.target.files)}
                  disabled={uploadDocMutation.isPending}
                />
                {uploadDocMutation.isPending && <span className="spinner" style={{ width: 16, height: 16 }}></span>}
              </div>
              {form.document_url && (
                <div style={{ marginTop: 8, fontSize: 12, color: 'var(--success)' }}>
                  📄 PDF 附件已上傳成功
                </div>
              )}
            </div>
            <div className="form-row">
              <div className="form-group">
                <label className="form-label" htmlFor="event-publish-time">預計發布時間 *</label>
                <input id="event-publish-time" type="datetime-local" className="form-input" value={form.publish_time} onChange={e => setForm(f => ({ ...f, publish_time: e.target.value }))} />
              </div>
              <div className="form-group">
                <label className="form-label" htmlFor="event-start-time">開始時間 *</label>
                <input id="event-start-time" type="datetime-local" className="form-input" value={form.start_time} onChange={e => setForm(f => ({ ...f, start_time: e.target.value }))} />
              </div>
            </div>
            <div className="form-row">
              <div className="form-group">
                <label className="form-label" htmlFor="event-apply-deadline">申請截止時間 *</label>
                <input id="event-apply-deadline" type="datetime-local" className="form-input" value={form.apply_deadline} onChange={e => setForm(f => ({ ...f, apply_deadline: e.target.value }))} />
              </div>
              <div className="form-group">
                <label className="form-label" htmlFor="event-end-time">結束時間 *</label>
                <input id="event-end-time" type="datetime-local" className="form-input" value={form.end_time} onChange={e => setForm(f => ({ ...f, end_time: e.target.value }))} />
              </div>
            </div>
            <div className="form-row">
              <div className="form-group">
                <label className="form-label" htmlFor="event-max-tickets">每人票數上限</label>
                <input id="event-max-tickets" type="number" min={1} className="form-input" value={form.max_tickets_per_person} onChange={e => setForm(f => ({ ...f, max_tickets_per_person: Number(e.target.value) }))} />
              </div>
            </div>
            <div className="form-group">
              <label className="form-label" htmlFor="event-region">活動地域（例如：台南、新竹，留空 = 不限）</label>
              <input id="event-region" className="form-input" placeholder="例：台南" value={form.region_restriction} onChange={e => setForm(f => ({ ...f, region_restriction: e.target.value }))} />
            </div>

            <div className="divider" />
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
              <div className="form-label" style={{ margin: 0 }}>票種 *</div>
              <button className="btn btn-secondary btn-sm" onClick={addTicketType}><Plus size={13} /> 新增票種</button>
            </div>
            {form.ticket_types.map((tt, i) => (
              <div key={tt.fieldKey} className="form-row" style={{ marginBottom: 10 }}>
                <div className="form-group" style={{ marginBottom: 0 }}>
                  <input className="form-input" placeholder="票種名稱" aria-label={`票種 ${i + 1} 名稱`} value={tt.name} onChange={e => updateTicketType(i, 'name', e.target.value)} />
                </div>
                <div className="form-group" style={{ marginBottom: 0, display: 'flex', gap: 8 }}>
                  <input type="number" className="form-input" placeholder="數量" aria-label={`票種 ${i + 1} 數量`} min={1} value={tt.total_quota} onChange={e => updateTicketType(i, 'total_quota', Number(e.target.value))} />
                  {form.ticket_types.length > 1 && (
                    <button className="btn btn-danger btn-sm btn-icon" aria-label={`移除票種 ${i + 1}`} onClick={() => removeTicketType(i)}><XCircle size={14} /></button>
                  )}
                </div>
              </div>
            ))}

            <div className="modal-footer">
              <button className="btn btn-secondary" onClick={closeEventModal}>取消</button>
              <button className="btn btn-primary" onClick={handleSave} disabled={createMutation.isPending || updateMutation.isPending}>
                {createMutation.isPending || updateMutation.isPending ? '儲存中…' : '儲存草稿'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
