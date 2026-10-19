package impact

import (
	"errors"
	"fmt"
	"os"
)

func openGitConfig(path string, expected os.FileInfo) (*os.File, error) {
	if expected == nil || !expected.Mode().IsRegular() {
		return nil, fmt.Errorf("impact: Git configuration must be a regular file")
	}
	file, err := openGitConfigFile(path)
	if err != nil {
		return nil, err
	}
	actual, err := file.Stat()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("impact: inspect opened Git configuration: %w", err), file.Close())
	}
	if !actual.Mode().IsRegular() || !os.SameFile(expected, actual) {
		return nil, errors.Join(fmt.Errorf("impact: Git configuration changed during inspection"), file.Close())
	}
	return file, nil
}
