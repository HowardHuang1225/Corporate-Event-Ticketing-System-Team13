import { useState, useEffect, useRef } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { ScanLine, History, Camera, X } from 'lucide-react'
import api from '../../api/client'

// QR code scanner modal component using native camera API and jsQR (dynamic CDN load)
function QrScannerModal({ onScan, onClose }: { onScan: (token: string) => void; onClose: () => void }) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const streamRef = useRef<MediaStream | null>(null)

  useEffect(() => {
    let active = true
    let animationFrameId: number

    const startCamera = async () => {
      try {
        setLoading(true)
        // 1. 動態加載 jsQR 庫（如不存在）
        if (!(window as any).jsQR) {
          await new Promise<void>((resolve, reject) => {
            const script = document.createElement('script')
            script.src = 'https://cdn.jsdelivr.net/npm/jsqr@1.4.0/dist/jsQR.min.js'
            script.onload = () => resolve()
            script.onerror = () => reject(new Error('無法載入 QR 解碼庫'))
            document.head.appendChild(script)
          })
        }

        if (!active) return

        // 2. 獲取相機權限與串流
        const stream = await navigator.mediaDevices.getUserMedia({
          video: { facingMode: 'environment', width: { ideal: 1280 }, height: { ideal: 720 } }
        })
        
        streamRef.current = stream
        if (videoRef.current) {
          videoRef.current.srcObject = stream
          videoRef.current.setAttribute('playsinline', 'true') // iOS Safari 必需
          await videoRef.current.play()
        }

        setLoading(false)

        // 3. 開始畫面循環擷取解碼
        const tick = () => {
          if (!active) return
          
          const video = videoRef.current
          const canvas = canvasRef.current
          if (video && canvas && video.readyState === video.HAVE_ENOUGH_DATA) {
            const ctx = canvas.getContext('2d')
            if (ctx) {
              canvas.width = video.videoWidth
              canvas.height = video.videoHeight
              
              // 繪製影像到 Canvas
              ctx.drawImage(video, 0, 0, canvas.width, canvas.height)
              
              // 獲取 Canvas 像素資料並透過 jsQR 解碼
              const imageData = ctx.getImageData(0, 0, canvas.width, canvas.height)
              const code = (window as any).jsQR(imageData.data, imageData.width, imageData.height, {
                inversionAttempts: 'dontInvert',
              })

              if (code && code.data) {
                // 掃描成功
                onScan(code.data)
                return
              }
            }
          }
          animationFrameId = requestAnimationFrame(tick)
        }
        
        tick()
      } catch (err: any) {
        console.error('Camera startup failed:', err)
        setError(err.message || '無法存取相機，請檢查瀏覽器權限。若在非安全連線 (http)，請改用 https 或 localhost。')
        setLoading(false)
      }
    }

    startCamera()

    return () => {
      active = false
      if (animationFrameId) cancelAnimationFrame(animationFrameId)
      if (streamRef.current) {
        streamRef.current.getTracks().forEach(track => track.stop())
      }
    }
  }, [onScan])

  return (
    <div style={{
      position: 'fixed', top: 0, left: 0, width: '100vw', height: '100vh',
      backgroundColor: 'rgba(0,0,0,0.85)', display: 'flex', justifyContent: 'center',
      alignItems: 'center', zIndex: 1000, backdropFilter: 'blur(10px)', transition: 'all 0.3s ease'
    }}>
      <style>{`
        .scanning-line {
          width: 100%;
          height: 3px;
          background-color: var(--accent, #6366f1);
          position: absolute;
          top: 0;
          left: 0;
          box-shadow: 0 0 10px var(--accent, #6366f1);
          animation: scan 2s linear infinite;
        }
        @keyframes scan {
          0% { top: 0%; }
          50% { top: 100%; }
          100% { top: 0%; }
        }
        .pulse-indicator {
          animation: pulse 1.5s ease-in-out infinite;
        }
        @keyframes pulse {
          0% { transform: scale(0.9); opacity: 0.6; }
          50% { transform: scale(1.2); opacity: 1; }
          100% { transform: scale(0.9); opacity: 0.6; }
        }
      `}</style>

      <div className="card" style={{
        width: '90%', maxWidth: '440px', padding: 24, position: 'relative',
        background: '#121212', border: '1px solid rgba(255,255,255,0.08)',
        boxShadow: '0 20px 40px rgba(0,0,0,0.6)', borderRadius: 16
      }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <div className="pulse-indicator" style={{ width: 8, height: 8, borderRadius: '50%', backgroundColor: 'var(--accent, #6366f1)' }} />
            <h3 style={{ fontSize: 17, fontWeight: 600, margin: 0, color: '#fff' }}>相機掃描 QR Code</h3>
          </div>
          <button onClick={onClose} className="btn" style={{ padding: '6px 10px', background: 'rgba(255,255,255,0.05)', display: 'flex', alignItems: 'center', cursor: 'pointer', border: 'none', borderRadius: 6, color: '#fff' }}>
            <X size={18} />
          </button>
        </div>

        <div style={{
          position: 'relative', width: '100%', aspectRatio: '1/1',
          background: '#000', borderRadius: 12, overflow: 'hidden',
          display: 'flex', justifyContent: 'center', alignItems: 'center',
          boxShadow: 'inset 0 0 20px rgba(0,0,0,0.8)'
        }}>
          {loading && (
            <div style={{ color: 'var(--text-muted)', fontSize: 13, textAlign: 'center' }}>
              正在授權相機權限及載理解析模組...
            </div>
          )}
          {error && (
            <div style={{ color: 'var(--error, #ef4444)', fontSize: 13, padding: '0 20px', textAlign: 'center', lineHeight: '1.5' }}>
              ⚠️ {error}
            </div>
          )}
          <video 
            ref={videoRef} 
            style={{ 
              width: '100%', height: '100%', objectFit: 'cover',
              display: (loading || error) ? 'none' : 'block' 
            }} 
          />
          <canvas ref={canvasRef} style={{ display: 'none' }} />
          
          {/* 掃描框效果 */}
          {!loading && !error && (
            <div style={{
              position: 'absolute', top: '50%', left: '50%',
              transform: 'translate(-50%, -50%)', width: '65%', height: '65%',
              border: '2px dashed var(--accent, #6366f1)', borderRadius: 12,
              boxShadow: '0 0 0 4000px rgba(0, 0, 0, 0.5)', pointerEvents: 'none'
            }}>
              <div className="scanning-line" />
            </div>
          )}
        </div>

        <div style={{ textAlign: 'center', marginTop: 16, fontSize: 12, color: 'var(--text-muted)', lineHeight: '1.4' }}>
          請將手機票券 QR Code 畫面置於框內。<br />
          偵測成功後會自動關閉並進行核銷。
        </div>
      </div>
    </div>
  )
}

export default function CheckIn() {
  const [token, setToken] = useState('')
  const [isScanning, setIsScanning] = useState(false)
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

  const handleSubmit = (e?: React.FormEvent) => {
    if (e) e.preventDefault()
    if (!token.trim()) return
    setResult(null)
    checkinMutation.mutate(token.trim())
  }

  // 掃描成功後自動填入並提交核銷
  const handleScanSuccess = (scannedToken: string) => {
    setIsScanning(false)
    setToken(scannedToken)
    setResult(null)
    checkinMutation.mutate(scannedToken)
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
              <div style={{ display: 'flex', gap: 8 }}>
                <input
                  id="qr-token-input"
                  className="form-input"
                  placeholder="輸入或掃描 QR Code 內容"
                  value={token}
                  onChange={e => setToken(e.target.value)}
                  autoFocus
                  autoComplete="off"
                  style={{ flex: 1 }}
                />
                <button
                  type="button"
                  className="btn"
                  style={{ 
                    display: 'flex', 
                    alignItems: 'center', 
                    justifyContent: 'center', 
                    background: 'rgba(255,255,255,0.06)', 
                    border: '1px solid var(--border)',
                    borderRadius: '8px',
                    padding: '0 14px',
                    cursor: 'pointer',
                    color: '#fff',
                    transition: 'all 0.2s ease'
                  }}
                  onClick={() => setIsScanning(true)}
                  title="開啟相機掃描"
                >
                  <Camera size={20} />
                </button>
              </div>
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
                    {new Date(c.checked_at).toLocaleString('zh-TW', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })}
                    <div style={{ color: 'var(--success)', fontSize: 10 }}>✅ 核銷</div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* QR 掃描 Modal */}
      {isScanning && (
        <QrScannerModal
          onScan={handleScanSuccess}
          onClose={() => setIsScanning(false)}
        />
      )}
    </div>
  )
}

