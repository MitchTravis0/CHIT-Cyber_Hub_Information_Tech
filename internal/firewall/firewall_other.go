//go:build !linux

package firewall

// detect reports nothing on Windows, macOS and everything else, deliberately,
// and this is a decision rather than a missing implementation.
//
// Both of the platforms that matter here **ask the tech a question** the first
// time a program opens a port: Windows shows the Defender Firewall dialog and
// macOS asks whether to allow incoming connections. So the silent failure this
// package exists to explain does not happen there in the same way, and each
// tool's help text already describes the prompt.
//
// The other half of the reason is honesty. A detector for those platforms would
// have to parse `netsh advfirewall show currentprofile` or
// `socketfilterfw --getglobalstate`, and nobody working on this project has a
// Windows or macOS machine to run either on. Guessing at the output format and
// shipping it would produce a confident sentence that might be wrong, which is
// worse than the nothing it replaces. Write these when there is a machine to
// check them against.
func detect() string { return "" }
