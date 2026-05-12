import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, XCircle, Globe, Edit, Trash2, Clock } from 'lucide-react'
import toast from 'react-hot-toast'
import api from '../../api/client'

type EventStatus = 'draft' | 'published' | 'closed' | 'ended'
function StatusBadge({ status, publishTime }: { status: EventStatus, publishTime?: string }) {
  const m: Record<string, string> = { draft: '草稿', published: '發布中', closed: '已截止', ended: '已結束' }
  const isScheduled = status === 'draft' && publishTime && new Date(publishTime) > new Date()
  
  if (isScheduled) {
    return <span className="badge badge-draft" style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
      <Clock size={12} /> 排程中
    </span>
  }
  
  return <span className={`badge badge-${status}`}>{m[status] ?? status}</span>
}

const EMPTY_FORM = {
  title: '', description: '', venue: '',
  publish_time: '', start_time: '', apply_deadline: '', end_time: '',
  region_restriction: '', max_tickets_per_person: 1,
  ticket_types: [{ name: '一般票', total_quota: 100 }],
}

function validatePublishTimeline(event: any, publishAt = new Date()): string | null {
  const applyDeadline = new Date(event.apply_deadline)
  const startTime = new Date(event.start_time)
  const endTime = new Date(event.end_time)

  console.log(applyDeadline)
  console.log(startTime)
  console.log(endTime)
  console.log(publishAt)

  if (!(publishAt < applyDeadline)) {
    return '發布時間必須早於申請截止時間'
  }
  if (!(startTime < endTime)) {
    return '活動結束時間必須晚於開始時間'
  }
  return null
}

export default function EventManage() {
  const qc = useQueryClient()
  const [showCreate, setShowCreate] = useState(false)
  const [editId, setEditId] = useState<string | null>(null)
  const [form, setForm] = useState({ ...EMPTY_FORM })

  const formatToLocalDatetime = (isoString: string) => {
    if (!isoString) return ''
    const date = new Date(isoString)
    if (isNaN(date.getTime())) return ''
    return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
  }

  const { data, isLoading } = useQuery({
    queryKey: ['events-manage'],
    queryFn: () => api.get('/events').then(r => r.data.data),
  })

  const createMutation = useMutation({
    mutationFn: (payload: any) => api.post('/events', payload),
    onSuccess: () => {
      toast.success('活動建立成功！')
      setShowCreate(false)
      setEditId(null)
      setForm({ ...EMPTY_FORM })
      qc.invalidateQueries({ queryKey: ['events-manage'] })
      qc.invalidateQueries({ queryKey: ['events'] })
    },
    onError: (err: any) => toast.error(err.response?.data?.error?.message ?? '建立失敗'),
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: any }) => api.put(`/events/${id}`, payload),
    onSuccess: () => {
      toast.success('活動更新成功！')
      setShowCreate(false)
      setEditId(null)
      setForm({ ...EMPTY_FORM })
      qc.invalidateQueries({ queryKey: ['events-manage'] })
      qc.invalidateQueries({ queryKey: ['events'] })
    },
    onError: (err: any) => toast.error(err.response?.data?.error?.message ?? '更新失敗'),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/events/${id}`),
    onSuccess: () => {
      toast.success('活動已刪除！')
      qc.invalidateQueries({ queryKey: ['events-manage'] })
    },
    onError: (err: any) => toast.error(err.response?.data?.error?.message ?? '刪除失敗'),
  })

  const publishMutation = useMutation({
    mutationFn: (id: string) => api.patch(`/events/${id}/publish`),
    onSuccess: () => { toast.success('活動已發布！'); qc.invalidateQueries({ queryKey: ['events-manage'] }) },
    onError: (err: any) => toast.error(err.response?.data?.error?.message ?? '發布失敗'),
  })

  const closeMutation = useMutation({
    mutationFn: (id: string) => api.patch(`/events/${id}/close`),
    onSuccess: () => { toast.success('已截止報名'); qc.invalidateQueries({ queryKey: ['events-manage'] }) },
  })

  const handleSave = () => {
    const payload = {
      ...form,
      publish_time: form.publish_time ? new Date(form.publish_time).toISOString() : '',
      start_time: form.start_time ? new Date(form.start_time).toISOString() : '',
      apply_deadline: form.apply_deadline ? new Date(form.apply_deadline).toISOString() : '',
      end_time: form.end_time ? new Date(form.end_time).toISOString() : '',
      region_restriction: form.region_restriction || null,
      ticket_types: form.ticket_types.map(tt => ({ ...tt, total_quota: Number(tt.total_quota) })),
    }
    if (editId) {
      updateMutation.mutate({ id: editId, payload })
    } else {
      createMutation.mutate(payload)
    }
  }

  const handleEdit = (e: any) => {
    setForm({
      title: e.title || '',
      description: e.description || '',
      venue: e.venue || '',
      publish_time: formatToLocalDatetime(e.publish_time),
      start_time: formatToLocalDatetime(e.start_time),
      apply_deadline: formatToLocalDatetime(e.apply_deadline),
      end_time: formatToLocalDatetime(e.end_time),
      region_restriction: e.region_restriction || '',
      max_tickets_per_person: e.max_tickets_per_person || 1,
      ticket_types: (e.ticket_types && e.ticket_types.length > 0) ? e.ticket_types.map((tt: any) => ({ name: tt.name, total_quota: tt.total_quota })) : [{ name: '一般票', total_quota: 100 }],
    })
    setEditId(e.id)
    setShowCreate(true)
  }

  const handleDelete = (id: string) => {
    if (window.confirm('確定要刪除此草稿嗎？刪除後無法復原。')) {
      deleteMutation.mutate(id)
    }
  }

  const addTicketType = () => setForm(f => ({ ...f, ticket_types: [...f.ticket_types, { name: '', total_quota: 50 }] }))
  const removeTicketType = (i: number) => setForm(f => ({ ...f, ticket_types: f.ticket_types.filter((_, idx) => idx !== i) }))
  const updateTicketType = (i: number, k: string, v: any) =>
    setForm(f => ({ ...f, ticket_types: f.ticket_types.map((tt, idx) => idx === i ? { ...tt, [k]: v } : tt) }))

  const events: any[] = data ?? []
  if (isLoading) return <div className="empty-state"><div className="spinner" /></div>

  return (
    <div>
      <div className="page-header" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <div>
          <h1 className="page-title">活動管理</h1>
          <p className="page-subtitle">建立、編輯、發布活動</p>
        </div>
        <button className="btn btn-primary" onClick={() => { setEditId(null); setForm({ ...EMPTY_FORM }); setShowCreate(true); }}>
          <Plus size={16} /> 建立新活動
        </button>
      </div>

      <div className="table-wrap">
        <table>
          <thead>
            <tr><th>活動名稱</th><th>地點</th><th>預計發布</th><th>申請截止</th><th>票種</th><th>狀態</th><th>操作</th></tr>
          </thead>
          <tbody>
            {events.map((e: any) => (
              <tr key={e.id}>
                <td style={{ fontWeight: 500 }}>{e.title}</td>
                <td style={{ color: 'var(--text-secondary)' }}>{e.venue}</td>
                <td style={{ fontSize: 12, color: 'var(--text-muted)' }}>{new Date(e.publish_time).toLocaleDateString('zh-TW')}</td>
                <td style={{ fontSize: 12, color: 'var(--text-muted)' }}>{new Date(e.apply_deadline).toLocaleDateString('zh-TW')}</td>
                <td>
                  {(e.ticket_types ?? []).map((tt: any) => (
                    <div key={tt.id} style={{ fontSize: 12 }}>
                      {tt.name}: <strong style={{ color: tt.remaining > 5 ? 'var(--success)' : 'var(--danger)' }}>{tt.remaining}</strong>/{tt.total_quota}
                    </div>
                  ))}
                </td>
                <td><StatusBadge status={e.status} publishTime={e.publish_time} /></td>
                <td>
                  <div style={{ display: 'flex', gap: 6 }}>
                    {e.status === 'draft' && (
                      <>
                        <button className="btn btn-success btn-sm" onClick={() => {
                          const error = validatePublishTimeline(e)
                          if (error) {
                            toast.error(error)
                            return
                          }
                          publishMutation.mutate(e.id)
                        }}>
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
        <div className="modal-overlay" onClick={e => e.target === e.currentTarget && setShowCreate(false)}>
          <div className="modal" style={{ maxWidth: 640 }}>
            <div className="modal-header">
              <span className="modal-title">{editId ? '編輯活動' : '建立新活動'}</span>
              <button className="modal-close" onClick={() => setShowCreate(false)}>✕</button>
            </div>

            <div className="form-group">
              <label className="form-label">活動名稱 *</label>
              <input className="form-input" value={form.title} onChange={e => setForm(f => ({ ...f, title: e.target.value }))} />
            </div>
            <div className="form-group">
              <label className="form-label">描述</label>
              <textarea className="form-textarea" value={form.description} onChange={e => setForm(f => ({ ...f, description: e.target.value }))} />
            </div>
            <div className="form-group">
              <label className="form-label">地點 *</label>
              <input className="form-input" value={form.venue} onChange={e => setForm(f => ({ ...f, venue: e.target.value }))} />
            </div>
            <div className="form-row">
              <div className="form-group">
                <label className="form-label">預計發布時間 *</label>
                <input type="datetime-local" className="form-input" value={form.publish_time} onChange={e => setForm(f => ({ ...f, publish_time: e.target.value }))} />
              </div>
              <div className="form-group">
                <label className="form-label">開始時間 *</label>
                <input type="datetime-local" className="form-input" value={form.start_time} onChange={e => setForm(f => ({ ...f, start_time: e.target.value }))} />
              </div>
            </div>
            <div className="form-row">
              <div className="form-group">
                <label className="form-label">申請截止時間 *</label>
                <input type="datetime-local" className="form-input" value={form.apply_deadline} onChange={e => setForm(f => ({ ...f, apply_deadline: e.target.value }))} />
              </div>
              <div className="form-group">
                <label className="form-label">結束時間 *</label>
                <input type="datetime-local" className="form-input" value={form.end_time} onChange={e => setForm(f => ({ ...f, end_time: e.target.value }))} />
              </div>
            </div>
            <div className="form-row">
              <div className="form-group">
                <label className="form-label">每人票數上限</label>
                <input type="number" min={1} className="form-input" value={form.max_tickets_per_person} onChange={e => setForm(f => ({ ...f, max_tickets_per_person: Number(e.target.value) }))} />
              </div>
            </div>
            <div className="form-group">
              <label className="form-label">活動地域（例如：台南、新竹，留空 = 不限）</label>
              <input className="form-input" placeholder="例：台南" value={form.region_restriction} onChange={e => setForm(f => ({ ...f, region_restriction: e.target.value }))} />
            </div>

            <div className="divider" />
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
              <label className="form-label" style={{ margin: 0 }}>票種 *</label>
              <button className="btn btn-secondary btn-sm" onClick={addTicketType}><Plus size={13} /> 新增票種</button>
            </div>
            {form.ticket_types.map((tt, i) => (
              <div key={i} className="form-row" style={{ marginBottom: 10 }}>
                <div className="form-group" style={{ marginBottom: 0 }}>
                  <input className="form-input" placeholder="票種名稱" value={tt.name} onChange={e => updateTicketType(i, 'name', e.target.value)} />
                </div>
                <div className="form-group" style={{ marginBottom: 0, display: 'flex', gap: 8 }}>
                  <input type="number" className="form-input" placeholder="數量" min={1} value={tt.total_quota} onChange={e => updateTicketType(i, 'total_quota', Number(e.target.value))} />
                  {form.ticket_types.length > 1 && (
                    <button className="btn btn-danger btn-sm btn-icon" onClick={() => removeTicketType(i)}><XCircle size={14} /></button>
                  )}
                </div>
              </div>
            ))}

            <div className="modal-footer">
              <button className="btn btn-secondary" onClick={() => setShowCreate(false)}>取消</button>
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
