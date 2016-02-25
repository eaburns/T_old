package re1

import (
	"fmt"
	"strings"
)

func ExampleRegexp_Match() {
	re, err := Compile(strings.NewReader("((a*)b)*"))
	if err != nil {
		panic(err)
	}
	fmt.Println(re.Match(None, strings.NewReader("abb"), None))
	// Output:
	// [[0 3] [2 3] [2 2]]
}
