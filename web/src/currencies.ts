// Devises proposées au choix de la devise de référence d'une BOM : celles pour lesquelles la source de
// prix publie des taux (GET /api/v1/fx), plus celles déjà utilisées par une BOM (jamais retirées du choix).
// Sans taux lisibles, seules les devises déjà utilisées sont proposées : aucune devise n'est inventée.

type FXPage = { rates: { base_currency: string; quote_currency: string }[] }

export async function loadCurrencies(): Promise<string[]> {
  const res = await fetch('/api/v1/fx')
  if (!res.ok) throw new Error(`taux de change : ${res.status} ${res.statusText}`)
  const page = (await res.json()) as FXPage
  return page.rates.flatMap((r) => [r.base_currency, r.quote_currency])
}

// currencyChoices fusionne les devises connues, sans doublon, triées par ordre alphabétique.
export function currencyChoices(...lists: readonly string[][]): string[] {
  return [...new Set(lists.flat().map((c) => c.trim().toUpperCase()).filter((c) => c !== ''))].sort()
}
