import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const sourcePath = join(dirname(fileURLToPath(import.meta.url)), 'EventManage.tsx')
const source = readFileSync(sourcePath, 'utf8')

describe('EventManage 建立表單的 publish_time 契約', () => {
  it('EMPTY_FORM 會初始化 publish_time 欄位', () => {
    console.info('確認 EventManage EMPTY_FORM 包含 publish_time')

    expect(/const EMPTY_FORM\s*=\s*\{[\s\S]*publish_time\s*:/.test(source)).toBe(true)
  })

  it('建立/編輯 modal 會渲染綁定 publish_time 的 datetime-local input', () => {
    console.info('確認 EventManage modal 渲染 publish_time datetime-local input')

    expect(/<input[^>]*type="datetime-local"[^>]*value=\{form\.publish_time\}/.test(source)).toBe(true)
    expect(source.includes('publish_time: e.target.value')).toBe(true)
  })

  it('送出 payload 時會把 publish_time 序列化成 ISO 字串', () => {
    console.info('確認 EventManage payload 會送出 ISO 格式 publish_time')

    expect(
      /publish_time\s*:\s*form\.publish_time\s*\?\s*new Date\(form\.publish_time\)\.toISOString\(\)\s*:\s*''/.test(source),
    ).toBe(true)
  })
})
