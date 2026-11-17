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
