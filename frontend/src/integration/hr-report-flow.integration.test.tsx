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

const hrUser = {
  id: 'hr-integration-1',
  employee_id: 'HR100',
  name: '人資小華',
  email: 'hr.integration@example.com',
  department: '人資部',
  region: '台北',
  role: 'hr' as const,
}

const eventFixture = {
  id: 'event-report-1',
  title: '年度家庭日',
  description: '員工家庭日活動',
  venue: '台南園區',
  status: 'published',
  publish_time: '2099-06-01T01:00:00.000Z',
  start_time: '2099-07-01T01:00:00.000Z',
  end_time: '2099-07-01T09:00:00.000Z',
  apply_deadline: '2099-06-20T09:00:00.000Z',
  region_restriction: '台南',
  ticket_types: [
    {
      id: 'ticket-type-report',
      name: '一般票',
      remaining: 4,
      total_quota: 20,
    },
  ],
}

const overviewFixture = [
  {
    event_id: 'event-report-1',
    title: '年度家庭日',
    applied_apps: 6,
    applied_tickets: 10,
    applied_users: 5,
    approved_tickets: 8,
    cancelled: 1,
    total_tickets: 7,
    checked_in: 4,
    check_in_rate: 57,
  },
]

const statsFixture = {
  event: { title: '年度家庭日' },
  applied_apps: 6,
  applied_tickets: 10,
  applied_users: 5,
  approved_tickets: 8,
  cancelled_tickets: 1,
  total_tickets: 7,
  approved_users: 5,
  checked_in_tickets: 4,
  check_in_rate: 57,
  by_department: [
    { department: '資訊部', count: 3 },
    { department: '行政部', count: 2 },
  ],
  by_region: [
    { region: '台南', count: 4 },
    { region: '新竹', count: 1 },
  ],
  by_ticket_type: [
    {
      ticket_type_name: '一般票',
      total: 10,
      approved: 8,
      cancelled: 1,
      active: 7,
    },
  ],
}

function configureHrApi() {
  const get = vi.mocked(api.get)

  get.mockImplementation((url: string) => {
    if (url === '/auth/me') {
      return Promise.resolve({ data: { data: hrUser } })
    }
    if (url === '/events') {
      return Promise.resolve({ data: { data: [eventFixture] } })
    }
    if (url === '/reports/overview') {
      return Promise.resolve({ data: { data: overviewFixture } })
    }
    if (url === '/reports/events/event-report-1/stats') {
      return Promise.resolve({ data: { data: statsFixture } })
    }
    return Promise.reject(new Error(`未處理的 API 路徑：${url}`))
  })

  return { get }
}

describe('HR 報表整合流程', () => {
  beforeEach(() => {
    resetAppQueryClient()
    localStorage.clear()
    window.history.pushState({}, '', '/reports')
    vi.mocked(api.get).mockReset()
    vi.mocked(api.post).mockReset()
  })

  it('HR 可查看報表總覽、選擇活動詳情，並透過導覽回活動列表', async () => {
    console.info('確認 HR 報表查詢與跨頁導覽的 App 層級整合流程')
    localStorage.setItem('token', 'hr-integration-token')
    const { get } = configureHrApi()
    render(<App />)

    expect(await screen.findByRole('heading', { name: '統計報表' })).toBeInTheDocument()
    expect(await screen.findByText('活動總覽')).toBeInTheDocument()
    expect(screen.getAllByText('年度家庭日').length).toBeGreaterThan(0)

    await userEvent.selectOptions(screen.getByRole('combobox'), 'event-report-1')

    await waitFor(() => {
      expect(get).toHaveBeenCalledWith('/reports/events/event-report-1/stats')
    })
    expect(await screen.findByText('1. 申請階段')).toBeInTheDocument()
    expect(screen.getByText('2. 核准與退票階段')).toBeInTheDocument()
    expect(screen.getByText('3. 核銷與出席率')).toBeInTheDocument()
    expect(screen.getByText('資訊部')).toBeInTheDocument()
    expect(screen.getByText('一般票')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('link', { name: /活動列表/ }))

    expect(await screen.findByText('瀏覽所有可參加的福委會活動')).toBeInTheDocument()
    expect(screen.getAllByText('年度家庭日').length).toBeGreaterThan(0)
  })
})
