import { describe, expect, it, beforeEach, vi, type Mock } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import EventManage from './EventManage'
import api from '../../api/client'
import { fillField, renderWithQueryClient } from '../../test/test-utils'
import toast from 'react-hot-toast'

vi.mock('../../api/client', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
    patch: vi.fn(),
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
const apiPut = api.put as Mock
const apiDelete = api.delete as Mock
const apiPatch = api.patch as Mock
const toastSuccess = toast.success as Mock
const toastError = toast.error as Mock

const draftEvent = {
  id: 'event-1',
  title: '年度家庭日草稿',
  description: '草稿描述',
  venue: '台南園區',
  status: 'draft',
  publish_time: '2099-06-01T01:00:00.000Z',
  start_time: '2099-07-01T01:00:00.000Z',
  end_time: '2099-07-01T09:00:00.000Z',
  apply_deadline: '2099-06-20T09:00:00.000Z',
  region_restriction: '台南',
  max_tickets_per_person: 2,
  ticket_types: [
    {
      id: 'ticket-type-1',
      name: '一般票',
      remaining: 12,
      total_quota: 30,
    },
  ],
}

const publishedEvent = {
  ...draftEvent,
  id: 'event-2',
  title: '已發布活動',
  status: 'published',
}

function isoFromLocal(localDatetime: string) {
  return new Date(localDatetime).toISOString()
}

function renderEventManage(events = [draftEvent, publishedEvent]) {
  apiGet.mockResolvedValue({ data: { data: events } })
  apiPost.mockResolvedValue({ data: { data: {} } })
  apiPut.mockResolvedValue({ data: { data: {} } })
  apiDelete.mockResolvedValue({ data: { data: {} } })
  apiPatch.mockResolvedValue({ data: { data: {} } })

  return renderWithQueryClient(<EventManage />)
}

function getModalFields(container: HTMLElement) {
  const inputs = Array.from(container.querySelectorAll<HTMLInputElement>('.modal input:not([type="file"])'))
  const textarea = container.querySelector<HTMLTextAreaElement>('.modal textarea')
  if (inputs.length < 8 || !textarea) {
    throw new Error('活動表單欄位沒有正確渲染')
  }
  return {
    title: inputs[0],
    description: textarea,
    venue: inputs[1],
    publishTime: inputs[2],
    startTime: inputs[3],
    applyDeadline: inputs[4],
    endTime: inputs[5],
    maxTickets: inputs[6],
    region: inputs[7],
    ticketName: screen.getByPlaceholderText('票種名稱'),
    ticketQuota: screen.getByPlaceholderText('數量'),
  }
}

describe('EventManage', () => {
  beforeEach(() => {
    apiGet.mockReset()
    apiPost.mockReset()
    apiPut.mockReset()
    apiDelete.mockReset()
    apiPatch.mockReset()
    toastSuccess.mockReset()
    toastError.mockReset()
    vi.stubGlobal('confirm', vi.fn(() => true))
    vi.spyOn(console, 'log').mockImplementation(() => undefined)
  })

  it('會載入活動列表並依狀態顯示操作按鈕', async () => {
    console.info('確認活動管理列表與主要操作按鈕')
    renderEventManage()

    expect(await screen.findByText('年度家庭日草稿')).toBeInTheDocument()
    expect(screen.getByText('已發布活動')).toBeInTheDocument()
    expect(screen.getByText('排程中')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /發布/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /編輯/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /刪除/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /截止/ })).toBeInTheDocument()
  })

  it('會顯示發布處理中、已截止與已結束狀態', async () => {
    console.info('確認活動狀態 badge 其他分支')
    renderEventManage([
      {
        ...draftEvent,
        id: 'event-pending',
        title: '待發布草稿',
        publish_time: '2000-06-01T01:00:00.000Z',
        apply_deadline: '2099-06-20T09:00:00.000Z',
      },
      {
        ...draftEvent,
        id: 'event-closed',
        title: '已截止活動',
        status: 'closed',
      },
      {
        ...draftEvent,
        id: 'event-ended',
        title: '已結束活動',
        status: 'ended',
      },
    ])

    expect(await screen.findByText('發布處理中')).toBeInTheDocument()
    expect(screen.getByText('已截止')).toBeInTheDocument()
    expect(screen.getByText('已結束')).toBeInTheDocument()
  })

  it('建立活動會把表單資料轉成後端 payload', async () => {
    console.info('確認建立活動 payload 轉換')
    const { container, queryClient } = renderEventManage([])
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    await userEvent.click(await screen.findByRole('button', { name: /建立新活動/ }))
    const fields = getModalFields(container)

    fillField(fields.title, '新品發表會')
    fillField(fields.description, '年度新品發表活動')
    fillField(fields.venue, '台北總部')
    fillField(fields.publishTime, '2099-06-01T10:00')
    fillField(fields.startTime, '2099-07-01T10:00')
    fillField(fields.applyDeadline, '2099-06-20T17:00')
    fillField(fields.endTime, '2099-07-01T18:00')
    fillField(fields.maxTickets, '4')
    fillField(fields.region, '台北')
    fillField(fields.ticketName, 'VIP票')
    fillField(fields.ticketQuota, '80')
    await userEvent.click(screen.getByRole('button', { name: '儲存草稿' }))

    await waitFor(() => {
      expect(apiPost).toHaveBeenCalledWith('/events', expect.objectContaining({
        title: '新品發表會',
        description: '年度新品發表活動',
        venue: '台北總部',
        image_url: '',
        document_url: '',
        publish_time: isoFromLocal('2099-06-01T10:00'),
        start_time: isoFromLocal('2099-07-01T10:00'),
        apply_deadline: isoFromLocal('2099-06-20T17:00'),
        end_time: isoFromLocal('2099-07-01T18:00'),
        region_restriction: '台北',
        max_tickets_per_person: 4,
        ticket_types: [{ name: 'VIP票', total_quota: 80 }],
      }))
    })
    expect(toastSuccess).toHaveBeenCalledWith('活動建立成功！')
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['events-manage'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['events'] })
  })

  it('建立活動未填日期與地域時會送出空字串與 null', async () => {
    console.info('確認建立活動空欄位 payload fallback')
    renderEventManage([])

    await userEvent.click(await screen.findByRole('button', { name: /建立新活動/ }))
    await userEvent.click(screen.getByRole('button', { name: '儲存草稿' }))

    await waitFor(() => {
      expect(apiPost).toHaveBeenCalledWith('/events', expect.objectContaining({
        publish_time: '',
        start_time: '',
        apply_deadline: '',
        end_time: '',
        region_restriction: null,
        ticket_types: [{ name: '一般票', total_quota: 100 }],
      }))
    })
  })

  it('可以新增與移除票種欄位', async () => {
    console.info('確認活動表單票種增刪分支')
    const { container } = renderEventManage([])

    await userEvent.click(await screen.findByRole('button', { name: /建立新活動/ }))
    expect(container.querySelectorAll('input[placeholder="票種名稱"]')).toHaveLength(1)

    await userEvent.click(screen.getByRole('button', { name: /新增票種/ }))
    expect(container.querySelectorAll('input[placeholder="票種名稱"]')).toHaveLength(2)

    const removeButton = container.querySelector<HTMLButtonElement>('.btn-icon')
    if (!removeButton) throw new Error('刪除票種按鈕沒有正確渲染')
    await userEvent.click(removeButton)

    expect(container.querySelectorAll('input[placeholder="票種名稱"]')).toHaveLength(1)
  })

  it('編輯活動會預填資料並送出 update API', async () => {
    console.info('確認編輯活動表單與 update payload')
    const { container } = renderEventManage([draftEvent])

    await screen.findByText('年度家庭日草稿')
    await userEvent.click(screen.getByRole('button', { name: /編輯/ }))
    const fields = getModalFields(container)
    expect(fields.title).toHaveValue('年度家庭日草稿')
    expect(screen.getByText('編輯活動')).toBeInTheDocument()

    fillField(fields.title, '更新後活動')
    await userEvent.click(screen.getByRole('button', { name: '儲存草稿' }))

    await waitFor(() => {
      expect(apiPut).toHaveBeenCalledWith('/events/event-1', expect.objectContaining({
        title: '更新後活動',
        venue: '台南園區',
        region_restriction: '台南',
      }))
    })
    expect(toastSuccess).toHaveBeenCalledWith('活動更新成功！')
  })

  it('編輯活動遇到無效日期與空票種時會使用表單 fallback', async () => {
    console.info('確認編輯活動的日期與票種 fallback')
    const { container } = renderEventManage([
      {
        ...draftEvent,
        publish_time: 'invalid-date',
        start_time: '',
        apply_deadline: '',
        end_time: '',
        region_restriction: '',
        max_tickets_per_person: 0,
        ticket_types: [],
      },
    ])

    await screen.findByText('年度家庭日草稿')
    await userEvent.click(screen.getByRole('button', { name: /編輯/ }))
    const fields = getModalFields(container)

    expect(fields.publishTime).toHaveValue('')
    expect(fields.startTime).toHaveValue('')
    expect(fields.applyDeadline).toHaveValue('')
    expect(fields.endTime).toHaveValue('')
    expect(fields.region).toHaveValue('')
    expect(fields.maxTickets).toHaveValue(1)
    expect(fields.ticketName).toHaveValue('一般票')
    expect(fields.ticketQuota).toHaveValue(100)
  })

  it('發布時間晚於申請截止時會阻擋發布並顯示錯誤', async () => {
    console.info('確認發布前 timeline 驗證')
    renderEventManage([
      {
        ...draftEvent,
        publish_time: '2099-06-01T01:00:00.000Z',
        apply_deadline: '2000-06-20T09:00:00.000Z',
      },
    ])

    await screen.findByText('年度家庭日草稿')
    await userEvent.click(screen.getByRole('button', { name: /發布/ }))

    expect(toastError).toHaveBeenCalledWith('發布時間必須早於申請截止時間')
    expect(apiPatch).not.toHaveBeenCalled()
  })

  it('活動結束時間早於開始時間時會阻擋發布', async () => {
    console.info('確認活動時間順序驗證分支')
    renderEventManage([
      {
        ...draftEvent,
        publish_time: '2099-06-01T01:00:00.000Z',
        apply_deadline: '2099-06-20T09:00:00.000Z',
        start_time: '2099-07-01T09:00:00.000Z',
        end_time: '2099-07-01T01:00:00.000Z',
      },
    ])

    await screen.findByText('年度家庭日草稿')
    await userEvent.click(screen.getByRole('button', { name: /發布/ }))

    expect(toastError).toHaveBeenCalledWith('活動結束時間必須晚於開始時間')
    expect(apiPatch).not.toHaveBeenCalled()
  })

  it('可發布、截止與刪除活動時會呼叫對應 API', async () => {
    console.info('確認活動發布、截止與刪除 API')
    renderEventManage()

    await screen.findByText('年度家庭日草稿')
    await userEvent.click(screen.getByRole('button', { name: /發布/ }))
    await waitFor(() => expect(apiPatch).toHaveBeenCalledWith('/events/event-1/publish'))

    await userEvent.click(screen.getByRole('button', { name: /截止/ }))
    await waitFor(() => expect(apiPatch).toHaveBeenCalledWith('/events/event-2/close'))

    await userEvent.click(screen.getByRole('button', { name: /刪除/ }))
    await waitFor(() => expect(apiDelete).toHaveBeenCalledWith('/events/event-1'))
  })

  it('刪除確認取消時不會呼叫 delete API', async () => {
    console.info('確認刪除取消分支')
    vi.stubGlobal('confirm', vi.fn(() => false))
    renderEventManage([draftEvent])

    await screen.findByText('年度家庭日草稿')
    await userEvent.click(screen.getByRole('button', { name: /刪除/ }))

    expect(apiDelete).not.toHaveBeenCalled()
  })

  it('建立活動失敗時會顯示預設錯誤訊息', async () => {
    console.info('確認建立活動錯誤分支')
    renderEventManage([])
    apiPost.mockRejectedValue(new Error('network error'))

    await userEvent.click(await screen.findByRole('button', { name: /建立新活動/ }))
    await userEvent.click(screen.getByRole('button', { name: '儲存草稿' }))

    await waitFor(() => expect(toastError).toHaveBeenCalledWith('建立失敗'))
  })

  it('可以上傳圖片與 PDF，成功後回填預覽與附件狀態', async () => {
    console.info('確認活動表單檔案上傳成功分支')
    renderEventManage([])
    apiPost.mockImplementation((url: string) => {
      if (url === '/upload') {
        return Promise.resolve({ data: { data: { url: 'https://cdn.example.com/uploaded-file' } } })
      }
      return Promise.resolve({ data: { data: {} } })
    })

    await userEvent.click(await screen.findByRole('button', { name: /建立新活動/ }))
    const imageInput = screen.getByLabelText('活動圖片')
    const docInput = screen.getByLabelText('活動文件 (僅限 PDF)')

    await userEvent.upload(imageInput, new File(['image'], 'cover.png', { type: 'image/png' }))
    await waitFor(() => expect(toastSuccess).toHaveBeenCalledWith('圖片上傳成功！'))
    expect(await screen.findByText('圖片已上傳成功')).toBeInTheDocument()

    await userEvent.upload(docInput, new File(['pdf'], 'guide.pdf', { type: 'application/pdf' }))
    await waitFor(() => expect(toastSuccess).toHaveBeenCalledWith('附件上傳成功！'))
    expect(await screen.findByText('📄 PDF 附件已上傳成功')).toBeInTheDocument()
    expect(apiPost).toHaveBeenCalledWith('/upload', expect.any(FormData), { headers: { 'Content-Type': 'multipart/form-data' } })
  })

  it('上傳檔案會拒絕過大檔案、錯誤型別並顯示 API 錯誤訊息', async () => {
    console.info('確認活動表單檔案上傳錯誤分支')
    renderEventManage([])

    await userEvent.click(await screen.findByRole('button', { name: /建立新活動/ }))
    const imageInput = screen.getByLabelText('活動圖片')
    const docInput = screen.getByLabelText('活動文件 (僅限 PDF)')

    const largeImage = new File(['x'], 'large.png', { type: 'image/png' })
    Object.defineProperty(largeImage, 'size', { value: 6 * 1024 * 1024 })
    await userEvent.upload(imageInput, largeImage)
    await waitFor(() => expect(toastError).toHaveBeenCalledWith('檔案大小不能超過 5MB'))

    toastError.mockClear()
    await userEvent.upload(imageInput, new File(['text'], 'note.txt', { type: 'text/plain' }), { applyAccept: false })
    await waitFor(() => expect(toastError).toHaveBeenCalledWith('請上傳圖片檔案'))

    toastError.mockClear()
    await userEvent.upload(docInput, new File(['image'], 'cover.png', { type: 'image/png' }), { applyAccept: false })
    await waitFor(() => expect(toastError).toHaveBeenCalledWith('請上傳 PDF 檔案'))

    toastError.mockClear()
    apiPost.mockRejectedValueOnce({ response: { data: { error: { message: 'Storage 暫時不可用' } } } })
    await userEvent.upload(docInput, new File(['pdf'], 'guide.pdf', { type: 'application/pdf' }))
    await waitFor(() => expect(toastError).toHaveBeenCalledWith('Storage 暫時不可用'))
  })

  it('建立活動 modal 可以用取消與 overlay 關閉', async () => {
    console.info('確認活動表單關閉分支')
    renderEventManage([])

    await userEvent.click(await screen.findByRole('button', { name: /建立新活動/ }))
    expect(screen.getAllByText('建立新活動')).toHaveLength(2)
    await userEvent.click(screen.getByRole('button', { name: '取消' }))
    expect(screen.getAllByText('建立新活動')).toHaveLength(1)

    await userEvent.click(screen.getByRole('button', { name: /建立新活動/ }))
    await userEvent.click(screen.getByRole('button', { name: '關閉活動表單' }))
    expect(screen.getAllByText('建立新活動')).toHaveLength(1)
  })

  it('更新、發布、刪除失敗時會顯示 fallback 或後端錯誤訊息', async () => {
    console.info('確認活動管理 mutation 錯誤分支')
    renderEventManage([draftEvent])

    apiPut.mockRejectedValueOnce({ response: { data: { error: { message: '更新資料不合法' } } } })
    await screen.findByText('年度家庭日草稿')
    await userEvent.click(screen.getByRole('button', { name: /編輯/ }))
    await userEvent.click(screen.getByRole('button', { name: '儲存草稿' }))
    await waitFor(() => expect(toastError).toHaveBeenCalledWith('更新資料不合法'))

    apiPatch.mockRejectedValueOnce(new Error('plain publish error'))
    await userEvent.click(screen.getByRole('button', { name: /發布/ }))
    await waitFor(() => expect(toastError).toHaveBeenCalledWith('發布失敗'))

    apiDelete.mockRejectedValueOnce({ response: { data: { error: { message: '草稿不可刪除' } } } })
    await userEvent.click(screen.getByRole('button', { name: /刪除/ }))
    await waitFor(() => expect(toastError).toHaveBeenCalledWith('草稿不可刪除'))
  })

})
