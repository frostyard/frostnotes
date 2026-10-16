package vault

import "golang.org/x/sys/unix"

// networkMagic holds the statfs types of network file systems, where edits
// from other machines raise no inotify events (M0 spike 3: the user's
// vault is on SMB 3, 0xfe534d42).
var networkMagic = map[uint32]bool{
	unix.SMB2_SUPER_MAGIC: true,
	unix.CIFS_SUPER_MAGIC: true,
	unix.SMB_SUPER_MAGIC:  true,
	unix.NFS_SUPER_MAGIC:  true,
	unix.V9FS_MAGIC:       true,
	unix.CEPH_SUPER_MAGIC: true,
	unix.AFS_FS_MAGIC:     true,
}

// onNetwork reports whether dir is on a network file system.
func onNetwork(dir string) (bool, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return false, err
	}
	return networkMagic[uint32(st.Type)], nil // the magic is 32 bits; Type is int32 on 32-bit systems
}
