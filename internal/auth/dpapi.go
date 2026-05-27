// Package auth stores and retrieves the Berget API key using Windows
// DPAPI (CryptProtectData / CryptUnprotectData). The encrypted blob
// is bound to both the current user and the current machine — it
// cannot be decrypted by another user, nor copied to another machine.
package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// dataBlob mirrors DATA_BLOB from wincrypt.h: a length-and-pointer pair.
type dataBlob struct {
	Size uint32
	Data *byte
}

var (
	crypt32                = syscall.NewLazyDLL("crypt32.dll")
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procCryptProtectData   = crypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
	procLocalFree          = kernel32.NewProc("LocalFree")
)

// KeyPath returns the absolute path where the encrypted API key
// lives: %LOCALAPPDATA%\Prata\apikey.dat
func KeyPath() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Prata", "apikey.dat")
}

// SaveAPIKey encrypts key with DPAPI and writes it to disk, creating
// the destination directory if needed.
func SaveAPIKey(key string) error {
	if key == "" {
		return fmt.Errorf("api key is empty")
	}

	encrypted, err := encrypt([]byte(key))
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}

	path := KeyPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	if err := os.WriteFile(path, encrypted, 0o600); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

// LoadAPIKey reads and decrypts the API key previously saved by
// SaveAPIKey. Returns an error if the file doesn't exist or the blob
// can't be decrypted under the current user on the current machine.
func LoadAPIKey() (string, error) {
	path := KeyPath()
	encrypted, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	if len(encrypted) == 0 {
		return "", fmt.Errorf("%s is empty", path)
	}

	plaintext, err := decrypt(encrypted)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}

func encrypt(plaintext []byte) ([]byte, error) {
	in := dataBlob{
		Size: uint32(len(plaintext)),
		Data: &plaintext[0],
	}
	var out dataBlob

	ret, _, sysErr := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, // szDataDescr — no description
		0, // pOptionalEntropy — no extra entropy
		0, // pvReserved
		0, // pPromptStruct — never prompt
		0, // dwFlags
		uintptr(unsafe.Pointer(&out)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("CryptProtectData: %v", sysErr)
	}

	result := blobToBytes(&out)
	freeBlob(&out)
	return result, nil
}

func decrypt(ciphertext []byte) ([]byte, error) {
	in := dataBlob{
		Size: uint32(len(ciphertext)),
		Data: &ciphertext[0],
	}
	var out dataBlob

	ret, _, sysErr := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&out)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("CryptUnprotectData: %v", sysErr)
	}

	result := blobToBytes(&out)
	freeBlob(&out)
	return result, nil
}

// blobToBytes copies the contents of a DATA_BLOB into a Go-owned slice
// so we can safely free the OS-allocated buffer afterwards.
func blobToBytes(b *dataBlob) []byte {
	if b.Size == 0 || b.Data == nil {
		return nil
	}
	out := make([]byte, b.Size)
	src := unsafe.Slice(b.Data, b.Size)
	copy(out, src)
	return out
}

// freeBlob releases memory allocated by Crypt(Un)ProtectData via the
// Windows LocalFree allocator.
func freeBlob(b *dataBlob) {
	if b.Data == nil {
		return
	}
	procLocalFree.Call(uintptr(unsafe.Pointer(b.Data)))
	b.Data = nil
	b.Size = 0
}
