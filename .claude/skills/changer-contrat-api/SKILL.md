---
name: changer-contrat-api
description: Ajouter ou modifier une route ou un JSON de l'API /api/v1 de BOM Studio sans casser les clients. À utiliser dès qu'un handler de internal/httpapi, un champ JSON ou un code HTTP change.
---

# Changer le contrat `/api/v1`

Le front et les tests e2e sont des clients : un champ renommé casse un écran sans erreur de compilation.

## 1. Classer le changement

| Changement | Compatible ? | Ce qu'il faut |
|---|---|---|
| route nouvelle, champ de réponse ajouté, paramètre optionnel ajouté | oui | test + golden des routes |
| champ retiré ou renommé, type changé, code HTTP changé, paramètre devenu obligatoire, route retirée | **non** | ADR (`rediger-adr`) acceptée **avant** le code, ou nouvelle route à côté |

Dans le doute : non compatible. Niveau de risque ≥ 2 dans tous les cas.

## 2. Conventions qui ne se négocient pas

- Préfixe `/api/v1`, motifs `net/http` standard (`GET /api/v1/boms/{id}`).
- Champs en `snake_case` ; montant = `{"amount": "92.00", "currency": "EUR"}`, **jamais** un nombre JSON.
- Erreur : `{"error": "message explicite"}` ; erreur métier → code HTTP choisi dans le handler
  (`errors.Is(err, bom.ErrCycle)` → 409, validation → 422, absent → 404).
- Handler fin : décoder → appeler un service → encoder. Aucune règle métier, aucun accès base.
- Entrée bornée : `http.MaxBytesReader`, listes et profondeurs plafonnées.

## 3. Étapes

1. Test `httptest` d'abord : statut, forme exacte du JSON (champ par champ), cas d'erreur.
2. Code du handler et du service.
3. Inventaire des routes : si une route est ajoutée,
   `go test ./internal/httpapi -run TestRoutesContract -update-routes`, puis relire le diff de
   `internal/httpapi/testdata/routes.golden` (une ligne en plus, aucune en moins).
   Une route retirée fait échouer le test même avec `-update-routes` : c'est voulu.
4. Côté front : mettre à jour le client typé dans `web/src/` (montants en `string`), puis
   `npm --prefix web run build` (le typage attrape les champs renommés).
5. Parcours Playwright concerné : `npm --prefix web run test:e2e -- <fichier>`.
6. `make verify LEVEL=pr`, puis skill `revue-par-niveau` (relecteur-architecture inclus).

## 4. Dans la PR

- Exemple de requête et de réponse.
- « Compatible » ou « non compatible + ADR NNNN ».
- Les clients touchés (écrans, tests e2e).
