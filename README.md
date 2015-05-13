# git-lob #
## About ##
git-lob is a git extension for efficiently handling binary files in [Git](http://git-scm.com). Instead of storing the binary file content inside the git repo, it is hashed & externalised, with the git commit only referring to the hash. The binary store is synchronised separately from the commits. 

## Why is it useful? ##
1. Adding large binaries to git slows it down & makes cloning slow
2. Most people don't need or want to have every version ever of large binary files on their disk when they only use the more recent ones
3. We need efficient ways to synchronise large binaries via deltas

git-lob keeps your Git repository smaller and faster, while providing rich functionality for managing binary files linked to the source code. 

The target audience is a project team on Windows, Mac or Linux who generally collaborate through one 'master' upstream repo (and maybe a small number of forks) and want to use the speed & power of Git for all their code & smaller resources while having lots of large binary files which are potentially inter-dependent with the history of that code.

## Why did you write it, rather than use X, and why Go? ##
[Discussed in more detail...](doc/why.md)

## Prerequisites ##

You must have git 1.8+ installed and on your path. 

## Installation ##
### Install from source ###

If you want to build from source, make sure you have a Go environment already set up, with $GOPATH/bin already on your $PATH (on Windows, %GOPATH%\bin on your %PATH%). Then run the following in a console:
```bash
> go get bitbucket.org/sinbad/git-lob
> go install bitbucket.org/sinbad/git-lob
```

Now edit your main .gitconfig file in your user directory and add a new filter definition. 

On Mac/Linux:
```ini
[filter "lob"]
  clean = "$GOPATH/bin/git-lob filter-clean %f"
  smudge = "$GOPATH/bin/git-lob filter-smudge %f"
  required = true
```

On Windows:
```ini
[filter "lob"]
  clean = "%GOPATH%/bin/git-lob.exe filter-clean %f"
  smudge = "%GOPATH%/bin/git-lob.exe filter-smudge %f"
  required = true
```

You can expand $GOTPATH/%GOPATH% inline if you need to support usage where GOPATH is not defined. Again on Windows, always use forward slashes, for example c:/path/to/git-lob.exe

### Install From binary distribution ###
If you downloaded a precompiled version for your platform, just extract git-lob[.exe] to a location of your choice.

Now edit your main .gitconfig file in your user directory and add a new filter definition as shown in the 'Install from source' section but set the path to git-lob[.exe] to be wherever you extracted it

## Repository Configuration ##
To start putting binary files into git-lob you need to create or modify a .gitattributes file in the root of your repository:
```ini
*.png filter=lob -crlf
*.jpg filter=lob -crlf
*.zip filter=lob -crlf
*.tiff filter=lob -crlf
*.tga filter=lob -crlf
*.dds filter=lob -crlf
*.bmp filter=lob -crlf
*.mov filter=lob -crlf
```
Include a line for all file types you want to be handled by git-lob. After saving this file, every time you 'git add' on a matching file, its content will be excluded from Git and put in the separate binary store, referenced by SHA in the commit.

## Configuring remote storage ##

Binaries in git-lob are not stored in the regular git repo, but a corresponding
binary store must always exist to provide the actual binary content. A remote
in git usually only gives you the real git repo, so git-lob needs to expand
the configuration parameters to git remotes to specify the location of the 
corresponding remote binary store. 

The parameters depend on the type of binary storage ('provider') being used; see `git-lob providers` for a list of available providers and `git-lob provider <provider>` for specific details of one provider.

As an example, let's take the 'filesystem' provider, which simply uses the OS's
file system as a remote transport (obviously very simplistic):

```ini
[remote "origin"]
    # these 2 lines are standard git
    url = ssh://git@bitbucket.org/something/somthing.git
    fetch = +refs/heads/*:refs/remotes/origin/*
    # these next 2 lines are required to configure the remote binary store
    git-lob-provider = filesystem
    git-lob-path = /Volumes/shared/something/something/binary/store
```
Other providers may require other parameters. It's important to note that you
can share a binary store among multiple remote repos if you wish, much like
the local git-lob.sharedstore option, since binaries are stored by SHA. 
Identical file content in multiple repos can be stored only once this way.
Of course, access control may be an issue to consider here though.

## Other options ##
git-lob supports a number of command-line parameters, and configuration parameters in your .gitconfig (user or repository level). Please call 'git lob help' for general help and a list of main commands, and 'git lob help topics' to list other topics.

# More Information #
## Host setup ##

* [How to configure Amazon S3 for team use](doc/s3_setup.md)
* [The Smart Sync Protocol](doc/smart_protocol.md)

## Information for git-lob developers ##

* [Initial developer setup](doc/devsetup.md)
* [Coding conventions](doc/conventions.md)
* [Third-party dependencies](doc/dependencies.md)
* [A note about dates in fetch & prune](doc/fetch_prune_dates.md)
