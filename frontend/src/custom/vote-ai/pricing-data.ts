export type FamilyId = 'codex'
export type GroupId = 'special' | 'plus' | 'pro'
export type PriceColumn = 'input' | 'output' | 'cacheRead'
export type PriceModel = { id: string } & Partial<Record<PriceColumn, number>>

export interface PricingFamily {
  id: FamilyId
  name: string
  mark: string
  columns: PriceColumn[]
  models: PriceModel[]
}

export interface PricingGroup {
  id: GroupId
  name: string
  multiplier: number
  badge: string
  summary: string
  description: string
}

export const pricingFamilies: PricingFamily[] = [
  {
    id: 'codex',
    name: 'Codex',
    mark: '◎',
    columns: ['input', 'output', 'cacheRead'],
    models: [
      { id: 'gpt-5.6-sol', input: 5, output: 30, cacheRead: 0.5 },
      { id: 'gpt-5.6-terra', input: 2.5, output: 15, cacheRead: 0.25 },
      { id: 'gpt-5.6-luna', input: 1, output: 6, cacheRead: 0.1 },
      { id: 'gpt-5.5', input: 5, output: 30, cacheRead: 0.5 },
      { id: 'gpt-5.4', input: 2.5, output: 15, cacheRead: 0.25 },
      { id: 'gpt-5.4-mini', input: 0.75, output: 4.5, cacheRead: 0.075 }
    ]
  }
]

export const pricingGroups: Record<'zh' | 'en', PricingGroup[]> = {
  zh: [
    { id: 'special', name: '特惠', multiplier: 0.05, badge: '0.5 折', summary: '官方 API 价格 × 0.05', description: '特惠分组，按官方 API 价格的 5% 计算。' },
    { id: 'plus', name: 'Plus', multiplier: 0.12, badge: '1.2 折', summary: '官方 API 价格 × 0.12', description: 'Plus 分组，按官方 API 价格的 12% 计算。' },
    { id: 'pro', name: 'Pro', multiplier: 0.2, badge: '2 折', summary: '官方 API 价格 × 0.2', description: 'Pro 分组，按官方 API 价格的 20% 计算。' }
  ],
  en: [
    { id: 'special', name: 'Special', multiplier: 0.05, badge: '5%', summary: 'Official API price × 0.05', description: 'Special group pricing is calculated at 5% of the official API price.' },
    { id: 'plus', name: 'Plus', multiplier: 0.12, badge: '12%', summary: 'Official API price × 0.12', description: 'Plus group pricing is calculated at 12% of the official API price.' },
    { id: 'pro', name: 'Pro', multiplier: 0.2, badge: '20%', summary: 'Official API price × 0.2', description: 'Pro group pricing is calculated at 20% of the official API price.' }
  ]
}
