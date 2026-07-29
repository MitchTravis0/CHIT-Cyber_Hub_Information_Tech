import { createContext, useContext, type ReactNode } from 'react'

export interface SidebarValue {
  /** True when the side menu is hidden and only the header toggle brings it back. */
  hidden: boolean
  toggle: () => void
}

const SidebarContext = createContext<SidebarValue>({ hidden: false, toggle: () => {} })

/**
 * Lets a page header show the menu toggle without every tool having to pass the
 * state down. App owns the state, ToolShell and the shell pages read it here.
 * This lives in lib rather than shell so components can import it without the
 * two folders importing each other.
 */
export function SidebarProvider({ value, children }: { value: SidebarValue; children: ReactNode }) {
  return <SidebarContext.Provider value={value}>{children}</SidebarContext.Provider>
}

export function useSidebar(): SidebarValue {
  return useContext(SidebarContext)
}
