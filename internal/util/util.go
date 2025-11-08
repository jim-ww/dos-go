package util

import (
	"bufio"
	"os"
	"strings"
)

func ReadFileEntries(filePath string) (entries []string, e error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			entries = append(entries, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}
