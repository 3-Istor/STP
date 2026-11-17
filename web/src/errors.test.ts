import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { ApiError, errorMessage } from './api.ts'
import { errorPhrases } from './errors.ts'

test('un code connu donne la phrase française, un code inconnu garde le texte du serveur', () => {
  assert.equal(errorMessage(new ApiError(409, 'no budget rate for GBP', 'no_budget_rate')), errorPhrases.no_budget_rate)
  assert.equal(errorMessage(new ApiError(409, 'texte brut', 'code_futur')), 'texte brut')
  assert.equal(errorMessage(new ApiError(502, '502 Bad Gateway')), '502 Bad Gateway')
  assert.equal(errorMessage('boom'), 'boom')
})

// Chaque code de errorTable (Go) a sa phrase : sinon l'interface retombe sur l'anglais brut.
test('toute la table des codes de l’API a une phrase française', () => {
  const src = readFileSync(new URL('../../internal/httpapi/errorcode.go', import.meta.url), 'utf8')
  const codes = [...src.matchAll(/http\.Status\w+, "([a-z_]+)"\}/g)].map((m) => m[1])
  assert.ok(codes.length > 30, `codes lus: ${codes.length}`)
  for (const c of codes) assert.ok(errorPhrases[c], `phrase manquante pour ${c}`)
})
