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
)

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

		content, err := os.ReadFile(filepath)
		if err != nil {
			log.Fatalf("Error opening file: %s\n", err)
		}

		size := len(content)

		prefix := []byte(fmt.Sprintf("blob %v\x00", size))

		content = append(prefix, content...)

		h := sha1.New()
		if _, err := h.Write(content); err != nil {
			log.Fatalf("Error hashing file: %s\n", err)
		}

		hash := hex.EncodeToString(h.Sum(nil))

		fmt.Println(hash)

		if writeObject {
			var compressedBytes bytes.Buffer
			w := zlib.NewWriter(&compressedBytes)
			w.Write(content)
			w.Close()

			writeDir := ".git/objects/" + hash[:2]
			if err := os.Mkdir(writeDir, 0755); err != nil {
				log.Fatalf("Error creating directory: %s\n", err)
			}

			if err := os.WriteFile(writeDir+"/"+hash[2:], compressedBytes.Bytes(), 0644); err != nil {
				log.Fatalf("Error writing file: %s\n", err)
			}
		}

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

		for _, part := range parts[1 : len(parts)-1] {
			spaceParts := strings.Split(part, " ")
			fmt.Println(spaceParts[len(spaceParts)-1])
		}

	default:
		log.Fatalf("Unknown command %s\n", command)
	}
}
