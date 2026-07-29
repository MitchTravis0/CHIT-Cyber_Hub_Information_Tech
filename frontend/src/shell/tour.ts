// The first-run tour, as data. The overlay that renders it is JSX and cannot be
// tested without a DOM; these steps can be, including the one thing in here that
// is not cosmetic: naming every tool that opens an inbound port.
import { MOD_LABEL } from './nav.ts'

export interface TourStep {
  id: string
  title: string
  body: string
  /** Tool ids this step talks about. Pinned against the registry by a test, so
   *  renaming a tool cannot leave the tour pointing at nothing. */
  tools?: string[]
}

/** The three tools that accept an incoming connection while they are running.
 *  Phase 5 and Phase 9 both asked for this to be in the tour by name. */
export const LISTENER_TOOLS = ['lan-file-drop', 'port-listener', 'lan-throughput']

export const TOUR_STEPS: TourStep[] = [
  {
    id: 'what',
    title: 'One file, no installer',
    body:
      'CHIT is a toolbox of small network and diagnostic tools in a single executable. Nothing was installed, nothing was added to this machine, and nothing here needs administrator or root. Delete the file and CHIT is gone.',
  },
  {
    id: 'find',
    title: 'Finding a tool',
    body:
      `Every tool is one click away in the sidebar, grouped by the job it does. Faster still, press ${MOD_LABEL}+K anywhere and type what you want: the tool's name, or the command it replaces, such as nslookup, ipconfig or tracert.`,
  },
  {
    id: 'favourites',
    title: 'Favourites and recents',
    body:
      'The star beside a tool pins it to the top of the home screen. The last few tools you opened appear there too, so the ones you use on every job are always in the same place. Both are saved between sessions.',
  },
  {
    id: 'listeners',
    title: 'Three tools open a port. Stop them when you are done',
    body:
      'Almost everything here only makes outgoing connections. LAN File Drop, Port Listener and LAN Throughput Test are the exceptions: while they are running, another machine on the network can connect to this one. That is the whole point of them, and it is why your firewall will ask. Each one shows a Stop button the entire time it is listening. Press it when you have finished, and nothing is listening again.',
    tools: LISTENER_TOOLS,
  },
  {
    id: 'data',
    title: 'Where your data lives',
    body:
      "Saved device lists, snippets, checklists and your settings are plain JSON files in this machine's config folder. Settings shows you the exact path. Put an empty file named portable.txt next to the executable and CHIT keeps everything in a folder beside itself instead, which is what you want on a USB stick.",
  },
]
