package api

import (
	"context"
	"errors"
	"os"
	"sync"

	assetservice "openreader/backend/services/assets"
)

type userAssetGate struct {
	token chan struct{}
	refs  int
}

func (s *Server) lockUserAssets(ctx context.Context, userID uint) (func(), error) {
	if userID == 0 {
		return nil, os.ErrPermission
	}
	s.assetLocksMu.Lock()
	if s.assetLocks == nil {
		s.assetLocks = make(map[uint]*userAssetGate)
	}
	gate := s.assetLocks[userID]
	if gate == nil {
		gate = &userAssetGate{token: make(chan struct{}, 1)}
		s.assetLocks[userID] = gate
	}
	gate.refs++
	s.assetLocksMu.Unlock()

	select {
	case gate.token <- struct{}{}:
	case <-ctx.Done():
		s.releaseUserAssetGate(userID, gate, false)
		return nil, ctx.Err()
	}
	var once sync.Once
	return func() {
		once.Do(func() { s.releaseUserAssetGate(userID, gate, true) })
	}, nil
}

func (s *Server) releaseUserAssetGate(userID uint, gate *userAssetGate, locked bool) {
	if locked {
		<-gate.token
	}
	s.assetLocksMu.Lock()
	gate.refs--
	if gate.refs == 0 && s.assetLocks[userID] == gate {
		delete(s.assetLocks, userID)
	}
	s.assetLocksMu.Unlock()
}

func (s *Server) openUserUploadAsset(asset userUploadAsset) (assetservice.OpenedFile, error) {
	if s.assetStore == nil {
		s.assetStore = assetservice.NewStore(s.cfg.DataDir)
	}
	return s.assetStore.Open(asset.UserID, asset.Kind, asset.Name)
}

func assetMissing(err error) bool {
	return errors.Is(err, assetservice.ErrNotFound) || errors.Is(err, os.ErrNotExist)
}
