import { useState, useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { QRCodeSVG } from 'qrcode.react'
import api from '../../api/client'
import { useAuth } from '../../contexts/AuthContext'
import { generateTOTP } from '../../utils/totp'

function StatusBadge({ status }: { status: string }) {
  const map: Record<string, string> = {
    pending: '待審核', approved: '已核准', rejected: '已拒絕', cancelled: '已取消',
  }
  return <span className={`badge badge-${status}`}>{map[status] ?? status}</span>
}

function DynamicTicketQR({ baseToken }: { baseToken: string }) {
  const [dynamicToken, setDynamicToken] = useState<string>('')
  const [timeLeft, setTimeLeft] = useState(60)

  useEffect(() => {
    const updateToken = async () => {
      const otp = await generateTOTP(baseToken, 60)
      setDynamicToken(`${baseToken}|${otp}`)
    }

    updateToken()

    const interval = setInterval(() => {
      const currentSeconds = Math.floor(Date.now() / 1000)
      const remainder = 60 - (currentSeconds % 60)
      setTimeLeft(remainder)

      if (remainder === 60) {
        updateToken()
      }
    }, 1000)

    return () => clearInterval(interval)
  }, [baseToken])

  if (!dynamicToken) return <div style={{ height: 180, display: 'flex', alignItems: 'center', justifyContent: 'center' }}><div className="spinner" /></div>

  return (
    <div className="qr-container">
      <QRCodeSVG value={dynamicToken} size={180} />
      <div className="qr-token" style={{ fontSize: 11 }}>{dynamicToken}</div>
      <div style={{ marginTop: 8, fontSize: 11, color: 'var(--text-muted)' }}>
        防偽驗證碼將在 <strong style={{ color: 'var(--accent)' }}>{timeLeft}</strong> 秒後更新
      </div>
    </div>
  )
}

export default function MyTickets() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const [expandedTicket, setExpandedTicket] = useState<string | null>(null)

  const { data: apps, isLoading } = useQuery({
    queryKey: ['my-applications', user?.id],
    queryFn: () => api.get('/applications/my').then(r => r.data.data),
    enabled: !!user,
  })

  const { data: tickets } = useQuery({
    queryKey: ['my-tickets', user?.id],
    queryFn: () => api.get('/tickets/my').then(r => r.data.data),
    enabled: !!user,
  })

  const handleReturnTicket = async (ticketId: string) => {
    if (!window.confirm('確定要退掉這張票嗎？退掉後票券將失效且無法恢復。')) return
    try {
      await api.post(`/tickets/${ticketId}/cancel`)
      // Refresh all related data
      queryClient.invalidateQueries({ queryKey: ['my-tickets'] })
      queryClient.invalidateQueries({ queryKey: ['my-applications'] })
    } catch (err: any) {
      alert(err.response?.data?.error || '退票失敗')
    }
  }

  if (isLoading) return <div className="empty-state"><div className="spinner" /></div>

  const allApps: any[] = apps ?? []
  const allTickets: any[] = tickets ?? []

  return (
    <div>
      <div className="page-header">
        <h1 className="page-title">我的票券</h1>
        <p className="page-subtitle">查看申請狀態與電子票券</p>
      </div>

      {/* Approved tickets with QR */}
      {allTickets.length > 0 && (
        <div style={{ marginBottom: 32 }}>
          <h2 style={{ fontSize: 16, fontWeight: 600, marginBottom: 16 }}>🎫 電子票券</h2>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
            {allTickets.map((ticket: any) => (
              <div key={ticket.id} className="ticket-card">
                <div className="ticket-card-header">
                  <div>
                    <div style={{ fontWeight: 600 }}>{ticket.event?.title ?? '活動'}</div>
                    <div style={{ fontSize: 12, color: 'var(--text-secondary)', marginTop: 4 }}>
                      {ticket.ticket_type?.name} · 到期：{new Date(ticket.expires_at).toLocaleDateString('zh-TW')}
                    </div>
                  </div>
                  <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                    <span className={`badge ${ticket.is_used ? 'badge-used' : 'badge-unused'}`}>
                      {ticket.is_used ? '已核銷' : '未使用'}
                    </span>
                    {!ticket.is_used && (
                      <>
                        <button className="btn btn-secondary btn-sm" onClick={() => setExpandedTicket(expandedTicket === ticket.id ? null : ticket.id)}>
                          {expandedTicket === ticket.id ? '收起 QR' : '顯示 QR'}
                        </button>
                        <button
                          className="btn btn-sm"
                          style={{ backgroundColor: 'rgba(239, 68, 68, 0.1)', color: '#ef4444', border: '1px solid rgba(239, 68, 68, 0.2)' }}
                          onClick={() => handleReturnTicket(ticket.id)}
                        >
                          退票
                        </button>
                      </>
                    )}
                  </div>
                </div>
                {expandedTicket === ticket.id && !ticket.is_used && (
                  <div className="ticket-card-body">
                    <DynamicTicketQR baseToken={ticket.qr_token} />
                    <p style={{ textAlign: 'center', fontSize: 12, color: 'var(--text-muted)', marginTop: 12 }}>
                      入場時出示此 QR Code 給現場工作人員掃描
                    </p>
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* All applications */}
      <h2 style={{ fontSize: 16, fontWeight: 600, marginBottom: 16 }}>📋 申請記錄</h2>
      {allApps.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon">🎟️</div>
          <div className="empty-state-text">還沒有申請記錄，快去瀏覽活動吧！</div>
        </div>
      ) : (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>活動</th><th>票種</th><th>數量</th><th>狀態</th><th>申請時間</th><th>備註</th>
              </tr>
            </thead>
            <tbody>
              {allApps.map((app: any) => (
                <tr key={app.id}>
                  <td>{app.event?.title ?? '—'}</td>
                  <td>{app.ticket_type?.name ?? '—'}</td>
                  <td>{app.quantity}</td>
                  <td><StatusBadge status={app.status} /></td>
                  <td style={{ fontSize: 12, color: 'var(--text-muted)' }}>
                    {new Date(app.applied_at).toLocaleDateString('zh-TW')}
                  </td>
                  <td style={{ fontSize: 12, color: 'var(--text-muted)' }}>{app.reason ?? '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
