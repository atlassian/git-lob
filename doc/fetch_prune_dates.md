# Notes on fetch and prune data periods #

Both fetch and prune have configuration options to control how far back to fetch or keep prior versions of files, on the current branch and other branches. In both cases, the periods are expressed in days. 

However, it's important to realise that they DON'T mean this:
> "Fetch or keep changes recorded in commits within X days" 

Instead, they DO mean this:
> "Fetch or keep all versions of files that would be needed for any checkout within X days"

The difference might seem subtle, but it's very important. A naive approach might be to look at commits within X days and look for binaries on the '+' side of the diff, but the problem is that a file which did not change for 3 years but is still present in the working copy is still needed for checkouts within the last X days. 

So instead the approach for fetch and prune is as follows:

1. Find all binaries required at the latest date (usually the end of the branch) via 'git ls-files'
2. Walk backwards in 'git log' looking for commits within X days that reference binaries but in the '-' side of the diff

## Why the '-' side of the diff? ##

That tells you which binary was referenced just before that commit was made. You already have the binary from the '+' side of the diff from the 'git ls-files' snapshot (or a later commit which had it on the '-' side), so that's not useful. It's important to note that if you checked out any commit prior to that commit which changed the binary, you'd need that binary on the '-' side.

We only walk backwards through commits which reference binaries, but of course there can be are other commits in between. Checking any of those out between binary commits would require the state as recorded in the '-' diff of the next commit after.

## Borderline cases ##

What this does mean is that in borderline cases, an extra version is fetched or kept. Take the example where the edge of X days range falls *exactly* on a commit which changed a binary. In this case that commit's '-' side will be used as well as the state afterwards (via ls-files snapshot or a later commit), because our 'git log' always uses '--since=' and that's inclusive of these borderline cases. 

However not only is this a safe default (particularly for prune) but the chances of the X days threshold falling precisely on a commit is rare. More likely it will be just after or just before, in which case it makes more sense - provided you remember that it's retaining by an arbitrary date including the state in between commits, not ON commits. That means even if there are no commits in between 2 binary changes, the date cut-off will behave as if there could be, only dropping the binaries required to represent the state in the middle once the date range passes completely into the period after the later commit.

## The meaning of 0 days ##

A setting of 0 days always means that only a snapshot is taken (the git ls-files), no previous versions are kept so no date period is calculated. This is the default for non-head branches and is useful for keeping the size down in busy repos.
