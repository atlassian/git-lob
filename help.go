package main

func cmdHelpUsage() {
	// For safety, these always go to stderr not stdout
	// That's because this is before the command has been chosen and therefore has not
	// had the chanced to call LogAllConsoleOutputToStdErr. A poorly
	// configured filter shouldn't be allowed to corrupt file output
	LogConsoleError(usageTxt)
}

// Map from topic->help function
// Replicate the help functions for all other commands here too
var helpTopicMap = map[string]func(){
	"topics":    cmdTopicsHelp,
	"config":    cmdConfigHelp,
	"commands":  cmdCommandsHelp,
	"remotes":   cmdRemotesHelp,
	"providers": cmdProvidersHelp,
	"fetch":     cmdFetchHelp,
	"pull":      cmdPullHelp,
	"push":      cmdPushHelp,
	"checkout":  cmdCheckoutHelp,
	"prune":     cmdPruneHelp,
}

func cmdHelp() {
	// See above for why this is stderr not stdout
	if len(GlobalOptions.Args) > 0 {
		// Topic or command-specific help requested
		arg := GlobalOptions.Args[0]

		// Find in topics list
		f, found := helpTopicMap[arg]

		if found {
			f()
			return
		} else {
			// Also search the providers for anything called that & use their help
			p, err := GetSyncProvider(arg)
			if err == nil {
				LogConsole(p.HelpTextDetail())
				return
			}

		}
	}

	// Top-level help
	LogConsoleError(rootHelpTxt)
	LogConsoleError(rootOptionsTxt)

}

func cmdConfigHelp() {
	LogConsole(`Config files:

  git-lob uses ~/.gitconfig and $REPO/.git/config to modify default behaviour.
  All settings are inside the [git-lob] section

General settings:

  git-lob.verbose    Same as --verbose on command line
  git-lob.quiet      Same as --quiet on the command line
  git-lob.logenabled Enable logging of messages to a file
  git-lob.logfile    Log file to write if logenabled (default: ~/git-lob.log)
  git-lob.logverbose Verbose logging in log file
                     (separate to console)
  git-lob.sharedstore
                     A shared folder in which to store binary content rather
                     than storing it inside each repo. This minimises storage
                     when you have multiple clones.
                     Files will be hard linked into your repo so that
                     they actually appear there as usual but storage is only
                     used once if the same SHA appears in multiple repos.
                     When the number of hard links on a file in the shared
                     area reaches 1 during cleanup, the shared file is deleted.
                     NOTE: requires a file system capable of hard links
                     e.g. ext3, HFS, NTFS, and the shared store and the repos
                     using it must be on the same filesystem (drive on Windows)

Checkout settings:

  git-lob.autofetch  Automatically download binaries required on checkout if
                     they're not already present in the binary store

Fetch settings:

  git-lob.recent-refs          What is considered a recent branch/tag from the
                               current date. Default 90 (days).
  git-lob.recent-commits-head  Recent commit period for fetching prior versions
                               on your current HEAD (from latest commit)
                               Default 30 (days)
  git-lob.recent-commits-other Recent commit period for fetching prior versions
                               on other branches (from latest commit)
                               Default 0 (fetch only latest)
  git-lob.fetch-include        Limits binaries fetched to only matching paths.
                               Comma-separated with wildcard matching. 
                               Note: wildcards do not match path separators, 
                               just like gitignore.
  git-lob.fetch-exclude        Do not fetch matching paths. Same comma
                               separator & wildcard rules as above

Remote settings:
  These settings are stored underneath the regular remote configuration in git.

  remote.<name>.git-lob-provider  Which 'provider' will be used to communicate
                                  with the remote binary store for this remote

  Each provider will require other configuration options to fully specify the
  location. Run 'git lob help remotes' for more details.

`)
}

func cmdTopicsHelp() {
	LogConsole(`Usage: git lob help <topic>
Available topics:
  topics        Show this list
  config        Help with configuration options
  commands      List all the commands available
  <command>     Same as git lob <command> --help
  remotes       General discussion of how remotes work with git-lob
  providers     Lists all the upload/download providers
  <provider>    Detailed help on one provider
`)
}

func cmdCommandsHelp() {
	// Start with root commands, then add
	LogConsole(rootHelpTxt)
	LogConsole(plumbingCommandsHelpTxt)
	LogConsole(rootOptionsTxt)
}

func cmdRemotesHelp() {
	LogConsole(`How remotes work in git-lob

Binaries in git-lob are not stored in the regular git repo, but a corresponding
binary store must always exist to provide the actual binary content. A remote
in git usually only gives you the real git repo, so git-lob needs to expand
the configuration parameters to git remotes to specify the location of the 
corresponding remote binary store. 

The <remote> parameter to commands like push and fetch refers to a named remote
in .git/config (plain URLs cannot be supported). The remote entry itself is the
same as any normal git remote, except that it requires additional git-lob 
specific parameters. The parameters depend on the type of binary storage 
('provider') being used; see 'git-lob providers' for a list of available 
providers and 'git-lob provider <provider>' for specific details of one 
provider.

As an example, let's take the 'filesystem' provider, which simply uses the OS's
file system as a remote transport (obviously very simplistic):

[remote "origin"]
    # these 2 lines are standard git
    url = ssh://git@bitbucket.org/something/somthing.git
    fetch = +refs/heads/*:refs/remotes/origin/*
    # these next 2 lines are required to configure the remote binary store
    git-lob-provider = filesystem
    git-lob-path = /Volumes/shared/something/something/binary/store
    
Other providers may require other parameters. It's important to note that you
can share a binary store among multiple remote repos if you wish, much like
the local git-lob.sharedstore option, since binaries are stored by SHA. 
Identical file content in multiple repos can be stored only once this way.
Of course, access control may be an issue to consider here though.
`)
}

func cmdProvidersHelp() {
	cmdListProviders()
}

const usageTxt = `Usage: git lob [command] [options] [file...]
Type 'git lob help' for more information
`

const rootHelpTxt = `Usage: git lob [command] [options] [file...]

  git-lob improves handling of large objects (including binary files) in git

  Use 'git lob <command> --help' or 'git lob help <topic>' for more details.
  'git lob help topics' lists topics available.

Commands:
  help                Display this help. Append a topic for general info
                      ('config', 'commands', 'topics' to list available topics)
                      or use 'git lob <command> --help' for command help.
  push                Upload local binaries to a remote.
  fetch               Download binaries from a remote.
  checkout            Check the working copy and fill in any binary content
                      that's missing
  pull                Perform 'fetch' then 'checkout'

  filter-smudge       Execute the git smudge filter (when checking out)
                      This should be set up in .gitattributes
  filter-clean        Execute the git clean filter (when adding/committing)
                      This should be set up in .gitattributes

  listproviders       List the available remote providers
  provider <name>     Print detail about named provider

  prune               Remove binaries unreferenced by any commit or the index
                      from the local repo binary store (and shared if no other
                      usage)
  prune-shared        Delete any binaries in the shared store which have become
                      unreferenced because repos were manually deleted

`
const rootOptionsTxt = `Global Options:
  --quiet, -q          Print less output
  --verbose, -v        Print more output
  --dry-run            Don't perform actions, just report
  --noninteractive, -n Never prompt for user input

  --help               Print this message
`
const plumbingCommandsHelpTxt = `Low-level plumbing commands:
  push-lob             Push an individual LOB to a remote by SHA
  fetch-lob            Fetch an individual LOB from a remote by SHA
  last-pushed          Report the last pushed ancestor of a ref
  mark-pushed          Mark a commit as having being pushed to a remote
  reset-pushed         Reset the pushed flag for a commit

`
