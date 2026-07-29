// frontend/wailsjs/runtime/runtime is the committed Wails runtime, not a
// generated binding, so it is safe to import (useJob and CopyButton do the
// same). BrowserOpenURL sends the link to the machine's normal browser instead
// of the app's webview, which is the required behaviour here: a vendor support
// page must not be trapped in a window with no address bar, no password manager
// and no back button.
import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime'

/** False when there is nothing to open with, which is the vite dev server. */
export function openExternal(url: string): boolean {
  try {
    BrowserOpenURL(url)
    return true
  } catch {
    return false
  }
}
