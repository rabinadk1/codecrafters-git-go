package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalln("usage: mygit <command> [<args>...]")
	}

	switch command := os.Args[1]; command {
	case "init":
		err := createGitDirs(".", "refs/heads/main")
		if err != nil {
			log.Fatalln("Error creating git dirs: ", err)
		}

		fmt.Println("Initialized git directory")

	case "cat-file":
		if len(os.Args) != 4 {
			log.Fatalln("usage: mygit cat-file <type> <object>")
		}

		hexHash := os.Args[3]

		p, err := loadAndDecompressObject(hexHash, ".")
		if err != nil {
			log.Fatalln("Error loading and decompressing object: ", err)
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

		_, err := writeBlob(filepath, writeObject, true)
		if err != nil {
			log.Fatalln("Error writing blob: ", err)
		}

	case "ls-tree":
		if len(os.Args) != 4 {
			log.Fatalln("usage: mygit ls-tree --name-only <tree_sha>")
		}

		hexHash := os.Args[3]

		p, err := loadAndDecompressObject(hexHash, ".")
		if err != nil {
			log.Fatalln("Error loading and decompressing object: ", err)
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
		_, err := writeTree(".", true)
		if err != nil {
			log.Fatalln("Error writing tree: ", err)
		}

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

		_, hexHash := getHexHash(content)

		fmt.Println(hexHash)

		err := writeCompressedObject(content, hexHash, ".")
		if err != nil {
			log.Fatalln("Error writing compressed object: ", err)
		}

	case "clone":
		if len(os.Args) < 4 {
			log.Fatalln("usage: mygit clone <url> <some_dir>")
		}

		err := myclone(os.Args)
		if err != nil {
			log.Fatalln("Error cloning: ", err)
		}

	default:
		log.Fatalf("Unknown command %s\n", command)
	}
}
