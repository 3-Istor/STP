# Ressources visuelles encore nécessaires

**Mise à jour du 18 novembre :** les extérieurs Fiat Betier Models et Valhalla OUTPISTON ont été
fournis, inspectés et intégrés, avec les deux kits mécaniques illustratifs. Le blocage de téléchargement
décrit dans l'audit du 17 ci-dessous est levé pour ces fichiers. Voir [SOUTENANCE_ASSETS.md](SOUTENANCE_ASSETS.md)
pour leurs licences, leur préparation et les manques mécaniques qui subsistent. Le candidat Aston
final est différent de celui recherché le 17 et porte une licence CC BY-NC-SA 4.0.

Recherche du 17 novembre 2026. Aucune géométrie constructeur acquise ; les 418 types restent
inventoriés dans `assets-manifest.json`. Le nouveau chargement doit permettre d'intégrer de vrais
modèles sans les remplacer par CarConcept et sans inventer une liaison de pièce.

## Candidats extérieurs identifiés auprès des sources publiques

Métadonnées consultées directement via `https://api.sketchfab.com/v3/models/<uid>`.
Le point d'accès officiel `/download` a répondu **401 Unauthorized** pour les quatre candidats :
la licence déclarée permet une réutilisation avec attribution, mais aucun fichier n'est accessible
dans cette session sans téléchargement authentifié. Aucune extraction du viewer n'a été tentée.

| Modèle / auteur | Source | Géométrie annoncée | État |
|---|---|---:|---|
| Fiat Multipla 1998 / Betier Models | [Page](https://sketchfab.com/3d-models/53cb07fa6a264cca811952c7ff3f3b38) | 1210611 faces | CC BY 4.0 déclaré ; extérieur Blender annoncé, origine et découpage à contrôler |
| Fiat Multipla / Grzesiek2229 | [Page](https://sketchfab.com/3d-models/6ca201059699408f88d5ad907a2cb9ea) | 60726 faces | CC BY 4.0 déclaré ; année, silhouette, origine et découpage à contrôler |
| Fiat Multipla / Nieve5677 | [Page](https://sketchfab.com/3d-models/6175014b48804d2284c7201622c37366) | 1219954 faces | CC BY 4.0 déclaré ; aucune description d'origine ni de précision mécanique |
| 2025 Aston Martin Valhalla / Kevin Love SketchFab | [Page](https://sketchfab.com/3d-models/6d6fa32d83b74d629a3a04f4fae3c293) | 370377 faces | CC BY 4.0 déclaré ; carrosserie de série et provenance encore à inspecter |

Une licence affichée sur une page ne prouve pas l'origine d'un mesh ni la version du véhicule.
Les géométries de Multipla ayant des nombres de faces très proches d'autres modèles publiés doivent
faire l'objet d'une vérification de provenance avant redistribution. Aucun candidat n'est embarqué.
La Fiat 600 Multipla 1956/1957, la 1000tipla et la Valhalla concept 2019 sont hors version demandée.

Aperçus publics inspectés visuellement durant cette session (sans les redistribuer dans le produit) :
le candidat Betier montre bien le bandeau et les trois étages de feux de la Multipla avant restylage ;
le candidat Kevin Love montre une silhouette de Valhalla de série, le pavillon, les ouvertures latérales
et les projecteurs. Ce sont de bons candidats d'extérieur, pas des preuves de dimensions ou de
mécanique exacte. L'aperçu de Grzesiek2229 est une image d'attente, donc n'apporte aucune validation.
Les fichiers complets, le découpage des ouvrants et les droits d'origine restent à contrôler.

## Manques précis pour un résultat fidèle

- **Multipla I 1.6 16V** : carrosserie avant restylage, ses trois niveaux de feux, vitrage/bandeau,
  portes, six places 3+3, roues et détails extérieurs ; pièces intérieures découpées et points de
  montage du moteur essence transversal, boîte, trains roulants. Un simple extérieur ne couvre
  pas le moteur ni les 220 références `mpl.*`.
- **Valhalla de série hybride** : carrosserie/aérodynamique de production, ouvrants dièdres et
  monocoque ; géométries/positions du V8 (`vlh.bloc`), DCT (`vlh.boite-dct`), deux moteurs avant
  (`vlh.moteur-av`, deux occurrences/instances vérifiées), moteur arrière (`vlh.moteur-ar`),
  batterie HT (`vlh.batterie-ht`), électronique (`vlh.onduleur`), câbles (`vlh.cable-ht`), refroidissement.
  Les itinéraires HT et hydrauliques ne sont pas fournis par les images extérieures.
- **Pièces communes** : géométries réutilisables, dimensions et placements des fixations/connecteurs ;
  ne pas convertir automatiquement les 45 clés `com.*` en 45 positions physiques supposées.

Avant intégration : fichier effectivement obtenu, auteur et droits conservés, empreinte calculée,
mesh inventory inspecté, unité/axe et placement explicités, bonnes occurrences associées et rendu
comparé aux références. CarConcept demeure un exemple technique sans fidélité Fiat/Aston.
