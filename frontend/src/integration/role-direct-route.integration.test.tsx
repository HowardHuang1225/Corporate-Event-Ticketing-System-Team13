import { render, screen, waitFor } from '@testing-library/react'
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
  id: 'employee-direct-route',
  employee_id: 'EMP-DIRECT',
  name: '直連員工',
  email: 'employee.direct@example.com',
  department: '資訊部',
  region: '台南',
  role: 'employee' as const,
}

const hrUser = {
  id: 'hr-direct-route',
  employee_id: 'HR-DIRECT',
  name: '直連人資',
  email: 'hr.direct@example.com',
  department: '人資部',
  region: '台北',
  role: 'hr' as const,
}

const eventFixture = {
  id: 'event-direct-route',
  title: '直連防護測試活動',
  description: '用來確認錯角色直接輸入 URL 時的前端防護',
  venue: '台南園區',
  status: 'published',
  publish_time: '2099-06-01T01:00:00.000Z',
  start_time: '2099-07-01T01:00:00.000Z',
  end_time: '2099-07-01T09:00:00.000Z',
  apply_deadline: '2099-06-20T09:00:00.000Z',
  region_restriction: '台南',
  ticket_types: [],
}

describe('角色直接輸入 URL 整合流程', () => {
  beforeEach(() => {
    resetAppQueryClient()
    localStorage.clear()
    vi.mocked(api.get).mockReset()
    vi.mocked(api.post).mockReset()
  })

  it('employee 直接進入 HR 報表頁時應回到活動列表且不查詢報表 API', async () => {
    console.info('確認 employee 不能靠直接 URL 進入 HR 報表頁')
    localStorage.setItem('token', 'employee-direct-route-token')
    window.history.pushState({}, '', '/reports')

    const get = vi.mocked(api.get)
    get.mockImplementation((url: string) => {
      if (url === '/auth/me') {
        return Promise.resolve({ data: { data: employeeUser } })
      }
      if (url === '/events') {
        return Promise.resolve({ data: { data: [eventFixture] } })
      }
      if (url === '/reports/overview') {
        return Promise.reject({
          response: {
            status: 403,
            data: { error: { code: 'FORBIDDEN', message: 'Forbidden' } },
          },
        })
      }
      return Promise.reject(new Error(`未處理的 API 路徑：${url}`))
    })

    render(<App />)

    expect(await screen.findByRole('heading', { name: '活動列表' })).toBeInTheDocument()
    await waitFor(() => expect(window.location.pathname).toBe('/events'))
    expect(get).not.toHaveBeenCalledWith('/reports/overview')
  })

  it('HR 直接進入現場核銷頁時應回到活動列表且不查詢核銷 API', async () => {
    console.info('確認 HR 不能靠直接 URL 進入 manager 現場核銷頁')
    localStorage.setItem('token', 'hr-direct-route-token')
    window.history.pushState({}, '', '/checkin')

    const get = vi.mocked(api.get)
    get.mockImplementation((url: string, config?: { params?: Record<string, string> }) => {
      if (url === '/auth/me') {
        return Promise.resolve({ data: { data: hrUser } })
      }
      if (url === '/events') {
        return Promise.resolve({ data: { data: [eventFixture] } })
      }
      if (url === '/checkins') {
        return Promise.reject({
          response: {
            status: 403,
            data: { error: { code: 'FORBIDDEN', message: 'Forbidden' } },
          },
        })
      }
      return Promise.reject(new Error(`未處理的 API 路徑：${url} ${JSON.stringify(config ?? {})}`))
    })

    render(<App />)

    expect(await screen.findByRole('heading', { name: '活動列表' })).toBeInTheDocument()
    await waitFor(() => expect(window.location.pathname).toBe('/events'))
    expect(get).not.toHaveBeenCalledWith('/checkins', expect.anything())
  })
})
