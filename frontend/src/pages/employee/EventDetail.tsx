import { useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { MapPin, Calendar, Clock, Users, ArrowLeft, Ticket } from 'lucide-react'
import toast from 'react-hot-toast'
import { v4 as uuidv4 } from 'uuid'
import api from '../../api/client'
import { useAuth } from '../../contexts/AuthContext'

function fmt(d: string) {
  return new Date(d).toLocaleString('zh-TW', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

export default function EventDetail() {
  const { id } = useParams<{ id: string }>()
  const { user } = useAuth()
  const navigate = useNavigate()
  const qc = useQueryClient()

  const [showModal, setShowModal] = useState(false)
  const [showImagePreview, setShowImagePreview] = useState(false)
  const [selectedType, setSelectedType] = useState('')
  const [quantity, setQuantity] = useState(1)

  const { data, isLoading } = useQuery({
    queryKey: ['event', id],
    queryFn: () => api.get(`/events/${id}`).then(r => r.data.data),
    refetchInterval: 10000, // 每 10 秒自動重新抓取一次
  })

  const applyMutation = useMutation({
    mutationFn: (payload: any) => api.post('/applications', payload),
    onSuccess: (res) => {
      if (res.status === 202 || res.data?.data?.status === 'queued') {
        toast.success('已進入排隊，系統會依序處理申請')
      } else {
        toast.success('搶票成功！已為您自動發票')
      }
      setShowModal(false)
      qc.invalidateQueries({ queryKey: ['event', id] })
      qc.invalidateQueries({ queryKey: ['my-applications'] })
      qc.invalidateQueries({ queryKey: ['my-tickets'] })
    },
    onError: (err: any) => {
      toast.error(err.response?.data?.error?.message ?? '申請失敗')
    },
  })

  const handleApply = () => {
    if (!selectedType) { toast.error('請選擇票種'); return }
    applyMutation.mutate({
      event_id: id,
      ticket_type_id: selectedType,
      quantity,
      idempotency_key: uuidv4(),
    })
  }

  if (isLoading) return <div className="empty-state"><div className="spinner" /></div>
  if (!data) return <div className="empty-state"><div className="empty-state-text">找不到活動</div></div>

  const event = data
  const canApply = user?.role === 'employee' && event.status === 'published' && new Date(event.apply_deadline) > new Date()
  const maxQ = event.max_tickets_per_person ?? 1
  const selectedTT = (event.ticket_types ?? []).find((tt: any) => tt.id === selectedType)
  const isRegionMismatch = (() => {
    if (!event.region_restriction || !user || !user.region) return false
    const eventReg = event.region_restriction.toLowerCase()
    const userReg = user.region.toLowerCase()
    if (userReg.includes(eventReg) || eventReg.includes(userReg)) return false
    const tainanKeywords = ['tainan', '台南']
    const hsinchuKeywords = ['hsinchu', '新竹']
    const isUserTainan = tainanKeywords.some(k => userReg.includes(k))
    const isEventTainan = tainanKeywords.some(k => eventReg.includes(k))
    if (isUserTainan && isEventTainan) return false
    const isUserHsinchu = hsinchuKeywords.some(k => userReg.includes(k))
    const isEventHsinchu = hsinchuKeywords.some(k => eventReg.includes(k))
    if (isUserHsinchu && isEventHsinchu) return false
    return true
  })()

  return (
    <div>
      <button className="btn btn-secondary btn-sm" onClick={() => navigate(-1)} style={{ marginBottom: 20 }}>
        <ArrowLeft size={14} /> 返回
      </button>

      <div className="card" style={{ marginBottom: 20 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 16 }}>
          <h1 style={{ fontSize: 22, fontWeight: 700 }}>{event.title}</h1>
          <span className={`badge badge-${event.status}`}>{event.status === 'published' ? '發布中' : event.status === 'draft' ? '草稿' : event.status === 'ended' ? '已結束' : '已截止'}</span>
        </div>
        <p style={{ color: 'var(--text-secondary)', lineHeight: 1.7, marginBottom: 20 }}>{event.description}</p>

        {event.image_url && (
          <div style={{ marginBottom: 20 }}>
            <div 
              style={{ display: 'inline-block', position: 'relative', cursor: 'pointer', maxWidth: '100%', borderRadius: 8, overflow: 'hidden', border: '1px solid var(--border)' }}
              onClick={() => setShowImagePreview(true)}
              title="點擊放大預覽"
            >
              <img src={event.image_url} alt="活動海報" style={{ display: 'block', maxWidth: '100%', maxHeight: 400, transition: 'transform 0.3s ease' }} onMouseOver={e => e.currentTarget.style.transform = 'scale(1.05)'} onMouseOut={e => e.currentTarget.style.transform = 'scale(1)'} />
            </div>
          </div>
        )}
        {event.document_url && (
          <div style={{ marginBottom: 20 }}>
            <a href={event.document_url} target="_blank" rel="noreferrer" className="btn btn-secondary btn-sm" style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
              📄 檢視活動附件 (PDF)
            </a>
          </div>
        )}
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
          {[
            { icon: <MapPin size={16} />, label: '地點', val: event.venue },
            { icon: <Calendar size={16} />, label: '活動時間', val: `${fmt(event.start_time)} — ${fmt(event.end_time)}` },
            { icon: <Clock size={16} />, label: '申請截止', val: fmt(event.apply_deadline) },
            { icon: <Users size={16} />, label: '活動地域', val: event.region_restriction ?? '不限制' },
            { icon: <Ticket size={16} />, label: '每人票數上限', val: `${event.max_tickets_per_person} 張` },
          ].map(item => (
            <div key={item.label} style={{ display: 'flex', gap: 10, alignItems: 'flex-start' }}>
              <span style={{ color: 'var(--accent)', marginTop: 2, flexShrink: 0 }}>{item.icon}</span>
              <div>
                <div style={{ fontSize: 11, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.5px' }}>{item.label}</div>
                <div style={{ fontSize: 14, marginTop: 2 }}>{item.val}</div>
              </div>
            </div>
          ))}
        </div>
      </div>

      {isRegionMismatch && (
        <div style={{
          background: 'var(--warning-light)',
          border: '1px solid var(--warning)',
          color: 'var(--warning)',
          padding: '12px 16px',
          borderRadius: 12,
          marginBottom: 20,
          fontSize: 14,
          display: 'flex',
          alignItems: 'center',
          gap: 10,
        }}>
          <span>⚠️</span>
          <div>
            <strong>地域提醒：</strong>本活動主要地域為「{event.region_restriction}」（您的所屬廠區為「{user?.region || '未設定'}」），但您仍可以報名搶票。
          </div>
        </div>
      )}

      <div className="card">
        <h2 style={{ fontSize: 16, fontWeight: 600, marginBottom: 16 }}>可選票種</h2>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          {(event.ticket_types ?? []).map((tt: any) => (
            <div key={tt.id} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '14px 16px', background: 'rgba(255,255,255,0.03)', borderRadius: 10, border: '1px solid var(--border)' }}>
              <div>
                <div style={{ fontWeight: 500 }}>{tt.name}</div>
                <div style={{ fontSize: 12, color: 'var(--text-muted)', marginTop: 4 }}>
                  剩餘 <strong style={{ color: tt.remaining > 0 ? 'var(--success)' : 'var(--danger)' }}>{tt.remaining}</strong> / {tt.total_quota} 張
                </div>
              </div>
              {canApply && tt.remaining > 0 && (
                <button className="btn btn-primary btn-sm" onClick={() => { setSelectedType(tt.id); setShowModal(true); setQuantity(1) }}>
                  申請
                </button>
              )}
              {tt.remaining === 0 && <span className="badge badge-rejected">已售罄</span>}
            </div>
          ))}
        </div>
        {!canApply && user?.role === 'employee' && (
          <p style={{ marginTop: 12, fontSize: 13, color: 'var(--text-muted)' }}>
            {event.status !== 'published' ? '此活動目前不開放申請' : '申請截止時間已過'}
          </p>
        )}
      </div>

      {showModal && (
        <div className="modal-overlay" onClick={e => e.target === e.currentTarget && setShowModal(false)}>
          <div className="modal">
            <div className="modal-header">
              <span className="modal-title">申請票券</span>
              <button className="modal-close" onClick={() => setShowModal(false)}>✕</button>
            </div>
            <p style={{ color: 'var(--text-secondary)', marginBottom: 20 }}>{event.title}</p>
            {isRegionMismatch && (
              <div style={{
                background: 'var(--warning-light)',
                border: '1px solid var(--warning)',
                color: 'var(--warning)',
                padding: '10px 14px',
                borderRadius: 8,
                marginBottom: 16,
                fontSize: 13,
              }}>
                ⚠️ 提醒：您的所屬廠區（{user?.region || '未設定'}）與本活動主要地域（{event.region_restriction}）不同，但仍可繼續報名。
              </div>
            )}
            <div className="form-group">
              <label className="form-label">票種</label>
              <div style={{ padding: '10px 14px', background: 'rgba(255,255,255,0.04)', borderRadius: 8, border: '1px solid var(--border)' }}>
                {selectedTT?.name ?? '—'}（剩餘 {selectedTT?.remaining} 張）
              </div>
            </div>
            <div className="form-group">
              <label className="form-label">數量（最多 {maxQ} 張）</label>
              <input
                type="number" min={1} max={Math.min(maxQ, selectedTT?.remaining ?? 1)}
                value={quantity}
                onChange={e => setQuantity(Number(e.target.value))}
                className="form-input"
              />
            </div>
            <div className="modal-footer">
              <button className="btn btn-secondary" onClick={() => setShowModal(false)}>取消</button>
              <button className="btn btn-primary" onClick={handleApply} disabled={applyMutation.isPending}>
                {applyMutation.isPending ? '送出中…' : '確認申請'}
              </button>
            </div>
          </div>
        </div>
      )}

      {showImagePreview && event.image_url && (
        <div className="modal-overlay" onClick={() => setShowImagePreview(false)} style={{ zIndex: 2000, display: 'flex', justifyContent: 'center', alignItems: 'center' }}>
          <button
            onClick={() => setShowImagePreview(false)}
            style={{ position: 'absolute', top: 24, right: 24, background: 'rgba(0,0,0,0.6)', border: 'none', color: '#fff', fontSize: 28, cursor: 'pointer', width: 48, height: 48, borderRadius: '50%', zIndex: 10, display: 'flex', justifyContent: 'center', alignItems: 'center', transition: 'background 0.2s' }}
            onMouseOver={e => e.currentTarget.style.background = 'rgba(0,0,0,0.8)'}
            onMouseOut={e => e.currentTarget.style.background = 'rgba(0,0,0,0.6)'}
          >
            ✕
          </button>
          <img src={event.image_url} alt="活動海報預覽" style={{ width: '90vw', height: '90vh', objectFit: 'contain' }} />
        </div>
      )}
    </div>
  )
}