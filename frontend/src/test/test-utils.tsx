import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, type RenderOptions } from '@testing-library/react'
import type { ReactElement, ReactNode } from 'react'
import { appQueryClient } from '../lib/queryClient'

/** 整合測試共用 App 的 QueryClient，並行跑測試前需清空避免殘留 loading 狀態 */
export function resetAppQueryClient() {
  appQueryClient.clear()
}

/** 一次填入欄位，避免 userEvent.type 在 datetime-local 上過慢或打亂焦點 */
export function fillField(element: HTMLElement, value: string) {
  fireEvent.change(element, { target: { value } })
}

export function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
}

export function renderWithQueryClient(
  ui: ReactElement,
  options: RenderOptions & { queryClient?: QueryClient } = {},
) {
  const { queryClient = createTestQueryClient(), ...renderOptions } = options

  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }

  return {
    queryClient,
    ...render(ui, { wrapper: Wrapper, ...renderOptions }),
  }
}
