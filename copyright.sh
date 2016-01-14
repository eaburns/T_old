#!/bin/sh
# Adds a Copyright comment to all .go files without one.

for f in $(find . -name \*.go); do
	if grep -q "Copyright" $f; then
		echo skipping $f
		continue
	fi
	mv $f ${f}~
	echo -e "// Copyright © 2016, The T Authors.\n" > $f
	cat ${f}~ >> $f	
	rm ${f}~
done