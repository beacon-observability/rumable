//go:build !windows

package rumable

func protectPrivateFile(path string) error           { return nil }
func protectSiteDirectory(root string, s Site) error { return nil }
