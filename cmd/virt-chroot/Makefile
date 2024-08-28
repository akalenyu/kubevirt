all:
	GOTRACEBACK=crash ./virt-chroot || true
	coredumpctl dump -o virt-chroot.core $(realpath ./virt-chroot)
	ls -sh ./virt-chroot.core
