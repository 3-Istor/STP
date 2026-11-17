// Libellés des lignes de ventilation (ADR 0031) : l'API y écrit les décimales exactes (« Prix coté
// 124.74059645460442 GBP », « taux 1.163666652231798 »). À l'affichage seulement, chaque nombre long est
// arrondi (half-even) : 2 décimales devant un code de devise, 4 sinon (taux, masses). Le libellé exact
// reste disponible en infobulle ; les montants, eux, ne passent jamais par ici.
const money2 = new Intl.NumberFormat('fr-FR', { minimumFractionDigits: 2, maximumFractionDigits: 2, roundingMode: 'halfEven' })
const plain4 = new Intl.NumberFormat('fr-FR', { maximumFractionDigits: 4, roundingMode: 'halfEven' })
const plainAll = new Intl.NumberFormat('fr-FR', { maximumFractionDigits: 20 })


export function roundedLabel(label: string): string {
  return label.replace(/(\d+(?:\.\d+)?)(\s?[A-Z]{3}\b)?/g, (_m, num: string, cur: string | undefined) => {
    const value = num as Intl.StringNumericLiteral
    const decimals = (num.split('.')[1] ?? '').length
    if (decimals === 0) return `${num}${cur ?? ''}` // entiers (années, volumes, paliers) tels quels
    if (cur) return `${money2.format(value)}${cur}`
    return decimals > 4 ? plain4.format(value) : plainAll.format(value)
  })
}
