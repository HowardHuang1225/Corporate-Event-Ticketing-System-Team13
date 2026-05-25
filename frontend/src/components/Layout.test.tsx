import { describe, expect, it, beforeEach, vi, type Mock } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import Layout from './Layout'
import { useAuth } from '../contexts/AuthContext'
import { renderWithQueryClient } from '../test/test-utils'

vi.mock('../contexts/AuthContext', () => ({
  useAuth: vi.fn(),
}))

const mockUseAuth = useAuth as Mock

function fakeUser(role: 'employee' | 'event_manager' | 'hr') {
  return {
    id: `user-${role}`,
    employee_id: 'EMP001',
    name: '王小明',
    email: 'ming@example.com',
    department: 'IT',
    region: '台南',
    role,
  }
}

function renderLayout(role: 'employee' | 'event_manager' | 'hr', logout = vi.fn()) {
  mockUseAuth.mockReturnValue({
    user: fakeUser(role),
    isLoading: false,
    login: vi.fn(),
    logout,
  })

  renderWithQueryClient(
    <MemoryRouter initialEntries={['/events']}>
      <Routes>
        <Route path="/" element={<Layout />}>
          <Route path="events" element={<div>子頁面內容</div>} />
        </Route>
        <Route path="/login" element={<div>登入頁</div>} />
      </Routes>
    </MemoryRouter>,
  )

  return { logout }
}

describe('Layout', () => {
  beforeEach(() => {
    mockUseAuth.mockReset()
  })

  it('employee 角色只會看到活動列表與我的票券導覽', () => {
    console.info('確認 employee 導覽項目')

    renderLayout('employee')

    expect(screen.getByText('活動列表')).toBeInTheDocument()
    expect(screen.getByText('我的票券')).toBeInTheDocument()
    expect(screen.queryByText('活動管理')).not.toBeInTheDocument()
    expect(screen.queryByText('統計報表')).not.toBeInTheDocument()
  })

  it('event_manager 角色會看到管理相關導覽', () => {
    console.info('確認 event_manager 導覽項目')

    renderLayout('event_manager')

    expect(screen.getByText('活動列表')).toBeInTheDocument()
    expect(screen.getByText('活動管理')).toBeInTheDocument()
    expect(screen.getByText('現場核銷')).toBeInTheDocument()
    expect(screen.queryByText('我的票券')).not.toBeInTheDocument()
  })

  it('hr 角色會看到統計報表導覽', () => {
    console.info('確認 hr 導覽項目')

    renderLayout('hr')

    expect(screen.getByText('活動列表')).toBeInTheDocument()
    expect(screen.getByText('統計報表')).toBeInTheDocument()
    expect(screen.queryByText('活動管理')).not.toBeInTheDocument()
  })

  it('按下登出時會呼叫 logout 並導向登入頁', async () => {
    console.info('確認側邊欄登出流程')
    const { logout } = renderLayout('employee')

    await userEvent.click(screen.getByRole('button', { name: /登出/ }))

    expect(logout).toHaveBeenCalled()
    expect(screen.getByText('登入頁')).toBeInTheDocument()
  })
})
