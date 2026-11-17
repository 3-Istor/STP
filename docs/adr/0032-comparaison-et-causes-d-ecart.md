# 0032 — Comparaison de deux versions et causes d'écart de coût (pont de coût)
Statut : Proposé — Date : 2026-11-17 — Décideurs : (à compléter par un humain)

## Contexte
Vision V2 §2 (« pont de coût entre deux versions », priorité 1) et §3 étape 5 : expliquer l'écart de coût entre
deux versions (snapshot publié ou brouillon) par causes lisibles, dont la **somme vaut exactement l'écart**.
Plusieurs effets interagissent (une hausse de prix d'une pièce en yen dépend aussi du taux) : sans règle écrite,
la même paire de versions pourrait donner deux ponts différents. Feature : bom-draft-and-snapshots, cost-explain.

## Options
1. **Ne rien faire** : seulement l'écart total — rien à maintenir ; l'acheteur ne sait pas d'où il vient.
2. **Écart par pièce, classé par nature après coup** (offre changée, prix changé…) — simple ; un nœud qui
   change à la fois d'offre et de taux est compté deux fois ou arbitrairement, la somme ne tombe pas juste.
3. **Remplacement séquentiel dans un ordre fixe** : on part de la version « de », on remplace une famille
   de faits à la fois par celle de la version « vers », et on rechiffre chaque marche avec le **même moteur
   que les totaux** (`bom.ComputeTotals`, ADR 0007/0031). Cause = coût(marche k) − coût(marche k−1).

## Décision
**Option 3**, ordre fixe : `structure` (arbre, quantités, attributs des pièces) → `offer_change` (offre choisie
d'un nœud apparié) → `price` (même offre, autre cotation) → `fx` (derniers taux et taux budget) → `discount`
(remise retenue par offre) → `logistics` (douane, transport, emballage) → `tooling` (outillage, volume
programme) → `method` (base et politique de change). La somme télescope : Σ causes = coût(vers) − coût(de),
exactement, sans arrondi (ADR 0004 en sortie seulement). Contrôle : la première et la dernière marche doivent
redonner les coûts des deux versions, sinon `ErrBridgeMismatch` (bug, jamais un écart maquillé).
- **Appariement des nœuds** par chemin depuis la racine : sous-ensemble = son nom, pièce = son id de pièce,
  doublons entre frères départagés par ordre d'id (un brouillon créé depuis un snapshot garde cet ordre).
  Non apparié = ajouté ou retiré (cause `structure`).
- **Fait absent d'un côté** (offre nouvelle, pièce retirée) : on prend celui de l'autre côté ; le nouveau prix
  d'une pièce ajoutée tombe donc dans `structure`, l'outillage d'une nouvelle offre dans `offer_change`.
- Les remises sont d'abord **résolues** de chaque côté (contrats à la date de calcul, ou figées au snapshot)
  puis traitées comme des faits figés : un contrat expiré entre deux versions sort en `discount`.
- Chaque cause porte les nœuds dont le coût propre bouge à cette marche (les 20 plus gros écarts, et leur
  nombre) et une phrase pour un acheteur (devises qui montent, part du coût hors devise de la BOM, contrats…).
- Devises de BOM différentes : refus nommé (`ErrCurrencyMismatch`) ; coût indisponible d'un côté : refus nommé.

## Conséquences
- Facile : rejouable (mêmes entrées → même pont), testé par construction (somme = écart au centime, chaque
  cause isolée, combinées, devises différentes). Un nouveau fait = une marche de plus, à une place fixée.
- Difficile : l'attribution dépend de l'ordre (comme tout pont séquentiel) ; c'est documenté, pas caché. Coût :
  9 calculs de l'arbre (~0,1 s chacun pour 30 000 nœuds). Deux méthodes différentes avec des faits manquants
  d'un côté peuvent ne pas se décomposer (`ErrBridgeIncomplete`).
- Retour arrière : supprimer la route `compare` ; aucune donnée stockée. Pivot multi-source de prix : la marche
  `price` se scinde par source ; carbone versionné : un pont CO₂ suivrait la même mécanique.
