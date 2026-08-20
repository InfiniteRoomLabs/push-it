// Command push-it celebrates git pushes with sound, light, and screen glow.
package main

import (
	"fmt"
	"io"
	"os"
	"sort"
)

// version is overwritten at build time via -ldflags "-X main.version=v1.2.3".
var version = "dev"

type command func(args []string, stdin io.Reader, stdout, stderr io.Writer) int

// commands is populated by init() functions in the other files of this package.
var commands = map[string]command{}

func init() {
	commands["version"] = func(_ []string, _ io.Reader, stdout, _ io.Writer) int {
		fmt.Fprintf(stdout, "push-it %s\n", version)
		return 0
	}
}

func usage(w io.Writer) {
	names := make([]string, 0, len(commands))
	for n := range commands {
		names = append(names, n)
	}
	sort.Strings(names)
	fmt.Fprintln(w, "usage: push-it <command> [flags]")
	fmt.Fprintln(w, "commands:")
	for _, n := range names {
		fmt.Fprintf(w, "  %s\n", n)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(stderr, "push-it: unknown command %q\n", args[0])
		usage(stderr)
		return 2
	}
	return cmd(args[1:], stdin, stdout, stderr)
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
