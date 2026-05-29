import { expect, test, type Request } from '@playwright/test'
import {
  approvedApplication,
  approvedTicket,
  draftEvent,
  eventsByStatus,
  fulfillData,
  fulfillError,
  loginAs,
  mockApi,
  mockTotp,
  postJson,
  users,
  type ApiHandler,
} from './apiMock'

type ApplicationPayload = {
  event_id: string
  ticket_type_id: string
  quantity: number
  idempotency_key: string
}

type AuthenticatedUser = typeof users.employee | typeof users.manager | typeof users.hr
type CrossRoleEvent = typeof draftEvent
type CrossRoleApplication = typeof approvedApplication
type CrossRoleTicket = typeof approvedTicket & {
  user?: {
    name: string
    employee_id: string
    department: string
    region: string
  }
  checked_in_at?: string | null
}

type CrossRoleState = {
  event: CrossRoleEvent
  applications: CrossRoleApplication[]
  tickets: CrossRoleTicket[]
  cancelledTickets: CrossRoleTicket[]
  applicationRequests: ApplicationPayload[]
  publishRequests: string[]
  closeRequests: string[]
  cancelRequests: string[]
  checkinRequests: string[]
}

function eventTicketType(state: CrossRoleState) {
  return state.event.ticket_types[0]
}

function activeTickets(state: CrossRoleState) {
  return state.tickets
}

function checkedInTickets(state: CrossRoleState) {
  return state.tickets.filter(ticket => ticket.is_used)
}

function approvedTicketCount(state: CrossRoleState) {
  return state.tickets.length + state.cancelledTickets.length
}

function checkInRate(state: CrossRoleState) {
  const activeCount = activeTickets(state).length
  return activeCount === 0 ? 0 : checkedInTickets(state).length / activeCount * 100
}

function overviewFor(state: CrossRoleState) {
  const appliedTickets = state.applications.reduce((sum, application) => sum + application.quantity, 0)
  return [{
    event_id: state.event.id,
    title: state.event.title,
    applied_apps: state.applications.length,
    applied_tickets: appliedTickets,
    applied_users: state.applications.length > 0 ? 1 : 0,
    approved_tickets: approvedTicketCount(state),
    cancelled: state.cancelledTickets.length,
    total_tickets: activeTickets(state).length,
    checked_in: checkedInTickets(state).length,
    check_in_rate: checkInRate(state),
  }]
}

function statsFor(state: CrossRoleState) {
  const appliedTickets = state.applications.reduce((sum, application) => sum + application.quantity, 0)
  const ticketType = eventTicketType(state)

  return {
    event: { id: state.event.id, title: state.event.title },
    applied_apps: state.applications.length,
    applied_tickets: appliedTickets,
    applied_users: state.applications.length > 0 ? 1 : 0,
    approved_tickets: approvedTicketCount(state),
    cancelled_tickets: state.cancelledTickets.length,
    total_tickets: activeTickets(state).length,
    approved_users: approvedTicketCount(state) > 0 ? 1 : 0,
    checked_in_tickets: checkedInTickets(state).length,
    check_in_rate: checkInRate(state),
    by_department: state.tickets.length > 0 ? [{ department: users.employee.department, count: 1 }] : [],
    by_region: state.tickets.length > 0 ? [{ region: users.employee.region, count: 1 }] : [],
    by_ticket_type: [{
      ticket_type_name: ticketType.name,
      total: appliedTickets,
      approved: approvedTicketCount(state),
      cancelled: state.cancelledTickets.length,
      active: activeTickets(state).length,
    }],
  }
}

function buildCrossRoleHandler(state: CrossRoleState): ApiHandler {
  const tokenUsers: Record<string, AuthenticatedUser> = {
    'employee-cross-role-token': users.employee,
    'manager-cross-role-token': users.manager,
    'hr-cross-role-token': users.hr,
  }

  const userFromRequest = (request: Request) => {
    const token = request.headers().authorization?.replace(/^Bearer\s+/i, '')
    return token ? tokenUsers[token] : undefined
  }

  return async ({ route, path, request, url }) => {
    if (path === '/auth/login' && request.method() === 'POST') {
      const body = postJson<{ employee_id: string; password: string }>(request)
      if (body.employee_id === 'EMP001' && body.password === 'password') {
        await fulfillData(route, { access_token: 'employee-cross-role-token', user: users.employee })
        return
      }
      if (body.employee_id === 'MGR001' && body.password === 'password') {
        await fulfillData(route, { access_token: 'manager-cross-role-token', user: users.manager })
        return
      }
      if (body.employee_id === 'HR001' && body.password === 'password') {
        await fulfillData(route, { access_token: 'hr-cross-role-token', user: users.hr })
        return
      }
      await fulfillError(route, 401, '帳號或密碼錯誤', 'INVALID_CREDENTIALS')
      return
    }

    if (path === '/auth/me' && request.method() === 'GET') {
      const user = userFromRequest(request)
      if (!user) {
        await fulfillError(route, 401, '尚未登入', 'UNAUTHORIZED')
        return
      }
      await fulfillData(route, user)
      return
    }

    if (path === '/events' && request.method() === 'GET') {
      const user = userFromRequest(request)
      const visibleEvents = user?.role === 'employee' && state.event.status !== 'published' ? [] : [state.event]
      await fulfillData(route, eventsByStatus(visibleEvents, url))
      return
    }

    if (path === `/events/${state.event.id}` && request.method() === 'GET') {
      await fulfillData(route, state.event)
      return
    }

    if (path === `/events/${state.event.id}/publish` && request.method() === 'PATCH') {
      state.publishRequests.push(path)
      state.event = { ...state.event, status: 'published' }
      await fulfillData(route, {})
      return
    }

    if (path === `/events/${state.event.id}/close` && request.method() === 'PATCH') {
      state.closeRequests.push(path)
      state.event = { ...state.event, status: 'closed' }
      await fulfillData(route, {})
      return
    }

    if (path === '/applications' && request.method() === 'POST') {
      const payload = postJson<ApplicationPayload>(request)
      state.applicationRequests.push(payload)
      const application = {
        ...approvedApplication,
        id: 'application-cross-role-approved',
        event: { title: state.event.title },
        quantity: payload.quantity,
      }
      const tickets = Array.from({ length: payload.quantity }, (_, index) => ({
        ...approvedTicket,
        id: `ticket-cross-role-approved-${index + 1}`,
        event: { title: state.event.title },
        ticket_type: { name: eventTicketType(state).name },
        qr_token: `QR-CROSS-ROLE-${String(index + 1).padStart(3, '0')}`,
        is_used: false,
        user: {
          name: users.employee.name,
          employee_id: users.employee.employee_id,
          department: users.employee.department,
          region: users.employee.region,
        },
        checked_in_at: null,
      }))
      state.applications = [application]
      state.tickets = tickets
      state.event = {
        ...state.event,
        ticket_types: state.event.ticket_types.map(ticketType => ({
          ...ticketType,
          remaining: ticketType.remaining - payload.quantity,
        })),
      }
      await fulfillData(route, { status: 'approved', application, tickets })
      return
    }

    if (path === '/applications/my' && request.method() === 'GET') {
      await fulfillData(route, state.applications)
      return
    }

    if (path === '/tickets/my' && request.method() === 'GET') {
      await fulfillData(route, state.tickets)
      return
    }

    const cancelMatch = path.match(/^\/tickets\/([^/]+)\/cancel$/)
    if (cancelMatch && request.method() === 'POST') {
      const [, ticketId] = cancelMatch
      state.cancelRequests.push(path)
      const ticketIndex = state.tickets.findIndex(ticket => ticket.id === ticketId)
      if (ticketIndex < 0) {
        await fulfillError(route, 404, '找不到此票券', 'NOT_FOUND')
        return
      }

      const [ticket] = state.tickets.splice(ticketIndex, 1)
      state.cancelledTickets.push(ticket)
      state.applications = state.applications.map(application => ({
        ...application,
        status: 'cancelled',
        reason: '員工自行退票',
      }))
      state.event = {
        ...state.event,
        ticket_types: state.event.ticket_types.map(ticketType => ({
          ...ticketType,
          remaining: ticketType.remaining + 1,
        })),
      }
      await fulfillData(route, {})
      return
    }

    if (path === '/checkin' && request.method() === 'POST') {
      const payload = postJson<{ qr_token: string }>(request)
      state.checkinRequests.push(payload.qr_token)
      const baseToken = payload.qr_token.split('|')[0]
      const ticket = state.tickets.find(item => item.qr_token === baseToken)
      if (!ticket) {
        await fulfillError(route, 404, '找不到此票券', 'NOT_FOUND')
        return
      }
      if (ticket.is_used) {
        await fulfillError(route, 409, '此票券已核銷', 'ALREADY_CHECKED_IN')
        return
      }

      ticket.is_used = true
      ticket.checked_in_at = '2099-06-12T02:30:00.000Z'
      await fulfillData(route, { ticket })
      return
    }

    if (path === '/checkins' && request.method() === 'GET') {
      await fulfillData(route, checkedInTickets(state).map(ticket => ({
        id: `checkin-${ticket.id}`,
        checked_at: ticket.checked_in_at,
        ticket,
      })))
      return
    }

    if (path === '/reports/overview' && request.method() === 'GET') {
      await fulfillData(route, overviewFor(state))
      return
    }

    if (path === `/reports/events/${state.event.id}/stats` && request.method() === 'GET') {
      await fulfillData(route, statsFor(state))
      return
    }

    await fulfillError(route, 404, `未處理的 API：${path}`, 'NOT_MOCKED')
  }
}

function createCrossRoleState(eventOverrides: Partial<CrossRoleEvent> = {}): CrossRoleState {
  return {
    event: {
      ...draftEvent,
      id: 'event-cross-role',
      title: '跨角色發布活動',
      description: 'manager 發布後 employee 才能看到並報名',
      venue: '台南園區',
      region_restriction: '台南',
      ticket_types: [
        {
          id: 'ticket-type-cross-role',
          name: '一般票',
          remaining: 5,
          total_quota: 5,
        },
      ],
      ...eventOverrides,
    },
    applications: [],
    tickets: [],
    cancelledTickets: [],
    applicationRequests: [],
    publishRequests: [],
    closeRequests: [],
    cancelRequests: [],
    checkinRequests: [],
  }
}

test.describe('跨角色互動 e2e', () => {
  test('manager 發布活動後，employee 重新整理即可報名並取得自動核准票券', async ({ browser, baseURL }) => {
    const state = createCrossRoleState()
    const apiHandler = buildCrossRoleHandler(state)
    const employeeContext = await browser.newContext({ baseURL })
    const managerContext = await browser.newContext({ baseURL })
    const employeePage = await employeeContext.newPage()
    const managerPage = await managerContext.newPage()

    await mockTotp(employeePage, '123456')
    await mockApi(employeePage, apiHandler)
    await mockApi(managerPage, apiHandler)

    try {
      await test.step('employee 先登入，草稿活動尚未出現在活動列表', async () => {
        await loginAs(employeePage, 'EMP001')
        await expect(employeePage.getByRole('heading', { name: '活動列表' })).toBeVisible()
        await expect(employeePage.getByText('目前沒有活動')).toBeVisible()
        await expect(employeePage.getByText(state.event.title)).toHaveCount(0)
      })

      await test.step('manager 另一個 session 登入並發布同一個活動', async () => {
        await loginAs(managerPage, 'MGR001')
        await managerPage.getByRole('link', { name: /活動管理/ }).click()
        await expect(managerPage.getByRole('heading', { name: '活動管理' })).toBeVisible()
        await expect(managerPage.getByText(state.event.title)).toBeVisible()

        await managerPage.getByRole('button', { name: /發布/ }).click()
        await expect.poll(() => state.publishRequests).toContain(`/events/${state.event.id}/publish`)
        await expect(managerPage.getByText('活動已發布！')).toBeVisible()
      })

      await test.step('employee 重新整理後看到已發布活動並送出申請', async () => {
        await employeePage.reload()
        await expect(employeePage.getByText(state.event.title)).toBeVisible()
        await employeePage.getByText(state.event.title).click()
        await expect(employeePage).toHaveURL(new RegExp(`/events/${state.event.id}$`))
        await expect(employeePage.locator('.badge-published', { hasText: '發布中' })).toBeVisible()

        await employeePage.getByRole('button', { name: '申請' }).click()
        const modal = employeePage.locator('.modal')
        await modal.locator('input[type="number"]').fill('2')
        await modal.getByRole('button', { name: '確認申請' }).click()

        await expect.poll(() => state.applicationRequests.length).toBe(1)
        expect(state.applicationRequests[0]).toMatchObject({
          event_id: state.event.id,
          ticket_type_id: 'ticket-type-cross-role',
          quantity: 2,
        })
        await expect(employeePage.getByText(/搶票成功.*自動發票/)).toBeVisible()
      })

      await test.step('employee 的我的票券顯示跨角色流程產生的自動核准票券', async () => {
        await employeePage.getByRole('link', { name: /我的票券/ }).click()
        await expect(employeePage.getByRole('heading', { name: /電子票券/ })).toBeVisible()
        await expect(employeePage.getByText(state.event.title).first()).toBeVisible()
        await expect(employeePage.getByText('已核准')).toBeVisible()
        await employeePage.getByRole('button', { name: '顯示 QR' }).first().click()
        await expect(employeePage.getByText('QR-CROSS-ROLE-001|123456')).toBeVisible()
      })
    } finally {
      await employeeContext.close()
      await managerContext.close()
    }
  })

  test('employee 報名與 manager 核銷後，HR 重新整理報表會看到統計同步更新', async ({ browser, baseURL }) => {
    const state = createCrossRoleState({
      id: 'event-cross-role-hr',
      title: '跨角色 HR 報表活動',
      description: 'employee 報名、manager 核銷後，HR 報表要反映最新統計',
      status: 'published',
    })
    const apiHandler = buildCrossRoleHandler(state)
    const employeeContext = await browser.newContext({ baseURL })
    const managerContext = await browser.newContext({ baseURL })
    const hrContext = await browser.newContext({ baseURL })
    const employeePage = await employeeContext.newPage()
    const managerPage = await managerContext.newPage()
    const hrPage = await hrContext.newPage()

    await mockTotp(employeePage, '123456')
    await mockApi(employeePage, apiHandler)
    await mockApi(managerPage, apiHandler)
    await mockApi(hrPage, apiHandler)

    try {
      await test.step('HR 先登入報表，確認活動目前尚無申請與核銷資料', async () => {
        await loginAs(hrPage, 'HR001')
        await hrPage.getByRole('link', { name: /統計報表/ }).click()
        await expect(hrPage.getByRole('heading', { name: '統計報表' })).toBeVisible()

        const overviewRow = hrPage.getByRole('row', { name: new RegExp(state.event.title) })
        await expect(overviewRow.locator('td').nth(1)).toHaveText('0')
        await expect(overviewRow.locator('td').nth(2)).toHaveText('0')
        await expect(overviewRow.locator('td').nth(3)).toHaveText('0')
        await expect(overviewRow.locator('td').nth(6)).toHaveText('0')
        await expect(overviewRow.locator('td').nth(7)).toContainText('0%')
      })

      await test.step('employee 另一個 session 申請活動並立即取得自動核准票券', async () => {
        await loginAs(employeePage, 'EMP001')
        await expect(employeePage.getByText(state.event.title)).toBeVisible()
        await employeePage.getByText(state.event.title).click()
        await employeePage.getByRole('button', { name: '申請' }).click()
        await employeePage.locator('.modal').getByRole('button', { name: '確認申請' }).click()

        await expect.poll(() => state.applicationRequests.length).toBe(1)
        await expect(employeePage.getByText(/搶票成功.*自動發票/)).toBeVisible()
      })

      await test.step('HR 重新整理後看到申請與核准統計已更新，但尚未核銷', async () => {
        await hrPage.reload()
        const overviewRow = hrPage.getByRole('row', { name: new RegExp(state.event.title) })
        await expect(overviewRow.locator('td').nth(1)).toHaveText('1')
        await expect(overviewRow.locator('td').nth(2)).toHaveText('1')
        await expect(overviewRow.locator('td').nth(3)).toHaveText('1')
        await expect(overviewRow.locator('td').nth(5)).toHaveText('1')
        await expect(overviewRow.locator('td').nth(6)).toHaveText('0')
        await expect(overviewRow.locator('td').nth(7)).toContainText('0%')

        await hrPage.locator('select').selectOption(state.event.id)
        await expect(hrPage.getByText('1. 申請階段')).toBeVisible()
        await expect(hrPage.getByText('2. 核准與退票階段')).toBeVisible()
        await expect(hrPage.getByText('3. 核銷與出席率')).toBeVisible()
      })

      await test.step('manager 用 employee 顯示的動態 QR token 完成核銷', async () => {
        await employeePage.getByRole('link', { name: /我的票券/ }).click()
        await employeePage.getByRole('button', { name: '顯示 QR' }).click()
        await expect(employeePage.getByText('QR-CROSS-ROLE-001|123456')).toBeVisible()

        await loginAs(managerPage, 'MGR001')
        await managerPage.getByRole('link', { name: /現場核銷/ }).click()
        await managerPage.locator('#qr-token-input').fill('QR-CROSS-ROLE-001|123456')
        await managerPage.getByRole('button', { name: '確認核銷' }).click()

        await expect.poll(() => state.checkinRequests).toContain('QR-CROSS-ROLE-001|123456')
        await expect(managerPage.getByText(/核銷成功/)).toBeVisible()
      })

      await test.step('employee 與 HR 重新整理後都看到核銷後狀態', async () => {
        await employeePage.reload()
        await expect(employeePage.getByText('已核銷')).toBeVisible()
        await expect(employeePage.getByRole('button', { name: '顯示 QR' })).toHaveCount(0)

        await hrPage.reload()
        const overviewRow = hrPage.getByRole('row', { name: new RegExp(state.event.title) })
        await expect(overviewRow.locator('td').nth(6)).toHaveText('1')
        await expect(overviewRow.locator('td').nth(7)).toContainText('100%')

        await hrPage.locator('select').selectOption(state.event.id)
        await expect(hrPage.getByText('核銷率 (基於有效票)')).toBeVisible()
        await expect(hrPage.getByText('100%').first()).toBeVisible()
      })
    } finally {
      await employeeContext.close()
      await managerContext.close()
      await hrContext.close()
    }
  })

  test('manager 截止活動後，employee 重新整理詳情頁不能再送出申請', async ({ browser, baseURL }) => {
    const state = createCrossRoleState({
      id: 'event-cross-role-close',
      title: '跨角色截止活動',
      description: 'employee 停在詳情頁時 manager 截止活動',
      status: 'published',
    })
    const apiHandler = buildCrossRoleHandler(state)
    const employeeContext = await browser.newContext({ baseURL })
    const managerContext = await browser.newContext({ baseURL })
    const employeePage = await employeeContext.newPage()
    const managerPage = await managerContext.newPage()

    await mockApi(employeePage, apiHandler)
    await mockApi(managerPage, apiHandler)

    try {
      await test.step('employee 先登入並停在可申請的活動詳情頁', async () => {
        await loginAs(employeePage, 'EMP001')
        await employeePage.getByText(state.event.title).click()
        await expect(employeePage).toHaveURL(new RegExp(`/events/${state.event.id}$`))
        await expect(employeePage.getByRole('button', { name: '申請' })).toBeVisible()
      })

      await test.step('manager 另一個 session 截止同一個活動', async () => {
        await loginAs(managerPage, 'MGR001')
        await managerPage.getByRole('link', { name: /活動管理/ }).click()
        await expect(managerPage.getByText(state.event.title)).toBeVisible()
        await managerPage.getByRole('button', { name: /截止/ }).click()

        await expect.poll(() => state.closeRequests).toContain(`/events/${state.event.id}/close`)
        await expect(managerPage.getByText('已截止報名')).toBeVisible()
      })

      await test.step('employee 重新整理詳情頁後看到已截止且沒有申請入口', async () => {
        await employeePage.reload()
        await expect(employeePage.locator('.badge-closed', { hasText: '已截止' })).toBeVisible()
        await expect(employeePage.getByRole('button', { name: '申請' })).toHaveCount(0)
        await expect(employeePage.getByText('此活動目前不開放申請')).toBeVisible()
      })
    } finally {
      await employeeContext.close()
      await managerContext.close()
    }
  })

  test('employee 退票後，HR 重新整理報表會看到有效票下降與退票數增加', async ({ browser, baseURL }) => {
    const state = createCrossRoleState({
      id: 'event-cross-role-cancel',
      title: '跨角色退票報表活動',
      description: 'employee 退票後 HR 報表要反映有效票和退票數',
      status: 'published',
    })
    const apiHandler = buildCrossRoleHandler(state)
    const employeeContext = await browser.newContext({ baseURL })
    const hrContext = await browser.newContext({ baseURL })
    const employeePage = await employeeContext.newPage()
    const hrPage = await hrContext.newPage()

    await mockTotp(employeePage, '123456')
    await mockApi(employeePage, apiHandler)
    await mockApi(hrPage, apiHandler)

    try {
      await test.step('employee 報名並在我的票券看到可退票票券', async () => {
        await loginAs(employeePage, 'EMP001')
        await employeePage.getByText(state.event.title).click()
        await employeePage.getByRole('button', { name: '申請' }).click()
        await employeePage.locator('.modal').getByRole('button', { name: '確認申請' }).click()
        await expect.poll(() => state.applicationRequests.length).toBe(1)

        await employeePage.getByRole('link', { name: /我的票券/ }).click()
        await expect(employeePage.getByRole('heading', { name: /電子票券/ })).toBeVisible()
        await expect(employeePage.getByRole('button', { name: '退票' })).toBeVisible()
      })

      await test.step('HR 登入後先看到有效票為 1、退票數為 0', async () => {
        await loginAs(hrPage, 'HR001')
        await hrPage.getByRole('link', { name: /統計報表/ }).click()
        const overviewRow = hrPage.getByRole('row', { name: new RegExp(state.event.title) })
        await expect(overviewRow.locator('td').nth(3)).toHaveText('1')
        await expect(overviewRow.locator('td').nth(4)).toHaveText('0')
        await expect(overviewRow.locator('td').nth(5)).toHaveText('1')
      })

      await test.step('employee 確認退票，票券列表與申請狀態同步更新', async () => {
        employeePage.once('dialog', dialog => dialog.accept())
        await employeePage.getByRole('button', { name: '退票' }).click()

        await expect.poll(() => state.cancelRequests).toContain('/tickets/ticket-cross-role-approved-1/cancel')
        await expect(employeePage.getByText('已取消')).toBeVisible()
        await expect(employeePage.getByRole('heading', { name: /電子票券/ })).toHaveCount(0)
        await expect(employeePage.getByRole('button', { name: '退票' })).toHaveCount(0)
      })

      await test.step('HR 重新整理後看到退票數增加且目前有效票歸零', async () => {
        await hrPage.reload()
        const overviewRow = hrPage.getByRole('row', { name: new RegExp(state.event.title) })
        await expect(overviewRow.locator('td').nth(3)).toHaveText('1')
        await expect(overviewRow.locator('td').nth(4)).toHaveText('1')
        await expect(overviewRow.locator('td').nth(5)).toHaveText('0')
        await expect(overviewRow.locator('td').nth(6)).toHaveText('0')
        await expect(overviewRow.locator('td').nth(7)).toContainText('0%')
      })
    } finally {
      await employeeContext.close()
      await hrContext.close()
    }
  })
})
