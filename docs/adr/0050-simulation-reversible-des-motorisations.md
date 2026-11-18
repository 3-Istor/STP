# 0050 — Simulation réversible des motorisations
Statut : Proposé — Date : 2026-11-18 — Décideurs : (à compléter par un humain)

Renumérotation lors de l’intégration de `main` : ancien numéro 0042 sur `feat/3d-pro-v2`, déjà utilisé pour une autre décision sur `main`. Le contenu de la décision reste inchangé.

## Contexte
La mission de soutenance exige trois configurations Valhalla avec BOM actives et coûts Go réels.
Le filtre 3D actuel ne change aucun coût. `docs/features.md:77` prévoit des variantes de BOM,
mais pas leur simulation économique temporaire. La référence hybride et les snapshots restent intacts.

## Options
1. **Simulation Go en mémoire, sans stockage** : route additive de comparaison, exclusions de
   sous-arbres explicitement identifiées et validées, une lecture des faits/prix/taux, puis
   `ComputeTotals` sur chaque arbre retenu. Pas de migration ni duplication du calcul frontend.
2. **Copies de travail persistées** : réutiliser l'édition des brouillons, mais il faut copier
   structure, contexte de prix et réglages ; davantage d'écritures et de règles de cycle de vie.
3. **Conserver le filtre visuel seul** : aucune évolution API, mais ne répond pas au comparateur demandé.

## Décision
**Option 1 et périmètre ci-dessous validés explicitement par l'utilisateur le 18 novembre 2026**
(réponse : « Valider l’option 1 et ces hypothèses »). Le statut formel et le nom du décideur
restent à renseigner par un humain conformément aux règles du dépôt.
L'API reçoit une définition explicite des scénarios, contrôle les IDs et exclusions, recalcule
l'arbre complet actif et rend coûts, écarts, totaux par nœud, sources et différences de composition.
Pas de changement des formules monétaires ni des contrats existants. Une même entrée de calcul
alimente les trois scénarios ; les prix manquants restent des erreurs nommées, jamais des zéros.

Périmètre proposé sur la référence Valhalla vérifiée : hybride conserve tout ; thermique retire
la chaîne de traction électrique #34537 ; électrique retire V8 #32413, échappement #35285,
carburant #35392. Descendants retirés une seule fois, composants communs inchangés. DCT #35054,
transmission #35205 et refroidissement #35207 sont conservés comme hypothèses communes faute
de découpage technique plus fin vérifié. Les identités attendues doivent être contrôlées contre
l'arbre courant ; aucun rapprochement automatique par nom ne remplace un manifeste.
La batterie hybride existante reste chiffrée, mais n'atteste pas une batterie BEV adaptée : coût
des composants disponibles, architecture électrique de conception incomplète et ajouts non chiffrés
explicitement indiqués. Aucun prix, dimensionnement ou composant manquant inventé.

## Conséquences
Sélecteur de simulation distinct du filtre visuel ; BOM, Synthèse et 3D utilisent le scénario actif,
lecture seule pendant la simulation. Quitter restitue la référence sans écriture ni restauration SQL.
Tests : recalcul Go complet, communs, quantités, change, absence de double comptage/prix, référence
inchangée, puis parcours navigateur et preuves chiffrées. Plan : définition et validation → service
Go et route additive → intégration React/3D/comparaison → tests et captures.
Impact futur : on pourra persister une définition versionnée après décision séparée ; aucun état
de simulation n'est ajouté aux snapshots. Retour arrière : retirer route et composant de simulation.
