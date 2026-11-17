import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'

// Shell V2 (ADR 0034) : thème clair / sombre enregistré par navigateur, préférence système par défaut,
// stockage indisponible toléré ; barre d'état et étapes.

async function mockAPI(page: Page, status: number | 'ready' = 'ready') {
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/pricing/status') {
      if (status === 'ready') await route.fulfill({ json: { status: 'ready', tick: 3 } })
      else await route.fulfill({ status, json: { error: 'source de prix injoignable' } })
      return
    }
    await route.fulfill({ json: path === '/api/v1/fx' ? { rates: [] } : [] })
  })
}

const theme = (page: Page) => page.evaluate(() => document.documentElement.dataset.theme)

test('thème : préférence système par défaut, choix enregistré et relu au rechargement', async ({ page }) => {
  await mockAPI(page)
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.goto('/#/boms')
  expect(await theme(page)).toBe('dark')
  const toggle = page.getByRole('button', { name: 'Passer au thème clair' })
  await toggle.click()
  expect(await theme(page)).toBe('light')
  // Le fond suit les variables du thème (une seule feuille de tokens).
  const bg = await page.evaluate(() => getComputedStyle(document.body).backgroundColor)
  expect(bg).toBe('rgb(244, 245, 247)')
  await page.reload()
  expect(await theme(page)).toBe('light')
  await expect(page.getByRole('button', { name: 'Passer au thème sombre' })).toBeVisible()
})

test('thème : localStorage indisponible, la bascule marche quand même', async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(window, 'localStorage', { configurable: true, get() { throw new Error('SecurityError') } })
  })
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(e.message))
  await mockAPI(page)
  await page.emulateMedia({ colorScheme: 'light' })
  await page.goto('/#/boms')
  expect(await theme(page)).toBe('light')
  await page.getByRole('button', { name: 'Passer au thème sombre' }).click()
  expect(await theme(page)).toBe('dark')
  expect(errors).toEqual([])
})

test('barre d’état : API de prix injoignable signalée, jamais masquée', async ({ page }) => {
  await mockAPI(page, 503)
  await page.goto('/#/boms')
  await expect(page.getByRole('contentinfo')).toContainText('API de prix injoignable')
})

test('barre d’état : API de prix joignable', async ({ page }) => {
  await mockAPI(page)
  await page.goto('/#/boms')
  await expect(page.getByRole('contentinfo')).toContainText('API de prix joignable')
  await expect(page.getByRole('contentinfo')).toContainText('0 BOM')
})
