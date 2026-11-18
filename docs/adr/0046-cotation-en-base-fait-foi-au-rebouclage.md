# 0046 — Au rebouclage, la cotation déjà en base fait foi
Statut : Accepté — Date : 2026-11-17 — Décideurs : Arthur.presle (accord explicite dans le chat)

## Contexte
Relecture de la PR #26 (relecteurs architecture et code). ADR 0043 affirme que le simulateur est « une fonction pure
de (graine, série, date) » et que, au rebouclage, les cotations rejouées sont identiques à celles déjà en base.
**C'est faux** : `pricesim.Simulator.Fetch` calcule `PriceMin + (PriceMax − PriceMin) × sample(graine, série, date)`
(`internal/pricesim/simulator.go:47-49`) ; le prix dépend donc aussi de la fourchette de l'offre, modifiable par
`PATCH /api/v1/offers/{id}`.

Panne : la fourchette d'une offre change pendant le tour 1. Au tour 2, la cotation rejouée d'un jour déjà en base
diffère de celle stockée ; `insertRuntimePrice` (`internal/pricing/pricingstore/runtime.go`) renvoie
`ErrRuntimeDataConflict`, le tick échoue, le run passe « indisponible » et recommence en boucle : plus aucun prix
en direct sans intervention manuelle.

## Options
1. **Ne rien faire** — le tick échoue au premier rejeu d'un jour dont la fourchette a changé.
2. **La cotation en base fait foi** — au conflit `(offre, source, date)`, la ligne existante est conservée telle
   quelle, sans comparaison. Seuls les jours absents sont insérés.
3. **Inclure la fourchette dans la source** (`source` = graine + hash de la fourchette) — chaque fourchette a ses
   propres cotations ; les lectures « dernière cotation ≤ as_of » devraient trier ou filtrer par source. Lourd.
4. **Interdire la modification de fourchette** après le premier tick — casse une fonction existante.

## Décision
**Option 2.** Une cotation écrite est un fait historique : on ne la compare plus, on ne la remplace pas.
- `insertRuntimePrice` : sur conflit, retourne `nil` (plus de lecture de comparaison, plus d'erreur).
- Les **taux de change** gardent leur comparaison : leur définition (référence, bande) fait partie de la config
  hachée ; la changer est déjà un `ErrRuntimeConfigConflict`, donc un rejeu différent y est une vraie anomalie.
- Le contrôle de manifeste (`Apply`, comparaison `oldOffers`/`p.Offers`) est inchangé : il garde l'intégrité du
  hachage, pas le contenu des cotations.
- Correction du fait faux dans ADR 0043 : le simulateur est une fonction pure de la date **et de la fourchette
  de l'offre**. Cette ADR **remplace la règle de comparaison de 0043** (« une cotation identique est acceptée sans
  écriture ») ; le reste de 0043 (`NextAsOf`, tours) est inchangé.

## Conséquences
- Facile : le tick avance toujours ; modifier une fourchette n'arrête plus le flux.
- Difficile : au tour 2, un jour déjà coté garde l'ancien prix (fourchette du moment où il a été coté) ; le prix
  d'une même date n'est plus reproductible par le seul simulateur. « Même entrée + même snapshot = même résultat »
  reste vrai : les lectures et snapshots portent sur les cotations stockées, pas sur un rejeu.
- Une vraie divergence (corruption) n'est plus détectée sur les prix ; acceptable pour un simulateur de démonstration.
- Une cotation déjà écrite garde sa devise d'origine si l'offre change de devise : le manifeste (devise comprise)
  change alors de hash et `Apply` refuse tout plan périmé ; l'historique reste un fait daté.
- Retour arrière : remettre la comparaison dans `insertRuntimePrice` (aucune migration).
- Portée : l'exception ne vaut que pour les **cotations de prix** du runtime. « Contenu divergent : refus sans
  écrasement » (ADR 0015) reste vrai pour les taux de change, la configuration et le manifeste.
- La règle remplacée est celle d'ADR 0015 (« une clé existante n'est acceptée que si le contenu est identique »),
  reprise par 0043 qui la croit sans conséquence. À faire par un humain (ADR acceptées) : en-tête de 0015 =
  « Règle d'horloge remplacée par 0043 ; règle de comparaison des cotations de prix remplacée par 0046 » ;
  en-tête de 0043 = « Fait faux sur la pureté du simulateur corrigé par 0046 ».
