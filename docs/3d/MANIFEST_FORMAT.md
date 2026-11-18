# Manifestes d'assemblage — contrat frontend v1

Implémentation autorisée de l'option 1 de l'ADR 0041. Le fichier d'inventaire
`assets-manifest.json` est un document de recherche, pas un manifeste d'assemblage.

Un manifeste d'assemblage contient :

- `schema_version: 1`, `revision` explicite, `scope: {kind: "draft"|"snapshot", id}` et `vehicle_label`.
- `assets[]` : `key`, `path` GLB autonome sous `/models/`, `sha256`, `unit_scale` en mètres,
  `source_url`, `license`, `license_url`, `author`, `geometry_status`.
- `bindings[]` : `node_id`, `parent_id` et `part_id` attendus, `asset_key`, `mesh_path`,
  `system`, `position_status`, `instances[]`.
- `assemblies[]` : `node_id` et `explode_vector` pour le déplacement des sous-ensembles.

Une instance déclare `key`, `position` en mètres, `rotation` en radians XYZ, `scale` et
`explode_vector`. Les vecteurs sont des triplets finis, l'échelle ne peut pas être nulle.
La géométrie conserve son repère source ; les transformations d'instance la placent dans le repère
du véhicule (Y vertical). Aucun recentrage ou choix d'axe ne doit être déduit du nom de la pièce.
`mesh_path` est une liste de noms d'enfants glTF successifs et non ambigus ; `[]` désigne tout l'asset.
Ces noms décrivent le fichier 3D, jamais une identité métier de la BOM.

Un asset peut préciser `display_role: "exterior"|"interior"`. Sans ce champ, le comportement
historique est conservé. Un intérieur illustratif est masqué en vue normale pour préserver le
meilleur extérieur fourni ; il apparaît avec l'éclatement, la transparence, le focus, un filtre
de motorisation ou la sélection d'une pièce interne. Ce rôle ne change aucune identité BOM.

`geometry_status` vaut `schematic`, `approximation` ou `verified_exact` ; ce dernier exige
`geometry_evidence_url`. `position_status` vaut `approximation` ou `verified` ; ce dernier exige
`position_evidence_url`. Les preuves sont déclarées par l'auteur du manifeste et doivent être
contrôlées humainement ; l'application ne certifie pas un modèle constructeur en lisant une URL.
`system` vaut `body`, `thermal`, `electric`, `shared` ou `other`. Cette classification explicite
sert aux filtres ; aucun câblage ni lien mécanique n'est déduit de la proximité de deux objets.

## Identité et réconciliation

La clé de picking est l'occurrence `node_id` et la clé d'instance, jamais le nom ou `offer_id`.
Plusieurs liaisons peuvent viser un même nœud uniquement si le parent ou la pièce physique attendue
diffèrent : ce sont ses placements/variantes autorisés. Deux liaisons identiques sont refusées.
Un remplacement sélectionne la variante dont `part_id` ET `parent_id` correspondent à l'arbre courant.
Sinon l'ancien objet disparaît et l'occurrence devient indisponible. Une modification d'offre seule
ne change ni la sélection de l'asset, ni les positions.

Les quantités métier restent des chaînes. Leur produit le long des parents est calculé exactement
pour le nombre d'instances, sans flottants. Le résolveur n'utilise que les emplacements explicitement
fournis, dans leur ordre déclaré ; quantité fractionnaire ou supérieure aux positions connues = état
explicite, sans inventer de demi-pièce ou d'emplacement supplémentaire. Les déplacements d'éclatement
des sous-ensembles se cumulent selon l'arbre, indépendamment de l'ordre des tables JSON.

La portée doit correspondre exactement au brouillon ou au snapshot chargé. Une révision attendue
et explicitement figée est nécessaire pour un snapshot ; son absence interdit le rendu lié plutôt
que de laisser croire que les correspondances ont été publiées avec le snapshot métier.
Le backend et ses données immuables ne sont jamais modifiés par ce contrat.

## Intégration d'un modèle obtenu et vérifié

1. Exporter un GLB autonome : géométrie et textures embarquées. Le chargeur prend en charge glTF 2
   et Meshopt déjà fourni par Three.js. Un fichier qui nécessite d'autres fichiers est refusé :
   l'empreinte doit couvrir le modèle effectivement affiché.
2. Placer le fichier dans `web/public/models/`, calculer `sha256sum`, conserver source, auteur,
   licence et références de validation. Les GLB racine sont ignorés par Git ; organiser leur
   distribution séparément selon leurs droits, sans ajouter un fichier payant au dépôt.
3. Relever les IDs de la **BOM réelle** avec sa route de lecture existante
   `GET /api/v1/boms/{id}/tree`. Les petits exports de démonstration ne constituent pas un
   manifeste de production. Relever également parent, `part_id` et quantités héritées.
4. Inspecter les noms du graphe glTF, unité, axe, origine, limites et matériaux. Déclarer les
   sous-arbres et leurs transformations ; ne pas recentrer automatiquement chaque pièce :
   cela détruirait ses points de montage. Fournir tous les emplacements des pièces répétées.
5. Déclarer explicitement les variantes physiques disponibles et leur même occurrence BOM.
   Une offre fournisseur n'est pas une variante géométrique. L'absence de variante ou de
   placement doit rester visible dans la liste d'indisponibilité.
6. Déposer le JSON dans `web/public/models/manifests/draft-{id}.json`, ou utiliser
   « Composants » → « Composants et sources » → « Manifeste d'assemblage ». Cet import reste
   en mémoire pendant la session de la vue : il ne publie pas de données, ne les enregistre
   pas dans PostgreSQL et ne les intègre pas aux snapshots métier.
7. Vérifier l'assemblage et l'éclaté, picking dans les deux sens, variantes, retrait et quantité,
   puis comparer les huit angles aux références automobiles avant d'annoncer une fidélité.

Les matériaux source restent le mode par défaut. Les couleurs système/budget sont des lectures
alternatives ; les modes de motorisation rendent les composants opposés transparents et exposent
la carrosserie. Ils n'inventent ni câblage, ni propulsion électrique dans une Multipla thermique.
L'application signale la couverture des occurrences physiques, la géométrie déclarée et les
placements approximatifs. Une URL de preuve ne vaut pas certification constructeur.

Pour les snapshots, le shell actuel ne dispose pas d'une révision de manifeste publiée et figée :
le mode lié affiche donc l'erreur de révision attendue. Les vues historiques Simple/Réaliste restent
disponibles. Une future source PostgreSQL peut implémenter `ManifestSource` sans déplacer le résolveur
vers le stockage ni changer le picking.
