#!/bin/sh

# This script runs a full test suite - not just Go tests but integration tests
# with git filters that cannot be run inside ginkgo because they require an
# external environment with a proper build of git-lob (tests don't produce a 
# binary)

# exit on non-zero result from any command & fail on undeclared
set -o errexit ; set -o nounset

# First, run normal tests
echo "Running main test suite"
ginkgo

# Build git-lob binary (local only, don't install) for use by filters
echo "Building git-lob binary"
go build

# Now, run any integration tests
for f in ./integration_tests/*.sh
do
    echo "Running $f"
    sh $f
done
