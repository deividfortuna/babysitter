package logbook

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
)

func ReadTail(path string, n int) ([]Record, error) {
	var records []Record
	for _, file := range []string{path + BackupSuffix, path} {
		read, err := readFile(file)
		if err != nil {
			return nil, err
		}
		records = append(records, read...)
	}
	if n > 0 && len(records) > n {
		records = records[len(records)-n:]
	}
	return records, nil
}

func readFile(path string) ([]Record, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var records []Record
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for scanner.Scan() {
		var r Record
		if json.Unmarshal(scanner.Bytes(), &r) == nil {
			records = append(records, r)
		}
	}
	return records, scanner.Err()
}
