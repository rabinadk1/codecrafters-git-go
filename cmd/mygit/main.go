package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

func hashObject(filepath string, writeObject bool, printHash bool) []byte {
	content, err := os.ReadFile(filepath)
	if err != nil {
		log.Fatalf("Error opening file: %s\n", err)
	}

	size := len(content)

	prefix := []byte(fmt.Sprintf("blob %v\x00", size))

	content = append(prefix, content...)

	hash := sha1.Sum(content)
	hexHash := hex.EncodeToString(hash[:])

	if printHash {
		fmt.Println(hexHash)
	}

	if writeObject {
		var compressedBytes bytes.Buffer
		w := zlib.NewWriter(&compressedBytes)
		w.Write(content)
		w.Close()

		writeDir := ".git/objects/" + hexHash[:2]
		if err := os.MkdirAll(writeDir, 0755); err != nil {
			log.Fatalf("Error creating directory: %s\n", err)
		}

		if err := os.WriteFile(writeDir+"/"+hexHash[2:], compressedBytes.Bytes(), 0644); err != nil {
			log.Fatalf("Error writing file: %s\n", err)
		}
	}

	return hash[:]
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

func main() {
	if len(os.Args) < 2 {
		log.Fatalln("usage: mygit <command> [<args>...]")
	}

	switch command := os.Args[1]; command {
	case "init":
		for _, dir := range []string{".git", ".git/objects", ".git/refs"} {
			if err := os.MkdirAll(dir, 0755); err != nil {
				log.Fatalf("Error creating directory: %s\n", err)
			}
		}

		headFileContents := []byte("ref: refs/heads/main\n")
		if err := os.WriteFile(".git/HEAD", headFileContents, 0644); err != nil {
			log.Fatalf("Error writing file: %s\n", err)
		}

		fmt.Println("Initialized git directory")

	case "cat-file":
		if len(os.Args) != 4 {
			log.Fatalln("usage: mygit cat-file <type> <object>")
		}

		sha := os.Args[3]
		path := fmt.Sprintf(".git/objects/%v/%v", sha[:2], sha[2:])
		compressedReader, err := os.Open(path)
		if err != nil {
			log.Fatalf("Error reading file: %s\n", err)
		}
		defer compressedReader.Close()

		z, err := zlib.NewReader(compressedReader)
		if err != nil {
			log.Fatalf("Error decompressing file: %s\n", err)
		}

		defer z.Close()

		p, err := io.ReadAll(z)
		if err != nil {
			log.Fatalf("Error reading decompressed reader: %s\n", err)
		}

		parts := strings.Split(string(p), "\x00")
		fmt.Print(parts[1])

	case "hash-object":

		if len(os.Args) < 3 {
			log.Fatalln("usage: mygit hash-object [-w] <file>")
		}

		thirdArg := os.Args[2]

		writeObject := thirdArg == "-w"

		var filepath string
		if writeObject {
			if len(os.Args) < 4 {
				log.Fatalln("usage: mygit hash-object -w <file>")
			}

			filepath = os.Args[3]
		} else {
			filepath = thirdArg
		}

		hashObject(filepath, writeObject, true)

	case "ls-tree":
		if len(os.Args) != 4 {
			log.Fatalln("usage: mygit ls-tree --name-only <tree_sha>")
		}

		sha := os.Args[3]
		path := fmt.Sprintf(".git/objects/%v/%v", sha[:2], sha[2:])

		compressedReader, err := os.Open(path)
		if err != nil {
			log.Fatalf("Error reading file: %s\n", err)
		}
		defer compressedReader.Close()

		z, err := zlib.NewReader(compressedReader)
		if err != nil {
			log.Fatalf("Error decompressing file: %s\n", err)
		}
		defer z.Close()

		p, err := io.ReadAll(z)
		if err != nil {
			log.Fatalf("Error reading decompressed reader: %s\n", err)
		}

		parts := strings.Split(string(p), "\x00")

		const sep = " "
		const offset = 26
		spaceParts := strings.SplitN(parts[1], sep, 2)
		fmt.Println(spaceParts[1])

		for _, part := range parts[2 : len(parts)-1] {
			fmt.Println(strings.TrimPrefix(part[offset:], sep))
		}

	case "write-tree":
		writeTree(".", true)

	case "commit-tree":
		if len(os.Args) < 5 {
			log.Fatalln("usage: mygit commit-tree <tree_sha> [-p <parent_sha>] -m <message>")
		}

		tree_sha := os.Args[2]

		containsParents := os.Args[3] == "-p"

		var parent_sha string
		var message string
		if containsParents {
			parent_sha = os.Args[4]
			message = os.Args[6]
		} else {
			message = os.Args[4]
		}

		var buffer bytes.Buffer

		buffer.WriteString(fmt.Sprintf("tree %v\n", tree_sha))

		if containsParents {
			buffer.WriteString(fmt.Sprintf("parent %v\n", parent_sha))
		}

		t := time.Now()
		tname, _ := t.Zone()
		buffer.WriteString(fmt.Sprintf("author My Git <mygit@mygit> %v %v\n", t.Unix(), tname))

		buffer.WriteString(fmt.Sprintf("committer His Git <hisgit@mygit> %v %v\n\n", t.Unix(), tname))

		buffer.WriteString(message + "\n")

		content := buffer.Bytes()

		size := len(content)
		content = append([]byte(fmt.Sprintf("commit %d\x00", size)), content...)

		hash := sha1.Sum(content)
		hexHash := hex.EncodeToString(hash[:])

		fmt.Println(hexHash)

		var compressedBytes bytes.Buffer
		w := zlib.NewWriter(&compressedBytes)
		w.Write(content)
		w.Close()

		writeDir := ".git/objects/" + hexHash[:2]
		if err := os.MkdirAll(writeDir, 0755); err != nil {
			log.Fatalf("Error creating directory: %s\n", err)
		}

		if err := os.WriteFile(writeDir+"/"+hexHash[2:], compressedBytes.Bytes(), 0644); err != nil {
			log.Fatalf("Error writing file: %s\n", err)
		}

	default:
		log.Fatalf("Unknown command %s\n", command)
	}
}
