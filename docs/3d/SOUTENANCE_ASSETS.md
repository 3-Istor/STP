# Assets reçus le 18 novembre 2026

Les cinq fichiers utilisateur ont été inventoriés et extraits sous `/tmp/stp-soutenance-assets-20261118`.
Le ZIP Valhalla intérieur imbriqué a été extrait ; les scripts des packs ont été lus comme documents,
jamais exécutés. Le Blender Fiat original est conservé dans les fichiers source, pas converti de nouveau.
Les GLB sont installés localement, ignorés par Git comme les autres modèles téléchargés.
Pour les réinstaller : `python3 docs/3d/tools/install_soutenance_assets.py`.
Les empreintes et métadonnées des quatre fichiers utilisés sont dans `soutenance-assets.json`.

| Asset | Crédit et licence réellement inscrits | Utilisation |
|---|---|---|
| `Multipla1998.glb` | Betier Models, [source](https://sketchfab.com/3d-models/fiat-multipla-1998-3d-model-53cb07fa6a264cca811952c7ff3f3b38), [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/) | Extérieur reconnaissable, sans certification dimensionnelle |
| `Valhalla2025.glb` | OUTPISTON, [source](https://sketchfab.com/3d-models/2025-aston-martin-valhalla-4294795fcf194e2b9da861b251192bd1), [CC BY-NC-SA 4.0](https://creativecommons.org/licenses/by-nc-sa/4.0/) | Extérieur fourni pour la démonstration locale non commerciale ; licence plus restrictive que le résumé utilisateur |
| `MultiplaInterior.glb` | Création illustrative originale fournie par l'utilisateur, licence publique non spécifiée | Usage local demandé ; ne pas annoncer une licence CC BY ni un CAD Fiat |
| `ValhallaInterior.glb` | Création illustrative originale fournie par l'utilisateur, licence publique non spécifiée | Usage local demandé ; ne pas annoncer une licence CC BY ni un CAD Aston Martin |

Les métadonnées intégrées au GLB priment sur les crédits indicatifs des prompts des ZIP. Les extérieurs
originaux restent intacts. Les transformations sont appliquées dans la scène et sont documentées dans
les manifestes ; aucune animation de rig n'est supposée prouver un mouvement mécanique.

Mesures GLTFLoader, avant calibration, en axes source : Fiat 2,333 × 2,135 × 4,490 ; Aston 2,210 ×
1,187 × 4,762. Ce sont des limites du mesh, pas des cotes constructeur. Les extérieurs ont Y vertical,
avant +Z ; les kits intérieurs sont en mètres, avant +X et Z vertical.
Fiat : échelle uniforme 0,89, rotation Y -90°, centrage sur le milieu des essieux et roues au sol.
Aston : échelle 1, rotation Y -90°, roues au sol. Le placement des organes demeure approximatif.

Premières captures réellement produites par le composant de l'application :
`captures/multipla-exterior-soutenance.png`, `captures/valhalla-exterior-soutenance.png`.
Les fixtures tronquées servent ici seulement à afficher les extérieurs ; couverture physique annoncée
zéro, en attendant les manifestes établis sur les vraies occurrences de la base isolée.
