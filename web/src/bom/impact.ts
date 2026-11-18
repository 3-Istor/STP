import { decimalSign, type Money } from '../money.ts'

type Totals = { cost: Money | null; co2_kg: string; mass_kg: string; lead_time_days: number }
// cost null : coût indisponible avant ou après (devise différente, ou erreur de coût) : jamais un écart inventé.
export type Impact = { cost: Money | null; co2_kg: string; mass_kg: string; lead_time_days: number }

// subDecimal : a − b sur des chaînes décimales, exact (BigInt), sans passer par number.
export function subDecimal(a: string, b: string): string {
  const re = /^(-?)(\d+)(?:\.(\d+))?$/
  const ma = re.exec(a), mb = re.exec(b)
  if (!ma || !mb) throw new Error(`décimal invalide : ${a} / ${b}`)
  const scale = Math.max((ma[3] ?? '').length, (mb[3] ?? '').length)
  const big = (m: RegExpExecArray) => BigInt(`${m[1]}${m[2]}${(m[3] ?? '').padEnd(scale, '0')}`)
  const diff = big(ma) - big(mb)
  const digits = (diff < 0n ? -diff : diff).toString().padStart(scale + 1, '0')
  const text = scale ? `${digits.slice(0, -scale)}.${digits.slice(-scale)}` : digits
  return (diff < 0n ? '-' : '') + text
}

export function computeImpact(before: Totals, after: Totals): Impact {
  const sameCurrency = before.cost && after.cost && before.cost.currency === after.cost.currency
  return {
    cost: sameCurrency ? { amount: subDecimal(after.cost!.amount, before.cost!.amount), currency: after.cost!.currency } : null,
    co2_kg: subDecimal(after.co2_kg, before.co2_kg),
    mass_kg: subDecimal(after.mass_kg, before.mass_kg),
    lead_time_days: after.lead_time_days - before.lead_time_days,
  }
}

export function isNoImpact(i: Impact): boolean {
  return (i.cost === null || decimalSign(i.cost.amount) === 0) && decimalSign(i.co2_kg) === 0 && decimalSign(i.mass_kg) === 0 && i.lead_time_days === 0
}
