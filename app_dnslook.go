package main

import "chit/internal/tools/dnslook"

// StartDNSLookup asks every selected server for every selected record type and
// streams one result per answer. Stop it with CancelJob.
func (a *App) StartDNSLookup(p dnslook.Params) (string, error) {
	return dnslook.New(a.jobs).StartLookup(p)
}

// DNSServers lists the servers the UI offers as tick boxes: this machine's own
// resolvers first, then the well known public ones.
func (a *App) DNSServers() ([]dnslook.ServerOption, error) {
	return dnslook.Servers(), nil
}
