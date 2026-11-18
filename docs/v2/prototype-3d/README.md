# Prototypes 3D jetables

Spikes pour décider du niveau de rendu de la vue 3D (voir `../vision-cible.md`). Pas de test, pas destinés
à être mergés tels quels. `three` seul, sans React.

| Fichier | Rendu | Licence |
|---|---|---|
| `car.html` | **Simple** : un objet généré par sous-ensemble | code maison |
| `car-technique.html` | **Technique** : châssis roulant procédural détaillé, carrosserie en radiographie | code maison |
| `car-realiste.html` | **Réaliste** : modèle sous licence + table de correspondance nœud 3D → sous-ensemble, éclaté automatique | modèle CC-BY 4.0 |

Modèle réaliste : « Car Concept », Khronos glTF-Sample-Assets, © 2024 Darmstadt Graphics Group GmbH, Eric Chadwick,
CC-BY 4.0 (attribution obligatoire dans l'interface). Les logos Khronos (marques) sont masqués : plaque et flancs de
pneus. Le fichier (11,8 Mo) n'est pas commité : le télécharger depuis
`https://raw.githubusercontent.com/KhronosGroup/glTF-Sample-Assets/main/Models/CarConcept/glTF-Binary/CarConcept.glb`
dans ce dossier. À compresser (meshopt/Draco) avant usage réel.

Lancer : servir ce dossier avec un lien `three` vers `web/node_modules/three`, puis par exemple
`car-realiste.html?mode=paint|xray&explode=0..1&focus=coin_avg&cam=iso|exploded|corner`.
