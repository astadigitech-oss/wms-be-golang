package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

const envFile = ".env"
const secretKeyVar = "JWT_SECRET_KEY"

func InitEnvSecretKey() string {
	// Coba baca file .env
	content, err := os.ReadFile(envFile)
	if err != nil {
		// Jika file belum ada, buat baru
		fmt.Println(".env belum ada, membuat baru...")
		content = []byte("")
	}

	lines := strings.Split(string(content), "\n")

	// Cek apakah sudah ada JWT_SECRET_KEY
	for _, line := range lines {
		if strings.HasPrefix(line, secretKeyVar+"=") {
			// Sudah ada, ambil nilainya
			existingKey := strings.TrimPrefix(line, secretKeyVar+"=")
			existingKey = strings.TrimSpace(existingKey)
			fmt.Println("✅ Secret key sudah ada di .env")
			return existingKey
		}
	}

	// Jika belum ada, generate baru
	newKey := generateSecretKey()
	newLine := fmt.Sprintf("%s=%s\n", secretKeyVar, newKey)

	// Tambahkan ke file .env
	f, err := os.OpenFile(envFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	if _, err := f.WriteString(newLine); err != nil {
		panic(err)
	}

	fmt.Println("🔐 Secret key baru berhasil dibuat dan disimpan di .env")

	return newKey
}

func generateSecretKey() string {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(key)
}

func main() {
	jwtSecret := InitEnvSecretKey()
	fmt.Println("JWT Secret aktif:", jwtSecret)
}