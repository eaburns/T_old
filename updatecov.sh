#!/bin/sh
test -n "$COVERALLS_TOKEN" || {
	echo "COVERALLS_TOKEN not set, skipping coveralls"
	exit 0
}
dirs=$(for dir in $(find . -name \*.go); do dirname $dir; done | sort | uniq)
gocov test $dirs > gocov.json || exit 1
goveralls -gocovdata=gocov.json -service=travis-ci -repotoken $COVERALLS_TOKEN
rm -f gocov.json

