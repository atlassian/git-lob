@echo off

rem Install script which inserts a build ID into the code (short git hash)
rem requires that git & go are on the path, which of course they are in your environment

rem backtick equivalent
git rev-parse --short HEAD > version.txt
set /p SHA=<version.txt
del version.txt

go install -ldflags "-X main.VersionBuildID %SHA%"