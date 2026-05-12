import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Check, X, RefreshCw } from 'lucide-react'
import toast from 'react-hot-toast'
import api from '../../api/client'

function StatusBadge({ status }: { status: string }) {
  const m: Record<string, string> = { pending: '待審核', approved: '已核准', rejected: '已拒絕' }
  return <span className={`badge badge-${status}`}>{m[status] ?? status}</span>
}

export default function Applications() {
  const qc = useQueryClient()
  const [eventFilter, setEventFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [rejectModal, setRejectModal] = useState<{ id: string; name: string } | null>(null)
  const [rejectReason, setRejectReason] = useState('')

  const { data: events } = useQuery({
    queryKey: ['events-manage'],
    queryFn: () => api.get('/events').then(r => r.data.data),
  })

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['applications', eventFilter, statusFilter],
    queryFn: () => api.get('/applications', {
      params: {
        ...(eventFilter ? { event_id: eventFilter } : {}),
        ...(statusFilter ? { status: statusFilter } : {}),
      },
    }).then(r => r.data.data),
  })

  const approveMutation = useMutation({
    mutationFn: (id: string) => api.post(`/applications/${id}/approve`),
    onSuccess: () => { toast.success('已核准申請，票券已生成'); qc.invalidateQueries({ queryKey: ['applications'] }) },
    onError: (err: any) => toast.error(err.response?.data?.error?.message ?? '操作失敗'),
  })

  const rejectMutation = useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) => api.post(`/applications/${id}/reject`, { reason }),
    onSuccess: () => {
      toast.success('已拒絕申請')
      setRejectModal(null)
      setRejectReason('')
      qc.invalidateQueries({ queryKey: ['applications'] })
    },
    onError: (err: any) => toast.error(err.response?.data?.error?.message ?? '操作失敗'),
  })

  const apps: any[] = data ?? []

  return (
    <div>
      <div className="page-header" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <div>
          <h1 className="page-title">申請審核</h1>
          <p className="page-subtitle">審核員工的票券申請</p>
        </div>
        <button className="btn btn-secondary btn-sm" onClick={() => refetch()}>
          <RefreshCw size={14} /> 重新整理
        </button>
      </div>

      <div className="filter-bar">
        <select className="form-select" value={eventFilter} onChange={e => setEventFilter(e.target.value)} style={{ width: 240 }}>
          <option value="">所有活動</option>
          {(events ?? []).map((e: any) => <option key={e.id} value={e.id}>{e.title}</option>)}
        </select>
        <select className="form-select" value={statusFilter} onChange={e => setStatusFilter(e.target.value)} style={{ width: 120 }}>
          <option value="">全部狀態</option>
          <option value="pending">待審核</option>
          <option value="approved">已核准</option>
          <option value="rejected">已拒絕</option>
        </select>
        <span style={{ color: 'var(--text-muted)', fontSize: 13 }}>共 {apps.length} 筆</span>
      </div>

      {isLoading ? (
        <div className="empty-state"><div className="spinner" /></div>
      ) : apps.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon">📋</div>
          <div className="empty-state-text">沒有符合條件的申請</div>
        </div>
      ) : (
        <div className="table-wrap">
          <table>
            <thead>
              <tr><th>員工</th><th>部門/廠區</th><th>活動</th><th>票種</th><th>數量</th><th>申請時間</th><th>狀態</th><th>操作</th></tr>
            </thead>
            <tbody>
              {apps.map((app: any) => (
                <tr key={app.id}>
                  <td>
                    <div style={{ fontWeight: 500 }}>{app.user?.name ?? '—'}</div>
                    <div style={{ fontSize: 11, color: 'var(--text-muted)' }}>{app.user?.employee_id}</div>
                  </td>
                  <td style={{ fontSize: 12, color: 'var(--text-secondary)' }}>
                    {app.user?.department}<br />{app.user?.region}
                  </td>
                  <td style={{ fontSize: 13 }}>{app.event?.title ?? '—'}</td>
                  <td style={{ fontSize: 13 }}>{app.ticket_type?.name ?? '—'}</td>
                  <td>{app.quantity}</td>
                  <td style={{ fontSize: 12, color: 'var(--text-muted)' }}>{new Date(app.applied_at).toLocaleDateString('zh-TW')}</td>
                  <td><StatusBadge status={app.status} /></td>
                  <td>
                    {app.status === 'pending' && (
                      <div style={{ display: 'flex', gap: 6 }}>
                        <button className="btn btn-success btn-sm" onClick={() => approveMutation.mutate(app.id)} disabled={approveMutation.isPending}>
                          <Check size={13} /> 核准
                        </button>
                        <button className="btn btn-danger btn-sm" onClick={() => setRejectModal({ id: app.id, name: app.user?.name ?? '' })}>
                          <X size={13} /> 拒絕
                        </button>
                      </div>
                    )}
                    {app.reason && <div style={{ fontSize: 11, color: 'var(--text-muted)', marginTop: 4 }}>{app.reason}</div>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {rejectModal && (
        <div className="modal-overlay" onClick={e => e.target === e.currentTarget && setRejectModal(null)}>
          <div className="modal" style={{ maxWidth: 400 }}>
            <div className="modal-header">
              <span className="modal-title">拒絕申請</span>
              <button className="modal-close" onClick={() => setRejectModal(null)}>✕</button>
            </div>
            <p style={{ color: 'var(--text-secondary)', marginBottom: 16 }}>拒絕 {rejectModal.name} 的申請</p>
            <div className="form-group">
              <label className="form-label">拒絕原因（選填）</label>
              <textarea className="form-textarea" style={{ minHeight: 80 }} value={rejectReason} onChange={e => setRejectReason(e.target.value)} placeholder="例：票券已全數分配完畢" />
            </div>
            <div className="modal-footer">
              <button className="btn btn-secondary" onClick={() => setRejectModal(null)}>取消</button>
              <button className="btn btn-danger" onClick={() => rejectMutation.mutate({ id: rejectModal.id, reason: rejectReason })} disabled={rejectMutation.isPending}>
                {rejectMutation.isPending ? '處理中…' : '確認拒絕'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
