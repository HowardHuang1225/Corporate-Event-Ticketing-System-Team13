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

const checkinFixture = {
  id: 'checkin-1',
  checked_at: '2099-07-01T02:00:00.000Z',
  ticket: {
    event: { title: '年度家庭日' },
    ticket_type: { name: '一般票' },
    qr_token: 'QR-TOKEN-123456',
  },
}

function renderCheckIn({
  events = [eventFixture],
  filteredCheckins = [checkinFixture],
  defaultCheckins = [],
}: {
  events?: any[]
  filteredCheckins?: any[]
  defaultCheckins?: any[]
} = {}) {
  apiGet.mockImplementation((url: string, config?: { params?: Record<string, string> }) => {
    if (url === '/events') return Promise.resolve({ data: { data: events } })
    if (url === '/checkins') {
      return Promise.resolve({
        data: {
          data: config?.params?.event_id
            ? filteredCheckins
            : defaultCheckins,
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
    await waitFor(() => {
      expect(apiGet.mock.calls.filter(call => call[0] === '/checkins').length).toBeGreaterThan(1)
    })
  })

  it('核銷成功但票券關聯資料缺漏時會顯示 fallback', async () => {
    console.info('確認核銷成功結果 fallback 資料')
    apiPost.mockResolvedValue({
      data: {
        data: {
          ticket: {
            event: null,
            ticket_type: null,
            user: {},
          },
        },
      },
    })
    const { container } = renderCheckIn()

    const tokenInput = container.querySelector<HTMLInputElement>('#qr-token-input')
    if (!tokenInput) throw new Error('QR token 欄位沒有正確渲染')

    await userEvent.type(tokenInput, 'QR-TOKEN-MISSING-RELATIONS')
    await userEvent.click(screen.getByRole('button', { name: '確認核銷' }))

    expect(await screen.findByText(/核銷成功！ -/)).toBeInTheDocument()
    expect(screen.getByText('— (—)')).toBeInTheDocument()
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

  it('核銷錯誤缺少後端訊息時會顯示預設失敗文案', async () => {
    console.info('確認核銷錯誤 fallback 訊息')
    apiPost.mockRejectedValue({})
    const { container } = renderCheckIn()

    const tokenInput = container.querySelector<HTMLInputElement>('#qr-token-input')
    if (!tokenInput) throw new Error('QR token 欄位沒有正確渲染')

    await userEvent.type(tokenInput, 'QR-TOKEN-ERROR')
    await userEvent.click(screen.getByRole('button', { name: '確認核銷' }))

    expect(await screen.findByText('❌ 核銷失敗')).toBeInTheDocument()
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

  it('沒有活動與核銷紀錄時會顯示空狀態', async () => {
    console.info('確認核銷紀錄空狀態與活動清單空資料')
    renderCheckIn({ events: [], defaultCheckins: [] })

    expect(await screen.findByText('尚無核銷記錄')).toBeInTheDocument()
    expect(screen.getByRole('option', { name: '所有活動' })).toBeInTheDocument()
  })

  it('核銷紀錄缺少關聯資料時會顯示 fallback', async () => {
    console.info('確認核銷紀錄 fallback 顯示')
    renderCheckIn({
      defaultCheckins: [
        {
          id: 'checkin-fallback',
          checked_at: '2099-07-02T02:00:00.000Z',
          ticket: {},
        },
      ],
    })

    expect(await screen.findByText('活動')).toBeInTheDocument()
    expect(screen.getByText(/Token: —/)).toBeInTheDocument()
  })

  it('可以開啟相機掃描 modal，遇到相機錯誤時顯示錯誤並可關閉', async () => {
    console.info('確認相機掃描錯誤與關閉流程')
    vi.stubGlobal('jsQR', vi.fn())
    vi.stubGlobal('navigator', {
      mediaDevices: {
        getUserMedia: vi.fn().mockRejectedValue(new Error('相機權限被拒絕')),
      },
    })

    renderCheckIn()

    await userEvent.click(screen.getByTitle('開啟相機掃描'))

    expect(await screen.findByText('相機掃描 QR Code')).toBeInTheDocument()
    expect(await screen.findByText(/相機權限被拒絕/)).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '' }))
    await waitFor(() => {
      expect(screen.queryByText('相機掃描 QR Code')).not.toBeInTheDocument()
    })
  })

  it('相機掃描成功後會自動送出核銷並釋放 camera stream', async () => {
    console.info('確認相機掃描成功會自動核銷並清理媒體串流')
    const stopTrack = vi.fn()
    const getUserMedia = vi.fn().mockResolvedValue({
      getTracks: () => [{ stop: stopTrack }],
    })
    const jsQR = vi.fn(() => ({ data: 'SCANNED-QR-TOKEN|123456' }))
    vi.stubGlobal('jsQR', jsQR)
    vi.stubGlobal('navigator', {
      mediaDevices: {
        getUserMedia,
      },
    })
    vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue(undefined)
    Object.defineProperty(HTMLMediaElement.prototype, 'readyState', {
      configurable: true,
      get: () => 4,
    })
    Object.defineProperty(HTMLMediaElement.prototype, 'videoWidth', {
      configurable: true,
      get: () => 320,
    })
    Object.defineProperty(HTMLMediaElement.prototype, 'videoHeight', {
      configurable: true,
      get: () => 240,
    })
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(() => ({
      drawImage: vi.fn(),
      getImageData: vi.fn(() => ({
        data: new Uint8ClampedArray(4),
        width: 1,
        height: 1,
      })),
    }) as unknown as CanvasRenderingContext2D)
    apiPost.mockResolvedValue({ data: { data: { ticket: ticketFixture } } })

    renderCheckIn()
    await userEvent.click(screen.getByTitle('開啟相機掃描'))

    await waitFor(() => {
      expect(getUserMedia).toHaveBeenCalledWith({
        video: { facingMode: 'environment', width: { ideal: 1280 }, height: { ideal: 720 } },
      })
    })
    await waitFor(() => {
      expect(jsQR).toHaveBeenCalled()
      expect(apiPost).toHaveBeenCalledWith('/checkin', { qr_token: 'SCANNED-QR-TOKEN|123456' })
    })
    expect(await screen.findByText(/核銷成功/)).toBeInTheDocument()
    await waitFor(() => {
      expect(stopTrack).toHaveBeenCalled()
      expect(screen.queryByText('相機掃描 QR Code')).not.toBeInTheDocument()
    })
  })
})
