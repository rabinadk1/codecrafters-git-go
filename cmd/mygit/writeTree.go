package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"os"
)

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
			hash = hashObject(rootDir+"/"+file.Name(), true, false)
		}

		byteContent = append(append(byteContent, []byte(fmt.Sprintf("%d %s\x00", mode, file.Name()))...), hash...)

	}

	size := len(byteContent)
	byteContent = append([]byte(fmt.Sprintf("tree %d\x00", size)), byteContent...)

	hash := sha1.Sum(byteContent)
	hexHash := hex.EncodeToString(hash[:])

	if printHash {
		fmt.Println(hexHash)
	}

	var compressedBytes bytes.Buffer
	w := zlib.NewWriter(&compressedBytes)
	w.Write(byteContent)
	w.Close()

	writeDir := ".git/objects/" + hexHash[:2]
	if err := os.MkdirAll(writeDir, 0755); err != nil {
		log.Fatalf("Error creating directory: %s\n", err)
	}

	if err := os.WriteFile(writeDir+"/"+hexHash[2:], compressedBytes.Bytes(), 0644); err != nil {
		log.Fatalf("Error writing file: %s\n", err)
	}

	return hash[:]
}
