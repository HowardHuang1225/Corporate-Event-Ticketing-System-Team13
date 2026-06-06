import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { BarChart3, Users, Ticket, CheckSquare, Download, FileText } from 'lucide-react'
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
    const csvContent = rows.map(row => row.map(cell => `"${String(cell).replace(/"/g, '""')}"`).join(",")).join("\n")
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
    const headers = ["活動名稱", "申請次數", "申請張數", "申請人數", "核准張數", "退票張數", "有效票數", "實體核銷數", "核銷率"]
    const data = overview.map((o: any) => [
      o.title,
      o.applied_apps,
      o.applied_tickets,
      o.applied_users,
      o.approved_tickets,
      o.cancelled,
      o.total_tickets,
      o.checked_in,
      `${Math.round(o.check_in_rate)}%`
    ])
    exportToCSV(`活動總覽報表_${new Date().toISOString().slice(0, 10)}.csv`, [headers, ...data])
  }

  const handleExportDetail = () => {
    if (!stats) return
    const rows = [
      ["活動標題", stats.event.title],
      ["", ""],
      ["【申請階段】", ""],
      ["申請次數", String(stats.applied_apps)],
      ["申請總張數", String(stats.applied_tickets)],
      ["申請人數", String(stats.applied_users)],
      ["", ""],
      ["【核准/退票階段】", ""],
      ["原始核准張數", String(stats.approved_tickets)],
      ["已退票張數", String(stats.cancelled_tickets)],
      ["目前有效票數 (核准 - 退票)", String(stats.total_tickets)],
      ["獲票人數", String(stats.approved_users)],
      ["", ""],
      ["【核銷階段】", ""],
      ["核銷張數", String(stats.checked_in_tickets)],
      ["核銷率", `${Math.round(stats.check_in_rate)}%`],
      ["", ""],
      ["部門分佈", "人數"],
      ...(stats.by_department || []).map((d: any) => [d.department, String(d.count)]),
      ["", ""],
      ["票種統計 (張數)", "申請張數", "核准張數", "已退票張數", "有效張數"],
      ...(stats.by_ticket_type || []).map((t: any) => [t.ticket_type_name, String(t.total), String(t.approved), String(t.cancelled), String(t.active)])
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

      {/* Overview Table */}
      {overview && (
        <div style={{ marginBottom: 32 }}>
          <h2 style={{ fontSize: 16, fontWeight: 600, marginBottom: 16 }}>活動總覽</h2>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>活動名稱</th>
                  <th>申請次數</th>
                  <th>申請張數</th>
                  <th>核准張數</th>
                  <th>退票張數</th>
                  <th>有效票數</th>
                  <th>核銷張數</th>
                  <th>核銷率</th>
                </tr>
              </thead>
              <tbody>
                {overview.map((o: any) => {
                  const rate = Math.round(o.check_in_rate)
                  return (
                    <tr key={o.event_id}>
                      <td style={{ fontWeight: 500 }}>{o.title}</td>
                      <td>{o.applied_apps}</td>
                      <td>{o.applied_tickets}</td>
                      <td>{o.approved_tickets}</td>
                      <td style={{ color: o.cancelled > 0 ? 'var(--error)' : 'inherit' }}>{o.cancelled}</td>
                      <td>{o.total_tickets}</td>
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
          {/* ── 申請階段 ── */}
          <div style={{ marginBottom: 8 }}>
            <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-muted)', letterSpacing: 1 }}>1. 申請階段</span>
          </div>
          <div className="stats-grid" style={{ marginBottom: 24 }}>
            {[
              { icon: <FileText size={20} />, label: '申請次數', value: stats.applied_apps, color: 'var(--info)' },
              { icon: <Ticket size={20} />, label: '申請張數', value: stats.applied_tickets, color: 'var(--info)' },
              { icon: <Users size={20} />, label: '申請人數', value: stats.applied_users, color: 'var(--info)' },
            ].map(s => (
              <div key={s.label} className="stat-card">
                <div style={{ color: s.color, marginBottom: 8 }}>{s.icon}</div>
                <div className="stat-value" style={{ color: s.color }}>{s.value}</div>
                <div className="stat-label">{s.label}</div>
              </div>
            ))}
          </div>

          {/* ── 核准與退票 ── */}
          <div style={{ marginBottom: 8 }}>
            <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-muted)', letterSpacing: 1 }}>2. 核准與退票階段</span>
          </div>
          <div className="stats-grid" style={{ marginBottom: 24 }}>
            {[
              { icon: <CheckSquare size={20} />, label: '核准總張數', value: stats.approved_tickets, color: 'var(--success)' },
              { icon: <Ticket size={20} />, label: '已退票張數', value: stats.cancelled_tickets, color: 'var(--error)' },
              { icon: <Ticket size={20} />, label: '目前有效票數', value: stats.total_tickets, color: 'var(--success)' },
              { icon: <Users size={20} />, label: '獲票總人數', value: stats.approved_users, color: 'var(--success)' },
            ].map(s => (
              <div key={s.label} className="stat-card">
                <div style={{ color: s.color, marginBottom: 8 }}>{s.icon}</div>
                <div className="stat-value" style={{ color: s.color }}>{s.value}</div>
                <div className="stat-label">{s.label}</div>
              </div>
            ))}
          </div>

          {/* ── 核銷階段 ── */}
          <div style={{ marginBottom: 8 }}>
            <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-muted)', letterSpacing: 1 }}>3. 核銷與出席率</span>
          </div>
          <div className="stats-grid" style={{ marginBottom: 24 }}>
            {[
              { icon: <BarChart3 size={20} />, label: '實體核銷張數', value: stats.checked_in_tickets, color: 'var(--accent)' },
              { icon: <CheckSquare size={20} />, label: '核銷率 (基於有效票)', value: `${Math.round(stats.check_in_rate)}%`, color: 'var(--warning)' },
            ].map(s => (
              <div key={s.label} className="stat-card">
                <div style={{ color: s.color, marginBottom: 8 }}>{s.icon}</div>
                <div className="stat-value" style={{ color: s.color }}>{s.value}</div>
                <div className="stat-label">{s.label}</div>
              </div>
            ))}
          </div>

          {/* ── 分佈圖表 ── */}
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(300px, 1fr))', gap: 20 }}>
            <div className="card">
              <h3 style={{ fontSize: 14, fontWeight: 600, marginBottom: 16, color: 'var(--text-secondary)' }}>部門分佈 (依獲票人數)</h3>
              {(stats.by_department ?? []).length === 0 ? (
                <div style={{ color: 'var(--text-muted)', fontSize: 13 }}>尚無資料</div>
              ) : (
                <div>
                  {(stats.by_department ?? []).map((d: any) => {
                    const pct = stats.approved_users > 0 ? Math.round(d.count / stats.approved_users * 100) : 0
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
              <h3 style={{ fontSize: 14, fontWeight: 600, marginBottom: 16, color: 'var(--text-secondary)' }}>廠區分佈 (依獲票人數)</h3>
              {(stats.by_region ?? []).length === 0 ? (
                <div style={{ color: 'var(--text-muted)', fontSize: 13 }}>尚無資料</div>
              ) : (
                <div>
                  {(stats.by_region ?? []).map((r: any) => {
                    const pct = stats.approved_users > 0 ? Math.round(r.count / stats.approved_users * 100) : 0
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
              <h3 style={{ fontSize: 14, fontWeight: 600, marginBottom: 16, color: 'var(--text-secondary)' }}>票種統計 (張數)</h3>
              {(stats.by_ticket_type ?? []).length === 0 ? (
                <div style={{ color: 'var(--text-muted)', fontSize: 13 }}>尚無資料</div>
              ) : (
                <div className="table-wrap" style={{ border: 'none' }}>
                  <table>
                    <thead><tr><th>票種</th><th>申請</th><th>核准</th><th>退票</th><th>有效</th></tr></thead>
                    <tbody>
                      {(stats.by_ticket_type ?? []).map((t: any) => (
                        <tr key={t.ticket_type_name}>
                          <td>{t.ticket_type_name}</td>
                          <td>{t.total}</td>
                          <td><span style={{ color: 'var(--success)' }}>{t.approved}</span></td>
                          <td><span style={{ color: t.cancelled > 0 ? 'var(--error)' : 'inherit' }}>{t.cancelled}</span></td>
                          <td style={{ fontWeight: 600 }}>{t.active}</td>
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
