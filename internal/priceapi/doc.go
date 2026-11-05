// Package priceapi est l'API de prix externe (simulée), servie par son propre
// binaire cmd/priceapi dans un conteneur dédié (voir ADR 0005).
//
// Le back ne l'importe jamais : il l'appellera en HTTP via un adapter qui valide
// et normalise les cotations (AGENTS §2). Les cinq opérations de simulation reçoivent
// leurs définitions et contexte explicites (ADR 0014), sans catalogue répliqué.
package priceapi
