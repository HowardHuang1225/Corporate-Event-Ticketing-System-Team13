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

    await mockTotp(page, '123456')
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
    })
  })
})
