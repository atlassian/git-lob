package main

import ()

// FileSystemSyncProvider implements the BasicSyncProvider interface
type FileSystemSyncProvider struct {
}

func (*FileSystemSyncProvider) TypeID() string {
	return "filesystem"
}

func (*FileSystemSyncProvider) HelpTextSummary() string {
	return `filesystem: transfers binaries via mounted volumes / mapped drives`
}

func (*FileSystemSyncProvider) HelpTextDetail() string {
	return `The "filesystem" provider transfers files solely by copying them to/from locations
on the file system, i.e. to remotes via mounted volumes / mapped drives. You
are assumed to have the required permissions set up via the file system.

Required parameters in remote section of .gitconfig:
    git-lob-path    The filesystem path to use as a remote binary store

Example configuration:
    [remote "origin"]
        url = git@blah.com/your/usual/git/repo
        git-lob-provider = filesystem
        git-lob-path = /Volumes/shared/your/remote/binary/store

When uploading & downloading, to avoid partially written files when interrupted
a temporary file is created first, then moved to the final location on 
completion.
`
}

func (*FileSystemSyncProvider) Upload(remoteName string, filenames []string, fromDir string) error {
	// TODO
	return nil
}

func (*FileSystemSyncProvider) UploadForce(remoteName string, filenames []string, fromDir string) error {
	// TODO
	return nil
}

func (*FileSystemSyncProvider) Download(remoteName string, filenames []string, toDir string) error {
	// TODO
	return nil
}
