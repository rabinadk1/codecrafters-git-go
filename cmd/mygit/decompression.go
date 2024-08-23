package main

import (
	"compress/zlib"
	"fmt"
	"io"
)

func getDecompressedObject(compressedReader io.Reader) ([]byte, error) {
	z, err := zlib.NewReader(compressedReader)
	if err != nil {
		return nil, fmt.Errorf("Error decompressing file: %s\n", err)
	}
	defer z.Close()

	uncompressedObject, err := io.ReadAll(z)
	if err != nil {
		return nil, fmt.Errorf("Error reading decompressed reader: %s\n", err)
	}

	return uncompressedObject, nil
}

func loadAndDecompressObject(hexHash, gitDir string) ([]byte, error) {
	compressedReader, err := getCompressedObjectReader(hexHash, gitDir)
	if err != nil {
		return nil, fmt.Errorf("Error loading object: %s\n", err)
	}
	defer compressedReader.Close()

	decompressedObj, err := getDecompressedObject(compressedReader)
	if err != nil {
		return nil, fmt.Errorf("Error decompressing object: %s\n", err)
	}

	return decompressedObj, nil
}
