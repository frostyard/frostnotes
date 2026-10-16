//go:build !linux

package vault

// onNetwork reports whether dir is on a network file system. Only Linux
// can tell yet; elsewhere a vault is rescanned at the local interval.
func onNetwork(string) (bool, error) { return false, nil }
