package controllers

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"image"
	_ "image/jpeg" // registers JPEG for image.DecodeConfig
	_ "image/png"  // registers PNG
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxImageSide   = 5000 // pixels
	productImgPath = "/uploads/clothes/"
)

// productImageTypes: what the file really is (by its first bytes) → our extension.
var productImageTypes = map[string]string{"image/jpeg": ".jpg", "image/png": ".png"}

// UploadDir is where uploaded files live: UPLOAD_DIR, or "uploads" next to the app.
// In Docker it is a volume, so photos survive a new container.
func UploadDir() string {
	if dir := os.Getenv("UPLOAD_DIR"); dir != "" {
		return dir
	}
	return "uploads"
}

// saveProductImage checks an uploaded photo and stores it under a random name.
// Returns the URL to put into clothes.image_url.
func saveProductImage(fh *multipart.FileHeader) (string, error) {
	data, err := readUpload(fh) // ≤ 5 MB, the same reader as helpdesk attachments
	if err != nil {
		return "", err
	}
	ext, ok := productImageTypes[http.DetectContentType(data)]
	if !ok {
		return "", errors.New("можно только JPG или PNG")
	}

	// DecodeConfig reads only the header with the width and height. A 1 KB file can
	// claim to be 100000×100000 pixels: decoding it fully would need ~40 GB of memory
	// (a "decompression bomb"), so such sizes are refused before anything is decoded.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", errors.New("файл повреждён или это не картинка")
	}
	if cfg.Width > maxImageSide || cfg.Height > maxImageSide {
		return "", errors.New("слишком большое разрешение: не больше 5000×5000")
	}

	// A random name: nobody can overwrite another file, guess names, or sneak "../" in.
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	name := hex.EncodeToString(b) + ext

	dir := filepath.Join(UploadDir(), "clothes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		return "", err
	}
	return productImgPath + name, nil
}

// removeProductImage deletes a photo we stored earlier. URLs that point anywhere else
// (an external link typed by the admin) are left alone. filepath.Base keeps
// "/uploads/clothes/../../main.go" from reaching outside the folder.
func removeProductImage(url string) {
	if !strings.HasPrefix(url, productImgPath) {
		return
	}
	_ = os.Remove(filepath.Join(UploadDir(), "clothes", filepath.Base(url)))
}
