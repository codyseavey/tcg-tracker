package services

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/codyseavey/tcg-tracker/backend/internal/metrics"
)

const (
	// Google Cloud Translation API v3 endpoint
	translationAPIURL = "https://translation.googleapis.com/v3/projects/%s/locations/global:translateText"

	// Default timeout for translation requests
	translationTimeout = 10 * time.Second
)

// TranslationService handles Google Cloud Translation API calls
type TranslationService struct {
	projectID   string
	accessToken string
	tokenExpiry time.Time
	httpClient  *http.Client
	credentials *googleCredentials
	privateKey  *rsa.PrivateKey
	enabled     bool
	mu          sync.Mutex // Protects token refresh
}

// googleCredentials represents a Google Cloud service account JSON key
type googleCredentials struct {
	Type                    string `json:"type"`
	ProjectID               string `json:"project_id"`
	PrivateKeyID            string `json:"private_key_id"`
	PrivateKey              string `json:"private_key"`
	ClientEmail             string `json:"client_email"`
	ClientID                string `json:"client_id"`
	AuthURI                 string `json:"auth_uri"`
	TokenURI                string `json:"token_uri"`
	AuthProviderX509CertURL string `json:"auth_provider_x509_cert_url"`
	ClientX509CertURL       string `json:"client_x509_cert_url"`
}

// translateRequest is the request body for Google Cloud Translation API v3
type translateRequest struct {
	SourceLanguageCode string   `json:"sourceLanguageCode,omitempty"`
	TargetLanguageCode string   `json:"targetLanguageCode"`
	Contents           []string `json:"contents"`
	MimeType           string   `json:"mimeType"`
}

// translateResponse is the response from Google Cloud Translation API v3
type translateResponse struct {
	Translations []struct {
		TranslatedText       string `json:"translatedText"`
		DetectedLanguageCode string `json:"detectedLanguageCode,omitempty"`
	} `json:"translations"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

// tokenResponse is the response from Google OAuth2 token endpoint
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
	Error       string `json:"error,omitempty"`
	ErrorDesc   string `json:"error_description,omitempty"`
}

// NewTranslationService creates a new translation service.
// It auto-enables if GOOGLE_APPLICATION_CREDENTIALS points to a valid file.
func NewTranslationService() *TranslationService {
	svc := &TranslationService{
		httpClient: &http.Client{Timeout: translationTimeout},
		enabled:    false,
	}

	// Check for credentials file
	credPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if credPath == "" {
		log.Println("Translation service: GOOGLE_APPLICATION_CREDENTIALS not set, translation API disabled")
		return svc
	}

	// Expand ~ to home directory
	if strings.HasPrefix(credPath, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			credPath = strings.Replace(credPath, "~", home, 1)
		}
	}

	// Read and parse credentials
	data, err := os.ReadFile(credPath)
	if err != nil {
		log.Printf("Translation service: failed to read credentials file %s: %v", credPath, err)
		return svc
	}

	var creds googleCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		log.Printf("Translation service: failed to parse credentials: %v", err)
		return svc
	}

	if creds.ProjectID == "" || creds.PrivateKey == "" || creds.ClientEmail == "" {
		log.Println("Translation service: credentials file missing required fields")
		return svc
	}

	// Parse the private key
	block, _ := pem.Decode([]byte(creds.PrivateKey))
	if block == nil {
		log.Println("Translation service: failed to decode PEM block from private key")
		return svc
	}

	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS1 format
		privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			log.Printf("Translation service: failed to parse private key: %v", err)
			return svc
		}
	}

	rsaKey, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		log.Println("Translation service: private key is not RSA")
		return svc
	}

	svc.credentials = &creds
	svc.privateKey = rsaKey
	svc.projectID = creds.ProjectID
	svc.enabled = true

	log.Printf("Translation service: enabled for project %s", svc.projectID)
	return svc
}

// IsEnabled returns whether the translation service is available
func (s *TranslationService) IsEnabled() bool {
	return s.enabled
}

// Translate translates text from source language to target language.
// If sourceLang is empty, the API will auto-detect the source language.
func (s *TranslationService) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, error) {
	if !s.enabled {
		return text, fmt.Errorf("translation service not enabled")
	}

	if text == "" {
		return "", nil
	}

	startTime := time.Now()

	// Ensure we have a valid access token
	if err := s.ensureAccessToken(ctx); err != nil {
		metrics.TranslationErrorsTotal.WithLabelValues("auth").Inc()
		return text, fmt.Errorf("failed to get access token: %w", err)
	}

	// Build request
	reqBody := translateRequest{
		TargetLanguageCode: targetLang,
		Contents:           []string{text},
		MimeType:           "text/plain",
	}
	if sourceLang != "" {
		reqBody.SourceLanguageCode = sourceLang
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return text, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make API request
	url := fmt.Sprintf(translationAPIURL, s.projectID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqJSON))
	if err != nil {
		return text, fmt.Errorf("failed to create request: %w", err)
	}

	s.mu.Lock()
	token := s.accessToken
	s.mu.Unlock()

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		metrics.TranslationErrorsTotal.WithLabelValues("api").Inc()
		return text, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Record latency
	metrics.TranslationAPILatency.Observe(time.Since(startTime).Seconds())

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		metrics.TranslationErrorsTotal.WithLabelValues("api").Inc()
		return text, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for HTTP error
	if resp.StatusCode != http.StatusOK {
		metrics.TranslationErrorsTotal.WithLabelValues("api").Inc()
		return text, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result translateResponse
	if err := json.Unmarshal(body, &result); err != nil {
		metrics.TranslationErrorsTotal.WithLabelValues("api").Inc()
		return text, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for API error
	if result.Error != nil {
		metrics.TranslationErrorsTotal.WithLabelValues("api").Inc()
		return text, fmt.Errorf("API error %d: %s", result.Error.Code, result.Error.Message)
	}

	if len(result.Translations) == 0 {
		return text, fmt.Errorf("no translations returned")
	}

	metrics.TranslationRequestsTotal.WithLabelValues("api").Inc()
	return result.Translations[0].TranslatedText, nil
}

// ensureAccessToken gets or refreshes the OAuth2 access token using the service account
func (s *TranslationService) ensureAccessToken(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if we have a valid token (with 1 minute buffer)
	if s.accessToken != "" && time.Now().Add(time.Minute).Before(s.tokenExpiry) {
		return nil
	}

	// Create JWT for service account authentication
	jwt, err := s.createJWT()
	if err != nil {
		return fmt.Errorf("failed to create JWT: %w", err)
	}

	// Exchange JWT for access token
	data := "grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&assertion=" + jwt
	req, err := http.NewRequestWithContext(ctx, "POST", s.credentials.TokenURI, strings.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read token response: %w", err)
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.Error != "" {
		return fmt.Errorf("token error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	s.accessToken = tokenResp.AccessToken
	s.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return nil
}

// createJWT creates a signed JWT for service account authentication
func (s *TranslationService) createJWT() (string, error) {
	// JWT Header
	header := map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	}
	headerJSON, _ := json.Marshal(header)
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)

	// JWT Claims
	now := time.Now().Unix()
	claims := map[string]interface{}{
		"iss":   s.credentials.ClientEmail,
		"sub":   s.credentials.ClientEmail,
		"aud":   s.credentials.TokenURI,
		"iat":   now,
		"exp":   now + 3600, // 1 hour
		"scope": "https://www.googleapis.com/auth/cloud-translation",
	}
	claimsJSON, _ := json.Marshal(claims)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	// Sign with private key using RS256 (RSASSA-PKCS1-v1_5 with SHA-256)
	signInput := headerB64 + "." + claimsB64
	hash := sha256.Sum256([]byte(signInput))
	signature, err := rsa.SignPKCS1v15(nil, s.privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	signatureB64 := base64.RawURLEncoding.EncodeToString(signature)
	return signInput + "." + signatureB64, nil
}
