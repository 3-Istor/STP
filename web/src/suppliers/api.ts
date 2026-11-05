export async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    ...options,
    headers: { ...(options.body ? { 'Content-Type': 'application/json' } : {}), ...options.headers },
  })
  const body: unknown = await response.json().catch(() => null)
  if (!response.ok) {
    const message = body && typeof body === 'object' && 'error' in body && typeof body.error === 'string'
      ? body.error : `Requête indisponible (${response.status})`
    throw new Error(message)
  }
  if (body === null) throw new Error('Le serveur a renvoyé une réponse vide.')
  return body as T
}
export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Une erreur empêche cette action.'
}
