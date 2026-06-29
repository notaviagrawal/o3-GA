package tools

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var Flags []string = ReturnAllFlags(os.Args[2])

func ReturnAllFlags(app string) []string {
	arg1 := "--help=optimizers"
	cmd := exec.Command(app, arg1)
	stdout, err := cmd.Output()
	if err != nil {
		fmt.Println(err.Error())
	}
	output := string(stdout)
	var re = regexp.MustCompile("(?m)^  (-f[a-z0-9-]+) ")

	var flags []string
	flags = append(flags, re.FindAllString(output, -1)...)
	// Removing each -f flag from begining.
	for idx, v := range flags {
		flags[idx] = strings.Replace(v, "-f", "", 1)
		flags[idx] = strings.Replace(flags[idx], " ", "", 3)

	}
	return flags
}
