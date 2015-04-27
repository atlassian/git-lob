package main

import (
	"bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/mitchellh/go-homedir"
	"bitbucket.org/sinbad/git-lob/util"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	BasePath           string
	AllowAbsolutePaths bool
	EnableDeltaReceive bool
	EnableDeltaSend    bool
	DeltaCachePath     string
	DeltaSizeLimit     int64
}

const defaultDeltaSizeLimit int64 = 2 * 1024 * 1024 * 1024

func NewConfig() *Config {
	return &Config{
		AllowAbsolutePaths: false,
		EnableDeltaReceive: true,
		EnableDeltaSend:    true,
		DeltaSizeLimit:     defaultDeltaSizeLimit, // 2GB
	}
}
func LoadConfig() *Config {
	// Support gitconfig-style configuration in:
	// Linux/Mac:
	// ~/.git-lob-serve
	// /etc/git-lob-serve.conf
	// Windows:
	// %USERPROFILE%\git-lob-serve.ini
	// %PROGRAMDATA%\Atlassian\git-lob\git-lob-serve.ini

	var configFiles []string
	home, herr := homedir.Dir()
	if herr != nil {
		fmt.Fprint(os.Stderr, "Warning, couldn't locate home directory: %v", herr.Error())
	}

	// Order is important; read global config files first then user config files so settings
	// in the latter override the former
	if util.IsWindows() {
		progdata := os.Getenv("PROGRAMDATA")
		if progdata != "" {
			configFiles = append(configFiles, filepath.Join(progdata, "Atlassian", "git-lob-serve.ini"))
		}
		if home != "" {
			configFiles = append(configFiles, filepath.Join(home, "git-lob-serve.ini"))
		}
	} else {
		configFiles = append(configFiles, "/etc/git-lob-serve.conf")
		if home != "" {
			configFiles = append(configFiles, filepath.Join(home, ".git-lob-serve"))
		}
	}

	var settings map[string]string
	for _, conf := range configFiles {
		confsettings, err := util.ReadConfigFile(conf)
		if err == nil {
			for key, val := range confsettings {
				settings[key] = val
			}
		}
	}

	// Convert to Config
	cfg := NewConfig()
	if v := settings["base-path"]; v != "" {
		cfg.BasePath = filepath.Clean(v)
	}
	if v := strings.ToLower(settings["allow-absolute-paths"]); v != "" {
		if v == "true" {
			cfg.AllowAbsolutePaths = true
		} else if v == "false" {
			cfg.AllowAbsolutePaths = false
		}
	}
	if v := strings.ToLower(settings["enable-delta-receive"]); v != "" {
		if v == "true" {
			cfg.EnableDeltaReceive = true
		} else if v == "false" {
			cfg.EnableDeltaReceive = false
		}
	}
	if v := strings.ToLower(settings["enable-delta-send"]); v != "" {
		if v == "true" {
			cfg.EnableDeltaSend = true
		} else if v == "false" {
			cfg.EnableDeltaSend = false
		}
	}
	if v := settings["delta-cache-path"]; v != "" {
		cfg.DeltaCachePath = v
	}

	if cfg.DeltaCachePath == "" && cfg.BasePath != "" {
		cfg.DeltaCachePath = filepath.Join(cfg.BasePath, ".deltacache")
	}

	if v := settings["delta-size-limit"]; v != "" {
		var err error
		cfg.DeltaSizeLimit, err = strconv.ParseInt(v, 0, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid configuration: delta-size-limit=%v\n", v)
			cfg.DeltaSizeLimit = defaultDeltaSizeLimit
		}
	}

	return cfg
}
