package audioreader

import "errors"

var (
	ErrMalformedCapability = errors.New("malformed audio resource capability")
	ErrInvalidCapability   = errors.New("invalid audio resource capability")
	ErrExpiredCapability   = errors.New("expired audio resource capability")
	ErrUnsafePath          = errors.New("unsafe audio resource path")
	ErrNotFound            = errors.New("audio resource not found")
	ErrUnsupportedMedia    = errors.New("unsupported audio media type")
	ErrNotAudio            = errors.New("not an audio book")
)
