package main

import (
	"fmt"
	"io/fs"
	"log"
	"os"
)

func parseFile(file fs.DirEntry, rootDir string) []byte {
	info, err := file.Info()
	if err != nil {
		log.Fatalf("Error getting file info: %s\n", err)
	}

	fMode := info.Mode()

	isSymlink := fMode&os.ModeSymlink != 0

	var mode uint32
	if file.IsDir() {
		mode = 40000
	} else if isSymlink {
		mode = 120000
	} else {
		fPerm := uint32(fMode.Perm())
		const maxBits = 7
		mode = 100000 + ((fPerm>>6)&maxBits)*100 + ((fPerm>>3)&maxBits)*10 + (fPerm & maxBits)
	}

	var hash []byte
	if file.IsDir() {
		hash = writeTree(rootDir+"/"+file.Name(), false)
	} else {
		hash = writeBlob(rootDir+"/"+file.Name(), true, false)
	}

	return append([]byte(fmt.Sprintf("%d %s\x00", mode, file.Name())), hash...)
}

func writeTree(rootDir string, printHash bool) []byte {
	files, err := os.ReadDir(rootDir)
	if err != nil {
		log.Fatalf("Error reading directory: %s\n", err)
	}

	var byteContent []byte

	for _, file := range files {
		if file.Name() == ".git" {
			continue
		}
		byteContent = append(byteContent, parseFile(file, rootDir)...)
	}

	size := len(byteContent)
	byteContent = append([]byte(fmt.Sprintf("tree %d\x00", size)), byteContent...)

	hash, hexHash := getHexHash(byteContent)

	if printHash {
		fmt.Println(hexHash)
	}

	writeCompressedObject(byteContent, hexHash, ".")

	return hash[:]
}
