package main

import (
	"fmt"
)

// Push command line tool
func Push() int {

	return 0
}

func printPushHelp() {
	fmt.Println(`Usage: git-lob push [options] [<remote> [<ref>...]]

  Uploads binaries to a remote, sending only binaries required to ensure 
  that remote has the binary resources referenced at a set of commits.

  Behaves much like 'git push' except there are no destination refs, only
  supporting binary files.

Parameters:
  <remote>: The destination to upload to. This should correspond to the 
            name of a remote (no direct URLs permitted) which is configured
            in .git/config. See REMOTES below for more details, additional
            config parameters are required in the remote.

            If no remote is specified, branch.*.remote configuration for the
            current branch is consulted to determine where to push. If the 
            configuration is missing, it defaults to origin.
     <ref>: Which local reference(s) up to which we should make sure binaries
            are uploaded for. You can specify zero, one, or many local refs.
            There is no destination ref as in git push.

            If no ref is specified, and --all is not used, remote.*.push 
            is consulted to see what to push, with push.default as a fallback.

            COMMIT RANGES

            You can also specify a range of refs in the form <ref1>..<ref2> to
            force git-lob push to check a specific range of commits for
            binaries, instead of using its own records of which commits it
            thinks are already up to date on this remote. 
            See HISTORY CHECKING below.


Options:
  --all         Push all branches; cannot be used with other refs.
  --recheck     Re-check entire commit history to each ref instead of only 
                back to last commit we believe is already pushed. 
                See HISTORY CHECKING below for more details.
  --force       Always upload files even if the provider believes the file is 
                already present on the remote. You shouldn't need this.
  --quiet, -q   Print less output
  --verbose, -v Print more output
  --dry-run     Don't actually delete anything, just report

REMOTES

Binaries in git-lob are not stored in the regular git repo, but a corresponding
binary store must always exist to provide the actual binary content. A remote
in git usually only gives you the real git repo, so git-lob needs to expand
the configuration parameters to git remotes to specify the location of the 
corresponding remote binary store. 

The <remote> parameter refers to a named remote in .git/config (plain URLs 
cannot be supported). The remote entry itself is the same as any normal git
remote, except that it requires additional git-lob specific parameters. The
parameters depend on the type of binary storage ('provider') being used; see
'git-lob providers' for a list of available providers and 
'git-lob provider <provider>' for specific details of one provider.

As an example, let's take the 'filesystem' provider, which simply uses the OS's
file system as a remote transport (obviously very simplistic):

[remote "origin"]
    # these 2 lines are standard git
    url = ssh://git@bitbucket.org/something/somthing.git
    fetch = +refs/heads/*:refs/remotes/origin/*
    # these next 2 lines are required to configure the remote binary store
    git-lob-provider = filesystem
    git-lob-url = /Volumes/shared/something/something/binary/store
    
Other providers may require other parameters. It's important to note that you
can share a binary store among multiple remote repos if you wish, much like
the local git-lob.sharedstore option, since binaries are stored by SHA. 
Identical file content in multiple repos can be stored only once this way.
Of course, access control may be an issue to consider here though.

HISTORY CHECKING

When pushing binaries for a given ref, git-lob performs a search for commits
which reference git-lob binaries from that ref backwards, before checking
which of those binaries it needs to upload. This is so that we only upload
binaries that are actually referenced by the ref you're choosing to push, 
and don't waste time on binaries in unpublished feature branches etc. 

Because searching the whole of the git history can be slow on large 
repositories, git-lob speeds this search up by keeping a record of which 
commits it believes the remote already has all binaries for. 

These records are updated whenever you git-lob push/pull. We do not use git's
own remote branch refs to track this, because pushing commits can be done
completely separately from binaries so we can't rely on that information.
So pushing and pulling branches in git has no effect on this state, only
git-lob push/pull.

If for some reason these records are wrong, and you need to push binaries
for a bigger range of commits, you can do this 2 ways:

1. Use the --recheck option. This is the 'nuclear option'; it will scan the
   entire history of the repo again to make 100% sure everything is correct.
   Can take a while on large repos.

2. Use a commit range for <ref>, i.e. <ref1>..<ref2>. git-lob will check that
   entire range of commits for binary references which will then be checked
   with the remote. 

There are not many circumstances where you need to manually override the commit
range that is checked for binaries. Even if you edit commits, rebase etc, 
git-lob should not miss any binaries, because the commit SHAs would change and
it would know to check any referenced binaries again. The main reason why
you would need to override the history checking is if the remote changed, for
example if someone manually deleted the remote binary store, or you moved to 
a new URL without copying the data and needed to re-populate it from your local
repo.

`)
}
