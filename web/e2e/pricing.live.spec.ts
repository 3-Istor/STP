import { expect, test } from '@playwright/test'
import { formatMoney, formatVariation } from '../src/suppliers/format'
import type { OffersPage } from '../src/suppliers/types'

// No route interception and no EventSource replacement: Go supplies the real
// source HTTP process, PostgreSQL, worker clock, REST handlers and SSE transport.
test('tick réel, contexte figé et reconnexion sans navigation', async ({ page, request }) => {
  const info = await (await request.get('/api/v1/__test/info')).json() as { first: number; second: number }
  const frozen = await request.post('/api/v1/price-snapshots', { data: { scope: 'catalog', target_currencies: ['EUR'] } })
  expect(frozen.status()).toBe(201)
  const location = frozen.headers().location
  const frozenBefore = await (await request.get(location)).json()
  await page.goto('/#fournisseurs')
  await page.getByRole('button', { name: 'Fournisseur Flux Boréal', exact: true }).click()
  const supplier = page.getByRole('button', { name: 'Fournisseur Flux Boréal', exact: true })
  const price = page.getByTestId(`price-${info.second}`)
  await expect(price).not.toHaveText('n/d')
  const oldPrice = await price.locator('..').textContent()
  await page.getByRole('button', { name: 'Historique de runtime part', exact: true }).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  let navigations = 0
  page.on('framenavigated', (frame) => { if (frame === page.mainFrame()) navigations++ })
  const before = await (await request.get('/api/v1/pricing/status')).json() as { tick: number }
  expect((await request.post('/api/v1/__test/tick')).status()).toBe(204)
  const current = await (await request.get('/api/v1/suppliers')).json() as { id: number; name: string }[]
  const id = current.find((item) => item.name === 'Flux Boréal')!.id
  const updated = await (await request.get(`/api/v1/suppliers/${id}/offers`)).json() as OffersPage
  const view = updated.offers.find((item) => item.offer.id === info.second)!
  expect(updated.context?.tick).toBe(before.tick + 1)
  await expect(price).toHaveText(formatMoney(view.statistics!.current_quote!.price))
  // Daily quotes can round to the same cents; the observed quote/date must still advance.
  expect(await price.locator('..').textContent()).not.toBe(oldPrice)
  await expect(page.getByTestId(`variation-${info.second}`)).toHaveText(formatVariation(view.statistics!.variation_percent, view.statistics!.trend))
  await expect(supplier).toHaveAttribute('aria-current', 'true')
  await expect(page.getByRole('dialog')).toBeVisible()
  expect(await (await request.get(location)).json()).toEqual(frozenBefore)
  // Drop the actual connection, commit while it is down, then let EventSource
  // reconnect with Last-Event-ID and refresh the current REST state.
  expect((await request.post('/api/v1/__test/disconnect')).status()).toBe(204)
  expect((await request.post('/api/v1/__test/tick')).status()).toBe(204)
  const resumed = await (await request.get(`/api/v1/suppliers/${id}/offers`)).json() as OffersPage
  await expect(price).toHaveText(formatMoney(resumed.offers[0].statistics!.current_quote!.price))
  await expect(supplier).toHaveAttribute('aria-current', 'true')
  expect(navigations).toBe(0)
})

test('panne réelle, repli visible puis reprise sur tick validé', async ({ page, request }) => {
  expect((await request.post('/api/v1/price-snapshots', { data: { scope: 'catalog', target_currencies: ['EUR'] } })).status()).toBe(201)
  await page.goto('/#fournisseurs')
  await expect(page.getByRole('heading', { name: 'Flux Atlas', exact: true })).toBeVisible()
  const before = await (await request.get('/api/v1/pricing/status')).json() as { tick: number }
  expect((await request.post('/api/v1/__test/source?available=false')).status()).toBe(204)
  try {
    expect((await request.post('/api/v1/__test/tick')).status()).toBe(503)
    await expect(page.getByText(/Source de prix indisponible[.] Prix du snapshot/)).toBeVisible()
    const failed = await (await request.get('/api/v1/pricing/status')).json() as { tick: number; status: string }
    expect(failed.tick).toBe(before.tick)
    expect(failed.status).toBe('unavailable')
  } finally {
    await request.post('/api/v1/__test/source?available=true')
  }
  expect((await request.post('/api/v1/__test/tick')).status()).toBe(204)
  await expect(page.getByText(/Source de prix indisponible[.] Prix du snapshot/)).toHaveCount(0)
  const after = await (await request.get('/api/v1/pricing/status')).json() as { tick: number; status: string }
  expect(after.tick).toBe(before.tick + 1)
  expect(after.status).toBe('ready')
})
