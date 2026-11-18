# 0051 — Un lot de prix figés se supprime tant qu'aucune BOM ne le référence
Statut : Proposé — Date : 2026-11-18 — Décideurs : à valider

## Contexte
Les lots de prix (`price_snapshots` et lignes `price_snapshot_quotes`, `price_snapshot_fx`) sont refusés en
modification **et** en suppression par trigger (`reject_mutation`, ADR 0002). Chaque « Figer maintenant » (ADR 0048)
ajoute un lot de ~1 000 liens ; rien ne permet de nettoyer ceux que personne n'utilise. Demande explicite : pouvoir
supprimer les lots de la liste « Prix utilisés ». Une version publiée doit en revanche garder ses prix à vie.

## Options
1. **Ne rien faire** — la liste grossit sans fin.
2. **`DELETE` libre, la clé étrangère protège** — suffit pour `price_snapshots`, mais rien n'empêche de vider les
   lignes enfants d'un lot utilisé (pas de FK dans ce sens).
3. **`DELETE` autorisé par trigger seulement si le lot n'est référencé nulle part** — la base garde la règle quel
   que soit l'appelant ; les FK (`bom_snapshots`, `boms`) restent le filet final.

## Décision
**Option 3.**
- Migration 00018 : `UPDATE` reste refusé (`reject_mutation`), `TRUNCATE` aussi ; `DELETE` sur les trois tables passe par
  `guard_price_snapshot_delete()`, qui refuse (`restrict_violation`) si une version publiée (`bom_snapshots`) ou un
  brouillon (`boms.price_snapshot_id`) référence le lot.
- Les cotations (`price_quotes`, `fx_quotes`) ne se suppriment jamais.
- `DELETE /api/v1/price-snapshots/{id}` : 204 ; 404 `price_snapshot_not_found` ; 409 `price_snapshot_in_use` dont le
  texte nomme les usages (« utilisé par V1 de … », « attaché au brouillon … »). Contrôle et suppression dans une
  transaction ; le trigger rattrape un brouillon attaché entre-temps.
- `GET /api/v1/price-snapshots` gagne `in_use` et `used_by` (champs additifs) pour griser l'action d'avance.

## Conséquences
- Facile : nettoyer les lots orphelins ; une version publiée ne perd jamais ses prix, même par SQL direct.
- Difficile : le lot attaché à un brouillon est protégé : on repasse en direct ou on change de lot avant de le supprimer.
  Un lot supprimé n'est pas récupérable (on refige, ce qui prend l'instant courant).
- Retour arrière : `goose down` remet les triggers d'origine.
