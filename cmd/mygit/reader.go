package main

import (
	"fmt"
	"log"
	"os"
)

func getCompressedObjectReader(hexHash, gitDir string) *os.File {
	path := fmt.Sprintf("%v/.git/objects/%v/%v", gitDir, hexHash[:2], hexHash[2:])

	compressedReader, err := os.Open(path)
	if err != nil {
		log.Fatalf("Error reading file: %s\n", err)
	}
	return compressedReader
}
