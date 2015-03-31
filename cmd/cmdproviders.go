package cmd

import (
	"bitbucket.org/sinbad/git-lob/providers"
	"bitbucket.org/sinbad/git-lob/util"
)

func ListProviders() int {
	util.LogConsole()
	util.LogConsole("Available remote providers:")
	for _, p := range providers.GetSyncProviders() {
		util.LogConsole(" *", p.HelpTextSummary())
	}
	util.LogConsole()
	return 0
}

func ProviderDetails() int {
	if len(util.GlobalOptions.Args) == 0 {
		return ListProviders()
	}

	util.LogConsole()
	// Potentially list many
	ret := 0
	for _, arg := range util.GlobalOptions.Args {
		p, err := providers.GetSyncProvider(arg)
		if err != nil {
			util.LogConsole(err)
			ret++
		} else {
			util.LogConsole(p.HelpTextDetail())
			util.LogConsole()
		}

	}
	return ret
}
