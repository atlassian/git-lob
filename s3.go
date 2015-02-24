package main

import (
	"errors"
	"fmt"
	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/s3"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// S3SyncProvider implements the basic SyncProvider interface for S3
type S3SyncProvider struct {
	S3Connection *s3.S3
	Buckets      []string
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
    git-lob-s3-bucket   The bucket to use as the root remote store. Must already exist.
    git-lob-s3-region   The AWS region to use. If not specified will use region settings
                        from your ~/.aws/config. If no region is specified, uses US East.
    git-lob-s3-profile  The profile to use to authenticate for this remote. Can also 
                        be set in other ways, see global settings below.

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

  If using the credentials file, the [default] profile is used unless
  you specify otherwise. You can specify what profile to use several ways:
    1. In .git/config, remote.REMOTE.git-lob-s3-profile 
    2. git-lob.s3-profile in repo or global gitconfig
    3. AWS_PROFILE in your environment.

  Region settings are also read from your config file in ~/.aws/config.
  See:
  http://docs.aws.amazon.com/cli/latest/userguide/cli-chap-getting-started.html
  for more details on the configuration process.
`
}

// Configure the profile to use for a given remote. Preferences in order:
// Git setting remote.REMOTENAME.git-lob-s3-profile
// Git setting git-lob.s3-profile
// AWS_PROFILE environment
func (self *S3SyncProvider) configureProfile(remoteName string) {
	// check whether git-lob-s3-profile has been specified; if so override local environment
	// so s3 library will pick it up
	// this allows per-repo credential profiles which is useful
	profilesetting := fmt.Sprintf("remote.%v.git-lob-s3-profile", remoteName)
	profile := strings.TrimSpace(GlobalOptions.GitConfig[profilesetting])
	if profile == "" {
		profilesetting = "git-lob.s3-profile"
		profile = strings.TrimSpace(GlobalOptions.GitConfig[profilesetting])
	}
	if profile != "" {
		// If we've retrieved the setting from our git config,
		// set it in the environment so S3 lib will use it
		os.Setenv("AWS_PROFILE", profile)
	}
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

	// Read bucket list right now since we have no way to probe whether a bucket exists
	self.S3Connection.ListBuckets()

	return nil
}
func (self *S3SyncProvider) getS3Connection() (*s3.S3, error) {
	if self.S3Connection == nil {
		err := self.initS3()
		if err != nil {
			return nil, err
		}
	}
	return self.S3Connection, nil
}
func (self *S3SyncProvider) getBucketName(remoteName string) (string, error) {
	bucketsetting := fmt.Sprintf("remote.%v.git-lob-s3-bucket", remoteName)
	bucket := strings.TrimSpace(GlobalOptions.GitConfig[bucketsetting])
	if bucket == "" {
		return "", fmt.Errorf("Configuration invalid for 'filesystem', missing setting %v", bucketsetting)
	}
	return bucket, nil
}
func (self *S3SyncProvider) getBucket(remoteName string) (*s3.Bucket, error) {
	bucketname, err := self.getBucketName(remoteName)
	if err != nil {
		return nil, err
	}
	conn, err := self.getS3Connection()
	if err != nil {
		return nil, err
	}
	// Make sure we configure the correct profile for access to bucket
	self.configureProfile(remoteName)
	return conn.Bucket(bucketname), nil
}

func (self *S3SyncProvider) ValidateConfig(remoteName string) error {
	_, err := self.getBucketName(remoteName)
	if err != nil {
		return err
	}
	return nil
}

func (self *S3SyncProvider) FileExists(remoteName, filename string) bool {
	bucket, err := self.getBucket(remoteName)
	if err != nil {
		return false
	}
	key, err := bucket.GetKey(filename)
	return err == nil && key != nil
}
func (self *S3SyncProvider) FileExistsAndIsOfSize(remoteName, filename string, sz int64) bool {
	bucket, err := self.getBucket(remoteName)
	if err != nil {
		return false
	}
	key, err := bucket.GetKey(filename)
	return err == nil && key != nil && key.Size == sz
}

func (*S3SyncProvider) uploadSingleFile(remoteName, filename, fromDir string, destBucket *s3.Bucket,
	force bool, callback SyncProgressCallback) (errorList []string, abort bool) {
	// Check to see if the file is already there, right size
	srcfilename := filepath.Join(fromDir, filename)
	srcfi, err := os.Stat(srcfilename)
	if err != nil {
		if callback != nil {
			if callback(filename, ProgressNotFound, 0, 0) {
				return errorList, true
			}
		}
		msg := fmt.Sprintf("Unable to stat %v: %v", srcfilename, err)
		errorList = append(errorList, msg)
		// Keep going with other files
		return errorList, false
	}

	if !force {
		// Check if already there before uploading
		if key, err := destBucket.GetKey(filename); key != nil && err == nil {
			// File exists on remote, check the size
			if key.Size == srcfi.Size() {
				// File already present and correct size, skip
				if callback != nil {
					if callback(filename, ProgressSkip, srcfi.Size(), srcfi.Size()) {
						return errorList, true
					}
				}
				return errorList, false
			}

		}
	}

	// We don't need to create a temporary file on S3 to deal with interrupted uploads, because
	// the file is not fully created in the bucket until fully uploaded
	inf, err := os.OpenFile(srcfilename, os.O_RDONLY, 0644)
	if err != nil {
		msg := fmt.Sprintf("Unable to read input file for upload %v: %v", srcfilename, err)
		errorList = append(errorList, msg)
		return errorList, false
	}
	defer inf.Close()

	// Initial callback
	if callback != nil {
		if callback(filename, ProgressTransferBytes, 0, srcfi.Size()) {
			return errorList, true
		}
	}

	// Create a Reader which reports progress as it is read from
	progressReader := NewSyncProgressReader(inf, filename, srcfi.Size(), callback)
	// Note default ACL
	err = destBucket.PutReader(filename, progressReader, srcfi.Size(), "binary/octet-stream", "")
	if err != nil {
		errorList = append(errorList, fmt.Sprintf("Problem while uploading %v to %v: %v", filename, remoteName, err))
	}

	return errorList, progressReader.Aborted

}

func (self *S3SyncProvider) Upload(remoteName string, filenames []string, fromDir string,
	force bool, callback SyncProgressCallback) error {

	bucket, err := self.getBucket(remoteName)
	if err != nil {
		return err
	}

	LogDebug("Uploading to S3 bucket", bucket.Name)

	// Check bucket exists (via HEAD endpoint)
	// This saves us failing on every file
	_, err = bucket.Head("/")
	if err != nil {
		return fmt.Errorf("Unable to access S3 bucket '%v' for remote '%v': %v", bucket.Name, err.Error())
	}

	var errorList []string
	for _, filename := range filenames {
		// Allow aborting
		newerrs, abort := self.uploadSingleFile(remoteName, filename, fromDir, bucket, force, callback)
		errorList = append(errorList, newerrs...)
		if abort {
			break
		}
	}

	if len(errorList) > 0 {
		return errors.New(strings.Join(errorList, "\n"))
	}

	return nil
}
