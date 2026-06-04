package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"golang.org/x/crypto/argon2"
)

func main() {
	password := "Admin1234!"

	// Generate random salt
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		fmt.Printf("Error generating salt: %v\n", err)
		return
	}

	// Argon2id parameters (must match backend)
	// m=65536 (64MB), t=3 iterations, p=4 parallelism, keyLen=32
	hash := argon2.IDKey([]byte(password), salt, 3, 65536, 4, 32)

	// Format: $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
	hashStr := fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=4$%s$%s",
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash))

	fmt.Println("Argon2id hash for 'Admin1234!':")
	fmt.Println(hashStr)
}
