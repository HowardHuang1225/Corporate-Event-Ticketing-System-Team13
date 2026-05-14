import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const sourcePath = join(dirname(fileURLToPath(import.meta.url)), 'EventManage.tsx')
const source = readFileSync(sourcePath, 'utf8')

test('EventManage create form publish_time contract', async (t) => {
  await t.test('form state stores publish_time', () => {
    console.info('checking EventManage EMPTY_FORM includes publish_time')
    assert.ok(
      /const EMPTY_FORM\s*=\s*\{[\s\S]*publish_time\s*:/.test(source),
      'EMPTY_FORM should initialize publish_time for the scheduled publish field',
    )
  })

  await t.test('modal renders a publish_time datetime input', () => {
    console.info('checking EventManage modal renders a publish_time datetime-local input')
    assert.ok(
      /<input[^>]*type="datetime-local"[^>]*value=\{form\.publish_time\}/.test(source),
      'the create modal should bind a datetime-local input to form.publish_time',
    )
    assert.ok(
      source.includes('publish_time: e.target.value'),
      'the publish_time input should update form.publish_time',
    )
  })

  await t.test('create payload serializes publish_time', () => {
    console.info('checking EventManage create payload sends publish_time')
    assert.ok(
      /publish_time\s*:\s*form\.publish_time\s*\?\s*new Date\(form\.publish_time\)\.toISOString\(\)\s*:\s*''/.test(source),
      'handleCreate should send publish_time as an ISO timestamp',
    )
  })
})
