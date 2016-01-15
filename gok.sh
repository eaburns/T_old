#!/bin/sh
#
# Verifies that go code passes go fmt, go vet, golint, and go test.
#

o=$(mktemp tmp.XXXXXXXXXX)

fail() {
	echo Failed
	cat $o
	rm $o
	exit 1
}

echo Formatting
gofmt -l $(find . -name '*.go') 2>&1 > $o
test $(wc -l $o | awk '{ print $1 }') = "0" || fail

echo Vetting
go vet ./... 2>&1 > $o || fail

echo Testing
go test -test.timeout=1s ./... 2>&1 > $o || fail

echo Linting
golint .\
	| grep -v 'should omit type Address'\
	| grep -v 'should omit type SimpleAddress'\
	> $o 2>&1
# Silly: diff the grepped golint output with empty.
# If it's non-empty, error, otherwise succeed.
e=$(tempfile)
touch $e
diff $o $e > /dev/null || { rm $e; fail; }

echo Leaks?
ls /tmp | egrep "edit" 2>&1 > $o && fail

rm $o $e
