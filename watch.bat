rem watches, compiles & runs tests
rem skips running any tests which have LONG in description
ginkgo watch -r --skip="(LONGTEST|REMOTETEST)"
