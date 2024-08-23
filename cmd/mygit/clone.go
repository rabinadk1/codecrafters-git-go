package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type PackObject struct {
	HexHash string
	Content []byte
	Type    byte
}

const (
	OBJ_COMMIT    = 1
	OBJ_TREE      = 2
	OBJ_BLOB      = 3
	OBJ_TAG       = 4
	OBJ_OFS_DELTA = 6
	OBJ_REF_DELTA = 7
)

const (
	COPY_OFFSET_BYTES = 4
	COPY_SIZE_BYTES   = 3
	COPY_ZERO_SIZE    = 0x10000
)

func readSizeEncoding(packData []byte, offset *uint32) uint32 {
	const varIntEncodingBits = 7

	var size uint32
	var shift uint32
	for {
		num := packData[*offset]
		*offset++

		size |= uint32(num&127) << shift

		if num < 128 {
			return size
		}

		shift += varIntEncodingBits
	}
}

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

func applyDelta(deltaContent []byte, baseObjectContent []byte) ([]byte, error) {
	var offset uint32

	baseSize := readSizeEncoding(deltaContent, &offset)
	// fmt.Printf("Offset after baseSize: %v/%v\n", offset, len(deltaContent))

	if int(baseSize) != len(baseObjectContent) {
		return nil, fmt.Errorf("Incorrect base length. delta: %v, base: %v\n", baseSize, len(baseObjectContent))
	}

	resultSize := readSizeEncoding(deltaContent, &offset)

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
				return nil, fmt.Errorf("Invalid data instruction: %v", instruction)
			}

			deltaOffset := uint32(instruction)
			data := deltaContent[offset : offset+deltaOffset]
			offset += deltaOffset

			result = append(result, data...)
		}
	}

	// fmt.Printf("Result: %s\n", result)

	if resultSize != uint32(len(result)) {
		return nil, fmt.Errorf("Incorrect result length. Read: %v, Computed: %v\n", resultSize, len(result))
	}

	return result, nil
}

func parsePackObject(packData []byte, offset *uint32, outputDir string) (*PackObject, error) {
	objectTypeMapper := map[byte]string{
		1: "commit",
		2: "tree",
		3: "blob",
		4: "tag",
		6: "ofs_delta",
		7: "ref_delta",
	}

	num := packData[*offset]
	*offset++

	objType := (num >> 4) & 7
	fmt.Printf("Object Type: %v\n", objectTypeMapper[objType])

	size := uint32(num & 15)
	shift := 4

	// fmt.Printf("num: %d, size: %d (%b)\n", num, size, size)
	for num > 128 {
		num = packData[*offset]
		*offset++
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
			num := packData[*offset]
			*offset++

			deltaOffset = (deltaOffset << 7) | (uint32(num & 127))

			if num < 128 {
				break
			}
			// Increase the value if there are more bytes, to avoid redundant encodings
			deltaOffset += 1
		}

		fmt.Printf("Delta offset: %d", deltaOffset)

		baseObjectOffset = *offset - deltaOffset

		if baseObjectOffset < 12 {
			return nil, fmt.Errorf("Invalid base object offset: %d", baseObjectOffset)
		}

	case OBJ_REF_DELTA:
		const hashLength = 20
		baseObjectHash = hex.EncodeToString(packData[*offset : *offset+hashLength])
		*offset += hashLength

		fmt.Printf("Referring hash %v\n", baseObjectHash)
	}

	br := bytes.NewReader(packData[*offset:])

	// fmt.Printf("Attempting to decompress data starting with bytes: %x\n", packData[offset:offset+2])

	objectContent, err := getDecompressedObject(br)
	if err != nil {
		return nil, fmt.Errorf("Error getting decompressed object: %s\n", err)
	}

	decompressedSize := uint32(len(objectContent))

	if size != decompressedSize {
		return nil, fmt.Errorf("Decompressed size %d does not match expected size %d", decompressedSize, size)
	}

	var hexHash string
	switch objType {
	case OBJ_BLOB:
		prefix := []byte(fmt.Sprintf("blob %d\x00", size))
		content := append(prefix, objectContent...)

		hexHash, err = hashAndSaveObjects(content, outputDir)
		if err != nil {
			return nil, fmt.Errorf("Error hashing and saving objects: %s\n", err)
		}

	case OBJ_TREE:
		prefix := []byte(fmt.Sprintf("tree %d\x00", size))
		content := append(prefix, objectContent...)

		hexHash, err = hashAndSaveObjects(content, outputDir)
		if err != nil {
			return nil, fmt.Errorf("Error hashing and saving objects: %s\n", err)
		}

	case OBJ_COMMIT:
		prefix := []byte(fmt.Sprintf("commit %d\x00", size))
		content := append(prefix, objectContent...)

		hexHash, err = hashAndSaveObjects(content, outputDir)
		if err != nil {
			return nil, fmt.Errorf("Error hashing and saving objects: %s\n", err)
		}

	case OBJ_OFS_DELTA:
		baseObj, err := parsePackObject(packData, &baseObjectOffset, outputDir)

		// fmt.Printf("Base object type: %v\n", baseObjType)
		// fmt.Printf("Base object content: %s\n", baseObjectContent)
		// fmt.Printf("Base Hex Hash: %v\n", baseHexHash)
		// fmt.Printf("Base Remaining: %v\n", newBaseOffset)

		result, err := applyDelta(objectContent, baseObj.Content)
		if err != nil {
			return nil, fmt.Errorf("Error applying delta: %s\n", err)
		}

		// fmt.Printf("Result: %s", result)

		size := len(result)
		prefix := []byte(fmt.Sprintf("%v %d\x00", objectTypeMapper[baseObj.Type], size))
		fmt.Printf("Prefix: %s", prefix)
		content := append(prefix, objectContent...)

		hexHash, err = hashAndSaveObjects(content, outputDir)
		if err != nil {
			return nil, fmt.Errorf("Error hashing and saving objects: %s\n", err)
		}

	case OBJ_REF_DELTA:
		baseObjectContent, err := loadAndDecompressObject(baseObjectHash, outputDir)
		if err != nil {
			return nil, fmt.Errorf("Error loading base object content: %s\n", err)
		}

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
			return nil, fmt.Errorf("Error parsing base size: %s", err)
		}

		if intBaseSize != len(baseObjectContent) {
			return nil, fmt.Errorf("Calculated base object size %v does not match read size %v", len(baseObjectContent), baseSize)
		}

		result, err := applyDelta(objectContent, baseObjectContent)
		if err != nil {
			return nil, fmt.Errorf("Error applying delta: %s", err)
		}
		// fmt.Printf("Result: %s", result)

		size := len(result)

		prefix := []byte(fmt.Sprintf("%v %d\x00", fileType, size))
		fmt.Printf("Prefix: %s\n", prefix)

		content := append(prefix, objectContent...)

		hexHash, err = hashAndSaveObjects(content, outputDir)
		if err != nil {
			return nil, fmt.Errorf("Error hashing and saving objects: %s\n", err)
		}
	case OBJ_TAG:
		fmt.Println("Tag object not implemented")
	}

	compressedSize := uint32(len(packData[*offset:]) - br.Len())

	*offset += compressedSize

	return &PackObject{
		Content: objectContent, Type: objType, HexHash: hexHash,
	}, nil
}

func myclone(args []string) error {
	url := strings.TrimSuffix(args[2], "/")
	outputDir := strings.TrimSuffix(args[3], "/")

	fmt.Printf("Downloading from %v to %v...\n", url, outputDir)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Error creating directory: %s\n", err)
	}

	resBody, err := readAllResponse(func() (*http.Response, error) {
		fetchUrl := url + "/info/refs?service=git-upload-pack"
		return http.Get(fetchUrl)
	})

	parts := strings.Split(string(resBody), "\n")
	lastLine := parts[len(parts)-2]

	lastLineParts := strings.Fields(lastLine)

	firstObjectHash, ref := lastLineParts[0], lastLineParts[1]
	firstObjectHash = firstObjectHash[4:]

	createGitDirs(outputDir, ref)

	packData, err := readAllResponse(func() (*http.Response, error) {
		body := fmt.Sprintf("0032want %s\n00000009done\n", firstObjectHash)
		fetchUrl := url + "/git-upload-pack"

		return http.Post(fetchUrl, "application/x-git-upload-pack-request", strings.NewReader(body))
	})

	checksumIndex := len(packData) - 20

	checksum := hex.EncodeToString(packData[checksumIndex:])
	fmt.Printf("Checksum: %s\n", checksum)

	// Remove checksum
	packData = packData[:checksumIndex]

	const nackOffset = 8
	if strings.Contains(string(packData[:nackOffset]), "NAK") {
		packData = packData[nackOffset:]
	}

	// Verify pack signature
	if string(packData[:4]) != "PACK" {
		return fmt.Errorf("Invalid pack file signature")
	}

	// Read version
	version := binary.BigEndian.Uint32(packData[4:8])
	if version != 2 && version != 3 {
		return fmt.Errorf("unsupported pack version: %d\n", version)
	}

	// Read number of objects
	numObjects := binary.BigEndian.Uint32(packData[8:12])
	fmt.Printf("Pack contains %d objects\n", numObjects)

	var offset uint32 = 12

	var rootTreeHash string

	for i := 1; i <= int(numObjects); i++ {
		fmt.Printf("\nProcessing object %d/%d at offset: %v\n", i, numObjects, offset)
		packObj, err := parsePackObject(packData, &offset, outputDir)
		if err != nil {
			return fmt.Errorf("Error parsing pack object: %s", err)
		}

		if packObj.Type == OBJ_TREE && rootTreeHash == "" {
			rootTreeHash = packObj.HexHash
		}
	}

	treeData, err := loadAndDecompressObject(rootTreeHash, outputDir)
	if err != nil {
		return fmt.Errorf("Error loading tree data: %s\n", err)
	}

	// fmt.Println(string(treeData))

	// Save files using the root tree
	err = parseTree(treeData, outputDir, outputDir)
	if err != nil {
		return fmt.Errorf("Error parsing tree: %s\n", err)
	}

	return nil
}
