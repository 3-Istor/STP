# Corrections finales 3D — 18 novembre 2026

Base stable préservée : `043df31`. Branche exclusive : `feat/3d-pro-v2`.

Le mode Réaliste est retiré de l’interface produit. Les vues 3D et Arbre + 3D, les modèles fournis, les couleurs, l’éclaté et le remplacement physique sont conservés. Un ancien état de mode est neutralisé et la scène est réinitialisée par identité de BOM. Le chargeur historique reste exercé par les fixtures de test isolées ; il ne peut pas activer CarConcept sur une vraie BOM de l’application.

## Comptes exhaustifs

Les colonnes suivantes sont disjointes et couvrent tous les IDs, groupes virtuels compris. « Sous-ensemble seulement » signifie une sélection de descendants ou une action de localisation explicite : aucune géométrie individuelle de la pièce n’est revendiquée.

| Voiture | Total BOM | Géométrie propre | Sous-ensemble seulement | Ni géométrie ni localisation fiable |
|---|---:|---:|---:|---:|
| multipla | 29977 | 157 | 25754 | 4066 |
| valhalla | 14903 | 69 | 7216 | 7618 |

Pièces physiques uniquement :

| Voiture | Total pièces | Individuelles | Localisables seulement | Sans localisation | Sans modèle individuel au total |
|---|---:|---:|---:|---:|---:|
| multipla | 20108 | 146 | 17257 | 2705 | 19962 |
| valhalla | 10002 | 47 | 4995 | 4960 | 9955 |

## Causes et corrections

| Cause / état restant | Fiat | Aston |
|---|---:|---:|
| Mapping individuel non prouvé, géométrie du type déjà disponible | 52 | 74 |
| Géométrie individuelle absente du registre | 19910 | 9881 |
| Présent, masqué en vue normale et révélé par sélection/éclaté | 148 | 54 |
| Identité de pièce ou parent incorrect | 0 | 0 |
| Erreur de chargement | 0 | 0 |
| Visibilité ou picking incorrect après sélection/isolation | 0 | 0 |
| Quantité ou emplacement non représentable pour une liaison active | 0 | 0 |
| Groupe virtuel sans géométrie propre | 9799 | 4853 |
| Groupe virtuel sélectionnable par ses descendants | 59 | 26 |

Les masquages de la vue normale sont intentionnels pour préserver la qualité extérieure ; ils ne sont pas comptés comme des pannes. Les quatre vues sont contrôlées en mode Composants, filtres remis à zéro. Aucun ancien GLB CarConcept n’est demandé pendant l’audit et les changements de route/actualisations.

Corrections concrètes : deux liaisons Fiat (#8 / pièce #60 et #12932 / pièce #104), chacune avec deux instances déjà présentes. Le retrait de Réaliste concerne les deux voitures. L’action de localisation devient disponible pour 25695 occurrences Fiat et 7190 occurrences Aston. Ces nombres comptent des actions de localisation ajoutées, pas de nouveaux modèles.

## Identifiants des correspondances encore non prouvées

Ces cas restent explicitement non individualisés. La liste de toutes les autres absences, avec chaque ID, parent, pièce, nom et cause, est dans [COMPOSANTS_NON_VISIBLES.md](COMPOSANTS_NON_VISIBLES.md). Les données structurées compressées de `audit-final/` conservent aussi tous les enregistrements.

| Voiture | Référence / nom | IDs exacts non individualisés |
|---|---|---|
| multipla | #48 Longeron avant | 6 |
| multipla | #49 Longeron arrière | 7 |
| multipla | #59 Passage de roue intérieur | 9 |
| multipla | #98 Soupape d'admission | 12934, 12940, 12946, 12952, 12958, 12964, 12970, 12976, 12982, 12988, 12994, 13000, 13006, 13012, 13018, 13024 |
| multipla | #100 Ressort de soupape | 12935, 12941, 12947, 12953, 12959, 12965, 12971, 12977, 12983, 12989, 12995, 13001, 13007, 13013, 13019, 13025 |
| multipla | #106 Galet tendeur de distribution | 13192 |
| multipla | #117 Support moteur hydraulique | 13466 |
| multipla | #144 Pignon de vitesse | 14468, 14471, 14474, 14477, 14480, 14483 |
| multipla | #134 Durite de refroidissement | 14496 |
| multipla | #129 Ligne carburant multicouche | 14769 |
| multipla | #152 Soufflet de cardan | 15052, 15205 |
| multipla | #170 Biellette de direction | 15562 |
| multipla | #191 Vis de roue M12×1,25 | 15869, 15876, 15883, 15890 |
| valhalla | #301 Piston forgé | 32418, 32425, 32432, 32439, 32446, 32453, 32460, 32467 |
| valhalla | #316 Injecteur direct 350 bar | 32421, 32428, 32435, 32442, 32449, 32456, 32463, 32470 |
| valhalla | #318 Bobine crayon haute énergie | 32422, 32429, 32436, 32443, 32450, 32457, 32464, 32471 |
| valhalla | #319 Bougie iridium | 32423, 32430, 32437, 32444, 32451, 32458, 32465, 32472 |
| valhalla | #338 Durite silicone renforcée | 33134, 33205, 35282 |
| valhalla | #349 Réducteur d'essieu avant | 34540, 34608 |
| valhalla | #323 Module de cellules lithium-ion | 34760, 34764, 34768, 34772, 34776, 34780, 34784, 34788, 34792, 34796, 34800, 34804 |
| valhalla | #331 Connecteur haute tension verrouillé | 34762, 34766, 34770, 34774, 34778, 34782, 34786, 34790, 34794, 34798, 34802, 34806, 34942, 34945, 34948, 35051 |
| valhalla | #327 Onduleur carbure de silicium | 34941, 34944, 34947 |
| valhalla | #330 Câble haute tension blindé (m) | 35050 |
| valhalla | #348 Arbre de transmission carbone | 35206 |
| valhalla | #334 Radiateur latéral aluminium brasé | 35209, 35245 |
| valhalla | #339 Vase d'expansion aluminium | 35281 |
| valhalla | #344 Écran thermique feuille d'or | 35391 |

## Responsabilités et cas de panne

- `Structurer.tsx` : choix de modes et identité de scène ; `demo/Demo.tsx` : retrait du bouton technique et refus du rendu historique sur une vraie BOM.
- `selection.ts` : localisation par ascendance BOM, sans substitution automatique ni rapprochement par nom ; `Vehicle3DView.tsx` : action explicite, message et cadrage, ID BOM conservé.
- Manifestes draft-1/3 et outils de préparation : deux liaisons quantifiées, source unique répétée aux deux positions existantes ; vecteurs d’éclatement appliqués aux deux instances.
- `audit_visibility.mjs`, identités source et rapports : inventaire exhaustif API/manifeste/scène et raycaster réel ; aucune mutation BOM.

Pannes protégées : pièce absente qui sélectionnerait automatiquement son parent (tests de non-substitution et de bouton explicite), seconde instance manquante (test de résolution rouge avant réparation), filtre système qui masquerait les intérieurs (test historique conservé), réactivation d’un ancien rendu après navigation (contrôle d’absence et zéro requête CarConcept).

La suite de coque a été scindée en deux scénarios indépendants pour isoler le nouveau parcours de localisation : toutes les assertions et les délais existants sont conservés. Aucun test supprimé ou désactivé.

## Preuves et limites

Contrôles finaux : **63 scénarios Chromium desktop PASS**, **14 Chromium mobile PASS**, tests unitaires,
lint, build et moulinette `make verify LEVEL=pr DATABASE_URL=''` verts. Ce passage Go est sans base
d'intégration : PostgreSQL `not_run`, ainsi que gitleaks/osv-scanner/actionlint absents ; aucun de ces
contrôles n'est annoncé vert. Les API réelles sont utilisées pour l'audit et le parcours de démonstration.

Parcours réel final : zéro erreur, zéro requête CarConcept. Retour 0/50/100/0, raycasts, sélection BOM,
localisations des pièces absentes #12934/#34760, arbres à cames #12932, filtres hybride/électrique/thermique
et remplacement physique sont vérifiés. L'offre seule conserve la géométrie ; les valeurs initiales
du brouillon B sont restaurées et relues après le remplacement. Les colonnes de visibilité de l'audit
décrivent les états du renderer ; elles ne certifient pas l'absence d'occultation dans toute vue globale.
Le picking effectif de chacune des 226 occurrences liées est vérifié après isolation et cadrage.

Revue indépendante niveau 1, base `043df31`, un tour : 0 Important, 0 Nit, 0 Pré-existant. L'agent a
revu la logique et les empreintes ; les artefacts finaux ont ensuite été régénérés et leurs sommes,
IDs uniques et empreintes de manifestes recontrôlés automatiquement. À relire humainement avant
fusion : sélection/localisation dans `Vehicle3DView.tsx`, `selection.ts`, et les deux liaisons quantifiées
de draft-1/3. La revue humaine ne vaut pas autorisation de pousser ou fusionner.

Captures : [galerie après corrections](captures/corrections-finales/README.md). Reproduction : voir [guide de soutenance](../../3D_DEMO_SOUTENANCE.md). Les fichiers binaires tiers restent locaux et ignorés ; les outils, empreintes, manifestes et captures sont suivis.

Les intérieurs et les placements restent illustratifs ; aucune certification mécanique. Les 15 modules du kit HV ne permettent pas d’inventer 12 identités de modules BOM. Les quantités de soupapes, cylindres, tuyauteries et radiateurs ambigus restent annoncées comme telles. La racine de toute la voiture ne remplace pas un sous-ensemble absent.

Aucun backend Go, schéma PostgreSQL ou donnée publiée modifié. Aucune dépendance ajoutée, aucun push, merge ou réécriture Git. Firefox/WebKit, GPU du jury et CI distante restent non vérifiés.
