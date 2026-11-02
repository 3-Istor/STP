// Package priceapi est l'API de prix externe (simulée), servie par son propre
// binaire cmd/priceapi dans un conteneur dédié (voir ADR 0005).
//
// Le back ne l'importe jamais : il l'appellera en HTTP via un adapter qui valide
// et normalise les cotations (AGENTS §2). Pour l'instant seul GET /healthz existe ;
// le simulateur et ses routes de cotations arrivent avec live-price-feed.
package priceapi
