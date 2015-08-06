# Dependencies #

## Runtime: ##

* [Goamz (forked)](https://github.com/sinbad/goamz) (Original: https://github.com/mitchellh/goamz)
  AWS library for Go. LGPLv3 with static/dynamic linking exception
* [Go-ini](https://github.com/vaughan0/go-ini)
  Dependency of goamz. MIT license
* [Go-homedir](https://github.com/mitchellh/go-homedir)
  Library for detecting home directory without cgo, thus allowing cross-compiling.
* [bm (forked)](https://github.com/sinbad/bm) (Original: https://github.com/cloudflare/bm)
  Go implementation of VCDIFF delta compression. BSD license

## Build / test (not distributed): ##

* [Ginkgo](https://github.com/onsi/ginkgo)
  Testing library. MIT license
* [Gomega](https://github.com/onsi/gomega)
  Matcher library for Ginkgo. MIT license
* [Gox](https://github.com/mitchellh/gox)
  Cross-compilation tool


## A note about godep ##

In order to simplify setup, all 3rd-party dependencies are captured inside
this repo using 'godep save -r'. See https://github.com/tools/godep

'godep save' copies your dependencies from GOPATH into a _workspace subdir of
the repo, snapshotting the dependencies and making it easier to set up the 
project. 
In addition, it's really useful for when you fork repos but don't change
the library name (say because you're hoping the pull requests get accepted,
see https://blog.splice.com/contributing-open-source-git-repositories-go/)

The '-r' parameter re-writes import statements in git-lob to refer directly
to the internal version, which means anyone building it doesn't have to 
use 'godep restore' (which copies the internal version into their GOPATH, 
which they might not want if other projects use a common lib), or the 
alternative which is to add _workspace to your GOPATH, or to prefix all
go commands with godep e.g. 'godep go build'. Rewriting the imports just
means everything works as if nothing happened. 

## Referencing dependencies ##

Remember to reference *existing* dependencies using their local workspace
path, i.e.
```
import "github.com/atlassian/git-lob/Godeps/_workspace/src/foo/bar"
```
and not
```
import "foo/bar"
```

*New* dependencies are different though, see below.

## Adding a new dependency ##

To add a new dependency foo/bar:
```
go get foo/bar
```
[reference foo/bar in our source code]
```
godep save -r ./...
```

The './...' parameter is to ensure godep looks in all of our packages. 

Note that while adding the dependency you use the external package name foo/bar
but that after 'godep save -r' this is re-written to "github.com/atlassian/git-lob/Godeps/_workspace/src/foo/bar", which you should use from then on.

Referencing it as its original package name is required pre-godep save.

## Updating an existing dependency ##

To update the existing packages
```
go get -u foo/bar (or customise & commit the package yourself)
godep update foo/bar/...
godep save -r ./...
```

Note the use of the '...' wildcard in the 'godep update' call, this is required
for nested packages & is generally a good idea to be sure. 

There is no '-r' option to 'godep update' so actually 'godep update' will 
reverse the rewriting of the package names to the internal _workspace version.
That's why you have to run 'godep save -r' again afterwards to fix this.

## Customising a dependency through a fork ##

When we create forks of a dependency to change it, we don't change the identifying
path of the dependency; e.g. my fork of goamz lives at https://github.com/sinbad/goamz
but we stil refer to it as https://github.com/mitchellh/goamz to avoid breaking cross-refs
inside the code. 

This works because inside my local src/github.com/mitchellh/goamz repo I've created a 
secondary remote of my fork and pulled the changes in from my fork there, before using
'godep save' (see above) to 'freeze' that version in this repo.

So when customising a dependency you need to ensure that you have any existing customisations
in your src/ folder (e.g. clone/pull from the fork), update the code, push to the fork, then:
```
godep update foo/bar/...
godep save -r ./...
```
