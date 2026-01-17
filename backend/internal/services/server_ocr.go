package services

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ServerOCRService provides server-side OCR processing using Tesseract
type ServerOCRService struct {
	tesseractPath string
	language      string
}

// ServerOCRResult contains the result of server-side OCR processing
type ServerOCRResult struct {
	Text       string   `json:"text"`
	Lines      []string `json:"lines"`
	Confidence float64  `json:"confidence"`
	Error      string   `json:"error,omitempty"`
}

// NewServerOCRService creates a new server OCR service
func NewServerOCRService() *ServerOCRService {
	// Find tesseract in PATH
	tesseractPath, err := exec.LookPath("tesseract")
	if err != nil {
		tesseractPath = "tesseract" // Will fail at runtime if not found
	}

	return &ServerOCRService{
		tesseractPath: tesseractPath,
		language:      "eng", // English by default
	}
}

// IsAvailable checks if Tesseract is available on the system
func (s *ServerOCRService) IsAvailable() bool {
	cmd := exec.Command(s.tesseractPath, "--version")
	err := cmd.Run()
	return err == nil
}

// ProcessImage processes an image file and returns OCR text.
// The image path is validated and sanitized to prevent path traversal attacks.
func (s *ServerOCRService) ProcessImage(imagePath string) (*ServerOCRResult, error) {
	// Sanitize and validate the image path to prevent command injection
	cleanPath, err := s.validateImagePath(imagePath)
	if err != nil {
		return &ServerOCRResult{
			Error: fmt.Sprintf("invalid image path: %v", err),
		}, err
	}

	// Run tesseract with custom config for card text
	// Use PSM 6 (Assume a single uniform block of text) or PSM 3 (Fully automatic page segmentation)
	cmd := exec.Command(
		s.tesseractPath,
		cleanPath,
		"stdout", // Output to stdout
		"-l", s.language,
		"--psm", "3", // Fully automatic page segmentation
		"--oem", "3", // Default OCR Engine Mode (LSTM + Legacy)
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		return &ServerOCRResult{
			Error: fmt.Sprintf("tesseract error: %v - %s", err, stderr.String()),
		}, err
	}

	text := stdout.String()
	lines := splitAndCleanLines(text)

	return &ServerOCRResult{
		Text:       text,
		Lines:      lines,
		Confidence: estimateConfidence(lines),
	}, nil
}

// ProcessImageBytes processes image data directly without saving to file
func (s *ServerOCRService) ProcessImageBytes(imageData []byte) (*ServerOCRResult, error) {
	// Verify this is a valid image
	_, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return &ServerOCRResult{
			Error: fmt.Sprintf("invalid image data: %v", err),
		}, err
	}

	// Write to stdin using process substitution
	cmd := exec.Command(
		s.tesseractPath,
		"stdin", // Read from stdin
		"stdout",
		"-l", s.language,
		"--psm", "3",
		"--oem", "3",
	)

	cmd.Stdin = bytes.NewReader(imageData)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		return &ServerOCRResult{
			Error: fmt.Sprintf("tesseract error: %v - %s", err, stderr.String()),
		}, err
	}

	text := stdout.String()
	lines := splitAndCleanLines(text)

	return &ServerOCRResult{
		Text:       text,
		Lines:      lines,
		Confidence: estimateConfidence(lines),
	}, nil
}

// ProcessBase64Image processes a base64-encoded image
func (s *ServerOCRService) ProcessBase64Image(base64Data string) (*ServerOCRResult, error) {
	// Remove data URL prefix if present
	if idx := strings.Index(base64Data, ","); idx != -1 {
		base64Data = base64Data[idx+1:]
	}

	imageData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return &ServerOCRResult{
			Error: fmt.Sprintf("invalid base64 data: %v", err),
		}, err
	}

	return s.ProcessImageBytes(imageData)
}

// splitAndCleanLines splits text into lines and removes empty/whitespace lines
func splitAndCleanLines(text string) []string {
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}

// estimateConfidence estimates OCR confidence based on extracted text quality
func estimateConfidence(lines []string) float64 {
	if len(lines) == 0 {
		return 0.0
	}

	totalChars := 0
	alphanumericChars := 0

	for _, line := range lines {
		for _, c := range line {
			totalChars++
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == ' ' {
				alphanumericChars++
			}
		}
	}

	if totalChars == 0 {
		return 0.0
	}

	// Higher ratio of alphanumeric characters indicates cleaner OCR
	ratio := float64(alphanumericChars) / float64(totalChars)

	// Scale confidence based on ratio and number of lines
	confidence := ratio * 0.8
	if len(lines) >= 3 {
		confidence += 0.2
	} else if len(lines) >= 1 {
		confidence += 0.1
	}

	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// validateImagePath validates and sanitizes an image path to prevent command injection
// and path traversal attacks.
func (s *ServerOCRService) validateImagePath(imagePath string) (string, error) {
	// Clean the path to remove any ".." or other traversal attempts
	cleanPath := filepath.Clean(imagePath)

	// Convert to absolute path
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	// Verify the file exists and is a regular file (not a directory, symlink, etc.)
	info, err := os.Lstat(absPath)
	if err != nil {
		return "", fmt.Errorf("file not found: %w", err)
	}

	// Only allow regular files, not symlinks (to prevent symlink attacks)
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("symbolic links are not allowed")
	}

	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("path is not a regular file")
	}

	// Verify it's likely an image file by checking extension
	ext := strings.ToLower(filepath.Ext(absPath))
	allowedExtensions := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
		".bmp":  true,
		".tiff": true,
		".tif":  true,
		".webp": true,
	}
	if !allowedExtensions[ext] {
		return "", fmt.Errorf("unsupported image format: %s", ext)
	}

	return absPath, nil
}
