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

test('un code qui nomme le champ en cause garde le texte du serveur après la phrase', () => {
  assert.equal(errorMessage(new ApiError(422, 'quantity must be > 0', 'invalid_threshold')), `${errorPhrases.invalid_threshold} (quantity must be > 0)`)
  assert.equal(errorMessage(new ApiError(409, 'no rate', 'no_rate')), `${errorPhrases.no_rate} (no rate)`)
})

test('bad_request et invalid_input : pas de Go interne, message français ou phrase seule', () => {
  const m = (msg: string, code = 'invalid_input') => errorMessage(new ApiError(400, msg, code))
  assert.equal(m('create bom: empty name: invalid bom input'), 'Le nom ne peut pas être vide.')
  assert.match(m('bom 3 has a global budget of 10 EUR: remove it before switching to USD (ADR 0045): invalid bom input'), /avant de passer en USD/)
  assert.match(m('invalid JSON body: json: cannot unmarshal number 100000000000000000000 into Go struct field createBOMRequest.volume of type int32: bad request', 'bad_request'), /1 et 2 147 483 647/)
  assert.equal(m('send currency and volume together: bad request', 'bad_request'), 'Indiquez la devise et le volume ensemble.')
  assert.equal(m("can't convert abc to decimal", 'bad_request'), 'Un nombre saisi n’est pas valide.')
  assert.equal(m('quelque chose d’inédit', 'bad_request'), errorPhrases.bad_request)
})
