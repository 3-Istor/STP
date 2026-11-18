// Phrases françaises des codes stables du champ `code` de {"error": …} (internal/httpapi/errorcode.go).
// Un code absent de la table retombe sur le texte du serveur : jamais d'erreur muette.
export const errorPhrases: Record<string, string> = {
  bom_not_found: 'Cette BOM n’existe pas (ou plus).',
  node_not_found: 'Ce nœud n’existe pas (ou plus).',
  snapshot_not_found: 'Cette version figée n’existe pas.',
  unknown_offer: 'Cette offre fournisseur n’existe pas.',
  price_snapshot_not_found: 'Ce lot de prix figés n’existe pas.',
  idea_not_found: 'Cette idée n’existe pas.',
  cycle: 'Ce déplacement créerait une boucle dans l’arbre : une pièce ne peut pas se contenir elle-même.',
  has_snapshots: 'Cette BOM a des versions figées : on repart d’un nouveau brouillon.',
  root_node: 'Le nœud racine ne peut pas être modifié ou supprimé ainsi.',
  no_quote: 'Aucun prix coté pour cette offre à cette date.',
  no_rate: 'Aucun taux de change disponible pour convertir cette devise.',
  missing_attribute: 'Un attribut de la pièce est manquant dans le catalogue.',
  missing_cost: 'Le coût d’une pièce n’est pas calculable : une donnée manque.',
  no_budget_rate: 'Aucun taux budget pour cette devise : ajoutez-le dans les paramètres de coût.',
  no_tooling: 'L’outillage de cette pièce n’est pas renseigné.',
  no_program_volume: 'Le volume du programme n’est pas renseigné.',
  no_cost_date: 'La date de calcul du coût n’est pas définie.',
  no_cost_facts: 'Les données de coût de cette BOM sont indisponibles.',
  incomplete_cost: 'Le coût est incomplet : corrigez les pièces sans coût avant de figer.',
  contract_selection_required: 'Plusieurs contrats sont actifs pour cette pièce : choisissez celui à appliquer.',
  invalid_logistics: 'Les données logistiques saisies ne sont pas valides.',
  analysis_unavailable: 'L’analyse n’est pas disponible pour le moment.',
  profiles_unavailable: 'Les profils de véhicule ne sont pas disponibles pour le moment.',
  not_found: 'Ressource introuvable.',
  conflict: 'Cette opération entre en conflit avec l’état actuel des données.',
  unavailable: 'Service momentanément indisponible. Réessayez dans un instant.',
  method_not_allowed: 'Cette opération n’est pas permise ici.',
  route_not_found: 'Adresse de l’API introuvable.',
  no_customs_duty: 'Aucun droit de douane connu pour cette origine.',
  no_freight_rate: 'Aucun tarif de fret connu pour cette destination.',
  no_packaging: 'L’emballage de cette pièce n’est pas renseigné.',
  no_destination: 'Le pays de destination n’est pas renseigné.',
  publication_blocked: 'La publication est bloquée par un contrôle : voir la liste des contrôles.',
  budget_exceeds_parent: 'Ce budget dépasse celui de l’ensemble parent.',
  bridge_unavailable: 'Le pont de coût est indisponible pour cette BOM.',
  bridge_incomplete: 'Le pont de coût est incomplet : des pièces n’ont pas de coût.',
  invalid_history_data: 'Une donnée de prix enregistrée est invalide : la conversion du budget est impossible pour le moment.',
  budgets_changed: 'Les budgets ont changé pendant l’enregistrement : rien n’a été écrit, recommencez.',
  currency_mismatch: 'Deux montants en devises différentes ne peuvent pas être additionnés sans taux.',
  justification_required: 'Une justification est nécessaire pour continuer.',
  bad_request: 'La demande est illisible ou incomplète.',
  invalid_input: 'Une valeur saisie n’est pas valide.',
  invalid_threshold: 'Le seuil saisi n’est pas valide.',
  offer_not_for_part: 'Cette offre n’appartient pas à cette pièce.',
  not_a_part: 'Ce nœud n’est pas une pièce.',
  part_not_found: 'Cette pièce n’existe pas dans le catalogue.',
  offer_not_found: 'Cette offre n’existe pas dans le catalogue.',
  unknown_currency: 'Cette devise n’est pas reconnue.',
  suggestions_unavailable: 'Les suggestions ne sont pas disponibles pour le moment.',
  logistics_unavailable: 'Les données logistiques ne sont pas disponibles pour le moment.',
  cost_sources_unavailable: 'Les sources de coût ne sont pas disponibles pour le moment.',
  ideas_unavailable: 'Les idées ne sont pas disponibles pour le moment.',
  internal: 'Erreur interne du serveur. Réessayez ; si elle persiste, prévenez l’équipe.',
}

// Codes dont le texte du serveur nomme le champ, la valeur ou le nœud en cause : on le garde après la phrase.
// no_rate / no_quote : the server text names the missing pair or offer and the price snapshot.
const detailedCodes = new Set(['bad_request', 'invalid_input', 'invalid_threshold', 'publication_blocked', 'budget_exceeds_parent', 'cycle', 'missing_cost', 'invalid_logistics', 'no_rate', 'no_quote'])
export const keepsServerDetail = (code: string | undefined): boolean => code !== undefined && detailedCodes.has(code)

export function phraseFor(code: string | undefined): string | undefined {
  return code ? errorPhrases[code] : undefined
}
