import { describe, expect, it, beforeEach, vi, type Mock } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AuthProvider } from './AuthProvider'
import { useAuth } from './AuthContext'
import api from '../api/client'
import { createTestQueryClient, renderWithQueryClient } from '../test/test-utils'

vi.mock('../api/client', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(),
  },
}))

const apiGet = api.get as Mock
const apiPost = api.post as Mock

const fakeUser = {
  id: 'user-1',
  employee_id: 'EMP001',
  name: '王小明',
  email: 'ming@example.com',
  department: 'IT',
  region: '台南',
  role: 'employee' as const,
}

function AuthProbe() {
  const { user, isLoading, login, logout } = useAuth()

  return (
    <div>
      <div data-testid="loading">{String(isLoading)}</div>
      <div data-testid="user">{user?.name ?? 'none'}</div>
      <button onClick={() => void login('EMP001', 'password')}>執行登入</button>
      <button onClick={logout}>執行登出</button>
    </div>
  )
}

function renderAuthProvider(queryClient = createTestQueryClient()) {
  return renderWithQueryClient(
    <AuthProvider>
      <AuthProbe />
    </AuthProvider>,
    { queryClient },
  )
}

describe('AuthProvider', () => {
  beforeEach(() => {
    apiGet.mockReset()
    apiPost.mockReset()
    localStorage.clear()
  })

  it('沒有 token 時會結束載入且不呼叫 /auth/me', async () => {
    console.info('確認初始化沒有 token 的登入狀態')

    renderAuthProvider()

    await waitFor(() => expect(screen.getByTestId('loading')).toHaveTextContent('false'))
    expect(screen.getByTestId('user')).toHaveTextContent('none')
    expect(apiGet).not.toHaveBeenCalled()
  })

  it('有 token 時會透過 /auth/me 載入目前使用者', async () => {
    console.info('確認初始化有 token 時會取得目前使用者')
    localStorage.setItem('token', 'existing-token')
    apiGet.mockResolvedValue({ data: { data: fakeUser } })

    renderAuthProvider()

    await waitFor(() => expect(screen.getByTestId('user')).toHaveTextContent('王小明'))
    expect(apiGet).toHaveBeenCalledWith('/auth/me')
    expect(screen.getByTestId('loading')).toHaveTextContent('false')
  })

  it('載入目前使用者失敗時會移除失效 token', async () => {
    console.info('確認 /auth/me 失敗時會清掉 token')
    localStorage.setItem('token', 'expired-token')
    apiGet.mockRejectedValue(new Error('unauthorized'))

    renderAuthProvider()

    await waitFor(() => expect(screen.getByTestId('loading')).toHaveTextContent('false'))
    expect(localStorage.getItem('token')).toBeNull()
    expect(screen.getByTestId('user')).toHaveTextContent('none')
  })

  it('login 成功時會儲存 access token 並更新使用者', async () => {
    console.info('確認 login 成功後更新 token 與 user state')
    apiPost.mockResolvedValue({
      data: {
        data: {
          access_token: 'new-token',
          user: fakeUser,
        },
      },
    })

    renderAuthProvider()
    await userEvent.click(screen.getByRole('button', { name: '執行登入' }))

    await waitFor(() => expect(screen.getByTestId('user')).toHaveTextContent('王小明'))
    expect(apiPost).toHaveBeenCalledWith('/auth/login', {
      employee_id: 'EMP001',
      password: 'password',
    })
    expect(localStorage.getItem('token')).toBe('new-token')
  })

  it('logout 會清除 token、使用者與 React Query 快取', async () => {
    console.info('確認 logout 會清除驗證狀態與 query cache')
    localStorage.setItem('token', 'existing-token')
    apiGet.mockResolvedValue({ data: { data: fakeUser } })
    const queryClient = createTestQueryClient()
    const clearSpy = vi.spyOn(queryClient, 'clear')

    renderAuthProvider(queryClient)
    await waitFor(() => expect(screen.getByTestId('user')).toHaveTextContent('王小明'))
    await userEvent.click(screen.getByRole('button', { name: '執行登出' }))

    expect(localStorage.getItem('token')).toBeNull()
    expect(screen.getByTestId('user')).toHaveTextContent('none')
    expect(clearSpy).toHaveBeenCalled()
  })
})
