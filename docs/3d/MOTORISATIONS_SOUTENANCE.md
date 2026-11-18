# Motorisations Valhalla — 18 novembre 2026

Branche unique `feat/3d-pro-v2`, sauvegarde `0d694cf`, option 1 de l'ADR 0042 explicitement
validée par l'utilisateur. Aucune migration, dépendance, écriture de BOM ou de snapshot publiée.
La fiche ADR conserve son statut formel à renseigner par un humain.

## Périmètre et résultats observés

Entrée : brouillon Valhalla **#2**, racine **#29978**, devise **GBP**, méthode actuelle **achat /
dernier taux**, volume 150. Cotations synthétiques `pricesim.v1/demo-v4`, date 01/01/2026 UTC.
Les trois résultats utilisent les mêmes cotations, taux, réglages et état de stock lus par le service.
Les montants sont des sorties Go, pas des constantes de l'interface.

| Scénario | Nœuds actifs | Occurrences physiques | Coût des composants disponibles / véhicule | Écart / hybride | Écart % |
|---|---:|---:|---:|---:|---:|
| Thermique | 14387 | 9621 | 393 970,50 GBP | −67 410,77 GBP | −14,61 % |
| Électrique | 12605 | 8395 | 369 705,74 GBP | −91 675,53 GBP | −19,87 % |
| Hybride de référence | 14904 | 10003 | 461 381,26 GBP | 0,00 GBP | 0,00 % |

La BOM actuelle contient une occurrence de plus que l'audit 3D précédent : **#74862**, pièce
**#67**, parent **#31005**, quantité 1 ; elle est conservée et calculée dans les trois scénarios.
Les comptes désignent des occurrences, pas la somme des quantités. Les quantités réelles sont
propagées par le moteur existant sur chaque arbre actif, même pour les pièces sans modèle 3D.

| Scénario | Sous-arbres retirés (descendants inclus une seule fois) | Conséquence visible |
|---|---|---|
| Thermique | #34537, chaîne de traction électrique ; 517 nœuds | V8/turbos conservés ; trois moteurs, batterie HV, onduleurs et faisceaux HV exclus |
| Électrique | #32413 V8, #35285 échappement, #35392 carburant ; 2299 nœuds | V8/turbos retirés ; moteurs électriques, batterie et électronique HV conservés |
| Hybride | Aucun | Architecture complète de référence |

**Hypothèses approuvées :** DCT #35054, transmission #35205 et refroidissement #35207 communs.
Les adaptations ne sont pas certifiées. Le scénario électrique est une étude de conception :
la batterie hybride existante est chiffrée, mais une batterie BEV adaptée, l'autonomie et les
transformations nécessaires ne sont pas dimensionnées ni chiffrées. Ce résultat ne constitue
pas le coût d'une Valhalla électrique techniquement complète ou proposée par le constructeur.
Le coût thermique couvre également le périmètre conservé, hors adaptations non conçues.

Masse / CO₂ / délai observés : thermique 945,800715 kg / 12897,558015 kg / 61 j ; électrique
954,278125 kg / 16187,635085 kg / 61 j ; hybride 1169,913515 kg / 18068,795815 kg / 61 j.
Ce sont les données catalogue de démonstration ; l'UI signale les sommes partielles si un attribut
manque. Le délai couvre approvisionnement et production de l'arbre actif, pas la conception des adaptations.

## Fichiers et responsabilités

| Fichiers | Responsabilité |
|---|---|
| `internal/bom/configurations.go` | Gardes d'occurrences, exclusions dédupliquées, réemploi de `ComputeTotals`, budgets et planning actifs ; aucune écriture au store |
| `internal/httpapi/configurations.go`, `router.go`, `testdata/routes.golden` | Route additive POST de calcul, décodage borné, erreurs JSON, sérialisation du contrat existant d'arbre |
| `web/src/bom/data/powertrain-valhalla.json`, `powertrain.ts` | Profil d'installation explicite, IDs/parents/pièces attendus, exclusions et hypothèses ; aucun rapprochement par nom |
| `web/src/api.ts`, `usePowertrainSimulation.ts` | Appel et cache commun des trois arbres ; garde contre réponse obsolète ; sélection/sortie locale et lecture seule |
| `PowertrainPanel.tsx`, `bom.css` | Comparaison, sélection, limites, liste paginée des retraits ; détails à la demande |
| `BomWorkspace.tsx`, `useBomData.ts`, `NodePanel.tsx`, `Summary.tsx` | BOM et indicateurs du scénario actif ; interdiction des mutations et absence de ventilation/anomalies provenant de la référence |
| `Structurer.tsx`, `Vehicle3DView.tsx`, `types.ts`, `demo/Demo.tsx` | Rendu Composants permanent sur les vraies BOM ; scène construite depuis les occurrences actives ; filtre visuel indépendant |
| Tests Go, `powertrain.test.ts`, `e2e/powertrain.spec.ts`, fixtures e2e | Calculs réels, communs/quantités, stratégies, erreurs, identité, absence de mutations et affichage API |
| `tools/validate_permanent_components.mjs`, `tools/validate_powertrain.mjs`, `tools/validate_soutenance.mjs` | Parcours Chromium local avec vrais assets/API et captures reproductibles |

Le schéma Simple et le chargeur CarConcept restent uniquement exercés par des fixtures techniques
pour conserver les tests existants. Les vraies BOM dans le produit et le démonstrateur utilisent
toujours `bound`, même sans manifeste : absence explicite, aucun ancien modèle de remplacement.

## Pannes prévisibles et limites

- Occurrence déplacée/remplacée ou installation différente : le profil frontend devient inapplicable
  et le service refuse une définition périmée (422). Les gardes ne s'appuient jamais sur les noms.
- Prix/taux manquant : total incomplet avec erreurs nommées ; les scénarios excluant la pièce peuvent
  rester calculables ; aucun écart avec une référence incomplète. Tests Go et navigateur dédiés.
- API indisponible : référence conservée, erreur explicite, actualisation manuelle proposée.
- Assets absents : BOM et comparaison restent calculables ; message de géométrie absente. Le rendu
  ne fabrique aucun modèle. Mécanique illustrative, extérieurs détaillés sans prétention de CAO constructeur.
- Simulation limitée à Structurer et au brouillon Valhalla #2 vérifié. Quitter ou changer d'étape
  restitue la référence. Pour modifier, quitter la simulation, éditer puis relancer le calcul.
- Les trois résultats restent sur leur entrée capturée jusqu'à **Actualiser la comparaison** ou
  une nouvelle lecture de BOM. Les dates/sources restent consultables ; pas de prix historique forcé.

## Rejouer les vérifications

Lancement et parcours humain : [guide de soutenance](../../3D_DEMO_SOUTENANCE.md).
Avec les services locaux du guide et Chromium installé :

```sh
export PLAYWRIGHT_BROWSERS_PATH=/tmp/stp-3d-toolchain/browsers
node docs/3d/tools/validate_permanent_components.mjs
node docs/3d/tools/validate_powertrain.mjs
```

Captures : `captures/rendu-permanent/` et `captures/motorisations/`.
Le rapport JSON des motorisations contient les sources, totaux, différences de composition,
contrôles de scène et empreintes de référence avant/après. Aucun de ces deux outils n'édite la BOM.
Résultats globaux et revue consignés dans `3D_PROGRESS.md` après leur exécution.
