#!/bin/sh

# watches, compiles & runs tests
# skips running any tests which have LONG in description
ginkgo watch --skip=LONGTEST
