# NLPF-67/68 : catalogue fournisseurs et flux de prix expliqués

Le catalogue et les fournisseurs disposent désormais d’offres, de contrats, d’historiques et d’un flux de prix simulés persistants. Les prix et taux portent leur provenance, peuvent être figés et alimentent l’écran fournisseurs sans rechargement.

Cette PR regroupe tout le travail NLPF-67 et NLPF-68 : `nlpf-68` inclut les commits de `nlpf-67`.

### Changements
- Catalogue, fournisseurs, contrats/remises et statistiques annuelles ; interface fournisseurs.
- Source HTTP déterministe, 365 jours initiaux, ingestion atomique et reprise sans rattrapage.
- Snapshots avec contrôle de couverture et fermeture SQL au commit ; conversion rationnelle exacte.
- Évaluateurs purs de budgets/volume de référence ; SSE borné, reconnexion et repli daté.
- ADR 0006–0016, documentation, migrations additives et job CI `web-live`.

### Validation locale
- `make lint`, `make test` avec PostgreSQL 18, tests Go sous `-race`, `make sec` et build TypeScript/Vite réussis.
- `go mod tidy -diff` vide ; sqlc stable ; migrations up/reset/up sur base jetable.
- 60 parcours UI contrôlés et 12 parcours réels HTTP/ingestion/PostgreSQL/SSE/UI réussis sur Chromium, Firefox et WebKit, ordinateur/mobile.
- Smoke Compose via nginx : panne/repli/reprise, arrêt avec SSE ouvert et redémarrage sans régénération.
- Mesure : 30 000 nœuds pièce, 300 offres uniques ; tick ~68 ms, historique initial ~21 s. Le recalcul du véritable moteur BOM n’est pas mesuré.
- CI distante non encore vérifiée ; Safari macOS/iOS réel non testé. Deux incohérences préexistantes de suivi des specs (`nlpf-70`, `docker-compose-stack`) restent inchangées.

### Points à revoir avec Arthur avant fusion
**Livraison partielle : NLPF-68/T10 reste bloquée.** Le moteur/éditeur BOM de NLPF-69, les budgets persistés et publiés, le gel des contrats et le seed Multipla restent à raccorder. L’accord d’Arthur et une base commune validée ne sont pas encore disponibles.
Les historiques de migration 00002 et les ADR 0006–0008 divergent entre les branches : aucune reprise/fusion automatique ni modification d’un snapshot publié.

- [Rapport détaillé NLPF-68](https://github.com/3-Istor/STP/blob/nlpf-68/.specs/nlpf-68/delivery-report.md)
- [Raccordements NLPF-69 et conflits à traiter](https://github.com/3-Istor/STP/blob/nlpf-68/.specs/nlpf-68/integration-nlpf-69.md)
- [Livraison NLPF-67](https://github.com/3-Istor/STP/blob/nlpf-68/.specs/nlpf-67/delivery.md)
