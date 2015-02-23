package main

import (
	"errors"
	"fmt"
	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/s3"
	"os"
	"os/user"
	"path/filepath"
)

// S3SyncProvider implements the basic SyncProvider interface for S3
type S3SyncProvider struct {
	S3Connection *s3.S3
}

func (*S3SyncProvider) TypeID() string {
	return "s3"
}

func (*S3SyncProvider) HelpTextSummary() string {
	return `s3: transfers binaries to/from an S3 bucket`
}

func (*S3SyncProvider) HelpTextDetail() string {
	return `The "s3" provider synchronises files with a bucket on Amazon's S3 cloud storage

Required parameters in remote section of .gitconfig:
    git-lob-s3-bucket   The bucket to use as the root remote store. Will be created if
                        it doesn't already exist
    git-lob-s3-region   The AWS region to use. If not specified will use region settings
                        from your ~/.aws/config. If no region is specified, uses US East.

Example configuration:
    [remote "origin"]
        url = git@blah.com/your/usual/git/repo
        git-lob-provider = s3
        git-lob-s3-bucket = my.binary.bucket

Global AWS settings:

  Authentication is performed using the same configuration you'd use with the
  command line AWS tools. Settings are read in this order:

  1. Environment variables i.e. AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY
  2. Credentials file in ~/.aws/credentials or %USERPROFILE%\.aws\credentials

  In addition, region settings are read from your config file in ~/.aws/config.
`
}

// get auth from the environment or config files
func (self *S3SyncProvider) getAuth() (aws.Auth, error) {
	auth, err := aws.EnvAuth()
	if err != nil {
		auth, err = aws.SharedAuth()
		if err != nil {
			return aws.Auth{}, errors.New("Unable to locate AWS authentication settings in environment or credentials file")
		}
	}
	return auth, nil
}

// get region from the environment or config files
func (self *S3SyncProvider) getRegion() (aws.Region, error) {
	regstr := os.Getenv("AWS_DEFAULT_REGION")
	if regstr == "" {
		// Look for config file
		profile := os.Getenv("AWS_PROFILE")
		if profile == "" {
			profile = "default"
		}

		cfgFile := os.Getenv("AWS_CONFIG_FILE")
		if cfgFile == "" {
			usr, usrerr := user.Current()
			if usrerr == nil {
				cfgFile = filepath.Join(usr.HomeDir, ".aws", "config")
			}
		}
		if cfgFile != "" {
			configmap, err := ReadConfigFile(cfgFile)
			if err == nil {
				regstr = configmap[fmt.Sprintf("%v.region", profile)]
			}
		}
	}
	if regstr != "" {
		reg, ok := aws.Regions[regstr]
		if ok {
			return reg, nil
		}
	}
	// default
	return aws.USEast, nil
}
func (self *S3SyncProvider) initS3() error {
	// Get auth - try environment first
	auth, err := self.getAuth()
	if err != nil {
		return err
	}
	region, err := self.getRegion()
	if err != nil {
		return err
	}
	self.S3Connection = s3.New(auth, region)
	return nil
}

func (*S3SyncProvider) ValidateConfig(remoteName string) error {
	bucketsetting := fmt.Sprintf("remote.%v.git-lob-s3-bucket", remoteName)
	bucket := GlobalOptions.GitConfig[bucketsetting]
	if bucket == "" {
		return fmt.Errorf("Configuration invalid for 'filesystem', missing setting %v", bucketsetting)
	}
	return nil
}
