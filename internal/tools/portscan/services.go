package portscan

// services names the ports a tech actually meets. It is deliberately short:
// the whole of IANA would be noise in a results table.
var services = map[int]string{
	20: "FTP data", 21: "FTP", 22: "SSH", 23: "Telnet", 25: "SMTP",
	53: "DNS", 67: "DHCP server", 68: "DHCP client", 69: "TFTP", 80: "HTTP",
	88: "Kerberos", 110: "POP3", 111: "RPC portmapper", 123: "NTP", 135: "Windows RPC",
	137: "NetBIOS name", 138: "NetBIOS datagram", 139: "NetBIOS session", 143: "IMAP",
	161: "SNMP", 162: "SNMP trap", 389: "LDAP", 443: "HTTPS", 445: "SMB file sharing",
	464: "Kerberos password change", 465: "SMTP over TLS", 500: "IPsec IKE", 514: "Syslog",
	515: "LPD printing", 587: "SMTP submission", 631: "IPP printing", 636: "LDAPS",
	993: "IMAP over TLS", 995: "POP3 over TLS", 1080: "SOCKS proxy", 1194: "OpenVPN",
	1433: "Microsoft SQL Server", 1521: "Oracle database", 1723: "PPTP VPN", 2049: "NFS",
	2222: "SSH (alternate)", 3000: "Web app (development)", 3128: "Squid proxy",
	3268: "LDAP global catalog", 3306: "MySQL or MariaDB", 3389: "RDP remote desktop",
	4444: "Often used by malware", 5000: "Web app or UPnP", 5060: "SIP voice",
	5061: "SIP over TLS", 5222: "XMPP chat", 5353: "mDNS (Bonjour)", 5432: "PostgreSQL",
	5900: "VNC remote control", 5985: "WinRM over HTTP", 5986: "WinRM over HTTPS",
	6379: "Redis", 8000: "Web app", 8006: "Proxmox web interface", 8080: "HTTP (alternate)",
	8081: "HTTP (alternate)", 8443: "HTTPS (alternate)", 8888: "Web app", 9000: "Web app",
	9100: "Raw printing (JetDirect)", 9200: "Elasticsearch", 10000: "Webmin",
	27017: "MongoDB",
}

// serviceName is what usually listens on a port, or "" for one nobody has to
// recognise.
func serviceName(port int) string {
	return services[port]
}
