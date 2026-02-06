package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

var dataPath string

func main() {
	logFilePath := filepath.Join(dataPath, "debug.log")

	// Wait for log file to exist
	for {
		if _, err := os.Stat(logFilePath); err == nil {
			break
		}
		log.Printf("Awaiting log file creation at %s", logFilePath)
		time.Sleep(5 * time.Second)
	}

	logFile, err := os.Open(logFilePath)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()

	// Seek to end of file
	if _, err := logFile.Seek(0, os.SEEK_END); err != nil {
		log.Fatalf("Failed to seek log file: %v", err)
	}

	scanner := bufio.NewScanner(logFile)

	for {
		// Read new lines
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}

		if err := scanner.Err(); err != nil {
			log.Printf("Scanner error: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		// Wait before checking for more lines
		time.Sleep(2 * time.Second)
	}
}
