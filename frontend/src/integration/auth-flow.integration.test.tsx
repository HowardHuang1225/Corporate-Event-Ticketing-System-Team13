import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from '../App'
import api from '../api/client'
import { resetAppQueryClient } from '../test/test-utils'

vi.mock('../api/client', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(),
  },
}))

const employeeUser = {
  id: 'employee-1',
  employee_id: 'EMP001',
  name: '員工小明',
  email: 'employee@example.com',
  department: '資訊部',
  region: '台南',
  role: 'employee' as const,
}

const managerUser = {
  id: 'manager-1',
  employee_id: 'MGR001',
  name: '管理者小美',
  email: 'manager@example.com',
  department: '行政部',
  region: '台北',
  role: 'event_manager' as const,
}

const eventFixture = {
  id: 'event-1',
  title: '年度家庭日',
  description: '員工家庭日活動',
  venue: '台南園區',
  status: 'published',
  publish_time: '2026-06-01T01:00:00.000Z',
  start_time: '2026-07-01T01:00:00.000Z',
  end_time: '2026-07-01T09:00:00.000Z',
  apply_deadline: '2026-06-20T09:00:00.000Z',
  region_restriction: '台南',
  ticket_types: [
    {
      id: 'ticket-type-1',
      name: '一般票',
      remaining: 10,
      total_quota: 30,
    },
  ],
}

async function renderApp(
  path: string,
  configureApi: (api: {
    get: ReturnType<typeof vi.fn>
    post: ReturnType<typeof vi.fn>
  }) => void = () => undefined,
) {
  window.history.pushState({}, '', path)
  const get = vi.mocked(api.get)
  const post = vi.mocked(api.post)
  get.mockReset()
  post.mockReset()
  configureApi({ get, post })

  return {
    ...render(<App />),
    api: { get, post },
  }
}

describe('登入與授權整合流程', () => {
  beforeEach(() => {
    resetAppQueryClient()
    localStorage.clear()
    window.history.pushState({}, '', '/')
  })

  it('未登入使用者進入受保護頁面時會被導向登入頁', async () => {
    console.info('確認 App + AuthProvider + ProtectedRoute 的未登入導向流程')
    const { api } = await renderApp('/events')

    await waitFor(() => expect(window.location.pathname).toBe('/login'))
    expect(screen.getByText('Ticketing System')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '登入' })).toBeInTheDocument()
    expect(api.get).not.toHaveBeenCalledWith('/auth/me')
  })

  it('登入成功後會儲存 token 並進入活動列表', async () => {
    console.info('確認登入表單、AuthProvider 與活動列表查詢可以串起來')
    const { container, api } = await renderApp('/login', ({ get, post }) => {
      post.mockResolvedValue({
        data: {
          data: {
            access_token: 'employee-token',
            user: employeeUser,
          },
        },
      })
      get.mockResolvedValue({ data: { data: [eventFixture] } })
    })
    const employeeInput = container.querySelector<HTMLInputElement>('#employee-id')
    const passwordInput = container.querySelector<HTMLInputElement>('#password')

    if (!employeeInput || !passwordInput) {
      throw new Error('登入表單欄位沒有正確渲染')
    }

    await userEvent.type(employeeInput, 'EMP001')
    await userEvent.type(passwordInput, 'password')
    await userEvent.click(screen.getByRole('button', { name: '登入' }))

    await waitFor(() => {
      expect(api.post).toHaveBeenCalledWith('/auth/login', {
        employee_id: 'EMP001',
        password: 'password',
      })
    })
    expect(await screen.findByText('年度家庭日')).toBeInTheDocument()
    expect(localStorage.getItem('token')).toBe('employee-token')
    expect(window.location.pathname).toBe('/events')
  })

  it('既有 token 可還原管理者身份並顯示管理導覽', async () => {
    console.info('確認既有 token 會透過 /auth/me 還原使用者與角色導覽')
    localStorage.setItem('token', 'manager-token')

    const { api } = await renderApp('/events', ({ get }) => {
      get.mockImplementation((url: string) => {
        if (url === '/auth/me') {
          return Promise.resolve({ data: { data: managerUser } })
        }
        if (url === '/events') {
          return Promise.resolve({ data: { data: [eventFixture] } })
        }
        return Promise.reject(new Error(`未處理的 API 路徑：${url}`))
      })
    })

    expect(await screen.findByText('年度家庭日')).toBeInTheDocument()
    expect(screen.getByText('活動管理')).toBeInTheDocument()
    expect(screen.getByText('現場核銷')).toBeInTheDocument()
    expect(api.get).toHaveBeenCalledWith('/auth/me')
  })

  it('既有 token 失效時會清除 token 並導向登入頁', async () => {
    console.info('確認既有 token 失效時 AuthProvider 會清除登入狀態並回到登入頁')
    localStorage.setItem('token', 'expired-token')

    const { api } = await renderApp('/events', ({ get }) => {
      get.mockImplementation((url: string) => {
        if (url === '/auth/me') {
          return Promise.reject({
            response: {
              status: 401,
              data: {
                error: {
                  code: 'UNAUTHORIZED',
                  message: 'Token expired',
                },
              },
            },
          })
        }
        return Promise.reject(new Error(`失效 token 流程不應呼叫其他 API：${url}`))
      })
    })

    await waitFor(() => expect(window.location.pathname).toBe('/login'))
    expect(localStorage.getItem('token')).toBeNull()
    expect(screen.getByRole('button', { name: '登入' })).toBeInTheDocument()
    expect(screen.queryByText('年度家庭日')).not.toBeInTheDocument()
    expect(api.get).toHaveBeenCalledWith('/auth/me')
  })

  it('已登入使用者登出後會清除 token 並回到登入頁', async () => {
    console.info('確認 App、AuthProvider 與 Layout 串起來的登出流程')
    localStorage.setItem('token', 'employee-token')

    await renderApp('/events', ({ get }) => {
      get.mockImplementation((url: string) => {
        if (url === '/auth/me') {
          return Promise.resolve({ data: { data: employeeUser } })
        }
        if (url === '/events') {
          return Promise.resolve({ data: { data: [eventFixture] } })
        }
        return Promise.reject(new Error(`未處理的 API 路徑：${url}`))
      })
    })

    expect(await screen.findByText('年度家庭日')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /登出/ }))

    await waitFor(() => expect(window.location.pathname).toBe('/login'))
    expect(localStorage.getItem('token')).toBeNull()
    expect(screen.getByRole('button', { name: '登入' })).toBeInTheDocument()
    expect(screen.queryByText('年度家庭日')).not.toBeInTheDocument()
  })
})
