/**
 * The offline lookups: pinouts, ports, status codes, channels, subnets, beep
 * codes and SLA downtime.
 *
 * Two of the seven cards are generated rather than typed (the subnet table and
 * the SLA table), because a hand-typed table of masks or downtimes is wrong in
 * a way nobody notices. The port list was checked against /etc/services and the
 * status codes against python's http.HTTPStatus while this file was written.
 */

export type CardId =
  | 'rj45'
  | 'ports'
  | 'http-status'
  | 'wifi-channels'
  | 'subnet-table'
  | 'beep-codes'
  | 'sla'

export interface CardEntry {
  /** Unique across every card: `${cardId}-${index}`. */
  id: string
  /** The thing you look up: '3389', '404', '/24', '1'. */
  key: string
  /** What it is: 'Remote Desktop', 'Not Found'. */
  label: string
  /** The remaining columns, in the card's column order. */
  extra: string[]
  /** Every searchable word about this entry, lowercased, space separated. */
  haystack: string
}

export interface CardDef {
  id: CardId
  name: string
  /** One line under the card name. */
  blurb: string
  /** A lucide-react export name for the picker button. */
  icon: string
  /** Column headers. columns.length === entry.extra.length + 2. */
  columns: string[]
  entries: CardEntry[]
  /** Sentences shown under the table. */
  notes: string[]
  /** Words that find this whole card, beyond what its rows say. */
  keywords: string[]
}

export interface SearchHit {
  card: CardDef
  entry: CardEntry
}

/** Rows the search shows before it stops and says so. */
export const MAX_SEARCH_HITS = 100

function entry(cardId: CardId, index: number, key: string, label: string, ...extra: string[]): CardEntry {
  return {
    id: `${cardId}-${index}`,
    key,
    label,
    extra,
    haystack: [key, label, ...extra].join(' ').toLowerCase(),
  }
}

function build(cardId: CardId, rows: string[][]): CardEntry[] {
  return rows.map((row, index) => entry(cardId, index, row[0], row[1], ...row.slice(2)))
}

function group(value: number): string {
  return String(value).replace(/\B(?=(\d{3})+(?!\d))/g, ',')
}

// --- RJ45 ------------------------------------------------------------------

const RJ45_ROWS: string[][] = [
  ['1', 'White/Orange', 'White/Green', 'Transmit +'],
  ['2', 'Orange', 'Green', 'Transmit -'],
  ['3', 'White/Green', 'White/Orange', 'Receive +'],
  ['4', 'Blue', 'Blue', 'Not used below gigabit'],
  ['5', 'White/Blue', 'White/Blue', 'Not used below gigabit'],
  ['6', 'Green', 'Orange', 'Receive -'],
  ['7', 'White/Brown', 'White/Brown', 'Not used below gigabit'],
  ['8', 'Brown', 'Brown', 'Not used below gigabit'],
]

// --- Ports -----------------------------------------------------------------

const PORT_ROWS: string[][] = [
  ['20', 'FTP data', 'TCP', 'The second connection FTP opens. Passive FTP uses a high port instead, which is why FTP and firewalls fight.'],
  ['21', 'FTP control', 'TCP', 'Plain text, including the password. Use SFTP on 22 instead.'],
  ['22', 'SSH and SFTP', 'TCP', 'Also SCP. One port for a shell and for file transfer.'],
  ['23', 'Telnet', 'TCP', 'Plain text. Still on switch and UPS management interfaces that should have it turned off.'],
  ['25', 'SMTP', 'TCP', 'Server to server mail. Most ISPs block outbound 25 from client networks, which is why a printer that scans to email cannot send.'],
  ['53', 'DNS', 'TCP and UDP', 'UDP for normal queries, TCP for anything over 512 bytes and for zone transfers.'],
  ['67', 'DHCP server', 'UDP', 'The server listens here. A second DHCP server on a LAN is the classic cause of wrong addresses.'],
  ['68', 'DHCP client', 'UDP', 'The client listens here for the offer.'],
  ['69', 'TFTP', 'UDP', 'No authentication at all. Used for switch firmware and PXE boot.'],
  ['80', 'HTTP', 'TCP', 'Unencrypted web. A device that only offers 80 should not be reachable from outside.'],
  ['88', 'Kerberos', 'TCP and UDP', 'Active Directory logins. Breaks when the clock is more than 5 minutes out.'],
  ['110', 'POP3', 'TCP', 'Downloads and usually deletes. If a user loses mail on one device, check for POP3.'],
  ['123', 'NTP', 'UDP', 'Time. Blocked outbound at many sites, which is why domain members drift.'],
  ['135', 'RPC endpoint mapper', 'TCP', 'Windows asks here which high port a service is really on. Never expose it.'],
  ['137', 'NetBIOS name service', 'UDP', 'Legacy Windows name lookup, before DNS did the job.'],
  ['138', 'NetBIOS datagram', 'UDP', 'Legacy browsing and announcements.'],
  ['139', 'NetBIOS session', 'TCP', 'Legacy file sharing. Modern Windows uses 445.'],
  ['143', 'IMAP', 'TCP', 'Leaves mail on the server. 993 is the encrypted one.'],
  ['161', 'SNMP', 'UDP', 'Monitoring reads. Community "public" on a switch is a finding, not a setting.'],
  ['162', 'SNMP trap', 'UDP', 'The device pushing an alert at the monitoring server.'],
  ['389', 'LDAP', 'TCP and UDP', 'Directory queries against one domain. Plain text unless StartTLS is used.'],
  ['443', 'HTTPS', 'TCP', 'Encrypted web, and the port nearly every VPN and cloud agent tunnels through.'],
  ['445', 'SMB file sharing', 'TCP', 'Windows shares and printer shares. Blocked at every sane firewall boundary.'],
  ['465', 'SMTPS (submissions)', 'TCP', 'Mail submission wrapped in TLS from the start. 587 is the other way of doing the same job.'],
  ['500', 'IKE', 'UDP', 'The first half of an IPsec VPN. If it is blocked, the tunnel never starts.'],
  ['514', 'Syslog', 'UDP', 'Where network devices send their logs. TCP 514 is the old rsh service, not syslog.'],
  ['515', 'LPD printing', 'TCP', 'The old Unix print protocol. Still on print servers and older copiers.'],
  ['587', 'SMTP submission', 'TCP', 'What a client or a scanner should use to send mail, with authentication.'],
  ['623', 'IPMI and RMCP', 'UDP', 'Out-of-band server management. Must never be reachable from a user network.'],
  ['631', 'IPP', 'TCP', 'Internet Printing Protocol, and the CUPS web interface on Linux and macOS.'],
  ['636', 'LDAPS', 'TCP', 'LDAP over TLS. Needs a certificate the client trusts, which is the usual failure.'],
  ['993', 'IMAPS', 'TCP', 'Encrypted IMAP. This is the one to use.'],
  ['995', 'POP3S', 'TCP', 'Encrypted POP3.'],
  ['1194', 'OpenVPN', 'UDP', 'Default OpenVPN port. Often moved to 443 to get through hotel Wi-Fi.'],
  ['1433', 'Microsoft SQL Server', 'TCP', 'Named instances use a dynamic port and ask UDP 1434 which one, which breaks through firewalls.'],
  ['1521', 'Oracle database', 'TCP', 'The Oracle listener. IANA has this port registered to something else entirely.'],
  ['1701', 'L2TP', 'UDP', 'Used with IPsec. Needs 500 and 4500 open as well.'],
  ['1723', 'PPTP', 'TCP', 'Obsolete and broken. If you find it, plan to replace it.'],
  ['1900', 'SSDP and UPnP', 'UDP', 'How TVs, printers and casting devices announce themselves. Multicast to 239.255.255.250.'],
  ['3128', 'Squid proxy', 'TCP', 'The usual proxy port on Linux. 8080 is the other one.'],
  ['3268', 'Global Catalog', 'TCP', 'Active Directory search across the whole forest. Port 389 only sees one domain.'],
  ['3306', 'MySQL and MariaDB', 'TCP', 'Should never face the internet.'],
  ['3389', 'Remote Desktop', 'TCP', 'RDP. The single most attacked port on the internet: never open it, use a VPN.'],
  ['4500', 'IPsec NAT traversal', 'UDP', 'Where an IPsec tunnel moves to when there is NAT in the path, which there almost always is.'],
  ['5060', 'SIP', 'TCP and UDP', 'VoIP call setup, unencrypted. One-way audio is nearly always NAT, not this port.'],
  ['5061', 'SIP over TLS', 'TCP', 'Encrypted call setup.'],
  ['5353', 'mDNS (Bonjour)', 'UDP', 'How printers and Chromecasts are found. Does not cross a router, which is why the printer vanishes on another VLAN.'],
  ['5432', 'PostgreSQL', 'TCP', 'Should never face the internet.'],
  ['5900', 'VNC', 'TCP', 'Screen sharing. 5901 is display :1, 5902 is :2, and so on.'],
  ['6379', 'Redis', 'TCP', 'No authentication by default in older versions. A frequent breach route.'],
  ['8080', 'HTTP alternate', 'TCP', 'Proxies, Tomcat, and the second web interface on half the appliances in a rack.'],
  ['8443', 'HTTPS alternate', 'TCP', 'Management interfaces on firewalls, NAS boxes and hypervisors.'],
  ['9100', 'Raw printing (JetDirect)', 'TCP', 'Send text straight to a network printer. The Raw Printer Test tool talks to this one.'],
  ['27017', 'MongoDB', 'TCP', 'Should never face the internet.'],
]

// --- HTTP status codes -----------------------------------------------------

const HTTP_ROWS: string[][] = [
  ['100', 'Continue', 'Informational', 'The server is happy for the client to send the body. You will never see this in a browser.'],
  ['101', 'Switching Protocols', 'Informational', 'Usually a WebSocket starting up.'],
  ['200', 'OK', 'Success', 'It worked.'],
  ['201', 'Created', 'Success', 'Something new was made, typically by an API.'],
  ['202', 'Accepted', 'Success', 'Taken for processing, not finished yet.'],
  ['204', 'No Content', 'Success', 'It worked and there is deliberately nothing to show.'],
  ['206', 'Partial Content', 'Success', 'A range request. Normal for downloads that resume and for video.'],
  ['301', 'Moved Permanently', 'Redirect', 'The address changed for good. Browsers cache this hard, which is why a wrong 301 is painful.'],
  ['302', 'Found', 'Redirect', 'A temporary move. The usual redirect to a login page.'],
  ['303', 'See Other', 'Redirect', 'Go and fetch the result somewhere else, with a GET.'],
  ['304', 'Not Modified', 'Redirect', 'Your cached copy is still good. Not an error.'],
  ['307', 'Temporary Redirect', 'Redirect', 'Like 302 but the method is kept, so a POST stays a POST.'],
  ['308', 'Permanent Redirect', 'Redirect', 'Like 301 but the method is kept.'],
  ['400', 'Bad Request', 'Client error', 'The server could not make sense of the request at all.'],
  ['401', 'Unauthorized', 'Client error', 'Not signed in. 403 means signed in but not allowed.'],
  ['403', 'Forbidden', 'Client error', 'Signed in and still not allowed, or the file permissions are wrong on the server.'],
  ['404', 'Not Found', 'Client error', 'The address is wrong or the page is gone. The server itself is fine.'],
  ['405', 'Method Not Allowed', 'Client error', 'The right address, the wrong verb: a POST where only GET is accepted.'],
  ['408', 'Request Timeout', 'Client error', 'The client took too long to send the request.'],
  ['409', 'Conflict', 'Client error', 'Two changes collided, or the thing already exists.'],
  ['410', 'Gone', 'Client error', 'Deliberately removed, and it is not coming back.'],
  ['413', 'Content Too Large', 'Client error', 'The upload is bigger than the server accepts. Check the upload limit, not the file.'],
  ['414', 'URI Too Long', 'Client error', 'Usually a redirect loop that keeps appending to the address.'],
  ['415', 'Unsupported Media Type', 'Client error', 'The server will not take that content type.'],
  ['418', "I'm a Teapot", 'Client error', 'A joke from 1998 that real servers occasionally return. Not a fault.'],
  ['422', 'Unprocessable Content', 'Client error', 'Well formed, but the values are wrong. Common from APIs.'],
  ['429', 'Too Many Requests', 'Client error', 'Rate limited. Back off and retry more slowly.'],
  ['431', 'Request Header Fields Too Large', 'Client error', 'Usually an enormous cookie. Clearing site data fixes it.'],
  ['451', 'Unavailable For Legal Reasons', 'Client error', 'Blocked by law or by a court order.'],
  ['500', 'Internal Server Error', 'Server error', 'The application crashed. The answer is in the server log, not in the browser.'],
  ['501', 'Not Implemented', 'Server error', 'The server does not support what was asked.'],
  ['502', 'Bad Gateway', 'Server error', 'The proxy or load balancer in front of the site could not reach the thing behind it. Usually the application is down, not the web server.'],
  ['503', 'Service Unavailable', 'Server error', 'Overloaded, or deliberately in maintenance. Often temporary.'],
  ['504', 'Gateway Timeout', 'Server error', 'The proxy reached the application and gave up waiting. Look for a slow database.'],
  ['505', 'HTTP Version Not Supported', 'Server error', 'Rare. Usually an old client and a hardened server.'],
  ['507', 'Insufficient Storage', 'Server error', 'The server is out of disk.'],
  ['511', 'Network Authentication Required', 'Server error', 'A captive portal wants you to log in first.'],
]

// --- Wi-Fi channels --------------------------------------------------------

function wifiRows(): string[][] {
  const rows: string[][] = []
  for (let channel = 1; channel <= 13; channel++) {
    const note =
      channel === 1 || channel === 6 || channel === 11
        ? 'One of the three that do not overlap'
        : 'Overlaps its neighbours: use 1, 6 or 11 instead'
    rows.push([String(channel), '2.4 GHz', `${2407 + 5 * channel} MHz`, note])
  }
  rows.push(['14', '2.4 GHz', '2484 MHz', 'Japan only, and 802.11b only'])

  const five: Array<[number[], string]> = [
    [[36, 40, 44, 48], 'UNII-1, indoor, no radar check'],
    [[52, 56, 60, 64], 'UNII-2A, DFS: must vacate for radar'],
    [[100, 104, 108, 112, 116, 120, 124, 128, 132, 136, 140, 144], 'UNII-2C, DFS: must vacate for radar'],
    [[149, 153, 157, 161, 165], 'UNII-3, higher power, no radar check'],
  ]
  for (const [channels, note] of five) {
    for (const channel of channels) {
      rows.push([String(channel), '5 GHz', `${5000 + 5 * channel} MHz`, note])
    }
  }

  rows.push(['1-93', '6 GHz', '5955-6415 MHz', 'UNII-5, the main indoor block for Wi-Fi 6E and 7'])
  rows.push(['97-113', '6 GHz', '6435-6515 MHz', 'UNII-6, indoor low power'])
  rows.push(['117-185', '6 GHz', '6535-6875 MHz', 'UNII-7, indoor and standard power where allowed'])
  rows.push(['189-233', '6 GHz', '6915-7115 MHz', 'UNII-8, availability varies by country'])
  return rows
}

// --- Subnet table ----------------------------------------------------------

function dotted(value: number): string {
  return [24, 16, 8, 0].map((shift) => (value >>> shift) & 255).join('.')
}

function maskValue(bits: number): number {
  return bits === 0 ? 0 : (0xffffffff << (32 - bits)) >>> 0
}

function subnetRows(): string[][] {
  const rows: string[][] = []
  for (let bits = 8; bits <= 32; bits++) {
    const addresses = 2 ** (32 - bits)
    const usable = bits <= 30 ? addresses - 2 : bits === 31 ? 2 : 1
    rows.push([
      `/${bits}`,
      dotted(maskValue(bits)),
      dotted(~maskValue(bits) >>> 0),
      group(addresses),
      group(usable),
    ])
  }
  return rows
}

// --- Beep codes ------------------------------------------------------------

const BEEP_ROWS: string[][] = [
  ['AMI', '1 short', 'Memory refresh failure', 'Reseat the memory, then try one stick at a time'],
  ['AMI', '2 short', 'Memory parity error', 'Reseat the memory, then try one stick at a time'],
  ['AMI', '3 short', 'Base memory failure', 'Reseat the memory, then try one stick at a time'],
  ['AMI', '5 short', 'Processor failure', 'Reseat the CPU power connector, then suspect the board or the CPU'],
  ['AMI', '8 short', 'Display memory failure', 'Reseat the graphics card, or try onboard video'],
  ['Award or Phoenix', '1 long, 2 short', 'Video failure', 'Reseat the graphics card, or try onboard video'],
  ['Award or Phoenix', '1 long, 3 short', 'Video or keyboard controller failure', 'Unplug everything except power and the monitor, then retry'],
  ['Award or Phoenix', 'Repeating long', 'Memory not detected or not seated', 'Reseat the memory, then try one stick at a time'],
  ['Award or Phoenix', 'Repeating short', 'Power problem', 'Suspect the power supply or the board'],
  ['Dell', '2 beeps', 'No memory detected', 'Reseat the memory, then try one stick at a time'],
  ['Dell', '3 beeps', 'Motherboard failure', 'Strip to the minimum, and expect a board replacement'],
  ['Dell', '4 beeps', 'Memory failure', 'Try one stick at a time in the first slot'],
  ['Dell', '5 beeps', 'Real time clock failure', 'Replace the CMOS battery'],
  ['Dell', '6 beeps', 'Video BIOS failure', 'Reseat or replace the graphics card'],
  ['Dell', '7 beeps', 'Processor failure', 'Expect a board or CPU replacement'],
  ['Apple', 'One beep every 5 seconds', 'No memory installed or detected', 'Reseat the memory if the model allows it'],
  ['Apple', 'Three beeps, then a pause, repeating', 'Memory failed its check', 'Try one stick at a time'],
  ['Apple', 'Three long, three short, three long', 'Firmware needs restoring', "Follow Apple's firmware restore procedure"],
]

// --- SLA -------------------------------------------------------------------

const SLA_PERCENTS = [90, 95, 98, 99, 99.5, 99.9, 99.95, 99.99, 99.999]
const DAY_SECONDS = 86400

export function downtimeSeconds(uptimePercent: number, windowSeconds: number): number {
  return (windowSeconds * (100 - uptimePercent)) / 100
}

export function formatSpan(seconds: number): string {
  if (seconds === 0) return 'none'
  const whole = Math.round(seconds)
  if (whole === 0) return 'under 1s'
  const days = Math.floor(whole / 86400)
  const hours = Math.floor((whole % 86400) / 3600)
  const minutes = Math.floor((whole % 3600) / 60)
  const rest = whole % 60
  const parts: string[] = []
  if (days > 0) parts.push(`${days}d`)
  if (hours > 0) parts.push(`${hours}h`)
  if (minutes > 0) parts.push(`${minutes}m`)
  if (rest > 0) parts.push(`${rest}s`)
  return parts.join(' ')
}

function slaRows(): string[][] {
  return SLA_PERCENTS.map((percent) => [
    `${percent}%`,
    formatSpan(downtimeSeconds(percent, DAY_SECONDS)),
    formatSpan(downtimeSeconds(percent, DAY_SECONDS * 7)),
    formatSpan(downtimeSeconds(percent, DAY_SECONDS * 30)),
    formatSpan(downtimeSeconds(percent, DAY_SECONDS * 365)),
  ])
}

// --- The cards -------------------------------------------------------------

export const CARDS: CardDef[] = [
  {
    id: 'rj45',
    name: 'RJ45 pinout',
    blurb: 'T568B and T568A, pin by pin, with the colours named.',
    icon: 'Cable',
    columns: ['Pin', 'T568B', 'T568A', '10/100 signal'],
    entries: build('rj45', RJ45_ROWS),
    notes: [
      'A straight-through cable uses the same standard at both ends. It does not matter which, as long as both ends match, and T568B is the common choice in most of the world.',
      'A crossover cable is T568B at one end and T568A at the other. Almost nothing needs one any more: every switch and network card made this century negotiates it automatically (auto MDI-X).',
      'Gigabit uses all four pairs in both directions, so a cable with only pairs 1-2 and 3-6 punched down will link at 100 Mbps and look fine until someone measures it.',
      'The pin numbers run 1 to 8 with the clip facing away from you and the copper contacts towards you.',
    ],
    keywords: ['568a', '568b', 'rj45', 'ethernet', 'patch cable', 'crossover', 'straight through', 'punch down', 'keystone', 'cat5e', 'cat6', 'pinout', 'wiring', 'colours', 'colors'],
  },
  {
    id: 'ports',
    name: 'Port numbers',
    blurb: 'The ports that come up in real tickets, and what listens on them.',
    icon: 'DoorOpen',
    columns: ['Port', 'Service', 'Protocol', 'Notes'],
    entries: build('ports', PORT_ROWS),
    notes: [
      'Checked against this machine\'s /etc/services, which comes from the IANA registry. A few ports are listed here under the name everyone actually uses rather than the registered one.',
      'A port being open only means something is listening. Use the Port Scanner to check one from the outside, and Listening Ports to see what this machine has open.',
    ],
    keywords: ['port', 'ports', 'well known ports', 'tcp', 'udp', 'firewall rule', 'what port is', 'services', 'iana'],
  },
  {
    id: 'http-status',
    name: 'HTTP status codes',
    blurb: 'What the number means and who is at fault.',
    icon: 'Globe',
    columns: ['Code', 'Meaning', 'Class', 'What it usually is'],
    entries: build('http-status', HTTP_ROWS),
    notes: [
      'A 4xx is the request being wrong, a 5xx is the server being wrong. That one distinction settles most arguments about whose problem it is.',
      'The official phrases here match the IANA registry. Some servers send their own wording with the same number.',
    ],
    keywords: ['http', 'status code', 'error code', 'web', '404', '403', '500', '502', '503', 'proxy', 'browser error'],
  },
  {
    id: 'wifi-channels',
    name: 'Wi-Fi channels',
    blurb: 'Channels, frequencies and which ones you can actually use.',
    icon: 'Wifi',
    columns: ['Channel', 'Band', 'Centre frequency', 'Notes'],
    entries: build('wifi-channels', wifiRows()),
    notes: [
      '2.4 GHz has exactly three channels that do not overlap: 1, 6 and 11. Every other choice sits on top of two neighbours and makes both worse.',
      'Never run 40 MHz wide on 2.4 GHz. It uses more than half the band and there is nowhere left for anyone else.',
      'DFS channels (52 to 144) must stop transmitting when the radio hears weather or airport radar. That shows up to a user as the Wi-Fi dropping for a minute for no reason.',
      'Every doubling of channel width halves the number of clean channels. On a busy site 20 MHz on 2.4 GHz and 40 MHz on 5 GHz beats a wider setting that keeps colliding.',
      'Which channels are allowed, and at what power, depends on the country the access point is set to. Check the local regulator before planning around the edges of a band.',
    ],
    keywords: ['wifi', 'wi-fi', 'wireless', 'channel', '2.4', '5ghz', '6ghz', 'dfs', 'unii', 'interference', 'overlap', 'width', 'band'],
  },
  {
    id: 'subnet-table',
    name: 'Subnet table',
    blurb: 'Every IPv4 prefix from /8 to /32, with mask, size and host count.',
    icon: 'Calculator',
    columns: ['Prefix', 'Mask', 'Wildcard', 'Addresses', 'Usable hosts'],
    entries: build('subnet-table', subnetRows()),
    notes: [
      'A /31 is a point-to-point link (RFC 3021) and both addresses are usable. A /32 is a single host route.',
      'For anything more than a lookup, use the Subnet Calculator, and to carve a block up use the Subnet Planner.',
    ],
    keywords: ['subnet', 'cidr', 'mask', 'netmask', 'prefix', 'slash 24', 'hosts', 'wildcard', 'acl'],
  },
  {
    id: 'beep-codes',
    name: 'Beep codes',
    blurb: 'What a machine that will not boot is telling you.',
    icon: 'Volume2',
    columns: ['BIOS', 'Beeps', 'Means', 'Try this'],
    entries: build('beep-codes', BEEP_ROWS),
    notes: [
      'Beep codes are set by the BIOS vendor and vary between boards from the same maker. Treat this as a starting point, then check the manual for that exact model.',
      'Whatever the code says, the first two things to do are the same: reseat the memory one stick at a time, and unplug everything that is not needed to POST.',
      'Many machines built after about 2015 have no speaker at all and blink a light instead. The pattern is in the same manual.',
    ],
    keywords: ['beep', 'beeps', 'post', 'bios', 'will not boot', 'no display', 'ami', 'award', 'phoenix', 'dell', 'apple', 'diagnostic', 'error codes'],
  },
  {
    id: 'sla',
    name: 'SLA downtime',
    blurb: 'What each number of nines actually allows.',
    icon: 'Timer',
    columns: ['Uptime', 'Per day', 'Per week', 'Per month', 'Per year'],
    entries: build('sla', slaRows()),
    notes: [
      'A month here is 30 days and a year is 365 days, which is how most contracts write it. Check the wording: some count only business hours, which changes everything.',
      '99.9% sounds close to perfect and allows nearly 9 hours a year, which is a whole working day.',
    ],
    keywords: ['sla', 'uptime', 'downtime', 'nines', 'availability', 'five nines', 'contract', 'service level'],
  },
]

const CARD_HAYSTACKS = new Map<CardId, string>(
  CARDS.map((card) => [
    card.id,
    [card.name, card.blurb, ...card.keywords].join(' ').toLowerCase(),
  ]),
)

export function searchCards(query: string): SearchHit[] {
  const terms = query.trim().toLowerCase().split(/\s+/).filter((term) => term !== '')
  if (terms.length === 0) return []

  const hits: SearchHit[] = []
  for (const card of CARDS) {
    const cardHaystack = CARD_HAYSTACKS.get(card.id) ?? ''
    // A card's own keywords only stand in for a term that none of its rows
    // answers. Without that, "404" matches the status card's keyword list and
    // returns all 37 rows instead of the one row the tech asked for, and the
    // same happens for "dfs", "dell" and 22 other keywords that repeat what is
    // already in the table.
    const viaCard = terms.filter(
      (term) =>
        cardHaystack.includes(term) && !card.entries.some((entry) => entry.haystack.includes(term)),
    )
    for (const entry of card.entries) {
      if (terms.every((term) => entry.haystack.includes(term) || viaCard.includes(term))) {
        hits.push({ card, entry })
        if (hits.length >= MAX_SEARCH_HITS) return hits
      }
    }
  }
  return hits
}
