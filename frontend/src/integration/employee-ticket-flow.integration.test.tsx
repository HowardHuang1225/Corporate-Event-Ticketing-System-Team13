import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from '../App'
import api from '../api/client'
import { resetAppQueryClient } from '../test/test-utils'
import { generateTOTP } from '../utils/totp'

vi.mock('../api/client', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(),
  },
}))

vi.mock('uuid', () => ({
  v4: vi.fn(() => 'fixed-integration-idempotency-key'),
}))

vi.mock('../utils/totp', () => ({
  generateTOTP: vi.fn(),
}))

const generateTOTPMock = vi.mocked(generateTOTP)

const employeeUser = {
  id: 'employee-integration-1',
  employee_id: 'EMP100',
  name: '員工小明',
  email: 'employee.integration@example.com',
  department: '資訊部',
  region: '台南',
  role: 'employee' as const,
}

const eventFixture = {
  id: 'event-auto-approved',
  title: '家庭同樂日',
  description: '員工與眷屬一起參與的活動',
  venue: '台南園區',
  status: 'published',
  publish_time: '2099-06-01T01:00:00.000Z',
  start_time: '2099-07-01T01:00:00.000Z',
  end_time: '2099-07-01T09:00:00.000Z',
  apply_deadline: '2099-06-20T09:00:00.000Z',
  region_restriction: '台南',
  max_tickets_per_person: 3,
  ticket_types: [
    {
      id: 'ticket-type-general',
      name: '一般票',
      remaining: 8,
      total_quota: 30,
    },
  ],
}

const approvedApplicationFixture = {
  id: 'application-auto-approved',
  event: { title: '家庭同樂日' },
  ticket_type: { name: '一般票' },
  quantity: 2,
  status: 'approved',
  applied_at: '2099-06-10T01:00:00.000Z',
  reason: null,
}

const ticketFixture = {
  id: 'ticket-auto-approved',
  event: { title: '家庭同樂日' },
  ticket_type: { name: '一般票' },
  qr_token: 'QR-AUTO-APPROVED-001',
  expires_at: '2099-07-01T09:00:00.000Z',
  is_used: false,
}

function configureEmployeeApi() {
  const get = vi.mocked(api.get)
  const post = vi.mocked(api.post)

  get.mockImplementation((url: string) => {
    if (url === '/auth/me') {
      return Promise.resolve({ data: { data: employeeUser } })
    }
    if (url === '/events') {
      return Promise.resolve({ data: { data: [eventFixture] } })
    }
    if (url === '/events/event-auto-approved') {
      return Promise.resolve({ data: { data: eventFixture } })
    }
    if (url === '/applications/my') {
      return Promise.resolve({ data: { data: [approvedApplicationFixture] } })
    }
    if (url === '/tickets/my') {
      return Promise.resolve({ data: { data: [ticketFixture] } })
    }
    return Promise.reject(new Error(`未處理的 API 路徑：${url}`))
  })

  post.mockImplementation((url: string) => {
    if (url === '/applications') {
      return Promise.resolve({
        status: 200,
        data: {
          data: {
            status: 'approved',
            tickets: [ticketFixture],
          },
        },
      })
    }
    if (url === '/tickets/ticket-auto-approved/cancel') {
      return Promise.resolve({ data: { data: {} } })
    }
    return Promise.reject(new Error(`未處理的 API 路徑：${url}`))
  })

  return { get, post }
}

describe('員工票券整合流程', () => {
  beforeEach(() => {
    resetAppQueryClient()
    localStorage.clear()
    window.history.pushState({}, '', '/events')
    vi.mocked(api.get).mockReset()
    vi.mocked(api.post).mockReset()
    generateTOTPMock.mockReset()
    generateTOTPMock.mockResolvedValue('123456')
  })

  it('員工可從活動列表進入詳情，送出申請後在我的票券看到自動核准票券', async () => {
    console.info('確認員工活動申請到自動核准票券的 App 層級整合流程')
    localStorage.setItem('token', 'employee-integration-token')
    const { post } = configureEmployeeApi()
    const { container } = render(<App />)

    expect(await screen.findByText('家庭同樂日')).toBeInTheDocument()
    await userEvent.click(screen.getByText('查看詳情 →'))

    expect(await screen.findByText('可選票種')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '申請' }))

    expect(screen.getByText('申請票券')).toBeInTheDocument()
    const quantityInput = container.querySelector<HTMLInputElement>('input[type="number"]')
    if (!quantityInput) {
      throw new Error('數量欄位沒有正確渲染')
    }
    await userEvent.clear(quantityInput)
    await userEvent.type(quantityInput, '2')
    await userEvent.click(screen.getByRole('button', { name: '確認申請' }))

    await waitFor(() => {
      expect(post).toHaveBeenCalledWith('/applications', {
        event_id: 'event-auto-approved',
        ticket_type_id: 'ticket-type-general',
        quantity: 2,
        idempotency_key: 'fixed-integration-idempotency-key',
      })
    })

    await userEvent.click(screen.getByText('我的票券'))

    expect(await screen.findByRole('heading', { name: /電子票券/ })).toBeInTheDocument()
    expect(screen.getByText('已核准')).toBeInTheDocument()
    expect(screen.getByText('未使用')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '顯示 QR' }))
    expect(await screen.findByText('QR-AUTO-APPROVED-001|123456')).toBeInTheDocument()
    expect(generateTOTPMock).toHaveBeenCalledWith('QR-AUTO-APPROVED-001', 60)
  })

  it('員工可在我的票券確認退票，並刷新票券與申請資料', async () => {
    console.info('確認員工退票會串接 App、MyTickets 與 React Query 重新查詢')
    window.history.pushState({}, '', '/my-tickets')
    localStorage.setItem('token', 'employee-integration-token')
    const confirmSpy = vi.fn(() => true)
    vi.stubGlobal('confirm', confirmSpy)
    const { get, post } = configureEmployeeApi()
    render(<App />)

    expect(await screen.findByRole('heading', { name: '我的票券' })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /電子票券/ })).toBeInTheDocument()

    const ticketQueryCountBeforeCancel = get.mock.calls.filter(([url]) => url === '/tickets/my').length
    await userEvent.click(screen.getByRole('button', { name: '退票' }))

    expect(confirmSpy).toHaveBeenCalled()
    await waitFor(() => {
      expect(post).toHaveBeenCalledWith('/tickets/ticket-auto-approved/cancel')
    })
    await waitFor(() => {
      const ticketQueryCountAfterCancel = get.mock.calls.filter(([url]) => url === '/tickets/my').length
      expect(ticketQueryCountAfterCancel).toBeGreaterThan(ticketQueryCountBeforeCancel)
    })
  })
})
