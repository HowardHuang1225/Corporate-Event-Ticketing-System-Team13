import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from 'react-hot-toast'
import { type ReactNode } from 'react'
import { AuthProvider } from './contexts/AuthProvider'
import { useAuth } from './contexts/AuthContext'
import ProtectedRoute from './components/ProtectedRoute'
import Layout from './components/Layout'
import Login from './pages/Login'
import EventList from './pages/employee/EventList'
import EventDetail from './pages/employee/EventDetail'
import MyTickets from './pages/employee/MyTickets'
import EventManage from './pages/manager/EventManage'
import CheckIn from './pages/manager/CheckIn'
import Reports from './pages/hr/Reports'
import { appQueryClient } from './lib/queryClient'

function RoleRoute({ allowedRoles, children }: { allowedRoles: string[]; children: ReactNode }) {
  const { user } = useAuth()

  if (!user || !allowedRoles.includes(user.role)) {
    return <Navigate to="/events" replace />
  }

  return <>{children}</>
}

export default function App() {
  return (
    <QueryClientProvider client={appQueryClient}>
      <AuthProvider>
        <BrowserRouter>
          <Toaster
            position="top-right"
            toastOptions={{
              style: { background: '#1e1e2e', color: '#f1f5f9', border: '1px solid rgba(255,255,255,0.08)' },
              success: { iconTheme: { primary: '#10b981', secondary: '#fff' } },
              error: { iconTheme: { primary: '#ef4444', secondary: '#fff' } },
            }}
          />
          <Routes>
            <Route path="/login" element={<Login />} />
            <Route element={<ProtectedRoute />}>
              <Route element={<Layout />}>
                <Route index element={<Navigate to="/events" replace />} />
                <Route path="events" element={<EventList />} />
                <Route path="events/:id" element={<EventDetail />} />
                <Route path="my-tickets" element={<RoleRoute allowedRoles={['employee']}><MyTickets /></RoleRoute>} />
                <Route path="manage/events" element={<RoleRoute allowedRoles={['event_manager']}><EventManage /></RoleRoute>} />
                <Route path="checkin" element={<RoleRoute allowedRoles={['event_manager']}><CheckIn /></RoleRoute>} />
                <Route path="reports" element={<RoleRoute allowedRoles={['hr']}><Reports /></RoleRoute>} />
              </Route>
            </Route>
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </BrowserRouter>
      </AuthProvider>
    </QueryClientProvider>
  )
}
