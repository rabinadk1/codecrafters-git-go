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

func getHexHash(content []byte) ([]byte, string) {
	hash := sha1.Sum(content)
	return hash[:], hex.EncodeToString(hash[:])
}

func writeCompressedObject(content []byte, hexHash string, rootDir string) {
	var compressedBytes bytes.Buffer
	w := zlib.NewWriter(&compressedBytes)
	w.Write(content)
	w.Close()

	writeDir := rootDir + "/.git/objects/" + hexHash[:2]
	if err := os.MkdirAll(writeDir, 0755); err != nil {
		log.Fatalf("Error creating directory: %s\n", err)
	}

	if err := os.WriteFile(writeDir+"/"+hexHash[2:], compressedBytes.Bytes(), 0644); err != nil {
		log.Fatalf("Error writing file: %s\n", err)
	}
}

func hashAndSaveObjects(content []byte, rootDir string) string {
	_, hexHash := getHexHash(content)

	writeCompressedObject(content, hexHash, rootDir)

	return hexHash
}

func writeBlob(filepath string, writeObject bool, printHash bool) []byte {
	content, err := os.ReadFile(filepath)
	if err != nil {
		log.Fatalf("Error opening file: %s\n", err)
	}

	size := len(content)

	prefix := []byte(fmt.Sprintf("blob %v\x00", size))

	content = append(prefix, content...)

	hash, hexHash := getHexHash(content)

	if printHash {
		fmt.Println(hexHash)
	}

	if writeObject {
		writeCompressedObject(content, hexHash, ".")
	}

	return hash
}
