package coverimage

import "errors"

var (
	ErrMalformedCapability = errors.New("malformed cover capability")
	ErrInvalidCapability   = errors.New("invalid cover capability")
	ErrExpiredCapability   = errors.New("expired cover capability")
	ErrUnsafeURL           = errors.New("unsafe cover URL")
	ErrUnsafePath          = errors.New("unsafe cover cache path")
	ErrUnavailable         = errors.New("cover unavailable")
	ErrCacheLimit          = errors.New("cover cache limit exceeded")
)
