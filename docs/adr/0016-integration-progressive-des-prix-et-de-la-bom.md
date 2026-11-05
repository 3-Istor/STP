# 0016 — Intégration progressive des prix et de la BOM
Statut : Accepté — Date : 2026-11-05 — Décideurs : Joe (accord explicite dans le chat)

## Contexte
NLPF-68 démarre depuis le catalogue/historique de NLPF-67. NLPF-69 est sur la branche d'Arthur ;
les deux branches ont une migration 00002 différente et des ADR 0006–0008 différentes déjà acceptées.
AGENTS §11/13 exige les accords avant fusion ou reprise de branche. L'impact est décrit dans
`.specs/nlpf-67/integration-nlpf-69.md` ; aucun accord d'Arthur n'a été observé.

## Options
1. Livrer les capacités pricing indépendantes sur nlpf-68 et attendre une base BOM coordonnée
   pour les raccordements — avance sans modifier le travail d'Arthur ; livraison complète différée.
2. Avec l'accord du propriétaire, préparer une branche d'intégration dédiée — validation bout en
   bout plus tôt ; migration des historiques et contrats à décider avant toute fusion.
3. Recréer ici arbre/éditeur/snapshots — évite la dépendance ; double moteur interdit par le périmètre.

## Décision
**Proposition : option 1** en attendant les accords de l'option 2. Créer nlpf-68 depuis
4dcdb48 (autorisé par spec-execute) ; l'accord sur cette ADR n'autorise aucune fusion/reprise d'Arthur.

## Conséquences
- Fournir Source, ingestion, lectures, historiques, snapshots pricing, conversion et SSE testables
  indépendamment. Conserver les vrais raccordements BOM/budgets/éditeur et leur acceptation ouverts.
  Un test à interface contrôlée aide au développement, sans devenir une preuve d'intégration finale.
- Pricing conserve ses ports de lecture existants ; gel et lecture snapshot passent par des ports
  étroits supplémentaires. Un adapter vers le contrat NLPF-69 est préparé après partage convenu,
  sans modifier silencieusement son interface, ses stubs ou la sémantique de ses taux exacts.
- Avant chaque nouvelle migration, revalider son numéro sur la base convenue ; aucun numéro
  global réservé par cette ADR. Avant raccordement, procédure validée pour v1, v2 catalogue et v2 BOM,
  contrôlée sur bases jetables en préservant IDs, cotations, snapshots et leurs faits.
- Les références aux ADR divergentes citent chemin complet et branche de provenance dans le
  dossier d'intégration. Ne pas réécrire les textes acceptés ni renuméroter des migrations appliquées.
  Toute décision de réconciliation reste à proposer à l'équipe avant code correspondant.
- Tests des raccordements : vrai gel/couverture, inverse agrégé, budgets figés, prix/SSE dans
  l'éditeur et archive concurrente ; NOT RUN tant que la base dépendante n'est pas disponible.
- Retour arrière : conserver les branches et commits individuels ; aucun reset/rebase forcé ni
  purge d'une base d'équipe. Un futur pivot ne justifie pas une fusion anticipée ou un second moteur.
