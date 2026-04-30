import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../contexts/AuthContext'

export default function Login() {
  const { login } = useAuth()
  const navigate = useNavigate()
  const [form, setForm] = useState({ employee_id: '', password: '' })
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const { user } = useAuth()
  if (user) { navigate('/events', { replace: true }); return null }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await login(form.employee_id, form.password)
      navigate('/events')
    } catch (err: any) {
      setError(err.response?.data?.error?.message || '登入失敗，請確認帳號密碼')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-page">
      <div className="login-box">
        <div className="login-logo">
          <div className="login-logo-icon">🎫</div>
          <div className="login-logo-title">員工票務系統</div>
          <div className="login-logo-sub">Corporate Event Ticketing System</div>
        </div>
        <div className="login-card">
          <form onSubmit={handleSubmit}>
            {error && <div className="login-error">{error}</div>}
            <div className="form-group">
              <label className="form-label">員工編號</label>
              <input
                id="employee-id"
                className="form-input"
                placeholder="例：EMP001"
                value={form.employee_id}
                onChange={e => setForm(f => ({ ...f, employee_id: e.target.value }))}
                required
              />
            </div>
            <div className="form-group">
              <label className="form-label">密碼</label>
              <input
                id="password"
                type="password"
                className="form-input"
                placeholder="••••••••"
                value={form.password}
                onChange={e => setForm(f => ({ ...f, password: e.target.value }))}
                required
              />
            </div>
            <button id="login-btn" type="submit" className="btn btn-primary btn-lg" style={{ width: '100%' }} disabled={loading}>
              {loading ? '登入中…' : '登入'}
            </button>
          </form>
          <div style={{ marginTop: 24, padding: 16, background: 'rgba(255,255,255,0.03)', borderRadius: 8, fontSize: 12, color: 'var(--text-muted)' }}>
            <strong style={{ color: 'var(--text-secondary)' }}>Demo 帳號</strong>
            <div style={{ marginTop: 8, display: 'flex', flexDirection: 'column', gap: 4 }}>
              <span>👔 MGR001 / password → 活動管理員</span>
              <span>👤 EMP001 / password → 員工（台南廠）</span>
              <span>👤 EMP002 / password → 員工（新竹廠）</span>
              <span>📊 HR001 / password → 人資部門</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
