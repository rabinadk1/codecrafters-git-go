package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

func getCompressedObjectReader(hexHash, gitDir string) (*os.File, error) {
	path := fmt.Sprintf("%v/.git/objects/%v/%v", gitDir, hexHash[:2], hexHash[2:])

	compressedReader, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("Error reading file: %s\n", err)
	}
	return compressedReader, nil
}

func readAllResponse(HttpRequestor func() (*http.Response, error)) ([]byte, error) {
	res, err := HttpRequestor()
	if err != nil {
		return nil, fmt.Errorf("Error making HTTP Request: %s\n", err)
	}

	defer res.Body.Close()
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("Error reading response body: %s\n", err)
	}

	return resBody, nil
}
