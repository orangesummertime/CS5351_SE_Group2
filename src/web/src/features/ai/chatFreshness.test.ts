import { describe, expect, it } from 'vitest'
import { citationDisplayStatus, isDisplayedAsCurrentlyValid, versionStatusLabel } from './chatFreshness'

describe('chat freshness presentation (FR-05 / NFR-05)', () => {
  it('labels current, expired and unavailable states', () => {
    expect(versionStatusLabel('current')).toBe('当前有效')
    expect(versionStatusLabel('expired')).toBe('已过期')
    expect(versionStatusLabel('unavailable')).toBe('来源不可用')
  })

  it('does not treat unknown or verified-only labels as currently valid', () => {
    expect(isDisplayedAsCurrentlyValid('current')).toBe(true)
    expect(isDisplayedAsCurrentlyValid('expired')).toBe(false)
    expect(isDisplayedAsCurrentlyValid('unavailable')).toBe(false)
    expect(isDisplayedAsCurrentlyValid('verified')).toBe(false)
    expect(isDisplayedAsCurrentlyValid(undefined)).toBe(false)
    expect(versionStatusLabel('verified')).toBe('')
  })

  it('lets freshness override a generation-time verified status after a source change', () => {
    expect(citationDisplayStatus({ status: 'verified', freshness: 'expired' })).toBe('expired')
    expect(isDisplayedAsCurrentlyValid(citationDisplayStatus({ status: 'verified', freshness: 'expired' }))).toBe(false)
    expect(citationDisplayStatus({ status: 'verified', freshness: 'unavailable' })).toBe('unavailable')
    expect(citationDisplayStatus({ status: 'verified', freshness: 'current' })).toBe('current')
  })
})
