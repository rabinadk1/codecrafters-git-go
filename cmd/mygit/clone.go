package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func loadAndDecompressObject(gitDir, hash string) []byte {
	path := fmt.Sprintf("%v/.git/objects/%v/%v", gitDir, hash[:2], hash[2:])

	fp, err := os.Open(path)
	if err != nil {
		fmt.Printf("Error opening file: %s\n", err)
		return nil
	}

	z, err := zlib.NewReader(fp)
	if err != nil {
		log.Fatalf("Error decompressing file: %s\n", err)
	}
	defer z.Close()

	p, err := io.ReadAll(z)
	if err != nil {
		log.Fatalf("Error reading decompressed reader: %s\n", err)
	}
	return p
}

func parseTree(treeData []byte, rootDir string, gitDir string) {
	// fmt.Printf("Creating directory: %v\n", rootDir)
	err := os.MkdirAll(rootDir, 0755)
	if err != nil {
		log.Fatalf("Error creating directory: %s\n", err)
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
			subTreeData := loadAndDecompressObject(gitDir, hexHash)

			parseTree(subTreeData, rootDir+"/"+fileName, gitDir)
		} else if fileMode[0] == '1' {
			// file or link
			blobData := loadAndDecompressObject(gitDir, hexHash)

			nullIndex := bytes.IndexByte(blobData, 0)

			fileContent := blobData[nullIndex+1:]

			outputFilePath := rootDir + "/" + fileName

			filePerms, err := strconv.ParseInt(fileMode[len(fileMode)-3:], 8, 0)
			if err != nil {
				log.Fatalf("Error parsing file mode: %s\n", err)
			}

			if err := os.WriteFile(outputFilePath, fileContent, os.FileMode(filePerms)); err != nil {
				log.Fatalf("Error writing file: %s\n", err)
			}
		} else {
			log.Fatalln("Saving is not curretly implemented for symbolic links.")
		}

		treeData = treeData[hashEndIndex:]

	}
}

func hashAndSaveObjects(content []byte, rootDir string) string {
	hash := sha1.Sum(content)
	hexHash := hex.EncodeToString(hash[:])

	fmt.Println(hexHash)

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
	return hexHash
}

const (
	OBJ_COMMIT    = 1
	OBJ_TREE      = 2
	OBJ_BLOB      = 3
	OBJ_TAG       = 4
	OBJ_OFS_DELTA = 6
	OBJ_REF_DELTA = 7
)

func readSizeEncoding(packData []byte, offset uint32) (uint32, uint32) {
	const varIntEncodingBits = 7

	var size uint32
	var shift uint32
	for {
		num := packData[offset]
		offset++

		size |= uint32(num&127) << shift

		if num < 128 {
			return size, offset
		}

		shift += varIntEncodingBits
	}
}

const (
	COPY_OFFSET_BYTES = 4
	COPY_SIZE_BYTES   = 3

	COPY_ZERO_SIZE = 0x10000
)

func readPartialInt(deltaContent []byte, offset *uint32, bytes byte, presentBytes *byte) uint32 {
	var value uint32

	var byteIndex byte
	for ; byteIndex < bytes; byteIndex++ {
		// Use one bit of `present_bytes` to determine if the byte exists
		if *presentBytes&1 != 0 {
			currByte := deltaContent[*offset]
			*offset++

			value |= uint32(currByte) << (byteIndex * 8)
		}
		*presentBytes >>= 1
	}
	return value
}

func applyDelta(deltaContent []byte, baseObjectContent []byte) []byte {
	var offset uint32

	baseSize, offset := readSizeEncoding(deltaContent, offset)
	// fmt.Printf("Offset after baseSize: %v/%v\n", offset, len(deltaContent))

	if int(baseSize) != len(baseObjectContent) {
		log.Fatalf("Incorrect base length. delta: %v, base: %v\n", baseSize, len(baseObjectContent))
	}

	resultSize, offset := readSizeEncoding(deltaContent, offset)

	var result []byte

	for int(offset) < len(deltaContent) {

		// fmt.Printf("Offset in start of loop: %v/%v\n", offset, len(deltaContent))
		instruction := deltaContent[offset]
		offset++
		// fmt.Printf("Offset after instruction: %v/%v, (%b)\n", offset, len(deltaContent), instruction)

		if instruction > 127 {
			// Copy instruction
			deltaOffset := readPartialInt(deltaContent, &offset, COPY_OFFSET_BYTES, &instruction)
			// fmt.Printf("Offset after deltaOffset: %v/%v (%b)\n", offset, len(deltaContent), deltaOffset)

			deltaSize := readPartialInt(deltaContent, &offset, COPY_SIZE_BYTES, &instruction)
			// fmt.Printf("Offset after deltaSize: %v/%v (%b)\n", offset, len(deltaContent), deltaSize)

			if deltaSize == 0 {
				// Copying 0 bytes doesn't make sense, so git assumes a different size
				deltaSize = COPY_ZERO_SIZE
			}

			data := baseObjectContent[deltaOffset : deltaOffset+deltaSize]

			result = append(result, data...)

		} else {
			// Data instruction
			if instruction == 0 {
				log.Fatalf("Invalid data instruction: %v", instruction)
			}

			deltaOffset := uint32(instruction)
			data := deltaContent[offset : offset+deltaOffset]
			offset += deltaOffset

			result = append(result, data...)
		}
	}

	// fmt.Printf("Result: %s\n", result)

	if resultSize != uint32(len(result)) {
		log.Fatalf("Incorrect result length. Read: %v, Computed: %v\n", resultSize, len(result))
	}

	return result
}

func parsePackObject(packData []byte, offset uint32, outputDir string) (byte, []byte, string, uint32) {
	objectTypeMapper := map[byte]string{
		1: "commit",
		2: "tree",
		3: "blob",
		4: "tag",
		6: "ofs_delta",
		7: "ref_delta",
	}

	num := packData[offset]
	offset++

	objType := (num >> 4) & 7
	fmt.Printf("Object Type: %v\n", objectTypeMapper[objType])

	size := uint32(num & 15)
	shift := 4

	// fmt.Printf("num: %d, size: %d (%b)\n", num, size, size)
	for num > 128 {
		num = packData[offset]
		offset++
		size |= uint32(num&127) << shift
		shift += 7
	}
	// fmt.Printf("num: %d, size: %d (%b)\n", num, size, size)

	var baseObjectOffset uint32
	var baseObjectHash string

	switch objType {
	case OBJ_OFS_DELTA:

		var deltaOffset uint32
		for {
			num := packData[offset]
			offset++

			deltaOffset = (deltaOffset << 7) | (uint32(num & 127))

			if num < 128 {
				break
			}
			// Increase the value if there are more bytes, to avoid redundant encodings
			deltaOffset += 1
		}

		fmt.Printf("Delta offset: %d", deltaOffset)

		baseObjectOffset = offset - deltaOffset

		if baseObjectOffset < 12 {
			log.Fatalf("Invalid base object offset: %d", baseObjectOffset)
		}

	case OBJ_REF_DELTA:
		const hashLength = 20
		baseObjectHash = hex.EncodeToString(packData[offset : offset+hashLength])
		offset += hashLength

		fmt.Printf("Referring hash %v\n", baseObjectHash)
	}

	br := bytes.NewReader(packData[offset:])

	// fmt.Printf("Attempting to decompress data starting with bytes: %x\n", packData[offset:offset+2])

	z, err := zlib.NewReader(br)
	if err != nil {
		log.Fatalf("Error decompressing file: %s\n", err)
	}
	defer z.Close()

	objectContent, err := io.ReadAll(z)
	if err != nil {
		log.Fatalf("Error reading decompressed reader: %s\n", err)
	}

	decompressedSize := uint32(len(objectContent))

	if size != decompressedSize {
		log.Fatalf("Decompressed size %d does not match expected size %d", decompressedSize, size)
	}

	var hexHash string
	switch objType {
	case OBJ_BLOB:
		prefix := []byte(fmt.Sprintf("blob %d\x00", size))
		content := append(prefix, objectContent...)

		hexHash = hashAndSaveObjects(content, outputDir)

	case OBJ_TREE:
		prefix := []byte(fmt.Sprintf("tree %d\x00", size))
		content := append(prefix, objectContent...)

		hexHash = hashAndSaveObjects(content, outputDir)

	case OBJ_COMMIT:
		prefix := []byte(fmt.Sprintf("commit %d\x00", size))
		content := append(prefix, objectContent...)

		hexHash = hashAndSaveObjects(content, outputDir)

	case OBJ_OFS_DELTA:
		baseObjType, baseObjectContent, _, _ := parsePackObject(packData, baseObjectOffset, outputDir)

		// fmt.Printf("Base object type: %v\n", baseObjType)
		// fmt.Printf("Base object content: %s\n", baseObjectContent)
		// fmt.Printf("Base Hex Hash: %v\n", baseHexHash)
		// fmt.Printf("Base Remaining: %v\n", newBaseOffset)

		result := applyDelta(objectContent, baseObjectContent)
		// fmt.Printf("Result: %s", result)

		size := len(result)
		prefix := []byte(fmt.Sprintf("%v %d\x00", objectTypeMapper[baseObjType], size))
		fmt.Printf("Prefix: %s", prefix)
		content := append(prefix, objectContent...)

		hexHash = hashAndSaveObjects(content, outputDir)

	case OBJ_REF_DELTA:
		baseObjectContent := loadAndDecompressObject(outputDir, baseObjectHash)

		if baseObjectContent == nil {
			break
		}

		nullIndex := bytes.IndexByte(baseObjectContent, 0)

		objectInfoParts := strings.SplitN(string(baseObjectContent[:nullIndex]), " ", 2)

		fileType, baseSize := objectInfoParts[0], objectInfoParts[1]

		fmt.Printf("Base object type: %v\n", fileType)
		// fmt.Printf("Base object size: %v\n", baseSize)

		baseObjectContent = baseObjectContent[nullIndex+1:]
		// fmt.Printf("Base object content: %s\n", baseObjectContent)
		intBaseSize, err := strconv.Atoi(baseSize)
		if err != nil {
			log.Fatalf("Error parsing base size: %s", err)
		}

		if intBaseSize != len(baseObjectContent) {
			log.Fatalf("Calculated base object size %v does not match read size %v", len(baseObjectContent), baseSize)
		}

		result := applyDelta(objectContent, baseObjectContent)
		// fmt.Printf("Result: %s", result)

		size := len(result)

		prefix := []byte(fmt.Sprintf("%v %d\x00", fileType, size))
		fmt.Printf("Prefix: %s\n", prefix)

		content := append(prefix, objectContent...)

		hash := sha1.Sum(objectContent)
		hexHash := hex.EncodeToString(hash[:])
		fmt.Printf("Raw Hash: %s\n", hexHash)

		hexHash = hashAndSaveObjects(content, outputDir)

	case OBJ_TAG:
		fmt.Println("Tag object not implemented")
	}

	compressedSize := uint32(len(packData[offset:]) - br.Len())

	newOffset := offset + compressedSize

	return objType, objectContent, hexHash, newOffset
}

func myclone() {
	url := strings.TrimSuffix(os.Args[2], "/")
	outputDir := strings.TrimSuffix(os.Args[3], "/")

	fmt.Printf("Downloading from %v to %v...\n", url, outputDir)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Error creating directory: %s\n", err)
	}

	for _, dir := range []string{".git", ".git/objects", ".git/refs"} {
		if err := os.MkdirAll(outputDir+"/"+dir, 0755); err != nil {
			log.Fatalf("Error creating directory: %s\n", err)
		}
	}

	fetchUrl := url + "/info/refs?service=git-upload-pack"
	res, err := http.Get(fetchUrl)
	if err != nil {
		log.Fatalf("Error getting %s: %s\n", fetchUrl, err)
	}

	defer res.Body.Close()
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		log.Fatalf("Error reading response body: %s\n", err)
	}

	parts := strings.Split(string(resBody), "\n")
	lastLine := parts[len(parts)-2]

	lastLineParts := strings.Fields(lastLine)

	firstObjectHash, ref := lastLineParts[0], lastLineParts[1]
	firstObjectHash = firstObjectHash[4:]

	headFileContents := []byte(fmt.Sprintf("ref: %v\n", ref))
	if err := os.WriteFile(outputDir+"/.git/HEAD", headFileContents, 0644); err != nil {
		log.Fatalf("Error writing file: %s\n", err)
	}

	body := fmt.Sprintf("0032want %s\n00000009done\n", firstObjectHash)
	// fmt.Println(body)
	fetchUrl = url + "/git-upload-pack"

	res, err = http.Post(fetchUrl, "application/x-git-upload-pack-request", strings.NewReader(body))
	if err != nil {
		log.Fatalf("Error getting %s: %s\n", fetchUrl, err)
	}

	defer res.Body.Close()
	packData, err := io.ReadAll(res.Body)
	if err != nil {
		log.Fatalf("Error reading response body: %s\n", err)
	}

	checksum := hex.EncodeToString(packData[len(packData)-20:])
	fmt.Printf("Checksum: %s\n", checksum)

	const nackOffset = 8
	if strings.Contains(string(packData[:nackOffset]), "NAK") {
		packData = packData[nackOffset:]
	}

	// Verify pack signature
	if string(packData[:4]) != "PACK" {
		log.Fatalln("Invalid pack file signature")
	}

	// Read version
	version := binary.BigEndian.Uint32(packData[4:8])
	if version != 2 && version != 3 {
		log.Fatalf("unsupported pack version: %d", version)
	}

	// Read number of objects
	numObjects := binary.BigEndian.Uint32(packData[8:12])
	fmt.Printf("Pack contains %d objects\n", numObjects)

	var offset uint32 = 12

	var rootTreeHash string

	for i := 1; i <= int(numObjects); i++ {
		fmt.Printf("\nProcessing object %d/%d at offset: %v\n", i, numObjects, offset)
		objType, _, hexHash, newOffset := parsePackObject(packData, offset, outputDir)
		if objType == OBJ_TREE && rootTreeHash == "" {
			rootTreeHash = hexHash
		}
		offset = newOffset
	}

	treeData := loadAndDecompressObject(outputDir, rootTreeHash)

	// fmt.Println(string(treeData))

	// Save files using the root tree
	parseTree(treeData, outputDir, outputDir)
}
