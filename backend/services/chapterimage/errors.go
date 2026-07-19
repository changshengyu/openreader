package chapterimage

import "errors"

var (
	ErrInvalidInput        = errors.New("invalid chapter image input")
	ErrUnsafeURL           = errors.New("unsafe chapter image URL")
	ErrImageLimit          = errors.New("chapter image limit exceeded")
	ErrUnsupportedImage    = errors.New("unsupported chapter image")
	ErrNotFound            = errors.New("chapter image not found")
	ErrUnsafePath          = errors.New("unsafe chapter image path")
	ErrMalformedCapability = errors.New("malformed chapter image capability")
	ErrInvalidCapability   = errors.New("invalid chapter image capability")
	ErrExpiredCapability   = errors.New("expired chapter image capability")
)
