package engine

import "errors"

var ErrLocalBookParseLimit = errors.New("local book exceeds parser safety limits")

type LocalBookParseLimits struct {
	MaxArchiveBytes         int64
	MaxArchiveEntries       int
	MaxArchiveEntryBytes    int64
	MaxArchiveExpandedBytes int64
	MaxPDFPages             int
	MaxParsedTextBytes      int64
	MaxUMDChapters          int
}

func DefaultLocalBookParseLimits() LocalBookParseLimits {
	return LocalBookParseLimits{
		MaxArchiveBytes:         128 << 20,
		MaxArchiveEntries:       20_000,
		MaxArchiveEntryBytes:    128 << 20,
		MaxArchiveExpandedBytes: 512 << 20,
		MaxPDFPages:             10_000,
		MaxParsedTextBytes:      256 << 20,
		MaxUMDChapters:          100_000,
	}
}

// LegacyLocalBookParseLimits keeps already-imported local archives readable
// during lazy cache/resource recovery. New preview/import requests use the
// stricter configurable limits supplied by the local-book importer instead.
func LegacyLocalBookParseLimits() LocalBookParseLimits {
	limits := DefaultLocalBookParseLimits()
	limits.MaxArchiveBytes = 1 << 30
	limits.MaxArchiveExpandedBytes = 2 << 30
	limits.MaxPDFPages = 50_000
	limits.MaxParsedTextBytes = 2 << 30
	limits.MaxUMDChapters = 1_000_000
	return limits
}

func (limits LocalBookParseLimits) normalized() LocalBookParseLimits {
	defaults := DefaultLocalBookParseLimits()
	if limits.MaxArchiveBytes <= 0 {
		limits.MaxArchiveBytes = defaults.MaxArchiveBytes
	}
	if limits.MaxArchiveEntries <= 0 {
		limits.MaxArchiveEntries = defaults.MaxArchiveEntries
	}
	if limits.MaxArchiveEntryBytes <= 0 {
		limits.MaxArchiveEntryBytes = defaults.MaxArchiveEntryBytes
	}
	if limits.MaxArchiveExpandedBytes <= 0 {
		limits.MaxArchiveExpandedBytes = defaults.MaxArchiveExpandedBytes
	}
	if limits.MaxPDFPages <= 0 {
		limits.MaxPDFPages = defaults.MaxPDFPages
	}
	if limits.MaxParsedTextBytes <= 0 {
		limits.MaxParsedTextBytes = defaults.MaxParsedTextBytes
	}
	if limits.MaxUMDChapters <= 0 {
		limits.MaxUMDChapters = defaults.MaxUMDChapters
	}
	return limits
}
