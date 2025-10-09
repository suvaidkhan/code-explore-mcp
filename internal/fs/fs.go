package fs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type FileFilter struct {
	workSpace string
	supported map[string]bool
}

func NewFileFilter(workSpace string, supported []string) *FileFilter {
	extMap := make(map[string]bool, len(supported))
	for _, val := range supported {
		extMap[val] = true
	}

	return &FileFilter{
		workSpace: workSpace,
		supported: extMap,
	}
}

func (f *FileFilter) ShouldIgnore(path string) bool {
	if f.IsGitIgnored(path) {
		return true
	}

	p := strings.ToLower(filepath.Ext(path))
	if p == "" {
		return false
	}
	return !f.supported[p]
}

func WalkSourceFiles(workspace string, supported []string, callback func(filePath string) error) error {
	filter := NewFileFilter(workspace, supported)

	return filepath.Walk(workspace, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if filter.ShouldIgnore(path) {
			if info.IsDir() {
				return filepath.SkipDir
			}

			return nil
		}

		relPath, err := filepath.Rel(workspace, path)
		if err != nil {
			relPath = path
		}

		return callback(relPath)
	})
}

func (f *FileFilter) IsGitIgnored(path string) bool {
	name := filepath.Base(path)
	if name == ".git" {
		return true
	}
	cmd := exec.Command("git", "check-ignore", path)
	cmd.Dir = f.workSpace
	return cmd.Run() == nil
}
