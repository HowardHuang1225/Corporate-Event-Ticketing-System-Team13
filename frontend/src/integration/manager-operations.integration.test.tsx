import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from '../App'
import api from '../api/client'

vi.mock('../api/client', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
    patch: vi.fn(),
  },
}))

const managerUser = {
  id: 'manager-integration-1',
  employee_id: 'MGR100',
  name: '活動管理員',
  email: 'manager.integration@example.com',
  department: '行政部',
  region: '台北',
  role: 'event_manager' as const,
}

const draftEventFixture = {
  id: 'draft-event-integration',
  title: '可發布草稿活動',
  description: '等待發布的活動',
  venue: '台北總部',
  status: 'draft',
  publish_time: '2099-06-01T01:00:00.000Z',
  start_time: '2099-07-01T01:00:00.000Z',
  end_time: '2099-07-01T09:00:00.000Z',
  apply_deadline: '2099-06-20T09:00:00.000Z',
  region_restriction: '台北',
  max_tickets_per_person: 2,
  ticket_types: [
    {
      id: 'ticket-type-draft',
      name: '一般票',
      remaining: 20,
      total_quota: 20,
    },
  ],
}

const publishedEventFixture = {
  ...draftEventFixture,
  id: 'published-event-integration',
  title: '可截止已發布活動',
  status: 'published',
}

const checkinTicketFixture = {
  id: 'ticket-checkin-1',
  event: { title: '家庭同樂日' },
  ticket_type: { name: '一般票' },
  qr_token: 'QR-CHECKIN-001',
  user: {
    name: '員工小明',
    employee_id: 'EMP100',
    department: '資訊部',
    region: '台南',
  },
}

function configureManagerApi() {
  const get = vi.mocked(api.get)
  const post = vi.mocked(api.post)
  const patch = vi.mocked(api.patch)

  get.mockImplementation((url: string) => {
    if (url === '/auth/me') {
      return Promise.resolve({ data: { data: managerUser } })
    }
    if (url === '/events') {
      return Promise.resolve({ data: { data: [draftEventFixture, publishedEventFixture] } })
    }
    if (url === '/checkins') {
      return Promise.resolve({ data: { data: [] } })
    }
    return Promise.reject(new Error(`未處理的 API 路徑：${url}`))
  })

  post.mockImplementation((url: string) => {
    if (url === '/events') {
      return Promise.resolve({ data: { data: { id: 'created-event' } } })
    }
    if (url === '/checkin') {
      return Promise.resolve({
        data: {
          data: {
            ticket: checkinTicketFixture,
          },
        },
      })
    }
    return Promise.reject(new Error(`未處理的 API 路徑：${url}`))
  })

  patch.mockImplementation((url: string) => {
    if (url === '/events/draft-event-integration/publish') {
      return Promise.resolve({ data: { data: {} } })
    }
    if (url === '/events/published-event-integration/close') {
      return Promise.resolve({ data: { data: {} } })
    }
    return Promise.reject(new Error(`未處理的 API 路徑：${url}`))
  })

  return { post, patch }
}

function getEventCreateFields(container: HTMLElement) {
  const inputs = Array.from(container.querySelectorAll<HTMLInputElement>('.modal input'))
  const textarea = container.querySelector<HTMLTextAreaElement>('.modal textarea')
  if (inputs.length < 10 || !textarea) {
    throw new Error('活動建立表單欄位沒有正確渲染')
  }

  return {
    title: inputs[0],
    description: textarea,
    venue: inputs[1],
    publishTime: inputs[2],
    startTime: inputs[3],
    applyDeadline: inputs[4],
    endTime: inputs[5],
    region: inputs[7],
  }
}

describe('Manager 操作整合流程', () => {
  beforeEach(() => {
    localStorage.clear()
    window.history.pushState({}, '', '/manage/events')
    vi.mocked(api.get).mockReset()
    vi.mocked(api.post).mockReset()
    vi.mocked(api.put).mockReset()
    vi.mocked(api.delete).mockReset()
    vi.mocked(api.patch).mockReset()
  })

  it('manager 可建立活動草稿，並切到現場核銷完成票券核銷', async () => {
    console.info('確認 manager 活動管理與現場核銷頁面之間的整合流程')
    localStorage.setItem('token', 'manager-integration-token')
    const { post } = configureManagerApi()
    const { container } = render(<App />)

    expect(await screen.findByRole('button', { name: /建立新活動/ })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /建立新活動/ }))

    const fields = getEventCreateFields(container)
    await userEvent.type(fields.title, '跨頁整合活動')
    await userEvent.type(fields.description, '用來確認 manager 整合流程')
    await userEvent.type(fields.venue, '台北總部')
    await userEvent.type(fields.publishTime, '2099-06-01T10:00')
    await userEvent.type(fields.startTime, '2099-07-01T10:00')
    await userEvent.type(fields.applyDeadline, '2099-06-20T17:00')
    await userEvent.type(fields.endTime, '2099-07-01T18:00')
    await userEvent.type(fields.region, '台北')
    await userEvent.click(screen.getByRole('button', { name: '儲存草稿' }))

    await waitFor(() => {
      expect(post).toHaveBeenCalledWith('/events', expect.objectContaining({
        title: '跨頁整合活動',
        venue: '台北總部',
        region_restriction: '台北',
      }))
    })

    await userEvent.click(screen.getByText('現場核銷'))
    expect(await screen.findByText('核銷入場')).toBeInTheDocument()

    const tokenInput = container.querySelector<HTMLInputElement>('#qr-token-input')
    if (!tokenInput) {
      throw new Error('QR Token 欄位沒有正確渲染')
    }
    await userEvent.type(tokenInput, '  QR-CHECKIN-001  ')
    await userEvent.click(screen.getByRole('button', { name: '確認核銷' }))

    await waitFor(() => {
      expect(post).toHaveBeenCalledWith('/checkin', { qr_token: 'QR-CHECKIN-001' })
    })
    expect(await screen.findByText(/核銷成功/)).toBeInTheDocument()
    expect(screen.getByText(/員工小明/)).toBeInTheDocument()
  })

  it('manager 可從活動管理發布草稿並截止已發布活動', async () => {
    console.info('確認 manager 活動發布與截止的 App 層級整合流程')
    localStorage.setItem('token', 'manager-integration-token')
    const { patch } = configureManagerApi()
    render(<App />)

    expect(await screen.findByText('可發布草稿活動')).toBeInTheDocument()
    expect(screen.getByText('可截止已發布活動')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /發布/ }))
    await waitFor(() => {
      expect(patch).toHaveBeenCalledWith('/events/draft-event-integration/publish')
    })

    await userEvent.click(screen.getByRole('button', { name: /截止/ }))
    await waitFor(() => {
      expect(patch).toHaveBeenCalledWith('/events/published-event-integration/close')
    })
  })

  it('manager 核銷遇到已核銷與找不到票券時會顯示對應錯誤', async () => {
    console.info('確認 manager 現場核銷錯誤分支的整合流程')
    window.history.pushState({}, '', '/checkin')
    localStorage.setItem('token', 'manager-integration-token')
    const { post } = configureManagerApi()
    let checkinErrorCode = 'ALREADY_CHECKED_IN'
    let checkinErrorMessage = '此票券已核銷'
    post.mockImplementation((url: string) => {
      if (url === '/checkin') {
        return Promise.reject({
          response: {
            data: {
              error: {
                code: checkinErrorCode,
                message: checkinErrorMessage,
              },
            },
          },
        })
      }
      return Promise.reject(new Error(`未處理的 API 路徑：${url}`))
    })
    const { container } = render(<App />)

    expect(await screen.findByText('核銷入場')).toBeInTheDocument()
    const tokenInput = container.querySelector<HTMLInputElement>('#qr-token-input')
    if (!tokenInput) {
      throw new Error('QR Token 欄位沒有正確渲染')
    }

    await userEvent.type(tokenInput, 'QR-CHECKED-IN')
    await userEvent.click(screen.getByRole('button', { name: '確認核銷' }))

    await waitFor(() => {
      expect(post).toHaveBeenCalledWith('/checkin', { qr_token: 'QR-CHECKED-IN' })
    })
    expect(await screen.findByText(/此票券已核銷/)).toBeInTheDocument()

    checkinErrorCode = 'NOT_FOUND'
    checkinErrorMessage = '找不到此票券'
    await userEvent.clear(tokenInput)
    await userEvent.type(tokenInput, 'QR-NOT-FOUND')
    await userEvent.click(screen.getByRole('button', { name: '確認核銷' }))

    await waitFor(() => {
      expect(post).toHaveBeenCalledWith('/checkin', { qr_token: 'QR-NOT-FOUND' })
    })
    expect(await screen.findByText(/找不到此票券/)).toBeInTheDocument()
  })
})
