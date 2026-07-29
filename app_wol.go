package main

import "chit/internal/tools/wol"

// WakeDevice sends a magic packet from every network this machine is on, to the
// subnet broadcast and the limited broadcast, because different network cards
// listen for different ones.
func (a *App) WakeDevice(params wol.WakeParams) (wol.WakeResult, error) {
	return wol.Wake(params)
}

// WakeTargets lists where a packet would be sent right now, so the UI can show
// it before anything is sent and explain a machine with no usable network.
func (a *App) WakeTargets() (wol.TargetList, error) {
	return wol.Targets()
}

// CheckAwake reports whether an address answers a TCP connection yet. The UI
// polls it after a wake-up so a tech can see the machine come back.
func (a *App) CheckAwake(ip string) (wol.Awake, error) {
	return wol.CheckAwake(a.ctx, ip)
}
