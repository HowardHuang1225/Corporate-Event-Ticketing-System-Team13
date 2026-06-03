import { expect, test } from '@playwright/test'
import { familyEvent, fulfillData, fulfillError, loginAs, mockApi, postJson, users } from './apiMock'

const usersByEmployeeId = {
  EMP001: users.employee,
  MGR001: users.manager,
  HR001: users.hr,
}

test.describe('登入與權限 e2e', () => {
  test('未登入訪問受保護頁面會導向登入頁', async ({ page }) => {
    await test.step('直接開啟我的票券', async () => {
      await page.goto('/my-tickets')
    })

    await test.step('確認被導向登入頁', async () => {
      await expect(page).toHaveURL(/\/login$/)
      await expect(page.locator('#employee-id')).toBeVisible()
      await expect(page.locator('#password')).toBeVisible()
    })
  })

  test('登入失敗時顯示後端錯誤訊息', async ({ page }) => {
    await mockApi(page, async ({ route, path, request }) => {
      if (path === '/auth/login' && request.method() === 'POST') {
        await fulfillError(route, 400, '帳號或密碼錯誤', 'INVALID_CREDENTIALS')
        return
      }
      await fulfillError(route, 404, `未處理的 API：${path}`, 'NOT_MOCKED')
    })

    await test.step('輸入錯誤密碼並送出', async () => {
      await loginAs(page, 'EMP001', 'wrong-password')
    })

    await test.step('停留登入頁並顯示錯誤', async () => {
      await expect(page).toHaveURL(/\/login$/)
      await expect(page.getByText('帳號或密碼錯誤')).toBeVisible()
    })
  })

  test('不同角色登入後會看到對應導覽項目', async ({ page }) => {
    await mockApi(page, async ({ route, path, request }) => {
      if (path === '/auth/login' && request.method() === 'POST') {
        const body = postJson<{ employee_id: keyof typeof usersByEmployeeId; password: string }>(request)
        const user = usersByEmployeeId[body.employee_id]
        if (!user || body.password !== 'password') {
          await fulfillError(route, 401, '帳號或密碼錯誤', 'INVALID_CREDENTIALS')
          return
        }
        await fulfillData(route, { access_token: `token-${body.employee_id}`, user })
        return
      }
      if (path === '/events' && request.method() === 'GET') {
        await fulfillData(route, [familyEvent])
        return
      }
      await fulfillError(route, 404, `未處理的 API：${path}`, 'NOT_MOCKED')
    })

    await test.step('以 manager 登入', async () => {
      await loginAs(page, 'MGR001')
      await expect(page).toHaveURL(/\/events$/)
      await expect(page.getByRole('heading', { name: '活動列表' })).toBeVisible()
    })

    await test.step('確認 manager 導覽具備管理功能且沒有員工票券入口', async () => {
      await expect(page.getByRole('link', { name: /活動管理/ })).toBeVisible()
      await expect(page.getByRole('link', { name: /現場核銷/ })).toBeVisible()
      await expect(page.getByRole('link', { name: /我的票券/ })).toHaveCount(0)
    })
  })
})
