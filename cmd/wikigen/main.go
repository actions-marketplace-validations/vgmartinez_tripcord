// Command wikigen renders docs/ into GitHub Wiki pages.
//
//	go run ./cmd/wikigen -out DIR [-root .]
//
// DIR must be absent or empty. The wiki-sync workflow pushes its contents
// to the wiki repository; nothing else should write the wiki.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/vgmartinez/tripcord/internal/wiki"
)

func main() {
	out := flag.String("out", "", "directory to write the pages to (must be absent or empty)")
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	if *out == "" || flag.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "usage: wikigen -out DIR [-root DIR]")
		os.Exit(2)
	}
	site, err := wiki.Render(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wikigen: %v\n", err)
		os.Exit(1)
	}
	if err := site.CheckRepoFile("README.md"); err != nil {
		fmt.Fprintf(os.Stderr, "wikigen: %v\n", err)
		os.Exit(1)
	}
	if err := wiki.Write(*out, site.Pages); err != nil {
		fmt.Fprintf(os.Stderr, "wikigen: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wikigen: wrote %d pages to %s\n", len(site.Pages), *out)
}
