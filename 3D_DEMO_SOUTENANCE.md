# Démonstration 3D — 18 novembre 2026

Branche : `feat/3d-pro-v2`. Les services de démonstration sont préparés séparément du Docker
habituel du projet. Le code Go, les migrations et les contrats API n'ont pas été modifiés.

## Ouvrir la démonstration déjà lancée

- [Fiat Multipla](http://127.0.0.1:41769/#/boms/1/structurer?tab=3d).
- [Aston Martin Valhalla](http://127.0.0.1:41769/#/boms/2/structurer?tab=3d).
- [Brouillon B : batterie remplaçable](http://127.0.0.1:41769/#/boms/3/structurer?tab=arbre-3d&node=60776).

Les installations vérifiées ouvrent le mode **Composants** : les modèles fournis sont chargés
depuis les manifestes d'occurrences. Les modes historiques Simple et Réaliste restent disponibles ;
le second utilise le modèle générique CarConcept et ne sert pas à présenter ces deux voitures.
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

1. Ouvrir la Fiat, laisser le mode Composants. Montrer sa silhouette et les crédits de l'auteur.
2. Glisser sur la voiture pour tourner, utiliser la molette pour zoomer, puis **Recentrer**.
3. Déplacer **Éclatement** à 50 puis 100 %. La coque se soulève, les organes et roues se dégagent.
4. Passer en **Arbre + 3D** et cliquer une roue dans l'éclaté : son occurrence et le panneau Détail
   apparaissent. Les quatre pneus ont des identités distinctes même s'ils partagent la même référence.
5. Rechercher une pièce mécanique dans l'arbre, par exemple le bloc moteur. La sélectionner : la
   géométrie liée est surlignée et cadrée. Dans **Composants et sources**, **Isoler la sélection**, puis
   **Tout afficher**. Sélectionner le groupe motopropulseur et utiliser **Focus sélection**.
6. **Échap**, puis **Réinitialiser** : revenir à la pose assemblée et à la caméra globale.
7. Ouvrir la Valhalla. Montrer l'extérieur, puis ouvrir **Composants et sources**.
8. Choisir successivement **Thermique**, **Électrique**, **Hybride** dans **Motorisation**. Le V8,
   les trois moteurs électriques et la batterie sont des représentations illustratives. Le filtre
   met les systèmes en évidence ; il ne simule pas un flux d'énergie ou une puissance.
9. Fermer le panneau, éclater, puis cliquer un moteur avant. En **Arbre + 3D**, voir son occurrence
   propre et les informations BOM. Revenir à **Réinitialiser** avant de quitter le véhicule.
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
