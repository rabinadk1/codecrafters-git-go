package main

import (
	"compress/zlib"
	"io"
	"log"
)

func loadAndDecompressObject(hexHash, gitDir string) []byte {
	compressedReader := getCompressedObjectReader(hexHash, gitDir)

	p := getDecompressedObject(compressedReader)
	compressedReader.Close()

	return p
}

func getDecompressedObject(compressedReader io.Reader) []byte {
	z, err := zlib.NewReader(compressedReader)
	if err != nil {
		log.Fatalf("Error decompressing file: %s\n", err)
	}

	defer z.Close()

	uncompressedObject, err := io.ReadAll(z)
	if err != nil {
		log.Fatalf("Error reading decompressed reader: %s\n", err)
	}

	return uncompressedObject
}
