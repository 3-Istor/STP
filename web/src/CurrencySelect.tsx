import { useContext } from 'react'
import { currencyChoices, CurrenciesContext } from './currencies.ts'

type Props = {
  value: string
  onChange: (currency: string) => void
  'aria-label': string
  required?: boolean
}

// CurrencySelect : seul moyen de choisir une devise dans l'UI. Les devises viennent des taux publiés
// (CurrenciesContext) ; la valeur courante reste toujours proposée, même absente de la liste.
export function CurrencySelect({ value, onChange, required, 'aria-label': label }: Props) {
  const choices = currencyChoices(useContext(CurrenciesContext), [value])
  return (
    <select aria-label={label} required={required} value={value} onChange={(e) => onChange(e.target.value)}>
      {choices.map((c) => (
        <option key={c} value={c}>
          {c}
        </option>
      ))}
    </select>
  )
}
