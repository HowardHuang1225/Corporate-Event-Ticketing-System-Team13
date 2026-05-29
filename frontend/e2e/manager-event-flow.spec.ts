import { expect, test } from '@playwright/test'
import {
  draftEvent,
  eventsByStatus,
  fulfillData,
  fulfillError,
  loginAs,
  mockApi,
  postJson,
  publishedManagerEvent,
  users,
} from './apiMock'

type EventPayload = {
  title: string
  description: string
  venue: string
  region_restriction: string | null
  max_tickets_per_person: number
  ticket_types: Array<{ name: string; total_quota: number }>
}

test.describe('Manager 活動與核銷 e2e', () => {
  test('manager 可建立活動草稿、發布草稿並截止已發布活動', async ({ page }) => {
    const createRequests: EventPayload[] = []
    const patchRequests: string[] = []
    const managerEvents = [draftEvent, publishedManagerEvent]

    await mockApi(page, async ({ route, path, request, url }) => {
      if (path === '/auth/login' && request.method() === 'POST') {
        await fulfillData(route, { access_token: 'manager-e2e-token', user: users.manager })
        return
      }
      if (path === '/events' && request.method() === 'GET') {
        await fulfillData(route, eventsByStatus(managerEvents, url))
        return
      }
      if (path === '/events' && request.method() === 'POST') {
        const payload = postJson<EventPayload>(request)
        createRequests.push(payload)
        await fulfillData(route, { id: 'created-event-e2e' })
        return
      }
      if (path === `/events/${draftEvent.id}/publish` && request.method() === 'PATCH') {
        patchRequests.push(path)
        await fulfillData(route, {})
        return
      }
      if (path === `/events/${publishedManagerEvent.id}/close` && request.method() === 'PATCH') {
        patchRequests.push(path)
        await fulfillData(route, {})
        return
      }
      await fulfillError(route, 404, `未處理的 API：${path}`, 'NOT_MOCKED')
    })

    await test.step('manager 登入並開啟活動管理', async () => {
      await loginAs(page, 'MGR001')
      await page.getByRole('link', { name: /活動管理/ }).click()
      await expect(page.getByRole('heading', { name: '活動管理' })).toBeVisible()
      await expect(page.getByText(draftEvent.title)).toBeVisible()
      await expect(page.getByText(publishedManagerEvent.title)).toBeVisible()
    })

    await test.step('填寫活動管理表單並排除圖片/PDF file input', async () => {
      await page.getByRole('button', { name: /建立新活動/ }).click()
      const modal = page.locator('.modal')
      const textInputs = modal.locator('input:not([type="file"])')
      await expect(textInputs).toHaveCount(10)

      await textInputs.nth(0).fill('跨頁 e2e 活動')
      await modal.locator('textarea').fill('用來確認 manager e2e 流程')
      await textInputs.nth(1).fill('台北總部')
      await textInputs.nth(2).fill('2099-06-01T10:00')
      await textInputs.nth(3).fill('2099-07-01T10:00')
      await textInputs.nth(4).fill('2099-06-20T17:00')
      await textInputs.nth(5).fill('2099-07-01T18:00')
      await textInputs.nth(6).fill('2')
      await textInputs.nth(7).fill('台北')
      await textInputs.nth(8).fill('VIP票')
      await textInputs.nth(9).fill('50')
      await modal.getByRole('button', { name: '儲存草稿' }).click()

      await expect.poll(() => createRequests.length).toBe(1)
      await expect(page.locator('.modal')).toHaveCount(0)
      expect(createRequests[0]).toMatchObject({
        title: '跨頁 e2e 活動',
        description: '用來確認 manager e2e 流程',
        venue: '台北總部',
        region_restriction: '台北',
        max_tickets_per_person: 2,
        ticket_types: [{ name: 'VIP票', total_quota: 50 }],
      })
    })

    await test.step('發布草稿並截止已發布活動', async () => {
      await page.getByRole('button', { name: /發布/ }).click()
      await expect.poll(() => patchRequests).toContain(`/events/${draftEvent.id}/publish`)

      await page.getByRole('button', { name: /截止/ }).click()
      await expect.poll(() => patchRequests).toContain(`/events/${publishedManagerEvent.id}/close`)
    })
  })

  test('manager 可使用動態 QR token 核銷，並看到常見錯誤訊息', async ({ page }) => {
    const checkinRequests: string[] = []

    await mockApi(page, async ({ route, path, request }) => {
      if (path === '/auth/login' && request.method() === 'POST') {
        await fulfillData(route, { access_token: 'manager-e2e-token', user: users.manager })
        return
      }
      if (path === '/events' && request.method() === 'GET') {
        await fulfillData(route, [publishedManagerEvent])
        return
      }
      if (path === '/checkins' && request.method() === 'GET') {
        await fulfillData(route, [])
        return
      }
      if (path === '/checkin' && request.method() === 'POST') {
        const payload = postJson<{ qr_token: string }>(request)
        checkinRequests.push(payload.qr_token)

        if (payload.qr_token === 'QR-CHECKED-IN|123456') {
          await fulfillError(route, 409, '此票券已核銷', 'ALREADY_CHECKED_IN')
          return
        }
        if (payload.qr_token === 'QR-NOT-FOUND|123456') {
          await fulfillError(route, 404, '找不到此票券', 'NOT_FOUND')
          return
        }

        await fulfillData(route, {
          ticket: {
            id: 'ticket-checkin-e2e',
            event: { title: '家庭同樂日' },
            ticket_type: { name: '一般票' },
            qr_token: payload.qr_token,
            user: {
              name: '員工小明',
              employee_id: 'EMP001',
              department: '資訊部',
              region: '台南',
            },
          },
        })
        return
      }
      await fulfillError(route, 404, `未處理的 API：${path}`, 'NOT_MOCKED')
    })

    await test.step('進入現場核銷頁並送出動態 QR token', async () => {
      await loginAs(page, 'MGR001')
      await page.getByRole('link', { name: /現場核銷/ }).click()
      await expect(page.getByRole('heading', { name: '現場核銷' })).toBeVisible()

      await page.locator('#qr-token-input').fill('  QR-AUTO-APPROVED-001|123456  ')
      await page.getByRole('button', { name: '確認核銷' }).click()
      await expect.poll(() => checkinRequests).toContain('QR-AUTO-APPROVED-001|123456')
      await expect(page.getByText(/核銷成功/)).toBeVisible()
      await expect(page.getByText(/員工小明/)).toBeVisible()
    })

    await test.step('已核銷與找不到票券會顯示對應錯誤', async () => {
      await page.locator('#qr-token-input').fill('QR-CHECKED-IN|123456')
      await page.getByRole('button', { name: '確認核銷' }).click()
      await expect(page.getByText(/此票券已核銷/)).toBeVisible()

      await page.locator('#qr-token-input').fill('QR-NOT-FOUND|123456')
      await page.getByRole('button', { name: '確認核銷' }).click()
      await expect(page.getByText(/找不到此票券/)).toBeVisible()
    })
  })
})
