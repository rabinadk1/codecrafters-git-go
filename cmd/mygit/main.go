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
		createGitDirs(".", "refs/heads/main")

		fmt.Println("Initialized git directory")

	case "cat-file":
		if len(os.Args) != 4 {
			log.Fatalln("usage: mygit cat-file <type> <object>")
		}

		hexHash := os.Args[3]

		p := loadAndDecompressObject(hexHash, ".")

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

		writeBlob(filepath, writeObject, true)

	case "ls-tree":
		if len(os.Args) != 4 {
			log.Fatalln("usage: mygit ls-tree --name-only <tree_sha>")
		}

		hexHash := os.Args[3]

		p := loadAndDecompressObject(hexHash, ".")

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

		_, hexHash := getHexHash(content)

		fmt.Println(hexHash)

		writeCompressedObject(content, hexHash, ".")

	case "clone":
		if len(os.Args) < 4 {
			log.Fatalln("usage: mygit clone <url> <some_dir>")
		}

		myclone()

	default:
		log.Fatalf("Unknown command %s\n", command)
	}
}
