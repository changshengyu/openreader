package epubreader

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"path/filepath"

	"openreader/backend/engine"
	"openreader/backend/models"
	"openreader/backend/services/rootedfs"
)

// PrepareImportedResources uses the importer's original archive session. It
// never re-admits a pathname or delegates ownership to a runtime cache rebuild.
func (s *Service) PrepareImportedResources(book models.Book, tree *rootedfs.OwnedTree) error {
	if !IsLocalEPUB(book) {
		return ErrNotEPUB
	}
	source, err := tree.Open(filepath.Base(book.OriginalFile))
	if err != nil {
		return err
	}
	defer source.Close()
	info := source.Info()
	if info.Size() < 0 || info.Size() > s.limits.MaxArchiveBytes {
		return ErrExtractionLimit
	}
	hash := sha256.New()
	count, err := io.Copy(hash, io.LimitReader(source, info.Size()+1))
	if err != nil {
		return err
	}
	if count != info.Size() {
		return ErrInvalidArchive
	}
	fingerprint := hex.EncodeToString(hash.Sum(nil))
	prefix := filepath.Join(extractionDirectoryName, fingerprint)
	if err := tree.Mkdir(prefix); err != nil {
		return err
	}
	reader, err := zip.NewReader(source, info.Size())
	if err != nil {
		return ErrInvalidArchive
	}
	if err := extractZipReaderOwned(reader, tree, prefix, s.limits); err != nil {
		return err
	}
	coverPath, err := engine.EPUBCoverImagePathFromZip(reader, engine.DefaultLocalBookParseLimits())
	if err != nil {
		coverPath = ""
	}
	marker, err := json.Marshal(extractionMarker{
		Fingerprint: fingerprint, Size: info.Size(), ModTimeNano: info.ModTime().UnixNano(),
		CoverPath: coverPath, CoverChecked: true,
	})
	if err != nil {
		return err
	}
	marker = append(marker, '\n')
	if _, err := tree.ReplaceOwned(filepath.Join(prefix, extractionMarkerName), bytes.NewReader(marker), int64(len(marker))); err != nil {
		return err
	}
	if err := tree.ChmodOwned(filepath.Join(prefix, extractionMarkerName), 0o600); err != nil {
		return err
	}
	return tree.Validate()
}
