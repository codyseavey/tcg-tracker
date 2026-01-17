package services

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
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
	// Decode the image
	img, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return &ServerOCRResult{
			Error: fmt.Sprintf("invalid image data: %v", err),
		}, err
	}

	// Preprocess the image for better OCR results
	processedData, err := preprocessImageForOCR(img)
	if err != nil {
		// Fall back to original image if preprocessing fails
		processedData = imageData
	}

	// Try multiple OCR configurations and pick the best result
	configs := []struct {
		psm  string
		desc string
	}{
		{"6", "single block"}, // PSM 6: Assume single uniform block of text
		{"3", "automatic"},    // PSM 3: Fully automatic page segmentation
		{"11", "sparse text"}, // PSM 11: Sparse text without OSD
	}

	var bestResult *ServerOCRResult
	bestScore := 0.0

	// Try with preprocessed image first
	imagesToTry := [][]byte{processedData}
	if !bytes.Equal(processedData, imageData) {
		// Also try original image if preprocessing changed it
		imagesToTry = append(imagesToTry, imageData)
	}

	for _, imgData := range imagesToTry {
		for _, cfg := range configs {
			cmd := exec.Command(
				s.tesseractPath,
				"stdin",
				"stdout",
				"-l", s.language,
				"--psm", cfg.psm,
				"--oem", "3",
			)

			cmd.Stdin = bytes.NewReader(imgData)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err = cmd.Run()
			if err != nil {
				continue // Try next configuration
			}

			text := stdout.String()
			lines := splitAndCleanLines(text)
			confidence := estimateConfidence(lines)

			// Score includes confidence and line count (more lines = more info extracted)
			// Also bonus for finding card-like patterns
			score := confidence + float64(len(lines))*0.05
			if hasCardPatterns(lines) {
				score += 0.3
			}

			if bestResult == nil || score > bestScore {
				bestScore = score
				bestResult = &ServerOCRResult{
					Text:       text,
					Lines:      lines,
					Confidence: confidence,
				}
			}
		}
	}

	if bestResult == nil {
		return &ServerOCRResult{
			Error: "all OCR configurations failed",
		}, fmt.Errorf("all OCR configurations failed")
	}

	return bestResult, nil
}

// preprocessImageForOCR applies image preprocessing to improve OCR quality
// - Converts to grayscale
// - Enhances contrast
// - Returns PNG-encoded bytes
func preprocessImageForOCR(img image.Image) ([]byte, error) {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()

	// Create grayscale image with enhanced contrast
	gray := image.NewGray(bounds)

	// Convert to grayscale and collect histogram
	histogram := make([]int, 256)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			// Use luminosity formula for grayscale conversion
			lum := uint8((0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 256)
			gray.SetGray(x, y, color.Gray{Y: lum})
			histogram[lum]++
		}
	}

	// Find min/max for contrast stretching (ignore bottom/top 1%)
	total := width * height
	threshold := total / 100
	minVal, maxVal := 0, 255

	count := 0
	for i := 0; i < 256; i++ {
		count += histogram[i]
		if count >= threshold {
			minVal = i
			break
		}
	}

	count = 0
	for i := 255; i >= 0; i-- {
		count += histogram[i]
		if count >= threshold {
			maxVal = i
			break
		}
	}

	// Apply contrast stretching
	if maxVal > minVal {
		scale := 255.0 / float64(maxVal-minVal)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				oldVal := gray.GrayAt(x, y).Y
				newVal := int(float64(int(oldVal)-minVal) * scale)
				if newVal < 0 {
					newVal = 0
				}
				if newVal > 255 {
					newVal = 255
				}
				gray.SetGray(x, y, color.Gray{Y: uint8(newVal)})
			}
		}
	}

	// Encode as PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, gray); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// hasCardPatterns checks if OCR output contains patterns typical of trading cards
func hasCardPatterns(lines []string) bool {
	for _, line := range lines {
		upper := strings.ToUpper(line)
		// Check for HP pattern (Pokemon)
		if strings.Contains(upper, "HP") {
			return true
		}
		// Check for card number pattern (XXX/YYY)
		if strings.Contains(line, "/") {
			for i, c := range line {
				if c == '/' && i > 0 && i < len(line)-1 {
					// Check if there are digits around the slash
					if line[i-1] >= '0' && line[i-1] <= '9' && line[i+1] >= '0' && line[i+1] <= '9' {
						return true
					}
				}
			}
		}
		// Check for common MTG types
		if strings.Contains(upper, "CREATURE") || strings.Contains(upper, "INSTANT") ||
			strings.Contains(upper, "SORCERY") || strings.Contains(upper, "ARTIFACT") ||
			strings.Contains(upper, "ENCHANTMENT") {
			return true
		}
	}
	return false
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
