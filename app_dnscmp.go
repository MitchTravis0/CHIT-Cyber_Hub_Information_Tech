package main

import "chit/internal/tools/dnscmp"

// CompareDNS asks every chosen resolver the same question and reports which of
// them disagree with the majority.
func (a *App) CompareDNS(p dnscmp.Params) (dnscmp.Comparison, error) {
	return dnscmp.Compare(a.ctx, p)
}

// DNSCompareServers lists the resolver tick boxes: the system resolver, this
// machine's own servers, then the public ones.
func (a *App) DNSCompareServers() ([]dnscmp.ServerOption, error) {
	return dnscmp.Servers(), nil
}
