import type { Page } from '@playwright/test'

const asOf = '2026-11-04T12:00:00Z'
const windowStart = '2025-11-04T12:00:00Z'

export function offerView({
  id,
  supplier = 1,
  name,
  amount = '110.00',
  reference = '100.00',
  currency = 'EUR',
  variation = '10.0',
  trend = 'up',
}: {
  id: number
  supplier?: number
  name: string
  amount?: string
  reference?: string
  currency?: string
  variation?: string
  trend?: string
}) {
  const price = (value: string) => ({ amount: value, currency })
  const quote = (quoteID: number, value: string, quotedAt: string) => ({
    id: quoteID, offer_id: id, price: price(value), source: 'e2e-fixture', quoted_at: quotedAt,
  })
  return {
    offer: {
      id, part_id: id, supplier_id: supplier, currency,
      price_min: price('0.0001'), price_max: price('999999999999999999999999.99'),
      lead_time_days: 3, origin_country: 'FR', quality_score: '4.25', archived_at: null,
    },
    part: {
      id, name, category: 'moteur', zone: 'moteur', mass_kg: '1.25', co2_kg_per_unit: '2.75',
      recycled_percent: '25', recyclability_percent: '95', manufacturing_cost: price('1.00'),
      production_days: 1, labor_hours: '0.5', hourly_rate: price('10.00'), archived_at: null,
    },
    statistics: {
      offer_id: id, as_of: asOf, window_start: windowStart,
      current_quote: quote(id * 10 + 3, amount, asOf),
      reference_quote: quote(id * 10 + 1, reference, '2025-11-03T12:00:00Z'),
      period_start: '2025-11-03T12:00:00Z', period_end: asOf,
      minimum: price(trend === 'down' ? amount : reference),
      maximum: price(trend === 'down' ? reference : amount),
      variation_percent: variation, trend,
      history: [quote(id * 10 + 2, reference, windowStart), quote(id * 10 + 3, amount, asOf)],
    },
  }
}

export type OfferView = ReturnType<typeof offerView>
export type OfferPage = { as_of: string | null; offers: Array<OfferView | {
  offer: OfferView['offer']; part: OfferView['part']; statistics: null; error: string
}> }

export const suppliers = [
  { id: 1, name: 'Atlas', quality_score: '4.25', trust_score: '4.5' },
  { id: 2, name: 'Boréal', quality_score: '4.75', trust_score: '4.25' },
]

export function defaultPages(): Record<string, OfferPage> {
  const missing = offerView({ id: 104, name: 'Sans cotation Atlas' })
  return {
    '1': { as_of: asOf, offers: [
      offerView({ id: 101, name: 'Injecteur Atlas' }),
      offerView({ id: 102, name: 'Filtre Atlas', amount: '90.00', variation: '-10.0', trend: 'down' }),
      offerView({ id: 103, name: 'Capteur Atlas', amount: '100.00', variation: '0.0', trend: 'stable' }),
      { offer: missing.offer, part: missing.part, statistics: null, error: 'Aucune cotation disponible' },
    ] },
    '2': { as_of: asOf, offers: [offerView({ id: 201, supplier: 2, name: 'Moteur Boréal', amount: '100.00', variation: '0.0', trend: 'stable' })] },
  }
}

type SseControl = {
  emit(type: string, data?: string): void
  disconnect(): void
  active: number
  created: number
  closed: number
}

declare global {
  interface Window {
    __specDrumSse: SseControl
  }
}

// Mock installé avant React : contrôle du signal, aucune connexion à un autre service local.
export async function mockEventSource(page: Page) {
  await page.addInitScript(() => {
    const sources: ControlledEventSource[] = []
    let closed = 0
    class ControlledEventSource extends EventTarget {
      static CONNECTING = 0
      static OPEN = 1
      static CLOSED = 2
      CONNECTING = 0
      OPEN = 1
      CLOSED = 2
      readyState = 1
      url: string
      withCredentials = false
      onmessage: ((event: MessageEvent) => void) | null = null
      onerror: ((event: Event) => void) | null = null
      onopen: ((event: Event) => void) | null = null
      constructor(url: string | URL) {
        super()
        this.url = String(url)
        sources.push(this)
        queueMicrotask(() => {
          if (this.readyState !== this.CLOSED) {
            const event = new Event('open')
            this.dispatchEvent(event)
            this.onopen?.(event)
          }
        })
      }
      close() {
        if (this.readyState !== this.CLOSED) {
          closed++
          this.readyState = this.CLOSED
        }
      }
    }
    Object.defineProperty(window, 'EventSource', { configurable: true, value: ControlledEventSource })
    window.__specDrumSse = {
      emit(type, data = '{}') {
        for (const source of sources) {
          if (source.readyState === source.CLOSED) continue
          if (type === 'open') {
            source.readyState = source.OPEN
            const event = new Event('open')
            source.dispatchEvent(event)
            source.onopen?.(event)
            continue
          }
          const event = new MessageEvent(type, { data, lastEventId: '42' })
          source.dispatchEvent(event)
          if (type === 'message') source.onmessage?.(event)
        }
      },
      disconnect() {
        for (const source of sources) {
          if (source.readyState === source.CLOSED) continue
          source.readyState = source.CONNECTING
          const event = new Event('error')
          source.dispatchEvent(event)
          source.onerror?.(event)
        }
      },
      get active() { return sources.filter((source) => source.readyState !== source.CLOSED).length },
      get created() { return sources.length },
      get closed() { return closed },
    }
  })
}

export async function mockSupplierAPI(page: Page, pages = defaultPages()) {
  const calls: Record<string, number> = {}
  const holds: Record<string, Promise<void> | undefined> = {}
  const release: Record<string, (() => void) | undefined> = {}
  const seen: Record<string, Promise<void> | undefined> = {}
  const markSeen: Record<string, (() => void) | undefined> = {}
  const settled: Record<string, Promise<void> | undefined> = {}
  const markSettled: Record<string, (() => void) | undefined> = {}
  await page.route('**/healthz', (route) => route.fulfill({ json: { status: 'ok' } }))
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/suppliers') {
      await route.fulfill({ json: suppliers })
      return
    }
    const match = path.match(/^\/api\/v1\/suppliers\/(\d+)\/(offers|contracts)$/)
    if (match) {
      const [, id, resource] = match
      if (resource === 'contracts') {
        await route.fulfill({ json: [{
          id: Number(id) * 10, supplier_id: Number(id), starts_on: '2026-01-01', ends_on: '2026-12-31',
          conditions: 'Remise sur toute la quantité', offer_ids: pages[id]?.offers.map((view) => view.offer.id) ?? [],
          tiers: [{ min_quantity: '500', discount_percent: '5' }],
        }] })
        return
      }
      calls[id] = (calls[id] ?? 0) + 1
      const snapshot = structuredClone(pages[id] ?? { as_of: asOf, offers: [] })
      markSeen[id]?.()
      if (holds[id]) await holds[id]
      // Un AbortController côté application peut avoir annulé la requête retenue.
      try {
        await route.fulfill({ json: snapshot })
      } catch (error) {
        if (!route.request().failure()) throw error
      } finally {
        markSettled[id]?.()
      }
      return
    }
    await route.fulfill({ status: 404, json: { error: `Fixture API absente : ${path}` } })
  })
  return {
    calls,
    pages,
    hold(id: string) {
      holds[id] = new Promise<void>((resolve) => { release[id] = resolve })
      seen[id] = new Promise<void>((resolve) => { markSeen[id] = resolve })
      settled[id] = new Promise<void>((resolve) => { markSettled[id] = resolve })
      return {
        requested: seen[id]!,
        settled: settled[id]!,
        release() { release[id]?.(); delete holds[id] },
      }
    },
  }
}
