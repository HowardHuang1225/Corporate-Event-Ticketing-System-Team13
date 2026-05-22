import { describe, expect, it, beforeEach, vi, type Mock } from 'vitest'
import { screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import ProtectedRoute from './ProtectedRoute'
import { useAuth } from '../contexts/AuthContext'
import { renderWithQueryClient } from '../test/test-utils'

vi.mock('../contexts/AuthContext', () => ({
  useAuth: vi.fn(),
}))

const mockUseAuth = useAuth as Mock

const fakeUser = {
  id: 'user-1',
  employee_id: 'EMP001',
  name: '王小明',
  email: 'ming@example.com',
  department: 'IT',
  region: '台南',
  role: 'employee' as const,
}

function renderProtectedRoute() {
  return renderWithQueryClient(
    <MemoryRouter initialEntries={['/private']}>
      <Routes>
        <Route element={<ProtectedRoute />}>
          <Route path="/private" element={<div>受保護內容</div>} />
        </Route>
        <Route path="/login" element={<div>登入頁</div>} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('ProtectedRoute', () => {
  beforeEach(() => {
    mockUseAuth.mockReset()
  })

  it('驗證狀態載入中時會顯示 loading 畫面', () => {
    console.info('確認 protected route 的 loading state')
    mockUseAuth.mockReturnValue({
      user: null,
      isLoading: true,
      login: vi.fn(),
      logout: vi.fn(),
    })

    renderProtectedRoute()

    expect(screen.getByText('載入中…')).toBeInTheDocument()
    expect(screen.queryByText('受保護內容')).not.toBeInTheDocument()
  })

  it('未登入時會導向登入頁', () => {
    console.info('確認未登入使用者會被導向 login route')
    mockUseAuth.mockReturnValue({
      user: null,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
    })

    renderProtectedRoute()

    expect(screen.getByText('登入頁')).toBeInTheDocument()
    expect(screen.queryByText('受保護內容')).not.toBeInTheDocument()
  })

  it('已登入時會顯示受保護的子路由內容', () => {
    console.info('確認已登入使用者可以看到 outlet 內容')
    mockUseAuth.mockReturnValue({
      user: fakeUser,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
    })

    renderProtectedRoute()

    expect(screen.getByText('受保護內容')).toBeInTheDocument()
  })
})
