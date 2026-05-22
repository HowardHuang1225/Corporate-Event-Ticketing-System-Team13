import { beforeEach, describe, expect, it, vi } from 'vitest'
import api from './client'

function requestInterceptor() {
  return (api.interceptors.request as any).handlers[0].fulfilled
}

function responseErrorInterceptor() {
  return (api.interceptors.response as any).handlers[0].rejected
}

function responseSuccessInterceptor() {
  return (api.interceptors.response as any).handlers[0].fulfilled
}

describe('api client', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('預設會把請求送到 /v1 API 前綴', () => {
    console.info('確認 axios client 的 baseURL')

    expect(api.defaults.baseURL).toBe('/v1')
  })

  it('localStorage 有 token 時會加上 Authorization header', () => {
    console.info('確認 request interceptor 會帶入 bearer token')
    localStorage.setItem('token', 'access-token')

    const config = requestInterceptor()({ headers: {} })

    expect(config.headers.Authorization).toBe('Bearer access-token')
  })

  it('localStorage 沒有 token 時不會加上 Authorization header', () => {
    console.info('確認沒有 token 時 request interceptor 不修改授權 header')

    const config = requestInterceptor()({ headers: {} })

    expect(config.headers.Authorization).toBeUndefined()
  })

  it('收到 401 response 時會清除 token 並導向登入頁', async () => {
    console.info('確認 response interceptor 處理 401 未授權錯誤')
    localStorage.setItem('token', 'expired-token')
    const fakeWindow = { location: { href: '/' } }
    vi.stubGlobal('window', fakeWindow)
    const error = { response: { status: 401 } }

    await expect(responseErrorInterceptor()(error)).rejects.toBe(error)

    expect(localStorage.getItem('token')).toBeNull()
    expect(fakeWindow.location.href).toBe('/login')
  })

  it('成功 response 會原樣回傳', () => {
    console.info('確認 response success interceptor 不改動成功回應')
    const response = { data: { data: 'ok' } }

    expect(responseSuccessInterceptor()(response)).toBe(response)
  })

  it('非 401 response error 不會清除 token 或導向登入頁', async () => {
    console.info('確認 response interceptor 不處理非 401 錯誤')
    localStorage.setItem('token', 'still-valid-token')
    const fakeWindow = { location: { href: '/' } }
    vi.stubGlobal('window', fakeWindow)
    const error = { response: { status: 500 } }

    await expect(responseErrorInterceptor()(error)).rejects.toBe(error)

    expect(localStorage.getItem('token')).toBe('still-valid-token')
    expect(fakeWindow.location.href).toBe('/')
  })
})
