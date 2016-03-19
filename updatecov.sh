#!/bin/bash

test -n "$COVERALLS_TOKEN" || {
	echo "COVERALLS_TOKEN not set, skipping coveralls"
	exit 0
}

echo "mode: set" > profile
for dir in $(find . -maxdepth 10 -name '*.go' -exec dirname '{}' \; | uniq ); do
	go test -v -coverprofile=p $dir > out 2>&1 || {
		cat out
		rm -f p out
		exit 1
	}
	test -f p && cat p | grep -v "mode: set" >> profile
	rm -f p out
done

$HOME/gopath/bin/goveralls -coverprofile=profile -service=travis-ci -repotoken $COVERALLS_TOKEN
rm -f profile
