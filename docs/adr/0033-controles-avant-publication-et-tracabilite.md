# 0033 — Contrôles avant publication et traçabilité de la décision
Statut : Proposé — Date : 2026-11-17 — Décideurs : (à compléter par un humain)

## Contexte
Vision V2 §2 (« contrôles avant publication », revue de jalon), §3 étape 6 et §8 (comptes utilisateurs : décision
d'équipe). Aujourd'hui publier fige tout ce qui a un coût, sans rien vérifier d'autre ni dire qui a décidé et
pourquoi. `docs/features.md` reste « mono-utilisateur, pas de droits d'accès ».

## Options
1. **Ne rien faire** — aucun coût ; un snapshot peut figer un contrat expiré, et personne ne sait qui l'a validé.
2. **Contrôles + traçabilité saisie** (vision §8 option A) : règles du domaine, `block` empêche, `warn` exige une
   justification écrite ; note, « proposé par », « validé par » (noms saisis) et résultats des contrôles copiés
   dans le snapshot, immuables.
3. **Comptes, rôles et workflow d'approbation** (option C) — vraie identité ; coûteux, contraire à features.md.

## Décision
**Option 2**, sans authentification : les noms sont du texte libre, non vérifié (un outil mono-utilisateur ne
peut pas garantir l'identité, il garantit la trace). Règles, une fonction pure par règle dans `internal/bom` :
`offers_active` (block), `fx_complete` (block), `prices_frozen` (warn : prix en direct, un snapshot de prix sera
créé à la publication), `method_complete` (block), `target_date` (warn si la date cible n'est pas tenue),
`budgets` (warn par sous-système au-dessus de son budget), `expired_contracts` (block : offre dont le seul
contrat est expiré à la date de calcul). Publication : un `block` → 409 `ErrPublicationBlocked` ; un `warn` sans
justification non vide → 422 `ErrJustificationRequired`. Les contrôles sont recalculés au moment de publier
(jamais repris d'un écran), puis copiés tels quels dans `bom_snapshot_checks` avec la justification de chaque
`warn` ; une justification pour un contrôle qui n'est pas en `warn` n'est pas stockée.

## Conséquences
- Facile : la revue de jalon se relit dans le snapshot (qui, quand, quoi, pourquoi) ; une règle = une fonction
  testée ; ajouter une règle ne touche ni l'API ni le schéma.
- Difficile : un nom saisi peut être faux (assumé, §8) ; un client qui publiait sans corps reçoit maintenant 422
  s'il reste un `warn` (le front doit envoyer les justifications). Les règles évoluent : un vieux snapshot garde
  les contrôles de son époque, pas ceux d'aujourd'hui (voulu).
- Retour arrière : la migration 00013 ajoute des colonnes NULL et une table ; les retirer ne touche pas aux
  snapshots. Pivot (comptes réels) : `proposed_by`/`validated_by` deviennent des références d'utilisateur, une
  nouvelle ADR remplacera celle-ci ; multi-usine : la règle `target_date` se lit par usine.
