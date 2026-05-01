import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { ScanLine, History } from 'lucide-react'
import toast from 'react-hot-toast'
import api from '../../api/client'

export default function CheckIn() {
  const [token, setToken] = useState('')
  const [result, setResult] = useState<{ type: 'success' | 'error'; message: string; ticket?: any } | null>(null)
  const [eventFilter, setEventFilter] = useState('')

  const { data: events } = useQuery({
    queryKey: ['events-manage'],
    queryFn: () => api.get('/events').then(r => r.data.data),
  })

  const { data: checkins, refetch: refetchCheckins } = useQuery({
    queryKey: ['checkins', eventFilter],
    queryFn: () => api.get('/checkins', { params: eventFilter ? { event_id: eventFilter } : {} }).then(r => r.data.data),
  })

  const checkinMutation = useMutation({
    mutationFn: (qr_token: string) => api.post('/checkin', { qr_token }),
    onSuccess: (res) => {
      const t = res.data.data.ticket
      setResult({ type: 'success', message: `✅ 核銷成功！${t.event?.title ?? ''} - ${t.ticket_type?.name ?? ''}`, ticket: t })
      setToken('')
      refetchCheckins()
    },
    onError: (err: any) => {
      const code = err.response?.data?.error?.code ?? ''
      const msg = err.response?.data?.error?.message ?? '核銷失敗'
      if (code === 'ALREADY_CHECKED_IN') {
        setResult({ type: 'error', message: '❌ 此票券已核銷，請勿重複使用' })
      } else if (code === 'NOT_FOUND') {
        setResult({ type: 'error', message: '❌ 找不到此票券，請確認 QR Code 是否正確' })
      } else {
        setResult({ type: 'error', message: `❌ ${msg}` })
      }
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!token.trim()) return
    setResult(null)
    checkinMutation.mutate(token.trim())
  }

  return (
    <div>
      <div className="page-header">
        <h1 className="page-title">現場核銷</h1>
        <p className="page-subtitle">掃描或輸入票券 QR Token 進行核銷</p>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 24 }}>
        {/* Checkin form */}
        <div className="card">
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 20 }}>
            <ScanLine size={20} style={{ color: 'var(--accent)' }} />
            <h2 style={{ fontSize: 16, fontWeight: 600 }}>核銷入場</h2>
          </div>
          <form onSubmit={handleSubmit}>
            <div className="form-group">
              <label className="form-label">票券 QR Token</label>
              <input
                id="qr-token-input"
                className="form-input"
                placeholder="輸入或掃描 QR Code 內容"
                value={token}
                onChange={e => setToken(e.target.value)}
                autoFocus
                autoComplete="off"
              />
            </div>
            <button type="submit" className="btn btn-primary" style={{ width: '100%' }} disabled={checkinMutation.isPending || !token.trim()}>
              {checkinMutation.isPending ? '核銷中…' : '確認核銷'}
            </button>
          </form>

          {result && (
            <div className={`checkin-result ${result.type}`} style={{ marginTop: 20 }}>
              <div className="checkin-result-icon">{result.type === 'success' ? '✅' : '❌'}</div>
              <div className="checkin-result-msg">{result.message}</div>
              {result.ticket && (
                <div style={{ fontSize: 13, marginTop: 12, padding: '10px', background: 'rgba(255,255,255,0.05)', borderRadius: 6 }}>
                  <div style={{ fontWeight: 600 }}>{result.ticket.user?.name ?? '—'} ({result.ticket.user?.employee_id ?? '—'})</div>
                  <div style={{ fontSize: 12, opacity: 0.8, marginTop: 2 }}>
                    {result.ticket.user?.department} · {result.ticket.user?.region}
                  </div>
                </div>
              )}
            </div>
          )}

          <div style={{ marginTop: 20, padding: 14, background: 'rgba(255,255,255,0.03)', borderRadius: 8, fontSize: 12, color: 'var(--text-muted)' }}>
            💡 員工可在「我的票券」頁面顯示 QR Code，由此處掃描核銷。每張票券只能核銷一次。
          </div>
        </div>

        {/* Recent checkins */}
        <div className="card">
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <History size={18} style={{ color: 'var(--accent)' }} />
              <h2 style={{ fontSize: 16, fontWeight: 600 }}>核銷記錄</h2>
            </div>
            <select className="form-select" style={{ width: 160 }} value={eventFilter} onChange={e => setEventFilter(e.target.value)}>
              <option value="">所有活動</option>
              {(events ?? []).map((e: any) => <option key={e.id} value={e.id}>{e.title.slice(0, 16)}…</option>)}
            </select>
          </div>

          {(checkins ?? []).length === 0 ? (
            <div className="empty-state" style={{ padding: '30px 0' }}>
              <div className="empty-state-icon" style={{ fontSize: 32 }}>📭</div>
              <div className="empty-state-text">尚無核銷記錄</div>
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8, maxHeight: 400, overflowY: 'auto' }}>
              {(checkins ?? []).map((c: any) => (
                <div key={c.id} style={{ padding: '10px 14px', background: 'rgba(255,255,255,0.03)', borderRadius: 8, border: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <div>
                    <div style={{ fontSize: 13, fontWeight: 500 }}>{c.ticket?.event?.title ?? '活動'}</div>
                    <div style={{ fontSize: 11, color: 'var(--text-muted)', marginTop: 2 }}>
                      {c.ticket?.ticket_type?.name} · Token: {c.ticket?.qr_token?.slice(0, 8) ?? '—'}…
                    </div>
                  </div>
                  <div style={{ fontSize: 11, color: 'var(--text-muted)', textAlign: 'right' }}>
                    {new Date(c.checked_at).toLocaleTimeString('zh-TW', { hour: '2-digit', minute: '2-digit' })}
                    <div style={{ color: 'var(--success)', fontSize: 10 }}>✅ 核銷</div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
