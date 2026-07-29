export interface Preset {
  id: string
  name: string
  spec: string
  note: string
}

export const PRESETS: Preset[] = [
  { id: 'common', name: 'Common', spec: '21,22,23,25,53,80,110,111,135,139,143,161,389,443,445,465,514,515,587,631,993,995,1080,1433,1521,1723,2049,2222,3000,3128,3306,3389,4444,5000,5060,5432,5900,5985,6379,8000,8006,8080,8081,8443,8888,9000,9100,9200,10000,27017', note: 'The 50 ports that turn up most on an office network' },
  { id: 'web', name: 'Web', spec: '80,443,8080,8443', note: 'Web servers and device admin pages' },
  { id: 'remote', name: 'Remote access', spec: '22,23,3389,5900', note: 'SSH, Telnet, RDP and VNC' },
  { id: 'windows', name: 'Windows', spec: '135,139,445,3389,5985,5986', note: 'File sharing, RPC and Windows remote management' },
  { id: 'printers', name: 'Printers', spec: '515,631,9100', note: 'LPD, IPP and raw JetDirect printing' },
  { id: 'mail', name: 'Mail', spec: '25,110,143,465,587,993,995', note: 'Mail submission and collection' },
  { id: 'databases', name: 'Databases', spec: '1433,1521,3306,5432,27017', note: 'SQL Server, Oracle, MySQL, PostgreSQL and MongoDB' },
  { id: 'wellknown', name: 'All well-known (1-1024)', spec: '1-1024', note: 'Slower: 1024 ports, about a minute at the normal settings' },
]
