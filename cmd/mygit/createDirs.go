package main

import (
	"fmt"
	"log"
	"os"
)

func createGitDirs(rootDir string, ref string) {
	for _, dir := range []string{".git", ".git/objects", ".git/refs"} {
		if err := os.MkdirAll(rootDir+"/"+dir, 0755); err != nil {
			log.Fatalf("Error creating directory: %s\n", err)
		}
	}

	headFileContents := []byte(fmt.Sprintf("ref: %v\n", ref))
	if err := os.WriteFile(rootDir+"/.git/HEAD", headFileContents, 0644); err != nil {
		log.Fatalf("Error writing file: %s\n", err)
	}
}
