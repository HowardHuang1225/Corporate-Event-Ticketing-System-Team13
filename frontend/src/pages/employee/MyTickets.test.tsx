import { describe, expect, it, beforeEach, vi, type Mock } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import MyTickets from './MyTickets'
import api from '../../api/client'
import { useAuth } from '../../contexts/AuthContext'
import { renderWithQueryClient } from '../../test/test-utils'

vi.mock('../../api/client', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(),
  },
}))

vi.mock('../../contexts/AuthContext', () => ({
  useAuth: vi.fn(),
}))

const apiGet = api.get as Mock
const apiPost = api.post as Mock
const mockUseAuth = useAuth as Mock

const employeeUser = {
  id: 'employee-1',
  employee_id: 'EMP001',
  name: '王小明',
  email: 'ming@example.com',
  department: '資訊部',
  region: '台南',
  role: 'employee' as const,
}

const applicationFixture = {
  id: 'app-1',
  event: { title: '年度家庭日' },
  ticket_type: { name: '一般票' },
  quantity: 2,
  status: 'approved',
  applied_at: '2099-06-01T01:00:00.000Z',
  reason: null,
}

const ticketFixture = {
  id: 'ticket-1',
  event: { title: '年度家庭日' },
  ticket_type: { name: '一般票' },
  expires_at: '2099-07-01T01:00:00.000Z',
  qr_token: 'QR-TOKEN-123456',
  is_used: false,
}

function renderMyTickets({
  applications = [applicationFixture],
  tickets = [ticketFixture],
} = {}) {
  mockUseAuth.mockReturnValue({
    user: employeeUser,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
  })
  apiGet.mockImplementation((url: string) => {
    if (url === '/applications/my') return Promise.resolve({ data: { data: applications } })
    if (url === '/tickets/my') return Promise.resolve({ data: { data: tickets } })
    return Promise.reject(new Error(`未處理的 API 路徑：${url}`))
  })
  apiPost.mockResolvedValue({ data: { data: {} } })

  return renderWithQueryClient(<MyTickets />)
}

describe('MyTickets', () => {
  beforeEach(() => {
    apiGet.mockReset()
    apiPost.mockReset()
    mockUseAuth.mockReset()
    vi.unstubAllGlobals()
  })

  it('會載入申請記錄與電子票券，並可展開 QR token', async () => {
    console.info('確認我的票券頁會顯示申請記錄與 QR')

    renderMyTickets()

    expect(await screen.findAllByText('年度家庭日')).toHaveLength(2)
    expect(screen.getByText('已核准')).toBeInTheDocument()
    expect(screen.queryByText('QR-TOKEN-123456')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '顯示 QR' }))

    await waitFor(() => {
      expect(screen.getByText(/QR-TOKEN-123456/)).toBeInTheDocument()
    })
    expect(screen.getByRole('button', { name: '收起 QR' })).toBeInTheDocument()
  })

  it('使用者取消確認退票時不會呼叫退票 API', async () => {
    console.info('確認退票取消時不送出 API')
    vi.stubGlobal('confirm', vi.fn(() => false))

    renderMyTickets()
    await screen.findAllByText('年度家庭日')
    await userEvent.click(screen.getByRole('button', { name: '退票' }))

    expect(apiPost).not.toHaveBeenCalled()
  })

  it('使用者確認退票時會呼叫 cancel API 並刷新相關 query', async () => {
    console.info('確認退票成功會刷新票券與申請資料')
    vi.stubGlobal('confirm', vi.fn(() => true))
    const { queryClient } = renderMyTickets()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    await screen.findAllByText('年度家庭日')
    await userEvent.click(screen.getByRole('button', { name: '退票' }))

    await waitFor(() => expect(apiPost).toHaveBeenCalledWith('/tickets/ticket-1/cancel'))
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['my-tickets'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['my-applications'] })
  })

  it('退票 API 失敗時會顯示錯誤訊息', async () => {
    console.info('確認退票失敗錯誤分支')
    vi.stubGlobal('confirm', vi.fn(() => true))
    const alertSpy = vi.fn()
    vi.stubGlobal('alert', alertSpy)
    renderMyTickets()
    apiPost.mockRejectedValue({
      response: {
        data: {
          error: '票券已核銷不可退票',
        },
      },
    })

    await screen.findAllByText('年度家庭日')
    await userEvent.click(screen.getByRole('button', { name: '退票' }))

    await waitFor(() => expect(alertSpy).toHaveBeenCalledWith('票券已核銷不可退票'))
  })

  it('已核銷票券不會顯示 QR 與退票操作', async () => {
    console.info('確認已核銷票券沒有操作按鈕')

    renderMyTickets({
      tickets: [
        {
          ...ticketFixture,
          is_used: true,
        },
      ],
    })

    expect(await screen.findByText('已核銷')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '顯示 QR' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '退票' })).not.toBeInTheDocument()
  })

  it('沒有申請記錄時會顯示空狀態', async () => {
    console.info('確認我的票券申請記錄空狀態')

    renderMyTickets({
      applications: [],
      tickets: [],
    })

    expect(await screen.findByText('還沒有申請記錄，快去瀏覽活動吧！')).toBeInTheDocument()
    expect(screen.queryByText('電子票券')).not.toBeInTheDocument()
  })
})
