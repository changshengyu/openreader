package api

import (
	"strings"

	"openreader/backend/engine"
)

func (s *Server) projectCoverResource(userID, sourceID uint, rawURL string) *string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" ||
		(!strings.HasPrefix(rawURL, "http://") &&
			!strings.HasPrefix(rawURL, "https://") &&
			!strings.HasPrefix(rawURL, "//")) {
		return nil
	}
	projected, err := s.coverImages.Project(userID, sourceID, rawURL)
	if err != nil {
		projected = ""
	}
	return &projected
}

func (s *Server) projectSearchResultCovers(userID uint, values []engine.SearchResult) []engine.SearchResult {
	projected := append([]engine.SearchResult(nil), values...)
	for index := range projected {
		projected[index].CoverResourceURL = s.projectCoverResource(
			userID,
			projected[index].SourceID,
			projected[index].CoverURL,
		)
	}
	return projected
}
