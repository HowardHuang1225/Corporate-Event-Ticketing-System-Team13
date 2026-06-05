import axios from 'axios'
import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Check, X, RefreshCw } from 'lucide-react'
import toast from 'react-hot-toast'
import api from '../../api/client'

type ApplicationStatus = 'pending' | 'approved' | 'rejected'

type EventOption = {
  id: string
  title: string
}

type Application = {
  id: string
  status: ApplicationStatus
  quantity: number
  applied_at: string
  reason?: string
  user?: {
    name?: string
    employee_id?: string
    department?: string
    region?: string
  }
  event?: {
    title?: string
  }
  ticket_type?: {
    name?: string
  }
}

type ApiErrorResponse = {
  error?: {
    message?: string
  }
}

const APPLICATION_STATUS_LABELS: Record<ApplicationStatus, string> = {
  pending: '待審核',
  approved: '已核准',
  rejected: '已拒絕',
}

function isApplicationStatus(status: string): status is ApplicationStatus {
  return status in APPLICATION_STATUS_LABELS
}

function getApiErrorMessage(error: unknown, fallback: string) {
  if (axios.isAxiosError<ApiErrorResponse>(error)) {
    return error.response?.data?.error?.message ?? fallback
  }

  return fallback
}

function renderStatusBadge(status: string) {
  const label = isApplicationStatus(status) ? APPLICATION_STATUS_LABELS[status] : status
  return <span className={`badge badge-${status}`}>{label}</span>
}

export default function Applications() {
  const qc = useQueryClient()
  const [eventFilter, setEventFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [rejectModal, setRejectModal] = useState<{ id: string; name: string } | null>(null)
  const [rejectReason, setRejectReason] = useState('')

  const { data: events } = useQuery({
    queryKey: ['events-manage'],
    queryFn: () => api.get<{ data: EventOption[] }>('/events').then(r => r.data.data),
  })

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['applications', eventFilter, statusFilter],
    queryFn: () => api.get<{ data: Application[] }>('/applications', {
      params: {
        ...(eventFilter ? { event_id: eventFilter } : {}),
        ...(statusFilter ? { status: statusFilter } : {}),
      },
    }).then(r => r.data.data),
  })

  const approveMutation = useMutation({
    mutationFn: (id: string) => api.post(`/applications/${id}/approve`),
    onSuccess: () => {
      toast.success('已核准申請，票券已生成')
      qc.invalidateQueries({ queryKey: ['applications'] })
    },
    onError: error => toast.error(getApiErrorMessage(error, '操作失敗')),
  })

  const rejectMutation = useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) => api.post(`/applications/${id}/reject`, { reason }),
    onSuccess: () => {
      toast.success('已拒絕申請')
      setRejectModal(null)
      setRejectReason('')
      qc.invalidateQueries({ queryKey: ['applications'] })
    },
    onError: error => toast.error(getApiErrorMessage(error, '操作失敗')),
  })

  function closeRejectModal() {
    setRejectModal(null)
  }

  const apps = data ?? []

  function renderApplicationsContent() {
    if (isLoading) {
      return <div className="empty-state"><div className="spinner" /></div>
    }

    if (apps.length === 0) {
      return (
        <div className="empty-state">
          <div className="empty-state-icon">📋</div>
          <div className="empty-state-text">沒有符合條件的申請</div>
        </div>
      )
    }

    return (
      <div className="table-wrap">
        <table>
          <thead>
            <tr><th>員工</th><th>部門/廠區</th><th>活動</th><th>票種</th><th>數量</th><th>申請時間</th><th>狀態</th><th>操作</th></tr>
          </thead>
          <tbody>
            {apps.map(app => (
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
                <td>{renderStatusBadge(app.status)}</td>
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
    )
  }

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
          {(events ?? []).map(e => <option key={e.id} value={e.id}>{e.title}</option>)}
        </select>
        <select className="form-select" value={statusFilter} onChange={e => setStatusFilter(e.target.value)} style={{ width: 120 }}>
          <option value="">全部狀態</option>
          <option value="pending">待審核</option>
          <option value="approved">已核准</option>
          <option value="rejected">已拒絕</option>
        </select>
        <span style={{ color: 'var(--text-muted)', fontSize: 13 }}>共 {apps.length} 筆</span>
      </div>

      {renderApplicationsContent()}

      {rejectModal && (
        <div className="modal-overlay">
          <button
            type="button"
            aria-label="關閉拒絕申請視窗"
            onClick={closeRejectModal}
            style={{ position: 'absolute', inset: 0, border: 0, background: 'transparent', padding: 0 }}
          />
          <div className="modal" style={{ maxWidth: 400, position: 'relative', zIndex: 1 }}>
            <div className="modal-header">
              <span className="modal-title">拒絕申請</span>
              <button className="modal-close" onClick={closeRejectModal}>✕</button>
            </div>
            <p style={{ color: 'var(--text-secondary)', marginBottom: 16 }}>拒絕 {rejectModal.name} 的申請</p>
            <div className="form-group">
              <label className="form-label" htmlFor="reject-reason">拒絕原因（選填）</label>
              <textarea id="reject-reason" className="form-textarea" style={{ minHeight: 80 }} value={rejectReason} onChange={e => setRejectReason(e.target.value)} placeholder="例：票券已全數分配完畢" />
            </div>
            <div className="modal-footer">
              <button className="btn btn-secondary" onClick={closeRejectModal}>取消</button>
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
