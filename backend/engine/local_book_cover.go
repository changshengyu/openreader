package engine

import "archive/zip"

// EPUBCoverImagePath reports the archive path of an EPUB's cover image.
func EPUBCoverImagePath(data []byte, limits LocalBookParseLimits) (string, error) {
	return "", nil
}

// EPUBCoverImagePathFromZip is EPUBCoverImagePath for an opened archive.
func EPUBCoverImagePathFromZip(reader *zip.Reader, limits LocalBookParseLimits) (string, error) {
	return "", nil
}

// UMDCoverImage returns the cover image stored in a reader-dev UMD.
func UMDCoverImage(data []byte, maxBytes int64) (string, []byte, bool) {
	return "", nil, false
}

// LocalBookCoverImage returns a parsed local book's cover image.
func LocalBookCoverImage(extension string, data []byte, parsed ParsedBook, limits LocalBookParseLimits, maxBytes int64) (string, []byte, bool) {
	return "", nil, false
}
