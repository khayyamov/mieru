package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	aesKey := os.Getenv("AES_KEY")
	if aesKey == "" {
		fmt.Println("Error: AES_KEY environment variable not set.")
		return
	}

	content := fmt.Sprintf(`package constant

var (
	ENCRYPTED_CONFIG = true
	ENCRYPT_KEY      = "%s"
	ENCRYPT_KEY_IV      = ""
)
`, aesKey)

	filePath := "./constant/constants.go"
	file, err := os.Create(filePath)
	if err != nil {
		fmt.Println("Error creating file:", err)
		return
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			panic("error writing to file")
		}
	}(file)

	_, err = io.WriteString(file, content)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}

	fmt.Println("File written successfully:", filePath)
}
