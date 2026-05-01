import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { useAuth } from '../contexts/AuthContext'
import {
  CalendarDays, Ticket, CheckSquare, BarChart3, Settings, LogOut, ClipboardList,
} from 'lucide-react'

function getNav(role?: string) {
  if (role === 'event_manager') return [
    { to: '/events', label: '活動列表', icon: <CalendarDays className="nav-icon" /> },
    { to: '/manage/events', label: '活動管理', icon: <Settings className="nav-icon" /> },
    { to: '/applications', label: '申請審核', icon: <ClipboardList className="nav-icon" /> },
    { to: '/checkin', label: '現場核銷', icon: <CheckSquare className="nav-icon" /> },
  ]
  if (role === 'hr') return [
    { to: '/events', label: '活動列表', icon: <CalendarDays className="nav-icon" /> },
    { to: '/reports', label: '統計報表', icon: <BarChart3 className="nav-icon" /> },
  ]
  return [
    { to: '/events', label: '活動列表', icon: <CalendarDays className="nav-icon" /> },
    { to: '/my-tickets', label: '我的票券', icon: <Ticket className="nav-icon" /> },
  ]
}

export default function Layout() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  const handleLogout = () => { logout(); navigate('/login') }
  const navItems = getNav(user?.role)

  return (
    <div className="layout">
      <aside className="sidebar">
        <div className="sidebar-logo">🎫 Tickets</div>
        <nav className="sidebar-nav">
          {navItems.map(item => (
            <NavLink key={item.to} to={item.to} className={({ isActive }) => `nav-item${isActive ? ' active' : ''}`}>
              {item.icon}
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="sidebar-footer">
          <div className="sidebar-user-name">{user?.name}</div>
          <div className="sidebar-user-role">
            {user?.role === 'event_manager' ? '活動管理員' : user?.role === 'hr' ? '人資部門' : '員工'}
            {user?.region && <span> · {user.region}</span>}
          </div>
          <button className="logout-btn" onClick={handleLogout}>
            <LogOut size={14} style={{ marginRight: 4 }} /> 登出
          </button>
        </div>
      </aside>
      <main className="main-content">
        <Outlet />
      </main>
    </div>
  )
}
