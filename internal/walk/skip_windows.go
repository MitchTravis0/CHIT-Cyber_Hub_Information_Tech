//go:build windows

package walk

// Windows has no pseudo filesystem mounted into the drive letters, so there is
// nothing to skip by path. Junctions and reparse points are handled by the
// symbolic-link rule in walkDir instead.
var skipRoots = []string{}
