import type { Page, Request, Route } from '@playwright/test'

type ApiHandlerArgs = {
  route: Route
  request: Request
  url: URL
  path: string
}

export type ApiHandler = (args: ApiHandlerArgs) => Promise<void> | void

export const users = {
  employee: {
    id: 'employee-e2e-1',
    employee_id: 'EMP001',
    name: '員工小明',
    email: 'employee.e2e@example.com',
    department: '資訊部',
    region: '台南',
    role: 'employee',
  },
  manager: {
    id: 'manager-e2e-1',
    employee_id: 'MGR001',
    name: '活動管理員',
    email: 'manager.e2e@example.com',
    department: '行政部',
    region: '台北',
    role: 'event_manager',
  },
  hr: {
    id: 'hr-e2e-1',
    employee_id: 'HR001',
    name: '人資同仁',
    email: 'hr.e2e@example.com',
    department: '人資部',
    region: '台北',
    role: 'hr',
  },
} as const

export const familyEvent = {
  id: 'event-family-day',
  title: '家庭同樂日',
  description: '員工與眷屬一起參與的活動',
  venue: '台南園區',
  status: 'published',
  publish_time: '2099-06-01T01:00:00.000Z',
  start_time: '2099-07-01T01:00:00.000Z',
  end_time: '2099-07-01T09:00:00.000Z',
  apply_deadline: '2099-06-20T09:00:00.000Z',
  region_restriction: '台南',
  max_tickets_per_person: 3,
  ticket_types: [
    {
      id: 'ticket-type-general',
      name: '一般票',
      remaining: 8,
      total_quota: 30,
    },
  ],
}

export const draftEvent = {
  id: 'event-draft-e2e',
  title: '可發布草稿活動',
  description: '等待發布的活動',
  venue: '台北總部',
  status: 'draft',
  publish_time: '2099-06-01T01:00:00.000Z',
  start_time: '2099-07-01T01:00:00.000Z',
  end_time: '2099-07-01T09:00:00.000Z',
  apply_deadline: '2099-06-20T09:00:00.000Z',
  region_restriction: '台北',
  max_tickets_per_person: 2,
  ticket_types: [
    {
      id: 'ticket-type-draft',
      name: '一般票',
      remaining: 20,
      total_quota: 20,
    },
  ],
}

export const publishedManagerEvent = {
  ...draftEvent,
  id: 'event-published-e2e',
  title: '可截止已發布活動',
  status: 'published',
}

export const approvedApplication = {
  id: 'application-auto-approved-e2e',
  event: { title: familyEvent.title },
  ticket_type: { name: '一般票' },
  quantity: 2,
  status: 'approved',
  applied_at: '2099-06-10T01:00:00.000Z',
  reason: null,
}

export const approvedTicket = {
  id: 'ticket-auto-approved-e2e',
  event: { title: familyEvent.title },
  ticket_type: { name: '一般票' },
  qr_token: 'QR-AUTO-APPROVED-001',
  expires_at: '2099-07-01T09:00:00.000Z',
  is_used: false,
}

export function postJson<T>(request: Request): T {
  const raw = request.postData()
  return (raw ? JSON.parse(raw) : {}) as T
}

export async function mockApi(page: Page, handler: ApiHandler) {
  await page.route('**/v1/**', async route => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/v1/, '') || '/'

    await handler({ route, request, url, path })
  })
}

export async function mockTotp(page: Page, otp = '123456') {
  await page.route('**/src/utils/totp.ts*', route => route.fulfill({
    status: 200,
    contentType: 'application/javascript',
    body: `export async function generateTOTP() { return '${otp}' }\n`,
  }))
}

export async function fulfillData(route: Route, data: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify({ data }),
  })
}

export async function fulfillError(route: Route, status: number, message: string, code = 'ERROR') {
  await route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify({ error: { code, message } }),
  })
}

export function eventsByStatus(events: Array<{ status: string }>, url: URL) {
  const status = url.searchParams.get('status')
  return status ? events.filter(event => event.status === status) : events
}

export async function loginAs(page: Page, employeeId: string, password = 'password') {
  await page.goto('/login')
  await page.locator('#employee-id').fill(employeeId)
  await page.locator('#password').fill(password)
  await page.locator('#login-btn').click()
}
