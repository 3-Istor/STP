# Démonstration 3D — 18 novembre 2026

Branche : `feat/3d-pro-v2`. Les services de démonstration sont préparés séparément du Docker
habituel du projet. La simulation Go en mémoire ajoute une route de lecture/calcul (ADR 0042),
sans migration ni modification des contrats existants ou des données de référence.

## Ouvrir la démonstration déjà lancée

- [Fiat Multipla](http://127.0.0.1:41769/#/boms/1/structurer?tab=3d).
- [Aston Martin Valhalla](http://127.0.0.1:41769/#/boms/2/structurer?tab=3d).
- [Brouillon B : batterie remplaçable](http://127.0.0.1:41769/#/boms/3/structurer?tab=arbre-3d&node=60776).

Le rendu **Composants** est permanent : les modèles fournis sont chargés depuis les manifestes
d'occurrences. Les options Simple, Réaliste et le sélecteur Rendu ont disparu de l'interface produit.
Au premier chargement, laisser finir le téléchargement local des GLB et la compilation des shaders.

## Redémarrer dans WSL

Depuis la racine du dépôt, dans chacun des trois terminaux :

```sh
export PATH=/tmp/stp-3d-toolchain/node-v24.22.3-linux-x64/bin:/tmp/stp-3d-toolchain/go/bin:$PATH
export GOMODCACHE=/tmp/stp-3d-toolchain/gomod
export GOCACHE=/tmp/stp-3d-toolchain/gocache
export GOPATH=/tmp/stp-3d-toolchain/gopath
```

Ces chemins sont ceux de cette machine ; `/tmp` peut être vidé au redémarrage. Dans ce cas utiliser
Node et Go aux versions de `web/package.json` / `go.mod` avec les outils usuels du projet.

Terminal 1 — base dédiée existante et simulateur de prix :

```sh
docker start stp-soutenance-db
make priceapi PRICE_API_PORT=18090
```

Terminal 2 — API :

```sh
make run PORT=18080 PRICE_API_PORT=18090 SIM_YEAR_DURATION=8760h \
  DATABASE_URL='postgres://stp3d:stp3d-demo-local@127.0.0.1:55439/stp3d_demo?sslmode=disable'
```

Terminal 3 — frontend :

```sh
python3 docs/3d/tools/install_soutenance_assets.py --verify --verify-prepared
cd web
STP_API_PROXY=http://127.0.0.1:18080 npm run dev -- --host 127.0.0.1 --port 41769 --strictPort
```

Les mots de passe ci-dessus sont propres à ce conteneur local de démonstration. Ne pas lancer une
seconde instance sur un port occupé. Le conteneur conserve ses données entre deux arrêts. La base
précédente `stp3d` reste conservée ; le serveur utilise désormais **stp3d_demo**. La durée annuelle
de 8760 heures utilise le réglage existant du simulateur : elle maintient la soutenance dans 2026,
année des taux budget semés. Les prix restent synthétiques et datés, sans taux inventé par la 3D.

## Parcours devant le jury

1. Ouvrir la **Valhalla**, vue **3D**. Montrer l'extérieur, tourner et zoomer, puis **Recentrer**.
2. Déplacer **Éclatement** à 50 puis 100 %. La coque se soulève ; les organes se dégagent.
3. Passer en **Arbre + 3D**, sélectionner le bloc V8 ou un moteur avant dans l'arbre. Dans
   **Composants et sources**, **Isoler la sélection**, puis **Tout afficher**. Un clic sur la
   géométrie choisit son occurrence BOM. Fermer le panneau de sources pour voir la scène.
4. Revenir en **3D**, éclatement 0. Dans **Motorisation simulée** (au-dessus de l'explorateur),
   cliquer **Thermique** et attendre la fin du calcul. Le sous-arbre électrique #34537 disparaît
   de la BOM et de la scène ; le coût de la Synthèse devient **393 970,50 GBP** sur l'entrée vérifiée.
5. Cliquer **Électrique** : V8 #32413, échappement #35285 et carburant #35392 retirés, moteurs
   électriques/batterie conservés. Coût des composants disponibles : **369 705,74 GBP**.
   Dire explicitement : batterie BEV adaptée et transformations non conçues/non chiffrées.
6. Cliquer **Hybride** : tous les composants reviennent, **461 381,26 GBP**, tableau des trois
   coûts et écarts. Les composants communs sont calculés une seule fois par configuration.
7. Ouvrir **Hypothèses et limites techniques** ou **Composants retirés** pour expliquer les IDs
   et quantités. La simulation est en lecture seule et utilise une même entrée Go de prix/change.
   **Actualiser la comparaison** relit les cotations et recalcule les trois scénarios ensemble.
8. Dans **Composants et sources**, le **Filtre visuel 3D** ne fait que mettre des systèmes en
   évidence. Il n'a aucune incidence sur la BOM active ou les coûts : montrer cette distinction.
9. **Quitter la simulation** restitue la référence éditable sans écriture de restauration SQL.
   Ouvrir la **Fiat**, montrer silhouette, éclaté, deux arbres à cames #12932 et sélection de roue.
   **Réinitialiser** revient à la pose assemblée. La coloration Matériaux/Budget/Système reste disponible.
10. Ouvrir le brouillon B sur la batterie, puis **Composants et sources → Isoler la sélection**.
    Fermer le panneau pour voir la pièce. Changer **Fournisseur pour cette pièce** : la forme reste
    identique. Ouvrir **Remplacement physique — variante 3D illustrative**, choisir **Batterie 12 V
    60 Ah**, son offre, puis **Confirmer le remplacement physique** : le gabarit change réellement.
    Il s'agit d'une variante de démonstration explicitement illustrative, pas de cotes fabricant.
11. Pour restaurer la démonstration, confirmer ensuite la batterie **50 Ah** et son offre initiale.
    Le test automatisé restaure toujours la référence et l'offre initiales dans son bloc `finally`.

## Fidélité et limites à expliquer

Les extérieurs sont les vrais assets téléchargés fournis, avec leurs matériaux/textures conservés.
Fiat : Betier Models, CC BY 4.0. Valhalla : OUTPISTON, **CC BY-NC-SA 4.0** selon les métadonnées
du fichier, plus restrictive que le résumé fourni. Crédits visibles dans le viewer et dans
`web/public/models/soutenance-credits.html`. Aucun fichier binaire tiers n'est commité.

Les intérieurs sont des prototypes mécaniques fournis et recalés visuellement : moteur/boîte,
six places Fiat, V8/DCT, trois moteurs électriques, batterie HV, refroidissement, châssis et trains.
L'identité BOM vérifiée n'est pas une preuve de géométrie constructeur. La coque Fiat reste monobloc ;
les circuits de câbles, les jeux et les collisions ne sont pas certifiés. Les milliers de fixations
sans modèle restent explicitement dans **Sans modèle 3D**, avec pagination. Une pièce supprimée,
déplacée hors de sa liaison ou une quantité sans emplacement validé ne reçoit pas de mesh inventé.

Les manifestes sont propres à l'installation semée et aux brouillons 1/2/3. Une autre base ou un
nouveau snapshot exige son manifeste vérifié ; l'absence est affichée. Les snapshots publiés ne sont
pas modifiés. Les performances observées en Chromium logiciel ne préjugent pas du GPU du jury.

Les prix sont des données industrielles **synthétiques**, source `pricesim.v1/demo-v4`, cotées au
1er janvier 2026 pour cette entrée. Les montants ci-dessus sont des résultats observés, jamais des
constantes React ; une modification de BOM, devise, offre ou cotation change le résultat Go.
Le profil vérifié ne s'applique qu'au brouillon Valhalla #2 avec les identités attendues. Une autre
installation nécessite une définition contrôlée. Les calculs couvrent toute la BOM active, pas
seulement les 47 occurrences physiques dotées d'une géométrie.
Périmètre et preuves : [motorisations et comparaison](docs/3d/MOTORISATIONS_SOUTENANCE.md).

## Présentation et navigation sur laptop

La vue électrique présente le résumé neutre **« Simulation électrique : scénario de démonstration
avec hypothèses techniques. »** Les détails sur la batterie BEV adaptée, les transformations non
dimensionnées et les coûts additionnels non établis restent accessibles dans **Hypothèses et limites
techniques**, fermé par défaut. Aucun résultat de calcul ou périmètre technique n'a été modifié.

À 1366 × 768 et 1280 × 720, la zone centrale et la colonne de droite défilent indépendamment.
**Composants et sources** défile à l'intérieur de la hauteur disponible du visualiseur, y compris
ses listes, sous-panneaux et licences. **Composants retirés** possède sa liste défilante avec pagination.
La molette sur la voiture conserve sa fonction de zoom ; utiliser le panneau ou la marge de la
zone centrale pour faire défiler leurs contenus.

L'import de manifeste et sa révision sont rangés dans **Composants et sources → Avancé**, fermé
par défaut. Il n'est pas nécessaire de l'ouvrir pour le parcours du jury. Les sources et licences
restent accessibles, et les crédits des modèles restent affichés dans la scène.

## Pièce sans modèle individuel

Un bouton **Localiser le sous-ensemble** apparaît seulement si l'ascendance de la BOM permet
de trouver un groupe réellement représenté. La pièce reste sélectionnée dans la BOM ; le message
précise que sa géométrie individuelle est absente. Exemple : soupape Fiat #12934 → Culasse,
module HV Aston #34760 → Batterie haute tension. Les deux arbres à cames Fiat #12932 sont désormais
liés à leurs deux emplacements existants, avec une quantité deux et deux clés d'instance distinctes.

Audit exhaustif de toutes les occurrences et causes : `docs/3d/COMPOSANTS_NON_VISIBLES.md`.
Captures après corrections : `docs/3d/captures/corrections-finales/`. Pour le régénérer en lecture seule :

```sh
PLAYWRIGHT_BROWSERS_PATH=/tmp/stp-3d-toolchain/browsers node docs/3d/tools/audit_visibility.mjs
```

## Reconstituer les fichiers locaux si nécessaire

Les originaux et copies préparées sont déjà installés et ignorés par Git. Pour les réinstaller depuis
les cinq pièces jointes conservées dans `~/Downloads`, utiliser le script d'installation, puis Vite
en fonctionnement et Chromium Playwright installé :

```sh
python3 docs/3d/tools/install_soutenance_assets.py
mkdir -p /tmp/stp-soutenance-rebuild
export PLAYWRIGHT_BROWSERS_PATH=/tmp/stp-3d-toolchain/browsers
node docs/3d/tools/bake_valhalla_pose.mjs /tmp/stp-soutenance-rebuild/valhalla-pose.json
python3 docs/3d/tools/prepare_soutenance_assets.py /tmp/stp-soutenance-rebuild
python3 docs/3d/tools/explode_soutenance.py
python3 docs/3d/tools/refresh_soutenance_clone.py
python3 docs/3d/tools/install_soutenance_assets.py --verify --verify-prepared
```

Cette chaîne utilise nos outils, sans exécuter les scripts des ZIP. Elle ne crée pas une nouvelle BOM.
Pour rejouer le parcours réel et produire les captures :

```sh
PLAYWRIGHT_BROWSERS_PATH=/tmp/stp-3d-toolchain/browsers \
STP_3D_MUTATIONS=1 node docs/3d/tools/validate_soutenance.mjs
```

Ce dernier parcours modifie temporairement uniquement la batterie du brouillon B de cette base de
démonstration et la restaure. Résultats, captures et limites : `docs/3d/captures/soutenance/` et
`docs/3d/VALIDATION_SOUTENANCE.md`.
