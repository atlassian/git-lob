package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Chunk size that we split stored data into so it's easier to resume uploads/downloads
// This used to be configurable, but it caused too many issues if different people had different
// settings in a shared repository
// This is only 'var' rather than 'const' to allow tests to modify
var ChunkSize = int64(32 * 1024 * 1024)

// Options (command line or config file)
// Only general options, command-specific ones dealt with in commands
type Options struct {
	// Help option was requested
	HelpRequested bool
	// Output verbosely (to console & log)
	Verbose bool
	// Output quietly (to console)
	Quiet bool
	// Don't actually perform any tasks
	DryRun bool
	// Never prompt for user input, rely on command line options only
	NonInteractive bool
	// The command to run
	Command string
	// Other value options not converted
	StringOpts map[string]string
	// Other boolean options not converted
	BoolOpts StringSet
	// Other arguments to the command
	Args []string
	// Whether to write output to a log
	LogEnabled bool
	// Log file (optional, defaults to ~/git-lob.log if not specified)
	LogFile string
	// Log verbosely even if main Verbose option is disabled for console
	VerboseLog bool
	// Shared folder in which to store binary files for all repos
	SharedStore string
	// Auto fetch (download) on checkout?
	AutoFetchEnabled bool
	// 'Recent' window in days for fetching all refs (branches/tags) compared to current date
	FetchRefsPeriodDays int
	// 'Recent' window in days for fetching commits on HEAD compared to latest commit date
	FetchCommitsPeriodHEAD int
	// 'Recent' window in days for fetching commits on other branches/tags compared to latest commit date
	FetchCommitsPeriodOther int
	// Retention window in days for refs compared to current date
	RetentionRefsPeriod int
	// Retention window in days for commits on HEAD compared to latest commit date
	RetentionCommitsPeriodHEAD int
	// Retention window in days for commits on other branches/tags compared to latest commit date
	RetentionCommitsPeriodOther int
	// List of paths to include when fetching
	FetchIncludePaths []string
	// List of paths to exclude when fetching
	FetchExcludePaths []string
	// Combination of root .gitconfig and repository config as map
	GitConfig map[string]string
}

func NewOptions() *Options {
	return &Options{
		StringOpts:                  make(map[string]string),
		BoolOpts:                    NewStringSet(),
		Args:                        make([]string, 0, 5),
		GitConfig:                   make(map[string]string),
		FetchRefsPeriodDays:         30,
		FetchCommitsPeriodHEAD:      7,
		FetchCommitsPeriodOther:     0,
		FetchIncludePaths:           []string{},
		FetchExcludePaths:           []string{},
		RetentionRefsPeriod:         30,
		RetentionCommitsPeriodHEAD:  7,
		RetentionCommitsPeriodOther: 0,
	}
}

// Load config from gitconfig and populate opts
func LoadConfig(opts *Options) {
	configmap := ReadConfig()
	parseConfig(configmap, opts)
}

// Parse a loaded config map and populate opts
func parseConfig(configmap map[string]string, opts *Options) {
	opts.GitConfig = configmap

	// Translate our settings to config
	if strings.ToLower(configmap["git-lob.verbose"]) == "true" {
		opts.Verbose = true
	}
	if strings.ToLower(configmap["git-lob.quiet"]) == "true" {
		opts.Quiet = true
	}
	if strings.ToLower(configmap["git-lob.logenabled"]) == "true" {
		opts.LogEnabled = true
	}
	logfile := configmap["git-lob.logfile"]
	if logfile != "" {
		opts.LogFile = logfile
	}
	if strings.ToLower(configmap["git-lob.logverbose"]) == "true" {
		opts.VerboseLog = true
	}
	if sharedStore := configmap["git-lob.sharedstore"]; sharedStore != "" {
		sharedStore = filepath.Clean(sharedStore)
		exists, isDir := FileOrDirExists(sharedStore)
		if exists && !isDir {
			LogErrorf("Invalid path for git-lob.sharedstore: %v\n", sharedStore)
		} else {
			if !exists {
				err := os.MkdirAll(sharedStore, 0755)
				if err != nil {
					LogErrorf("Unable to create path for git-lob.sharedstore: %v\n", sharedStore)
				} else {
					exists = true
					isDir = true
				}
			}

			if exists && isDir {
				opts.SharedStore = sharedStore
			}
		}
	}
	if strings.ToLower(configmap["git-lob.autofetch"]) == "true" {
		opts.AutoFetchEnabled = true
	}

	//git-lob.fetch-refs
	//git-lob.fetch-commits-head
	//git-lob.fetch-commits-other default
	if recentrefs := configmap["git-lob.fetch-refs"]; recentrefs != "" {
		n, err := strconv.ParseInt(recentrefs, 10, 0)
		if err == nil {
			opts.FetchRefsPeriodDays = int(n)
		}
	}
	if recent := configmap["git-lob.fetch-commits-head"]; recent != "" {
		n, err := strconv.ParseInt(recent, 10, 0)
		if err == nil {
			opts.FetchCommitsPeriodHEAD = int(n)
		}
	}
	if recent := configmap["git-lob.fetch-commits-other"]; recent != "" {
		n, err := strconv.ParseInt(recent, 10, 0)
		if err == nil {
			opts.FetchCommitsPeriodOther = int(n)
		}
	}
	//git-lob.retention-period-refs
	//git-lob.retention-period-head
	//git-lob.retention-period-other
	if recentrefs := configmap["git-lob.retention-period-refs"]; recentrefs != "" {
		n, err := strconv.ParseInt(recentrefs, 10, 0)
		if err == nil {
			opts.RetentionRefsPeriod = int(n)
		}
	}
	if recent := configmap["git-lob.retention-period-head"]; recent != "" {
		n, err := strconv.ParseInt(recent, 10, 0)
		if err == nil {
			opts.RetentionCommitsPeriodHEAD = int(n)
		}
	}
	if recent := configmap["git-lob.retention-period-other"]; recent != "" {
		n, err := strconv.ParseInt(recent, 10, 0)
		if err == nil {
			opts.RetentionCommitsPeriodOther = int(n)
		}
	}
	if fetchincludes := configmap["git-lob.fetch-include"]; fetchincludes != "" {
		// Split on comma
		for _, inc := range strings.Split(fetchincludes, ",") {
			inc = strings.TrimSpace(inc)
			opts.FetchIncludePaths = append(opts.FetchIncludePaths, inc)
		}
	}
	if fetchexcludes := configmap["git-lob.fetch-exclude"]; fetchexcludes != "" {
		// Split on comma
		for _, ex := range strings.Split(fetchexcludes, ",") {
			ex = strings.TrimSpace(ex)
			opts.FetchExcludePaths = append(opts.FetchExcludePaths, ex)
		}
	}

}

// Read .gitconfig / .git/config for specific options to override
// Returns a map of setting=value, where group levels are indicated by dot-notation
// e.g. git-lob.logfile=blah
// all keys are converted to lower case for easier matching
func ReadConfig() map[string]string {
	// Don't call out to 'git config' to read file, that's slower and forces a dependency on git
	// which we may not want to have (e.g. support for libgit clients)
	// Read files directly, it's a simple format anyway

	// TODO system git config?

	var ret map[string]string = nil

	// User config
	usr, err := user.Current()
	if err != nil {
		LogError("Unable to access user home directory")
		// continue anyway
	} else {
		userConfigFile := path.Join(usr.HomeDir, ".gitconfig")
		userConfig, err := ReadConfigFile(userConfigFile)
		if err == nil {
			if ret == nil {
				ret = userConfig
			} else {
				for key, val := range userConfig {
					ret[key] = val
				}
			}
		}
	}

	// repo config
	gitDir := GetGitDir()
	repoConfigFile := path.Join(gitDir, "config")
	repoConfig, err := ReadConfigFile(repoConfigFile)
	if err == nil {
		if ret == nil {
			ret = repoConfig
		} else {
			for key, val := range repoConfig {
				ret[key] = val
			}
		}
	}

	if ret == nil {
		ret = make(map[string]string)
	}

	return ret

}

// Read a specific .gitconfig-formatted config file
// Returns a map of setting=value, where group levels are indicated by dot-notation
// e.g. git-lob.logfile=blah
// all keys are converted to lower case for easier matching
func ReadConfigFile(filepath string) (map[string]string, error) {
	f, err := os.OpenFile(filepath, os.O_RDONLY, 0644)
	if err != nil {
		return make(map[string]string), err
	}
	defer f.Close()

	// Need the directory for relative path includes
	dir := path.Dir(filepath)
	return ReadConfigStream(f, dir)

}
func ReadConfigStream(in io.Reader, dir string) (map[string]string, error) {
	ret := make(map[string]string, 10)
	sectionRegex := regexp.MustCompile(`^\[(.*)\]$`)                    // simple section regex ie [section]
	namedSectionRegex := regexp.MustCompile(`^\[(.*)\s+\"(.*)\"\s*\]$`) // named section regex ie [section "name"]

	scanner := bufio.NewScanner(in)
	var currentSection string
	var currentSectionName string
	for scanner.Scan() {
		// Reads lines by default, \n is already stripped
		line := strings.TrimSpace(scanner.Text())
		// Detect comments - discard any of the line after the comment but keep anything before
		commentPos := strings.IndexAny(line, "#;")
		if commentPos != -1 {
			// skip comments
			if commentPos == 0 {
				continue
			} else {
				// just strip rest of line after the comment
				line = strings.TrimSpace(line[0:commentPos])
				if len(line) == 0 {
					continue
				}
			}
		}

		// Check for sections
		if secmatch := sectionRegex.FindStringSubmatch(line); secmatch != nil {
			// named section? [section "name"]
			if namedsecmatch := namedSectionRegex.FindStringSubmatch(line); namedsecmatch != nil {
				// Named section
				currentSection = namedsecmatch[1]
				currentSectionName = namedsecmatch[2]

			} else {
				// Normal section
				currentSection = secmatch[1]
				currentSectionName = ""
			}
			continue
		}

		// Otherwise, probably a standard setting
		equalPos := strings.Index(line, "=")
		if equalPos != -1 {
			name := strings.TrimSpace(line[0:equalPos])
			value := strings.TrimSpace(line[equalPos+1:])
			if currentSection != "" {
				if currentSectionName != "" {
					name = fmt.Sprintf("%v.%v.%v", currentSection, currentSectionName, name)
				} else {
					name = fmt.Sprintf("%v.%v", currentSection, name)
				}
			}
			// convert key to lower case for easier matching
			name = strings.ToLower(name)

			// Check for includes and expand immediately
			if name == "include.path" {
				// if this is a relative, prepend containing dir context
				includeFile := value
				if !path.IsAbs(includeFile) {
					includeFile = path.Join(dir, includeFile)
				}
				includemap, err := ReadConfigFile(includeFile)
				if err == nil {
					for key, value := range includemap {
						ret[key] = value
					}
				}
			} else {
				ret[name] = value
			}
		}

	}
	if scanner.Err() != nil {
		// Problem (other than io.EOF)
		// return content we read up to here anyway
		return ret, scanner.Err()
	}

	return ret, nil

}

// Write a .gitconfig-style config file
// Takes a map of setting=value, where group levels are indicated by dot-notation
// e.g. git-lob.logfile=blah
// Note: overwrites whole file & loses comments if you ReadConfigFile then WriteConfigFile
// only intended for internal use, don't use on user-edited files
// This is NOT a merge-on-write user-friendly config updater (like SourceTree has)
func WriteConfigFile(filepath string, contents map[string]string) error {
	f, err := os.OpenFile(filepath, os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	return WriteConfigStream(f, contents)

}
func WriteConfigStream(out io.Writer, contents map[string]string) error {
	// We need to iterate over content IN ORDER so that we can group correctly
	// golang map iteration is not ordered though so we need to sort keys & iterate on those
	keys := make([]string, 0, len(contents))
	for key, _ := range contents {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lastGroup := ""
	for _, key := range keys {
		val := contents[key]
		splitkey := strings.SplitN(key, ".", 1)
		var group string
		if len(splitkey) > 1 {
			group = splitkey[0]
			if strings.ContainsAny(group, " \t") {
				group = fmt.Sprintf("\"%v\"", group)
			}
			key = splitkey[1]
		}
		if group != lastGroup {
			_, err := out.Write([]byte(fmt.Sprintf("[%v]\n", group)))
			if err != nil {
				return err
			}
			lastGroup = group
		}

		if group != "" {
			// Indent values in group
			out.Write([]byte{'\t'})
		}
		_, err := out.Write([]byte(fmt.Sprintf("%v = %v\n", key, val)))
		if err != nil {
			return err
		}

	}
	return nil

}
