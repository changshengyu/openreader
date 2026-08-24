package api

import (
	"fmt"
	"io"
	"math"
	"os"

	"openreader/backend/engine"
)

// readBoundedLocalBookSource applies the documented legacy input ceiling
// before a complete archived source is allocated for explicit refresh or lazy
// cache reconstruction. The caller has already resolved the path below the
// authenticated book's archive root; errors deliberately contain no path.
func readBoundedLocalBookSource(path string, maxBytes int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	return readBoundedOpenedLocalBookSource(file, info, maxBytes)
}

func readBoundedOpenedLocalBookSource(file *os.File, info os.FileInfo, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("%w: invalid local source input limit", engine.ErrLocalBookParseLimit)
	}
	if file == nil || info == nil {
		return nil, os.ErrInvalid
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("local source is not a regular file")
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("%w: local source input exceeds the limit", engine.ErrLocalBookParseLimit)
	}
	readLimit := maxBytes
	if maxBytes < math.MaxInt64 {
		readLimit++
	}
	data, err := io.ReadAll(io.LimitReader(file, readLimit))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%w: local source input exceeds the limit", engine.ErrLocalBookParseLimit)
	}
	return data, nil
}
