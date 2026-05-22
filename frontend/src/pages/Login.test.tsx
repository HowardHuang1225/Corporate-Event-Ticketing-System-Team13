import { describe, expect, it, beforeEach, vi, type Mock } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import Login from './Login'
import { useAuth } from '../contexts/AuthContext'
import { renderWithQueryClient } from '../test/test-utils'

vi.mock('../contexts/AuthContext', () => ({
  useAuth: vi.fn(),
}))

const mockUseAuth = useAuth as Mock

function renderLogin(login = vi.fn()) {
  mockUseAuth.mockReturnValue({
    user: null,
    isLoading: false,
    login,
    logout: vi.fn(),
  })

  const result = renderWithQueryClient(
    <MemoryRouter initialEntries={['/login']}>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="/events" element={<div>活動頁</div>} />
      </Routes>
    </MemoryRouter>,
  )

  const employeeInput = result.container.querySelector<HTMLInputElement>('#employee-id')
  const passwordInput = result.container.querySelector<HTMLInputElement>('#password')

  if (!employeeInput || !passwordInput) {
    throw new Error('登入表單欄位沒有正確渲染')
  }

  return { ...result, employeeInput, passwordInput }
}

describe('Login', () => {
  beforeEach(() => {
    mockUseAuth.mockReset()
  })

  it('送出帳密後會呼叫 login 並導向活動頁', async () => {
    console.info('確認登入成功流程')
    const login = vi.fn().mockResolvedValue(undefined)
    const { employeeInput, passwordInput } = renderLogin(login)

    await userEvent.type(employeeInput, 'EMP001')
    await userEvent.type(passwordInput, 'password')
    await userEvent.click(screen.getByRole('button', { name: '登入' }))

    await waitFor(() => {
      expect(login).toHaveBeenCalledWith('EMP001', 'password')
    })
    expect(screen.getByText('活動頁')).toBeInTheDocument()
  })

  it('login 失敗時會顯示後端回傳的錯誤訊息', async () => {
    console.info('確認登入失敗錯誤訊息')
    const login = vi.fn().mockRejectedValue({
      response: {
        data: {
          error: {
            message: '帳號或密碼錯誤',
          },
        },
      },
    })
    const { employeeInput, passwordInput } = renderLogin(login)

    await userEvent.type(employeeInput, 'EMP001')
    await userEvent.type(passwordInput, 'wrong-password')
    await userEvent.click(screen.getByRole('button', { name: '登入' }))

    expect(await screen.findByText('帳號或密碼錯誤')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '登入' })).toBeEnabled()
  })

  it('login 失敗且後端沒有 message 時會顯示預設錯誤訊息', async () => {
    console.info('確認登入失敗 fallback 錯誤訊息')
    const login = vi.fn().mockRejectedValue(new Error('network error'))
    const { employeeInput, passwordInput } = renderLogin(login)

    await userEvent.type(employeeInput, 'EMP001')
    await userEvent.type(passwordInput, 'wrong-password')
    await userEvent.click(screen.getByRole('button', { name: '登入' }))

    expect(await screen.findByText('登入失敗，請確認帳號密碼')).toBeInTheDocument()
  })
})
