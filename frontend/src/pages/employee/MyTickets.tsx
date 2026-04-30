import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { QRCodeSVG } from 'qrcode.react'
import api from '../../api/client'
import { useAuth } from '../../contexts/AuthContext'

function StatusBadge({ status }: { status: string }) {
  const map: Record<string, string> = {
    pending: '待審核', approved: '已核准', rejected: '已拒絕', cancelled: '已取消',
  }
  return <span className={`badge badge-${status}`}>{map[status] ?? status}</span>
}

export default function MyTickets() {
  const { user } = useAuth()
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
                      <button className="btn btn-secondary btn-sm" onClick={() => setExpandedTicket(expandedTicket === ticket.id ? null : ticket.id)}>
                        {expandedTicket === ticket.id ? '收起 QR' : '顯示 QR'}
                      </button>
                    )}
                  </div>
                </div>
                {expandedTicket === ticket.id && (
                  <div className="ticket-card-body">
                    <div className="qr-container">
                      <QRCodeSVG value={ticket.qr_token} size={180} />
                      <div className="qr-token">{ticket.qr_token}</div>
                    </div>
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
