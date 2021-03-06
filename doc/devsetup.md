# Setting up Go environment #

1. Install [Go tools](https://golang.org/doc/install)
   If you need to cross-compile to other platforms, install from source. 
   Otherwise the binary install is fine
2. If you don't have a GOPATH already:
  1. Create a folder somewhere in your user area, e.g. ~/Go
  2. Create subfolders 'src', 'bin' and 'pkg'
  3. Create an environment variable GOPATH pointing at the root dir e.g. ~/Go
      On Windows, open Control Panel > System > Advanced > Environment variables
      On Mac/Linux, edit ~/.profile or ~/.bash_profile and export GOPATH=blah
  4. If you wish, add $GOPATH/bin to your PATH the same way
3. If you want to cross-compile, install [Gox](https://github.com/mitchellh/gox)
4. Make sure you cloned git-lob into $GOPATH/src/github.com/atlassian/git-lob 

## A note about dependencies ##

Dependencies are managed with godep, see [Dependencies](dependencies.md).