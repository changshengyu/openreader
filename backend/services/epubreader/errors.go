package epubreader

import "errors"

var (
	ErrMalformedCapability = errors.New("malformed EPUB resource capability")
	ErrInvalidCapability   = errors.New("invalid EPUB resource capability")
	ErrExpiredCapability   = errors.New("expired EPUB resource capability")
	ErrUnsafePath          = errors.New("unsafe EPUB resource path")
	ErrUnsupportedMedia    = errors.New("unsupported EPUB resource media type")
	ErrNotFound            = errors.New("EPUB resource not found")
	ErrInvalidArchive      = errors.New("invalid EPUB archive")
	ErrExtractionLimit     = errors.New("EPUB extraction limit exceeded")
	ErrNotEPUB             = errors.New("book is not a local EPUB")
)
