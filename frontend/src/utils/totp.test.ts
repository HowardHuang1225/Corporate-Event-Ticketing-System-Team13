import { afterEach, describe, expect, it, vi } from 'vitest'


describe('generateTOTP', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('generates a six digit OTP using the default 15 second window', async () => {
    const importKey = vi.fn().mockResolvedValue({ kind: 'hmac-key' })
    const sign = vi.fn().mockResolvedValue(new Uint8Array([
      0, 0, 0, 1,
      0, 0, 0, 0,
      0, 0, 0, 0,
      0, 0, 0, 0,
      0, 0, 0, 0,
    ]).buffer)

    vi.stubGlobal('crypto', {
      subtle: { importKey, sign },
    })
    vi.spyOn(Date, 'now').mockReturnValue(0)

    const { generateTOTP } = await vi.importActual<typeof import('./totp')>('./totp')
    await expect(generateTOTP('test-secret')).resolves.toBe('000001')

    expect(importKey).toHaveBeenCalledWith(
      'raw',
      new TextEncoder().encode('test-secret'),
      { name: 'HMAC', hash: 'SHA-256' },
      false,
      ['sign'],
    )
    expect(sign).toHaveBeenCalled()
  })

  it('uses the provided window to calculate the counter', async () => {
    const importKey = vi.fn().mockResolvedValue({ kind: 'hmac-key' })
    const sign = vi.fn().mockResolvedValue(new Uint8Array([
      0, 0, 0, 42,
      0, 0, 0, 0,
      0, 0, 0, 0,
      0, 0, 0, 0,
      0, 0, 0, 0,
    ]).buffer)

    vi.stubGlobal('crypto', {
      subtle: { importKey, sign },
    })
    vi.spyOn(Date, 'now').mockReturnValue(60_000)

    const { generateTOTP } = await vi.importActual<typeof import('./totp')>('./totp')
    await expect(generateTOTP('test-secret', 60)).resolves.toBe('000042')

    const counterBytes = sign.mock.calls[0][2] as Uint8Array
    expect(new DataView(counterBytes.buffer).getBigUint64(0, false)).toBe(1n)
  })
})