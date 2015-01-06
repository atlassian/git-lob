#!/bin/sh

# Install script which inserts a build ID into the code (short git hash)
# requires that git & go are on the path, which of course they are in your environment

sha=`git rev-parse --short HEAD`
go install -ldflags "-X main.VersionBuildID $sha"