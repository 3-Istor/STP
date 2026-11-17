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

const dateTimeFormat = new Intl.DateTimeFormat('fr-FR', { dateStyle: 'short', timeStyle: 'medium', timeZone: 'UTC' })

// formatDateTime : date et heure à la seconde près (nom d'un snapshot de prix).
export function formatDateTime(iso: string): string {
  return `${dateTimeFormat.format(new Date(iso))} UTC`
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

// formatSignedMoney : écart signé « + 348,40 € » / « − 125,00 € » ; le signe vient de la chaîne, jamais d'un
// calcul. « 0,00 € » sans signe.
export function formatSignedMoney(m: Money | null | undefined): string {
  if (!m) return 'n/d'
  const neg = m.amount.startsWith('-')
  const abs = neg ? m.amount.slice(1) : m.amount
  const text = formatMoney({ amount: abs, currency: m.currency })
  if (/^0+(\.0+)?$/.test(abs)) return text
  return `${neg ? '−' : '+'} ${text}`
}

/** Signe d'une chaîne décimale : -1, 0 ou 1 (couleur d'un écart), sans conversion en nombre. */
export function decimalSign(value: string): -1 | 0 | 1 {
  if (/^-?0*(\.0*)?$/.test(value)) return 0
  return value.startsWith('-') ? -1 : 1
}

const percentFormats = new Map<number, Intl.NumberFormat>()

// formatPercent : pourcentage déjà calculé par l'API (chaîne), affiché avec `digits` décimales au plus.
export function formatPercent(value: string | null | undefined, digits = 1): string {
  if (value === null || value === undefined) return 'n/d'
  let f = percentFormats.get(digits)
  if (!f) {
    f = new Intl.NumberFormat('fr-FR', { maximumFractionDigits: digits, minimumFractionDigits: digits, roundingMode: 'halfEven' })
    percentFormats.set(digits, f)
  }
  return `${f.format(value as Decimal)}\u00a0%`
}

function scaledPair(a: string, b: string): [bigint, bigint] | null {
  const re = /^(-?)(\d+)(?:\.(\d+))?$/
  const ma = re.exec(a), mb = re.exec(b)
  if (!ma || !mb) return null
  const scale = Math.max((ma[3] ?? '').length, (mb[3] ?? '').length)
  const big = (m: RegExpExecArray) => BigInt(`${m[2]}${(m[3] ?? '').padEnd(scale, '0')}`)
  return [big(ma), big(mb)]
}

/** Compare |a| et |b| (chaînes décimales) exactement, sans `number` : < 0 si |a| < |b|. */
export function compareAbsDecimal(a: string, b: string): number {
  const p = scaledPair(a, b)
  if (!p) return 0
  return p[0] < p[1] ? -1 : p[0] > p[1] ? 1 : 0
}

/** Montant sans son signe (affichage d'une marge : « budget tenu, marge de 510,60 € »). */
export function absMoney(m: Money): Money {
  return { amount: m.amount.replace(/^-/, ''), currency: m.currency }
}
