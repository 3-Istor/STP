// Montants et décimaux : toujours des chaînes venues de l'API, jamais convertis en `number` (AGENTS §3, §5).
// Intl.NumberFormat formate la chaîne décimale exacte ; l'arrondi aux unités mineures de la devise se fait
// ici, à l'affichage seulement, en half-even (ADR 0004).

export type Money = { amount: string; currency: string }

type Decimal = Intl.StringNumericLiteral

const moneyFormats = new Map<string, Intl.NumberFormat>()

export function formatMoney(m: Money | null | undefined): string {
  if (!m) return 'n/d'
  let f = moneyFormats.get(m.currency)
  if (!f) {
    f = new Intl.NumberFormat('fr-FR', { style: 'currency', currency: m.currency, roundingMode: 'halfEven' })
    moneyFormats.set(m.currency, f)
  }
  return f.format(m.amount as Decimal)
}

const decimalFormats = new Map<number, Intl.NumberFormat>()

// formatDecimal affiche une quantité, un CO2 ou une masse (chaîne décimale) avec au plus `digits` décimales.
export function formatDecimal(value: string, digits = 3): string {
  let f = decimalFormats.get(digits)
  if (!f) {
    f = new Intl.NumberFormat('fr-FR', { maximumFractionDigits: digits, roundingMode: 'halfEven' })
    decimalFormats.set(digits, f)
  }
  return f.format(value as Decimal)
}

const dateFormat = new Intl.DateTimeFormat('fr-FR', { dateStyle: 'short', timeStyle: 'short', timeZone: 'UTC' })

export function formatDate(iso: string): string {
  return `${dateFormat.format(new Date(iso))} UTC`
}

// multiplyDecimal multiplie une chaîne décimale par un entier, exactement (BigInt).
export function multiplyDecimal(value: string, factor: number): string {
  const neg = value.startsWith('-')
  const [intPart, frac = ''] = (neg ? value.slice(1) : value).split('.')
  const digits = BigInt(intPart + frac) * BigInt(factor)
  let s = digits.toString().padStart(frac.length + 1, '0')
  if (frac.length > 0) s = `${s.slice(0, s.length - frac.length)}.${s.slice(s.length - frac.length)}`
  return (neg ? '-' : '') + s
}

// decimalInput lit une saisie décimale fr-FR ou en point (« 1 600,5 » → « 1600.5 ») sans passer par `number`.
export function decimalInput(text: string): string {
  return text.trim().replace(/[\s ]/g, '').replace(',', '.')
}

// sameDecimal compare deux chaînes décimales sans `number` : « 1600.50 » et « 1600.5 » sont égales.
export function sameDecimal(a: string, b: string): boolean {
  const canon = (s: string) => {
    let [i, f = ''] = s.split('.')
    i = i.replace(/^(-?)0+(?=\d)/, '$1')
    f = f.replace(/0+$/, '')
    return f ? `${i}.${f}` : i
  }
  return canon(a) === canon(b)
}
