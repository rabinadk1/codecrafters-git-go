package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"os"
	"strconv"
	"strings"
)

func parseFile(file fs.DirEntry, rootDir string) ([]byte, error) {
	info, err := file.Info()
	if err != nil {
		return nil, fmt.Errorf("Error getting file info: %s\n", err)
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
		hash, err = writeTree(rootDir+"/"+file.Name(), false)
		if err != nil {
			return nil, fmt.Errorf("Error writing tree: %s\n", err)
		}
	} else {
		hash, err = writeBlob(rootDir+"/"+file.Name(), true, false)
		if err != nil {
			return nil, fmt.Errorf("Error writing blob: %s\n", err)
		}
	}

	return append([]byte(fmt.Sprintf("%d %s\x00", mode, file.Name())), hash...), nil
}

func writeTree(rootDir string, printHash bool) ([]byte, error) {
	files, err := os.ReadDir(rootDir)
	if err != nil {
		return nil, fmt.Errorf("Error reading directory: %s\n", err)
	}

	var byteContent []byte

	for _, file := range files {
		if file.Name() == ".git" {
			continue
		}
		currFileContent, err := parseFile(file, rootDir)
		if err != nil {
			return nil, fmt.Errorf("Error parsing file: %s\n", err)
		}

		byteContent = append(byteContent, currFileContent...)
	}

	size := len(byteContent)
	byteContent = append([]byte(fmt.Sprintf("tree %d\x00", size)), byteContent...)

	hash, hexHash := getHexHash(byteContent)

	if printHash {
		fmt.Println(hexHash)
	}

	writeCompressedObject(byteContent, hexHash, ".")

	return hash[:], nil
}

func parseTree(treeData []byte, rootDir string, gitDir string) error {
	// fmt.Printf("Creating directory: %v\n", rootDir)
	err := os.MkdirAll(rootDir, 0755)
	if err != nil {
		return fmt.Errorf("Error creating directory: %s\n", err)
	}

	// Skip initial tree<size>\0 prefix
	nullIndex := bytes.IndexByte(treeData, 0)
	treeData = treeData[nullIndex+1:]

	for len(treeData) > 0 {
		nullIndex = bytes.IndexByte(treeData, 0)

		fileInfo := string(treeData[:nullIndex])

		parts := strings.SplitN(fileInfo, " ", 2)

		if len(parts) != 2 {
			log.Fatalf("Malformed tree data: %s\n", fileInfo)
		}

		fileMode, fileName := parts[0], parts[1]

		// fmt.Println(fileMode, fileName)

		hashEndIndex := nullIndex + 21
		fileHash := treeData[nullIndex+1 : hashEndIndex]

		hexHash := hex.EncodeToString(fileHash)

		if fileMode == "40000" {
			// tree
			subTreeData, err := loadAndDecompressObject(hexHash, gitDir)
			if err != nil {
				return fmt.Errorf("Error loading tree data: %s\n", err)
			}

			parseTree(subTreeData, rootDir+"/"+fileName, gitDir)
		} else if fileMode[0] == '1' {
			// file or link
			blobData, err := loadAndDecompressObject(hexHash, gitDir)
			if err != nil {
				return fmt.Errorf("Error loading blob data: %s\n", err)
			}

			nullIndex := bytes.IndexByte(blobData, 0)

			fileContent := blobData[nullIndex+1:]

			outputFilePath := rootDir + "/" + fileName

			filePerms, err := strconv.ParseInt(fileMode[len(fileMode)-3:], 8, 0)
			if err != nil {
				return fmt.Errorf("Error parsing file mode: %s\n", err)
			}

			if err := os.WriteFile(outputFilePath, fileContent, os.FileMode(filePerms)); err != nil {
				return fmt.Errorf("Error writing file: %s\n", err)
			}
		} else {
			return fmt.Errorf("Saving is not curretly implemented for symbolic links.")
		}

		treeData = treeData[hashEndIndex:]

	}

	return nil
}
