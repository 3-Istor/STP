import { useCallback, useState } from 'react'
import { AppDialog, type Options, type PromptOptions, type Request } from './AppDialog.tsx'

// useAppDialog : `confirm` résout true/false, `prompt` la saisie (ou null si annulée). Le composant
// qui l'utilise rend `element` là où la fenêtre doit vivre (une seule à la fois).
export function useAppDialog() {
  const [request, setRequest] = useState<Request | null>(null)
  const confirm = useCallback(
    (o: Options) => new Promise<boolean>((resolve) => setRequest({ ...o, kind: 'confirm', resolve })),
    [],
  )
  const prompt = useCallback(
    (o: PromptOptions) => new Promise<string | null>((resolve) => setRequest({ ...o, kind: 'prompt', resolve })),
    [],
  )
  const element = request ? (
    <AppDialog
      key={request.title}
      request={request}
      onDone={(value) => {
        if (request.kind === 'confirm') request.resolve(value !== null)
        else request.resolve(value)
        setRequest(null)
      }}
    />
  ) : null
  return { confirm, prompt, element }
}
