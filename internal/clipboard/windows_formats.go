package clipboard

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/png"
	"strconv"
	"strings"

	"golang.org/x/image/bmp"
)

func buildHTMLClipboard(fragment []byte) []byte {
	body := append([]byte("<html><body><!--StartFragment-->"), fragment...)
	body = append(body, []byte("<!--EndFragment--></body></html>")...)
	const headerTemplate = "Version:0.9\r\nStartHTML:%010d\r\nEndHTML:%010d\r\nStartFragment:%010d\r\nEndFragment:%010d\r\n"
	headerLength := len(fmt.Sprintf(headerTemplate, 0, 0, 0, 0))
	startFragmentInBody := bytes.Index(body, []byte("<!--StartFragment-->")) + len("<!--StartFragment-->")
	endFragmentInBody := bytes.Index(body, []byte("<!--EndFragment-->"))
	header := fmt.Sprintf(headerTemplate,
		headerLength,
		headerLength+len(body),
		headerLength+startFragmentInBody,
		headerLength+endFragmentInBody,
	)
	return append([]byte(header), append(body, 0)...)
}

func extractHTMLFragment(data []byte) []byte {
	data = bytes.TrimRight(data, "\x00")
	start, startOK := htmlOffset(data, "StartFragment:")
	end, endOK := htmlOffset(data, "EndFragment:")
	if startOK && endOK && start >= 0 && end >= start && end <= len(data) {
		return append([]byte(nil), data[start:end]...)
	}
	startMarker := []byte("<!--StartFragment-->")
	endMarker := []byte("<!--EndFragment-->")
	start = bytes.Index(data, startMarker)
	end = bytes.Index(data, endMarker)
	if start >= 0 && end >= start {
		start += len(startMarker)
		return append([]byte(nil), data[start:end]...)
	}
	return append([]byte(nil), data...)
}

func htmlOffset(data []byte, key string) (int, bool) {
	index := bytes.Index(data, []byte(key))
	if index < 0 {
		return 0, false
	}
	value := data[index+len(key):]
	if newline := bytes.IndexAny(value, "\r\n"); newline >= 0 {
		value = value[:newline]
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(string(value)))
	return parsed, err == nil
}

func pngToDIB(data []byte) ([]byte, error) {
	image, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode PNG: %w", err)
	}
	var encoded bytes.Buffer
	if err := bmp.Encode(&encoded, image); err != nil {
		return nil, fmt.Errorf("encode DIB: %w", err)
	}
	if encoded.Len() < 14 {
		return nil, fmt.Errorf("encoded bitmap is too short")
	}
	return append([]byte(nil), encoded.Bytes()[14:]...), nil
}

func dibToPNG(dib []byte) ([]byte, error) {
	if len(dib) < 40 {
		return nil, fmt.Errorf("DIB is too short")
	}
	headerSize := int(binary.LittleEndian.Uint32(dib[0:4]))
	if headerSize < 12 || headerSize > len(dib) {
		return nil, fmt.Errorf("invalid DIB header size %d", headerSize)
	}
	bitCount := int(binary.LittleEndian.Uint16(dib[14:16]))
	compression := binary.LittleEndian.Uint32(dib[16:20])
	colors := 0
	if headerSize >= 36 {
		colors = int(binary.LittleEndian.Uint32(dib[32:36]))
	}
	if colors == 0 && bitCount > 0 && bitCount <= 8 {
		colors = 1 << bitCount
	}
	maskBytes := 0
	if compression == 3 && headerSize == 40 {
		maskBytes = 12
	}
	pixelOffset := 14 + headerSize + maskBytes + colors*4
	fileSize := 14 + len(dib)
	bmpData := make([]byte, fileSize)
	copy(bmpData[:2], "BM")
	binary.LittleEndian.PutUint32(bmpData[2:6], uint32(fileSize))
	binary.LittleEndian.PutUint32(bmpData[10:14], uint32(pixelOffset))
	copy(bmpData[14:], dib)

	image, err := bmp.Decode(bytes.NewReader(bmpData))
	if err != nil {
		return nil, fmt.Errorf("decode DIB: %w", err)
	}
	var output bytes.Buffer
	if err := png.Encode(&output, image); err != nil {
		return nil, fmt.Errorf("encode PNG: %w", err)
	}
	return output.Bytes(), nil
}
