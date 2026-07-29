package netinfo

import (
	"errors"
	"net"
	"unsafe"

	"golang.org/x/sys/windows"
)

// IP_ADAPTER_DHCP_ENABLED from iptypes.h. x/sys/windows declares the struct
// but not the adapter flag bits.
const ipAdapterDHCPEnabled = 0x00000004

// platformEnrich fills in the fields the Go standard library does not expose.
// Windows hands all of them over in one GetAdaptersAddresses call, so nothing
// here shells out to ipconfig or PowerShell.
func platformEnrich(r *Report) string {
	rows, err := adapterAddresses()
	if err != nil {
		r.markUnsupported(FieldGateway, FieldDNS, FieldAdapterDNS, FieldDHCP)
		return ""
	}

	byIndex := map[uint32]*Adapter{}
	for i := range r.Adapters {
		byIndex[uint32(r.Adapters[i].Index)] = &r.Adapters[i]
	}

	defaultIface := ""
	bestMetric := ^uint32(0)
	for _, row := range rows {
		a := byIndex[row.IfIndex]
		if a == nil {
			a = byIndex[row.Ipv6IfIndex]
		}
		if a == nil {
			continue
		}

		a.FriendlyName = windows.UTF16PtrToString(row.FriendlyName)
		a.Description = windows.UTF16PtrToString(row.Description)
		a.Up = row.OperStatus == windows.IfOperStatusUp
		a.Loopback = row.IfType == windows.IF_TYPE_SOFTWARE_LOOPBACK
		a.Virtual = !a.Loopback &&
			(row.IfType == windows.IF_TYPE_TUNNEL || isVirtual(a.FriendlyName, a.Description))
		// Windows reports 0xffffffff for adapters with no meaningful MTU.
		if row.Mtu > 0 && row.Mtu < 1<<20 {
			a.MTU = int(row.Mtu)
		}
		if row.Flags&ipAdapterDHCPEnabled != 0 {
			a.DHCP = DHCPDynamic
		} else {
			a.DHCP = DHCPStatic
		}

		a.Gateway = firstGateway(row)
		for d := row.FirstDnsServerAddress; d != nil; d = d.Next {
			ip := d.Address.IP()
			if ip == nil || siteLocalStub(ip) {
				continue
			}
			a.DNS = append(a.DNS, ip.String())
		}

		if a.Up && a.Gateway != "" && row.Ipv4Metric < bestMetric {
			bestMetric, defaultIface = row.Ipv4Metric, a.Name
			r.DNS = a.DNS
			if suffix := windows.UTF16PtrToString(row.DnsSuffix); suffix != "" {
				r.SearchDomains = []string{suffix}
			}
		}
	}
	return defaultIface
}

// firstGateway prefers the IPv4 gateway, which is the one a tech types into a
// browser, and falls back to IPv6 on an IPv6-only adapter.
func firstGateway(row *windows.IpAdapterAddresses) string {
	v6 := ""
	for g := row.FirstGatewayAddress; g != nil; g = g.Next {
		ip := g.Address.IP()
		if ip == nil {
			continue
		}
		if v4 := ip.To4(); v4 != nil {
			return v4.String()
		}
		if v6 == "" {
			v6 = ip.String()
		}
	}
	return v6
}

// siteLocalStub matches the fec0::/10 placeholder DNS servers Windows lists on
// adapters that have no real IPv6 resolver.
func siteLocalStub(ip net.IP) bool {
	v6 := ip.To16()
	return ip.To4() == nil && v6 != nil && v6[0] == 0xfe && v6[1]&0xc0 == 0xc0
}

func adapterAddresses() ([]*windows.IpAdapterAddresses, error) {
	size := uint32(15000)
	for attempt := 0; attempt < 5; attempt++ {
		buf := make([]byte, size)
		head := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		flags := uint32(windows.GAA_FLAG_INCLUDE_GATEWAYS | windows.GAA_FLAG_INCLUDE_PREFIX)
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, flags, 0, head, &size)
		switch {
		case errors.Is(err, windows.ERROR_BUFFER_OVERFLOW):
			continue
		case errors.Is(err, windows.ERROR_NO_DATA):
			return nil, nil
		case err != nil:
			return nil, err
		}
		var out []*windows.IpAdapterAddresses
		for a := head; a != nil; a = a.Next {
			out = append(out, a)
		}
		return out, nil
	}
	return nil, errors.New("adapter table kept growing")
}
