import { describe, expect, it } from 'vitest'

import { pricingFamilies, pricingGroups } from '../pricing-data'

describe('Vote AI static pricing data', () => {
  it('keeps model identifiers unique', () => {
    const ids = pricingFamilies.flatMap((family) => family.models.map((model) => model.id))
    expect(new Set(ids).size).toBe(ids.length)
  })

  it('keeps the published group multipliers stable in both locales', () => {
    expect(pricingGroups.zh.map((group) => group.multiplier)).toEqual([0.05, 0.12, 0.2])
    expect(pricingGroups.en.map((group) => group.multiplier)).toEqual([0.05, 0.12, 0.2])
    expect(pricingGroups.zh.map((group) => group.id)).toEqual(pricingGroups.en.map((group) => group.id))
  })
})
