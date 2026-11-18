# 0030 — Historique des prix fournisseur converti au taux du jour de chaque cotation
Statut : Accepté — Date : 2026-11-14 — Décideurs : Brian P. (accord explicite dans le chat, 17/11/2026)

## Contexte
La page fournisseur affiche l'historique annuel d'une offre dans sa devise d'origine (GBP, JPY…), ce qui
rend les offres difficiles à comparer : l'équipe veut lire les courbes en euros. `docs/features.md` exige
de garder montant et devise d'origine et de convertir « au calcul, avec un taux daté », chaque montant
converti indiquant son taux et sa date (l. 205), mais ne dit pas **quel** taux appliquer à un historique.
AGENTS §3 interdit de convertir côté front : la conversion se fait en Go.

## Options
1. **Taux coté le jour de chaque cotation** — montre le vrai coût en euros dans le temps, change compris ;
   les taux quotidiens datés existent déjà en base. Inconvénient : la courbe mélange prix fournisseur et change.
2. Taux le plus récent pour toute l'année — isole l'évolution du prix fournisseur ; mais un prix d'il y a
   onze mois est converti avec un taux qu'il n'a jamais connu.
3. Ne rien faire (devise d'origine) — aucune règle nouvelle ; comparaison entre offres toujours impossible.

## Décision
**Option 1.** `GET /api/v1/suppliers/{id}/offers?currency=EUR` ajoute, par offre, un bloc `converted` :
chaque cotation de l'historique (et les cotations courante et de référence) est convertie avec le dernier
taux coté **au plus tard à l'instant de la cotation**, et au plus 7 jours avant (`pricing.RateLookback`).
Paire directe d'abord, puis inverse (stratégie unique de l'ADR 0017), valeur exacte jusqu'à l'arrondi de
sortie half-even (ADR 0004). Minimum, maximum et variation sont recalculés sur les valeurs converties
exactes. Une cotation sans taux dans la fenêtre n'est pas convertie : `price` à null et erreur nommée,
comptée dans `missing` ; jamais de zéro ni de taux plus ancien.

## Conséquences
- Rétrocompatible : sans `currency`, la réponse est identique (aucune clé `converted`). Devise invalide ou
  sans source de taux : 400, la conversion n'est jamais ignorée en silence.
- Chaque prix converti porte son `fx_quote` (id, paire, taux, source, date) et son sens : explicable et
  reproductible (mêmes cotations + mêmes taux = même résultat, fonction pure `ConvertStatistics`).
- Le front affiche par défaut l'euro ; la devise d'origine reste lisible dans le détail d'une cotation.
- Coût : deux lectures de taux par offre (une par sens de paire). À surveiller si un fournisseur a des
  centaines d'offres (lecture groupée possible plus tard sans changer le contrat).
- Retour arrière : retirer le paramètre côté front ; l'API garde son contrat d'origine.
- Pivot multi-devise / multi-usine : la devise d'affichage est un paramètre, pas une constante ; hors-ligne :
  mêmes taux datés en base, même résultat ; carbone versionné : non concerné.
