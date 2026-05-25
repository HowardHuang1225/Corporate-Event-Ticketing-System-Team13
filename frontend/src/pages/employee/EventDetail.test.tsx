import { describe, expect, it, beforeEach, vi, type Mock } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import EventDetail from './EventDetail'
import api from '../../api/client'
import { useAuth } from '../../contexts/AuthContext'
import { renderWithQueryClient } from '../../test/test-utils'
import toast from 'react-hot-toast'

vi.mock('../../api/client', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(),
  },
}))

vi.mock('../../contexts/AuthContext', () => ({
  useAuth: vi.fn(),
}))

vi.mock('react-hot-toast', () => ({
  default: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

vi.mock('uuid', () => ({
  v4: vi.fn(() => 'fixed-idempotency-key'),
}))

const apiGet = api.get as Mock
const apiPost = api.post as Mock
const mockUseAuth = useAuth as Mock
const toastSuccess = toast.success as Mock
const toastError = toast.error as Mock

const employeeUser = {
  id: 'employee-1',
  employee_id: 'EMP001',
  name: '王小明',
  email: 'ming@example.com',
  department: '資訊部',
  region: '新竹',
  role: 'employee' as const,
}

const publishedEvent = {
  id: 'event-1',
  title: '年度家庭日',
  description: '員工家庭日活動',
  venue: '台南園區',
  status: 'published',
  start_time: '2099-07-01T01:00:00.000Z',
  end_time: '2099-07-01T09:00:00.000Z',
  apply_deadline: '2099-06-20T09:00:00.000Z',
  region_restriction: '台南',
  max_tickets_per_person: 3,
  ticket_types: [
    {
      id: 'ticket-type-1',
      name: '一般票',
      remaining: 5,
      total_quota: 20,
    },
  ],
}

function renderEventDetail(eventData = publishedEvent) {
  mockUseAuth.mockReturnValue({
    user: employeeUser,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
  })
  apiGet.mockResolvedValue({ data: { data: eventData } })
  apiPost.mockResolvedValue({ status: 200, data: { data: { status: 'approved' } } })

  return renderWithQueryClient(
    <MemoryRouter initialEntries={['/events/event-1']}>
      <Routes>
        <Route path="/events/:id" element={<EventDetail />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('EventDetail', () => {
  beforeEach(() => {
    apiGet.mockReset()
    apiPost.mockReset()
    mockUseAuth.mockReset()
    toastSuccess.mockReset()
    toastError.mockReset()
  })

  it('可申請活動會顯示地域提醒，並送出正確的申請 payload', async () => {
    console.info('確認活動詳情頁的申請票券流程')
    const { container, queryClient } = renderEventDetail()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    expect(await screen.findByText('年度家庭日')).toBeInTheDocument()
    expect(screen.getByText(/地域提醒/)).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '申請' }))
    expect(screen.getByText('申請票券')).toBeInTheDocument()

    const quantityInput = container.querySelector<HTMLInputElement>('input[type="number"]')
    if (!quantityInput) throw new Error('數量欄位沒有正確渲染')
    await userEvent.clear(quantityInput)
    await userEvent.type(quantityInput, '2')
    await userEvent.click(screen.getByRole('button', { name: '確認申請' }))

    await waitFor(() => {
      expect(apiPost).toHaveBeenCalledWith('/applications', {
        event_id: 'event-1',
        ticket_type_id: 'ticket-type-1',
        quantity: 2,
        idempotency_key: 'fixed-idempotency-key',
      })
    })
    expect(toastSuccess).toHaveBeenCalledWith('搶票成功！已為您自動發票')
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['event', 'event-1'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['my-applications'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['my-tickets'] })
  })

  it('未發布活動會提示目前不開放申請', async () => {
    console.info('確認未發布活動不提供申請按鈕')
    renderEventDetail({
      ...publishedEvent,
      status: 'draft',
    })

    expect(await screen.findByText('年度家庭日')).toBeInTheDocument()
    expect(screen.getByText('此活動目前不開放申請')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '申請' })).not.toBeInTheDocument()
  })

  it('申請截止時間已過時會提示截止且不提供申請按鈕', async () => {
    console.info('確認申請截止後不可申請')
    renderEventDetail({
      ...publishedEvent,
      apply_deadline: '2000-06-20T09:00:00.000Z',
    })

    expect(await screen.findByText('年度家庭日')).toBeInTheDocument()
    expect(screen.getByText('申請截止時間已過')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '申請' })).not.toBeInTheDocument()
  })

  it('票種售罄時會顯示已售罄且不提供申請按鈕', async () => {
    console.info('確認售罄票種不允許申請')
    renderEventDetail({
      ...publishedEvent,
      ticket_types: [
        {
          ...publishedEvent.ticket_types[0],
          remaining: 0,
        },
      ],
    })

    expect(await screen.findByText('年度家庭日')).toBeInTheDocument()
    expect(screen.getByText('已售罄')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '申請' })).not.toBeInTheDocument()
  })

  it('後端回傳排隊狀態時會顯示排隊成功訊息', async () => {
    console.info('確認申請進入排隊分支')
    renderEventDetail()
    apiPost.mockResolvedValue({ status: 202, data: { data: { status: 'queued' } } })

    await userEvent.click(await screen.findByRole('button', { name: '申請' }))
    await userEvent.click(screen.getByRole('button', { name: '確認申請' }))

    await waitFor(() => {
      expect(toastSuccess).toHaveBeenCalledWith('已進入排隊，系統會依序處理申請')
    })
  })

  it('申請失敗時會顯示後端錯誤訊息', async () => {
    console.info('確認申請失敗錯誤分支')
    renderEventDetail()
    apiPost.mockRejectedValue({
      response: {
        data: {
          error: {
            message: '超過每人票數上限',
          },
        },
      },
    })

    await userEvent.click(await screen.findByRole('button', { name: '申請' }))
    await userEvent.click(screen.getByRole('button', { name: '確認申請' }))

    await waitFor(() => {
      expect(toastError).toHaveBeenCalledWith('超過每人票數上限')
    })
  })

  it('API 沒有回傳活動資料時會顯示找不到活動', async () => {
    console.info('確認活動不存在的空資料分支')
    apiGet.mockResolvedValue({ data: { data: null } })
    mockUseAuth.mockReturnValue({
      user: employeeUser,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
    })

    renderWithQueryClient(
      <MemoryRouter initialEntries={['/events/event-404']}>
        <Routes>
          <Route path="/events/:id" element={<EventDetail />} />
        </Routes>
      </MemoryRouter>,
    )

    expect(await screen.findByText('找不到活動')).toBeInTheDocument()
  })

  it('同地域活動不會顯示地域提醒', async () => {
    console.info('確認同地域時不顯示提醒')
    mockUseAuth.mockReturnValue({
      user: { ...employeeUser, region: 'Tainan Plant' },
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
    })
    apiGet.mockResolvedValue({
      data: {
        data: {
          ...publishedEvent,
          region_restriction: '台南',
        },
      },
    })

    renderWithQueryClient(
      <MemoryRouter initialEntries={['/events/event-1']}>
        <Routes>
          <Route path="/events/:id" element={<EventDetail />} />
        </Routes>
      </MemoryRouter>,
    )

    expect(await screen.findByText('年度家庭日')).toBeInTheDocument()
    expect(screen.queryByText(/地域提醒/)).not.toBeInTheDocument()
  })
})
