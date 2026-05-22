import { describe, expect, it, beforeEach, vi, type Mock } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import CheckIn from './CheckIn'
import api from '../../api/client'
import { renderWithQueryClient } from '../../test/test-utils'

vi.mock('../../api/client', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(),
  },
}))

const apiGet = api.get as Mock
const apiPost = api.post as Mock

const eventFixture = {
  id: 'event-1',
  title: '年度家庭日',
}

const ticketFixture = {
  event: { title: '年度家庭日' },
  ticket_type: { name: '一般票' },
  user: {
    name: '王小明',
    employee_id: 'EMP001',
    department: '資訊部',
    region: '台南',
  },
}

function renderCheckIn() {
  apiGet.mockImplementation((url: string, config?: { params?: Record<string, string> }) => {
    if (url === '/events') return Promise.resolve({ data: { data: [eventFixture] } })
    if (url === '/checkins') {
      return Promise.resolve({
        data: {
          data: config?.params?.event_id
            ? [
                {
                  id: 'checkin-1',
                  checked_at: '2099-07-01T02:00:00.000Z',
                  ticket: {
                    event: { title: '年度家庭日' },
                    ticket_type: { name: '一般票' },
                    qr_token: 'QR-TOKEN-123456',
                  },
                },
              ]
            : [],
        },
      })
    }
    return Promise.reject(new Error(`未處理的 API 路徑：${url}`))
  })

  return renderWithQueryClient(<CheckIn />)
}

describe('CheckIn', () => {
  beforeEach(() => {
    apiGet.mockReset()
    apiPost.mockReset()
  })

  it('輸入 QR token 後會 trim 並送出核銷 API', async () => {
    console.info('確認核銷成功流程與 token trim')
    apiPost.mockResolvedValue({ data: { data: { ticket: ticketFixture } } })
    const { container } = renderCheckIn()

    const tokenInput = container.querySelector<HTMLInputElement>('#qr-token-input')
    if (!tokenInput) throw new Error('QR token 欄位沒有正確渲染')

    await userEvent.type(tokenInput, '  QR-TOKEN-123456  ')
    await userEvent.click(screen.getByRole('button', { name: '確認核銷' }))

    await waitFor(() => {
      expect(apiPost).toHaveBeenCalledWith('/checkin', { qr_token: 'QR-TOKEN-123456' })
    })
    expect(await screen.findByText(/核銷成功/)).toBeInTheDocument()
    expect(screen.getByText(/王小明/)).toBeInTheDocument()
    expect(tokenInput).toHaveValue('')
  })

  it('空白 token 時不會送出核銷 API', async () => {
    console.info('確認空白 token 不允許送出')
    const { container } = renderCheckIn()

    const tokenInput = container.querySelector<HTMLInputElement>('#qr-token-input')
    if (!tokenInput) throw new Error('QR token 欄位沒有正確渲染')

    await userEvent.type(tokenInput, '   ')

    expect(screen.getByRole('button', { name: '確認核銷' })).toBeDisabled()
    expect(apiPost).not.toHaveBeenCalled()
  })

  it('票券已核銷錯誤會顯示對應訊息', async () => {
    console.info('確認 ALREADY_CHECKED_IN 錯誤訊息')
    apiPost.mockRejectedValue({
      response: {
        data: {
          error: {
            code: 'ALREADY_CHECKED_IN',
            message: '已核銷',
          },
        },
      },
    })
    const { container } = renderCheckIn()

    const tokenInput = container.querySelector<HTMLInputElement>('#qr-token-input')
    if (!tokenInput) throw new Error('QR token 欄位沒有正確渲染')

    await userEvent.type(tokenInput, 'QR-TOKEN-123456')
    await userEvent.click(screen.getByRole('button', { name: '確認核銷' }))

    expect(await screen.findByText('❌ 此票券已核銷，請勿重複使用')).toBeInTheDocument()
  })

  it('找不到票券錯誤會顯示對應訊息', async () => {
    console.info('確認 NOT_FOUND 錯誤訊息')
    apiPost.mockRejectedValue({
      response: {
        data: {
          error: {
            code: 'NOT_FOUND',
            message: '找不到票券',
          },
        },
      },
    })
    const { container } = renderCheckIn()

    const tokenInput = container.querySelector<HTMLInputElement>('#qr-token-input')
    if (!tokenInput) throw new Error('QR token 欄位沒有正確渲染')

    await userEvent.type(tokenInput, 'UNKNOWN-TOKEN')
    await userEvent.click(screen.getByRole('button', { name: '確認核銷' }))

    expect(await screen.findByText('❌ 找不到此票券，請確認 QR Code 是否正確')).toBeInTheDocument()
  })

  it('未知核銷錯誤會顯示後端訊息', async () => {
    console.info('確認泛用核銷錯誤訊息')
    apiPost.mockRejectedValue({
      response: {
        data: {
          error: {
            code: 'UNKNOWN',
            message: '核銷服務暫時不可用',
          },
        },
      },
    })
    const { container } = renderCheckIn()

    const tokenInput = container.querySelector<HTMLInputElement>('#qr-token-input')
    if (!tokenInput) throw new Error('QR token 欄位沒有正確渲染')

    await userEvent.type(tokenInput, 'QR-TOKEN-123456')
    await userEvent.click(screen.getByRole('button', { name: '確認核銷' }))

    expect(await screen.findByText('❌ 核銷服務暫時不可用')).toBeInTheDocument()
  })

  it('切換活動篩選會用 event_id 重新查詢核銷紀錄', async () => {
    console.info('確認核銷紀錄活動篩選參數')
    renderCheckIn()

    await screen.findByText('年度家庭日…')
    const select = await screen.findByRole('combobox')
    await userEvent.selectOptions(select, 'event-1')

    await waitFor(() => {
      expect(apiGet).toHaveBeenCalledWith('/checkins', { params: { event_id: 'event-1' } })
    })
    expect(await screen.findByText('年度家庭日')).toBeInTheDocument()
  })
})
