package serverauth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const passwordStoreVersion = 1

type encryptedPasswordStore struct {
	path string
}

type encryptedPasswordFile struct {
	Version    int    `json:"version"`
	KDF        string `json:"kdf"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func NewEncryptedPasswordStore(path string) PasswordStore {
	return encryptedPasswordStore{path: path}
}

func (s encryptedPasswordStore) LoadOrInitialize(initialPassword string) (string, error) {
	raw, err := os.ReadFile(s.path)
	if err == nil {
		return s.decrypt(raw)
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	password := strings.TrimSpace(initialPassword)
	if password == "" {
		return "", fmt.Errorf("initial password is required")
	}
	if err := s.Save(password); err != nil {
		return "", err
	}
	return password, nil
}

func (s encryptedPasswordStore) Save(password string) error {
	password = strings.TrimSpace(password)
	if password == "" {
		return fmt.Errorf("password is required")
	}
	block, err := aes.NewCipher(devicePasswordKey())
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	payload := encryptedPasswordFile{
		Version:    passwordStoreVersion,
		KDF:        "sha256-device-v1",
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, []byte(password), nil)),
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0o600)
}

func (s encryptedPasswordStore) decrypt(raw []byte) (string, error) {
	var payload encryptedPasswordFile
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	if payload.Version != passwordStoreVersion {
		return "", fmt.Errorf("unsupported password store version %d", payload.Version)
	}
	nonce, err := base64.StdEncoding.DecodeString(payload.Nonce)
	if err != nil {
		return "", err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(payload.Ciphertext)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(devicePasswordKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	password := strings.TrimSpace(string(plaintext))
	if password == "" {
		return "", fmt.Errorf("stored password is empty")
	}
	return password, nil
}

func devicePasswordKey() []byte {
	sum := sha256.Sum256([]byte("ai-gate server admin password\n" + deviceFingerprint()))
	return sum[:]
}

func deviceFingerprint() string {
	parts := []string{runtime.GOOS, runtime.GOARCH}
	if hostname, err := os.Hostname(); err == nil {
		parts = append(parts, strings.TrimSpace(hostname))
	}
	for _, path := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if raw, err := os.ReadFile(path); err == nil {
			if value := strings.TrimSpace(string(raw)); value != "" {
				parts = append(parts, value)
				break
			}
		}
	}
	return strings.Join(parts, "|")
}
