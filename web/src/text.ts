// Outils de texte partagés par les espaces BOM et fournisseurs (recherche, tri).

// normalize rend une recherche insensible à la casse et aux accents (« ecrou » trouve « Écrou »).
export function normalize(text: string): string {
  return text.normalize('NFD').replace(/\p{Diacritic}/gu, '').toLowerCase().trim()
}

// byName : ordre alphabétique français, nombres compris (« Vis 2 » avant « Vis 10 »).
export const byName = new Intl.Collator('fr', { sensitivity: 'base', numeric: true })
