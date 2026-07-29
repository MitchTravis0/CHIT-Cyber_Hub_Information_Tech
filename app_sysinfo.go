package main

import "chit/internal/tools/sysinfo"

// SystemInfo reads everything this operating system will tell us about the
// machine CHIT is running on. Fields an OS refuses are returned empty and
// named in Report.Unsupported, never guessed.
func (a *App) SystemInfo() (sysinfo.Report, error) {
	r, err := sysinfo.New()
	// The version is stamped here because internal/tools/sysinfo must not
	// import package main, and a copied report needs to say which CHIT made it.
	r.AppVersion = Version
	return r, err
}
