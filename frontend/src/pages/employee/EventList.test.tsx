import { describe, expect, it, beforeEach, vi, type Mock } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import EventList from './EventList'
import api from '../../api/client'
import { useAuth } from '../../contexts/AuthContext'
import { renderWithQueryClient } from '../../test/test-utils'

vi.mock('../../api/client', () => ({
  default: {
    get: vi.fn(),
  },
}))

vi.mock('../../contexts/AuthContext', () => ({
  useAuth: vi.fn(),
}))

const apiGet = api.get as Mock
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

const managerUser = {
  ...employeeUser,
  id: 'manager-1',
  employee_id: 'MGR001',
  role: 'event_manager' as const,
}

const eventFixture = {
  id: 'event-1',
  title: '年度家庭日',
  venue: '台南園區',
  status: 'published',
  start_time: '2099-07-01T01:00:00.000Z',
  apply_deadline: '2099-06-20T09:00:00.000Z',
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

function renderEventList({
  user = employeeUser,
  events = [eventFixture],
}: { user?: any; events?: any[] } = {}) {
  mockUseAuth.mockReturnValue({
    user,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
  })
  apiGet.mockResolvedValue({ data: { data: events } })

  return renderWithQueryClient(
    <MemoryRouter initialEntries={['/events']}>
      <Routes>
        <Route path="/events" element={<EventList />} />
        <Route path="/events/:id" element={<div>活動詳情頁</div>} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('EventList', () => {
  beforeEach(() => {
    apiGet.mockReset()
    mockUseAuth.mockReset()
  })

  it('會載入活動列表並點擊卡片進入詳情頁', async () => {
    console.info('確認活動列表渲染與卡片導頁')
    renderEventList()

    await screen.findByText('年度家庭日')
    expect(screen.getAllByText('發布中').length).toBeGreaterThan(0)
    expect(screen.getByText(/共 1 個活動/)).toBeInTheDocument()

    await userEvent.click(screen.getByText('年度家庭日'))

    expect(screen.getByText('活動詳情頁')).toBeInTheDocument()
  })

  it('event_manager 可以篩選草稿狀態並帶入查詢參數', async () => {
    console.info('確認 manager 活動狀態篩選分支')
    renderEventList({ user: managerUser })

    const select = await screen.findByRole('combobox')
    expect(screen.getByRole('option', { name: '草稿' })).toBeInTheDocument()

    await userEvent.selectOptions(select, 'draft')

    await waitFor(() => {
      expect(apiGet).toHaveBeenCalledWith('/events', { params: { status: 'draft' } })
    })
  })

  it('employee 不會看到草稿篩選選項', async () => {
    console.info('確認 employee 不顯示草稿篩選')
    renderEventList()

    await screen.findByText('年度家庭日')

    expect(screen.queryByRole('option', { name: '草稿' })).not.toBeInTheDocument()
  })

  it('沒有活動資料時會顯示空狀態', async () => {
    console.info('確認活動列表空資料畫面')
    renderEventList({ events: [] })

    expect(await screen.findByText('目前沒有活動')).toBeInTheDocument()
    expect(screen.getByText(/共 0 個活動/)).toBeInTheDocument()
  })

  it('未知活動狀態會直接顯示原始狀態文字', async () => {
    console.info('確認活動狀態 badge fallback')
    renderEventList({
      events: [
        {
          ...eventFixture,
          status: 'archived',
        },
      ],
    })

    expect(await screen.findByText('archived')).toBeInTheDocument()
  })
})
