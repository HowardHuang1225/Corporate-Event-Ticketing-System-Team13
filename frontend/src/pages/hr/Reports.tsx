import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { BarChart3, Users, Ticket, CheckSquare, Download } from 'lucide-react'
import api from '../../api/client'

export default function Reports() {
  const [selectedEvent, setSelectedEvent] = useState('')

  const { data: events } = useQuery({
    queryKey: ['events-all'],
    queryFn: () => api.get('/events').then(r => r.data.data),
  })

  const { data: overview } = useQuery({
    queryKey: ['overview'],
    queryFn: () => api.get('/reports/overview').then(r => r.data.data),
  })

  const { data: stats, isLoading: statsLoading } = useQuery({
    queryKey: ['event-stats', selectedEvent],
    queryFn: () => api.get(`/reports/events/${selectedEvent}/stats`).then(r => r.data.data),
    enabled: !!selectedEvent,
  })

  const exportToCSV = (filename: string, rows: string[][]) => {
    const csvContent = rows.map(row => row.map(cell => `"${cell}"`).join(",")).join("\n")
    const blob = new Blob(["\ufeff" + csvContent], { type: 'text/csv;charset=utf-8;' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement("a")
    link.setAttribute("href", url)
    link.setAttribute("download", filename)
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
  }

  const handleExportOverview = () => {
    if (!overview) return
    const headers = ["活動名稱", "申請人數", "核准人數", "核銷人數", "核銷率"]
    const data = overview.map((o: any) => [
      o.title,
      o.applied,
      o.approved,
      o.checked_in,
      `${o.approved > 0 ? Math.round((o.checked_in / o.approved) * 100) : 0}%`
    ])
    exportToCSV(`活動總覽報表_${new Date().toISOString().slice(0, 10)}.csv`, [headers, ...data])
  }

  const handleExportDetail = () => {
    if (!stats) return
    const rows = [
      ["活動標題", stats.event.title],
      ["總申請數", stats.total_applied],
      ["已核准數", stats.total_approved],
      ["已核銷數", stats.total_checked_in],
      ["核銷率", stats.total_approved > 0 ? `${Math.round(stats.total_checked_in / stats.total_approved * 100)}%` : "0%"],
      ["", ""],
      ["部門分佈", "人數"],
      ...(stats.by_department || []).map((d: any) => [d.department, d.count]),
      ["", ""],
      ["票種統計", "申請數", "核准數"],
      ...(stats.by_ticket_type || []).map((t: any) => [t.ticket_type_name, t.total, t.approved])
    ]
    exportToCSV(`活動詳情_${stats.event.title}_${new Date().toISOString().slice(0, 10)}.csv`, rows)
  }

  return (
    <div>
      <div className="page-header">
        <div>
          <h1 className="page-title">統計報表</h1>
          <p className="page-subtitle">各活動參與人數與票務統計</p>
        </div>
        <button className="btn btn-secondary" onClick={handleExportOverview} disabled={!overview}>
          <Download size={16} style={{ marginRight: 8 }} /> 匯出總覽
        </button>
      </div>

      {/* Overview */}
      {overview && (
        <div style={{ marginBottom: 32 }}>
          <h2 style={{ fontSize: 16, fontWeight: 600, marginBottom: 16 }}>活動總覽</h2>
          <div className="table-wrap">
            <table>
              <thead>
                <tr><th>活動名稱</th><th>申請人數</th><th>核准人數</th><th>核銷人數</th><th>核銷率</th></tr>
              </thead>
              <tbody>
                {overview.map((o: any) => {
                  const rate = o.approved > 0 ? Math.round((o.checked_in / o.approved) * 100) : 0
                  return (
                    <tr key={o.event_id}>
                      <td style={{ fontWeight: 500 }}>{o.title}</td>
                      <td>{o.applied}</td>
                      <td>{o.approved}</td>
                      <td>{o.checked_in}</td>
                      <td>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                          <div style={{ flex: 1, height: 6, background: 'rgba(255,255,255,0.1)', borderRadius: 3, overflow: 'hidden' }}>
                            <div style={{ width: `${rate}%`, height: '100%', background: 'var(--success)', transition: 'width 0.5s' }} />
                          </div>
                          <span style={{ fontSize: 12 }}>{rate}%</span>
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Per-event detail */}
      <div style={{ marginBottom: 20, display: 'flex', gap: 12, alignItems: 'center' }}>
        <select className="form-select" style={{ width: 300 }} value={selectedEvent} onChange={e => setSelectedEvent(e.target.value)}>
          <option value="">── 選擇活動查看詳情 ──</option>
          {(events ?? []).map((e: any) => <option key={e.id} value={e.id}>{e.title}</option>)}
        </select>
        {selectedEvent && stats && (
          <button className="btn btn-primary btn-sm" onClick={handleExportDetail}>
            <Download size={14} style={{ marginRight: 6 }} /> 匯出詳情 CSV
          </button>
        )}
      </div>

      {statsLoading && <div className="empty-state"><div className="spinner" /></div>}

      {stats && (
        <>
          <div className="stats-grid">
            {[
              { icon: <Ticket size={20} />, label: '總申請數', value: stats.total_applied, color: 'var(--info)' },
              { icon: <CheckSquare size={20} />, label: '已核准', value: stats.total_approved, color: 'var(--success)' },
              { icon: <BarChart3 size={20} />, label: '已核銷', value: stats.total_checked_in, color: 'var(--accent)' },
              { icon: <Users size={20} />, label: '核銷率', value: stats.total_approved > 0 ? `${Math.round(stats.total_checked_in / stats.total_approved * 100)}%` : '—', color: 'var(--warning)' },
            ].map(s => (
              <div key={s.label} className="stat-card">
                <div style={{ color: s.color, marginBottom: 8 }}>{s.icon}</div>
                <div className="stat-value" style={{ color: s.color }}>{s.value}</div>
                <div className="stat-label">{s.label}</div>
              </div>
            ))}
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(300px, 1fr))', gap: 20 }}>
            <div className="card">
              <h3 style={{ fontSize: 14, fontWeight: 600, marginBottom: 16, color: 'var(--text-secondary)' }}>部門分佈</h3>
              {(stats.by_department ?? []).length === 0 ? (
                <div style={{ color: 'var(--text-muted)', fontSize: 13 }}>尚無資料</div>
              ) : (
                <div>
                  {(stats.by_department ?? []).map((d: any) => {
                    const pct = stats.total_approved > 0 ? Math.round(d.count / stats.total_approved * 100) : 0
                    return (
                      <div key={d.department} style={{ marginBottom: 12 }}>
                        <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13, marginBottom: 4 }}>
                          <span>{d.department}</span>
                          <span style={{ color: 'var(--text-muted)' }}>{d.count} 人 ({pct}%)</span>
                        </div>
                        <div style={{ height: 6, background: 'rgba(255,255,255,0.08)', borderRadius: 3, overflow: 'hidden' }}>
                          <div style={{ width: `${pct}%`, height: '100%', background: 'linear-gradient(90deg, var(--accent-from), var(--accent-to))', transition: 'width 0.5s' }} />
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </div>

            <div className="card">
              <h3 style={{ fontSize: 14, fontWeight: 600, marginBottom: 16, color: 'var(--text-secondary)' }}>廠區分佈</h3>
              {(stats.by_region ?? []).length === 0 ? (
                <div style={{ color: 'var(--text-muted)', fontSize: 13 }}>尚無資料</div>
              ) : (
                <div>
                  {(stats.by_region ?? []).map((r: any) => {
                    const pct = stats.total_approved > 0 ? Math.round(r.count / stats.total_approved * 100) : 0
                    return (
                      <div key={r.region} style={{ marginBottom: 12 }}>
                        <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13, marginBottom: 4 }}>
                          <span>{r.region || '未設定'}</span>
                          <span style={{ color: 'var(--text-muted)' }}>{r.count} 人 ({pct}%)</span>
                        </div>
                        <div style={{ height: 6, background: 'rgba(255,255,255,0.08)', borderRadius: 3, overflow: 'hidden' }}>
                          <div style={{ width: `${pct}%`, height: '100%', background: 'linear-gradient(90deg, var(--info), var(--accent))', transition: 'width 0.5s' }} />
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </div>

            <div className="card">
              <h3 style={{ fontSize: 14, fontWeight: 600, marginBottom: 16, color: 'var(--text-secondary)' }}>票種統計</h3>
              {(stats.by_ticket_type ?? []).length === 0 ? (
                <div style={{ color: 'var(--text-muted)', fontSize: 13 }}>尚無資料</div>
              ) : (
                <div className="table-wrap" style={{ border: 'none' }}>
                  <table>
                    <thead><tr><th>票種</th><th>申請</th><th>核准</th></tr></thead>
                    <tbody>
                      {(stats.by_ticket_type ?? []).map((t: any) => (
                        <tr key={t.ticket_type_name}>
                          <td>{t.ticket_type_name}</td>
                          <td>{t.total}</td>
                          <td><span style={{ color: 'var(--success)' }}>{t.approved}</span></td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          </div>
        </>
      )}

      {!selectedEvent && !statsLoading && (
        <div className="empty-state">
          <div className="empty-state-icon">📊</div>
          <div className="empty-state-text">請選擇一個活動以查看詳細統計</div>
        </div>
      )}
    </div>
  )
}
