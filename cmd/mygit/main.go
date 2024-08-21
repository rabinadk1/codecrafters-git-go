package main

import (
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"strings"
)

// Usage: your_program.sh <command> <arg1> <arg2> ...
func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: mygit <command> [<args>...]\n")
		os.Exit(1)
	}

	switch command := os.Args[1]; command {
	case "init":
		for _, dir := range []string{".git", ".git/objects", ".git/refs"} {
			if err := os.MkdirAll(dir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating directory: %s\n", err)
			}
		}

		headFileContents := []byte("ref: refs/heads/main\n")
		if err := os.WriteFile(".git/HEAD", headFileContents, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file: %s\n", err)
		}

		fmt.Println("Initialized git directory")

	case "cat-file":
		if len(os.Args) != 4 {
			fmt.Fprintf(os.Stderr, "usage: mygit cat-file <type> <object>\n")
			os.Exit(1)
		}

		sha := os.Args[3]
		path := fmt.Sprintf(".git/objects/%v/%v", sha[:2], sha[2:])
		compressedReader, err := os.Open(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %s\n", err)
			os.Exit(1)
		}
		defer compressedReader.Close()

		z, err := zlib.NewReader(compressedReader)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error decompressing file: %s\n", err)
			os.Exit(1)
		}

		defer z.Close()

		p, err := io.ReadAll(z)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading decompressed reader: %s\n", err)
			os.Exit(1)
		}
		parts := strings.Split(string(p), "\x00")
		fmt.Print(parts[1])

	default:
		fmt.Fprintf(os.Stderr, "Unknown command %s\n", command)
		os.Exit(1)
	}
}
