import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import axios from 'axios'
import toast from 'react-hot-toast'
import Applications from './Applications'
import api from '../../api/client'
import { renderWithQueryClient } from '../../test/test-utils'

vi.mock('../../api/client', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(),
  },
}))

vi.mock('react-hot-toast', () => ({
  default: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

const apiGet = api.get as Mock
const apiPost = api.post as Mock
const toastSuccess = toast.success as Mock
const toastError = toast.error as Mock

const eventFixture = {
  id: 'event-1',
  title: '年度家庭日',
}

const pendingApplication = {
  id: 'app-pending',
  status: 'pending',
  quantity: 2,
  applied_at: '2099-06-01T02:00:00.000Z',
  user: {
    name: '王小明',
    employee_id: 'EMP001',
    department: '資訊部',
    region: '台南',
  },
  event: {
    title: '年度家庭日',
  },
  ticket_type: {
    name: '一般票',
  },
}

const approvedApplication = {
  id: 'app-approved',
  status: 'approved',
  quantity: 1,
  applied_at: '2099-06-02T02:00:00.000Z',
  reason: '已完成審核',
  user: {
    name: '陳小華',
    employee_id: 'EMP002',
    department: '行政部',
    region: '台北',
  },
  event: {
    title: '年度尾牙',
  },
  ticket_type: {
    name: 'VIP票',
  },
}

function renderApplications({
  events = [eventFixture],
  applications = [pendingApplication, approvedApplication],
}: {
  events?: any[]
  applications?: any[]
} = {}) {
  apiGet.mockImplementation((url: string, config?: { params?: Record<string, string> }) => {
    if (url === '/events') {
      return Promise.resolve({ data: { data: events } })
    }
    if (url === '/applications') {
      return Promise.resolve({
        data: {
          data: applications.filter(app => {
            if (config?.params?.event_id && app.event?.title !== eventFixture.title) return false
            if (config?.params?.status && app.status !== config.params.status) return false
            return true
          }),
        },
      })
    }
    return Promise.reject(new Error(`未處理的 API 路徑：${url}`))
  })
  apiPost.mockResolvedValue({ data: { data: {} } })

  return renderWithQueryClient(<Applications />)
}

describe('Applications', () => {
  beforeEach(() => {
    apiGet.mockReset()
    apiPost.mockReset()
    toastSuccess.mockReset()
    toastError.mockReset()
  })

  it('會載入申請列表、狀態 badge、原因與筆數', async () => {
    console.info('確認申請審核列表渲染')
    renderApplications()

    expect(await screen.findByRole('heading', { name: '申請審核' })).toBeInTheDocument()
    expect(await screen.findByText('王小明')).toBeInTheDocument()
    expect(screen.getByText('EMP001')).toBeInTheDocument()
    expect(screen.getByText('王小明').closest('tr')).toHaveTextContent('資訊部')
    expect(screen.getByText('王小明').closest('tr')).toHaveTextContent('台南')
    expect(screen.getByText('王小明').closest('tr')).toHaveTextContent('年度家庭日')
    expect(screen.getByText('王小明').closest('tr')).toHaveTextContent('一般票')
    expect(screen.getByText('王小明').closest('tr')).toHaveTextContent('待審核')
    expect(screen.getByText('陳小華').closest('tr')).toHaveTextContent('已核准')
    expect(screen.getByText('已完成審核')).toBeInTheDocument()
    expect(screen.getByText('共 2 筆')).toBeInTheDocument()
  })

  it('切換活動與狀態篩選會帶入查詢參數', async () => {
    console.info('確認申請列表篩選參數')
    renderApplications()

    await screen.findByText('王小明')
    const selects = screen.getAllByRole('combobox')

    await userEvent.selectOptions(selects[0], 'event-1')
    await waitFor(() => {
      expect(apiGet).toHaveBeenCalledWith('/applications', { params: { event_id: 'event-1' } })
    })

    await userEvent.selectOptions(selects[1], 'pending')
    await waitFor(() => {
      expect(apiGet).toHaveBeenCalledWith('/applications', {
        params: { event_id: 'event-1', status: 'pending' },
      })
    })
    expect(await screen.findByText('共 1 筆')).toBeInTheDocument()
  })

  it('按重新整理會重新查詢 applications', async () => {
    console.info('確認重新整理按鈕會 refetch')
    renderApplications()

    await screen.findByText('王小明')
    const before = apiGet.mock.calls.filter(call => call[0] === '/applications').length
    await userEvent.click(screen.getByRole('button', { name: /重新整理/ }))

    await waitFor(() => {
      const after = apiGet.mock.calls.filter(call => call[0] === '/applications').length
      expect(after).toBeGreaterThan(before)
    })
  })

  it('核准申請成功會呼叫 API、顯示 toast 並 invalidate applications query', async () => {
    console.info('確認核准申請成功流程')
    const { queryClient } = renderApplications()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    await userEvent.click(await screen.findByRole('button', { name: /核准/ }))

    await waitFor(() => {
      expect(apiPost).toHaveBeenCalledWith('/applications/app-pending/approve')
    })
    expect(toastSuccess).toHaveBeenCalledWith('已核准申請，票券已生成')
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['applications'] })
  })

  it('拒絕申請成功會送出原因、關閉 modal 並清空表單', async () => {
    console.info('確認拒絕申請成功流程')
    const { queryClient } = renderApplications()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    await userEvent.click(await screen.findByRole('button', { name: /拒絕/ }))
    expect(await screen.findByText('拒絕 王小明 的申請')).toBeInTheDocument()

    await userEvent.type(screen.getByLabelText('拒絕原因（選填）'), '票券已全數分配完畢')
    await userEvent.click(screen.getByRole('button', { name: '確認拒絕' }))

    await waitFor(() => {
      expect(apiPost).toHaveBeenCalledWith('/applications/app-pending/reject', {
        reason: '票券已全數分配完畢',
      })
    })
    expect(toastSuccess).toHaveBeenCalledWith('已拒絕申請')
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['applications'] })
    await waitFor(() => {
      expect(screen.queryByText('拒絕 王小明 的申請')).not.toBeInTheDocument()
    })
  })

  it('拒絕 modal 可以用取消、關閉按鈕與 overlay 關閉', async () => {
    console.info('確認拒絕申請 modal 關閉分支')
    renderApplications()

    await userEvent.click(await screen.findByRole('button', { name: /拒絕/ }))
    await userEvent.click(screen.getByRole('button', { name: '取消' }))
    expect(screen.queryByText('拒絕 王小明 的申請')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /拒絕/ }))
    await userEvent.click(screen.getByRole('button', { name: '✕' }))
    expect(screen.queryByText('拒絕 王小明 的申請')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /拒絕/ }))
    await userEvent.click(screen.getByRole('button', { name: '關閉拒絕申請視窗' }))
    expect(screen.queryByText('拒絕 王小明 的申請')).not.toBeInTheDocument()
  })

  it('mutation 失敗時會顯示 axios error message 或 fallback', async () => {
    console.info('確認核准與拒絕錯誤訊息分支')
    vi.spyOn(axios, 'isAxiosError').mockReturnValue(true)
    apiPost.mockRejectedValueOnce({
      response: { data: { error: { message: '審核服務暫時不可用' } } },
    })
    renderApplications()

    await userEvent.click(await screen.findByRole('button', { name: /核准/ }))
    await waitFor(() => {
      expect(toastError).toHaveBeenCalledWith('審核服務暫時不可用')
    })

    apiPost.mockRejectedValueOnce(new Error('plain error'))
    vi.mocked(axios.isAxiosError).mockReturnValue(false)
    await userEvent.click(screen.getByRole('button', { name: /拒絕/ }))
    await userEvent.click(screen.getByRole('button', { name: '確認拒絕' }))

    await waitFor(() => {
      expect(toastError).toHaveBeenCalledWith('操作失敗')
    })
  })

  it('沒有申請資料時會顯示空狀態', async () => {
    console.info('確認申請列表空資料畫面')
    renderApplications({ applications: [] })

    expect(await screen.findByText('沒有符合條件的申請')).toBeInTheDocument()
    expect(screen.getByText('共 0 筆')).toBeInTheDocument()
  })

  it('未知狀態與缺少關聯資料時會顯示 fallback', async () => {
    console.info('確認未知狀態與缺少關聯資料 fallback')
    renderApplications({
      applications: [
        {
          id: 'app-unknown',
          status: 'archived',
          quantity: 1,
          applied_at: '2099-06-03T02:00:00.000Z',
        },
      ],
    })

    expect(await screen.findByText('archived')).toBeInTheDocument()
    expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(2)
    expect(screen.getByText('共 1 筆')).toBeInTheDocument()
  })

  it('員工資料缺少部分欄位時仍可顯示列表 fallback', async () => {
    console.info('確認申請列表員工欄位缺漏分支')
    renderApplications({
      applications: [
        {
          ...pendingApplication,
          id: 'app-partial-user',
          user: {
            name: '資料不完整員工',
          },
        },
      ],
    })

    expect(await screen.findByText('資料不完整員工')).toBeInTheDocument()
    expect(screen.getByText('資料不完整員工').closest('tr')).toHaveTextContent('待審核')
    expect(screen.getByText('共 1 筆')).toBeInTheDocument()
  })

})
