import { useEffect, useRef, useState, type FormEvent } from 'react'

// Fenêtres de confirmation et de saisie de l'app : jamais window.confirm / prompt / alert, dont
// l'affichage dépend du navigateur et sort de la direction artistique de l'app.

export type Options = { title: string; message?: string; confirmLabel: string; danger?: boolean }
export type PromptOptions = Options & { label: string; initial: string }
export type Request =
  | (Options & { kind: 'confirm'; resolve: (ok: boolean) => void })
  | (PromptOptions & { kind: 'prompt'; resolve: (value: string | null) => void })

// onDone(null) : annulé ; onDone(texte) : validé (texte vide pour une confirmation).
export function AppDialog({ request, onDone }: { request: Request; onDone: (value: string | null) => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const [value, setValue] = useState(request.kind === 'prompt' ? request.initial : '')
  useEffect(() => {
    if (dialog.current && !dialog.current.open) dialog.current.showModal()
  }, [])
  function submit(e: FormEvent) {
    e.preventDefault()
    onDone(request.kind === 'prompt' ? value.trim() : '')
  }
  return (
    <dialog
      ref={dialog}
      className="form-dialog app-dialog"
      aria-labelledby="app-dialog-title"
      onCancel={(e) => {
        e.preventDefault()
        onDone(null)
      }}
    >
      <form onSubmit={submit}>
        <div className="dialog-heading">
          <h2 id="app-dialog-title">{request.title}</h2>
        </div>
        {request.message && <p className="app-dialog-message">{request.message}</p>}
        {request.kind === 'prompt' && (
          <label>
            {request.label}
            <input autoFocus required value={value} onChange={(e) => setValue(e.target.value)} />
          </label>
        )}
        <div className="form-actions">
          <button type="button" className="secondary-button" autoFocus={request.kind === 'confirm'} onClick={() => onDone(null)}>
            Annuler
          </button>
          <button type="submit" className={request.danger ? 'primary-button danger-button' : 'primary-button'}>
            {request.confirmLabel}
          </button>
        </div>
      </form>
    </dialog>
  )
}
