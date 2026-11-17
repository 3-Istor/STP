# 0043 — Simulateur de prix en boucle sur une année
Statut : Accepté — Date : 2026-11-17 — Décideurs : Arthur.presle (accord explicite dans le chat)

## Contexte
Tests de l'interface du 17/11 : avec `SIM_YEAR_DURATION=40m` (ADR 0015), l'horloge simulée sort de 2026 en
40 minutes (2028-09 constaté). Les taux budget n'existent que pour 2026 → 8 151 pièces de la BOM 3 sans coût
(`no_budget_rate`) ; les contrats du jeu (fin 2026–2027) passent tous « expirés ». Demande de l'équipe :
**l'API de prix tourne en cycle d'une année ; repasser par les mêmes valeurs n'est pas un problème.**
Faits vérifiés : le simulateur (`pricesim.sample`) est une fonction pure de (graine, série, **date**) ; une cotation
déjà en base au même `(offre|paire, source, date)` est acceptée sans écriture si son contenu est identique
(ADR 0015) ; toutes les lectures prennent « la dernière cotation `quoted_at ≤ as_of` ». Seul le journal
`pricing_ticks` refuse une date simulée déjà vue (`UNIQUE (dataset_version, as_of)`).
Changement de la stratégie d'horloge d'ADR 0015 → AGENTS §7.

## Options
1. **Ne rien faire** — les deux bugs reviennent après 40 min d'exécution.
2. **Rejouer la même année** — le lendemain du dernier jour de l'année (31/12/2026 pour un départ au
   01/01/2026) est le jour de départ ; les cotations rejouées sont identiques et déjà en base. Migration :
   l'unicité de la date dans le journal des ticks est retirée (le numéro de tick reste unique et croissant).
3. **Boucle avec numéro de tour** (valeurs différentes à chaque tour) — tour dans le run, les ticks et la source ;
   toutes les lectures filtrées par tour. Lourd, inutile puisque des valeurs identiques conviennent.
4. **Arrêt au 31/12/2026** — simple, mais plus de prix « en direct » après 40 min.
5. **Rythme réel** (`SIM_YEAR_DURATION=8760h`) — un prix par jour réel, démo invisible.

## Décision
**Option 2**, choisie par l'équipe : la seule règle nouvelle est `pricing.NextAsOf` (un seul endroit, utilisé par
la source et par l'écriture du tick). Aucune cotation n'est créée, modifiée ni supprimée au rebouclage.

## Conséquences
- Facile : taux budget 2026 et contrats du jeu restent valides indéfiniment ; démo en direct conservée ; un run
  déjà au-delà de l'année (2028 en dev) revient au départ au tick suivant, sans rechargement.
- Le temps simulé n'est plus croissant : un snapshot de prix pris le 20/12 n'est plus candidat au repli
  (ADR 0015 : date ≤ instant courant) quand l'horloge est revenue au 05/01 ; un snapshot déjà figé dans une BOM
  reste lu tel quel. Les graphiques d'historique montrent les 12 mois avant la date courante : mêmes valeurs à
  chaque tour.
- Le journal `pricing_ticks` contient plusieurs ticks pour une même date (un par tour) ; le numéro de tick et la
  notification SSE restent croissants.
- Un « prix d'il y a un an » vaut le prix d'aujourd'hui au tour suivant : les tendances annuelles sont celles
  du premier tour. Valable pour un simulateur de démonstration, pas pour une vraie source de prix.
- Retour arrière : `NextAsOf` redevient « +1 jour » ; la migration descendante échoue si un 2e tour existe déjà
  (on ne supprime pas l'historique).
- Remplace ADR 0015 sur la règle « horloge croissante » uniquement ; le reste de 0015 est inchangé.
