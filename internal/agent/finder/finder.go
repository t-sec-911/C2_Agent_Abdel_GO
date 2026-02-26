package finder

import (
	"io/fs"
	"path/filepath"
	"time"
)

type SearchCriteria struct {
	Pattern       string
	Extensions    []string
	MinSize       int64
	MaxSize       int64
	ModifiedAfter time.Time
	MaxDepth      int
	RootPaths     []string // Where to search
}
type FileInfo struct {
	Path        string
	Name        string
	Size        int64
	ModTime     time.Time
	IsDir       bool
	Permissions fs.FileMode
}
type Finder struct {
	maxFileSize int64
}

func NewFinder(maxFileSize int64) *Finder {
	return &Finder{
		maxFileSize: maxFileSize,
	}
}

// Finder func
func (f *Finder) Search(criteria SearchCriteria) ([]FileInfo, error) {
	var results []FileInfo

	for _, root := range criteria.RootPaths {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip errors, continue searching
			}

			// Check depth limit
			depth := getDepth(root, path)
			if criteria.MaxDepth > 0 && depth > criteria.MaxDepth {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if d.IsDir() {
				return nil
			}

			// Match pattern
			if criteria.Pattern != "" && criteria.Pattern != "*" {
				matched, _ := filepath.Match(criteria.Pattern, d.Name())
				if !matched {
					return nil
				}
			}

			// Match extension
			if len(criteria.Extensions) > 0 {
				ext := filepath.Ext(d.Name())
				if !contains(criteria.Extensions, ext) {
					return nil
				}
			}

			info, err := d.Info()
			if err != nil {
				return nil
			}

			// Size filters
			if criteria.MinSize > 0 && info.Size() < criteria.MinSize {
				return nil
			}
			if criteria.MaxSize > 0 && info.Size() > criteria.MaxSize {
				return nil
			}
			if info.Size() > f.maxFileSize {
				return nil // skip files that are too large
			}

			// Date filter
			if !criteria.ModifiedAfter.IsZero() && info.ModTime().Before(criteria.ModifiedAfter) {
				return nil
			}
			if len(results) >= 10000 {
				return filepath.SkipAll
			}

			results = append(results, FileInfo{
				Path:        path,
				Name:        d.Name(),
				Size:        info.Size(),
				ModTime:     info.ModTime(),
				IsDir:       false,
				Permissions: info.Mode(),
			})

			return nil
		})

		if err != nil {
			return results, err
		}
	}

	return results, nil
}

func getDepth(root, path string) int {
	rel, _ := filepath.Rel(root, path)
	if rel == "." {
		return 0
	}
	return len(filepath.SplitList(rel))
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
