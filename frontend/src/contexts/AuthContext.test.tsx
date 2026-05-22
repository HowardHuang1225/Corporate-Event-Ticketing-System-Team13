import { describe, expect, it } from 'vitest'
import { render } from '@testing-library/react'
import { useAuth } from './AuthContext'

function BrokenConsumer() {
  useAuth()
  return <div>不會渲染</div>
}

describe('useAuth', () => {
  it('不在 AuthProvider 內使用時會拋出錯誤', () => {
    console.info('確認 useAuth 必須包在 AuthProvider 內')

    expect(() => render(<BrokenConsumer />)).toThrow('useAuth must be inside AuthProvider')
  })
})
