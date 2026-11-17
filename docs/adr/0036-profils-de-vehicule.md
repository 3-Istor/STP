# 0036 — Profils de véhicule : pondérations du classement des alternatives
Statut : Accepté — Date : 2026-11-17 — Décideurs : Brian P. (accord explicite dans le chat)

## Contexte
Vision V2 §4 (étape Structurer, onglet Réglages, maquette `I4-1-Explorateur`) et plan V2 lot E : « ce qui compte
pour ce véhicule » se règle une fois par BOM (citadine éco, familiale, premium, sportive, personnalisé) sur cinq
critères : coût, CO₂, délai, masse, qualité fournisseur. Aujourd'hui (ADR 0028) seule une stratégie par nœud
(coût, délai, CO₂, équilibré à poids égaux sur trois critères) classe les alternatives ; la masse et la qualité
fournisseur n'y entrent pas, et rien n'est réglé au niveau du véhicule. Le classement doit rester explicable et
reproductible (AGENTS §3) et le profil doit être figé au snapshot.

## Options
1. **Ne rien faire** : stratégies par nœud seulement — rien à migrer ; pas de masse ni de qualité, réglage
   répété nœud par nœud, l'écran Réglages n'a rien à montrer.
2. **Colonnes sur `boms` et `bom_snapshots`** — une lecture de moins ; tables partagées modifiées (AGENTS §7),
   conflit avec les autres lots qui touchent ces tables et leurs requêtes.
3. **Tables dédiées** `bom_vehicle_profiles` (brouillon) et `bom_snapshot_vehicle_profiles` (figée, protégée par
   `reject_mutation()`), cinq poids entiers dont la somme vaut 100 (contrainte en base et dans le domaine).

## Décision
**Option 3.** Le score d'une candidate devient une **moyenne pondérée** des scores normalisés (ADR 0028 :
(v − min) ÷ (max − min), 0 = meilleur) : Σ poids × score ÷ Σ poids, à 28 décimales. Les stratégies de nœud
deviennent des poids, au seul endroit `strategyWeights` (AGENTS §4) : `cost` = coût seul, `lead_time` = délai
seul, `co2` = CO₂ seul, `balanced` = coût, délai et CO₂ à poids égaux (calcul identique à ADR 0028).
- **Priorité** : stratégie du nœud ou de son ancêtre le plus proche (choix explicite, ADR 0028) > profil de
  la BOM > aucun classement (ordre par offre). Une BOM sans profil se comporte exactement comme avant.
- **Masse** = masse unitaire × quantité réelle (plus léger = meilleur) ; **qualité** = note de l'offre (plus
  haute = meilleure, score (max − v) ÷ (max − min)). Un critère de poids nul n'est pas évalué ; une candidate
  sans note qualité alors que la qualité pèse est **exclue avec la raison** (`ErrNoQualityScore`), jamais notée 0.
- Préréglages (coût/CO₂/délai/masse/qualité) : citadine éco 55/15/15/5/10, familiale 40/25/20/5/10, premium
  20/25/10/10/35, sportive 20/10/10/45/15 ; `custom` = poids libres. Un préréglage impose ses poids.
- **Gel** : la publication copie le profil dans le snapshot ; un brouillon créé depuis un snapshot le reprend.

## Conséquences
- Remplace **en partie** ADR 0028 (qui reste acceptée et n'est pas réécrite) : « équilibré » est le cas à poids
  égaux d'un score pondéré ; l'héritage, la normalisation, les exclusions et l'ordre à égalité sont inchangés.
- Facile : ajouter un critère (un poids, un score) ou un préréglage. Difficile : des poids non entiers (refusés).
- Retour arrière : migration 00014 down ; sans profil, le classement redevient celui d'ADR 0028.
- Pivot : multi-usine → un profil par véhicule reste valable ; carbone versionné → le score CO₂ suit le facteur
  figé de la pièce, sans changement ici.
