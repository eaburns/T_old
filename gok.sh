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

trap fail INT TERM

echo Formatting
gofmt -l $(find . -name '*.go') 2>&1 > $o
test $(wc -l $o | awk '{ print $1 }') = "0" || fail

echo Vetting
go vet ./... 2>&1 > $o || fail

echo Testing
go test -test.timeout=10s ./... 2>&1 > $o || fail

echo Linting
golint .\
	| grep -v 'should omit type SimpleAddress'\
	| grep -v 'Buffer.Mark should have comment'\
	| grep -v 'Buffer.SetMark should have comment'\
	| grep -v 'Buffer.Change should have comment'\
	| grep -v 'Buffer.Apply should have comment'\
	| grep -v 'Buffer.Cancel should have comment'\
	| grep -v 'Buffer.Undo should have comment'\
	| grep -v 'Buffer.Reader should have comment'\
	| grep -v 'Buffer.Redo should have comment'\
	| grep -v 'Substitute.Do should have comment'\
	| grep -v 'MarshalText should have comment'\
	| grep -v 'UnmarshalText should have comment'\
	| egrep -v "don't use underscores.*Test.*"\
	> $o 2>&1
# Silly: diff the grepped golint output with empty.
# If it's non-empty, error, otherwise succeed.
e=$(tempfile)
touch $e
diff $o $e > /dev/null || { rm $e; fail; }

echo Leaks?
ls /tmp | egrep "edit" 2>&1 > $o && fail

rm $o $e
