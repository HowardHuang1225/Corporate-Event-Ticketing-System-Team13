import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from 'react-hot-toast'
import { AuthProvider } from './contexts/AuthProvider'
import ProtectedRoute from './components/ProtectedRoute'
import Layout from './components/Layout'
import Login from './pages/Login'
import EventList from './pages/employee/EventList'
import EventDetail from './pages/employee/EventDetail'
import MyTickets from './pages/employee/MyTickets'
import EventManage from './pages/manager/EventManage'
import Applications from './pages/manager/Applications'
import CheckIn from './pages/manager/CheckIn'
import Reports from './pages/hr/Reports'

const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 30_000, retry: 1 } },
})

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
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
                <Route path="my-tickets" element={<MyTickets />} />
                <Route path="manage/events" element={<EventManage />} />
                <Route path="applications" element={<Applications />} />
                <Route path="checkin" element={<CheckIn />} />
                <Route path="reports" element={<Reports />} />
              </Route>
            </Route>
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </BrowserRouter>
      </AuthProvider>
    </QueryClientProvider>
  )
}
