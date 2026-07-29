import { useCallback, useEffect, useRef, useState } from 'react'
import { FileDown, FileUp, KeyRound, Lock, LockOpen, Plus, Trash2 } from 'lucide-react'
import {
  Button,
  CopyButton,
  ProgressBar,
  Textarea,
  TextInput,
  ToolShell,
  useToast,
} from '../../components'
import { downloadText } from '../../lib/download'
import { errorMessage } from '../../lib/format'
import {
  addAccount,
  createVault,
  currentCodes,
  exportVault,
  importVault,
  listAccounts,
  lockVault,
  removeAccount,
  totpStatus,
  unlockVault,
  type Account,
  type Code,
  type Status,
} from './api'
import {
  accountSubtitle,
  accountTitle,
  filterAccounts,
  groupCode,
  importSummary,
  secondsTone,
  sortAccounts,
  vaultFileName,
  withCodes,
} from './codes'

const REFRESH_MS = 1000

const NO_APP_MESSAGE =
  'The code vault needs the desktop app. Codes and seeds never leave the machine, so there is nothing to show in a browser.'

const MISMATCH_MESSAGE = 'Those two do not match. Type the same passphrase in both boxes.'

const HELP = (
  <>
    <p>
      This holds the two-factor seeds for accounts a team shares, such as the firewall's admin login
      or a vendor portal. It is not for your own personal accounts: those belong on your own phone,
      where only you have them.
    </p>
    <p className="mt-1.5">
      The vault is one encrypted file in the CHIT data folder. Anyone with the file and the
      passphrase can read every code in it, so treat the passphrase the way you would treat the
      accounts themselves. Export gives you that file, still encrypted, to put on a share or hand to
      a colleague. They Import it and type the same passphrase.
    </p>
    <p className="mt-1.5">
      To add an account, use the otpauth:// link the service shows next to its QR code, usually
      behind a "can't scan the code?" link. CHIT cannot read a QR code out of a picture, so pasting
      that link, or typing the secret it contains, is how a seed gets in.
    </p>
    <p className="mt-1.5">
      A code changes every 30 seconds and the bar shows how long the one on screen has left. If a
      code is refused, check this machine's clock: two factor codes are calculated from the time, and
      a clock more than a minute out will produce codes that never work.
    </p>
  </>
)

export default function TotpGeneratorPage() {
  const toast = useToast()

  const [status, setStatus] = useState<Status | null>(null)
  const [inApp, setInApp] = useState(true)
  const [error, setError] = useState('')

  const [passphrase, setPassphrase] = useState('')
  const [confirm, setConfirm] = useState('')
  const [confirmError, setConfirmError] = useState<string>()

  const [accounts, setAccounts] = useState<Account[]>([])
  const [codes, setCodes] = useState<Code[]>([])
  const [query, setQuery] = useState('')
  const [confirmId, setConfirmId] = useState<string | null>(null)

  const [adding, setAdding] = useState(false)
  const [uri, setUri] = useState('')
  const [secret, setSecret] = useState('')
  const [issuer, setIssuer] = useState('')
  const [label, setLabel] = useState('')
  const [addError, setAddError] = useState('')

  const [importFile, setImportFile] = useState<string | null>(null)
  const [importPass, setImportPass] = useState('')
  const [importError, setImportError] = useState('')
  const fileRef = useRef<HTMLInputElement>(null)

  const unlocked = status?.unlocked === true

  const refreshAccounts = useCallback(async () => {
    const list = await listAccounts()
    setAccounts(sortAccounts(list.accounts ?? []))
  }, [])

  useEffect(() => {
    totpStatus()
      .then((next) => {
        setInApp(next !== null)
        setStatus(next)
      })
      .catch((err) => setError(errorMessage(err)))
  }, [])

  useEffect(() => {
    if (!unlocked) {
      setCodes([])
      return
    }
    let stopped = false

    const tick = async () => {
      try {
        const set = await currentCodes()
        if (!stopped) setCodes(set.codes ?? [])
      } catch (err) {
        if (stopped) return
        // The vault locked itself. Say why, and drop back to the locked panel.
        setError(errorMessage(err))
        setStatus((prev) => (prev === null ? prev : { ...prev, unlocked: false }))
      }
    }

    void tick()
    const timer = setInterval(() => void tick(), REFRESH_MS)
    return () => {
      stopped = true
      clearInterval(timer)
    }
  }, [unlocked])

  useEffect(() => {
    if (!unlocked) {
      setAccounts([])
      return
    }
    refreshAccounts().catch((err) => setError(errorMessage(err)))
  }, [unlocked, refreshAccounts])

  const onCreate = async () => {
    if (passphrase !== confirm) {
      setConfirmError(MISMATCH_MESSAGE)
      return
    }
    setConfirmError(undefined)
    try {
      setStatus(await createVault(passphrase))
      setPassphrase('')
      setConfirm('')
      setError('')
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  const onUnlock = async () => {
    try {
      setStatus(await unlockVault(passphrase))
      setPassphrase('')
      setError('')
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  const onLock = async () => {
    try {
      setStatus(await lockVault())
      setError('')
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  const onAdd = async () => {
    try {
      await addAccount({ uri: uri.trim(), secret: secret.trim(), issuer, label })
      await refreshAccounts()
      setUri('')
      setSecret('')
      setIssuer('')
      setLabel('')
      setAdding(false)
      setAddError('')
      toast.push('success', 'Account saved to the vault.')
    } catch (err) {
      setAddError(errorMessage(err))
    }
  }

  const onRemove = async (id: string) => {
    setConfirmId(null)
    try {
      setStatus(await removeAccount(id))
      await refreshAccounts()
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  const onExport = async () => {
    try {
      downloadText(vaultFileName(new Date().toISOString()), await exportVault(), 'application/json')
    } catch (err) {
      toast.push('error', errorMessage(err))
    }
  }

  const onImport = async () => {
    if (importFile === null) return
    try {
      const report = await importVault(importFile, importPass)
      await refreshAccounts()
      setImportFile(null)
      setImportPass('')
      setImportError('')
      toast.push('success', importSummary(report.added, report.skipped))
    } catch (err) {
      setImportError(errorMessage(err))
    }
  }

  const shown = filterAccounts(withCodes(accounts, codes), query)

  return (
    <ToolShell
      title="TOTP Code Generator"
      description="Hold the two-factor seeds for shared accounts in an encrypted vault and read the current code."
      help={HELP}
      actions={
        unlocked ? (
          <>
            <Button variant="primary" onClick={() => setAdding(true)} icon={<Plus size={14} aria-hidden />}>
              Add account
            </Button>
            <Button onClick={() => void onExport()} icon={<FileDown size={14} aria-hidden />}>
              Export vault
            </Button>
            <Button onClick={() => void onLock()} icon={<Lock size={14} aria-hidden />}>
              Lock
            </Button>
          </>
        ) : undefined
      }
    >
      <div className="flex max-w-3xl flex-col gap-4">
        {!inApp && <p className="text-sm text-warn">{NO_APP_MESSAGE}</p>}

        {error !== '' && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-danger"
          >
            {error}
          </p>
        )}

        {status !== null && !status.hasVault && (
          <form
            className="flex max-w-sm flex-col gap-3 rounded border border-border bg-surface-2 px-3 py-3"
            onSubmit={(event) => {
              event.preventDefault()
              void onCreate()
            }}
          >
            <p className="text-sm text-fg">
              There is no vault on this machine yet. Create one, or import a vault file a colleague
              exported.
            </p>
            <TextInput
              label="Passphrase"
              type="password"
              value={passphrase}
              onChange={(event) => setPassphrase(event.target.value)}
              autoComplete="new-password"
              hint="At least 12 characters. Everyone who needs these codes has to know it, so pick something the team can share and remember."
            />
            <TextInput
              label="Passphrase again"
              type="password"
              value={confirm}
              onChange={(event) => setConfirm(event.target.value)}
              autoComplete="new-password"
              error={confirmError}
            />
            <div className="flex flex-wrap gap-2">
              <Button type="submit" variant="primary" icon={<KeyRound size={14} aria-hidden />}>
                Create vault
              </Button>
              <Button onClick={() => fileRef.current?.click()} icon={<FileUp size={14} aria-hidden />}>
                Import a vault file
              </Button>
            </div>
          </form>
        )}

        {status !== null && status.hasVault && !status.unlocked && (
          <form
            className="flex max-w-sm flex-col gap-3 rounded border border-border bg-surface-2 px-3 py-3"
            onSubmit={(event) => {
              event.preventDefault()
              void onUnlock()
            }}
          >
            {status.note !== '' && (
              <p role="alert" className="text-xs text-danger">
                {status.note}
              </p>
            )}
            <TextInput
              label="Passphrase"
              type="password"
              value={passphrase}
              onChange={(event) => setPassphrase(event.target.value)}
              autoComplete="current-password"
              autoFocus
            />
            <div className="flex flex-wrap gap-2">
              <Button type="submit" variant="primary" icon={<LockOpen size={14} aria-hidden />}>
                Unlock
              </Button>
              <Button onClick={() => fileRef.current?.click()} icon={<FileUp size={14} aria-hidden />}>
                Import a vault file
              </Button>
            </div>
          </form>
        )}

        {importFile !== null && (
          <form
            className="flex max-w-sm flex-col gap-3 rounded border border-border bg-surface-2 px-3 py-3"
            onSubmit={(event) => {
              event.preventDefault()
              void onImport()
            }}
          >
            <p className="text-xs text-fg-muted">
              Type the passphrase that vault file was exported with. Its accounts are added to the
              vault open on this machine.
            </p>
            <TextInput
              label="Passphrase of the file"
              type="password"
              value={importPass}
              onChange={(event) => setImportPass(event.target.value)}
              autoComplete="off"
              error={importError === '' ? undefined : importError}
            />
            <div className="flex flex-wrap gap-2">
              <Button type="submit" variant="primary">
                Add its accounts
              </Button>
              <Button
                onClick={() => {
                  setImportFile(null)
                  setImportPass('')
                  setImportError('')
                }}
              >
                Cancel
              </Button>
            </div>
          </form>
        )}

        <input
          ref={fileRef}
          type="file"
          accept="application/json,.json"
          className="hidden"
          onChange={(event) => {
            const file = event.target.files?.[0]
            event.target.value = ''
            if (file === undefined) return
            void file.text().then((text) => {
              setImportError('')
              setImportFile(text)
            })
          }}
        />

        {unlocked && adding && (
          <form
            className="flex flex-col gap-3 rounded border border-border bg-surface-2 px-3 py-3"
            onSubmit={(event) => {
              event.preventDefault()
              void onAdd()
            }}
          >
            <Textarea
              id="otpauth-link"
              label="Paste the otpauth:// link"
              hint={'Most sites show this behind a "can\'t scan the code?" link next to the QR code.'}
              rows={3}
              value={uri}
              onChange={(event) => setUri(event.target.value)}
              spellCheck={false}
              autoComplete="off"
              placeholder="otpauth://totp/Firewall:admin?secret=..."
              className="resize-y font-mono text-xs"
            />

            <p className="text-xs text-fg-muted">or</p>
            <TextInput
              label="Secret (base32)"
              value={secret}
              onChange={(event) => setSecret(event.target.value)}
              spellCheck={false}
              autoComplete="off"
              className="font-mono"
            />
            <TextInput
              label="Issuer"
              value={issuer}
              onChange={(event) => setIssuer(event.target.value)}
              placeholder="Firewall"
            />
            <TextInput
              label="Label"
              value={label}
              onChange={(event) => setLabel(event.target.value)}
              placeholder="admin@head-office"
            />

            {addError !== '' && (
              <p role="alert" className="text-xs text-danger">
                {addError}
              </p>
            )}

            <div className="flex flex-wrap gap-2">
              <Button type="submit" variant="primary">
                Add
              </Button>
              <Button
                onClick={() => {
                  setAdding(false)
                  setAddError('')
                }}
              >
                Cancel
              </Button>
            </div>
          </form>
        )}

        {unlocked && accounts.length > 5 && (
          <div className="max-w-sm">
            <TextInput
              label="Filter accounts"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Filter by issuer or label"
              spellCheck={false}
              autoComplete="off"
            />
          </div>
        )}

        {unlocked && shown.length === 0 && (
          <p className="text-sm text-fg-muted">
            {accounts.length === 0
              ? 'No accounts in this vault yet. Press Add account and paste the otpauth:// link the service showed you.'
              : 'No account matches that filter. Clear the filter box.'}
          </p>
        )}

        {unlocked &&
          shown.map((account) => (
            <div
              key={account.id}
              className="flex flex-col gap-2 rounded border border-border bg-surface-2 px-3 py-3"
            >
              <div className="flex flex-wrap items-start justify-between gap-2">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium text-fg">{accountTitle(account)}</p>
                  {accountSubtitle(account) !== '' && (
                    <p className="truncate text-xs text-fg-muted">{accountSubtitle(account)}</p>
                  )}
                </div>
                {confirmId === account.id ? (
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-xs text-fg">
                      Remove {accountTitle(account)}? The seed is deleted from the vault and cannot
                      be recovered from CHIT.
                    </span>
                    <Button size="sm" variant="danger" onClick={() => void onRemove(account.id)}>
                      Remove
                    </Button>
                    <Button size="sm" onClick={() => setConfirmId(null)}>
                      Cancel
                    </Button>
                  </div>
                ) : (
                  <Button
                    size="sm"
                    variant="ghost"
                    aria-label={`Remove ${accountTitle(account)} ${account.label}`}
                    onClick={() => setConfirmId(account.id)}
                    icon={<Trash2 size={14} aria-hidden />}
                  />
                )}
              </div>

              {account.code === null ? (
                <p className="text-xs text-fg-muted">Working out the code.</p>
              ) : (
                <>
                  <div className="flex flex-wrap items-center gap-3">
                    <span className="font-mono text-3xl tracking-widest tabular-nums text-fg">
                      {groupCode(account.code.code)}
                    </span>
                    <CopyButton value={account.code.code} label="Copy" />
                  </div>
                  <ProgressBar
                    value={account.code.expiresIn}
                    max={account.code.period}
                    label={`${account.code.expiresIn} seconds left`}
                  />
                  {secondsTone(account.code.expiresIn) === 'warn' && (
                    <p className="text-xs text-warn">
                      This code is about to change. Wait for the next one.
                    </p>
                  )}
                </>
              )}
            </div>
          ))}
      </div>
    </ToolShell>
  )
}
