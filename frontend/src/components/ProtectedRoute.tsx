import { Navigate, Outlet } from 'react-router-dom'
import { useAuth } from '../contexts/AuthContext'

export default function ProtectedRoute() {
  const { user, isLoading } = useAuth()
  if (isLoading) return <div className="loading-screen"><div><div className="spinner" /><p>載入中…</p></div></div>
  if (!user) return <Navigate to="/login" replace />
  return <Outlet />
}
