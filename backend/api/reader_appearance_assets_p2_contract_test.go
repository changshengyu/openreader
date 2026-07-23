package api

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"openreader/backend/models"
)

func TestReaderAppearanceUploadRejectsSpoofedAndMismatchedContentWithoutWriting(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		filename string
		data     []byte
	}{
		{name: "html disguised as png", kind: "background", filename: "paper.png", data: []byte("<html><script>alert(1)</script></html>")},
		{name: "png disguised as jpeg", kind: "cover", filename: "cover.jpg", data: readerAppearancePNG(t, 1, 1)},
		{name: "truncated png", kind: "background", filename: "paper.png", data: []byte("\x89PNG\r\n\x1a\n")},
		{name: "random ttf", kind: "font", filename: "reader.ttf", data: []byte("not-a-font")},
		{name: "empty woff2", kind: "font", filename: "reader.woff2", data: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			token := authHeader(t, router)
			response := uploadReaderAppearanceAsset(t, router, token, tt.kind, tt.filename, tt.data)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "file content does not match type") {
				t.Fatalf("expected content mismatch 400, got %d: %s", response.Code, response.Body.String())
			}
			assertNoReaderAppearanceAssetFiles(t, server)
		})
	}
}

func TestReaderAppearanceUploadAcceptsValidatedImageAndFontSignatures(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		filename string
		data     []byte
	}{
		{name: "png background", kind: "background", filename: "paper.png", data: readerAppearancePNG(t, 2, 2)},
		{name: "jpeg cover", kind: "cover", filename: "cover.jpg", data: readerAppearanceJPEG(t)},
		{name: "gif background", kind: "background", filename: "paper.gif", data: readerAppearanceGIF(t)},
		{name: "webp background", kind: "background", filename: "paper.webp", data: readerAppearanceWebP()},
		{name: "ttf font", kind: "font", filename: "reader.ttf", data: readerAppearanceFont("ttf")},
		{name: "otf font", kind: "font", filename: "reader.otf", data: readerAppearanceFont("otf")},
		{name: "woff font", kind: "font", filename: "reader.woff", data: readerAppearanceFont("woff")},
		{name: "woff2 font", kind: "font", filename: "reader.woff2", data: readerAppearanceFont("woff2")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _ := setupTestServer(t)
			token := authHeader(t, router)
			response := uploadReaderAppearanceAsset(t, router, token, tt.kind, tt.filename, tt.data)
			if response.Code != http.StatusCreated {
				t.Fatalf("expected validated asset 201, got %d: %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestReaderAppearanceUploadRejectsUnsafeImageDimensions(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	response := uploadReaderAppearanceAsset(
		t,
		router,
		token,
		"background",
		"oversized.png",
		readerAppearancePNGHeader(100_000, 100_000),
	)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "image dimensions are too large") {
		t.Fatalf("expected oversized image 400, got %d: %s", response.Code, response.Body.String())
	}
	assertNoReaderAppearanceAssetFiles(t, server)
}

func uploadReaderAppearanceAsset(t *testing.T, router http.Handler, token, kind, filename string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("type", kind); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/uploads", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func readerAppearancePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var data bytes.Buffer
	picture := image.NewNRGBA(image.Rect(0, 0, width, height))
	picture.Set(0, 0, color.NRGBA{R: 0x80, G: 0x60, B: 0x40, A: 0xff})
	if err := png.Encode(&data, picture); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func readerAppearanceJPEG(t *testing.T) []byte {
	t.Helper()
	var data bytes.Buffer
	picture := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	picture.Set(0, 0, color.NRGBA{R: 0x80, G: 0x60, B: 0x40, A: 0xff})
	if err := jpeg.Encode(&data, picture, nil); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func readerAppearanceGIF(t *testing.T) []byte {
	t.Helper()
	var data bytes.Buffer
	picture := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
	if err := gif.Encode(&data, picture, nil); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func readerAppearanceWebP() []byte {
	data := make([]byte, 26)
	copy(data[0:4], "RIFF")
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
	copy(data[8:12], "WEBP")
	copy(data[12:16], "VP8L")
	binary.LittleEndian.PutUint32(data[16:20], 5)
	data[20] = 0x2f
	return data
}

func readerAppearancePNGHeader(width, height uint32) []byte {
	var data bytes.Buffer
	data.Write([]byte("\x89PNG\r\n\x1a\n"))
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8] = 8
	ihdr[9] = 6
	writeReaderAppearancePNGChunk(&data, "IHDR", ihdr)
	writeReaderAppearancePNGChunk(&data, "IDAT", nil)
	writeReaderAppearancePNGChunk(&data, "IEND", nil)
	return data.Bytes()
}

func writeReaderAppearancePNGChunk(target *bytes.Buffer, kind string, payload []byte) {
	_ = binary.Write(target, binary.BigEndian, uint32(len(payload)))
	target.WriteString(kind)
	target.Write(payload)
	checksum := crc32.NewIEEE()
	_, _ = checksum.Write([]byte(kind))
	_, _ = checksum.Write(payload)
	_ = binary.Write(target, binary.BigEndian, checksum.Sum32())
}

func readerAppearanceFont(kind string) []byte {
	header := make([]byte, 16)
	switch kind {
	case "ttf":
		copy(header, []byte{0x00, 0x01, 0x00, 0x00})
	case "otf":
		copy(header, "OTTO")
	case "woff":
		copy(header, "wOFF")
	case "woff2":
		copy(header, "wOF2")
	}
	return header
}

func assertNoReaderAppearanceAssetFiles(t *testing.T, server *Server) {
	t.Helper()
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(server.cfg.DataDir, "uploads", "users", strconv.FormatUint(uint64(user.ID), 10))
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			t.Fatalf("rejected upload left file %s", path)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
