package main

import (
	"fmt"
	"os"
)

func createGitDirs(rootDir string, ref string) error {
	for _, dir := range []string{".git", ".git/objects", ".git/refs"} {
		if err := os.MkdirAll(rootDir+"/"+dir, 0755); err != nil {
			return fmt.Errorf("Error creating directory: %s\n", err)
		}
	}

	headFileContents := []byte(fmt.Sprintf("ref: %v\n", ref))
	if err := os.WriteFile(rootDir+"/.git/HEAD", headFileContents, 0644); err != nil {
		return fmt.Errorf("Error writing file: %s\n", err)
	}

	return nil
}
