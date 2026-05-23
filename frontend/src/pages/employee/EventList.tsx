import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { MapPin, Calendar, Clock, Users } from 'lucide-react'
import api from '../../api/client'
import { useAuth } from '../../contexts/AuthContext'

function StatusBadge({ status }: { status: string }) {
  const map: Record<string, string> = {
    published: '發布中', draft: '草稿', closed: '已截止', ended: '已結束',
  }
  return <span className={`badge badge-${status}`}>{map[status] ?? status}</span>
}

function fmt(d: string) {
  return new Date(d).toLocaleDateString('zh-TW', { year: 'numeric', month: '2-digit', day: '2-digit' })
}

export default function EventList() {
  const { user } = useAuth()
  const navigate = useNavigate()
  const [statusFilter, setStatusFilter] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['events', statusFilter],
    queryFn: () => api.get('/events', { params: statusFilter ? { status: statusFilter } : {} }).then(r => r.data.data),
    refetchInterval: 30000, // 列表頁每 30 秒更新一次即可
  })

  if (isLoading) return <div className="empty-state"><div className="spinner" /></div>

  const events: any[] = data ?? []

  return (
    <div>
      <div className="page-header" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <div>
          <h1 className="page-title">活動列表</h1>
          <p className="page-subtitle">瀏覽所有可參加的福委會活動</p>
        </div>
      </div>

      <div className="filter-bar">
        <select className="form-select" value={statusFilter} onChange={e => setStatusFilter(e.target.value)} style={{ width: 140 }}>
          <option value="">全部狀態</option>
          <option value="published">發布中</option>
          <option value="closed">已截止</option>
          {user?.role === 'event_manager' && (
            <>
              <option value="draft">草稿</option>
              <option value="ended">已結束</option>
            </>
          )}
        </select>
        <span style={{ color: 'var(--text-muted)', fontSize: 13 }}>共 {events.length} 個活動</span>
      </div>

      {events.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon">📅</div>
          <div className="empty-state-text">目前沒有活動</div>
        </div>
      ) : (
        <div className="event-grid">
          {events.map((event: any) => (
            <div key={event.id} className="card card-clickable" onClick={() => navigate(`/events/${event.id}`)}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 12 }}>
                <div className="event-card-title">
                  {event.title}
                </div>
                <StatusBadge status={event.status} />
              </div>
              <div className="event-card-meta">
                <span><MapPin size={13} /> {event.venue}</span>
                <span><Calendar size={13} /> {fmt(event.start_time)}</span>
                <span><Clock size={13} /> 截止：{fmt(event.apply_deadline)}</span>
                {event.region_restriction && <span><Users size={13} />{event.region_restriction}</span>}
              </div>
              <div className="event-card-footer">
                <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                  {(event.ticket_types ?? []).map((tt: any) => (
                    <span key={tt.id} style={{ fontSize: 12, color: 'var(--text-secondary)' }}>
                      {tt.name}: <strong style={{ color: tt.remaining > 0 ? 'var(--success)' : 'var(--danger)' }}>{tt.remaining}</strong>/{tt.total_quota}
                    </span>
                  ))}
                </div>
                <span style={{ fontSize: 12, color: 'var(--accent)', fontWeight: 500 }}>查看詳情 →</span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
