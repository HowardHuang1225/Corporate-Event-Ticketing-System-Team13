import { describe, expect, it, beforeEach, vi, type Mock } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import Reports from './Reports'
import api from '../../api/client'
import { renderWithQueryClient } from '../../test/test-utils'

vi.mock('../../api/client', () => ({
  default: {
    get: vi.fn(),
  },
}))

const apiGet = api.get as Mock

const eventsFixture = [
  {
    id: 'event-1',
    title: '年度家庭日',
  },
]

const overviewFixture = [
  {
    event_id: 'event-1',
    title: '年度家庭日',
    applied_apps: 5,
    applied_tickets: 8,
    applied_users: 4,
    approved_tickets: 6,
    cancelled: 1,
    total_tickets: 5,
    checked_in: 3,
    check_in_rate: 60,
  },
]

const statsFixture = {
  event: { title: '年度家庭日' },
  applied_apps: 5,
  applied_tickets: 8,
  applied_users: 4,
  approved_tickets: 6,
  cancelled_tickets: 1,
  total_tickets: 5,
  approved_users: 4,
  checked_in_tickets: 3,
  check_in_rate: 60,
  by_department: [
    { department: '資訊部', count: 2 },
    { department: '行政部', count: 2 },
  ],
  by_region: [
    { region: '台南', count: 3 },
    { region: '新竹', count: 1 },
  ],
  by_ticket_type: [
    {
      ticket_type_name: '一般票',
      total: 8,
      approved: 6,
      cancelled: 1,
      active: 5,
    },
  ],
}

function renderReports() {
  apiGet.mockImplementation((url: string) => {
    if (url === '/events') return Promise.resolve({ data: { data: eventsFixture } })
    if (url === '/reports/overview') return Promise.resolve({ data: { data: overviewFixture } })
    if (url === '/reports/events/event-1/stats') return Promise.resolve({ data: { data: statsFixture } })
    return Promise.reject(new Error(`未處理的 API 路徑：${url}`))
  })

  return renderWithQueryClient(<Reports />)
}

function renderReportsWithStats(stats: typeof statsFixture, overview = overviewFixture) {
  apiGet.mockImplementation((url: string) => {
    if (url === '/events') return Promise.resolve({ data: { data: eventsFixture } })
    if (url === '/reports/overview') return Promise.resolve({ data: { data: overview } })
    if (url === '/reports/events/event-1/stats') return Promise.resolve({ data: { data: stats } })
    return Promise.reject(new Error(`未處理的 API 路徑：${url}`))
  })

  return renderWithQueryClient(<Reports />)
}

function stubCsvDownload() {
  const blobs: Blob[] = []
  const createObjectURL = vi.fn((blob: Blob) => {
    blobs.push(blob)
    return 'blob:report'
  })
  Object.defineProperty(URL, 'createObjectURL', {
    value: createObjectURL,
    configurable: true,
  })
  const click = vi.fn()
  const originalCreateElement = document.createElement.bind(document)
  vi.spyOn(document, 'createElement').mockImplementation(((tagName: string) => {
    const element = originalCreateElement(tagName)
    if (tagName === 'a') {
      Object.defineProperty(element, 'click', {
        value: click,
        configurable: true,
      })
    }
    return element
  }) as typeof document.createElement)

  return { createObjectURL, click, blobs }
}

describe('Reports', () => {
  beforeEach(() => {
    apiGet.mockReset()
  })

  it('會載入總覽資料，且未選活動時不查詢詳細統計', async () => {
    console.info('確認報表總覽與詳細統計的啟用條件')
    renderReports()

    expect(await screen.findByText('活動總覽')).toBeInTheDocument()
    expect(screen.getAllByText('年度家庭日').length).toBeGreaterThan(0)
    expect(screen.getByText('請選擇一個活動以查看詳細統計')).toBeInTheDocument()
    expect(apiGet).toHaveBeenCalledWith('/events')
    expect(apiGet).toHaveBeenCalledWith('/reports/overview')
    expect(apiGet).not.toHaveBeenCalledWith('/reports/events//stats')
  })

  it('選擇活動後會查詢詳細統計並顯示各區塊資料', async () => {
    console.info('確認選擇活動後查詢詳細統計')
    renderReports()

    await screen.findByRole('option', { name: '年度家庭日' })
    const select = await screen.findByRole('combobox')
    await userEvent.selectOptions(select, 'event-1')

    await waitFor(() => {
      expect(apiGet).toHaveBeenCalledWith('/reports/events/event-1/stats')
    })
    expect(await screen.findByText('1. 申請階段')).toBeInTheDocument()
    expect(screen.getByText('2. 核准與退票階段')).toBeInTheDocument()
    expect(screen.getByText('3. 核銷與出席率')).toBeInTheDocument()
    expect(screen.getByText('資訊部')).toBeInTheDocument()
    expect(screen.getByText('一般票')).toBeInTheDocument()
  })

  it('匯出總覽會建立 CSV 檔案下載', async () => {
    console.info('確認總覽 CSV 匯出流程')
    const { createObjectURL, click } = stubCsvDownload()
    renderReports()

    await screen.findByText('活動總覽')
    await userEvent.click(screen.getByRole('button', { name: /匯出總覽/ }))

    expect(createObjectURL).toHaveBeenCalledWith(expect.any(Blob))
    expect(click).toHaveBeenCalled()
  })

  it('選擇活動後可匯出詳細 CSV', async () => {
    console.info('確認詳細 CSV 匯出流程')
    const { createObjectURL, click } = stubCsvDownload()
    renderReports()

    await screen.findByRole('option', { name: '年度家庭日' })
    const select = await screen.findByRole('combobox')
    await userEvent.selectOptions(select, 'event-1')
    await screen.findByText('1. 申請階段')
    await userEvent.click(screen.getByRole('button', { name: /匯出詳情 CSV/ }))

    expect(createObjectURL).toHaveBeenCalledWith(expect.any(Blob))
    expect(click).toHaveBeenCalled()
  })

  it('詳細統計沒有分佈資料時會顯示尚無資料', async () => {
    console.info('確認報表詳細統計空分佈分支')
    renderReportsWithStats({
      ...statsFixture,
      approved_users: 0,
      by_department: [],
      by_region: [{ region: '', count: 0 }],
      by_ticket_type: [],
    }, [
      {
        ...overviewFixture[0],
        cancelled: 0,
      },
    ])

    await screen.findByRole('option', { name: '年度家庭日' })
    await userEvent.selectOptions(screen.getByRole('combobox'), 'event-1')

    const emptyTexts = await screen.findAllByText('尚無資料')
    expect(emptyTexts.length).toBeGreaterThanOrEqual(2)
    expect(screen.getByText('未設定')).toBeInTheDocument()
    expect(screen.getAllByText('0').length).toBeGreaterThan(0)
  })

  it('CSV 匯出會正確處理逗號與雙引號欄位', async () => {
    console.info('確認 CSV 匯出會正確 escape 特殊字元')
    const { blobs } = stubCsvDownload()
    renderReportsWithStats({
      ...statsFixture,
      event: { title: '年度 "家庭",日' },
      by_department: [
        { department: '資訊,部 "A"', count: 2 },
      ],
      by_ticket_type: [
        {
          ticket_type_name: '一般票 "A",B',
          total: 8,
          approved: 6,
          cancelled: 1,
          active: 5,
        },
      ],
    }, [
      {
        ...overviewFixture[0],
        title: '年度 "家庭",日',
      },
    ])

    await screen.findByRole('option', { name: '年度家庭日' })
    await userEvent.selectOptions(screen.getByRole('combobox'), 'event-1')
    await screen.findByText('1. 申請階段')
    await userEvent.click(screen.getByRole('button', { name: /匯出詳情 CSV/ }))

    expect(blobs).toHaveLength(1)
    const csvText = await blobs[0].text()
    expect(csvText).toContain('"活動標題","年度 ""家庭"",日"')
    expect(csvText).toContain('"資訊,部 ""A""","2"')
    expect(csvText).toContain('"一般票 ""A"",B","8","6","1","5"')
  })
})
