import { expect, test } from '@playwright/test'
import {
  approvedApplication,
  approvedTicket,
  eventsByStatus,
  familyEvent,
  fulfillData,
  fulfillError,
  loginAs,
  mockApi,
  mockTotp,
  postJson,
  users,
} from './apiMock'

type ApplicationPayload = {
  event_id: string
  ticket_type_id: string
  quantity: number
  idempotency_key: string
}

test.describe('員工票券 e2e', () => {
  test('員工送出申請後會自動核准並在我的票券看到動態 QR', async ({ page }) => {
    const applicationRequests: ApplicationPayload[] = []
    let currentEvent = { ...familyEvent, ticket_types: familyEvent.ticket_types.map(ticketType => ({ ...ticketType })) }
    let applications: Array<typeof approvedApplication> = []
    let tickets: Array<typeof approvedTicket> = []

    await page.clock.install({ time: new Date('2099-06-10T01:00:00.000Z') })
    await mockTotp(page, ['123456', '123456', '654321', '654321'])
    await mockApi(page, async ({ route, path, request, url }) => {
      if (path === '/auth/login' && request.method() === 'POST') {
        await fulfillData(route, { access_token: 'employee-e2e-token', user: users.employee })
        return
      }
      if (path === '/events' && request.method() === 'GET') {
        await fulfillData(route, eventsByStatus([currentEvent], url))
        return
      }
      if (path === `/events/${familyEvent.id}` && request.method() === 'GET') {
        await fulfillData(route, currentEvent)
        return
      }
      if (path === '/applications' && request.method() === 'POST') {
        const payload = postJson<ApplicationPayload>(request)
        applicationRequests.push(payload)
        applications = [{ ...approvedApplication, quantity: payload.quantity }]
        tickets = [{ ...approvedTicket }]
        currentEvent = {
          ...currentEvent,
          ticket_types: currentEvent.ticket_types.map(ticketType => ({
            ...ticketType,
            remaining: ticketType.remaining - payload.quantity,
          })),
        }
        await fulfillData(route, { status: 'approved', application: applications[0], tickets })
        return
      }
      if (path === '/applications/my' && request.method() === 'GET') {
        await fulfillData(route, applications)
        return
      }
      if (path === '/tickets/my' && request.method() === 'GET') {
        await fulfillData(route, tickets)
        return
      }
      await fulfillError(route, 404, `未處理的 API：${path}`, 'NOT_MOCKED')
    })

    await test.step('員工登入並進入活動詳情', async () => {
      await loginAs(page, 'EMP001')
      await expect(page.getByRole('heading', { name: '活動列表' })).toBeVisible()
      await page.getByText(familyEvent.title).click()
      await expect(page).toHaveURL(new RegExp(`/events/${familyEvent.id}$`))
      await expect(page.getByText('可選票種')).toBeVisible()
    })

    await test.step('送出申請並確認前端呼叫申請 API', async () => {
      await page.getByRole('button', { name: '申請' }).click()
      const modal = page.locator('.modal')
      await expect(modal.getByText('申請票券')).toBeVisible()
      await modal.locator('input[type="number"]').fill('2')
      await modal.getByRole('button', { name: '確認申請' }).click()

      await expect.poll(() => applicationRequests.length).toBe(1)
      expect(applicationRequests[0]).toMatchObject({
        event_id: familyEvent.id,
        ticket_type_id: 'ticket-type-general',
        quantity: 2,
      })
      expect(applicationRequests[0].idempotency_key).toBeTruthy()
      await expect(page.getByText(/搶票成功.*自動發票/)).toBeVisible()
    })

    await test.step('我的票券顯示已核准申請與完整動態 QR 格式', async () => {
      await page.getByRole('link', { name: /我的票券/ }).click()
      await expect(page.getByRole('heading', { name: '我的票券' })).toBeVisible()
      await expect(page.getByRole('heading', { name: /電子票券/ })).toBeVisible()
      await expect(page.getByText('已核准')).toBeVisible()
      await expect(page.getByText('未使用')).toBeVisible()

      await page.getByRole('button', { name: '顯示 QR' }).click()
      await expect(page.getByText('QR-AUTO-APPROVED-001|123456')).toBeVisible()
      await expect(page.getByText(/防偽驗證碼將在/)).toBeVisible()

      await page.clock.runFor(60_000)
      await expect(page.getByText('QR-AUTO-APPROVED-001|654321')).toBeVisible()
      await expect(page.getByText('QR-AUTO-APPROVED-001|123456')).toHaveCount(0)
    })
  })

  test('員工申請進入排隊時會顯示排隊訊息且不立即產出票券', async ({ page }) => {
    const applicationRequests: ApplicationPayload[] = []

    await mockApi(page, async ({ route, path, request, url }) => {
      if (path === '/auth/login' && request.method() === 'POST') {
        await fulfillData(route, { access_token: 'employee-queue-e2e-token', user: users.employee })
        return
      }
      if (path === '/events' && request.method() === 'GET') {
        await fulfillData(route, eventsByStatus([familyEvent], url))
        return
      }
      if (path === `/events/${familyEvent.id}` && request.method() === 'GET') {
        await fulfillData(route, familyEvent)
        return
      }
      if (path === '/applications' && request.method() === 'POST') {
        const payload = postJson<ApplicationPayload>(request)
        applicationRequests.push(payload)
        await fulfillData(route, {
          id: 'queue-application-e2e',
          event_id: payload.event_id,
          ticket_type_id: payload.ticket_type_id,
          quantity: payload.quantity,
          status: 'queued',
          idempotency_key: payload.idempotency_key,
        }, 202)
        return
      }
      if (path === '/applications/my' && request.method() === 'GET') {
        await fulfillData(route, [])
        return
      }
      if (path === '/tickets/my' && request.method() === 'GET') {
        await fulfillData(route, [])
        return
      }
      await fulfillError(route, 404, `未處理的 API：${path}`, 'NOT_MOCKED')
    })

    await test.step('員工登入並進入活動詳情', async () => {
      await loginAs(page, 'EMP001')
      await page.getByText(familyEvent.title).click()
      await expect(page).toHaveURL(new RegExp(`/events/${familyEvent.id}$`))
    })

    await test.step('送出申請並確認排隊訊息', async () => {
      await page.getByRole('button', { name: '申請' }).click()
      const modal = page.locator('.modal')
      await modal.getByRole('button', { name: '確認申請' }).click()

      await expect.poll(() => applicationRequests.length).toBe(1)
      expect(applicationRequests[0]).toMatchObject({
        event_id: familyEvent.id,
        ticket_type_id: 'ticket-type-general',
        quantity: 1,
      })
      await expect(page.getByText('已進入排隊，系統會依序處理申請')).toBeVisible()
      await expect(page).toHaveURL(new RegExp(`/events/${familyEvent.id}$`))
    })

    await test.step('切到我的票券時不會顯示立即產生的電子票券', async () => {
      await page.getByRole('link', { name: /我的票券/ }).click()
      await expect(page.getByRole('heading', { name: '我的票券' })).toBeVisible()
      await expect(page.getByText('還沒有申請記錄，快去瀏覽活動吧！')).toBeVisible()
      await expect(page.getByRole('heading', { name: /電子票券/ })).toHaveCount(0)
    })
  })
})
