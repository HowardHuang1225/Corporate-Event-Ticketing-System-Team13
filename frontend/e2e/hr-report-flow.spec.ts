import { expect, test } from '@playwright/test'
import { familyEvent, fulfillData, fulfillError, loginAs, mockApi, users } from './apiMock'

const overview = [
  {
    event_id: familyEvent.id,
    title: familyEvent.title,
    applied_apps: 3,
    applied_tickets: 6,
    applied_users: 3,
    approved_tickets: 6,
    cancelled: 1,
    total_tickets: 5,
    checked_in: 4,
    check_in_rate: 80,
  },
]

const stats = {
  event: { id: familyEvent.id, title: familyEvent.title },
  applied_apps: 3,
  applied_tickets: 6,
  applied_users: 3,
  approved_tickets: 6,
  cancelled_tickets: 1,
  total_tickets: 5,
  approved_users: 3,
  checked_in_tickets: 4,
  check_in_rate: 80,
  by_department: [{ department: '資訊部', count: 2 }],
  by_region: [{ region: '台南', count: 3 }],
  by_ticket_type: [{ ticket_type_name: '一般票', total: 6, approved: 6, cancelled: 1, active: 5 }],
}

test.describe('HR 報表 e2e', () => {
  test('HR 可查看總覽、切換活動詳情並匯出 CSV', async ({ page }) => {
    await mockApi(page, async ({ route, path, request }) => {
      if (path === '/auth/login' && request.method() === 'POST') {
        await fulfillData(route, { access_token: 'hr-e2e-token', user: users.hr })
        return
      }
      if (path === '/events' && request.method() === 'GET') {
        await fulfillData(route, [familyEvent])
        return
      }
      if (path === '/reports/overview' && request.method() === 'GET') {
        await fulfillData(route, overview)
        return
      }
      if (path === `/reports/events/${familyEvent.id}/stats` && request.method() === 'GET') {
        await fulfillData(route, stats)
        return
      }
      await fulfillError(route, 404, `未處理的 API：${path}`, 'NOT_MOCKED')
    })

    await test.step('HR 登入並開啟統計報表', async () => {
      await loginAs(page, 'HR001')
      await page.getByRole('link', { name: /統計報表/ }).click()
      await expect(page.getByRole('heading', { name: '統計報表' })).toBeVisible()
      await expect(page.getByRole('heading', { name: '活動總覽' })).toBeVisible()
      await expect(page.getByRole('row', { name: /家庭同樂日/ })).toContainText('80%')
    })

    await test.step('匯出活動總覽 CSV', async () => {
      const downloadPromise = page.waitForEvent('download')
      await page.getByRole('button', { name: /匯出總覽/ }).click()
      const download = await downloadPromise
      expect(download.suggestedFilename()).toMatch(/^活動總覽報表_\d{4}-\d{2}-\d{2}\.csv$/)
    })

    await test.step('選擇活動並確認詳細統計', async () => {
      await page.locator('select').selectOption(familyEvent.id)
      await expect(page.getByText('1. 申請階段')).toBeVisible()
      await expect(page.getByText('2. 核准與退票階段')).toBeVisible()
      await expect(page.getByText('3. 核銷與出席率')).toBeVisible()
      await expect(page.getByText('核准總張數')).toBeVisible()
      await expect(page.getByText('目前有效票數')).toBeVisible()
      await expect(page.getByText('實體核銷張數')).toBeVisible()
      await expect(page.getByText('資訊部')).toBeVisible()
      await expect(page.getByText('台南')).toBeVisible()
      await expect(page.getByText('一般票')).toBeVisible()
    })

    await test.step('匯出活動詳情 CSV', async () => {
      const downloadPromise = page.waitForEvent('download')
      await page.getByRole('button', { name: /匯出詳情 CSV/ }).click()
      const download = await downloadPromise
      expect(download.suggestedFilename()).toMatch(/^活動詳情_家庭同樂日_\d{4}-\d{2}-\d{2}\.csv$/)
    })
  })
})
